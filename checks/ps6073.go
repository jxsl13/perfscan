package checks

import (
	"go/ast"
	"go/token"
	"go/types"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS6073 implements owner issue #790. It detects dtype-specific runtime work
// guarded by a build/architecture flag inside a generic hot function even
// though the function can also be instantiated for other dtypes.
var PS6073 = register(&lint.Check{
	ID:       "PS6073",
	Category: "verify",
	Slug:     "generic-dtype-fastpath-layout",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "a generic hot function carries a flag-gated fast path for only one dtype",
		Text: `A compile-time architecture or build flag does not necessarily make a
runtime type assertion disappear from every instantiation of a generic
function. Enabling a float32 fast arm in a driver also instantiated for
float64 can leave the impossible float32 assertion, branch state, and spills in
the float64 machine code. The non-target instantiation can therefore acquire a
larger frame and entry guard even though it can never take the new path.

This check implements owner issue #790. It reports only the high-signal static
composition where:

  - a generic function, or a method on a generic receiver, is a numeric hot
    path by name/documentation or has a direct same-package call in a loop;
  - a package-level boolean constant or variable controls an enclosing if;
  - that flag is declared in a build-constrained or GOOS/GOARCH-suffixed file,
    depends on runtime.GOOS/runtime.GOARCH, or has an explicit architecture,
    SIMD, dtype, vector, native, acceleration, or fast-path name;
  - the guarded code directly converts generic data to an interface and asserts
    one concrete numeric dtype, such as any(values).([]float32); and
  - the relevant type-parameter constraint admits both that built-in dtype and
    at least one other dtype (or is unrestricted).

The detector is type-resolved: local flags, same-named fields, assertions on
non-generic data, single-dtype constraints, and user functions named any stay
silent. Calls in a range expression or for initializer do not establish
hotness, and a closure merely created in a loop does not make its body hot. A
//perfscan:generic-fastpath-layout-validated annotation records a deliberate
same-binary result.

There is NO automatic fix. Extracting a concrete driver can change escape
behavior, inlining, aliasing, fallback order, and the ABI between the generic
wrapper and specialized kernel. Compile representative target and non-target
instantiations with the flag both disabled and enabled. Compare full-symbol
objdump instruction counts, frame size, branches, and spills, then benchmark
the identical end-to-end route in one binary. If a non-target instantiation
grows, move the dtype-specific implementation behind an explicit non-generic
driver and keep the generic function free of the runtime assertion.`,
		Before: `const enableF32Fast = true // selected by an architecture build file

func rmsNormFwd[T ~float32 | ~float64](values []T) {
	if enableF32Fast {
		if values32, ok := any(values).([]float32); ok {
			rmsNormF32Fast(values32)
		}
	}
	// shared implementation
}`,
		After: `func rmsNormF32(values []float32) {
	if enableF32Fast { rmsNormF32Fast(values); return }
	rmsNormF32Scalar(values)
}

// The shared generic implementation no longer carries the F32 assertion or
// its fast-path state in float64 machine code.
func rmsNormFwd[T ~float32 | ~float64](values []T) {
	rmsNormShared(values)
}`,
		MeasuredWin: `In the Go 1.26.6/arm64 owner reproduction on Apple M2 Pro,
enabling the float32 flag grew the impossible float64 rmsNormFwd instantiation
from a 96-byte to a 144-byte frame and its entry guard from 112 to 153
instructions. Moving the float32 implementation to an explicit non-generic
driver restored the 96-byte float64 frame. Full-symbol instruction counts fell
from 311 to 244 for RMSNorm and from 376 to 294 for LayerNorm. The structurally
unchanged float32 backward paths retained equal frames and identical mnemonic
sequences (648/690 instructions), providing a negative control; timing was
thermally confounded, so the deterministic layout evidence is the gate here.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6073",
		Doc:  "flag-gated concrete dtype work bloats non-target generic instantiations",
		Run:  runPS6073,
	},
})

type ps6073Flag struct {
	object      types.Object
	specialized bool
}

type ps6073Finding struct {
	flag   types.Object
	target string
	others []string
}

func runPS6073(pass *analysis.Pass) (any, error) {
	declarations := make(map[*types.Func]*ast.FuncDecl)
	files := make(map[*ast.FuncDecl]*ast.File)
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			object, ok := pass.TypesInfo.Defs[function.Name].(*types.Func)
			if !ok {
				continue
			}
			declarations[object] = function
			files[function] = file
		}
	}

	flags := ps6073PackageFlags(pass)
	loopCalls := ps6072LoopCalls(pass, declarations)
	for object, function := range declarations {
		if ps6073Validated(function) || !ps6073Hot(function, loopCalls[object]) {
			continue
		}
		typeParams := ps6073FunctionTypeParams(object)
		if len(typeParams) == 0 {
			continue
		}
		finding, ok := ps6073Find(pass, files[function], function, flags, typeParams)
		if !ok {
			continue
		}
		otherText := strings.Join(finding.others, ", ")
		if otherText == "" {
			otherText = "other dtypes"
		}
		pass.Reportf(function.Name.Pos(), "%s guards a runtime assertion to %s with package flag %s even though its generic constraint also admits %s; compile target and non-target instantiations with both flag values and compare frame size, full-symbol instructions, branches, and spills, then extract the dtype path into an explicit non-generic driver if non-target code grows (advisory, no automatic fix)",
			function.Name.Name,
			finding.target,
			finding.flag.Name(),
			otherText,
		)
	}
	return nil, nil
}

func ps6073PackageFlags(pass *analysis.Pass) map[types.Object]ps6073Flag {
	flags := make(map[types.Object]ps6073Flag)
	for _, file := range pass.Files {
		fileSpecialized := ps6073SpecializedFile(pass, file)
		for _, declaration := range file.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok || (general.Tok != token.CONST && general.Tok != token.VAR) {
				continue
			}
			for _, specification := range general.Specs {
				values, ok := specification.(*ast.ValueSpec)
				if !ok {
					continue
				}
				initializerSpecialized := ps6073ArchitectureInitializer(pass, values.Values)
				for _, name := range values.Names {
					object := pass.TypesInfo.Defs[name]
					if object == nil || object.Parent() != pass.Pkg.Scope() || !ps6073Boolean(object.Type()) {
						continue
					}
					flags[object] = ps6073Flag{
						object:      object,
						specialized: fileSpecialized || initializerSpecialized || ps6073FlagName(name.Name),
					}
				}
			}
		}
	}
	return flags
}

func ps6073Boolean(value types.Type) bool {
	basic, ok := types.Unalias(value).Underlying().(*types.Basic)
	return ok && basic.Info()&types.IsBoolean != 0
}

func ps6073SpecializedFile(pass *analysis.Pass, file *ast.File) bool {
	for _, group := range file.Comments {
		for _, comment := range group.List {
			if strings.HasPrefix(comment.Text, "//go:build ") {
				return true
			}
		}
	}
	base := strings.ToLower(filepath.Base(pass.Fset.Position(file.Pos()).Filename))
	base = strings.TrimSuffix(base, ".go")
	for _, suffix := range []string{
		"aix", "android", "darwin", "dragonfly", "freebsd", "illumos", "ios",
		"js", "linux", "netbsd", "openbsd", "plan9", "solaris", "wasip1",
		"windows", "386", "amd64", "arm", "arm64", "loong64", "mips",
		"mips64", "mips64le", "mipsle", "ppc64", "ppc64le", "riscv64",
		"s390x", "wasm",
	} {
		if strings.HasSuffix(base, "_"+suffix) {
			return true
		}
	}
	return false
}

func ps6073ArchitectureInitializer(pass *analysis.Pass, expressions []ast.Expr) bool {
	found := false
	for _, expression := range expressions {
		ast.Inspect(expression, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok {
				return !found
			}
			object := pass.TypesInfo.Uses[selector.Sel]
			if object != nil && object.Pkg() != nil && object.Pkg().Path() == "runtime" &&
				(object.Name() == "GOARCH" || object.Name() == "GOOS") {
				found = true
				return false
			}
			return !found
		})
		if found {
			return true
		}
	}
	return false
}

func ps6073FlagName(name string) bool {
	name = strings.ToLower(name)
	for _, fragment := range []string{
		"f32", "float32", "f64", "float64", "simd", "neon", "avx", "sse",
		"vector", "native", "accelerat", "fastpath", "fast_path", "arch",
	} {
		if strings.Contains(name, fragment) {
			return true
		}
	}
	return false
}

func ps6073FunctionTypeParams(function *types.Func) map[*types.TypeParam]bool {
	result := make(map[*types.TypeParam]bool)
	signature, ok := function.Type().(*types.Signature)
	if !ok {
		return result
	}
	for _, list := range []*types.TypeParamList{signature.TypeParams(), signature.RecvTypeParams()} {
		if list == nil {
			continue
		}
		for i := range list.Len() {
			result[list.At(i)] = true
		}
	}
	return result
}

func ps6073Hot(function *ast.FuncDecl, loopCalls int) bool {
	if loopCalls != 0 {
		return true
	}
	var evidence strings.Builder
	evidence.WriteString(strings.ToLower(function.Name.Name))
	if function.Doc != nil {
		evidence.WriteByte(' ')
		evidence.WriteString(strings.ToLower(function.Doc.Text()))
	}
	text := evidence.String()
	for _, fragment := range []string{
		"norm", "kernel", "fwd", "forward", "bwd", "backward", "gemm",
		"matmul", "matrix", "softmax", "attention", "convolution", "conv",
		"embedding", "activation", "transform", "quant", "tensor", "vector",
		"reduce", "dotproduct", "dot_product",
	} {
		if strings.Contains(text, fragment) {
			return true
		}
	}
	return false
}

func ps6073Validated(function *ast.FuncDecl) bool {
	if function.Doc == nil {
		return false
	}
	for _, comment := range function.Doc.List {
		if strings.Contains(comment.Text, "perfscan:generic-fastpath-layout-validated") {
			return true
		}
	}
	return false
}

func ps6073Find(pass *analysis.Pass, file *ast.File, function *ast.FuncDecl, flags map[types.Object]ps6073Flag, functionParams map[*types.TypeParam]bool) (ps6073Finding, bool) {
	parents := ps6071Parents(file)
	var result ps6073Finding
	found := false
	ast.Inspect(function.Body, func(node ast.Node) bool {
		if found {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		assertion, ok := node.(*ast.TypeAssertExpr)
		if !ok {
			return true
		}
		source, ok := ps6073AssertionSource(pass, assertion)
		if !ok {
			return true
		}
		flag := ps6073GuardFlag(pass, assertion, parents, flags)
		if flag == nil {
			return true
		}
		for _, asserted := range ps6073AssertedTypes(pass, assertion, parents) {
			target, ok := ps6073NumericTarget(asserted)
			if !ok {
				continue
			}
			parameter, ok := ps6073SourceTargetParameter(source, asserted)
			if !ok || !functionParams[parameter] {
				continue
			}
			others, ok := ps6073NonTargetAlternatives(parameter, target)
			if !ok {
				continue
			}
			result = ps6073Finding{
				flag:   flag,
				target: types.TypeString(asserted, func(pkg *types.Package) string { return pkg.Name() }),
				others: others,
			}
			found = true
			return false
		}
		return true
	})
	return result, found
}

func ps6073AssertionSource(pass *analysis.Pass, assertion *ast.TypeAssertExpr) (types.Type, bool) {
	call, ok := ps2110Unparen(assertion.X).(*ast.CallExpr)
	if !ok || len(call.Args) != 1 || !pass.TypesInfo.Types[call.Fun].IsType() {
		return nil, false
	}
	destinationType := pass.TypesInfo.TypeOf(call.Fun)
	if destinationType == nil {
		return nil, false
	}
	destination, ok := destinationType.Underlying().(*types.Interface)
	if !ok || destination.NumMethods() != 0 {
		return nil, false
	}
	source := pass.TypesInfo.TypeOf(call.Args[0])
	return source, source != nil
}

// ps6073SourceTargetParameter proves that the asserted dynamic type is exactly
// one permitted instantiation of the generic source. Matching the complete
// pointer/slice/array shape avoids reporting impossible assertions such as
// any([]T).(*float32) or arrays of different lengths.
func ps6073SourceTargetParameter(source, target types.Type) (*types.TypeParam, bool) {
	for {
		source = types.Unalias(source)
		target = types.Unalias(target)
		switch current := source.(type) {
		case *types.TypeParam:
			_, ok := target.(*types.Basic)
			return current, ok
		case *types.Pointer:
			other, ok := target.(*types.Pointer)
			if !ok {
				return nil, false
			}
			source, target = current.Elem(), other.Elem()
		case *types.Slice:
			other, ok := target.(*types.Slice)
			if !ok {
				return nil, false
			}
			source, target = current.Elem(), other.Elem()
		case *types.Array:
			other, ok := target.(*types.Array)
			if !ok || current.Len() != other.Len() {
				return nil, false
			}
			source, target = current.Elem(), other.Elem()
		default:
			return nil, false
		}
	}
}

func ps6073GuardFlag(pass *analysis.Pass, node ast.Node, parents map[ast.Node]ast.Node, flags map[types.Object]ps6073Flag) types.Object {
	for parent := parents[node]; parent != nil; parent = parents[parent] {
		switch current := parent.(type) {
		case *ast.FuncLit, *ast.FuncDecl:
			return nil
		case *ast.IfStmt:
			var matched types.Object
			ast.Inspect(current.Cond, func(part ast.Node) bool {
				identifier, ok := part.(*ast.Ident)
				if !ok {
					return matched == nil
				}
				object := pass.TypesInfo.Uses[identifier]
				if flag, ok := flags[object]; ok && flag.specialized {
					matched = flag.object
					return false
				}
				return matched == nil
			})
			if matched != nil {
				return matched
			}
		}
	}
	return nil
}

func ps6073AssertedTypes(pass *analysis.Pass, assertion *ast.TypeAssertExpr, parents map[ast.Node]ast.Node) []types.Type {
	if assertion.Type != nil {
		if asserted := pass.TypesInfo.TypeOf(assertion.Type); asserted != nil {
			return []types.Type{asserted}
		}
		return nil
	}
	for parent := parents[assertion]; parent != nil; parent = parents[parent] {
		switch current := parent.(type) {
		case *ast.TypeSwitchStmt:
			var asserted []types.Type
			for _, statement := range current.Body.List {
				clause, ok := statement.(*ast.CaseClause)
				if !ok {
					continue
				}
				for _, expression := range clause.List {
					if value := pass.TypesInfo.TypeOf(expression); value != nil {
						asserted = append(asserted, value)
					}
				}
			}
			return asserted
		case *ast.FuncLit, *ast.FuncDecl:
			return nil
		}
	}
	return nil
}

func ps6073NumericTarget(value types.Type) (string, bool) {
	for {
		switch current := value.(type) {
		case *types.Alias:
			value = types.Unalias(current)
		case *types.Pointer:
			value = current.Elem()
		case *types.Slice:
			value = current.Elem()
		case *types.Array:
			value = current.Elem()
		default:
			basic, ok := value.(*types.Basic)
			if !ok || !ps6073NumericBasic(basic) {
				return "", false
			}
			return basic.Name(), true
		}
	}
}

func ps6073NumericBasic(value *types.Basic) bool {
	return value.Info()&(types.IsInteger|types.IsFloat|types.IsComplex) != 0 && value.Info()&types.IsUntyped == 0
}

type ps6073Constraint struct {
	broad        bool
	alternatives map[string]bool
	admitted     map[string]bool
}

func ps6073NonTargetAlternatives(parameter *types.TypeParam, target string) ([]string, bool) {
	summary := ps6073Constraint{
		alternatives: make(map[string]bool),
		admitted:     make(map[string]bool),
	}
	ps6073ConstraintTypes(parameter.Constraint(), &summary, make(map[types.Type]bool))
	if !summary.broad && !summary.admitted[target] {
		return nil, false
	}
	if summary.broad {
		return nil, true
	}
	otherSet := make(map[string]bool)
	for alternative := range summary.alternatives {
		if alternative != target {
			otherSet[alternative] = true
		}
	}
	if len(otherSet) == 0 {
		return nil, false
	}
	others := make([]string, 0, len(otherSet))
	for alternative := range otherSet {
		others = append(others, alternative)
	}
	slices.Sort(others)
	if len(others) > 3 {
		others = append(others[:3], "other dtypes")
	}
	return others, true
}

func ps6073ConstraintTypes(value types.Type, result *ps6073Constraint, seen map[types.Type]bool) {
	if value == nil || seen[value] {
		return
	}
	seen[value] = true
	switch current := value.(type) {
	case *types.Alias:
		ps6073ConstraintTypes(types.Unalias(current), result, seen)
	case *types.Named:
		if underlying, ok := current.Underlying().(*types.Interface); ok {
			ps6073ConstraintTypes(underlying, result, seen)
			return
		}
		if basic, ok := current.Underlying().(*types.Basic); ok && ps6073NumericBasic(basic) {
			result.alternatives[current.Obj().Name()] = true
		}
	case *types.Interface:
		current.Complete()
		// A built-in numeric type cannot satisfy explicit methods, so the
		// asserted []float32/[]float64 dynamic type is not admitted.
		if current.NumMethods() != 0 {
			return
		}
		if current.NumEmbeddeds() == 0 {
			result.broad = true
			return
		}
		for i := range current.NumEmbeddeds() {
			ps6073ConstraintTypes(current.EmbeddedType(i), result, seen)
		}
	case *types.Union:
		for i := range current.Len() {
			term := current.Term(i)
			if basic, ok := types.Unalias(term.Type()).(*types.Basic); ok && ps6073NumericBasic(basic) {
				result.alternatives[basic.Name()] = true
				result.admitted[basic.Name()] = true
				continue
			}
			if term.Tilde() {
				if basic, ok := types.Unalias(term.Type()).Underlying().(*types.Basic); ok && ps6073NumericBasic(basic) {
					result.alternatives[basic.Name()] = true
					result.admitted[basic.Name()] = true
					continue
				}
			}
			ps6073ConstraintTypes(term.Type(), result, seen)
		}
	case *types.Basic:
		if ps6073NumericBasic(current) {
			result.alternatives[current.Name()] = true
			result.admitted[current.Name()] = true
		}
	default:
		// Predeclared comparable and other non-enumerated, method-free
		// constraints admit the numeric target and representative alternatives.
		result.broad = true
	}
}
