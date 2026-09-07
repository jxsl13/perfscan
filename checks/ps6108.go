package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"slices"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS6108 implements owner issue #838. It recognizes repeated calls to one
// narrowly proven packed-field helper in an exact coefficient-building loop.
var PS6108 = register(&lint.Check{
	ID:       "PS6108",
	Category: "verify",
	Slug:     "repeated-packed-bitfield-helper-extraction",
	Level:    lint.LevelAggressive,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "a coefficient loop repeatedly calls the same packed-field helper",
		Text: `Small packed-data helpers can reload and re-decode the same header
bytes when adjacent logical fields are requested separately in a hot loop. A
bulk or paired decode may reuse those bytes, but only when the layout, indices,
bounds, unsigned arithmetic, coefficient placement, and compiler output are all
understood.

PS6108 deliberately recognizes only an owner-shaped source candidate. A direct
package-local helper takes an int index and one []byte header and returns two
byte/uint8 values. Its bounded body contains one constant index
threshold and only byte indexing, masks, and constant shifts. One reachable
top-level exact 4-64 trip zero-based unit-increment loop calls it twice per
header at related affine indices and immediately
stores all results, after destination-typed float conversion and multiplication
by stable scalars, into a private fixed coefficient array. One or two independent
header/output streams are accepted. Every result is used once, every generated
index is in range on its reachable helper branch, and every written coefficient
slot reaches a reachable direct top-level return or a single-use local
arithmetic reduction whose direct return is reachable. Source aliases, mutations,
escapes, dynamic layouts, signed or target-width arithmetic, helper effects,
unbounded proof work, nested/unreachable loops, output escapes, and unclassified
uses remain silent.

This is source-level evidence only. Go analysis does not model the final
compiler's inlining, bounds-check elimination, common-subexpression elimination,
or instruction selection. Repeated source calls therefore do not prove repeated
executed loads or a speedup. Profile the real caller, inspect optimized native
code, and benchmark the same retained work before replacing the calls. Preserve
the original slice evaluation and panic boundary, unsigned field semantics,
float conversion and multiplication order, coefficient slots, and all aliases.
There is NO automatic fix.`,
		Before: `for pair := range 4 {
	logical := pair * 2
	coefficient := pair * 4
	s0, m0 := fieldPair(logical, packed)
	s1, m1 := fieldPair(logical+1, packed)
	coeff[coefficient+0] = scale * float32(s0)
	coeff[coefficient+1] = minimum * float32(m0)
	coeff[coefficient+2] = scale * float32(s1)
	coeff[coefficient+3] = minimum * float32(m1)
}`,
		After: `// After profiling and native-code inspection:
for field := range 4 {
	low, middle, highBits := packed[field], packed[field+4], packed[field+8]
	lowCoefficient, highCoefficient := field*2, (field+4)*2
	coeff[lowCoefficient+0] = scale * float32(low&63)
	coeff[lowCoefficient+1] = minimum * float32(middle&63)
	coeff[highCoefficient+0] = scale * float32((highBits&0x0f)|((low>>6)<<4))
	coeff[highCoefficient+1] = minimum * float32((highBits>>4)|((middle>>6)<<4))
}`,
		MeasuredWin: `Owner issue #838 attributes Apple M2 Go 1.26.6 evidence to
the exact Q4_K bulk-header change: seven of seven paired-row samples improved
from a 571.4 ns median to 536.4 ns (1.065x), and seven of seven paired FFN
samples improved from 555.135 us to 528.617 us (1.050x), with unchanged
allocation counts. An independent single-row campaign improved from 312.0 ns
to 288.6 ns (1.081x). The five-pair 64-token production campaign preserved its
output digest but was neutral-to-positive overall, so these are attributed
project results, not a generic timing guarantee. The repository mechanism
benchmark was code-generation checked and measured on Apple M2 Pro with Go
1.27.0 in six alternating AB/BA fresh-process two-second pairs. All six favored
the bulk form: Before to After was 20.38 to 18.80, 20.36 to 18.60, 20.32 to
18.57, 20.29 to 18.51, 20.17 to 18.50, and 20.16 to 18.62 ns/op. The separate
arm medians were 20.305 and 18.585 ns/op (1.093x; 8.47% lower), with 0 B/op and
0 allocs/op in every arm. This supports only the retained repository benchmark
shape; real candidates still require their own profiling, native-code
inspection, and bit-identical same-work timing.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6108",
		Doc:  "repeated pure packed-field helper calls in an exact coefficient loop",
		Run:  runPS6108,
	},
})

const (
	ps6108MinIterations  = 4
	ps6108MaxIterations  = 64
	ps6108MaxHelperNodes = 256
)

type ps6108HelperProof struct {
	function       *types.Func
	index          *types.Var
	packed         *types.Var
	indexPosition  int
	packedPosition int
	threshold      int64
	lowOffsets     []int64
	highOffsets    []int64
	valid          bool
}

type ps6108Loop struct {
	statement ast.Stmt
	body      *ast.BlockStmt
	index     types.Object
	count     int64
}

type ps6108Affine struct {
	coefficient int64
	constant    int64
}

type ps6108Call struct {
	assignment *ast.AssignStmt
	call       *ast.CallExpr
	helper     ps6108HelperProof
	source     types.Object
	index      ps6108Affine
	results    [2]types.Object
}

type ps6108Store struct {
	statement *ast.AssignStmt
	output    types.Object
	index     ps6108Affine
	result    types.Object
}

type ps6108Source struct {
	object     types.Object
	definition ast.Node
	base       types.Object
	length     int64
}

type ps6108Context struct {
	pass        *analysis.Pass
	functions   map[*types.Func]*ast.FuncDecl
	helperProof map[*types.Func]ps6108HelperProof
}

func runPS6108(pass *analysis.Pass) (any, error) {
	context := &ps6108Context{
		pass:        pass,
		functions:   make(map[*types.Func]*ast.FuncDecl),
		helperProof: make(map[*types.Func]ps6108HelperProof),
	}
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			object, _ := pass.TypesInfo.Defs[function.Name].(*types.Func)
			if object != nil {
				context.functions[object.Origin()] = function
			}
		}
	}
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil || ps6108UnsafeOrReflect(pass, function.Body) {
				continue
			}
			parents := ps6087Parents(function.Body)
			ast.Inspect(function.Body, func(node ast.Node) bool {
				if _, nested := node.(*ast.FuncLit); nested {
					return false
				}
				loop, ok := ps6108LoopInfo(pass, node)
				if ok && parents[loop.statement] == function.Body {
					ps6108CheckLoop(context, function, loop, parents)
				}
				return true
			})
		}
	}
	return nil, nil
}

func ps6108LoopInfo(pass *analysis.Pass, node ast.Node) (ps6108Loop, bool) {
	var statement ast.Stmt
	var body *ast.BlockStmt
	var index types.Object
	var count uint64
	var exact bool
	switch loop := node.(type) {
	case *ast.RangeStmt:
		identifier, ok := loop.Key.(*ast.Ident)
		if !ok || identifier.Name == "_" || loop.Value != nil {
			return ps6108Loop{}, false
		}
		statement, body, index = loop, loop.Body, pass.TypesInfo.Defs[identifier]
		integer, constant := ps6108PositiveIntConstant(pass, loop.X)
		if index != nil && ps6108ExactInt(index.Type()) && constant {
			count, exact = uint64(integer), true
		}
	case *ast.ForStmt:
		initialization, ok := loop.Init.(*ast.AssignStmt)
		if !ok || initialization.Tok != token.DEFINE || len(initialization.Lhs) != 1 || len(initialization.Rhs) != 1 {
			return ps6108Loop{}, false
		}
		identifier, ok := initialization.Lhs[0].(*ast.Ident)
		if !ok {
			return ps6108Loop{}, false
		}
		statement, body, index = loop, loop.Body, pass.TypesInfo.Defs[identifier]
		count, exact = ps6108CanonicalForIterations(pass, loop, index)
	default:
		return ps6108Loop{}, false
	}
	if index == nil || !exact || count < ps6108MinIterations || count > ps6108MaxIterations {
		return ps6108Loop{}, false
	}
	return ps6108Loop{statement: statement, body: body, index: index, count: int64(count)}, true
}

// ps6108CanonicalForIterations accepts only the induction sequence used by
// the proof below: 0, 1, ..., count-1. The broader PS6099 loop helper also
// recognizes nonzero starts, larger steps, and descending loops; treating its
// result as the induction values would make bounds and output-slot proofs
// unsound.
func ps6108CanonicalForIterations(pass *analysis.Pass, loop *ast.ForStmt, index types.Object) (uint64, bool) {
	if index == nil {
		return 0, false
	}
	initialization, ok := loop.Init.(*ast.AssignStmt)
	if !ok || initialization.Tok != token.DEFINE || len(initialization.Lhs) != 1 || len(initialization.Rhs) != 1 {
		return 0, false
	}
	identifier, ok := initialization.Lhs[0].(*ast.Ident)
	if !ok || pass.TypesInfo.Defs[identifier] != index || !ps6108ExactInt(index.Type()) {
		return 0, false
	}
	start, ok := ps6108IntConstant(pass, initialization.Rhs[0])
	if !ok || start != 0 {
		return 0, false
	}
	condition, ok := ps2110Unparen(loop.Cond).(*ast.BinaryExpr)
	if !ok || condition.Op != token.LSS || !ps6108ObjectExpression(pass, condition.X, index) {
		return 0, false
	}
	count, ok := ps6108PositiveIntConstant(pass, condition.Y)
	if !ok {
		return 0, false
	}
	post, ok := loop.Post.(*ast.IncDecStmt)
	if !ok || post.Tok != token.INC || !ps6108ObjectExpression(pass, post.X, index) {
		return 0, false
	}
	return uint64(count), true
}

func ps6108CheckLoop(context *ps6108Context, function *ast.FuncDecl, loop ps6108Loop, parents map[ast.Node]ast.Node) {
	aliases := make(map[types.Object]ps6108Affine)
	var calls []ps6108Call
	var stores []ps6108Store
	storePhase := false
	for _, statement := range loop.body.List {
		if aliasObject, affine, ok := ps6108AffineAlias(context.pass, statement, loop.index, aliases); ok && !storePhase {
			aliases[aliasObject] = affine
			continue
		}
		if call, ok := ps6108CallStatement(context, statement, loop.index, aliases); ok && !storePhase {
			calls = append(calls, call)
			continue
		}
		store, ok := ps6108StoreStatement(context.pass, statement, loop.index, aliases)
		if !ok {
			return
		}
		storePhase = true
		stores = append(stores, store)
	}
	if len(calls) != 2 && len(calls) != 4 || len(stores) != len(calls)*2 {
		return
	}
	helper := calls[0].helper
	for callIndex := 1; callIndex < len(calls); callIndex++ {
		call := &calls[callIndex]
		if call.helper.function != helper.function {
			return
		}
	}
	groups, order, ok := ps6108CallGroups(calls)
	if !ok || len(order) != 1 && len(order) != 2 {
		return
	}
	firstPair := groups[order[0]]
	for _, source := range order {
		pair := groups[source]
		if len(pair) != 2 || pair[0].index.coefficient <= 0 || pair[0].index.coefficient != pair[1].index.coefficient {
			return
		}
		delta, exact := ps6066SafeAdd(pair[1].index.constant, -pair[0].index.constant)
		if !exact || delta <= 0 || pair[0].index != firstPair[0].index || pair[1].index != firstPair[1].index {
			return
		}
	}
	sources := make(map[types.Object]ps6108Source, len(order))
	packedReads := int64(0)
	for _, sourceObject := range order {
		source, ok := ps6108SourceInfo(context.pass, function, loop, sourceObject)
		if !ok || source.base == nil || source.base == source.object {
			return
		}
		sources[sourceObject] = source
		pair := groups[sourceObject]
		for callIndex := range pair {
			reads, valid := ps6108CallBounds(context.pass, loop, &pair[callIndex], source.length)
			if !valid {
				return
			}
			packedReads += reads
		}
	}
	if len(order) == 2 && (sources[order[0]].base == sources[order[1]].base || order[0] == order[1]) {
		return
	}
	resultOrder := make([]types.Object, 0, len(calls)*2)
	allowedResultUses := make(map[types.Object]ast.Expr, len(calls)*2)
	for callIndex := range calls {
		call := &calls[callIndex]
		resultOrder = append(resultOrder, call.results[:]...)
	}
	outputs := make(map[types.Object][]int64, len(order))
	for index, store := range stores {
		if store.result != resultOrder[index] || !ps6108StoreRHS(context.pass, store.statement.Rhs[0], store.result, store.output, function.Body) {
			return
		}
		allowedResultUses[store.result] = store.statement.Rhs[0]
		slots, ok := ps6108OutputSlots(context.pass, loop, store)
		if !ok {
			return
		}
		outputs[store.output] = append(outputs[store.output], slots...)
	}
	for _, slots := range outputs {
		slices.Sort(slots)
		for slotIndex := 1; slotIndex < len(slots); slotIndex++ {
			if slots[slotIndex] == slots[slotIndex-1] {
				return
			}
		}
	}
	if len(outputs) != len(order) || !ps6108OutputGroupOrder(calls, stores) {
		return
	}
	if !ps6108AliasesClosed(context.pass, function.Body, aliases, calls, stores) ||
		!ps6108ResultsClosed(context.pass, function.Body, allowedResultUses) {
		return
	}
	allowedSources := make(map[types.Object][]ast.Expr, len(calls))
	for callIndex := range calls {
		call := &calls[callIndex]
		allowedSources[call.source] = append(allowedSources[call.source], call.call.Args[call.helper.packedPosition])
	}
	for object, source := range sources {
		if !ps6108SourceClosed(context.pass, function.Body, source, allowedSources[object]) {
			return
		}
	}
	// CFG construction is intentionally lazy: unrelated functions and loops
	// that fail the typed helper/source/store proof never pay for it.
	reachable := ps6099ReachableNodesInBlock(context.pass, function.Body, parents)
	if !reachable[calls[0].call] {
		return
	}
	for output, slots := range outputs {
		if !ps6108OutputClosed(context.pass, function, loop, output, slots, stores, parents, reachable) {
			return
		}
	}
	context.pass.Reportf(calls[0].call.Pos(), "%d repeated calls per packed source to pure local helper %s decode statically related fields from the same %d-byte header in an exact %d-trip coefficient loop (%d source-level packed reads across %d stream(s)); profile and inspect optimized code, then benchmark a bit-identical bulk/pair decode that preserves bounds, unsigned field semantics, coefficient placement, and evaluation order (advisory, no automatic fix)",
		2, helper.function.Name(), sources[order[0]].length, loop.count, packedReads, len(order))
}

func ps6108AffineAlias(pass *analysis.Pass, statement ast.Stmt, loopIndex types.Object, aliases map[types.Object]ps6108Affine) (types.Object, ps6108Affine, bool) {
	assignment, ok := statement.(*ast.AssignStmt)
	if !ok || assignment.Tok != token.DEFINE || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
		return nil, ps6108Affine{}, false
	}
	identifier, ok := assignment.Lhs[0].(*ast.Ident)
	if !ok || identifier.Name == "_" {
		return nil, ps6108Affine{}, false
	}
	affine, ok := ps6108AffineExpression(pass, assignment.Rhs[0], loopIndex, aliases, map[types.Object]bool{})
	object := pass.TypesInfo.Defs[identifier]
	return object, affine, ok && object != nil
}

func ps6108CallStatement(context *ps6108Context, statement ast.Stmt, loopIndex types.Object, aliases map[types.Object]ps6108Affine) (ps6108Call, bool) {
	assignment, ok := statement.(*ast.AssignStmt)
	if !ok || assignment.Tok != token.DEFINE || len(assignment.Lhs) != 2 || len(assignment.Rhs) != 1 {
		return ps6108Call{}, false
	}
	call, ok := ps2110Unparen(assignment.Rhs[0]).(*ast.CallExpr)
	if !ok || call.Ellipsis.IsValid() {
		return ps6108Call{}, false
	}
	callee, signature, ok := typedCallee(context.pass, call.Fun)
	if !ok || callee == nil || signature == nil || signature.Recv() != nil || callee.Pkg() != context.pass.Pkg {
		return ps6108Call{}, false
	}
	proof := context.proveHelper(callee)
	if !proof.valid || len(call.Args) != signature.Params().Len() {
		return ps6108Call{}, false
	}
	sourceIdentifier, ok := ps2110Unparen(call.Args[proof.packedPosition]).(*ast.Ident)
	if !ok {
		return ps6108Call{}, false
	}
	source := context.pass.TypesInfo.ObjectOf(sourceIdentifier)
	index, ok := ps6108AffineExpression(context.pass, call.Args[proof.indexPosition], loopIndex, aliases, map[types.Object]bool{})
	if !ok || source == nil {
		return ps6108Call{}, false
	}
	var results [2]types.Object
	for resultIndex, lhs := range assignment.Lhs {
		identifier, ok := lhs.(*ast.Ident)
		if !ok || identifier.Name == "_" {
			return ps6108Call{}, false
		}
		results[resultIndex] = context.pass.TypesInfo.Defs[identifier]
		if results[resultIndex] == nil {
			return ps6108Call{}, false
		}
	}
	return ps6108Call{assignment: assignment, call: call, helper: proof, source: source, index: index, results: results}, true
}

func (context *ps6108Context) proveHelper(function *types.Func) ps6108HelperProof {
	key := function.Origin()
	if proof, ok := context.helperProof[key]; ok {
		return proof
	}
	proof := ps6108ProveHelper(context.pass, key, context.functions[key])
	context.helperProof[key] = proof
	return proof
}

func ps6108ProveHelper(pass *analysis.Pass, function *types.Func, declaration *ast.FuncDecl) ps6108HelperProof {
	proof := ps6108HelperProof{function: function}
	if function == nil || declaration == nil || declaration.Recv != nil || declaration.Type.TypeParams != nil || declaration.Body == nil {
		return proof
	}
	nodes := 0
	ast.Inspect(declaration.Body, func(node ast.Node) bool {
		if node != nil {
			nodes++
		}
		return nodes <= ps6108MaxHelperNodes
	})
	if nodes > ps6108MaxHelperNodes {
		return proof
	}
	signature, _ := function.Type().(*types.Signature)
	if signature == nil || signature.Variadic() || signature.TypeParams().Len() != 0 || signature.Params().Len() != 2 || signature.Results().Len() != 2 {
		return proof
	}
	for index := 0; index < signature.Params().Len(); index++ {
		parameter := signature.Params().At(index)
		if ps6108ExactInt(parameter.Type()) {
			if proof.index != nil {
				return ps6108HelperProof{function: function}
			}
			proof.index, proof.indexPosition = parameter, index
		} else if ps6108ByteSlice(parameter.Type()) {
			if proof.packed != nil {
				return ps6108HelperProof{function: function}
			}
			proof.packed, proof.packedPosition = parameter, index
		} else {
			return ps6108HelperProof{function: function}
		}
	}
	resultType := signature.Results().At(0).Type()
	if proof.index == nil || proof.packed == nil || !types.Identical(resultType, signature.Results().At(1).Type()) {
		return ps6108HelperProof{function: function}
	}
	bits, ok := ps6108FixedUnsignedBits(resultType)
	if !ok || bits != 8 || len(declaration.Body.List) < 2 {
		return ps6108HelperProof{function: function}
	}
	branch, ok := declaration.Body.List[0].(*ast.IfStmt)
	if !ok || branch.Init != nil || branch.Else != nil {
		return ps6108HelperProof{function: function}
	}
	condition, ok := ps2110Unparen(branch.Cond).(*ast.BinaryExpr)
	if !ok || condition.Op != token.LSS || !ps6108ObjectExpression(pass, condition.X, proof.index) {
		return ps6108HelperProof{function: function}
	}
	threshold, ok := ps6108PositiveIntConstant(pass, condition.Y)
	if !ok {
		return ps6108HelperProof{function: function}
	}
	lowResults, ok := ps6108HelperPath(pass, branch.Body.List, signature)
	if !ok {
		return ps6108HelperProof{function: function}
	}
	highResults, ok := ps6108HelperPath(pass, declaration.Body.List[1:], signature)
	if !ok {
		return ps6108HelperProof{function: function}
	}
	lowOffsets, lowOK := ps6108HelperExpressions(pass, lowResults, proof.index, proof.packed, resultType, bits)
	highOffsets, highOK := ps6108HelperExpressions(pass, highResults, proof.index, proof.packed, resultType, bits)
	if !lowOK || !highOK {
		return ps6108HelperProof{function: function}
	}
	proof.threshold, proof.lowOffsets, proof.highOffsets, proof.valid = threshold, lowOffsets, highOffsets, true
	return proof
}

func ps6108HelperPath(pass *analysis.Pass, statements []ast.Stmt, signature *types.Signature) ([2]ast.Expr, bool) {
	var result [2]ast.Expr
	if len(statements) == 1 {
		statement, ok := statements[0].(*ast.ReturnStmt)
		if ok && len(statement.Results) == 2 {
			result[0], result[1] = statement.Results[0], statement.Results[1]
			return result, true
		}
		return result, false
	}
	if len(statements) != 3 {
		return result, false
	}
	returned, ok := statements[2].(*ast.ReturnStmt)
	if !ok || len(returned.Results) != 0 {
		return result, false
	}
	for _, raw := range statements[:2] {
		assignment, ok := raw.(*ast.AssignStmt)
		if !ok || assignment.Tok != token.ASSIGN || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
			return [2]ast.Expr{}, false
		}
		identifier, ok := ps2110Unparen(assignment.Lhs[0]).(*ast.Ident)
		if !ok {
			return [2]ast.Expr{}, false
		}
		object := pass.TypesInfo.ObjectOf(identifier)
		position := -1
		for index := 0; index < 2; index++ {
			if signature.Results().At(index) == object {
				position = index
			}
		}
		if position < 0 || result[position] != nil {
			return [2]ast.Expr{}, false
		}
		result[position] = assignment.Rhs[0]
	}
	return result, result[0] != nil && result[1] != nil
}

func ps6108HelperExpressions(pass *analysis.Pass, expressions [2]ast.Expr, index, packed types.Object, resultType types.Type, bits uint) ([]int64, bool) {
	var offsets []int64
	for _, expression := range expressions {
		if !types.Identical(pass.TypesInfo.TypeOf(expression), resultType) {
			return nil, false
		}
		before := len(offsets)
		if !ps6108HelperExpression(pass, expression, index, packed, bits, &offsets) || len(offsets) == before {
			return nil, false
		}
	}
	return offsets, true
}

func ps6108HelperExpression(pass *analysis.Pass, expression ast.Expr, index, packed types.Object, bits uint, offsets *[]int64) bool {
	expression = ps2110Unparen(expression)
	if value := pass.TypesInfo.Types[expression].Value; value != nil {
		integer, exact := constant.Uint64Val(value)
		return exact && (bits == 64 || integer < uint64(1)<<bits)
	}
	switch value := expression.(type) {
	case *ast.IndexExpr:
		if !ps6108ObjectExpression(pass, value.X, packed) {
			return false
		}
		offset, ok := ps6108IndexOffset(pass, value.Index, index)
		if !ok {
			return false
		}
		*offsets = append(*offsets, offset)
		return true
	case *ast.UnaryExpr:
		return value.Op == token.ADD && ps6108HelperExpression(pass, value.X, index, packed, bits, offsets)
	case *ast.BinaryExpr:
		switch value.Op {
		case token.AND, token.OR:
			return ps6108HelperExpression(pass, value.X, index, packed, bits, offsets) &&
				ps6108HelperExpression(pass, value.Y, index, packed, bits, offsets)
		case token.SHL, token.SHR:
			shift, ok := ps6108NonnegativeIntConstant(pass, value.Y)
			return ok && uint64(shift) < uint64(bits) && ps6108HelperExpression(pass, value.X, index, packed, bits, offsets)
		}
	}
	return false
}

func ps6108IndexOffset(pass *analysis.Pass, expression ast.Expr, index types.Object) (int64, bool) {
	expression = ps2110Unparen(expression)
	if ps6108ObjectExpression(pass, expression, index) {
		return 0, true
	}
	binary, ok := expression.(*ast.BinaryExpr)
	if !ok || binary.Op != token.ADD && binary.Op != token.SUB {
		return 0, false
	}
	if ps6108ObjectExpression(pass, binary.X, index) {
		constantValue, ok := ps6108IntConstant(pass, binary.Y)
		if !ok {
			return 0, false
		}
		if binary.Op == token.SUB {
			return ps6066SafeMul(constantValue, -1)
		}
		return constantValue, true
	}
	if binary.Op == token.ADD && ps6108ObjectExpression(pass, binary.Y, index) {
		return ps6108IntConstant(pass, binary.X)
	}
	return 0, false
}

func ps6108StoreStatement(pass *analysis.Pass, statement ast.Stmt, loopIndex types.Object, aliases map[types.Object]ps6108Affine) (ps6108Store, bool) {
	assignment, ok := statement.(*ast.AssignStmt)
	if !ok || assignment.Tok != token.ASSIGN || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
		return ps6108Store{}, false
	}
	indexed, ok := ps2110Unparen(assignment.Lhs[0]).(*ast.IndexExpr)
	if !ok {
		return ps6108Store{}, false
	}
	identifier, ok := ps2110Unparen(indexed.X).(*ast.Ident)
	if !ok {
		return ps6108Store{}, false
	}
	output := pass.TypesInfo.ObjectOf(identifier)
	index, ok := ps6108AffineExpression(pass, indexed.Index, loopIndex, aliases, map[types.Object]bool{})
	if !ok || output == nil {
		return ps6108Store{}, false
	}
	result := ps6108ConvertedResult(pass, assignment.Rhs[0])
	if result == nil {
		return ps6108Store{}, false
	}
	return ps6108Store{statement: assignment, output: output, index: index, result: result}, true
}

func ps6108ConvertedResult(pass *analysis.Pass, expression ast.Expr) types.Object {
	binary, ok := ps2110Unparen(expression).(*ast.BinaryExpr)
	if !ok || binary.Op != token.MUL {
		return nil
	}
	if result := ps6108ConversionResult(pass, binary.X); result != nil {
		return result
	}
	return ps6108ConversionResult(pass, binary.Y)
}

func ps6108ConversionResult(pass *analysis.Pass, expression ast.Expr) types.Object {
	call, ok := ps2110Unparen(expression).(*ast.CallExpr)
	if !ok || len(call.Args) != 1 || call.Ellipsis.IsValid() || !pass.TypesInfo.Types[call.Fun].IsType() {
		return nil
	}
	identifier, ok := ps2110Unparen(call.Args[0]).(*ast.Ident)
	if !ok {
		return nil
	}
	return pass.TypesInfo.Uses[identifier]
}

func ps6108StoreRHS(pass *analysis.Pass, expression ast.Expr, result, output types.Object, body *ast.BlockStmt) bool {
	array, ok := types.Unalias(output.Type()).Underlying().(*types.Array)
	if !ok || array.Len() <= 0 || !ps6108Float(array.Elem()) {
		return false
	}
	binary, ok := ps2110Unparen(expression).(*ast.BinaryExpr)
	if !ok || binary.Op != token.MUL {
		return false
	}
	conversion, scalar := binary.X, binary.Y
	if ps6108ConversionResult(pass, conversion) != result {
		conversion, scalar = binary.Y, binary.X
	}
	if ps6108ConversionResult(pass, conversion) != result || !types.Identical(pass.TypesInfo.TypeOf(conversion), array.Elem()) ||
		!types.Identical(pass.TypesInfo.TypeOf(scalar), array.Elem()) {
		return false
	}
	return ps6108StableScalar(pass, body, scalar)
}

func ps6108StableScalar(pass *analysis.Pass, body *ast.BlockStmt, expression ast.Expr) bool {
	expression = ps2110Unparen(expression)
	if pass.TypesInfo.Types[expression].Value != nil {
		return true
	}
	identifier, ok := expression.(*ast.Ident)
	if !ok {
		return false
	}
	object, ok := pass.TypesInfo.Uses[identifier].(*types.Var)
	if !ok || object.Parent() == pass.Pkg.Scope() {
		return false
	}
	parents := ps6087Parents(body)
	valid := true
	ast.Inspect(body, func(node ast.Node) bool {
		if !valid {
			return false
		}
		candidate, ok := node.(*ast.Ident)
		if !ok || pass.TypesInfo.Uses[candidate] != object && pass.TypesInfo.Defs[candidate] != object {
			return true
		}
		for parent := parents[candidate]; parent != nil; parent = parents[parent] {
			switch value := parent.(type) {
			case *ast.AssignStmt:
				for _, lhs := range value.Lhs {
					if ps6099NodeWithin(candidate, lhs) && pass.TypesInfo.Defs[candidate] == nil {
						valid = false
					}
				}
				return false
			case *ast.IncDecStmt:
				valid = false
				return false
			case *ast.UnaryExpr:
				if value.Op == token.AND {
					valid = false
				}
				return false
			}
		}
		return true
	})
	return valid
}

func ps6108CallGroups(calls []ps6108Call) (map[types.Object][]ps6108Call, []types.Object, bool) {
	groups := make(map[types.Object][]ps6108Call, len(calls))
	var order []types.Object
	for callIndex := range calls {
		call := &calls[callIndex]
		if _, exists := groups[call.source]; !exists {
			order = append(order, call.source)
		}
		groups[call.source] = append(groups[call.source], *call)
	}
	if len(order) == 2 {
		grouped := len(calls) == 4 && calls[0].source == order[0] && calls[1].source == order[0] &&
			calls[2].source == order[1] && calls[3].source == order[1]
		return groups, order, grouped && len(groups[order[0]]) == 2 && len(groups[order[1]]) == 2
	}
	return groups, order, len(order) == 1 && len(groups[order[0]]) == 2
}

func ps6108CallBounds(pass *analysis.Pass, loop ps6108Loop, call *ps6108Call, length int64) (int64, bool) {
	reads := int64(0)
	for iteration := int64(0); iteration < loop.count; iteration++ {
		index, ok := ps6108AffineAt(pass, call.index, iteration)
		if !ok {
			return 0, false
		}
		offsets := call.helper.highOffsets
		if index < call.helper.threshold {
			offsets = call.helper.lowOffsets
		}
		for _, offset := range offsets {
			position, exact := ps6066SafeAdd(index, offset)
			if !exact || position < 0 || position >= length {
				return 0, false
			}
			reads++
		}
	}
	return reads, true
}

func ps6108SourceInfo(pass *analysis.Pass, function *ast.FuncDecl, loop ps6108Loop, object types.Object) (ps6108Source, bool) {
	if !ps6108ByteSlice(object.Type()) || object.Parent() == pass.Pkg.Scope() {
		return ps6108Source{}, false
	}
	var result ps6108Source
	count := 0
	ast.Inspect(function.Body, func(node ast.Node) bool {
		if node == nil || node.Pos() >= loop.statement.Pos() {
			return node != nil
		}
		assignment, ok := node.(*ast.AssignStmt)
		if !ok || assignment.Tok != token.DEFINE || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
			return true
		}
		identifier, ok := assignment.Lhs[0].(*ast.Ident)
		if !ok || pass.TypesInfo.Defs[identifier] != object {
			return true
		}
		slice, ok := ps2110Unparen(assignment.Rhs[0]).(*ast.SliceExpr)
		if !ok || slice.High == nil || slice.Max == nil {
			return true
		}
		low := int64(0)
		if slice.Low != nil {
			var exact bool
			low, exact = ps6108IntConstant(pass, slice.Low)
			if !exact {
				return true
			}
		}
		high, highOK := ps6108IntConstant(pass, slice.High)
		maximum, maxOK := ps6108IntConstant(pass, slice.Max)
		baseIdentifier, baseOK := ps2110Unparen(slice.X).(*ast.Ident)
		if !highOK || !maxOK || maximum != high || high <= low || !baseOK {
			return true
		}
		base := pass.TypesInfo.ObjectOf(baseIdentifier)
		arrayLength, arrayOK := ps6108ByteArrayLength(pass.TypesInfo.TypeOf(baseIdentifier))
		if base == nil || base.Parent() == pass.Pkg.Scope() || !arrayOK || low < 0 || high > arrayLength {
			return true
		}
		result = ps6108Source{object: object, definition: assignment, base: base, length: high - low}
		count++
		return true
	})
	return result, count == 1
}

func ps6108SourceClosed(pass *analysis.Pass, body *ast.BlockStmt, source ps6108Source, allowed []ast.Expr) bool {
	valid := true
	ast.Inspect(body, func(node ast.Node) bool {
		if !valid {
			return false
		}
		identifier, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		object := pass.TypesInfo.Uses[identifier]
		if object == nil {
			object = pass.TypesInfo.Defs[identifier]
		}
		if object != source.object && object != source.base {
			return true
		}
		if pass.TypesInfo.Defs[identifier] == object {
			return true
		}
		if ps6099NodeWithin(identifier, source.definition) {
			return true
		}
		if object == source.object {
			for _, expression := range allowed {
				if ps6099NodeWithin(identifier, expression) {
					return true
				}
			}
		}
		valid = false
		return false
	})
	return valid
}

func ps6108OutputSlots(pass *analysis.Pass, loop ps6108Loop, store ps6108Store) ([]int64, bool) {
	array, ok := types.Unalias(store.output.Type()).Underlying().(*types.Array)
	if !ok || store.output.Parent() == pass.Pkg.Scope() || !ps6108Float(array.Elem()) {
		return nil, false
	}
	result := make([]int64, 0, loop.count)
	for iteration := int64(0); iteration < loop.count; iteration++ {
		slot, exact := ps6108AffineAt(pass, store.index, iteration)
		if !exact || slot < 0 || slot >= array.Len() {
			return nil, false
		}
		result = append(result, slot)
	}
	return result, true
}

func ps6108OutputGroupOrder(calls []ps6108Call, stores []ps6108Store) bool {
	groupOutput := make(map[types.Object]types.Object, len(calls))
	for callIndex := range calls {
		call := &calls[callIndex]
		for resultIndex := 0; resultIndex < 2; resultIndex++ {
			store := stores[callIndex*2+resultIndex]
			if existing := groupOutput[call.source]; existing != nil && existing != store.output {
				return false
			}
			groupOutput[call.source] = store.output
		}
	}
	seen := make(map[types.Object]types.Object, len(groupOutput))
	for source, output := range groupOutput {
		if previous := seen[output]; previous != nil && previous != source {
			return false
		}
		seen[output] = source
	}
	return true
}

func ps6108OutputClosed(pass *analysis.Pass, function *ast.FuncDecl, loop ps6108Loop, output types.Object, slots []int64, stores []ps6108Store, parents map[ast.Node]ast.Node, reachable map[ast.Node]bool) bool {
	definition := false
	read := make([]bool, len(slots))
	valid := true
	ast.Inspect(function.Body, func(node ast.Node) bool {
		if !valid {
			return false
		}
		identifier, ok := node.(*ast.Ident)
		if !ok || pass.TypesInfo.Uses[identifier] != output && pass.TypesInfo.Defs[identifier] != output {
			return true
		}
		if pass.TypesInfo.Defs[identifier] == output {
			if identifier.Pos() >= loop.statement.Pos() {
				valid = false
			} else {
				definition = true
			}
			return true
		}
		for _, store := range stores {
			if store.output == output && ps6099NodeWithin(identifier, store.statement.Lhs[0]) {
				return true
			}
		}
		parent := parents[identifier]
		for {
			if parentheses, ok := parent.(*ast.ParenExpr); ok {
				parent = parents[parentheses]
				continue
			}
			break
		}
		indexed, ok := parent.(*ast.IndexExpr)
		if !ok || indexed.X != identifier || indexed.Pos() <= loop.statement.End() {
			valid = false
			return false
		}
		slot, ok := ps6108IntConstant(pass, indexed.Index)
		position, present := slices.BinarySearch(slots, slot)
		if !ok || !present || ps6108AssignmentLHS(indexed, parents) ||
			!ps6108PureOutputRead(pass, indexed, function, parents, reachable) {
			valid = false
			return false
		}
		read[position] = true
		return true
	})
	if !valid || !definition {
		return false
	}
	for _, found := range read {
		if !found {
			return false
		}
	}
	return true
}

func ps6108PureOutputRead(pass *analysis.Pass, node ast.Node, function *ast.FuncDecl, parents map[ast.Node]ast.Node, reachable map[ast.Node]bool) bool {
	if !reachable[node] {
		return false
	}
	for parent := parents[node]; parent != nil; parent = parents[parent] {
		switch value := parent.(type) {
		case *ast.CallExpr, *ast.CompositeLit, *ast.FuncLit, *ast.GoStmt, *ast.DeferStmt,
			*ast.SendStmt, *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt, *ast.SwitchStmt,
			*ast.TypeSwitchStmt, *ast.SelectStmt, *ast.CaseClause, *ast.CommClause,
			*ast.IncDecStmt:
			return false
		case *ast.UnaryExpr:
			if value.Op == token.AND || value.Op == token.ARROW {
				return false
			}
		case *ast.ReturnStmt:
			if parents[value] != function.Body {
				return false
			}
			for _, expression := range value.Results {
				if ps6099NodeWithin(node, expression) {
					return ps6108PureReductionExpression(pass, expression)
				}
			}
			return false
		case *ast.AssignStmt:
			if parents[value] != function.Body || value.Tok != token.DEFINE || len(value.Lhs) != 1 || len(value.Rhs) != 1 ||
				!ps6099NodeWithin(node, value.Rhs[0]) || !ps6108PureReductionExpression(pass, value.Rhs[0]) {
				return false
			}
			identifier, ok := value.Lhs[0].(*ast.Ident)
			if !ok || identifier.Name == "_" {
				return false
			}
			return ps6108ReductionReturned(pass, function, pass.TypesInfo.Defs[identifier], value, parents, reachable)
		case ast.Stmt:
			return false
		}
	}
	return false
}

// ps6108PureReductionExpression recognizes only arithmetic over constant
// indexes of private fixed floating arrays. This is deliberately smaller than
// Go's side-effect-free expression set: it proves coefficient work reaches a
// returned value without calls, conditional execution, or hidden capture.
func ps6108PureReductionExpression(pass *analysis.Pass, expression ast.Expr) bool {
	expression = ps2110Unparen(expression)
	if pass.TypesInfo.Types[expression].Value != nil {
		return true
	}
	switch value := expression.(type) {
	case *ast.IndexExpr:
		identifier, ok := ps2110Unparen(value.X).(*ast.Ident)
		if !ok {
			return false
		}
		object := pass.TypesInfo.ObjectOf(identifier)
		if object == nil || object.Parent() == pass.Pkg.Scope() {
			return false
		}
		array, ok := types.Unalias(object.Type()).Underlying().(*types.Array)
		index, exact := ps6108IntConstant(pass, value.Index)
		return ok && ps6108Float(array.Elem()) && exact && index >= 0 && index < array.Len()
	case *ast.UnaryExpr:
		return (value.Op == token.ADD || value.Op == token.SUB) && ps6108PureReductionExpression(pass, value.X)
	case *ast.BinaryExpr:
		switch value.Op {
		case token.ADD, token.SUB, token.MUL, token.QUO:
			return ps6108PureReductionExpression(pass, value.X) && ps6108PureReductionExpression(pass, value.Y)
		}
	}
	return false
}

func ps6108ReductionReturned(pass *analysis.Pass, function *ast.FuncDecl, object types.Object, definition ast.Node, parents map[ast.Node]ast.Node, reachable map[ast.Node]bool) bool {
	if object == nil {
		return false
	}
	uses := 0
	valid := true
	ast.Inspect(function.Body, func(node ast.Node) bool {
		if !valid {
			return false
		}
		identifier, ok := node.(*ast.Ident)
		if !ok || pass.TypesInfo.Uses[identifier] != object && pass.TypesInfo.Defs[identifier] != object {
			return true
		}
		if pass.TypesInfo.Defs[identifier] == object {
			valid = ps6099NodeWithin(identifier, definition)
			return valid
		}
		uses++
		if !reachable[identifier] {
			valid = false
			return false
		}
		parent := parents[identifier]
		for {
			parentheses, ok := parent.(*ast.ParenExpr)
			if !ok {
				break
			}
			parent = parents[parentheses]
		}
		returned, ok := parent.(*ast.ReturnStmt)
		valid = ok && parents[returned] == function.Body
		return valid
	})
	return valid && uses == 1
}

func ps6108AssignmentLHS(node ast.Node, parents map[ast.Node]ast.Node) bool {
	for parent := parents[node]; parent != nil; parent = parents[parent] {
		assignment, ok := parent.(*ast.AssignStmt)
		if !ok {
			continue
		}
		for _, lhs := range assignment.Lhs {
			if ps6099NodeWithin(node, lhs) {
				return true
			}
		}
		return false
	}
	return false
}

func ps6108ResultsClosed(pass *analysis.Pass, body *ast.BlockStmt, allowed map[types.Object]ast.Expr) bool {
	counts := make(map[types.Object]int)
	valid := true
	ast.Inspect(body, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		object := pass.TypesInfo.Uses[identifier]
		if object == nil || allowed[object] == nil {
			return true
		}
		if !ps6099NodeWithin(identifier, allowed[object]) {
			valid = false
			return false
		}
		counts[object]++
		return true
	})
	if !valid {
		return false
	}
	for object := range allowed {
		if counts[object] != 1 {
			return false
		}
	}
	return true
}

func ps6108AliasesClosed(pass *analysis.Pass, body *ast.BlockStmt, aliases map[types.Object]ps6108Affine, calls []ps6108Call, stores []ps6108Store) bool {
	allowed := make([]ast.Expr, 0, len(aliases)+len(calls)+len(stores))
	ast.Inspect(body, func(node ast.Node) bool {
		assignment, ok := node.(*ast.AssignStmt)
		if !ok || assignment.Tok != token.DEFINE || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
			return true
		}
		identifier, ok := assignment.Lhs[0].(*ast.Ident)
		if ok {
			if _, proven := aliases[pass.TypesInfo.Defs[identifier]]; proven {
				allowed = append(allowed, assignment.Rhs[0])
			}
		}
		return true
	})
	for callIndex := range calls {
		call := &calls[callIndex]
		allowed = append(allowed, call.call.Args[call.helper.indexPosition])
	}
	for _, store := range stores {
		allowed = append(allowed, store.statement.Lhs[0])
	}
	for object := range aliases {
		valid := true
		ast.Inspect(body, func(node ast.Node) bool {
			identifier, ok := node.(*ast.Ident)
			if !ok || pass.TypesInfo.Uses[identifier] != object && pass.TypesInfo.Defs[identifier] != object {
				return true
			}
			if pass.TypesInfo.Defs[identifier] == object {
				return true
			}
			for _, expression := range allowed {
				if ps6099NodeWithin(identifier, expression) {
					return true
				}
			}
			valid = false
			return false
		})
		if !valid {
			return false
		}
	}
	return true
}

func ps6108AffineExpression(pass *analysis.Pass, expression ast.Expr, loopIndex types.Object, aliases map[types.Object]ps6108Affine, seen map[types.Object]bool) (ps6108Affine, bool) {
	expression = ps2110Unparen(expression)
	if integer, ok := ps6108IntConstant(pass, expression); ok {
		return ps6108Affine{constant: integer}, true
	}
	switch value := expression.(type) {
	case *ast.Ident:
		object := pass.TypesInfo.ObjectOf(value)
		if object == loopIndex {
			return ps6108Affine{coefficient: 1}, true
		}
		alias, ok := aliases[object]
		if !ok || seen[object] {
			return ps6108Affine{}, false
		}
		seen[object] = true
		delete(seen, object)
		return alias, true
	case *ast.UnaryExpr:
		if value.Op != token.ADD && value.Op != token.SUB {
			return ps6108Affine{}, false
		}
		result, ok := ps6108AffineExpression(pass, value.X, loopIndex, aliases, seen)
		if !ok || value.Op == token.ADD {
			return result, ok
		}
		return ps6108ScaleAffine(result, -1)
	case *ast.BinaryExpr:
		switch value.Op {
		case token.ADD, token.SUB:
			left, leftOK := ps6108AffineExpression(pass, value.X, loopIndex, aliases, seen)
			right, rightOK := ps6108AffineExpression(pass, value.Y, loopIndex, aliases, seen)
			if !leftOK || !rightOK {
				return ps6108Affine{}, false
			}
			if value.Op == token.SUB {
				var ok bool
				right, ok = ps6108ScaleAffine(right, -1)
				if !ok {
					return ps6108Affine{}, false
				}
			}
			return ps6108AddAffine(left, right)
		case token.MUL:
			if factor, ok := ps6108IntConstant(pass, value.X); ok {
				term, valid := ps6108AffineExpression(pass, value.Y, loopIndex, aliases, seen)
				if !valid {
					return ps6108Affine{}, false
				}
				return ps6108ScaleAffine(term, factor)
			}
			if factor, ok := ps6108IntConstant(pass, value.Y); ok {
				term, valid := ps6108AffineExpression(pass, value.X, loopIndex, aliases, seen)
				if !valid {
					return ps6108Affine{}, false
				}
				return ps6108ScaleAffine(term, factor)
			}
		}
	}
	return ps6108Affine{}, false
}

func ps6108AddAffine(left, right ps6108Affine) (ps6108Affine, bool) {
	coefficient, coefficientOK := ps6066SafeAdd(left.coefficient, right.coefficient)
	constantValue, constantOK := ps6066SafeAdd(left.constant, right.constant)
	return ps6108Affine{coefficient: coefficient, constant: constantValue}, coefficientOK && constantOK
}

func ps6108ScaleAffine(value ps6108Affine, factor int64) (ps6108Affine, bool) {
	coefficient, coefficientOK := ps6066SafeMul(value.coefficient, factor)
	constantValue, constantOK := ps6066SafeMul(value.constant, factor)
	return ps6108Affine{coefficient: coefficient, constant: constantValue}, coefficientOK && constantOK
}

func ps6108AffineAt(pass *analysis.Pass, affine ps6108Affine, iteration int64) (int64, bool) {
	product, ok := ps6066SafeMul(affine.coefficient, iteration)
	if !ok {
		return 0, false
	}
	result, ok := ps6066SafeAdd(product, affine.constant)
	if !ok || !ps6108TargetInt(pass, result) {
		return 0, false
	}
	return result, true
}

func ps6108IntConstant(pass *analysis.Pass, expression ast.Expr) (int64, bool) {
	value := pass.TypesInfo.Types[ps2110Unparen(expression)].Value
	if value == nil || value.Kind() != constant.Int {
		return 0, false
	}
	return constant.Int64Val(value)
}

func ps6108PositiveIntConstant(pass *analysis.Pass, expression ast.Expr) (int64, bool) {
	value, ok := ps6108IntConstant(pass, expression)
	return value, ok && value > 0 && ps6108TargetInt(pass, value)
}

func ps6108NonnegativeIntConstant(pass *analysis.Pass, expression ast.Expr) (int64, bool) {
	value, ok := ps6108IntConstant(pass, expression)
	return value, ok && value >= 0
}

func ps6108TargetInt(pass *analysis.Pass, value int64) bool {
	bytes := pass.TypesSizes.Sizeof(types.Typ[types.Int])
	if bytes <= 0 || bytes > 8 {
		return false
	}
	if bytes == 8 {
		return true
	}
	maximum := int64(1)<<(uint(bytes)*8-1) - 1
	minimum := -maximum - 1
	return value >= minimum && value <= maximum
}

func ps6108ObjectExpression(pass *analysis.Pass, expression ast.Expr, object types.Object) bool {
	identifier, ok := ps2110Unparen(expression).(*ast.Ident)
	return ok && pass.TypesInfo.ObjectOf(identifier) == object
}

func ps6108ExactInt(value types.Type) bool {
	basic, ok := types.Unalias(value).Underlying().(*types.Basic)
	return ok && basic.Kind() == types.Int
}

func ps6108ByteSlice(value types.Type) bool {
	slice, ok := types.Unalias(value).Underlying().(*types.Slice)
	if !ok {
		return false
	}
	basic, ok := types.Unalias(slice.Elem()).Underlying().(*types.Basic)
	return ok && basic.Kind() == types.Uint8
}

func ps6108ByteArrayLength(value types.Type) (int64, bool) {
	value = types.Unalias(value)
	if pointer, ok := value.(*types.Pointer); ok {
		value = types.Unalias(pointer.Elem())
	}
	array, ok := value.Underlying().(*types.Array)
	if !ok {
		return 0, false
	}
	basic, ok := types.Unalias(array.Elem()).Underlying().(*types.Basic)
	return array.Len(), ok && basic.Kind() == types.Uint8
}

func ps6108FixedUnsignedBits(value types.Type) (uint, bool) {
	basic, ok := types.Unalias(value).Underlying().(*types.Basic)
	if !ok {
		return 0, false
	}
	switch basic.Kind() {
	case types.Uint8:
		return 8, true
	case types.Uint16:
		return 16, true
	case types.Uint32:
		return 32, true
	case types.Uint64:
		return 64, true
	}
	return 0, false
}

func ps6108Float(value types.Type) bool {
	basic, ok := types.Unalias(value).Underlying().(*types.Basic)
	return ok && (basic.Kind() == types.Float32 || basic.Kind() == types.Float64)
}

func ps6108UnsafeOrReflect(pass *analysis.Pass, body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(node ast.Node) bool {
		if found {
			return false
		}
		identifier, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		packageName, ok := pass.TypesInfo.Uses[identifier].(*types.PkgName)
		if ok && packageName.Imported() != nil && (packageName.Imported().Path() == "unsafe" || packageName.Imported().Path() == "reflect") {
			found = true
			return false
		}
		return true
	})
	return found
}
