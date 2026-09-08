package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"math/big"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/internal/astutil"
)

const ps4001PlaneMaximumFields = 64

type ps4001PlaneState struct {
	initializers map[types.Object]ast.Expr
	writes       map[types.Object]int
	escaped      map[types.Object]bool
}

type ps4001PlaneLoop struct {
	variable types.Object
	count    int64
}

type ps4001PlaneFinding struct {
	method string
	fields int64
	width  int64
	span   int64
}

type ps4001PlaneStore struct {
	object types.Object
}

func ps4001PlaneReport(pass *analysis.Pass, call *ast.CallExpr, finding ps4001PlaneFinding) {
	pass.Reportf(call.Pos(), "fixed %d-field/%d-byte contiguous binary.%s plane is decoded one scalar at a time into a local array and then completely reduced in order inside each surrounding record; on validated little-endian amd64/arm64 paths benchmark one architecture-keyed uint16 view, hoist the capability outside the record loop, validate the complete record before forming the view, and retain binary.LittleEndian as the endian/alignment-safe fallback with bit-exact raw-field and complete-consumer gates (advisory, no automatic fix)", finding.fields, finding.span, finding.method)
}

func ps4001PlaneFixedPlane(pass *analysis.Pass, call *ast.CallExpr, loop ast.Node, stack []ast.Node, state ps4001PlaneState) (ps4001PlaneFinding, bool) {
	method, ok := isBinaryEndianCall(pass.TypesInfo, call)
	if !ok || len(call.Args) != 1 {
		return ps4001PlaneFinding{}, false
	}
	if method != "LittleEndian.Uint16" {
		return ps4001PlaneFinding{}, false
	}
	const width = int64(2)
	inner, ok := ps4001PlaneFixedLoop(pass, loop)
	if !ok {
		return ps4001PlaneFinding{}, false
	}
	outer, ok := ps4001PlaneOuterLoop(stack, loop)
	if !ok || !ps4001PlaneLoopObjectsStable(pass, outer, state) {
		return ps4001PlaneFinding{}, false
	}
	loopVariables := ps4001PlaneLoopObjects(pass, outer, state)
	loopVariables[inner.variable] = true
	slice, ok := ps2110Unparen(call.Args[0]).(*ast.SliceExpr)
	if !ok || slice.Low == nil || slice.Slice3 || !ps4001PlaneByteSlice(pass, slice) ||
		!ps4001PlaneIndependentOfObject(pass, slice.X, inner.variable, state, make(map[types.Object]bool)) ||
		!ps4001PlaneStableValue(pass, slice.X, state, loopVariables, make(map[types.Object]bool)) ||
		!ps4001PlaneStableValue(pass, slice.Low, state, loopVariables, make(map[types.Object]bool)) ||
		!ps4001PlaneCallMustExecute(stack, loop) {
		return ps4001PlaneFinding{}, false
	}
	store, ok := ps4001PlaneDirectDecodeStore(pass, call, loop, slice.X, inner, state)
	if !ok || !ps4001PlaneCompleteConsumer(pass, outer, loop, store) {
		return ps4001PlaneFinding{}, false
	}
	coefficient, ok := ps4001PlaneCoefficient(pass, slice.Low, inner.variable, state, make(map[types.Object]bool))
	if !ok || coefficient != width || !ps4001PlaneNativeIndex(pass, slice.Low) || !ps4001PlaneHighMatches(pass, slice.Low, slice.High, width) {
		return ps4001PlaneFinding{}, false
	}
	if !ps4001PlaneDependsOnOuter(pass, slice.X, slice.Low, outer, state) {
		return ps4001PlaneFinding{}, false
	}
	return ps4001PlaneFinding{method: method, fields: inner.count, width: width, span: inner.count * width}, true
}

// ps4001PlaneIndependentOfObject rejects per-field source selection. The
// scalar offsets may depend on the inner induction, but all words in one
// reported plane must come from the same backing byte slice.
func ps4001PlaneIndependentOfObject(pass *analysis.Pass, expression ast.Expr, target types.Object,
	state ps4001PlaneState, seen map[types.Object]bool) bool {
	if ps4001PlaneMentions(pass, expression, target) {
		return false
	}
	independent := true
	ast.Inspect(expression, func(node ast.Node) bool {
		if !independent {
			return false
		}
		identifier, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		object := identObject(pass, identifier)
		initializer, found := state.initializers[object]
		if object == nil || !found || state.writes[object] != 1 || state.escaped[object] {
			return true
		}
		if seen[object] {
			independent = false
			return false
		}
		seen[object] = true
		independent = ps4001PlaneIndependentOfObject(pass, initializer, target, state, seen)
		delete(seen, object)
		return independent
	})
	return independent
}

