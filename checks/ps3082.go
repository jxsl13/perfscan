package checks

import (
	"bytes"
	"go/ast"
	"go/constant"
	"go/format"
	"go/parser"
	"go/token"
	"go/types"
	"go/version"
	"runtime"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS3082 reports math.Min/math.Max calls inside data-scaled hot loops
// (excluding clamps, which PS3077 owns).
var PS3082 = register(&lint.Check{
	ID:       "PS3082",
	Category: "indirect",
	Slug:     "minmax-call-in-loop",
	Level:    lint.LevelStructured,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "math.Min or math.Max called inside a data-scaled hot loop",
		Text: `On Go 1.27/arm64, math.Min compiles to CALL math.archMin inside a
48-byte frame with a stack-growth check; the min/max builtins compile at the
call site. But the two are NOT the same function:
math.Max documents +Inf as beating NaN and math.Min documents -Inf as
beating NaN, while the builtins propagate NaN — they disagree on exactly
four ordered pairs. Substituting raw builtins is a real bug on NaN-carrying
data.

The exact helper has two compile-time architecture arms. On arm64 it first
preserves the documented dominant infinity, then swaps the builtin operands
to reproduce that toolchain's math.archMin/math.archMax NaN payload choice.
The compiler folds runtime.GOARCH and inlines this 24-cost arm. Other
architectures use the portable rare-NaN fallback: take the builtin and consult
math only when its result is NaN, the only outcome on which the two functions
can differ. This is deliberately NOT a universal operand-order claim.

The detector limits itself to loops with a data-dependent domain, unbounded
loops, or a statically known domain larger than eight iterations. Calls
repeated only by loops proven to run at most eight times stay silent because
the extra guards can lose on cold/short work. Even at a reported site, retain
the change only after a same-binary benchmark of the complete operation.

The automatic fix covers exactly type-resolved math.Max(A, B) and math.Min(A,
B) calls in those loops. Only the selector is rewritten to psFmax/psFmin, so
arbitrary argument evaluation — including conversions, calls, panics, and
left-to-right side effects — remains once and in the same order. The helper is
appended once to the package and a normal or aliased runtime import is reused;
a missing import is added safely. A language version predating Go 1.21, a
package-level min/max or conflicting helper declaration, a locally shadowed
helper, a dot/blank runtime import, or a cgo file requiring an import edit stays
advisory. The architecture-specific automatic fix is frozen to Go 1.27;
another analyzer toolchain reports the site without offering this edit until
its native raw-bit behavior is revalidated. Raw-bit tests must cover both
operand orders, signed zeros, infinities, and signed/payload NaNs on every
supported target.`,
		Before: `for _, v := range xs {
	hi = math.Max(hi, v)
}`,
		After: `// psFmax: exact math.Max semantics with an inlined arm64 fast path.
func psFmax(a, b float64) float64 {
	if runtime.GOARCH == "arm64" {
		inf := math.Float64frombits(0x7ff0000000000000)
		if a == inf || b == inf { return inf }
		return max(b, a)
	}
	if r := max(a, b); r == r { return r }
	return math.Max(a, b)
}

for _, v := range xs {
	hi = psFmax(hi, v)
}`,
		MeasuredWin: `Go 1.27/Apple M2 owner frozen-binary campaigns: reduce-all
Max F64 1,393,048→274,035 ns/op (5.083x, 9/9 paired wins); trailing Max
23,571,097→6,069,971 (3.883x, 9/9); trailing Min 27,269,331→7,545,648
(3.614x, 9/9). Reducing the arm64 helper's inline cost from 83 to 24 was
material. Leading-axis Max, which retains an indirect reducer callback, was
only 1.115x (7/9), reinforcing the complete-route benchmark gate.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS3082",
		Doc:  "math.Min/math.Max call in a data-scaled hot loop",
		Run:  runPS3082,
	},
})

// ps3082Wrapper maps the math function name to its NaN-correct wrapper.
var ps3082Wrapper = map[string]string{"Max": "psFmax", "Min": "psFmin"}

// ps3082FmaxText and ps3082FminText are appended at the end of the first fixed
// file of a package that does not already declare the helper. The runtime
// placeholder is replaced with that file's existing import alias, or with the
// default name when the fix adds the import.
const ps3082FmaxText = `

// psFmax preserves math.Max bit for bit while keeping its common path inline
// (see perfscan PS3082).
func psFmax(a, b float64) float64 {
	if {{runtime}}.GOARCH == "arm64" {
		const positiveInfinityBits = uint64(0x7ff0000000000000)
		positiveInfinity := {{math}}.Float64frombits(positiveInfinityBits)
		if a == positiveInfinity || b == positiveInfinity {
			return positiveInfinity
		}
		return max(b, a)
	}
	if r := max(a, b); r == r {
		return r
	}
	return {{math}}.Max(a, b)
}`

const ps3082FminText = `

// psFmin preserves math.Min bit for bit while keeping its common path inline
// (see perfscan PS3082).
func psFmin(a, b float64) float64 {
	if {{runtime}}.GOARCH == "arm64" {
		const negativeInfinityBits = uint64(0xfff0000000000000)
		negativeInfinity := {{math}}.Float64frombits(negativeInfinityBits)
		if a == negativeInfinity || b == negativeInfinity {
			return negativeInfinity
		}
		return min(b, a)
	}
	if r := min(a, b); r == r {
		return r
	}
	return {{math}}.Min(a, b)
}`

// These are the exact helpers emitted before PS3082 gained the arm64-specific
// fast path. Keeping them recognizable makes a second perfscan run idempotent
// for repositories fixed by an earlier release without trusting a comment
// marker on an arbitrary same-named function.
const ps3082LegacyFmaxText = `
func psFmax(a, b float64) float64 {
	if r := max(a, b); r == r {
		return r
	}
	return math.Max(a, b)
}`

const ps3082LegacyFminText = `
func psFmin(a, b float64) float64 {
	if r := min(a, b); r == r {
		return r
	}
	return math.Min(a, b)
}`

// ps3082Finding is one in-loop math.Min/math.Max call; fix reports whether
// the exact eligible shape matched.
type ps3082Finding struct {
	name string
	call *ast.CallExpr
	fix  bool
}

func runPS3082(pass *analysis.Pass) (any, error) {
	fmaxUsable, fmaxDeclared := ps3082HelperStatus(pass, "psFmax")
	fminUsable, fminDeclared := ps3082HelperStatus(pass, "psFmin")
	toolchainOK := ps3082ToolchainOK()
	// A helper that must be injected uses the min/max builtins: the
	// package's language version must have them, and no package-level
	// declaration may capture the builtin name inside the helper body.
	if !fmaxDeclared {
		fmaxUsable = fmaxUsable && ps3082VersionOK(pass) && pass.Pkg.Scope().Lookup("max") == nil
	}
	if !fminDeclared {
		fminUsable = fminUsable && ps3082VersionOK(pass) && pass.Pkg.Scope().Lookup("min") == nil
	}
	usable := map[string]bool{"Max": fmaxUsable && toolchainOK, "Min": fminUsable && toolchainOK}
	placed := map[string]bool{"Max": fmaxDeclared, "Min": fminDeclared}
	for _, f := range pass.Files {
		var findings []ps3082Finding
		astutil.WithStack(f, func(n ast.Node, stack []ast.Node) bool {
			expr, ok := n.(ast.Expr)
			if !ok {
				return true
			}
			name, call, ok := ps3082MathMinMax(pass.TypesInfo, expr)
			if !ok {
				return true
			}
			// Clamps belong to PS3077: skip a call that wraps the
			// opposite call, and skip a call directly wrapped by one.
			for _, arg := range call.Args {
				if inner, _, ok := ps3082MathMinMax(pass.TypesInfo, arg); ok && inner != name {
					return true
				}
			}
			if len(stack) > 0 {
				if outerName, _, ok := ps3082MathMinMax(pass.TypesInfo, exprOrNil(stack[len(stack)-1])); ok && outerName != name {
					return true
				}
			}
			if !ps3082HotLoop(pass, stack) {
				return true
			}
			fix := usable[name] && ps3082Fixable(pass, name, call)
			findings = append(findings, ps3082Finding{name: name, call: call, fix: fix})
			return true
		})
		fixable, needs := ps3082FixNeeds(findings)
		missingHelper := (needs["Max"] && !placed["Max"]) || (needs["Min"] && !placed["Min"])
		runtimeName, mathName, needRuntime, runtimeOK := "runtime", "math", false, true
		if missingHelper {
			runtimeName, needRuntime, runtimeOK = ps3082RuntimeName(pass, f)
			if imported, ok := ps3082ImportedName(f, "math"); ok {
				mathName = imported
			} else {
				runtimeOK = false
			}
			if needRuntime && ps2110ImportsC(f) {
				runtimeOK = false
			}
		}
		if !runtimeOK {
			// A pre-existing helper remains usable, but findings needing a new
			// helper in this file stay advisory. A later file may safely host it.
			for i := range findings {
				if findings[i].fix && !placed[findings[i].name] {
					findings[i].fix = false
				}
			}
			fixable, needs = ps3082FixNeeds(findings)
		}
		// hosts reports whether this file receives a helper insertion: it
		// is the first file of the run that needs a not-yet-available
		// wrapper. The helper's own math call keeps the file's math import
		// alive.
		hosts := (needs["Max"] && !placed["Max"]) || (needs["Min"] && !placed["Min"])
		// Each fix removes exactly one math.* reference (the call's
		// selector); the runner never prunes imports, so a file that does
		// not receive a helper must keep at least one reference of its own
		// or the whole file stays advisory.
		if fixable > 0 && !hosts && ps3077MathRefs(pass.TypesInfo, f)-fixable <= 0 {
			for i := range findings {
				findings[i].fix = false
			}
			fixable = 0
			needs = nil
		}
		helperCarried := false
		for _, fd := range findings {
			diag := analysis.Diagnostic{
				Pos:     fd.call.Pos(),
				End:     fd.call.End(),
				Message: "math." + fd.name + " in a data-scaled loop can stay an out-of-line architecture call per iteration; use the exact architecture-aware " + map[string]string{"Min": "min", "Max": "max"}[fd.name] + "-builtin helper, validate signed-zero/infinity/NaN raw bits on every target, and retain it only after a complete-operation benchmark",
			}
			if fd.fix {
				// Only the math.Max/math.Min selector is replaced; the
				// argument text (and any comments in it) survives verbatim.
				edits := []analysis.TextEdit{
					{Pos: fd.call.Fun.Pos(), End: fd.call.Fun.End(), NewText: []byte(ps3082Wrapper[fd.name])},
				}
				// The first fix of the file carries every helper the file's
				// fixes need; all fixes of a run are applied together, so
				// each helper lands in the package exactly once.
				if hosts && !helperCarried {
					var text []byte
					if needs["Max"] && !placed["Max"] {
						text = append(text, ps3082HelperText(ps3082FmaxText, runtimeName, mathName)...)
						placed["Max"] = true
					}
					if needs["Min"] && !placed["Min"] {
						text = append(text, ps3082HelperText(ps3082FminText, runtimeName, mathName)...)
						placed["Min"] = true
					}
					edits = append(edits, analysis.TextEdit{Pos: f.End(), End: f.End(), NewText: text})
					if needRuntime {
						edits = append(edits, ps2110ImportEdit(f, "runtime"))
					}
					helperCarried = true
				}
				diag.SuggestedFixes = []analysis.SuggestedFix{{
					Message:   "replace math." + fd.name + " with the NaN-correct " + ps3082Wrapper[fd.name] + " wrapper",
					TextEdits: edits,
				}}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps3082Fixable reports whether call is provably a math.Min/math.Max call whose
// wrapper name is free at the call site. The fix replaces only call.Fun, so
// argument evaluation is neither duplicated nor reordered and need not be
// restricted to side-effect-free syntax.
func ps3082Fixable(pass *analysis.Pass, name string, call *ast.CallExpr) bool {
	if len(call.Args) != 2 || !ps3082IsMathFunc(pass, call, name) {
		return false
	}
	return ps3082NameFree(pass, ps3082Wrapper[name], call.Pos())
}

// ps3082FixNeeds recomputes per-file fix bookkeeping after an import or
// package-collision guard has made some findings advisory.
func ps3082FixNeeds(findings []ps3082Finding) (int, map[string]bool) {
	fixable := 0
	needs := make(map[string]bool, len(findings))
	for _, finding := range findings {
		if finding.fix {
			fixable++
			needs[finding.name] = true
		}
	}
	return fixable, needs
}

const ps3082ShortLoopLimit int64 = 8

// ps3082HotLoop reports whether the call is repeatedly evaluated by at least
// one enclosing loop with statically more than eight iterations, a
// data-dependent canonical domain, or no condition (an unbounded loop). Known
// short domains stay silent; function literals remain an execution boundary.
func ps3082HotLoop(pass *analysis.Pass, stack []ast.Node) bool {
	for i := len(stack) - 1; i >= 1; i-- {
		if _, literal := stack[i-1].(*ast.FuncLit); literal {
			return false
		}
		loop := stack[i-1]
		if !astutil.IsLoop(loop) || astutil.LoopBody(loop) != stack[i] {
			continue
		}
		if atMost, known := ps6088LoopAtMost(pass, loop, ps3082ShortLoopLimit); known {
			if !atMost {
				return true
			}
			continue
		}
		if _, ranged := loop.(*ast.RangeStmt); ranged || ps3082DataCountedLoop(pass, loop) {
			return true
		}
		if counted, ok := loop.(*ast.ForStmt); ok && counted.Cond == nil {
			return true
		}
	}
	return false
}

// ps3082DataCountedLoop recognizes a counted loop whose trip limit is a
// runtime value and whose induction variable moves monotonically toward that
// limit. It admits the common ++/-- and +=/-= positive-constant forms rather
// than equating "hot" with one exact spelling.
func ps3082DataCountedLoop(pass *analysis.Pass, loop ast.Node) bool {
	counted, ok := loop.(*ast.ForStmt)
	if !ok || counted.Cond == nil || counted.Post == nil {
		return false
	}
	var index types.Object
	direction := 0
	switch post := counted.Post.(type) {
	case *ast.IncDecStmt:
		identifier, ok := ps2110Unparen(post.X).(*ast.Ident)
		if !ok {
			return false
		}
		index = pass.TypesInfo.ObjectOf(identifier)
		if post.Tok == token.INC {
			direction = 1
		} else if post.Tok == token.DEC {
			direction = -1
		}
	case *ast.AssignStmt:
		if len(post.Lhs) != 1 || len(post.Rhs) != 1 || post.Tok != token.ADD_ASSIGN && post.Tok != token.SUB_ASSIGN {
			return false
		}
		identifier, ok := ps2110Unparen(post.Lhs[0]).(*ast.Ident)
		value := pass.TypesInfo.Types[ps2110Unparen(post.Rhs[0])].Value
		if !ok || value == nil || value.Kind() != constant.Int || constant.Sign(value) <= 0 {
			return false
		}
		index = pass.TypesInfo.ObjectOf(identifier)
		if post.Tok == token.ADD_ASSIGN {
			direction = 1
		} else {
			direction = -1
		}
	}
	if index == nil || direction == 0 {
		return false
	}
	comparison, ok := ps2110Unparen(counted.Cond).(*ast.BinaryExpr)
	if !ok {
		return false
	}
	left, leftIsIndex := ps2110Unparen(comparison.X).(*ast.Ident)
	right, rightIsIndex := ps2110Unparen(comparison.Y).(*ast.Ident)
	var bound ast.Expr
	compatible := false
	if leftIsIndex && pass.TypesInfo.ObjectOf(left) == index {
		bound = comparison.Y
		compatible = direction > 0 && (comparison.Op == token.LSS || comparison.Op == token.LEQ) ||
			direction < 0 && (comparison.Op == token.GTR || comparison.Op == token.GEQ)
	} else if rightIsIndex && pass.TypesInfo.ObjectOf(right) == index {
		bound = comparison.X
		compatible = direction > 0 && (comparison.Op == token.GTR || comparison.Op == token.GEQ) ||
			direction < 0 && (comparison.Op == token.LSS || comparison.Op == token.LEQ)
	}
	return compatible && bound != nil && pass.TypesInfo.Types[ps2110Unparen(bound)].Value == nil
}

// ps3082RuntimeName returns the qualifier the generated package-level helper
// can use for runtime. Existing ordinary/aliased imports are reused; dot and
// blank imports are deliberately not rewritten. A missing import uses the
// default name only when that name is free in the file/package scope.
func ps3082RuntimeName(pass *analysis.Pass, file *ast.File) (name string, needImport, usable bool) {
	for _, specification := range file.Imports {
		if specification.Path == nil || specification.Path.Value != `"runtime"` {
			continue
		}
		name := "runtime"
		if specification.Name != nil {
			name = specification.Name.Name
		}
		if name == "." || name == "_" {
			return "", false, false
		}
		return name, false, true
	}
	needImport, usable = ps2110PkgUsable(pass, file.End(), "runtime", "runtime")
	return "runtime", needImport, usable && needImport
}

func ps3082HelperText(template, runtimeName, mathName string) string {
	text := strings.ReplaceAll(template, "{{runtime}}", runtimeName)
	return strings.ReplaceAll(text, "{{math}}", mathName)
}

// ps3082MathMinMax also recognizes a function brought into scope by a dot
// import. Such calls are advisory-only because the automatic fix deliberately
// edits a qualified selector and never rewrites an unqualified identifier.
func ps3082MathMinMax(info *types.Info, expression ast.Expr) (string, *ast.CallExpr, bool) {
	if name, call, ok := isMathMinMax(info, expression); ok {
		return name, call, true
	}
	call, ok := expression.(*ast.CallExpr)
	if !ok {
		return "", nil, false
	}
	identifier, ok := call.Fun.(*ast.Ident)
	if !ok {
		return "", nil, false
	}
	function, ok := info.Uses[identifier].(*types.Func)
	if !ok || function.Pkg() == nil || function.Pkg().Path() != "math" || !mathMinMax[function.Name()] {
		return "", nil, false
	}
	return function.Name(), call, true
}

// ps3082IsMathFunc reports whether the call's selector resolves (by type
// information, not spelling) to the math package function of that name.
func ps3082IsMathFunc(pass *analysis.Pass, call *ast.CallExpr, name string) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	return ok && fn.Pkg() != nil && fn.Pkg().Path() == "math" && fn.Name() == name
}

// ps3082NameFree reports whether name resolves to nothing or to a
// package-level object at pos: a local object of that name would capture
// the injected wrapper's call.
func ps3082NameFree(pass *analysis.Pass, name string, pos token.Pos) bool {
	scope := pass.Pkg.Scope().Innermost(pos)
	if scope == nil {
		return true
	}
	_, obj := scope.LookupParent(name, pos)
	return obj == nil || obj.Parent() == pass.Pkg.Scope()
}

// ps3082VersionOK reports whether the package's language version has the
// min/max builtins (Go 1.21). An unknown version is treated as current.
func ps3082VersionOK(pass *analysis.Pass) bool {
	v := pass.Pkg.GoVersion()
	return v == "" || version.Compare(version.Lang(v), "go1.21") >= 0
}

// ps3082ToolchainOK freezes the architecture-specific operand-order proof to
// the compiler release whose native arm64 and Rosetta-amd64 raw bits were
// measured. Other toolchains still receive the advisory diagnostic.
func ps3082ToolchainOK() bool {
	toolchain := runtime.Version()
	return toolchain == "go1.27" || strings.HasPrefix(toolchain, "go1.27.")
}

// ps3082HelperStatus reports whether name (psFmax or psFmin) may be
// referenced by a fix and whether the package already declares it. A
// package-level declaration is only reused when it has both this check's
// marker and the exact generated structure; any other declaration of the name
// disables the fix.
func ps3082HelperStatus(pass *analysis.Pass, name string) (usable, declared bool) {
	if pass.Pkg.Scope().Lookup(name) == nil {
		return true, false
	}
	for _, f := range pass.Files {
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Recv != nil || fd.Name.Name != name {
				continue
			}
			if fd.Doc != nil && strings.Contains(fd.Doc.Text(), "perfscan PS3082") &&
				ps3082HelperMatches(pass, f, fd, name) {
				return true, true
			}
			return false, true
		}
	}
	return false, true
}

// ps3082HelperMatches verifies the complete generated function rather than
// trusting its marker comment. It accepts both the current architecture-aware
// helper and the exact portable helper emitted by older releases. Qualifiers
// are checked through type information, so lookalike package variables cannot
// make a colliding declaration reusable.
func ps3082HelperMatches(pass *analysis.Pass, file *ast.File, declaration *ast.FuncDecl, name string) bool {
	actual, ok := ps3082FormattedFunc(declaration)
	if !ok {
		return false
	}
	builtin, mathFuncName := "max", "Max"
	current, legacy := ps3082FmaxText, ps3082LegacyFmaxText
	if name == "psFmin" {
		builtin, mathFuncName = "min", "Min"
		current, legacy = ps3082FminText, ps3082LegacyFminText
	}
	if expected, ok := ps3082FormattedText(legacy); ok && bytes.Equal(actual, expected) {
		return ps3082HelperReferences(pass, declaration, builtin, mathFuncName, false)
	}
	runtimeName, ok := ps3082ImportedName(file, "runtime")
	if !ok {
		return false
	}
	mathQualifier, ok := ps3082ImportedName(file, "math")
	if !ok {
		return false
	}
	expected, ok := ps3082FormattedText(ps3082HelperText(current, runtimeName, mathQualifier))
	return ok && bytes.Equal(actual, expected) &&
		ps3082HelperReferences(pass, declaration, builtin, mathFuncName, true)
}

func ps3082FormattedFunc(declaration *ast.FuncDecl) ([]byte, bool) {
	copy := *declaration
	copy.Doc = nil
	var buffer bytes.Buffer
	if err := format.Node(&buffer, token.NewFileSet(), &copy); err != nil {
		return nil, false
	}
	return buffer.Bytes(), true
}

func ps3082FormattedText(text string) ([]byte, bool) {
	file, err := parser.ParseFile(token.NewFileSet(), "ps3082_helper.go", "package p\n"+text, 0)
	if err != nil || len(file.Decls) != 1 {
		return nil, false
	}
	declaration, ok := file.Decls[0].(*ast.FuncDecl)
	if !ok {
		return nil, false
	}
	return ps3082FormattedFunc(declaration)
}

func ps3082ImportedName(file *ast.File, path string) (string, bool) {
	for _, specification := range file.Imports {
		if specification.Path == nil || specification.Path.Value != strconv.Quote(path) {
			continue
		}
		name := path
		if slash := strings.LastIndexByte(name, '/'); slash >= 0 {
			name = name[slash+1:]
		}
		if specification.Name != nil {
			name = specification.Name.Name
		}
		return name, name != "." && name != "_"
	}
	return "", false
}

func ps3082HelperReferences(pass *analysis.Pass, declaration *ast.FuncDecl, builtin, mathName string, runtimeRequired bool) bool {
	builtinOK, mathOK, runtimeOK := false, false, !runtimeRequired
	ast.Inspect(declaration.Body, func(node ast.Node) bool {
		switch node := node.(type) {
		case *ast.Ident:
			if node.Name == builtin {
				object, ok := pass.TypesInfo.Uses[node].(*types.Builtin)
				builtinOK = builtinOK || ok && object.Name() == builtin
			}
		case *ast.SelectorExpr:
			qualifier, ok := node.X.(*ast.Ident)
			if !ok {
				break
			}
			pkg, ok := pass.TypesInfo.Uses[qualifier].(*types.PkgName)
			if !ok || pkg.Imported() == nil {
				break
			}
			if pkg.Imported().Path() == "math" && node.Sel.Name == mathName {
				fn, ok := pass.TypesInfo.Uses[node.Sel].(*types.Func)
				mathOK = mathOK || ok && fn.Pkg() != nil && fn.Pkg().Path() == "math"
			}
			if pkg.Imported().Path() == "runtime" && node.Sel.Name == "GOARCH" {
				runtimeOK = true
			}
		}
		return true
	})
	return builtinOK && mathOK && runtimeOK
}

func exprOrNil(n ast.Node) ast.Expr {
	e, _ := n.(ast.Expr)
	return e
}
