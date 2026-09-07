package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/cfg"

	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS6103 implements owner issue #897. It reports an allocation-producing
// operation that dominates a fixed-count loop and whose per-iteration result
// is consumed before the next iteration. Existing Into siblings are preferred;
// otherwise the allocation must be visible in a package-local callee.
var PS6103 = register(&lint.Check{
	ID:       "PS6103",
	Category: "alloc",
	Slug:     "fixed-loop-operation-needs-caller-owned-output",
	Level:    lint.LevelAggressive,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "a fixed-count loop repeatedly creates a short-lived operation result that can use caller-owned output",
		Text: `Fixed-shape iterative kernels can spend most of their allocation budget
constructing the same temporary result on every step. Replacing a shared,
optimized operation with a local loop may remove allocation but also discard
architecture routing. A caller-owned output seam preserves that dispatch: an
optional backend Into capability writes into receiver-owned workspace, while
unsupported backends retain the allocating operation plus copy fallback.

PS6103 reports only fixed positive trip counts and a direct operation
assignment that dominates the loop body. The call arguments may not contain
the induction value, nested calls, channel receives, or per-iteration slicing.
Exactly one reusable pointer or slice result is bound to a temporary; every use
of that result must occur after the call in the same iteration and be consumed
by an ordinary call. Returns, closure/go/defer capture, sends, append retention,
stores, and uses across the loop boundary stay silent. The operation must
source-prove on every returning path that its reusable result comes from make,
new, an addressed composite literal, or a transitively proven direct wrapper;
that proof travels across imports as an analysis fact. It must then either
expose a type-compatible <Name>Into sibling with one explicit destination and
matching non-data results, or be a package-local function for which the report
recommends adding that seam.

There is NO automatic fix. Types do not prove stable runtime shape, destination
freshness, legal operand aliasing, backend fallback contracts, or whether an
opaque consumer retains an argument. Preserve the optimized dispatch path;
add an optional Into capability whose unsupported result leaves the destination
unchanged, retain allocating-plus-copy compatibility, and validate dtype,
shape, layout, aliasing, panic/partial-output behavior, exact outputs, and the
complete fixed-step algorithm before promotion.`,
		Before: `for step := 0; step < 5; step++ {
	product := ops.MatMul(left, right)
	state = update(state, product)
}`,
		After: `product := newWorkspace(shape)
for step := 0; step < 5; step++ {
	if !backend.MatMulInto(product, left, right) {
		copyResult(product, ops.MatMul(left, right)) // compatibility fallback
	}
	state = update(state, product)
}`,
		MeasuredWin: `Owner issue #897 measured GoAI's five-step Muon
Newton-Schulz path on an Apple M2 Pro. At 256x512, preserving the optimized
MatMul leaf behind MatMulInto reduced the preflight from 48.926 ms to 12.581 ms
(3.89x) and from 952,710 to 2,880 B/op with bit-exact output. Seven
order-alternated end-to-end campaigns improved the median from 57.504 ms to
15.534 ms (3.70x; every pair 3.29x–3.98x), cut bytes from 7,888,289 to 7,021
per operation, and retained the exact F64/F32 three-step digests; an Adam
control remained at 0.998x. The validated implementation and retained evidence
are in github.com/jxsl13/goai pull request 1218.`,
	},
	Analyzer: &analysis.Analyzer{
		Name:      "PS6103",
		Doc:       "fixed-count loop repeatedly creates a short-lived operation result that can use caller-owned output",
		Run:       runPS6103,
		FactTypes: []analysis.Fact{new(ps6103AllocationFact)},
	},
})

type ps6103AllocationFact struct{}

func (*ps6103AllocationFact) AFact()         {}
func (*ps6103AllocationFact) String() string { return "fresh-reusable-result-allocation" }

type ps6103Loop struct {
	node  ast.Node
	body  *ast.BlockStmt
	index types.Object
	count int64
}

type ps6103Evidence struct {
	kind string
	name string
}

func runPS6103(pass *analysis.Pass) (any, error) {
	locals := make(map[*types.Func]*ast.FuncDecl)
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			object, ok := pass.TypesInfo.Defs[function.Name].(*types.Func)
			if ok {
				locals[object.Origin()] = function
			}
		}
	}
	allocating := make(map[*types.Func]bool)
	state := make(map[*types.Func]uint8)
	var resolvesAllocation func(*types.Func) bool
	resolvesAllocation = func(object *types.Func) bool {
		if object == nil {
			return false
		}
		object = object.Origin()
		if state[object] == 2 {
			return allocating[object]
		}
		if state[object] == 1 {
			return false
		}
		function := locals[object]
		if function == nil {
			if object.Pkg() == pass.Pkg {
				return false
			}
			var fact ps6103AllocationFact
			return pass.ImportObjectFact(object, &fact)
		}
		state[object] = 1
		signature, _ := object.Type().(*types.Signature)
		resultIndex, _, ok := ps6103DataResult(signature)
		allocated := ok && ps6103AllocatesResult(pass, function, resultIndex, resolvesAllocation)
		state[object] = 2
		allocating[object] = allocated
		return allocated
	}
	for object := range locals {
		resolvesAllocation(object)
	}
	for object, allocated := range allocating {
		if allocated {
			pass.ExportObjectFact(object, new(ps6103AllocationFact))
		}
	}
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if ok && function.Body != nil {
				ps6103Function(pass, function, locals, allocating)
			}
		}
	}
	return nil, nil
}

