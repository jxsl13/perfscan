package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/lint"
)

// PS6114 implements owner issue #874 as a configured, source-proved advisory.
var PS6114 = register(&lint.Check{
	ID:          "PS6114",
	Category:    "verify",
	Slug:        "row-local-sparse-gather",
	Level:       lint.LevelAggressive,
	AutoFix:     false,
	NeedsConfig: true,
	Vocab:       []string{"rowLocalSparseGatherContracts"},
	Doc: lint.Documentation{
		Title: "a row-local transform computes packed rows discarded by a regular gather",
		Text: `A transform over packed batch*stride rows can do dead work when its
only successful-path consumers select row i*stride for each batch item and
concatenate those width-one rows. Moving the deterministic gather before a
truly row-local transform can reduce transformed rows from batch*stride to
batch, eliminating batch*(stride-1) row transforms.

PS6114 does not infer row locality, ownership, differentiation, backend
coverage, or fallback parity from LayerNorm, Slice, or Concat spelling. A
rowLocalSparseGatherContracts entry binds one exact concrete transform method,
gather and concat functions, typed operation constants and attribute fields,
argument roles, and every semantic promise. Interfaces, promoted methods,
method expressions, generics, wrappers, closures, and cross-function flows are
outside V1.

The source grammar is deliberately narrow: in one configured non-generic
function body, an exact transform assignment is immediately checked by a
terminal error guard; an exact make([]T, batch) is followed by a canonical full
range or zero/unit induction loop containing only gather, terminal guard, and
collection[i]=row; an exact concat and terminal guard immediately follow.
Slice attributes must be Axis:0, Start:i*stride, End:i*stride+1, and concat
must use Axis:0. Batch and stride may be integer parameters or dominating
single-definition locals, including shape-accessor snapshots, but cannot be
rebound, addressed, or captured. Constant batch must be positive and constant
stride greater than one; runtime geometry relies on the explicit checked
contract.

The transformed value has versioned closure: pre-transform uses of an in-place
assignment are distinct, while the post-transform value may occur only at the
matched gather input. Collection and gathered-row values likewise have only
their make/store/concat roles. Extra uses, aliases, stores, append, mutation,
retention, partial or conditional loops, break/continue/goto, async/deferred
calls, swallowed/nonterminal errors, irregular or value-dependent indices,
and source-proven unreachable shapes stay silent. A contract marked as already
containing the reviewed fused-capability fallback is intentionally silent.

There is NO automatic fix. The selected-first implementation must preserve
forward arithmetic, error/panic and partial-output behavior, recorder order,
all five input gradients of the owner VJP, dtype/layout/backend coverage, and
the exact fallback unless both forward and backward kernels exist. PS6012
recognizes a batch-indexed Slice dispatch loop that concatenates its results;
it does not prove that a preceding full-result transform is row-local or dead
outside the selected rows. Host/UMA evidence is configured evidence, never a
placement or speed guarantee.`,
		Before: `h, err = transform(ctx, h)
if err != nil { return nil, err }
rows := make([]*Tensor, batch)
for i := range batch {
    row, err := gather(ctx, sliceOp, SliceAttrs{Axis: 0, Start: i*stride, End: i*stride + 1}, h)
    if err != nil { return nil, err }
    rows[i] = row
}
selected, err := concat(ctx, concatOp, rows, ConcatAttrs{Axis: 0})`,
		After: `// Candidate only: select the batch rows first, then use the exact
// configured differentiable selected-first transform/classifier capability.`,
		MeasuredWin: `Owner issue #874 reports Apple M2 Pro evidence at B=8,
S=65, D=128, C=10: a fused host route measured roughly 0.08-0.15 ms versus
1.5-2.4 ms at the composite boundary, with isolated full-step campaigns around
1.20-1.28x and fewer boundary allocations. Those are attributed GoAI results,
not a generic PS6114 or host-placement speed claim. The retained implementation
uses an exact five-input VJP and falls back unless both forward and backward
kernels are available. Revalidate the configured backend and current parent.`,
	},
	Analyzer: &analysis.Analyzer{Name: "PS6114", Doc: "configured row-local transform followed only by a regular sparse row gather", Run: runPS6114},
})

type ps6114Transform struct {
	assignment *ast.AssignStmt
	call       *ast.CallExpr
	receiver   ps6106Storage
	output     *ast.Ident
	errorValue *ast.Ident
	input      ast.Expr
}

type ps6114Loop struct {
	statement  ast.Stmt
	index      *ast.Ident
	batch      ast.Expr
	stride     ast.Expr
	gather     *ast.CallExpr
	row        *ast.Ident
	rowStore   *ast.AssignStmt
	collection *ast.Ident
}

