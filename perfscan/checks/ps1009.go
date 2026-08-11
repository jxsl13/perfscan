package checks

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/config"
	"github.com/jxsl13/perfscan/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS1009 reports a NAMED dtype-switch case left on the per-element
// accessor while a sibling case already takes typed storage.
var PS1009 = register(&lint.Check{
	ID:          "PS1009",
	Category:    "access",
	Slug:        "unconverted-dtype-arm",
	Level:       lint.LevelStructured,
	NeedsConfig: true,
	Vocab:       []string{"elementAccessors", "fastPathHelpers", "dtypeMethods"},
	Doc: lint.Documentation{
		Title: "a named dtype-switch case on the per-element accessor beside a typed sibling case",
		Text: `A dtype switch with a typed fast arm for some dtypes and a
per-element accessor walk for others routes every workload of the
unconverted dtype through interface dispatch. PS1001 structurally cannot
report this: it suppresses on the presence of a fast path, which the
sibling arm satisfies — so a helper fast for f64 and slow for f32 reads as
already optimized.

Which arm is hot is a workload fact, not a syntax one — confirm which
dtype the traffic actually takes (a panic in the clause under the relevant
benchmark settles it in one run) before converting. Bit-identical where
the missing arm is a WIDENING (reading f32 as float64 is exact).

NARROWED per the reference's shipped experience: a switch whose named
cases are all converted and which leaves only its DEFAULT on the accessor
is the FINISHED state — what remains there (half-precision, packed
quantized storage) needs a real conversion, not a widening, and reporting
it produced pure permanent noise. A NAMED case left on the accessor still
fires: listing a dtype and not converting it is a choice, not an accepted
tail.`,
		Before: `switch t.Dtype() {
case F64:
	xs, _ := t.flatF64()
	sum(xs)
case F32: // named, but still on the accessor: fires
	n := t.Numel()
	for i := 0; i < n; i++ {
		s += t.AtF64(i)
	}
}`,
		After: `switch t.Dtype() {
case F64:
	xs, _ := t.flatF64()
	sum(xs)
case F32:
	xs, _ := t.flatF32()
	sum32(xs) // widening-exact typed arm
}`,
		MeasuredWin: "reference corpus: adding one f32 arm to a 20+-call-site helper measured geomean -6.46% on a whole quantized prefill (all cells p=0.000)",
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS1009",
		Doc:  "named dtype case on the accessor beside a typed sibling",
		Run:  runPS1009,
	},
})

func runPS1009(pass *analysis.Pass) (any, error) {
	ns := config.Current()
	if len(ns.ElementAccessors) == 0 || len(ns.FastPathHelpers) == 0 || len(ns.DtypeMethods) == 0 {
		return nil, nil
	}
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			sw, ok := n.(*ast.SwitchStmt)
			if !ok || sw.Tag == nil || !exprCallsAnyOf(sw.Tag, ns.DtypeMethods) {
				return true
			}
			// Classify the clauses.
			anyTyped := false
			type accessorClause struct {
				clause *ast.CaseClause
				name   string
			}
			namedOnAccessor := make([]accessorClause, 0, len(sw.Body.List))
			for _, stmt := range sw.Body.List {
				cc, ok := stmt.(*ast.CaseClause)
				if !ok {
					continue
				}
				typed := false
				accessor := ""
				for _, s := range cc.Body {
					ast.Inspect(s, func(m ast.Node) bool {
						call, ok := m.(*ast.CallExpr)
						if !ok {
							return true
						}
						name := astutil.CalleeName(call.Fun)
						if ns.FastPathHelpers[name] {
							typed = true
						}
						if ns.ElementAccessors[name] {
							accessor = name
						}
						return true
					})
				}
				if typed {
					anyTyped = true
				}
				// cc.List == nil is the default clause: the finished
				// state's accepted tail, never reported.
				if accessor != "" && !typed && cc.List != nil {
					namedOnAccessor = append(namedOnAccessor, accessorClause{cc, accessor})
				}
			}
			if !anyTyped {
				return true
			}
			for _, ac := range namedOnAccessor {
				pass.Report(analysis.Diagnostic{
					Pos:     ac.clause.Pos(),
					End:     ac.clause.Colon,
					Message: "this named dtype case walks ." + ac.name + " while a sibling case takes typed storage — the listed dtype routes through per-element dispatch; add its typed arm (widening-exact for narrower floats), confirming with a planted panic which dtype the workload takes",
				})
			}
			return true
		})
	}
	return nil, nil
}

// exprCallsAnyOf reports whether e contains a call to any named method.
func exprCallsAnyOf(e ast.Expr, set map[string]bool) bool {
	found := false
	ast.Inspect(e, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok && set[astutil.CalleeName(call.Fun)] {
			found = true
			return false
		}
		return true
	})
	return found
}