func ps6103Function(pass *analysis.Pass, function *ast.FuncDecl, locals map[*types.Func]*ast.FuncDecl, allocating map[*types.Func]bool) {
	parents := ps6087Parents(function.Body)
	exposed := ps6103ExposureFacts(pass, function.Body, parents)
	ast.Inspect(function.Body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		loop := ps6103FixedLoop(pass, node)
		if loop == nil || !ps6103ControlSafe(pass, loop, parents, exposed) {
			return true
		}
		for _, statement := range loop.body.List {
			assignment, ok := statement.(*ast.AssignStmt)
			if !ok || len(assignment.Rhs) != 1 {
				continue
			}
			call, ok := ps2110Unparen(assignment.Rhs[0]).(*ast.CallExpr)
			if !ok || call.Ellipsis.IsValid() || !ps6103StableArguments(pass, call, loop, parents, exposed) || !ps6103NoEarlierReturn(loop.body, call.Pos()) {
				continue
			}
			callee, _, ok := typedCallee(pass, call.Fun)
			if !ok {
				continue
			}
			signature, ok := pass.TypesInfo.TypeOf(call.Fun).(*types.Signature)
			if !ok {
				signature, _ = callee.Type().(*types.Signature)
			}
			resultIndex, resultType, ok := ps6103DataResult(signature)
			if !ok || len(assignment.Lhs) != signature.Results().Len() {
				continue
			}
			target, ok := ps2110Unparen(assignment.Lhs[resultIndex]).(*ast.Ident)
			if !ok || target.Name == "_" {
				continue
			}
			object := ps6103AssignedObject(pass, target, assignment.Tok)
			if object == nil || !ps6103ShortLived(pass, function.Body, loop, call, assignment, target, object, parents) {
				continue
			}
			evidence, ok := ps6103OperationEvidence(pass, call, callee, signature, resultIndex, resultType, locals, allocating)
			if !ok {
				continue
			}
			var message strings.Builder
			message.Grow(768)
			message.WriteString("fixed ")
			message.WriteString(strconv.FormatInt(loop.count, 10))
			message.WriteString("-step loop repeatedly creates short-lived ")
			message.WriteString(exprTextRendered(call.Fun))
			message.WriteString(" result ")
			message.WriteString(target.Name)
			message.WriteString("; ")
			if evidence.kind == "sibling" {
				message.WriteString("the type-compatible ")
				message.WriteString(evidence.name)
				message.WriteString(" sibling can preserve optimized dispatch while writing caller-owned workspace")
			} else {
				message.WriteString("the package-local callee source-proves a fresh result allocation and has no compatible Into sibling; add an optional caller-owned-output seam without bypassing optimized dispatch")
			}
			message.WriteString("; retain allocating-plus-copy fallback and validate stable runtime shape, dtype/layout, aliasing, unsupported-destination immutability, panic/partial-output behavior, exact fixed-step outputs, and end-to-end crossover (advisory, no automatic fix)")
			pass.Report(analysis.Diagnostic{Pos: call.Pos(), End: call.End(), Message: message.String()})
		}
		return true
	})
}

func ps6103FixedLoop(pass *analysis.Pass, node ast.Node) *ps6103Loop {
	switch loop := node.(type) {
	case *ast.ForStmt:
		initializer, ok := loop.Init.(*ast.AssignStmt)
		if !ok || len(initializer.Lhs) != 1 || len(initializer.Rhs) != 1 || !ps6103ConstantInt(pass, initializer.Rhs[0], 0) {
			return nil
		}
		identifier, ok := ps2110Unparen(initializer.Lhs[0]).(*ast.Ident)
		if !ok {
			return nil
		}
		index := ps6103AssignedObject(pass, identifier, initializer.Tok)
		condition, ok := ps2110Unparen(loop.Cond).(*ast.BinaryExpr)
		if index == nil || !ok || condition.Op != token.LSS || !ps6103Object(pass, condition.X, index) || !ps6103Increment(pass, loop.Post, index) {
			return nil
		}
		count, ok := ps6103PositiveConstant(pass, condition.Y)
		if !ok || count < 2 || loop.Body == nil {
			return nil
		}
		return &ps6103Loop{node: loop, body: loop.Body, index: index, count: count}
	case *ast.RangeStmt:
		if loop.Body == nil || loop.Value != nil {
			return nil
		}
		count, ok := ps6103RangeCount(pass, loop.X)
		if !ok || count < 2 {
			return nil
		}
		var index types.Object
		if identifier, ok := ps2110Unparen(loop.Key).(*ast.Ident); ok && identifier.Name != "_" {
			index = ps6103AssignedObject(pass, identifier, loop.Tok)
		}
		return &ps6103Loop{node: loop, body: loop.Body, index: index, count: count}
	}
	return nil
}

