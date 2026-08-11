package checks

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS1007 reports an inner loop accumulating into a loop-INVARIANT output
// row — all of OUT is loaded and stored once per OUTER step instead of
// once in total.
var PS1007 = register(&lint.Check{
	ID:       "PS1007",
	Category: "access",
	Slug:     "output-row-restreamed",
	Level:    lint.LevelAggressive,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "an inner loop accumulating into an output row that does not vary with the outer loop",
		Text: `OUT[d] += f(outer)*IN[…] re-streams all of OUT once per outer
step: per element that is three memory ops (load IN, load OUT, store OUT)
times the outer trip count.

TWO remedies, and WHICH one depends on whether IN is already contiguous in
the inner variable — both bit-identical when OUT enters zeroed, since each
element still sums the outer variable ascending:

(a) IN is NOT contiguous in d (a d-strided gather): strip-mine the d loop
by 4 and hold four partial sums in registers with the outer loop innermost.

(b) IN IS contiguous in d (a row-major rank-1 update): do NOT strip-mine —
that trades one contiguous pass for strided ones and its gain decays with
the outer trip count. Instead unroll the OUTER loop by 2 with separate
accumulating adds, so OUT[d] stays in a register across the pair.

(c) SEVERAL output rows already accumulated together (a band kernel):
(b)'s refusal does not transfer — strip-mining there is a register TILE
and the removed output traffic dominates. The separating quantity is
arithmetic intensity: one output row gives 4 FMAs per 5 loads, four rows
give 16 per 8.

L3 (aggressive): rank by the loop's share of runtime × the achievable
unroll factor, not by cache residency — the saving is a fixed fraction of
the loop's own traffic either way.`,
		Before: `for i := 0; i < n; i++ {
	v := w[i]
	for d := 0; d < dim; d++ {
		out[d] += v * in[i*dim+d] // out fully re-streamed per i
	}
}`,
		After: `for i := 0; i+1 < n; i += 2 { // case (b): outer unroll, separate adds
	v0, v1 := w[i], w[i+1]
	r0, r1 := in[i*dim:(i+1)*dim], in[(i+1)*dim:(i+2)*dim]
	for d := 0; d < dim; d++ {
		out[d] += v0 * r0[d]
		out[d] += v1 * r1[d] // out[d] stays in a register across the pair
	}
}
// (serial tail for odd n)`,
		MeasuredWin: "reference corpus: case (a) sparse P·V 1.24x; case (b) QR outer-unroll geomean -2.69%, SnapKV -21.08% at U=4; case (c) band kernels -31..-50% with the gain growing with size",
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS1007",
		Doc:  "inner loop accumulates into an outer-invariant output row",
		Run:  runPS1007,
	},
})

func runPS1007(pass *analysis.Pass) (any, error) {
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
			var hit *ast.IndexExpr
			ast.Inspect(body, func(m ast.Node) bool {
				if hit != nil {
					return false
				}
				as, ok := m.(*ast.AssignStmt)
				if !ok || as.Tok != token.ADD_ASSIGN || len(as.Lhs) != 1 || len(as.Rhs) != 1 {
					return true
				}
				ix, ok := as.Lhs[0].(*ast.IndexExpr)
				if !ok {
					return true
				}
				// The output slot varies with the inner variable only —
				// the row is outer-invariant.
				if exprMentions(ix, ov) || !exprMentions(ix.Index, iv) {
					return true
				}
				// The added term must involve the outer iteration (a
				// weight or a row read varying with it) — otherwise the
				// accumulation is a plain per-element sum.
				if !exprMentions(as.Rhs[0], ov) && !rhsMentionsOuterLocal(as.Rhs[0], outerLoop, body) {
					return true
				}
				hit = ix
				return false
			})
			if hit == nil {
				return true
			}
			diag := analysis.Diagnostic{
				Pos:     hit.Pos(),
				End:     hit.End(),
				Message: fmt.Sprintf("this inner loop accumulates into an output row invariant in outer variable %s — all of it is loaded and stored once per outer step; the remedy depends on whether the input is contiguous in %s (strip-mine the gather, outer-unroll the rank-1 update, register-tile a band) — see PS1007 docs", ov, iv),
			}
			if fix := ps1007Fix(pass, outerLoop, n); fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*fix}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}

