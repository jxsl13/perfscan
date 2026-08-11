package checks

import (
	"fmt"
	"go/ast"
	"go/token"
	"slices"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS3083 reports an integer-keyed map built fresh inside a loop and probed
// in a nested loop.
var PS3083 = register(&lint.Check{
	ID:       "PS3083",
	Category: "indirect",
	Slug:     "int-keyed-map-built-per-pass",
	Level:    lint.LevelAggressive,
	Doc: lint.Documentation{
		Title: "an integer-keyed map allocated per outer iteration and probed in a nested loop",
		Text: `Every probe is a hash and a bucket walk, and every outer pass
allocates a fresh map. When the keys are dense in a known range — block
indices, row indices, token ids under a vocabulary — a slice is the natural
container, and stamping it with a GENERATION COUNTER makes the per-pass
reset free instead of a clear proportional to the range:

	gen++
	stamp[key] = gen      // insert
	stamp[key] == gen     // probe

L3 (aggressive): the generation-counter idiom is easy to get subtly wrong.
Watch the length — code that bounded a loop on len(m) needs an explicit
counter, and that is only equivalent when the inserted keys are DISTINCT;
check the source of the keys before trusting it.`,
		Before: `for _, q := range queries {
	sel := make(map[int64]bool, k)
	for _, b := range topBlocks(q) { sel[b] = true }
	for _, key := range keys {
		if sel[key.block] { ... }
	}
}`,
		After: `stamp := make([]uint32, numBlocks)
gen := uint32(0)
for _, q := range queries {
	gen++
	for _, b := range topBlocks(q) { stamp[b] = gen }
	for _, key := range keys {
		if stamp[key.block] == gen { ... }
	}
}`,
		MeasuredWin: "reference corpus: MoBA top-K block selection -9.3% time, allocations -49.8% (20533→10301), mapaccess1_fast64 was 5.4% of the profile before",
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS3083",
		Doc:  "int-keyed map allocated per pass and probed in a nested loop",
		Run:  runPS3083,
	},
})

func runPS3083(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		astutil.WithStack(f, func(n ast.Node, stack []ast.Node) bool {
			as, ok := n.(*ast.AssignStmt)
			if !ok || len(as.Lhs) != 1 || len(as.Rhs) != 1 {
				return true
			}
			id, ok := as.Lhs[0].(*ast.Ident)
			if !ok {
				return true
			}
			mk, ok := as.Rhs[0].(*ast.CallExpr)
			if !ok || astutil.CalleeName(mk.Fun) != "make" || len(mk.Args) == 0 {
				return true
			}
			if _, isMap := mk.Args[0].(*ast.MapType); !isMap {
				return true
			}
			t := pass.TypesInfo.TypeOf(as.Rhs[0])
			if t == nil || !isIntKeyMap(t) {
				return true
			}
			outer, inLoop := astutil.InLoop(stack)
			if !inLoop {
				return true
			}
			// Read (not just written) in a loop nested deeper inside the
			// same outer loop? A pure write target is the build, not a
			// probe.
			probed := false
			astutil.WithStack(astutil.LoopBody(outer), func(p ast.Node, pstack []ast.Node) bool {
				ix, ok := p.(*ast.IndexExpr)
				if !ok {
					return true
				}
				x, ok := ix.X.(*ast.Ident)
				if !ok || x.Name != id.Name {
					return true
				}
				if len(pstack) > 0 {
					if pas, ok := pstack[len(pstack)-1].(*ast.AssignStmt); ok && pas.Tok == token.ASSIGN {
						if slices.Contains(pas.Lhs, ast.Expr(ix)) {
							return true
						}
					}
				}
				if _, in := astutil.InLoop(pstack); in {
					probed = true
				}
				return true
			})
			if !probed {
				return true
			}
			pass.Report(analysis.Diagnostic{
				Pos:     as.Pos(),
				End:     as.End(),
				Message: fmt.Sprintf("int-keyed map %s is allocated every outer iteration and probed in a nested loop; for dense keys a slice with a generation-counter stamp removes the per-pass allocation and the hash per probe", id.Name),
			})
			return true
		})
	}
	return nil, nil
}