func runPS6114(pass *analysis.Pass) (any, error) {
	return runPS6114WithContracts(pass, config.Current().RowLocalSparseGatherContracts)
}

func runPS6114WithContracts(pass *analysis.Pass, configured []config.RowLocalSparseGatherContract) (any, error) {
	contracts := ps6114Contracts(configured)
	for _, file := range pass.Files {
		for _, declarationNode := range file.Decls {
			declaration, ok := declarationNode.(*ast.FuncDecl)
			if !ok || declaration.Body == nil || ps6113GenericFunction(pass, declaration) {
				continue
			}
			functionID := ps6113DeclarationID(pass, declaration)
			parents := ps6087Parents(declaration.Body)
			unreachable := ps2144Unreachable(pass, declaration.Body)
			for contractIndex := range contracts {
				contract := contracts[contractIndex]
				if contract.ConfiguredSite != functionID || contract.ExistingFusedCapabilityFallback {
					continue
				}
				ps6114ScanBody(pass, declaration, contract, parents, unreachable)
			}
		}
	}
	return nil, nil
}

func ps6114Contracts(configured []config.RowLocalSparseGatherContract) []*config.RowLocalSparseGatherContract {
	nameCount := make(map[string]int)
	siteCount := make(map[string]int)
	shapeCount := make(map[string]int)
	for index := range configured {
		contract := &configured[index]
		if contract.Valid() {
			nameCount[contract.Name]++
			siteCount[contract.ConfiguredSite]++
			shapeCount[contract.ConfiguredSite+"\x00"+contract.Transform+"\x00"+contract.Gather+"\x00"+contract.Concat]++
		}
	}
	var result []*config.RowLocalSparseGatherContract
	for index := range configured {
		contract := &configured[index]
		key := contract.ConfiguredSite + "\x00" + contract.Transform + "\x00" + contract.Gather + "\x00" + contract.Concat
		if contract.Valid() && nameCount[contract.Name] == 1 && siteCount[contract.ConfiguredSite] == 1 && shapeCount[key] == 1 {
			result = append(result, contract)
		}
	}
	slices.SortFunc(result, func(left, right *config.RowLocalSparseGatherContract) int {
		return strings.Compare(left.Name, right.Name)
	})
	return result
}

func ps6114ScanBody(pass *analysis.Pass, declaration *ast.FuncDecl, contract *config.RowLocalSparseGatherContract, parents map[ast.Node]ast.Node, unreachable []tokenSpan) {
	statements := declaration.Body.List
	for index := range statements {
		transform, next, ok := ps6114TransformAt(pass, statements, index, contract)
		if !ok || next+3 >= len(statements) {
			continue
		}
		collection, batch, ok := ps6114MakeCollection(pass, statements[next])
		if !ok {
			continue
		}
		loop, ok := ps6114GatherLoop(pass, statements[next+1], collection, batch, transform, contract)
		if !ok {
			continue
		}
		concat, concatOutput, concatError, ok := ps6114Concat(pass, statements[next+2], collection, contract)
		if !ok || !ps6114TerminalGuard(pass, statements[next+3], pass.TypesInfo.ObjectOf(concatError)) ||
			!ps6114ProvenClosure(pass, declaration, transform, &loop, concat, concatOutput, concatError, contract, parents) ||
			ps2144PositionIn(transform.call.Pos(), unreachable) || ps2144PositionIn(loop.gather.Pos(), unreachable) || ps2144PositionIn(concat.Pos(), unreachable) {
			continue
		}
		work := ps6114EliminatedRows(pass, batch, loop.stride)
		pass.Report(analysis.Diagnostic{
			Pos: transform.call.Pos(), End: transform.call.End(),
			Message: contract.Name + ": configured row-local " + contract.Transform + " transforms packed rows whose only successful-path consumers gather row i*stride; evaluate the selected-first differentiable capability to eliminate " + work + " row transforms, retaining only after forward, five-input VJP, float, error/panic, partial-output, recorder, backend, fallback, and current-parent gates (PS6114 advisory, no automatic fix)",
			Related: []analysis.RelatedInformation{{Pos: loop.gather.Pos(), End: loop.gather.End(), Message: "the canonical width-one sparse gather starts here"}, {Pos: concat.Pos(), End: concat.End(), Message: "the gathered rows are concatenated here"}},
		})
	}
}