// ps4001PlaneDirectDecodeStore pins the inner loop to one direct scalar store.
// A sibling call or assignment could rebind a selector/index source between
// fields, in which case the iterations do not describe one backing plane.
func ps4001PlaneDirectDecodeStore(pass *analysis.Pass, call *ast.CallExpr, loop ast.Node, source ast.Expr,
	inner ps4001PlaneLoop, state ps4001PlaneState) (ps4001PlaneStore, bool) {
	body := astutil.LoopBody(loop)
	if body == nil || len(body.List) != 1 {
		return ps4001PlaneStore{}, false
	}
	assignment, ok := body.List[0].(*ast.AssignStmt)
	if !ok || (assignment.Tok != token.ASSIGN && assignment.Tok != token.DEFINE) || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 || ps2110Unparen(assignment.Rhs[0]) != call {
		return ps4001PlaneStore{}, false
	}
	destination, ok := ps2110Unparen(assignment.Lhs[0]).(*ast.IndexExpr)
	if !ok || !ps4001PlaneObject(pass, destination.Index, inner.variable) {
		return ps4001PlaneStore{}, false
	}
	plane, ok := ps2110Unparen(destination.X).(*ast.Ident)
	if !ok {
		return ps4001PlaneStore{}, false
	}
	planeObject := identObject(pass, plane)
	planeVariable, ok := planeObject.(*types.Var)
	if !ok {
		return ps4001PlaneStore{}, false
	}
	array, arrayOK := types.Unalias(planeVariable.Type()).Underlying().(*types.Array)
	if !arrayOK {
		return ps4001PlaneStore{}, false
	}
	element, elementOK := array.Elem().Underlying().(*types.Basic)
	if !elementOK || element.Kind() != types.Uint16 || array.Len() != inner.count ||
		planeVariable.Pkg() == nil || planeVariable.Parent() == planeVariable.Pkg().Scope() ||
		state.writes[planeObject] > 1 || state.escaped[planeObject] {
		return ps4001PlaneStore{}, false
	}
	sourceRoot, sourceOK := ps4001PlaneStorageRoot(pass, source)
	return ps4001PlaneStore{object: planeObject}, sourceOK && sourceRoot != planeObject
}

// ps4001PlaneCompleteConsumer keeps the specialized view advice to a local
// materialization whose complete contents are consumed immediately and never
// escape. Calls are deliberately excluded: an opaque consumer could retain the
// proposed zero-copy view or mutate its source before finishing the read.
func ps4001PlaneCompleteConsumer(pass *analysis.Pass, outer, inner ast.Node, store ps4001PlaneStore) bool {
	body := astutil.LoopBody(outer)
	if body == nil {
		return false
	}
	innerIndex := -1
	for index, statement := range body.List {
		if statement == inner {
			innerIndex = index
			break
		}
	}
	if innerIndex <= 0 || innerIndex+1 >= len(body.List) ||
		!ps4001PlaneDeclaresObject(pass, body.List[innerIndex-1], store.object) {
		return false
	}
	consumer, ok := body.List[innerIndex+1].(*ast.RangeStmt)
	if !ok || consumer.Tok != token.DEFINE || consumer.Key == nil || consumer.Value == nil ||
		!ps4001PlaneObject(pass, consumer.X, store.object) || len(consumer.Body.List) != 1 {
		return false
	}
	key, keyOK := ps2110Unparen(consumer.Key).(*ast.Ident)
	value, valueOK := ps2110Unparen(consumer.Value).(*ast.Ident)
	if !keyOK || key.Name != "_" || !valueOK || value.Name == "_" {
		return false
	}
	valueObject := identObject(pass, value)
	assignment, ok := consumer.Body.List[0].(*ast.AssignStmt)
	if !ok || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 ||
		assignment.Tok == token.ASSIGN || assignment.Tok == token.DEFINE ||
		!ps4001PlaneConsumerValue(pass, assignment.Rhs[0], valueObject) {
		return false
	}
	accumulator, ok := ps2110Unparen(assignment.Lhs[0]).(*ast.Ident)
	if !ok || accumulator.Name == "_" || identObject(pass, accumulator) == store.object {
		return false
	}
	uses := 0
	ast.Inspect(body, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if ok && pass.TypesInfo.Uses[identifier] == store.object {
			uses++
		}
		return true
	})
	return uses == 2
}

func ps4001PlaneConsumerValue(pass *analysis.Pass, expression ast.Expr, value types.Object) bool {
	expression = ps2110Unparen(expression)
	if ps4001PlaneObject(pass, expression, value) {
		return true
	}
	call, ok := expression.(*ast.CallExpr)
	if !ok || len(call.Args) != 1 || call.Ellipsis.IsValid() {
		return false
	}
	typed, ok := pass.TypesInfo.Types[call.Fun]
	if !ok || !typed.IsType() {
		return false
	}
	basic, ok := types.Unalias(typed.Type).Underlying().(*types.Basic)
	return ok && basic.Info()&types.IsInteger != 0 && ps4001PlaneConsumerValue(pass, call.Args[0], value)
}

