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

// PS1006 reports a reduction whose INNER loop variable is the high-stride
// (multiplied) part of a flat index while the outer variable is the
// contiguous (additive) part.
var PS1006 = register(&lint.Check{
	ID:       "PS1006",
	Category: "access",
	Slug:     "strided-inner-reduction",
	Level:    lint.LevelStructured,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "a reduction striding a flat array by the inner loop variable",
		Text: `ARR[inner*stride + outer] strides ARR by stride every inner
step — cache-thrashing when stride × working set exceeds L2. Interchange
the loops so ARR is walked contiguously; per output element the reduction
keeps its order, so the result is bit-identical.

The win SCALES WITH stride × working-set: big when it exceeds L2, ~noise
when L1-resident — rank candidates by the strided dimension's size. When
the strided access sits inside a fused scan that cannot be interchanged,
the remedy is GATHER/SCATTER: copy the column into contiguous scratch
once, scan the scratch, scatter back once — same bit-exact remedy.`,
		Before: `for c := 0; c < cols; c++ {
	s := 0.0
	for r := 0; r < rows; r++ {
		s += a[r*cols+c] // strides by cols every step
	}
	out[c] = s
}`,
		After: `sums := make([]float64, cols)
for r := 0; r < rows; r++ { // contiguous walk
	base := r * cols
	for c := 0; c < cols; c++ {
		sums[c] += a[base+c]
	}
}
copy(out, sums)`,
		MeasuredWin: "reference corpus: MLA value-mix 1.13x/1.27x, spectral-norm power-iter 2.57x; WKV backward 2.2–2.9x via the gather/scatter form",
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS1006",
		Doc:  "inner loop strides a flat array",
		Run:  runPS1006,
	},
})

func runPS1006(pass *analysis.Pass) (any, error) {
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
			// A read ARR[iv*s + ov-part]: the inner variable multiplied,
			// the outer variable additive.
			var hit *ast.IndexExpr
			ast.Inspect(body, func(m ast.Node) bool {
				if hit != nil {
					return false
				}
				ix, ok := m.(*ast.IndexExpr)
				if !ok {
					return true
				}
				if stridedByInner(ix.Index, iv, ov) {
					hit = ix
					return false
				}
				return true
			})
			if hit == nil {
				return true
			}
			// Must be a reduction: an accumulation into a target free of
			// the inner variable.
			reduces := false
			ast.Inspect(body, func(m ast.Node) bool {
				as, ok := m.(*ast.AssignStmt)
				if !ok || as.Tok != token.ADD_ASSIGN && as.Tok != token.SUB_ASSIGN {
					return true
				}
				for _, lhs := range as.Lhs {
					if !exprMentions(lhs, iv) {
						reduces = true
						return false
					}
				}
				return true
			})
			if !reduces {
				return true
			}
			diag := analysis.Diagnostic{
				Pos:     hit.Pos(),
				End:     hit.End(),
				Message: "the inner loop variable is the multiplied (high-stride) part of this flat index — the array is strided every inner step; interchange the loops so it is walked contiguously (bit-identical per output element), or gather into contiguous scratch when the scan cannot be interchanged",
			}
			if fix := ps1006InterchangeFix(pass, f, outerLoop, n); fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*fix}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}

// stridedByInner matches iv*s + <term with ov> / <term> + iv*s: the inner
// variable inside a product, an additive sibling mentioning the outer
// variable but not the inner one.
func stridedByInner(e ast.Expr, iv, ov string) bool {
	be, ok := e.(*ast.BinaryExpr)
	if !ok || be.Op != token.ADD {
		return false
	}
	isMulWithIv := func(x ast.Expr) bool {
		m, ok := x.(*ast.BinaryExpr)
		return ok && m.Op == token.MUL && exprMentions(m, iv)
	}
	additiveOK := func(x ast.Expr) bool {
		return exprMentions(x, ov) && !exprMentions(x, iv)
	}
	return (isMulWithIv(be.X) && additiveOK(be.Y)) || (isMulWithIv(be.Y) && additiveOK(be.X))
}

