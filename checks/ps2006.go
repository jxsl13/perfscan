package checks

import (
	"go/ast"
	"go/printer"
	"go/token"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS2006 reports a cache slot reassigned to a concat of itself and a new
// row inside a loop of a per-token step function — O(T²) copy traffic.
var PS2006 = register(&lint.Check{
	ID:       "PS2006",
	Category: "alloc",
	Slug:     "quadratic-cache-append",
	Level:    lint.LevelAggressive,
	Doc: lint.Documentation{
		Title: "a per-token cache slot reassigned to a concat of itself and a new row",
		Text: `c.K[l] = concatRows(c.K[l], k) allocates a fresh [t+1, width]
buffer and recopies all t existing rows EVERY token: a T-token decode moves
O(T²) bytes and allocates O(T²). An amortized row buffer — write row t in
place into a doubling backing store, hand back a zero-copy prefix view —
makes the same sequence O(T) total.

Only fires inside functions whose name marks a per-token step (…Step,
…DecodeStep, …StepKV, …StreamStep): without that, the shape is an ordinary
one-shot concatenation and pooling it would be premature.

L3 (aggressive): the risk the fix carries is ALIASING, not numerics — the
concat hands back a fresh buffer each step while a row buffer hands back a
VIEW into a growing one. Audit for callers that retain an earlier view and
mutate it in place before switching.`,
		Before: `func (m *Model) DecodeStep(k tensor.T) {
	for l := range m.cache.K {
		m.cache.K[l] = concatRows(m.cache.K[l], k)
	}
}`,
		After: `// rowBuffer doubles its backing store and returns a prefix view:
// O(T) total instead of O(T²).
for l := range m.cache.K {
	m.cache.K[l] = m.kBuf[l].appendRow(k)
}`,
		MeasuredWin: "reference corpus: width 2048, T=512: 101x time and 104x bytes on the append microbenchmark; 500-token GPT decode 2.21GB→159MB (13.9x) and 1.32x wall clock",
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2006",
		Doc:  "cache slot reassigned to concat of itself in a per-token step",
		Run:  runPS2006,
	},
})

func perTokenStepName(name string) bool {
	for _, suffix := range []string{"DecodeStep", "StepKV", "StreamStep", "Step"} {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return false
}

func concatLikeName(name string) bool {
	switch name {
	case "concatRows", "Concat", "concat", "ConcatRows", "appendRows", "stackRows":
		return true
	}
	return false
}

func exprTextRendered(e ast.Expr) string {
	var b strings.Builder
	_ = printer.Fprint(&b, token.NewFileSet(), e)
	return b.String()
}

func runPS2006(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || fn.Name == nil || !perTokenStepName(fn.Name.Name) {
				continue
			}
			astutil.WithStack(fn.Body, func(n ast.Node, stack []ast.Node) bool {
				as, ok := n.(*ast.AssignStmt)
				if !ok || as.Tok != token.ASSIGN {
					return true
				}
				if _, inLoop := astutil.InLoop(stack); !inLoop {
					return true
				}
				for i, lhs := range as.Lhs {
					if i >= len(as.Rhs) {
						break
					}
					target, ok := lhs.(*ast.IndexExpr)
					if !ok {
						continue
					}
					call, ok := as.Rhs[i].(*ast.CallExpr)
					if !ok || len(call.Args) < 2 {
						continue
					}
					name := astutil.CalleeName(call.Fun)
					if !concatLikeName(name) {
						continue
					}
					// Render target once: it is compared here and reused in the
					// message below (exprTextRendered runs the printer + allocates).
					targetText := exprTextRendered(target)
					if exprTextRendered(call.Args[0]) != targetText {
						continue
					}
					pass.Report(analysis.Diagnostic{
						Pos:     as.Pos(),
						End:     as.End(),
						Message: targetText + " is reassigned to " + name + " of ITSELF plus a new row inside a loop of per-token step " + fn.Name.Name + " — a T-token run moves O(T²) bytes; replace with an amortized row buffer returning a zero-copy prefix view (AUDIT FOR ALIASING FIRST)",
					})
				}
				return true
			})
		}
	}
	return nil, nil
}