func ps4001PlaneDeclaresObject(pass *analysis.Pass, statement ast.Stmt, object types.Object) bool {
	switch current := statement.(type) {
	case *ast.DeclStmt:
		general, ok := current.Decl.(*ast.GenDecl)
		if !ok || general.Tok != token.VAR {
			return false
		}
		for _, spec := range general.Specs {
			values, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for index, name := range values.Names {
				if pass.TypesInfo.Defs[name] == object {
					return len(values.Values) == 0 || index < len(values.Values) && ps4001PlaneEmptyComposite(values.Values[index])
				}
			}
		}
	case *ast.AssignStmt:
		if current.Tok != token.DEFINE {
			return false
		}
		for index, left := range current.Lhs {
			identifier, ok := ps2110Unparen(left).(*ast.Ident)
			if ok && pass.TypesInfo.Defs[identifier] == object {
				return index < len(current.Rhs) && ps4001PlaneEmptyComposite(current.Rhs[index])
			}
		}
	}
	return false
}

func ps4001PlaneEmptyComposite(expression ast.Expr) bool {
	literal, ok := ps2110Unparen(expression).(*ast.CompositeLit)
	return ok && len(literal.Elts) == 0
}

func ps4001PlaneStorageRoot(pass *analysis.Pass, expression ast.Expr) (types.Object, bool) {
	switch current := ps2110Unparen(expression).(type) {
	case *ast.Ident:
		object := identObject(pass, current)
		return object, object != nil
	case *ast.SelectorExpr:
		return ps4001PlaneStorageRoot(pass, current.X)
	case *ast.IndexExpr:
		return ps4001PlaneStorageRoot(pass, current.X)
	case *ast.IndexListExpr:
		return ps4001PlaneStorageRoot(pass, current.X)
	case *ast.SliceExpr:
		return ps4001PlaneStorageRoot(pass, current.X)
	default:
		return nil, false
	}
}

func ps4001PlaneCollectState(pass *analysis.Pass, function *ast.FuncDecl) ps4001PlaneState {
	state := ps4001PlaneState{
		initializers: make(map[types.Object]ast.Expr),
		writes:       make(map[types.Object]int),
		escaped:      make(map[types.Object]bool),
	}
	ast.Inspect(function.Body, func(node ast.Node) bool {
		switch current := node.(type) {
		case *ast.ValueSpec:
			if len(current.Names) != len(current.Values) {
				return true
			}
			for index, name := range current.Names {
				if object := pass.TypesInfo.Defs[name]; object != nil {
					state.writes[object]++
					state.initializers[object] = current.Values[index]
				}
			}
		case *ast.AssignStmt:
			paired := len(current.Lhs) == len(current.Rhs)
			for index, left := range current.Lhs {
				identifier, ok := ps2110Unparen(left).(*ast.Ident)
				if !ok || identifier.Name == "_" {
					continue
				}
				object := identObject(pass, identifier)
				if object == nil {
					continue
				}
				state.writes[object]++
				if current.Tok == token.DEFINE && pass.TypesInfo.Defs[identifier] != nil && paired {
					state.initializers[object] = current.Rhs[index]
				}
			}
		case *ast.IncDecStmt:
			if identifier, ok := ps2110Unparen(current.X).(*ast.Ident); ok {
				if object := identObject(pass, identifier); object != nil {
					state.writes[object]++
				}
			}
		case *ast.RangeStmt:
			for _, expression := range []ast.Expr{current.Key, current.Value} {
				if identifier, ok := ps2110Unparen(expression).(*ast.Ident); ok && identifier.Name != "_" {
					if object := identObject(pass, identifier); object != nil {
						state.writes[object]++
					}
				}
			}
		case *ast.UnaryExpr:
			if current.Op == token.AND {
				if identifier, ok := ps2110Unparen(current.X).(*ast.Ident); ok {
					if object := identObject(pass, identifier); object != nil {
						state.escaped[object] = true
					}
				}
			}
		case *ast.SelectorExpr:
			selection := pass.TypesInfo.Selections[current]
			if selection == nil || selection.Kind() != types.MethodVal {
				return true
			}
			signature, _ := selection.Obj().Type().(*types.Signature)
			if signature == nil || signature.Recv() == nil {
				return true
			}
			if _, pointer := signature.Recv().Type().(*types.Pointer); pointer {
				if identifier, ok := ps2110Unparen(current.X).(*ast.Ident); ok {
					if object := identObject(pass, identifier); object != nil {
						state.escaped[object] = true
					}
				}
			}
		}
		return true
	})
	return state
}