func ps6114TransformAt(pass *analysis.Pass, statements []ast.Stmt, index int, contract *config.RowLocalSparseGatherContract) (ps6114Transform, int, bool) {
	if guard, ok := statements[index].(*ast.IfStmt); ok && guard.Init != nil {
		assignment, assignmentOK := guard.Init.(*ast.AssignStmt)
		transform, transformOK := ps6114TransformAssignment(pass, assignment, contract)
		return transform, index + 1, assignmentOK && transformOK && ps6114TerminalGuardNode(pass, guard, pass.TypesInfo.ObjectOf(transform.errorValue))
	}
	assignment, ok := statements[index].(*ast.AssignStmt)
	if !ok || index+1 >= len(statements) {
		return ps6114Transform{}, 0, false
	}
	transform, ok := ps6114TransformAssignment(pass, assignment, contract)
	return transform, index + 2, ok && ps6114TerminalGuard(pass, statements[index+1], pass.TypesInfo.ObjectOf(transform.errorValue))
}

func ps6114TransformAssignment(pass *analysis.Pass, assignment *ast.AssignStmt, contract *config.RowLocalSparseGatherContract) (ps6114Transform, bool) {
	if assignment == nil || len(assignment.Lhs) != 2 || len(assignment.Rhs) != 1 {
		return ps6114Transform{}, false
	}
	output, outputOK := ps2110Unparen(assignment.Lhs[0]).(*ast.Ident)
	errorValue, errorOK := ps2110Unparen(assignment.Lhs[1]).(*ast.Ident)
	call, callOK := ps2110Unparen(assignment.Rhs[0]).(*ast.CallExpr)
	method, methodOK := ps6113DirectMethod(pass, call, contract.Transform)
	if !outputOK || !errorOK || !callOK || !methodOK || ps6106Interface(method.signature.Recv().Type()) ||
		call.Ellipsis.IsValid() || len(call.Args) != method.signature.Params().Len() ||
		contract.TransformInputArgument > len(call.Args) || method.signature.Results().Len() != 2 ||
		!ps6114ErrorType(method.signature.Results().At(1).Type()) {
		return ps6114Transform{}, false
	}
	input := call.Args[contract.TransformInputArgument-1]
	if !types.AssignableTo(pass.TypesInfo.TypeOf(input), method.signature.Params().At(contract.TransformInputArgument-1).Type()) ||
		!types.AssignableTo(method.signature.Results().At(0).Type(), pass.TypesInfo.TypeOf(output)) ||
		!ps6114ArgumentsHaveNoCalls(call) {
		return ps6114Transform{}, false
	}
	return ps6114Transform{assignment: assignment, call: call, receiver: method.receiver, output: output, errorValue: errorValue, input: input}, true
}

func ps6114ErrorType(value types.Type) bool {
	return value != nil && types.AssignableTo(value, types.Universe.Lookup("error").Type())
}

func ps6114TerminalGuard(pass *analysis.Pass, statement ast.Stmt, object types.Object) bool {
	guard, ok := statement.(*ast.IfStmt)
	return ok && guard.Init == nil && ps6114TerminalGuardNode(pass, guard, object)
}

func ps6114TerminalGuardNode(pass *analysis.Pass, guard *ast.IfStmt, object types.Object) bool {
	if guard == nil || guard.Else != nil || len(guard.Body.List) != 1 || object == nil {
		return false
	}
	condition, ok := ps2110Unparen(guard.Cond).(*ast.BinaryExpr)
	if !ok || condition.Op != token.NEQ || !ps6114ObjectExpr(pass, condition.X, object) || !ps6114Nil(pass, condition.Y) {
		return false
	}
	result, ok := guard.Body.List[0].(*ast.ReturnStmt)
	if !ok {
		return false
	}
	foundError := false
	for _, expression := range result.Results {
		if !ps6114SimpleReturnExpression(pass, expression) {
			return false
		}
		if ps6114ObjectExpr(pass, expression, object) {
			foundError = true
		}
	}
	return foundError
}

func ps6114SimpleReturnExpression(pass *analysis.Pass, expression ast.Expr) bool {
	switch value := ps2110Unparen(expression).(type) {
	case *ast.Ident:
		return pass.TypesInfo.ObjectOf(value) != nil
	case *ast.BasicLit:
		return true
	default:
		return false
	}
}

func ps6114Nil(pass *analysis.Pass, expression ast.Expr) bool {
	identifier, ok := ps2110Unparen(expression).(*ast.Ident)
	return ok && identifier.Name == "nil" && pass.TypesInfo.Uses[identifier] == types.Universe.Lookup("nil")
}

func ps6114ObjectExpr(pass *analysis.Pass, expression ast.Expr, object types.Object) bool {
	identifier, ok := ps2110Unparen(expression).(*ast.Ident)
	return ok && pass.TypesInfo.ObjectOf(identifier) == object
}

