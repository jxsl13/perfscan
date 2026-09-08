package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"slices"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/lint"
)

// PS6113 implements owner issue #881 as a deliberately configured candidate
// generator. Recorder and projection method names do not imply fusion parity.
var PS6113 = register(&lint.Check{
	ID:          "PS6113",
	Category:    "verify",
	Slug:        "recorder-projection-residual-add",
	Level:       lint.LevelAggressive,
	AutoFix:     false,
	NeedsConfig: true,
	Vocab:       []string{"recorderResidualAddContracts"},
	Doc: lint.Documentation{
		Title: "a recorder projection writes scratch immediately consumed by an in-place residual add",
		Text: `A recorder-driven inference path may write a projection into a
temporary buffer and immediately launch a second recorder operation that adds
that buffer into a residual destination. A provider-specific accumulate
epilogue can sometimes remove one dispatch and the temporary write/read
boundary.

PS6113 never infers that relationship from record, Binary, recordAdd, MatMul,
or Acc spelling. A recorderResidualAddContracts entry names the exact typed
projection, accumulate sibling, recorder binary operation, add constant, and
optional eager first-error sequence. One-based argument roles describe the
recorder, source, temporary, destination, operation, projection extents, and an
optional len(destination) accumulate argument. The contract must explicitly
affirm complete temporary overwrite, nonaliasing, synchronous non-retention,
temporary scratch legality, composition/error/panic/partial-output parity,
floating-point policy, and dtype/layout/backend support.

The source recognizer accepts only two shapes: consecutive expression
statements in one lexical block, or consecutive call arguments of the exact
configured eager first-error helper. Both calls must be direct, unpromoted,
non-generic method values with exact signatures. The projection receiver,
recorder, source, temporary, destination, and extent expressions must be
stable typed identifiers or selector paths. The same recorder is both the
projection argument and binary receiver; the same destination is both binary
input and output; the exact configured add constant is used; every temporary
use in the function is one of the two matched arguments; its containing owner
is not passed, returned, copied, or captured; and source-proven unreachable,
deferred, or asynchronous paths stay silent. A projection receiver that
contains any recorder or buffer role is also rejected as definite overlap.

An interface-typed projection is accepted only at an exact configured caller
site. Its contract must enumerate concrete projection/accumulate method pairs,
affirm that every dynamic projection type at that site is covered, and each
pair must resolve in the analyzed package with signatures compatible with the
static interface methods. This is a project-owned closed-world assertion, not
runtime provider inference.

Version one intentionally excludes projection -> bias -> residual-add chains.
Combining those three operations can change association and rounding and needs
a separate three-operation parity contract. The commuted residual operand
order also stays silent without a separate explicit equivalence promise.
Aliased buffers, a temporary used again, rebinding or address exposure,
whole-owner aliases, intervening calls or control flow, method expressions,
promoted/interface implementations that are not completely configured,
variadic/generic APIs, and incomplete or ambiguous contracts stay silent.

There is NO automatic fix. A finding identifies a source-visible dispatch and
scratch boundary. Retain an accumulate path only after exact parity, fallback,
native event-count, temporary traffic, and current-parent end-to-end gates on
the configured dtype, layout, backend, and workload.`,
		Before: `projection.record(recorder, source, temporary, rows)
recorder.Binary(destination, temporary, destination, add)`,
		After: `// Candidate only; the exact signature comes from the contract.
projection.recordAdd(recorder, source, temporary, destination, rows, len(destination))`,
		MeasuredWin: `At GoAI commit 3c2ded37, the direct attention-output
projection -> residual-add shape occurred once per layer in both Step and
StepN, or 12 candidate boundaries per invocation for a 12-layer decoder. The
separate projection -> bias -> residual-add shape is intentionally excluded.
A later owner campaign retained in GoAI commit
1bf9229e7491834ef32c6ddb4457b8a74f217168 reported all 21 paired residual
measurements faster with a 1.076643x median. That is
project-specific evidence, not a generic PS6113 speed claim; remeasure the
exact configured backend and current parent.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6113",
		Doc:  "configured recorder projection immediately followed by an in-place residual add",
		Run:  runPS6113,
	},
})

type ps6113Pair struct {
	projection *ast.CallExpr
	binary     *ast.CallExpr
	contract   *config.RecorderResidualAddContract
	context    string
	eager      bool
}

func runPS6113(pass *analysis.Pass) (any, error) {
	return runPS6113WithContracts(pass, config.Current().RecorderResidualAddContracts)
}

func runPS6113WithContracts(pass *analysis.Pass, configured []config.RecorderResidualAddContract) (any, error) {
	contracts := ps6113Contracts(configured)
	if len(contracts) == 0 {
		return nil, nil
	}
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil || ps6113GenericFunction(pass, function) {
				continue
			}
			functionID := ps6113DeclarationID(pass, function)
			parents := ps6087Parents(function.Body)
			unreachable := ps2144Unreachable(pass, function.Body)
			reported := make(map[*ast.CallExpr]bool)
			for contractIndex := range contracts {
				contract := contracts[contractIndex]
				if contract.ConfiguredSite != "" && contract.ConfiguredSite != functionID {
					continue
				}
				pairs := ps6113StatementPairs(pass, function.Body, contract)
				pairs = append(pairs, ps6113EagerPairs(pass, function.Body, contract)...)
				for pairIndex := range pairs {
					pair := &pairs[pairIndex]
					if reported[pair.projection] || ps2144PositionIn(pair.projection.Pos(), unreachable) ||
						ps2144PositionIn(pair.binary.Pos(), unreachable) || ps6113AsyncContext(pair.projection, parents) ||
						!ps6113CallNesting(pair, parents) ||
						!ps6113ProvenPair(pass, function, pair, parents) {
						continue
					}
					reported[pair.projection] = true
					pass.Report(analysis.Diagnostic{
						Pos:     pair.projection.Pos(),
						End:     pair.projection.End(),
						Message: pair.contract.Name + ": configured " + pair.contract.Projection + " immediately feeds an in-place " + pair.contract.RecorderBinary + " residual add through one private temporary in " + pair.context + "; evaluate exact sibling " + pair.contract.Accumulate + " to remove one recorder dispatch and scratch write/read boundary, retaining only after floating-point, error/panic, partial-output, fallback, native-event, and current-parent gates (PS6113 advisory, no automatic fix)",
						Related: []analysis.RelatedInformation{{Pos: pair.binary.Pos(), End: pair.binary.End(), Message: "the configured in-place residual add consumes the projection temporary here"}},
					})
				}
			}
		}
	}
	return nil, nil
}

// ps6113Contracts rejects ambiguous entries independent of input order.
func ps6113Contracts(configured []config.RecorderResidualAddContract) []*config.RecorderResidualAddContract {
	nameCount := make(map[string]int)
	shapeCount := make(map[string]int)
	for index := range configured {
		contract := &configured[index]
		if contract.Valid() {
			nameCount[contract.Name]++
			shapeCount[ps6113ContractKey(contract)]++
		}
	}
	var result []*config.RecorderResidualAddContract
	for index := range configured {
		contract := &configured[index]
		if contract.Valid() && nameCount[contract.Name] == 1 && shapeCount[ps6113ContractKey(contract)] == 1 {
			result = append(result, contract)
		}
	}
	slices.SortFunc(result, func(left, right *config.RecorderResidualAddContract) int {
		return strings.Compare(left.Name, right.Name)
	})
	return result
}

func ps6113ContractKey(contract *config.RecorderResidualAddContract) string {
	return contract.ConfiguredSite + "\x00" + contract.Projection + "\x00" + contract.RecorderBinary + "\x00" + contract.EagerSequence
}

func ps6113GenericFunction(pass *analysis.Pass, declaration *ast.FuncDecl) bool {
	function, _ := pass.TypesInfo.Defs[declaration.Name].(*types.Func)
	if function == nil {
		return true
	}
	signature, _ := function.Type().(*types.Signature)
	return signature == nil || signature.TypeParams().Len() != 0 || signature.RecvTypeParams().Len() != 0
}

func ps6113DeclarationID(pass *analysis.Pass, declaration *ast.FuncDecl) string {
	function, _ := pass.TypesInfo.Defs[declaration.Name].(*types.Func)
	if function == nil || function.Pkg() == nil {
		return ""
	}
	signature, _ := function.Type().(*types.Signature)
	if signature == nil || signature.Recv() == nil {
		return function.Pkg().Path() + "." + function.Name()
	}
	receiver := ps6087Named(signature.Recv().Type())
	if receiver == nil {
		return ""
	}
	return function.Pkg().Path() + "." + receiver.Obj().Name() + "." + function.Name()
}

func ps6113StatementPairs(pass *analysis.Pass, body *ast.BlockStmt, contract *config.RecorderResidualAddContract) []ps6113Pair {
	var result []ps6113Pair
	ps6032Blocks(body, func(block *ast.BlockStmt) {
		for index := 0; index+1 < len(block.List); index++ {
			projection := ps6113ExpressionStatementCall(block.List[index])
			binary := ps6113ExpressionStatementCall(block.List[index+1])
			if projection != nil && binary != nil && ps6087FunctionID(pass, projection) == contract.Projection && ps6087FunctionID(pass, binary) == contract.RecorderBinary {
				result = append(result, ps6113Pair{projection: projection, binary: binary, contract: contract, context: "two consecutive statements"})
			}
		}
	})
	return result
}

func ps6113ExpressionStatementCall(statement ast.Stmt) *ast.CallExpr {
	expression, ok := statement.(*ast.ExprStmt)
	if !ok {
		return nil
	}
	call, _ := ps2110Unparen(expression.X).(*ast.CallExpr)
	return call
}

func ps6113EagerPairs(pass *analysis.Pass, body *ast.BlockStmt, contract *config.RecorderResidualAddContract) []ps6113Pair {
	if contract.EagerSequence == "" {
		return nil
	}
	var result []ps6113Pair
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		outer, ok := node.(*ast.CallExpr)
		if !ok || outer.Ellipsis.IsValid() || ps6087FunctionID(pass, outer) != contract.EagerSequence {
			return true
		}
		for index := 0; index+1 < len(outer.Args); index++ {
			projection, projectionOK := ps2110Unparen(outer.Args[index]).(*ast.CallExpr)
			binary, binaryOK := ps2110Unparen(outer.Args[index+1]).(*ast.CallExpr)
			if projectionOK && binaryOK && ps6087FunctionID(pass, projection) == contract.Projection && ps6087FunctionID(pass, binary) == contract.RecorderBinary {
				result = append(result, ps6113Pair{projection: projection, binary: binary, contract: contract, context: contract.EagerSequence + "'s consecutive eager arguments", eager: true})
			}
		}
		return true
	})
	return result
}

func ps6113AsyncContext(node ast.Node, parents map[ast.Node]ast.Node) bool {
	for parent := parents[node]; parent != nil; parent = parents[parent] {
		switch parent.(type) {
		case *ast.GoStmt, *ast.DeferStmt, *ast.FuncLit:
			return true
		}
	}
	return false
}

func ps6113CallNesting(pair *ps6113Pair, parents map[ast.Node]ast.Node) bool {
	want := 0
	if pair.eager {
		want = 1
	}
	for _, node := range []ast.Node{pair.projection, pair.binary} {
		calls := 0
		for parent := parents[node]; parent != nil; parent = parents[parent] {
			if _, ok := parent.(*ast.CallExpr); ok {
				calls++
			}
		}
		if calls != want {
			return false
		}
	}
	return true
}

type ps6113Call struct {
	call      *ast.CallExpr
	function  *types.Func
	signature *types.Signature
	receiver  ps6106Storage
}

func ps6113DirectMethod(pass *analysis.Pass, call *ast.CallExpr, id string) (ps6113Call, bool) {
	function, signature, ok := typedCallee(pass, call.Fun)
	selector := ps6099CallSelector(call.Fun)
	selection := pass.TypesInfo.Selections[selector]
	if !ok || function == nil || signature == nil || signature.Recv() == nil || signature.Variadic() ||
		signature.TypeParams().Len() != 0 || signature.RecvTypeParams().Len() != 0 ||
		selector == nil || selection == nil || selection.Kind() != types.MethodVal || len(selection.Index()) != 1 ||
		call.Ellipsis.IsValid() || len(call.Args) != signature.Params().Len() || ps6087FunctionID(pass, call) != id {
		return ps6113Call{}, false
	}
	receiver, ok := ps6106StorageExpression(pass, selector.X)
	if !ok || !types.AssignableTo(receiver.typeOf, signature.Recv().Type()) {
		return ps6113Call{}, false
	}
	return ps6113Call{call: call, function: function, signature: signature, receiver: receiver}, true
}

func ps6113ProvenPair(pass *analysis.Pass, declaration *ast.FuncDecl, pair *ps6113Pair, parents map[ast.Node]ast.Node) bool {
	contract := pair.contract
	projection, ok := ps6113DirectMethod(pass, pair.projection, contract.Projection)
	if !ok {
		return false
	}
	binary, ok := ps6113DirectMethod(pass, pair.binary, contract.RecorderBinary)
	if !ok || !ps6113StaticCapability(pass, declaration, &projection, contract) {
		return false
	}
	accumulate, ok := ps6113ResolveMethod(pass, contract.Accumulate)
	if !ok || !ps6113CompatibleRoles(pass, &projection, &binary, accumulate, contract) {
		return false
	}
	projectionReceiver := projection.receiver
	recorder, ok := ps6113ArgumentStorage(pass, pair.projection, contract.ProjectionRecorderArgument)
	if !ok || !ps6106SameStorage(recorder, binary.receiver) {
		return false
	}
	source, ok := ps6113ArgumentStorage(pass, pair.projection, contract.ProjectionSourceArgument)
	if !ok {
		return false
	}
	temporary, temporaryExpression, ok := ps6113ArgumentStorageExpr(pass, pair.projection, contract.ProjectionTemporaryArgument)
	if !ok {
		return false
	}
	binaryTemporary, binaryTemporaryExpression, ok := ps6113ArgumentStorageExpr(pass, pair.binary, contract.BinaryTemporaryArgument)
	if !ok || !ps6106SameStorage(temporary, binaryTemporary) {
		return false
	}
	destination, ok := ps6113ArgumentStorage(pass, pair.binary, contract.BinaryDestinationArgument)
	if !ok {
		return false
	}
	output, ok := ps6113ArgumentStorage(pass, pair.binary, contract.BinaryOutputArgument)
	if !ok || !ps6106SameStorage(destination, output) || ps6113StoragesOverlap(source, temporary) ||
		ps6113StoragesOverlap(source, destination) || ps6113StoragesOverlap(temporary, destination) ||
		ps6113StoragesOverlap(projectionReceiver, recorder) || ps6113StoragesOverlap(projectionReceiver, source) ||
		ps6113StoragesOverlap(projectionReceiver, temporary) || ps6113StoragesOverlap(projectionReceiver, destination) {
		return false
	}
	operationType := binary.signature.Params().At(contract.BinaryOperationArgument - 1).Type()
	if !ps6113AddConstant(pass, pair.binary.Args[contract.BinaryOperationArgument-1], contract.AddOperation, contract.AddOperationValue, operationType) ||
		!ps6113TemporaryClosed(pass, declaration.Body, temporary, temporaryExpression, binaryTemporaryExpression, parents) {
		return false
	}
	storages := []ps6106Storage{projectionReceiver, recorder, source, temporary, destination}
	for _, storage := range storages {
		if !ps6113StableStorage(pass, declaration.Body, storage, pair.projection, pair.binary, parents) {
			return false
		}
	}
	for _, position := range contract.ProjectionExtentArguments {
		if !ps6113StableExtent(pass, declaration.Body, pair.projection.Args[position-1], parents) {
			return false
		}
	}
	return true
}

func ps6113ArgumentStorage(pass *analysis.Pass, call *ast.CallExpr, position int) (ps6106Storage, bool) {
	storage, _, ok := ps6113ArgumentStorageExpr(pass, call, position)
	return storage, ok
}

func ps6113ArgumentStorageExpr(pass *analysis.Pass, call *ast.CallExpr, position int) (ps6106Storage, ast.Expr, bool) {
	index := position - 1
	if index < 0 || index >= len(call.Args) {
		return ps6106Storage{}, nil, false
	}
	storage, ok := ps6106StorageExpression(pass, call.Args[index])
	return storage, call.Args[index], ok
}

func ps6113CompatibleRoles(pass *analysis.Pass, projection, binary *ps6113Call, accumulate *types.Func, contract *config.RecorderResidualAddContract) bool {
	accumulateSignature, _ := accumulate.Type().(*types.Signature)
	if accumulateSignature == nil || accumulateSignature.Recv() == nil || accumulateSignature.Variadic() ||
		accumulateSignature.TypeParams().Len() != 0 || accumulateSignature.RecvTypeParams().Len() != 0 ||
		!types.Identical(projection.signature.Recv().Type(), accumulateSignature.Recv().Type()) ||
		!ps6113ExactPositions(projection.signature.Params().Len(), append([]int{contract.ProjectionRecorderArgument, contract.ProjectionSourceArgument, contract.ProjectionTemporaryArgument}, contract.ProjectionExtentArguments...)) ||
		!ps6113ExactPositions(binary.signature.Params().Len(), []int{contract.BinaryDestinationArgument, contract.BinaryTemporaryArgument, contract.BinaryOutputArgument, contract.BinaryOperationArgument}) {
		return false
	}
	accumulatePositions := []int{contract.AccumulateRecorderArgument, contract.AccumulateSourceArgument, contract.AccumulateTemporaryArgument, contract.AccumulateDestinationArgument}
	accumulatePositions = append(accumulatePositions, contract.AccumulateExtentArguments...)
	if contract.AccumulateDestinationLengthArgument > 0 {
		accumulatePositions = append(accumulatePositions, contract.AccumulateDestinationLengthArgument)
	}
	if !ps6113ExactPositions(accumulateSignature.Params().Len(), accumulatePositions) {
		return false
	}
	typeAt := func(signature *types.Signature, position int) types.Type {
		return signature.Params().At(position - 1).Type()
	}
	if !types.Identical(typeAt(projection.signature, contract.ProjectionRecorderArgument), binary.signature.Recv().Type()) ||
		!types.Identical(typeAt(projection.signature, contract.ProjectionRecorderArgument), typeAt(accumulateSignature, contract.AccumulateRecorderArgument)) ||
		!types.Identical(typeAt(projection.signature, contract.ProjectionSourceArgument), typeAt(accumulateSignature, contract.AccumulateSourceArgument)) ||
		!types.Identical(typeAt(projection.signature, contract.ProjectionTemporaryArgument), typeAt(binary.signature, contract.BinaryTemporaryArgument)) ||
		!types.Identical(typeAt(projection.signature, contract.ProjectionTemporaryArgument), typeAt(accumulateSignature, contract.AccumulateTemporaryArgument)) ||
		!types.Identical(typeAt(binary.signature, contract.BinaryDestinationArgument), typeAt(binary.signature, contract.BinaryOutputArgument)) ||
		!types.Identical(typeAt(binary.signature, contract.BinaryDestinationArgument), typeAt(accumulateSignature, contract.AccumulateDestinationArgument)) {
		return false
	}
	for index := range contract.ProjectionExtentArguments {
		if !types.Identical(typeAt(projection.signature, contract.ProjectionExtentArguments[index]), typeAt(accumulateSignature, contract.AccumulateExtentArguments[index])) {
			return false
		}
	}
	if contract.AccumulateDestinationLengthArgument > 0 && (!ps6106Integer(typeAt(accumulateSignature, contract.AccumulateDestinationLengthArgument)) ||
		!ps6113LenType(typeAt(binary.signature, contract.BinaryDestinationArgument))) {
		return false
	}
	return ps6113ResultParity(projection.signature, binary.signature, accumulateSignature)
}

func ps6113ExactPositions(total int, positions []int) bool {
	if len(positions) != total {
		return false
	}
	seen := make([]bool, total+1)
	for _, position := range positions {
		if position <= 0 || position > total || seen[position] {
			return false
		}
		seen[position] = true
	}
	return true
}

func ps6113ResultParity(projection, binary, accumulate *types.Signature) bool {
	// A single accumulate call must expose the same first-error surface as the
	// composed pair: either all three return nothing, or each returns one error.
	if projection.Results().Len() == 0 && binary.Results().Len() == 0 && accumulate.Results().Len() == 0 {
		return true
	}
	if projection.Results().Len() != 1 || binary.Results().Len() != 1 || accumulate.Results().Len() != 1 {
		return false
	}
	errorType := types.Universe.Lookup("error").Type()
	return types.AssignableTo(projection.Results().At(0).Type(), errorType) &&
		types.AssignableTo(binary.Results().At(0).Type(), errorType) &&
		types.AssignableTo(accumulate.Results().At(0).Type(), errorType)
}

func ps6113LenType(value types.Type) bool {
	if value == nil {
		return false
	}
	switch types.Unalias(value).Underlying().(type) {
	case *types.Slice, *types.Array:
		return true
	}
	return false
}

func ps6113AddConstant(pass *analysis.Pass, expression ast.Expr, id, exactValue string, expectedType types.Type) bool {
	var object types.Object
	switch value := ps2110Unparen(expression).(type) {
	case *ast.Ident:
		object = pass.TypesInfo.Uses[value]
	case *ast.SelectorExpr:
		object = pass.TypesInfo.Uses[value.Sel]
	default:
		return false
	}
	constantObject, ok := object.(*types.Const)
	return ok && constantObject.Pkg() != nil && constantObject.Parent() == constantObject.Pkg().Scope() &&
		constantObject.Pkg().Path()+"."+constantObject.Name() == id &&
		constantObject.Val().ExactString() == exactValue && types.Identical(constantObject.Type(), expectedType)
}

func ps6113TemporaryClosed(pass *analysis.Pass, body *ast.BlockStmt, temporary ps6106Storage, projectionArgument, binaryArgument ast.Expr, parents map[ast.Node]ast.Node) bool {
	valid := true
	ast.Inspect(body, func(node ast.Node) bool {
		if !valid {
			return false
		}
		expression, ok := node.(ast.Expr)
		if !ok {
			return true
		}
		// Inspect only the outermost transparent-parenthesis/field-selector
		// spine. The root in values.source is not by itself a whole-owner use,
		// while consume(values), return values, and aggregate/captured values
		// expose every field, including values.temporary.
		if ps6113OutermostFieldStorage(pass, expression, parents) != expression {
			return true
		}
		storage, ok := ps6106StorageExpression(pass, expression)
		if !ok || !ps6113StoragesOverlap(storage, temporary) {
			return true
		}
		if ps6106SameStorage(storage, temporary) &&
			(ps6099NodeWithin(expression, projectionArgument) || ps6099NodeWithin(expression, binaryArgument)) {
			return true
		}
		valid = false
		return false
	})
	return valid
}

func ps6113OutermostFieldStorage(pass *analysis.Pass, expression ast.Expr, parents map[ast.Node]ast.Node) ast.Expr {
	current := expression
	for {
		switch parent := parents[current].(type) {
		case *ast.ParenExpr:
			if parent.X != current {
				return current
			}
			current = parent
		case *ast.SelectorExpr:
			selection := pass.TypesInfo.Selections[parent]
			if parent.X != current || selection == nil || selection.Kind() != types.FieldVal {
				return current
			}
			current = parent
		default:
			return current
		}
	}
}

func ps6113StableStorage(pass *analysis.Pass, body *ast.BlockStmt, storage ps6106Storage, projection, binary *ast.CallExpr, parents map[ast.Node]ast.Node) bool {
	valid := true
	ast.Inspect(body, func(node ast.Node) bool {
		if !valid {
			return false
		}
		switch value := node.(type) {
		case *ast.AssignStmt:
			for _, lhs := range value.Lhs {
				if ps6113StorageOverlaps(pass, lhs, storage) && !ps6113DefinesStorage(pass, lhs, storage) {
					valid = false
					return false
				}
			}
			for _, rhs := range value.Rhs {
				if ps6113DirectStorageAlias(pass, rhs, storage) && !ps6099NodeWithin(rhs, projection) && !ps6099NodeWithin(rhs, binary) {
					valid = false
					return false
				}
			}
		case *ast.IncDecStmt:
			if ps6113StorageOverlaps(pass, value.X, storage) {
				valid = false
				return false
			}
		case *ast.UnaryExpr:
			if value.Op == token.AND && ps6113StorageOverlaps(pass, value.X, storage) {
				valid = false
				return false
			}
		case *ast.GoStmt, *ast.DeferStmt:
			if ps6113NodeUsesStorage(pass, node, storage) {
				valid = false
				return false
			}
		case ast.Expr:
			resolved, ok := ps6106StorageExpression(pass, value)
			if ok && ps6106SameStorage(resolved, storage) && ps6113InsideClosure(value, parents) {
				valid = false
				return false
			}
		}
		return true
	})
	return valid
}

func ps6113InsideClosure(node ast.Node, parents map[ast.Node]ast.Node) bool {
	for parent := parents[node]; parent != nil; parent = parents[parent] {
		if _, ok := parent.(*ast.FuncLit); ok {
			return true
		}
	}
	return false
}

func ps6113DefinesStorage(pass *analysis.Pass, expression ast.Expr, storage ps6106Storage) bool {
	identifier, ok := ps2110Unparen(expression).(*ast.Ident)
	return ok && len(storage.fields) == 0 && pass.TypesInfo.Defs[identifier] == storage.root
}

func ps6113DirectStorageAlias(pass *analysis.Pass, expression ast.Expr, storage ps6106Storage) bool {
	expression = ps2110Unparen(expression)
	if address, ok := expression.(*ast.UnaryExpr); ok && address.Op == token.AND {
		expression = ps2110Unparen(address.X)
	}
	return ps6113StorageOverlaps(pass, expression, storage)
}

func ps6113StorageOverlaps(pass *analysis.Pass, expression ast.Expr, storage ps6106Storage) bool {
	candidate, ok := ps6106StorageExpression(pass, expression)
	return ok && ps6113StoragesOverlap(candidate, storage)
}

func ps6113StoragesOverlap(left, right ps6106Storage) bool {
	if left.root == nil || left.root != right.root {
		return false
	}
	limit := min(len(left.fields), len(right.fields))
	for index := 0; index < limit; index++ {
		if left.fields[index] != right.fields[index] {
			return false
		}
	}
	return true
}

func ps6113NodeUsesStorage(pass *analysis.Pass, node ast.Node, storage ps6106Storage) bool {
	uses := false
	ast.Inspect(node, func(candidate ast.Node) bool {
		expression, ok := candidate.(ast.Expr)
		if ok {
			resolved, resolvedOK := ps6106StorageExpression(pass, expression)
			uses = uses || resolvedOK && ps6106SameStorage(resolved, storage)
		}
		return !uses
	})
	return uses
}

func ps6113StableExtent(pass *analysis.Pass, body *ast.BlockStmt, expression ast.Expr, parents map[ast.Node]ast.Node) bool {
	expression = ps2110Unparen(expression)
	if value := pass.TypesInfo.Types[expression].Value; value != nil && value.Kind() == constant.Int {
		return true
	}
	if storage, ok := ps6106StorageExpression(pass, expression); ok {
		return ps6113StableStorage(pass, body, storage, nil, nil, parents)
	}
	call, ok := expression.(*ast.CallExpr)
	if !ok || !typedBuiltinName(pass, call.Fun, "len") || len(call.Args) != 1 {
		return false
	}
	storage, ok := ps6106StorageExpression(pass, call.Args[0])
	return ok && ps6113LenType(storage.typeOf) && ps6113StableStorage(pass, body, storage, nil, nil, parents)
}

func ps6113StaticCapability(pass *analysis.Pass, declaration *ast.FuncDecl, projection *ps6113Call, contract *config.RecorderResidualAddContract) bool {
	accumulate, ok := ps6113ResolveMethod(pass, contract.Accumulate)
	if !ok || ps6113MethodObjectID(accumulate) != contract.Accumulate {
		return false
	}
	if !ps6106Interface(projection.signature.Recv().Type()) {
		return len(contract.Implementations) == 0 && ps6113MethodOwner(contract.Projection) == ps6113MethodOwner(contract.Accumulate)
	}
	if contract.ConfiguredSite == "" || contract.ConfiguredSite != ps6113DeclarationID(pass, declaration) ||
		len(contract.Implementations) == 0 || !contract.AllDynamicProjectionTypesCovered {
		return false
	}
	for _, implementation := range contract.Implementations {
		concreteProjection, projectionOK := ps6113ResolveMethod(pass, implementation.Projection)
		concreteAccumulate, accumulateOK := ps6113ResolveMethod(pass, implementation.Accumulate)
		if !projectionOK || !accumulateOK {
			return false
		}
		concreteSignature, _ := concreteProjection.Type().(*types.Signature)
		concreteReceiver := types.Type(nil)
		if concreteSignature != nil && concreteSignature.Recv() != nil {
			concreteReceiver = concreteSignature.Recv().Type()
		}
		staticInterface, _ := types.Unalias(projection.signature.Recv().Type()).Underlying().(*types.Interface)
		if ps6113MethodOwner(implementation.Projection) != ps6113MethodOwner(implementation.Accumulate) ||
			concreteReceiver == nil || ps6106Interface(concreteReceiver) || staticInterface == nil || !types.Implements(concreteReceiver, staticInterface) ||
			!ps6113SignatureCompatible(projection.function, concreteProjection) || !ps6113SignatureCompatible(accumulate, concreteAccumulate) {
			return false
		}
	}
	return true
}

func ps6113SignatureCompatible(static, concrete *types.Func) bool {
	staticSignature, _ := static.Type().(*types.Signature)
	concreteSignature, _ := concrete.Type().(*types.Signature)
	if staticSignature == nil || concreteSignature == nil || staticSignature.Variadic() != concreteSignature.Variadic() ||
		staticSignature.Params().Len() != concreteSignature.Params().Len() || staticSignature.Results().Len() != concreteSignature.Results().Len() {
		return false
	}
	for index := 0; index < staticSignature.Params().Len(); index++ {
		if !types.Identical(staticSignature.Params().At(index).Type(), concreteSignature.Params().At(index).Type()) {
			return false
		}
	}
	for index := 0; index < staticSignature.Results().Len(); index++ {
		if !types.Identical(staticSignature.Results().At(index).Type(), concreteSignature.Results().At(index).Type()) {
			return false
		}
	}
	return true
}

func ps6113ResolveMethod(pass *analysis.Pass, id string) (*types.Func, bool) {
	path, typeName, methodName, ok := ps6113MethodParts(id)
	if !ok {
		return nil, false
	}
	packageValue := ps6113Package(pass, path)
	if packageValue == nil {
		return nil, false
	}
	typeObject, _ := packageValue.Scope().Lookup(typeName).(*types.TypeName)
	if typeObject == nil || ps6087UninstantiatedGeneric(typeObject.Type()) {
		return nil, false
	}
	named := ps6087Named(typeObject.Type())
	if named == nil {
		return nil, false
	}
	for _, receiver := range []types.Type{named, types.NewPointer(named)} {
		methodSet := types.NewMethodSet(receiver)
		for index := 0; index < methodSet.Len(); index++ {
			selection := methodSet.At(index)
			function, _ := selection.Obj().(*types.Func)
			if function != nil && function.Name() == methodName && len(selection.Index()) == 1 && ps6113MethodObjectID(function) == id {
				return function, true
			}
		}
	}
	return nil, false
}

func ps6113Package(pass *analysis.Pass, path string) *types.Package {
	if pass.Pkg.Path() == path {
		return pass.Pkg
	}
	for _, imported := range pass.Pkg.Imports() {
		if imported.Path() == path {
			return imported
		}
	}
	return nil
}

func ps6113MethodParts(id string) (string, string, string, bool) {
	methodSeparator := strings.LastIndexByte(id, '.')
	if methodSeparator <= 0 {
		return "", "", "", false
	}
	receiverSeparator := strings.LastIndexByte(id[:methodSeparator], '.')
	if receiverSeparator <= 0 {
		return "", "", "", false
	}
	return id[:receiverSeparator], id[receiverSeparator+1 : methodSeparator], id[methodSeparator+1:], true
}

func ps6113MethodOwner(id string) string {
	path, receiver, _, ok := ps6113MethodParts(id)
	if !ok {
		return ""
	}
	return path + "." + receiver
}

func ps6113MethodObjectID(function *types.Func) string {
	if function == nil || function.Pkg() == nil {
		return ""
	}
	signature, _ := function.Type().(*types.Signature)
	if signature == nil || signature.Recv() == nil {
		return ""
	}
	receiver := ps6087Named(signature.Recv().Type())
	if receiver == nil {
		return ""
	}
	return function.Pkg().Path() + "." + receiver.Obj().Name() + "." + function.Name()
}