func ps6103RangeCount(pass *analysis.Pass, expression ast.Expr) (int64, bool) {
	if count, ok := ps6103PositiveConstant(pass, expression); ok {
		return count, true
	}
	typ := pass.TypesInfo.TypeOf(expression)
	if typ == nil {
		return 0, false
	}
	array, ok := types.Unalias(typ).Underlying().(*types.Array)
	if !ok {
		return 0, false
	}
	return array.Len(), true
}

func ps6103PositiveConstant(pass *analysis.Pass, expression ast.Expr) (int64, bool) {
	value := pass.TypesInfo.Types[expression].Value
	if value == nil || value.Kind() != constant.Int {
		return 0, false
	}
	count, ok := constant.Int64Val(value)
	return count, ok && count > 0
}

func ps6103ConstantInt(pass *analysis.Pass, expression ast.Expr, expected int64) bool {
	value := pass.TypesInfo.Types[expression].Value
	return value != nil && value.Kind() == constant.Int && constant.Compare(value, token.EQL, constant.MakeInt64(expected))
}

func ps6103AssignedObject(pass *analysis.Pass, identifier *ast.Ident, assignment token.Token) types.Object {
	if assignment == token.DEFINE {
		return pass.TypesInfo.Defs[identifier]
	}
	return pass.TypesInfo.Uses[identifier]
}

func ps6103Object(pass *analysis.Pass, expression ast.Expr, object types.Object) bool {
	identifier, ok := ps2110Unparen(expression).(*ast.Ident)
	return ok && pass.TypesInfo.ObjectOf(identifier) == object
}

func ps6103Increment(pass *analysis.Pass, statement ast.Stmt, object types.Object) bool {
	switch value := statement.(type) {
	case *ast.IncDecStmt:
		return value.Tok == token.INC && ps6103Object(pass, value.X, object)
	case *ast.AssignStmt:
		return value.Tok == token.ADD_ASSIGN && len(value.Lhs) == 1 && len(value.Rhs) == 1 &&
			ps6103Object(pass, value.Lhs[0], object) && ps6103ConstantInt(pass, value.Rhs[0], 1)
	}
	return false
}

