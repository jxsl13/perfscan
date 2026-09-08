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

// PS6115 implements owner issue #875 as a configured screening advisory. Its
// representative geometry is evidence to revisit a route, never runtime proof.
var PS6115 = register(&lint.Check{
	ID:          "PS6115",
	Category:    "verify",
	Slug:        "tiny-synchronous-accelerator-screen",
	Level:       lint.LevelAggressive,
	AutoFix:     false,
	NeedsConfig: true,
	Vocab:       []string{"tinySynchronousAcceleratorScreenContracts"},
	Doc: lint.Documentation{
		Title: "a tiny synchronous accelerator boundary may merit same-semantics host screening",
		Text: `A fused accelerator operation can be numerically appropriate yet
spend most of a tiny boundary in submission and blocking completion. On a
unified or shared-memory target, an exact host implementation may be worth
screening when every input and output is already host-accessible and no
transfer, residency transition, graph, recorder, stream, or command-buffer
context is required.

PS6115 is deliberately config-gated and fail-closed. Each unique
tinySynchronousAcceleratorScreenContracts group contains one or two exact call
records, normally forward and gradient. A record binds an exact non-generic
function site, concrete accelerator callable, one-based row and column roles,
an optional exact typed operation constant, and an exact typed host
alternative. The group supplies positive representative rows, columns and
working-set bytes, one or both inclusive limits, a submission count equal to
the records, exact GOOS/GOARCH, and a unified/shared memory model. Both limits
must pass when both are configured, and rows*columns is checked in positive
int64 arithmetic.

The configured geometry is representative evidence, not runtime proof. Source
operands must be positive matching constants or direct stable integer roots,
optionally through a lossless integer conversion. A local root needs one
dominating initialized definition and may not be rebound, incremented,
addressed, captured, or opaquely passed. Every grouped record and host
alternative must resolve. Calls must be direct and statically resolved; V1
rejects interfaces, promoted methods, method expressions, function values,
wrappers or closures at the site, go/defer calls, generics, variadics, duplicate
matches, and source-proven unreachable calls. Real cgo C.foo selectors are
supported only when the file truly imports C; C-shaped Go values are rejected.

Configuration must explicitly affirm blocking completion, host-accessible
inputs and output, no transfer or pre/post device residency, no graph,
recorder, or command-buffer context, exact dtype/layout/attributes/reduction
coverage, and forward, gradient, floating-point, error, panic, mutation,
alias, ownership, recorder, autograd, and backend-selection parity. It must
also require paired end-to-end validation. Source-visible transfers, explicit
device buffers/tensors, graphs, recorders, streams, command buffers, and later
device consumers suppress the group. A contract marked with an existing
measured host selector or an intentionally retained accelerator route remains
silent; ordinary //perfscan:ignore PS6115 annotations remain available for
reviewed sites.

There is NO automatic fix. The diagnostic asks only for an exact
same-semantics host screen followed by paired application validation. It does
not say the host is faster, that the application wins, or that routing should
change. Preserve the accelerator route until the configured current-parent
correctness and end-to-end gates pass.`,
		Before: `loss, err := accelerator.CrossEntropy(ctx, rows, columns, logits, labels)
gradient, err := accelerator.CrossEntropyGradient(ctx, rows, columns, logits, labels)`,
		After: `// Candidate only: benchmark the exact typed host alternatives with
// identical semantics, then retain a route only after paired end-to-end gates.`,
		MeasuredWin: `GoAI PR #1198 recorded the rejected B=8, C=10
CrossEntropy experiment. At isolated boundary scope, avoiding two synchronous
Metal submissions was about 120x faster. The stronger same-binary benchmark
alternated direct Metal and host routing inside every iteration and measured
seven full-step speedups of 1.049x, 1.022x, 1.075x, 1.010x, 1.067x, 1.026x,
and 1.041x: median about 1.041x. That failed both frozen promotion gates
(1.05x median and 1.03x every pair), so selector commit a0e7a529 was removed
by 921abe97 and rejected in fc44e371. This boundary evidence establishes no
application win or routing conclusion; use paired end-to-end validation from
the first screen.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6115",
		Doc:  "configured tiny synchronous accelerator boundaries that merit same-semantics host screening",
		Run:  runPS6115,
	},
})

type ps6115Match struct {
	record      *config.TinySynchronousAcceleratorScreenCall
	declaration *ast.FuncDecl
	file        *ast.File
	call        *ast.CallExpr
	rowsRoot    types.Object
	columnsRoot types.Object
}

func runPS6115(pass *analysis.Pass) (any, error) {
	return runPS6115WithContracts(pass, config.Current().TinySynchronousAcceleratorScreenContracts)
}

func runPS6115WithContracts(pass *analysis.Pass, configured []config.TinySynchronousAcceleratorScreenContract) (any, error) {
	declarations := ps6115Declarations(pass)
	for _, contract := range ps6115Contracts(configured) {
		if contract.ExistingMeasuredHostSelector || contract.IntentionalRetainedAcceleratorRoute {
			continue
		}
		matches := make([]ps6115Match, 0, len(contract.Calls))
		complete := true
		for index := range contract.Calls {
			record := &contract.Calls[index]
			declaration := declarations[record.ConfiguredSite]
			if declaration == nil || ps6113GenericFunction(pass, declaration) || !ps6115CallableResolves(pass, record.HostAlternativeCallable) {
				complete = false
				break
			}
			file := ps6115FileContaining(pass, declaration)
			match, ok := ps6115UniqueCall(pass, file, declaration, record, contract)
			if !ok {
				complete = false
				break
			}
			matches = append(matches, match)
		}
		if !complete || len(matches) != len(contract.Calls) || ps6115UnsafeContext(pass, matches) {
			continue
		}
		elements := contract.ConfiguredRows * contract.ConfiguredColumns
		related := make([]analysis.RelatedInformation, 0, len(matches))
		for _, match := range matches {
			related = append(related, analysis.RelatedInformation{
				Pos: match.call.Pos(), End: match.call.End(),
				Message: "configured direct accelerator submission " + match.record.AcceleratorCallable + " with exact host alternative " + match.record.HostAlternativeCallable,
			})
		}
		first := matches[0].call
		pass.Report(analysis.Diagnostic{
			Pos: first.Pos(), End: first.End(),
			Message: contract.Name + ": configured tiny synchronous accelerator screen is " + strconv.FormatInt(contract.ConfiguredRows, 10) + "x" + strconv.FormatInt(contract.ConfiguredColumns, 10) + "=" + strconv.FormatInt(elements, 10) + " elements, " + strconv.FormatInt(contract.ConfiguredWorkingSetBytes, 10) + " bytes, with submission count " + strconv.Itoa(contract.ConfiguredSubmissionCount) + " on " + contract.TargetGOOS + "/" + contract.TargetGOARCH + " " + contract.MemoryModel + " memory; screen only the exact same-semantics host alternatives and require paired end-to-end application validation—this is neither a host/application win nor a routing conclusion (PS6115 advisory, no automatic fix)",
			Related: related,
		})
	}
	return nil, nil
}

func ps6115Contracts(configured []config.TinySynchronousAcceleratorScreenContract) []*config.TinySynchronousAcceleratorScreenContract {
	nameCount := make(map[string]int)
	groupCount := make(map[string]int)
	claimCount := make(map[string]int)
	for index := range configured {
		contract := &configured[index]
		if !contract.Valid() {
			continue
		}
		nameCount[contract.Name]++
		groupCount[ps6115ContractKey(contract)]++
		for _, call := range contract.Calls {
			claimCount[ps6115CallKey(call)]++
		}
	}
	var result []*config.TinySynchronousAcceleratorScreenContract
	for index := range configured {
		contract := &configured[index]
		if !contract.Valid() || nameCount[contract.Name] != 1 || groupCount[ps6115ContractKey(contract)] != 1 {
			continue
		}
		unique := true
		for _, call := range contract.Calls {
			if claimCount[ps6115CallKey(call)] != 1 {
				unique = false
			}
		}
		if unique {
			result = append(result, contract)
		}
	}
	slices.SortFunc(result, func(left, right *config.TinySynchronousAcceleratorScreenContract) int {
		return strings.Compare(left.Name, right.Name)
	})
	return result
}

func ps6115ContractKey(contract *config.TinySynchronousAcceleratorScreenContract) string {
	parts := make([]string, 0, len(contract.Calls))
	for _, call := range contract.Calls {
		parts = append(parts, ps6115CallKey(call))
	}
	slices.Sort(parts)
	return strings.Join(parts, "\x01")
}

func ps6115CallKey(call config.TinySynchronousAcceleratorScreenCall) string {
	return call.ConfiguredSite + "\x00" + call.AcceleratorCallable + "\x00" + call.OperationConstant
}

func ps6115Declarations(pass *analysis.Pass) map[string]*ast.FuncDecl {
	result := make(map[string]*ast.FuncDecl)
	for _, file := range pass.Files {
		for _, node := range file.Decls {
			declaration, ok := node.(*ast.FuncDecl)
			if !ok || declaration.Body == nil {
				continue
			}
			id := ps6113DeclarationID(pass, declaration)
			if id == "" || result[id] != nil {
				result[id] = nil
				continue
			}
			result[id] = declaration
		}
	}
	return result
}

func ps6115FileContaining(pass *analysis.Pass, declaration *ast.FuncDecl) *ast.File {
	for _, file := range pass.Files {
		if declaration.Pos() >= file.Pos() && declaration.End() <= file.End() {
			return file
		}
	}
	return nil
}

func ps6115UniqueCall(pass *analysis.Pass, file *ast.File, declaration *ast.FuncDecl,
	record *config.TinySynchronousAcceleratorScreenCall, contract *config.TinySynchronousAcceleratorScreenContract,
) (ps6115Match, bool) {
	if file == nil {
		return ps6115Match{}, false
	}
	parents := ps6087Parents(declaration.Body)
	unreachable := ps2144Unreachable(pass, declaration.Body)
	var matches []ps6115Match
	ast.Inspect(declaration.Body, func(node ast.Node) bool {
		if _, closure := node.(*ast.FuncLit); closure {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok || ps2144PositionIn(call.Pos(), unreachable) || !ps6115DirectCall(pass, file, call, record.AcceleratorCallable) ||
			!ps6115DirectResultContext(call, parents) || ps6115AsyncCall(call, parents) {
			return true
		}
		rowsIndex := record.RowsArgument - 1
		columnsIndex := record.ColumnsArgument - 1
		if rowsIndex < 0 || columnsIndex < 0 || rowsIndex >= len(call.Args) || columnsIndex >= len(call.Args) {
			return true
		}
		rowsRoot, rowsOK := ps6115StableIntegerOperand(pass, file, declaration, call.Args[rowsIndex], call, parents, contract, contract.ConfiguredRows)
		columnsRoot, columnsOK := ps6115StableIntegerOperand(pass, file, declaration, call.Args[columnsIndex], call, parents, contract, contract.ConfiguredColumns)
		if !rowsOK || !columnsOK || !ps6115Operation(pass, call, record) {
			return true
		}
		matches = append(matches, ps6115Match{record: record, declaration: declaration, file: file, call: call, rowsRoot: rowsRoot, columnsRoot: columnsRoot})
		return true
	})
	if len(matches) != 1 {
		return ps6115Match{}, false
	}
	return matches[0], true
}

// ps6115DirectResultContext rejects calls embedded in wrappers or any other
// expression. Parentheses may surround the call, but its result must flow
// directly to a statement-level assignment, declaration, return, or discard.
func ps6115DirectResultContext(call *ast.CallExpr, parents map[ast.Node]ast.Node) bool {
	for current := ast.Node(call); current != nil; {
		parent := parents[current]
		switch value := parent.(type) {
		case *ast.ParenExpr:
			current = value
		case *ast.AssignStmt, *ast.ValueSpec, *ast.ReturnStmt, *ast.ExprStmt:
			return true
		default:
			return false
		}
	}
	return false
}

func ps6115DirectCall(pass *analysis.Pass, file *ast.File, call *ast.CallExpr, id string) bool {
	if call == nil || call.Ellipsis.IsValid() {
		return false
	}
	if strings.HasPrefix(id, "C.") {
		name, ok := ps6110CgoName(pass, file, call)
		return ok && id == "C."+name && !ps6115ExplicitInstantiation(call.Fun)
	}
	function, signature, ok := typedCallee(pass, call.Fun)
	if !ok || function == nil || signature == nil || signature.Variadic() || signature.TypeParams().Len() != 0 ||
		signature.RecvTypeParams().Len() != 0 || ps6115ExplicitInstantiation(call.Fun) || ps6087FunctionID(pass, call) != id {
		return false
	}
	if signature.Recv() == nil {
		switch functionExpression := ps2110Unparen(call.Fun).(type) {
		case *ast.Ident:
			return pass.TypesInfo.Uses[functionExpression] == function
		case *ast.SelectorExpr:
			return pass.TypesInfo.Selections[functionExpression] == nil && pass.TypesInfo.Uses[functionExpression.Sel] == function
		default:
			return false
		}
	}
	selector, ok := ps2110Unparen(call.Fun).(*ast.SelectorExpr)
	if !ok || ps6091MethodExpression(pass, call.Fun) {
		return false
	}
	selection := pass.TypesInfo.Selections[selector]
	if selection == nil || selection.Kind() != types.MethodVal || len(selection.Index()) != 1 || selection.Obj() != function {
		return false
	}
	_, interfaceReceiver := types.Unalias(selection.Recv()).Underlying().(*types.Interface)
	return !interfaceReceiver
}

func ps6115ExplicitInstantiation(expression ast.Expr) bool {
	switch ps2110Unparen(expression).(type) {
	case *ast.IndexExpr, *ast.IndexListExpr:
		return true
	}
	return false
}

func ps6115AsyncCall(call *ast.CallExpr, parents map[ast.Node]ast.Node) bool {
	for current := ast.Node(call); current != nil; current = parents[current] {
		switch parents[current].(type) {
		case *ast.GoStmt, *ast.DeferStmt, *ast.FuncLit:
			return true
		}
	}
	return false
}

func ps6115Operation(pass *analysis.Pass, call *ast.CallExpr, record *config.TinySynchronousAcceleratorScreenCall) bool {
	if record.OperationConstant == "" {
		return true
	}
	index := record.OperationArgument - 1
	if index < 0 || index >= len(call.Args) {
		return false
	}
	expectedType := pass.TypesInfo.TypeOf(call.Args[index])
	if function, signature, ok := typedCallee(pass, call.Fun); ok && function != nil && signature != nil && index < signature.Params().Len() {
		expectedType = signature.Params().At(index).Type()
	}
	return expectedType != nil && ps6113AddConstant(pass, call.Args[index], record.OperationConstant, record.OperationConstantValue, expectedType)
}

func ps6115StableIntegerOperand(pass *analysis.Pass, file *ast.File, declaration *ast.FuncDecl, expression ast.Expr,
	matchedCall *ast.CallExpr, parents map[ast.Node]ast.Node, contract *config.TinySynchronousAcceleratorScreenContract, configured int64,
) (types.Object, bool) {
	expression = ps2110Unparen(expression)
	if value := pass.TypesInfo.Types[expression].Value; value != nil {
		integer, exact := constant.Int64Val(value)
		return nil, value.Kind() == constant.Int && exact && integer > 0 && integer == configured
	}
	rootExpression := expression
	if conversion, ok := expression.(*ast.CallExpr); ok {
		if len(conversion.Args) != 1 || conversion.Ellipsis.IsValid() || !ps6115LosslessIntegerConversion(pass, conversion) {
			return nil, false
		}
		rootExpression = ps2110Unparen(conversion.Args[0])
	}
	identifier, ok := rootExpression.(*ast.Ident)
	if !ok {
		return nil, false
	}
	object := pass.TypesInfo.ObjectOf(identifier)
	if object == nil || !ps6106Integer(object.Type()) || !ps6115StableRoot(pass, file, declaration, object, matchedCall, expression, parents, contract) {
		return nil, false
	}
	return object, true
}

func ps6115LosslessIntegerConversion(pass *analysis.Pass, call *ast.CallExpr) bool {
	var object types.Object
	switch function := ps2110Unparen(call.Fun).(type) {
	case *ast.Ident:
		object = pass.TypesInfo.ObjectOf(function)
	case *ast.SelectorExpr:
		object = pass.TypesInfo.ObjectOf(function.Sel)
	}
	if _, ok := object.(*types.TypeName); !ok {
		return false
	}
	from := pass.TypesInfo.TypeOf(call.Args[0])
	to := pass.TypesInfo.TypeOf(call)
	fromSigned, fromBits, fromOK := ps6115IntegerRange(pass, from)
	toSigned, toBits, toOK := ps6115IntegerRange(pass, to)
	if !fromOK || !toOK {
		return false
	}
	if fromSigned && !toSigned {
		return false
	}
	if !fromSigned && toSigned {
		return toBits > fromBits
	}
	return toBits >= fromBits
}

func ps6115IntegerRange(pass *analysis.Pass, value types.Type) (signed bool, bits int64, ok bool) {
	basic, ok := types.Unalias(value).Underlying().(*types.Basic)
	if !ok || basic.Info()&types.IsInteger == 0 || basic.Info()&types.IsUntyped != 0 || pass.TypesSizes == nil {
		return false, 0, false
	}
	signed = basic.Kind() != types.Uint && basic.Kind() != types.Uint8 && basic.Kind() != types.Uint16 &&
		basic.Kind() != types.Uint32 && basic.Kind() != types.Uint64 && basic.Kind() != types.Uintptr
	return signed, pass.TypesSizes.Sizeof(value) * 8, true
}

func ps6115StableRoot(pass *analysis.Pass, file *ast.File, declaration *ast.FuncDecl, object types.Object, matchedCall *ast.CallExpr,
	allowedOperand ast.Expr, parents map[ast.Node]ast.Node, contract *config.TinySynchronousAcceleratorScreenContract,
) bool {
	variable, ok := object.(*types.Var)
	if !ok || variable.Pkg() != nil && variable.Parent() == variable.Pkg().Scope() {
		return false
	}
	definition := ps6115InitializedDefinition(pass, declaration, object, parents)
	if !ps6115Parameter(pass, declaration, object) && definition == nil || definition != nil && !ps6115Dominates(definition, matchedCall, parents) {
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
			if pass.TypesInfo.ObjectOf(value) != object {
				return true
			}
			if ps6113InsideClosure(value, parents) || ps6115OpaqueArgument(pass, file, value, matchedCall, allowedOperand, parents, contract) {
				valid = false
				return false
			}
		}
		return true
	})
	return valid
}

func ps6115Parameter(pass *analysis.Pass, declaration *ast.FuncDecl, object types.Object) bool {
	if declaration.Type.Params == nil {
		return false
	}
	for _, field := range declaration.Type.Params.List {
		for _, name := range field.Names {
			if pass.TypesInfo.Defs[name] == object {
				return true
			}
		}
	}
	return false
}

func ps6115InitializedDefinition(pass *analysis.Pass, declaration *ast.FuncDecl, object types.Object, parents map[ast.Node]ast.Node) ast.Node {
	var definition ast.Node
	count := 0
	ast.Inspect(declaration.Body, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if !ok || pass.TypesInfo.Defs[identifier] != object {
			return true
		}
		count++
		for current := ast.Node(identifier); current != nil; current = parents[current] {
			switch parent := parents[current].(type) {
			case *ast.AssignStmt:
				if len(parent.Rhs) > 0 {
					definition = parent
				}
				return false
			case *ast.ValueSpec:
				if len(parent.Values) > 0 {
					definition = parent
				}
				return false
			case *ast.BlockStmt:
				return false
			}
		}
		return false
	})
	if count != 1 {
		return nil
	}
	return definition
}

func ps6115Dominates(definition ast.Node, call *ast.CallExpr, parents map[ast.Node]ast.Node) bool {
	if definition == nil || definition.Pos() >= call.Pos() {
		return false
	}
	for blockNode := ast.Node(definition); blockNode != nil; blockNode = parents[blockNode] {
		block, ok := parents[blockNode].(*ast.BlockStmt)
		if !ok {
			continue
		}
		definitionStatement := blockNode
		callStatement := ast.Node(call)
		for callStatement != nil && parents[callStatement] != block {
			callStatement = parents[callStatement]
		}
		if callStatement == nil {
			continue
		}
		for index, statement := range block.List {
			if statement != definitionStatement {
				continue
			}
			for later := index + 1; later < len(block.List); later++ {
				if block.List[later] == callStatement {
					return true
				}
			}
			return false
		}
	}
	return false
}

func ps6115OpaqueArgument(pass *analysis.Pass, file *ast.File, identifier *ast.Ident, matchedCall *ast.CallExpr,
	allowedOperand ast.Expr, parents map[ast.Node]ast.Node, contract *config.TinySynchronousAcceleratorScreenContract,
) bool {
	if ps6099NodeWithin(identifier, allowedOperand) {
		return false
	}
	for current := ast.Node(identifier); current != nil; current = parents[current] {
		call, ok := parents[current].(*ast.CallExpr)
		if !ok {
			continue
		}
		if call == matchedCall && ps6099NodeWithin(identifier, allowedOperand) {
			return false
		}
		if len(call.Args) == 1 && ps6099NodeWithin(identifier, call.Args[0]) && ps6115LosslessIntegerConversion(pass, call) {
			continue
		}
		for index := range contract.Calls {
			record := &contract.Calls[index]
			if !ps6115DirectCall(pass, file, call, record.AcceleratorCallable) {
				continue
			}
			for _, position := range []int{record.RowsArgument, record.ColumnsArgument} {
				argumentIndex := position - 1
				if argumentIndex >= 0 && argumentIndex < len(call.Args) && ps6099NodeWithin(identifier, call.Args[argumentIndex]) {
					return false
				}
			}
		}
		for _, argument := range call.Args {
			if ps6099NodeWithin(identifier, argument) {
				return true
			}
		}
	}
	return false
}

func ps6115CallableResolves(pass *analysis.Pass, id string) bool {
	packages := []*types.Package{pass.Pkg}
	seen := map[string]bool{pass.Pkg.Path(): true}
	for index := 0; index < len(packages); index++ {
		for _, imported := range packages[index].Imports() {
			if !seen[imported.Path()] {
				seen[imported.Path()] = true
				packages = append(packages, imported)
			}
		}
	}
	for _, pkg := range packages {
		prefix := pkg.Path() + "."
		if !strings.HasPrefix(id, prefix) {
			continue
		}
		suffix := strings.TrimPrefix(id, prefix)
		parts := strings.Split(suffix, ".")
		if len(parts) == 1 {
			function, ok := pkg.Scope().Lookup(parts[0]).(*types.Func)
			return ok && ps6115ConcreteSignature(function)
		}
		if len(parts) != 2 {
			return false
		}
		typeName, ok := pkg.Scope().Lookup(parts[0]).(*types.TypeName)
		if !ok {
			return false
		}
		named, namedOK := types.Unalias(typeName.Type()).(*types.Named)
		if !namedOK {
			return false
		}
		for index := 0; index < named.NumMethods(); index++ {
			method := named.Method(index)
			if method.Name() == parts[1] {
				return ps6115ConcreteSignature(method)
			}
		}
		return false
	}
	return false
}

func ps6115ConcreteSignature(function *types.Func) bool {
	if function == nil || function.Pkg() == nil {
		return false
	}
	signature, ok := function.Type().(*types.Signature)
	if !ok || signature.Variadic() || signature.TypeParams().Len() != 0 || signature.RecvTypeParams().Len() != 0 {
		return false
	}
	if signature.Recv() == nil {
		return true
	}
	_, isInterface := types.Unalias(signature.Recv().Type()).Underlying().(*types.Interface)
	return !isInterface
}

func ps6115UnsafeContext(pass *analysis.Pass, matches []ps6115Match) bool {
	matchedCalls := make(map[*ast.CallExpr]bool, len(matches))
	for _, match := range matches {
		matchedCalls[match.call] = true
		if ps6115ExplicitDeviceType(pass.TypesInfo.TypeOf(match.call)) || ps6115DeclarationContext(pass, match.declaration) {
			return true
		}
		for _, argument := range match.call.Args {
			if ps6115ExplicitDeviceType(pass.TypesInfo.TypeOf(argument)) {
				return true
			}
		}
	}
	for _, match := range matches {
		unreachable := ps2144Unreachable(pass, match.declaration.Body)
		parents := ps6087Parents(match.declaration.Body)
		outputs, trackable := ps6115CallOutputs(pass, match.declaration, match.call, parents)
		if !trackable || !ps6115TrackOutputAliases(pass, match.declaration, match.call, unreachable, outputs) {
			return true
		}
		unsafe := false
		ast.Inspect(match.declaration.Body, func(node ast.Node) bool {
			if unsafe {
				return false
			}
			call, ok := node.(*ast.CallExpr)
			if !ok || matchedCalls[call] || ps2144PositionIn(call.Pos(), unreachable) {
				return true
			}
			id := strings.ToLower(ps6087FunctionID(pass, call))
			if id == "" {
				if file := ps6115FileContaining(pass, match.declaration); file != nil {
					if name, cgo := ps6110CgoName(pass, file, call); cgo {
						id = "c." + strings.ToLower(name)
					}
				}
			}
			if ps6115TransferOrContextName(id) || call.Pos() > match.call.Pos() && ps6115DeviceConsumerName(id) && ps6115CallUsesAny(pass, call, outputs) {
				unsafe = true
				return false
			}
			return true
		})
		if unsafe {
			return true
		}
	}
	return false
}

func ps6115DeclarationContext(pass *analysis.Pass, declaration *ast.FuncDecl) bool {
	function, _ := pass.TypesInfo.Defs[declaration.Name].(*types.Func)
	if function == nil {
		return true
	}
	signature, _ := function.Type().(*types.Signature)
	if signature == nil {
		return true
	}
	if signature.Recv() != nil && ps6115ExplicitDeviceType(signature.Recv().Type()) {
		return true
	}
	for tupleIndex := 0; tupleIndex < 2; tupleIndex++ {
		tuple := signature.Params()
		if tupleIndex == 1 {
			tuple = signature.Results()
		}
		for index := 0; index < tuple.Len(); index++ {
			if ps6115ExplicitDeviceType(tuple.At(index).Type()) {
				return true
			}
		}
	}
	return false
}

func ps6115ExplicitDeviceType(value types.Type) bool {
	if value == nil {
		return false
	}
	name := strings.ToLower(types.TypeString(value, func(pkg *types.Package) string { return pkg.Path() }))
	for _, marker := range []string{"devicebuffer", "devicetensor", "commandbuffer", "command_buffer", "graph", "recorder", "stream"} {
		if strings.Contains(name, marker) {
			return true
		}
	}
	return false
}

func ps6115TransferOrContextName(id string) bool {
	for _, marker := range []string{"tohost", "to_host", "todevice", "to_device", "upload", "download", "transfer", "copytohost", "copy_to_host", "copytodevice", "copy_to_device", "commandbuffer", "command_buffer", ".newgraph", ".begingraph", ".newrecorder", ".newstream"} {
		if strings.Contains(id, marker) {
			return true
		}
	}
	return false
}

func ps6115DeviceConsumerName(id string) bool {
	for _, marker := range []string{"metal", "cuda", "gpu", "device", "accelerator", "graph", "recorder", "stream", "command"} {
		if strings.Contains(id, marker) {
			return true
		}
	}
	return false
}

func ps6115CallOutputs(pass *analysis.Pass, declaration *ast.FuncDecl, call *ast.CallExpr,
	parents map[ast.Node]ast.Node,
) (map[types.Object]bool, bool) {
	result := make(map[types.Object]bool)
	for current := ast.Node(call); current != nil; current = parents[current] {
		switch parent := parents[current].(type) {
		case *ast.ParenExpr:
			continue
		case *ast.AssignStmt:
			if !ps6115AddOutputStorage(pass, declaration, parent.Lhs, result) {
				return nil, false
			}
			return result, true
		case *ast.ValueSpec:
			names := make([]ast.Expr, len(parent.Names))
			for index, name := range parent.Names {
				names[index] = name
			}
			if !ps6115AddOutputStorage(pass, declaration, names, result) {
				return nil, false
			}
			return result, true
		case *ast.ReturnStmt, *ast.ExprStmt:
			return result, true
		default:
			return nil, false
		}
	}
	return nil, false
}

func ps6115AddOutputStorage(pass *analysis.Pass, declaration *ast.FuncDecl,
	expressions []ast.Expr, objects map[types.Object]bool,
) bool {
	for _, expression := range expressions {
		identifier, ok := ps2110Unparen(expression).(*ast.Ident)
		if !ok {
			return false
		}
		object := identObject(pass, identifier)
		if object == nil {
			if identifier.Name != "_" {
				return false
			}
			continue
		}
		variable, variableOK := object.(*types.Var)
		if !variableOK || variable.Pkg() != nil && variable.Parent() == variable.Pkg().Scope() ||
			object.Pos() < declaration.Pos() || object.Pos() > declaration.End() {
			return false
		}
		objects[object] = true
	}
	return true
}

// ps6115TrackOutputAliases performs a source-ordered, function-local alias
// closure. Go declarations cannot use a later local, so one bounded pass is
// sufficient. Storing a tracked result in selector/index/pointer storage is
// intentionally untrackable and suppresses the advisory.
func ps6115TrackOutputAliases(pass *analysis.Pass, declaration *ast.FuncDecl, call *ast.CallExpr,
	unreachable []tokenSpan, objects map[types.Object]bool,
) bool {
	trackable := true
	ast.Inspect(declaration.Body, func(node ast.Node) bool {
		if !trackable || node == nil {
			return trackable
		}
		if node.Pos() <= call.Pos() {
			return true
		}
		if ps2144PositionIn(node.Pos(), unreachable) {
			return false
		}
		switch value := node.(type) {
		case *ast.AssignStmt:
			if ps6115ExpressionsUseAny(pass, value.Rhs, objects) &&
				!ps6115AddOutputStorage(pass, declaration, value.Lhs, objects) {
				trackable = false
				return false
			}
		case *ast.ValueSpec:
			if ps6115ExpressionsUseAny(pass, value.Values, objects) {
				names := make([]ast.Expr, len(value.Names))
				for index, name := range value.Names {
					names[index] = name
				}
				if !ps6115AddOutputStorage(pass, declaration, names, objects) {
					trackable = false
					return false
				}
			}
		}
		return true
	})
	return trackable
}

func ps6115ExpressionsUseAny(pass *analysis.Pass, expressions []ast.Expr, objects map[types.Object]bool) bool {
	for _, expression := range expressions {
		used := false
		ast.Inspect(expression, func(node ast.Node) bool {
			identifier, ok := node.(*ast.Ident)
			if ok && objects[pass.TypesInfo.ObjectOf(identifier)] {
				used = true
				return false
			}
			return !used
		})
		if used {
			return true
		}
	}
	return false
}

func ps6115CallUsesAny(pass *analysis.Pass, call *ast.CallExpr, objects map[types.Object]bool) bool {
	return ps6115ExpressionsUseAny(pass, []ast.Expr{call}, objects)
}