func ps4001PlaneFixedLoop(pass *analysis.Pass, node ast.Node) (ps4001PlaneLoop, bool) {
	switch loop := node.(type) {
	case *ast.ForStmt:
		initializer, ok := loop.Init.(*ast.AssignStmt)
		if !ok || initializer.Tok != token.DEFINE || len(initializer.Lhs) != 1 || len(initializer.Rhs) != 1 || !ps4001PlaneConstant(pass, initializer.Rhs[0], 0) {
			return ps4001PlaneLoop{}, false
		}
		identifier, ok := ps2110Unparen(initializer.Lhs[0]).(*ast.Ident)
		if !ok {
			return ps4001PlaneLoop{}, false
		}
		variable := identObject(pass, identifier)
		condition, ok := ps2110Unparen(loop.Cond).(*ast.BinaryExpr)
		if variable == nil || !ok || condition.Op != token.LSS || !ps4001PlaneObject(pass, condition.X, variable) {
			return ps4001PlaneLoop{}, false
		}
		count, ok := ps4001PlaneConstantInt(pass, condition.Y)
		if !ok || count < 2 || count > ps4001PlaneMaximumFields {
			return ps4001PlaneLoop{}, false
		}
		post, ok := loop.Post.(*ast.IncDecStmt)
		if !ok || post.Tok != token.INC || !ps4001PlaneObject(pass, post.X, variable) || ps4001PlaneLoopVariableEscapes(pass, loop.Body, variable) {
			return ps4001PlaneLoop{}, false
		}
		return ps4001PlaneLoop{variable: variable, count: count}, true
	case *ast.RangeStmt:
		identifier, ok := ps2110Unparen(loop.Key).(*ast.Ident)
		if !ok || identifier.Name == "_" || loop.Value != nil || loop.Tok != token.DEFINE {
			return ps4001PlaneLoop{}, false
		}
		variable := identObject(pass, identifier)
		count, ok := ps4001PlaneRangeCount(pass, loop.X)
		if variable == nil || !ok || count < 2 || count > ps4001PlaneMaximumFields || ps4001PlaneLoopVariableEscapes(pass, loop.Body, variable) {
			return ps4001PlaneLoop{}, false
		}
		return ps4001PlaneLoop{variable: variable, count: count}, true
	default:
		return ps4001PlaneLoop{}, false
	}
}

func ps4001PlaneRangeCount(pass *analysis.Pass, expression ast.Expr) (int64, bool) {
	if count, ok := ps4001PlaneConstantInt(pass, expression); ok {
		return count, true
	}
	typeOf := pass.TypesInfo.TypeOf(expression)
	if typeOf == nil {
		return 0, false
	}
	if pointer, ok := typeOf.Underlying().(*types.Pointer); ok {
		typeOf = pointer.Elem()
	}
	array, ok := typeOf.Underlying().(*types.Array)
	if !ok {
		return 0, false
	}
	return array.Len(), true
}

func ps4001PlaneLoopVariableEscapes(pass *analysis.Pass, body *ast.BlockStmt, variable types.Object) bool {
	bad := false
	ast.Inspect(body, func(node ast.Node) bool {
		if bad {
			return false
		}
		switch current := node.(type) {
		case *ast.AssignStmt:
			for _, left := range current.Lhs {
				if ps4001PlaneObject(pass, left, variable) {
					bad = true
					return false
				}
			}
		case *ast.IncDecStmt:
			bad = ps4001PlaneObject(pass, current.X, variable)
		case *ast.RangeStmt:
			bad = ps4001PlaneObject(pass, current.Key, variable) || ps4001PlaneObject(pass, current.Value, variable)
		case *ast.UnaryExpr:
			bad = current.Op == token.AND && ps4001PlaneObject(pass, current.X, variable)
		case *ast.SelectorExpr:
			selection := pass.TypesInfo.Selections[current]
			if selection != nil && selection.Kind() == types.MethodVal {
				signature, _ := selection.Obj().Type().(*types.Signature)
				if signature != nil && signature.Recv() != nil {
					_, pointer := signature.Recv().Type().(*types.Pointer)
					bad = pointer && ps4001PlaneObject(pass, current.X, variable)
				}
			}
		case *ast.BranchStmt, *ast.ReturnStmt:
			bad = true
		case *ast.CallExpr:
			if identifier, ok := ps2110Unparen(current.Fun).(*ast.Ident); ok {
				if builtin, ok := pass.TypesInfo.Uses[identifier].(*types.Builtin); ok && builtin.Name() == "panic" {
					bad = true
				}
			}
		}
		return !bad
	})
	return bad
}

