package closureenv

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/go/packages"
)

// PackageRequest describes an exact package build context. An empty target
// uses the selected go command's environment. Tags are passed to both go list
// and go build. This collector deliberately does not guess cgo or experiment
// settings: callers must put them in Env when they differ from the process.
type PackageRequest struct {
	GoBinary string
	Dir      string
	Pattern  string
	GOOS     string
	GOARCH   string
	Tags     []string
	Env      []string
	// Function limits extraction to one enclosing source function. Other
	// unsupported closures in the package do not erase target evidence.
	Function string
	File     string
	Line     int
	Column   int
}

type listedPackage struct {
	ImportPath string
	Dir        string
	GoFiles    []string
	CgoFiles   []string
	Module     *struct {
		Path      string
		Version   string
		GoVersion string
	}
}

// CollectPackage compiles one resolved package with its normal module/import
// context and extracts compiler-confirmed escaping closure layouts. It rejects
// patterns resolving to zero or multiple packages.
func CollectPackage(ctx context.Context, request *PackageRequest) ([]Evidence, error) {
	goBinaryName := request.GoBinary
	if goBinaryName == "" {
		goBinaryName = "go"
	}
	if request.Pattern == "" {
		return nil, errors.New("missing package pattern")
	}
	goBinary, err := exec.LookPath(goBinaryName)
	if err != nil {
		return nil, fmt.Errorf("resolve selected go command: %w", err)
	}
	goBinary, err = filepath.Abs(goBinary)
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
	tagFlag := strings.Join(request.Tags, ",")
	goenv := exec.CommandContext(ctx, goBinary, "env", "GOROOT", "GOVERSION", "GOOS", "GOARCH", "GOFLAGS", "GOEXPERIMENT", "GOAMD64", "GOARM64", "CGO_ENABLED")
	goenv.Dir, goenv.Env = request.Dir, environment
	envOutput, err := goenv.Output()
	if err != nil {
		return nil, fmt.Errorf("go env: %w", err)
	}
	lines := strings.Split(strings.TrimSpace(string(envOutput)), "\n")
	if len(lines) != 9 {
		return nil, errors.New("incomplete go env")
	}
	goroot, version, goos, goarch := lines[0], lines[1], lines[2], lines[3]
	architecture, err := targetArchitecture(ctx, goBinary, request.Dir, environment, goarch)
	if err != nil {
		return nil, err
	}
	if strings.Contains(lines[4], "-gcflags") {
		return nil, errors.New("GOFLAGS containing -gcflags is unsupported")
	}
	path := filepath.Dir(goBinary) + string(os.PathListSeparator) + os.Getenv("PATH")
	environment = append(environment, "GOROOT="+goroot, "PATH="+path)

	listArgs := []string{"list", "-json"}
	if tagFlag != "" {
		listArgs = append(listArgs, "-tags="+tagFlag)
	}
	listArgs = append(listArgs, request.Pattern)
	list := exec.CommandContext(ctx, goBinary, listArgs...)
	list.Dir, list.Env = request.Dir, environment
	output, err := list.Output()
	if err != nil {
		return nil, fmt.Errorf("resolve package: %w", err)
	}
	dependencyArgs := append([]string{"list", "-deps", "-json"}, listArgs[2:]...)
	dependencies := exec.CommandContext(ctx, goBinary, dependencyArgs...)
	dependencies.Dir, dependencies.Env = request.Dir, environment
	dependencyOutput, err := dependencies.Output()
	if err != nil {
		return nil, fmt.Errorf("resolve dependency context: %w", err)
	}
	dependencyDigest, err := dependencySourceDigest(dependencyOutput)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(output))
	var described []listedPackage
	for decoder.More() {
		var current listedPackage
		if err := decoder.Decode(&current); err != nil {
			return nil, fmt.Errorf("decode package: %w", err)
		}
		described = append(described, current)
	}
	if len(described) != 1 || described[0].ImportPath == "" || described[0].Dir == "" {
		return nil, fmt.Errorf("package pattern resolved to %d packages", len(described))
	}
	description := described[0]
	listedFiles := slices.Concat(description.GoFiles, description.CgoFiles)
	if len(listedFiles) == 0 {
		return nil, errors.New("package has no compiled Go files")
	}
	beforeBuild := make(map[string][32]byte, len(listedFiles))
	for _, name := range listedFiles {
		path := filepath.Join(description.Dir, name)
		source, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		beforeBuild[name] = sha256.Sum256(source)
	}
	flags := "-S -m=2 -d=closure=1"
	buildArgs := []string{"build", "-gcflags=" + description.ImportPath + "=" + flags}
	if tagFlag != "" {
		buildArgs = append(buildArgs, "-tags="+tagFlag)
	}
	buildArgs = append(buildArgs, request.Pattern)
	outputFile, err := os.CreateTemp("", "perfscan-closure-package-*")
	if err != nil {
		return nil, err
	}
	outputPath := outputFile.Name()
	if err := outputFile.Close(); err != nil {
		return nil, err
	}
	defer os.Remove(outputPath)
	buildArgs = append([]string{"build", "-o", outputPath}, buildArgs[1:]...)
	build := exec.CommandContext(ctx, goBinary, buildArgs...)
	build.Dir, build.Env = request.Dir, environment
	compilerOutput, err := build.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("compile package evidence: %w: %s", err, compilerOutput)
	}

	allocator, err := LoadAllocator(goroot, goarch)
	if err != nil {
		return nil, err
	}
	sizes := types.SizesFor("gc", goarch)
	if sizes == nil {
		return nil, fmt.Errorf("unsupported gc target %s", goarch)
	}
	config := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo,
		Dir:  request.Dir, Env: environment, Context: ctx,
	}
	if tagFlag != "" {
		config.BuildFlags = []string{"-tags=" + tagFlag}
	}
	loaded, err := packages.Load(config, request.Pattern)
	if err != nil || len(loaded) != 1 || packages.PrintErrors(loaded) != 0 {
		return nil, fmt.Errorf("load typed package: packages=%d: %w", len(loaded), err)
	}
	pkg := loaded[0]
	for name, expected := range beforeBuild {
		source, err := os.ReadFile(filepath.Join(description.Dir, name))
		if err != nil || sha256.Sum256(source) != expected {
			return nil, fmt.Errorf("compiled source set changed during collection: %s", name)
		}
	}
	dependencies = exec.CommandContext(ctx, goBinary, dependencyArgs...)
	dependencies.Dir, dependencies.Env = request.Dir, environment
	dependencyOutput, err = dependencies.Output()
	currentDependencies, digestErr := dependencySourceDigest(dependencyOutput)
	if err != nil || digestErr != nil || currentDependencies != dependencyDigest {
		return nil, errors.New("dependency context changed during collection")
	}
	byBase := make(map[string]int, len(pkg.CompiledGoFiles))
	for index, path := range pkg.CompiledGoFiles {
		base := filepath.Base(path)
		if _, duplicate := byBase[base]; duplicate {
			return nil, fmt.Errorf("ambiguous compiled filename %q", base)
		}
		byBase[base] = index
	}

	modulePath, moduleVersion, language := "", "", ""
	if description.Module != nil {
		modulePath, moduleVersion, language = description.Module.Path, description.Module.Version, description.Module.GoVersion
	}
	tags := slices.Clone(request.Tags)
	slices.Sort(tags)
	result := make([]Evidence, 0)
	compilerText := string(compilerOutput)
	for _, escaped := range escapePattern.FindAllStringSubmatch(compilerText, -1) {
		filename := filepath.Base(escaped[1])
		index, exists := byBase[filename]
		if !exists {
			continue
		}
		line, _ := strconvAtoi(escaped[2])
		column, _ := strconvAtoi(escaped[3])
		function := enclosingFunction(pkg.Fset, pkg.Syntax[index], line)
		if request.Function != "" && function != request.Function {
			continue
		}
		if request.File != "" && filename != filepath.Base(request.File) || request.Line > 0 && line != request.Line || request.Column > 0 && column != request.Column {
			continue
		}
		typeFields := closureTypeAtFile(compilerText, filename, line)
		if typeFields == "" {
			return nil, fmt.Errorf("compiler omitted closure type evidence at %s:%d:%d", filename, line, column)
		}
		source, err := os.ReadFile(pkg.CompiledGoFiles[index])
		if err != nil {
			return nil, err
		}
		captures, bytes, scanned, err := capturesAt(pkg.Fset, pkg.Syntax[index], pkg.TypesInfo, sizes, line, column, escaped[4], typeFields)
		if err != nil {
			return nil, fmt.Errorf("%s:%d:%d: %w", filename, line, column, err)
		}
		class, err := allocator.ClassForClosure(bytes, scanned)
		if err != nil {
			return nil, err
		}
		digest := sha256.Sum256(source)
		if digest != beforeBuild[filename] {
			return nil, fmt.Errorf("compiled source changed during collection: %s", filename)
		}
		result = append(result, Evidence{GoVersion: version, GOOS: goos, GOARCH: goarch,
			SourceSHA256: hex.EncodeToString(digest[:]), Package: description.ImportPath,
			Function: function, File: filename, Line: line, Column: column,
			Escapes: true, Captures: captures, EnvironmentBytes: bytes, SizeClassBytes: class, Scanned: scanned,
			ModulePath: modulePath, ModuleVersion: moduleVersion, GoLanguageVersion: language, BuildTags: tags,
			GOFLAGS: lines[4], GOEXPERIMENT: lines[5], ArchitectureLevel: architecture, CGOEnabled: lines[8]})
	}
	return result, nil
}

