package checks

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"slices"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/cfg"

	"github.com/jxsl13/perfscan/lint"
)

// PS6093 implements owner issue #904. It finds slice indexes whose loop trip
// count comes from an unrelated, size-like method on a non-slice receiver.
var PS6093 = register(&lint.Check{
	ID:       "PS6093",
	Category: "access",
	Slug:     "method-sized-loop-needs-slice-length-proof",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "a method-sized loop indexes a slice without a source-visible length proof",
		Text: `A tensor, image, codec, or columnar loop can take its trip count
from metadata such as Numel, Len, or Size and then index one or more raw
storage slices. Even when the program's runtime invariant says that the
metadata and storage agree, that relationship is not visible to Go's bounds
check elimination. The loop can therefore retain a bounds check for every
indexed slice on every iteration.

PS6093 reports a slice only when the loop bound is a local identifier defined
by an exact typed method call named Numel, Len, or Size on a statically
non-slice receiver, the loop induction variable reaches that slice through a
non-negative affine index, and no source-visible proof establishes the
required length. Integer-range loops and canonical zero-based counted loops
are covered. Named and constrained generic slice types are resolved through
go/types.

For the overflow-free identity index i, the check recognizes a dominating
zero-based reslice or make definition whose extent is exactly the method
bound, a dominating len(slice) comparison (including value-preserving integer
conversions of len), a terminating inverse guard, and a direct loop or
enclosing-if index guard. Algebraically comparable affine extents are not
treated as proofs without a source-visible range that excludes machine-integer
overflow. Distinct affine accesses are reported separately.

For package-level slice headers, one-time guards and reslices are not accepted
because pointer aliases can be established outside the analyzed function. A
direct per-iteration length guard remains usable only when no header write,
indirect store, synchronization, or intervening non-builtin call can stale it.
Copy or validate the storage view into a local slice for a durable proof.
Synchronization includes sends, receives, close, channel ranges (including
constrained channel types), and range-over-function iterator execution.

The check remains silent for slice-receiver Len methods, non-canonical loops,
constant indexes, unstable bound or slice headers (including implicit pointer-
receiver mutation), captured/address-taken slice headers, dead CFG blocks and
constant-empty ranges, empty generic receiver type sets, and indexes that
occur only inside a nested function body whose invocation timing is not
inferred.

The finding is advisory. A source-only analyzer cannot prove that metadata
equals storage length, choose which slice owns the contract, or preserve panic
timing by inserting a reslice. Establish that invariant at the API boundary,
reslice each relevant storage view once to the required exclusive extent, and
range over one proven slice. For affine indexes other than i, prove the affine
maximum rather than blindly slicing to the metadata count.`,
		Before: `n := tensor.Numel()
for i := range n {
	out[i] = input[i] * scale
}`,
		After: `n := tensor.Numel()
input = input[:n] // validate the runtime shape/storage invariant once
out = out[:n]
for i := range out {
	out[i] = input[i] * scale
}`,
		MeasuredWin: `GoAI reference tensor kernels on Apple M2 Pro with Go
1.27 and GOMAXPROCS=1 measured 1.2955x for F64 Add 4K, 1.6529x for F32 Add
4K, 1.4152x for F64 ReLU 64K, and 1.1405x for F64 Tanh 64K after hoisting
storage-length checks. Allocations and bytes were unchanged, and registered
unary/binary outputs remained bit-identical, including NaNs, infinities,
signed zero, and F32 widen-compute-narrow behavior.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6093",
		Doc:  "method-sized loop indexes a slice without a source-visible length proof",
		Run:  runPS6093,
	},
})

var ps6093SizeMethods = map[string]bool{
	"Len":   true,
	"Numel": true,
	"Size":  true,
}

const ps6093AffineLimit int64 = 1 << 30

type ps6093Affine struct {
	coefficient int64
	offset      int64
}

type ps6093Loop struct {
	statement ast.Stmt
	body      *ast.BlockStmt
	condition ast.Expr
	index     types.Object
	bound     types.Object
	indexName string
	boundName string
}

type ps6093Bound struct {
	call   *ast.CallExpr
	method string
	start  token.Pos
}

type ps6093LoopBound struct {
	loop  ps6093Loop
	bound ps6093Bound
}

type ps6093ProofPathKey struct {
	proof       *cfg.Block
	access      *cfg.Block
	proofEnd    token.Pos
	accessStart token.Pos
}

type ps6093BlockPair struct {
	before *cfg.Block
	after  *cfg.Block
}

type ps6093BoolResult struct {
	value bool
	known bool
}

type ps6093ProofPaths struct {
	results      map[ps6093ProofPathKey]bool
	dominance    map[ps6093BlockPair]bool
	predecessors map[*cfg.Block][]*cfg.Block
	blocks       map[token.Pos]*cfg.Block
	nodeOrder    map[token.Pos]int
	live         map[token.Pos]bool
	booleans     map[ast.Expr]ps6093BoolResult
	remaining    int
	spent        int
	exhausted    bool
}

func runPS6093(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		var bodies []*ast.BlockStmt
		ast.Inspect(file, func(node ast.Node) bool {
			switch value := node.(type) {
			case *ast.FuncDecl:
				if value.Body != nil {
					bodies = append(bodies, value.Body)
				}
			case *ast.FuncLit:
				if value.Body != nil {
					bodies = append(bodies, value.Body)
				}
			}
			return true
		})
		for _, body := range bodies {
			ps6093Function(pass, body)
		}
	}
	return nil, nil
}

func ps6093Function(pass *analysis.Pass, body *ast.BlockStmt) {
	parents := ps6087Parents(body)
	graph := cfg.New(body, func(call *ast.CallExpr) bool { return !ps6079PanicCall(pass, call) })
	proofPaths := ps6093ProofPaths{
		results:      make(map[ps6093ProofPathKey]bool),
		dominance:    make(map[ps6093BlockPair]bool),
		predecessors: make(map[*cfg.Block][]*cfg.Block, len(graph.Blocks)),
		blocks:       make(map[token.Pos]*cfg.Block),
		nodeOrder:    make(map[token.Pos]int),
		live:         make(map[token.Pos]bool),
		booleans:     make(map[ast.Expr]ps6093BoolResult),
	}
	for _, block := range graph.Blocks {
		ps6093IndexBlockNodes(&proofPaths, block)
		for _, successor := range block.Succs {
			proofPaths.predecessors[successor] = append(proofPaths.predecessors[successor], block)
		}
	}
	definitions := make(map[types.Object]*ast.Ident)
	syntaxNodes := 0
	ast.Inspect(body, func(node ast.Node) bool {
		if node != nil {
			syntaxNodes++
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		identifier, ok := node.(*ast.Ident)
		if ok {
			if object := pass.TypesInfo.Defs[identifier]; object != nil {
				definitions[object] = identifier
			}
		}
		return true
	})
	proofPaths.remaining = ps6093ProofBudget(syntaxNodes, len(graph.Blocks))

	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		loop, ok := ps6093LoopHeader(pass, node)
		if !ok {
			return true
		}
		unsafe, complete := ps6093ObjectUnsafeBudgeted(pass, loop.body, parents, &proofPaths, loop.index, loop.body.Pos(), loop.body.End())
		if !complete || unsafe {
			return true
		}
		if loop.condition != nil {
			unsafe, complete = ps6093ObjectUnsafeBudgeted(pass, loop.condition, parents, &proofPaths, loop.index, loop.condition.Pos(), loop.condition.End())
		}
		if !complete || unsafe {
			return true
		}
		bounds := ps6093LoopMethodBounds(pass, loop, definitions, parents)
		safeBounds := bounds[:0]
		for index := range bounds {
			candidate := &bounds[index]
			unsafe, complete := ps6093ObjectUnsafeBudgeted(pass, body, parents, &proofPaths, candidate.loop.bound, candidate.bound.start, candidate.loop.statement.End())
			if !complete {
				// Exhaustion must not certify a proof. Retain all unknown
				// candidates so the reporting path stays conservative.
				safeBounds = append(safeBounds, bounds[index:]...)
				break
			}
			if !unsafe {
				safeBounds = append(safeBounds, *candidate)
			}
		}
		bounds = safeBounds
		if len(bounds) == 0 {
			return true
		}
		ps6093ReportLoop(pass, body, graph, parents, &proofPaths, bounds)
		return true
	})
}

func ps6093LoopHeader(pass *analysis.Pass, node ast.Node) (ps6093Loop, bool) {
	switch loop := node.(type) {
	case *ast.RangeStmt:
		if loop.Body == nil || loop.Value != nil || loop.Tok != token.DEFINE {
			return ps6093Loop{}, false
		}
		indexIdentifier, ok := ps2110Unparen(loop.Key).(*ast.Ident)
		if !ok || indexIdentifier.Name == "_" {
			return ps6093Loop{}, false
		}
		boundIdentifier, ok := ps2110Unparen(loop.X).(*ast.Ident)
		if !ok {
			return ps6093Loop{}, false
		}
		index := pass.TypesInfo.Defs[indexIdentifier]
		if index == nil {
			index = pass.TypesInfo.Uses[indexIdentifier]
		}
		bound := pass.TypesInfo.Uses[boundIdentifier]
		if index == nil || bound == nil || !ps6091Integer(bound.Type()) {
			return ps6093Loop{}, false
		}
		return ps6093Loop{statement: loop, body: loop.Body, index: index, bound: bound, indexName: indexIdentifier.Name, boundName: boundIdentifier.Name}, true
	case *ast.ForStmt:
		if loop.Body == nil || loop.Init == nil || loop.Cond == nil || loop.Post == nil {
			return ps6093Loop{}, false
		}
		initialization, ok := loop.Init.(*ast.AssignStmt)
		if !ok || initialization.Tok != token.DEFINE || len(initialization.Lhs) != 1 || len(initialization.Rhs) != 1 || !ps6091ConstantInteger(pass, initialization.Rhs[0], 0) {
			return ps6093Loop{}, false
		}
		indexIdentifier, ok := ps2110Unparen(initialization.Lhs[0]).(*ast.Ident)
		if !ok || indexIdentifier.Name == "_" {
			return ps6093Loop{}, false
		}
		index := pass.TypesInfo.Defs[indexIdentifier]
		if index == nil {
			index = pass.TypesInfo.Uses[indexIdentifier]
		}
		post, ok := loop.Post.(*ast.IncDecStmt)
		postIdentifier, postOK := ps2110Unparen(ps6093PostX(post)).(*ast.Ident)
		if !ok || !postOK || post.Tok != token.INC || pass.TypesInfo.Uses[postIdentifier] != index {
			return ps6093Loop{}, false
		}
		bound, boundName, ok := ps6093ConditionBound(pass, loop.Cond, index)
		if !ok || bound == nil || !ps6091Integer(bound.Type()) {
			return ps6093Loop{}, false
		}
		return ps6093Loop{statement: loop, body: loop.Body, condition: loop.Cond, index: index, bound: bound, indexName: indexIdentifier.Name, boundName: boundName}, true
	}
	return ps6093Loop{}, false
}

func ps6093PostX(statement *ast.IncDecStmt) ast.Expr {
	if statement == nil {
		return nil
	}
	return statement.X
}

func ps6093ConditionBound(pass *analysis.Pass, expression ast.Expr, index types.Object) (types.Object, string, bool) {
	var object types.Object
	var name string
	found := ps6093VisitConditionBounds(pass, expression, index, func(candidate types.Object, candidateName string) bool {
		object = candidate
		name = candidateName
		return true
	})
	return object, name, found
}

func ps6093LoopMethodBounds(pass *analysis.Pass, loop ps6093Loop, definitions map[types.Object]*ast.Ident, parents map[ast.Node]ast.Node) []ps6093LoopBound {
	if loop.condition == nil {
		bound, ok := ps6093MethodBound(pass, definitions[loop.bound], parents)
		if !ok {
			return nil
		}
		return []ps6093LoopBound{{loop: loop, bound: bound}}
	}
	var result []ps6093LoopBound
	ps6093VisitConditionBounds(pass, loop.condition, loop.index, func(object types.Object, name string) bool {
		bound, ok := ps6093MethodBound(pass, definitions[object], parents)
		if !ok {
			return false
		}
		candidate := loop
		candidate.bound = object
		candidate.boundName = name
		result = append(result, ps6093LoopBound{loop: candidate, bound: bound})
		return false
	})
	return result
}

func ps6093VisitConditionBounds(pass *analysis.Pass, expression ast.Expr, index types.Object, visit func(types.Object, string) bool) bool {
	expression = ps2110Unparen(expression)
	if binary, ok := expression.(*ast.BinaryExpr); ok && binary.Op == token.LAND {
		if ps6093VisitConditionBounds(pass, binary.X, index, visit) {
			return true
		}
		return ps6093VisitConditionBounds(pass, binary.Y, index, visit)
	}
	binary, ok := expression.(*ast.BinaryExpr)
	if !ok {
		return false
	}
	left, leftOK := ps2110Unparen(binary.X).(*ast.Ident)
	right, rightOK := ps2110Unparen(binary.Y).(*ast.Ident)
	if binary.Op == token.LSS && leftOK && pass.TypesInfo.Uses[left] == index && rightOK {
		object := pass.TypesInfo.Uses[right]
		return object != nil && visit(object, right.Name)
	}
	if binary.Op == token.GTR && rightOK && pass.TypesInfo.Uses[right] == index && leftOK {
		object := pass.TypesInfo.Uses[left]
		return object != nil && visit(object, left.Name)
	}
	return false
}

func ps6093MethodBound(pass *analysis.Pass, definition *ast.Ident, parents map[ast.Node]ast.Node) (ps6093Bound, bool) {
	if definition == nil {
		return ps6093Bound{}, false
	}
	var rhs ast.Expr
	switch parent := parents[definition].(type) {
	case *ast.AssignStmt:
		if len(parent.Lhs) != 1 || len(parent.Rhs) != 1 || ps2110Unparen(parent.Lhs[0]) != definition {
			return ps6093Bound{}, false
		}
		rhs = parent.Rhs[0]
	case *ast.ValueSpec:
		if len(parent.Names) != 1 || len(parent.Values) != 1 || parent.Names[0] != definition {
			return ps6093Bound{}, false
		}
		rhs = parent.Values[0]
	default:
		return ps6093Bound{}, false
	}
	call, ok := ps2110Unparen(rhs).(*ast.CallExpr)
	if !ok || !ps6091Integer(pass.TypesInfo.TypeOf(call)) {
		return ps6093Bound{}, false
	}
	selector, ok := ps2110Unparen(call.Fun).(*ast.SelectorExpr)
	if !ok {
		return ps6093Bound{}, false
	}
	selection := pass.TypesInfo.Selections[selector]
	method, ok := selectionObject(selection).(*types.Func)
	if !ok || !ps6093SizeMethods[method.Name()] || ps6093SliceReceiver(selection.Recv()) {
		return ps6093Bound{}, false
	}
	switch selection.Kind() {
	case types.MethodVal:
		if len(call.Args) != 0 {
			return ps6093Bound{}, false
		}
	case types.MethodExpr:
		if len(call.Args) != 1 {
			return ps6093Bound{}, false
		}
	default:
		return ps6093Bound{}, false
	}
	return ps6093Bound{call: call, method: method.Name(), start: definition.Pos()}, true
}

func ps6093SliceReceiver(value types.Type) bool {
	value = types.Unalias(value)
	if pointer, ok := value.Underlying().(*types.Pointer); ok {
		return ps6093SliceReceiver(pointer.Elem())
	}
	if _, ok := value.Underlying().(*types.Slice); ok {
		return true
	}
	parameter, ok := value.(*types.TypeParam)
	if !ok {
		return false
	}
	constraint, ok := types.Unalias(parameter.Constraint()).Underlying().(*types.Interface)
	if !ok {
		return true
	}
	set, ok := ps6091ConstraintTypeSet(constraint, make(map[types.Type]bool))
	if !ok || set.unrestricted || len(set.terms) == 0 {
		return true
	}
	feasibleTerms := 0
	for _, term := range set.terms {
		feasible, known := ps6091TypeTermFeasible(term, constraint, parameter)
		if !known {
			return true
		}
		if !feasible {
			continue
		}
		feasibleTerms++
		if ps6093SliceReceiver(term.value) {
			return true
		}
	}
	return feasibleTerms == 0
}

func ps6093ReportLoop(pass *analysis.Pass, body *ast.BlockStmt, graph *cfg.CFG, parents map[ast.Node]ast.Node, proofPaths *ps6093ProofPaths, bounds []ps6093LoopBound) {
	loop := bounds[0].loop
	reported := make(map[types.Object]map[ps6093Affine]bool)
	ast.Inspect(loop.body, func(node ast.Node) bool {
		if node != nil && !ps6093ProofWork(proofPaths) {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		index, ok := node.(*ast.IndexExpr)
		if !ok || !ps6093PositionLive(proofPaths, index.Pos()) || !ps6093ConstantPathLive(pass, parents, proofPaths, index) {
			return true
		}
		eligible := make([]ps6093LoopBound, 0, len(bounds))
		for boundIndex := range bounds {
			candidate := &bounds[boundIndex]
			dominates, complete := ps6093GraphPositionDominates(graph, proofPaths, candidate.bound.call.Pos(), index.Pos())
			if !complete {
				// Preserve unknown candidates: the exhausted proof budget will
				// make every subsequent proof attempt decline safely.
				eligible = append(eligible, bounds[boundIndex:]...)
				break
			}
			if dominates {
				eligible = append(eligible, *candidate)
			}
		}
		if len(eligible) == 0 {
			return true
		}
		sequence, ok := ps2110Unparen(index.X).(*ast.Ident)
		if !ok {
			return true
		}
		object := pass.TypesInfo.Uses[sequence]
		if object == nil || !ps6093SliceType(object.Type()) {
			return true
		}
		affine, ok := ps6093AffineExpression(pass, index.Index, loop.index)
		if !ok || affine.coefficient <= 0 || affine.offset < 0 {
			return true
		}
		seen := reported[object]
		if seen != nil && seen[affine] {
			return true
		}
		packageSlice := ps6093PackageVariable(object)
		if !packageSlice {
			escapes, complete := ps6093HeaderEscapesBudgeted(pass, body, parents, proofPaths, object)
			if !complete {
				return false
			}
			unsafe, complete := ps6093ObjectUnsafeBudgeted(pass, body, parents, proofPaths, object, eligible[0].bound.call.End(), index.Pos())
			if !complete {
				return false
			}
			if escapes || unsafe {
				return true
			}
		}
		if !packageSlice {
			unsafe, complete := ps6093ObjectUnsafeBudgeted(pass, loop.body, parents, proofPaths, object, loop.body.Pos(), loop.body.End())
			if !complete {
				return false
			}
			if unsafe {
				return true
			}
		}
		if slices.ContainsFunc(eligible, func(candidate ps6093LoopBound) bool {
			return ps6093LengthProven(pass, body, graph, parents, proofPaths, candidate.loop, object, index, affine)
		}) {
			return true
		}
		if seen == nil {
			seen = make(map[ps6093Affine]bool)
			reported[object] = seen
		}
		seen[affine] = true
		candidate := eligible[0]
		pass.Reportf(index.Pos(), "slice %s is indexed by %s in a loop bounded by %s from non-slice .%s(); no dominating source-level proof relates len(%s) to the required affine extent — validate/reslice this storage view once and range over a proven slice (advisory, no automatic fix)", sequence.Name, ps6093AffineText(loop.indexName, affine), candidate.loop.boundName, candidate.bound.method, sequence.Name)
		return true
	})
}

func ps6093SliceType(value types.Type) bool {
	value = types.Unalias(value)
	if _, ok := value.Underlying().(*types.Slice); ok {
		return true
	}
	parameter, ok := value.(*types.TypeParam)
	if !ok {
		return false
	}
	constraint, ok := types.Unalias(parameter.Constraint()).Underlying().(*types.Interface)
	if !ok {
		return false
	}
	set, ok := ps6091ConstraintTypeSet(constraint, make(map[types.Type]bool))
	if !ok || set.unrestricted || len(set.terms) == 0 {
		return false
	}
	feasible := 0
	for _, term := range set.terms {
		possible, known := ps6091TypeTermFeasible(term, constraint, parameter)
		if !known {
			return false
		}
		if !possible {
			continue
		}
		feasible++
		if _, slice := types.Unalias(term.value).Underlying().(*types.Slice); !slice {
			return false
		}
	}
	return feasible > 0
}

func ps6093LengthProven(pass *analysis.Pass, body *ast.BlockStmt, graph *cfg.CFG, parents map[ast.Node]ast.Node, proofPaths *ps6093ProofPaths, loop ps6093Loop, slice types.Object, access *ast.IndexExpr, indexAffine ps6093Affine) bool {
	// Algebraically equivalent affine extents can overflow at machine integer
	// width even when their coefficients compare favorably. Without a proven
	// range for the method result, only the identity index has an overflow-free
	// bound-to-length relation.
	if indexAffine != (ps6093Affine{coefficient: 1}) {
		return false
	}
	proved, complete := ps6093ConditionGuarantees(pass, proofPaths, loop.condition, true, func(binary *ast.BinaryExpr, truth bool) bool {
		return ps6093DirectIndexProof(pass, binary, truth, loop.index, slice, indexAffine) ||
			ps6093DirectExtentProof(pass, binary, truth, loop.bound, slice, ps6093RequiredExtent(indexAffine))
	}, func(binary *ast.BinaryExpr) (bool, bool) {
		return ps6093PackageGuardStable(pass, body, graph, parents, proofPaths, loop, slice, binary, access)
	})
	if !complete {
		return false
	}
	if proved {
		return true
	}
	for node := ast.Node(access); node != nil; node = parents[node] {
		if !ps6093ProofWork(proofPaths) {
			return false
		}
		if statement, ok := node.(*ast.IfStmt); ok {
			truth, contains := ps6093IfBranch(statement, access.Pos())
			if !contains {
				continue
			}
			proved, complete := ps6093ConditionGuarantees(pass, proofPaths, statement.Cond, truth, func(binary *ast.BinaryExpr, value bool) bool {
				return ps6093DirectIndexProof(pass, binary, value, loop.index, slice, indexAffine) ||
					ps6093DirectExtentProof(pass, binary, value, loop.bound, slice, ps6093RequiredExtent(indexAffine))
			}, func(binary *ast.BinaryExpr) (bool, bool) {
				return ps6093PackageGuardStable(pass, body, graph, parents, proofPaths, loop, slice, binary, access)
			})
			if !complete {
				return false
			}
			if proved {
				return true
			}
		}
	}
	if ps6093PriorGuardProof(pass, body, graph, parents, proofPaths, access, loop, slice, indexAffine) {
		return true
	}
	return ps6093ResliceProof(pass, body, graph, parents, proofPaths, loop, slice, access, indexAffine)
}

func ps6093PriorGuardProof(pass *analysis.Pass, body *ast.BlockStmt, graph *cfg.CFG, parents map[ast.Node]ast.Node, proofPaths *ps6093ProofPaths, access ast.Node, loop ps6093Loop, slice types.Object, indexAffine ps6093Affine) bool {
	proved := false
	exhausted := false
	ast.Inspect(body, func(node ast.Node) bool {
		if proved || exhausted {
			return false
		}
		if node != nil && !ps6093ProofWork(proofPaths) {
			exhausted = true
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		guard, ok := node.(*ast.IfStmt)
		if !ok || guard.Pos() <= access.Pos() && access.End() <= guard.End() || !ps6093TerminatesBeforeAccess(pass, parents, proofPaths, guard.Body, access) {
			return true
		}
		dominates, complete := ps6093GraphPositionDominates(graph, proofPaths, guard.Cond.Pos(), access.Pos())
		if !complete {
			exhausted = true
			return false
		}
		if !dominates {
			dominates, complete = ps6093ConstantSwitchGuardDominates(pass, parents, proofPaths, guard, access)
			if !complete {
				exhausted = true
				return false
			}
		}
		if !dominates {
			return true
		}
		proved, complete = ps6093ConditionGuarantees(pass, proofPaths, guard.Cond, false, func(binary *ast.BinaryExpr, truth bool) bool {
			return ps6093DirectIndexProof(pass, binary, truth, loop.index, slice, indexAffine) ||
				ps6093DirectExtentProof(pass, binary, truth, loop.bound, slice, ps6093RequiredExtent(indexAffine))
		}, func(binary *ast.BinaryExpr) (bool, bool) {
			return ps6093PackageGuardStable(pass, body, graph, parents, proofPaths, loop, slice, binary, access)
		})
		if !complete {
			exhausted = true
			return false
		}
		return !proved
	})
	return proved && !exhausted
}

func ps6093PackageGuardStable(pass *analysis.Pass, body *ast.BlockStmt, graph *cfg.CFG, parents map[ast.Node]ast.Node, proofPaths *ps6093ProofPaths, loop ps6093Loop, slice types.Object, proof *ast.BinaryExpr, access ast.Node) (bool, bool) {
	if !ps6093PackageVariable(slice) {
		return true, true
	}
	// Source-order effect windows cannot represent a forward jump to a proof
	// followed by a backward jump to an earlier access. Decline such package
	// proofs instead of constructing an inverted interval that misses writes.
	if proof == nil || access == nil || proof.End() > access.Pos() {
		return false, true
	}
	recurring, complete := ps6093RecurringProofSafe(graph, proofPaths, loop, proof, access)
	if !complete || !recurring {
		return false, complete
	}
	return ps6093PackageProofStableOnPaths(pass, body, parents, proofPaths, slice, proof, access, recurring)
}

func ps6093IfBranch(statement *ast.IfStmt, position token.Pos) (bool, bool) {
	if statement.Body != nil && statement.Body.Pos() <= position && position <= statement.Body.End() {
		return true, true
	}
	if statement.Else != nil && statement.Else.Pos() <= position && position <= statement.Else.End() {
		return false, true
	}
	return false, false
}

func ps6093ConstantPathLive(pass *analysis.Pass, parents map[ast.Node]ast.Node, paths *ps6093ProofPaths, node ast.Node) bool {
	for parent := parents[node]; parent != nil; parent = parents[parent] {
		if !ps6093ProofWork(paths) {
			return false
		}
		switch statement := parent.(type) {
		case *ast.IfStmt:
			value, known, complete := ps6093KnownBool(pass, paths, statement.Cond)
			if !complete {
				return false
			}
			if !known {
				continue
			}
			branch, contained := ps6093IfBranch(statement, node.Pos())
			if contained && branch != value {
				return false
			}
		case *ast.ForStmt:
			value, known, complete := ps6093KnownBool(pass, paths, statement.Cond)
			if !complete || known && !value && statement.Body.Pos() <= node.Pos() && node.End() <= statement.Body.End() {
				return false
			}
		case *ast.RangeStmt:
			if ps6093RangeStaticallyEmpty(pass, statement.X) && statement.Body.Pos() <= node.Pos() && node.End() <= statement.Body.End() {
				return false
			}
		case *ast.SwitchStmt:
			live, known, complete := ps6093SwitchPathLive(pass, paths, statement, node.Pos())
			if !complete || known && !live {
				return false
			}
		}
	}
	return true
}

func ps6093RangeStaticallyEmpty(pass *analysis.Pass, expression ast.Expr) bool {
	if value := pass.TypesInfo.Types[expression].Value; value != nil {
		switch value.Kind() {
		case constant.Int:
			return constant.Sign(value) <= 0
		case constant.String:
			return constant.StringVal(value) == ""
		}
	}
	valueType := types.Unalias(pass.TypesInfo.TypeOf(expression))
	if valueType == nil {
		return false
	}
	if pointer, ok := valueType.Underlying().(*types.Pointer); ok {
		valueType = types.Unalias(pointer.Elem())
	}
	array, ok := valueType.Underlying().(*types.Array)
	return ok && array.Len() == 0
}

func ps6093SwitchPathLive(pass *analysis.Pass, paths *ps6093ProofPaths, statement *ast.SwitchStmt, position token.Pos) (bool, bool, bool) {
	selected, known, complete := ps6093SwitchSelectedCase(pass, paths, statement)
	if !complete || !known {
		return false, false, complete
	}
	target := -1
	for index, node := range statement.Body.List {
		if !ps6093ProofWork(paths) {
			return false, false, false
		}
		clause, ok := node.(*ast.CaseClause)
		if !ok {
			return false, false, true
		}
		if clause.Pos() <= position && position <= clause.End() {
			target = index
		}
	}
	if target < 0 {
		return false, false, true
	}
	if selected < 0 || selected > target {
		return false, true, true
	}
	fallsThrough, complete := ps6093SwitchFallsThrough(paths, statement, selected, target)
	return fallsThrough, true, complete
}

func ps6093SwitchSelectedCase(pass *analysis.Pass, paths *ps6093ProofPaths, statement *ast.SwitchStmt) (int, bool, bool) {
	tag := constant.MakeBool(true)
	if statement.Tag != nil {
		tag = pass.TypesInfo.Types[statement.Tag].Value
		if tag == nil {
			return -1, false, true
		}
	}
	selected := -1
	defaultCase := -1
	for index, node := range statement.Body.List {
		if !ps6093ProofWork(paths) {
			return -1, false, false
		}
		clause, ok := node.(*ast.CaseClause)
		if !ok {
			return -1, false, true
		}
		if clause.List == nil {
			defaultCase = index
			continue
		}
		for _, expression := range clause.List {
			if !ps6093ProofWork(paths) {
				return -1, false, false
			}
			value := pass.TypesInfo.Types[expression].Value
			if value == nil {
				return -1, false, true
			}
			if selected < 0 && constant.Compare(tag, token.EQL, value) {
				selected = index
			}
		}
	}
	if selected < 0 {
		selected = defaultCase
	}
	return selected, true, true
}

func ps6093SwitchFallsThrough(paths *ps6093ProofPaths, statement *ast.SwitchStmt, from, to int) (bool, bool) {
	if from < 0 || from > to {
		return false, true
	}
	for index := from; index < to; index++ {
		if !ps6093ProofWork(paths) {
			return false, false
		}
		clause := statement.Body.List[index].(*ast.CaseClause)
		if len(clause.Body) == 0 {
			return false, true
		}
		branch, ok := clause.Body[len(clause.Body)-1].(*ast.BranchStmt)
		if !ok || branch.Tok != token.FALLTHROUGH {
			return false, true
		}
	}
	return true, true
}

func ps6093ConstantSwitchGuardDominates(pass *analysis.Pass, parents map[ast.Node]ast.Node, paths *ps6093ProofPaths, guard *ast.IfStmt, access ast.Node) (bool, bool) {
	guardCase, guardIndex, guardSwitch, complete := ps6093EnclosingSwitchCase(parents, paths, guard)
	if !complete {
		return false, false
	}
	accessCase, accessIndex, accessSwitch, complete := ps6093EnclosingSwitchCase(parents, paths, access)
	if !complete {
		return false, false
	}
	if guardCase == nil || accessCase == nil || guardSwitch == nil || guardSwitch != accessSwitch || parents[guard] != guardCase || guard.Pos() >= access.Pos() {
		return false, true
	}
	selected, known, complete := ps6093SwitchSelectedCase(pass, paths, guardSwitch)
	if !complete || !known || selected < 0 || selected > guardIndex {
		return false, complete
	}
	fallsThrough, complete := ps6093SwitchFallsThrough(paths, guardSwitch, selected, accessIndex)
	if !complete || !fallsThrough {
		return false, complete
	}
	unsafeJump := false
	complete = true
	ast.Inspect(guardSwitch.Body, func(node ast.Node) bool {
		if unsafeJump || !complete {
			return false
		}
		if node != nil && !ps6093ProofWork(paths) {
			complete = false
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		switch value := node.(type) {
		case *ast.BranchStmt:
			unsafeJump = value.Tok == token.GOTO
		case *ast.LabeledStmt:
			unsafeJump = true
		}
		return !unsafeJump
	})
	return !unsafeJump && complete, complete
}

func ps6093EnclosingSwitchCase(parents map[ast.Node]ast.Node, paths *ps6093ProofPaths, node ast.Node) (*ast.CaseClause, int, *ast.SwitchStmt, bool) {
	var clause *ast.CaseClause
	for parent := parents[node]; parent != nil; parent = parents[parent] {
		if !ps6093ProofWork(paths) {
			return nil, -1, nil, false
		}
		if current, ok := parent.(*ast.CaseClause); ok && clause == nil {
			clause = current
		}
		statement, ok := parent.(*ast.SwitchStmt)
		if !ok || clause == nil {
			continue
		}
		for index, candidate := range statement.Body.List {
			if !ps6093ProofWork(paths) {
				return nil, -1, nil, false
			}
			if candidate == clause {
				return clause, index, statement, true
			}
		}
		return nil, -1, nil, true
	}
	return nil, -1, nil, true
}

func ps6093KnownBool(pass *analysis.Pass, paths *ps6093ProofPaths, expression ast.Expr) (bool, bool, bool) {
	if expression == nil {
		return false, false, true
	}
	if paths == nil {
		return false, false, false
	}
	if paths.booleans == nil {
		paths.booleans = make(map[ast.Expr]ps6093BoolResult)
	}
	original := expression
	if result, ok := paths.booleans[original]; ok {
		return result.value, result.known, true
	}
	if !ps6093ProofWork(paths) {
		return false, false, false
	}
	expression = ps2110Unparen(expression)
	if value := pass.TypesInfo.Types[expression].Value; value != nil && value.Kind() == constant.Bool {
		result := ps6093BoolResult{value: constant.BoolVal(value), known: true}
		paths.booleans[original] = result
		return result.value, result.known, true
	}
	if unary, ok := expression.(*ast.UnaryExpr); ok && unary.Op == token.NOT {
		value, known, complete := ps6093KnownBool(pass, paths, unary.X)
		if complete {
			paths.booleans[original] = ps6093BoolResult{value: !value, known: known}
		}
		return !value, known, complete
	}
	binary, ok := expression.(*ast.BinaryExpr)
	if !ok || (binary.Op != token.LAND && binary.Op != token.LOR) {
		paths.booleans[original] = ps6093BoolResult{}
		return false, false, true
	}
	left, leftKnown, complete := ps6093KnownBool(pass, paths, binary.X)
	if !complete {
		return false, false, false
	}
	right, rightKnown, complete := ps6093KnownBool(pass, paths, binary.Y)
	if !complete {
		return false, false, false
	}
	result := ps6093BoolResult{}
	if binary.Op == token.LAND {
		if leftKnown && !left || rightKnown && !right {
			result.known = true
			paths.booleans[original] = result
			return result.value, result.known, true
		}
		if leftKnown && rightKnown {
			result = ps6093BoolResult{value: left && right, known: true}
			paths.booleans[original] = result
			return result.value, result.known, true
		}
		paths.booleans[original] = result
		return false, false, true
	}
	if leftKnown && left || rightKnown && right {
		result = ps6093BoolResult{value: true, known: true}
		paths.booleans[original] = result
		return result.value, result.known, true
	}
	if leftKnown && rightKnown {
		result = ps6093BoolResult{value: left || right, known: true}
		paths.booleans[original] = result
		return result.value, result.known, true
	}
	paths.booleans[original] = result
	return false, false, true
}

func ps6093ConditionGuarantees(pass *analysis.Pass, paths *ps6093ProofPaths, expression ast.Expr, truth bool, direct func(*ast.BinaryExpr, bool) bool, proofStable func(*ast.BinaryExpr) (bool, bool)) (bool, bool) {
	if expression == nil {
		return false, true
	}
	if !ps6093ProofWork(paths) {
		return false, false
	}
	expression = ps2110Unparen(expression)
	if unary, ok := expression.(*ast.UnaryExpr); ok && unary.Op == token.NOT {
		return ps6093ConditionGuarantees(pass, paths, unary.X, !truth, direct, proofStable)
	}
	if binary, ok := expression.(*ast.BinaryExpr); ok {
		switch binary.Op {
		case token.LAND:
			if truth {
				proved, complete := ps6093ConditionGuarantees(pass, paths, binary.X, true, direct, proofStable)
				if !complete || proved {
					return proved, complete
				}
				return ps6093ConditionGuarantees(pass, paths, binary.Y, true, direct, proofStable)
			}
			left, complete := ps6093ConditionGuarantees(pass, paths, binary.X, false, direct, proofStable)
			if !complete || !left {
				return false, complete
			}
			right, complete := ps6093ConditionGuarantees(pass, paths, binary.Y, false, direct, proofStable)
			return left && right, complete
		case token.LOR:
			if truth {
				left, complete := ps6093ConditionGuarantees(pass, paths, binary.X, true, direct, proofStable)
				if !complete || !left {
					return false, complete
				}
				right, complete := ps6093ConditionGuarantees(pass, paths, binary.Y, true, direct, proofStable)
				return left && right, complete
			}
			proved, complete := ps6093ConditionGuarantees(pass, paths, binary.X, false, direct, proofStable)
			if !complete || proved {
				return proved, complete
			}
			return ps6093ConditionGuarantees(pass, paths, binary.Y, false, direct, proofStable)
		default:
			if !direct(binary, truth) {
				return false, true
			}
			return proofStable(binary)
		}
	}
	return false, true
}

func ps6093ProofWork(paths *ps6093ProofPaths) bool {
	if paths == nil || paths.remaining == 0 {
		if paths != nil {
			paths.exhausted = true
		}
		return false
	}
	paths.remaining--
	paths.spent++
	return true
}

func ps6093ProofBudget(syntaxNodes, blocks int) int {
	return max(1024, 32*(syntaxNodes+blocks))
}

func ps6093GraphPositionDominates(graph *cfg.CFG, paths *ps6093ProofPaths, before, after token.Pos) (bool, bool) {
	if graph == nil || paths == nil || len(graph.Blocks) == 0 {
		return false, true
	}
	if !ps6093ProofWork(paths) {
		return false, false
	}
	beforeBlock := paths.blocks[before]
	afterBlock := paths.blocks[after]
	if beforeBlock == nil || afterBlock == nil || !beforeBlock.Live || !afterBlock.Live {
		return false, true
	}
	if beforeBlock == afterBlock {
		return before < after, true
	}
	key := ps6093BlockPair{before: beforeBlock, after: afterBlock}
	if result, ok := paths.dominance[key]; ok {
		return result, true
	}
	if paths.dominance == nil {
		paths.dominance = make(map[ps6093BlockPair]bool)
	}
	seen := map[*cfg.Block]bool{graph.Blocks[0]: true}
	queue := []*cfg.Block{graph.Blocks[0]}
	for len(queue) > 0 {
		if !ps6093ProofWork(paths) {
			return false, false
		}
		block := queue[0]
		queue = queue[1:]
		if block == beforeBlock {
			continue
		}
		if block == afterBlock {
			paths.dominance[key] = false
			return false, true
		}
		for _, successor := range block.Succs {
			if !seen[successor] {
				seen[successor] = true
				queue = append(queue, successor)
			}
		}
	}
	paths.dominance[key] = true
	return true, true
}

func ps6093DirectIndexProof(pass *analysis.Pass, binary *ast.BinaryExpr, truth bool, index, slice types.Object, required ps6093Affine) bool {
	op := ps6093TruthOperator(binary.Op, truth)
	left, leftAffine := ps6093AffineExpression(pass, binary.X, index)
	right, rightAffine := ps6093AffineExpression(pass, binary.Y, index)
	leftLen := ps6093LenObject(pass, binary.X) == slice
	rightLen := ps6093LenObject(pass, binary.Y) == slice
	if rightLen && leftAffine && left == required {
		return op == token.LSS
	}
	if leftLen && rightAffine && right == required {
		return op == token.GTR
	}
	if adjusted, ok := ps6093LenMinusOne(pass, binary.Y, slice); ok && adjusted && leftAffine && left == required {
		return op == token.LEQ
	}
	if adjusted, ok := ps6093LenMinusOne(pass, binary.X, slice); ok && adjusted && rightAffine && right == required {
		return op == token.GEQ
	}
	return false
}

func ps6093LenMinusOne(pass *analysis.Pass, expression ast.Expr, slice types.Object) (bool, bool) {
	binary, ok := ps2110Unparen(expression).(*ast.BinaryExpr)
	if !ok || binary.Op != token.SUB || !ps6091ConstantInteger(pass, binary.Y, 1) {
		return false, false
	}
	return ps6093LenObject(pass, binary.X) == slice, true
}

func ps6093DirectExtentProof(pass *analysis.Pass, binary *ast.BinaryExpr, truth bool, bound, slice types.Object, required ps6093Affine) bool {
	op := ps6093TruthOperator(binary.Op, truth)
	if ps6093LenObject(pass, binary.X) == slice {
		provided, ok := ps6093AffineExpression(pass, binary.Y, bound)
		if !ok || provided != required {
			return false
		}
		switch op {
		case token.GEQ, token.GTR, token.EQL:
			return true
		}
	}
	if ps6093LenObject(pass, binary.Y) == slice {
		provided, ok := ps6093AffineExpression(pass, binary.X, bound)
		if !ok || provided != required {
			return false
		}
		switch op {
		case token.LEQ, token.LSS, token.EQL:
			return true
		}
	}
	return false
}

func ps6093TruthOperator(operator token.Token, truth bool) token.Token {
	if truth {
		return operator
	}
	switch operator {
	case token.LSS:
		return token.GEQ
	case token.LEQ:
		return token.GTR
	case token.GTR:
		return token.LEQ
	case token.GEQ:
		return token.LSS
	case token.EQL:
		return token.NEQ
	case token.NEQ:
		return token.EQL
	}
	return token.ILLEGAL
}

func ps6093LenObject(pass *analysis.Pass, expression ast.Expr) types.Object {
	expression = ps2110Unparen(expression)
	for {
		conversion, ok := expression.(*ast.CallExpr)
		if !ok || len(conversion.Args) != 1 || !pass.TypesInfo.Types[conversion.Fun].IsType() {
			break
		}
		if !ps6093LenConversionPreserves(pass, pass.TypesInfo.TypeOf(conversion)) {
			return nil
		}
		expression = ps2110Unparen(conversion.Args[0])
	}
	call, ok := expression.(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return nil
	}
	identifier, ok := ps2110Unparen(call.Fun).(*ast.Ident)
	if !ok {
		return nil
	}
	builtin, ok := pass.TypesInfo.Uses[identifier].(*types.Builtin)
	if !ok || builtin.Name() != "len" {
		return nil
	}
	argument, ok := ps2110Unparen(call.Args[0]).(*ast.Ident)
	if !ok {
		return nil
	}
	return pass.TypesInfo.Uses[argument]
}

func ps6093LenConversionPreserves(pass *analysis.Pass, target types.Type) bool {
	if pass.TypesSizes == nil || target == nil {
		return false
	}
	basic, ok := types.Unalias(target).Underlying().(*types.Basic)
	if !ok || basic.Info()&types.IsInteger == 0 {
		return false
	}
	return pass.TypesSizes.Sizeof(target) >= pass.TypesSizes.Sizeof(types.Typ[types.Int])
}

func ps6093ResliceProof(pass *analysis.Pass, body *ast.BlockStmt, graph *cfg.CFG, parents map[ast.Node]ast.Node, proofPaths *ps6093ProofPaths, loop ps6093Loop, slice types.Object, access *ast.IndexExpr, indexAffine ps6093Affine) bool {
	// A package slice can have pointer aliases established in other files or
	// initializers. A local reslice/make cannot prove that such an alias leaves
	// the header stable; require a direct per-iteration length guard instead.
	if ps6093PackageVariable(slice) {
		return false
	}
	required := ps6093RequiredExtent(indexAffine)
	proved := false
	exhausted := false
	ast.Inspect(body, func(node ast.Node) bool {
		if proved || exhausted {
			return false
		}
		if node != nil && !ps6093ProofWork(proofPaths) {
			exhausted = true
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		var lhs *ast.Ident
		var rhs ast.Expr
		switch value := node.(type) {
		case *ast.AssignStmt:
			if len(value.Lhs) != 1 || len(value.Rhs) != 1 {
				return true
			}
			lhs, _ = ps2110Unparen(value.Lhs[0]).(*ast.Ident)
			rhs = value.Rhs[0]
		case *ast.ValueSpec:
			if len(value.Names) != 1 || len(value.Values) != 1 {
				return true
			}
			lhs = value.Names[0]
			rhs = value.Values[0]
		default:
			return true
		}
		if lhs == nil || ps6093IdentObject(pass, lhs) != slice {
			return true
		}
		definition, extentExpression, ok := ps6093DefinedExtent(pass, rhs)
		if !ok {
			return true
		}
		dominates, complete := ps6093GraphPositionDominates(graph, proofPaths, definition.Pos(), access.Pos())
		if !complete {
			exhausted = true
			return false
		}
		if !dominates {
			return true
		}
		extent, ok := ps6093AffineExpression(pass, extentExpression, loop.bound)
		if !ok || !ps6093AffineAtLeast(extent, required) {
			return true
		}
		unsafe, complete := ps6093ObjectUnsafeBudgeted(pass, body, parents, proofPaths, slice, definition.End(), access.Pos())
		if !complete {
			exhausted = true
			return false
		}
		if unsafe {
			return true
		}
		proved = true
		return false
	})
	return proved && !exhausted
}

func ps6093DefinedExtent(pass *analysis.Pass, expression ast.Expr) (ast.Node, ast.Expr, bool) {
	expression = ps2110Unparen(expression)
	if reslice, ok := expression.(*ast.SliceExpr); ok {
		if reslice.High == nil || reslice.Low != nil && !ps6091ConstantInteger(pass, reslice.Low, 0) {
			return nil, nil, false
		}
		return reslice, reslice.High, true
	}
	call, ok := expression.(*ast.CallExpr)
	if !ok || len(call.Args) < 2 || len(call.Args) > 3 {
		return nil, nil, false
	}
	identifier, ok := ps2110Unparen(call.Fun).(*ast.Ident)
	if !ok {
		return nil, nil, false
	}
	builtin, ok := pass.TypesInfo.Uses[identifier].(*types.Builtin)
	if !ok || builtin.Name() != "make" {
		return nil, nil, false
	}
	return call, call.Args[1], true
}

func ps6093IdentObject(pass *analysis.Pass, identifier *ast.Ident) types.Object {
	if object := pass.TypesInfo.Defs[identifier]; object != nil {
		return object
	}
	return pass.TypesInfo.Uses[identifier]
}

func ps6093RequiredExtent(index ps6093Affine) ps6093Affine {
	return ps6093Affine{coefficient: index.coefficient, offset: index.offset - index.coefficient + 1}
}

func ps6093AffineAtLeast(provided, required ps6093Affine) bool {
	return provided == required
}

func ps6093AffineExpression(pass *analysis.Pass, expression ast.Expr, variable types.Object) (ps6093Affine, bool) {
	if expression == nil {
		return ps6093Affine{}, false
	}
	expression = ps2110Unparen(expression)
	if identifier, ok := expression.(*ast.Ident); ok && pass.TypesInfo.Uses[identifier] == variable {
		return ps6093Affine{coefficient: 1}, true
	}
	if value := pass.TypesInfo.Types[expression].Value; value != nil && value.Kind() == constant.Int {
		integer, exact := constant.Int64Val(value)
		if exact && ps6093AffineValid(integer) {
			return ps6093Affine{offset: integer}, true
		}
		return ps6093Affine{}, false
	}
	switch value := expression.(type) {
	case *ast.UnaryExpr:
		affine, ok := ps6093AffineExpression(pass, value.X, variable)
		if !ok {
			return ps6093Affine{}, false
		}
		switch value.Op {
		case token.ADD:
			return affine, true
		case token.SUB:
			return ps6093ScaleAffine(affine, -1)
		}
	case *ast.BinaryExpr:
		left, leftOK := ps6093AffineExpression(pass, value.X, variable)
		right, rightOK := ps6093AffineExpression(pass, value.Y, variable)
		if !leftOK || !rightOK {
			return ps6093Affine{}, false
		}
		switch value.Op {
		case token.ADD:
			return ps6093AddAffine(left, right)
		case token.SUB:
			right, ok := ps6093ScaleAffine(right, -1)
			if !ok {
				return ps6093Affine{}, false
			}
			return ps6093AddAffine(left, right)
		case token.MUL:
			switch {
			case left.coefficient == 0:
				return ps6093ScaleAffine(right, left.offset)
			case right.coefficient == 0:
				return ps6093ScaleAffine(left, right.offset)
			}
		}
	}
	return ps6093Affine{}, false
}

func ps6093AddAffine(left, right ps6093Affine) (ps6093Affine, bool) {
	result := ps6093Affine{coefficient: left.coefficient + right.coefficient, offset: left.offset + right.offset}
	return result, ps6093AffineValid(result.coefficient) && ps6093AffineValid(result.offset)
}

func ps6093ScaleAffine(value ps6093Affine, scale int64) (ps6093Affine, bool) {
	if !ps6093AffineValid(scale) || value.coefficient != 0 && !ps6093AffineValid(value.coefficient*scale) || value.offset != 0 && !ps6093AffineValid(value.offset*scale) {
		return ps6093Affine{}, false
	}
	result := ps6093Affine{coefficient: value.coefficient * scale, offset: value.offset * scale}
	return result, ps6093AffineValid(result.coefficient) && ps6093AffineValid(result.offset)
}

func ps6093AffineValid(value int64) bool {
	return -ps6093AffineLimit <= value && value <= ps6093AffineLimit
}

func ps6093AffineText(name string, affine ps6093Affine) string {
	if affine.coefficient == 1 && affine.offset == 0 {
		return name
	}
	if affine.offset == 0 {
		return fmt.Sprintf("%d*%s", affine.coefficient, name)
	}
	sign := "+"
	offset := affine.offset
	if offset < 0 {
		sign = "-"
		offset = -offset
	}
	return fmt.Sprintf("%d*%s%s%d", affine.coefficient, name, sign, offset)
}

func ps6093ObjectUnsafeBudgeted(pass *analysis.Pass, root ast.Node, parents map[ast.Node]ast.Node, paths *ps6093ProofPaths, object types.Object, start, end token.Pos) (bool, bool) {
	unsafe := false
	complete := true
	ast.Inspect(root, func(node ast.Node) bool {
		if unsafe {
			return false
		}
		if literal, ok := node.(*ast.FuncLit); ok && ps6093UninvokedFuncLiteral(pass, literal, parents) {
			return false
		}
		if node != nil && paths != nil && !ps6093ProofWork(paths) {
			complete = false
			return false
		}
		identifier, ok := node.(*ast.Ident)
		if !ok || identifier.Pos() < start || identifier.Pos() > end || pass.TypesInfo.Uses[identifier] != object {
			return true
		}
		if ps6093DirectLValue(identifier, parents) {
			unsafe = true
			return false
		}
		if ps6093ImplicitPointerReceiver(pass, identifier, parents) {
			unsafe = true
			return false
		}
		if ps6093AddressTaken(identifier, parents) {
			unsafe = true
			return false
		}
		return true
	})
	return unsafe, complete
}

func ps6093UninvokedFuncLiteral(pass *analysis.Pass, literal *ast.FuncLit, parents map[ast.Node]ast.Node) bool {
	if pass == nil || literal == nil {
		return false
	}
	var expression ast.Expr = literal
	for {
		switch parent := parents[expression].(type) {
		case *ast.ParenExpr:
			if parent.X != expression {
				return false
			}
			expression = parent
		case *ast.CallExpr:
			if len(parent.Args) != 1 || parent.Args[0] != expression || parent.Ellipsis.IsValid() ||
				!pass.TypesInfo.Types[parent.Fun].IsType() {
				return false
			}
			expression = parent
		default:
			binary, ok := parent.(*ast.BinaryExpr)
			if !ok || binary.Op != token.EQL && binary.Op != token.NEQ {
				return false
			}
			other := binary.X
			if binary.X == expression {
				other = binary.Y
			} else if binary.Y != expression {
				return false
			}
			identifier, ok := ps2110Unparen(other).(*ast.Ident)
			return ok && pass.TypesInfo.Uses[identifier] == types.Universe.Lookup("nil")
		}
	}
}

func ps6093HeaderEscapesBudgeted(pass *analysis.Pass, body *ast.BlockStmt, parents map[ast.Node]ast.Node, paths *ps6093ProofPaths, object types.Object) (bool, bool) {
	escapes := false
	complete := true
	ast.Inspect(body, func(node ast.Node) bool {
		if escapes || !complete {
			return false
		}
		if node != nil && !ps6093ProofWork(paths) {
			complete = false
			return false
		}
		identifier, ok := node.(*ast.Ident)
		if !ok || pass.TypesInfo.Uses[identifier] != object {
			return true
		}
		if ps6093AddressTaken(identifier, parents) {
			escapes = true
			return false
		}
		if ps6093ImplicitPointerReceiver(pass, identifier, parents) {
			escapes = true
			return false
		}
		if !ps6093DirectLValue(identifier, parents) {
			return true
		}
		for parent := parents[identifier]; parent != nil; parent = parents[parent] {
			if !ps6093ProofWork(paths) {
				complete = false
				return false
			}
			if _, capturedWrite := parent.(*ast.FuncLit); capturedWrite {
				escapes = true
				return false
			}
		}
		return true
	})
	return escapes, complete
}

func ps6093ImplicitPointerReceiver(pass *analysis.Pass, identifier *ast.Ident, parents map[ast.Node]ast.Node) bool {
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
		_, pointer := types.Unalias(signature.Recv().Type()).Underlying().(*types.Pointer)
		return pointer
	}
}

func ps6093DirectLValue(identifier *ast.Ident, parents map[ast.Node]ast.Node) bool {
	parent := ps6093ParentAfterParens(identifier, parents)
	switch value := parent.(type) {
	case *ast.AssignStmt:
		for _, expression := range value.Lhs {
			if ps2110Unparen(expression) == identifier {
				return true
			}
		}
	case *ast.IncDecStmt:
		return ps2110Unparen(value.X) == identifier
	case *ast.RangeStmt:
		return ps2110Unparen(value.Key) == identifier || ps2110Unparen(value.Value) == identifier
	}
	return false
}

func ps6093AddressTaken(identifier *ast.Ident, parents map[ast.Node]ast.Node) bool {
	unary, ok := ps6093ParentAfterParens(identifier, parents).(*ast.UnaryExpr)
	return ok && unary.Op == token.AND && ps2110Unparen(unary.X) == identifier
}

func ps6093ParentAfterParens(node ast.Node, parents map[ast.Node]ast.Node) ast.Node {
	for parent := parents[node]; parent != nil; parent = parents[node] {
		parenthesis, ok := parent.(*ast.ParenExpr)
		if !ok {
			return parent
		}
		node = parenthesis
	}
	return nil
}

func ps6093PackageVariable(object types.Object) bool {
	return object != nil && object.Pkg() != nil && object.Parent() == object.Pkg().Scope()
}

func ps6093PackageProofStableOnPaths(pass *analysis.Pass, body *ast.BlockStmt, parents map[ast.Node]ast.Node, paths *ps6093ProofPaths, object types.Object, proof, access ast.Node, recurring bool) (bool, bool) {
	proofBlock := paths.blocks[proof.Pos()]
	accessBlock := paths.blocks[access.Pos()]
	if proofBlock == nil || accessBlock == nil || !proofBlock.Live || !accessBlock.Live {
		return false, true
	}
	reachable, complete := ps6093BlocksReachableBeforeAccess(paths, proofBlock, accessBlock, recurring)
	if !complete {
		return false, false
	}
	var stop *cfg.Block
	if recurring {
		stop = proofBlock
	}
	canReachAccess, complete := ps6093BlocksReaching(paths, accessBlock, stop)
	if !complete || !canReachAccess[proofBlock] {
		return false, complete
	}
	accessCycles := false
	if !recurring {
		for _, successor := range accessBlock.Succs {
			if canReachAccess[successor] {
				accessCycles = true
				break
			}
		}
	}
	stable := ps6093PackageEffectsStable(pass, body, parents, paths, object, func(node ast.Node) bool {
		block := ps6093EffectBlock(paths, parents, node)
		if block == nil {
			return false
		}
		if !reachable[block] || !canReachAccess[block] {
			return true
		}
		if block == proofBlock && ps6093BlockExecutesBefore(paths, block, node, proof, parents) {
			return true
		}
		if block == accessBlock && ps6093BlockExecutesAfter(paths, block, node, access, parents) && (recurring || !accessCycles) {
			return true
		}
		return false
	})
	return stable, !paths.exhausted
}

func ps6093PackageEffectsStable(
	pass *analysis.Pass,
	body ast.Node,
	parents map[ast.Node]ast.Node,
	paths *ps6093ProofPaths,
	object types.Object,
	stableEffect func(ast.Node) bool,
) bool {
	stable := true
	complete := true
	ast.Inspect(body, func(node ast.Node) bool {
		if !stable || !complete {
			return false
		}
		if node != nil && !ps6093ProofWork(paths) {
			complete = false
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		if !ps6093PackageEffect(pass, node, parents, object) {
			return true
		}
		stable = stableEffect(node)
		return stable
	})
	return stable && complete
}

func ps6093BlocksReachableBeforeAccess(paths *ps6093ProofPaths, proofBlock, accessBlock *cfg.Block, stopAtAccess bool) (map[*cfg.Block]bool, bool) {
	reachable := map[*cfg.Block]bool{proofBlock: true}
	queue := []*cfg.Block{proofBlock}
	for len(queue) > 0 {
		if !ps6093ProofWork(paths) {
			return nil, false
		}
		block := queue[0]
		queue = queue[1:]
		if stopAtAccess && block == accessBlock {
			continue
		}
		for _, successor := range block.Succs {
			if !reachable[successor] {
				reachable[successor] = true
				queue = append(queue, successor)
			}
		}
	}
	return reachable, true
}

func ps6093BlocksReaching(paths *ps6093ProofPaths, target, stop *cfg.Block) (map[*cfg.Block]bool, bool) {
	reaching := map[*cfg.Block]bool{target: true}
	queue := []*cfg.Block{target}
	for len(queue) > 0 {
		if !ps6093ProofWork(paths) {
			return nil, false
		}
		block := queue[0]
		queue = queue[1:]
		if block == stop {
			continue
		}
		for _, predecessor := range paths.predecessors[block] {
			if !reaching[predecessor] {
				reaching[predecessor] = true
				queue = append(queue, predecessor)
			}
		}
	}
	return reaching, true
}

func ps6093PackageEffect(pass *analysis.Pass, node ast.Node, parents map[ast.Node]ast.Node, object types.Object) bool {
	switch value := node.(type) {
	case *ast.Ident:
		return pass.TypesInfo.Uses[value] == object && ps6093DirectLValue(value, parents)
	case *ast.AssignStmt:
		return ps6093IndirectTargets(value.Lhs)
	case *ast.IncDecStmt:
		return ps6093IndirectTargets([]ast.Expr{value.X})
	case *ast.RangeStmt:
		return ps6093IndirectTargets([]ast.Expr{value.Key, value.Value}) ||
			ps6093RangeHasImplicitEffect(pass, pass.TypesInfo.TypeOf(value.X))
	case *ast.SendStmt:
		return true
	case *ast.UnaryExpr:
		return value.Op == token.ARROW
	case *ast.CallExpr:
		if statement, deferred := parents[value].(*ast.DeferStmt); deferred && statement.Call == value {
			return false
		}
		if pass.TypesInfo.Types[value.Fun].IsType() {
			return false
		}
		if identifier, ok := ps2110Unparen(value.Fun).(*ast.Ident); ok {
			if builtin, ok := pass.TypesInfo.Uses[identifier].(*types.Builtin); ok {
				return builtin.Name() == "close"
			}
		}
		return true
	}
	return false
}

func ps6093IndirectTargets(targets []ast.Expr) bool {
	for _, target := range targets {
		if target != nil {
			if _, indirect := ps2110Unparen(target).(*ast.StarExpr); indirect {
				return true
			}
		}
	}
	return false
}

func ps6093EffectBlock(paths *ps6093ProofPaths, parents map[ast.Node]ast.Node, node ast.Node) *cfg.Block {
	if statement, ok := node.(*ast.RangeStmt); ok {
		return ps6093BlockAt(paths, statement.X.Pos())
	}
	if block := ps6093BlockAt(paths, node.Pos()); block != nil {
		return block
	}
	for parent := parents[node]; parent != nil; parent = parents[parent] {
		if !ps6093ProofWork(paths) {
			return nil
		}
		if statement, ok := parent.(*ast.RangeStmt); ok {
			return ps6093BlockAt(paths, statement.X.Pos())
		}
	}
	var block *cfg.Block
	ast.Inspect(node, func(child ast.Node) bool {
		if child == nil || block != nil {
			return false
		}
		if !ps6093ProofWork(paths) {
			return false
		}
		block = ps6093BlockAt(paths, child.Pos())
		return block == nil
	})
	return block
}

func ps6093BlockAt(paths *ps6093ProofPaths, position token.Pos) *cfg.Block {
	return paths.blocks[position]
}

func ps6093EffectPosition(node ast.Node, parents map[ast.Node]ast.Node) token.Pos {
	if statement, ok := node.(*ast.RangeStmt); ok {
		return statement.X.End()
	}
	if _, ok := node.(*ast.Ident); ok {
		switch statement := ps6093ParentAfterParens(node, parents).(type) {
		case *ast.AssignStmt:
			// Assignment writes happen in phase two, after every RHS has
			// been evaluated and bounds-checked.
			return statement.End()
		case *ast.IncDecStmt:
			return statement.End()
		case *ast.RangeStmt:
			return statement.X.End()
		}
	}
	return node.End()
}

func ps6093BlockExecutesBefore(paths *ps6093ProofPaths, block *cfg.Block, node, reference ast.Node, parents map[ast.Node]ast.Node) bool {
	nodeIndex, nodeOK := ps6093BlockNodeIndex(paths, block, node.Pos())
	referenceIndex, referenceOK := ps6093BlockNodeIndex(paths, block, reference.Pos())
	if !nodeOK || !referenceOK {
		return false
	}
	if nodeIndex != referenceIndex {
		return nodeIndex < referenceIndex
	}
	return ps6093EffectPosition(node, parents) <= reference.End()
}

func ps6093BlockExecutesAfter(paths *ps6093ProofPaths, block *cfg.Block, node, reference ast.Node, parents map[ast.Node]ast.Node) bool {
	nodeIndex, nodeOK := ps6093BlockNodeIndex(paths, block, node.Pos())
	referenceIndex, referenceOK := ps6093BlockNodeIndex(paths, block, reference.Pos())
	if !nodeOK || !referenceOK {
		return false
	}
	if nodeIndex != referenceIndex {
		return nodeIndex > referenceIndex
	}
	return ps6093EffectPosition(node, parents) > reference.Pos()
}

func ps6093IndexBlockNodes(paths *ps6093ProofPaths, block *cfg.Block) {
	if paths.blocks == nil {
		paths.blocks = make(map[token.Pos]*cfg.Block)
	}
	if paths.nodeOrder == nil {
		paths.nodeOrder = make(map[token.Pos]int)
	}
	if paths.live == nil {
		paths.live = make(map[token.Pos]bool)
	}
	for index, node := range block.Nodes {
		ast.Inspect(node, func(child ast.Node) bool {
			if child != nil {
				paths.live[child.Pos()] = paths.live[child.Pos()] || block.Live
				if _, exists := paths.blocks[child.Pos()]; !exists {
					paths.blocks[child.Pos()] = block
					paths.nodeOrder[child.Pos()] = index
				}
			}
			return true
		})
	}
}

func ps6093BlockNodeIndex(paths *ps6093ProofPaths, block *cfg.Block, position token.Pos) (int, bool) {
	if !ps6093ProofWork(paths) || paths.blocks[position] != block {
		return 0, false
	}
	index, ok := paths.nodeOrder[position]
	return index, ok
}

func ps6093RangeHasImplicitEffect(pass *analysis.Pass, value types.Type) bool {
	if value == nil {
		return true
	}
	value = types.Unalias(value)
	if parameter, ok := value.(*types.TypeParam); ok {
		constraint, ok := types.Unalias(parameter.Constraint()).Underlying().(*types.Interface)
		if !ok {
			return true
		}
		set, ok := ps6091ConstraintTypeSet(constraint, make(map[types.Type]bool))
		if !ok || set.unrestricted {
			return true
		}
		for _, term := range set.terms {
			possible, known := ps6091TypeTermFeasible(term, constraint, parameter)
			if !known {
				return true
			}
			if !possible {
				continue
			}
			if ps6093RangeHasImplicitEffect(pass, term.value) {
				return true
			}
		}
		return false
	}
	switch value.Underlying().(type) {
	case *types.Chan, *types.Signature:
		return true
	}
	return false
}

func ps6093RecurringProofSafe(graph *cfg.CFG, paths *ps6093ProofPaths, loop ps6093Loop, proof, access ast.Node) (bool, bool) {
	if graph == nil || paths == nil || loop.body == nil || proof == nil || access == nil {
		return false, true
	}
	inBody := loop.body.Pos() <= proof.Pos() && proof.End() <= loop.body.End()
	inCondition := loop.condition != nil && loop.condition.Pos() <= proof.Pos() && proof.End() <= loop.condition.End()
	if !inBody && !inCondition {
		return false, true
	}
	proofBlock := paths.blocks[proof.Pos()]
	accessBlock := paths.blocks[access.Pos()]
	if proofBlock == nil || accessBlock == nil || !proofBlock.Live || !accessBlock.Live {
		return false, true
	}
	if proofBlock == accessBlock {
		return proof.Pos() < access.Pos(), true
	}
	key := ps6093ProofPathKey{proof: proofBlock, access: accessBlock, proofEnd: proof.End(), accessStart: access.Pos()}
	if result, ok := paths.results[key]; ok {
		return result, true
	}
	result, complete := ps6093AccessCyclesThroughProof(paths, proofBlock, accessBlock)
	if !complete {
		return false, false
	}
	paths.results[key] = result
	return result, true
}

func ps6093AccessCyclesThroughProof(paths *ps6093ProofPaths, proofBlock, accessBlock *cfg.Block) (bool, bool) {
	seen := make(map[*cfg.Block]bool)
	queue := make([]*cfg.Block, 0, len(accessBlock.Succs))
	for _, successor := range accessBlock.Succs {
		if successor != proofBlock {
			seen[successor] = true
			queue = append(queue, successor)
		}
	}
	for len(queue) > 0 {
		if !ps6093ProofWork(paths) {
			return false, false
		}
		block := queue[0]
		queue = queue[1:]
		if block == accessBlock {
			return false, true
		}
		for _, successor := range block.Succs {
			if successor == proofBlock || seen[successor] {
				continue
			}
			seen[successor] = true
			queue = append(queue, successor)
		}
	}
	return true, true
}

func ps6093TerminatesBeforeAccess(pass *analysis.Pass, parents map[ast.Node]ast.Node, paths *ps6093ProofPaths, block *ast.BlockStmt, access ast.Node) bool {
	if block == nil || len(block.List) == 0 {
		return false
	}
	hasGoto := false
	complete := true
	ast.Inspect(block, func(node ast.Node) bool {
		if !complete {
			return false
		}
		if node != nil && !ps6093ProofWork(paths) {
			complete = false
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		if branch, ok := node.(*ast.BranchStmt); ok && branch.Tok == token.GOTO {
			hasGoto = true
			return false
		}
		return !hasGoto
	})
	if hasGoto || !complete {
		return false
	}
	switch statement := block.List[len(block.List)-1].(type) {
	case *ast.ReturnStmt:
		return true
	case *ast.BranchStmt:
		return ps6093BranchBypassesAccess(parents, paths, statement, access)
	case *ast.ExprStmt:
		call, ok := ps2110Unparen(statement.X).(*ast.CallExpr)
		return ok && ps6079PanicCall(pass, call)
	case *ast.IfStmt:
		elseBlock, ok := statement.Else.(*ast.BlockStmt)
		return ok && ps6093TerminatesBeforeAccess(pass, parents, paths, statement.Body, access) &&
			ps6093TerminatesBeforeAccess(pass, parents, paths, elseBlock, access)
	}
	return false
}

func ps6093BranchBypassesAccess(parents map[ast.Node]ast.Node, paths *ps6093ProofPaths, branch *ast.BranchStmt, access ast.Node) bool {
	if branch == nil || branch.Label != nil || branch.Tok != token.BREAK && branch.Tok != token.CONTINUE {
		return false
	}
	for parent := parents[branch]; parent != nil; parent = parents[parent] {
		if !ps6093ProofWork(paths) {
			return false
		}
		var target ast.Node
		switch statement := parent.(type) {
		case *ast.ForStmt:
			target = statement
		case *ast.RangeStmt:
			target = statement
		case *ast.SwitchStmt:
			if branch.Tok == token.BREAK {
				target = statement
			}
		case *ast.TypeSwitchStmt:
			if branch.Tok == token.BREAK {
				target = statement
			}
		case *ast.SelectStmt:
			if branch.Tok == token.BREAK {
				target = statement
			}
		}
		if target != nil {
			return target.Pos() <= access.Pos() && access.End() <= target.End()
		}
	}
	return false
}

func ps6093PositionLive(paths *ps6093ProofPaths, position token.Pos) bool {
	return ps6093ProofWork(paths) && paths.live[position]
}