func ps4001PlaneOuterLoop(stack []ast.Node, inner ast.Node) (ast.Node, bool) {
	var previous ast.Node
	for index := 1; index < len(stack); index++ {
		if _, boundary := stack[index-1].(*ast.FuncLit); boundary {
			previous = nil
			continue
		}
		if !astutil.IsLoop(stack[index-1]) || astutil.LoopBody(stack[index-1]) != stack[index] {
			continue
		}
		current := stack[index-1]
		if current == inner {
			return previous, previous != nil
		}
		previous = current
	}
	return nil, false
}

func ps4001PlaneCallMustExecute(stack []ast.Node, loop ast.Node) bool {
	loopIndex := -1
	for index, ancestor := range stack {
		if ancestor == loop {
			loopIndex = index
			break
		}
	}
	if loopIndex < 0 {
		return false
	}
	for _, ancestor := range stack[loopIndex+1:] {
		switch current := ancestor.(type) {
		case *ast.IfStmt, *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt,
			*ast.CaseClause, *ast.CommClause, *ast.GoStmt, *ast.DeferStmt, *ast.FuncLit:
			return false
		case *ast.BinaryExpr:
			if current.Op == token.LAND || current.Op == token.LOR {
				return false
			}
		}
	}
	return true
}

func ps4001PlaneCoefficient(pass *analysis.Pass, expression ast.Expr, variable types.Object, state ps4001PlaneState, seen map[types.Object]bool) (int64, bool) {
	expression = ps2110Unparen(expression)
	if ps4001PlaneObject(pass, expression, variable) {
		return 1, true
	}
	if identifier, ok := expression.(*ast.Ident); ok {
		object := identObject(pass, identifier)
		initializer, found := state.initializers[object]
		if object != nil && found && state.writes[object] == 1 && !state.escaped[object] && !seen[object] {
			seen[object] = true
			coefficient, valid := ps4001PlaneCoefficient(pass, initializer, variable, state, seen)
			delete(seen, object)
			return coefficient, valid
		}
	}
	switch current := expression.(type) {
	case *ast.BasicLit:
		return 0, true
	case *ast.UnaryExpr:
		if current.Op != token.ADD && current.Op != token.SUB {
			return 0, false
		}
		coefficient, ok := ps4001PlaneCoefficient(pass, current.X, variable, state, seen)
		if current.Op == token.SUB {
			return ps4001PlaneSafeNegate(coefficient, ok)
		}
		return coefficient, ok
	case *ast.BinaryExpr:
		switch current.Op {
		case token.ADD, token.SUB:
			left, leftOK := ps4001PlaneCoefficient(pass, current.X, variable, state, seen)
			right, rightOK := ps4001PlaneCoefficient(pass, current.Y, variable, state, seen)
			if !leftOK || !rightOK {
				return 0, false
			}
			if current.Op == token.SUB {
				var valid bool
				right, valid = ps4001PlaneSafeNegate(right, true)
				if !valid {
					return 0, false
				}
			}
			return ps4001PlaneSafeAdd(left, right)
		case token.MUL:
			if factor, ok := ps4001PlaneConstantInt(pass, current.X); ok {
				coefficient, valid := ps4001PlaneCoefficient(pass, current.Y, variable, state, seen)
				return ps4001PlaneSafeMultiply(coefficient, factor, valid)
			}
			if factor, ok := ps4001PlaneConstantInt(pass, current.Y); ok {
				coefficient, valid := ps4001PlaneCoefficient(pass, current.X, variable, state, seen)
				return ps4001PlaneSafeMultiply(coefficient, factor, valid)
			}
		}
	}
	if !ps4001PlaneMentions(pass, expression, variable) {
		return 0, true
	}
	return 0, false
}

func ps4001PlaneSafeNegate(value int64, valid bool) (int64, bool) {
	if !valid {
		return 0, false
	}
	result := new(big.Int).Neg(big.NewInt(value))
	if !result.IsInt64() {
		return 0, false
	}
	return result.Int64(), true
}

func ps4001PlaneSafeAdd(left, right int64) (int64, bool) {
	result := new(big.Int).Add(big.NewInt(left), big.NewInt(right))
	if !result.IsInt64() {
		return 0, false
	}
	return result.Int64(), true
}

func ps4001PlaneSafeMultiply(value, factor int64, valid bool) (int64, bool) {
	if !valid {
		return 0, false
	}
	result := new(big.Int).Mul(big.NewInt(value), big.NewInt(factor))
	if !result.IsInt64() {
		return 0, false
	}
	return result.Int64(), true
}

