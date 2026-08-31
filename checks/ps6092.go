package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/cfg"

	"github.com/jxsl13/perfscan/lint"
)

const ps6092SmallTripLimit int64 = 4

// PS6092 implements owner issue #905. It identifies interface-constraint
// method calls on type-parameter receivers along a genuinely repeated loop
// path. The finding is a machine-code verification obligation: Go may share a
// shape instantiation and retain dictionary dispatch, but source alone cannot
// prove that a particular compiler and target emitted an indirect call.
var PS6092 = register(&lint.Check{
	ID:       "PS6092",
	Category: "verify",
	Slug:     "generic-constraint-call-in-hot-loop",
	Level:    lint.LevelAggressive,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "a type-parameter receiver calls an interface-constraint method on a repeated hot-loop path",
		Text: `Even a zero-size operation value passed as a generic type parameter does not
guarantee static dispatch. Go can stencil one machine body for several type
arguments with the same GC shape and load the selected interface method from a
generic dictionary on every iteration. The source looks monomorphic while the
element loop still pays an indirect branch and the associated state spills.

PS6092 reports only a typed selector call whose receiver is exactly a
go/types.TypeParam and whose selected method is an exact member of that type
parameter's interface constraint. The call must belong to a live control-flow
cycle within an enclosing for/range statement, so some path can actually
revisit it. Calls in loop initializers, range operands, unreachable blocks,
constant-dead if/switch/short-circuit branches, and loops that unconditionally leave
before a second visit are excluded. Proven loop bounds are combined across
nesting; at most four total executions are treated as trivial. Runtime/complex
bounds and larger fixed bounds remain candidates. A locally visible pair of
distinct empty-struct type arguments strengthens the diagnostic's shared-shape
evidence, but is not required: exported generic helpers may be instantiated
elsewhere.

This is not a claim that generic calls are universally indirect. Inspect the
exact compiler version, GOOS/GOARCH, and instantiated symbol with -S or
go tool objdump. A narrow adjacent comment can record verified direct-call
output and suppress the finding only when it contains
"perfscan:generic-dispatch-verified", a go1.x toolchain, an assembly command
or disassembly reference, and a direct/no-indirect-call result. The ordinary
//perfscan:ignore PS6092 mechanism remains available and should carry the same
evidence so the exception can be rechecked after toolchain upgrades.

There is NO automatic fix. Moving a type or enum switch outside the loop,
generating monomorphic helpers, or duplicating operation-specific loops can
change code size, floating-point instruction selection, panic timing, and
maintainability. Preserve exact results and operation semantics, confirm the
indirect branch in assembly first, then retain a rewrite only after an
order-controlled end-to-end benchmark. A direct-call result is a valid reason
to keep the generic source unchanged.`,
		Before: `func elemBinary[F Float, Op Binary[F]](dst, x, y []F, op Op) {
	for i := range dst {
		dst[i] = op.Apply(x[i], y[i]) // may load Apply from a dictionary
	}
}`,
		After: `// Evidence-driven candidate; not an automatic rewrite:
switch any(op).(type) {
case Add[F]:
	for i := range dst { dst[i] = x[i] + y[i] }
case Sub[F]:
	for i := range dst { dst[i] = x[i] - y[i] }
default:
	for i := range dst { dst[i] = op.Apply(x[i], y[i]) }
}
// Keep only after exact-output, assembly, code-size, and end-to-end gates.`,
		MeasuredWin: `GoAI backend/ref on Apple M2 Pro with Go 1.27.0: a
one-time operation type switch moved dispatch outside the element loop. Across
nine alternating frozen pairs, Add F64 4K improved from median 20,192 to 7,762
ns/op (2.6014x, 9/9 wins), Add F32 4K from 14,038 to 6,604 ns/op (2.1257x,
8/9 wins), and ReLU F64 64K from 1,050,402 to 731,328 ns/op (1.4363x, 7/9
wins). Tanh F64 64K was neutral at 0.9805x because libm dominated. Bytes and
allocations were unchanged, and registered F32/F64 operations retained exact
outputs.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6092",
		Doc:  "type-parameter receiver calls an interface-constraint method on a repeated loop path",
		Run:  runPS6092,
	},
})

type ps6092FunctionBody struct {
	file *ast.File
	body *ast.BlockStmt
}

type ps6092MethodCall struct {
	selector  *ast.SelectorExpr
	typeParam *types.TypeParam
	method    *types.Func
}

type ps6092Candidate struct {
	call    *ast.CallExpr
	matched ps6092MethodCall
}

type ps6092Comparison struct {
	expression ast.Expr
	operator   token.Token
	bound      int64
}

func runPS6092(pass *analysis.Pass) (any, error) {
	sharedShapes := ps6092SharedZeroShapeInstantiations(pass)
	reported := make(map[*ast.CallExpr]bool)
	for _, file := range pass.Files {
		var bodies []ps6092FunctionBody
		fileParents := ps6092ParentMap(file)
		constructionFlows := make(map[*ast.BlockStmt]ps6092Flow)
		ast.Inspect(file, func(node ast.Node) bool {
			switch value := node.(type) {
			case *ast.FuncDecl:
				if value.Body != nil {
					bodies = append(bodies, ps6092FunctionBody{file: file, body: value.Body})
				}
			case *ast.FuncLit:
				independent := !ps6092ImmediatelyInvoked(fileParents, value) || !ps6092HasEnclosingFunction(fileParents, value)
				if independent && ps6092LiteralBodyMayRun(pass, fileParents, constructionFlows, value) {
					bodies = append(bodies, ps6092FunctionBody{file: file, body: value.Body})
				}
			}
			return true
		})
		for _, function := range bodies {
			ps6092InspectBody(pass, function, sharedShapes, reported)
		}
	}
	return nil, nil
}

func ps6092HasEnclosingFunction(parents map[ast.Node]ast.Node, node ast.Node) bool {
	for parent := parents[node]; parent != nil; parent = parents[parent] {
		switch parent.(type) {
		case *ast.FuncDecl, *ast.FuncLit:
			return true
		}
	}
	return false
}

func ps6092LiteralBodyMayRun(
	pass *analysis.Pass,
	parents map[ast.Node]ast.Node,
	flows map[*ast.BlockStmt]ps6092Flow,
	literal *ast.FuncLit,
) bool {
	for current := literal; current != nil; {
		if ps6092StaticallyDead(pass, parents, current) || ps6092DiscardedLiteral(pass, parents, current) {
			return false
		}
		body, owner := ps6092EnclosingFunctionBody(parents, current)
		if body == nil {
			break
		}
		flow, built := flows[body]
		if !built {
			flow = ps6092BuildFlow(pass, body)
			flows[body] = flow
		}
		if flow.blockFor(current) == nil {
			return false
		}
		current = owner
	}
	for parent := parents[literal]; parent != nil; parent = parents[parent] {
		statement, loop := parent.(ast.Stmt)
		if !loop {
			continue
		}
		switch value := statement.(type) {
		case *ast.ForStmt:
			if ps6092Within(literal, value.Body) {
				if bound, known := ps6092LoopTripBound(pass, value); known && bound == 0 {
					return false
				}
			}
		case *ast.RangeStmt:
			if ps6092Within(literal, value.Body) {
				if bound, known := ps6092LoopTripBound(pass, value); known && bound == 0 {
					return false
				}
			}
		}
	}
	return true
}

func ps6092EnclosingFunctionBody(parents map[ast.Node]ast.Node, node ast.Node) (*ast.BlockStmt, *ast.FuncLit) {
	for parent := parents[node]; parent != nil; parent = parents[parent] {
		switch function := parent.(type) {
		case *ast.FuncDecl:
			return function.Body, nil
		case *ast.FuncLit:
			return function.Body, function
		}
	}
	return nil, nil
}

func ps6092InspectBody(
	pass *analysis.Pass,
	function ps6092FunctionBody,
	sharedShapes map[*types.TypeParam]int,
	reported map[*ast.CallExpr]bool,
) {
	parents := ps6092ParentMap(function.body)
	var candidates []ps6092Candidate
	ast.Inspect(function.body, func(node ast.Node) bool {
		if literal, nested := node.(*ast.FuncLit); nested && literal.Body != function.body && !ps6092ImmediatelyInvoked(parents, literal) {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok || reported[call] {
			return true
		}
		matched, ok := ps6092ConstraintMethodCall(pass, call)
		if !ok || ps6092StaticallyDead(pass, parents, call) || ps6092CompilerEvidenceSuppressed(pass, function.file, call) {
			return true
		}
		candidates = append(candidates, ps6092Candidate{call: call, matched: matched})
		return true
	})
	if len(candidates) == 0 {
		return
	}
	flows := make(map[*ast.BlockStmt]ps6092Flow)
	flows[function.body] = ps6092BuildFlow(pass, function.body)
	for _, candidate := range candidates {
		var loops []ast.Stmt
		seenLoops := make(map[ast.Stmt]bool)
		internalCycle := false
		live := true
		for _, body := range ps6092InvokedBodies(function.body, parents, candidate.call) {
			flow, built := flows[body]
			if !built {
				flow = ps6092BuildFlow(pass, body)
				flows[body] = flow
			}
			if flow.blockFor(candidate.call) == nil {
				live = false
				break
			}
			bodyLoops, bodyInternalCycle := flow.repeatingLoops(parents, candidate.call, body)
			internalCycle = internalCycle || bodyInternalCycle
			for _, loop := range bodyLoops {
				if !seenLoops[loop] {
					seenLoops[loop] = true
					loops = append(loops, loop)
				}
			}
		}
		if !live {
			continue
		}
		loopLabel, nontrivial := ps6092NontrivialLoops(pass, loops, internalCycle, candidate.call)
		if !nontrivial {
			continue
		}
		matched := candidate.matched
		var message strings.Builder
		message.Grow(256)
		message.WriteString("type-parameter receiver ")
		message.WriteString(matched.typeParam.Obj().Name())
		message.WriteString(" calls interface-constraint method ")
		message.WriteString(matched.method.Name())
		message.WriteString(" on ")
		message.WriteString(loopLabel)
		message.WriteString("; Go shape stenciling may retain a generic-dictionary indirect call per iteration—inspect the exact toolchain/target assembly before moving dispatch outside the loop (advisory, no automatic fix)")
		if count := sharedShapes[matched.typeParam]; count >= 2 {
			message.WriteString("; local instantiations include ")
			message.WriteString(strconv.Itoa(count))
			message.WriteString(" distinct zero-size struct operation types, strengthening (but not proving) shared-shape dispatch risk")
		}
		pass.Report(analysis.Diagnostic{
			Pos:     matched.selector.Sel.Pos(),
			End:     candidate.call.End(),
			Message: message.String(),
		})
		reported[candidate.call] = true
	}
}

func ps6092ConstraintMethodCall(pass *analysis.Pass, call *ast.CallExpr) (ps6092MethodCall, bool) {
	selector, ok := ps2110Unparen(call.Fun).(*ast.SelectorExpr)
	if !ok {
		return ps6092MethodCall{}, false
	}
	selection := pass.TypesInfo.Selections[selector]
	if selection == nil || selection.Kind() != types.MethodVal && selection.Kind() != types.MethodExpr {
		return ps6092MethodCall{}, false
	}
	receiver := types.Unalias(selection.Recv())
	typeParam, ok := receiver.(*types.TypeParam)
	if !ok || !types.Identical(types.Unalias(pass.TypesInfo.TypeOf(selector.X)), typeParam) {
		return ps6092MethodCall{}, false
	}
	method, ok := selection.Obj().(*types.Func)
	if !ok {
		return ps6092MethodCall{}, false
	}
	constraint, ok := types.Unalias(typeParam.Constraint()).Underlying().(*types.Interface)
	if !ok {
		return ps6092MethodCall{}, false
	}
	constraint.Complete()
	for index := range constraint.NumMethods() {
		required := constraint.Method(index)
		if required.Id() == method.Id() && types.Identical(required.Type(), method.Type()) {
			return ps6092MethodCall{selector: selector, typeParam: typeParam, method: method}, true
		}
	}
	return ps6092MethodCall{}, false
}

type ps6092Flow struct {
	blocks        []*cfg.Block
	component     map[*cfg.Block]int
	cyclic        map[int]bool
	loopComponent map[ast.Stmt][]bool
}

func ps6092BuildFlow(pass *analysis.Pass, body *ast.BlockStmt) ps6092Flow {
	graph := cfg.New(body, func(call *ast.CallExpr) bool { return !ps6088NonreturnCall(pass, call) })
	component, cyclic := ps6092Components(graph.Blocks)
	loopComponent := make(map[ast.Stmt][]bool)
	for _, block := range graph.Blocks {
		if !block.Live || block.Stmt == nil {
			continue
		}
		switch block.Kind {
		case cfg.KindForBody, cfg.KindForLoop, cfg.KindForPost, cfg.KindRangeBody, cfg.KindRangeLoop:
			if loopComponent[block.Stmt] == nil {
				loopComponent[block.Stmt] = make([]bool, len(cyclic)+1)
			}
			loopComponent[block.Stmt][component[block]] = true
		}
	}
	return ps6092Flow{
		blocks:        graph.Blocks,
		component:     component,
		cyclic:        cyclic,
		loopComponent: loopComponent,
	}
}

func ps6092Components(blocks []*cfg.Block) (map[*cfg.Block]int, map[int]bool) {
	indices := make(map[*cfg.Block]int)
	lowlink := make(map[*cfg.Block]int)
	onStack := make(map[*cfg.Block]bool)
	component := make(map[*cfg.Block]int)
	cyclic := make(map[int]bool)
	var stack []*cfg.Block
	nextIndex := 1
	nextComponent := 1
	var visit func(*cfg.Block)
	visit = func(block *cfg.Block) {
		indices[block] = nextIndex
		lowlink[block] = nextIndex
		nextIndex++
		stack = append(stack, block)
		onStack[block] = true
		for _, successor := range block.Succs {
			if !successor.Live {
				continue
			}
			if indices[successor] == 0 {
				visit(successor)
				lowlink[block] = min(lowlink[block], lowlink[successor])
			} else if onStack[successor] {
				lowlink[block] = min(lowlink[block], indices[successor])
			}
		}
		if lowlink[block] != indices[block] {
			return
		}
		size := 0
		selfEdge := false
		for {
			last := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			onStack[last] = false
			component[last] = nextComponent
			size++
			for _, successor := range last.Succs {
				if successor == last {
					selfEdge = true
				}
			}
			if last == block {
				break
			}
		}
		cyclic[nextComponent] = size > 1 || selfEdge
		nextComponent++
	}
	for _, block := range blocks {
		if block.Live && indices[block] == 0 {
			visit(block)
		}
	}
	return component, cyclic
}

func (flow ps6092Flow) blockFor(node ast.Node) *cfg.Block {
	var best *cfg.Block
	bestSpan := int(^uint(0) >> 1)
	for _, block := range flow.blocks {
		if !block.Live {
			continue
		}
		for _, candidate := range block.Nodes {
			if candidate.Pos() <= node.Pos() && node.End() <= candidate.End() {
				span := int(candidate.End() - candidate.Pos())
				if span < bestSpan {
					best = block
					bestSpan = span
				}
			}
		}
	}
	return best
}

func (flow ps6092Flow) repeatingLoops(
	parents map[ast.Node]ast.Node,
	node ast.Node,
	boundary *ast.BlockStmt,
) ([]ast.Stmt, bool) {
	block := flow.blockFor(node)
	if block == nil {
		return nil, false
	}
	callComponent := flow.component[block]
	if !flow.cyclic[callComponent] {
		return nil, false
	}
	var loops []ast.Stmt
	var enclosing []ast.Stmt
	for parent := parents[node]; parent != nil; parent = parents[parent] {
		if parent == boundary {
			break
		}
		if literal, boundary := parent.(*ast.FuncLit); boundary {
			if !ps6092ImmediatelyInvoked(parents, literal) {
				break
			}
			continue
		}
		statement, loop := parent.(ast.Stmt)
		if !loop {
			continue
		}
		switch statement.(type) {
		case *ast.ForStmt, *ast.RangeStmt:
			enclosing = append(enclosing, statement)
			components := flow.loopComponent[statement]
			if callComponent < len(components) && components[callComponent] {
				loops = append(loops, statement)
			}
		}
	}
	if len(loops) == 0 && len(enclosing) > 0 {
		// A labeled goto can form a cycle wholly inside a loop body without
		// traversing that loop's own header/backedge blocks. The call is still on
		// a genuinely repeated path; retain every enclosing loop so a zero-trip
		// outer loop can still prove that the cycle is unreachable.
		return enclosing, true
	}
	return loops, flow.backwardGotoCycle(boundary, node, callComponent)
}

func (flow ps6092Flow) backwardGotoCycle(boundary *ast.BlockStmt, node ast.Node, callComponent int) bool {
	labels := make(map[string]token.Pos)
	ast.Inspect(boundary, func(candidate ast.Node) bool {
		if _, nested := candidate.(*ast.FuncLit); nested {
			return false
		}
		if labeled, ok := candidate.(*ast.LabeledStmt); ok {
			labels[labeled.Label.Name] = labeled.Pos()
		}
		return true
	})
	backward := false
	ast.Inspect(boundary, func(candidate ast.Node) bool {
		if _, nested := candidate.(*ast.FuncLit); nested {
			return false
		}
		branch, ok := candidate.(*ast.BranchStmt)
		if !ok || branch.Tok != token.GOTO || branch.Label == nil {
			return true
		}
		label := labels[branch.Label.Name]
		if label == token.NoPos || label > node.Pos() || branch.Pos() < node.End() {
			return true
		}
		for _, target := range flow.blocks {
			labeled, isLabel := target.Stmt.(*ast.LabeledStmt)
			if !target.Live || !isLabel || labeled.Label.Name != branch.Label.Name ||
				flow.component[target] != callComponent {
				continue
			}
			predecessors := 0
			for _, source := range flow.blocks {
				if !source.Live || flow.component[source] != callComponent {
					continue
				}
				for _, successor := range source.Succs {
					if successor == target {
						predecessors++
					}
				}
			}
			if predecessors >= 2 {
				backward = true
				return false
			}
		}
		return true
	})
	return backward
}

func ps6092InvokedBodies(
	outer *ast.BlockStmt,
	parents map[ast.Node]ast.Node,
	node ast.Node,
) []*ast.BlockStmt {
	bodies := []*ast.BlockStmt{outer}
	for parent := parents[node]; parent != nil; parent = parents[parent] {
		literal, ok := parent.(*ast.FuncLit)
		if !ok || literal.Body == outer {
			continue
		}
		if ps6092ImmediatelyInvoked(parents, literal) {
			bodies = append(bodies, literal.Body)
		}
	}
	return bodies
}

func ps6092ParentMap(body ast.Node) map[ast.Node]ast.Node {
	parents := make(map[ast.Node]ast.Node)
	if body == nil {
		return parents
	}
	var stack []ast.Node
	ast.Inspect(body, func(node ast.Node) bool {
		if node == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		if len(stack) > 0 {
			parents[node] = stack[len(stack)-1]
		}
		stack = append(stack, node)
		return true
	})
	return parents
}

func ps6092ImmediatelyInvoked(parents map[ast.Node]ast.Node, literal *ast.FuncLit) bool {
	var expression ast.Expr = literal
	parent := parents[literal]
	for {
		parentheses, ok := parent.(*ast.ParenExpr)
		if !ok || parentheses.X != expression {
			break
		}
		expression = parentheses
		parent = parents[parent]
	}
	call, ok := parent.(*ast.CallExpr)
	if !ok || ps2110Unparen(call.Fun) != literal {
		return false
	}
	switch parents[call].(type) {
	case *ast.GoStmt, *ast.DeferStmt:
		return false
	}
	return true
}

func ps6092NontrivialLoops(
	pass *analysis.Pass,
	loops []ast.Stmt,
	internalCycle bool,
	candidate *ast.CallExpr,
) (string, bool) {
	if len(loops) == 0 {
		return "", false
	}
	product := int64(1)
	unknownLabel := ""
	overLimit := false
	for _, loop := range loops {
		bound, known := ps6092CandidateTripBound(pass, loop, candidate)
		if !known {
			if unknownLabel == "" {
				switch loop.(type) {
				case *ast.RangeStmt:
					unknownLabel = "a runtime-bound range loop"
				default:
					unknownLabel = "a runtime or complex-bound for loop"
				}
			}
			continue
		}
		if bound == 0 {
			return "", false
		}
		if bound > ps6092SmallTripLimit || product > ps6092SmallTripLimit/bound {
			overLimit = true
			continue
		}
		product *= bound
	}
	if internalCycle {
		return "a runtime or complex-bound for loop", true
	}
	if unknownLabel != "" {
		return unknownLabel, true
	}
	if overLimit {
		return "a loop path with more than " + strconv.FormatInt(ps6092SmallTripLimit, 10) + " possible executions", true
	}
	if product <= ps6092SmallTripLimit {
		return "", false
	}
	return "nested loops with up to " + strconv.FormatInt(product, 10) + " executions", true
}

func ps6092CandidateTripBound(pass *analysis.Pass, statement ast.Stmt, candidate *ast.CallExpr) (int64, bool) {
	loop, counted := statement.(*ast.ForStmt)
	if !counted || !ps6092Within(candidate, loop.Cond) {
		return ps6092LoopTripBound(pass, statement)
	}
	if value, known := ps6092BoolConstant(pass, loop.Cond); known && !value {
		return 0, true
	}
	return ps6092CountedTripBoundFor(pass, loop, candidate)
}

func ps6092LoopTripBound(pass *analysis.Pass, statement ast.Stmt) (int64, bool) {
	switch loop := statement.(type) {
	case *ast.ForStmt:
		if value, known := ps6092BoolConstant(pass, loop.Cond); known && !value {
			return 0, true
		}
		return ps6092CountedTripBound(pass, loop)
	case *ast.RangeStmt:
		return ps6092RangeTripBound(pass, loop.X)
	}
	return 0, false
}

func ps6092CountedTripBound(pass *analysis.Pass, loop *ast.ForStmt) (int64, bool) {
	return ps6092CountedTripBoundFor(pass, loop, nil)
}

func ps6092CountedTripBoundFor(pass *analysis.Pass, loop *ast.ForStmt, candidate *ast.CallExpr) (int64, bool) {
	if bound, known := ps6092BooleanTripBound(pass, loop, candidate); known {
		return bound, true
	}
	initialization, ok := loop.Init.(*ast.AssignStmt)
	if !ok || len(initialization.Lhs) != 1 || len(initialization.Rhs) != 1 ||
		initialization.Tok != token.DEFINE {
		return 0, false
	}
	identifier, ok := ps2110Unparen(initialization.Lhs[0]).(*ast.Ident)
	if !ok {
		return 0, false
	}
	object := pass.TypesInfo.Defs[identifier]
	if object == nil {
		object = pass.TypesInfo.Uses[identifier]
	}
	start, ok := ps6092Int64Constant(pass, initialization.Rhs[0])
	if object == nil || !ok || ps6092NodeMutates(pass, loop.Body, object) || ps6092NodeMutates(pass, loop.Cond, object) {
		return 0, false
	}
	step, ok := ps6092LoopStep(pass, loop.Post, object)
	if !ok || step == 0 {
		return 0, false
	}
	minimum, maximum, ok := ps6092IntegerBounds(pass, object.Type())
	if !ok || start < minimum || start > maximum {
		return 0, false
	}
	comparisons := ps6092InductionComparisons(pass, loop.Cond, object)
	if len(comparisons) == 0 {
		return 0, false
	}
	best := ps6092SmallTripLimit + 1
	found := false
	for _, comparison := range comparisons {
		bound, terminal, known := ps6092SimulateTripBound(start, step, minimum, maximum, comparison)
		if !known {
			continue
		}
		if candidate != nil && bound <= ps6092SmallTripLimit {
			_, _, reachesCandidate := ps6092BooleanPossibilities(pass, loop.Cond, object, terminal, candidate)
			if reachesCandidate {
				bound++
			}
		}
		found = true
		best = min(best, bound)
	}
	return best, found
}

func ps6092BooleanTripBound(pass *analysis.Pass, loop *ast.ForStmt, candidate *ast.CallExpr) (int64, bool) {
	initialization, ok := loop.Init.(*ast.AssignStmt)
	if !ok || len(initialization.Lhs) != 1 || len(initialization.Rhs) != 1 || initialization.Tok != token.DEFINE {
		return 0, false
	}
	identifier, ok := ps2110Unparen(initialization.Lhs[0]).(*ast.Ident)
	if !ok {
		return 0, false
	}
	object := pass.TypesInfo.Defs[identifier]
	if object == nil {
		object = pass.TypesInfo.Uses[identifier]
	}
	start, ok := ps6092BoolConstant(pass, initialization.Rhs[0])
	if object == nil || !ok || ps6092NodeMutates(pass, loop.Body, object) || ps6092NodeMutates(pass, loop.Cond, object) {
		return 0, false
	}
	next, ok := ps6092BooleanPost(pass, loop.Post, object)
	if !ok {
		return 0, false
	}
	firstTrue, _, firstReaches := ps6092BooleanCondition(pass, loop.Cond, object, start, candidate)
	if !firstTrue {
		if candidate != nil && ps6092Within(candidate, loop.Cond) && firstReaches {
			return 1, true
		}
		return 0, true
	}
	secondTrue, _, secondReaches := ps6092BooleanCondition(pass, loop.Cond, object, next, candidate)
	if secondTrue {
		return 0, false
	}
	if candidate != nil && ps6092Within(candidate, loop.Cond) {
		bound := int64(0)
		if firstReaches {
			bound++
		}
		if secondReaches {
			bound++
		}
		return bound, true
	}
	return 1, true
}

func ps6092BooleanPost(pass *analysis.Pass, statement ast.Stmt, object types.Object) (bool, bool) {
	assignment, ok := statement.(*ast.AssignStmt)
	if !ok || assignment.Tok != token.ASSIGN || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 ||
		!ps6092DirectObject(pass, assignment.Lhs[0], object) {
		return false, false
	}
	return ps6092BoolConstant(pass, assignment.Rhs[0])
}

func ps6092BooleanCondition(
	pass *analysis.Pass,
	expression ast.Expr,
	object types.Object,
	value bool,
	candidate *ast.CallExpr,
) (canTrue, canFalse, reachesCandidate bool) {
	expression = ps2110Unparen(expression)
	if ps6092DirectObject(pass, expression, object) {
		return value, !value, candidate != nil && ps6092Within(candidate, expression)
	}
	if constantValue, known := ps6092BoolConstant(pass, expression); known {
		return constantValue, !constantValue, candidate != nil && ps6092Within(candidate, expression)
	}
	if unary, ok := expression.(*ast.UnaryExpr); ok && unary.Op == token.NOT {
		innerTrue, innerFalse, innerReaches := ps6092BooleanCondition(pass, unary.X, object, value, candidate)
		return innerFalse, innerTrue, innerReaches
	}
	if binary, ok := expression.(*ast.BinaryExpr); ok && (binary.Op == token.LAND || binary.Op == token.LOR) {
		leftTrue, leftFalse, leftReaches := ps6092BooleanCondition(pass, binary.X, object, value, candidate)
		rightTrue, rightFalse, rightReaches := ps6092BooleanCondition(pass, binary.Y, object, value, candidate)
		if binary.Op == token.LAND {
			return leftTrue && rightTrue,
				leftFalse || leftTrue && rightFalse,
				leftReaches || leftTrue && rightReaches
		}
		return leftTrue || leftFalse && rightTrue,
			leftFalse && rightFalse,
			leftReaches || leftFalse && rightReaches
	}
	binary, ok := expression.(*ast.BinaryExpr)
	if !ok || binary.Op != token.EQL && binary.Op != token.NEQ {
		return true, true, candidate != nil && ps6092Within(candidate, expression)
	}
	constantExpression := binary.Y
	if !ps6092DirectObject(pass, binary.X, object) {
		if !ps6092DirectObject(pass, binary.Y, object) {
			return true, true, candidate != nil && ps6092Within(candidate, expression)
		}
		constantExpression = binary.X
	}
	right, ok := ps6092BoolConstant(pass, constantExpression)
	if !ok {
		return true, true, candidate != nil && ps6092Within(candidate, expression)
	}
	result := value != right
	if binary.Op == token.EQL {
		result = value == right
	}
	return result, !result, candidate != nil && ps6092Within(candidate, expression)
}

func ps6092InductionComparisons(pass *analysis.Pass, expression ast.Expr, object types.Object) []ps6092Comparison {
	binary, ok := ps2110Unparen(expression).(*ast.BinaryExpr)
	if !ok {
		return nil
	}
	if binary.Op == token.LAND {
		return append(
			ps6092InductionComparisons(pass, binary.X, object),
			ps6092InductionComparisons(pass, binary.Y, object)...,
		)
	}
	operator := binary.Op
	boundExpression := binary.Y
	if !ps6092DirectObject(pass, binary.X, object) {
		if !ps6092DirectObject(pass, binary.Y, object) {
			return nil
		}
		boundExpression = binary.X
		operator = ps6092ReverseComparison(operator)
	}
	if operator == token.ILLEGAL {
		return nil
	}
	bound, ok := ps6092Int64Constant(pass, boundExpression)
	if !ok {
		return nil
	}
	return []ps6092Comparison{{expression: binary, operator: operator, bound: bound}}
}

func ps6092BooleanPossibilities(
	pass *analysis.Pass,
	expression ast.Expr,
	object types.Object,
	induction int64,
	candidate *ast.CallExpr,
) (canTrue, canFalse, reachesCandidate bool) {
	expression = ps2110Unparen(expression)
	if binary, boolean := expression.(*ast.BinaryExpr); boolean && (binary.Op == token.LAND || binary.Op == token.LOR) {
		leftTrue, leftFalse, leftReaches := ps6092BooleanPossibilities(pass, binary.X, object, induction, candidate)
		rightTrue, rightFalse, rightReaches := ps6092BooleanPossibilities(pass, binary.Y, object, induction, candidate)
		if binary.Op == token.LAND {
			return leftTrue && rightTrue,
				leftFalse || leftTrue && rightFalse,
				leftReaches || leftTrue && rightReaches
		}
		return leftTrue || leftFalse && rightTrue,
			leftFalse && rightFalse,
			leftReaches || leftFalse && rightReaches
	}
	reachesCandidate = ps6092Within(candidate, expression)
	if value, known := ps6092BoolConstant(pass, expression); known {
		return value, !value, reachesCandidate
	}
	comparisons := ps6092InductionComparisons(pass, expression, object)
	if len(comparisons) == 1 && ps6092Within(comparisons[0].expression, expression) {
		value := ps6092CompareInt(induction, comparisons[0].operator, comparisons[0].bound)
		return value, !value, reachesCandidate
	}
	return true, true, reachesCandidate
}

func ps6092SimulateTripBound(
	start, step, minimum, maximum int64,
	comparison ps6092Comparison,
) (iterations, terminal int64, known bool) {
	current := start
	for iterations := int64(0); iterations <= ps6092SmallTripLimit; iterations++ {
		if !ps6092CompareInt(current, comparison.operator, comparison.bound) {
			return iterations, current, true
		}
		if iterations == ps6092SmallTripLimit {
			return ps6092SmallTripLimit + 1, current, true
		}
		next, safe := ps6092WrappedAdd(current, step, minimum, maximum)
		if !safe {
			return 0, 0, false
		}
		current = next
	}
	return 0, 0, false
}

func ps6092RangeTripBound(pass *analysis.Pass, expression ast.Expr) (int64, bool) {
	expression = ps2110Unparen(expression)
	if typed, ok := pass.TypesInfo.Types[expression]; ok && typed.Value != nil {
		switch typed.Value.Kind() {
		case constant.Int:
			value, exact := constant.Int64Val(typed.Value)
			if !exact {
				return 0, false
			}
			return max(value, 0), true
		case constant.String:
			return int64(utf8.RuneCountInString(constant.StringVal(typed.Value))), true
		}
	}
	typeValue := pass.TypesInfo.TypeOf(expression)
	if typeValue == nil {
		return 0, false
	}
	switch underlying := types.Unalias(typeValue).Underlying().(type) {
	case *types.Array:
		return underlying.Len(), true
	case *types.Pointer:
		if array, fixed := types.Unalias(underlying.Elem()).Underlying().(*types.Array); fixed {
			return array.Len(), true
		}
	}
	literal, ok := expression.(*ast.CompositeLit)
	if !ok {
		return 0, false
	}
	switch types.Unalias(typeValue).Underlying().(type) {
	case *types.Map:
		return ps6092MapLiteralTripBound(pass, literal), true
	case *types.Slice:
		return ps6092SliceLiteralTripBound(pass, literal)
	}
	return 0, false
}

func ps6092SliceLiteralTripBound(pass *analysis.Pass, literal *ast.CompositeLit) (int64, bool) {
	nextIndex := int64(0)
	length := int64(0)
	for _, element := range literal.Elts {
		index := nextIndex
		if keyed, explicit := element.(*ast.KeyValueExpr); explicit {
			var known bool
			index, known = ps6092Int64Constant(pass, keyed.Key)
			if !known || index < 0 {
				return 0, false
			}
		}
		if index == math.MaxInt64 {
			return 0, false
		}
		nextIndex = index + 1
		length = max(length, nextIndex)
	}
	return length, true
}

func ps6092MapLiteralTripBound(pass *analysis.Pass, literal *ast.CompositeLit) int64 {
	bound, _ := ps6092MapLiteralTripBoundWork(pass, literal)
	return bound
}

func ps6092MapLiteralTripBoundWork(pass *analysis.Pass, literal *ast.CompositeLit) (int64, int) {
	bound := int64(len(literal.Elts))
	// The element count is always an upper bound. Repeated reads of the same
	// reflexive pure key expressions collapse to one entry when every intervening
	// value expression is also pure, so no evaluation can mutate their inputs
	// between reads.
	// Float, complex, interface, and composites containing them remain
	// conservative: NaN values are not equal to themselves and can create
	// multiple entries.
	keyer := ps6092ExpressionKeyer{
		pass:      pass,
		typeKeys:  make(map[types.Type]string),
		objectIDs: make(map[types.Object]int),
	}
	seen := make(map[string]bool, len(literal.Elts))
	duplicates := int64(0)
	for _, element := range literal.Elts {
		keyValue, keyed := element.(*ast.KeyValueExpr)
		if !keyed {
			return bound, keyer.nodes
		}
		if _, pure := keyer.key(keyValue.Value); !pure {
			return bound, keyer.nodes
		}
		keyTyped, known := pass.TypesInfo.Types[ps2110Unparen(keyValue.Key)]
		if known && keyTyped.Value != nil {
			continue
		}
		if !known || !ps6092ReflexiveMapKey(keyTyped.Type, make(map[types.Type]bool)) {
			return bound, keyer.nodes
		}
		key, pure := keyer.key(keyValue.Key)
		if !pure {
			return bound, keyer.nodes
		}
		if seen[key] {
			duplicates++
			continue
		}
		seen[key] = true
	}
	return bound - duplicates, keyer.nodes
}

type ps6092ExpressionKeyer struct {
	pass      *analysis.Pass
	typeKeys  map[types.Type]string
	objectIDs map[types.Object]int
	nodes     int
}

func (keyer *ps6092ExpressionKeyer) key(expression ast.Expr) (string, bool) {
	var key strings.Builder
	key.Grow(64)
	if !keyer.append(&key, expression) {
		return "", false
	}
	return key.String(), true
}

func (keyer *ps6092ExpressionKeyer) append(key *strings.Builder, expression ast.Expr) bool {
	key.Grow(32)
	expression = ps2110Unparen(expression)
	keyer.nodes++
	key.WriteByte('(')
	switch value := expression.(type) {
	case *ast.Ident:
		key.WriteByte('i')
		keyer.appendType(key, keyer.pass.TypesInfo.TypeOf(value))
		key.WriteByte(':')
		key.WriteString(strconv.Quote(value.Name))
		key.WriteByte(':')
		key.WriteString(strconv.Itoa(keyer.objectID(keyer.pass.TypesInfo.ObjectOf(value))))
	case *ast.BasicLit:
		key.WriteByte('l')
		keyer.appendType(key, keyer.pass.TypesInfo.TypeOf(value))
		key.WriteByte(':')
		key.WriteString(strconv.Itoa(int(value.Kind)))
		key.WriteByte(':')
		key.WriteString(strconv.Quote(value.Value))
	case *ast.UnaryExpr:
		if value.Op == token.AND || value.Op == token.ARROW {
			return false
		}
		key.WriteByte('u')
		keyer.appendType(key, keyer.pass.TypesInfo.TypeOf(value))
		key.WriteByte(':')
		key.WriteString(strconv.Itoa(int(value.Op)))
		if !keyer.append(key, value.X) {
			return false
		}
	case *ast.BinaryExpr:
		key.WriteByte('b')
		keyer.appendType(key, keyer.pass.TypesInfo.TypeOf(value))
		key.WriteByte(':')
		key.WriteString(strconv.Itoa(int(value.Op)))
		if !keyer.append(key, value.X) || !keyer.append(key, value.Y) {
			return false
		}
	case *ast.SelectorExpr:
		key.WriteByte('s')
		keyer.appendType(key, keyer.pass.TypesInfo.TypeOf(value))
		key.WriteByte(':')
		key.WriteString(strconv.Itoa(keyer.objectID(ps6092SelectorObject(keyer.pass, value))))
		if !keyer.append(key, value.X) {
			return false
		}
	case *ast.IndexExpr:
		key.WriteByte('x')
		keyer.appendType(key, keyer.pass.TypesInfo.TypeOf(value))
		if !keyer.append(key, value.X) || !keyer.append(key, value.Index) {
			return false
		}
	case *ast.IndexListExpr:
		key.WriteByte('g')
		keyer.appendType(key, keyer.pass.TypesInfo.TypeOf(value))
		key.WriteByte(':')
		key.WriteString(strconv.Itoa(len(value.Indices)))
		if !keyer.append(key, value.X) {
			return false
		}
		for _, index := range value.Indices {
			if !keyer.append(key, index) {
				return false
			}
		}
	case *ast.TypeAssertExpr:
		key.WriteByte('a')
		keyer.appendType(key, keyer.pass.TypesInfo.TypeOf(value))
		if !keyer.append(key, value.X) {
			return false
		}
	case *ast.CallExpr:
		typed, known := keyer.pass.TypesInfo.Types[ps2110Unparen(value.Fun)]
		if !known || !typed.IsType() {
			return false
		}
		key.WriteByte('c')
		keyer.appendType(key, keyer.pass.TypesInfo.TypeOf(value))
		key.WriteByte(':')
		key.WriteString(strconv.Itoa(len(value.Args)))
		if value.Ellipsis.IsValid() {
			key.WriteByte('e')
		}
		for _, argument := range value.Args {
			if !keyer.append(key, argument) {
				return false
			}
		}
	case *ast.CompositeLit:
		key.WriteByte('o')
		keyer.appendType(key, keyer.pass.TypesInfo.TypeOf(value))
		key.WriteByte(':')
		key.WriteString(strconv.Itoa(len(value.Elts)))
		for _, element := range value.Elts {
			keyed, hasKey := element.(*ast.KeyValueExpr)
			if hasKey {
				key.WriteByte('k')
				if !keyer.append(key, keyed.Key) || !keyer.append(key, keyed.Value) {
					return false
				}
				continue
			}
			key.WriteByte('v')
			if !keyer.append(key, element) {
				return false
			}
		}
	default:
		return false
	}
	key.WriteByte(')')
	return true
}

func (keyer *ps6092ExpressionKeyer) appendType(key *strings.Builder, value types.Type) {
	value = types.Unalias(value)
	if value == nil {
		key.WriteString(":-")
		return
	}
	typeKey, known := keyer.typeKeys[value]
	if !known {
		typeKey = types.TypeString(value, func(pkg *types.Package) string {
			if pkg == nil {
				return ""
			}
			return pkg.Path()
		})
		keyer.typeKeys[value] = typeKey
	}
	key.WriteByte(':')
	key.WriteString(strconv.Quote(typeKey))
}

func (keyer *ps6092ExpressionKeyer) objectID(object types.Object) int {
	if object == nil {
		return 0
	}
	if identifier := keyer.objectIDs[object]; identifier != 0 {
		return identifier
	}
	identifier := len(keyer.objectIDs) + 1
	keyer.objectIDs[object] = identifier
	return identifier
}

func ps6092SelectorObject(pass *analysis.Pass, selector *ast.SelectorExpr) types.Object {
	if selection := pass.TypesInfo.Selections[selector]; selection != nil {
		return selection.Obj()
	}
	return pass.TypesInfo.ObjectOf(selector.Sel)
}

func ps6092ReflexiveMapKey(value types.Type, visiting map[types.Type]bool) bool {
	value = types.Unalias(value)
	if value == nil || visiting[value] {
		return false
	}
	visiting[value] = true
	defer delete(visiting, value)
	switch underlying := value.Underlying().(type) {
	case *types.Basic:
		return underlying.Kind() != types.Float32 && underlying.Kind() != types.Float64 &&
			underlying.Kind() != types.Complex64 && underlying.Kind() != types.Complex128
	case *types.Pointer, *types.Chan:
		return true
	case *types.Array:
		return ps6092ReflexiveMapKey(underlying.Elem(), visiting)
	case *types.Struct:
		for index := range underlying.NumFields() {
			if !ps6092ReflexiveMapKey(underlying.Field(index).Type(), visiting) {
				return false
			}
		}
		return true
	default:
		// Interfaces and type parameters may carry a float, complex value, or a
		// composite containing one. Functions, maps, and slices cannot be keys.
		return false
	}
}

func ps6092LoopStep(pass *analysis.Pass, statement ast.Stmt, object types.Object) (int64, bool) {
	switch value := statement.(type) {
	case *ast.IncDecStmt:
		if !ps6092DirectObject(pass, value.X, object) {
			return 0, false
		}
		if value.Tok == token.INC {
			return 1, true
		}
		if value.Tok == token.DEC {
			return -1, true
		}
	case *ast.AssignStmt:
		if len(value.Lhs) != 1 || len(value.Rhs) != 1 || !ps6092DirectObject(pass, value.Lhs[0], object) {
			return 0, false
		}
		step, ok := ps6092Int64Constant(pass, value.Rhs[0])
		if !ok {
			return 0, false
		}
		switch value.Tok {
		case token.ADD_ASSIGN:
			return step, true
		case token.SUB_ASSIGN:
			if step == math.MinInt64 {
				return math.MinInt64, true
			}
			return -step, true
		}
	}
	return 0, false
}

func ps6092NodeMutates(pass *analysis.Pass, node ast.Node, object types.Object) bool {
	if node == nil {
		return false
	}
	mutated := false
	parents := ps6092ParentMap(node)
	ast.Inspect(node, func(node ast.Node) bool {
		if mutated {
			return false
		}
		if literal, nested := node.(*ast.FuncLit); nested && !ps6092ImmediatelyInvoked(parents, literal) {
			if ps6092NonexecutingLiteral(pass, parents, literal) {
				return false
			}
		}
		switch value := node.(type) {
		case *ast.AssignStmt:
			for _, left := range value.Lhs {
				if ps6092DirectObject(pass, left, object) {
					mutated = true
					return false
				}
			}
		case *ast.IncDecStmt:
			mutated = ps6092DirectObject(pass, value.X, object)
		case *ast.RangeStmt:
			mutated = ps6092DirectObject(pass, value.Key, object) || ps6092DirectObject(pass, value.Value, object)
		case *ast.UnaryExpr:
			mutated = value.Op == token.AND && ps6092DirectObject(pass, value.X, object)
		case *ast.SelectorExpr:
			if !ps6092DirectObject(pass, value.X, object) {
				break
			}
			selection := pass.TypesInfo.Selections[value]
			method, ok := pass.TypesInfo.Uses[value.Sel].(*types.Func)
			if selection == nil || !ok {
				break
			}
			signature, ok := method.Type().(*types.Signature)
			if ok && signature.Recv() != nil {
				_, mutated = types.Unalias(signature.Recv().Type()).(*types.Pointer)
			}
		}
		return !mutated
	})
	return mutated
}

func ps6092NonexecutingLiteral(pass *analysis.Pass, parents map[ast.Node]ast.Node, literal *ast.FuncLit) bool {
	expression, parent := ps6092LiteralParent(pass, parents, literal)
	if assignment, ok := parent.(*ast.AssignStmt); ok && ps6092BlankAssignedValue(assignment, expression) {
		return true
	}
	if declaration, ok := parent.(*ast.ValueSpec); ok && ps6092BlankValue(declaration, expression) {
		return true
	}
	call, ok := parent.(*ast.CallExpr)
	if !ok || call.Fun != expression {
		return false
	}
	_, deferred := parents[call].(*ast.DeferStmt)
	return deferred
}

func ps6092DiscardedLiteral(pass *analysis.Pass, parents map[ast.Node]ast.Node, literal *ast.FuncLit) bool {
	expression, parent := ps6092LiteralParent(pass, parents, literal)
	if assignment, ok := parent.(*ast.AssignStmt); ok {
		return ps6092BlankAssignedValue(assignment, expression)
	}
	declaration, ok := parent.(*ast.ValueSpec)
	return ok && ps6092BlankValue(declaration, expression)
}

func ps6092LiteralParent(pass *analysis.Pass, parents map[ast.Node]ast.Node, literal *ast.FuncLit) (ast.Expr, ast.Node) {
	var expression ast.Expr = literal
	parent := parents[literal]
	for {
		if parentheses, ok := parent.(*ast.ParenExpr); ok && parentheses.X == expression {
			expression = parentheses
			parent = parents[parent]
			continue
		}
		conversion, ok := parent.(*ast.CallExpr)
		if !ok || len(conversion.Args) != 1 || conversion.Args[0] != expression || conversion.Ellipsis.IsValid() ||
			!ps6092TypeConversion(pass, conversion) {
			break
		}
		expression = conversion
		parent = parents[parent]
	}
	return expression, parent
}

func ps6092TypeConversion(pass *analysis.Pass, call *ast.CallExpr) bool {
	typed, known := pass.TypesInfo.Types[ps2110Unparen(call.Fun)]
	return known && typed.IsType()
}

func ps6092BlankAssignedValue(assignment *ast.AssignStmt, expression ast.Expr) bool {
	if len(assignment.Lhs) != len(assignment.Rhs) {
		return false
	}
	for index, right := range assignment.Rhs {
		identifier, blank := ps2110Unparen(assignment.Lhs[index]).(*ast.Ident)
		if right == expression && blank && identifier.Name == "_" {
			return true
		}
	}
	return false
}

func ps6092BlankValue(declaration *ast.ValueSpec, expression ast.Expr) bool {
	if len(declaration.Names) != len(declaration.Values) {
		return false
	}
	for index, value := range declaration.Values {
		if value == expression && declaration.Names[index].Name == "_" {
			return true
		}
	}
	return false
}

func ps6092DirectObject(pass *analysis.Pass, expression ast.Expr, object types.Object) bool {
	if expression == nil {
		return false
	}
	identifier, ok := ps2110Unparen(expression).(*ast.Ident)
	return ok && (pass.TypesInfo.Uses[identifier] == object || pass.TypesInfo.Defs[identifier] == object)
}

func ps6092Int64Constant(pass *analysis.Pass, expression ast.Expr) (int64, bool) {
	typed, ok := pass.TypesInfo.Types[ps2110Unparen(expression)]
	if !ok || typed.Value == nil || typed.Value.Kind() != constant.Int {
		return 0, false
	}
	return constant.Int64Val(typed.Value)
}

func ps6092BoolConstant(pass *analysis.Pass, expression ast.Expr) (bool, bool) {
	if expression == nil {
		return false, false
	}
	typed, ok := pass.TypesInfo.Types[ps2110Unparen(expression)]
	if !ok || typed.Value == nil || typed.Value.Kind() != constant.Bool {
		return false, false
	}
	return constant.BoolVal(typed.Value), true
}

func ps6092ReverseComparison(operator token.Token) token.Token {
	switch operator {
	case token.LSS:
		return token.GTR
	case token.LEQ:
		return token.GEQ
	case token.GTR:
		return token.LSS
	case token.GEQ:
		return token.LEQ
	case token.EQL, token.NEQ:
		return operator
	default:
		return token.ILLEGAL
	}
}

func ps6092CompareInt(left int64, operator token.Token, right int64) bool {
	switch operator {
	case token.LSS:
		return left < right
	case token.LEQ:
		return left <= right
	case token.GTR:
		return left > right
	case token.GEQ:
		return left >= right
	case token.EQL:
		return left == right
	case token.NEQ:
		return left != right
	}
	return false
}

func ps6092IntegerBounds(pass *analysis.Pass, value types.Type) (int64, int64, bool) {
	basic, ok := types.Unalias(value).Underlying().(*types.Basic)
	if !ok || basic.Info()&types.IsInteger == 0 {
		return 0, 0, false
	}
	switch basic.Kind() {
	case types.Int8:
		return math.MinInt8, math.MaxInt8, true
	case types.Int16:
		return math.MinInt16, math.MaxInt16, true
	case types.Int32:
		return math.MinInt32, math.MaxInt32, true
	case types.Int64:
		return math.MinInt64, math.MaxInt64, true
	case types.Uint8:
		return 0, math.MaxUint8, true
	case types.Uint16:
		return 0, math.MaxUint16, true
	case types.Uint32:
		return 0, math.MaxUint32, true
	case types.Uint64:
		return 0, math.MaxInt64, true
	case types.Int, types.Uint, types.Uintptr:
		if pass.TypesSizes == nil {
			return 0, 0, false
		}
		bits := pass.TypesSizes.Sizeof(value) * 8
		if bits <= 0 || bits > 64 {
			return 0, 0, false
		}
		if basic.Kind() == types.Int {
			if bits == 64 {
				return math.MinInt64, math.MaxInt64, true
			}
			maximum := int64(1)<<(bits-1) - 1
			return -maximum - 1, maximum, true
		}
		if bits == 64 {
			return 0, math.MaxInt64, true
		}
		return 0, int64(1)<<bits - 1, true
	}
	return 0, 0, false
}

func ps6092WrappedAdd(value, step, minimum, maximum int64) (int64, bool) {
	if minimum == math.MinInt64 && maximum == math.MaxInt64 {
		return int64(uint64(value) + uint64(step)), true
	}
	// uint64 (and 64-bit uint/uintptr) values above MaxInt64 cannot be
	// represented by this deliberately small simulator. Abandon the proof
	// instead of pretending that MaxInt64 is their wrap boundary.
	if minimum == 0 && maximum == math.MaxInt64 {
		if step > 0 && value > maximum-step || step < 0 && (step == math.MinInt64 || value < -step) {
			return 0, false
		}
		return value + step, true
	}
	if step > 0 && value > maximum-step {
		overflow := step - (maximum - value) - 1
		return minimum + overflow, true
	}
	if step < 0 {
		if step == math.MinInt64 {
			return 0, false
		}
		if value < minimum-step {
			underflow := -step - (value - minimum) - 1
			return maximum - underflow, true
		}
	}
	return value + step, true
}

func ps6092StaticallyDead(pass *analysis.Pass, parents map[ast.Node]ast.Node, node ast.Node) bool {
	child := node
	for parent := parents[node]; parent != nil; child, parent = parent, parents[parent] {
		switch value := parent.(type) {
		case *ast.IfStmt:
			condition, known := ps6092BoolConstant(pass, value.Cond)
			if !known {
				break
			}
			if ps6092Within(child, value.Body) && !condition || value.Else != nil && ps6092Within(child, value.Else) && condition {
				return true
			}
		case *ast.BinaryExpr:
			if !ps6092Within(child, value.Y) {
				break
			}
			left, known := ps6092BoolConstant(pass, value.X)
			if known && (value.Op == token.LAND && !left || value.Op == token.LOR && left) {
				return true
			}
		case *ast.CaseClause:
			if ps6092ConstantSwitchCaseDead(pass, parents, child, value) {
				return true
			}
		}
	}
	return false
}

func ps6092ConstantSwitchCaseDead(
	pass *analysis.Pass,
	parents map[ast.Node]ast.Node,
	child ast.Node,
	clause *ast.CaseClause,
) bool {
	switchBody, inSwitchBody := parents[clause].(*ast.BlockStmt)
	switchStatement, expressionSwitch := parents[switchBody].(*ast.SwitchStmt)
	if !inSwitchBody || !expressionSwitch || switchStatement.Body != switchBody || len(clause.List) == 0 || len(clause.Body) == 0 ||
		child.Pos() < clause.Body[0].Pos() || child.End() > clause.Body[len(clause.Body)-1].End() {
		return false
	}
	for index, statement := range switchStatement.Body.List {
		if statement != clause {
			continue
		}
		if index > 0 && ps6092CaseFallsThrough(switchStatement.Body.List[index-1]) {
			return false
		}
		break
	}

	switchValue := constant.MakeBool(true)
	if switchStatement.Tag != nil {
		typed, known := pass.TypesInfo.Types[ps2110Unparen(switchStatement.Tag)]
		if !known || typed.Value == nil {
			return false
		}
		switchValue = typed.Value
	}
	for _, expression := range clause.List {
		typed, known := pass.TypesInfo.Types[ps2110Unparen(expression)]
		if !known || typed.Value == nil {
			return false
		}
		equal, comparable := ps6092ConstantEqual(switchValue, typed.Value)
		if !comparable || equal {
			return false
		}
	}
	return true
}

func ps6092CaseFallsThrough(statement ast.Stmt) bool {
	clause, ok := statement.(*ast.CaseClause)
	if !ok || len(clause.Body) == 0 {
		return false
	}
	statement = clause.Body[len(clause.Body)-1]
	for {
		labeled, labeledStatement := statement.(*ast.LabeledStmt)
		if !labeledStatement {
			break
		}
		statement = labeled.Stmt
	}
	branch, ok := statement.(*ast.BranchStmt)
	return ok && branch.Tok == token.FALLTHROUGH
}

func ps6092ConstantEqual(left, right constant.Value) (bool, bool) {
	if left == nil || right == nil || left.Kind() == constant.Unknown || right.Kind() == constant.Unknown {
		return false, false
	}
	if left.Kind() == constant.Bool || right.Kind() == constant.Bool {
		if left.Kind() != constant.Bool || right.Kind() != constant.Bool {
			return false, false
		}
		return constant.Compare(left, token.EQL, right), true
	}
	if left.Kind() == constant.String || right.Kind() == constant.String {
		if left.Kind() != constant.String || right.Kind() != constant.String {
			return false, false
		}
		return constant.Compare(left, token.EQL, right), true
	}
	if ps6092NumericConstant(left) && ps6092NumericConstant(right) {
		return constant.Compare(left, token.EQL, right), true
	}
	return false, false
}

func ps6092NumericConstant(value constant.Value) bool {
	switch value.Kind() {
	case constant.Int, constant.Float, constant.Complex:
		return true
	default:
		return false
	}
}

func ps6092Within(node, container ast.Node) bool {
	return node != nil && container != nil && container.Pos() <= node.Pos() && node.End() <= container.End()
}

func ps6092CompilerEvidenceSuppressed(pass *analysis.Pass, file *ast.File, call *ast.CallExpr) bool {
	callLine := pass.Fset.Position(call.Pos()).Line
	for _, group := range file.Comments {
		start := pass.Fset.Position(group.Pos()).Line
		end := pass.Fset.Position(group.End()).Line
		if start > callLine || end < callLine-1 {
			continue
		}
		var raw strings.Builder
		raw.Grow(int(group.End() - group.Pos()))
		for _, comment := range group.List {
			raw.WriteString(comment.Text)
			raw.WriteByte('\n')
		}
		text := strings.ToLower(raw.String())
		marker := strings.Contains(text, "perfscan:generic-dispatch-verified") || strings.Contains(text, "perfscan:ignore ps6092")
		toolchain := strings.Contains(text, "go1.")
		assembly := strings.Contains(text, "objdump") || strings.Contains(text, "compile -s") ||
			strings.Contains(text, "gcflags") && strings.Contains(text, "-s") || strings.Contains(text, "disassembly")
		result := strings.Contains(text, "shows a direct call") || strings.Contains(text, "direct call emitted") ||
			strings.Contains(text, "no indirect") || strings.Contains(text, "no blr") || strings.Contains(text, "no jalr") || strings.Contains(text, "no call *")
		if marker && toolchain && assembly && result {
			return true
		}
	}
	return false
}

func ps6092SharedZeroShapeInstantiations(pass *analysis.Pass) map[*types.TypeParam]int {
	instances := make(map[types.Object][]types.Instance)
	for identifier, instance := range pass.TypesInfo.Instances {
		if object := pass.TypesInfo.Uses[identifier]; object != nil {
			instances[object] = append(instances[object], instance)
		}
	}
	evidence := make(map[*types.TypeParam]int)
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok {
				continue
			}
			object, ok := pass.TypesInfo.Defs[function.Name].(*types.Func)
			if !ok {
				continue
			}
			signature, ok := object.Type().(*types.Signature)
			if !ok {
				continue
			}
			for index := range signature.TypeParams().Len() {
				if count := ps6092SameShapeInstantiationCount(instances[object], index); count >= 2 {
					evidence[signature.TypeParams().At(index)] = count
				}
			}
		}
	}
	return evidence
}

func ps6092SameShapeInstantiationCount(instances []types.Instance, target int) int {
	type group struct {
		exemplar types.Instance
		targets  []types.Type
	}
	var groups []group
	maximum := 0
	for _, instance := range instances {
		if target >= instance.TypeArgs.Len() || !ps6092EmptyStruct(instance.TypeArgs.At(target)) {
			continue
		}
		groupIndex := -1
		for index := range groups {
			if ps6092SameOtherTypeArguments(instance, groups[index].exemplar, target) {
				groupIndex = index
				break
			}
		}
		if groupIndex < 0 {
			groups = append(groups, group{exemplar: instance})
			groupIndex = len(groups) - 1
		}
		argument := instance.TypeArgs.At(target)
		seen := false
		for _, existing := range groups[groupIndex].targets {
			if types.Identical(existing, argument) {
				seen = true
				break
			}
		}
		if !seen {
			groups[groupIndex].targets = append(groups[groupIndex].targets, argument)
			maximum = max(maximum, len(groups[groupIndex].targets))
		}
	}
	return maximum
}

func ps6092SameOtherTypeArguments(left, right types.Instance, target int) bool {
	if left.TypeArgs.Len() != right.TypeArgs.Len() {
		return false
	}
	for index := range left.TypeArgs.Len() {
		if index != target && !types.Identical(left.TypeArgs.At(index), right.TypeArgs.At(index)) {
			return false
		}
	}
	return true
}

func ps6092EmptyStruct(value types.Type) bool {
	structure, ok := types.Unalias(value).Underlying().(*types.Struct)
	return ok && structure.NumFields() == 0
}
