package checks

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS6010 reports an output loop whose inner accumulator re-reads an
// operand that does not vary with the output index.
var PS6010 = register(&lint.Check{
	ID:       "PS6010",
	Category: "verify",
	Slug:     "output-invariant-operand-reload",
	Level:    lint.LevelAggressive,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "an accumulator loop re-reading an operand invariant in the output index",
		Text: `for o { for i { acc += A[i] * B[f(i,o)] } } re-streams all of A
once per output element: A[i] does not vary with o, so unrolling the OUTPUT
loop by 4 — four accumulators sharing one pass over A — amortizes that load
across four outputs (register blocking / unroll-and-jam).

L3 (aggressive): each accumulator keeps its own order, so the per-output
sums are bit-identical at any unroll factor; the tail loop handles the
remainder. This is the inline-accumulator sibling of the restreamed-row
shape (PS1007) — here the INPUT is re-read, there the OUTPUT is
re-streamed. Rank by the loop's share of runtime × the achievable factor.`,
		Before: `for o := 0; o < out; o++ {
	acc := 0.0
	for i := 0; i < n; i++ {
		acc += a[i] * w[i*out+o] // a re-streamed per output
	}
	dst[o] = acc
}`,
		After: `for o := 0; o+3 < out; o += 4 { // 4 accumulators share one pass over a
	var a0, a1, a2, a3 float64
	for i := 0; i < n; i++ {
		ai := a[i]
		a0 += ai * w[i*out+o]
		a1 += ai * w[i*out+o+1]
		a2 += ai * w[i*out+o+2]
		a3 += ai * w[i*out+o+3]
	}
	dst[o], dst[o+1], dst[o+2], dst[o+3] = a0, a1, a2, a3
}
// (serial tail for the remainder)`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6010",
		Doc:  "operand re-read per output element in an accumulator loop",
		Run:  runPS6010,
	},
})

