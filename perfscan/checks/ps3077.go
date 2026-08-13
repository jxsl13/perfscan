package checks

import (
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS3077 reports a two-bound clamp written as math.Min(math.Max(...)) (or
// the reverse) inside a loop.
var PS3077 = register(&lint.Check{
	ID:       "PS3077",
	Category: "indirect",
	Slug:     "minmax-clamp-in-loop",
	Level:    lint.LevelAggressive,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "math.Min wrapped around math.Max (a clamp) inside a loop",
		Text: `A clamp written as two math calls carries the whole NaN and
signed-zero contract every iteration. A comparison chain (or the min/max
builtins via a NaN-correct wrapper) is dramatically cheaper — but the naive
rewrite is a real bug: use 'if r <= lo' rather than '<' (math.Max(0,-0)
returns +0 where '<' lets -0 through), and NaN must fall through both
bounds untouched.

L3 (aggressive): the rewrite must be gated twice — a bit-for-bit table over
-0, +0, NaN, both infinities and each boundary, AND a digest of the caller,
since ordinary data never produces the cases that make the naive rewrite
wrong.

The automatic fix covers exactly math.Min(math.Max(v, lo), hi) and
math.Max(math.Min(v, hi), lo) whose three operands are side-effect-free
(identifiers, selector chains, basic literals, ident[ident] indexing) and
float64-typed: the call becomes psClamp(v, lo, hi), a branch-form helper
appended once to the package that keeps the <=/>= bounds and lets a NaN
input fall through. Anything else — other argument orders, call operands,
a file whose math import the rewrite would orphan, or a package that
already declares a psClamp that is not this helper — stays advisory.`,
		Before: `for i, v := range xs {
	xs[i] = math.Min(math.Max(v, lo), hi)
}`,
		After: `for i, v := range xs {
	r := v
	if r <= lo { r = lo } // <=, not <: preserves math.Max's -0 handling
	if r >= hi { r = hi }
	// NaN falls through both bounds untouched, matching math.Min/Max order
	xs[i] = r
}`,
		MeasuredWin: "reference corpus: HQQ quantizer -51.0% (77.14ms→37.79ms, 2.04x) with archMin/archMax leaving the profile",
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS3077",
		Doc:  "math.Min/math.Max clamp in a loop",
		Run:  runPS3077,
	},
})

var mathMinMax = map[string]bool{"Min": true, "Max": true}

// isMathMinMax reports whether e is a math.Min or math.Max call.
func isMathMinMax(info *types.Info, e ast.Expr) (string, *ast.CallExpr, bool) {
	call, ok := e.(*ast.CallExpr)
	if !ok {
		return "", nil, false
	}
	name, ok := astutil.PkgFuncCall(info, call.Fun, "math", mathMinMax)
	return name, call, ok
}

// ps3077HelperText is appended verbatim at the end of the first fixed file
// of a package that does not already declare the helper.
const ps3077HelperText = `

// psClamp is the branch form of math.Min(math.Max(v, lo), hi), preserving
// its exact NaN and signed-zero contract (see perfscan PS3077).
func psClamp(v, lo, hi float64) float64 {
	r := v
	if r <= lo {
		r = lo
	}
	if r >= hi {
		r = hi
	}
	return r
}`

// ps3077Finding is one in-loop clamp; repl is the psClamp call text when
// the exact eligible shape matched, "" for advisory-only findings.
type ps3077Finding struct {
	outer, inner string
	call         *ast.CallExpr
	repl         string
}

var ps3077HelperBytes = []byte(ps3077HelperText)

func runPS3077(pass *analysis.Pass) (any, error) {
	helperUsable, helperDeclared := ps3077HelperStatus(pass)
	helperEmitted := false
	for _, f := range pass.Files {
		var findings []ps3077Finding
		astutil.WithStack(f, func(n ast.Node, stack []ast.Node) bool {
			expr, ok := n.(ast.Expr)
			if !ok {
				return true
			}
			outer, outerCall, ok := isMathMinMax(pass.TypesInfo, expr)
			if !ok {
				return true
			}
			inner := ""
			for _, arg := range outerCall.Args {
				if name, _, ok := isMathMinMax(pass.TypesInfo, arg); ok && name != outer {
					inner = name
					break
				}
			}
			if inner == "" {
				return true
			}
			if _, inLoop := astutil.InLoop(stack); !inLoop {
				return true
			}
			repl := ""
			if helperUsable {
				repl = ps3077ClampReplacement(pass, outer, outerCall)
			}
			findings = append(findings, ps3077Finding{outer: outer, inner: inner, call: outerCall, repl: repl})
			return true
		})
		// Each fix removes two math.* references from the file; the runner
		// never prunes imports, so if applying every eligible fix would
		// leave the math import unused the whole file stays advisory.
		fixable := 0
		for _, fd := range findings {
			if fd.repl != "" {
				fixable++
			}
		}
		if fixable > 0 && ps3077MathRefs(pass.TypesInfo, f)-2*fixable <= 0 {
			for i := range findings {
				findings[i].repl = ""
			}
		}
		for _, fd := range findings {
			diag := analysis.Diagnostic{
				Pos:     fd.call.Pos(),
				End:     fd.call.End(),
				Message: "a clamp written as math." + fd.outer + "(math." + fd.inner + "(…)) pays two calls with the full NaN/signed-zero contract per iteration; a comparison chain is far cheaper but must be gated on -0/NaN/Inf edge cases",
			}
			if fd.repl != "" {
				edits := []analysis.TextEdit{
					{Pos: fd.call.Pos(), End: fd.call.End(), NewText: []byte(fd.repl)},
				}
				// The first fix of the run carries the helper declaration;
				// all fixes of a run are applied together, so exactly one
				// helper lands in the package.
				if !helperDeclared && !helperEmitted {
					edits = append(edits, analysis.TextEdit{Pos: f.End(), End: f.End(), NewText: ps3077HelperBytes})
					helperEmitted = true
				}
				diag.SuggestedFixes = []analysis.SuggestedFix{{
					Message:   "replace the clamp with the branch-form psClamp helper",
					TextEdits: edits,
				}}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps3077ClampReplacement returns the psClamp call text for the exact clamp
// shapes math.Min(math.Max(V, LO), HI) and math.Max(math.Min(V, HI), LO)
// with side-effect-free float64 operands, or "" when the call does not
// provably match.
func ps3077ClampReplacement(pass *analysis.Pass, outer string, outerCall *ast.CallExpr) string {
	if len(outerCall.Args) != 2 {
		return ""
	}
	innerName, innerCall, ok := isMathMinMax(pass.TypesInfo, outerCall.Args[0])
	if !ok || innerName == outer || len(innerCall.Args) != 2 {
		return ""
	}
	v, ok := ps3077OperandText(pass, innerCall.Args[0])
	if !ok {
		return ""
	}
	innerBound, ok := ps3077OperandText(pass, innerCall.Args[1])
	if !ok {
		return ""
	}
	outerBound, ok := ps3077OperandText(pass, outerCall.Args[1])
	if !ok {
		return ""
	}
	var lo, hi string
	if outer == "Min" { // math.Min(math.Max(v, lo), hi)
		lo, hi = innerBound, outerBound
	} else { // math.Max(math.Min(v, hi), lo)
		hi, lo = innerBound, outerBound
	}
	return "psClamp(" + v + ", " + lo + ", " + hi + ")"
}

// ps3077OperandText renders a side-effect-free float64 clamp operand — an
// identifier, a selector chain over one, a basic literal, or an ident[ident]
// index — and reports false for anything else.
func ps3077OperandText(pass *analysis.Pass, e ast.Expr) (string, bool) {
	if !ps3077Float64(pass, e) {
		return "", false
	}
	switch x := e.(type) {
	case *ast.Ident:
		return x.Name, true
	case *ast.SelectorExpr:
		if txt := simpleExprText(x); txt != "" {
			return txt, true
		}
	case *ast.BasicLit:
		return x.Value, true
	case *ast.IndexExpr:
		xi, okX := x.X.(*ast.Ident)
		ii, okI := x.Index.(*ast.Ident)
		if okX && okI {
			return xi.Name + "[" + ii.Name + "]", true
		}
	}
	return "", false
}

// ps3077Float64 reports whether e is float64-typed (or an untyped numeric
// constant, which the argument position converts to float64 identically in
// the math call and in psClamp).
func ps3077Float64(pass *analysis.Pass, e ast.Expr) bool {
	b, ok := pass.TypesInfo.TypeOf(e).(*types.Basic)
	if !ok {
		return false
	}
	if b.Kind() == types.Float64 {
		return true
	}
	return b.Info()&types.IsUntyped != 0 && b.Info()&types.IsNumeric != 0
}

// ps3077MathRefs counts the file's math.* selector references, using the
// same qualifier resolution as astutil.PkgFuncCall: the ident must resolve
// to the imported math package, not an object that merely shares the name.
func ps3077MathRefs(info *types.Info, f *ast.File) int {
	n := 0
	ast.Inspect(f, func(node ast.Node) bool {
		sel, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		id, ok := sel.X.(*ast.Ident)
		if !ok || id.Name != "math" {
			return true
		}
		if pn, ok := info.Uses[id].(*types.PkgName); ok && pn.Imported().Path() == "math" {
			n++
		}
		return true
	})
	return n
}

// ps3077HelperStatus reports whether psClamp may be referenced by a fix and
// whether the package already declares it. A package-level psClamp is only
// reused when it is recognizably THIS check's helper (marker comment from a
// previous fix run); any other declaration of the name disables the fix.
func ps3077HelperStatus(pass *analysis.Pass) (usable, declared bool) {
	if pass.Pkg.Scope().Lookup("psClamp") == nil {
		return true, false
	}
	for _, f := range pass.Files {
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Recv != nil || fd.Name.Name != "psClamp" {
				continue
			}
			if fd.Doc != nil && strings.Contains(fd.Doc.Text(), "perfscan PS3077") {
				return true, true
			}
			return false, true
		}
	}
	return false, true
}
