package closureenv

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

// BinaryBuild records a binary produced by this collector, not an independently
// supplied source/binary pair. Data is never serialized. MaterialSHA256 covers
// selected dependency/test/native/embed inputs and module manifests, before and
// after the fixed compiler invocation. SourceSHA256 keys are package basenames.
type BinaryBuild struct {
	Package           string            `json:"package"`
	GoVersion         string            `json:"goVersion"`
	GOOS              string            `json:"goos"`
	GOARCH            string            `json:"goarch"`
	GOFLAGS           string            `json:"goflags"`
	GOEXPERIMENT      string            `json:"goexperiment"`
	ArchitectureLevel string            `json:"architectureLevel"`
	CGOEnabled        string            `json:"cgoEnabled"`
	MaterialSHA256    string            `json:"materialSHA256"`
	ToolchainSHA256   string            `json:"toolchainSHA256"`
	BinarySHA256      string            `json:"binarySHA256"`
	SourceSHA256      map[string]string `json:"sourceSHA256"`
	Data              []byte            `json:"-"`
}

// CollectBinary uses the same exact package/module/toolchain selection as
// CollectPackage, with a fixed test-binary compile (no test execution). This
// bounded mode rejects workspaces, replacements, overlays, cgo and custom
// compiler/linker flags; it never executes a build command from an artifact.
func CollectBinary(ctx context.Context, request *PackageRequest, requiredSDKPackages ...string) (*BinaryBuild, error) {
	if request == nil || request.Pattern == "" || strings.HasPrefix(request.Pattern, "-") {
		return nil, errors.New("missing exact package pattern")
	}
	goBinary := request.GoBinary
	if goBinary == "" {
		goBinary = "go"
	}
	resolved, err := exec.LookPath(goBinary)
	if err != nil {
		return nil, err
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return nil, err
	}
	environment := append(os.Environ(), request.Env...)
	if request.GOOS != "" {
		environment = append(environment, "GOOS="+request.GOOS)
	}
	if request.GOARCH != "" {
		environment = append(environment, "GOARCH="+request.GOARCH)
	}
	run := func(args ...string) ([]byte, error) {
		c := exec.CommandContext(ctx, resolved, args...)
		c.Dir = request.Dir
		c.Env = environment
		out, err := c.CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("selected compiler %s: %w: %s", args[0], err, out)
		}
		return out, nil
	}
	out, err := run("env", "-json", "GOROOT", "GOTOOLDIR", "GOVERSION", "GOOS", "GOARCH", "GOFLAGS", "GOEXPERIMENT", "GOARM64", "CGO_ENABLED", "GOWORK")
	if err != nil {
		return nil, err
	}
	var selected map[string]string
	if err := json.Unmarshal(out, &selected); err != nil {
		return nil, err
	}
	if workspace := selected["GOWORK"]; selected["GOOS"] != "darwin" || selected["GOARCH"] != "arm64" || selected["CGO_ENABLED"] != "0" || (workspace != "" && workspace != "off") {
		return nil, errors.New("binary evidence requires Darwin ARM64, CGO_ENABLED=0 and no workspace")
	}
	flags := strings.Fields(selected["GOFLAGS"])
	for _, flag := range flags {
		if !strings.HasPrefix(flag, "-tags=") {
			return nil, errors.New("binary evidence supports only -tags= in GOFLAGS")
		}
	}
	if len(request.Tags) != 0 {
		return nil, errors.New("select build tags through the same GOFLAGS environment used for scanning")
	}
	// Pin the actual module-selected SDK rather than allowing a second
	// automatic toolchain selection during compilation.
	suffix := ""
	if runtime.GOOS == "windows" {
		suffix = ".exe"
	}
	resolved = filepath.Join(selected["GOROOT"], "bin", "go"+suffix)
	tools := []string{resolved, filepath.Join(selected["GOTOOLDIR"], "compile"+suffix), filepath.Join(selected["GOTOOLDIR"], "asm"+suffix), filepath.Join(selected["GOTOOLDIR"], "link"+suffix)}
	toolchain, err := binaryFileDigest(tools)
	if err != nil {
		return nil, err
	}
	environment = append(environment, "GOROOT="+selected["GOROOT"], "GOTOOLCHAIN=local", "GOWORK=off", "GOPROXY=off", "GOSUMDB=off")
	out, err = run("list", "-mod=readonly", "-json", request.Pattern)
	if err != nil {
		return nil, err
	}
	var target listedBinaryPackage
	d := json.NewDecoder(bytes.NewReader(out))
	if err := d.Decode(&target); err != nil {
		return nil, err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF || target.ImportPath == "" || target.Dir == "" {
		return nil, errors.New("binary collection requires exactly one package")
	}
	list := func() ([]byte, error) {
		return run("list", "-mod=readonly", "-deps", "-test", "-json", request.Pattern)
	}
	out, err = list()
	if err != nil {
		return nil, err
	}
	if err := binarySDKPackages(out, selected["GOROOT"], requiredSDKPackages); err != nil {
		return nil, err
	}
	material, err := binaryMaterialDigest(out, target.ImportPath)
	if err != nil {
		return nil, err
	}
	sources := make(map[string]string, len(target.GoFiles))
	for _, name := range target.GoFiles {
		data, err := os.ReadFile(filepath.Join(target.Dir, name))
		if err != nil {
			return nil, err
		}
		digest := sha256.Sum256(data)
		sources[name] = hex.EncodeToString(digest[:])
	}
	if len(sources) == 0 {
		return nil, errors.New("no compiled source inventory")
	}
	directory, err := os.MkdirTemp("", "perfscan-codegen-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(directory)
	binaryPath := filepath.Join(directory, "evidence.test")
	if _, err := run("test", "-mod=readonly", "-buildvcs=false", "-c", "-trimpath", "-o", binaryPath, request.Pattern); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(binaryPath)
	if err != nil {
		return nil, err
	}
	out, err = list()
	if err != nil {
		return nil, err
	}
	if err := binarySDKPackages(out, selected["GOROOT"], requiredSDKPackages); err != nil {
		return nil, err
	}
	after, err := binaryMaterialDigest(out, target.ImportPath)
	if err != nil || after != material {
		return nil, errors.New("binary build materials changed during collection")
	}
	currentToolchain, err := binaryFileDigest(tools)
	if err != nil || currentToolchain != toolchain {
		return nil, errors.New("compiler tools changed during collection")
	}
	digest := sha256.Sum256(data)
	return &BinaryBuild{Package: target.ImportPath, GoVersion: selected["GOVERSION"], GOOS: selected["GOOS"], GOARCH: selected["GOARCH"], GOFLAGS: selected["GOFLAGS"], GOEXPERIMENT: selected["GOEXPERIMENT"], ArchitectureLevel: selected["GOARM64"], CGOEnabled: selected["CGO_ENABLED"], MaterialSHA256: hex.EncodeToString(material[:]), ToolchainSHA256: hex.EncodeToString(toolchain[:]), BinarySHA256: hex.EncodeToString(digest[:]), SourceSHA256: sources, Data: data}, nil
}

type listedBinaryPackage struct {
	Standard, Goroot                                                                                                                                       bool
	ImportPath                                                                                                                                             string
	Dir                                                                                                                                                    string
	GoFiles, CgoFiles, TestGoFiles, XTestGoFiles, SFiles, CFiles, CXXFiles, MFiles, HFiles, FFiles, SysoFiles, EmbedFiles, TestEmbedFiles, XTestEmbedFiles []string
	Module                                                                                                                                                 *struct {
		Dir, GoMod string
		Replace    *struct{ Path string }
	}
}

// The import path alone is insufficient: an older SDK permits a main module
// named simd to impersonate the newer simd/archsimd standard package.
func binarySDKPackages(output []byte, root string, required []string) error {
	packages := make(map[string]listedBinaryPackage)
	decoder := json.NewDecoder(bytes.NewReader(output))
	for decoder.More() {
		var pkg listedBinaryPackage
		if err := decoder.Decode(&pkg); err != nil {
			return err
		}
		packages[pkg.ImportPath] = pkg
	}
	for _, path := range required {
		pkg, ok := packages[path]
		if !ok || !pkg.Standard || !pkg.Goroot || pkg.Module != nil || filepath.Clean(pkg.Dir) != filepath.Join(root, "src", filepath.FromSlash(path)) {
			return fmt.Errorf("required package %s is not provided by the selected SDK", path)
		}
	}
	return nil
}

func binaryMaterialDigest(output []byte, targetPackage ...string) ([32]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(output))
	inputs := map[string]bool{}
	for decoder.More() {
		var pkg listedBinaryPackage
		if err := decoder.Decode(&pkg); err != nil {
			return [32]byte{}, err
		}
		if len(pkg.CgoFiles) != 0 || pkg.Module != nil && pkg.Module.Replace != nil {
			return [32]byte{}, errors.New("cgo or replacement materials unsupported")
		}
		for _, name := range slices.Concat(pkg.GoFiles, pkg.TestGoFiles, pkg.XTestGoFiles, pkg.SFiles, pkg.CFiles, pkg.CXXFiles, pkg.MFiles, pkg.HFiles, pkg.FFiles, pkg.SysoFiles, pkg.EmbedFiles, pkg.TestEmbedFiles, pkg.XTestEmbedFiles) {
			path := name
			if !filepath.IsAbs(path) {
				path = filepath.Join(pkg.Dir, path)
			}
			inputs[path] = true
		}
		if pkg.Module != nil && pkg.Module.GoMod != "" {
			inputs[pkg.Module.GoMod] = true
			sum := filepath.Join(pkg.Module.Dir, "go.sum")
			if _, err := os.Stat(sum); err == nil {
				inputs[sum] = true
			}
		}
	}
	paths := make([]string, 0, len(inputs))
	for path := range inputs {
		paths = append(paths, path)
	}
	slices.Sort(paths)
	if len(targetPackage) != 0 {
		for _, path := range paths {
			data, err := os.ReadFile(path)
			if err != nil {
				return [32]byte{}, err
			}
			if filepath.Ext(path) == ".go" {
				for line := range strings.SplitSeq(string(data), "\n") {
					fields := strings.Fields(line)
					if len(fields) >= 3 && fields[0] == "//go:linkname" && strings.HasPrefix(fields[2], targetPackage[0]+".") {
						return [32]byte{}, errors.New("external linkname reference to inspected package")
					}
				}
			} else if filepath.Ext(path) != ".mod" && filepath.Base(path) != "go.mod" && filepath.Base(path) != "go.sum" {
				text := binaryNativeNameReplacer.Replace(string(data))
				if strings.Contains(text, targetPackage[0]+".") {
					return [32]byte{}, errors.New("qualified native reference to inspected package")
				}
			}
		}
	}
	return binaryFileDigest(paths)
}

var binaryNativeNameReplacer = strings.NewReplacer("∕", "/", "·", ".")

func binaryFileDigest(paths []string) ([32]byte, error) {
	h := sha256.New()
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return [32]byte{}, err
		}
		_, _ = io.WriteString(h, path)
		_, _ = h.Write([]byte{0})
		digest := sha256.Sum256(data)
		_, _ = h.Write(digest[:])
		_, _ = h.Write([]byte{0})
	}
	var digest [32]byte
	copy(digest[:], h.Sum(nil))
	return digest, nil
}