func ps6114MakeCollection(pass *analysis.Pass, statement ast.Stmt) (*ast.Ident, ast.Expr, bool) {
	assignment, ok := statement.(*ast.AssignStmt)
	if !ok || assignment.Tok != token.DEFINE || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
		return nil, nil, false
	}
	collection, collectionOK := ps2110Unparen(assignment.Lhs[0]).(*ast.Ident)
	call, callOK := ps2110Unparen(assignment.Rhs[0]).(*ast.CallExpr)
	if !collectionOK || !callOK || !typedBuiltinName(pass, call.Fun, "make") || len(call.Args) != 2 {
		return nil, nil, false
	}
	if _, ok := types.Unalias(pass.TypesInfo.TypeOf(call.Args[0])).Underlying().(*types.Slice); !ok {
		return nil, nil, false
	}
	if !ps6106Integer(pass.TypesInfo.TypeOf(call.Args[1])) {
		return nil, nil, false
	}
	return collection, call.Args[1], true
}

func ps6114GatherLoop(pass *analysis.Pass, statement ast.Stmt, collection *ast.Ident, batch ast.Expr, transform ps6114Transform, contract *config.RowLocalSparseGatherContract) (ps6114Loop, bool) {
	index, bound, body, ok := ps6114CanonicalLoop(pass, statement)
	batchObject := ps6114ExprObject(pass, batch)
	boundObject := ps6114ExprObject(pass, bound)
	if !ok || batchObject == nil || batchObject != boundObject || !ps6106Integer(batchObject.Type()) || len(body.List) != 3 {
		return ps6114Loop{}, false
	}
	assignment, ok := body.List[0].(*ast.AssignStmt)
	if !ok || len(assignment.Lhs) != 2 || len(assignment.Rhs) != 1 {
		return ps6114Loop{}, false
	}
	row, rowOK := ps2110Unparen(assignment.Lhs[0]).(*ast.Ident)
	errorValue, errorOK := ps2110Unparen(assignment.Lhs[1]).(*ast.Ident)
	gather, callOK := ps2110Unparen(assignment.Rhs[0]).(*ast.CallExpr)
	function, functionOK := ps6114DirectFunction(pass, gather, contract.Gather)
	if !rowOK || !errorOK || !callOK || !functionOK || function.Results().Len() != 2 ||
		!ps6114ErrorType(function.Results().At(1).Type()) || !ps6114TerminalGuard(pass, body.List[1], pass.TypesInfo.ObjectOf(errorValue)) ||
		!ps6114ArgumentsHaveNoCalls(gather) {
		return ps6114Loop{}, false
	}
	collectionSlice, _ := types.Unalias(pass.TypesInfo.TypeOf(collection)).Underlying().(*types.Slice)
	if collectionSlice == nil || !types.AssignableTo(function.Results().At(0).Type(), collectionSlice.Elem()) {
		return ps6114Loop{}, false
	}
	operation, attrs, input, ok := ps6114CallRoles(gather, contract.GatherOperationArgument, contract.GatherAttrsArgument, contract.GatherInputArgument)
	if !ok || !ps6113AddConstant(pass, operation, contract.SliceOperation, contract.SliceOperationValue, function.Params().At(contract.GatherOperationArgument-1).Type()) ||
		!ps6114SameObject(pass, input, transform.output) {
		return ps6114Loop{}, false
	}
	stride, ok := ps6114SliceAttrs(pass, attrs, index, contract)
	if !ok {
		return ps6114Loop{}, false
	}
	store, ok := body.List[2].(*ast.AssignStmt)
	if !ok || store.Tok != token.ASSIGN || len(store.Lhs) != 1 || len(store.Rhs) != 1 || !ps6114SameObject(pass, store.Rhs[0], row) {
		return ps6114Loop{}, false
	}
	indexed, ok := ps2110Unparen(store.Lhs[0]).(*ast.IndexExpr)
	if !ok || !ps6114SameObject(pass, indexed.X, collection) || !ps6114SameObject(pass, indexed.Index, index) {
		return ps6114Loop{}, false
	}
	return ps6114Loop{statement: statement, index: index, batch: batch, stride: stride, gather: gather, row: row, rowStore: store, collection: collection}, true
}