func dependencySourceDigest(output []byte) ([32]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(output))
	var entries []string
	for decoder.More() {
		var pkg listedPackage
		if err := decoder.Decode(&pkg); err != nil {
			return [32]byte{}, err
		}
		for _, name := range slices.Concat(pkg.GoFiles, pkg.CgoFiles) {
			entries = append(entries, pkg.ImportPath+"\x00"+filepath.Join(pkg.Dir, name))
		}
	}
	slices.Sort(entries)
	digest := sha256.New()
	for _, entry := range entries {
		separator := strings.IndexByte(entry, 0)
		path := entry[separator+1:]
		data, err := os.ReadFile(path)
		if err != nil {
			return [32]byte{}, err
		}
		_, _ = io.WriteString(digest, entry[:separator])
		_, _ = digest.Write([]byte{0})
		_, _ = io.WriteString(digest, filepath.Base(path))
		_, _ = digest.Write([]byte{0})
		_, _ = digest.Write(data)
	}
	var result [32]byte
	copy(result[:], digest.Sum(nil))
	return result, nil
}

func capturesAt(fset *token.FileSet, file *ast.File, info *types.Info, sizes types.Sizes, line, column int, namesText, fieldsText string) ([]Capture, int64, bool, error) {
	fields, names := strings.Split(fieldsText, ";"), strings.Fields(namesText)
	if len(fields) != len(names) {
		return nil, 0, false, errors.New("capture/type count mismatch")
	}
	captures := make([]Capture, 0, len(fields))
	for fieldIndex, field := range fields {
		parts := strings.Fields(strings.TrimSpace(field))
		if len(parts) != 2 || parts[0] != "X"+strconv.Itoa(fieldIndex) {
			return nil, 0, false, fmt.Errorf("unsupported compiler closure field %q", field)
		}
		sourceType := capturedSourceType(fset, file, info, line, column, names[fieldIndex])
		if sourceType == nil {
			return nil, 0, false, fmt.Errorf("cannot resolve compiler capture %q", names[fieldIndex])
		}
		typeValue, byReference, err := compilerCaptureType(parts[1], sourceType)
		if err != nil {
			return nil, 0, false, err
		}
		captures = append(captures, Capture{Name: names[fieldIndex], Type: parts[1], Size: sizes.Sizeof(typeValue), Align: sizes.Alignof(typeValue), ByReference: byReference, HasPointers: typeHasPointers(typeValue, make(map[types.Type]bool))})
	}
	bytes, err := Layout(sizes, captures)
	if err != nil {
		return nil, 0, false, err
	}
	scanned := false
	for _, capture := range captures {
		scanned = scanned || capture.HasPointers
	}
	return captures, bytes, scanned, nil
}

func closureTypeAtFile(output, filename string, line int) string {
	suffix := ":" + strconv.Itoa(line) + ")"
	var found string
	matches := 0
	for text := range strings.SplitSeq(output, "\n") {
		end := strings.Index(text, suffix)
		if end < 0 {
			continue
		}
		start := strings.LastIndexByte(text[:end], '(')
		if start < 0 || filepath.Base(text[start+1:end]) != filename {
			continue
		}
		if match := typePattern.FindStringSubmatch(text); match != nil {
			matches++
			if matches > 1 {
				return ""
			}
			found = match[1]
		}
	}
	return found
}

func strconvAtoi(text string) (int, error) { return strconv.Atoi(text) }