func ps6103ControlSafe(pass *analysis.Pass, loop *ps6103Loop, parents map[ast.Node]ast.Node, exposed map[types.Object]bool) bool {
	// A counted for loop is fixed only while its induction variable is governed
	// by the recognized post statement. Writes or address exposure in the body
	// can change the trip count. A range index does not govern its trip count.
	if _, counted := loop.node.(*ast.ForStmt); counted && ps6103ObjectMutable(pass, loop.body, loop.index, parents, exposed) {
		return false
	}
	safe := true
	astutil.WithStack(loop.body, func(node ast.Node, stack []ast.Node) bool {
		if !safe {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		switch value := node.(type) {
		case *ast.BranchStmt:
			if value.Label != nil || value.Tok == token.GOTO || !ps6103InsideNestedLoop(stack) {
				safe = false
			}
		}
		return safe
	})
	return safe
}

func ps6103InsideNestedLoop(stack []ast.Node) bool {
	for _, node := range stack {
		switch node.(type) {
		case *ast.ForStmt, *ast.RangeStmt:
			return true
		}
	}
	return false
}

func ps6103StableArguments(pass *analysis.Pass, call *ast.CallExpr, loop *ps6103Loop, parents map[ast.Node]ast.Node, exposed map[types.Object]bool) bool {
	for _, argument := range call.Args {
		if !ps6103StableExpression(pass, argument, loop, parents, exposed) {
			return false
		}
	}
	if selector := ps6103CallSelector(call.Fun); selector != nil && pass.TypesInfo.Selections[selector] != nil {
		return ps6103StableExpression(pass, selector.X, loop, parents, exposed)
	}
	return true
}

func ps6103StableExpression(pass *analysis.Pass, expression ast.Expr, loop *ps6103Loop, parents map[ast.Node]ast.Node, exposed map[types.Object]bool) bool {
	stable := true
	roots := make(map[types.Object]bool)
	ast.Inspect(expression, func(node ast.Node) bool {
		if !stable {
			return false
		}
		switch value := node.(type) {
		case *ast.CallExpr, *ast.SliceExpr, *ast.CompositeLit, *ast.FuncLit:
			stable = false
			return false
		case *ast.UnaryExpr:
			if value.Op == token.ARROW {
				stable = false
				return false
			}
		case *ast.Ident:
			object := pass.TypesInfo.ObjectOf(value)
			if object == loop.index || object != nil && object.Pos() > loop.node.Pos() && object.Pos() < loop.node.End() {
				stable = false
				return false
			}
			if _, variable := object.(*types.Var); variable {
				roots[object] = true
			}
		}
		return true
	})
	for object := range roots {
		if ps6103ObjectMutable(pass, loop.body, object, parents, exposed) {
			return false
		}
	}
	return stable
}

// ps6103ObjectMutable reports the bounded mutations that can invalidate either
// a counted-loop trip proof or an operation's fixed-shape argument proof. It
// deliberately does not attempt general pointer or call-effect interpretation:
// direct writes plus cached function-wide exposure facts are sufficient to
// reject the supported source shapes conservatively.
func ps6103ObjectMutable(pass *analysis.Pass, scope ast.Node, object types.Object, parents map[ast.Node]ast.Node, exposed map[types.Object]bool) bool {
	if object == nil {
		return false
	}
	if exposed[object] {
		return true
	}
	mutable := false
	ast.Inspect(scope, func(node ast.Node) bool {
		if mutable {
			return false
		}
		identifier, ok := node.(*ast.Ident)
		if !ok || pass.TypesInfo.ObjectOf(identifier) != object {
			return true
		}
		if ps6103ObjectWrite(identifier, parents) {
			mutable = true
			return false
		}
		return true
	})
	return mutable
}

// ps6103ExposureFacts records address-taking, closure capture, and implicit
// pointer-method values once per function. Exposure may precede a loop and
// permit mutation through an alias during the loop; ordinary initialization
// assignments are intentionally not exposure facts.
func ps6103ExposureFacts(pass *analysis.Pass, body *ast.BlockStmt, parents map[ast.Node]ast.Node) map[types.Object]bool {
	exposed := make(map[types.Object]bool)
	astutil.WithStack(body, func(node ast.Node, stack []ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		object, variable := pass.TypesInfo.ObjectOf(identifier).(*types.Var)
		if !variable {
			return true
		}
		if ps6103AddressExposure(identifier, parents) || ps6103ImplicitPointerReceiver(pass, identifier, parents) {
			exposed[object] = true
			return true
		}
		for _, ancestor := range stack {
			if _, captured := ancestor.(*ast.FuncLit); captured {
				exposed[object] = true
				break
			}
		}
		return true
	})
	return exposed
}

func ps6103ObjectWrite(identifier *ast.Ident, parents map[ast.Node]ast.Node) bool {
	node := ps6103LValueRoot(identifier, parents)
	switch value := parents[node].(type) {
	case *ast.AssignStmt:
		for _, left := range value.Lhs {
			if left == node {
				return true
			}
		}
	case *ast.IncDecStmt:
		return value.X == node
	case *ast.RangeStmt:
		return value.Key == node || value.Value == node
	}
	return false
}

func ps6103AddressExposure(identifier *ast.Ident, parents map[ast.Node]ast.Node) bool {
	node := ps6103LValueRoot(identifier, parents)
	unary, ok := parents[node].(*ast.UnaryExpr)
	return ok && unary.Op == token.AND && unary.X == node
}

func ps6103LValueRoot(identifier *ast.Ident, parents map[ast.Node]ast.Node) ast.Node {
	var node ast.Node = identifier
	for {
		parent := parents[node]
		switch value := parent.(type) {
		case *ast.ParenExpr:
			node = value
		case *ast.SelectorExpr:
			if value.X != node && value.Sel != node {
				return node
			}
			node = value
		case *ast.IndexExpr:
			if value.X != node {
				return node
			}
			node = value
		case *ast.StarExpr:
			if value.X != node {
				return node
			}
			node = value
		default:
			return node
		}
	}
}

func ps6103ImplicitPointerReceiver(pass *analysis.Pass, identifier *ast.Ident, parents map[ast.Node]ast.Node) bool {
	var node ast.Node = identifier
	for {
		parent := parents[node]
		if parenthesis, ok := parent.(*ast.ParenExpr); ok {
			node = parenthesis
			continue
		}
		selector, ok := parent.(*ast.SelectorExpr)
		if !ok || ps2110Unparen(selector.X) != identifier {
			return false
		}
		return ps6103ImplicitPointerMethod(pass, selector, identifier)
	}
}

func ps6103ImplicitPointerMethod(pass *analysis.Pass, selector *ast.SelectorExpr, identifier *ast.Ident) bool {
	if identifier == nil || pass.TypesInfo.ObjectOf(identifier) == nil {
		return false
	}
	selection := pass.TypesInfo.Selections[selector]
	if selection == nil || selection.Kind() != types.MethodVal {
		return false
	}
	method, ok := selection.Obj().(*types.Func)
	if !ok {
		return false
	}
	signature, ok := method.Type().(*types.Signature)
	if !ok || signature.Recv() == nil {
		return false
	}
	if _, pointer := types.Unalias(signature.Recv().Type()).Underlying().(*types.Pointer); !pointer {
		return false
	}
	// An already-pointer receiver does not expose or permit rebinding the local
	// pointer itself. The unsafe case is the language's implicit &receiver.
	_, alreadyPointer := types.Unalias(pass.TypesInfo.TypeOf(selector.X)).Underlying().(*types.Pointer)
	return !alreadyPointer
}

func ps6103NoEarlierReturn(body *ast.BlockStmt, before token.Pos) bool {
	safe := true
	ast.Inspect(body, func(node ast.Node) bool {
		if !safe || node == nil || node.Pos() >= before {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		if _, returned := node.(*ast.ReturnStmt); returned {
			safe = false
			return false
		}
		return true
	})
	return safe
}

func ps6103CallSelector(expression ast.Expr) *ast.SelectorExpr {
	expression = ps2110Unparen(expression)
	switch value := expression.(type) {
	case *ast.IndexExpr:
		expression = ps2110Unparen(value.X)
	case *ast.IndexListExpr:
		expression = ps2110Unparen(value.X)
	}
	selector, _ := expression.(*ast.SelectorExpr)
	return selector
}

func ps6103DataResult(signature *types.Signature) (int, types.Type, bool) {
	if signature == nil || signature.Results() == nil {
		return 0, nil, false
	}
	index := -1
	var result types.Type
	for position := 0; position < signature.Results().Len(); position++ {
		typ := signature.Results().At(position).Type()
		if ps6103ErrorType(typ) {
			continue
		}
		if !ps6103ReusableType(typ) || index >= 0 {
			return 0, nil, false
		}
		index, result = position, typ
	}
	return index, result, index >= 0
}

func ps6103ReusableType(typ types.Type) bool {
	if typ == nil {
		return false
	}
	switch types.Unalias(typ).Underlying().(type) {
	case *types.Pointer, *types.Slice:
		return true
	}
	return false
}

func ps6103ErrorType(typ types.Type) bool {
	errorType := types.Universe.Lookup("error").Type()
	return typ != nil && types.Identical(types.Unalias(typ), errorType)
}

func ps6103ShortLived(pass *analysis.Pass, function *ast.BlockStmt, loop *ps6103Loop, call *ast.CallExpr, assignment *ast.AssignStmt, target *ast.Ident, object types.Object, parents map[ast.Node]ast.Node) bool {
	uses := 0
	safe := true
	ast.Inspect(function, func(node ast.Node) bool {
		if !safe {
			return false
		}
		identifier, ok := node.(*ast.Ident)
		if !ok || pass.TypesInfo.ObjectOf(identifier) != object {
			return true
		}
		if identifier == target {
			return true
		}
		if pass.TypesInfo.Defs[identifier] == object {
			return true
		}
		if identifier.Pos() <= call.End() || identifier.Pos() < loop.node.Pos() || identifier.End() > loop.node.End() || !ps6103ConsumedByCall(pass, identifier, loop.node, assignment, parents) {
			safe = false
			return false
		}
		uses++
		return true
	})
	return safe && uses > 0
}

func ps6103ConsumedByCall(pass *analysis.Pass, identifier *ast.Ident, loop ast.Node, assignment *ast.AssignStmt, parents map[ast.Node]ast.Node) bool {
	var child ast.Node = identifier
	consumed := false
	for node := parents[child]; node != nil && node != loop; node = parents[node] {
		switch value := node.(type) {
		case *ast.FuncLit, *ast.GoStmt, *ast.DeferStmt, *ast.SendStmt, *ast.ReturnStmt, *ast.CompositeLit:
			return false
		case *ast.UnaryExpr:
			if value.Op == token.AND {
				return false
			}
		case *ast.AssignStmt:
			if value != assignment && !consumed {
				return false
			}
		case *ast.CallExpr:
			if typedBuiltinName(pass, value.Fun, "append") {
				return false
			}
			if pass.TypesInfo.Types[value.Fun].IsType() {
				break
			}
			consumed = true
		}
		child = node
	}
	return consumed
}

func ps6103OperationEvidence(pass *analysis.Pass, call *ast.CallExpr, callee *types.Func, signature *types.Signature, resultIndex int, resultType types.Type, locals map[*types.Func]*ast.FuncDecl, allocating map[*types.Func]bool) (ps6103Evidence, bool) {
	if ps6103ConstructorName(callee.Name()) {
		return ps6103Evidence{}, false
	}
	allocated := allocating[callee.Origin()]
	if locals[callee.Origin()] == nil {
		var fact ps6103AllocationFact
		allocated = pass.ImportObjectFact(callee.Origin(), &fact)
	}
	if !allocated {
		return ps6103Evidence{}, false
	}
	if sibling, ok := ps6103IntoSibling(pass, call, callee, signature, resultIndex, resultType); ok {
		return ps6103Evidence{kind: "sibling", name: sibling.Name()}, true
	}
	if locals[callee.Origin()] == nil {
		return ps6103Evidence{}, false
	}
	return ps6103Evidence{kind: "local", name: callee.Name()}, true
}

func ps6103ConstructorName(name string) bool {
	return strings.HasPrefix(name, "New") || strings.HasPrefix(name, "Make")
}

func ps6103IntoSibling(pass *analysis.Pass, call *ast.CallExpr, callee *types.Func, signature *types.Signature, resultIndex int, resultType types.Type) (*types.Func, bool) {
	if callee == nil || callee.Pkg() == nil || signature == nil || signature.Variadic() {
		return nil, false
	}
	name := callee.Name() + "Into"
	declared, _ := callee.Type().(*types.Signature)
	var sibling *types.Func
	var siblingType types.Type
	if declared == nil || declared.Recv() == nil {
		sibling, _ = callee.Pkg().Scope().Lookup(name).(*types.Func)
		if sibling != nil {
			siblingType = sibling.Type()
		}
	} else if receiver := ps6103CallReceiver(pass, call.Fun); receiver != nil {
		if selection := types.NewMethodSet(receiver).Lookup(callee.Pkg(), name); selection != nil {
			sibling, _ = selection.Obj().(*types.Func)
			siblingType = selection.Type()
		}
	}
	if sibling == nil {
		return nil, false
	}
	if declared != nil && declared.Recv() != nil {
		siblingDeclared, _ := sibling.Type().(*types.Signature)
		if siblingDeclared == nil || siblingDeclared.Recv() == nil || ps6103ReceiverObject(declared.Recv().Type()) != ps6103ReceiverObject(siblingDeclared.Recv().Type()) {
			return nil, false
		}
	}
	if siblingSignature, ok := siblingType.(*types.Signature); ok && siblingSignature.TypeParams().Len() > 0 {
		arguments := ps6103TypeArguments(pass, call.Fun)
		if arguments == nil || arguments.Len() != siblingSignature.TypeParams().Len() {
			return nil, false
		}
		instantiated, err := types.Instantiate(nil, siblingType, ps6103Types(arguments), true)
		if err != nil {
			return nil, false
		}
		siblingType = instantiated
	}
	into, ok := siblingType.(*types.Signature)
	if !ok || into.Variadic() || into.Params().Len() != len(call.Args)+1 || !ps6103ResidualResults(signature.Results(), resultIndex, into.Results()) {
		return nil, false
	}
	for destination := 0; destination < into.Params().Len(); destination++ {
		if !types.Identical(resultType, into.Params().At(destination).Type()) {
			continue
		}
		argument := 0
		compatible := true
		for parameter := 0; parameter < into.Params().Len(); parameter++ {
			if parameter == destination {
				continue
			}
			if argument >= len(call.Args) || !types.AssignableTo(pass.TypesInfo.TypeOf(call.Args[argument]), into.Params().At(parameter).Type()) {
				compatible = false
				break
			}
			argument++
		}
		if compatible {
			return sibling, true
		}
	}
	return nil, false
}

func ps6103ReceiverObject(typ types.Type) *types.TypeName {
	typ = types.Unalias(typ)
	if pointer, ok := typ.(*types.Pointer); ok {
		typ = types.Unalias(pointer.Elem())
	}
	named, _ := typ.(*types.Named)
	if named == nil {
		return nil
	}
	return named.Origin().Obj()
}

func ps6103CallReceiver(pass *analysis.Pass, expression ast.Expr) types.Type {
	expression = ps2110Unparen(expression)
	switch value := expression.(type) {
	case *ast.IndexExpr:
		expression = ps2110Unparen(value.X)
	case *ast.IndexListExpr:
		expression = ps2110Unparen(value.X)
	}
	selector, ok := expression.(*ast.SelectorExpr)
	if !ok {
		return nil
	}
	selection := pass.TypesInfo.Selections[selector]
	if selection == nil {
		return nil
	}
	return selection.Recv()
}

func ps6103Types(list *types.TypeList) []types.Type {
	result := make([]types.Type, list.Len())
	for index := range list.Len() {
		result[index] = list.At(index)
	}
	return result
}

func ps6103ResidualResults(original *types.Tuple, data int, into *types.Tuple) bool {
	if original == nil || ps6103TupleLen(into) != original.Len()-1 {
		return false
	}
	position := 0
	for index := 0; index < original.Len(); index++ {
		if index == data {
			continue
		}
		if !types.Identical(original.At(index).Type(), into.At(position).Type()) {
			return false
		}
		position++
	}
	return true
}

func ps6103TupleLen(tuple *types.Tuple) int {
	if tuple == nil {
		return 0
	}
	return tuple.Len()
}

func ps6103TypeArguments(pass *analysis.Pass, expression ast.Expr) *types.TypeList {
	expression = ps2110Unparen(expression)
	switch value := expression.(type) {
	case *ast.IndexExpr:
		expression = ps2110Unparen(value.X)
	case *ast.IndexListExpr:
		expression = ps2110Unparen(value.X)
	}
	switch value := expression.(type) {
	case *ast.Ident:
		instance, ok := pass.TypesInfo.Instances[value]
		if ok {
			return instance.TypeArgs
		}
	case *ast.SelectorExpr:
		instance, ok := pass.TypesInfo.Instances[value.Sel]
		if ok {
			return instance.TypeArgs
		}
	}
	return nil
}

func ps6103AllocatesResult(pass *analysis.Pass, function *ast.FuncDecl, resultIndex int, resolvesAllocation func(*types.Func) bool) bool {
	if function == nil || function.Body == nil {
		return false
	}
	returns := 0
	valid := true
	ast.Inspect(function.Body, func(node ast.Node) bool {
		if !valid {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		returned, ok := node.(*ast.ReturnStmt)
		if !ok {
			return true
		}
		if len(returned.Results) == 1 {
			if call, ok := ps2110Unparen(returned.Results[0]).(*ast.CallExpr); ok && ps6103AllocatingCallResult(pass, call, resultIndex, resolvesAllocation) {
				returns++
				return true
			}
		}
		if resultIndex < 0 || resultIndex >= len(returned.Results) || !ps6103AllocationExpression(pass, function, returned.Results[resultIndex], resolvesAllocation) {
			valid = false
			return false
		}
		returns++
		return true
	})
	return valid && returns > 0
}

func ps6103AllocationExpression(pass *analysis.Pass, function *ast.FuncDecl, expression ast.Expr, resolvesAllocation func(*types.Func) bool) bool {
	expression = ps2110Unparen(expression)
	if ps6103DirectAllocation(pass, expression) {
		return true
	}
	if call, ok := expression.(*ast.CallExpr); ok && ps6103AllocatingCall(pass, call, resolvesAllocation) {
		return true
	}
	identifier, ok := expression.(*ast.Ident)
	if !ok {
		return false
	}
	object := pass.TypesInfo.ObjectOf(identifier)
	if object == nil || ps6103AllocationObjectEscapes(pass, function.Body, object) {
		return false
	}
	return ps6103AllocationStateAt(pass, function, object, expression.Pos(), resolvesAllocation) == ps6103Fresh
}

type ps6103AllocationState uint8

const (
	ps6103Unreachable ps6103AllocationState = iota
	ps6103Fresh
	ps6103NotFresh
	ps6103MaybeFresh
)

func ps6103AllocationStateAt(pass *analysis.Pass, function *ast.FuncDecl, object types.Object, position token.Pos, resolvesAllocation func(*types.Func) bool) ps6103AllocationState {
	graph := cfg.New(function.Body, func(*ast.CallExpr) bool { return true })
	states := make([]ps6103AllocationState, len(graph.Blocks))
	states[0] = ps6103NotFresh
	queue := []int{0}
	queued := make([]bool, len(graph.Blocks))
	queued[0] = true
	for len(queue) > 0 {
		index := queue[0]
		queue = queue[1:]
		queued[index] = false
		block := graph.Blocks[index]
		state := states[index]
		for _, node := range block.Nodes {
			state = ps6103AllocationTransfer(pass, node, object, state, resolvesAllocation)
		}
		for _, successor := range block.Succs {
			merged := ps6103MergeAllocationState(states[successor.Index], state)
			if merged == states[successor.Index] {
				continue
			}
			states[successor.Index] = merged
			if !queued[successor.Index] {
				queue = append(queue, int(successor.Index))
				queued[successor.Index] = true
			}
		}
	}
	for _, block := range graph.Blocks {
		if !block.Live || states[block.Index] == ps6103Unreachable {
			continue
		}
		state := states[block.Index]
		for _, node := range block.Nodes {
			if node.Pos() <= position && position <= node.End() {
				return state
			}
			state = ps6103AllocationTransfer(pass, node, object, state, resolvesAllocation)
		}
	}
	return ps6103NotFresh
}

func ps6103MergeAllocationState(current, incoming ps6103AllocationState) ps6103AllocationState {
	if current == ps6103Unreachable {
		return incoming
	}
	if incoming == ps6103Unreachable || current == incoming {
		return current
	}
	return ps6103MaybeFresh
}

func ps6103AllocationTransfer(pass *analysis.Pass, node ast.Node, object types.Object, current ps6103AllocationState, resolvesAllocation func(*types.Func) bool) ps6103AllocationState {
	switch value := node.(type) {
	case *ast.AssignStmt:
		for index, left := range value.Lhs {
			identifier, ok := ps2110Unparen(left).(*ast.Ident)
			if !ok || pass.TypesInfo.ObjectOf(identifier) != object {
				continue
			}
			if len(value.Rhs) == 1 && len(value.Lhs) > 1 {
				call, ok := ps2110Unparen(value.Rhs[0]).(*ast.CallExpr)
				if ok && ps6103AllocatingCallResult(pass, call, index, resolvesAllocation) {
					return ps6103Fresh
				}
				return ps6103NotFresh
			}
			if index >= len(value.Rhs) {
				return ps6103NotFresh
			}
			return ps6103AllocationExpressionState(pass, value.Rhs[index], object, current, resolvesAllocation)
		}
	case *ast.ValueSpec:
		for index, name := range value.Names {
			if pass.TypesInfo.ObjectOf(name) != object {
				continue
			}
			if len(value.Values) == 1 && len(value.Names) > 1 {
				call, ok := ps2110Unparen(value.Values[0]).(*ast.CallExpr)
				if ok && ps6103AllocatingCallResult(pass, call, index, resolvesAllocation) {
					return ps6103Fresh
				}
				return ps6103NotFresh
			}
			if index >= len(value.Values) {
				return ps6103NotFresh
			}
			return ps6103AllocationExpressionState(pass, value.Values[index], object, current, resolvesAllocation)
		}
	}
	return current
}

func ps6103AllocationExpressionState(pass *analysis.Pass, expression ast.Expr, object types.Object, current ps6103AllocationState, resolvesAllocation func(*types.Func) bool) ps6103AllocationState {
	expression = ps2110Unparen(expression)
	if identifier, ok := expression.(*ast.Ident); ok && pass.TypesInfo.ObjectOf(identifier) == object {
		return current
	}
	if ps6103DirectAllocation(pass, expression) {
		return ps6103Fresh
	}
	if call, ok := expression.(*ast.CallExpr); ok && ps6103AllocatingCall(pass, call, resolvesAllocation) {
		return ps6103Fresh
	}
	return ps6103NotFresh
}

func ps6103AllocationObjectEscapes(pass *analysis.Pass, body *ast.BlockStmt, object types.Object) bool {
	escaped := false
	ast.Inspect(body, func(node ast.Node) bool {
		if escaped {
			return false
		}
		if function, ok := node.(*ast.FuncLit); ok {
			ast.Inspect(function.Body, func(nested ast.Node) bool {
				identifier, ok := nested.(*ast.Ident)
				if ok && pass.TypesInfo.ObjectOf(identifier) == object {
					escaped = true
					return false
				}
				return !escaped
			})
			return false
		}
		if selector, ok := node.(*ast.SelectorExpr); ok {
			identifier, _ := ps2110Unparen(selector.X).(*ast.Ident)
			if identifier != nil && pass.TypesInfo.ObjectOf(identifier) == object && ps6103ImplicitPointerMethod(pass, selector, identifier) {
				escaped = true
				return false
			}
		}
		unary, ok := node.(*ast.UnaryExpr)
		if !ok || unary.Op != token.AND {
			return true
		}
		identifier, ok := ps2110Unparen(unary.X).(*ast.Ident)
		if ok && pass.TypesInfo.ObjectOf(identifier) == object {
			escaped = true
			return false
		}
		return true
	})
	return escaped
}

func ps6103AllocatingCall(pass *analysis.Pass, call *ast.CallExpr, resolvesAllocation func(*types.Func) bool) bool {
	return ps6103AllocatingCallResult(pass, call, 0, resolvesAllocation)
}

func ps6103AllocatingCallResult(pass *analysis.Pass, call *ast.CallExpr, resultIndex int, resolvesAllocation func(*types.Func) bool) bool {
	callee, signature, ok := typedCallee(pass, call.Fun)
	if !ok {
		return false
	}
	dataIndex, _, ok := ps6103DataResult(signature)
	if !ok || dataIndex != resultIndex {
		return false
	}
	return resolvesAllocation(callee.Origin())
}

func ps6103DirectAllocation(pass *analysis.Pass, expression ast.Expr) bool {
	switch value := ps2110Unparen(expression).(type) {
	case *ast.CallExpr:
		return len(value.Args) > 0 && (typedBuiltinName(pass, value.Fun, "make") || typedBuiltinName(pass, value.Fun, "new"))
	case *ast.UnaryExpr:
		_, composite := ps2110Unparen(value.X).(*ast.CompositeLit)
		return value.Op == token.AND && composite
	}
	return false
}
