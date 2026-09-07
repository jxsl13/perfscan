package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"math/big"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS2144 implements the deliberately narrow, local form of owner issue #800:
// adjacent sibling scratch slices with the same element type and stable length
// whose backing storage can be evaluated for coalescing.
var PS2144 = register(&lint.Check{
	ID:       "PS2144",
	Category: "alloc",
	Slug:     "coalescible-sibling-scratch-slices",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "adjacent same-shape scratch slices remain live together and may share one guarded backing allocation",
		Text: `Several same-type scratch slices created together can sometimes use
fixed, non-overlapping lanes of one backing slice. That reduces the number of
source-level backing objects while preserving the total payload size. Whether
those source objects become heap allocations is a compiler and size decision:
PS2144 does not claim heap allocation for a source-provably stack-only shape.
Even when allocations do reach the heap, fewer objects are not an automatic
clock-time win; allocator and GC pressure, cache behavior, and the operation's
share of the profile determine profitability, so benchmark the real caller.

The diagnostic is intentionally bounded to an auditable statement-list proof.
It reports one maximal source-reachable run of two or more adjacent,
single-name short declarations of exactly make([]T, n), where T is identical,
non-zero-sized, and has a concrete layout with no nested type parameter. The
length n is either the same positive integer constant or the same int parameter
that is never assigned, incremented, or addressed in the function. Every slice
must have a later reachable use after the final make, proving that all members
are live together. Constant-false branches and statements after a return,
branch, or builtin panic are skipped; this is a bounded structured test, not a
general CFG proof.

Parentheses are transparent, but the complete enclosing use is checked. Direct
indexing is allowed only for an evident lane write or a value-only index,
operator, field, or conversion context. An array-valued element resliced into a
view, direct storage/return/send of an indexed projection, explicit or implicit
address-taking (including pointer-receiver methods), and other uncertain
ownership all suppress the finding. Whole-slice uses are limited to ranging,
len, copy, and clear. Reassignment, reslicing, aliasing, append, cap, ordinary
calls, closure capture, channel sends, returns of the slice itself, and storage
in another value likewise suppress it. Explicit-capacity make, generic element
layouts, local size temporaries, tuple declarations, non-adjacent declarations,
and expressions such as len(x) are outside this deliberately small proof
boundary.

There is NO automatic fix. The combined size k*n can overflow int even though
each original make([]T, n) succeeds. Before changing code, prove the product
fits or guard n <= maxInt/k (while preserving the original negative-length
failure). Also confirm the aggregate element-byte size can fit in one runtime
allocation; separate objects can occasionally fit where one giant object
cannot. Each lane should use a full slice expression, scratch[lo:hi:hi], so its
capacity remains n. A two-index slice would expose the following lanes as spare
capacity: a later append could overwrite a sibling instead of allocating, which
changes both values and aliasing. PS2144 rejects current append/cap uses, but
the full-cap form keeps that invariant explicit and robust.

The shared backing array also couples retention: one live lane retains every
lane. Confirm that this does not increase peak live memory, and preserve any
zeroing, pooling, ownership, and concurrency contracts before adopting the
change.`,
		Before: `a := make([]float32, n)
b := make([]float32, n)
c := make([]float32, n)`,
		After: `// k == 3; preserve the original failure for n < 0 and guard 3*n.
if n < 0 || n > int(^uint(0)>>1)/3 {
	panic("scratch size out of range")
}
scratch := make([]float32, 3*n)
a := scratch[:n:n]
b := scratch[n : 2*n : 2*n]
c := scratch[2*n : 3*n : 3*n]`,
		MeasuredWin: `GoAI quantized prefill on Apple M2 Pro / Go 1.26.6,
three per-worker QMatMul scratch rows: 62 -> 40 allocs/op (-35.48%) and
76.05 us -> 74.31 us (-2.29%), with B/op flat at about 137.4 KiB
(order-alternated, n=10). The 22-object reduction was structural across up to
eleven workers; the modest timing delta is workload-specific and must not be
generalized. The repository benchmark isolates the object-count trade, not a
universal latency claim. In its 4096-element three-lane arm on the same machine,
six fresh-process pairs consistently measured 3 -> 1 allocs/op with payload
flat at about 96 KiB/op; the short wall-time samples were noisy and support no
additional timing claim.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2144",
		Doc:  "adjacent same-shape scratch slices remain live together and may share one guarded backing allocation",
		Run:  runPS2144,
	},
})

type ps2144Length struct {
	object types.Object
	value  string
}

type ps2144Allocation struct {
	stmt   ast.Stmt
	call   *ast.CallExpr
	object types.Object
	elem   types.Type
	length ps2144Length
}

func runPS2144(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ps2144ScanFunction(pass, fn)
		}
	}
	return nil, nil
}

func ps2144ScanFunction(pass *analysis.Pass, fn *ast.FuncDecl) {
	params := ps2144Params(pass, fn)
	unreachable := ps2144Unreachable(pass, fn.Body)
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		block, ok := node.(*ast.BlockStmt)
		if ok {
			ps2144ScanBlock(pass, fn.Body, block, params, unreachable)
		}
		return true
	})
}

func ps2144Params(pass *analysis.Pass, fn *ast.FuncDecl) map[types.Object]bool {
	params := make(map[types.Object]bool)
	if fn.Type.Params == nil {
		return params
	}
	for _, field := range fn.Type.Params.List {
		for _, name := range field.Names {
			if object := pass.TypesInfo.Defs[name]; object != nil {
				params[object] = true
			}
		}
	}
	return params
}

func ps2144ScanBlock(pass *analysis.Pass, function, block *ast.BlockStmt, params map[types.Object]bool, unreachable []tokenSpan) {
	for i := 0; i < len(block.List); {
		if ps2144PositionIn(block.List[i].Pos(), unreachable) {
			i++
			continue
		}
		first, ok := ps2144AllocationAt(pass, function, block.List[i], params)
		if !ok {
			i++
			continue
		}

		group := []ps2144Allocation{first}
		j := i + 1
		for j < len(block.List) {
			if ps2144PositionIn(block.List[j].Pos(), unreachable) {
				break
			}
			next, ok := ps2144AllocationAt(pass, function, block.List[j], params)
			if !ok || !ps2144SameShape(first, next) {
				break
			}
			group = append(group, next)
			j++
		}
		if len(group) >= 2 && ps2144OverlappingSafeUses(pass, block, group, unreachable) {
			pass.Report(analysis.Diagnostic{
				Pos: group[0].call.Pos(),
				End: group[len(group)-1].call.End(),
				Message: "adjacent same-shape scratch slices remain live together; evaluate one overflow-guarded backing allocation with " +
					"full-capacity non-overlapping lanes (fewer backing objects is structural, but heap placement and timing must be measured)",
			})
		}
		i = j
	}
}

func ps2144AllocationAt(pass *analysis.Pass, function *ast.BlockStmt, stmt ast.Stmt, params map[types.Object]bool) (ps2144Allocation, bool) {
	assign, ok := stmt.(*ast.AssignStmt)
	if !ok || assign.Tok != token.DEFINE || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
		return ps2144Allocation{}, false
	}
	lhs, ok := assign.Lhs[0].(*ast.Ident)
	if !ok || lhs.Name == "_" {
		return ps2144Allocation{}, false
	}
	object := pass.TypesInfo.Defs[lhs]
	if object == nil {
		return ps2144Allocation{}, false
	}
	call, ok := ps2110Unparen(assign.Rhs[0]).(*ast.CallExpr)
	if !ok || len(call.Args) != 2 || call.Ellipsis.IsValid() || !ps2144Builtin(pass, call, "make") {
		return ps2144Allocation{}, false
	}
	slice, ok := pass.TypesInfo.TypeOf(call).(*types.Slice)
	if !ok || !ps2144KnownNonzeroSize(pass.TypesSizes, slice.Elem()) {
		return ps2144Allocation{}, false
	}
	length, ok := ps2144StableLength(pass, function, call.Args[1], params)
	if !ok {
		return ps2144Allocation{}, false
	}
	return ps2144Allocation{stmt: stmt, call: call, object: object, elem: slice.Elem(), length: length}, true
}

func ps2144StableLength(pass *analysis.Pass, function *ast.BlockStmt, expr ast.Expr, params map[types.Object]bool) (ps2144Length, bool) {
	expr = ps2110Unparen(expr)
	if tv, ok := pass.TypesInfo.Types[expr]; ok && tv.Value != nil && tv.Value.Kind() == constant.Int {
		if constant.Sign(tv.Value) <= 0 {
			return ps2144Length{}, false
		}
		return ps2144Length{value: tv.Value.ExactString()}, true
	}
	id, ok := expr.(*ast.Ident)
	if !ok || !types.Identical(pass.TypesInfo.TypeOf(id), types.Typ[types.Int]) {
		return ps2144Length{}, false
	}
	object, ok := pass.TypesInfo.Uses[id].(*types.Var)
	if !ok || !params[object] || ps2144LengthMutated(pass, function, object) {
		return ps2144Length{}, false
	}
	return ps2144Length{object: object}, true
}

func ps2144LengthMutated(pass *analysis.Pass, function *ast.BlockStmt, object types.Object) bool {
	mutated := false
	ast.Inspect(function, func(node ast.Node) bool {
		if mutated {
			return false
		}
		switch n := node.(type) {
		case *ast.AssignStmt:
			for _, lhs := range n.Lhs {
				if id, ok := ps2110Unparen(lhs).(*ast.Ident); ok && pass.TypesInfo.Uses[id] == object {
					mutated = true
					return false
				}
			}
		case *ast.IncDecStmt:
			if id, ok := ps2110Unparen(n.X).(*ast.Ident); ok && pass.TypesInfo.Uses[id] == object {
				mutated = true
				return false
			}
		case *ast.RangeStmt:
			for _, target := range []ast.Expr{n.Key, n.Value} {
				if id, ok := ps2110Unparen(target).(*ast.Ident); ok && pass.TypesInfo.Uses[id] == object {
					mutated = true
					return false
				}
			}
		case *ast.UnaryExpr:
			if id, ok := ps2110Unparen(n.X).(*ast.Ident); n.Op == token.AND && ok && pass.TypesInfo.Uses[id] == object {
				mutated = true
				return false
			}
		}
		return true
	})
	return mutated
}

func ps2144SameShape(a, b ps2144Allocation) bool {
	return types.Identical(a.elem, b.elem) && a.length.object == b.length.object && a.length.value == b.length.value
}

func ps2144OverlappingSafeUses(pass *analysis.Pass, block *ast.BlockStmt, group []ps2144Allocation, unreachable []tokenSpan) bool {
	if group[0].length.value != "" && !ps2144ConstantProductFits(pass, group[0].length.value, len(group)) {
		return false
	}
	lastMake := group[len(group)-1].stmt.End()
	for _, allocation := range group {
		lastUse, ok := ps2144SafeUses(pass, block, allocation.object, unreachable)
		if !ok || lastUse <= lastMake {
			return false
		}
	}
	return true
}

func ps2144ConstantProductFits(pass *analysis.Pass, value string, lanes int) bool {
	if pass.TypesSizes == nil {
		return false
	}
	n, ok := new(big.Int).SetString(value, 10)
	if !ok {
		return false
	}
	product := new(big.Int).Mul(n, big.NewInt(int64(lanes)))
	bits := uint(pass.TypesSizes.Sizeof(types.Typ[types.Int]) * 8)
	maxInt := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), bits-1), big.NewInt(1))
	return product.Cmp(maxInt) <= 0
}

// ps2144KnownNonzeroSize fails closed before asking go/types for a layout.
// Go 1.26's Sizes implementations panic on type parameters, including when a
// parameter is nested inside a composite element type. Layout inference from
// a constraint is deliberately outside this check's bounded proof.
func ps2144KnownNonzeroSize(sizes types.Sizes, element types.Type) (ok bool) {
	if sizes == nil || ps2144ContainsTypeParam(element, make(map[types.Type]bool)) {
		return false
	}
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	return sizes.Sizeof(element) > 0
}

func ps2144ContainsTypeParam(t types.Type, seen map[types.Type]bool) bool {
	if t == nil {
		return false
	}
	t = types.Unalias(t)
	if seen[t] {
		return false
	}
	seen[t] = true
	switch t := t.(type) {
	case *types.TypeParam:
		return true
	case *types.Array:
		return ps2144ContainsTypeParam(t.Elem(), seen)
	case *types.Slice:
		return ps2144ContainsTypeParam(t.Elem(), seen)
	case *types.Pointer:
		return ps2144ContainsTypeParam(t.Elem(), seen)
	case *types.Map:
		return ps2144ContainsTypeParam(t.Key(), seen) || ps2144ContainsTypeParam(t.Elem(), seen)
	case *types.Chan:
		return ps2144ContainsTypeParam(t.Elem(), seen)
	case *types.Struct:
		for i := 0; i < t.NumFields(); i++ {
			if ps2144ContainsTypeParam(t.Field(i).Type(), seen) {
				return true
			}
		}
	case *types.Tuple:
		for i := 0; i < t.Len(); i++ {
			if ps2144ContainsTypeParam(t.At(i).Type(), seen) {
				return true
			}
		}
	case *types.Signature:
		for _, list := range []*types.TypeParamList{t.RecvTypeParams(), t.TypeParams()} {
			for i := 0; list != nil && i < list.Len(); i++ {
				if ps2144ContainsTypeParam(list.At(i), seen) {
					return true
				}
			}
		}
		if ps2144ContainsTypeParam(t.Params(), seen) ||
			ps2144ContainsTypeParam(t.Results(), seen) {
			return true
		}
	case *types.Named:
		for i := 0; t.TypeArgs() != nil && i < t.TypeArgs().Len(); i++ {
			if ps2144ContainsTypeParam(t.TypeArgs().At(i), seen) {
				return true
			}
		}
		if ps2144ContainsTypeParam(t.Underlying(), seen) {
			return true
		}
	case *types.Interface:
		for i := 0; i < t.NumEmbeddeds(); i++ {
			if ps2144ContainsTypeParam(t.EmbeddedType(i), seen) {
				return true
			}
		}
		for i := 0; i < t.NumExplicitMethods(); i++ {
			if ps2144ContainsTypeParam(t.ExplicitMethod(i).Type(), seen) {
				return true
			}
		}
	case *types.Union:
		for i := 0; i < t.Len(); i++ {
			if ps2144ContainsTypeParam(t.Term(i).Type(), seen) {
				return true
			}
		}
	}
	return false
}

func ps2144PositionIn(pos token.Pos, spans []tokenSpan) bool {
	for _, span := range spans {
		if span.contains(pos) {
			return true
		}
	}
	return false
}

// ps2144Unreachable records only source-proven dead regions: constant-false
// bodies, constant-true else arms, and statements after a structured terminal
// transfer. It intentionally does not grow into general CFG interpretation.
func ps2144Unreachable(pass *analysis.Pass, body *ast.BlockStmt) []tokenSpan {
	var spans []tokenSpan
	var visitStmt func(ast.Stmt) bool
	var visitBlock func(*ast.BlockStmt) bool

	mark := func(node ast.Node) {
		if node != nil {
			spans = append(spans, tokenSpan{start: node.Pos(), end: node.End() + 1})
		}
	}
	visitBlock = func(block *ast.BlockStmt) bool {
		fallsThrough := true
		for _, stmt := range block.List {
			if !fallsThrough {
				mark(stmt)
				continue
			}
			fallsThrough = visitStmt(stmt)
		}
		return fallsThrough
	}
	visitElse := func(stmt ast.Stmt) bool { return visitStmt(stmt) }
	visitStmt = func(stmt ast.Stmt) bool {
		switch stmt := stmt.(type) {
		case *ast.BlockStmt:
			return visitBlock(stmt)
		case *ast.IfStmt:
			if truth, known := ps2144BoolConstant(pass, stmt.Cond); known {
				if truth {
					mark(stmt.Else)
					return visitBlock(stmt.Body)
				}
				mark(stmt.Body)
				if stmt.Else == nil {
					return true
				}
				return visitElse(stmt.Else)
			}
			bodyFalls := visitBlock(stmt.Body)
			if stmt.Else == nil {
				return true
			}
			elseFalls := visitElse(stmt.Else)
			return bodyFalls || elseFalls
		case *ast.ForStmt:
			if stmt.Cond != nil {
				if truth, known := ps2144BoolConstant(pass, stmt.Cond); known && !truth {
					mark(stmt.Body)
					return true
				}
			}
			visitBlock(stmt.Body)
			return true
		case *ast.RangeStmt:
			visitBlock(stmt.Body)
			return true
		case *ast.LabeledStmt:
			return visitStmt(stmt.Stmt)
		case *ast.ReturnStmt, *ast.BranchStmt:
			return false
		case *ast.ExprStmt:
			call, ok := ps2110Unparen(stmt.X).(*ast.CallExpr)
			return !ok || !ps2144Builtin(pass, call, "panic")
		}
		ps2144VisitNestedBlocks(stmt, visitBlock)
		return true
	}
	visitBlock(body)
	return spans
}

func ps2144VisitNestedBlocks(root ast.Node, visit func(*ast.BlockStmt) bool) {
	first := true
	ast.Inspect(root, func(node ast.Node) bool {
		if first {
			first = false
			return true
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		if block, ok := node.(*ast.BlockStmt); ok {
			visit(block)
			return false
		}
		return true
	})
}

func ps2144BoolConstant(pass *analysis.Pass, expr ast.Expr) (bool, bool) {
	value := pass.TypesInfo.Types[ps2110Unparen(expr)].Value
	if value == nil || value.Kind() != constant.Bool {
		return false, false
	}
	return constant.BoolVal(value), true
}

func ps2144SafeUses(pass *analysis.Pass, block *ast.BlockStmt, object types.Object, unreachable []tokenSpan) (token.Pos, bool) {
	var lastUse token.Pos
	safe := true
	astutil.WithStack(block, func(node ast.Node, stack []ast.Node) bool {
		if !safe {
			return false
		}
		id, ok := node.(*ast.Ident)
		if !ok || pass.TypesInfo.Uses[id] != object {
			return true
		}
		if ps2144PositionIn(id.Pos(), unreachable) {
			return true
		}
		for _, ancestor := range stack {
			if _, captured := ancestor.(*ast.FuncLit); captured {
				safe = false
				return false
			}
		}
		if len(stack) == 0 {
			safe = false
			return false
		}
		parent, parentIndex, child := ps2144ParentThroughParens(stack, id)
		switch p := parent.(type) {
		case *ast.IndexExpr:
			if ps2110Unparen(p.X) != ast.Expr(id) || !ps2144SafeIndexedUse(pass, stack, parentIndex, p) {
				safe = false
				return false
			}
		case *ast.RangeStmt:
			if p.X != child {
				safe = false
				return false
			}
		case *ast.CallExpr:
			if !ps2144SafeBuiltinUse(pass, p, child) {
				safe = false
				return false
			}
		default:
			safe = false
			return false
		}
		if id.Pos() > lastUse {
			lastUse = id.Pos()
		}
		return true
	})
	return lastUse, safe
}

func ps2144ParentThroughParens(stack []ast.Node, node ast.Node) (ast.Node, int, ast.Node) {
	child := node
	for i := len(stack) - 1; i >= 0; i-- {
		parent := stack[i]
		if paren, ok := parent.(*ast.ParenExpr); ok && paren.X == child {
			child = paren
			continue
		}
		return parent, i, child
	}
	return nil, -1, child
}

// ps2144SafeIndexedUse follows the complete projection rooted at a[i]. It
// allows local element mutation and scalar/value computation, but rejects
// projections and contexts that can retain the lane's backing storage or whose
// ownership is not obvious from syntax.
func ps2144SafeIndexedUse(pass *analysis.Pass, stack []ast.Node, indexAt int, index *ast.IndexExpr) bool {
	var current ast.Node = index
	for i := indexAt - 1; i >= 0; i-- {
		parent := stack[i]
		switch parent := parent.(type) {
		case *ast.ParenExpr:
			if parent.X != current {
				return false
			}
			current = parent
		case *ast.IndexExpr:
			if parent.X == current {
				current = parent
				continue
			}
			// The lane element has been consumed as an integer index.
			return parent.Index == current
		case *ast.SliceExpr:
			if parent.X == current {
				return false // an array element sliced here aliases lane storage
			}
			// A scalar lane element used only as a slice bound cannot retain
			// the lane backing object.
			return parent.Low == current || parent.High == current || parent.Max == current
		case *ast.SelectorExpr:
			if parent.X != current {
				return false
			}
			selection := pass.TypesInfo.Selections[parent]
			if selection == nil || selection.Kind() != types.FieldVal {
				return false // method values/calls may implicitly take &a[i]
			}
			current = parent
		case *ast.UnaryExpr:
			if parent.X != current || parent.Op == token.AND || parent.Op == token.MUL || parent.Op == token.ARROW {
				return false
			}
			return true // +, -, !, and ^ produce a value detached from storage
		case *ast.BinaryExpr:
			return parent.X == current || parent.Y == current
		case *ast.CallExpr:
			if len(parent.Args) != 1 || parent.Args[0] != current {
				return false
			}
			tv, ok := pass.TypesInfo.Types[parent.Fun]
			return ok && tv.IsType() // a conversion copies the indexed value
		case *ast.AssignStmt:
			for _, lhs := range parent.Lhs {
				if lhs == current {
					return true // direct write into the lane
				}
			}
			return false // direct storage of the indexed projection is out of scope
		case *ast.IncDecStmt:
			return parent.X == current
		case *ast.ReturnStmt, *ast.SendStmt, *ast.CompositeLit, *ast.KeyValueExpr:
			return false
		default:
			return false
		}
	}
	return false
}

func ps2144SafeBuiltinUse(pass *analysis.Pass, call *ast.CallExpr, child ast.Node) bool {
	name := ""
	if fn, ok := ps2110Unparen(call.Fun).(*ast.Ident); ok {
		if builtin, ok := pass.TypesInfo.Uses[fn].(*types.Builtin); ok {
			name = builtin.Name()
		}
	}
	if name != "len" && name != "copy" && name != "clear" {
		return false
	}
	for _, arg := range call.Args {
		if arg == child {
			return true
		}
	}
	return false
}

func ps2144Builtin(pass *analysis.Pass, call *ast.CallExpr, name string) bool {
	id, ok := ps2110Unparen(call.Fun).(*ast.Ident)
	if !ok {
		return false
	}
	builtin, ok := pass.TypesInfo.Uses[id].(*types.Builtin)
	return ok && builtin.Name() == name
}