// ps1006InterchangeFix builds the loop-interchange rewrite for the exact
// canonical column-reduction shape and nothing wider:
//
//	for C := 0; C < COLS; C++ {
//		S := 0.0 // or 0
//		for R := 0; R < ROWS; R++ {
//			S += A[R*COLS+C]
//		}
//		OUT[C] = S
//	}
//
// with A and OUT plain identifiers and COLS/ROWS side-effect-free simple
// expressions (identifier or selector chain). The replacement accumulates
// into a scratch slice while walking A contiguously; per output element the
// accumulation order is unchanged, so the result is bit-identical:
//
//	psSumsN := make([]T, COLS)
//	for R := 0; R < ROWS; R++ {
//		psBase := R * COLS
//		for C := 0; C < COLS; C++ {
//			psSumsN[C] += A[psBase+C]
//		}
//	}
//	copy(OUT[:COLS], psSumsN)
//
// Extra statements, non-zero seeds, range loops, or non-ident arrays keep
// the plain advisory report.
func ps1006InterchangeFix(pass *analysis.Pass, file *ast.File, outerNode, innerNode ast.Node) *analysis.SuggestedFix {
	outer, ok := outerNode.(*ast.ForStmt)
	if !ok {
		return nil
	}
	cVar, colsExpr := ps1006CountedLoop(outer)
	cols := simpleExprText(colsExpr)
	if cVar == "" || cols == "" || len(outer.Body.List) != 3 {
		return nil
	}
	inner, ok := outer.Body.List[1].(*ast.ForStmt)
	if !ok || ast.Node(inner) != innerNode {
		return nil
	}
	rVar, rowsExpr := ps1006CountedLoop(inner)
	rows := simpleExprText(rowsExpr)
	if rVar == "" || rows == "" || len(inner.Body.List) != 1 {
		return nil
	}
	// S := 0.0 (or S := 0): a zero seed, so make()'s zero value matches.
	seed, ok := outer.Body.List[0].(*ast.AssignStmt)
	if !ok || seed.Tok != token.DEFINE || len(seed.Lhs) != 1 || len(seed.Rhs) != 1 {
		return nil
	}
	sID, ok := seed.Lhs[0].(*ast.Ident)
	if !ok {
		return nil
	}
	zero, ok := seed.Rhs[0].(*ast.BasicLit)
	if !ok || (zero.Value != "0" && zero.Value != "0.0") {
		return nil
	}
	// S += A[R*COLS+C]
	acc, ok := inner.Body.List[0].(*ast.AssignStmt)
	if !ok || acc.Tok != token.ADD_ASSIGN || len(acc.Lhs) != 1 || len(acc.Rhs) != 1 {
		return nil
	}
	if lhs, ok := acc.Lhs[0].(*ast.Ident); !ok || lhs.Name != sID.Name {
		return nil
	}
	ix, ok := acc.Rhs[0].(*ast.IndexExpr)
	if !ok {
		return nil
	}
	arrID, ok := ix.X.(*ast.Ident)
	if !ok || !ps1006StrideIndex(ix.Index, rVar, cols, cVar) {
		return nil
	}
	// OUT[C] = S
	fin, ok := outer.Body.List[2].(*ast.AssignStmt)
	if !ok || fin.Tok != token.ASSIGN || len(fin.Lhs) != 1 || len(fin.Rhs) != 1 {
		return nil
	}
	outIx, ok := fin.Lhs[0].(*ast.IndexExpr)
	if !ok {
		return nil
	}
	outID, ok := outIx.X.(*ast.Ident)
	if !ok {
		return nil
	}
	if idx, ok := outIx.Index.(*ast.Ident); !ok || idx.Name != cVar {
		return nil
	}
	if rhs, ok := fin.Rhs[0].(*ast.Ident); !ok || rhs.Name != sID.Name {
		return nil
	}
	// All five participating names must be pairwise distinct, and the bound
	// roots must not be any of the loop/accumulator variables (a triangular
	// bound like `r < c` would make the interchange wrong).
	names := []string{cVar, rVar, sID.Name, arrID.Name, outID.Name}
	for i := range names {
		for j := i + 1; j < len(names); j++ {
			if names[i] == names[j] {
				return nil
			}
		}
	}
	for _, bound := range []string{cols, rows} {
		root := bound
		if i := strings.IndexByte(root, '.'); i >= 0 {
			root = root[:i]
		}
		if root == cVar || root == rVar || root == sID.Name {
			return nil
		}
	}
	// Element type of the scratch slice: the accumulator's own basic type.
	tname := ""
	if obj := pass.TypesInfo.Defs[sID]; obj != nil {
		if b, ok := obj.Type().(*types.Basic); ok && b.Info()&types.IsNumeric != 0 {
			tname = b.Name()
		}
	}
	if tname == "" {
		if zero.Value != "0.0" {
			return nil
		}
		tname = "float64"
	}
	// Fresh names: line-derived scratch name, fixed psBase. Bail on any
	// user-written identifier with either name anywhere in the file.
	line := pass.Fset.Position(outer.Pos()).Line
	sums := fmt.Sprintf("psSums%d", line)
	collides := false
	ast.Inspect(file, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && (id.Name == sums || id.Name == "psBase") {
			collides = true
		}
		return !collides
	})
	if collides {
		return nil
	}
	// Assume gofmt indentation (tabs), matching the outer loop's column.
	ind := strings.Repeat("\t", pass.Fset.Position(outer.Pos()).Column-1)
	var b strings.Builder
	fmt.Fprintf(&b, "%s := make([]%s, %s)\n", sums, tname, cols)
	fmt.Fprintf(&b, "%sfor %s := 0; %s < %s; %s++ {\n", ind, rVar, rVar, rows, rVar)
	fmt.Fprintf(&b, "%s\tpsBase := %s * %s\n", ind, rVar, cols)
	fmt.Fprintf(&b, "%s\tfor %s := 0; %s < %s; %s++ {\n", ind, cVar, cVar, cols, cVar)
	fmt.Fprintf(&b, "%s\t\t%s[%s] += %s[psBase+%s]\n", ind, sums, cVar, arrID.Name, cVar)
	fmt.Fprintf(&b, "%s\t}\n", ind)
	fmt.Fprintf(&b, "%s}\n", ind)
	fmt.Fprintf(&b, "%scopy(%s[:%s], %s)", ind, outID.Name, cols, sums)
	return &analysis.SuggestedFix{
		Message: fmt.Sprintf("interchange the loops so %s is walked contiguously", arrID.Name),
		TextEdits: []analysis.TextEdit{
			{Pos: outer.Pos(), End: outer.End(), NewText: []byte(b.String())},
		},
	}
}