func ps6114CanonicalLoop(pass *analysis.Pass, statement ast.Stmt) (*ast.Ident, ast.Expr, *ast.BlockStmt, bool) {
	if loop, ok := statement.(*ast.RangeStmt); ok {
		index, indexOK := loop.Key.(*ast.Ident)
		return index, loop.X, loop.Body, indexOK && loop.Value == nil && loop.Tok == token.DEFINE && ps6106Integer(pass.TypesInfo.TypeOf(loop.X))
	}
	loop, ok := statement.(*ast.ForStmt)
	if !ok || loop.Init == nil || loop.Cond == nil || loop.Post == nil {
		return nil, nil, nil, false
	}
	initial, initialOK := loop.Init.(*ast.AssignStmt)
	condition, conditionOK := ps2110Unparen(loop.Cond).(*ast.BinaryExpr)
	post, postOK := loop.Post.(*ast.IncDecStmt)
	if !initialOK || initial.Tok != token.DEFINE || len(initial.Lhs) != 1 || len(initial.Rhs) != 1 ||
		!conditionOK || condition.Op != token.LSS || !postOK || post.Tok != token.INC ||
		!ps6114IntConstant(pass, initial.Rhs[0], 0) {
		return nil, nil, nil, false
	}
	index, indexOK := initial.Lhs[0].(*ast.Ident)
	if !indexOK || !ps6114SameObject(pass, condition.X, index) || !ps6114SameObject(pass, post.X, index) {
		return nil, nil, nil, false
	}
	return index, condition.Y, loop.Body, true
}

func ps6114DirectFunction(pass *analysis.Pass, call *ast.CallExpr, id string) (*types.Signature, bool) {
	if call == nil || call.Ellipsis.IsValid() || ps6087FunctionID(pass, call) != id {
		return nil, false
	}
	function, signature, ok := typedCallee(pass, call.Fun)
	if !ok || function == nil || signature == nil || signature.Recv() != nil || signature.Variadic() || signature.TypeParams().Len() != 0 || len(call.Args) != signature.Params().Len() {
		return nil, false
	}
	return signature, true
}

func ps6114CallRoles(call *ast.CallExpr, positions ...int) (ast.Expr, ast.Expr, ast.Expr, bool) {
	if len(positions) != 3 {
		return nil, nil, nil, false
	}
	for _, position := range positions {
		if position <= 0 || position > len(call.Args) {
			return nil, nil, nil, false
		}
	}
	return call.Args[positions[0]-1], call.Args[positions[1]-1], call.Args[positions[2]-1], true
}

func ps6114SliceAttrs(pass *analysis.Pass, expression ast.Expr, index *ast.Ident, contract *config.RowLocalSparseGatherContract) (ast.Expr, bool) {
	literal, ok := ps2110Unparen(expression).(*ast.CompositeLit)
	if !ok || ps6114TypeID(pass, literal.Type) != contract.SliceAttrsType || len(literal.Elts) != 3 {
		return nil, false
	}
	fields, ok := ps6114KeyedFields(pass, literal)
	if !ok {
		return nil, false
	}
	axis := fields[contract.SliceAxisField]
	start := fields[contract.SliceStartField]
	end := fields[contract.SliceEndField]
	if axis == nil || start == nil || end == nil || !ps6114IntConstant(pass, axis, 0) {
		return nil, false
	}
	stride, ok := ps6114IndexProduct(pass, start, index)
	return stride, ok && ps6114End(pass, end, index, stride)
}

func ps6114KeyedFields(pass *analysis.Pass, literal *ast.CompositeLit) (map[string]ast.Expr, bool) {
	fields := make(map[string]ast.Expr, len(literal.Elts))
	for _, element := range literal.Elts {
		keyValue, ok := element.(*ast.KeyValueExpr)
		if !ok {
			return nil, false
		}
		key, keyOK := keyValue.Key.(*ast.Ident)
		field, fieldOK := pass.TypesInfo.Uses[key].(*types.Var)
		if !keyOK || !fieldOK || field.Pkg() == nil {
			return nil, false
		}
		named := ps6087Named(pass.TypesInfo.TypeOf(literal.Type))
		if named == nil || named.Obj().Pkg() == nil {
			return nil, false
		}
		id := named.Obj().Pkg().Path() + "." + named.Obj().Name() + "." + field.Name()
		if fields[id] != nil {
			return nil, false
		}
		fields[id] = keyValue.Value
	}
	return fields, true
}

func ps6114TypeID(pass *analysis.Pass, expression ast.Expr) string {
	named := ps6087Named(pass.TypesInfo.TypeOf(expression))
	if named == nil || named.Obj().Pkg() == nil {
		return ""
	}
	return named.Obj().Pkg().Path() + "." + named.Obj().Name()
}

func ps6114IndexProduct(pass *analysis.Pass, expression ast.Expr, index *ast.Ident) (ast.Expr, bool) {
	product, ok := ps2110Unparen(expression).(*ast.BinaryExpr)
	if !ok || product.Op != token.MUL {
		return nil, false
	}
	if ps6114SameObject(pass, product.X, index) && ps6106Integer(pass.TypesInfo.TypeOf(product.Y)) {
		return product.Y, true
	}
	if ps6114SameObject(pass, product.Y, index) && ps6106Integer(pass.TypesInfo.TypeOf(product.X)) {
		return product.X, true
	}
	return nil, false
}