func ps4001PlaneHighMatches(pass *analysis.Pass, low, high ast.Expr, width int64) bool {
	if high == nil {
		return true
	}
	binary, ok := ps2110Unparen(high).(*ast.BinaryExpr)
	if !ok || binary.Op != token.ADD {
		return false
	}
	if value, ok := ps4001PlaneConstantInt(pass, binary.X); ok && value == width {
		return ps4001PlaneSameSource(pass, binary.Y, low)
	}
	if value, ok := ps4001PlaneConstantInt(pass, binary.Y); ok && value == width {
		return ps4001PlaneSameSource(pass, binary.X, low)
	}
	return false
}

func ps4001PlaneSameSource(pass *analysis.Pass, left, right ast.Expr) bool {
	left, right = ps2110Unparen(left), ps2110Unparen(right)
	switch leftValue := left.(type) {
	case *ast.Ident:
		rightValue, ok := right.(*ast.Ident)
		return ok && identObject(pass, leftValue) == identObject(pass, rightValue) && leftValue.Name == rightValue.Name
	case *ast.BasicLit:
		rightValue, ok := right.(*ast.BasicLit)
		return ok && leftValue.Kind == rightValue.Kind && leftValue.Value == rightValue.Value
	case *ast.UnaryExpr:
		rightValue, ok := right.(*ast.UnaryExpr)
		return ok && leftValue.Op == rightValue.Op && ps4001PlaneSameSource(pass, leftValue.X, rightValue.X)
	case *ast.BinaryExpr:
		rightValue, ok := right.(*ast.BinaryExpr)
		return ok && leftValue.Op == rightValue.Op && ps4001PlaneSameSource(pass, leftValue.X, rightValue.X) && ps4001PlaneSameSource(pass, leftValue.Y, rightValue.Y)
	case *ast.IndexExpr:
		rightValue, ok := right.(*ast.IndexExpr)
		return ok && ps4001PlaneSameSource(pass, leftValue.X, rightValue.X) && ps4001PlaneSameSource(pass, leftValue.Index, rightValue.Index)
	case *ast.SelectorExpr:
		rightValue, ok := right.(*ast.SelectorExpr)
		return ok && pass.TypesInfo.ObjectOf(leftValue.Sel) == pass.TypesInfo.ObjectOf(rightValue.Sel) && ps4001PlaneSameSource(pass, leftValue.X, rightValue.X)
	case *ast.CallExpr:
		rightValue, ok := right.(*ast.CallExpr)
		if !ok || len(leftValue.Args) != len(rightValue.Args) || !ps4001PlaneSameSource(pass, leftValue.Fun, rightValue.Fun) {
			return false
		}
		for index := range leftValue.Args {
			if !ps4001PlaneSameSource(pass, leftValue.Args[index], rightValue.Args[index]) {
				return false
			}
		}
		return true
	case *ast.StarExpr:
		rightValue, ok := right.(*ast.StarExpr)
		return ok && ps4001PlaneSameSource(pass, leftValue.X, rightValue.X)
	default:
		return false
	}
}

func ps4001PlaneByteSlice(pass *analysis.Pass, expression ast.Expr) bool {
	typeOf := pass.TypesInfo.TypeOf(expression)
	if typeOf == nil {
		return false
	}
	slice, ok := typeOf.Underlying().(*types.Slice)
	if !ok {
		return false
	}
	basic, ok := slice.Elem().Underlying().(*types.Basic)
	return ok && basic.Kind() == types.Uint8
}

func ps4001PlaneNativeIndex(pass *analysis.Pass, expression ast.Expr) bool {
	typeOf := pass.TypesInfo.TypeOf(expression)
	if typeOf == nil {
		return false
	}
	basic, ok := typeOf.Underlying().(*types.Basic)
	return ok && basic.Kind() == types.Int
}

func ps4001PlaneStableSource(pass *analysis.Pass, expression ast.Expr) bool {
	stable := true
	ast.Inspect(expression, func(node ast.Node) bool {
		if !stable {
			return false
		}
		switch current := node.(type) {
		case *ast.FuncLit:
			stable = false
		case *ast.UnaryExpr:
			if current.Op == token.ARROW {
				stable = false
			}
		case *ast.CallExpr:
			typed, ok := pass.TypesInfo.Types[current.Fun]
			if !ok || !typed.IsType() {
				stable = false
				break
			}
			basic, ok := typed.Type.Underlying().(*types.Basic)
			if !ok || basic.Info()&types.IsInteger == 0 {
				stable = false
			}
		case *ast.IndexExpr:
			typeOf := pass.TypesInfo.TypeOf(current.X)
			if typeOf == nil {
				stable = false
				break
			}
			if _, mapType := typeOf.Underlying().(*types.Map); mapType {
				stable = false
			}
		}
		return stable
	})
	return stable
}

