package crossover

import (
	"crypto/sha256"
	"encoding/hex"
	"go/types"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/jxsl13/perfscan/internal/closureenv"
	"github.com/jxsl13/perfscan/internal/observedpath"
	"golang.org/x/tools/go/packages"
)

func TestTypedBinaryInventoriesWindowsJunctionAgreement(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	physical := filepath.Join(root, "physical-package")
	if err := os.Mkdir(physical, 0700); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string]string{
		"go.mod":    "module example.test/junction\n\ngo 1.25.0\n",
		"leaf.go":   "package junction\nimport \"fmt\"\nfunc Leaf()string{return fmt.Sprint(1)}\n",
		HarnessFile: "package junction\nimport \"testing\"\nfunc TestHarness(t *testing.T){}\n",
	} {
		if err := os.WriteFile(filepath.Join(physical, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	alias := filepath.Join(root, "package-junction")
	if out, err := exec.CommandContext(t.Context(), "cmd.exe", "/d", "/c", "mklink", "/J", alias, physical).CombinedOutput(); err != nil {
		t.Fatalf("ordinary task-owned junction: %v: %s", err, out)
	}
	env, sdk, err := (&BuildSelection{Root: alias}).Environment(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := packages.Load(&packages.Config{Context: t.Context(), Dir: alias, Env: env, Mode: packages.NeedName | packages.NeedFiles | packages.NeedTypes | packages.NeedImports | packages.NeedDeps}, ".")
	if err != nil || len(loaded) != 1 {
		t.Fatalf("typed load: %v", err)
	}
	model := &HarnessModel{SourceSHA256: map[string]string{}, TypedFileSHA256: map[string]string{}, TypedPackageFiles: map[string][]string{}}
	visited := map[*packages.Package]bool{}
	var visit func(*packages.Package)
	visit = func(pkg *packages.Package) {
		if visited[pkg] {
			return
		}
		visited[pkg] = true
		if len(pkg.Errors) != 0 {
			t.Fatalf("typed package: %v", pkg.Errors)
		}
		if pkg.Types == types.Unsafe {
			return
		}
		for _, path := range pkg.GoFiles {
			content, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256(content)
			hash := hex.EncodeToString(sum[:])
			canonical, err := observedpath.Canonical(path)
			if err != nil {
				t.Fatal(err)
			}
			model.TypedFileSHA256[canonical] = hash
			model.TypedPackageFiles[pkg.ID] = append(model.TypedPackageFiles[pkg.ID], canonical)
			if pkg == loaded[0] {
				model.SourceSHA256[filepath.Base(path)] = hash
			}
		}
		slices.Sort(model.TypedPackageFiles[pkg.ID])
		for _, dependency := range pkg.Imports {
			visit(dependency)
		}
	}
	visit(loaded[0])
	build, err := closureenv.CollectPortableBinary(t.Context(), &closureenv.PackageRequest{Dir: alias, Pattern: ".", GoBinary: sdk, GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, Env: env})
	if err != nil {
		t.Fatal(err)
	}
	harness, err := observedpath.Canonical(filepath.Join(alias, HarnessFile))
	if err != nil {
		t.Fatal(err)
	}
	if err := validateTypedInputs(model, build, harness); err != nil {
		t.Fatal(err)
	}
	for path := range model.TypedFileSHA256 {
		model.TypedFileSHA256[path] = "changed"
		if err := validateTypedInputs(model, build, harness); err == nil {
			t.Fatal("normalization relaxed exact source hash join")
		}
		break
	}
}
