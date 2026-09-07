package checks

import (
	"go/ast"
	"go/token"
	"go/types"
	"strconv"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/lint"
)

// PS6107 implements owner issue #893. Ownership, synchronization, lifetime,
// and retention policy are not encoded by Go types, so the source matcher is
// enabled only by a complete project-owned contract.
var PS6107 = register(&lint.Check{
	ID:          "PS6107",
	Category:    "alloc",
	Slug:        "receiver-high-water-staging",
	Level:       lint.LevelAggressive,
	AutoFix:     false,
	NeedsConfig: true,
	Vocab:       []string{"receiverStagingContracts"},
	Doc: lint.Documentation{
		Title: "fully overwritten per-call staging has a contract-safe receiver high-water lifetime",
		Text: `A hot sequential receiver method can allocate input-sized host staging on
every call, completely populate it, pass it to a synchronous non-retaining
consumer, and discard it. A receiver-owned exact high-water slice can remove
that allocation, but only when the receiver's concurrency, the overwriter and
consumer ownership behavior, retained-data policy, size bound, and lifecycle
are explicit project contracts.

PS6107 therefore has no name heuristics. It requires a complete
receiverStagingContracts entry naming the exact typed candidate, optional row
overwriter, consumer, and lifecycle methods. The contract must affirm
sequential receiver calls; synchronous, non-retaining overwrite and consume;
row-overwriter access limited to the passed length with no capacity or identity
dependence and no mutation of extent inputs; safe content retention;
nonnegative nonoverflowing tiled extents;
and a positive retained-byte ceiling no larger than the analyzer's conservative
64 MiB initial policy. A Release or Close spelling alone means nothing.

The source match is deliberately small: a direct builtin make([]T,
inputDerivedExtent) whose extent uses only direct integer arithmetic,
conversions, fields, parameters, and builtin len in the configured pointer
method; immediately followed by
either one canonical unconditional whole-slice assignment loop or one exact
row-partition loop over a contiguous array or slice calling the configured
overwriter; immediately followed by one direct typed consumer. Captured extent
snapshots must remain equal to the current range and row width through the
overwrite. Mutable package globals are not accepted as extent roots, and a
captured snapshot-to-overwrite prefix may contain no opaque call that could
invalidate it. Receiver and aggregate roots exposed earlier also stay silent.
The local slice may have no other use. Reads before
the overwrite, compound/partial/conditional writes, aliases, address exposure,
stores, returns, sends, append, closures, go/defer, multiple consumers, later
uses, mutable or exposed loop indices and extent roots, arbitrary size calls,
string/map row ranges, pointer-bearing elements, independent capacity,
unreachable candidates, and non-straight-line control flow stay silent.

There is NO automatic fix. Evaluate a receiver field that grows to exactly the
required length, reuses field[:need:need] for smaller calls, falls back to the
original per-call allocation above the configured cap, and is set to nil at the
configured lifecycle boundary. Checked size arithmetic must preserve the
original extent's evaluation order and panic/error behavior. The full slice
expression preserves the fresh allocation's len/cap boundary; equivalence must
also cover zero-size nil/non-nil behavior, panics, errors, partial writes, and
failed consumers, not only successful output. Retained contents may survive
until lifecycle end, so sensitive data needing eager clear is ineligible.

Allocation removal is not a wall-time claim. Require a same-work,
order-alternating before/after benchmark with allocation counts, realistic
growing/shrinking/oversize calls, and end-to-end throughput before promotion.`,
		Before: `func (d *Decoder) prefill(tokens []int) error {
	k := len(tokens)
	host := make([]float32, k*d.dim)
	for row, token := range tokens {
		d.gatherInto(host[row*d.dim:(row+1)*d.dim], token)
	}
	return d.device.UploadF32(host)
}`,
		After: `func (d *Decoder) prefill(tokens []int) error {
	need := len(tokens) * d.dim // evaluate the original extent exactly once
	if need <= 0 || need > configuredElementLimit {
		host := make([]float32, need) // preserves zero non-nil and invalid-size panic
		return d.fillAndUpload(host, tokens)
	}
	if cap(d.host) < need {
		d.host = make([]float32, need) // exact growth
	}
	host := d.host[:need:need]
	return d.fillAndUpload(host, tokens)
}

func (d *Decoder) Release() { d.host = nil }`,
		MeasuredWin: `Owner issue #893 measured two GoAI boundaries on Apple
M2 Pro. Shared Decoder pp16/Dim512 removed exactly 32,768 staging bytes per
call: StepNLast moved from 36,864 B/op and 2 allocs/op to 4,096 B/op and 1
alloc/op, while its Into form reached 0 allocs/op. Seven paired campaigns were
near-neutral (median 1.001x, range 0.9725-1.002x). GPT-2-small pp16/Dim768
attributed 49,152 bytes to the batch staging slice; the final Into form reached
0 B/op and 0 allocs/op, with reported paired medians 1.000x decode and 1.027x
prefill. These are allocation wins, not a broad speedup claim; PS2004 documents
other receiver-owned scratch variants that regressed 25-34%.

The repository's isolated same-work mechanism benchmark on Apple M2 Pro used
six alternating fresh-process pairs. Warmed steady-state 4/16/8/16-row reuse
moved from 22,527-22,528 B/op and 1 alloc/op to 0 B/op and 0 allocs/op in every
arm; medians moved from 7,372 to 6,537 ns/op (-11.3%). The oversize fallback
retained exactly 73,728 B/op and 1 alloc/op in every arm and moved from a
21,513.5 to 21,855 ns/op median (+1.59%). These local mechanism timings include
neither growth nor application work and are not a universal throughput claim.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6107",
		Doc:  "fully overwritten per-call staging has a contract-safe receiver high-water lifetime",
		Run:  runPS6107,
	},
})

type ps6107Method struct {
	declaration *ast.FuncDecl
	object      *types.Func
	signature   *types.Signature
	named       *types.Named
	pointer     bool
}

type ps6107Allocation struct {
	call      *ast.CallExpr
	object    types.Object
	name      *ast.Ident
	size      ast.Expr
	elem      types.Type
	elemBytes int64
	body      *ast.BlockStmt
}

func runPS6107(pass *analysis.Pass) (any, error) {
	return runPS6107WithContracts(pass, config.Current().ReceiverStagingContracts)
}

func runPS6107WithContracts(pass *analysis.Pass, contracts []config.ReceiverStagingContract) (any, error) {
	if len(contracts) == 0 {
		return nil, nil
	}
	methods := ps6107Methods(pass)
	for index := range contracts {
		contract := &contracts[index]
		if !contract.Valid() {
			continue
		}
		candidate, candidateOK := methods[contract.CandidateMethod]
		lifecycle, lifecycleOK := methods[contract.LifecycleMethod]
		if !candidateOK || !lifecycleOK || !candidate.pointer || !lifecycle.pointer ||
			candidate.named == nil || lifecycle.named == nil ||
			candidate.named.Obj() != lifecycle.named.Obj() ||
			lifecycle.signature.Params().Len() != 0 || lifecycle.signature.Results().Len() != 0 {
			continue
		}
		ps6107ConfiguredMethod(pass, candidate, contract)
	}
	return nil, nil
}

func ps6107Methods(pass *analysis.Pass) map[string]ps6107Method {
	methods := make(map[string]ps6107Method)
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil || function.Recv == nil {
				continue
			}
			object, ok := pass.TypesInfo.Defs[function.Name].(*types.Func)
			if !ok {
				continue
			}
			signature, ok := object.Type().(*types.Signature)
			if !ok || signature.Recv() == nil {
				continue
			}
			receiver := types.Unalias(signature.Recv().Type())
			pointer := false
			if value, ok := receiver.(*types.Pointer); ok {
				pointer = true
				receiver = types.Unalias(value.Elem())
			}
			named, _ := receiver.(*types.Named)
			methods[ps6090FunctionID(object)] = ps6107Method{
				declaration: function,
				object:      object,
				signature:   signature,
				named:       named,
				pointer:     pointer,
			}
		}
	}
	return methods
}

func ps6107ConfiguredMethod(pass *analysis.Pass, method ps6107Method, contract *config.ReceiverStagingContract) {
	body := method.declaration.Body
	params := ps2140ParamObjs(pass, method.declaration)
	aliases := ps6107Aliases(pass, body)
	unreachable := ps2144Unreachable(pass, body)
	for index, statement := range body.List {
		if ps2144PositionIn(statement.Pos(), unreachable) {
			continue
		}
		allocation, ok := ps6107AllocationAt(pass, body, statement, params, aliases)
		if !ok || index+2 >= len(body.List) {
			continue
		}
		if contract.MaxRetainedBytes < allocation.elemBytes {
			continue
		}
		allowed := map[*ast.Ident]bool{allocation.name: true}
		overwriteKind := "inline whole-slice overwrite"
		if contract.Overwrite == "" {
			if !ps6107InlineOverwrite(pass, body.List[index+1], allocation.object, allowed) {
				continue
			}
		} else {
			if !ps6107PartitionOverwrite(pass, body.List[index+1], allocation, aliases, contract, allowed) {
				continue
			}
			overwriteKind = "contract-proven exact row-partition overwrite"
		}
		consumer, ok := ps6107Consumer(pass, body.List[index+2], allocation.object, contract, allowed)
		if !ok || !ps6107OnlyAllowedUses(pass, body, allocation.object, allowed) {
			continue
		}
		pass.Report(analysis.Diagnostic{
			Pos: allocation.call.Pos(),
			End: allocation.call.End(),
			Message: contract.Name + ": configured sequential method " + contract.CandidateMethod +
				" allocates []" + ps2140ElemName(allocation.elem) + " staging with runtime extent " +
				exprTextRendered(allocation.size) + " (" + strconv.FormatInt(allocation.elemBytes, 10) +
				" bytes/element), then performs a " + overwriteKind +
				" and passes it once to configured synchronous non-retaining " + ps6090FunctionID(consumer) +
				"; evaluate receiver-owned exact high-water reuse with full slice expression [:need:need], " +
				"checked nonoverflowing size arithmetic, per-call fallback above " +
				strconv.FormatInt(contract.MaxRetainedBytes, 10) + " bytes, and clearing in " + contract.LifecycleMethod +
				"; preserve zero-size nilness and panic/error/partial-write behavior, and require alternating " +
				"allocation plus end-to-end throughput evidence (advisory, no automatic fix)",
		})
	}
}

func ps6107AllocationAt(pass *analysis.Pass, body *ast.BlockStmt, statement ast.Stmt, params map[types.Object]bool, aliases map[types.Object]ast.Expr) (ps6107Allocation, bool) {
	assignment, ok := statement.(*ast.AssignStmt)
	if !ok || assignment.Tok != token.DEFINE || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
		return ps6107Allocation{}, false
	}
	name, ok := ps2110Unparen(assignment.Lhs[0]).(*ast.Ident)
	if !ok || name.Name == "_" {
		return ps6107Allocation{}, false
	}
	call, ok := ps2110Unparen(assignment.Rhs[0]).(*ast.CallExpr)
	if !ok || !ps2140IsMake(pass, call) || len(call.Args) != 2 || call.Ellipsis.IsValid() {
		return ps6107Allocation{}, false
	}
	slice, ok := types.Unalias(pass.TypesInfo.TypeOf(call)).Underlying().(*types.Slice)
	if !ok || !ps6107Scalar(slice.Elem()) ||
		!ps6107BoundedExtent(pass, call.Args[1], aliases, make(map[types.Object]bool)) ||
		!ps6107DependsOnParam(pass, call.Args[1], params, aliases, make(map[types.Object]bool)) ||
		ps6107ExtentAliasAddressed(pass, body, call.Args[1], aliases, make(map[types.Object]bool)) {
		return ps6107Allocation{}, false
	}
	elemBytes := pass.TypesSizes.Sizeof(slice.Elem())
	object := pass.TypesInfo.Defs[name]
	if object == nil || elemBytes <= 0 {
		return ps6107Allocation{}, false
	}
	return ps6107Allocation{call, object, name, call.Args[1], slice.Elem(), elemBytes, body}, true
}

// ps6107BoundedExtent accepts the intentionally small arithmetic grammar
// documented by PS6107. In particular, a parameter hidden behind an arbitrary
// call is not evidence that the returned size preserves evaluation or failure
// behavior. Type conversions and direct builtin len remain explicit.
func ps6107BoundedExtent(pass *analysis.Pass, expression ast.Expr, aliases map[types.Object]ast.Expr, visiting map[types.Object]bool) bool {
	expression = ps2110Unparen(expression)
	switch value := expression.(type) {
	case *ast.Ident:
		object := identObject(pass, value)
		if alias := aliases[object]; alias != nil && !visiting[object] {
			visiting[object] = true
			valid := ps6107BoundedExtent(pass, alias, aliases, visiting)
			delete(visiting, object)
			return valid
		}
		basic, ok := types.Unalias(pass.TypesInfo.TypeOf(value)).Underlying().(*types.Basic)
		return object != nil && !ps6107MutablePackageRoot(object) && ok && basic.Info()&types.IsInteger != 0
	case *ast.BasicLit:
		return value.Kind == token.INT
	case *ast.SelectorExpr:
		selection := pass.TypesInfo.Selections[value]
		basic, ok := types.Unalias(pass.TypesInfo.TypeOf(value)).Underlying().(*types.Basic)
		return selection != nil && selection.Kind() == types.FieldVal && ok && basic.Info()&types.IsInteger != 0 &&
			ps6107BoundedSelectorBase(pass, value.X, aliases, visiting)
	case *ast.BinaryExpr:
		switch value.Op {
		case token.ADD, token.SUB, token.MUL, token.QUO, token.REM,
			token.SHL, token.SHR, token.AND, token.OR, token.XOR, token.AND_NOT:
			return ps6107BoundedExtent(pass, value.X, aliases, visiting) && ps6107BoundedExtent(pass, value.Y, aliases, visiting)
		}
		return false
	case *ast.UnaryExpr:
		return (value.Op == token.ADD || value.Op == token.SUB || value.Op == token.XOR) &&
			ps6107BoundedExtent(pass, value.X, aliases, visiting)
	case *ast.CallExpr:
		if value.Ellipsis.IsValid() || len(value.Args) != 1 {
			return false
		}
		if typedBuiltinName(pass, value.Fun, "len") {
			return ps6107BoundedLenOperand(pass, value.Args[0], aliases, visiting)
		}
		return pass.TypesInfo.Types[ps2110Unparen(value.Fun)].IsType() &&
			ps6107BoundedExtent(pass, value.Args[0], aliases, visiting)
	}
	return false
}

func ps6107BoundedSelectorBase(pass *analysis.Pass, expression ast.Expr, aliases map[types.Object]ast.Expr, visiting map[types.Object]bool) bool {
	expression = ps2110Unparen(expression)
	switch value := expression.(type) {
	case *ast.Ident:
		object := identObject(pass, value)
		if alias := aliases[object]; alias != nil && !visiting[object] {
			visiting[object] = true
			valid := ps6107BoundedSelectorBase(pass, alias, aliases, visiting)
			delete(visiting, object)
			return valid
		}
		return object != nil && !ps6107MutablePackageRoot(object)
	case *ast.SelectorExpr:
		selection := pass.TypesInfo.Selections[value]
		return selection != nil && selection.Kind() == types.FieldVal &&
			ps6107BoundedSelectorBase(pass, value.X, aliases, visiting)
	}
	return false
}

func ps6107BoundedLenOperand(pass *analysis.Pass, expression ast.Expr, aliases map[types.Object]ast.Expr, visiting map[types.Object]bool) bool {
	expression = ps2110Unparen(expression)
	if identifier, ok := expression.(*ast.Ident); ok {
		object := identObject(pass, identifier)
		if alias := aliases[object]; alias != nil && !visiting[object] {
			visiting[object] = true
			valid := ps6107BoundedLenOperand(pass, alias, aliases, visiting)
			delete(visiting, object)
			return valid
		}
		return object != nil && !ps6107MutablePackageRoot(object)
	}
	selector, ok := expression.(*ast.SelectorExpr)
	return ok && pass.TypesInfo.Selections[selector] != nil && ps6107BoundedSelectorBase(pass, selector.X, aliases, visiting)
}

func ps6107MutablePackageRoot(object types.Object) bool {
	variable, ok := object.(*types.Var)
	return ok && variable.Pkg() != nil && variable.Parent() == variable.Pkg().Scope()
}

func ps6107Scalar(value types.Type) bool {
	basic, ok := types.Unalias(value).Underlying().(*types.Basic)
	return ok && basic.Info()&(types.IsInteger|types.IsFloat) != 0
}

func ps6107Aliases(pass *analysis.Pass, body *ast.BlockStmt) map[types.Object]ast.Expr {
	aliases := make(map[types.Object]ast.Expr)
	for _, statement := range body.List {
		assignment, ok := statement.(*ast.AssignStmt)
		if !ok || assignment.Tok != token.DEFINE || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
			continue
		}
		name, ok := ps2110Unparen(assignment.Lhs[0]).(*ast.Ident)
		if !ok {
			continue
		}
		object := pass.TypesInfo.Defs[name]
		if object != nil && ps6107AssignmentCount(pass, body, object) == 1 {
			aliases[object] = assignment.Rhs[0]
		}
	}
	return aliases
}

func ps6107AssignmentCount(pass *analysis.Pass, body *ast.BlockStmt, object types.Object) int {
	count := 0
	ast.Inspect(body, func(node ast.Node) bool {
		switch statement := node.(type) {
		case *ast.AssignStmt:
			for _, lhs := range statement.Lhs {
				if ps6107ContainsObject(pass, lhs, object) {
					count++
				}
			}
		case *ast.IncDecStmt:
			if ps6107ContainsObject(pass, statement.X, object) {
				count++
			}
		}
		return true
	})
	return count
}

func ps6107ExtentAliasAddressed(pass *analysis.Pass, body *ast.BlockStmt, expression ast.Expr, aliases map[types.Object]ast.Expr, visiting map[types.Object]bool) bool {
	unsafe := false
	ast.Inspect(expression, func(node ast.Node) bool {
		if unsafe {
			return false
		}
		identifier, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		object := identObject(pass, identifier)
		alias := aliases[object]
		if alias == nil || visiting[object] {
			return true
		}
		if ps6107Addressed(pass, body, object) {
			unsafe = true
			return false
		}
		visiting[object] = true
		unsafe = ps6107ExtentAliasAddressed(pass, body, alias, aliases, visiting)
		delete(visiting, object)
		return !unsafe
	})
	return unsafe
}

func ps6107Addressed(pass *analysis.Pass, body *ast.BlockStmt, object types.Object) bool {
	addressed := false
	ast.Inspect(body, func(node ast.Node) bool {
		unary, ok := node.(*ast.UnaryExpr)
		if ok && unary.Op == token.AND && ps6107ContainsObject(pass, unary.X, object) {
			addressed = true
			return false
		}
		return !addressed
	})
	return addressed
}

func ps6107DependsOnParam(pass *analysis.Pass, expression ast.Expr, params map[types.Object]bool, aliases map[types.Object]ast.Expr, visiting map[types.Object]bool) bool {
	found := false
	ast.Inspect(expression, func(node ast.Node) bool {
		if found {
			return false
		}
		identifier, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		object := identObject(pass, identifier)
		if params[object] {
			found = true
			return false
		}
		alias := aliases[object]
		if alias != nil && !visiting[object] {
			visiting[object] = true
			found = ps6107DependsOnParam(pass, alias, params, aliases, visiting)
			delete(visiting, object)
		}
		return !found
	})
	return found
}

func ps6107InlineOverwrite(pass *analysis.Pass, statement ast.Stmt, object types.Object, allowed map[*ast.Ident]bool) bool {
	var loop ast.Node
	var body *ast.BlockStmt
	var loopIndex types.Object
	switch value := statement.(type) {
	case *ast.RangeStmt:
		loop, body = value, value.Body
		identifier, ok := ps2110Unparen(value.X).(*ast.Ident)
		if !ok || identObject(pass, identifier) != object || value.Value != nil {
			return false
		}
		key, ok := ps2110Unparen(value.Key).(*ast.Ident)
		if !ok || key.Name == "_" {
			return false
		}
		loopIndex = identObject(pass, key)
		allowed[identifier] = true
	case *ast.ForStmt:
		loop, body = value, value.Body
		loopIndex = ps6107ForIndex(pass, value)
	default:
		return false
	}
	if body == nil || loopIndex == nil || len(body.List) != 1 ||
		ps6107ObjectWritten(pass, body, loopIndex) || ps6107Addressed(pass, body, loopIndex) {
		return false
	}
	assignment, ok := body.List[0].(*ast.AssignStmt)
	if !ok || assignment.Tok != token.ASSIGN || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
		return false
	}
	index, ok := ps2110Unparen(assignment.Lhs[0]).(*ast.IndexExpr)
	if !ok || ps6024BaseObject(pass, index.X) != object || !ps2140IndexMatchesLoop(pass, ps2110Unparen(index.Index), loop, object) || ps6107ContainsObject(pass, assignment.Rhs[0], object) {
		return false
	}
	container, _ := ps2110Unparen(index.X).(*ast.Ident)
	allowed[container] = true
	if forLoop, ok := loop.(*ast.ForStmt); ok {
		condition, ok := ps2110Unparen(forLoop.Cond).(*ast.BinaryExpr)
		if !ok {
			return false
		}
		length, ok := ps2110Unparen(condition.Y).(*ast.CallExpr)
		if !ok || len(length.Args) != 1 || !typedBuiltinName(pass, length.Fun, "len") {
			return false
		}
		identifier, ok := ps2110Unparen(length.Args[0]).(*ast.Ident)
		if !ok || identObject(pass, identifier) != object {
			return false
		}
		allowed[identifier] = true
	}
	return true
}

func ps6107ForIndex(pass *analysis.Pass, loop *ast.ForStmt) types.Object {
	assignment, ok := loop.Init.(*ast.AssignStmt)
	if !ok || assignment.Tok != token.DEFINE || len(assignment.Lhs) != 1 {
		return nil
	}
	identifier, ok := ps2110Unparen(assignment.Lhs[0]).(*ast.Ident)
	if !ok {
		return nil
	}
	return identObject(pass, identifier)
}

func ps6107ObjectWritten(pass *analysis.Pass, root ast.Node, object types.Object) bool {
	written := false
	ast.Inspect(root, func(node ast.Node) bool {
		if written {
			return false
		}
		switch value := node.(type) {
		case *ast.AssignStmt:
			for _, lhs := range value.Lhs {
				identifier, ok := ps2110Unparen(lhs).(*ast.Ident)
				if ok && identObject(pass, identifier) == object {
					written = true
					return false
				}
			}
		case *ast.IncDecStmt:
			identifier, ok := ps2110Unparen(value.X).(*ast.Ident)
			if ok && identObject(pass, identifier) == object {
				written = true
				return false
			}
		}
		return true
	})
	return written
}

func ps6107PartitionOverwrite(pass *analysis.Pass, statement ast.Stmt, allocation ps6107Allocation, aliases map[types.Object]ast.Expr, contract *config.ReceiverStagingContract, allowed map[*ast.Ident]bool) bool {
	loop, ok := statement.(*ast.RangeStmt)
	if !ok || loop.Body == nil || len(loop.Body.List) != 1 || !contract.ExtentIsNonNegativeAndNonOverflowing {
		return false
	}
	rangedType := types.Unalias(pass.TypesInfo.TypeOf(loop.X)).Underlying()
	switch rangedType.(type) {
	case *types.Array, *types.Slice:
	default:
		return false
	}
	row, ok := ps2110Unparen(loop.Key).(*ast.Ident)
	if !ok || row.Name == "_" {
		return false
	}
	rowObject := identObject(pass, row)
	call := ps6107ExpressionCall(loop.Body.List[0])
	if call == nil || !ps6107ConfiguredCall(pass, call, contract.Overwrite, contract.OverwriteKind) || contract.OverwriteArg >= len(call.Args) {
		return false
	}
	slice, ok := ps2110Unparen(call.Args[contract.OverwriteArg]).(*ast.SliceExpr)
	if !ok || slice.Slice3 || slice.Low == nil || slice.High == nil || slice.Max != nil {
		return false
	}
	container, ok := ps2110Unparen(slice.X).(*ast.Ident)
	if !ok || identObject(pass, container) != allocation.object {
		return false
	}
	width, ok := ps6107ExtentWidth(pass, allocation.size, loop.X, aliases)
	if !ok || !ps6107RowProduct(pass, slice.Low, rowObject, width, false) || !ps6107RowProduct(pass, slice.High, rowObject, width, true) ||
		!ps6107StablePartitionInputs(pass, allocation, loop, width, aliases, contract) {
		return false
	}
	allowed[container] = true
	return true
}

func ps6107ExpressionCall(statement ast.Stmt) *ast.CallExpr {
	expression, ok := statement.(*ast.ExprStmt)
	if !ok {
		return nil
	}
	call, _ := ps2110Unparen(expression.X).(*ast.CallExpr)
	return call
}

func ps6107ExtentWidth(pass *analysis.Pass, extent, ranged ast.Expr, aliases map[types.Object]ast.Expr) (ast.Expr, bool) {
	binary, ok := ps2110Unparen(ps6107ResolveAlias(pass, extent, aliases, make(map[types.Object]bool))).(*ast.BinaryExpr)
	if !ok || binary.Op != token.MUL {
		return nil, false
	}
	if ps6107MatchesLen(pass, binary.X, ranged, aliases) {
		return binary.Y, ps6107StableExpression(pass, binary.Y, aliases)
	}
	if ps6107MatchesLen(pass, binary.Y, ranged, aliases) {
		return binary.X, ps6107StableExpression(pass, binary.X, aliases)
	}
	return nil, false
}

type ps6107Dependencies struct {
	values    map[types.Object]bool
	fields    map[types.Object]bool
	receivers map[types.Object]bool
	start     token.Pos
}

func ps6107StablePartitionInputs(pass *analysis.Pass, allocation ps6107Allocation, loop *ast.RangeStmt, width ast.Expr, aliases map[types.Object]ast.Expr, contract *config.ReceiverStagingContract) bool {
	dependencies := ps6107Dependencies{
		values:    make(map[types.Object]bool),
		fields:    make(map[types.Object]bool),
		receivers: make(map[types.Object]bool),
		start:     allocation.call.Pos(),
	}
	visiting := make(map[types.Object]bool)
	ps6107CollectDependencies(pass, allocation.size, aliases, visiting, &dependencies)
	ps6107CollectDependencies(pass, loop.X, aliases, visiting, &dependencies)
	ps6107CollectDependencies(pass, width, aliases, visiting, &dependencies)
	if ps6107DependencyCaptured(pass, allocation.body, &dependencies) ||
		ps6107DependencyAddressed(pass, allocation.body, &dependencies) ||
		ps6107DependencyEscaped(pass, allocation.body, allocation.call, loop.End(), aliases, contract, &dependencies) {
		return false
	}

	overwrite := ps6107ExpressionCall(loop.Body.List[0])
	stable := true
	ast.Inspect(loop, func(node ast.Node) bool {
		if !stable {
			return false
		}
		switch value := node.(type) {
		case *ast.AssignStmt:
			for _, lhs := range value.Lhs {
				if ps6107MutatesDependency(pass, lhs, &dependencies) {
					stable = false
					return false
				}
			}
		case *ast.IncDecStmt:
			if ps6107MutatesDependency(pass, value.X, &dependencies) {
				stable = false
				return false
			}
		case *ast.CallExpr:
			if value == overwrite || ps6107HarmlessValueCall(pass, value) {
				return true
			}
			stable = false
			return false
		}
		return true
	})
	if !stable {
		return false
	}

	// Scan the straight-line prefix between the earliest captured snapshot and
	// the row loop. This catches slice-header and receiver-field changes that
	// would make an old allocation extent disagree with the current partition.
	ast.Inspect(allocation.body, func(node ast.Node) bool {
		if !stable || node == nil || node.Pos() < dependencies.start || node.Pos() >= loop.Pos() {
			return stable
		}
		switch value := node.(type) {
		case *ast.AssignStmt:
			for _, lhs := range value.Lhs {
				if ps6107MutatesDependency(pass, lhs, &dependencies) {
					stable = false
					return false
				}
			}
		case *ast.IncDecStmt:
			if ps6107MutatesDependency(pass, value.X, &dependencies) {
				stable = false
				return false
			}
		case *ast.CallExpr:
			if value != allocation.call && !ps6107HarmlessValueCall(pass, value) {
				stable = false
				return false
			}
		}
		return true
	})
	return stable
}

func ps6107CollectDependencies(pass *analysis.Pass, expression ast.Expr, aliases map[types.Object]ast.Expr, visiting map[types.Object]bool, dependencies *ps6107Dependencies) {
	expression = ps2110Unparen(expression)
	switch value := expression.(type) {
	case *ast.Ident:
		object := identObject(pass, value)
		if alias := aliases[object]; alias != nil && !visiting[object] {
			if alias.Pos() < dependencies.start {
				dependencies.start = alias.Pos()
			}
			visiting[object] = true
			ps6107CollectDependencies(pass, alias, aliases, visiting, dependencies)
			delete(visiting, object)
			return
		}
		if object != nil {
			dependencies.values[object] = true
		}
	case *ast.SelectorExpr:
		ps6107CollectSelectorDependencies(pass, value, aliases, visiting, dependencies)
	case *ast.BinaryExpr:
		ps6107CollectDependencies(pass, value.X, aliases, visiting, dependencies)
		ps6107CollectDependencies(pass, value.Y, aliases, visiting, dependencies)
	case *ast.UnaryExpr:
		ps6107CollectDependencies(pass, value.X, aliases, visiting, dependencies)
	case *ast.CallExpr:
		for _, argument := range value.Args {
			ps6107CollectDependencies(pass, argument, aliases, visiting, dependencies)
		}
	}
}

func ps6107CollectSelectorDependencies(pass *analysis.Pass, selector *ast.SelectorExpr, aliases map[types.Object]ast.Expr, visiting map[types.Object]bool, dependencies *ps6107Dependencies) {
	if selection := pass.TypesInfo.Selections[selector]; selection != nil && selection.Kind() == types.FieldVal {
		dependencies.fields[selection.Obj()] = true
	}
	if nested, ok := ps2110Unparen(selector.X).(*ast.SelectorExpr); ok {
		ps6107CollectSelectorDependencies(pass, nested, aliases, visiting, dependencies)
	}
	if root := ps6107RootObject(pass, selector.X, aliases, visiting); root != nil {
		dependencies.receivers[root] = true
	}
}

func ps6107RootObject(pass *analysis.Pass, expression ast.Expr, aliases map[types.Object]ast.Expr, visiting map[types.Object]bool) types.Object {
	expression = ps2110Unparen(expression)
	switch value := expression.(type) {
	case *ast.Ident:
		object := identObject(pass, value)
		if alias := aliases[object]; alias != nil && !visiting[object] {
			visiting[object] = true
			root := ps6107RootObject(pass, alias, aliases, visiting)
			delete(visiting, object)
			return root
		}
		return object
	case *ast.SelectorExpr:
		return ps6107RootObject(pass, value.X, aliases, visiting)
	}
	return nil
}

func ps6107MutatesDependency(pass *analysis.Pass, expression ast.Expr, dependencies *ps6107Dependencies) bool {
	expression = ps2110Unparen(expression)
	switch value := expression.(type) {
	case *ast.Ident:
		object := identObject(pass, value)
		return dependencies.values[object] || dependencies.receivers[object]
	case *ast.SelectorExpr:
		selection := pass.TypesInfo.Selections[value]
		return selection != nil && dependencies.fields[selection.Obj()]
	case *ast.StarExpr:
		return ps6107ReferencesDependency(pass, value.X, dependencies)
	}
	return false
}

func ps6107HarmlessValueCall(pass *analysis.Pass, call *ast.CallExpr) bool {
	if typedBuiltinName(pass, call.Fun, "len") {
		return true
	}
	return pass.TypesInfo.Types[ps2110Unparen(call.Fun)].IsType()
}

func ps6107CallExposesReceiver(pass *analysis.Pass, call *ast.CallExpr, aliases map[types.Object]ast.Expr, receivers map[types.Object]bool) bool {
	if selector, ok := ps2110Unparen(call.Fun).(*ast.SelectorExpr); ok {
		root := ps6107RootObject(pass, selector.X, aliases, make(map[types.Object]bool))
		if receivers[root] {
			return true
		}
	}
	for _, argument := range call.Args {
		if ps6107ExpressionExposesReceiver(pass, argument, aliases, receivers) {
			return true
		}
	}
	return false
}

func ps6107ExpressionExposesReceiver(pass *analysis.Pass, expression ast.Expr, aliases map[types.Object]ast.Expr, receivers map[types.Object]bool) bool {
	return ps6107ExpressionExposesReceiverSeen(pass, expression, aliases, receivers, make(map[types.Object]bool))
}

func ps6107ExpressionExposesReceiverSeen(pass *analysis.Pass, expression ast.Expr, aliases map[types.Object]ast.Expr, receivers map[types.Object]bool, visiting map[types.Object]bool) bool {
	exposed := false
	ast.Inspect(expression, func(node ast.Node) bool {
		if exposed {
			return false
		}
		switch value := node.(type) {
		case *ast.Ident:
			object := identObject(pass, value)
			exposed = receivers[object]
			if alias := aliases[object]; !exposed && alias != nil && !visiting[object] {
				visiting[object] = true
				exposed = ps6107ExpressionExposesReceiverSeen(pass, alias, aliases, receivers, visiting)
				delete(visiting, object)
			}
		case *ast.SelectorExpr:
			exposed = receivers[ps6107RootObject(pass, value, aliases, make(map[types.Object]bool))]
		}
		return !exposed
	})
	return exposed
}

func ps6107DependencyEscaped(pass *analysis.Pass, body *ast.BlockStmt, allocation *ast.CallExpr, through token.Pos, aliases map[types.Object]ast.Expr, contract *config.ReceiverStagingContract, dependencies *ps6107Dependencies) bool {
	escaped := false
	ast.Inspect(body, func(node ast.Node) bool {
		if escaped || node == nil || node.Pos() >= through {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok || call == allocation || ps6107HarmlessValueCall(pass, call) || ps6107ConfiguredCall(pass, call, contract.Overwrite, contract.OverwriteKind) {
			return true
		}
		if ps6107CallExposesReceiver(pass, call, aliases, dependencies.receivers) {
			escaped = true
			return false
		}
		return true
	})
	return escaped
}

func ps6107DependencyCaptured(pass *analysis.Pass, body *ast.BlockStmt, dependencies *ps6107Dependencies) bool {
	captured := false
	ast.Inspect(body, func(node ast.Node) bool {
		literal, ok := node.(*ast.FuncLit)
		if !ok {
			return true
		}
		if ps6107ReferencesDependency(pass, literal.Body, dependencies) {
			captured = true
		}
		return false
	})
	return captured
}

func ps6107DependencyAddressed(pass *analysis.Pass, body *ast.BlockStmt, dependencies *ps6107Dependencies) bool {
	addressed := false
	ast.Inspect(body, func(node ast.Node) bool {
		unary, ok := node.(*ast.UnaryExpr)
		if ok && unary.Op == token.AND && ps6107MutatesDependency(pass, unary.X, dependencies) {
			addressed = true
			return false
		}
		return !addressed
	})
	return addressed
}

func ps6107ReferencesDependency(pass *analysis.Pass, root ast.Node, dependencies *ps6107Dependencies) bool {
	found := false
	ast.Inspect(root, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.Ident:
			object := identObject(pass, value)
			if dependencies.values[object] || dependencies.receivers[object] {
				found = true
				return false
			}
		case *ast.SelectorExpr:
			selection := pass.TypesInfo.Selections[value]
			if selection != nil && dependencies.fields[selection.Obj()] {
				found = true
				return false
			}
		}
		return !found
	})
	return found
}

func ps6107ResolveAlias(pass *analysis.Pass, expression ast.Expr, aliases map[types.Object]ast.Expr, visiting map[types.Object]bool) ast.Expr {
	identifier, ok := ps2110Unparen(expression).(*ast.Ident)
	if !ok {
		return expression
	}
	object := identObject(pass, identifier)
	alias := aliases[object]
	if alias == nil || visiting[object] {
		return expression
	}
	visiting[object] = true
	resolved := ps6107ResolveAlias(pass, alias, aliases, visiting)
	delete(visiting, object)
	return resolved
}

func ps6107MatchesLen(pass *analysis.Pass, expression, ranged ast.Expr, aliases map[types.Object]ast.Expr) bool {
	expression = ps6107ResolveAlias(pass, expression, aliases, make(map[types.Object]bool))
	call, ok := ps2110Unparen(expression).(*ast.CallExpr)
	if !ok || len(call.Args) != 1 || call.Ellipsis.IsValid() || !typedBuiltinName(pass, call.Fun, "len") {
		return false
	}
	return ps6107SameExpression(pass, call.Args[0], ranged)
}

func ps6107StableExpression(pass *analysis.Pass, expression ast.Expr, aliases map[types.Object]ast.Expr) bool {
	stable := true
	ast.Inspect(expression, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.CallExpr, *ast.UnaryExpr:
			stable = false
			return false
		case *ast.Ident:
			object := identObject(pass, value)
			if alias := aliases[object]; alias != nil && alias != expression {
				stable = stable && ps6107StableExpression(pass, alias, aliases)
			}
		}
		return stable
	})
	return stable
}

func ps6107RowProduct(pass *analysis.Pass, expression ast.Expr, row types.Object, width ast.Expr, plusOne bool) bool {
	binary, ok := ps2110Unparen(expression).(*ast.BinaryExpr)
	if !ok || binary.Op != token.MUL || !ps6107SameExpression(pass, binary.Y, width) {
		return false
	}
	if !plusOne {
		identifier, ok := ps2110Unparen(binary.X).(*ast.Ident)
		return ok && identObject(pass, identifier) == row
	}
	addition, ok := ps2110Unparen(binary.X).(*ast.BinaryExpr)
	if !ok || addition.Op != token.ADD {
		return false
	}
	identifier, ok := ps2110Unparen(addition.X).(*ast.Ident)
	literal, literalOK := ps2110Unparen(addition.Y).(*ast.BasicLit)
	return ok && identObject(pass, identifier) == row && literalOK && literal.Kind == token.INT && literal.Value == "1"
}

func ps6107SameExpression(pass *analysis.Pass, left, right ast.Expr) bool {
	left, right = ps2110Unparen(left), ps2110Unparen(right)
	switch left := left.(type) {
	case *ast.Ident:
		right, ok := right.(*ast.Ident)
		return ok && identObject(pass, left) == identObject(pass, right)
	case *ast.BasicLit:
		right, ok := right.(*ast.BasicLit)
		return ok && left.Kind == right.Kind && left.Value == right.Value
	case *ast.SelectorExpr:
		right, ok := right.(*ast.SelectorExpr)
		return ok && pass.TypesInfo.Selections[left] != nil && pass.TypesInfo.Selections[right] != nil &&
			pass.TypesInfo.Selections[left].Obj() == pass.TypesInfo.Selections[right].Obj() &&
			ps6107SameExpression(pass, left.X, right.X)
	case *ast.BinaryExpr:
		right, ok := right.(*ast.BinaryExpr)
		return ok && left.Op == right.Op && ps6107SameExpression(pass, left.X, right.X) && ps6107SameExpression(pass, left.Y, right.Y)
	}
	return false
}

func ps6107Consumer(pass *analysis.Pass, statement ast.Stmt, object types.Object, contract *config.ReceiverStagingContract, allowed map[*ast.Ident]bool) (*types.Func, bool) {
	var matches []*ast.CallExpr
	ast.Inspect(statement, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if ok && ps6107ConfiguredCall(pass, call, contract.Consumer, contract.ConsumerKind) {
			matches = append(matches, call)
		}
		return true
	})
	if len(matches) != 1 || contract.ConsumerArg >= len(matches[0].Args) || !ps6107DirectConsumerStatement(statement, matches[0]) {
		return nil, false
	}
	identifier, ok := ps2110Unparen(matches[0].Args[contract.ConsumerArg]).(*ast.Ident)
	if !ok || identObject(pass, identifier) != object {
		return nil, false
	}
	allowed[identifier] = true
	function, _, ok := typedCallee(pass, matches[0].Fun)
	return function, ok
}

func ps6107DirectConsumerStatement(statement ast.Stmt, target *ast.CallExpr) bool {
	direct := func(expression ast.Expr) bool { return ps2110Unparen(expression) == ast.Expr(target) }
	switch value := statement.(type) {
	case *ast.ExprStmt:
		return direct(value.X)
	case *ast.AssignStmt:
		return len(value.Rhs) == 1 && direct(value.Rhs[0])
	case *ast.DeclStmt:
		declaration, ok := value.Decl.(*ast.GenDecl)
		if !ok || len(declaration.Specs) != 1 {
			return false
		}
		spec, ok := declaration.Specs[0].(*ast.ValueSpec)
		return ok && len(spec.Values) == 1 && direct(spec.Values[0])
	case *ast.ReturnStmt:
		return len(value.Results) == 1 && direct(value.Results[0])
	case *ast.IfStmt:
		assignment, ok := value.Init.(*ast.AssignStmt)
		return ok && len(assignment.Rhs) == 1 && direct(assignment.Rhs[0])
	default:
		return false
	}
}

func ps6107ConfiguredCall(pass *analysis.Pass, call *ast.CallExpr, id string, kind config.ReceiverStagingCallKind) bool {
	function, signature, ok := typedCallee(pass, call.Fun)
	if !ok || function.Pkg() == nil || ps6090FunctionID(function) != id {
		return false
	}
	if kind == config.ReceiverStagingCallMethod {
		selector, selectorOK := ps2110Unparen(call.Fun).(*ast.SelectorExpr)
		if !selectorOK || signature.Recv() == nil {
			return false
		}
		selection := pass.TypesInfo.Selections[selector]
		return selection != nil && selection.Kind() == types.MethodVal
	}
	return kind == config.ReceiverStagingCallFunction && signature.Recv() == nil
}

func ps6107OnlyAllowedUses(pass *analysis.Pass, body *ast.BlockStmt, object types.Object, allowed map[*ast.Ident]bool) bool {
	valid := true
	ast.Inspect(body, func(node ast.Node) bool {
		if !valid {
			return false
		}
		identifier, ok := node.(*ast.Ident)
		if ok && identObject(pass, identifier) == object && !allowed[identifier] {
			valid = false
			return false
		}
		return true
	})
	return valid
}

func ps6107ContainsObject(pass *analysis.Pass, node ast.Node, object types.Object) bool {
	found := false
	ast.Inspect(node, func(child ast.Node) bool {
		identifier, ok := child.(*ast.Ident)
		if ok && identObject(pass, identifier) == object {
			found = true
			return false
		}
		return !found
	})
	return found
}