// rhsMentionsOuterLocal reports whether the accumulated term mentions a
// local bound in the OUTER loop body before the inner loop (v := w[i]
// hoisted by hand) — the shape still varies per outer step.
func rhsMentionsOuterLocal(rhs ast.Expr, outerLoop ast.Node, innerBody *ast.BlockStmt) bool {
	outerBody := astutil.LoopBody(outerLoop)
	if outerBody == nil {
		return false
	}
	locals := map[string]bool{}
	ast.Inspect(outerBody, func(n ast.Node) bool {
		if n == ast.Node(innerBody) {
			return false // stop at the inner loop
		}
		if as, ok := n.(*ast.AssignStmt); ok && as.Tok == token.DEFINE {
			for _, lhs := range as.Lhs {
				if id, ok := lhs.(*ast.Ident); ok && id.Name != "_" {
					locals[id.Name] = true
				}
			}
		}
		return true
	})
	return mentionsAnyOf(rhs, locals)
}

// ps1007Fix builds the outer-unroll-by-2 rewrite (case (b): row-major rank-1
// update) for the exact canonical shape and nothing else:
//
//	for i := 0; i < n; i++ {
//		v := w[i]
//		for d := 0; d < dim; d++ {
//			out[d] += v * in[i*dim+d]
//		}
//	}
//
// or the un-hoisted variant whose accumulated term is w[i] * in[i*dim+d],
// with every name a plain identifier, out/w/in plain slices, and n/dim
// side-effect-free expressions free of both loop variables. The replacement
// unrolls the OUTER loop by 2 with SEPARATE accumulating adds — each out[d]
// still receives its terms in ascending i order, so the sums are
// bit-identical — and a serial tail loop (sharing the hoisted index
// variable) handles odd n. It indexes in directly rather than through the
// Doc.After row subslices: taking in[i*dim:(i+1)*dim] would evaluate slice
// bounds the original never computes (a fresh panic when dim < 0, or a
// silent read past len via cap), whereas direct indexing performs exactly
// the original element accesses. The pair reads w[i+1] and in[(i+1)*dim+d]
// before the original iteration boundary, so the fix is refused when the
// written row shares a name with a read operand. Any deviation from the
// canonical shape returns nil and the diagnostic stays advisory.
func ps1007Fix(pass *analysis.Pass, outer, inner ast.Node) *analysis.SuggestedFix {
	ol, ok := outer.(*ast.ForStmt)
	if !ok {
		return nil
	}
	il, ok := inner.(*ast.ForStmt)
	if !ok {
		return nil
	}
	i, nBound, ok := ps6010CountedHeader(ol)
	if !ok {
		return nil
	}
	d, dimBound, ok := ps6010CountedHeader(il)
	if !ok || d == i {
		return nil
	}
	n := simpleExprText(nBound)
	dim := simpleExprText(dimBound)
	if n == "" || dim == "" ||
		exprMentions(nBound, i) || exprMentions(nBound, d) ||
		exprMentions(dimBound, i) || exprMentions(dimBound, d) {
		return nil
	}
	// Outer body: exactly { v := w[i]; <inner loop> } or { <inner loop> }.
	v := ""
	var wID *ast.Ident
	switch len(ol.Body.List) {
	case 2:
		hoist, ok := ol.Body.List[0].(*ast.AssignStmt)
		if !ok || hoist.Tok != token.DEFINE || len(hoist.Lhs) != 1 || len(hoist.Rhs) != 1 {
			return nil
		}
		vID, ok := hoist.Lhs[0].(*ast.Ident)
		if !ok || vID.Name == "_" {
			return nil
		}
		wIx, ok := hoist.Rhs[0].(*ast.IndexExpr)
		if !ok {
			return nil
		}
		wID, ok = wIx.X.(*ast.Ident)
		if !ok {
			return nil
		}
		if ix, ok := wIx.Index.(*ast.Ident); !ok || ix.Name != i {
			return nil
		}
		if ol.Body.List[1] != ast.Stmt(il) {
			return nil
		}
		v = vID.Name
	case 1:
		if ol.Body.List[0] != ast.Stmt(il) {
			return nil
		}
	default:
		return nil
	}
	// Inner body: exactly { out[d] += TERM * in[i*dim+d] }.
	if len(il.Body.List) != 1 {
		return nil
	}
	as, ok := il.Body.List[0].(*ast.AssignStmt)
	if !ok || as.Tok != token.ADD_ASSIGN || len(as.Lhs) != 1 || len(as.Rhs) != 1 {
		return nil
	}
	outIx, ok := as.Lhs[0].(*ast.IndexExpr)
	if !ok {
		return nil
	}
	outID, ok := outIx.X.(*ast.Ident)
	if !ok {
		return nil
	}
	if ix, ok := outIx.Index.(*ast.Ident); !ok || ix.Name != d {
		return nil
	}
	mul, ok := as.Rhs[0].(*ast.BinaryExpr)
	if !ok || mul.Op != token.MUL {
		return nil
	}
	// Left factor: the hoisted v, or w[i] directly in the un-hoisted variant.
	if v != "" {
		if id, ok := mul.X.(*ast.Ident); !ok || id.Name != v {
			return nil
		}
	} else {
		wIx, ok := mul.X.(*ast.IndexExpr)
		if !ok {
			return nil
		}
		wID, ok = wIx.X.(*ast.Ident)
		if !ok {
			return nil
		}
		if ix, ok := wIx.Index.(*ast.Ident); !ok || ix.Name != i {
			return nil
		}
	}
	inIx, ok := mul.Y.(*ast.IndexExpr)
	if !ok {
		return nil
	}
	inID, ok := inIx.X.(*ast.Ident)
	if !ok {
		return nil
	}
	sum, ok := inIx.Index.(*ast.BinaryExpr)
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
	if simpleExprText(stride.Y) != dim {
		return nil
	}
	if sy, ok := sum.Y.(*ast.Ident); !ok || sy.Name != d {
		return nil
	}
	out, w, in := outID.Name, wID.Name, inID.Name
	// The unrolled pair reads w[i+1] and in[(i+1)*dim+d] before the writes
	// the original interleaves; refuse when the written row shares a name
	// with a read operand.
	if out == w || out == in {
		return nil
	}
	// All three operands must be plain slices — no maps, strings, or arrays.
	for _, id := range []*ast.Ident{outID, wID, inID} {
		t := pass.TypesInfo.TypeOf(id)
		if t == nil {
			return nil
		}
		if _, ok := t.Underlying().(*types.Slice); !ok {
			return nil
		}
	}
	// The hoisted name may shadow something the templated body resolves at
	// outer scope (the unrolled half drops the shadow).
	if v != "" {
		for _, name := range []string{i, d, out, w, in, rootIdentName(nBound), rootIdentName(dimBound)} {
			if v == name {
				return nil
			}
		}
	}
	pos := pass.Fset.Position(ol.Pos())
	po := fmt.Sprintf("psI%d", pos.Line)
	// Names introduced by the rewrite must not capture anything the
	// templated body references.
	fresh := map[string]bool{po: true, "v0": true, "v1": true}
	for _, name := range []string{i, d, v, out, w, in, rootIdentName(nBound), rootIdentName(dimBound)} {
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
	wf("%sfor ; %s+1 < %s; %s += 2 {\n", ind, po, n, po)
	wf("%s\tv0, v1 := %s[%s], %s[%s+1]\n", ind, w, po, w, po)
	wf("%s\tfor %s := 0; %s < %s; %s++ {\n", ind, d, d, dim, d)
	wf("%s\t\t%s[%s] += v0 * %s[%s*%s+%s]\n", ind, out, d, in, po, dim, d)
	wf("%s\t\t%s[%s] += v1 * %s[(%s+1)*%s+%s]\n", ind, out, d, in, po, dim, d)
	wf("%s\t}\n", ind)
	wf("%s}\n", ind)
	wf("%sfor ; %s < %s; %s++ {\n", ind, po, n, po)
	if v != "" {
		wf("%s\t%s := %s[%s]\n", ind, v, w, po)
		wf("%s\tfor %s := 0; %s < %s; %s++ {\n", ind, d, d, dim, d)
		wf("%s\t\t%s[%s] += %s * %s[%s*%s+%s]\n", ind, out, d, v, in, po, dim, d)
		wf("%s\t}\n", ind)
	} else {
		wf("%s\tfor %s := 0; %s < %s; %s++ {\n", ind, d, d, dim, d)
		wf("%s\t\t%s[%s] += %s[%s] * %s[%s*%s+%s]\n", ind, out, d, w, po, in, po, dim, d)
		wf("%s\t}\n", ind)
	}
	wf("%s}", ind)
	return &analysis.SuggestedFix{
		Message: "unroll the outer loop by 2 with separate accumulating adds (serial tail)",
		TextEdits: []analysis.TextEdit{
			{Pos: ol.Pos(), End: ol.End(), NewText: []byte(b.String())},
		},
	}
}
