package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/lint"
)

// PS6110 implements owner issue #855. It reports one narrow configured shape
// where a bulk native snapshot is copied into owned Go strings once per record.
var PS6110 = register(&lint.Check{
	ID:          "PS6110",
	Category:    "alloc",
	Slug:        "native-snapshot-repeated-string-ownership",
	Level:       lint.LevelAggressive,
	AutoFix:     false,
	NeedsConfig: true,
	Vocab:       []string{"nativeSnapshotStringCopyContracts"},
	Doc: lint.Documentation{
		Title: "bulk native snapshot clones an owned Go string once per record",
		Text: `A bulk native snapshot can remove repeated cgo crossings yet still
allocate one owned Go string per record when C.GoString, C.GoStringN, or an
equivalent typed wrapper runs in the materialization loop. When labels repeat,
an extraction-scoped exact-content cache can clone each distinct string once.

PS6110 is deliberately configured and narrow. A contract identifies the exact
typed candidate, the exact local snapshot acquisition, one-based pointer/count
out-argument and result roles, direct native/result fields, copying operation,
and external lifecycle. Version one requires local out-arguments, an integer
status guard, and a resolved external lifecycle; result acquisition, optional
status, local release, and token contracts are deferred. Source must locally
acquire pointer and count, validate the status, construct native and returned record slices from the same count, and
use one canonical zero-based index for both. Only direct fields and one direct
per-record binding are followed. Matching writes to the returned record slot
are expected; destination rebinding, escaping, capture, or opaque calls are not.

There is no automatic fix. The remedy must keep only extraction-local state,
compare exact native content and clone each distinct value into Go-owned memory
before the native lifetime ends. Never return or retain unsafe native string views, and do not
introduce receiver/global caches with wider retention or contention.

Exact-content sharing preserves immutable Go string values and Go ownership,
not per-occurrence allocation or backing-pointer identity. Code that observes
or mutates string storage through unsafe operations, or whose API requires
distinct identities, needs a separate audit; this advisory proves no such
identity equivalence.

An owner may separately expose a compact native identity after API-specific
stability, lifetime, and collision review. That optional producer-side ABI is
not represented or inferred by version one, and identity equality must never
replace exact content comparison.

C.GoBytes and typed []byte wrappers are always excluded. Their fresh results
are independently mutable slices, so sharing an equal cached result changes
aliasing, mutation, identity, and race behavior. PS6110 is string-only.
Preserve NUL-versus-explicit-length semantics, invalid UTF-8 bytes, empty
labels, ordering, errors, partial results, and ownership after native release.
Adopt the cache only after representative timing of repeated, mixed, cold,
one-record, and disabled paths; an allocation reduction alone does not imply a
net performance win. The five matched benchmark cells below are native-like Go
evidence for NUL-scanning strings, not cgo, GoStringN, or application timing.`,
		Before: `nativeRecords := unsafe.Slice(native, n)
out := make([]Event, n)
for i := range out {
	out[i].Label = C.GoString(&nativeRecords[i].label[0])
}`,
		After: `nativeRecords := unsafe.Slice(native, n)
out := make([]Event, n)
var labels extractionLabels
for i := range out {
	// own compares exact bytes, then clones each distinct value once.
	out[i].Label = labels.own(&nativeRecords[i].label[0])
}`,
		MeasuredWin: `In six alternating fresh-process pairs on Apple M2 Pro, the
native-like 340-record Go benchmark reduced warm and cold allocations from 341
to 11 and bytes from 11,584 to 6,304, but was respectively 24.7% and 10.8%
slower. The mixed-label case reduced allocations from 341 to 184 while bytes
rose from 11,584 to 27,736 and time rose 204.3%. One-record and empty paths were
12.5% and 40.8% faster with unchanged allocation counts. These NUL-scanning Go
results are not cgo, GoStringN, or application measurements; time a
representative workload before adopting the cache.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6110",
		Doc:  "bulk native snapshots that repeat owned string copies per record",
		Run:  runPS6110,
	},
})

func runPS6110(pass *analysis.Pass) (any, error) {
	return runPS6110WithContracts(pass, config.Current().NativeSnapshotStringCopyContracts)
}

func runPS6110WithContracts(pass *analysis.Pass, contracts []config.NativeSnapshotStringCopyContract) (any, error) {
	configured := make(map[string][]*config.NativeSnapshotStringCopyContract)
	for index := range contracts {
		contract := &contracts[index]
		if contract.Valid() {
			configured[contract.CandidateCallable] = append(configured[contract.CandidateCallable], contract)
		}
	}
	if len(configured) == 0 {
		return nil, nil
	}
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			object, ok := pass.TypesInfo.Defs[function.Name].(*types.Func)
			if !ok {
				continue
			}
			for _, contract := range configured[ps6090FunctionID(object)] {
				if !ps6110FunctionKind(object, contract.CandidateKind) {
					continue
				}
				ps6110Function(pass, file, function, object, contract)
			}
		}
	}
	return nil, nil
}

type ps6110Acquisition struct {
	statement int
	call      *ast.CallExpr
	records   *types.Var
	count     *types.Var
	status    *types.Var
}

type ps6110Shape struct {
	destination *types.Var
	nativeView  *types.Var
	countValue  types.Object
	loopIndex   *types.Var
	recordAlias *types.Var
	loop        ast.Stmt
	loopBody    *ast.BlockStmt
}

func ps6110Function(pass *analysis.Pass, file *ast.File, function *ast.FuncDecl, object *types.Func, contract *config.NativeSnapshotStringCopyContract) {
	signature, ok := object.Type().(*types.Signature)
	if !ok || !ps6110DestinationType(signature, contract) || !ps6110LifecycleResolved(pass, contract) {
		return
	}
	acquisition, ok := ps6110FindAcquisition(pass, file, function.Body, contract)
	if !ok || !ps6110NativeFields(acquisition.records.Type(), contract) {
		return
	}
	guard, ok := ps6110StatusGuard(pass, function.Body, acquisition, contract.AcquireSuccessInteger)
	if !ok {
		return
	}
	shape, ok := ps6110FindShape(pass, function.Body, signature, acquisition, guard, contract)
	if !ok || !ps6110TerminalReturn(pass, function.Body, shape, contract.DestinationResultPosition) ||
		ps6110CallsLifecycle(pass, function.Body, contract) {
		return
	}
	copyCall, ok := ps6110MaterializingCopy(pass, file, shape, contract)
	parents := ps6087Parents(function.Body)
	reachable := ps6099ReachableNodesInBlock(pass, function.Body, parents)
	if !ok || !reachable[acquisition.call] || !reachable[copyCall] ||
		!ps6110StraightLoop(pass, shape.loopBody) ||
		ps6110UnsafeState(pass, file, function.Body, acquisition, shape, copyCall, contract) {
		return
	}
	label := contract.Name
	if label == "" {
		label = contract.CandidateCallable
	}
	copyName := "C.GoString"
	if contract.CopyKind == config.NativeSnapshotCopyGoStringN {
		copyName = "C.GoStringN"
	} else if contract.CopyCallable != "" {
		copyName = contract.CopyCallable
	}
	pass.Reportf(copyCall.Pos(), "%s: %s copies an owned Go string once per record from the configured bulk snapshot %s; consider extraction-scoped exact-content deduplication that clones each distinct string once, and benchmark representative inputs before adoption (advisory, no automatic fix)", label, copyName, contract.AcquireCallable)
}

func ps6110FunctionKind(function *types.Func, kind config.NativeSnapshotCallKind) bool {
	signature, ok := function.Type().(*types.Signature)
	if !ok {
		return false
	}
	return signature.Recv() == nil && kind == config.NativeSnapshotCallFunction ||
		signature.Recv() != nil && kind == config.NativeSnapshotCallMethod
}

func ps6110FindAcquisition(pass *analysis.Pass, file *ast.File, body *ast.BlockStmt, contract *config.NativeSnapshotStringCopyContract) (ps6110Acquisition, bool) {
	for statementIndex, statement := range body.List {
		assignment, ok := statement.(*ast.AssignStmt)
		if !ok || len(assignment.Rhs) != 1 {
			continue
		}
		call, ok := ps2110Unparen(assignment.Rhs[0]).(*ast.CallExpr)
		if !ok || !ps6110CallMatches(pass, file, call, contract.AcquireCallable, contract.AcquireKind) {
			continue
		}
		statusIndex := contract.AcquireStatusResultPosition - 1
		if statusIndex < 0 || statusIndex >= len(assignment.Lhs) || !ps6110IntegerResult(pass, call, statusIndex) {
			continue
		}
		statusIdentifier, ok := ps2110Unparen(assignment.Lhs[statusIndex]).(*ast.Ident)
		if !ok || statusIdentifier.Name == "_" {
			continue
		}
		status, ok := ps6110Binding(pass, statusIdentifier, assignment.Tok).(*types.Var)
		if !ok {
			continue
		}
		records, recordsOK := ps6110OutArgument(pass, body, call, contract.RecordsOutArgumentPosition)
		count, countOK := ps6110OutArgument(pass, body, call, contract.CountOutArgumentPosition)
		if !recordsOK || !countOK || records == count || !ps6110RecordPointer(records.Type()) || !ps6110Integer(count.Type()) {
			continue
		}
		return ps6110Acquisition{statement: statementIndex, call: call, records: records, count: count, status: status}, true
	}
	return ps6110Acquisition{}, false
}

func ps6110Binding(pass *analysis.Pass, identifier *ast.Ident, tokenKind token.Token) types.Object {
	if tokenKind == token.DEFINE {
		return pass.TypesInfo.Defs[identifier]
	}
	return pass.TypesInfo.Uses[identifier]
}

func ps6110OutArgument(pass *analysis.Pass, body *ast.BlockStmt, call *ast.CallExpr, position int) (*types.Var, bool) {
	index := position - 1
	if index < 0 {
		return nil, false
	}
	argument := ast.Expr(nil)
	if index < len(call.Args) {
		argument = call.Args[index]
	} else if literal, ok := ps2110Unparen(call.Fun).(*ast.FuncLit); ok {
		inner := ps6110GeneratedCgoCall(literal.Body)
		if inner == nil || index >= len(inner.Args) {
			return nil, false
		}
		argument = ps6110ResolveLocalAlias(pass, literal.Body, inner.Args[index], nil)
	}
	address, ok := ps2110Unparen(argument).(*ast.UnaryExpr)
	if !ok || address.Op != token.AND {
		return nil, false
	}
	identifier, ok := ps2110Unparen(address.X).(*ast.Ident)
	if !ok {
		return nil, false
	}
	variable, ok := pass.TypesInfo.Uses[identifier].(*types.Var)
	return variable, ok && ps6110LocalDeclared(pass, body, variable)
}

func ps6110LocalDeclared(pass *analysis.Pass, body *ast.BlockStmt, variable *types.Var) bool {
	if variable.Pos() >= body.Pos() && variable.Pos() <= body.End() {
		return true
	}
	local := false
	ast.Inspect(body, func(node ast.Node) bool {
		if local {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		identifier, ok := node.(*ast.Ident)
		if ok && pass.TypesInfo.Defs[identifier] == variable {
			local = true
			return false
		}
		return true
	})
	return local
}

func ps6110ResolveLocalAlias(pass *analysis.Pass, body *ast.BlockStmt, expression ast.Expr, seen map[types.Object]bool) ast.Expr {
	identifier, ok := ps2110Unparen(expression).(*ast.Ident)
	if !ok {
		return expression
	}
	object := pass.TypesInfo.Uses[identifier]
	if object == nil {
		object = pass.TypesInfo.Defs[identifier]
	}
	if object == nil {
		return expression
	}
	if seen == nil {
		seen = make(map[types.Object]bool)
	}
	if seen[object] {
		return expression
	}
	seen[object] = true
	var value ast.Expr
	ast.Inspect(body, func(node ast.Node) bool {
		if value != nil {
			return false
		}
		switch declaration := node.(type) {
		case *ast.AssignStmt:
			for index, lhs := range declaration.Lhs {
				id, direct := ps2110Unparen(lhs).(*ast.Ident)
				if direct && pass.TypesInfo.Defs[id] == object && index < len(declaration.Rhs) {
					value = declaration.Rhs[index]
					return false
				}
			}
		case *ast.ValueSpec:
			for index, name := range declaration.Names {
				if pass.TypesInfo.Defs[name] == object && index < len(declaration.Values) {
					value = declaration.Values[index]
					return false
				}
			}
		}
		return true
	})
	if value == nil {
		return expression
	}
	return ps6110ResolveLocalAlias(pass, body, value, seen)
}

func ps6110StatusGuard(pass *analysis.Pass, body *ast.BlockStmt, acquisition ps6110Acquisition, success int64) (int, bool) {
	for index, statement := range body.List[acquisition.statement+1:] {
		guard, ok := statement.(*ast.IfStmt)
		if !ok {
			return 0, false
		}
		binary, ok := ps2110Unparen(guard.Cond).(*ast.BinaryExpr)
		if !ok || binary.Op != token.NEQ || !ps6110ObjectExpression(pass, binary.X, acquisition.status) ||
			!ps6110IntegerConstant(pass, binary.Y, success) || !ps6110BlockReturns(guard.Body) {
			continue
		}
		return acquisition.statement + index + 1, true
	}
	return 0, false
}

func ps6110FindShape(pass *analysis.Pass, body *ast.BlockStmt, signature *types.Signature, acquisition ps6110Acquisition, guard int, contract *config.NativeSnapshotStringCopyContract) (ps6110Shape, bool) {
	var destination, nativeView *types.Var
	var countValue types.Object = acquisition.count
	start := guard + 1
	for index := start; index < len(body.List); index++ {
		statement := body.List[index]
		if assignment, ok := statement.(*ast.AssignStmt); ok {
			if value, ok := ps6110DirectDefinition(pass, assignment); ok {
				if ps6110CountConversion(pass, value.expression, acquisition.count) {
					countValue = value.object
				}
				if ps6110NativeSlice(pass, value.expression, acquisition.records, countValue) {
					nativeView = value.object
				}
				if ps6110DestinationValue(pass, value.expression, countValue, signature, contract) {
					destination = value.object
				}
			}
		}
		loopBody, loopIndex, ok := ps6110Loop(pass, statement, destination, countValue, contract)
		if !ok {
			continue
		}
		if destination == nil || nativeView == nil || loopIndex == nil || !ps6110MultiRecordPath(pass, body.List[guard+1:index], acquisition.count) {
			return ps6110Shape{}, false
		}
		return ps6110Shape{destination: destination, nativeView: nativeView, countValue: countValue, loopIndex: loopIndex,
			recordAlias: ps6110RecordAlias(pass, loopBody, nativeView, loopIndex), loop: statement, loopBody: loopBody}, true
	}
	return ps6110Shape{}, false
}

type ps6110Definition struct {
	object     *types.Var
	expression ast.Expr
}

func ps6110DirectDefinition(pass *analysis.Pass, assignment *ast.AssignStmt) (ps6110Definition, bool) {
	if assignment.Tok != token.DEFINE || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
		return ps6110Definition{}, false
	}
	identifier, ok := ps2110Unparen(assignment.Lhs[0]).(*ast.Ident)
	if !ok || identifier.Name == "_" {
		return ps6110Definition{}, false
	}
	object, ok := pass.TypesInfo.Defs[identifier].(*types.Var)
	return ps6110Definition{object: object, expression: ps2110Unparen(assignment.Rhs[0])}, ok
}

func ps6110CountConversion(pass *analysis.Pass, expression ast.Expr, count *types.Var) bool {
	call, ok := expression.(*ast.CallExpr)
	if !ok || len(call.Args) != 1 || !ps6110ObjectExpression(pass, call.Args[0], count) {
		return false
	}
	identifier, identifierOK := ps2110Unparen(call.Fun).(*ast.Ident)
	if !identifierOK {
		return false
	}
	typeName, typeOK := pass.TypesInfo.Uses[identifier].(*types.TypeName)
	return typeOK && types.Identical(typeName.Type(), types.Typ[types.Int])
}

func ps6110NativeSlice(pass *analysis.Pass, expression ast.Expr, records *types.Var, count types.Object) bool {
	call, ok := expression.(*ast.CallExpr)
	if !ok || len(call.Args) != 2 || !ps6110ObjectExpression(pass, call.Args[0], records) || !ps6110ObjectExpression(pass, call.Args[1], count) {
		return false
	}
	selector, ok := ps2110Unparen(call.Fun).(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "Slice" {
		return false
	}
	qualifier, qualifierOK := ps2110Unparen(selector.X).(*ast.Ident)
	if !qualifierOK {
		return false
	}
	packageName, packageOK := pass.TypesInfo.Uses[qualifier].(*types.PkgName)
	return packageOK && packageName.Imported().Path() == "unsafe"
}

func ps6110DestinationValue(pass *analysis.Pass, expression ast.Expr, count types.Object, signature *types.Signature, contract *config.NativeSnapshotStringCopyContract) bool {
	if contract.DestinationSliceField == "" {
		return ps6110MakeSlice(pass, expression, count)
	}
	literal, ok := expression.(*ast.CompositeLit)
	if !ok || !types.Identical(pass.TypesInfo.TypeOf(literal), signature.Results().At(contract.DestinationResultPosition-1).Type()) {
		return false
	}
	for _, element := range literal.Elts {
		keyValue, ok := element.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, keyOK := ps2110Unparen(keyValue.Key).(*ast.Ident)
		if keyOK && key.Name == contract.DestinationSliceField {
			return ps6110MakeSlice(pass, ps2110Unparen(keyValue.Value), count)
		}
	}
	return false
}

func ps6110MakeSlice(pass *analysis.Pass, expression ast.Expr, count types.Object) bool {
	call, ok := expression.(*ast.CallExpr)
	if !ok || len(call.Args) != 2 || !ps6110ObjectExpression(pass, call.Args[1], count) {
		return false
	}
	identifier, identifierOK := ps2110Unparen(call.Fun).(*ast.Ident)
	if !identifierOK {
		return false
	}
	builtin, builtinOK := pass.TypesInfo.Uses[identifier].(*types.Builtin)
	return builtinOK && builtin.Name() == "make"
}

func ps6110Loop(pass *analysis.Pass, statement ast.Stmt, destination *types.Var, count types.Object, contract *config.NativeSnapshotStringCopyContract) (*ast.BlockStmt, *types.Var, bool) {
	if destination == nil {
		return nil, nil, false
	}
	switch loop := statement.(type) {
	case *ast.RangeStmt:
		if loop.Tok != token.DEFINE || loop.Value != nil || !ps6110DestinationSliceExpression(pass, loop.X, destination, contract.DestinationSliceField) {
			return nil, nil, false
		}
		identifier, identifierOK := ps2110Unparen(loop.Key).(*ast.Ident)
		if !identifierOK {
			return nil, nil, false
		}
		index, indexOK := pass.TypesInfo.Defs[identifier].(*types.Var)
		return loop.Body, index, indexOK
	case *ast.ForStmt:
		assignment, ok := loop.Init.(*ast.AssignStmt)
		if !ok || assignment.Tok != token.DEFINE || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 ||
			!ps6110IntegerConstant(pass, assignment.Rhs[0], 0) {
			return nil, nil, false
		}
		identifier, identifierOK := ps2110Unparen(assignment.Lhs[0]).(*ast.Ident)
		if !identifierOK {
			return nil, nil, false
		}
		index, indexOK := pass.TypesInfo.Defs[identifier].(*types.Var)
		condition, conditionOK := ps2110Unparen(loop.Cond).(*ast.BinaryExpr)
		post, postOK := loop.Post.(*ast.IncDecStmt)
		if !indexOK || !conditionOK || condition.Op != token.LSS || !ps6110ObjectExpression(pass, condition.X, index) ||
			!ps6110ObjectExpression(pass, condition.Y, count) || !postOK || post.Tok != token.INC ||
			!ps6110ObjectExpression(pass, post.X, index) {
			return nil, nil, false
		}
		return loop.Body, index, true
	}
	return nil, nil, false
}

func ps6110MultiRecordPath(pass *analysis.Pass, statements []ast.Stmt, count *types.Var) bool {
	for _, statement := range statements {
		guard, ok := statement.(*ast.IfStmt)
		if !ok || !ps6110BlockReturns(guard.Body) {
			continue
		}
		binary, ok := ps2110Unparen(guard.Cond).(*ast.BinaryExpr)
		if !ok || !ps6110ObjectExpression(pass, binary.X, count) && !ps6110ObjectExpression(pass, binary.Y, count) {
			continue
		}
		other := binary.Y
		if ps6110ObjectExpression(pass, binary.Y, count) {
			other = binary.X
		}
		if binary.Op == token.EQL && ps6110IntegerConstant(pass, other, 1) {
			return true
		}
	}
	return false
}

func ps6110MaterializingCopy(pass *analysis.Pass, file *ast.File, shape ps6110Shape, contract *config.NativeSnapshotStringCopyContract) (*ast.CallExpr, bool) {
	parents := ps6087Parents(shape.loopBody)
	var match *ast.CallExpr
	valid := true
	ast.Inspect(shape.loopBody, func(node ast.Node) bool {
		if !valid {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if !ps6110CopyCall(pass, file, call, shape, contract) {
			return true
		}
		if match != nil || !ps6110CopyDestination(pass, call, parents, shape, contract) {
			valid = false
			return false
		}
		match = call
		return false
	})
	return match, valid && match != nil
}

func ps6110StraightLoop(pass *analysis.Pass, body *ast.BlockStmt) bool {
	straight := true
	ast.Inspect(body, func(node ast.Node) bool {
		if !straight {
			return false
		}
		if node != body {
			if _, nested := node.(*ast.FuncLit); nested {
				return false
			}
		}
		switch value := node.(type) {
		case *ast.BranchStmt, *ast.ReturnStmt, *ast.GoStmt, *ast.DeferStmt:
			straight = false
			return false
		case *ast.CallExpr:
			if typedBuiltinName(pass, value.Fun, "panic") {
				straight = false
				return false
			}
		}
		return true
	})
	return straight
}

func ps6110CopyCall(pass *analysis.Pass, file *ast.File, call *ast.CallExpr, shape ps6110Shape, contract *config.NativeSnapshotStringCopyContract) bool {
	if contract.CopyKind == config.NativeSnapshotCopyGoString || contract.CopyKind == config.NativeSnapshotCopyGoStringN {
		want := "C.GoString"
		if contract.CopyKind == config.NativeSnapshotCopyGoStringN {
			want = "C.GoStringN"
		}
		if !ps6110CallMatches(pass, file, call, want, config.NativeSnapshotCallCgo) {
			return false
		}
	} else if !ps6110CallMatches(pass, file, call, contract.CopyCallable, contract.CopyKind) {
		return false
	}
	pointerIndex := contract.CopyPointerArgumentPosition - 1
	pointerOK := pointerIndex >= 0 && pointerIndex < len(call.Args) && ps6110NativeLabelPointer(pass, call.Args[pointerIndex], shape, contract)
	if !pointerOK {
		return false
	}
	if contract.CopyLengthArgumentPosition > 0 {
		lengthIndex := contract.CopyLengthArgumentPosition - 1
		if lengthIndex >= len(call.Args) || !ps6110NativeLength(pass, call.Args[lengthIndex], shape, contract.NativeLengthField) {
			return false
		}
	}
	return ps6110CopySignature(pass, call, contract)
}

func ps6110NativeLabelPointer(pass *analysis.Pass, expression ast.Expr, shape ps6110Shape, contract *config.NativeSnapshotStringCopyContract) bool {
	address, ok := ps2110Unparen(expression).(*ast.UnaryExpr)
	if !ok || address.Op != token.AND {
		return false
	}
	index, ok := ps2110Unparen(address.X).(*ast.IndexExpr)
	if !ok || !ps6110IntegerConstant(pass, index.Index, 0) {
		return false
	}
	selector, ok := ps2110Unparen(index.X).(*ast.SelectorExpr)
	return ok && selector.Sel.Name == contract.NativeStringField && ps6110NativeRecordExpression(pass, selector.X, shape)
}

func ps6110NativeLength(pass *analysis.Pass, expression ast.Expr, shape ps6110Shape, field string) bool {
	selector, ok := ps2110Unparen(expression).(*ast.SelectorExpr)
	return ok && selector.Sel.Name == field && ps6110NativeRecordExpression(pass, selector.X, shape)
}

func ps6110NativeRecordExpression(pass *analysis.Pass, expression ast.Expr, shape ps6110Shape) bool {
	expression = ps2110Unparen(expression)
	if star, ok := expression.(*ast.StarExpr); ok {
		expression = ps2110Unparen(star.X)
	}
	if identifier, ok := expression.(*ast.Ident); ok {
		return shape.recordAlias != nil && pass.TypesInfo.Uses[identifier] == shape.recordAlias
	}
	index, ok := expression.(*ast.IndexExpr)
	return ok && ps6110ObjectExpression(pass, index.X, shape.nativeView) && ps6110ObjectExpression(pass, index.Index, shape.loopIndex)
}

func ps6110RecordAlias(pass *analysis.Pass, body *ast.BlockStmt, nativeView, loopIndex *types.Var) *types.Var {
	for _, statement := range body.List {
		assignment, ok := statement.(*ast.AssignStmt)
		if !ok || assignment.Tok != token.DEFINE || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
			continue
		}
		identifier, ok := ps2110Unparen(assignment.Lhs[0]).(*ast.Ident)
		address, addressOK := ps2110Unparen(assignment.Rhs[0]).(*ast.UnaryExpr)
		if !ok || !addressOK || address.Op != token.AND {
			continue
		}
		index, indexOK := ps2110Unparen(address.X).(*ast.IndexExpr)
		if !indexOK || !ps6110ObjectExpression(pass, index.X, nativeView) || !ps6110ObjectExpression(pass, index.Index, loopIndex) {
			continue
		}
		alias, _ := pass.TypesInfo.Defs[identifier].(*types.Var)
		return alias
	}
	return nil
}

func ps6110CopyDestination(pass *analysis.Pass, call *ast.CallExpr, parents map[ast.Node]ast.Node, shape ps6110Shape, contract *config.NativeSnapshotStringCopyContract) bool {
	for node := ast.Node(call); node != nil; node = parents[node] {
		switch parent := parents[node].(type) {
		case *ast.KeyValueExpr:
			key, ok := ps2110Unparen(parent.Key).(*ast.Ident)
			if !ok || key.Name != contract.DestinationStringField || parent.Value != node {
				continue
			}
			literal, ok := parents[parent].(*ast.CompositeLit)
			if !ok {
				return false
			}
			assignment, ok := parents[literal].(*ast.AssignStmt)
			return ok && parents[assignment] == shape.loopBody && len(assignment.Lhs) == 1 &&
				ps6110DestinationIndex(pass, assignment.Lhs[0], shape, contract)
		case *ast.AssignStmt:
			if len(parent.Lhs) != 1 || len(parent.Rhs) != 1 || parent.Rhs[0] != node {
				return false
			}
			selector, ok := ps2110Unparen(parent.Lhs[0]).(*ast.SelectorExpr)
			return ok && parents[parent] == shape.loopBody && selector.Sel.Name == contract.DestinationStringField &&
				ps6110DestinationIndex(pass, selector.X, shape, contract)
		}
	}
	return false
}

func ps6110DestinationIndex(pass *analysis.Pass, expression ast.Expr, shape ps6110Shape, contract *config.NativeSnapshotStringCopyContract) bool {
	index, ok := ps2110Unparen(expression).(*ast.IndexExpr)
	return ok && ps6110DestinationSliceExpression(pass, index.X, shape.destination, contract.DestinationSliceField) &&
		ps6110ObjectExpression(pass, index.Index, shape.loopIndex)
}

func ps6110DestinationSliceExpression(pass *analysis.Pass, expression ast.Expr, destination *types.Var, field string) bool {
	if field == "" {
		return ps6110ObjectExpression(pass, expression, destination)
	}
	selector, ok := ps2110Unparen(expression).(*ast.SelectorExpr)
	return ok && selector.Sel.Name == field && ps6110ObjectExpression(pass, selector.X, destination)
}

func ps6110TerminalReturn(pass *analysis.Pass, body *ast.BlockStmt, shape ps6110Shape, resultPosition int) bool {
	loopIndex := -1
	for index, statement := range body.List {
		if statement == shape.loop {
			loopIndex = index
			break
		}
	}
	if loopIndex < 0 || loopIndex+1 != len(body.List)-1 {
		return false
	}
	terminal, ok := body.List[len(body.List)-1].(*ast.ReturnStmt)
	index := resultPosition - 1
	return ok && index >= 0 && index < len(terminal.Results) && ps6110ObjectExpression(pass, terminal.Results[index], shape.destination)
}

func ps6110UnsafeState(pass *analysis.Pass, file *ast.File, body *ast.BlockStmt, acquisition ps6110Acquisition, shape ps6110Shape, copyCall *ast.CallExpr, contract *config.NativeSnapshotStringCopyContract) bool {
	critical := map[types.Object]bool{acquisition.records: true, acquisition.count: true, acquisition.status: true,
		shape.nativeView: true, shape.loopIndex: true}
	if shape.countValue != acquisition.count {
		critical[shape.countValue] = true
	}
	if shape.recordAlias != nil {
		critical[shape.recordAlias] = true
	}
	parents := ps6087Parents(body)
	mutated := false
	ast.Inspect(body, func(node ast.Node) bool {
		if mutated {
			return false
		}
		switch value := node.(type) {
		case *ast.FuncLit:
			if value == ps2110Unparen(acquisition.call.Fun) || value == ps2110Unparen(copyCall.Fun) {
				return false
			}
			if ps2004NodeUsesObject(pass, value.Body, shape.destination) {
				mutated = true
				return false
			}
			for object := range critical {
				if ps2004NodeUsesObject(pass, value.Body, object) {
					mutated = true
					return false
				}
			}
			return false
		case *ast.AssignStmt:
			for _, lhs := range value.Lhs {
				root := ps6110LValueRoot(pass, lhs)
				if critical[root] || root == shape.destination &&
					!ps6110AllowedDestinationStore(pass, lhs, parents, shape, copyCall, contract) {
					mutated = true
					return false
				}
			}
			if !ps6110AllowedStateAssignment(pass, value, parents, acquisition, shape, copyCall, contract) &&
				ps6110ExpressionsUseState(pass, value.Rhs, critical, shape.destination) {
				mutated = true
				return false
			}
		case *ast.ValueSpec:
			if ps6110ExpressionsUseState(pass, value.Values, critical, shape.destination) {
				mutated = true
				return false
			}
		case *ast.IncDecStmt:
			mutated = critical[ps6110LValueRoot(pass, value.X)] && !ps6110LoopPost(shape.loop, value)
		case *ast.RangeStmt:
			if value != shape.loop {
				mutated = critical[ps6110LValueRoot(pass, value.Key)] || critical[ps6110LValueRoot(pass, value.Value)]
			}
		case *ast.SendStmt:
			mutated = ps6110ExpressionsUseState(pass, []ast.Expr{value.Chan, value.Value}, critical, shape.destination)
		case *ast.ReturnStmt:
			terminal, _ := body.List[len(body.List)-1].(*ast.ReturnStmt)
			for index, result := range value.Results {
				if value == terminal && index == contract.DestinationResultPosition-1 &&
					ps6110ObjectExpression(pass, result, shape.destination) {
					continue
				}
				if ps6110ReturnUsesState(pass, file, result, critical, shape.destination, contract) {
					mutated = true
					break
				}
			}
		case *ast.UnaryExpr:
			if value.Op == token.AND {
				root := ps6110LValueRoot(pass, value.X)
				if root == shape.destination || critical[root] &&
					!ps6110Inside(value, acquisition.call, parents) && !ps6110Inside(value, copyCall, parents) &&
					!ps6110InsideAllowedCall(pass, file, value, parents, acquisition, shape, contract) &&
					!ps6110RecordBindingAddress(pass, value, shape) {
					mutated = true
				}
			}
		case *ast.CallExpr:
			if value == acquisition.call || value == copyCall || ps6110AllowedStateCall(pass, file, value, acquisition, shape, contract) {
				return true
			}
			for _, argument := range value.Args {
				for object := range critical {
					if object != nil && ps2004NodeUsesObject(pass, argument, object) {
						mutated = true
						return false
					}
				}
				if ps2004NodeUsesObject(pass, argument, shape.destination) {
					mutated = true
					return false
				}
			}
			for object := range critical {
				if object != nil && ps2004NodeUsesObject(pass, value.Fun, object) {
					mutated = true
					return false
				}
			}
			if ps2004NodeUsesObject(pass, value.Fun, shape.destination) {
				mutated = true
				return false
			}
		}
		return !mutated
	})
	return mutated
}

func ps6110ReturnUsesState(pass *analysis.Pass, file *ast.File, expression ast.Expr, critical map[types.Object]bool,
	destination types.Object, contract *config.NativeSnapshotStringCopyContract,
) bool {
	used := false
	ast.Inspect(expression, func(node ast.Node) bool {
		if used {
			return false
		}
		if call, ok := node.(*ast.CallExpr); ok {
			if ps6110ConfiguredCopyCall(pass, file, call, contract) {
				return false
			}
		}
		identifier, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		object := pass.TypesInfo.ObjectOf(identifier)
		used = object == destination || critical[object]
		return !used
	})
	return used
}

func ps6110ConfiguredCopyCall(pass *analysis.Pass, file *ast.File, call *ast.CallExpr,
	contract *config.NativeSnapshotStringCopyContract,
) bool {
	id := contract.CopyCallable
	kind := contract.CopyKind
	if contract.CopyKind == config.NativeSnapshotCopyGoString {
		id = "C.GoString"
		kind = config.NativeSnapshotCallCgo
	} else if contract.CopyKind == config.NativeSnapshotCopyGoStringN {
		id = "C.GoStringN"
		kind = config.NativeSnapshotCallCgo
	}
	return ps6110CallMatches(pass, file, call, id, kind)
}

func ps6110LoopPost(loop ast.Stmt, increment *ast.IncDecStmt) bool {
	forLoop, ok := loop.(*ast.ForStmt)
	return ok && forLoop.Post == increment
}

func ps6110ExpressionsUseState(pass *analysis.Pass, expressions []ast.Expr, critical map[types.Object]bool, destination types.Object) bool {
	for _, expression := range expressions {
		if ps2004NodeUsesObject(pass, expression, destination) {
			return true
		}
		for object := range critical {
			if object != nil && ps2004NodeUsesObject(pass, expression, object) {
				return true
			}
		}
	}
	return false
}

func ps6110AllowedStateAssignment(pass *analysis.Pass, assignment *ast.AssignStmt,
	parents map[ast.Node]ast.Node, acquisition ps6110Acquisition, shape ps6110Shape, copyCall *ast.CallExpr,
	contract *config.NativeSnapshotStringCopyContract,
) bool {
	if ps6110Inside(acquisition.call, assignment, parents) {
		return true
	}
	if ps6110Inside(copyCall, assignment, parents) {
		if !ps6110CopyDestination(pass, copyCall, parents, shape, contract) {
			return false
		}
		exposed := map[types.Object]bool{
			acquisition.records: true,
			shape.nativeView:    true,
			shape.recordAlias:   shape.recordAlias != nil,
			shape.destination:   true,
		}
		return !ps6110ExpressionsUseStateOutside(pass, assignment.Rhs, exposed, copyCall)
	}
	if definition, ok := ps6110DirectDefinition(pass, assignment); ok {
		if definition.object == shape.destination || definition.object == shape.nativeView || definition.object == shape.countValue {
			return true
		}
		if definition.object == shape.recordAlias {
			address, addressOK := definition.expression.(*ast.UnaryExpr)
			return addressOK && address.Op == token.AND && ps6110RecordBindingAddress(pass, address, shape)
		}
	}
	for _, lhs := range assignment.Lhs {
		if ps6110LValueRoot(pass, lhs) == shape.destination &&
			ps6110AllowedDestinationStore(pass, lhs, parents, shape, copyCall, contract) {
			sources := map[types.Object]bool{acquisition.records: true, acquisition.count: true, shape.nativeView: true}
			if shape.countValue != acquisition.count {
				sources[shape.countValue] = true
			}
			if shape.recordAlias != nil {
				sources[shape.recordAlias] = true
			}
			return !ps6110ExpressionsUseState(pass, assignment.Rhs, sources, shape.destination)
		}
	}
	return false
}

func ps6110ExpressionsUseStateOutside(pass *analysis.Pass, expressions []ast.Expr, objects map[types.Object]bool, allowed ast.Node) bool {
	used := false
	for _, expression := range expressions {
		ast.Inspect(expression, func(node ast.Node) bool {
			if used || node == allowed {
				return false
			}
			identifier, ok := node.(*ast.Ident)
			if ok && objects[pass.TypesInfo.ObjectOf(identifier)] {
				used = true
				return false
			}
			return true
		})
		if used {
			return true
		}
	}
	return false
}

func ps6110InsideAllowedCall(pass *analysis.Pass, file *ast.File, node ast.Node, parents map[ast.Node]ast.Node, acquisition ps6110Acquisition, shape ps6110Shape, contract *config.NativeSnapshotStringCopyContract) bool {
	for current := node; current != nil; current = parents[current] {
		if call, ok := current.(*ast.CallExpr); ok {
			return ps6110AllowedStateCall(pass, file, call, acquisition, shape, contract)
		}
	}
	return false
}

func ps6110AllowedDestinationStore(pass *analysis.Pass, expression ast.Expr, parents map[ast.Node]ast.Node,
	shape ps6110Shape, copyCall *ast.CallExpr, contract *config.NativeSnapshotStringCopyContract,
) bool {
	if ps6110DestinationIndex(pass, expression, shape, contract) {
		return ps6110SameAssignment(expression, copyCall, parents)
	}
	selector, ok := ps2110Unparen(expression).(*ast.SelectorExpr)
	if !ok || !ps6110DestinationIndex(pass, selector.X, shape, contract) {
		return false
	}
	return selector.Sel.Name != contract.DestinationStringField || ps6110SameAssignment(expression, copyCall, parents)
}

func ps6110SameAssignment(node ast.Node, copyCall *ast.CallExpr, parents map[ast.Node]ast.Node) bool {
	for current := node; current != nil; current = parents[current] {
		if assignment, ok := current.(*ast.AssignStmt); ok {
			return ps6110Inside(copyCall, assignment, parents)
		}
	}
	return false
}

func ps6110AllowedStateCall(pass *analysis.Pass, file *ast.File, call *ast.CallExpr, acquisition ps6110Acquisition, shape ps6110Shape, contract *config.NativeSnapshotStringCopyContract) bool {
	if ps6110CountConversion(pass, call, acquisition.count) {
		return true
	}
	if ps6110NativeSlice(pass, call, acquisition.records, shape.countValue) {
		return true
	}
	if ps6110MakeSlice(pass, call, shape.countValue) {
		return true
	}
	if contract.CopyKind == config.NativeSnapshotCopyGoString || contract.CopyKind == config.NativeSnapshotCopyGoStringN {
		name := "C.GoString"
		if contract.CopyKind == config.NativeSnapshotCopyGoStringN {
			name = "C.GoStringN"
		}
		return ps6110CallMatches(pass, file, call, name, config.NativeSnapshotCallCgo)
	}
	return ps6110CallMatches(pass, file, call, contract.CopyCallable, contract.CopyKind)
}

func ps6110Inside(node, ancestor ast.Node, parents map[ast.Node]ast.Node) bool {
	for current := node; current != nil; current = parents[current] {
		if current == ancestor {
			return true
		}
	}
	return false
}

func ps6110RecordBindingAddress(pass *analysis.Pass, address *ast.UnaryExpr, shape ps6110Shape) bool {
	index, ok := ps2110Unparen(address.X).(*ast.IndexExpr)
	return ok && shape.recordAlias != nil && ps6110ObjectExpression(pass, index.X, shape.nativeView) &&
		ps6110ObjectExpression(pass, index.Index, shape.loopIndex)
}

func ps6110LValueRoot(pass *analysis.Pass, expression ast.Expr) types.Object {
	expression = ps2110Unparen(expression)
	for {
		switch value := expression.(type) {
		case *ast.Ident:
			return pass.TypesInfo.Uses[value]
		case *ast.SelectorExpr:
			expression = ps2110Unparen(value.X)
		case *ast.IndexExpr:
			expression = ps2110Unparen(value.X)
		case *ast.StarExpr:
			expression = ps2110Unparen(value.X)
		default:
			return nil
		}
	}
}

func ps6110CallsLifecycle(pass *analysis.Pass, body *ast.BlockStmt, contract *config.NativeSnapshotStringCopyContract) bool {
	found := false
	ast.Inspect(body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if ok && ps6087FunctionID(pass, call) == contract.LifecycleCallable {
			found = true
			return false
		}
		return !found
	})
	return found
}

func ps6110LifecycleResolved(pass *analysis.Pass, contract *config.NativeSnapshotStringCopyContract) bool {
	for _, object := range pass.TypesInfo.Defs {
		function, ok := object.(*types.Func)
		if ok && ps6090FunctionID(function) == contract.LifecycleCallable && ps6110FunctionKind(function, contract.LifecycleKind) {
			return true
		}
	}
	return false
}

func ps6110DestinationType(signature *types.Signature, contract *config.NativeSnapshotStringCopyContract) bool {
	index := contract.DestinationResultPosition - 1
	if signature.Results() == nil || index < 0 || index >= signature.Results().Len() {
		return false
	}
	value := signature.Results().At(index).Type()
	if contract.DestinationSliceField != "" {
		field, ok := ps6110DirectResultField(value, contract.DestinationSliceField)
		if !ok {
			return false
		}
		value = field.Type()
	}
	slice, ok := types.Unalias(value).Underlying().(*types.Slice)
	if !ok {
		return false
	}
	field, ok := ps6110DirectField(slice.Elem(), contract.DestinationStringField)
	return ok && types.Identical(types.Unalias(field.Type()), types.Typ[types.String])
}

func ps6110DirectResultField(value types.Type, name string) (*types.Var, bool) {
	value = types.Unalias(value)
	if _, pointer := value.(*types.Pointer); pointer {
		return nil, false
	}
	return ps6110DirectField(value, name)
}

func ps6110DirectField(value types.Type, name string) (*types.Var, bool) {
	value = types.Unalias(value)
	if pointer, ok := value.(*types.Pointer); ok {
		value = types.Unalias(pointer.Elem())
	}
	structure, ok := value.Underlying().(*types.Struct)
	if !ok {
		return nil, false
	}
	for index := range structure.NumFields() {
		field := structure.Field(index)
		if field.Name() == name && !field.Embedded() {
			return field, true
		}
	}
	return nil, false
}

func ps6110RecordPointer(value types.Type) bool {
	pointer, ok := types.Unalias(value).(*types.Pointer)
	if !ok {
		return false
	}
	_, ok = types.Unalias(pointer.Elem()).Underlying().(*types.Struct)
	return ok
}

func ps6110NativeFields(value types.Type, contract *config.NativeSnapshotStringCopyContract) bool {
	pointer, ok := types.Unalias(value).(*types.Pointer)
	if !ok {
		return false
	}
	label, ok := ps6110DirectField(pointer.Elem(), contract.NativeStringField)
	if !ok || !ps6110NativeBytes(label.Type()) {
		return false
	}
	if contract.NativeLengthField == "" {
		return true
	}
	length, ok := ps6110DirectField(pointer.Elem(), contract.NativeLengthField)
	return ok && ps6110Integer(length.Type())
}

func ps6110NativeBytes(value types.Type) bool {
	value = types.Unalias(value)
	var element types.Type
	switch value := value.Underlying().(type) {
	case *types.Array:
		element = value.Elem()
	case *types.Slice:
		element = value.Elem()
	case *types.Pointer:
		element = value.Elem()
	default:
		return false
	}
	basic, ok := types.Unalias(element).Underlying().(*types.Basic)
	return ok && (basic.Kind() == types.Int8 || basic.Kind() == types.Uint8)
}

func ps6110Integer(value types.Type) bool {
	basic, ok := types.Unalias(value).Underlying().(*types.Basic)
	return ok && basic.Info()&types.IsInteger != 0
}

func ps6110IntegerResult(pass *analysis.Pass, call *ast.CallExpr, index int) bool {
	signature, ok := pass.TypesInfo.TypeOf(call.Fun).(*types.Signature)
	return ok && signature.Results() != nil && index >= 0 && index < signature.Results().Len() &&
		ps6110Integer(signature.Results().At(index).Type())
}

func ps6110CopySignature(pass *analysis.Pass, call *ast.CallExpr, contract *config.NativeSnapshotStringCopyContract) bool {
	signature, ok := pass.TypesInfo.TypeOf(call.Fun).(*types.Signature)
	if !ok || signature.Results() == nil {
		// cgo built-ins can lack an ordinary callable object in source form;
		// their exact selector/generated identity and result type still suffice.
		return (contract.CopyKind == config.NativeSnapshotCopyGoString || contract.CopyKind == config.NativeSnapshotCopyGoStringN) &&
			types.Identical(types.Unalias(pass.TypesInfo.TypeOf(call)), types.Typ[types.String])
	}
	index := contract.CopyStringResultPosition - 1
	return index >= 0 && index < signature.Results().Len() &&
		types.Identical(types.Unalias(signature.Results().At(index).Type()), types.Typ[types.String])
}

func ps6110CallMatches(pass *analysis.Pass, file *ast.File, call *ast.CallExpr, id string, kind config.NativeSnapshotCallKind) bool {
	if kind == config.NativeSnapshotCallCgo {
		name, ok := ps6110CgoName(pass, file, call)
		return ok && id == "C."+name
	}
	functionID := ps6087FunctionID(pass, call)
	if functionID != id {
		return false
	}
	if kind == config.NativeSnapshotCallMethod && ps6091MethodExpression(pass, call.Fun) {
		return false
	}
	_, signature, ok := typedCallee(pass, call.Fun)
	return ok && (signature.Recv() == nil && kind == config.NativeSnapshotCallFunction ||
		signature.Recv() != nil && kind == config.NativeSnapshotCallMethod)
}

func ps6110CgoName(pass *analysis.Pass, file *ast.File, call *ast.CallExpr) (string, bool) {
	switch function := ps2110Unparen(call.Fun).(type) {
	case *ast.Ident:
		if !ps6110SyntheticCgoObject(pass, function) {
			return "", false
		}
		return ps2004GeneratedCgoName(function.Name)
	case *ast.FuncLit:
		generated := ps6110GeneratedCgoCall(function.Body)
		if generated == nil {
			return "", false
		}
		identifier, ok := ps2110Unparen(generated.Fun).(*ast.Ident)
		if !ok {
			return "", false
		}
		if !ps6110SyntheticCgoObject(pass, identifier) {
			return "", false
		}
		return ps2004GeneratedCgoName(identifier.Name)
	case *ast.SelectorExpr:
		qualifier, ok := ps2110Unparen(function.X).(*ast.Ident)
		if !ok || qualifier.Name != "C" || !ps2110ImportsC(file) {
			return "", false
		}
		if packageName, ok := pass.TypesInfo.Uses[qualifier].(*types.PkgName); ok && packageName.Imported().Path() != "C" {
			return "", false
		}
		return function.Sel.Name, true
	}
	return "", false
}

func ps6110SyntheticCgoObject(pass *analysis.Pass, identifier *ast.Ident) bool {
	object := pass.TypesInfo.Uses[identifier]
	if object == nil {
		return false
	}
	function, ok := object.(*types.Func)
	if !ok {
		return false
	}
	filename := pass.Fset.PositionFor(function.Pos(), false).Filename
	return !strings.HasSuffix(filename, ".go") || strings.HasSuffix(filename, "_cgo_gotypes.go")
}

func ps6110GeneratedCgoCall(body *ast.BlockStmt) *ast.CallExpr {
	var generated *ast.CallExpr
	ast.Inspect(body, func(node ast.Node) bool {
		if generated != nil {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		identifier, ok := ps2110Unparen(call.Fun).(*ast.Ident)
		if !ok {
			return true
		}
		if _, cgo := ps2004GeneratedCgoName(identifier.Name); cgo {
			generated = call
			return false
		}
		return true
	})
	return generated
}

func ps6110ObjectExpression(pass *analysis.Pass, expression ast.Expr, object types.Object) bool {
	identifier, ok := ps2110Unparen(expression).(*ast.Ident)
	return ok && (pass.TypesInfo.Uses[identifier] == object || pass.TypesInfo.Defs[identifier] == object)
}

func ps6110IntegerConstant(pass *analysis.Pass, expression ast.Expr, want int64) bool {
	value := pass.TypesInfo.Types[ps2110Unparen(expression)].Value
	if value == nil || value.Kind() != constant.Int {
		return false
	}
	got, exact := constant.Int64Val(value)
	return exact && got == want
}

func ps6110BlockReturns(block *ast.BlockStmt) bool {
	if block == nil || len(block.List) == 0 {
		return false
	}
	_, ok := block.List[len(block.List)-1].(*ast.ReturnStmt)
	return ok
}