func ps6114End(pass *analysis.Pass, expression ast.Expr, index *ast.Ident, stride ast.Expr) bool {
	addition, ok := ps2110Unparen(expression).(*ast.BinaryExpr)
	if !ok || addition.Op != token.ADD {
		return false
	}
	var product ast.Expr
	if ps6114IntConstant(pass, addition.X, 1) {
		product = addition.Y
	} else if ps6114IntConstant(pass, addition.Y, 1) {
		product = addition.X
	} else {
		return false
	}
	endStride, ok := ps6114IndexProduct(pass, product, index)
	return ok && ps6114SameValueExpr(pass, endStride, stride)
}

func ps6114IntConstant(pass *analysis.Pass, expression ast.Expr, wanted int64) bool {
	value := pass.TypesInfo.Types[ps2110Unparen(expression)].Value
	if value == nil || value.Kind() != constant.Int {
		return false
	}
	actual, ok := constant.Int64Val(value)
	return ok && actual == wanted
}

func ps6114SameObject(pass *analysis.Pass, expression ast.Expr, identifier *ast.Ident) bool {
	other, ok := ps2110Unparen(expression).(*ast.Ident)
	return ok && pass.TypesInfo.ObjectOf(other) != nil && pass.TypesInfo.ObjectOf(other) == pass.TypesInfo.ObjectOf(identifier)
}

func ps6114SameValueExpr(pass *analysis.Pass, left, right ast.Expr) bool {
	leftObject := ps6114ExprObject(pass, left)
	rightObject := ps6114ExprObject(pass, right)
	if leftObject != nil || rightObject != nil {
		return leftObject != nil && leftObject == rightObject
	}
	leftStorage, leftOK := ps6106StorageExpression(pass, left)
	rightStorage, rightOK := ps6106StorageExpression(pass, right)
	return leftOK && rightOK && ps6106SameStorage(leftStorage, rightStorage)
}

func ps6114Concat(pass *analysis.Pass, statement ast.Stmt, collection *ast.Ident, contract *config.RowLocalSparseGatherContract) (*ast.CallExpr, *ast.Ident, *ast.Ident, bool) {
	assignment, ok := statement.(*ast.AssignStmt)
	if !ok || len(assignment.Lhs) != 2 || len(assignment.Rhs) != 1 {
		return nil, nil, nil, false
	}
	output, outputOK := ps2110Unparen(assignment.Lhs[0]).(*ast.Ident)
	errorValue, errorOK := ps2110Unparen(assignment.Lhs[1]).(*ast.Ident)
	call, callOK := ps2110Unparen(assignment.Rhs[0]).(*ast.CallExpr)
	signature, functionOK := ps6114DirectFunction(pass, call, contract.Concat)
	if !outputOK || !errorOK || !callOK || !functionOK || signature.Results().Len() != 2 || !ps6114ErrorType(signature.Results().At(1).Type()) {
		return nil, nil, nil, false
	}
	operation, attrs, collectionArgument, ok := ps6114CallRoles(call, contract.ConcatOperationArgument, contract.ConcatAttrsArgument, contract.ConcatCollectionArgument)
	if !ok || !ps6113AddConstant(pass, operation, contract.ConcatOperation, contract.ConcatOperationValue, signature.Params().At(contract.ConcatOperationArgument-1).Type()) ||
		!ps6114SameObject(pass, collectionArgument, collection) || !ps6114ConcatAttrs(pass, attrs, contract) ||
		!ps6114ArgumentsHaveNoCalls(call) {
		return nil, nil, nil, false
	}
	return call, output, errorValue, true
}

func ps6114ConcatAttrs(pass *analysis.Pass, expression ast.Expr, contract *config.RowLocalSparseGatherContract) bool {
	literal, ok := ps2110Unparen(expression).(*ast.CompositeLit)
	if !ok || ps6114TypeID(pass, literal.Type) != contract.ConcatAttrsType || len(literal.Elts) != 1 {
		return false
	}
	fields, ok := ps6114KeyedFields(pass, literal)
	return ok && fields[contract.ConcatAxisField] != nil && ps6114IntConstant(pass, fields[contract.ConcatAxisField], 0)
}