func ps4001PlaneStableValue(pass *analysis.Pass, expression ast.Expr, state ps4001PlaneState, loopVariables map[types.Object]bool, seen map[types.Object]bool) bool {
	if !ps4001PlaneStableSource(pass, expression) {
		return false
	}
	stable := true
	ast.Inspect(expression, func(node ast.Node) bool {
		if !stable {
			return false
		}
		identifier, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		object := identObject(pass, identifier)
		if loopVariables[object] {
			return true
		}
		if variable, ok := object.(*types.Var); ok && variable.Pkg() != nil && variable.Parent() == variable.Pkg().Scope() {
			stable = false
			return false
		}
		initializer, found := state.initializers[object]
		if object == nil {
			return true
		}
		if !found {
			if state.writes[object] > 0 || state.escaped[object] {
				stable = false
				return false
			}
			return true
		}
		if state.writes[object] != 1 || state.escaped[object] || seen[object] {
			stable = false
			return false
		}
		seen[object] = true
		stable = ps4001PlaneStableValue(pass, initializer, state, loopVariables, seen)
		delete(seen, object)
		return stable
	})
	return stable
}

func ps4001PlaneDependsOnOuter(pass *analysis.Pass, source, low ast.Expr, outer ast.Node, state ps4001PlaneState) bool {
	objects := ps4001PlaneLoopObjects(pass, outer, state)
	seen := make(map[types.Object]bool)
	return ps4001PlaneExpressionDepends(pass, source, objects, state, seen) || ps4001PlaneExpressionDepends(pass, low, objects, state, seen)
}

func ps4001PlaneExpressionDepends(pass *analysis.Pass, expression ast.Expr, loopObjects map[types.Object]bool, state ps4001PlaneState, seen map[types.Object]bool) bool {
	for object := range loopObjects {
		if ps4001PlaneDependsOnObject(pass, expression, object, state, seen) {
			return true
		}
	}
	return false
}

// ps4001PlaneDependsOnObject proves a non-cancelling dependency. Merely
// finding the induction variable is insufficient: record-record is constant
// and must not make an otherwise whole-buffer loop look record-relative.
func ps4001PlaneDependsOnObject(pass *analysis.Pass, expression ast.Expr, object types.Object, state ps4001PlaneState, seen map[types.Object]bool) bool {
	expression = ps2110Unparen(expression)
	if coefficient, ok := ps4001PlaneCoefficient(pass, expression, object, state, make(map[types.Object]bool)); ok {
		return coefficient != 0
	}
	if identifier, ok := expression.(*ast.Ident); ok {
		alias := identObject(pass, identifier)
		initializer, found := state.initializers[alias]
		if alias == nil || !found || state.writes[alias] != 1 || state.escaped[alias] || seen[alias] {
			return false
		}
		seen[alias] = true
		depends := ps4001PlaneDependsOnObject(pass, initializer, object, state, seen)
		delete(seen, alias)
		return depends
	}
	if !ps4001PlaneMentions(pass, expression, object) {
		return false
	}
	switch current := expression.(type) {
	case *ast.IndexExpr:
		return ps4001PlaneDependsOnObject(pass, current.X, object, state, seen) ||
			ps4001PlaneDependsOnObject(pass, current.Index, object, state, seen)
	case *ast.IndexListExpr:
		if ps4001PlaneDependsOnObject(pass, current.X, object, state, seen) {
			return true
		}
		for _, index := range current.Indices {
			if ps4001PlaneDependsOnObject(pass, index, object, state, seen) {
				return true
			}
		}
	case *ast.SliceExpr:
		if ps4001PlaneDependsOnObject(pass, current.X, object, state, seen) {
			return true
		}
		for _, bound := range []ast.Expr{current.Low, current.High, current.Max} {
			if bound != nil && ps4001PlaneDependsOnObject(pass, bound, object, state, seen) {
				return true
			}
		}
	case *ast.SelectorExpr:
		return ps4001PlaneDependsOnObject(pass, current.X, object, state, seen)
	case *ast.StarExpr:
		return ps4001PlaneDependsOnObject(pass, current.X, object, state, seen)
	}
	return false
}

func ps4001PlaneLoopObjects(pass *analysis.Pass, loop ast.Node, state ps4001PlaneState) map[types.Object]bool {
	result := make(map[types.Object]bool)
	add := func(expression ast.Expr) types.Object {
		if identifier, ok := ps2110Unparen(expression).(*ast.Ident); ok && identifier.Name != "_" {
			if object := identObject(pass, identifier); object != nil {
				return object
			}
		}
		return nil
	}
	switch current := loop.(type) {
	case *ast.ForStmt:
		if assignment, ok := current.Init.(*ast.AssignStmt); ok {
			for _, left := range assignment.Lhs {
				object := add(left)
				if object != nil && ps4001PlaneConditionDepends(pass, current.Cond, object, state) &&
					ps4001PlanePostAdvances(pass, current.Post, object) {
					result[object] = true
				}
			}
		}
	case *ast.RangeStmt:
		if object := add(current.Key); object != nil {
			result[object] = true
		}
		if object := add(current.Value); object != nil {
			result[object] = true
		}
	}
	return result
}

