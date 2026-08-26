package checks

import (
	"cmp"
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"slices"
	"strings"
	"sync"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/cfg"

	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS6089 implements owner issue #826. It deliberately reports candidates and
// evidence defects only: fusing recorder operations changes GPU ordering,
// resource lifetimes, and command topology and therefore cannot be auto-fixed.
var PS6089 = register(&lint.Check{
	ID:       "PS6089",
	Category: "verify",
	Slug:     "gpu-fusion-coverage-command-lifecycle",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "GPU fusion claims must cover production topologies and command lifecycle",
		Text: `A leaf GPU benchmark can hide a useful fusion behind command-buffer
creation, commit, and synchronization on every operation. Conversely, a fast
exact fusion may cover only one production topology while grouped or packed
variants keep the old recorder boundaries.

This opt-in advisory has three deliberately separate inputs:

  - typed static recorder calls: adjacent RoPE/rotary and KV append/store/copy
    methods must resolve to the same command receiver and share both a data
    buffer and a shape key before they are nominated as a fusion candidate;
  - real testing.B loops: creating, committing, and waiting for a command
    buffer in every leaf iteration is flagged when the loop records the
    candidate operations; and
  - GPUFusionCoverageEvidence, RecorderFusionCampaign,
    CommandLifecycleFusionEvidence, or FusionTopologyCoverageReport manifests:
    production/covered/unfused topology sets, leaf and production command
    depths and timings, event counts, exactness/profile/event oracles, frozen
    promotion threshold, and decision state are cross-checked.

Static call identity never mixes receivers or leaf and production paths.
Lookalike names, CPU helpers, fused calls, different buffers or shape keys,
and invalid-only vocabulary without a typed manifest stay silent. Partial
coverage and per-leaf command lifecycle remain advisory findings even when an
individual fused path is exact.

There is NO automatic fix. Promotion requires full declared production
coverage, fewer recorder events, exact output, a profile oracle, a same-workload
production benchmark above its frozen threshold, and human review of GPU
ordering and resource lifetime changes.`,
		Before: `for i := 0; i < b.N; i++ {
	cmd := device.NewCommandBuffer()
	cmd.EncodeRoPE(key, shape)
	cmd.AppendKV(key, shape)
	cmd.Commit()
	cmd.WaitUntilCompleted()
}`,
		After: `// Candidate only: benchmark the exact production command lifecycle.
cmd := device.NewCommandBuffer()
for i := 0; i < productionDepth; i++ {
	cmd.EncodeFusedRoPEKV(key[i], shape)
}
cmd.Commit()
cmd.WaitUntilCompleted()
// Retain only with full topology, event-count, profile, and exactness gates.`,
		MeasuredWin: `The issue #826 investigation measured only 1.07–1.09x when
each leaf invocation created and committed a command buffer, but 1.67–1.82x
for the production-shaped 22-boundary command. The first selector covered only
10 of 22 layer variants because 12 used grouped-QKV topology. Covering both
topologies reduced 54 RoPE/copy events to 22 fused events and produced
1.0163–1.0574x end-to-end gains.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6089",
		Doc:  "audit GPU fusion topology coverage and command lifecycle parity",
		Run:  runPS6089,
	},
})

type ps6089Call struct {
	call       *ast.CallExpr
	name       string
	receiverTy types.Type
	receiverID []types.Object
	data       [][]types.Object
	shape      [][]types.Object
	shapeConst []ps6089ShapeConstant
}

type ps6089ShapeConstant struct {
	typeOf types.Type
	value  string
}

func runPS6089(pass *analysis.Pass) (any, error) {
	defer ps6089FunctionReturns.Delete(pass)
	for _, file := range pass.Files {
		ps6089ReportManifests(pass, file)
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			ps6089ReportStaticFusion(pass, function)
			ps6089ReportLeafLifecycle(pass, function)
		}
	}
	return nil, nil
}

type ps6089FunctionReturnIndex struct {
	once         sync.Once
	declarations map[*types.Func]*ast.FuncDecl
	known        map[*types.Func]bool
	returns      map[*types.Func]bool
	mu           sync.Mutex
}

var ps6089FunctionReturns sync.Map // map[*analysis.Pass]*ps6089FunctionReturnIndex

func ps6089ReportStaticFusion(pass *analysis.Pass, function *ast.FuncDecl) {
	ps6032Blocks(function.Body, func(block *ast.BlockStmt) {
		for index := 0; index+1 < len(block.List); index++ {
			rotary, rotaryOK := ps6089RecorderCall(pass, ps6089UnlabelStatement(block.List[index]))
			cache, cacheOK := ps6089RecorderCall(pass, ps6089UnlabelStatement(block.List[index+1]))
			if !rotaryOK || !cacheOK || !ps6089RotaryName(rotary.name) || !ps6089CacheName(cache.name) {
				continue
			}
			if !ps6089SameReceiver(&rotary, &cache) || !ps6089GPUCommandType(rotary.receiverTy) || !types.Identical(rotary.receiverTy, cache.receiverTy) {
				continue
			}
			if !ps6089SharedData(&rotary, &cache) || !ps6089SharedShape(&rotary, &cache) {
				continue
			}
			pass.Reportf(rotary.call.Pos(), "adjacent typed GPU recorder operations %s -> %s share one command receiver, data buffer, and shape key; inventory every production topology before evaluating a fused candidate, and validate event-count, profile, exactness, and end-to-end production gates", rotary.name, cache.name)
		}
	})
}

func ps6089RecorderCall(pass *analysis.Pass, statement ast.Stmt) (ps6089Call, bool) {
	call := ps6032StatementCall(statement)
	if call == nil {
		return ps6089Call{}, false
	}
	function, signature, ok := typedCallee(pass, call.Fun)
	if !ok || signature.Recv() == nil {
		return ps6089Call{}, false
	}
	receiverID, receiverType, firstArgument, ok := ps6089CallReceiver(pass, call)
	if !ok {
		return ps6089Call{}, false
	}
	result := ps6089Call{call: call, name: function.Name(), receiverTy: receiverType, receiverID: receiverID}
	for _, argument := range call.Args[firstArgument:] {
		typeOf := pass.TypesInfo.TypeOf(argument)
		if ps6089DataType(typeOf) {
			if path := ps6089StableReceiverID(pass, argument); len(path) > 0 {
				result.data = append(result.data, path)
			}
			continue
		}
		if ps6089ShapeType(typeOf) {
			if path := ps6089StableReceiverID(pass, argument); len(path) > 0 {
				result.shape = append(result.shape, path)
				continue
			}
			if value := pass.TypesInfo.Types[ps2110Unparen(argument)].Value; value != nil {
				result.shapeConst = append(result.shapeConst, ps6089ShapeConstant{typeOf: types.Unalias(typeOf), value: value.ExactString()})
			}
		}
	}
	return result, true
}

func ps6089StableReceiverID(pass *analysis.Pass, expression ast.Expr) []types.Object {
	switch value := ps2110Unparen(expression).(type) {
	case *ast.Ident:
		object := identObject(pass, value)
		if object == nil {
			return nil
		}
		return []types.Object{object}
	case *ast.SelectorExpr:
		selection := pass.TypesInfo.Selections[value]
		if selection == nil || selection.Kind() != types.FieldVal {
			return nil
		}
		path := ps6089StableReceiverID(pass, value.X)
		if len(path) == 0 {
			return nil
		}
		fields := ps6089SelectionFields(selection)
		if len(fields) == 0 {
			return nil
		}
		return append(path, fields...)
	case *ast.StarExpr:
		return ps6089StableReceiverID(pass, value.X)
	}
	return nil
}

func ps6089SelectionFields(selection *types.Selection) []types.Object {
	fields, _, ok := ps6089SelectionFieldPath(selection.Recv(), selection.Index())
	if !ok {
		return nil
	}
	return fields
}

func ps6089SelectionFieldPath(receiver types.Type, indexes []int) ([]types.Object, types.Type, bool) {
	current := receiver
	fields := make([]types.Object, 0, len(indexes))
	for _, index := range indexes {
		current = ps6089DereferenceType(current)
		if current == nil {
			return nil, nil, false
		}
		structure, ok := current.Underlying().(*types.Struct)
		if !ok || index < 0 || index >= structure.NumFields() {
			return nil, nil, false
		}
		field := structure.Field(index)
		fields = append(fields, field)
		current = field.Type()
	}
	return fields, current, true
}

func ps6089CallReceiver(pass *analysis.Pass, call *ast.CallExpr) ([]types.Object, types.Type, int, bool) {
	selector, ok := ps2110Unparen(call.Fun).(*ast.SelectorExpr)
	if !ok {
		return nil, nil, 0, false
	}
	selection := pass.TypesInfo.Selections[selector]
	if selection == nil {
		return nil, nil, 0, false
	}
	switch selection.Kind() {
	case types.MethodVal:
		receiverID, receiverType, valid := ps6089SelectedReceiver(pass, selector.X, selection)
		if !valid {
			return nil, nil, 0, false
		}
		return receiverID, receiverType, 0, true
	case types.MethodExpr:
		if len(call.Args) == 0 {
			return nil, nil, 0, false
		}
		receiverID, receiverType, valid := ps6089SelectedReceiver(pass, call.Args[0], selection)
		if !valid {
			return nil, nil, 0, false
		}
		return receiverID, receiverType, 1, true
	}
	return nil, nil, 0, false
}

func ps6089SelectedReceiver(pass *analysis.Pass, expression ast.Expr, selection *types.Selection) ([]types.Object, types.Type, bool) {
	receiverID := ps6089StableReceiverID(pass, expression)
	indexes := selection.Index()
	if len(receiverID) == 0 || len(indexes) == 0 {
		return nil, nil, false
	}
	fields, receiverType, valid := ps6089SelectionFieldPath(pass.TypesInfo.TypeOf(expression), indexes[:len(indexes)-1])
	if !valid {
		return nil, nil, false
	}
	return append(receiverID, fields...), receiverType, true
}

func ps6089DereferenceType(value types.Type) types.Type {
	for value != nil {
		value = types.Unalias(value)
		pointer, ok := value.(*types.Pointer)
		if !ok {
			return value
		}
		value = pointer.Elem()
	}
	return nil
}

func ps6089SameReceiver(left, right *ps6089Call) bool {
	return len(left.receiverID) > 0 && slices.Equal(left.receiverID, right.receiverID)
}

func ps6089RotaryName(name string) bool {
	switch ps6007NormalizeName(name) {
	case "encoderope", "applyrope", "rope", "encoderotary", "applyrotary", "rotary":
		return true
	}
	return false
}

func ps6089CacheName(name string) bool {
	switch ps6007NormalizeName(name) {
	case "appendkv", "appendkvcache", "appendkeyvaluecache", "storekv", "storekvcache", "copykv", "copykvcache", "updatekv", "updatekvcache":
		return true
	}
	return false
}

func ps6089ShapeType(value types.Type) bool {
	if value == nil {
		return false
	}
	basic, ok := types.Unalias(value).Underlying().(*types.Basic)
	return ok && basic.Info()&(types.IsInteger|types.IsBoolean|types.IsString) != 0
}

func ps6089GPUCommandType(value types.Type) bool {
	value = types.Unalias(value)
	if !ps6022CommandType(value) {
		return false
	}
	name := ps6007NormalizeName(types.TypeString(value, func(pkg *types.Package) string {
		if pkg == nil {
			return ""
		}
		return pkg.Path()
	}))
	return ps6007ContainsAny(name, "gpu", "metal", "mps", "cuda", "vulkan")
}

func ps6089DataType(value types.Type) bool {
	return value != nil && ps6022DataType(types.Unalias(value))
}

func ps6089SharedData(left, right *ps6089Call) bool {
	for _, first := range left.data {
		for _, second := range right.data {
			if slices.Equal(first, second) {
				return true
			}
		}
	}
	return false
}

func ps6089SharedShape(left, right *ps6089Call) bool {
	for _, first := range left.shape {
		for _, second := range right.shape {
			if slices.Equal(first, second) {
				return true
			}
		}
	}
	for _, first := range left.shapeConst {
		for _, second := range right.shapeConst {
			if first.value == second.value && types.Identical(first.typeOf, second.typeOf) {
				return true
			}
		}
	}
	return false
}

func ps6089ReportLeafLifecycle(pass *analysis.Pass, function *ast.FuncDecl) {
	if !ps6011Harness(pass, function) || !strings.HasPrefix(function.Name.Name, "Benchmark") {
		return
	}
	parents := ps6089Parents(function.Body)
	flows := make(map[*ast.BlockStmt]*ps6089LifecycleFlow)
	aliasesByBody := make(map[*ast.BlockStmt]*ps6089ReceiverAliasIndex)
	promotedPaths := make(map[types.Type]ps6089PromotedLifecyclePath)
	ast.Inspect(function.Body, func(node ast.Node) bool {
		if literal, nested := node.(*ast.FuncLit); nested {
			return ps6089BenchmarkSubbenchmark(pass, parents, literal)
		}
		var position ast.Node
		var body *ast.BlockStmt
		switch loop := node.(type) {
		case *ast.ForStmt:
			if loop.Body != nil && ps6089BenchmarkForIterations(pass, loop) {
				position, body = loop, loop.Body
			}
		case *ast.RangeStmt:
			if loop.Body != nil && ps6089BenchmarkRangeIterations(pass, loop.X) {
				position, body = loop, loop.Body
			}
		}
		if body == nil {
			return true
		}
		executionBody := ps6089ExecutionBody(parents, position, function.Body)
		aliases := aliasesByBody[executionBody]
		if aliases == nil {
			aliases = ps6089ReceiverAliases(pass, executionBody)
			aliasesByBody[executionBody] = aliases
		}
		groups := ps6089CollectLifecycleEvents(pass, parents, body, executionBody, aliases, promotedPaths)
		for _, events := range groups {
			for _, statement := range events.scope.List {
				for _, write := range ps6089ExpandReceiverWrites(ps6089WrittenReceivers(pass, statement, aliases.values), aliases) {
					events.writes = append(events.writes, ps6089OrderedReceiver{id: write.id, order: int(write.pos), pos: write.pos})
				}
			}
		}
		if !aliases.complete {
			return true
		}
		lifecycle := false
		for groupBody, events := range groups {
			groupFlow := flows[groupBody]
			if groupFlow == nil {
				groupFlow = ps6089NewLifecycleFlow(pass, groupBody)
				flows[groupBody] = groupFlow
			}
			entry := ps6089LifecycleEntryPosition(groupFlow, events.scope)
			if entry == token.NoPos {
				continue
			}
			budget := ps6089NewLifecycleBudget(groupFlow, events)
			invocationsReturn := true
			for _, invocation := range events.invocations {
				invocationFlow := flows[invocation.body]
				if invocationFlow == nil {
					invocationFlow = ps6089NewLifecycleFlow(pass, invocation.body)
					flows[invocation.body] = invocationFlow
				}
				invocationEntry := ps6089LifecycleEntryPosition(invocationFlow, invocation.scope)
				if invocation.call == nil || invocationEntry == token.NoPos ||
					!ps6089PositionPostDominatesOrEqualBudget(pass, invocationFlow, invocationEntry, invocation.call.Pos(), budget) ||
					!ps6088ExpressionReturnsNormally(pass, invocation.call) ||
					!ps6089NamedCallsReturn(pass, invocation.call, nil) {
					invocationsReturn = false
					break
				}
			}
			if !invocationsReturn {
				continue
			}
			repeats := func(created, waited ps6089OrderedReceiver) bool {
				if len(events.invocations) == 0 {
					return ps6089PositionCanReachRecurringBudget(pass, groupFlow, waited.pos, created.pos, budget)
				}
				outer := events.invocations[len(events.invocations)-1]
				outerFlow := flows[outer.body]
				return outer.call != nil && outerFlow != nil &&
					ps6089PositionCanReachRecurringBudget(pass, outerFlow, outer.call.Pos(), outer.call.Pos(), budget)
			}
			for _, candidate := range events.candidates {
				if !budget.complete {
					break
				}
				if ps6089LifecycleSequenceBudget(pass, groupFlow, aliases, entry, events.created, events.committed, events.waited, events.writes, candidate, budget, repeats) {
					lifecycle = true
					break
				}
			}
			if lifecycle {
				break
			}
		}
		if lifecycle {
			pass.Reportf(position.Pos(), "leaf GPU benchmark creates, commits, and waits for one command buffer per iteration around the RoPE/KV candidate; compare a production-shaped batched command, report commands-per-buffer and event counts, and do not promote from lifecycle-diluted leaf timing")
		}
		return true
	})
}

type ps6089LifecycleEvents struct {
	created     []ps6089OrderedReceiver
	committed   []ps6089OrderedReceiver
	waited      []ps6089OrderedReceiver
	candidates  []ps6089OrderedReceiver
	writes      []ps6089OrderedReceiver
	scope       *ast.BlockStmt
	invocations []ps6089LifecycleInvocation
}

type ps6089LifecycleInvocation struct {
	call  *ast.CallExpr
	body  *ast.BlockStmt
	scope *ast.BlockStmt
}

func ps6089CollectLifecycleEvents(pass *analysis.Pass, parents map[ast.Node]ast.Node, body, fallback *ast.BlockStmt, aliases *ps6089ReceiverAliasIndex, promotedPaths map[types.Type]ps6089PromotedLifecyclePath) map[*ast.BlockStmt]*ps6089LifecycleEvents {
	groups := make(map[*ast.BlockStmt]*ps6089LifecycleEvents)
	groupFor := func(node ast.Node) *ps6089LifecycleEvents {
		executionBody := ps6089ExecutionBody(parents, node, fallback)
		group := groups[executionBody]
		if group == nil {
			group = &ps6089LifecycleEvents{scope: body}
			if executionBody != fallback {
				group.scope = executionBody
				for invocationBody := executionBody; invocationBody != fallback; {
					literal, ok := parents[invocationBody].(*ast.FuncLit)
					if !ok {
						break
					}
					var expression ast.Expr = literal
					parent := parents[literal]
					for {
						switch wrapper := parent.(type) {
						case *ast.ParenExpr:
							if wrapper.X != expression {
								break
							}
							expression = wrapper
							parent = parents[wrapper]
							continue
						case *ast.CallExpr:
							if !ps6089TypeConversion(pass, wrapper) || len(wrapper.Args) != 1 || wrapper.Args[0] != expression {
								break
							}
							expression = wrapper
							parent = parents[wrapper]
							continue
						}
						break
					}
					if call, immediate := parent.(*ast.CallExpr); immediate && call.Fun == expression {
						ownerBody := ps6089ExecutionBody(parents, call, fallback)
						scope := body
						if ownerBody != fallback {
							scope = ownerBody
						}
						group.invocations = append(group.invocations, ps6089LifecycleInvocation{call: call, body: ownerBody, scope: scope})
						invocationBody = ownerBody
						continue
					}
					break
				}
			}
			groups[executionBody] = group
		}
		return group
	}
	astutil.WithStack(body, func(node ast.Node, stack []ast.Node) bool {
		if literal, ok := node.(*ast.FuncLit); ok && !ps6089SynchronousFuncLit(pass, stack, literal) {
			return false
		}
		if block, ok := node.(*ast.BlockStmt); ok {
			group := groupFor(block)
			for index := 0; index+1 < len(block.List); index++ {
				rotary, rotaryOK := ps6089RecorderCall(pass, ps6089UnlabelStatement(block.List[index]))
				cache, cacheOK := ps6089RecorderCall(pass, ps6089UnlabelStatement(block.List[index+1]))
				if rotaryOK && cacheOK && ps6089SameReceiver(&rotary, &cache) && ps6089RotaryName(rotary.name) && ps6089CacheName(cache.name) && ps6089GPUCommandType(rotary.receiverTy) && types.Identical(rotary.receiverTy, cache.receiverTy) && ps6089SharedData(&rotary, &cache) && ps6089SharedShape(&rotary, &cache) {
					group.candidates = ps6089RecordReceiver(group.candidates, ps6089CanonicalReceiverID(rotary.receiverID, rotary.call.Pos(), aliases), int(rotary.call.Pos()), rotary.call.Pos())
				}
			}
			return true
		}
		statement, ok := node.(ast.Stmt)
		if !ok {
			return true
		}
		eventStatement := ps6089UnlabelStatement(statement)
		group := groupFor(statement)
		order := int(eventStatement.Pos())
		if receiverID, created := ps6089CreatedCommand(pass, eventStatement, promotedPaths); created {
			// The result binding is the creation itself. Keep its LHS write from
			// looking like a later receiver rebind while retaining the statement's
			// CFG position for reachability.
			group.created = ps6089RecordReceiver(group.created, ps6089CanonicalReceiverID(receiverID, eventStatement.Pos(), aliases), int(eventStatement.End()), eventStatement.Pos())
		}
		call := ps6032StatementCall(eventStatement)
		if call != nil {
			callee, signature, resolved := typedCallee(pass, call.Fun)
			if resolved && signature.Recv() != nil {
				receiverID, receiverType, _, receiverOK := ps6089CallReceiver(pass, call)
				if receiverOK && ps6089GPUCommandType(receiverType) {
					name := ps6007NormalizeName(callee.Name())
					if ps6089CommitName(name) {
						group.committed = ps6089RecordReceiver(group.committed, ps6089CanonicalReceiverID(receiverID, call.Pos(), aliases), order, call.Pos())
					}
					if ps6089WaitName(name) {
						group.waited = ps6089RecordReceiver(group.waited, ps6089CanonicalReceiverID(receiverID, call.Pos(), aliases), order, call.Pos())
					}
				}
			}
		}
		return true
	})
	return groups
}

func ps6089SynchronousFuncLit(pass *analysis.Pass, stack []ast.Node, literal *ast.FuncLit) bool {
	_, index, ok := ps6089FuncLitCall(pass, stack, literal)
	if !ok || index == 0 {
		return ok
	}
	switch stack[index-1].(type) {
	case *ast.GoStmt, *ast.DeferStmt:
		return false
	}
	return true
}

func ps6089ExecutionBody(parents map[ast.Node]ast.Node, position ast.Node, fallback *ast.BlockStmt) *ast.BlockStmt {
	for current := position; current != nil; current = parents[current] {
		if literal, ok := current.(*ast.FuncLit); ok && literal.Body != nil {
			return literal.Body
		}
	}
	return fallback
}

func ps6089UnlabelStatement(statement ast.Stmt) ast.Stmt {
	for {
		labeled, ok := statement.(*ast.LabeledStmt)
		if !ok {
			return statement
		}
		statement = labeled.Stmt
	}
}

type ps6089OrderedReceiver struct {
	id    []types.Object
	order int
	pos   token.Pos
}

type ps6089PositionPair struct {
	before token.Pos
	after  token.Pos
}

type ps6089LifecycleFlow struct {
	graph         *cfg.CFG
	body          *ast.BlockStmt
	blocks        map[token.Pos]*cfg.Block
	cost          int
	postDominates map[ps6089PositionPair]bool
	postKnown     map[ps6089PositionPair]bool
	reaches       map[ps6089PositionPair]bool
	reachKnown    map[ps6089PositionPair]bool
}

type ps6089LifecycleBudget struct {
	work     int
	limit    int
	complete bool
}

func ps6089NewLifecycleBudget(flow *ps6089LifecycleFlow, events *ps6089LifecycleEvents) *ps6089LifecycleBudget {
	size := 1 + flow.cost + len(events.created) + len(events.committed) + len(events.waited) +
		len(events.candidates) + len(events.writes) + len(events.invocations)
	return &ps6089LifecycleBudget{limit: 64 * size, complete: true}
}

func (budget *ps6089LifecycleBudget) take(amount int) bool {
	if budget == nil {
		return true
	}
	if amount < 0 || budget.work > budget.limit-amount {
		budget.complete = false
		return false
	}
	budget.work += amount
	return true
}

func ps6089NewLifecycleFlow(pass *analysis.Pass, body *ast.BlockStmt) *ps6089LifecycleFlow {
	graph := cfg.New(body, func(call *ast.CallExpr) bool { return !ps6088NonreturnCall(pass, call) })
	result := &ps6089LifecycleFlow{
		graph:         graph,
		body:          body,
		blocks:        make(map[token.Pos]*cfg.Block),
		postDominates: make(map[ps6089PositionPair]bool),
		postKnown:     make(map[ps6089PositionPair]bool),
		reaches:       make(map[ps6089PositionPair]bool),
		reachKnown:    make(map[ps6089PositionPair]bool),
	}
	for _, block := range graph.Blocks {
		result.cost += len(block.Nodes) + len(block.Succs) + 1
		for _, node := range block.Nodes {
			ast.Inspect(node, func(child ast.Node) bool {
				if literal, ok := child.(*ast.FuncLit); ok && literal.Body != body {
					// Nested literal bodies own separate CFGs and lifecycle groups.
					return false
				}
				if child != nil {
					if _, exists := result.blocks[child.Pos()]; !exists {
						result.blocks[child.Pos()] = block
					}
					if _, exists := result.blocks[child.End()]; !exists {
						result.blocks[child.End()] = block
					}
				}
				return true
			})
		}
	}
	return result
}

func ps6089RecordReceiver(receivers []ps6089OrderedReceiver, id []types.Object, order int, position token.Pos) []ps6089OrderedReceiver {
	return append(receivers, ps6089OrderedReceiver{id: id, order: order, pos: position})
}

func ps6089LifecycleSequenceWork(pass *analysis.Pass, flow *ps6089LifecycleFlow, aliases *ps6089ReceiverAliasIndex, entry token.Pos, created, committed, waited, writes []ps6089OrderedReceiver, candidate ps6089OrderedReceiver) (bool, int) {
	budget := &ps6089LifecycleBudget{
		limit:    16 * (len(created) + len(committed) + len(waited) + len(writes) + 1),
		complete: true,
	}
	result := ps6089LifecycleSequenceBudget(
		pass, flow, aliases, entry, created, committed, waited, writes, candidate, budget,
		func(ps6089OrderedReceiver, ps6089OrderedReceiver) bool { return true },
	)
	return result, budget.work
}

func ps6089LifecycleSequenceBudget(pass *analysis.Pass, flow *ps6089LifecycleFlow, aliases *ps6089ReceiverAliasIndex, entry token.Pos, created, committed, waited, writes []ps6089OrderedReceiver, candidate ps6089OrderedReceiver, budget *ps6089LifecycleBudget, repeats func(ps6089OrderedReceiver, ps6089OrderedReceiver) bool) bool {
	eligibleWaits := make([]ps6089OrderedReceiver, 0, len(waited))
	for _, waitedEvent := range waited {
		for _, committedEvent := range committed {
			if !budget.take(1) {
				return false
			}
			if committedEvent.order <= candidate.order || waitedEvent.order <= committedEvent.order || !slices.Equal(committedEvent.id, candidate.id) || !slices.Equal(waitedEvent.id, candidate.id) || !ps6089PositionPostDominatesBudget(pass, flow, candidate.pos, committedEvent.pos, budget) || !ps6089PositionPostDominatesBudget(pass, flow, committedEvent.pos, waitedEvent.pos, budget) {
				continue
			}
			eligibleWaits = append(eligibleWaits, waitedEvent)
			break
		}
	}
	for _, createdEvent := range created {
		if !budget.take(1) {
			return false
		}
		if createdEvent.order >= candidate.order || !slices.Equal(createdEvent.id, candidate.id) || !ps6089PositionPostDominatesOrEqualBudget(pass, flow, entry, createdEvent.pos, budget) || !ps6089PositionPostDominatesBudget(pass, flow, createdEvent.pos, candidate.pos, budget) {
			continue
		}
		for _, waitedEvent := range eligibleWaits {
			if !budget.take(1) {
				return false
			}
			if !aliases.receiverUncertain(candidate.id, createdEvent.pos, waitedEvent.pos) &&
				!ps6089ReceiverReboundOnPathBudget(pass, flow, aliases, writes, candidate.id, createdEvent, waitedEvent, budget) &&
				repeats(createdEvent, waitedEvent) {
				return true
			}
		}
	}
	return false
}

func ps6089LifecycleEntryPosition(flow *ps6089LifecycleFlow, scope *ast.BlockStmt) token.Pos {
	if scope == nil {
		return token.NoPos
	}
	entry := token.NoPos
	for position, block := range flow.blocks {
		if block == nil || !block.Live || position <= scope.Lbrace || position >= scope.Rbrace {
			continue
		}
		if entry == token.NoPos || position < entry {
			entry = position
		}
	}
	return entry
}

func ps6089PositionPostDominatesOrEqualBudget(pass *analysis.Pass, flow *ps6089LifecycleFlow, before, after token.Pos, budget *ps6089LifecycleBudget) bool {
	return before == after || ps6089PositionPostDominatesBudget(pass, flow, before, after, budget)
}

func ps6089PositionPostDominatesBudget(pass *analysis.Pass, flow *ps6089LifecycleFlow, before, after token.Pos, budget *ps6089LifecycleBudget) bool {
	pair := ps6089PositionPair{before: before, after: after}
	if flow.postKnown[pair] {
		return budget.take(1) && flow.postDominates[pair]
	}
	result := ps6089PositionPostDominatesUncached(pass, flow, before, after, budget)
	if !budget.complete {
		return false
	}
	flow.postDominates[pair] = result
	flow.postKnown[pair] = true
	return result
}

func ps6089PositionPostDominatesUncached(pass *analysis.Pass, flow *ps6089LifecycleFlow, before, after token.Pos, budget *ps6089LifecycleBudget) bool {
	beforeBlock := flow.blocks[before]
	afterBlock := flow.blocks[after]
	if !budget.take(1) || beforeBlock == nil || afterBlock == nil || !beforeBlock.Live || !afterBlock.Live {
		return false
	}
	if beforeBlock == afterBlock {
		return before < after && ps6089CFGIntervalReturns(pass, beforeBlock, before, after, budget)
	}
	seen := map[*cfg.Block]bool{beforeBlock: true}
	adjacent := make(map[*cfg.Block][]*cfg.Block)
	indegree := make(map[*cfg.Block]int)
	reachesAfter := false
	queue := []*cfg.Block{beforeBlock}
	for len(queue) > 0 {
		if !budget.take(1) {
			return false
		}
		block := queue[0]
		queue = queue[1:]
		minimum := token.NoPos
		if block == beforeBlock {
			minimum = before
		}
		maximum := token.NoPos
		if block == afterBlock {
			maximum = after
		}
		if !ps6089CFGIntervalReturns(pass, block, minimum, maximum, budget) {
			return false
		}
		if block == afterBlock {
			reachesAfter = true
			continue
		}
		if len(block.Succs) == 0 {
			return false
		}
		for _, successor := range ps6089LifecycleSuccessors(pass, block, budget) {
			if !budget.complete {
				return false
			}
			if successor != afterBlock {
				adjacent[block] = append(adjacent[block], successor)
				indegree[successor]++
			}
			if !seen[successor] {
				seen[successor] = true
				queue = append(queue, successor)
			}
		}
	}
	if !reachesAfter {
		return false
	}
	acyclic := make([]*cfg.Block, 0, len(seen))
	for block := range seen {
		if block != afterBlock && indegree[block] == 0 {
			acyclic = append(acyclic, block)
		}
	}
	visited := 0
	for len(acyclic) > 0 {
		if !budget.take(1) {
			return false
		}
		block := acyclic[len(acyclic)-1]
		acyclic = acyclic[:len(acyclic)-1]
		visited++
		for _, successor := range adjacent[block] {
			indegree[successor]--
			if indegree[successor] == 0 {
				acyclic = append(acyclic, successor)
			}
		}
	}
	return visited == len(seen)-1
}

func ps6089CFGIntervalReturns(pass *analysis.Pass, block *cfg.Block, minimum, maximum token.Pos, budget *ps6089LifecycleBudget) bool {
	for _, node := range block.Nodes {
		if minimum.IsValid() && node.End() <= minimum || maximum.IsValid() && node.Pos() >= maximum {
			continue
		}
		if !budget.take(1) {
			return false
		}
		if pass == nil {
			continue
		}
		switch value := node.(type) {
		case ast.Stmt:
			if !ps6088StatementReturnsNormally(pass, value, false) || !ps6089NamedCallsReturnBudget(pass, value, nil, budget) {
				return false
			}
		case ast.Expr:
			if !ps6088ExpressionReturnsNormally(pass, value) || !ps6089NamedCallsReturnBudget(pass, value, nil, budget) {
				return false
			}
		}
	}
	return true
}

func ps6089NamedCallsReturn(pass *analysis.Pass, node ast.Node, visiting map[*types.Func]bool) bool {
	return ps6089NamedCallsReturnBudget(pass, node, visiting, nil)
}

func ps6089NamedCallsReturnBudget(pass *analysis.Pass, node ast.Node, visiting map[*types.Func]bool, budget *ps6089LifecycleBudget) bool {
	if pass == nil || node == nil {
		return true
	}
	if visiting == nil {
		visiting = make(map[*types.Func]bool)
	}
	returns := true
	ast.Inspect(node, func(current ast.Node) bool {
		if !returns || current == nil {
			return false
		}
		if budget != nil && !budget.take(1) {
			returns = false
			return false
		}
		call, ok := current.(*ast.CallExpr)
		if !ok || ps6089TypeConversion(pass, call) {
			return true
		}
		function, _, ok := typedCallee(pass, call.Fun)
		if ok && !ps6089NamedFunctionReturns(pass, function, visiting) {
			returns = false
			return false
		}
		return true
	})
	return returns
}

func ps6089NamedFunctionReturns(pass *analysis.Pass, function *types.Func, visiting map[*types.Func]bool) bool {
	if function == nil || function.Pkg() == nil || function.Pkg() != pass.Pkg {
		return true
	}
	raw, _ := ps6089FunctionReturns.LoadOrStore(pass, &ps6089FunctionReturnIndex{})
	index := raw.(*ps6089FunctionReturnIndex)
	index.once.Do(func() {
		index.declarations = make(map[*types.Func]*ast.FuncDecl)
		index.known = make(map[*types.Func]bool)
		index.returns = make(map[*types.Func]bool)
		for _, file := range pass.Files {
			for _, declaration := range file.Decls {
				functionDeclaration, ok := declaration.(*ast.FuncDecl)
				if !ok || functionDeclaration.Body == nil {
					continue
				}
				object, _ := pass.TypesInfo.Defs[functionDeclaration.Name].(*types.Func)
				if object != nil {
					index.declarations[object] = functionDeclaration
				}
			}
		}
	})
	index.mu.Lock()
	if index.known[function] {
		result := index.returns[function]
		index.mu.Unlock()
		return result
	}
	declaration := index.declarations[function]
	index.mu.Unlock()
	if declaration == nil || visiting[function] {
		return declaration == nil
	}
	visiting[function] = true
	defer delete(visiting, function)
	graph := cfg.New(declaration.Body, func(call *ast.CallExpr) bool { return !ps6088NonreturnCall(pass, call) })
	state := make(map[*cfg.Block]uint8)
	memo := make(map[*cfg.Block]bool)
	var blockReturns func(*cfg.Block) bool
	blockReturns = func(block *cfg.Block) bool {
		if state[block] == 1 {
			// A feasible cycle is a possible non-returning execution.
			return false
		}
		if state[block] == 2 {
			return memo[block]
		}
		state[block] = 1
		result := true
		for _, child := range block.Nodes {
			switch value := child.(type) {
			case ast.Stmt:
				result = ps6088StatementReturnsNormally(pass, value, false)
			case ast.Expr:
				result = ps6088ExpressionReturnsNormally(pass, value)
			}
			if !result || !ps6089NamedCallsReturn(pass, child, visiting) {
				result = false
				break
			}
		}
		if result && block.Return() == nil {
			successors := ps6088BlockSuccessors(pass, block)
			result = len(successors) > 0
			for _, successor := range successors {
				if !blockReturns(successor) {
					result = false
					break
				}
			}
		}
		state[block] = 2
		memo[block] = result
		return result
	}
	result := len(graph.Blocks) > 0 && blockReturns(graph.Blocks[0])
	index.mu.Lock()
	index.known[function] = true
	index.returns[function] = result
	index.mu.Unlock()
	return result
}

func ps6089LifecycleSuccessors(pass *analysis.Pass, block *cfg.Block, budget *ps6089LifecycleBudget) []*cfg.Block {
	if !budget.take(len(block.Succs) + 1) {
		return nil
	}
	if pass == nil {
		return block.Succs
	}
	if len(block.Succs) == 2 {
		if condition, known := ps6088CFGCondition(pass, block); known {
			if condition {
				return block.Succs[:1]
			}
			return block.Succs[1:]
		}
	}
	return block.Succs
}

func ps6089ReceiverReboundOnPathBudget(pass *analysis.Pass, flow *ps6089LifecycleFlow, aliases *ps6089ReceiverAliasIndex, writes []ps6089OrderedReceiver, receiverID []types.Object, created, waited ps6089OrderedReceiver, budget *ps6089LifecycleBudget) bool {
	createdBlock := flow.blocks[created.pos]
	waitedBlock := flow.blocks[waited.pos]
	for _, write := range writes {
		if !budget.take(1) {
			return true
		}
		if len(write.id) > len(receiverID) || !slices.Equal(write.id, receiverID[:len(write.id)]) {
			continue
		}
		writeBlock := ps6089LifecycleBlockAt(flow, write.pos)
		if createdBlock == writeBlock && write.order <= created.order || writeBlock == waitedBlock && write.order > waited.order {
			continue
		}
		writeToWait, writeToWaitComplete := aliases.positionCanReachAvoiding(write.pos, waited.pos, created.pos)
		if !writeToWaitComplete {
			return true
		}
		if ps6089PositionCanReachBudget(pass, flow, created.pos, write.pos, budget) && writeToWait {
			return true
		}
	}
	return false
}

func ps6089LifecycleBlockAt(flow *ps6089LifecycleFlow, position token.Pos) *cfg.Block {
	if flow == nil {
		return nil
	}
	return flow.blocks[position]
}

func ps6089PositionCanReachBudget(pass *analysis.Pass, flow *ps6089LifecycleFlow, before, after token.Pos, budget *ps6089LifecycleBudget) bool {
	pair := ps6089PositionPair{before: before, after: after}
	if flow.reachKnown[pair] {
		return budget.take(1) && flow.reaches[pair]
	}
	result := ps6089PositionCanReachUncachedBudget(pass, flow, before, after, budget, false)
	if !budget.complete {
		return false
	}
	flow.reaches[pair] = result
	flow.reachKnown[pair] = true
	return result
}

func ps6089PositionCanReachRecurringBudget(pass *analysis.Pass, flow *ps6089LifecycleFlow, before, after token.Pos, budget *ps6089LifecycleBudget) bool {
	return ps6089PositionCanReachUncachedBudget(pass, flow, before, after, budget, true)
}

func ps6089PositionCanReachUncachedBudget(pass *analysis.Pass, flow *ps6089LifecycleFlow, before, after token.Pos, budget *ps6089LifecycleBudget, recurring bool) bool {
	beforeBlock := flow.blocks[before]
	afterBlock := flow.blocks[after]
	if !budget.take(1) || beforeBlock == nil || afterBlock == nil || !beforeBlock.Live || !afterBlock.Live {
		return false
	}
	if beforeBlock == afterBlock && before < after && !recurring {
		return ps6089CFGIntervalReturns(pass, beforeBlock, before, after, budget)
	}
	if recurring && !ps6089CFGIntervalReturns(pass, beforeBlock, before, token.NoPos, budget) {
		return false
	}
	seen := map[*cfg.Block]bool{beforeBlock: true}
	queue := ps6089LifecycleSuccessors(pass, beforeBlock, budget)
	for len(queue) > 0 {
		if !budget.take(1) {
			return false
		}
		block := queue[0]
		queue = queue[1:]
		if block == afterBlock {
			return ps6089CFGIntervalReturns(pass, block, token.NoPos, after, budget)
		}
		if !ps6089CFGIntervalReturns(pass, block, token.NoPos, token.NoPos, budget) {
			continue
		}
		for _, successor := range ps6089LifecycleSuccessors(pass, block, budget) {
			if successor == afterBlock {
				return ps6089CFGIntervalReturns(pass, successor, token.NoPos, after, budget)
			}
			if !seen[successor] {
				seen[successor] = true
				queue = append(queue, successor)
			}
		}
	}
	return false
}

func ps6089Parents(root ast.Node) map[ast.Node]ast.Node {
	parents := make(map[ast.Node]ast.Node)
	astutil.WithStack(root, func(node ast.Node, stack []ast.Node) bool {
		if len(stack) > 0 {
			parents[node] = stack[len(stack)-1]
		}
		return true
	})
	return parents
}

func ps6089BenchmarkSubbenchmark(pass *analysis.Pass, parents map[ast.Node]ast.Node, literal *ast.FuncLit) bool {
	var expression ast.Expr = literal
	parent := parents[literal]
	for {
		paren, ok := parent.(*ast.ParenExpr)
		if !ok {
			break
		}
		expression = paren
		parent = parents[paren]
	}
	call, ok := parent.(*ast.CallExpr)
	if !ok || !slices.Contains(call.Args, expression) {
		return false
	}
	callee, signature, ok := typedCallee(pass, call.Fun)
	if !ok || callee.Name() != "Run" || signature.Recv() == nil {
		return false
	}
	selector, ok := ps2110Unparen(call.Fun).(*ast.SelectorExpr)
	return ok && ps6089TestingBType(pass.TypesInfo.TypeOf(selector.X))
}

type ps6089ReceiverWrite struct {
	id         []types.Object
	pos        token.Pos
	deref      int
	persistent bool
}

type ps6089ReceiverAlias struct {
	id         []types.Object
	definition token.Pos
	address    bool
}

type ps6089ReceiverAliasIndex struct {
	aliases   map[types.Object]ps6089ReceiverAlias
	flow      *ps6089LifecycleFlow
	writes    *ps6089ReceiverWritePath
	work      int
	limit     int
	buildWork int
	complete  bool
	uncertain []ps6089ReceiverWrite
	values    *ps6089ReceiverValueIndex
}

type ps6089ReceiverWritePath struct {
	children map[types.Object]*ps6089ReceiverWritePath
	writes   []ps6089ReceiverWrite
	effects  map[ps6089ReceiverEffect]bool
}

type ps6089ReceiverEffect struct {
	deref      int
	persistent bool
}

type ps6089CallableSource struct {
	literal  *ast.FuncLit
	receiver ast.Expr
	alias    types.Object
	function *types.Func
	known    bool
}

type ps6089CallableDefinition struct {
	source   ps6089CallableSource
	count    int
	effects  []ps6089ReceiverWrite
	ready    bool
	active   bool
	complete bool
}

type ps6089ReceiverValueDefinition struct {
	expression ast.Expr
	count      int
}

type ps6089ReceiverValueSummary struct {
	writes   []ps6089ReceiverWrite
	complete bool
}

type ps6089ReceiverValuePath struct {
	definition *ps6089ReceiverValueDefinition
	children   map[types.Object]*ps6089ReceiverValuePath
}

type ps6089ReceiverValueIndex struct {
	pass           *analysis.Pass
	root           *ps6089ReceiverValuePath
	take           func(int) bool
	composites     map[*ast.CompositeLit]map[types.Object]ast.Expr
	methodKnown    map[types.Type]bool
	methodMutable  map[types.Type]bool
	escapeCache    map[*ps6089ReceiverValuePath]ps6089ReceiverValueSummary
	callable       func(ast.Expr) ([]ps6089ReceiverWrite, bool)
	callableResult func(*ast.CallExpr) (ast.Expr, bool)
	functions      map[*types.Func]*ast.FuncDecl
	functionKnown  map[*types.Func]bool
	functionPure   map[*types.Func]bool
}

func ps6089CallableExpression(pass *analysis.Pass, values *ps6089ReceiverValueIndex, expression ast.Expr) ps6089CallableSource {
	expression = ps2110Unparen(expression)
	if conversion, ok := expression.(*ast.CallExpr); ok && ps6089TypeConversion(pass, conversion) && len(conversion.Args) == 1 {
		return ps6089CallableExpression(pass, values, conversion.Args[0])
	}
	if literal, ok := expression.(*ast.FuncLit); ok {
		return ps6089CallableSource{literal: literal, known: true}
	}
	if selector, ok := expression.(*ast.SelectorExpr); ok {
		selection := pass.TypesInfo.Selections[selector]
		if selection != nil && selection.Kind() == types.MethodVal {
			function, _ := selection.Obj().(*types.Func)
			if !values.methodReceiverMayMutate(selection) {
				return ps6089CallableSource{function: function, known: true}
			}
			return ps6089CallableSource{receiver: selector.X, function: function, known: true}
		}
		if selection != nil && selection.Kind() == types.FieldVal {
			if exact := values.expression(ps6089StableReceiverID(pass, selector)); exact != nil && exact != expression {
				return ps6089CallableExpression(pass, values, exact)
			}
		}
	}
	if identifier, ok := expression.(*ast.Ident); ok {
		object := identObject(pass, identifier)
		if function, ok := object.(*types.Func); ok {
			return ps6089CallableSource{function: function, known: true}
		}
		if _, variable := object.(*types.Var); variable {
			if _, signature := types.Unalias(object.Type()).Underlying().(*types.Signature); signature {
				return ps6089CallableSource{alias: object, known: true}
			}
		}
	}
	return ps6089CallableSource{}
}

func ps6089ImmediateLiteralResult(pass *analysis.Pass, expression ast.Expr) (ast.Expr, bool, bool) {
	expression = ps2110Unparen(expression)
	call, ok := expression.(*ast.CallExpr)
	if !ok {
		return nil, false, false
	}
	values := &ps6089ReceiverValueIndex{pass: pass, root: &ps6089ReceiverValuePath{}, take: func(int) bool { return true }}
	source := ps6089CallableExpression(pass, values, call.Fun)
	if source.literal == nil {
		return nil, false, false
	}
	result, complete := ps6089FuncLitResult(pass, source.literal, call)
	return result, true, complete
}

func ps6089FuncLitResult(pass *analysis.Pass, literal *ast.FuncLit, call *ast.CallExpr) (ast.Expr, bool) {
	return ps6089CallableBodyResult(pass, literal.Type, literal.Body, call)
}

func ps6089CallableBodyResult(pass *analysis.Pass, functionType *ast.FuncType, body *ast.BlockStmt, call *ast.CallExpr) (ast.Expr, bool) {
	if body == nil || len(body.List) != 1 {
		return nil, false
	}
	returned, ok := body.List[0].(*ast.ReturnStmt)
	if !ok || len(returned.Results) != 1 {
		return nil, false
	}
	result := returned.Results[0]
	path, _ := ps6089ReceiverAliasSourceID(pass, result)
	if len(path) == 0 {
		return nil, false
	}
	parameterIndex := -1
	argumentIndex := 0
	if functionType != nil && functionType.Params != nil {
		for _, field := range functionType.Params.List {
			for _, name := range field.Names {
				if identObject(pass, name) == path[0] {
					parameterIndex = argumentIndex
				}
				argumentIndex++
			}
		}
	}
	if parameterIndex >= 0 {
		if len(path) != 1 || parameterIndex >= len(call.Args) || len(call.Args) != argumentIndex {
			return nil, false
		}
		return call.Args[parameterIndex], true
	}
	return result, true
}

func ps6089EscapingReceiverWrites(values *ps6089ReceiverValueIndex, expression ast.Expr, position token.Pos, persistent bool) []ps6089ReceiverWrite {
	direct := *values
	direct.root = &ps6089ReceiverValuePath{}
	direct.escapeCache = nil
	direct.callable = nil
	direct.callableResult = nil
	writes, _ := direct.escaping(expression, position, persistent, nil)
	values.methodKnown = direct.methodKnown
	values.methodMutable = direct.methodMutable
	return writes
}

func (index *ps6089ReceiverValueIndex) record(path []types.Object, expression ast.Expr) {
	if index == nil || len(path) == 0 {
		return
	}
	node := index.root
	for _, object := range path {
		if node.children == nil {
			node.children = make(map[types.Object]*ps6089ReceiverValuePath)
		}
		child := node.children[object]
		if child == nil {
			child = &ps6089ReceiverValuePath{}
			node.children[object] = child
		}
		node = child
	}
	if node.definition == nil {
		node.definition = &ps6089ReceiverValueDefinition{}
	}
	node.definition.count++
	node.definition.expression = expression
}

func (index *ps6089ReceiverValueIndex) lookup(path []types.Object) *ps6089ReceiverValuePath {
	if index == nil || len(path) == 0 {
		return nil
	}
	node := index.root
	for _, object := range path {
		if !index.take(1) || node == nil {
			return nil
		}
		node = node.children[object]
	}
	return node
}

func (index *ps6089ReceiverValueIndex) expression(path []types.Object) ast.Expr {
	if index == nil || len(path) == 0 {
		return nil
	}
	node := index.root
	var definition *ps6089ReceiverValueDefinition
	matched := 0
	for pathIndex, object := range path {
		if !index.take(1) || node == nil {
			return nil
		}
		node = node.children[object]
		if node == nil {
			break
		}
		if node.definition != nil {
			definition = node.definition
			matched = pathIndex + 1
		}
	}
	if definition == nil || definition.count != 1 || definition.expression == nil {
		return nil
	}
	if matched == len(path) {
		return definition.expression
	}
	return index.compositeReceiverValue(definition.expression, path[matched:])
}

func (index *ps6089ReceiverValueIndex) exactCompositeValue(expression ast.Expr) bool {
	for {
		expression = ps2110Unparen(expression)
		if assertion, ok := expression.(*ast.TypeAssertExpr); ok && assertion.Type != nil {
			expression = assertion.X
			continue
		}
		conversion, ok := expression.(*ast.CallExpr)
		if !ok || !ps6089TypeConversion(index.pass, conversion) || len(conversion.Args) != 1 {
			break
		}
		expression = conversion.Args[0]
	}
	if ps6089CompositeReceiverExpression(index.pass, expression) {
		return true
	}
	if _, direct := expression.(*ast.Ident); !direct {
		return false
	}
	path := ps6089StableReceiverID(index.pass, expression)
	return len(path) > 0 && ps6089CompositeReceiverExpression(index.pass, index.expression(path))
}

func (index *ps6089ReceiverValueIndex) compositeReceiverValue(expression ast.Expr, fields []types.Object) ast.Expr {
	if len(fields) == 0 {
		return ps2110Unparen(expression)
	}
	for {
		expression = ps2110Unparen(expression)
		if address, ok := expression.(*ast.UnaryExpr); ok && address.Op == token.AND {
			expression = address.X
			continue
		}
		if conversion, ok := expression.(*ast.CallExpr); ok && ps6089TypeConversion(index.pass, conversion) && len(conversion.Args) == 1 {
			expression = conversion.Args[0]
			continue
		}
		break
	}
	literal, ok := expression.(*ast.CompositeLit)
	if !ok {
		return nil
	}
	typeOf := ps6089DereferenceType(index.pass.TypesInfo.TypeOf(literal))
	if typeOf == nil {
		return nil
	}
	structure, ok := typeOf.Underlying().(*types.Struct)
	if !ok {
		return nil
	}
	if index.composites == nil {
		index.composites = make(map[*ast.CompositeLit]map[types.Object]ast.Expr)
	}
	fieldValues := index.composites[literal]
	if fieldValues == nil {
		if !index.take(structure.NumFields() + len(literal.Elts) + 1) {
			return nil
		}
		fieldValues = make(map[types.Object]ast.Expr)
		keyed := false
		for _, element := range literal.Elts {
			if _, ok := element.(*ast.KeyValueExpr); ok {
				keyed = true
				break
			}
		}
		if keyed {
			for _, element := range literal.Elts {
				pair, ok := element.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				identifier, ok := ps2110Unparen(pair.Key).(*ast.Ident)
				if ok {
					if object := identObject(index.pass, identifier); object != nil {
						fieldValues[object] = pair.Value
					}
				}
			}
		} else {
			for fieldIndex, element := range literal.Elts {
				if fieldIndex >= structure.NumFields() {
					break
				}
				fieldValues[structure.Field(fieldIndex)] = element
			}
		}
		index.composites[literal] = fieldValues
	}
	value := fieldValues[fields[0]]
	if value == nil {
		return nil
	}
	return index.compositeReceiverValue(value, fields[1:])
}

func ps6089CompositeReceiverExpression(pass *analysis.Pass, expression ast.Expr) bool {
	for {
		expression = ps2110Unparen(expression)
		if address, ok := expression.(*ast.UnaryExpr); ok && address.Op == token.AND {
			expression = address.X
			continue
		}
		if conversion, ok := expression.(*ast.CallExpr); ok && ps6089TypeConversion(pass, conversion) && len(conversion.Args) == 1 {
			expression = conversion.Args[0]
			continue
		}
		_, ok := expression.(*ast.CompositeLit)
		return ok
	}
}

func (index *ps6089ReceiverValueIndex) resolveWrite(path []types.Object, deref int, local *ast.FuncLit) (ps6089ReceiverWrite, bool) {
	result := ps6089ReceiverWrite{id: slices.Clone(path), deref: deref}
	seen := make(map[types.Object]bool)
	for len(result.id) > 0 {
		if !index.take(len(result.id)) {
			return result, false
		}
		node := index.root
		var matched *ps6089ReceiverValueDefinition
		matchedLength := 0
		for pathIndex, object := range result.id {
			if node == nil {
				break
			}
			node = node.children[object]
			if node == nil {
				break
			}
			if node.definition != nil {
				matched = node.definition
				matchedLength = pathIndex + 1
			}
		}
		if matched == nil {
			break
		}
		if matched.count != 1 || matched.expression == nil {
			return result, false
		}
		root := result.id[0]
		localRoot := local != nil && root.Pos() > local.Pos() && root.Pos() < local.End()
		if !localRoot && !ps6089CompositeReceiverExpression(index.pass, matched.expression) {
			break
		}
		if seen[root] {
			return result, false
		}
		seen[root] = true
		expression := matched.expression
		if matchedLength == 1 && len(result.id) > 1 {
			if fieldValue := index.compositeReceiverValue(expression, result.id[1:]); fieldValue != nil {
				expression = fieldValue
				matchedLength = len(result.id)
			}
		}
		source, address := ps6089ReceiverAliasSourceID(index.pass, expression)
		if len(source) == 0 {
			break
		}
		if !index.take(len(source) + len(result.id) - matchedLength) {
			return result, false
		}
		result.id = append(slices.Clone(source), result.id[matchedLength:]...)
		if address && result.deref > 0 {
			result.deref--
		}
	}
	return result, true
}

func (index *ps6089ReceiverValueIndex) escaping(expression ast.Expr, position token.Pos, persistent bool, local *ast.FuncLit) ([]ps6089ReceiverWrite, bool) {
	cacheable := position == token.NoPos && !persistent && local == nil
	var cacheKey *ps6089ReceiverValuePath
	if cacheable {
		cacheKey = index.lookup(ps6089StableReceiverID(index.pass, expression))
	}
	if cacheKey != nil && index.escapeCache != nil {
		if summary, ok := index.escapeCache[cacheKey]; ok {
			if !index.take(len(summary.writes) + 1) {
				return nil, false
			}
			return slices.Clone(summary.writes), summary.complete
		}
	}
	var result []ps6089ReceiverWrite
	complete := true
	active := make(map[*ps6089ReceiverValuePath]bool)
	var collect func(ast.Expr)
	var collectPath func([]types.Object, types.Type)
	var collectNode func(*ps6089ReceiverValuePath)
	collectNode = func(node *ps6089ReceiverValuePath) {
		if node == nil {
			return
		}
		if running := active[node]; running {
			complete = false
			return
		}
		if !index.take(1) {
			complete = false
			return
		}
		active[node] = true
		defer delete(active, node)
		if definition := node.definition; definition != nil {
			if definition.count != 1 || definition.expression == nil {
				complete = false
			} else {
				collect(definition.expression)
			}
		}
		for _, child := range node.children {
			collectNode(child)
		}
	}
	collectPath = func(path []types.Object, typeOf types.Type) {
		if len(path) == 0 {
			return
		}
		if ps6089ReceiverStorageIndirect(typeOf) {
			write, resolved := index.resolveWrite(path, 1, local)
			if !resolved {
				complete = false
				return
			}
			write.pos = position
			write.persistent = persistent
			result = append(result, write)
			if expression := index.expression(path); expression != nil {
				collect(expression)
			}
			if node := index.lookup(path); node != nil && len(node.children) > 0 {
				collectNode(node)
			}
			return
		}
		expression := index.expression(path)
		if ps6089CompositeReceiverExpression(index.pass, expression) {
			collect(expression)
			return
		}
		if index.methodValueMayMutate(typeOf, make(map[types.Type]bool)) {
			// A known child does not make an opaque aggregate root complete: any
			// unresolved sibling may still carry the lifecycle address.
			complete = false
		}
		node := index.lookup(path)
		if node != nil && len(node.children) > 0 {
			collectNode(node)
		}
	}
	collect = func(current ast.Expr) {
		if current == nil || !index.take(1) {
			complete = false
			return
		}
		current = ps2110Unparen(current)
		if ps6089CallableType(index.pass.TypesInfo.TypeOf(current)) && index.callable != nil {
			effects, effectsComplete := index.callable(current)
			if !effectsComplete {
				complete = false
				return
			}
			for _, effect := range effects {
				if !index.take(1) {
					complete = false
					break
				}
				effect.pos = position
				effect.persistent = effect.persistent || persistent
				result = append(result, effect)
			}
			return
		}
		if conversion, ok := current.(*ast.CallExpr); ok {
			if ps6089TypeConversion(index.pass, conversion) && len(conversion.Args) == 1 {
				collect(conversion.Args[0])
				return
			}
			if index.callableResult != nil {
				if result, exact := index.callableResult(conversion); exact {
					collect(result)
					return
				}
			}
			// Ordinary nested calls are visited once by the surrounding AST
			// effect walk. Re-walking their argument trees here is quadratic.
			if index.methodValueMayMutate(index.pass.TypesInfo.TypeOf(conversion), make(map[types.Type]bool)) {
				complete = false
			}
			return
		}
		if address, ok := current.(*ast.UnaryExpr); ok && address.Op == token.AND {
			if path := ps6089StableReceiverID(index.pass, address.X); len(path) > 0 {
				if expression := index.expression(path); ps6089CompositeReceiverExpression(index.pass, expression) {
					collect(expression)
					return
				}
				write, resolved := index.resolveWrite(path, 0, local)
				if !resolved {
					complete = false
					return
				}
				write.pos = position
				write.persistent = persistent
				result = append(result, write)
				return
			}
			collect(address.X)
			return
		}
		if path := ps6089StableReceiverID(index.pass, current); len(path) > 0 {
			collectPath(path, index.pass.TypesInfo.TypeOf(current))
			return
		}
		switch value := current.(type) {
		case *ast.CompositeLit:
			literalType := ps6089DereferenceType(index.pass.TypesInfo.TypeOf(value))
			mapLiteral := false
			if literalType != nil {
				_, mapLiteral = literalType.Underlying().(*types.Map)
			}
			for _, element := range value.Elts {
				if !complete {
					break
				}
				switch element := element.(type) {
				case *ast.KeyValueExpr:
					if mapLiteral {
						collect(element.Key)
					}
					collect(element.Value)
				case ast.Expr:
					collect(element)
				}
			}
		case *ast.IndexExpr:
			collect(value.X)
		case *ast.IndexListExpr:
			collect(value.X)
		}
	}
	collect(expression)
	if cacheKey != nil {
		deduplicated, dedupComplete := ps6089DeduplicateReceiverEffects(result, index.take)
		result = deduplicated
		complete = complete && dedupComplete
		if index.escapeCache == nil {
			index.escapeCache = make(map[*ps6089ReceiverValuePath]ps6089ReceiverValueSummary)
		}
		index.escapeCache[cacheKey] = ps6089ReceiverValueSummary{writes: slices.Clone(result), complete: complete}
	}
	return result, complete
}

func ps6089DeduplicateReceiverEffects(writes []ps6089ReceiverWrite, take func(int) bool) ([]ps6089ReceiverWrite, bool) {
	root := &ps6089ReceiverWritePath{}
	result := make([]ps6089ReceiverWrite, 0, len(writes))
	for _, write := range writes {
		node := root
		for _, object := range write.id {
			if !take(1) {
				return result, false
			}
			if node.children == nil {
				node.children = make(map[types.Object]*ps6089ReceiverWritePath)
			}
			child := node.children[object]
			if child == nil {
				child = &ps6089ReceiverWritePath{}
				node.children[object] = child
			}
			node = child
		}
		if node.effects == nil {
			node.effects = make(map[ps6089ReceiverEffect]bool)
		}
		key := ps6089ReceiverEffect{deref: write.deref, persistent: write.persistent}
		if !node.effects[key] {
			node.effects[key] = true
			result = append(result, write)
		}
	}
	return result, true
}

func ps6089ReceiverAliases(pass *analysis.Pass, body *ast.BlockStmt) *ps6089ReceiverAliasIndex {
	type definition struct {
		id       []types.Object
		position token.Pos
		address  bool
		invalid  bool
	}
	counts := make(map[types.Object]int)
	definitions := make(map[types.Object]definition)
	var sourceWrites []ps6089ReceiverWrite
	var uncertainWrites []ps6089ReceiverWrite
	callables := make(map[types.Object]*ps6089CallableDefinition)
	values := &ps6089ReceiverValueIndex{pass: pass, root: &ps6089ReceiverValuePath{}, take: func(int) bool { return true }}
	values.functions = make(map[*types.Func]*ast.FuncDecl)
	values.functionKnown = make(map[*types.Func]bool)
	values.functionPure = make(map[*types.Func]bool)
	bodyObjects := make(map[types.Object]bool)
	ast.Inspect(body, func(node ast.Node) bool {
		if identifier, ok := node.(*ast.Ident); ok {
			if object := identObject(pass, identifier); object != nil {
				bodyObjects[object] = true
			}
		}
		return true
	})
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			object, _ := pass.TypesInfo.Defs[function.Name].(*types.Func)
			if object != nil {
				values.functions[object] = function
			}
		}
	}
	nodeCount := 0
	recordCallable := func(left ast.Expr, right ast.Expr) {
		identifier, ok := ps2110Unparen(left).(*ast.Ident)
		if !ok || right == nil {
			return
		}
		object := identObject(pass, identifier)
		if object == nil {
			return
		}
		if !ps6089CallableType(object.Type()) {
			return
		}
		definition := callables[object]
		if definition == nil {
			definition = &ps6089CallableDefinition{}
			callables[object] = definition
		}
		definition.count++
		definition.source = ps6089CallableExpression(pass, values, right)
	}
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok || general.Tok != token.VAR {
				continue
			}
			for _, specification := range general.Specs {
				value, ok := specification.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for index, name := range value.Names {
					if index < len(value.Values) && bodyObjects[identObject(pass, name)] {
						recordCallable(name, value.Values[index])
					}
				}
			}
		}
	}
	ast.Inspect(body, func(node ast.Node) bool {
		if node == nil {
			return true
		}
		nodeCount++
		switch value := node.(type) {
		case *ast.AssignStmt:
			if len(value.Lhs) == len(value.Rhs) {
				for index := range value.Lhs {
					recordCallable(value.Lhs[index], value.Rhs[index])
				}
			} else {
				for _, left := range value.Lhs {
					if identifier, ok := ps2110Unparen(left).(*ast.Ident); ok {
						object := identObject(pass, identifier)
						if object != nil {
							if ps6089CallableType(object.Type()) {
								definition := callables[object]
								if definition == nil {
									definition = &ps6089CallableDefinition{}
									callables[object] = definition
								}
								definition.count++
							}
						}
					}
				}
			}
		case *ast.ValueSpec:
			for index, name := range value.Names {
				if index < len(value.Values) {
					recordCallable(name, value.Values[index])
				}
			}
		}
		return true
	})
	collectionWork := 0
	collectionLimit := 64 * (nodeCount + 1)
	collectionComplete := true
	takeCollection := func(amount int) bool {
		if amount < 0 || collectionWork > collectionLimit-amount {
			collectionComplete = false
			return false
		}
		collectionWork += amount
		return true
	}
	values.take = takeCollection
	appendUncertain := func(writes ...ps6089ReceiverWrite) bool {
		if !takeCollection(len(writes)) {
			return false
		}
		uncertainWrites = append(uncertainWrites, writes...)
		return true
	}
	literalSummaries := make(map[*ast.FuncLit]*ps6089CallableDefinition)
	appliedCallables := make(map[types.Object][]int)
	var callableEffects func(types.Object) ([]ps6089ReceiverWrite, bool)
	var expressionEffects func(ast.Expr) ([]ps6089ReceiverWrite, bool)
	var expressionEscapeEffects func(ast.Expr) ([]ps6089ReceiverWrite, bool)
	var literalEffects func(*ast.FuncLit) ([]ps6089ReceiverWrite, bool)
	var sourceEffects func(ps6089CallableSource) ([]ps6089ReceiverWrite, bool)
	var callableResult func(*ast.CallExpr) (ast.Expr, bool)
	appendEscaping := func(effects *[]ps6089ReceiverWrite, expression ast.Expr, persistent bool, local *ast.FuncLit) bool {
		if ps6089CommandFactoryExpression(pass, expression) {
			return true
		}
		direct := ps6089EscapingReceiverWrites(values, expression, token.NoPos, persistent)
		writes, complete := values.escaping(expression, token.NoPos, persistent, local)
		if len(writes) == 0 && len(direct) > 0 {
			*effects = append(*effects, direct...)
			return complete
		}
		if call, ok := ps2110Unparen(expression).(*ast.CallExpr); ok && !ps6089TypeConversion(pass, call) && len(writes) == 0 {
			// The literal AST walk visits the ordinary nested call and each of its
			// arguments separately. Do not turn the outer result wrapper into an
			// opaque escape before those exact argument effects are collected.
			return complete
		}
		*effects = append(*effects, direct...)
		*effects = append(*effects, writes...)
		if complete && len(writes) == 0 &&
			values.methodValueMayMutate(pass.TypesInfo.TypeOf(expression), make(map[types.Type]bool)) &&
			!values.exactCompositeValue(expression) {
			return false
		}
		return complete
	}
	literalEffects = func(literal *ast.FuncLit) ([]ps6089ReceiverWrite, bool) {
		summary := literalSummaries[literal]
		if summary == nil {
			summary = &ps6089CallableDefinition{}
			literalSummaries[literal] = summary
		}
		if summary.ready {
			return summary.effects, summary.complete
		}
		if summary.active {
			return nil, false
		}
		summary.active = true
		var effects []ps6089ReceiverWrite
		complete := true
		ast.Inspect(literal.Body, func(node ast.Node) bool {
			if node == nil {
				return true
			}
			if !takeCollection(1) {
				return false
			}
			if nested, ok := node.(*ast.FuncLit); ok && nested != literal {
				return false
			}
			appendWrite := func(expression ast.Expr) {
				path, deref := ps6089ReceiverWriteID(pass, expression)
				if len(path) == 0 || deref == 0 && !ps6089ReceiverStorageIndirect(pass.TypesInfo.TypeOf(expression)) {
					return
				}
				rootLocal := path[0].Pos() > literal.Pos() && path[0].Pos() < literal.End()
				if deref == 0 && rootLocal && (len(path) == 1 || !ps6089ReceiverStorageIndirect(path[0].Type())) {
					return
				}
				write, resolved := values.resolveWrite(path, deref, literal)
				if !resolved {
					complete = false
					return
				}
				effects = append(effects, write)
			}
			switch value := node.(type) {
			case *ast.AssignStmt:
				for index, left := range value.Lhs {
					appendWrite(left)
					if index < len(value.Rhs) && ps6089PackageVariable(pass, left) {
						complete = appendEscaping(&effects, value.Rhs[index], true, literal) && complete
					}
				}
			case *ast.IncDecStmt:
				appendWrite(value.X)
			case *ast.RangeStmt:
				for _, target := range []ast.Expr{value.Key, value.Value} {
					if target != nil {
						appendWrite(target)
					}
				}
			case *ast.SendStmt:
				complete = appendEscaping(&effects, value.Value, true, literal) && complete
			case *ast.CallExpr:
				if ps6089TypeConversion(pass, value) {
					return true
				}
				knownLifecycle := ps6089KnownLifecycleCall(pass, value)
				if !knownLifecycle {
					invoked, invocationComplete := expressionEffects(value.Fun)
					effects = append(effects, invoked...)
					complete = invocationComplete && complete
				}
				for _, argument := range value.Args {
					if ps6089CallableType(pass.TypesInfo.TypeOf(argument)) {
						invoked, invocationComplete := expressionEffects(argument)
						effects = append(effects, invoked...)
						complete = invocationComplete && complete
					}
					complete = appendEscaping(&effects, argument, true, literal) && complete
				}
				if selector, ok := ps2110Unparen(value.Fun).(*ast.SelectorExpr); ok && !knownLifecycle {
					selection := pass.TypesInfo.Selections[selector]
					if selection != nil && selection.Kind() == types.MethodVal && values.methodReceiverMayMutate(selection) {
						complete = appendEscaping(&effects, selector.X, false, literal) && complete
					}
				}
				return false
			}
			return true
		})
		deduplicated, dedupComplete := ps6089DeduplicateReceiverEffects(effects, takeCollection)
		summary.effects = deduplicated
		summary.complete = complete && dedupComplete
		summary.active = false
		summary.ready = true
		return summary.effects, summary.complete
	}
	callableEffects = func(object types.Object) ([]ps6089ReceiverWrite, bool) {
		definition := callables[object]
		if definition == nil || definition.count != 1 {
			return nil, false
		}
		if definition.ready {
			return definition.effects, definition.complete
		}
		if definition.active {
			return nil, false
		}
		definition.active = true
		definition.effects, definition.complete = sourceEffects(definition.source)
		if definition.complete {
			definition.effects, definition.complete = ps6089DeduplicateReceiverEffects(definition.effects, takeCollection)
		}
		definition.active = false
		definition.ready = true
		return definition.effects, definition.complete
	}
	callableResult = func(call *ast.CallExpr) (ast.Expr, bool) {
		source := ps6089CallableExpression(pass, values, call.Fun)
		seen := make(map[types.Object]bool)
		for source.alias != nil {
			if seen[source.alias] {
				return nil, false
			}
			seen[source.alias] = true
			definition := callables[source.alias]
			if definition == nil || definition.count != 1 || !definition.source.known {
				return nil, false
			}
			source = definition.source
		}
		if source.literal != nil {
			return ps6089FuncLitResult(pass, source.literal, call)
		}
		if declaration := values.functions[source.function]; declaration != nil {
			return ps6089CallableBodyResult(pass, declaration.Type, declaration.Body, call)
		}
		return nil, false
	}
	callableSourceResult := func(source ps6089CallableSource) (ast.Expr, bool, bool) {
		seen := make(map[types.Object]bool)
		for source.alias != nil {
			if seen[source.alias] {
				return nil, false, true
			}
			seen[source.alias] = true
			definition := callables[source.alias]
			if definition == nil || definition.count != 1 || !definition.source.known {
				return nil, false, true
			}
			source = definition.source
		}
		var functionType *ast.FuncType
		var body *ast.BlockStmt
		switch {
		case source.literal != nil:
			functionType, body = source.literal.Type, source.literal.Body
		case values.functions[source.function] != nil:
			declaration := values.functions[source.function]
			functionType, body = declaration.Type, declaration.Body
		default:
			return nil, false, false
		}
		if functionType != nil && functionType.Params != nil && len(functionType.Params.List) > 0 {
			return nil, false, true
		}
		if body == nil || len(body.List) != 1 {
			return nil, false, true
		}
		returned, ok := body.List[0].(*ast.ReturnStmt)
		if !ok {
			return nil, false, false
		}
		if len(returned.Results) != 1 {
			return nil, false, len(returned.Results) != 0
		}
		return returned.Results[0], true, true
	}
	expressionEffects = func(expression ast.Expr) ([]ps6089ReceiverWrite, bool) {
		source := ps6089CallableExpression(pass, values, expression)
		if !source.known {
			return nil, false
		}
		return sourceEffects(source)
	}
	expressionEscapeEffects = func(expression ast.Expr) ([]ps6089ReceiverWrite, bool) {
		effects, complete := expressionEffects(expression)
		source := ps6089CallableExpression(pass, values, expression)
		result, exact, hasResult := callableSourceResult(source)
		if exact {
			returned, returnedComplete := values.escaping(result, token.NoPos, false, nil)
			effects = append(effects, returned...)
			complete = returnedComplete && complete
		} else if hasResult {
			// An opaque indirect/callable result can expose lifecycle storage.
			complete = false
		}
		return effects, complete
	}
	sourceEffects = func(source ps6089CallableSource) ([]ps6089ReceiverWrite, bool) {
		if source.function != nil && !values.functionReadOnly(source.function) {
			return nil, false
		}
		switch {
		case source.literal != nil:
			return literalEffects(source.literal)
		case source.receiver != nil:
			direct := ps6089EscapingReceiverWrites(values, source.receiver, token.NoPos, false)
			path := ps6089StableReceiverID(pass, source.receiver)
			resolved, complete := values.escaping(source.receiver, token.NoPos, false, nil)
			if len(resolved) == 0 && len(direct) > 0 {
				return direct, complete
			}
			if len(path) > 0 && !ps6089CompositeReceiverExpression(pass, values.expression(path)) {
				node := values.lookup(path)
				if (node == nil || len(node.children) == 0) && values.methodValueMayMutate(pass.TypesInfo.TypeOf(source.receiver), make(map[types.Type]bool)) {
					complete = false
				}
			}
			return append(direct, resolved...), complete
		case source.alias != nil:
			return callableEffects(source.alias)
		default:
			return nil, source.known
		}
	}
	values.callable = expressionEscapeEffects
	values.callableResult = callableResult
	normalizeResult := func(right ast.Expr) (ast.Expr, bool) {
		call, ok := ps2110Unparen(right).(*ast.CallExpr)
		if !ok || ps6089TypeConversion(pass, call) {
			return right, true
		}
		source := ps6089CallableExpression(pass, values, call.Fun)
		if source.literal == nil && source.alias == nil && values.functions[source.function] == nil {
			return right, true
		}
		result, complete := callableResult(call)
		if !complete {
			if source.literal != nil {
				return nil, false
			}
			return right, true
		}
		return result, true
	}
	recordValue := func(left ast.Expr, right ast.Expr) {
		if right != nil {
			var complete bool
			right, complete = normalizeResult(right)
			if !complete {
				right = nil
			}
		}
		path, deref := ps6089ReceiverWriteID(pass, left)
		if deref == 0 && len(path) > 0 {
			values.record(path, right)
		}
	}
	// Exact package initializers participate in the same provenance graph as
	// body-local values, but their storage is not a runtime write in this body.
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok || general.Tok != token.VAR {
				continue
			}
			for _, specification := range general.Specs {
				value, ok := specification.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for index, name := range value.Names {
					if index < len(value.Values) && bodyObjects[identObject(pass, name)] {
						recordValue(name, value.Values[index])
					}
				}
			}
		}
	}
	ast.Inspect(body, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.AssignStmt:
			if len(value.Lhs) == len(value.Rhs) {
				for index := range value.Lhs {
					recordValue(value.Lhs[index], value.Rhs[index])
				}
			} else {
				for _, left := range value.Lhs {
					recordValue(left, nil)
				}
			}
		case *ast.ValueSpec:
			for index, name := range value.Names {
				if index < len(value.Values) {
					recordValue(name, value.Values[index])
				}
			}
		case *ast.RangeStmt:
			if value.Key != nil {
				recordValue(value.Key, nil)
			}
			if value.Value != nil {
				recordValue(value.Value, nil)
			}
		}
		return true
	})
	record := func(left ast.Expr, right ast.Expr) {
		identifier, ok := ps2110Unparen(left).(*ast.Ident)
		if !ok {
			return
		}
		object := identObject(pass, identifier)
		if object == nil {
			return
		}
		// A declaration without an initializer only reserves the storage. The
		// sole later alias-producing assignment is still a single definition.
		if right != nil || pass.TypesInfo.Defs[identifier] == nil {
			counts[object]++
		}
		if right == nil || !ps6089ReceiverAliasType(pass.TypesInfo.TypeOf(left)) {
			return
		}
		definitionPosition := right.End()
		var complete bool
		right, complete = normalizeResult(right)
		if !complete {
			collectionComplete = false
			return
		}
		path, address := ps6089ReceiverAliasSourceID(pass, right)
		_, asserted := ps2110Unparen(right).(*ast.TypeAssertExpr)
		if len(path) > 1 || asserted {
			if exact := values.expression(path); exact != nil {
				if resolved, resolvedAddress := ps6089ReceiverAliasSourceID(pass, exact); len(resolved) > 0 {
					path, address = resolved, resolvedAddress
				}
			}
		}
		if len(path) > 0 && path[0] != object {
			// The source value is snapshotted while evaluating this RHS. All
			// assignment LHS stores happen only after every RHS has completed.
			definitions[object] = definition{id: path, position: definitionPosition, address: address}
		}
	}
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok || general.Tok != token.VAR {
				continue
			}
			for _, specification := range general.Specs {
				value, ok := specification.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for index, name := range value.Names {
					if index < len(value.Values) && bodyObjects[identObject(pass, name)] {
						record(name, value.Values[index])
					}
				}
			}
		}
	}
	receiverWriteID := func(expression ast.Expr) ([]types.Object, int, bool) {
		if path, deref := ps6089ReceiverWriteID(pass, expression); len(path) > 0 {
			return path, deref, false
		}
		deref := 0
		for {
			expression = ps2110Unparen(expression)
			star, ok := expression.(*ast.StarExpr)
			if !ok {
				break
			}
			deref++
			expression = star.X
		}
		call, ok := ps2110Unparen(expression).(*ast.CallExpr)
		if !ok || ps6089TypeConversion(pass, call) {
			return nil, 0, false
		}
		result, exact := callableResult(call)
		if !exact {
			return nil, 0, true
		}
		path, address := ps6089ReceiverAliasSourceID(pass, result)
		if address && deref > 0 {
			deref--
		}
		return path, deref, true
	}
	recordSourceWrite := func(expression ast.Expr, position token.Pos) {
		if path, deref, special := receiverWriteID(expression); len(path) > 0 {
			sourceWrites = append(sourceWrites, ps6089ReceiverWrite{id: path, pos: position, deref: deref})
			if special {
				appendUncertain(ps6089ReceiverWrite{id: path, pos: position, deref: deref})
			}
		}
	}
	appendEscapingAt := func(expression ast.Expr, position token.Pos, persistent bool) {
		if ps6089CommandFactoryExpression(pass, expression) {
			return
		}
		// Runtime expressions in the execution body retain their ordinary alias
		// generation. The value index is only for symbolic callable summaries;
		// resolving a live outer alias here would collapse an old snapshot into a
		// later rebound root before the CFG-aware alias index can reject it.
		direct := ps6089EscapingReceiverWrites(values, expression, position, persistent)
		if len(direct) > 0 && ps6089PointerType(pass.TypesInfo.TypeOf(expression)) &&
			!ps6089UnsafePointerType(pass.TypesInfo.TypeOf(expression)) && !values.exactCompositeValue(expression) {
			appendUncertain(direct...)
			return
		}
		writes, complete := values.escaping(expression, position, persistent, nil)
		if !complete {
			collectionComplete = false
			return
		}
		appendUncertain(append(direct, writes...)...)
	}
	appendAggregateEscapingAt := func(expression ast.Expr, position token.Pos, persistent bool) {
		if ps6089CommandFactoryExpression(pass, expression) {
			return
		}
		direct := ps6089EscapingReceiverWrites(values, expression, position, persistent)
		writes, complete := values.escaping(expression, position, persistent, nil)
		if !complete {
			collectionComplete = false
			return
		}
		if len(writes) == 0 && len(direct) > 0 {
			appendUncertain(direct...)
			return
		}
		if len(writes) == 0 &&
			values.methodValueMayMutate(pass.TypesInfo.TypeOf(expression), make(map[types.Type]bool)) &&
			!values.exactCompositeValue(expression) {
			collectionComplete = false
			return
		}
		appendUncertain(writes...)
	}
	appendCallableAt := func(expression ast.Expr, position token.Pos, persistent bool) {
		effects, complete := expressionEffects(expression)
		if !complete {
			collectionComplete = false
			return
		}
		callableObject := ps6089CallableObject(pass, expression)
		if prior := appliedCallables[callableObject]; callableObject != nil && len(prior) > 0 {
			if !takeCollection(1) {
				return
			}
			for _, index := range prior {
				uncertainWrites[index].persistent = true
			}
			return
		}
		start := len(uncertainWrites)
		for _, effect := range effects {
			effect.pos = position
			effect.persistent = effect.persistent || persistent
			if !appendUncertain(effect) {
				break
			}
		}
		if callableObject != nil && len(uncertainWrites) > start {
			indexes := make([]int, len(uncertainWrites)-start)
			for index := range indexes {
				indexes[index] = start + index
			}
			appliedCallables[callableObject] = indexes
		}
	}
	insidePreWaitFuncLit := make(map[ast.Node]bool)
	astutil.WithStack(body, func(node ast.Node, stack []ast.Node) bool {
		enclosingPreWaitFuncLit := false
		if len(stack) > 0 {
			enclosingPreWaitFuncLit = insidePreWaitFuncLit[stack[len(stack)-1]]
		}
		insidePreWaitFuncLit[node] = enclosingPreWaitFuncLit
		if literal, ok := node.(*ast.FuncLit); ok {
			if !ps6089FuncLitMayRunBeforeWait(pass, stack, literal, enclosingPreWaitFuncLit) {
				return false
			}
			insidePreWaitFuncLit[node] = true
		}
		switch value := node.(type) {
		case *ast.AssignStmt:
			for _, left := range value.Lhs {
				recordSourceWrite(left, value.End())
			}
			if len(value.Lhs) == len(value.Rhs) {
				for index := range value.Lhs {
					record(value.Lhs[index], value.Rhs[index])
					if _, ok := ps2110Unparen(value.Lhs[index]).(*ast.Ident); !ok || ps6089PackageVariable(pass, value.Lhs[index]) {
						appendEscapingAt(value.Rhs[index], value.End(), true)
					}
				}
				// Every RHS is evaluated before any LHS write. An alias sourced
				// from storage rebound by the same assignment snapshots the old
				// storage and cannot be canonicalized to the post-assignment root.
				for leftIndex, left := range value.Lhs {
					identifier, ok := ps2110Unparen(left).(*ast.Ident)
					if !ok {
						continue
					}
					object := identObject(pass, identifier)
					candidate, ok := definitions[object]
					if !ok || candidate.position != value.Rhs[leftIndex].End() {
						continue
					}
					for _, other := range value.Lhs {
						path, deref := ps6089ReceiverWriteID(pass, other)
						if deref == 0 && ps6089ReceiverAliasSourceWriteMatches(candidate.id, candidate.address, path) {
							candidate.invalid = true
							definitions[object] = candidate
							break
						}
					}
				}
			} else {
				if len(value.Rhs) == 1 {
					if _, immediate, complete := ps6089ImmediateLiteralResult(pass, value.Rhs[0]); immediate && !complete {
						for _, left := range value.Lhs {
							if ps6089ReceiverAliasType(pass.TypesInfo.TypeOf(left)) {
								collectionComplete = false
								break
							}
						}
					}
				}
				for _, left := range value.Lhs {
					record(left, nil)
				}
			}
		case *ast.ValueSpec:
			if len(value.Values) == 1 && len(value.Names) != 1 {
				if _, immediate, complete := ps6089ImmediateLiteralResult(pass, value.Values[0]); immediate && !complete {
					for _, name := range value.Names {
						if ps6089ReceiverAliasType(pass.TypesInfo.TypeOf(name)) {
							collectionComplete = false
							break
						}
					}
				}
			}
			for index, name := range value.Names {
				recordSourceWrite(name, value.End())
				var right ast.Expr
				if index < len(value.Values) {
					right = value.Values[index]
				}
				record(name, right)
			}
		case *ast.RangeStmt:
			// Key/value destinations are assigned on each selected body entry,
			// before any body-local alias definition or lifecycle use.
			if value.Key != nil {
				recordSourceWrite(value.Key, value.Key.Pos())
			}
			if value.Value != nil {
				recordSourceWrite(value.Value, value.Value.Pos())
			}
		case *ast.SendStmt:
			appendEscapingAt(value.Value, value.End(), true)
		case *ast.CallExpr:
			if ps6089DirectDeferredCall(stack, value, insidePreWaitFuncLit[node]) {
				break
			}
			if ps6089TypeConversion(pass, value) {
				break
			}
			knownLifecycle := ps6089KnownLifecycleCall(pass, value)
			_, immediateResult, completeImmediateResult := ps6089ImmediateLiteralResult(pass, value)
			async := ps6089DirectGoCall(stack, value)
			directMethod := false
			if selector, ok := ps2110Unparen(value.Fun).(*ast.SelectorExpr); ok {
				selection := pass.TypesInfo.Selections[selector]
				directMethod = selection != nil && selection.Kind() != types.FieldVal
			}
			directFunction := false
			if identifier, ok := ps2110Unparen(value.Fun).(*ast.Ident); ok {
				function, functionCall := identObject(pass, identifier).(*types.Func)
				directFunction = functionCall
				if functionCall && values.functions[function] != nil && !knownLifecycle && len(value.Args) == 0 {
					appendCallableAt(value.Fun, value.End(), async)
				}
			}
			if !directMethod && !directFunction && !knownLifecycle {
				appendCallableAt(value.Fun, value.End(), async)
			}
			receiverArgument := -1
			if selector, ok := ps2110Unparen(value.Fun).(*ast.SelectorExpr); ok {
				selection := pass.TypesInfo.Selections[selector]
				if selection != nil && selection.Kind() == types.MethodExpr {
					receiverArgument = 0
					if !knownLifecycle && len(value.Args) > 0 && values.methodReceiverMayMutate(selection) {
						if path := ps6089StableReceiverID(pass, value.Args[0]); len(path) > 0 && ps6089ReceiverStorageIndirect(pass.TypesInfo.TypeOf(value.Args[0])) {
							sourceWrites = append(sourceWrites, ps6089ReceiverWrite{id: path, pos: value.End(), deref: 1})
							appendAggregateEscapingAt(value.Args[0], value.End(), async)
						} else {
							appendAggregateEscapingAt(value.Args[0], value.End(), async)
						}
					}
				} else if !knownLifecycle && selection != nil && selection.Kind() == types.MethodVal && values.methodReceiverMayMutate(selection) {
					if path := ps6089StableReceiverID(pass, selector.X); len(path) > 0 && ps6089ReceiverStorageIndirect(pass.TypesInfo.TypeOf(selector.X)) {
						sourceWrites = append(sourceWrites, ps6089ReceiverWrite{id: path, pos: value.End(), deref: 1})
						appendAggregateEscapingAt(selector.X, value.End(), async)
					} else {
						appendAggregateEscapingAt(selector.X, value.End(), async)
					}
				}
			}
			for index, argument := range value.Args {
				if immediateResult && completeImmediateResult {
					continue
				}
				if ps6089CallableType(pass.TypesInfo.TypeOf(argument)) {
					appendCallableAt(argument, value.End(), async)
				}
				if index == receiverArgument {
					continue
				}
				argumentType := pass.TypesInfo.TypeOf(argument)
				if !ps6089ReceiverStorageIndirect(argumentType) {
					if values.methodValueMayMutate(argumentType, make(map[types.Type]bool)) {
						appendAggregateEscapingAt(argument, value.End(), true)
					}
					continue
				}
				expression := ps2110Unparen(argument)
				if address, ok := expression.(*ast.UnaryExpr); ok && address.Op == token.AND {
					if path := ps6089StableReceiverID(pass, address.X); len(path) > 0 {
						sourceWrites = append(sourceWrites, ps6089ReceiverWrite{id: path, pos: value.End()})
					}
				} else if path := ps6089StableReceiverID(pass, expression); len(path) > 0 {
					sourceWrites = append(sourceWrites, ps6089ReceiverWrite{id: path, pos: value.End(), deref: 1})
				}
				appendEscapingAt(argument, value.End(), true)
			}
		}
		return true
	})
	aliases := make(map[types.Object]ps6089ReceiverAlias)
	for object, definition := range definitions {
		if counts[object] == 1 && !definition.invalid {
			aliases[object] = ps6089ReceiverAlias{id: definition.id, definition: definition.position, address: definition.address}
		}
	}
	// Binding writes through aliases affect the aliased storage. Normalize to a
	// bounded fixed point: a newly exposed root rebind may invalidate a later
	// pointer-copy expansion on the following pass.
	slices.SortFunc(sourceWrites, func(left, right ps6089ReceiverWrite) int {
		return cmp.Compare(left.pos, right.pos)
	})
	addWrite := func(root *ps6089ReceiverWritePath, write ps6089ReceiverWrite) {
		node := root
		for _, object := range write.id {
			if node.children == nil {
				node.children = make(map[types.Object]*ps6089ReceiverWritePath)
			}
			child := node.children[object]
			if child == nil {
				child = &ps6089ReceiverWritePath{}
				node.children[object] = child
			}
			node = child
		}
		node.writes = append(node.writes, write)
	}
	buildRoot := func(writes []ps6089ReceiverWrite) *ps6089ReceiverWritePath {
		root := &ps6089ReceiverWritePath{}
		for _, write := range writes {
			addWrite(root, write)
		}
		return root
	}
	index := &ps6089ReceiverAliasIndex{
		aliases:  aliases,
		flow:     ps6089NewLifecycleFlow(pass, body),
		limit:    16 * (len(aliases) + len(sourceWrites) + len(uncertainWrites) + nodeCount + 1),
		complete: collectionComplete,
		values:   values,
	}
	for passIndex := 0; passIndex <= len(aliases); passIndex++ {
		index.writes = buildRoot(sourceWrites)
		next := make([]ps6089ReceiverWrite, len(sourceWrites))
		changed := false
		for writeIndex, write := range sourceWrites {
			next[writeIndex] = index.canonicalWrite(write)
			changed = changed || next[writeIndex].deref != write.deref || !slices.Equal(next[writeIndex].id, write.id)
		}
		if !index.complete {
			break
		}
		sourceWrites = next
		if !changed {
			break
		}
		if passIndex == len(aliases) {
			index.complete = false
		}
	}
	index.writes = buildRoot(sourceWrites)
	index.work = 0
	for _, write := range uncertainWrites {
		index.uncertain = append(index.uncertain, index.canonicalWrite(write))
	}
	index.work = 0
	index.buildWork = collectionWork
	return index
}

func ps6089ReceiverAliasSourceWriteMatches(source []types.Object, address bool, write []types.Object) bool {
	limit := len(source)
	if address {
		limit--
	}
	for prefix := 0; prefix < limit; prefix++ {
		if len(write) == prefix+1 && slices.Equal(write, source[:prefix+1]) {
			return true
		}
	}
	return false
}

func (index *ps6089ReceiverAliasIndex) canonical(id []types.Object, use token.Pos) []types.Object {
	if index == nil || len(id) == 0 {
		return id
	}
	result := id
	seen := make(map[types.Object]bool)
	for len(result) > 0 {
		root := result[0]
		alias, ok := index.aliases[root]
		stable := ok && !seen[root] && index.stable(alias, use)
		if !stable {
			break
		}
		copyWork := len(alias.id) + len(result) - 1
		if !index.takeWorkN(copyWork) {
			return id
		}
		seen[root] = true
		result = append(slices.Clone(alias.id), result[1:]...)
		// Keep the original event position for every link. If an intermediate
		// source is rebound after a snapshot, the final generation cannot be
		// represented by that source object's current syntactic identity.
	}
	return result
}

func (index *ps6089ReceiverAliasIndex) stable(alias ps6089ReceiverAlias, use token.Pos) bool {
	limit := len(alias.id)
	if alias.address {
		limit--
	}
	node := index.writes
	for prefix := 0; prefix < limit; prefix++ {
		if !index.takeWork() {
			return false
		}
		if node != nil {
			node = node.children[alias.id[prefix]]
		}
		if node == nil {
			continue
		}
		for _, write := range node.writes {
			if !index.takeWork() {
				return false
			}
			if ps6089ReceiverWriteDetaches(alias.id, limit, prefix, write.deref) && write.pos != alias.definition {
				reaches, complete := index.positionCanReachAvoiding(write.pos, use, alias.definition)
				if !complete || reaches {
					return false
				}
			}
		}
	}
	return true
}

func (index *ps6089ReceiverAliasIndex) takeWork() bool {
	return index.takeWorkN(1)
}

func (index *ps6089ReceiverAliasIndex) takeWorkN(amount int) bool {
	if amount < 0 || index.work > index.limit-amount {
		index.complete = false
		return false
	}
	index.work += amount
	return true
}

func (index *ps6089ReceiverAliasIndex) receiverUncertain(id []types.Object, created, waited token.Pos) bool {
	for _, write := range index.uncertain {
		if !index.takeWork() {
			return true
		}
		if len(write.id) <= len(id) && slices.Equal(write.id, id[:len(write.id)]) {
			if write.persistent {
				// Persistent storage remains live across a benchmark-loop backedge.
				// A post-wait escape is harmless only when CFG reachability proves
				// that it cannot reach the next selected creation event.
				reaches, complete := index.positionCanReachAvoiding(write.pos, created, token.NoPos)
				if !complete || reaches {
					return true
				}
				continue
			}
			before, beforeComplete := index.positionCanReachAvoiding(created, write.pos, waited)
			after, afterComplete := index.positionCanReachAvoiding(write.pos, waited, created)
			if !beforeComplete || !afterComplete || before && after {
				return true
			}
		}
	}
	return false
}

func (index *ps6089ReceiverAliasIndex) canonicalWrite(write ps6089ReceiverWrite) ps6089ReceiverWrite {
	seen := make(map[types.Object]bool)
	for len(write.id) > 0 {
		root := write.id[0]
		alias, ok := index.aliases[root]
		if !ok || seen[root] || !index.stable(alias, write.pos) {
			break
		}
		reaches, complete := index.aliasDefinitionReaches(alias, write.pos)
		if !complete || !reaches {
			break
		}
		// Assigning the alias variable binds the snapshot; it does not write
		// through that snapshot into the source variable. Descendant selector
		// writes and explicit/implicit dereferences still affect source storage.
		if write.deref == 0 && len(write.id) == 1 {
			break
		}
		copyWork := len(alias.id) + len(write.id) - 1
		if !index.takeWorkN(copyWork) {
			break
		}
		seen[root] = true
		write.id = append(slices.Clone(alias.id), write.id[1:]...)
		if alias.address && write.deref > 0 {
			write.deref--
		}
	}
	return write
}

func (index *ps6089ReceiverAliasIndex) aliasDefinitionReaches(alias ps6089ReceiverAlias, use token.Pos) (bool, bool) {
	if index.flow != nil && index.flow.blocks[alias.definition] == nil && index.flow.body != nil && alias.definition < index.flow.body.Pos() {
		return true, true
	}
	return index.positionCanReachAvoiding(alias.definition, use, token.NoPos)
}

func ps6089ReceiverWriteDetaches(source []types.Object, limit, prefix, deref int) bool {
	if deref == 0 {
		return true
	}
	if ps6089PointerDepth(source[prefix].Type()) > deref {
		return true
	}
	for index := prefix + 1; index < limit; index++ {
		if ps6089ReceiverStorageIndirect(source[index].Type()) {
			return true
		}
	}
	return false
}

func ps6089PointerDepth(value types.Type) int {
	depth := 0
	seen := make(map[types.Type]bool)
	for value != nil {
		value = types.Unalias(value)
		if seen[value] {
			return 1 << 20
		}
		seen[value] = true
		if _, ok := value.(*types.TypeParam); ok {
			// A type parameter may instantiate to another pointer layer. Keep
			// dereference writes conservative rather than assuming its interface
			// constraint is the runtime storage shape.
			return 1 << 20
		}
		if ps6089UnsafePointerType(value) {
			return depth + 1
		}
		pointer, ok := value.Underlying().(*types.Pointer)
		if !ok {
			return depth
		}
		depth++
		value = pointer.Elem()
	}
	return depth
}

// ps6089PositionCanReachAvoiding reports whether execution can reach after
// from after before without executing barrier. It is the storage-snapshot
// question needed by receiver aliases: a source rebind is harmless when every
// path to the use refreshes the alias definition first.

func (index *ps6089ReceiverAliasIndex) positionCanReachAvoiding(before, after, barrier token.Pos) (bool, bool) {
	flow := index.flow
	if flow == nil || before == token.NoPos || after == token.NoPos {
		return false, true
	}
	beforeBlock := flow.blocks[before]
	afterBlock := flow.blocks[after]
	barrierBlock := flow.blocks[barrier]
	if beforeBlock == nil || afterBlock == nil || !beforeBlock.Live || !afterBlock.Live {
		return false, true
	}
	blocked := func(block *cfg.Block, minimum, maximum token.Pos) bool {
		return block == barrierBlock && (minimum == token.NoPos || barrier > minimum) && (maximum == token.NoPos || barrier < maximum)
	}
	if beforeBlock == afterBlock && before < after {
		return !blocked(beforeBlock, before, after), true
	}
	type state struct {
		block *cfg.Block
		first bool
	}
	queue := []state{{block: beforeBlock, first: true}}
	seen := make(map[state]bool)
	for len(queue) > 0 {
		if !index.takeWork() {
			return false, false
		}
		current := queue[0]
		queue = queue[1:]
		if seen[current] {
			continue
		}
		seen[current] = true
		minimum := token.NoPos
		if current.first {
			minimum = before
		}
		if current.block == afterBlock && !current.first {
			if !blocked(current.block, token.NoPos, after) {
				return true, true
			}
			continue
		}
		if blocked(current.block, minimum, token.NoPos) {
			continue
		}
		for _, successor := range current.block.Succs {
			queue = append(queue, state{block: successor})
		}
	}
	return false, true
}

func ps6089ReceiverWriteID(pass *analysis.Pass, expression ast.Expr) ([]types.Object, int) {
	deref := 0
	for {
		expression = ps2110Unparen(expression)
		star, ok := expression.(*ast.StarExpr)
		if !ok {
			break
		}
		deref++
		expression = star.X
	}
	return ps6089StableReceiverID(pass, expression), deref
}

func ps6089ReceiverAliasSourceID(pass *analysis.Pass, expression ast.Expr) ([]types.Object, bool) {
	for {
		expression = ps2110Unparen(expression)
		if assertion, ok := expression.(*ast.TypeAssertExpr); ok && assertion.Type != nil {
			expression = assertion.X
			continue
		}
		conversion, ok := expression.(*ast.CallExpr)
		if !ok || !ps6089TypeConversion(pass, conversion) || len(conversion.Args) != 1 {
			break
		}
		expression = conversion.Args[0]
	}
	if address, ok := expression.(*ast.UnaryExpr); ok && address.Op == token.AND {
		return ps6089StableReceiverID(pass, address.X), true
	}
	return ps6089StableReceiverID(pass, expression), false
}

func ps6089ReceiverStorageIndirect(value types.Type) bool {
	if value == nil {
		return false
	}
	value = types.Unalias(value)
	if _, parameter := value.(*types.TypeParam); parameter {
		return true
	}
	switch value.Underlying().(type) {
	case *types.Pointer, *types.Interface:
		return true
	case *types.Basic:
		return ps6089UnsafePointerType(value)
	default:
		return false
	}
}

func ps6089UnsafePointerType(value types.Type) bool {
	if value == nil {
		return false
	}
	basic, ok := types.Unalias(value).Underlying().(*types.Basic)
	return ok && basic.Kind() == types.UnsafePointer
}

func (index *ps6089ReceiverValueIndex) methodReceiverMayMutate(selection *types.Selection) bool {
	function, ok := selection.Obj().(*types.Func)
	if !ok {
		return false
	}
	signature, ok := function.Type().(*types.Signature)
	if !ok || signature.Recv() == nil {
		return false
	}
	receiver := types.Unalias(signature.Recv().Type())
	if _, pointer := receiver.Underlying().(*types.Pointer); pointer || types.IsInterface(receiver) {
		return true
	}
	if index.methodKnown == nil {
		index.methodKnown = make(map[types.Type]bool)
		index.methodMutable = make(map[types.Type]bool)
	}
	return index.methodValueMayMutate(receiver, make(map[types.Type]bool))
}

func (index *ps6089ReceiverValueIndex) functionReadOnly(function *types.Func) bool {
	if function == nil {
		return true
	}
	if index.functionKnown[function] {
		return index.functionPure[function]
	}
	index.functionKnown[function] = true
	declaration := index.functions[function]
	if declaration == nil || declaration.Body == nil {
		return false
	}
	pure := true
	ast.Inspect(declaration.Body, func(node ast.Node) bool {
		if !pure || node == nil {
			return false
		}
		if !index.take(1) {
			pure = false
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			pure = false
			return false
		}
		switch value := node.(type) {
		case *ast.AssignStmt:
			for _, left := range value.Lhs {
				identifier, ok := ps2110Unparen(left).(*ast.Ident)
				if !ok || identifier.Name != "_" && (identObject(index.pass, identifier) == nil || identObject(index.pass, identifier).Pos() < declaration.Pos() || identObject(index.pass, identifier).Pos() > declaration.End()) {
					pure = false
					return false
				}
			}
		case *ast.IncDecStmt, *ast.SendStmt, *ast.GoStmt, *ast.DeferStmt, *ast.RangeStmt:
			pure = false
			return false
		case *ast.CallExpr:
			pure = false
			return false
		}
		return true
	})
	index.functionPure[function] = pure
	return pure
}

func (index *ps6089ReceiverValueIndex) methodValueMayMutate(value types.Type, seen map[types.Type]bool) bool {
	if value == nil {
		return false
	}
	if index.methodKnown == nil {
		index.methodKnown = make(map[types.Type]bool)
		index.methodMutable = make(map[types.Type]bool)
	}
	value = types.Unalias(value)
	if index.methodKnown[value] {
		return index.methodMutable[value]
	}
	if seen[value] {
		return false
	}
	seen[value] = true
	if _, parameter := value.(*types.TypeParam); parameter {
		index.methodKnown[value] = true
		index.methodMutable[value] = true
		return true
	}
	var result bool
	switch aggregate := value.Underlying().(type) {
	case *types.Interface, *types.Signature:
		result = true
	case *types.Pointer:
		result = true
	case *types.Basic:
		result = ps6089UnsafePointerType(value)
	case *types.Slice:
		result = index.methodValueMayMutate(aggregate.Elem(), seen)
	case *types.Map:
		result = index.methodValueMayMutate(aggregate.Key(), seen) || index.methodValueMayMutate(aggregate.Elem(), seen)
	case *types.Chan:
		result = index.methodValueMayMutate(aggregate.Elem(), seen)
	case *types.Array:
		result = index.methodValueMayMutate(aggregate.Elem(), seen)
	case *types.Struct:
		if !index.take(aggregate.NumFields() + 1) {
			result = true
			break
		}
		for fieldIndex := 0; fieldIndex < aggregate.NumFields(); fieldIndex++ {
			if index.methodValueMayMutate(aggregate.Field(fieldIndex).Type(), seen) {
				result = true
				break
			}
		}
	}
	index.methodKnown[value] = true
	index.methodMutable[value] = result
	return result
}

func ps6089ExpandReceiverWrites(writes []ps6089ReceiverWrite, aliases *ps6089ReceiverAliasIndex) []ps6089ReceiverWrite {
	result := slices.Clone(writes)
	for _, write := range writes {
		if len(write.id) == 0 {
			continue
		}
		canonical := aliases.canonicalWrite(write)
		if canonical.deref == write.deref && canonical.persistent == write.persistent && slices.Equal(canonical.id, write.id) {
			continue
		}
		result = append(result, canonical)
	}
	return result
}

func ps6089CanonicalReceiverID(id []types.Object, position token.Pos, aliases *ps6089ReceiverAliasIndex) []types.Object {
	return aliases.canonical(id, position)
}

func ps6089WrittenReceivers(pass *analysis.Pass, statement ast.Stmt, values *ps6089ReceiverValueIndex) []ps6089ReceiverWrite {
	var result []ps6089ReceiverWrite
	recordWrite := func(path []types.Object, deref int, position token.Pos) {
		if len(path) > 0 {
			result = append(result, ps6089ReceiverWrite{id: path, deref: deref, pos: position})
		}
	}
	record := func(path []types.Object, position token.Pos) {
		recordWrite(path, 0, position)
	}
	ordinaryArgument := make(map[ast.Node]bool)
	insideOrdinaryArgument := make(map[ast.Node]bool)
	deferredArgument := make(map[ast.Node]bool)
	insideDeferredArgument := make(map[ast.Node]bool)
	insidePreWaitFuncLit := make(map[ast.Node]bool)
	recognizedMethodReceiver := make(map[*ast.SelectorExpr]bool)
	astutil.WithStack(statement, func(node ast.Node, stack []ast.Node) bool {
		preWaitFuncLit := false
		if len(stack) > 0 {
			preWaitFuncLit = insidePreWaitFuncLit[stack[len(stack)-1]]
		}
		insidePreWaitFuncLit[node] = preWaitFuncLit
		if literal, nested := node.(*ast.FuncLit); nested {
			if !ps6089FuncLitMayRunBeforeWait(pass, stack, literal, preWaitFuncLit) {
				return false
			}
			preWaitFuncLit = true
			insidePreWaitFuncLit[node] = true
		}
		var writes []ast.Expr
		switch value := node.(type) {
		case *ast.AssignStmt:
			writes = value.Lhs
		case *ast.RangeStmt:
			if value.Key != nil {
				writes = append(writes, value.Key)
			}
			if value.Value != nil {
				writes = append(writes, value.Value)
			}
		case *ast.DeclStmt:
			if declaration, ok := value.Decl.(*ast.GenDecl); ok {
				for _, raw := range declaration.Specs {
					if specification, ok := raw.(*ast.ValueSpec); ok {
						for _, name := range specification.Names {
							writes = append(writes, name)
						}
					}
				}
			}
		}
		for _, expression := range writes {
			path, deref := ps6089ReceiverWriteID(pass, expression)
			recordWrite(path, deref, expression.Pos())
		}
		if len(stack) > 0 {
			insideOrdinaryArgument[node] = insideOrdinaryArgument[stack[len(stack)-1]]
			insideDeferredArgument[node] = insideDeferredArgument[stack[len(stack)-1]]
		}
		if ordinaryArgument[node] {
			insideOrdinaryArgument[node] = true
		}
		if deferredArgument[node] {
			insideDeferredArgument[node] = true
		}
		if call, ok := node.(*ast.CallExpr); ok && !ps6089TypeConversion(pass, call) {
			selector, selected := ps2110Unparen(call.Fun).(*ast.SelectorExpr)
			receiverArgument := -1
			deferred := ps6089DirectDeferredCall(stack, call, preWaitFuncLit)
			if !deferred && insideDeferredArgument[node] {
				ordinaryArgument[call.Fun] = true
			}
			if selected {
				selection := pass.TypesInfo.Selections[selector]
				if deferred || ps6089KnownLifecycleCall(pass, call) {
					recognizedMethodReceiver[selector] = true
					if selection != nil && selection.Kind() == types.MethodExpr {
						receiverArgument = 0
					}
				} else if selection != nil && selection.Kind() == types.MethodExpr {
					receiverArgument = 0
					if values.methodReceiverMayMutate(selection) {
						if receiverID, _, _, valid := ps6089CallReceiver(pass, call); valid {
							record(receiverID, call.Pos())
						}
					}
				}
			}
			if !deferred {
				for index, argument := range call.Args {
					if index == receiverArgument {
						continue
					}
					ordinaryArgument[argument] = true
				}
			} else {
				deferredArgument[call.Fun] = true
				for _, argument := range call.Args {
					deferredArgument[argument] = true
				}
			}
		}
		deferredOnly := insideDeferredArgument[node] && !insideOrdinaryArgument[node]
		if selector, ok := node.(*ast.SelectorExpr); ok && !deferredOnly && !recognizedMethodReceiver[selector] {
			selection := pass.TypesInfo.Selections[selector]
			if selection != nil && selection.Kind() == types.MethodVal && values.methodReceiverMayMutate(selection) {
				if path, _, valid := ps6089SelectedReceiver(pass, selector.X, selection); valid {
					record(path, selector.Pos())
				}
			}
		}
		if address, ok := node.(*ast.UnaryExpr); ok && !deferredOnly && address.Op == token.AND {
			record(ps6089StableReceiverID(pass, address.X), address.Pos())
		}
		if !insideOrdinaryArgument[node] || ps6089NestedStableReceiverExpression(pass, node, stack) {
			return true
		}
		expression, ok := node.(ast.Expr)
		if !ok || !ps6089PointerType(pass.TypesInfo.TypeOf(expression)) {
			return true
		}
		record(ps6089StableReceiverID(pass, expression), expression.Pos())
		return true
	})
	return result
}

func ps6089FuncLitMayRunBeforeWait(pass *analysis.Pass, stack []ast.Node, literal *ast.FuncLit, enclosingPreWaitFuncLit bool) bool {
	call, index, ok := ps6089FuncLitCall(pass, stack, literal)
	if !ok {
		return false
	}
	if index == 0 {
		return true
	}
	deferred, direct := stack[index-1].(*ast.DeferStmt)
	return !direct || deferred.Call != call || enclosingPreWaitFuncLit
}

func ps6089FuncLitCall(pass *analysis.Pass, stack []ast.Node, literal *ast.FuncLit) (*ast.CallExpr, int, bool) {
	var expression ast.Expr = literal
	index := len(stack) - 1
	for index >= 0 {
		switch parent := stack[index].(type) {
		case *ast.ParenExpr:
			if parent.X != expression {
				break
			}
			expression = parent
			index--
			continue
		case *ast.CallExpr:
			if !ps6089TypeConversion(pass, parent) || len(parent.Args) != 1 || parent.Args[0] != expression {
				break
			}
			expression = parent
			index--
			continue
		}
		break
	}
	if index < 0 {
		return nil, 0, false
	}
	call, ok := stack[index].(*ast.CallExpr)
	if !ok || call.Fun != expression {
		return nil, 0, false
	}
	return call, index, true
}

func ps6089DirectDeferredCall(stack []ast.Node, call *ast.CallExpr, insidePreWaitFuncLit bool) bool {
	if len(stack) == 0 {
		return false
	}
	deferred, ok := stack[len(stack)-1].(*ast.DeferStmt)
	return ok && deferred.Call == call && !insidePreWaitFuncLit
}

func ps6089DirectGoCall(stack []ast.Node, call *ast.CallExpr) bool {
	if len(stack) == 0 {
		return false
	}
	started, ok := stack[len(stack)-1].(*ast.GoStmt)
	return ok && started.Call == call
}

func ps6089KnownLifecycleCall(pass *analysis.Pass, call *ast.CallExpr) bool {
	callee, signature, ok := typedCallee(pass, call.Fun)
	if !ok || signature.Recv() == nil {
		return false
	}
	_, receiverType, _, receiverOK := ps6089CallReceiver(pass, call)
	if !receiverOK || !ps6089GPUCommandType(receiverType) {
		return false
	}
	name := ps6007NormalizeName(callee.Name())
	return ps6089RotaryName(name) || ps6089CacheName(name) || ps6089CommitName(name) || ps6089WaitName(name)
}

func ps6089CommandFactoryExpression(pass *analysis.Pass, expression ast.Expr) bool {
	call, ok := ps2110Unparen(expression).(*ast.CallExpr)
	if !ok || ps6089TypeConversion(pass, call) {
		return false
	}
	callee, _, ok := typedCallee(pass, call.Fun)
	if !ok || !ps6089CommandCreateName(ps6007NormalizeName(callee.Name())) {
		return false
	}
	result := pass.TypesInfo.TypeOf(call)
	if tuple, ok := result.(*types.Tuple); ok {
		for index := 0; index < tuple.Len(); index++ {
			if ps6089GPUCommandType(tuple.At(index).Type()) {
				return true
			}
		}
		return false
	}
	return ps6089GPUCommandType(result)
}

func ps6089TypeConversion(pass *analysis.Pass, call *ast.CallExpr) bool {
	typeAndValue, ok := pass.TypesInfo.Types[ps2110Unparen(call.Fun)]
	return ok && typeAndValue.IsType()
}

func ps6089CallableType(value types.Type) bool {
	if value == nil {
		return false
	}
	_, ok := types.Unalias(value).Underlying().(*types.Signature)
	return ok
}

func ps6089CallableObject(pass *analysis.Pass, expression ast.Expr) types.Object {
	for {
		expression = ps2110Unparen(expression)
		conversion, ok := expression.(*ast.CallExpr)
		if !ok || !ps6089TypeConversion(pass, conversion) || len(conversion.Args) != 1 {
			break
		}
		expression = conversion.Args[0]
	}
	identifier, ok := expression.(*ast.Ident)
	if !ok {
		return nil
	}
	return identObject(pass, identifier)
}

func ps6089NestedStableReceiverExpression(pass *analysis.Pass, node ast.Node, stack []ast.Node) bool {
	if len(stack) == 0 {
		return false
	}
	parent := stack[len(stack)-1]
	switch value := parent.(type) {
	case *ast.ParenExpr:
		return value.X == node
	case *ast.SelectorExpr:
		selection := pass.TypesInfo.Selections[value]
		return value.X == node && selection != nil && selection.Kind() == types.FieldVal
	case *ast.StarExpr:
		return value.X == node
	}
	return false
}

func ps6089PointerType(value types.Type) bool {
	if value == nil {
		return false
	}
	value = types.Unalias(value)
	if _, parameter := value.(*types.TypeParam); parameter {
		return true
	}
	_, ok := value.Underlying().(*types.Pointer)
	return ok || ps6089UnsafePointerType(value)
}

func ps6089PackageVariable(pass *analysis.Pass, expression ast.Expr) bool {
	identifier, ok := ps2110Unparen(expression).(*ast.Ident)
	if !ok {
		return false
	}
	variable, ok := identObject(pass, identifier).(*types.Var)
	return ok && variable.Parent() == pass.Pkg.Scope()
}

func ps6089ReceiverAliasType(value types.Type) bool {
	return ps6089PointerType(value) || types.IsInterface(types.Unalias(value)) && ps6089GPUCommandType(value)
}

type ps6089PromotedLifecyclePath struct {
	fields []types.Object
	valid  bool
}

func ps6089CreatedCommand(pass *analysis.Pass, statement ast.Stmt, promotedPaths map[types.Type]ps6089PromotedLifecyclePath) ([]types.Object, bool) {
	var left []ast.Expr
	switch value := statement.(type) {
	case *ast.AssignStmt:
		if len(value.Rhs) != 1 {
			return nil, false
		}
		left = value.Lhs
	case *ast.DeclStmt:
		declaration, ok := value.Decl.(*ast.GenDecl)
		if !ok || len(declaration.Specs) != 1 {
			return nil, false
		}
		specification, ok := declaration.Specs[0].(*ast.ValueSpec)
		if !ok || len(specification.Values) != 1 {
			return nil, false
		}
		left = make([]ast.Expr, len(specification.Names))
		for index, name := range specification.Names {
			left[index] = name
		}
	default:
		return nil, false
	}
	call := ps6032StatementCall(statement)
	if call == nil {
		return nil, false
	}
	callee, _, ok := typedCallee(pass, call.Fun)
	if !ok || !ps6089CommandCreateName(ps6007NormalizeName(callee.Name())) {
		return nil, false
	}
	signature, ok := pass.TypesInfo.TypeOf(call.Fun).(*types.Signature)
	if !ok {
		return nil, false
	}
	results := signature.Results()
	if results == nil || results.Len() != len(left) {
		return nil, false
	}
	commandResult := -1
	for index := 0; index < results.Len(); index++ {
		resultType := results.At(index).Type()
		if ps6089ErrorType(resultType) {
			continue
		}
		if ps6089GPUCommandType(resultType) {
			if commandResult >= 0 {
				return nil, false
			}
			commandResult = index
			continue
		}
		return nil, false
	}
	if commandResult < 0 || !ps6089GPUCommandType(pass.TypesInfo.TypeOf(left[commandResult])) {
		return nil, false
	}
	receiverID := ps6089StableReceiverID(pass, left[commandResult])
	if fields, promoted := ps6089UniquePromotedLifecyclePath(pass.TypesInfo.TypeOf(left[commandResult]), promotedPaths); promoted {
		receiverID = append(receiverID, fields...)
	}
	return receiverID, len(receiverID) > 0
}

func ps6089UniquePromotedLifecyclePath(value types.Type, cache map[types.Type]ps6089PromotedLifecyclePath) ([]types.Object, bool) {
	methodType := types.Unalias(value)
	if result, cached := cache[methodType]; cached {
		return result.fields, result.valid
	}
	fields, valid := ps6089UniquePromotedLifecyclePathUncached(methodType)
	cache[methodType] = ps6089PromotedLifecyclePath{fields: fields, valid: valid}
	return fields, valid
}

func ps6089UniquePromotedLifecyclePathUncached(methodType types.Type) ([]types.Object, bool) {
	if _, pointer := methodType.(*types.Pointer); !pointer {
		if _, interfaceType := methodType.Underlying().(*types.Interface); !interfaceType {
			methodType = types.NewPointer(methodType)
		}
	}
	methods := types.NewMethodSet(methodType)
	var path []types.Object
	pathSeen := false
	var rotary, cache, commit, wait bool
	for index := 0; index < methods.Len(); index++ {
		selection := methods.At(index)
		function, ok := selection.Obj().(*types.Func)
		if !ok {
			continue
		}
		name := ps6007NormalizeName(function.Name())
		role := 0
		switch {
		case ps6089RotaryName(name):
			role = 1
		case ps6089CacheName(name):
			role = 2
		case ps6089CommitName(name):
			role = 3
		case ps6089WaitName(name):
			role = 4
		default:
			continue
		}
		indexes := selection.Index()
		if len(indexes) == 0 {
			return nil, false
		}
		fields, receiverType, valid := ps6089SelectionFieldPath(selection.Recv(), indexes[:len(indexes)-1])
		if !valid || !ps6089GPUCommandType(receiverType) {
			return nil, false
		}
		if !pathSeen {
			path = slices.Clone(fields)
			pathSeen = true
		} else if !slices.Equal(path, fields) {
			return nil, false
		}
		switch role {
		case 1:
			rotary = true
		case 2:
			cache = true
		case 3:
			commit = true
		case 4:
			wait = true
		}
	}
	return path, len(path) > 0 && rotary && cache && commit && wait
}

func ps6089ErrorType(value types.Type) bool {
	errorObject := types.Universe.Lookup("error")
	return value != nil && errorObject != nil && types.AssignableTo(value, errorObject.Type())
}

func ps6089BenchmarkForIterations(pass *analysis.Pass, loop *ast.ForStmt) bool {
	expression := ps2110Unparen(loop.Cond)
	if call, ok := expression.(*ast.CallExpr); ok {
		if loop.Init != nil || loop.Post != nil || len(call.Args) != 0 {
			return false
		}
		callee, signature, resolved := typedCallee(pass, call.Fun)
		selector, selected := ps2110Unparen(call.Fun).(*ast.SelectorExpr)
		return resolved && selected && callee.Name() == "Loop" && signature.Recv() != nil && ps6089TestingBType(pass.TypesInfo.TypeOf(selector.X))
	}
	binary, ok := expression.(*ast.BinaryExpr)
	if !ok || binary.Op != token.LSS {
		return false
	}
	counter, ok := ps2110Unparen(binary.X).(*ast.Ident)
	if !ok {
		return false
	}
	counterObject := identObject(pass, counter)
	if counterObject == nil || !ps6089BenchmarkCounterInit(pass, loop.Init, counterObject) || !ps6089BenchmarkCounterPost(pass, loop.Post, counterObject) {
		return false
	}
	return ps6089BenchmarkN(pass, binary.Y)
}

func ps6089BenchmarkCounterInit(pass *analysis.Pass, statement ast.Stmt, counter types.Object) bool {
	assignment, ok := statement.(*ast.AssignStmt)
	if !ok || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
		return false
	}
	id, ok := ps2110Unparen(assignment.Lhs[0]).(*ast.Ident)
	if !ok || identObject(pass, id) != counter {
		return false
	}
	value := pass.TypesInfo.Types[ps2110Unparen(assignment.Rhs[0])].Value
	return value != nil && value.Kind() == constant.Int && value.ExactString() == "0"
}

func ps6089BenchmarkCounterPost(pass *analysis.Pass, statement ast.Stmt, counter types.Object) bool {
	increment, ok := statement.(*ast.IncDecStmt)
	if !ok || increment.Tok != token.INC {
		return false
	}
	id, ok := ps2110Unparen(increment.X).(*ast.Ident)
	return ok && identObject(pass, id) == counter
}

func ps6089BenchmarkRangeIterations(pass *analysis.Pass, expression ast.Expr) bool {
	return ps6089BenchmarkN(pass, expression)
}

func ps6089BenchmarkN(pass *analysis.Pass, expression ast.Expr) bool {
	selector, ok := ps2110Unparen(expression).(*ast.SelectorExpr)
	return ok && selector.Sel.Name == "N" && ps6089TestingBType(pass.TypesInfo.TypeOf(selector.X))
}

func ps6089TestingBType(value types.Type) bool {
	pointer, ok := types.Unalias(value).(*types.Pointer)
	if !ok {
		return false
	}
	named, ok := types.Unalias(pointer.Elem()).(*types.Named)
	return ok && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == "testing" && named.Obj().Name() == "B"
}

func ps6089CommandCreateName(name string) bool {
	switch name {
	case "newcommandbuffer", "createcommandbuffer", "begincommandbuffer", "newrecorder", "createrecorder":
		return true
	}
	return false
}

func ps6089CommitName(name string) bool {
	switch name {
	case "commit", "submit", "enqueue":
		return true
	}
	return false
}

func ps6089WaitName(name string) bool {
	switch name {
	case "wait", "waituntilcompleted", "synchronize", "sync":
		return true
	}
	return false
}

type ps6089Manifest struct {
	lit    *ast.CompositeLit
	fields map[string]ps6016Field
}

func ps6089ReportManifests(pass *analysis.Pass, root ast.Node) {
	ast.Inspect(root, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		literal, ok := node.(*ast.CompositeLit)
		if !ok || !ps6089ManifestType(pass, literal.Type) {
			return true
		}
		manifest := ps6089Manifest{lit: literal, fields: ps6016Fields(pass, literal)}
		if missing := ps6089Missing(manifest.fields); len(missing) > 0 {
			pass.Reportf(manifest.lit.Pos(), "GPU fusion coverage evidence is incomplete; missing %s", strings.Join(missing, ", "))
			return true
		}
		if invalid := ps6089Audit(manifest.fields); len(invalid) > 0 {
			pass.Reportf(manifest.lit.Pos(), "GPU fusion coverage evidence is invalid: %s", strings.Join(invalid, "; "))
		}
		return true
	})
}

func ps6089ManifestType(pass *analysis.Pass, expression ast.Expr) bool {
	value := types.Unalias(pass.TypesInfo.TypeOf(expression))
	named, ok := value.(*types.Named)
	if !ok || named.Obj() == nil {
		return false
	}
	name := named.Obj().Name()
	switch ps6007NormalizeName(name) {
	case "gpufusioncoverageevidence", "recorderfusioncampaign", "commandlifecyclefusionevidence", "fusiontopologycoveragereport":
		return true
	}
	return false
}

type ps6089Axis struct {
	name string
	key  string
	kind byte
}

var ps6089Axes = []ps6089Axis{
	{name: "hardware identity", key: "hardware", kind: 's'},
	{name: "workload identity", key: "workloadidentity", kind: 's'},
	{name: "production variant labels", key: "productionvariantlabels", kind: 'S'},
	{name: "covered variant labels", key: "coveredvariantlabels", kind: 'S'},
	{name: "unfused variant labels", key: "unfusedvariantlabels", kind: 'S'},
	{name: "covered production variant count", key: "coveredproductionvariantcount", kind: 'n'},
	{name: "total production variant count", key: "totalproductionvariantcount", kind: 'n'},
	{name: "leaf commands per buffer", key: "leafcommandsperbuffer", kind: 'n'},
	{name: "production commands per buffer", key: "productioncommandsperbuffer", kind: 'n'},
	{name: "leaf control time", key: "leafcontrolns", kind: 'n'},
	{name: "leaf candidate time", key: "leafcandidatens", kind: 'n'},
	{name: "production control time", key: "productioncontrolns", kind: 'n'},
	{name: "production candidate time", key: "productioncandidatens", kind: 'n'},
	{name: "control event count", key: "controleventcount", kind: 'n'},
	{name: "candidate event count", key: "candidateeventcount", kind: 'n'},
	{name: "event-count oracle", key: "eventcountoraclepassed", kind: 'b'},
	{name: "profile oracle", key: "profileoraclepassed", kind: 'b'},
	{name: "exactness oracle", key: "exactnesspassed", kind: 'b'},
	{name: "same-workload gate", key: "sameworkloadpassed", kind: 'b'},
	{name: "alternating-order gate", key: "alternatingorderpassed", kind: 'b'},
	{name: "promotion threshold", key: "promotionthreshold", kind: 'n'},
	{name: "candidate promotion status", key: "candidatepromoted", kind: 'b'},
	{name: "final decision", key: "finaldecision", kind: 's'},
}

func ps6089Missing(fields map[string]ps6016Field) []string {
	missing := make([]string, 0, len(ps6089Axes))
	for _, axis := range ps6089Axes {
		field, ok := fields[axis.key]
		if !ok || axis.kind == 's' && !field.hasString || axis.kind == 'S' && !field.hasStringValues || axis.kind == 'n' && !field.hasNumber || axis.kind == 'b' && !field.hasBool {
			missing = append(missing, axis.name)
		}
	}
	return missing
}

func ps6089Audit(fields map[string]ps6016Field) []string {
	invalid := make([]string, 0, 16)
	production := fields["productionvariantlabels"].stringValues
	covered := fields["coveredvariantlabels"].stringValues
	unfused := fields["unfusedvariantlabels"].stringValues
	coveredCount := fields["coveredproductionvariantcount"].number
	totalCount := fields["totalproductionvariantcount"].number

	duplicateLabels := ps6089Duplicate(production) || ps6089Duplicate(covered) || ps6089Duplicate(unfused)
	if duplicateLabels {
		invalid = append(invalid, "topology label sets contain duplicates")
	}
	if strings.TrimSpace(fields["hardware"].stringVal) == "" || strings.TrimSpace(fields["workloadidentity"].stringVal) == "" {
		invalid = append(invalid, "hardware and workload identities must be non-empty")
	}
	if len(production) == 0 || totalCount <= 0 || totalCount != float64(len(production)) {
		invalid = append(invalid, fmt.Sprintf("production topology count %.0f disagrees with %d labels", totalCount, len(production)))
	}
	if coveredCount < 0 || coveredCount > totalCount || coveredCount != float64(len(covered)) {
		invalid = append(invalid, fmt.Sprintf("covered topology count %.0f disagrees with %d labels", coveredCount, len(covered)))
	}
	expectedUnfused, setsValid := ps6089CoverageDifference(production, covered)
	if !setsValid {
		invalid = append(invalid, "covered topology labels are not a subset of production labels")
	} else if !slices.Equal(expectedUnfused, ps6089SortedCopy(unfused)) {
		invalid = append(invalid, "unfused topology labels do not equal production minus covered labels")
	}
	if coveredCount >= 0 && coveredCount < totalCount {
		invalid = append(invalid, fmt.Sprintf("covered topology count %.0f of %.0f leaves %.0f unfused production variants", coveredCount, totalCount, totalCount-coveredCount))
	}

	leafDepth := fields["leafcommandsperbuffer"].number
	productionDepth := fields["productioncommandsperbuffer"].number
	if leafDepth <= 0 || productionDepth <= 0 || leafDepth != float64(int64(leafDepth)) || productionDepth != float64(int64(productionDepth)) {
		invalid = append(invalid, "commands-per-buffer depths must be positive")
	} else if leafDepth <= 1 && productionDepth > 1 {
		invalid = append(invalid, fmt.Sprintf("leaf benchmark records %.0f command per buffer while production records %.0f, so leaf timing is command-lifecycle diluted", leafDepth, productionDepth))
	}

	_, leafOK := ps6089Ratio(fields["leafcontrolns"].number, fields["leafcandidatens"].number)
	productionRatio, productionOK := ps6089Ratio(fields["productioncontrolns"].number, fields["productioncandidatens"].number)
	if !leafOK || !productionOK {
		invalid = append(invalid, "leaf and production control/candidate times must be positive")
	}
	controlEvents := fields["controleventcount"].number
	candidateEvents := fields["candidateeventcount"].number
	if controlEvents <= 0 || candidateEvents <= 0 || candidateEvents >= controlEvents || controlEvents != float64(int64(controlEvents)) || candidateEvents != float64(int64(candidateEvents)) {
		invalid = append(invalid, fmt.Sprintf("candidate event count %.0f does not reduce positive control count %.0f", candidateEvents, controlEvents))
	}
	for _, oracle := range []struct {
		key  string
		name string
	}{
		{key: "eventcountoraclepassed", name: "event-count oracle"},
		{key: "profileoraclepassed", name: "profile oracle"},
		{key: "exactnesspassed", name: "exactness oracle"},
		{key: "sameworkloadpassed", name: "same-workload gate"},
		{key: "alternatingorderpassed", name: "alternating-order gate"},
	} {
		if !fields[oracle.key].boolVal {
			invalid = append(invalid, oracle.name+" is explicitly false")
		}
	}
	threshold := fields["promotionthreshold"].number
	promoted := fields["candidatepromoted"].boolVal
	unfusedMatches := setsValid && slices.Equal(expectedUnfused, ps6089SortedCopy(unfused))
	qualified := !duplicateLabels && coveredCount == totalCount && setsValid && unfusedMatches && productionOK && productionRatio >= threshold && threshold > 1 && controlEvents > candidateEvents && fields["eventcountoraclepassed"].boolVal && fields["profileoraclepassed"].boolVal && fields["exactnesspassed"].boolVal && fields["sameworkloadpassed"].boolVal && fields["alternatingorderpassed"].boolVal
	if threshold <= 1 {
		invalid = append(invalid, fmt.Sprintf("promotion threshold %.6gx is not above parity", threshold))
	}
	if promoted && !qualified {
		invalid = append(invalid, fmt.Sprintf("candidate is promoted without full production coverage and a production ratio above the frozen %.6gx threshold", threshold))
	}
	decisionPromotes, decisionKnown := ps6089Decision(fields["finaldecision"].stringVal)
	if !decisionKnown {
		invalid = append(invalid, fmt.Sprintf("final decision %q is not recognized", fields["finaldecision"].stringVal))
	}
	if !qualified && decisionPromotes {
		invalid = append(invalid, fmt.Sprintf("final decision %q treats partial or invalid evidence as promotion proof", fields["finaldecision"].stringVal))
	}
	if decisionKnown && promoted != decisionPromotes {
		invalid = append(invalid, "candidate promotion status disagrees with final decision")
	}
	return invalid
}

func ps6089Decision(value string) (promotes, known bool) {
	switch ps6030StatusName(value) {
	case "promote", "promoted", "retain", "retained", "retaincandidate", "ship", "shipped", "selected", "selectcandidate":
		return true, true
	case "candidateonly", "reject", "rejected", "rejectedcandidate", "donotpromote", "notpromoted", "donotship", "retaincontrol":
		return false, true
	}
	return false, false
}

func ps6089Ratio(control, candidate float64) (float64, bool) {
	if control <= 0 || candidate <= 0 {
		return 0, false
	}
	return control / candidate, true
}

func ps6089Duplicate(values []string) bool {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		key := ps6007NormalizeName(value)
		if key == "" {
			return true
		}
		if _, ok := seen[key]; ok {
			return true
		}
		seen[key] = struct{}{}
	}
	return false
}

func ps6089CoverageDifference(production, covered []string) ([]string, bool) {
	productionSet := make(map[string]struct{}, len(production))
	for _, value := range production {
		productionSet[ps6007NormalizeName(value)] = struct{}{}
	}
	for _, value := range covered {
		key := ps6007NormalizeName(value)
		if _, ok := productionSet[key]; !ok {
			return nil, false
		}
		delete(productionSet, key)
	}
	result := make([]string, 0, len(productionSet))
	for value := range productionSet {
		result = append(result, value)
	}
	slices.Sort(result)
	return result, true
}

func ps6089SortedCopy(values []string) []string {
	result := make([]string, len(values))
	for index, value := range values {
		result[index] = ps6007NormalizeName(value)
	}
	slices.Sort(result)
	return result
}