func ps6114ProvenClosure(pass *analysis.Pass, declaration *ast.FuncDecl, transform ps6114Transform, loop *ps6114Loop, concat *ast.CallExpr, concatOutput, concatError *ast.Ident, contract *config.RowLocalSparseGatherContract, parents map[ast.Node]ast.Node) bool {
	outputObject := pass.TypesInfo.ObjectOf(transform.output)
	collectionObject := pass.TypesInfo.ObjectOf(loop.collection)
	rowObject := pass.TypesInfo.ObjectOf(loop.row)
	batchObject := ps6114ExprObject(pass, loop.batch)
	strideObject := ps6114ExprObject(pass, loop.stride)
	if outputObject == nil || collectionObject == nil || rowObject == nil || batchObject == nil || strideObject == nil ||
		!ps6114StableIntegerObject(pass, declaration, batchObject, parents) || !ps6114StableIntegerObject(pass, declaration, strideObject, parents) ||
		!ps6114ConstantGeometry(pass, loop.batch, loop.stride) {
		return false
	}
	receiverExpression := ps6099CallSelector(transform.call.Fun).X
	if !ps6113StableStorage(pass, declaration.Body, transform.receiver, transform.call, loop.gather, parents) ||
		!ps6114StorageClosed(pass, declaration.Body, transform.receiver, parents, receiverExpression) ||
		!ps6114StableCallStorages(pass, declaration.Body, transform.call, outputObject, parents) ||
		!ps6114StableCallStorages(pass, declaration.Body, loop.gather, outputObject, parents) ||
		!ps6114StableCallStorages(pass, declaration.Body, concat, collectionObject, parents) {
		return false
	}
	allowedOutput := []ast.Node{transform.output, transform.input, loop.gather.Args[contract.GatherInputArgument-1]}
	allowedCollection := []ast.Node{loop.collection, loop.rowStore.Lhs[0], concat.Args[contract.ConcatCollectionArgument-1]}
	allowedRow := []ast.Node{loop.row, loop.rowStore.Rhs[0]}
	if !ps6114VersionClosed(pass, declaration.Body, outputObject, transform.assignment.Pos(), parents, allowedOutput...) ||
		!ps6114ObjectClosed(pass, declaration.Body, collectionObject, allowedCollection...) ||
		!ps6114ObjectClosed(pass, declaration.Body, rowObject, allowedRow...) {
		return false
	}
	_ = concatOutput
	_ = concatError
	return true
}

func ps6114ArgumentsHaveNoCalls(call *ast.CallExpr) bool {
	for _, argument := range call.Args {
		hasCall := false
		ast.Inspect(argument, func(node ast.Node) bool {
			if _, ok := node.(*ast.CallExpr); ok {
				hasCall = true
				return false
			}
			return !hasCall
		})
		if hasCall {
			return false
		}
	}
	return true
}

func ps6114StorageClosed(pass *analysis.Pass, body *ast.BlockStmt, storage ps6106Storage, parents map[ast.Node]ast.Node, allowed ...ast.Node) bool {
	valid := true
	ast.Inspect(body, func(node ast.Node) bool {
		if !valid {
			return false
		}
		expression, ok := node.(ast.Expr)
		if !ok || ps6113OutermostFieldStorage(pass, expression, parents) != expression {
			return true
		}
		candidate, ok := ps6106StorageExpression(pass, expression)
		if !ok || !ps6113StoragesOverlap(candidate, storage) || ps6114WithinAny(expression, allowed) {
			return true
		}
		valid = false
		return false
	})
	return valid
}

func ps6114StableCallStorages(pass *analysis.Pass, body *ast.BlockStmt, call *ast.CallExpr, excluded types.Object, parents map[ast.Node]ast.Node) bool {
	for _, argument := range call.Args {
		storage, ok := ps6106StorageExpression(pass, argument)
		if ok && storage.root != excluded && !ps6113StableStorage(pass, body, storage, call, nil, parents) {
			return false
		}
	}
	return true
}

func ps6114ExprObject(pass *analysis.Pass, expression ast.Expr) types.Object {
	identifier, ok := ps2110Unparen(expression).(*ast.Ident)
	if !ok {
		return nil
	}
	return pass.TypesInfo.ObjectOf(identifier)
}

func ps6114StableIntegerObject(pass *analysis.Pass, declaration *ast.FuncDecl, object types.Object, parents map[ast.Node]ast.Node) bool {
	if constantObject, ok := object.(*types.Const); ok {
		return ps6106Integer(constantObject.Type())
	}
	variable, ok := object.(*types.Var)
	if !ok || !ps6106Integer(variable.Type()) ||
		variable.Pkg() != nil && variable.Parent() == variable.Pkg().Scope() {
		return false
	}
	valid := true
	ast.Inspect(declaration.Body, func(node ast.Node) bool {
		if !valid {
			return false
		}
		switch value := node.(type) {
		case *ast.AssignStmt:
			for _, lhs := range value.Lhs {
				identifier, identifierOK := ps2110Unparen(lhs).(*ast.Ident)
				if identifierOK && pass.TypesInfo.ObjectOf(identifier) == object && pass.TypesInfo.Defs[identifier] == nil {
					valid = false
					return false
				}
			}
		case *ast.IncDecStmt:
			if ps6114ExprObject(pass, value.X) == object {
				valid = false
				return false
			}
		case *ast.UnaryExpr:
			if value.Op == token.AND && ps6114ExprObject(pass, value.X) == object {
				valid = false
				return false
			}
		case *ast.Ident:
			if pass.TypesInfo.ObjectOf(value) == object && ps6113InsideClosure(value, parents) {
				valid = false
				return false
			}
		}
		return true
	})
	return valid
}

