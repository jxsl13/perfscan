package closureenv

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var (
	escapePattern = regexp.MustCompile(`(?m)^(.+):(\d+):(\d+): heap closure, captured vars = \[([^]]*)\]`)
	typePattern   = regexp.MustCompile(`type:noalg\.struct \{ F uintptr; ([^}]*) \}`)
)

// CollectFile invokes the requested gc compiler and extracts only closure
// layouts whose emitted field types this collector can size without imports.
func CollectFile(ctx context.Context, goBinary, goos, goarch, path string) ([]Evidence, error) {
	source, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	environment := append(os.Environ(), "GOOS="+goos, "GOARCH="+goarch)
	envCommand := exec.CommandContext(ctx, goBinary, "env", "GOROOT", "GOVERSION", "GOFLAGS", "GOEXPERIMENT", "GOAMD64", "GOARM64", "CGO_ENABLED")
	envCommand.Env = environment
	envOut, err := envCommand.Output()
	if err != nil {
		return nil, fmt.Errorf("go env: %w", err)
	}
	lines := strings.Split(string(envOut), "\n")
	if len(lines) < 7 {
		return nil, errors.New("incomplete go env")
	}
	goroot, version := strings.TrimSpace(lines[0]), strings.TrimSpace(lines[1])
	architecture, err := targetArchitecture(ctx, goBinary, "", environment, goarch)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(lines[2]) != "" {
		return nil, errors.New("bare-file collection does not support GOFLAGS; use package mode")
	}
	object, err := os.CreateTemp("", "perfscan-closureenv-*.o")
	if err != nil {
		return nil, err
	}
	objectPath := object.Name()
	if err := object.Close(); err != nil {
		return nil, err
	}
	defer os.Remove(objectPath)
	command := exec.CommandContext(ctx, goBinary, "tool", "compile", "-o", objectPath, "-S", "-m", "-d=closure=1", path)
	command.Env = environment
	output, err := command.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("compile evidence: %w: %s", err, output)
	}
	allocator, err := LoadAllocator(goroot, goarch)
	if err != nil {
		return nil, err
	}
	sizes := types.SizesFor("gc", goarch)
	if sizes == nil {
		return nil, fmt.Errorf("unsupported gc target %s", goarch)
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, source, 0)
	if err != nil {
		return nil, err
	}
	info := &types.Info{Defs: make(map[*ast.Ident]types.Object), Uses: make(map[*ast.Ident]types.Object)}
	goLanguage := strings.TrimPrefix(version, "go")
	if index := strings.LastIndexByte(goLanguage, '.'); index > strings.IndexByte(goLanguage, '.') {
		goLanguage = goLanguage[:index]
	}
	if _, err := (&types.Config{Importer: importer.Default(), Sizes: sizes, GoVersion: "go" + goLanguage}).Check(file.Name.Name, fset, []*ast.File{file}, info); err != nil {
		return nil, fmt.Errorf("type-check compiler input: %w", err)
	}
	compilerText := string(output)
	escapeLines := escapePattern.FindAllStringSubmatch(compilerText, -1)
	digest := sha256.Sum256(source)
	result := make([]Evidence, 0, len(escapeLines))
	for _, escaped := range escapeLines {
		line, _ := strconv.Atoi(escaped[2])
		column, _ := strconv.Atoi(escaped[3])
		typeFields := closureTypeAtFile(compilerText, filepath.Base(path), line)
		if typeFields == "" {
			return nil, fmt.Errorf("compiler omitted closure type evidence at %d:%d", line, column)
		}
		fields := strings.Split(typeFields, ";")
		names := strings.Fields(escaped[4])
		if len(fields) != len(names) {
			return nil, fmt.Errorf("capture/type count mismatch at %d:%d", line, column)
		}
		captures := make([]Capture, 0, len(fields))
		for fieldIndex, field := range fields {
			parts := strings.Fields(strings.TrimSpace(field))
			if len(parts) != 2 || parts[0] != "X"+strconv.Itoa(fieldIndex) {
				return nil, fmt.Errorf("unsupported compiler closure field %q", field)
			}
			sourceType := capturedSourceType(fset, file, info, line, column, names[fieldIndex])
			if sourceType == nil {
				return nil, fmt.Errorf("cannot resolve compiler capture %q", names[fieldIndex])
			}
			typeValue, byReference, err := compilerCaptureType(parts[1], sourceType)
			if err != nil {
				return nil, err
			}
			captures = append(captures, Capture{Name: names[fieldIndex], Type: parts[1], Size: sizes.Sizeof(typeValue), Align: sizes.Alignof(typeValue), ByReference: byReference, HasPointers: typeHasPointers(typeValue, make(map[types.Type]bool))})
		}
		bytes, err := Layout(sizes, captures)
		if err != nil {
			return nil, err
		}
		scanned := false
		for _, capture := range captures {
			scanned = scanned || capture.HasPointers
		}
		class, err := allocator.ClassForClosure(bytes, scanned)
		if err != nil {
			return nil, err
		}
		function := enclosingFunction(fset, file, line)
		result = append(result, Evidence{GoVersion: version, GOOS: goos, GOARCH: goarch, SourceSHA256: hex.EncodeToString(digest[:]), Package: file.Name.Name, Function: function, File: filepath.Base(path), Line: line, Column: column, Escapes: true, Captures: captures, EnvironmentBytes: bytes, SizeClassBytes: class, Scanned: scanned, GOFLAGS: strings.TrimSpace(lines[2]), GOEXPERIMENT: strings.TrimSpace(lines[3]), ArchitectureLevel: architecture, CGOEnabled: strings.TrimSpace(lines[6])})
	}
	return result, nil
}