func ps4001PlaneConditionDepends(pass *analysis.Pass, expression ast.Expr, object types.Object, state ps4001PlaneState) bool {
	binary, ok := ps2110Unparen(expression).(*ast.BinaryExpr)
	if !ok {
		return false
	}
	if binary.Op == token.LAND || binary.Op == token.LOR {
		return ps4001PlaneConditionDepends(pass, binary.X, object, state) ||
			ps4001PlaneConditionDepends(pass, binary.Y, object, state)
	}
	switch binary.Op {
	case token.EQL, token.NEQ, token.LSS, token.LEQ, token.GTR, token.GEQ:
		return ps4001PlaneDependsOnObject(pass, binary.X, object, state, make(map[types.Object]bool)) ||
			ps4001PlaneDependsOnObject(pass, binary.Y, object, state, make(map[types.Object]bool))
	default:
		return false
	}
}

func ps4001PlanePostAdvances(pass *analysis.Pass, statement ast.Stmt, object types.Object) bool {
	switch post := statement.(type) {
	case *ast.IncDecStmt:
		return ps4001PlaneObject(pass, post.X, object)
	case *ast.AssignStmt:
		for index, left := range post.Lhs {
			if !ps4001PlaneObject(pass, left, object) {
				continue
			}
			if post.Tok != token.ASSIGN {
				return (post.Tok == token.ADD_ASSIGN || post.Tok == token.SUB_ASSIGN) &&
					index < len(post.Rhs) && ps4001PlaneNonzeroInteger(pass, post.Rhs[index])
			}
			return index < len(post.Rhs) && ps4001PlaneInductionStep(pass, post.Rhs[index], object)
		}
	}
	return false
}

func ps4001PlaneInductionStep(pass *analysis.Pass, expression ast.Expr, object types.Object) bool {
	binary, ok := ps2110Unparen(expression).(*ast.BinaryExpr)
	if !ok {
		return false
	}
	switch binary.Op {
	case token.ADD:
		return ps4001PlaneObject(pass, binary.X, object) && ps4001PlaneNonzeroInteger(pass, binary.Y) ||
			ps4001PlaneObject(pass, binary.Y, object) && ps4001PlaneNonzeroInteger(pass, binary.X)
	case token.SUB:
		return ps4001PlaneObject(pass, binary.X, object) && ps4001PlaneNonzeroInteger(pass, binary.Y)
	default:
		return false
	}
}

func ps4001PlaneNonzeroInteger(pass *analysis.Pass, expression ast.Expr) bool {
	value, ok := ps4001PlaneConstantInt(pass, expression)
	return ok && value != 0
}

func ps4001PlaneLoopObjectsStable(pass *analysis.Pass, loop ast.Node, state ps4001PlaneState) bool {
	objects := ps4001PlaneLoopObjects(pass, loop, state)
	if len(objects) == 0 {
		return false
	}
	switch current := loop.(type) {
	case *ast.ForStmt:
		for object := range objects {
			if state.writes[object] != 2 || state.escaped[object] ||
				!ps4001PlaneConditionDepends(pass, current.Cond, object, state) ||
				!ps4001PlanePostAdvances(pass, current.Post, object) {
				return false
			}
		}
	case *ast.RangeStmt:
		for object := range objects {
			if state.writes[object] != 1 || state.escaped[object] {
				return false
			}
		}
	}
	return true
}

func ps4001PlaneMentions(pass *analysis.Pass, expression ast.Expr, object types.Object) bool {
	found := false
	ast.Inspect(expression, func(node ast.Node) bool {
		if found {
			return false
		}
		identifier, ok := node.(*ast.Ident)
		if ok && identObject(pass, identifier) == object {
			found = true
		}
		return !found
	})
	return found
}

func ps4001PlaneObject(pass *analysis.Pass, expression ast.Expr, object types.Object) bool {
	identifier, ok := ps2110Unparen(expression).(*ast.Ident)
	return ok && identObject(pass, identifier) == object
}

func ps4001PlaneConstant(pass *analysis.Pass, expression ast.Expr, expected int64) bool {
	value, ok := ps4001PlaneConstantInt(pass, expression)
	return ok && value == expected
}

func ps4001PlaneConstantInt(pass *analysis.Pass, expression ast.Expr) (int64, bool) {
	typed, ok := pass.TypesInfo.Types[ps2110Unparen(expression)]
	if !ok || typed.Value == nil || typed.Value.Kind() != constant.Int {
		return 0, false
	}
	return constant.Int64Val(typed.Value)
}