// ps1006CountedLoop matches `for v := 0; v < bound; v++` and returns the
// loop variable name and the bound expression.
func ps1006CountedLoop(f *ast.ForStmt) (string, ast.Expr) {
	init, ok := f.Init.(*ast.AssignStmt)
	if !ok || init.Tok != token.DEFINE || len(init.Lhs) != 1 || len(init.Rhs) != 1 {
		return "", nil
	}
	v, ok := init.Lhs[0].(*ast.Ident)
	if !ok {
		return "", nil
	}
	lit, ok := init.Rhs[0].(*ast.BasicLit)
	if !ok || lit.Kind != token.INT || lit.Value != "0" {
		return "", nil
	}
	cond, ok := f.Cond.(*ast.BinaryExpr)
	if !ok || cond.Op != token.LSS {
		return "", nil
	}
	if x, ok := cond.X.(*ast.Ident); !ok || x.Name != v.Name {
		return "", nil
	}
	post, ok := f.Post.(*ast.IncDecStmt)
	if !ok || post.Tok != token.INC {
		return "", nil
	}
	if x, ok := post.X.(*ast.Ident); !ok || x.Name != v.Name {
		return "", nil
	}
	return v.Name, cond.Y
}

// ps1006StrideIndex matches R*COLS + C (mul and add sides in either order,
// mul operands in either order): the inner variable times the outer bound,
// plus the outer variable.
func ps1006StrideIndex(e ast.Expr, rVar, cols, cVar string) bool {
	be, ok := e.(*ast.BinaryExpr)
	if !ok || be.Op != token.ADD {
		return false
	}
	mul, add := be.X, be.Y
	if _, ok := mul.(*ast.BinaryExpr); !ok {
		mul, add = be.Y, be.X
	}
	m, ok := mul.(*ast.BinaryExpr)
	if !ok || m.Op != token.MUL {
		return false
	}
	if id, ok := add.(*ast.Ident); !ok || id.Name != cVar {
		return false
	}
	x, y := m.X, m.Y
	if id, ok := x.(*ast.Ident); ok && id.Name == rVar {
		return simpleExprText(y) == cols
	}
	if id, ok := y.(*ast.Ident); ok && id.Name == rVar {
		return simpleExprText(x) == cols
	}
	return false
}