func compilerCaptureType(emitted string, source types.Type) (types.Type, bool, error) {
	if compilerTypeMatches(emitted, source) {
		return source, false, nil
	}
	if strings.HasPrefix(emitted, "*") && compilerTypeMatches(emitted[1:], source) {
		return types.NewPointer(source), true, nil
	}
	return nil, false, fmt.Errorf("unsupported or mismatched compiler capture type %q", emitted)
}

func compilerTypeMatches(emitted string, source types.Type) bool {
	source = types.Unalias(source)
	if value, err := basicCompilerType(emitted); err == nil && types.Identical(value, source) {
		return true
	}
	switch value := source.(type) {
	case *types.Pointer:
		return strings.HasPrefix(emitted, "*") && compilerTypeMatches(emitted[1:], value.Elem())
	case *types.Slice:
		return strings.HasPrefix(emitted, "[]") && compilerTypeMatches(emitted[2:], value.Elem())
	case *types.Array:
		prefix := "[" + strconv.FormatInt(value.Len(), 10) + "]"
		return strings.HasPrefix(emitted, prefix) && compilerTypeMatches(emitted[len(prefix):], value.Elem())
	case *types.Named:
		object := value.Obj()
		if object == nil {
			return false
		}
		if object.Pkg() == nil {
			return emitted == object.Name()
		}
		return emitted == object.Pkg().Name()+"."+object.Name() || emitted == object.Pkg().Path()+"."+object.Name() || emitted == "<unlinkable>."+object.Name()
	}
	return false
}

func typeHasPointers(value types.Type, seen map[types.Type]bool) bool {
	value = types.Unalias(value)
	if seen[value] {
		return false
	}
	seen[value] = true
	switch current := value.Underlying().(type) {
	case *types.Pointer, *types.Slice, *types.Map, *types.Chan, *types.Signature, *types.Interface:
		return true
	case *types.Basic:
		return current.Kind() == types.String || current.Kind() == types.UnsafePointer
	case *types.Array:
		if current.Len() == 0 {
			return false
		}
		return typeHasPointers(current.Elem(), seen)
	case *types.Struct:
		for index := range current.NumFields() {
			if typeHasPointers(current.Field(index).Type(), seen) {
				return true
			}
		}
	}
	return false
}

func capturedSourceType(fset *token.FileSet, file *ast.File, info *types.Info, line, column int, name string) types.Type {
	var literal *ast.FuncLit
	ast.Inspect(file, func(node ast.Node) bool {
		candidate, ok := node.(*ast.FuncLit)
		if !ok {
			return literal == nil
		}
		position := fset.Position(candidate.Pos())
		if position.Line == line && position.Column == column {
			literal = candidate
			return false
		}
		return literal == nil
	})
	if literal == nil {
		return nil
	}
	objects := make(map[types.Object]bool)
	ast.Inspect(literal.Body, func(node ast.Node) bool {
		if nested, ok := node.(*ast.FuncLit); ok && nested != literal {
			return false
		}
		identifier, ok := node.(*ast.Ident)
		if !ok || identifier.Name != name {
			return true
		}
		candidate, ok := info.Uses[identifier].(*types.Var)
		if ok && !candidate.IsField() && (candidate.Pos() < literal.Pos() || candidate.Pos() > literal.End()) {
			objects[candidate] = true
		}
		return true
	})
	if len(objects) != 1 {
		return nil
	}
	for object := range objects {
		return object.Type()
	}
	return nil
}

func basicCompilerType(name string) (types.Type, error) {
	if strings.HasPrefix(name, "[]") {
		element, err := basicCompilerType(name[2:])
		if err != nil {
			return nil, err
		}
		return types.NewSlice(element), nil
	}
	if strings.HasPrefix(name, "[") {
		end := strings.IndexByte(name, ']')
		if end < 2 {
			return nil, fmt.Errorf("unsupported compiler type %q", name)
		}
		length, err := strconv.ParseInt(name[1:end], 10, 64)
		if err != nil {
			return nil, err
		}
		element, err := basicCompilerType(name[end+1:])
		if err != nil {
			return nil, err
		}
		return types.NewArray(element, length), nil
	}
	if strings.HasPrefix(name, "*") {
		element, err := basicCompilerType(name[1:])
		if err != nil {
			return nil, err
		}
		return types.NewPointer(element), nil
	}
	for _, basic := range []*types.Basic{types.Typ[types.Bool], types.Typ[types.Uint8], types.Typ[types.Uint16], types.Typ[types.Uint32], types.Typ[types.Uint64], types.Typ[types.Int8], types.Typ[types.Int16], types.Typ[types.Int32], types.Typ[types.Int64], types.Typ[types.Int], types.Typ[types.Uint], types.Typ[types.Uintptr], types.Typ[types.Float32], types.Typ[types.Float64], types.Typ[types.String]} {
		if basic.Name() == name || name == "byte" && basic.Kind() == types.Uint8 {
			return basic, nil
		}
	}
	return nil, fmt.Errorf("unsupported named, imported, generic, or internal compiler type %q", name)
}

func enclosingFunction(fset *token.FileSet, file *ast.File, line int) string {
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok {
			continue
		}
		start, end := fset.Position(function.Pos()), fset.Position(function.End())
		if line >= start.Line && line <= end.Line {
			return function.Name.Name
		}
	}
	return ""
}