func runPS6010(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		astutil.WithStack(f, func(n ast.Node, stack []ast.Node) bool {
			iv, body := loopVarAndBody(n)
			if iv == "" || body == nil || containsLoop(body) {
				return true
			}
			outerLoop, in := astutil.InLoop(stack)
			if !in {
				return true
			}
			ov := outermostLoopVar(outerLoop)
			if ov == "" || ov == iv {
				return true
			}
			var hit ast.Expr
			ast.Inspect(body, func(m ast.Node) bool {
				if hit != nil {
					return false
				}
				as, ok := m.(*ast.AssignStmt)
				if !ok || as.Tok != token.ADD_ASSIGN || len(as.Lhs) != 1 || len(as.Rhs) != 1 {
					return true
				}
				// Accumulator must be a scalar free of both loop vars.
				if _, ok := as.Lhs[0].(*ast.Ident); !ok {
					return true
				}
				mul, ok := as.Rhs[0].(*ast.BinaryExpr)
				if !ok || mul.Op != token.MUL {
					return true
				}
				// One factor indexed by the inner var only (invariant in
				// the output index), the other mentioning the outer var.
				check := func(x, y ast.Expr) bool {
					xi, ok := x.(*ast.IndexExpr)
					if !ok {
						return false
					}
					return exprMentions(xi.Index, iv) && !exprMentions(xi.Index, ov) && exprMentions(y, ov)
				}
				if check(mul.X, mul.Y) {
					hit = mul.X
				} else if check(mul.Y, mul.X) {
					hit = mul.Y
				}
				return hit == nil
			})
			if hit == nil {
				return true
			}
			diag := analysis.Diagnostic{
				Pos:     hit.Pos(),
				End:     hit.End(),
				Message: "this operand does not vary with the output index " + ov + " but is re-streamed once per output element; unroll the output loop by 4 with independent accumulators to amortize the load (bit-identical per output)",
			}
			if fix := ps6010Fix(pass, outerLoop, n); fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*fix}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}

// ps6010CountedHeader matches the counted loop header `for v := 0; v < BOUND;
// v++` and returns the loop variable name and the bound expression.
func ps6010CountedHeader(l *ast.ForStmt) (string, ast.Expr, bool) {
	init, ok := l.Init.(*ast.AssignStmt)
	if !ok || init.Tok != token.DEFINE || len(init.Lhs) != 1 || len(init.Rhs) != 1 {
		return "", nil, false
	}
	iv, ok := init.Lhs[0].(*ast.Ident)
	if !ok {
		return "", nil, false
	}
	zero, ok := init.Rhs[0].(*ast.BasicLit)
	if !ok || zero.Kind != token.INT || zero.Value != "0" {
		return "", nil, false
	}
	cond, ok := l.Cond.(*ast.BinaryExpr)
	if !ok || cond.Op != token.LSS {
		return "", nil, false
	}
	if lhs, ok := cond.X.(*ast.Ident); !ok || lhs.Name != iv.Name {
		return "", nil, false
	}
	post, ok := l.Post.(*ast.IncDecStmt)
	if !ok || post.Tok != token.INC {
		return "", nil, false
	}
	if pv, ok := post.X.(*ast.Ident); !ok || pv.Name != iv.Name {
		return "", nil, false
	}
	return iv.Name, cond.Y, true
}

// ps6010Fix builds the unroll-by-4 rewrite for the exact canonical matvec
// shape and nothing else:
//
//	for o := 0; o < out; o++ {
//		acc := 0.0
//		for i := 0; i < n; i++ {
//			acc += a[i] * w[i*out+o]
//		}
//		dst[o] = acc
//	}
//
// with every name a plain identifier and out/n side-effect-free invariant
// expressions. The replacement keeps each output's accumulation order, so the
// per-output sums are bit-identical; a serial tail loop (sharing the hoisted
// index variable) handles the remainder. Any deviation from the canonical
// shape returns nil and the diagnostic stays advisory.
func ps6010Fix(pass *analysis.Pass, outer, inner ast.Node) *analysis.SuggestedFix {
	ol, ok := outer.(*ast.ForStmt)
	if !ok {
		return nil
	}
	il, ok := inner.(*ast.ForStmt)
	if !ok {
		return nil
	}
	o, outBound, ok := ps6010CountedHeader(ol)
	if !ok {
		return nil
	}
	i, nBound, ok := ps6010CountedHeader(il)
	if !ok || i == o {
		return nil
	}
	out := simpleExprText(outBound)
	n := simpleExprText(nBound)
	if out == "" || n == "" ||
		exprMentions(outBound, o) || exprMentions(outBound, i) ||
		exprMentions(nBound, o) || exprMentions(nBound, i) {
		return nil
	}
	// Outer body: exactly { acc := 0.0; <inner loop>; dst[o] = acc }.
	if len(ol.Body.List) != 3 || ol.Body.List[1] != ast.Stmt(il) {
		return nil
	}
	accInit, ok := ol.Body.List[0].(*ast.AssignStmt)
	if !ok || accInit.Tok != token.DEFINE || len(accInit.Lhs) != 1 || len(accInit.Rhs) != 1 {
		return nil
	}
	accID, ok := accInit.Lhs[0].(*ast.Ident)
	if !ok {
		return nil
	}
	seed, ok := accInit.Rhs[0].(*ast.BasicLit)
	if !ok || seed.Kind != token.FLOAT || seed.Value != "0.0" {
		return nil
	}
	acc := accID.Name
	store, ok := ol.Body.List[2].(*ast.AssignStmt)
	if !ok || store.Tok != token.ASSIGN || len(store.Lhs) != 1 || len(store.Rhs) != 1 {
		return nil
	}
	dstIx, ok := store.Lhs[0].(*ast.IndexExpr)
	if !ok {
		return nil
	}
	dstID, ok := dstIx.X.(*ast.Ident)
	if !ok {
		return nil
	}
	if ix, ok := dstIx.Index.(*ast.Ident); !ok || ix.Name != o {
		return nil
	}
	if rhs, ok := store.Rhs[0].(*ast.Ident); !ok || rhs.Name != acc {
		return nil
	}
	dst := dstID.Name
	// Inner body: exactly { acc += a[i] * w[i*out+o] }.
	if len(il.Body.List) != 1 {
		return nil
	}
	as, ok := il.Body.List[0].(*ast.AssignStmt)
	if !ok || as.Tok != token.ADD_ASSIGN || len(as.Lhs) != 1 || len(as.Rhs) != 1 {
		return nil
	}
	if lhs, ok := as.Lhs[0].(*ast.Ident); !ok || lhs.Name != acc {
		return nil
	}
	mul, ok := as.Rhs[0].(*ast.BinaryExpr)
	if !ok || mul.Op != token.MUL {
		return nil
	}
	aIx, ok := mul.X.(*ast.IndexExpr)
	if !ok {
		return nil
	}
	aID, ok := aIx.X.(*ast.Ident)
	if !ok {
		return nil
	}
	if ix, ok := aIx.Index.(*ast.Ident); !ok || ix.Name != i {
		return nil
	}
	wIx, ok := mul.Y.(*ast.IndexExpr)
	if !ok {
		return nil
	}
	wID, ok := wIx.X.(*ast.Ident)
	if !ok {
		return nil
	}
	sum, ok := wIx.Index.(*ast.BinaryExpr)
	if !ok || sum.Op != token.ADD {
		return nil
	}
	stride, ok := sum.X.(*ast.BinaryExpr)
	if !ok || stride.Op != token.MUL {
		return nil
	}
	if sx, ok := stride.X.(*ast.Ident); !ok || sx.Name != i {
		return nil
	}
	if simpleExprText(stride.Y) != out {
		return nil
	}
	if oy, ok := sum.Y.(*ast.Ident); !ok || oy.Name != o {
		return nil
	}
	a, w := aID.Name, wID.Name
	// The unrolled version delays each block's dst stores past later reads
	// of a and w; refuse when dst shares a name with a read operand.
	if dst == a || dst == w {
		return nil
	}
	pos := pass.Fset.Position(ol.Pos())
	po := fmt.Sprintf("psO%d", pos.Line)
	// Names introduced by the rewrite must not capture anything the
	// templated body references.
	fresh := map[string]bool{po: true, "a0": true, "a1": true, "a2": true, "a3": true, "ai": true}
	for _, name := range []string{o, i, acc, a, w, dst, rootIdentName(outBound), rootIdentName(nBound)} {
		if fresh[name] {
			return nil
		}
	}
	// Assume gofmt indentation (tabs): the loop starts at column pos.Column,
	// i.e. pos.Column-1 tabs of indentation.
	ind := strings.Repeat("\t", pos.Column-1)
	var b strings.Builder
	wf := func(format string, args ...any) { fmt.Fprintf(&b, format, args...) }
	wf("%s := 0\n", po)
	wf("%sfor ; %s+3 < %s; %s += 4 {\n", ind, po, out, po)
	wf("%s\tvar a0, a1, a2, a3 float64\n", ind)
	wf("%s\tfor %s := 0; %s < %s; %s++ {\n", ind, i, i, n, i)
	wf("%s\t\tai := %s[%s]\n", ind, a, i)
	wf("%s\t\ta0 += ai * %s[%s*%s+%s]\n", ind, w, i, out, po)
	wf("%s\t\ta1 += ai * %s[%s*%s+%s+1]\n", ind, w, i, out, po)
	wf("%s\t\ta2 += ai * %s[%s*%s+%s+2]\n", ind, w, i, out, po)
	wf("%s\t\ta3 += ai * %s[%s*%s+%s+3]\n", ind, w, i, out, po)
	wf("%s\t}\n", ind)
	wf("%s\t%s[%s], %s[%s+1], %s[%s+2], %s[%s+3] = a0, a1, a2, a3\n", ind, dst, po, dst, po, dst, po, dst, po)
	wf("%s}\n", ind)
	wf("%sfor ; %s < %s; %s++ {\n", ind, po, out, po)
	wf("%s\t%s := 0.0\n", ind, acc)
	wf("%s\tfor %s := 0; %s < %s; %s++ {\n", ind, i, i, n, i)
	wf("%s\t\t%s += %s[%s] * %s[%s*%s+%s]\n", ind, acc, a, i, w, i, out, po)
	wf("%s\t}\n", ind)
	wf("%s\t%s[%s] = %s\n", ind, dst, po, acc)
	wf("%s}", ind)
	return &analysis.SuggestedFix{
		Message: "unroll the output loop by 4 with independent accumulators (serial tail)",
		TextEdits: []analysis.TextEdit{
			{Pos: ol.Pos(), End: ol.End(), NewText: []byte(b.String())},
		},
	}
}
