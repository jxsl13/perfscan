package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS3013 reports an ascending slices.SortFunc whose comparator is a
// hand-rolled three-way if/switch chain (a<b -> negative, a>b -> positive,
// else 0) — slices.Sort spelled the slow way — and rewrites it to
// slices.Sort. Sibling of PS3107, which catches the same anti-pattern
// spelled with cmp.Compare; PS3013 catches the PRE-cmp hand-rolled
// spelling that survives in code migrated from sort.Slice.
var PS3013 = register(&lint.Check{
	ID:       "PS3013",
	Category: "indirect",
	Slug:     "sortfunc-handrolled-threeway-to-slices-sort",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "slices.SortFunc with a hand-rolled three-way comparator (a<b/a>b/-1/1/0) is slices.Sort spelled the slow way",
		Text: `slices.SortFunc(s, func(a, b T) int { if a < b { return -1 };
if a > b { return 1 }; return 0 }) sorts ascending — exactly what
slices.Sort(s) does — but pays for it twice per comparison: the
comparator is a func value invoked through an indirect call on every one
of the O(n log n) comparisons, and its body performs up to TWO
relational comparisons (a<b, then a>b) to synthesize a -1/0/1 sign that
the sort only ever consumes as a bool. slices.Sort is a distinct
monomorphized entry point that inlines a SINGLE ordered '<' (via
cmp.Less) directly into pdqsort: same algorithm, zero comparator
indirection, one comparison instead of up to two.

The result is BIT-IDENTICAL for the element types the fix accepts —
exactly PS3107's policy: the fix is offered only when the element type's
underlying type is an integer or string kind. For those types the
if-chain returns a value whose SIGN equals cmp.Compare(a, b) on every
pair (a<b -> negative, a>b -> positive, equal -> 0), and slices.SortFunc
consumes only that sign; slices.Sort is defined via cmp.Less with
cmp.Compare(a, b) < 0 iff cmp.Less(a, b), so both sides answer every
comparison the same and produce the same permutation. Because equal
integer/string elements are bitwise-identical values, even the unstable
sort's freedom to arrange ties is unobservable and the output slice is
byte-for-byte identical. '<'/'>' on integers and strings are
side-effect-free and non-overloadable; no String()/Error()/Format()
method is ever consulted by ordering. Named element types
(type Celsius int) are fixed too.

FLOAT elements are reported ADVISORY only, never auto-fixed — and here
the reason is even stronger than in the PS3002/PS3104/PS3105/PS3107
family: besides the usual -0.0/+0.0 and NaN-payload ties an unstable
sort may arrange either way, the hand-rolled chain answers 0 for a NaN
against ANYTHING ('<' and '>' are both false), i.e. it is not even a
consistent ordering, while slices.Sort orders NaNs first. A
TYPE-PARAMETER element is likewise advisory: its instantiations may
include floats.

The match is deliberately EXACT. The comparator must be a fresh func
literal whose whole body is the three-way chain over the two BARE
parameters, resolved by object identity, in any of the equivalent
spellings: two sequential ifs plus a trailing return, an if/else-if
chain (with the default either a trailing return or a final else), or an
expressionless switch whose two cases hold the comparisons (the default
0 either as a default clause or a trailing return). Each condition must
be a single '<' or '>' between the two parameters — a<b and b>a both
mean "less" — with the "less" branch returning a NEGATIVE integer
literal and the "greater" branch a POSITIVE one (magnitude irrelevant:
only the sign is consumed), one branch per direction, and the default
returning literal 0. Anything looser stays silent: a subtraction
comparator (return a - b) is explicitly NOT matched because it can
overflow; '<='/'>=' are not the three-way; a swapped sign pair is a
DESCENDING sort; a field selector (a.f < b.f), a captured variable, a
named constant, extra statements, or a named comparator value all fail
the match. Only returns that are integer literals (with an optional
unary sign) match, so deleting the comparator can never orphan an
import or any other reference.

The fix replaces only the SortFunc selector name with Sort and deletes
the comparator argument: the target slice expression is kept VERBATIM in
place (single evaluation preserved), the package qualifier keeps
whatever alias the file used, and an explicit instantiation
slices.SortFunc[S, E](...) keeps its brackets — slices.Sort has the same
two type parameters, and every fixable E (integer/string underlying)
satisfies its cmp.Ordered constraint. A comment anywhere in the deleted
span (the comparator body included) keeps the report advisory rather
than destroy it. The comparator literal references nothing but its own
parameters and integer literals, so no import ever needs pruning.

The report only fires when the effective language version is at least
go1.21 (slices.SortFunc and slices.Sort appeared there) — the same gate
PS3104/PS3105/PS3107 apply; in practice code containing this pattern
already compiles only on go1.21+.`,
		Before: `slices.SortFunc(s, func(a, b int) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
})`,
		After: `slices.Sort(s)`,
		MeasuredWin: `BenchmarkPS3013 (a shuffled 4096-element []int copied and
sorted per op, Apple M2 Pro, go1.26): 216 µs/op before vs 131 µs/op
after — about 1.6x — 0 allocs either way, so the delta is pure
comparison overhead: the indirect comparator call plus the second
relational comparison per element pair that the monomorphized
slices.Sort inlines away. Same measured character as PS3107's
cmp.Compare spelling of the identical anti-pattern.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS3013",
		Doc:  "slices.SortFunc with a hand-rolled three-way comparator instead of slices.Sort",
		Run:  runPS3013,
	},
})

func runPS3013(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		if !ps3104SlicesSortAvailable(pass, f) {
			// slices.Sort exists only from go1.21 on; below that the
			// advice is moot, so stay silent entirely (same gate as
			// PS3104/PS3105/PS3107).
			continue
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 2 || call.Ellipsis.IsValid() {
				return true
			}
			// The callee must be the receiver-less package function
			// slices.SortFunc, resolved through type info — never a
			// shadowed slices or a same-named method. An explicit
			// instantiation slices.SortFunc[S, E] is unwrapped; its
			// brackets survive the fix (slices.Sort has the same two type
			// parameters).
			sel, ok := ps3107PkgFunc(pass, call.Fun, "slices", "SortFunc")
			if !ok {
				return true
			}
			// The comparator must be a fresh literal that IS the exact
			// hand-rolled three-way (less -> negative literal, greater ->
			// positive literal, default -> literal 0) — anything looser is
			// not provably slices.Sort and is never reported.
			lit, ok := ps2110Unparen(call.Args[1]).(*ast.FuncLit)
			if !ok || !ps3013ThreeWay(pass, lit) {
				return true
			}
			elem, fixable, why := ps3013Elem(pass, lit)
			order := " sorts the " + elem + " elements with the identical ascending order and a single inlined comparison"
			if why != "" {
				// The advisory reasons (float, type parameter) are exactly
				// the cases where "identical" cannot be claimed — a NaN
				// makes this comparator inconsistent while slices.Sort
				// orders NaNs first — so the message drops the claim.
				order = " sorts the " + elem + " elements ascending with a single inlined comparison"
			}
			diag := analysis.Diagnostic{
				Pos:     call.Pos(),
				End:     call.End(),
				Message: "slices.SortFunc with a hand-rolled three-way comparator (a<b/a>b/-1/1/0) pays an indirect comparator call plus up to two relational comparisons per comparison; slices.Sort" + order + why,
			}
			if fixable {
				if fix := ps3013Fix(f, call, sel); fix != nil {
					diag.SuggestedFixes = []analysis.SuggestedFix{*fix}
				}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}

// ps3013ThreeWay reports whether lit's whole body is EXACTLY the ascending
// hand-rolled three-way over its two parameters: one "less" branch (a<b or
// b>a) returning a negative integer literal, one "greater" branch (a>b or
// b<a) returning a positive one, and a default returning literal 0 — in the
// if-chain, if/else-if, or expressionless-switch spelling. Everything else —
// swapped signs (descending), '<='/'>=', a subtraction, selectors, captured
// variables, named constants, extra statements — fails the match.
func ps3013ThreeWay(pass *analysis.Pass, lit *ast.FuncLit) bool {
	// Exactly two named parameters, in source order across the fields
	// (func(a, b T) and func(a T, b T) both). A blank _ parameter cannot
	// be compared in the body, so requiring resolvable names loses
	// nothing.
	var params []*types.Var
	for _, field := range lit.Type.Params.List {
		for _, name := range field.Names {
			if name.Name == "_" {
				return false
			}
			v, isVar := pass.TypesInfo.Defs[name].(*types.Var)
			if !isVar {
				return false
			}
			params = append(params, v)
		}
	}
	if len(params) != 2 {
		return false
	}
	arms, zeroDefault := ps3013Arms(lit.Body.List)
	if len(arms) != 2 || !zeroDefault {
		return false
	}
	var haveLess, haveGreater bool
	for _, arm := range arms {
		dir, ok := ps3013Direction(pass, arm.cond, params)
		if !ok {
			return false
		}
		// The branch's returned sign must MATCH its direction: less ->
		// negative, greater -> positive. A mismatch is a descending sort
		// (or nonsense) and never slices.Sort.
		switch dir {
		case ps3013Less:
			if haveLess || !ps3013IntLit(pass, arm.ret, -1) {
				return false
			}
			haveLess = true
		case ps3013Greater:
			if haveGreater || !ps3013IntLit(pass, arm.ret, +1) {
				return false
			}
			haveGreater = true
		}
	}
	return haveLess && haveGreater
}

// ps3013Arm is one conditional branch of the three-way: its comparison and
// the expression it returns.
type ps3013Arm struct {
	cond ast.Expr
	ret  ast.Expr
}

// ps3013Arms decomposes a comparator body into its conditional arms and
// whether the fall-through default is a bare `return 0` — accepting exactly
// the equivalent spellings of the three-way chain:
//
//	if C1 { return k1 }; if C2 { return k2 }; return 0
//	if C1 { return k1 } else if C2 { return k2 }; return 0
//	if C1 { return k1 } else if C2 { return k2 } else { return 0 }
//	switch { case C1: return k1; case C2: return k2 }; return 0
//	switch { case C1: return k1; case C2: return k2; default: return 0 }
//
// Any other statement, an if with an Init, a tagged switch, fallthrough, or
// a multi-statement branch yields no arms.
func ps3013Arms(stmts []ast.Stmt) (arms []ps3013Arm, zeroDefault bool) {
	switch len(stmts) {
	case 1:
		switch s := stmts[0].(type) {
		case *ast.IfStmt:
			// Fully chained: if/else if/else — the else block carries the
			// default return.
			return ps3013IfChain(s, true)
		case *ast.SwitchStmt:
			// Self-contained switch: the default clause carries the
			// default return.
			return ps3013Switch(s, true)
		}
	case 2:
		if !ps3013IsZeroReturn(stmts[1]) {
			return nil, false
		}
		switch s := stmts[0].(type) {
		case *ast.IfStmt:
			// if/else-if chain with the default as the trailing return.
			arms, zeroDefault = ps3013IfChain(s, false)
			return arms, zeroDefault
		case *ast.SwitchStmt:
			return ps3013Switch(s, false)
		}
		return nil, false
	case 3:
		// Two sequential ifs plus the trailing return.
		if !ps3013IsZeroReturn(stmts[2]) {
			return nil, false
		}
		for _, st := range stmts[:2] {
			ifs, ok := st.(*ast.IfStmt)
			if !ok || ifs.Init != nil || ifs.Else != nil {
				return nil, false
			}
			ret, ok := ps3013SingleReturn(ifs.Body.List)
			if !ok {
				return nil, false
			}
			arms = append(arms, ps3013Arm{cond: ifs.Cond, ret: ret})
		}
		return arms, true
	}
	return nil, false
}

// ps3013IfChain decomposes if C1 { return k1 } else if C2 { return k2 }
// [else { return 0 }] — needElse selects whether the default must be the
// final else block (true) or must be absent because a trailing return
// follows the chain (false).
func ps3013IfChain(first *ast.IfStmt, needElse bool) (arms []ps3013Arm, zeroDefault bool) {
	if first.Init != nil {
		return nil, false
	}
	ret1, ok := ps3013SingleReturn(first.Body.List)
	if !ok {
		return nil, false
	}
	second, ok := first.Else.(*ast.IfStmt)
	if !ok || second.Init != nil {
		return nil, false
	}
	ret2, ok := ps3013SingleReturn(second.Body.List)
	if !ok {
		return nil, false
	}
	arms = []ps3013Arm{{cond: first.Cond, ret: ret1}, {cond: second.Cond, ret: ret2}}
	if !needElse {
		if second.Else != nil {
			return nil, false
		}
		return arms, true
	}
	blk, ok := second.Else.(*ast.BlockStmt)
	if !ok || len(blk.List) != 1 || !ps3013IsZeroReturn(blk.List[0]) {
		return nil, false
	}
	return arms, true
}

// ps3013Switch decomposes switch { case C1: return k1; case C2: return k2;
// [default: return 0] } — needDefault selects whether the default clause
// must be present (true) or absent because a trailing return follows the
// switch (false). A tagged switch, an Init, fallthrough, or a
// multi-statement clause fails.
func ps3013Switch(sw *ast.SwitchStmt, needDefault bool) (arms []ps3013Arm, zeroDefault bool) {
	if sw.Init != nil || sw.Tag != nil {
		return nil, false
	}
	sawDefault := false
	for _, clause := range sw.Body.List {
		cc, ok := clause.(*ast.CaseClause)
		if !ok {
			return nil, false
		}
		ret, ok := ps3013SingleReturn(cc.Body)
		if !ok {
			return nil, false
		}
		if cc.List == nil {
			// The default clause: must be the bare `return 0` and must
			// be wanted.
			if sawDefault || !needDefault || len(cc.Body) != 1 || !ps3013IsZeroReturn(cc.Body[0]) {
				return nil, false
			}
			sawDefault = true
			continue
		}
		if len(cc.List) != 1 {
			return nil, false
		}
		arms = append(arms, ps3013Arm{cond: cc.List[0], ret: ret})
	}
	if needDefault && !sawDefault {
		return nil, false
	}
	return arms, true
}

// ps3013SingleReturn returns the sole expression of a branch body that is
// exactly one single-result return statement.
func ps3013SingleReturn(stmts []ast.Stmt) (ast.Expr, bool) {
	if len(stmts) != 1 {
		return nil, false
	}
	ret, ok := stmts[0].(*ast.ReturnStmt)
	if !ok || len(ret.Results) != 1 {
		return nil, false
	}
	return ret.Results[0], true
}

// ps3013IsZeroReturn reports whether stmt is `return 0` with the zero
// spelled as a plain integer literal (no named constant, no conversion) —
// the default arm of the three-way.
func ps3013IsZeroReturn(stmt ast.Stmt) bool {
	ret, ok := stmt.(*ast.ReturnStmt)
	if !ok || len(ret.Results) != 1 {
		return false
	}
	bl, ok := ps2110Unparen(ret.Results[0]).(*ast.BasicLit)
	return ok && bl.Kind == token.INT && bl.Value == "0"
}

type ps3013Dir int

const (
	ps3013Less ps3013Dir = iota
	ps3013Greater
)

// ps3013Direction classifies cond as the "less" (a<b, b>a) or "greater"
// (a>b, b<a) comparison between EXACTLY the two comparator parameters,
// resolved by object identity. Any other operator ('<=', '>=', '==', ...),
// a selector, a captured variable, or a repeated parameter fails.
func ps3013Direction(pass *analysis.Pass, cond ast.Expr, params []*types.Var) (ps3013Dir, bool) {
	be, ok := ps2110Unparen(cond).(*ast.BinaryExpr)
	if !ok || (be.Op != token.LSS && be.Op != token.GTR) {
		return 0, false
	}
	x, ok := ps2110Unparen(be.X).(*ast.Ident)
	if !ok {
		return 0, false
	}
	y, ok := ps2110Unparen(be.Y).(*ast.Ident)
	if !ok {
		return 0, false
	}
	xv, yv := pass.TypesInfo.Uses[x], pass.TypesInfo.Uses[y]
	switch {
	case xv == params[0] && yv == params[1]:
		if be.Op == token.LSS {
			return ps3013Less, true
		}
		return ps3013Greater, true
	case xv == params[1] && yv == params[0]:
		// b > a is "less", b < a is "greater".
		if be.Op == token.GTR {
			return ps3013Less, true
		}
		return ps3013Greater, true
	}
	return 0, false
}

// ps3013IntLit reports whether e is an integer literal with an optional
// unary sign whose constant sign equals wantSign. Restricting the shape to
// literals (never a named constant like math.MinInt) guarantees the deleted
// comparator references nothing an import or declaration provides, so the
// fix never orphans anything; the sign itself is read from the type-checked
// constant value, so -(-1) and friends cannot slip through.
func ps3013IntLit(pass *analysis.Pass, e ast.Expr, wantSign int) bool {
	inner := ps2110Unparen(e)
	if u, ok := inner.(*ast.UnaryExpr); ok && (u.Op == token.SUB || u.Op == token.ADD) {
		inner = ps2110Unparen(u.X)
	}
	bl, ok := inner.(*ast.BasicLit)
	if !ok || bl.Kind != token.INT {
		return false
	}
	tv, ok := pass.TypesInfo.Types[e]
	if !ok || tv.Value == nil || tv.Value.Kind() != constant.Int {
		return false
	}
	return constant.Sign(tv.Value) == wantSign
}

// ps3013Elem inspects the comparator's element type and classifies the
// site: fixable when the underlying type is an integer or string kind
// (equal elements are bitwise-identical, so the unstable rewrite is
// byte-for-byte identical), advisory with a reason suffix for floats (NaN
// makes the hand-rolled chain inconsistent while slices.Sort orders NaNs
// first, and -0.0/+0.0 ties are equal-but-distinguishable) and type
// parameters (instantiations may include floats).
func ps3013Elem(pass *analysis.Pass, lit *ast.FuncLit) (elem string, fixable bool, why string) {
	sig, ok := pass.TypesInfo.TypeOf(lit).(*types.Signature)
	if !ok || sig.Params().Len() != 2 {
		return "", false, " (no auto-fix: comparator type unresolved)"
	}
	t := sig.Params().At(0).Type()
	elem = types.TypeString(t, types.RelativeTo(pass.Pkg))
	if _, isParam := t.(*types.TypeParam); isParam {
		return elem, false, " (no auto-fix: type-parameter element, instantiations may include floats — NaN makes this comparator inconsistent and slices.Sort orders NaNs first)"
	}
	b, isBasic := t.Underlying().(*types.Basic)
	if !isBasic {
		return elem, false, " (no auto-fix: element type unresolved)"
	}
	switch {
	case b.Info()&(types.IsInteger|types.IsString) != 0:
		return elem, true, ""
	case b.Info()&types.IsFloat != 0:
		return elem, false, " (no auto-fix: float elements — NaN compares neither < nor >, so this comparator calls NaN equal to everything while slices.Sort orders NaNs first, and -0.0/+0.0 ties are equal-but-distinguishable under an unstable sort)"
	default:
		return elem, false, " (no auto-fix: element type unresolved)"
	}
}

// ps3013Fix builds the slices.Sort(s) rewrite for one call, or nil when a
// guard fails and the report must stay advisory. Only the SortFunc selector
// name and the text after the slice argument are replaced: the slice
// expression stays untouched in place (text and single evaluation
// preserved), the package qualifier keeps the file's alias, and explicit
// instantiation brackets survive — slices.Sort takes the same two type
// parameters. The comparator references nothing but its parameters and
// integer literals, so no import edit is ever needed.
func ps3013Fix(f *ast.File, call *ast.CallExpr, sel *ast.SelectorExpr) *analysis.SuggestedFix {
	// The span from the slice argument's end to the call's end — the
	// comma, the whole comparator (its body comments included) and the
	// closing parenthesis — is deleted; a comment there would be silently
	// destroyed — advisory then. (The other replaced span is the SortFunc
	// identifier itself, which cannot contain a comment.)
	if ps2111CommentIn(f, call.Args[0].End(), call.End()) {
		return nil
	}
	return &analysis.SuggestedFix{
		Message: "replace with " + ps3107Qualifier(sel) + "Sort(...)",
		TextEdits: []analysis.TextEdit{
			{Pos: sel.Sel.Pos(), End: sel.Sel.End(), NewText: []byte("Sort")},
			{Pos: call.Args[0].End(), End: call.End(), NewText: []byte(")")},
		},
	}
}
