package checks

import (
	"go/ast"
	"go/token"
	"go/types"
	"go/version"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS3082 reports math.Min/math.Max calls inside loops (excluding clamps,
// which PS3077 owns).
var PS3082 = register(&lint.Check{
	ID:       "PS3082",
	Category: "indirect",
	Slug:     "minmax-call-in-loop",
	Level:    lint.LevelStructured,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "math.Min or math.Max called inside a loop",
		Text: `On arm64, math.Min compiles to CALL math.archMin inside a
48-byte frame with a stack-growth check; the min/max builtins compile to a
single instruction in a leaf frame. But the two are NOT the same function:
math.Max documents +Inf as beating NaN and math.Min documents -Inf as
beating NaN, while the builtins propagate NaN — they disagree on exactly
four ordered pairs. Substituting raw builtins is a real bug on NaN-carrying
data.

The remedy is a thin wrapper that takes the instruction and consults math
only when the instruction returns NaN (the only case they can disagree on).
It does NOT beat an existing comparison chain — this replaces calls, not
branchless code. Gate the change on one planted NaN/Inf value per call
site; a kernel that reduces to a scalar cannot see the divergence.

The automatic fix covers exactly math.Max(A, B) and math.Min(A, B) in-loop
calls whose two operands are side-effect-free (identifiers, selector
chains, basic literals, ident[ident] indexing) and float64-typed: only the
math.Max/math.Min selector is rewritten, to psFmax/psFmin, and the wrapper
is appended once to the package. The wrapper returns the builtin's result
whenever it is not NaN and delegates to math.Max/math.Min otherwise — the
only inputs on which the two disagree — so the result stays bit-identical,
and the wrapper's own math call keeps the import alive. Anything else —
call operands, a language version predating the Go 1.21 builtins, a
package-level min/max or a psFmax/psFmin declaration that is not this
helper, a locally shadowed wrapper name, or a file whose math import the
rewrite would orphan — stays advisory.`,
		Before: `for _, v := range xs {
	hi = math.Max(hi, v)
}`,
		After: `// psFmax: the max builtin, falling back to math.Max only on NaN.
func psFmax(a, b float64) float64 {
	if r := max(a, b); r == r { return r }
	return math.Max(a, b)
}

for _, v := range xs {
	hi = psFmax(hi, v)
}`,
		MeasuredWin: "reference corpus: PPOClip batch 4096 -58.7% (100.5µs→41.6µs), GRPO -37.0%; converting an existing comparison chain measured +13% and was reverted",
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS3082",
		Doc:  "math.Min/math.Max call in a loop",
		Run:  runPS3082,
	},
})

// ps3082Wrapper maps the math function name to its NaN-correct wrapper.
var ps3082Wrapper = map[string]string{"Max": "psFmax", "Min": "psFmin"}

// ps3082FmaxText and ps3082FminText are appended verbatim at the end of the
// first fixed file of a package that does not already declare the helper.
const ps3082FmaxText = `

// psFmax is the max builtin with a fallback to math.Max whenever the
// builtin returns NaN — the only inputs on which the two disagree (see
// perfscan PS3082).
func psFmax(a, b float64) float64 {
	if r := max(a, b); r == r {
		return r
	}
	return math.Max(a, b)
}`

const ps3082FminText = `

// psFmin is the min builtin with a fallback to math.Min whenever the
// builtin returns NaN — the only inputs on which the two disagree (see
// perfscan PS3082).
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
	// A helper that must be injected uses the min/max builtins: the
	// package's language version must have them, and no package-level
	// declaration may capture the builtin name inside the helper body.
	if !fmaxDeclared {
		fmaxUsable = fmaxUsable && ps3082VersionOK(pass) && pass.Pkg.Scope().Lookup("max") == nil
	}
	if !fminDeclared {
		fminUsable = fminUsable && ps3082VersionOK(pass) && pass.Pkg.Scope().Lookup("min") == nil
	}
	usable := map[string]bool{"Max": fmaxUsable, "Min": fminUsable}
	placed := map[string]bool{"Max": fmaxDeclared, "Min": fminDeclared}
	for _, f := range pass.Files {
		var findings []ps3082Finding
		astutil.WithStack(f, func(n ast.Node, stack []ast.Node) bool {
			expr, ok := n.(ast.Expr)
			if !ok {
				return true
			}
			name, call, ok := isMathMinMax(pass.TypesInfo, expr)
			if !ok {
				return true
			}
			// Clamps belong to PS3077: skip a call that wraps the
			// opposite call, and skip a call directly wrapped by one.
			for _, arg := range call.Args {
				if inner, _, ok := isMathMinMax(pass.TypesInfo, arg); ok && inner != name {
					return true
				}
			}
			if len(stack) > 0 {
				if outerName, _, ok := isMathMinMax(pass.TypesInfo, exprOrNil(stack[len(stack)-1])); ok && outerName != name {
					return true
				}
			}
			if _, inLoop := astutil.InLoop(stack); !inLoop {
				return true
			}
			fix := usable[name] && ps3082Fixable(pass, name, call)
			findings = append(findings, ps3082Finding{name: name, call: call, fix: fix})
			return true
		})
		fixable := 0
		needs := make(map[string]bool, len(findings))
		for _, fd := range findings {
			if fd.fix {
				fixable++
				needs[fd.name] = true
			}
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
		}
		helperCarried := false
		for _, fd := range findings {
			diag := analysis.Diagnostic{
				Pos:     fd.call.Pos(),
				End:     fd.call.End(),
				Message: "math." + fd.name + " in a loop pays a function call per iteration; the " + map[string]string{"Min": "min", "Max": "max"}[fd.name] + " builtin is one instruction but differs on NaN-vs-Inf pairs — use a NaN-correct wrapper and gate with planted edge values",
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
						text = append(text, ps3082FmaxText...)
						placed["Max"] = true
					}
					if needs["Min"] && !placed["Min"] {
						text = append(text, ps3082FminText...)
						placed["Min"] = true
					}
					edits = append(edits, analysis.TextEdit{Pos: f.End(), End: f.End(), NewText: text})
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

// ps3082Fixable reports whether call is provably a math.Min/math.Max call
// over two side-effect-free float64 operands whose wrapper name is free at
// the call site — the exact shape the selector rewrite is proven for.
func ps3082Fixable(pass *analysis.Pass, name string, call *ast.CallExpr) bool {
	if len(call.Args) != 2 || !ps3082IsMathFunc(pass, call, name) {
		return false
	}
	for _, arg := range call.Args {
		if _, ok := ps3077OperandText(pass, arg); !ok {
			return false
		}
	}
	return ps3082NameFree(pass, ps3082Wrapper[name], call.Pos())
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

// ps3082HelperStatus reports whether name (psFmax or psFmin) may be
// referenced by a fix and whether the package already declares it. A
// package-level declaration is only reused when it is recognizably THIS
// check's helper (marker comment from a previous fix run); any other
// declaration of the name disables the fix.
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
			if fd.Doc != nil && strings.Contains(fd.Doc.Text(), "perfscan PS3082") {
				return true, true
			}
			return false, true
		}
	}
	return false, true
}

func exprOrNil(n ast.Node) ast.Expr {
	e, _ := n.(ast.Expr)
	return e
}