func ps6114ConstantGeometry(pass *analysis.Pass, batch, stride ast.Expr) bool {
	batchValue, batchKnown, batchOK := ps6114NativeIntConstant(pass, batch)
	strideValue, strideKnown, strideOK := ps6114NativeIntConstant(pass, stride)
	if !batchOK || !strideOK || batchKnown && batchValue <= 0 || strideKnown && strideValue <= 1 {
		return false
	}
	maxInt := ps6114MaxInt(pass)
	return !batchKnown || !strideKnown || batchValue <= maxInt/strideValue
}

func ps6114NativeIntConstant(pass *analysis.Pass, expression ast.Expr) (int64, bool, bool) {
	value := pass.TypesInfo.Types[ps2110Unparen(expression)].Value
	if value == nil {
		return 0, false, true
	}
	if value.Kind() != constant.Int {
		return 0, true, false
	}
	integer, ok := constant.Int64Val(value)
	return integer, true, ok && integer <= ps6114MaxInt(pass)
}

func ps6114MaxInt(pass *analysis.Pass) int64 {
	if pass.TypesSizes != nil && pass.TypesSizes.Sizeof(types.Typ[types.Int]) < 8 {
		return 1<<31 - 1
	}
	return 1<<63 - 1
}

func ps6114VersionClosed(pass *analysis.Pass, body *ast.BlockStmt, object types.Object, version token.Pos, parents map[ast.Node]ast.Node, allowed ...ast.Node) bool {
	variable, ok := object.(*types.Var)
	if !ok || variable.Pkg() != nil && variable.Parent() == variable.Pkg().Scope() {
		return false
	}
	return ps6114Closed(pass, body, object, func(identifier *ast.Ident) bool {
		if ps6113InsideClosure(identifier, parents) || ps6114Addressed(identifier, parents) {
			return false
		}
		return identifier.Pos() < version || ps6114WithinAny(identifier, allowed)
	})
}

func ps6114Addressed(node ast.Node, parents map[ast.Node]ast.Node) bool {
	current := node
	for {
		switch parent := parents[current].(type) {
		case *ast.ParenExpr:
			if parent.X != current {
				return false
			}
			current = parent
		case *ast.UnaryExpr:
			return parent.Op == token.AND && parent.X == current
		default:
			return false
		}
	}
}

func ps6114ObjectClosed(pass *analysis.Pass, body *ast.BlockStmt, object types.Object, allowed ...ast.Node) bool {
	return ps6114Closed(pass, body, object, func(identifier *ast.Ident) bool { return ps6114WithinAny(identifier, allowed) })
}

func ps6114Closed(pass *analysis.Pass, body *ast.BlockStmt, object types.Object, allowed func(*ast.Ident) bool) bool {
	valid := true
	ast.Inspect(body, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if ok && pass.TypesInfo.ObjectOf(identifier) == object && !allowed(identifier) {
			valid = false
			return false
		}
		return valid
	})
	return valid
}

func ps6114WithinAny(node ast.Node, allowed []ast.Node) bool {
	for _, container := range allowed {
		if container != nil && ps6099NodeWithin(node, container) {
			return true
		}
	}
	return false
}

func ps6114EliminatedRows(pass *analysis.Pass, batch, stride ast.Expr) string {
	batchValue := pass.TypesInfo.Types[ps2110Unparen(batch)].Value
	strideValue := pass.TypesInfo.Types[ps2110Unparen(stride)].Value
	batchInt, batchOK := int64(0), false
	strideInt, strideOK := int64(0), false
	if batchValue != nil && batchValue.Kind() == constant.Int {
		batchInt, batchOK = constant.Int64Val(batchValue)
	}
	if strideValue != nil && strideValue.Kind() == constant.Int {
		strideInt, strideOK = constant.Int64Val(strideValue)
	}
	if batchOK && strideOK && batchInt > 0 && strideInt > 1 && batchInt <= (1<<63-1)/(strideInt-1) {
		return strconv.FormatInt(batchInt*(strideInt-1), 10)
	}
	return "batch*(stride-1)"
}
