package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"slices"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/cfg"

	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS6088 implements owner issue #827. It recognizes a narrow per-call
// goroutine fan-out and fresh WaitGroup barrier that has source-level repeated
// caller evidence. The finding deliberately asks for caller-attributed profile
// and controlled lifecycle evidence; scheduler wait share alone does not prove
// that goroutine construction is the bottleneck or that a pool will be faster.
var PS6088 = register(&lint.Check{
	ID:       "PS6088",
	Category: "verify",
	Slug:     "repeated-fanout-barrier-needs-lifecycle-ab",
	Level:    lint.LevelAggressive,
	Doc: lint.Documentation{
		Title: "a repeated fresh fan-out barrier has no caller-attributed lifecycle A/B evidence",
		Text: `A helper can create a fresh sync.WaitGroup, launch one goroutine for
each dynamically chosen chunk, and immediately wait on every invocation. When
that helper is reached repeatedly, the process pays a fork/join lifecycle on
each dispatch. This shape is worth measuring, but a profile dominated by
runtime.pthread_cond_wait or pthread_cond_signal does not establish that
goroutine construction is the cause: those samples can primarily represent
otherwise idle workers parked by the scheduler.

PS6088 reports only a narrow source shape already proven by PS6086's hardened
barrier analysis:

  - a named package function or method owns a fresh local zero/new/composite
    sync.WaitGroup that has not escaped or been copied before the join;
  - a for or value-domain range loop contains exactly one direct unconditional
    inline goroutine launch, one exact direct Add(1), and a first-statement
    defer of the matching Done; channel control, unsafe iteration capture,
    earlier barrier generations, and paths that can miss the join are rejected;
  - the matching Wait is the very next statement after the fan-out loop; and
  - exact type-resolved, non-test callers provide repetition evidence: either a
    direct call occurs in a loop body or the helper has at least two distinct
    direct production call sites in the analyzed package. Calls in loop
    init/condition/post clauses, calls behind a non-invoked function literal, self
    recursion, statically false Boolean/short-circuit paths, constant-dead
    switch cases, zero-trip ranges, and _test.go files do not establish
    repetition.

The caller evidence is intentionally local and syntactic. It does not prove
that a loop is hot, how often exported helpers are called by other packages, or
whether the fan-out crossover is already tuned. The diagnostic is therefore a
measurement request, not a persistent-pool recommendation and not an automatic
fix. Attribute scheduler wait samples through the exact callers (labels or
equivalent call-path evidence), then compare the unchanged per-call control
against each lifecycle candidate in the same production workload. A pool
candidate must have bounded workers, an inline/serial fallback, explicit
in-worker and nested-call behavior, exact output parity, race coverage, and
independent order-alternating end-to-end samples. Retain it only if it clears
the campaign's predeclared speedup gate; allocation reduction or wait-share
movement alone is insufficient.`,
		Before: `func parallelRows(rows int, body func(int)) {
	var wg sync.WaitGroup
	for row := range rows {
		wg.Add(1)
		go func() {
			defer wg.Done()
			body(row)
		}()
	}
	wg.Wait() // a fresh fork/join on every helper call
}`,
		After: `// Evidence plan, not a source rewrite:
//  1. label/attribute wait samples through every production caller;
//  2. compare unchanged per-call fan-out with bounded-pool and serial/inline
//     candidates using order-alternating end-to-end samples;
//  3. require exact outputs, -race, nesting/in-worker safety, and the retained
//     speedup gate before changing lifecycle policy.`,
		MeasuredWin: `GoAI TinyLlama Q4_K_M on Apple M2 Pro with Go 1.26.6 and
GOMAXPROCS=8: the original profile attributed 84.9% of samples to
pthread_cond_wait and 9.4% to pthread_cond_signal, but replacing per-call
fan-out with the existing bounded pool preserved the exact digest and produced
no win. The first pair measured 2.0700 s control versus 2.0712 s candidate; the
reversed pair measured 2.1564 s control versus 2.5156 s candidate. Candidate
wait shares remained 86.90%/8.49%, so the pool was rejected under the 1.10x
end-to-end gate.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6088",
		Doc:  "repeated fresh fan-out barrier requires caller-attributed controlled lifecycle evidence",
		Run:  runPS6088,
	},
})

type ps6088Candidate struct {
	declaration         *ast.FuncDecl
	launch              *ast.GoStmt
	domain              ast.Expr
	evaluationDomain    ast.Expr
	stableDomainObjects map[types.Object]struct{}
	domainEvidenceSafe  bool
	rangeValueEvaluated bool
	evaluationTerminal  bool
	callback            string
}

type ps6088CallEvidence struct {
	count int
	loop  bool
}

type ps6088SwitchOwnerIndex struct {
	once   sync.Once
	owners map[ast.Expr]*ast.SwitchStmt
}

var ps6088SwitchOwners sync.Map // map[*analysis.Pass]*ps6088SwitchOwnerIndex

type ps6088LocalDomainPassIndex struct {
	functions   sync.Map // map[*ast.FuncDecl]*ps6088LocalDomainIndexEntry
	packageOnce sync.Once
	packageData *ps6088LocalDomainIndex
	ownerOnce   sync.Once
	owners      map[types.Object]*ast.FuncDecl
}

type ps6088LocalDomainIndexEntry struct {
	once  sync.Once
	index *ps6088LocalDomainIndex
}

type ps6088DynamicTypeWrite struct {
	execution token.Pos
	node      token.Pos
}

type ps6088DynamicTypeQuery struct {
	object       types.Object
	position     token.Pos
	wholeRuntime bool
	ignoredStart token.Pos
	ignoredEnd   token.Pos
}

type ps6088PositionBlock struct {
	position token.Pos
	block    *cfg.Block
}

type ps6088ControlFact struct {
	guard   types.Object
	control ast.Node
	clause  *ast.CaseClause
	branch  string
}

type ps6088PositionFacts struct {
	position token.Pos
	facts    []ps6088ControlFact
}

type ps6088BlockStatement struct {
	block *ast.BlockStmt
	index int
}

type ps6088TokenRange struct {
	start token.Pos
	end   token.Pos
}

type ps6088DirectClose struct {
	index int
	call  *ast.CallExpr
}

type ps6088DynamicTypeFlow struct {
	body                   *ast.BlockStmt
	flow                   *cfg.CFG
	blocksAt               []ps6088PositionBlock
	successors             map[*cfg.Block][]*cfg.Block
	predecessors           map[*cfg.Block][]*cfg.Block
	entry                  map[*cfg.Block]bool
	externalWritesRelevant bool
	component              map[*cfg.Block]int
	reach                  [][]uint64
	reverseReach           [][]uint64
	cyclic                 []bool
	entryComponent         int
	parentBody             *ast.BlockStmt
	invocation             token.Pos
	synchronousInvocation  bool
	deferredInvocation     bool
}

type ps6088DynamicTypeFlowWrites struct {
	components []uint64
	byBlock    map[*cfg.Block][]token.Pos
	external   bool
	controlled bool
	detailed   bool
}

type ps6088DynamicTypeFlowQuery struct {
	object types.Object
	body   *ast.BlockStmt
}

type ps6088LocalDomainIndex struct {
	definitions         map[types.Object]ast.Expr
	zero                map[types.Object]bool
	unsafe              map[types.Object]bool
	unsafePositions     map[types.Object][]token.Pos
	dynamicTypeWrites   map[types.Object][]ps6088DynamicTypeWrite
	dynamicTypeQueries  sync.Map // map[ps6088DynamicTypeQuery]bool
	dynamicTypeFlowData sync.Map // map[ps6088DynamicTypeFlowQuery]*ps6088DynamicTypeFlowWrites
	definitionMayRepeat map[types.Object]bool
	flows               []ps6088DynamicTypeFlow
	controlFacts        []ps6088PositionFacts
	hasGoto             bool
	closePaths          map[ast.Node][]ps6088BlockStatement
	evaluationEnds      map[ast.Expr]token.Pos
	sendSnapshotRanges  map[*ast.SendStmt][]ps6088TokenRange
	directCloses        map[*ast.BlockStmt]map[types.Object][]ps6088DirectClose
}

var ps6088LocalDomains sync.Map // map[*analysis.Pass]*ps6088LocalDomainPassIndex

type ps6088StatementReturnPassIndex struct {
	statements sync.Map // map[ast.Stmt]*ps6088StatementReturnEntry
}

type ps6088StatementReturnEntry struct {
	once    [2]sync.Once
	returns [2]bool
}

var ps6088StatementReturns sync.Map // map[*analysis.Pass]*ps6088StatementReturnPassIndex

type ps6088ExpressionReturnPassIndex struct {
	expressions sync.Map // map[ast.Expr]*ps6088ExpressionReturnEntry
}

type ps6088ExpressionReturnEntry struct {
	once    sync.Once
	returns bool
}

var ps6088ExpressionReturns sync.Map // map[*analysis.Pass]*ps6088ExpressionReturnPassIndex

type ps6088DirectPanicPassIndex struct {
	expressions sync.Map // map[ast.Expr]*ps6088DirectPanicEntry
}

type ps6088DirectPanicEntry struct {
	once   sync.Once
	panics bool
}

var ps6088DirectPanics sync.Map // map[*analysis.Pass]*ps6088DirectPanicPassIndex

type ps6088LabelTargetPassIndex struct {
	once    sync.Once
	targets map[types.Object]ast.Stmt
	labels  map[ast.Stmt]string
}

var ps6088LabelTargets sync.Map // map[*analysis.Pass]*ps6088LabelTargetPassIndex

func runPS6088(pass *analysis.Pass) (any, error) {
	defer ps6088SwitchOwners.Delete(pass)
	defer ps6088LocalDomains.Delete(pass)
	defer ps6088StatementReturns.Delete(pass)
	defer ps6088ExpressionReturns.Delete(pass)
	defer ps6088DirectPanics.Delete(pass)
	defer ps6088LabelTargets.Delete(pass)
	candidates := ps6088Candidates(pass)
	if len(candidates) == 0 {
		return nil, nil
	}
	evidence := ps6088CallerEvidence(pass, candidates)
	for _, matches := range candidates {
		for _, match := range matches {
			calls := evidence[match.launch]
			if !calls.loop && calls.count < 2 {
				continue
			}
			var repeated []string
			if calls.loop {
				repeated = append(repeated, "a direct call in a syntactic loop body")
			}
			if calls.count >= 2 {
				repeated = append(repeated, strconv.Itoa(calls.count)+" direct production call sites")
			}
			callback := ""
			if match.callback != "" {
				callback = "; its launched body directly invokes callback parameter " + match.callback
			}
			pass.Reportf(match.launch.Go,
				"%s creates a fresh function-local sync.WaitGroup generation, launches one goroutine per fan-out iteration, and immediately waits; repetition evidence is %s%s — this is a lifecycle measurement candidate, not proof that goroutine construction is the bottleneck: attribute scheduler wait samples through the callers and run a controlled order-alternating end-to-end A/B against bounded-pool and serial/inline candidates with exact outputs, -race, nesting/in-worker safety, and the retained speedup gate (advisory, no automatic fix)",
				match.declaration.Name.Name, strings.Join(repeated, " and "), callback)
		}
	}
	return nil, nil
}

func ps6088Candidates(pass *analysis.Pass) map[*types.Func][]ps6088Candidate {
	result := make(map[*types.Func][]ps6088Candidate)
	for _, file := range pass.Files {
		if ps6088TestFile(pass, file) {
			continue
		}
		for _, declaration := range file.Decls {
			fn, ok := declaration.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			function, ok := pass.TypesInfo.Defs[fn.Name].(*types.Func)
			if !ok {
				continue
			}
			astutil.WithStack(fn.Body, func(node ast.Node, stack []ast.Node) bool {
				if _, literal := node.(*ast.FuncLit); literal {
					return false
				}
				block, ok := node.(*ast.BlockStmt)
				if !ok {
					return true
				}
				loopAncestors := append(stack, block)
				for index := 0; index+1 < len(block.List); index++ {
					candidate, ok := ps6088BlockCandidate(pass, fn, block, index, loopAncestors)
					if ok {
						result[function] = append(result[function], candidate)
					}
				}
				return true
			})
		}
	}
	return result
}

func ps6088BlockCandidate(
	pass *analysis.Pass,
	fn *ast.FuncDecl,
	block *ast.BlockStmt,
	index int,
	loopAncestors []ast.Node,
) (ps6088Candidate, bool) {
	loop := ps6086Loop(block.List[index])
	if loop == nil {
		return ps6088Candidate{}, false
	}
	loopStatement, ok := loop.(ast.Stmt)
	if !ok || !ps6088PriorStatementsReturn(pass, loop, loopAncestors) ||
		!ps6088StatementHeaderReturns(pass, loopStatement) ||
		!ps6088NodeEntryReturns(pass, loop, loopAncestors) {
		return ps6088Candidate{}, false
	}
	launches := ps6086GoStatements(loop)
	if len(launches) != 1 {
		return ps6088Candidate{}, false
	}
	waitObject, ok := ps6086DirectWait(pass, block.List[index+1])
	if !ok || !ps6086FanoutDomainSafe(pass, loop) || !ps6088StaticPathLive(pass, loop, loopAncestors) {
		return ps6088Candidate{}, false
	}
	prefix := ps6088LoopPrefix(loop, loopAncestors)
	if ps6088GotoCrosses(fn.Body, loop.Pos()) ||
		!ps6088LoopMayFanout(pass, loop, launches[0], prefix) ||
		!ps6088PositionLive(pass, fn.Body, launches[0].Go) ||
		!ps6088PositionLive(pass, fn.Body, block.List[index+1].Pos()) {
		return ps6088Candidate{}, false
	}
	fresh := ps6086FreshLocalWaitGroup(pass, fn, waitObject, block.List[index+1].Pos())
	backedge := ps6086NoBackedgeBarrierUse(pass, fn.Body, waitObject, loop, block.List[index+1].End())
	prior := ps6086NoPriorBarrierMethods(pass, fn.Body, waitObject, loop.Pos())
	unconditional := ps6086LaunchUnconditional(pass, loop, launches[0])
	join := ps6086JoinReached(pass, loop)
	capture := ps6086IterationCaptureSafe(pass, fn.Body, loop, launches[0])
	if !fresh || !backedge || !prior || !unconditional || !join || !capture {
		return ps6088Candidate{}, false
	}
	matching := ps6086MatchingLaunches(pass, launches, waitObject)
	unit := len(matching) == 1 && ps6086HasExactUnitAdd(pass, loop, waitObject, matching[0])
	if len(matching) != 1 || !unit {
		return ps6088Candidate{}, false
	}
	domain, _ := ps6086LoopDomainExpression(pass, loop)
	evaluationDomain := ps6088LoopEvaluationDomain(pass, loop, domain)
	localDomain := ps6088ExpressionUsesFunctionLocal(pass, fn, domain)
	var domainSourceSafe bool
	domain, domainSourceSafe = ps6088ResolveImmutableLocalDomain(pass, fn, domain)
	if !domainSourceSafe {
		return ps6088Candidate{}, false
	}
	if localDomain {
		evaluationDomain = domain
		if !ps6088DomainShapeSupported(pass, domain) {
			return ps6088Candidate{}, false
		}
		if atMostOne, known := ps6088DomainAtMost(pass, domain, 1); known && atMostOne {
			return ps6088Candidate{}, false
		}
	}
	stableDomainObjects, domainEvidenceSafe :=
		ps6088DomainObjectState(pass, fn, domain, prefix, loopAncestors)
	rangeValueEvaluated := false
	if rangeLoop, ok := loop.(*ast.RangeStmt); ok && rangeLoop.Value != nil {
		rangeValueEvaluated = true
	}
	evaluationTerminal := !rangeValueEvaluated && !localDomain
	return ps6088Candidate{
		declaration:         fn,
		launch:              matching[0],
		domain:              domain,
		evaluationDomain:    evaluationDomain,
		stableDomainObjects: stableDomainObjects,
		domainEvidenceSafe:  domainEvidenceSafe,
		rangeValueEvaluated: rangeValueEvaluated,
		evaluationTerminal:  evaluationTerminal,
		callback:            ps6088CallbackParameter(pass, fn, loop, matching[0], waitObject),
	}, true
}

func ps6088ResolveImmutableLocalDomain(
	pass *analysis.Pass,
	fn *ast.FuncDecl,
	expression ast.Expr,
) (ast.Expr, bool) {
	seen := make(map[types.Object]bool)
	index := ps6088FunctionLocalDomainIndex(pass, fn)
	for {
		identifier, ok := ps2110Unparen(expression).(*ast.Ident)
		if !ok {
			return expression, !ps6088ExpressionUsesFunctionLocal(pass, fn, expression)
		}
		object := pass.TypesInfo.ObjectOf(identifier)
		if !ps6088FunctionLocalObject(pass, fn, object) {
			return expression, true
		}
		if seen[object] || !ps6088LocalDomainValueType(object.Type()) {
			return expression, false
		}
		seen[object] = true
		value, found := index.definitions[object]
		if !found || index.unsafe[object] {
			return expression, false
		}
		expression = value
	}
}

func ps6088ExpressionUsesFunctionLocal(
	pass *analysis.Pass,
	fn *ast.FuncDecl,
	expression ast.Expr,
) bool {
	local := false
	ast.Inspect(expression, func(node ast.Node) bool {
		if local {
			return false
		}
		identifier, ok := node.(*ast.Ident)
		local = ok && ps6088FunctionLocalObject(pass, fn, pass.TypesInfo.ObjectOf(identifier))
		return !local
	})
	return local
}

func ps6088LocalDomainValueType(value types.Type) bool {
	basic, ok := types.Unalias(value).Underlying().(*types.Basic)
	return ok && basic.Info()&(types.IsInteger|types.IsString) != 0
}

func ps6088FunctionLocalDomainIndex(
	pass *analysis.Pass,
	fn *ast.FuncDecl,
) *ps6088LocalDomainIndex {
	value, ok := ps6088LocalDomains.Load(pass)
	if !ok {
		value, _ = ps6088LocalDomains.LoadOrStore(pass, new(ps6088LocalDomainPassIndex))
	}
	passIndex := value.(*ps6088LocalDomainPassIndex)
	value, ok = passIndex.functions.Load(fn)
	if !ok {
		value, _ = passIndex.functions.LoadOrStore(fn, new(ps6088LocalDomainIndexEntry))
	}
	entry := value.(*ps6088LocalDomainIndexEntry)
	entry.once.Do(func() { entry.index = ps6088BuildLocalDomainIndex(pass, fn) })
	return entry.index
}

func ps6088BuildLocalDomainIndex(pass *analysis.Pass, fn *ast.FuncDecl) *ps6088LocalDomainIndex {
	index := &ps6088LocalDomainIndex{
		definitions:         make(map[types.Object]ast.Expr),
		zero:                make(map[types.Object]bool),
		unsafe:              make(map[types.Object]bool),
		unsafePositions:     make(map[types.Object][]token.Pos),
		dynamicTypeWrites:   make(map[types.Object][]ps6088DynamicTypeWrite),
		definitionMayRepeat: make(map[types.Object]bool),
		closePaths:          make(map[ast.Node][]ps6088BlockStatement),
		evaluationEnds:      make(map[ast.Expr]token.Pos),
		sendSnapshotRanges:  make(map[*ast.SendStmt][]ps6088TokenRange),
		directCloses:        make(map[*ast.BlockStmt]map[types.Object][]ps6088DirectClose),
	}
	statementPositions := make(map[ast.Stmt]ps6088BlockStatement)
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		block, ok := node.(*ast.BlockStmt)
		if !ok {
			return true
		}
		for statementIndex, statement := range block.List {
			statementPositions[statement] = ps6088BlockStatement{block, statementIndex}
			closeCall := ps6088DirectCloseCall(pass, statement)
			if closeCall == nil {
				continue
			}
			identifier, ok := ps2110Unparen(closeCall.Args[0]).(*ast.Ident)
			if !ok {
				continue
			}
			object := pass.TypesInfo.ObjectOf(identifier)
			if object == nil {
				continue
			}
			closes := index.directCloses[block]
			if closes == nil {
				closes = make(map[types.Object][]ps6088DirectClose)
				index.directCloses[block] = closes
			}
			closes[object] = append(
				closes[object], ps6088DirectClose{statementIndex, closeCall},
			)
		}
		return true
	})
	astutil.WithStack(fn.Body, func(node ast.Node, stack []ast.Node) bool {
		if expression, ok := node.(*ast.IndexExpr); ok {
			for stackIndex := len(stack) - 1; stackIndex >= 0; stackIndex-- {
				statement, ok := stack[stackIndex].(ast.Stmt)
				if ok {
					index.evaluationEnds[expression] = statement.End()
					break
				}
			}
		}
		if node != nil {
			if branch, ok := node.(*ast.BranchStmt); ok && branch.Tok == token.GOTO {
				index.hasGoto = true
			}
			if facts := ps6088ControlFactsForPosition(pass, node.Pos(), stack); len(facts) > 0 {
				index.controlFacts = append(
					index.controlFacts, ps6088PositionFacts{node.Pos(), facts},
				)
			}
		}
		closeConsumer := false
		if call, ok := node.(*ast.CallExpr); ok {
			closeConsumer = typedBuiltinName(pass, call.Fun, "close") &&
				len(call.Args) == 1 && !call.Ellipsis.IsValid()
		} else if send, ok := node.(*ast.SendStmt); ok {
			closeConsumer = true
			var ownerClause *ast.CommClause
			var ownerSelect *ast.SelectStmt
			for stackIndex := len(stack) - 1; stackIndex >= 0; stackIndex-- {
				switch ancestor := stack[stackIndex].(type) {
				case *ast.CommClause:
					if ancestor.Comm == send {
						ownerClause = ancestor
					}
				case *ast.SelectStmt:
					ownerSelect = ancestor
				}
				if ownerClause != nil && ownerSelect != nil {
					break
				}
			}
			if ownerClause != nil && ownerSelect != nil {
				afterOwner := false
				for _, item := range ownerSelect.Body.List {
					clause, ok := item.(*ast.CommClause)
					if !ok {
						continue
					}
					if clause == ownerClause {
						afterOwner = true
						continue
					}
					if !afterOwner {
						continue
					}
					for _, operand := range ps6088SelectCommOperandExpressions(clause.Comm) {
						index.sendSnapshotRanges[send] = append(
							index.sendSnapshotRanges[send],
							ps6088TokenRange{operand.Pos(), operand.End()},
						)
					}
				}
			}
		}
		if closeConsumer {
			for stackIndex, ancestor := range stack {
				if _, ok := ancestor.(*ast.BlockStmt); !ok || stackIndex+1 >= len(stack) {
					continue
				}
				child, ok := stack[stackIndex+1].(ast.Stmt)
				if !ok {
					continue
				}
				if position, found := statementPositions[child]; found {
					index.closePaths[node] = append(index.closePaths[node], position)
				}
			}
		}
		identifier, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		object := pass.TypesInfo.Defs[identifier]
		if _, variable := object.(*types.Var); !variable {
			return true
		}
		for _, ancestor := range stack {
			if !astutil.IsLoop(ancestor) {
				continue
			}
			body := astutil.LoopBody(ancestor)
			if body != nil && body.Pos() <= identifier.Pos() && identifier.Pos() < body.End() {
				index.definitionMayRepeat[object] = true
				break
			}
		}
		return true
	})
	slices.SortFunc(index.controlFacts, func(left, right ps6088PositionFacts) int {
		switch {
		case left.position < right.position:
			return -1
		case left.position > right.position:
			return 1
		default:
			return 0
		}
	})
	index.controlFacts = slices.CompactFunc(index.controlFacts, func(left, right ps6088PositionFacts) bool {
		return left.position == right.position
	})
	unsafePositionSeen := make(map[types.Object]map[token.Pos]bool)
	markObject := func(object types.Object, position token.Pos) {
		if object == nil {
			return
		}
		positions := unsafePositionSeen[object]
		if positions == nil {
			positions = make(map[token.Pos]bool)
			unsafePositionSeen[object] = positions
		}
		if positions[position] {
			return
		}
		positions[position] = true
		index.unsafe[object] = true
		index.unsafePositions[object] = append(index.unsafePositions[object], position)
	}
	markDynamicTypeUnsafe := func(object types.Object, execution, node token.Pos) {
		if object == nil {
			return
		}
		index.dynamicTypeWrites[object] = append(
			index.dynamicTypeWrites[object],
			ps6088DynamicTypeWrite{execution: execution, node: node},
		)
	}
	mark := func(expression ast.Expr) {
		if expression == nil {
			return
		}
		ast.Inspect(expression, func(node ast.Node) bool {
			identifier, ok := node.(*ast.Ident)
			if ok {
				if object := pass.TypesInfo.ObjectOf(identifier); object != nil {
					markObject(object, identifier.Pos())
				}
			}
			return true
		})
	}
	markDynamic := func(expression ast.Expr) {
		if expression == nil {
			return
		}
		ast.Inspect(expression, func(node ast.Node) bool {
			identifier, ok := node.(*ast.Ident)
			if ok {
				markDynamicTypeUnsafe(
					pass.TypesInfo.ObjectOf(identifier), identifier.Pos(), identifier.Pos(),
				)
			}
			return true
		})
	}
	var markDestinationDynamic func(ast.Expr)
	markDestinationDynamic = func(expression ast.Expr) {
		switch destination := ps2110Unparen(expression).(type) {
		case *ast.IndexExpr:
			// Storing through an index may mutate the indexed container, but
			// merely evaluating the key cannot rebind identifiers used by it.
			markDestinationDynamic(destination.X)
		case *ast.SelectorExpr:
			markDestinationDynamic(destination.X)
		case *ast.StarExpr:
			markDynamic(destination.X)
		default:
			markDynamic(expression)
		}
	}
	markReferences := func(expression ast.Expr) {
		if expression == nil {
			return
		}
		ast.Inspect(expression, func(node ast.Node) bool {
			identifier, ok := node.(*ast.Ident)
			if ok {
				object := pass.TypesInfo.ObjectOf(identifier)
				if object != nil && ps6088ContainsReference(object.Type(), make(map[types.Type]bool)) {
					markObject(object, identifier.Pos())
				}
			}
			return true
		})
	}
	aliases := make(map[types.Object]types.Object)
	benignAliasExpressions := make(map[ast.Expr]struct{})
	recordDefinition := func(object types.Object, expression ast.Expr) {
		if object == nil {
			return
		}
		index.definitions[object] = expression
		identifier, ok := ps2110Unparen(expression).(*ast.Ident)
		if !ok || !ps6088ContainsReference(object.Type(), make(map[types.Type]bool)) {
			return
		}
		source := pass.TypesInfo.ObjectOf(identifier)
		if source == nil {
			return
		}
		aliases[object] = source
		benignAliasExpressions[expression] = struct{}{}
	}
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.AssignStmt:
			if value.Tok == token.DEFINE && len(value.Lhs) == len(value.Rhs) {
				for position, left := range value.Lhs {
					identifier, ok := ps2110Unparen(left).(*ast.Ident)
					if ok {
						recordDefinition(pass.TypesInfo.Defs[identifier], value.Rhs[position])
					}
				}
			}
			for _, left := range value.Lhs {
				identifier, ok := ps2110Unparen(left).(*ast.Ident)
				if ok {
					if object := pass.TypesInfo.Uses[identifier]; object != nil {
						markObject(object, identifier.Pos())
						markDynamicTypeUnsafe(object, value.End(), identifier.Pos())
					}
				} else {
					mark(left)
					markDestinationDynamic(left)
				}
			}
			for _, right := range value.Rhs {
				if _, benign := benignAliasExpressions[right]; benign {
					continue
				}
				markReferences(right)
			}
		case *ast.ValueSpec:
			if len(value.Names) == len(value.Values) {
				for position, name := range value.Names {
					recordDefinition(pass.TypesInfo.Defs[name], value.Values[position])
				}
			} else if len(value.Values) == 0 {
				for _, name := range value.Names {
					if object := pass.TypesInfo.Defs[name]; object != nil {
						index.zero[object] = true
					}
				}
			}
			for _, expression := range value.Values {
				if _, benign := benignAliasExpressions[expression]; benign {
					continue
				}
				markReferences(expression)
			}
		case *ast.CallExpr:
			for _, argument := range value.Args {
				markReferences(argument)
			}
		case *ast.ReturnStmt:
			for _, result := range value.Results {
				markReferences(result)
			}
		case *ast.SendStmt:
			markReferences(value.Value)
		case *ast.IncDecStmt:
			mark(value.X)
			markDestinationDynamic(value.X)
		case *ast.RangeStmt:
			if value.Tok != token.DEFINE {
				mark(value.Key)
				mark(value.Value)
				markDestinationDynamic(value.Key)
				markDestinationDynamic(value.Value)
			}
		case *ast.UnaryExpr:
			if value.Op == token.AND {
				mark(value.X)
				markDynamic(value.X)
			}
		case *ast.SelectorExpr:
			selection := pass.TypesInfo.Selections[value]
			if selection == nil || selection.Kind() != types.MethodVal {
				break
			}
			method, ok := selection.Obj().(*types.Func)
			if !ok {
				break
			}
			signature, ok := method.Type().(*types.Signature)
			if !ok || signature.Recv() == nil {
				break
			}
			if _, pointer := types.Unalias(signature.Recv().Type()).Underlying().(*types.Pointer); pointer {
				mark(value.X)
				markDynamic(value.X)
			}
		}
		return true
	})
	earliestUnsafe := make(map[types.Object]token.Pos)
	for object, positions := range index.unsafePositions {
		for _, position := range positions {
			if current, found := earliestUnsafe[object]; !found || position < current {
				earliestUnsafe[object] = position
			}
		}
	}
	orderedAliases := make([]types.Object, 0, len(aliases))
	for alias := range aliases {
		orderedAliases = append(orderedAliases, alias)
	}
	slices.SortFunc(orderedAliases, func(left, right types.Object) int {
		switch {
		case left.Pos() > right.Pos():
			return -1
		case left.Pos() < right.Pos():
			return 1
		default:
			return strings.Compare(left.Name(), right.Name())
		}
	})
	for _, alias := range orderedAliases {
		position, unsafe := earliestUnsafe[alias]
		if !unsafe {
			continue
		}
		source := aliases[alias]
		if current, found := earliestUnsafe[source]; !found || position < current {
			earliestUnsafe[source] = position
		}
		index.unsafe[source] = true
	}
	for object, position := range earliestUnsafe {
		index.unsafePositions[object] = []token.Pos{position}
	}
	rootFlow := ps6088BuildDynamicTypeFlow(pass, fn.Body)
	rootFlow.externalWritesRelevant = true
	index.flows = append(index.flows, rootFlow)
	astutil.WithStack(fn.Body, func(node ast.Node, stack []ast.Node) bool {
		literal, ok := node.(*ast.FuncLit)
		if !ok {
			return true
		}
		flow := ps6088BuildDynamicTypeFlow(pass, literal.Body)
		flow.parentBody = fn.Body
		for index := len(stack) - 1; index >= 0; index-- {
			if parent, ok := stack[index].(*ast.FuncLit); ok {
				flow.parentBody = parent.Body
				break
			}
		}
		invocation := ps6088ImmediateFuncLit(pass, literal, stack)
		if invocation != nil {
			flow.invocation = invocation.Pos()
			flow.synchronousInvocation = !ps6088AsyncCall(invocation, stack, false)
			flow.deferredInvocation = ps6088DeferredCall(invocation, stack)
		}
		flow.externalWritesRelevant = !ps6088DiscardedFuncLit(pass, literal, stack)
		index.flows = append(index.flows, flow)
		return true
	})
	return index
}

func ps6088ControlFactsForPosition(
	pass *analysis.Pass,
	position token.Pos,
	stack []ast.Node,
) []ps6088ControlFact {
	var facts []ps6088ControlFact
	for _, ancestor := range stack {
		switch statement := ancestor.(type) {
		case *ast.IfStmt:
			guard, truth, ok := ps6088BooleanGuard(pass, statement.Cond)
			if !ok {
				continue
			}
			branch := ""
			if statement.Body.Pos() <= position && position < statement.Body.End() {
				branch = strconv.FormatBool(truth)
			} else if statement.Else != nil && statement.Else.Pos() <= position && position < statement.Else.End() {
				branch = strconv.FormatBool(!truth)
			}
			if branch != "" {
				facts = append(facts, ps6088ControlFact{guard: guard, branch: branch})
			}
		case *ast.SwitchStmt:
			identifier, ok := ps2110Unparen(statement.Tag).(*ast.Ident)
			if !ok {
				continue
			}
			guard := pass.TypesInfo.ObjectOf(identifier)
			if guard == nil {
				continue
			}
			for _, item := range statement.Body.List {
				clause, ok := item.(*ast.CaseClause)
				if !ok || !(clause.Pos() <= position && position < clause.End()) {
					continue
				}
				branch := "default"
				if len(clause.List) == 1 {
					if value := pass.TypesInfo.Types[clause.List[0]].Value; value != nil {
						branch = "case:" + value.ExactString()
					}
				}
				facts = append(facts, ps6088ControlFact{
					guard: guard, control: statement, clause: clause, branch: branch,
				})
				break
			}
		}
	}
	return facts
}

func ps6088BooleanGuard(
	pass *analysis.Pass,
	expression ast.Expr,
) (types.Object, bool, bool) {
	truth := true
	expression = ps2110Unparen(expression)
	if unary, ok := expression.(*ast.UnaryExpr); ok && unary.Op == token.NOT {
		truth = false
		expression = ps2110Unparen(unary.X)
	}
	identifier, ok := expression.(*ast.Ident)
	if !ok {
		return nil, false, false
	}
	object := pass.TypesInfo.ObjectOf(identifier)
	typeOf := pass.TypesInfo.TypeOf(identifier)
	if typeOf == nil {
		return nil, false, false
	}
	basic, ok := types.Unalias(typeOf).Underlying().(*types.Basic)
	return object, truth, object != nil && ok && basic.Kind() == types.Bool
}

func ps6088DiscardedFuncLit(
	pass *analysis.Pass,
	literal *ast.FuncLit,
	stack []ast.Node,
) bool {
	if ps6088ImmediateFuncLit(pass, literal, stack) != nil {
		return false
	}
	for index := len(stack) - 1; index >= 0; index-- {
		assignment, ok := stack[index].(*ast.AssignStmt)
		if !ok {
			continue
		}
		if len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 ||
			ps2110Unparen(assignment.Rhs[0]) != literal {
			return false
		}
		identifier, ok := ps2110Unparen(assignment.Lhs[0]).(*ast.Ident)
		return ok && identifier.Name == "_"
	}
	return false
}

func ps6088BuildDynamicTypeFlow(
	pass *analysis.Pass,
	body *ast.BlockStmt,
) ps6088DynamicTypeFlow {
	result := ps6088DynamicTypeFlow{
		body:         body,
		flow:         cfg.New(body, func(call *ast.CallExpr) bool { return !ps6088NonreturnCall(pass, call) }),
		successors:   make(map[*cfg.Block][]*cfg.Block),
		predecessors: make(map[*cfg.Block][]*cfg.Block),
	}
	for _, block := range result.flow.Blocks {
		for _, node := range block.Nodes {
			ast.Inspect(node, func(child ast.Node) bool {
				if child == nil {
					return true
				}
				result.blocksAt = append(
					result.blocksAt, ps6088PositionBlock{child.Pos(), block},
				)
				_, literal := child.(*ast.FuncLit)
				return !literal
			})
		}
	}
	slices.SortFunc(result.blocksAt, func(left, right ps6088PositionBlock) int {
		switch {
		case left.position < right.position:
			return -1
		case left.position > right.position:
			return 1
		default:
			return 0
		}
	})
	result.blocksAt = slices.CompactFunc(result.blocksAt, func(left, right ps6088PositionBlock) bool {
		return left.position == right.position
	})
	for _, block := range result.flow.Blocks {
		next := ps6088StaticCFGSuccessors(pass, block)
		result.successors[block] = next
		for _, successor := range next {
			result.predecessors[successor] = append(result.predecessors[successor], block)
		}
	}
	if len(result.flow.Blocks) > 0 {
		result.entry = ps6088ReachableBlocks(result.flow.Blocks[0], result.successors)
		ps6088BuildDynamicTypeReachability(&result)
	}
	return result
}

func ps6088BuildDynamicTypeReachability(flow *ps6088DynamicTypeFlow) {
	indices := make(map[*cfg.Block]int)
	lowlinks := make(map[*cfg.Block]int)
	onStack := make(map[*cfg.Block]bool)
	flow.component = make(map[*cfg.Block]int)
	stack := make([]*cfg.Block, 0, len(flow.flow.Blocks))
	nextIndex := 1
	var visit func(*cfg.Block)
	visit = func(block *cfg.Block) {
		indices[block] = nextIndex
		lowlinks[block] = nextIndex
		nextIndex++
		stack = append(stack, block)
		onStack[block] = true
		for _, successor := range flow.successors[block] {
			if indices[successor] == 0 {
				visit(successor)
				lowlinks[block] = min(lowlinks[block], lowlinks[successor])
			} else if onStack[successor] {
				lowlinks[block] = min(lowlinks[block], indices[successor])
			}
		}
		if lowlinks[block] != indices[block] {
			return
		}
		component := len(flow.cyclic)
		size := 0
		for {
			last := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			onStack[last] = false
			flow.component[last] = component
			size++
			if last == block {
				break
			}
		}
		flow.cyclic = append(flow.cyclic, size > 1)
	}
	for _, block := range flow.flow.Blocks {
		if indices[block] == 0 {
			visit(block)
		}
	}
	componentSuccessors := make([][]int, len(flow.cyclic))
	for _, block := range flow.flow.Blocks {
		component := flow.component[block]
		for _, successor := range flow.successors[block] {
			next := flow.component[successor]
			if next == component {
				if successor == block {
					flow.cyclic[component] = true
				}
				continue
			}
			if !slices.Contains(componentSuccessors[component], next) {
				componentSuccessors[component] = append(componentSuccessors[component], next)
			}
		}
	}
	words := (len(flow.cyclic) + 63) / 64
	flow.reach = make([][]uint64, len(flow.cyclic))
	built := make([]bool, len(flow.cyclic))
	var build func(int)
	build = func(component int) {
		if built[component] {
			return
		}
		built[component] = true
		flow.reach[component] = make([]uint64, words)
		ps6088SetBit(flow.reach[component], component)
		for _, successor := range componentSuccessors[component] {
			build(successor)
			for word := range words {
				flow.reach[component][word] |= flow.reach[successor][word]
			}
		}
	}
	for component := range flow.cyclic {
		build(component)
	}
	flow.reverseReach = make([][]uint64, len(flow.cyclic))
	for component := range flow.cyclic {
		flow.reverseReach[component] = make([]uint64, words)
	}
	for source, reached := range flow.reach {
		for target := range flow.cyclic {
			if ps6088HasBit(reached, target) {
				ps6088SetBit(flow.reverseReach[target], source)
			}
		}
	}
	flow.entryComponent = flow.component[flow.flow.Blocks[0]]
}

func ps6088SetBit(bits []uint64, index int) {
	bits[index/64] |= uint64(1) << (index % 64)
}

func ps6088HasBit(bits []uint64, index int) bool {
	return bits[index/64]&(uint64(1)<<(index%64)) != 0
}

func ps6088StableExpression(
	pass *analysis.Pass,
	expression ast.Expr,
) (ast.Expr, bool, bool) {
	return ps6088StableExpressionAt(pass, expression, expression.Pos())
}

func ps6088StableExpressionAt(
	pass *analysis.Pass,
	expression ast.Expr,
	position token.Pos,
) (ast.Expr, bool, bool) {
	seen := make(map[types.Object]bool)
	for {
		expression = ps2110Unparen(expression)
		identifier, ok := expression.(*ast.Ident)
		if !ok {
			return expression, false, true
		}
		object := pass.TypesInfo.ObjectOf(identifier)
		if object == types.Universe.Lookup("nil") {
			return expression, true, true
		}
		if object == nil || seen[object] {
			return expression, false, false
		}
		seen[object] = true
		index := ps6088StableObjectIndex(pass, object, position)
		packageVariable := ps6088PackageVariable(object)
		unsafe := index != nil && (packageVariable && (object.Exported() || index.unsafe[object]) ||
			!packageVariable && ps6088ObjectUnsafeBefore(index, object, position))
		if index == nil || unsafe {
			return expression, false, index == nil
		}
		if index.zero[object] {
			return expression, true, true
		}
		definition, found := index.definitions[object]
		if !found {
			return expression, false, false
		}
		expression = definition
	}
}

func ps6088StableExpressionForComparisonAt(
	pass *analysis.Pass,
	expression ast.Expr,
	position token.Pos,
	wholeRuntime bool,
) (ast.Expr, bool, bool, token.Pos, bool) {
	return ps6088StableExpressionForRuntimeAt(
		pass, expression, position, wholeRuntime, token.NoPos, token.NoPos,
	)
}

func ps6088StableExpressionForRuntimeAt(
	pass *analysis.Pass,
	expression ast.Expr,
	position token.Pos,
	wholeRuntime bool,
	ignoredStart token.Pos,
	ignoredEnd token.Pos,
) (ast.Expr, bool, bool, token.Pos, bool) {
	seen := make(map[types.Object]bool)
	for {
		expression = ps2110Unparen(expression)
		identifier, ok := expression.(*ast.Ident)
		if !ok {
			return expression, false, true, position, wholeRuntime
		}
		object := pass.TypesInfo.ObjectOf(identifier)
		if object == types.Universe.Lookup("nil") {
			return expression, true, true, position, wholeRuntime
		}
		if object == nil || seen[object] {
			return expression, false, false, position, wholeRuntime
		}
		seen[object] = true
		index := ps6088StableObjectIndex(pass, object, position)
		packageVariable := ps6088PackageVariable(object)
		unsafe := index != nil && packageVariable &&
			(object.Exported() || index.unsafe[object])
		if index != nil && !packageVariable {
			unsafe = ps6088DynamicTypeUnsafeAt(
				pass, index, object, position, wholeRuntime, ignoredStart, ignoredEnd,
			)
		}
		if index == nil || unsafe {
			return expression, false, index == nil, position, wholeRuntime
		}
		if index.zero[object] {
			return expression, true, true, position, wholeRuntime
		}
		definition, found := index.definitions[object]
		if !found {
			return expression, false, false, position, wholeRuntime
		}
		expression = definition
		position = definition.Pos()
		wholeRuntime = index.definitionMayRepeat[object]
	}
}

func ps6088DynamicTypeUnsafeAt(
	pass *analysis.Pass,
	index *ps6088LocalDomainIndex,
	object types.Object,
	position token.Pos,
	wholeRuntime bool,
	ignoredStart token.Pos,
	ignoredEnd token.Pos,
) bool {
	query := ps6088DynamicTypeQuery{
		object, position, wholeRuntime, ignoredStart, ignoredEnd,
	}
	if cached, found := index.dynamicTypeQueries.Load(query); found {
		return cached.(bool)
	}
	writes := index.dynamicTypeWrites[object]
	flow := ps6088DynamicTypeFlowAt(index.flows, position)
	if len(writes) == 0 {
		index.dynamicTypeQueries.Store(query, false)
		return false
	}
	if flow == nil {
		index.dynamicTypeQueries.Store(query, true)
		return true
	}
	target := ps6088BlockAtPosition(flow.blocksAt, position)
	if target == nil {
		index.dynamicTypeQueries.Store(query, true)
		return true
	}
	flowWrites := ps6088DynamicTypeWritesForFlow(pass, index, object, flow)
	targetComponent := flow.component[target]
	targetCycles := flow.cyclic[targetComponent]
	targetFacts := ps6088FactsAtPosition(index.controlFacts, position)
	unsafe := flowWrites.external && !ignoredStart.IsValid()
	if !unsafe && !ignoredStart.IsValid() && !flowWrites.detailed &&
		(!flowWrites.controlled || len(targetFacts) == 0) {
		entryReach := flow.reach[flow.entryComponent]
		fromTarget := flow.reach[targetComponent]
		canReachTarget := flow.reverseReach[targetComponent]
		for word, writesInComponents := range flowWrites.components {
			relevant := writesInComponents & entryReach[word] & canReachTarget[word]
			if wholeRuntime {
				relevant |= writesInComponents & fromTarget[word] & canReachTarget[word]
			}
			targetWord := targetComponent / 64
			if word == targetWord {
				relevant &^= uint64(1) << (targetComponent % 64)
			}
			if relevant != 0 {
				unsafe = true
				break
			}
		}
		if !unsafe && ps6088HasBit(flowWrites.components, targetComponent) {
			for block, executions := range flowWrites.byBlock {
				if flow.component[block] != targetComponent {
					continue
				}
				if block != target {
					unsafe = true
					break
				}
				for _, execution := range executions {
					if execution < position || wholeRuntime && execution >= position && targetCycles {
						unsafe = true
						break
					}
				}
			}
		}
		index.dynamicTypeQueries.Store(query, unsafe)
		return unsafe
	}
	for _, write := range writes {
		if unsafe {
			break
		}
		if ignoredStart.IsValid() && ignoredStart <= write.execution && write.execution <= ignoredEnd {
			continue
		}
		if ps6088ControlFactsExclude(index, write.node, position) {
			continue
		}
		block := ps6088BlockAtPosition(flow.blocksAt, write.node)
		if block == nil {
			writeFlow := ps6088DynamicTypeFlowAt(index.flows, write.node)
			if writeFlow != nil && ps6088DeferredFlowRunsAfterTarget(
				pass, index.flows, writeFlow, flow,
			) {
				continue
			}
			if writeFlow != nil && !writeFlow.externalWritesRelevant {
				continue
			}
			if writeFlow != nil {
				if relevant, correlated := ps6088DynamicWriteRelevantAcrossFlows(
					index.flows, flow, writeFlow, write,
				); correlated && !relevant {
					continue
				}
			}
			if writeFlow == nil || writeFlow.externalWritesRelevant {
				unsafe = true
				break
			}
			continue
		}
		component := flow.component[block]
		if block == target {
			if write.execution < position ||
				wholeRuntime && write.execution >= position && targetCycles {
				unsafe = true
				break
			}
			continue
		}
		canReach := ps6088HasBit(flow.reach[component], targetComponent)
		entryReachable := ps6088HasBit(flow.reach[flow.entryComponent], component)
		fromTarget := ps6088HasBit(flow.reach[targetComponent], component)
		if entryReachable && canReach || wholeRuntime && fromTarget && canReach {
			unsafe = true
			break
		}
	}
	index.dynamicTypeQueries.Store(query, unsafe)
	return unsafe
}

func ps6088DeferredFlowRunsAfterTarget(
	pass *analysis.Pass,
	flows []ps6088DynamicTypeFlow,
	deferredFlow, targetFlow *ps6088DynamicTypeFlow,
) bool {
	if !deferredFlow.deferredInvocation {
		return false
	}
	parent := ps6088DynamicTypeFlowForBody(flows, deferredFlow.parentBody)
	if targetFlow.deferredInvocation && deferredFlow.parentBody == targetFlow.parentBody &&
		deferredFlow.invocation.IsValid() && targetFlow.invocation.IsValid() {
		return ps6088DeferredSiblingWriterRunsAfter(pass, parent, deferredFlow, targetFlow)
	}
	if targetOwner := ps6088DeferredOwnerInBody(
		flows, targetFlow, deferredFlow.parentBody,
	); targetOwner != nil {
		return ps6088DeferredSiblingWriterRunsAfter(pass, parent, deferredFlow, targetOwner)
	}
	if targetFlow.deferredInvocation {
		targetParent := ps6088DynamicTypeFlowForBody(flows, targetFlow.parentBody)
		if parent != nil && targetParent != nil &&
			(parent.body == targetParent.body ||
				ps6088DynamicTypeFlowIsSynchronousAncestor(flows, targetParent, parent)) {
			// The descendant body's defers complete while its synchronous
			// invocation returns. An ancestor body's defer cannot run until that
			// body itself later unwinds.
			return true
		}
	}
	return parent != nil && (parent.body == targetFlow.body ||
		ps6088DynamicTypeFlowIsSynchronousAncestor(flows, targetFlow, parent))
}

func ps6088DeferredOwnerInBody(
	flows []ps6088DynamicTypeFlow,
	targetFlow *ps6088DynamicTypeFlow,
	body *ast.BlockStmt,
) *ps6088DynamicTypeFlow {
	for current := targetFlow; current != nil; {
		if current.deferredInvocation && current.parentBody == body {
			return current
		}
		if !current.deferredInvocation && !current.synchronousInvocation {
			return nil
		}
		parent := ps6088DynamicTypeFlowForBody(flows, current.parentBody)
		if parent == nil {
			return nil
		}
		current = parent
	}
	return nil
}

func ps6088DeferredSiblingWriterRunsAfter(
	pass *analysis.Pass,
	parent, writer, target *ps6088DynamicTypeFlow,
) bool {
	if parent == nil || !writer.invocation.IsValid() || !target.invocation.IsValid() {
		return false
	}
	// Defers registered in the same execution body run in LIFO order. Ignore
	// the writer only when the parent's CFG proves it cannot be registered
	// after the target; source positions alone are unsound in the presence of
	// goto and loop backedges.
	return !ps6088FlowPositionCanReach(parent, target.invocation, writer.invocation) ||
		writer.invocation < target.invocation && ps6088PositionsShareOnlySingleTripLoops(
			pass, parent.body, writer.invocation, target.invocation,
		)
}

func ps6088PositionsShareOnlySingleTripLoops(
	pass *analysis.Pass,
	body *ast.BlockStmt,
	left, right token.Pos,
) bool {
	found := false
	safe := true
	ast.Inspect(body, func(node ast.Node) bool {
		if !safe || node == nil {
			return false
		}
		if !astutil.IsLoop(node) {
			return true
		}
		loopBody := astutil.LoopBody(node)
		if loopBody == nil || !(loopBody.Pos() <= left && left < loopBody.End()) ||
			!(loopBody.Pos() <= right && right < loopBody.End()) {
			return true
		}
		found = true
		atMostOne, known := ps6088LoopAtMost(pass, node, 1)
		if !known || !atMostOne {
			safe = false
			return false
		}
		ast.Inspect(loopBody, func(child ast.Node) bool {
			branch, ok := child.(*ast.BranchStmt)
			if ok && branch.Tok == token.GOTO {
				safe = false
				return false
			}
			return safe
		})
		return safe
	})
	return found && safe
}

func ps6088FlowPositionCanReach(
	flow *ps6088DynamicTypeFlow,
	fromPosition, toPosition token.Pos,
) bool {
	from := ps6088BlockAtPosition(flow.blocksAt, fromPosition)
	to := ps6088BlockAtPosition(flow.blocksAt, toPosition)
	if from == nil || to == nil {
		return true
	}
	fromComponent := flow.component[from]
	if from == to {
		return fromPosition < toPosition || flow.cyclic[fromComponent]
	}
	return ps6088HasBit(flow.reach[fromComponent], flow.component[to])
}

func ps6088DynamicWriteRelevantAcrossFlows(
	flows []ps6088DynamicTypeFlow,
	targetFlow, writeFlow *ps6088DynamicTypeFlow,
	write ps6088DynamicTypeWrite,
) (bool, bool) {
	for current := targetFlow; current != nil && current.synchronousInvocation &&
		current.invocation.IsValid(); {
		parent := ps6088DynamicTypeFlowForBody(flows, current.parentBody)
		if parent == nil {
			return true, false
		}
		if parent.body == writeFlow.body {
			invocationBlock := ps6088BlockAtPosition(parent.blocksAt, current.invocation)
			invocationRepeats := invocationBlock != nil &&
				parent.cyclic[parent.component[invocationBlock]]
			return ps6088DynamicWriteRelevantInFlow(
				parent, write, current.invocation, invocationRepeats,
			), true
		}
		current = parent
	}
	return true, false
}

func ps6088DynamicTypeFlowForBody(
	flows []ps6088DynamicTypeFlow,
	body *ast.BlockStmt,
) *ps6088DynamicTypeFlow {
	for index := range flows {
		if flows[index].body == body {
			return &flows[index]
		}
	}
	return nil
}

func ps6088DynamicWriteRelevantInFlow(
	flow *ps6088DynamicTypeFlow,
	write ps6088DynamicTypeWrite,
	targetPosition token.Pos,
	wholeRuntime bool,
) bool {
	target := ps6088BlockAtPosition(flow.blocksAt, targetPosition)
	block := ps6088BlockAtPosition(flow.blocksAt, write.node)
	if target == nil || block == nil {
		return true
	}
	targetComponent := flow.component[target]
	if block == target {
		return write.execution < targetPosition ||
			wholeRuntime && write.execution >= targetPosition && flow.cyclic[targetComponent]
	}
	component := flow.component[block]
	canReach := ps6088HasBit(flow.reach[component], targetComponent)
	return ps6088HasBit(flow.reach[flow.entryComponent], component) && canReach ||
		wholeRuntime && ps6088HasBit(flow.reach[targetComponent], component) && canReach
}

func ps6088DynamicTypeWritesForFlow(
	pass *analysis.Pass,
	index *ps6088LocalDomainIndex,
	object types.Object,
	flow *ps6088DynamicTypeFlow,
) *ps6088DynamicTypeFlowWrites {
	query := ps6088DynamicTypeFlowQuery{object, flow.body}
	if cached, found := index.dynamicTypeFlowData.Load(query); found {
		return cached.(*ps6088DynamicTypeFlowWrites)
	}
	result := &ps6088DynamicTypeFlowWrites{
		components: make([]uint64, (len(flow.cyclic)+63)/64),
		byBlock:    make(map[*cfg.Block][]token.Pos),
	}
	for _, write := range index.dynamicTypeWrites[object] {
		if len(ps6088FactsAtPosition(index.controlFacts, write.node)) > 0 {
			result.controlled = true
		}
		block := ps6088BlockAtPosition(flow.blocksAt, write.node)
		if block == nil {
			writeFlow := ps6088DynamicTypeFlowAt(index.flows, write.node)
			if writeFlow != nil && ps6088DeferredFlowRunsAfterTarget(
				pass, index.flows, writeFlow, flow,
			) {
				result.detailed = true
				continue
			}
			if writeFlow != nil && ps6088DynamicTypeFlowIsSynchronousAncestor(
				index.flows, flow, writeFlow,
			) {
				// A synchronous immediate literal observes its parent body at the
				// invocation point. Keep these writes in the target-specific slow
				// path so writes after a one-shot invocation do not invalidate the
				// literal's interface state.
				result.detailed = true
				continue
			}
			if writeFlow == nil || writeFlow.externalWritesRelevant {
				result.external = true
			}
			continue
		}
		ps6088SetBit(result.components, flow.component[block])
		result.byBlock[block] = append(result.byBlock[block], write.execution)
	}
	actual, _ := index.dynamicTypeFlowData.LoadOrStore(query, result)
	return actual.(*ps6088DynamicTypeFlowWrites)
}

func ps6088DynamicTypeFlowIsSynchronousAncestor(
	flows []ps6088DynamicTypeFlow,
	targetFlow, possibleAncestor *ps6088DynamicTypeFlow,
) bool {
	for current := targetFlow; current != nil && current.synchronousInvocation &&
		current.invocation.IsValid(); {
		parent := ps6088DynamicTypeFlowForBody(flows, current.parentBody)
		if parent == nil {
			return false
		}
		if parent.body == possibleAncestor.body {
			return true
		}
		current = parent
	}
	return false
}

func ps6088DynamicTypeFlowAt(
	flows []ps6088DynamicTypeFlow,
	position token.Pos,
) *ps6088DynamicTypeFlow {
	var result *ps6088DynamicTypeFlow
	for index := range flows {
		flow := &flows[index]
		if flow.body.Pos() <= position && position < flow.body.End() &&
			(result == nil || flow.body.End()-flow.body.Pos() < result.body.End()-result.body.Pos()) {
			result = flow
		}
	}
	return result
}

func ps6088BlockAtPosition(blocks []ps6088PositionBlock, position token.Pos) *cfg.Block {
	index, found := slices.BinarySearchFunc(blocks, position, func(
		block ps6088PositionBlock,
		position token.Pos,
	) int {
		switch {
		case block.position < position:
			return -1
		case block.position > position:
			return 1
		default:
			return 0
		}
	})
	if !found {
		return nil
	}
	return blocks[index].block
}

func ps6088ControlFactsExclude(
	index *ps6088LocalDomainIndex,
	writePosition token.Pos,
	targetPosition token.Pos,
) bool {
	writes := ps6088FactsAtPosition(index.controlFacts, writePosition)
	targets := ps6088FactsAtPosition(index.controlFacts, targetPosition)
	for _, write := range writes {
		if write.guard == nil || ps6088PackageVariable(write.guard) ||
			len(index.dynamicTypeWrites[write.guard]) > 0 {
			continue
		}
		for _, target := range targets {
			if target.guard != write.guard || target.branch == write.branch {
				continue
			}
			if write.control == target.control && write.control != nil {
				statement, _ := write.control.(*ast.SwitchStmt)
				if ps6088SwitchClauseFallsTo(statement, write.clause, target.clause) {
					continue
				}
				return true
			}
			if write.control == nil && target.control == nil ||
				strings.HasPrefix(write.branch, "case:") && strings.HasPrefix(target.branch, "case:") {
				return true
			}
		}
	}
	return false
}

func ps6088SwitchClauseFallsTo(
	statement *ast.SwitchStmt,
	from *ast.CaseClause,
	to *ast.CaseClause,
) bool {
	if statement == nil || from == nil || to == nil || from == to {
		return false
	}
	found := false
	for _, item := range statement.Body.List {
		clause, ok := item.(*ast.CaseClause)
		if !ok {
			return false
		}
		if clause == from {
			found = true
		}
		if !found {
			continue
		}
		if clause == to {
			return true
		}
		if !ps6088FallsThrough(clause) {
			return false
		}
	}
	return false
}

func ps6088FactsAtPosition(
	positions []ps6088PositionFacts,
	position token.Pos,
) []ps6088ControlFact {
	index, found := slices.BinarySearchFunc(positions, position, func(
		facts ps6088PositionFacts,
		position token.Pos,
	) int {
		switch {
		case facts.position < position:
			return -1
		case facts.position > position:
			return 1
		default:
			return 0
		}
	})
	if !found {
		return nil
	}
	return positions[index].facts
}

func ps6088StaticCFGSuccessors(pass *analysis.Pass, block *cfg.Block) []*cfg.Block {
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

func ps6088ReachableBlocks(
	start *cfg.Block,
	edges map[*cfg.Block][]*cfg.Block,
) map[*cfg.Block]bool {
	reachable := make(map[*cfg.Block]bool)
	queue := []*cfg.Block{start}
	for len(queue) > 0 {
		block := queue[0]
		queue = queue[1:]
		if reachable[block] {
			continue
		}
		reachable[block] = true
		queue = append(queue, edges[block]...)
	}
	return reachable
}

func ps6088ObjectUnsafeBefore(
	index *ps6088LocalDomainIndex,
	object types.Object,
	position token.Pos,
) bool {
	for _, unsafePosition := range index.unsafePositions[object] {
		if unsafePosition < position {
			return true
		}
	}
	return false
}

func ps6088StableObjectIndex(
	pass *analysis.Pass,
	object types.Object,
	position token.Pos,
) *ps6088LocalDomainIndex {
	if ps6088PackageVariable(object) {
		return ps6088PackageDomainIndex(pass)
	}
	fn := ps6088LocalObjectOwner(pass, object)
	if fn != nil && fn.Body.Pos() <= position && position < fn.Body.End() {
		return ps6088FunctionLocalDomainIndex(pass, fn)
	}
	return nil
}

func ps6088LocalObjectOwner(pass *analysis.Pass, object types.Object) *ast.FuncDecl {
	value, ok := ps6088LocalDomains.Load(pass)
	if !ok {
		value, _ = ps6088LocalDomains.LoadOrStore(pass, new(ps6088LocalDomainPassIndex))
	}
	passIndex := value.(*ps6088LocalDomainPassIndex)
	passIndex.ownerOnce.Do(func() {
		passIndex.owners = make(map[types.Object]*ast.FuncDecl)
		for _, file := range pass.Files {
			for _, declaration := range file.Decls {
				fn, ok := declaration.(*ast.FuncDecl)
				if !ok || fn.Body == nil {
					continue
				}
				ast.Inspect(fn.Body, func(node ast.Node) bool {
					identifier, ok := node.(*ast.Ident)
					if ok {
						if local := pass.TypesInfo.Defs[identifier]; local != nil {
							passIndex.owners[local] = fn
						}
					}
					return true
				})
				if fn.Type.Results != nil {
					for _, field := range fn.Type.Results.List {
						for _, name := range field.Names {
							if local := pass.TypesInfo.Defs[name]; local != nil {
								passIndex.owners[local] = fn
							}
						}
					}
				}
			}
		}
	})
	return passIndex.owners[object]
}

func ps6088PackageDomainIndex(pass *analysis.Pass) *ps6088LocalDomainIndex {
	value, ok := ps6088LocalDomains.Load(pass)
	if !ok {
		value, _ = ps6088LocalDomains.LoadOrStore(pass, new(ps6088LocalDomainPassIndex))
	}
	passIndex := value.(*ps6088LocalDomainPassIndex)
	passIndex.packageOnce.Do(func() {
		index := &ps6088LocalDomainIndex{
			definitions:     make(map[types.Object]ast.Expr),
			zero:            make(map[types.Object]bool),
			unsafe:          make(map[types.Object]bool),
			unsafePositions: make(map[types.Object][]token.Pos),
		}
		markObject := func(object types.Object, position token.Pos) {
			if object == nil {
				return
			}
			index.unsafe[object] = true
			index.unsafePositions[object] = append(index.unsafePositions[object], position)
		}
		mark := func(expression ast.Expr) {
			if expression == nil {
				return
			}
			ast.Inspect(expression, func(node ast.Node) bool {
				identifier, ok := node.(*ast.Ident)
				if ok {
					object := pass.TypesInfo.ObjectOf(identifier)
					if ps6088PackageVariable(object) {
						markObject(object, identifier.Pos())
					}
				}
				return true
			})
		}
		markReferences := func(expression ast.Expr) {
			if expression == nil {
				return
			}
			ast.Inspect(expression, func(node ast.Node) bool {
				identifier, ok := node.(*ast.Ident)
				if ok {
					object := pass.TypesInfo.ObjectOf(identifier)
					if ps6088PackageVariable(object) &&
						ps6088ContainsReference(object.Type(), make(map[types.Type]bool)) {
						markObject(object, identifier.Pos())
					}
				}
				return true
			})
		}
		for _, file := range pass.Files {
			ast.Inspect(file, func(node ast.Node) bool {
				switch item := node.(type) {
				case *ast.ValueSpec:
					if len(item.Names) == len(item.Values) {
						for position, name := range item.Names {
							object := pass.TypesInfo.Defs[name]
							if ps6088PackageVariable(object) {
								index.definitions[object] = item.Values[position]
							}
						}
					} else if len(item.Values) == 0 {
						for _, name := range item.Names {
							object := pass.TypesInfo.Defs[name]
							if ps6088PackageVariable(object) {
								index.zero[object] = true
							}
						}
					}
					for _, expression := range item.Values {
						markReferences(expression)
					}
				case *ast.AssignStmt:
					for _, left := range item.Lhs {
						mark(left)
					}
					for _, right := range item.Rhs {
						markReferences(right)
					}
				case *ast.IncDecStmt:
					mark(item.X)
				case *ast.RangeStmt:
					if item.Tok != token.DEFINE {
						mark(item.Key)
						mark(item.Value)
					}
				case *ast.CallExpr:
					for _, argument := range item.Args {
						markReferences(argument)
					}
				case *ast.ReturnStmt:
					for _, result := range item.Results {
						markReferences(result)
					}
				case *ast.SendStmt:
					markReferences(item.Value)
				case *ast.UnaryExpr:
					if item.Op == token.AND {
						mark(item.X)
					}
				}
				return true
			})
		}
		passIndex.packageData = index
	})
	return passIndex.packageData
}

func ps6088FunctionLocalObject(pass *analysis.Pass, fn *ast.FuncDecl, object types.Object) bool {
	variable, ok := object.(*types.Var)
	if !ok {
		return false
	}
	if variable.Pos() >= fn.Body.Pos() && variable.Pos() < fn.Body.End() {
		return true
	}
	if fn.Type.Results == nil {
		return false
	}
	for _, field := range fn.Type.Results.List {
		for _, name := range field.Names {
			if pass.TypesInfo.Defs[name] == object {
				return true
			}
		}
	}
	return false
}

func ps6088DomainObjectState(
	pass *analysis.Pass,
	fn *ast.FuncDecl,
	domain ast.Expr,
	prefix []ast.Stmt,
	ancestors []ast.Node,
) (map[types.Object]struct{}, bool) {
	formals := make(map[types.Object]struct{})
	for _, fields := range []*ast.FieldList{fn.Recv, fn.Type.Params} {
		if fields == nil {
			continue
		}
		for _, field := range fields.List {
			for _, name := range field.Names {
				if object := pass.TypesInfo.Defs[name]; object != nil {
					formals[object] = struct{}{}
				}
			}
		}
	}
	used := make(map[types.Object]struct{})
	ast.Inspect(domain, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if ok {
			object := pass.TypesInfo.ObjectOf(identifier)
			if _, formal := formals[object]; formal {
				used[object] = struct{}{}
			}
		}
		return true
	})
	stable := make(map[types.Object]struct{}, len(used))
	safe := true
	for object := range used {
		var parameters map[types.Object]struct{}
		if ps6088ContainsReference(object.Type(), make(map[types.Type]bool)) {
			parameters = map[types.Object]struct{}{object: {}}
		}
		controls := &ps6088LoopControls{
			objects: map[types.Object]struct{}{object: {}},
			aliases: make(map[types.Object]struct{}),
			params:  parameters,
		}
		ps6088SeedAliases(pass, prefix, controls)
		written := ps6088EnclosingRangeWritesObject(pass, ancestors, controls) ||
			ps6088StatementsWriteControl(pass, prefix, controls, nil, false)
		if !written {
			stable[object] = struct{}{}
		} else {
			safe = false
		}
	}
	return stable, safe
}

func ps6088EnclosingRangeWritesObject(
	pass *analysis.Pass,
	ancestors []ast.Node,
	controls *ps6088LoopControls,
) bool {
	for _, ancestor := range ancestors {
		rangeLoop, ok := ancestor.(*ast.RangeStmt)
		if !ok || rangeLoop.Tok == token.DEFINE {
			continue
		}
		if ps6088WritesControl(pass, rangeLoop.Key, controls) ||
			ps6088WritesControl(pass, rangeLoop.Value, controls) {
			return true
		}
	}
	return false
}

func ps6088LoopPrefix(loop ast.Node, ancestors []ast.Node) []ast.Stmt {
	var prefix []ast.Stmt
	for _, ancestor := range ancestors {
		switch statement := ancestor.(type) {
		case *ast.IfStmt:
			prefix = ps6088AppendPrefixStatement(prefix, statement.Init)
			prefix = ps6088AppendPrefixExpression(prefix, statement.Cond)
		case *ast.ForStmt:
			prefix = ps6088AppendPrefixStatement(prefix, statement.Init)
			prefix = ps6088AppendPrefixExpression(prefix, statement.Cond)
			prefix = ps6088AppendPrefixStatement(prefix, statement.Post)
		case *ast.RangeStmt:
			prefix = ps6088AppendPrefixExpression(prefix, statement.X)
		case *ast.SwitchStmt:
			prefix = ps6088AppendPrefixStatement(prefix, statement.Init)
			prefix = ps6088AppendPrefixExpression(prefix, statement.Tag)
			for _, item := range statement.Body.List {
				clause, ok := item.(*ast.CaseClause)
				if !ok {
					continue
				}
				for _, expression := range clause.List {
					prefix = ps6088AppendPrefixExpression(prefix, expression)
				}
				prefix = append(prefix, clause.Body...)
			}
		case *ast.TypeSwitchStmt:
			prefix = ps6088AppendPrefixStatement(prefix, statement.Init)
			prefix = ps6088AppendPrefixStatement(prefix, statement.Assign)
			for _, item := range statement.Body.List {
				if clause, ok := item.(*ast.CaseClause); ok {
					prefix = append(prefix, clause.Body...)
				}
			}
		case *ast.SelectStmt:
			for _, item := range statement.Body.List {
				clause, ok := item.(*ast.CommClause)
				if ok {
					prefix = ps6088AppendPrefixStatement(prefix, clause.Comm)
				}
			}
		}
		block, ok := ancestor.(*ast.BlockStmt)
		if !ok {
			continue
		}
		for _, statement := range block.List {
			if statement.End() >= loop.Pos() {
				break
			}
			prefix = append(prefix, statement)
		}
	}
	return prefix
}

func ps6088GotoCrosses(body *ast.BlockStmt, position token.Pos) bool {
	labels := make(map[string]token.Pos)
	var gotos []*ast.BranchStmt
	ast.Inspect(body, func(node ast.Node) bool {
		if _, literal := node.(*ast.FuncLit); literal {
			return false
		}
		switch value := node.(type) {
		case *ast.LabeledStmt:
			labels[value.Label.Name] = value.Pos()
		case *ast.BranchStmt:
			if value.Tok == token.GOTO && value.Label != nil {
				gotos = append(gotos, value)
			}
		}
		return true
	})
	for _, branch := range gotos {
		target, ok := labels[branch.Label.Name]
		if !ok || (branch.Pos() < position && target > position) ||
			(branch.Pos() > position && target < position) {
			return true
		}
	}
	return false
}

func ps6088AppendPrefixStatement(prefix []ast.Stmt, statement ast.Stmt) []ast.Stmt {
	if statement == nil {
		return prefix
	}
	return append(prefix, statement)
}

func ps6088AppendPrefixExpression(prefix []ast.Stmt, expression ast.Expr) []ast.Stmt {
	if expression == nil {
		return prefix
	}
	return append(prefix, &ast.ExprStmt{X: expression})
}

func ps6088CallbackParameter(
	pass *analysis.Pass,
	fn *ast.FuncDecl,
	loop ast.Node,
	launch *ast.GoStmt,
	waitObject *types.Var,
) string {
	worker, ok := ps6086WorkObject(pass, loop, launch, waitObject)
	if !ok {
		return ""
	}
	for _, field := range fn.Type.Params.List {
		for _, name := range field.Names {
			if pass.TypesInfo.Defs[name] == worker.object {
				return name.Name
			}
		}
	}
	return ""
}

func ps6088CallerEvidence(
	pass *analysis.Pass,
	candidates map[*types.Func][]ps6088Candidate,
) map[*ast.GoStmt]ps6088CallEvidence {
	result := make(map[*ast.GoStmt]ps6088CallEvidence)
	parameters := ps6088Parameters(pass)
	ps6088PackageInitializerEvidence(pass, candidates, result)
	for _, file := range pass.Files {
		if ps6088TestFile(pass, file) {
			continue
		}
		for _, declaration := range file.Decls {
			fn, ok := declaration.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			caller, _ := pass.TypesInfo.Defs[fn.Name].(*types.Func)
			flow := cfg.New(fn.Body, func(call *ast.CallExpr) bool { return !ps6088NonreturnCall(pass, call) })
			literalFlows := make(map[*ast.FuncLit]*cfg.CFG)
			literalInvocations := make(map[*ast.FuncLit]*ast.CallExpr)
			astutil.WithStack(fn.Body, func(node ast.Node, stack []ast.Node) bool {
				if literal, ok := node.(*ast.FuncLit); ok {
					invocation := ps6088ImmediateFuncLit(pass, literal, stack)
					if invocation == nil {
						return false
					}
					literalInvocations[literal] = invocation
					literalFlows[literal] = cfg.New(
						literal.Body,
						func(call *ast.CallExpr) bool { return !ps6088NonreturnCall(pass, call) },
					)
					return true
				}
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				if ps6088UnevaluatedUnsafeBuiltin(pass, call) {
					return false
				}
				callee, _, ok := typedCallee(pass, call.Fun)
				if !ok {
					return true
				}
				candidateFunction := callee.Origin()
				if candidateFunction == caller || len(candidates[candidateFunction]) == 0 {
					return true
				}
				block := ps6079CFGBlockAt(flow, call.Pos())
				if block == nil || !block.Live || !ps6088CFGPositionLive(pass, flow, call.Pos()) ||
					!ps6088CallOperandsReturn(pass, call) ||
					!ps6088StaticPathLive(pass, call, stack) ||
					!ps6088PriorStatementsReturn(pass, call, stack) ||
					!ps6088ExpressionCallReached(pass, call, call, stack) ||
					!ps6088LiteralPathLive(pass, call, stack, literalFlows) ||
					!ps6088DeferredExecutionViable(
						pass, call, stack, flow, literalFlows, literalInvocations,
					) {
					return true
				}
				repeated := ps6088FunctionCallRepeated(
					pass, call, stack, flow, literalFlows, literalInvocations, parameters,
				)
				ps6088RecordCallEvidence(pass, fn, call, callee, candidates[candidateFunction], repeated, result)
				return true
			})
		}
	}
	return result
}

func ps6088PackageInitializerEvidence(
	pass *analysis.Pass,
	candidates map[*types.Func][]ps6088Candidate,
	result map[*ast.GoStmt]ps6088CallEvidence,
) {
	returns := true
	for _, initializer := range pass.TypesInfo.InitOrder {
		if !returns {
			break
		}
		expression := initializer.Rhs
		if expression == nil || ps6088TestPosition(pass, expression.Pos()) {
			continue
		}
		literalFlows := make(map[*ast.FuncLit]*cfg.CFG)
		literalInvocations := make(map[*ast.FuncLit]*ast.CallExpr)
		astutil.WithStack(expression, func(node ast.Node, stack []ast.Node) bool {
			if literal, ok := node.(*ast.FuncLit); ok {
				invocation := ps6088ImmediateFuncLit(pass, literal, stack)
				if invocation == nil {
					return false
				}
				literalInvocations[literal] = invocation
				literalFlows[literal] = cfg.New(
					literal.Body,
					func(call *ast.CallExpr) bool { return !ps6088NonreturnCall(pass, call) },
				)
				return true
			}
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			if ps6088UnevaluatedUnsafeBuiltin(pass, call) {
				return false
			}
			callee, _, ok := typedCallee(pass, call.Fun)
			if !ok {
				return true
			}
			candidateFunction := callee.Origin()
			if len(candidates[candidateFunction]) == 0 {
				return true
			}
			if !ps6088StaticPathLive(pass, node, stack) ||
				!ps6088PriorStatementsReturn(pass, call, stack) ||
				!ps6088ExpressionCallReached(pass, expression, call, stack) ||
				!ps6088CallOperandsReturn(pass, call) ||
				!ps6088LiteralPathLive(pass, call, stack, literalFlows) ||
				!ps6088DeferredExecutionViable(
					pass, call, stack, nil, literalFlows, literalInvocations,
				) {
				return true
			}
			ps6088RecordCallEvidence(pass, nil, call, callee, candidates[candidateFunction], false, result)
			return true
		})
		returns = ps6088InitializerReturnsNormally(pass, initializer)
	}
}

func ps6088InitializerReturnsNormally(pass *analysis.Pass, initializer *types.Initializer) bool {
	assertion, commaOK := ps2110Unparen(initializer.Rhs).(*ast.TypeAssertExpr)
	if commaOK && assertion.Type != nil && len(initializer.Lhs) == 2 {
		return ps6088ExpressionReturnsNormally(pass, assertion.X)
	}
	return ps6088ExpressionReturnsNormally(pass, initializer.Rhs)
}

func ps6088TestPosition(pass *analysis.Pass, position token.Pos) bool {
	for _, file := range pass.Files {
		if file.Pos() <= position && position < file.End() {
			return ps6088TestFile(pass, file)
		}
	}
	return false
}

func ps6088RecordCallEvidence(
	pass *analysis.Pass,
	caller *ast.FuncDecl,
	call *ast.CallExpr,
	callee *types.Func,
	candidates []ps6088Candidate,
	repeated bool,
	result map[*ast.GoStmt]ps6088CallEvidence,
) {
	actuals := ps6088CallDomainActuals(pass, caller, call, callee)
	if !ps6088CallReceiverReturns(callee, actuals) {
		return
	}
	for _, candidate := range candidates {
		if !candidate.domainEvidenceSafe {
			continue
		}
		if ps6088CandidateDomainEmpty(pass, candidate, actuals) {
			continue
		}
		evidence := result[candidate.launch]
		evidence.count++
		evidence.loop = evidence.loop || repeated
		result[candidate.launch] = evidence
	}
}

func ps6088CallReceiverReturns(
	callee *types.Func,
	actuals map[types.Object]ps6088DomainActual,
) bool {
	signature, ok := callee.Origin().Type().(*types.Signature)
	if !ok || signature.Recv() == nil {
		return true
	}
	actual, exists := actuals[signature.Recv()]
	return !exists || !actual.noExecution && !actual.evidenceUnsafe
}

type ps6088DomainActual struct {
	expression     ast.Expr
	typeOf         types.Type
	fieldPath      []int
	atMostZero     bool
	known          bool
	zero           bool
	noExecution    bool
	evidenceUnsafe bool
	elements       []ps6088DomainActual
}

func ps6088CandidateDomainEmpty(
	pass *analysis.Pass,
	candidate ps6088Candidate,
	actuals map[types.Object]ps6088DomainActual,
) bool {
	for object := range candidate.stableDomainObjects {
		if actual, exists := actuals[object]; exists && actual.evidenceUnsafe {
			return true
		}
	}
	if !ps6088DomainEvaluationReturns(
		pass, candidate.evaluationDomain, candidate.stableDomainObjects, actuals,
		candidate.evaluationTerminal,
	) {
		return true
	}
	if candidate.rangeValueEvaluated && ps6088NilPointerArrayRangeActual(
		pass, candidate.domain, candidate.stableDomainObjects, actuals,
	) {
		return true
	}
	empty, known := ps6088DomainAtMostWithActuals(
		pass, candidate.domain, candidate.stableDomainObjects, actuals,
	)
	return known && empty
}

func ps6088DomainEvaluationReturns(
	pass *analysis.Pass,
	expression ast.Expr,
	stable map[types.Object]struct{},
	actuals map[types.Object]ps6088DomainActual,
	terminalRange bool,
) bool {
	expression = ps2110Unparen(expression)
	if terminalRange && ps6088ArrayOrPointerArray(pass.TypesInfo.TypeOf(expression)) &&
		ps6088ConstantArrayRangeExpression(pass, expression) {
		return true
	}
	switch value := expression.(type) {
	case *ast.Ident, *ast.BasicLit:
		return true
	case *ast.SelectorExpr:
		if !ps6088DomainEvaluationReturns(pass, value.X, stable, actuals, false) {
			return false
		}
		if ps6088NilPointerOrSliceExpression(pass, value.X, pass.TypesInfo.TypeOf(value.X)) {
			return false
		}
		actual, ok := ps6088DomainExpressionActual(pass, value.X, stable, actuals, false)
		return !ok || !ps6088DomainActualNilPointer(pass, actual)
	case *ast.IndexExpr:
		if !ps6088DomainEvaluationReturns(pass, value.X, stable, actuals, false) ||
			!ps6088DomainEvaluationReturns(pass, value.Index, stable, actuals, false) {
			return false
		}
		if ps6088IndexDefinitelyOutOfBounds(pass, value.X, value.Index) {
			return false
		}
		actual, ok := ps6088DomainExpressionActual(pass, value, stable, actuals, false)
		return !ok || !actual.noExecution
	case *ast.IndexListExpr:
		if !ps6088DomainEvaluationReturns(pass, value.X, stable, actuals, false) {
			return false
		}
		for _, index := range value.Indices {
			if !ps6088DomainEvaluationReturns(pass, index, stable, actuals, false) {
				return false
			}
		}
		return true
	case *ast.StarExpr:
		if !ps6088DomainEvaluationReturns(pass, value.X, stable, actuals, false) {
			return false
		}
		if ps6088NilPointerOrSliceExpression(pass, value.X, pass.TypesInfo.TypeOf(value.X)) {
			return false
		}
		actual, ok := ps6088DomainExpressionActual(pass, value.X, stable, actuals, false)
		return !ok || !ps6088DomainActualNilPointer(pass, actual)
	case *ast.CallExpr:
		if len(value.Args) == 1 && !value.Ellipsis.IsValid() &&
			(typedBuiltinName(pass, value.Fun, "len") || typedBuiltinName(pass, value.Fun, "cap")) &&
			ps6088ArrayOrPointerArray(pass.TypesInfo.TypeOf(value.Args[0])) &&
			ps6088ConstantArrayRangeExpression(pass, value.Args[0]) {
			return true
		}
		for _, argument := range value.Args {
			if !ps6088DomainEvaluationReturns(pass, argument, stable, actuals, false) {
				return false
			}
		}
		if targetLength, sliceConversion := ps6088SliceToArrayConversion(pass, value); sliceConversion {
			actualLength, known := ps6088ActualSliceLength(pass, value.Args[0], stable, actuals)
			return known && actualLength >= targetLength
		}
		return true
	}
	return false
}

func ps6088ConstantArrayRangeExpression(pass *analysis.Pass, expression ast.Expr) bool {
	safe := true
	ast.Inspect(expression, func(node ast.Node) bool {
		if !safe || node == nil {
			return false
		}
		switch value := node.(type) {
		case *ast.UnaryExpr:
			if value.Op == token.ARROW {
				safe = false
				return false
			}
		case *ast.CallExpr:
			if pass.TypesInfo.Types[value.Fun].IsType() || pass.TypesInfo.Types[value].Value != nil {
				return true
			}
			safe = false
			return false
		}
		return true
	})
	return safe
}

func ps6088IndexDefinitelyOutOfBounds(
	pass *analysis.Pass,
	container ast.Expr,
	index ast.Expr,
) bool {
	typeOf := pass.TypesInfo.TypeOf(container)
	if typeOf == nil {
		return false
	}
	typeOf = types.Unalias(typeOf)
	if pointer, ok := typeOf.Underlying().(*types.Pointer); ok {
		typeOf = types.Unalias(pointer.Elem())
	}
	array, ok := typeOf.Underlying().(*types.Array)
	if !ok || array.Len() > 1 {
		return false
	}
	found := false
	ast.Inspect(index, func(node ast.Node) bool {
		if found || node == nil {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		callee, _, ok := typedCallee(pass, call.Fun)
		found = ok && callee.Pkg() != nil && callee.Pkg().Path() == "runtime" &&
			callee.Name() == "GOMAXPROCS"
		return !found
	})
	return found
}

func ps6088SliceToArrayConversion(pass *analysis.Pass, call *ast.CallExpr) (int64, bool) {
	if len(call.Args) != 1 || call.Ellipsis.IsValid() || !pass.TypesInfo.Types[call.Fun].IsType() {
		return 0, false
	}
	from := pass.TypesInfo.TypeOf(call.Args[0])
	to := pass.TypesInfo.TypeOf(call)
	if from == nil || to == nil {
		return 0, false
	}
	if _, slice := types.Unalias(from).Underlying().(*types.Slice); !slice {
		return 0, false
	}
	target := types.Unalias(to)
	if pointer, ok := target.Underlying().(*types.Pointer); ok {
		target = types.Unalias(pointer.Elem())
	}
	array, ok := target.Underlying().(*types.Array)
	if !ok {
		return 0, false
	}
	return array.Len(), true
}

func ps6088ActualSliceLength(
	pass *analysis.Pass,
	expression ast.Expr,
	stable map[types.Object]struct{},
	actuals map[types.Object]ps6088DomainActual,
) (int64, bool) {
	actual, substituted := ps6088DomainExpressionActual(pass, expression, stable, actuals, true)
	if substituted {
		if actual.known {
			if actual.atMostZero {
				return 0, true
			}
			return 0, false
		}
		resolved, typeOf, zero, ok := ps6088DomainActualExpression(pass, actual)
		if !ok || typeOf == nil {
			return 0, false
		}
		if _, slice := types.Unalias(typeOf).Underlying().(*types.Slice); !slice {
			return 0, false
		}
		if zero || ps6088NilRangeExpression(pass, resolved, typeOf) {
			return 0, true
		}
		expression = resolved
	}
	if typeOf := pass.TypesInfo.TypeOf(expression); ps6088NilRangeExpression(pass, expression, typeOf) {
		return 0, true
	}
	if length, known := ps6088MadeCollectionLength(pass, expression); known {
		return length, true
	}
	literal := ps6088CompositeExpression(pass, expression)
	if literal == nil {
		return 0, false
	}
	return ps6088CompositeLength(pass, literal)
}

func ps6088ArrayOrPointerArray(value types.Type) bool {
	if value == nil {
		return false
	}
	value = types.Unalias(value)
	if pointer, ok := value.Underlying().(*types.Pointer); ok {
		value = types.Unalias(pointer.Elem())
	}
	_, ok := value.Underlying().(*types.Array)
	return ok
}

func ps6088DomainActualNilPointer(pass *analysis.Pass, actual ps6088DomainActual) bool {
	if actual.known {
		return actual.noExecution
	}
	expression, typeOf, zero, ok := ps6088DomainActualExpression(pass, actual)
	if !ok || typeOf == nil {
		return false
	}
	if _, pointer := types.Unalias(typeOf).Underlying().(*types.Pointer); !pointer {
		return false
	}
	return zero || ps6088NilPointerOrSliceExpression(pass, expression, typeOf)
}

func ps6088LoopEvaluationDomain(pass *analysis.Pass, loop ast.Node, normalized ast.Expr) ast.Expr {
	if rangeLoop, ok := loop.(*ast.RangeStmt); ok {
		return rangeLoop.X
	}
	if forLoop, ok := loop.(*ast.ForStmt); ok {
		if domain, found := ps6088ForLoopRawDomain(pass, forLoop); found {
			return domain
		}
	}
	return normalized
}

func ps6088NilPointerArrayRangeActual(
	pass *analysis.Pass,
	expression ast.Expr,
	stable map[types.Object]struct{},
	actuals map[types.Object]ps6088DomainActual,
) bool {
	expression = ps2110Unparen(expression)
	if dereference, ok := expression.(*ast.StarExpr); ok {
		expression = dereference.X
	}
	if typeOf := pass.TypesInfo.TypeOf(expression); ps6088ArrayOrPointerArray(typeOf) &&
		ps6088NilPointerOrSliceExpression(pass, expression, typeOf) {
		return true
	}
	actual, ok := ps6088DomainExpressionActual(pass, expression, stable, actuals, false)
	if !ok || actual.known {
		return false
	}
	actualExpression, actualType, zero, ok := ps6088DomainActualExpression(pass, actual)
	if !ok || actualType == nil {
		return false
	}
	pointer, ok := types.Unalias(actualType).Underlying().(*types.Pointer)
	if !ok {
		return false
	}
	if _, array := types.Unalias(pointer.Elem()).Underlying().(*types.Array); !array {
		return false
	}
	return zero || ps6088NilPointerOrSliceExpression(pass, actualExpression, actualType)
}

func ps6088CallDomainActuals(
	pass *analysis.Pass,
	caller *ast.FuncDecl,
	call *ast.CallExpr,
	callee *types.Func,
) map[types.Object]ps6088DomainActual {
	actuals := make(map[types.Object]ps6088DomainActual)
	origin, ok := callee.Origin().Type().(*types.Signature)
	if !ok {
		return actuals
	}
	callSignature, _ := pass.TypesInfo.TypeOf(call.Fun).(*types.Signature)
	tupleArity := -1
	if len(call.Args) == 1 && !call.Ellipsis.IsValid() {
		if tuple, ok := pass.TypesInfo.TypeOf(call.Args[0]).(*types.Tuple); ok {
			tupleArity = tuple.Len()
		}
	}
	selector := ps6088CallSelector(call.Fun)
	selection := pass.TypesInfo.Selections[selector]
	offset := 0
	if origin.Recv() != nil && selection != nil {
		actual := ps6088DomainActual{}
		switch selection.Kind() {
		case types.MethodExpr:
			offset = 1
			if len(call.Args) > 0 && tupleArity < 0 {
				actual.expression = call.Args[0]
				if callSignature != nil && callSignature.Params().Len() > 0 {
					actual.typeOf = callSignature.Params().At(0).Type()
				}
				actual = ps6088CallerDomainActual(pass, caller, actual.expression, actual.typeOf)
			}
		case types.MethodVal:
			actual = ps6088CallerDomainActual(
				pass, caller, selector.X, pass.TypesInfo.TypeOf(selector.X),
			)
		}
		if actual.expression != nil {
			path := selection.Index()
			if len(path) > 1 {
				actual.fieldPath = slices.Clone(path[:len(path)-1])
			}
			if adjusted, ok := ps6088ReceiverActual(pass, actual, origin.Recv().Type(), selection); ok {
				actuals[origin.Recv()] = adjusted
			}
		}
	}
	for index := range origin.Params().Len() {
		argumentIndex := offset + index
		actual := ps6088DomainActual{}
		if callSignature != nil && argumentIndex < callSignature.Params().Len() {
			actual.typeOf = callSignature.Params().At(argumentIndex).Type()
		}
		variadic := origin.Variadic() && index == origin.Params().Len()-1
		if variadic && !call.Ellipsis.IsValid() {
			argumentCount := len(call.Args)
			if tupleArity >= 0 {
				argumentCount = tupleArity
			}
			if argumentCount == argumentIndex {
				actual.known = true
				actual.atMostZero = true
			} else if tupleArity < 0 {
				for _, argument := range call.Args[argumentIndex:] {
					element := ps6088CallerDomainActual(
						pass, caller, argument, pass.TypesInfo.TypeOf(argument),
					)
					actual.elements = append(actual.elements, element)
					actual.evidenceUnsafe = actual.evidenceUnsafe || element.evidenceUnsafe
				}
			}
			actuals[origin.Params().At(index)] = actual
			continue
		}
		if tupleArity < 0 && argumentIndex < len(call.Args) {
			actual = ps6088CallerDomainActual(pass, caller, call.Args[argumentIndex], actual.typeOf)
			actuals[origin.Params().At(index)] = actual
		}
	}
	return actuals
}

func ps6088CallerDomainActual(
	pass *analysis.Pass,
	caller *ast.FuncDecl,
	expression ast.Expr,
	typeOf types.Type,
) ps6088DomainActual {
	actual := ps6088DomainActual{expression: expression, typeOf: typeOf}
	if caller == nil {
		actual.evidenceUnsafe = ps6088ExpressionUsesNonpackageVariable(pass, expression)
		return actual
	}
	if !ps6088ExpressionUsesFunctionLocal(pass, caller, expression) {
		return actual
	}
	resolved, safe := ps6088ResolveImmutableLocalDomain(pass, caller, expression)
	if !safe {
		actual.evidenceUnsafe = true
		return actual
	}
	actual.expression = resolved
	return actual
}

func ps6088ExpressionUsesNonpackageVariable(pass *analysis.Pass, expression ast.Expr) bool {
	found := false
	ast.Inspect(expression, func(node ast.Node) bool {
		if found {
			return false
		}
		identifier, ok := node.(*ast.Ident)
		variable, variableOK := pass.TypesInfo.ObjectOf(identifier).(*types.Var)
		found = ok && variableOK && variable.Parent() != nil && variable.Parent() != pass.Pkg.Scope()
		return !found
	})
	return found
}

func ps6088ReceiverActual(
	pass *analysis.Pass,
	actual ps6088DomainActual,
	receiverType types.Type,
	selection *types.Selection,
) (ps6088DomainActual, bool) {
	if actual.evidenceUnsafe {
		return actual, true
	}
	selectedType := ps6088FieldPathType(actual.typeOf, actual.fieldPath)
	if selectedType == nil {
		return ps6088DomainActual{}, false
	}
	if ps6088SameReceiverType(selectedType, receiverType) {
		return actual, true
	}
	pointer, pointerActual := types.Unalias(selectedType).Underlying().(*types.Pointer)
	if pointerActual && selection.Indirect() && ps6088SameReceiverType(pointer.Elem(), receiverType) {
		return ps6088ImplicitReceiverDereference(pass, actual, receiverType)
	}
	if receiverPointer, ok := types.Unalias(receiverType).Underlying().(*types.Pointer); ok &&
		ps6088SameReceiverType(receiverPointer.Elem(), selectedType) {
		// The language takes the address of an addressable value for a pointer
		// receiver. Keeping the value expression is sufficient for field paths;
		// direct pointer domains remain conservatively non-empty/unknown.
		return actual, true
	}
	return ps6088DomainActual{}, false
}

func ps6088SameReceiverType(left, right types.Type) bool {
	if types.Identical(left, right) {
		return true
	}
	leftPointer, leftPointerOK := types.Unalias(left).Underlying().(*types.Pointer)
	rightPointer, rightPointerOK := types.Unalias(right).Underlying().(*types.Pointer)
	if leftPointerOK || rightPointerOK {
		return leftPointerOK && rightPointerOK &&
			ps6088SameReceiverType(leftPointer.Elem(), rightPointer.Elem())
	}
	leftNamed, leftOK := types.Unalias(left).(*types.Named)
	rightNamed, rightOK := types.Unalias(right).(*types.Named)
	return leftOK && rightOK && leftNamed.Origin().Obj() == rightNamed.Origin().Obj()
}

func ps6088ImplicitReceiverDereference(
	pass *analysis.Pass,
	actual ps6088DomainActual,
	receiverType types.Type,
) (ps6088DomainActual, bool) {
	expression, typeOf, zero, ok := ps6088DomainActualExpression(pass, actual)
	if !ok || typeOf == nil {
		return ps6088DomainActual{}, false
	}
	if zero || ps6088NilPointerOrSliceExpression(pass, expression, typeOf) {
		return ps6088NoExecutionDomainActual(), true
	}
	if call, ok := ps2110Unparen(expression).(*ast.CallExpr); ok &&
		typedBuiltinName(pass, call.Fun, "new") {
		return ps6088DomainActual{typeOf: receiverType, zero: true}, true
	}
	if address, ok := ps2110Unparen(expression).(*ast.UnaryExpr); ok && address.Op == token.AND {
		return ps6088DomainActual{expression: address.X, typeOf: receiverType}, true
	}
	return ps6088DomainActual{}, false
}

func ps6088CallSelector(expression ast.Expr) *ast.SelectorExpr {
	expression = ps2110Unparen(expression)
	switch value := expression.(type) {
	case *ast.IndexExpr:
		expression = ps2110Unparen(value.X)
	case *ast.IndexListExpr:
		expression = ps2110Unparen(value.X)
	}
	selector, _ := expression.(*ast.SelectorExpr)
	return selector
}

func ps6088DomainAtMostWithActuals(
	pass *analysis.Pass,
	expression ast.Expr,
	stable map[types.Object]struct{},
	actuals map[types.Object]ps6088DomainActual,
) (bool, bool) {
	expression = ps2110Unparen(expression)
	if actual, substitutable := ps6088DomainExpressionActual(pass, expression, stable, actuals, true); substitutable {
		return ps6088DomainActualAtMost(pass, actual)
	}
	if identifier, ok := expression.(*ast.Ident); ok {
		object := pass.TypesInfo.ObjectOf(identifier)
		if _, substitutable := stable[object]; substitutable {
			actual, exists := actuals[object]
			if !exists {
				return false, false
			}
			return ps6088DomainActualAtMost(pass, actual)
		}
	}
	call, ok := expression.(*ast.CallExpr)
	if !ok {
		return ps6088DomainAtMost(pass, expression, 0)
	}
	if (typedBuiltinName(pass, call.Fun, "len") || typedBuiltinName(pass, call.Fun, "cap")) &&
		len(call.Args) == 1 && !call.Ellipsis.IsValid() {
		return ps6088CollectionAtMostWithActuals(
			pass, call.Args[0], stable, actuals, typedBuiltinName(pass, call.Fun, "cap"),
		)
	}
	if (typedBuiltinName(pass, call.Fun, "min") || typedBuiltinName(pass, call.Fun, "max")) &&
		len(call.Args) > 0 && !call.Ellipsis.IsValid() {
		isMin := typedBuiltinName(pass, call.Fun, "min")
		unknown := false
		for _, argument := range call.Args {
			atMost, known := ps6088DomainAtMostWithActuals(pass, argument, stable, actuals)
			if known && atMost == isMin {
				return isMin, true
			}
			unknown = unknown || !known
		}
		if unknown {
			return false, false
		}
		return !isMin, true
	}
	if argument, conversion := ps6088SameBasicConversion(pass, call); conversion {
		return ps6088DomainAtMostWithActuals(pass, argument, stable, actuals)
	}
	if len(call.Args) == 1 && !call.Ellipsis.IsValid() &&
		pass.TypesInfo.Types[call.Fun].IsType() && ps6088RangePreservingTypeParamConversion(pass, call) {
		return ps6088DomainAtMostWithActuals(pass, call.Args[0], stable, actuals)
	}
	return ps6088DomainAtMost(pass, expression, 0)
}

func ps6088DomainExpressionActual(
	pass *analysis.Pass,
	expression ast.Expr,
	stable map[types.Object]struct{},
	actuals map[types.Object]ps6088DomainActual,
	terminal bool,
) (ps6088DomainActual, bool) {
	switch value := ps2110Unparen(expression).(type) {
	case *ast.Ident:
		object := pass.TypesInfo.ObjectOf(value)
		if _, substitutable := stable[object]; !substitutable {
			return ps6088DomainActual{}, false
		}
		actual, ok := actuals[object]
		return actual, ok
	case *ast.SelectorExpr:
		actual, ok := ps6088DomainExpressionActual(pass, value.X, stable, actuals, false)
		if !ok {
			actual = ps6088DirectDomainActual(pass, value.X)
			ok = actual.typeOf != nil
		}
		selection := pass.TypesInfo.Selections[value]
		if !ok || selection == nil || selection.Kind() != types.FieldVal {
			return ps6088DomainActual{}, false
		}
		actual.fieldPath = append(actual.fieldPath, selection.Index()...)
		return actual, true
	case *ast.IndexExpr:
		actual, ok := ps6088DomainExpressionActual(pass, value.X, stable, actuals, false)
		if !ok {
			actual = ps6088DirectDomainActual(pass, value.X)
			ok = actual.typeOf != nil
		}
		if !ok {
			return ps6088DomainActual{}, false
		}
		return ps6088IndexedDomainActual(pass, actual, value.Index, stable, actuals)
	case *ast.StarExpr:
		actual, ok := ps6088DomainExpressionActual(pass, value.X, stable, actuals, false)
		if !ok {
			actual = ps6088DirectDomainActual(pass, value.X)
			ok = actual.typeOf != nil
		}
		if !ok {
			return ps6088DomainActual{}, false
		}
		return ps6088DereferencedDomainActual(pass, actual, terminal)
	}
	return ps6088DomainActual{}, false
}

func ps6088DirectDomainActual(pass *analysis.Pass, expression ast.Expr) ps6088DomainActual {
	return ps6088DomainActual{expression: expression, typeOf: pass.TypesInfo.TypeOf(expression)}
}

func ps6088IndexedDomainActual(
	pass *analysis.Pass,
	container ps6088DomainActual,
	indexExpression ast.Expr,
	stable map[types.Object]struct{},
	actuals map[types.Object]ps6088DomainActual,
) (ps6088DomainActual, bool) {
	if container.known {
		if container.atMostZero {
			return ps6088NoExecutionDomainActual(), true
		}
		return ps6088DomainActual{}, false
	}
	indexValue := ps6088DomainIndexValue(pass, indexExpression, stable, actuals)
	if indexValue == nil {
		return ps6088DomainActual{}, false
	}
	if len(container.elements) > 0 {
		target, exact := constant.Int64Val(indexValue)
		if !exact {
			return ps6088DomainActual{}, false
		}
		if target < 0 || target >= int64(len(container.elements)) {
			return ps6088NoExecutionDomainActual(), true
		}
		return container.elements[target], true
	}
	expression, typeOf, zero, ok := ps6088DomainActualExpression(pass, container)
	if !ok || typeOf == nil {
		return ps6088DomainActual{}, false
	}
	underlying := types.Unalias(typeOf).Underlying()
	var elementType types.Type
	pointerContainer := false
	switch value := underlying.(type) {
	case *types.Basic:
		if value.Info()&types.IsString == 0 {
			return ps6088DomainActual{}, false
		}
		elementType = types.Typ[types.Uint8]
	case *types.Array:
		elementType = value.Elem()
	case *types.Slice:
		elementType = value.Elem()
	case *types.Map:
		elementType = value.Elem()
	case *types.Pointer:
		array, ok := types.Unalias(value.Elem()).Underlying().(*types.Array)
		if !ok {
			return ps6088DomainActual{}, false
		}
		underlying = array
		elementType = array.Elem()
		pointerContainer = true
	default:
		return ps6088DomainActual{}, false
	}
	if zero {
		if pointerContainer {
			return ps6088NoExecutionDomainActual(), true
		}
		switch underlying.(type) {
		case *types.Slice:
			return ps6088NoExecutionDomainActual(), true
		}
		atMost, known := ps6088ZeroDomainAtMost(elementType)
		return ps6088DomainActual{typeOf: elementType, atMostZero: atMost, known: known}, known
	}
	if ps6088NilPointerOrSliceExpression(pass, expression, typeOf) {
		return ps6088NoExecutionDomainActual(), true
	}
	if basic, stringType := underlying.(*types.Basic); stringType && basic.Info()&types.IsString != 0 {
		target, exact := constant.Int64Val(indexValue)
		if exact && target < 0 {
			return ps6088NoExecutionDomainActual(), true
		}
		value := pass.TypesInfo.Types[expression].Value
		if !exact || value == nil || value.Kind() != constant.String {
			return ps6088DomainActual{}, false
		}
		text := constant.StringVal(value)
		if target >= int64(len(text)) {
			return ps6088NoExecutionDomainActual(), true
		}
		return ps6088DomainActual{atMostZero: text[target] == 0, known: true}, true
	}
	if _, mapType := underlying.(*types.Map); mapType {
		if ps6088NilRangeExpression(pass, expression, typeOf) {
			atMost, known := ps6088ZeroDomainAtMost(elementType)
			return ps6088DomainActual{typeOf: elementType, atMostZero: atMost, known: known}, known
		}
		if atMost, known := ps6088MadeCollectionAtMost(pass, expression, false); known && atMost {
			zeroAtMost, zeroKnown := ps6088ZeroDomainAtMost(elementType)
			return ps6088DomainActual{typeOf: elementType, atMostZero: zeroAtMost, known: zeroKnown}, zeroKnown
		}
		literal := ps6088CompositeExpression(pass, expression)
		if literal == nil {
			return ps6088DomainActual{}, false
		}
		for _, element := range literal.Elts {
			keyed, ok := element.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			key := pass.TypesInfo.Types[keyed.Key].Value
			if key == nil {
				return ps6088DomainActual{}, false
			}
			if constant.Compare(key, token.EQL, indexValue) {
				return ps6088DomainActual{expression: keyed.Value, typeOf: elementType}, true
			}
		}
		atMost, known := ps6088ZeroDomainAtMost(elementType)
		return ps6088DomainActual{typeOf: elementType, atMostZero: atMost, known: known}, known
	}
	target, exact := constant.Int64Val(indexValue)
	if !exact {
		return ps6088DomainActual{}, false
	}
	if target < 0 {
		return ps6088NoExecutionDomainActual(), true
	}
	if array, arrayType := underlying.(*types.Array); arrayType && target >= array.Len() {
		return ps6088NoExecutionDomainActual(), true
	}
	if length, known := ps6088MadeCollectionLength(pass, expression); known {
		if target >= length {
			return ps6088NoExecutionDomainActual(), true
		}
		atMost, known := ps6088ZeroDomainAtMost(elementType)
		return ps6088DomainActual{typeOf: elementType, atMostZero: atMost, known: known}, known
	}
	literal := ps6088CompositeExpression(pass, expression)
	if literal == nil {
		return ps6088DomainActual{}, false
	}
	if _, sliceType := underlying.(*types.Slice); sliceType {
		length, known := ps6088CompositeLength(pass, literal)
		if !known {
			return ps6088DomainActual{}, false
		}
		if target >= length {
			return ps6088NoExecutionDomainActual(), true
		}
	}
	next := int64(0)
	for _, element := range literal.Elts {
		value := element
		position := next
		if keyed, ok := element.(*ast.KeyValueExpr); ok {
			key := pass.TypesInfo.Types[keyed.Key].Value
			if key == nil || key.Kind() != constant.Int {
				return ps6088DomainActual{}, false
			}
			position, exact = constant.Int64Val(key)
			if !exact {
				return ps6088DomainActual{}, false
			}
			value = keyed.Value
		}
		if position == target {
			return ps6088DomainActual{expression: value, typeOf: elementType}, true
		}
		next = position + 1
	}
	atMost, known := ps6088ZeroDomainAtMost(elementType)
	return ps6088DomainActual{typeOf: elementType, atMostZero: atMost, known: known}, known
}

func ps6088DomainIndexValue(
	pass *analysis.Pass,
	expression ast.Expr,
	stable map[types.Object]struct{},
	actuals map[types.Object]ps6088DomainActual,
) constant.Value {
	if value := pass.TypesInfo.Types[expression].Value; value != nil {
		return value
	}
	actual, ok := ps6088DomainExpressionActual(pass, expression, stable, actuals, false)
	if !ok || actual.known {
		return nil
	}
	resolved, _, zero, ok := ps6088DomainActualExpression(pass, actual)
	if !ok || zero {
		return nil
	}
	return pass.TypesInfo.Types[resolved].Value
}

func ps6088DereferencedDomainActual(
	pass *analysis.Pass,
	actual ps6088DomainActual,
	terminal bool,
) (ps6088DomainActual, bool) {
	expression, typeOf, zero, ok := ps6088DomainActualExpression(pass, actual)
	if !ok || typeOf == nil {
		return ps6088DomainActual{}, false
	}
	pointer, ok := types.Unalias(typeOf).Underlying().(*types.Pointer)
	if !ok {
		return ps6088DomainActual{}, false
	}
	if array, arrayPointer := types.Unalias(pointer.Elem()).Underlying().(*types.Array); arrayPointer && terminal {
		return ps6088DomainActual{atMostZero: array.Len() == 0, known: true}, true
	}
	if zero || ps6088NilPointerOrSliceExpression(pass, expression, typeOf) {
		return ps6088NoExecutionDomainActual(), true
	}
	if call, ok := ps2110Unparen(expression).(*ast.CallExpr); ok &&
		typedBuiltinName(pass, call.Fun, "new") {
		return ps6088DomainActual{typeOf: pointer.Elem(), zero: true}, true
	}
	if address, ok := ps2110Unparen(expression).(*ast.UnaryExpr); ok && address.Op == token.AND {
		return ps6088DomainActual{expression: address.X, typeOf: pointer.Elem()}, true
	}
	return ps6088DomainActual{}, false
}

func ps6088NoExecutionDomainActual() ps6088DomainActual {
	return ps6088DomainActual{atMostZero: true, known: true, noExecution: true}
}

func ps6088DomainActualAtMost(pass *analysis.Pass, actual ps6088DomainActual) (bool, bool) {
	if actual.known {
		return actual.atMostZero, true
	}
	if actual.zero {
		return ps6088ZeroDomainAtMost(ps6088FieldPathType(actual.typeOf, actual.fieldPath))
	}
	if actual.expression == nil {
		return false, false
	}
	switch ps2110Unparen(actual.expression).(type) {
	case *ast.SelectorExpr, *ast.IndexExpr, *ast.StarExpr:
		if resolved, ok := ps6088DomainExpressionActual(
			pass, actual.expression, nil, nil, true,
		); ok {
			return ps6088DomainActualAtMost(pass, resolved)
		}
	}
	expression, typeOf, zero, ok := ps6088DomainActualExpression(pass, actual)
	if !ok {
		return false, false
	}
	if zero {
		if pointer, ok := types.Unalias(typeOf).Underlying().(*types.Pointer); ok {
			if _, array := types.Unalias(pointer.Elem()).Underlying().(*types.Array); !array {
				return true, true
			}
		}
		return ps6088ZeroDomainAtMost(typeOf)
	}
	if ps6088NilRangeExpression(pass, expression, typeOf) {
		return true, true
	}
	if atMost, known := ps6088MadeCollectionAtMost(pass, expression, false); known {
		return atMost, true
	}
	return ps6088DomainAtMost(pass, expression, 0)
}

func ps6088CollectionAtMostWithActuals(
	pass *analysis.Pass,
	expression ast.Expr,
	stable map[types.Object]struct{},
	actuals map[types.Object]ps6088DomainActual,
	capacity bool,
) (bool, bool) {
	expression = ps2110Unparen(expression)
	if actual, substitutable := ps6088DomainExpressionActual(pass, expression, stable, actuals, true); substitutable {
		if actual.known {
			return actual.atMostZero, true
		}
		actualExpression, actualType, zero, ok := ps6088DomainActualExpression(pass, actual)
		if !ok {
			return false, false
		}
		if zero {
			return ps6088ZeroDomainAtMost(actualType)
		}
		if ps6088NilRangeExpression(pass, actualExpression, actualType) {
			return true, true
		}
		expression = ps2110Unparen(actualExpression)
	}
	if atMost, known := ps6088MadeCollectionAtMost(pass, expression, capacity); known {
		return atMost, true
	}
	return ps6088DomainAtMost(pass, expression, 0)
}

func ps6088DomainActualExpression(
	pass *analysis.Pass,
	actual ps6088DomainActual,
) (ast.Expr, types.Type, bool, bool) {
	if actual.zero {
		return nil, ps6088FieldPathType(actual.typeOf, actual.fieldPath), true, true
	}
	if len(actual.fieldPath) == 0 {
		return actual.expression, actual.typeOf, false, actual.expression != nil
	}
	expression := actual.expression
	typeOf := actual.typeOf
	for position, fieldIndex := range actual.fieldPath {
		if ps6088NilPointerOrSliceExpression(pass, expression, typeOf) ||
			ps6088NewZeroExpression(pass, expression, typeOf) {
			return nil, ps6088FieldPathType(typeOf, actual.fieldPath[position:]), true, true
		}
		structure := ps6088StructType(typeOf)
		if structure == nil || fieldIndex < 0 || fieldIndex >= structure.NumFields() {
			return nil, nil, false, false
		}
		field := structure.Field(fieldIndex)
		literal := ps6088CompositeExpression(pass, expression)
		if literal == nil {
			return nil, nil, false, false
		}
		value, present := ps6088CompositeField(pass, literal, structure, fieldIndex)
		typeOf = field.Type()
		if !present {
			return nil, ps6088FieldPathType(typeOf, actual.fieldPath[position+1:]), true, true
		}
		expression = value
	}
	return expression, typeOf, false, true
}

func ps6088NewZeroExpression(pass *analysis.Pass, expression ast.Expr, valueType types.Type) bool {
	if valueType == nil {
		return false
	}
	if _, pointer := types.Unalias(valueType).Underlying().(*types.Pointer); !pointer {
		return false
	}
	call, ok := ps2110Unparen(expression).(*ast.CallExpr)
	return ok && typedBuiltinName(pass, call.Fun, "new")
}

func ps6088StructType(value types.Type) *types.Struct {
	if value == nil {
		return nil
	}
	value = types.Unalias(value)
	if pointer, ok := value.(*types.Pointer); ok {
		value = types.Unalias(pointer.Elem())
	}
	structure, _ := value.Underlying().(*types.Struct)
	return structure
}

func ps6088CompositeExpression(pass *analysis.Pass, expression ast.Expr) *ast.CompositeLit {
	for expression != nil {
		expression = ps2110Unparen(expression)
		switch value := expression.(type) {
		case *ast.CompositeLit:
			return value
		case *ast.UnaryExpr:
			if value.Op != token.AND {
				return nil
			}
			expression = value.X
		case *ast.StarExpr:
			expression = value.X
		case *ast.CallExpr:
			if len(value.Args) != 1 || value.Ellipsis.IsValid() ||
				!pass.TypesInfo.Types[value.Fun].IsType() ||
				!ps6088FieldPreservingConversion(pass, value) {
				return nil
			}
			expression = value.Args[0]
		default:
			return nil
		}
	}
	return nil
}

func ps6088FieldPreservingConversion(pass *analysis.Pass, call *ast.CallExpr) bool {
	from, to := pass.TypesInfo.TypeOf(call.Args[0]), pass.TypesInfo.TypeOf(call)
	if from == nil || to == nil {
		return false
	}
	return types.Identical(types.Unalias(from).Underlying(), types.Unalias(to).Underlying())
}

func ps6088CompositeField(
	pass *analysis.Pass,
	literal *ast.CompositeLit,
	structure *types.Struct,
	fieldIndex int,
) (ast.Expr, bool) {
	for index, element := range literal.Elts {
		keyed, keyedOK := element.(*ast.KeyValueExpr)
		if !keyedOK {
			if index == fieldIndex {
				return element, true
			}
			continue
		}
		identifier, ok := ps2110Unparen(keyed.Key).(*ast.Ident)
		if ok && (pass.TypesInfo.ObjectOf(identifier) == structure.Field(fieldIndex) ||
			identifier.Name == structure.Field(fieldIndex).Name()) {
			return keyed.Value, true
		}
	}
	return nil, false
}

func ps6088FieldPathType(value types.Type, path []int) types.Type {
	for _, index := range path {
		structure := ps6088StructType(value)
		if structure == nil || index < 0 || index >= structure.NumFields() {
			return nil
		}
		value = structure.Field(index).Type()
	}
	return value
}

func ps6088ZeroDomainAtMost(value types.Type) (bool, bool) {
	if value == nil {
		return false, false
	}
	switch underlying := types.Unalias(value).Underlying().(type) {
	case *types.Basic:
		return underlying.Info()&(types.IsInteger|types.IsString) != 0, true
	case *types.Array:
		return underlying.Len() == 0, true
	case *types.Slice, *types.Map, *types.Chan:
		return true, true
	case *types.Pointer:
		array, ok := types.Unalias(underlying.Elem()).Underlying().(*types.Array)
		if ok {
			return array.Len() == 0, true
		}
	}
	return false, false
}

func ps6088MadeCollectionAtMost(
	pass *analysis.Pass,
	expression ast.Expr,
	capacity bool,
) (bool, bool) {
	expression = ps6088UnwrapCollectionConversions(pass, expression)
	call, ok := ps2110Unparen(expression).(*ast.CallExpr)
	if !ok || !typedBuiltinName(pass, call.Fun, "make") {
		return false, false
	}
	typeOf := pass.TypesInfo.TypeOf(call)
	if typeOf == nil {
		return false, false
	}
	switch types.Unalias(typeOf).Underlying().(type) {
	case *types.Slice:
		argument := 1
		if capacity && len(call.Args) > 2 {
			argument = 2
		}
		if argument < len(call.Args) {
			return ps6088DomainAtMost(pass, call.Args[argument], 0)
		}
	case *types.Map:
		if !capacity {
			return true, true
		}
	case *types.Chan:
		if !capacity {
			return true, true
		}
		if len(call.Args) > 1 {
			return ps6088DomainAtMost(pass, call.Args[1], 0)
		}
	}
	return false, false
}

func ps6088MadeCollectionLength(pass *analysis.Pass, expression ast.Expr) (int64, bool) {
	expression = ps6088UnwrapCollectionConversions(pass, expression)
	call, ok := ps2110Unparen(expression).(*ast.CallExpr)
	if !ok || !typedBuiltinName(pass, call.Fun, "make") || len(call.Args) < 2 {
		return 0, false
	}
	if _, slice := types.Unalias(pass.TypesInfo.TypeOf(call)).Underlying().(*types.Slice); !slice {
		return 0, false
	}
	value := ps6088SliceBound(pass, call.Args[1], 0, false)
	if value == nil {
		return 0, false
	}
	return constant.Int64Val(value)
}

func ps6088UnwrapCollectionConversions(pass *analysis.Pass, expression ast.Expr) ast.Expr {
	for {
		call, ok := ps2110Unparen(expression).(*ast.CallExpr)
		if !ok || len(call.Args) != 1 || call.Ellipsis.IsValid() ||
			!pass.TypesInfo.Types[call.Fun].IsType() {
			return expression
		}
		from, to := pass.TypesInfo.TypeOf(call.Args[0]), pass.TypesInfo.TypeOf(call)
		if from == nil || to == nil ||
			!types.Identical(types.Unalias(from).Underlying(), types.Unalias(to).Underlying()) {
			return expression
		}
		switch types.Unalias(to).Underlying().(type) {
		case *types.Slice, *types.Map, *types.Chan:
			expression = call.Args[0]
		default:
			return expression
		}
	}
}

func ps6088RangePreservingTypeParamConversion(pass *analysis.Pass, call *ast.CallExpr) bool {
	to, ok := types.Unalias(pass.TypesInfo.TypeOf(call)).Underlying().(*types.Basic)
	if !ok || to.Info()&(types.IsInteger|types.IsString) == 0 {
		return false
	}
	from := types.Unalias(pass.TypesInfo.TypeOf(call.Args[0]))
	parameter, ok := from.(*types.TypeParam)
	return ok && ps6088ConstraintHasBasicKind(parameter.Constraint(), to.Kind(), make(map[types.Type]bool))
}

func ps6088ConstraintHasBasicKind(
	value types.Type,
	kind types.BasicKind,
	seen map[types.Type]bool,
) bool {
	if value == nil || seen[value] {
		return false
	}
	seen[value] = true
	value = types.Unalias(value)
	switch typed := value.(type) {
	case *types.Basic:
		return typed.Kind() == kind
	case *types.Named:
		basic, ok := typed.Underlying().(*types.Basic)
		return ok && basic.Kind() == kind
	case *types.TypeParam:
		return ps6088ConstraintHasBasicKind(typed.Constraint(), kind, seen)
	case *types.Union:
		if typed.Len() == 0 {
			return false
		}
		for index := range typed.Len() {
			if !ps6088ConstraintHasBasicKind(typed.Term(index).Type(), kind, seen) {
				return false
			}
		}
		return true
	case *types.Interface:
		typed.Complete()
		if typed.NumEmbeddeds() == 0 {
			return false
		}
		for index := range typed.NumEmbeddeds() {
			if !ps6088ConstraintHasBasicKind(typed.EmbeddedType(index), kind, seen) {
				return false
			}
		}
		return true
	}
	return false
}

func ps6088NilRangeExpression(pass *analysis.Pass, expression ast.Expr, rangeType types.Type) bool {
	if rangeType == nil {
		return false
	}
	switch types.Unalias(rangeType).Underlying().(type) {
	case *types.Slice, *types.Map, *types.Chan:
	default:
		return false
	}
	return ps6088NilTypedExpression(pass, expression)
}

func ps6088NilPointerOrSliceExpression(
	pass *analysis.Pass,
	expression ast.Expr,
	valueType types.Type,
) bool {
	if valueType == nil {
		return false
	}
	switch types.Unalias(valueType).Underlying().(type) {
	case *types.Pointer, *types.Slice:
	default:
		return false
	}
	return ps6088NilTypedExpression(pass, expression)
}

func ps6088NilFunctionExpression(pass *analysis.Pass, expression ast.Expr, valueType types.Type) bool {
	if valueType == nil {
		return false
	}
	if _, ok := types.Unalias(valueType).Underlying().(*types.Signature); !ok {
		return false
	}
	return ps6088NilTypedExpression(pass, expression)
}

func ps6088NilTypedExpression(pass *analysis.Pass, expression ast.Expr) bool {
	for {
		resolved, zero, safe := ps6088StableExpression(pass, expression)
		if !safe {
			return false
		}
		if zero {
			return true
		}
		expression = ps2110Unparen(resolved)
		conversion, ok := expression.(*ast.CallExpr)
		if !ok || len(conversion.Args) != 1 || conversion.Ellipsis.IsValid() ||
			!pass.TypesInfo.Types[conversion.Fun].IsType() {
			return false
		}
		expression = conversion.Args[0]
	}
}

func ps6088ImmediateFuncLit(
	pass *analysis.Pass,
	literal *ast.FuncLit,
	stack []ast.Node,
) *ast.CallExpr {
	var expression ast.Expr = literal
	for index := len(stack) - 1; index >= 0; index-- {
		switch parent := stack[index].(type) {
		case *ast.ParenExpr:
			expression = parent
			continue
		case *ast.CallExpr:
			if ps2110Unparen(parent.Fun) == ps2110Unparen(expression) {
				return parent
			}
			if len(parent.Args) != 1 ||
				ps2110Unparen(parent.Args[0]) != ps2110Unparen(expression) ||
				!ps6088FunctionConversion(pass, parent) {
				return nil
			}
			expression = parent
			continue
		default:
			return nil
		}
	}
	return nil
}

func ps6088FunctionCallRepeated(
	pass *analysis.Pass,
	call *ast.CallExpr,
	stack []ast.Node,
	functionFlow *cfg.CFG,
	literalFlows map[*ast.FuncLit]*cfg.CFG,
	literalInvocations map[*ast.FuncLit]*ast.CallExpr,
	parameters map[types.Object]struct{},
) bool {
	for _, ancestor := range stack {
		if _, typeSwitch := ancestor.(*ast.TypeSwitchStmt); typeSwitch {
			return false
		}
	}
	type literalScope struct {
		literal *ast.FuncLit
		index   int
	}
	var scopes []literalScope
	for index, ancestor := range stack {
		if literal, ok := ancestor.(*ast.FuncLit); ok {
			scopes = append(scopes, literalScope{literal: literal, index: index})
		}
	}
	if !ps6088DeferredExecutionViable(
		pass, call, stack, functionFlow, literalFlows, literalInvocations,
	) {
		return false
	}
	if len(scopes) == 0 {
		return ps6088RepeatedLoop(pass, call, call, stack, functionFlow, parameters)
	}
	innermost := scopes[len(scopes)-1]
	if ps6088RepeatedLoop(
		pass, call, call, stack[innermost.index+1:], literalFlows[innermost.literal], parameters,
	) {
		return true
	}
	completionCall := call
	for index := len(scopes) - 1; index >= 0; index-- {
		invocation := literalInvocations[scopes[index].literal]
		invocationIndex := ps6088AncestorIndex(stack, invocation)
		if invocation == nil || invocationIndex < 0 {
			return false
		}
		if !ps6088CallOperandsReturn(pass, invocation) {
			return false
		}
		scopeStart := 0
		scopeFlow := functionFlow
		if index > 0 {
			outer := scopes[index-1]
			scopeStart = outer.index + 1
			scopeFlow = literalFlows[outer.literal]
		}
		if invocationIndex < scopeStart {
			return false
		}
		if !ps6088GoCall(invocation, stack[:invocationIndex]) &&
			(!ps6088LiteralCFGCanReachReturn(
				pass, scopes[index].literal, literalFlows[scopes[index].literal], completionCall.Pos(),
			) || ps6088LiteralMayPreventReturn(pass, scopes[index].literal)) {
			return false
		}
		if ps6088DeferredCall(invocation, stack[:invocationIndex]) &&
			!ps6088CFGCanReachUnwind(pass, scopeFlow, invocation.Pos()) {
			return false
		}
		if ps6088RepeatedLoop(
			pass, invocation, call, stack[scopeStart:invocationIndex], scopeFlow, parameters,
		) {
			return true
		}
		completionCall = invocation
	}
	return false
}

func ps6088DeferredExecutionViable(
	pass *analysis.Pass,
	call *ast.CallExpr,
	stack []ast.Node,
	functionFlow *cfg.CFG,
	literalFlows map[*ast.FuncLit]*cfg.CFG,
	literalInvocations map[*ast.FuncLit]*ast.CallExpr,
) bool {
	type literalScope struct {
		literal *ast.FuncLit
		index   int
	}
	var scopes []literalScope
	for index, ancestor := range stack {
		if literal, ok := ancestor.(*ast.FuncLit); ok {
			scopes = append(scopes, literalScope{literal: literal, index: index})
		}
	}
	executionFlow := functionFlow
	if len(scopes) > 0 {
		executionFlow = literalFlows[scopes[len(scopes)-1].literal]
	}
	if ps6088DeferredCall(call, stack) &&
		(executionFlow == nil || !ps6088CFGCanReachUnwind(pass, executionFlow, call.Pos())) {
		return false
	}
	for index := len(scopes) - 1; index >= 0; index-- {
		invocation := literalInvocations[scopes[index].literal]
		invocationIndex := ps6088AncestorIndex(stack, invocation)
		if invocation == nil || invocationIndex < 0 {
			return false
		}
		if !ps6088CallOperandsReturn(pass, invocation) {
			return false
		}
		if !ps6088DeferredCall(invocation, stack[:invocationIndex]) {
			continue
		}
		scopeFlow := functionFlow
		if index > 0 {
			scopeFlow = literalFlows[scopes[index-1].literal]
		}
		if scopeFlow == nil || !ps6088CFGCanReachUnwind(pass, scopeFlow, invocation.Pos()) {
			return false
		}
	}
	return true
}

func ps6088CFGCanReachReturn(pass *analysis.Pass, flow *cfg.CFG, position token.Pos) bool {
	start := ps6079CFGBlockAt(flow, position)
	if start == nil || !start.Live {
		return false
	}
	return ps6088CFGBlockCanReachReturn(pass, start)
}

func ps6088LiteralCFGCanReachReturn(
	pass *analysis.Pass,
	literal *ast.FuncLit,
	flow *cfg.CFG,
	position token.Pos,
) bool {
	start := ps6079CFGBlockAt(flow, position)
	if start == nil || !start.Live {
		return false
	}
	return ps6088LiteralCFGBlockCanReachReturn(pass, literal, start)
}

func ps6088CFGCanReachUnwind(pass *analysis.Pass, flow *cfg.CFG, position token.Pos) bool {
	if !ps6088CFGCanReachReturn(pass, flow, position) {
		return false
	}
	for _, block := range flow.Blocks {
		if !block.Live {
			continue
		}
		for _, node := range block.Nodes {
			deferred, ok := node.(*ast.DeferStmt)
			if ok && ps6088DeferredCallMayPreventReturn(pass, deferred.Call) {
				return false
			}
			if ps6088NodeHasSynchronousNonreturnIIFE(pass, node) {
				return false
			}
		}
	}
	return true
}

func ps6088NodeHasSynchronousNonreturnIIFE(pass *analysis.Pass, node ast.Node) bool {
	nonreturn := false
	astutil.WithStack(node, func(current ast.Node, stack []ast.Node) bool {
		if nonreturn || current == nil {
			return false
		}
		if _, literal := current.(*ast.FuncLit); literal {
			return false
		}
		call, ok := current.(*ast.CallExpr)
		if !ok {
			return true
		}
		if !ps6088StaticPathLive(pass, call, stack) || ps6088CallOperandUnevaluated(pass, call) {
			return false
		}
		invoked := ps6088InvokedFuncLit(pass, call)
		if invoked == nil {
			return true
		}
		if ps6088AsyncCall(call, stack, false) {
			return false
		}
		flow := cfg.New(
			invoked.Body,
			func(call *ast.CallExpr) bool { return !ps6088NonreturnCall(pass, call) },
		)
		nonreturn = len(flow.Blocks) == 0 || !ps6088LiteralCFGBlockCanReachReturn(pass, invoked, flow.Blocks[0]) ||
			ps6088LiteralMayPreventReturn(pass, invoked)
		return false
	})
	return nonreturn
}

func ps6088CFGBlockCanReachReturn(pass *analysis.Pass, start *cfg.Block) bool {
	seen := make(map[*cfg.Block]bool)
	queue := []*cfg.Block{start}
	for len(queue) > 0 {
		block := queue[0]
		queue = queue[1:]
		if seen[block] {
			continue
		}
		seen[block] = true
		if block.Return() != nil && !ps6088BlockHasNonreturnBefore(pass, block, token.NoPos) {
			return true
		}
		queue = append(queue, ps6088BlockSuccessors(pass, block)...)
	}
	return false
}

func ps6088LiteralCFGBlockCanReachReturn(
	pass *analysis.Pass,
	literal *ast.FuncLit,
	start *cfg.Block,
) bool {
	seen := make(map[*cfg.Block]bool)
	queue := []*cfg.Block{start}
	for len(queue) > 0 {
		block := queue[0]
		queue = queue[1:]
		if seen[block] {
			continue
		}
		seen[block] = true
		preventsReturn, recoveredReturn := ps6088LiteralBlockCompletion(
			pass, literal, block, token.NoPos,
		)
		if recoveredReturn || block.Return() != nil && !preventsReturn {
			return true
		}
		if preventsReturn {
			continue
		}
		queue = append(queue, ps6088BlockSuccessors(pass, block)...)
	}
	return false
}

func ps6088LiteralBlockCompletion(
	pass *analysis.Pass,
	literal *ast.FuncLit,
	block *cfg.Block,
	position token.Pos,
) (bool, bool) {
	for _, node := range block.Nodes {
		statement, ok := node.(ast.Stmt)
		if !ok || position.IsValid() && statement.End() > position {
			continue
		}
		if expression, ok := statement.(*ast.ExprStmt); ok {
			call, directCall := ps2110Unparen(expression.X).(*ast.CallExpr)
			if directCall && ps6088CallInvocationPanics(pass, call) &&
				ps6088CallOperandsReturn(pass, call) &&
				ps6088LiteralHasPriorRecoveringDefer(pass, literal, call.Pos()) {
				return false, true
			}
		}
		if ps6088LiteralSimpleStatementPreventsReturn(pass, literal, statement) {
			return true, false
		}
	}
	return false, false
}

func ps6088LiteralSimpleStatementPreventsReturn(
	pass *analysis.Pass,
	literal *ast.FuncLit,
	statement ast.Stmt,
) bool {
	switch value := statement.(type) {
	case *ast.ExprStmt:
		call, ok := ps2110Unparen(value.X).(*ast.CallExpr)
		if ok {
			if ps6088NonreturnCall(pass, call) {
				return true
			}
			if ps6088CallInvocationPanics(pass, call) {
				return !ps6088CallOperandsReturn(pass, call) ||
					!ps6088LiteralHasPriorRecoveringDefer(pass, literal, call.Pos())
			}
		}
		return !ps6088StatementReturnsNormally(pass, statement, false)
	case *ast.AssignStmt, *ast.DeclStmt, *ast.IncDecStmt, *ast.SendStmt,
		*ast.GoStmt, *ast.DeferStmt, *ast.ReturnStmt:
		return !ps6088StatementReturnsNormally(pass, statement, false)
	}
	return false
}

func ps6088LiteralMayPreventReturn(pass *analysis.Pass, literal *ast.FuncLit) bool {
	nonreturn := false
	recoveredReturn := false
	astutil.WithStack(literal.Body, func(node ast.Node, stack []ast.Node) bool {
		if nonreturn || recoveredReturn || node == nil {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		if statement, ok := node.(ast.Stmt); ok &&
			ps6088LiteralSimpleStatementPreventsReturn(pass, literal, statement) {
			nonreturn = true
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if !ps6088AsyncCall(call, stack, true) && ps6088CallInvocationPanics(pass, call) {
			recovered := ps6088CallOperandsReturn(pass, call) &&
				ps6088LiteralHasPriorRecoveringDefer(pass, literal, call.Pos())
			if recovered {
				recoveredReturn = true
				return false
			}
			nonreturn = true
			return false
		}
		if ps6088DeferredCall(call, stack) && ps6088DeferredCallMayPreventReturn(pass, call) {
			nonreturn = true
			return false
		}
		invoked := ps6088InvokedFuncLit(pass, call)
		if invoked == nil {
			return true
		}
		if ps6088AsyncCall(call, stack, true) {
			return false
		}
		invokedFlow := cfg.New(
			invoked.Body,
			func(call *ast.CallExpr) bool { return !ps6088NonreturnCall(pass, call) },
		)
		nonreturn = len(invokedFlow.Blocks) == 0 ||
			!ps6088LiteralCFGBlockCanReachReturn(pass, invoked, invokedFlow.Blocks[0]) ||
			ps6088LiteralMayPreventReturn(pass, invoked)
		return false
	})
	return nonreturn
}

func ps6088LiteralHasPriorRecoveringDefer(
	pass *analysis.Pass,
	literal *ast.FuncLit,
	panicPosition token.Pos,
) bool {
	directPanic := false
	for _, statement := range literal.Body.List {
		expression, ok := statement.(*ast.ExprStmt)
		if !ok {
			continue
		}
		candidate, ok := ps2110Unparen(expression.X).(*ast.CallExpr)
		if ok && candidate.Pos() == panicPosition {
			directPanic = true
			break
		}
	}
	if !directPanic {
		return false
	}
	hasGoto := false
	ast.Inspect(literal.Body, func(node ast.Node) bool {
		if hasGoto || node == nil {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		branch, ok := node.(*ast.BranchStmt)
		hasGoto = ok && branch.Tok == token.GOTO
		return !hasGoto
	})
	if hasGoto {
		return false
	}
	for _, statement := range literal.Body.List {
		if statement.Pos() >= panicPosition {
			break
		}
		deferred, ok := statement.(*ast.DeferStmt)
		if !ok || deferred.Call == nil || !ps6088CallOperandsReturn(pass, deferred.Call) {
			continue
		}
		invoked := ps6088InvokedFuncLit(pass, deferred.Call)
		if invoked == nil || len(invoked.Body.List) != 1 {
			continue
		}
		if ps6088StatementDirectlyRecovers(pass, invoked.Body.List[0]) {
			return true
		}
	}
	return false
}

func ps6088StatementDirectlyRecovers(pass *analysis.Pass, statement ast.Stmt) bool {
	var expression ast.Expr
	switch value := statement.(type) {
	case *ast.ExprStmt:
		expression = value.X
	case *ast.AssignStmt:
		if len(value.Lhs) != 1 || len(value.Rhs) != 1 {
			return false
		}
		if _, safeDestination := ps2110Unparen(value.Lhs[0]).(*ast.Ident); !safeDestination {
			return false
		}
		expression = value.Rhs[0]
	default:
		return false
	}
	call, ok := ps2110Unparen(expression).(*ast.CallExpr)
	return ok && len(call.Args) == 0 && !call.Ellipsis.IsValid() &&
		typedBuiltinName(pass, call.Fun, "recover")
}

func ps6088DeferredCallMayPreventReturn(pass *analysis.Pass, call *ast.CallExpr) bool {
	if ps6088NonreturnCall(pass, call) || ps6088CallInvocationPanics(pass, call) {
		return true
	}
	invoked := ps6088InvokedFuncLit(pass, call)
	if invoked == nil {
		return false
	}
	flow := cfg.New(
		invoked.Body,
		func(call *ast.CallExpr) bool { return !ps6088NonreturnCall(pass, call) },
	)
	return len(flow.Blocks) == 0 || !ps6088LiteralCFGBlockCanReachReturn(pass, invoked, flow.Blocks[0]) ||
		ps6088LiteralMayPreventReturn(pass, invoked)
}

func ps6088AncestorIndex(stack []ast.Node, target ast.Node) int {
	for index, ancestor := range stack {
		if ancestor == target {
			return index
		}
	}
	return -1
}

func ps6088FunctionConversion(pass *analysis.Pass, call *ast.CallExpr) bool {
	typedFunction, ok := pass.TypesInfo.Types[ps2110Unparen(call.Fun)]
	if !ok || !typedFunction.IsType() {
		return false
	}
	converted, ok := pass.TypesInfo.Types[call]
	if !ok || converted.Type == nil {
		return false
	}
	_, ok = converted.Type.Underlying().(*types.Signature)
	return ok
}

func ps6088UnevaluatedUnsafeBuiltin(pass *analysis.Pass, call *ast.CallExpr) bool {
	for _, name := range []string{"Alignof", "Offsetof", "Sizeof"} {
		if ps6088UnsafeBuiltinName(pass, call.Fun, name) {
			return true
		}
	}
	return false
}

func ps6088CallOperandUnevaluated(pass *analysis.Pass, call *ast.CallExpr) bool {
	return ps6088UnevaluatedUnsafeBuiltin(pass, call) ||
		(len(call.Args) == 1 && !call.Ellipsis.IsValid() &&
			(typedBuiltinName(pass, call.Fun, "len") || typedBuiltinName(pass, call.Fun, "cap")) &&
			ps6088ArrayOrPointerArray(pass.TypesInfo.TypeOf(call.Args[0])) &&
			ps6088ConstantArrayRangeExpression(pass, call.Args[0]))
}

func ps6088UnsafeBuiltinName(pass *analysis.Pass, expression ast.Expr, name string) bool {
	switch value := ps2110Unparen(expression).(type) {
	case *ast.Ident:
		builtin, ok := pass.TypesInfo.Uses[value].(*types.Builtin)
		return ok && builtin.Name() == name
	case *ast.SelectorExpr:
		if pass.TypesInfo.Selections[value] != nil {
			return false
		}
		qualifier, ok := ps2110Unparen(value.X).(*ast.Ident)
		if !ok {
			return false
		}
		imported, ok := pass.TypesInfo.ObjectOf(qualifier).(*types.PkgName)
		if !ok || imported.Imported().Path() != "unsafe" {
			return false
		}
		builtin, ok := pass.TypesInfo.Uses[value.Sel].(*types.Builtin)
		return ok && builtin.Name() == name
	}
	return false
}

func ps6088LiteralPathLive(
	pass *analysis.Pass,
	call *ast.CallExpr,
	stack []ast.Node,
	flows map[*ast.FuncLit]*cfg.CFG,
) bool {
	for index := len(stack) - 1; index >= 0; index-- {
		literal, ok := stack[index].(*ast.FuncLit)
		if !ok {
			continue
		}
		flow := flows[literal]
		block := ps6079CFGBlockAt(flow, call.Pos())
		if block == nil || !block.Live || !ps6088CFGPositionLive(pass, flow, call.Pos()) {
			return false
		}
	}
	return true
}

func ps6088StaticPathLive(pass *analysis.Pass, node ast.Node, stack []ast.Node) bool {
	if loop, ok := node.(*ast.ForStmt); ok && loop.Cond != nil {
		if condition, known := ps6088Boolean(pass, loop.Cond); known && !condition {
			return false
		}
	}
	if loop, ok := node.(*ast.RangeStmt); ok {
		if empty, known := ps6088LoopAtMost(pass, loop, 0); known && empty {
			return false
		}
	}
	for index, ancestor := range stack {
		child := node
		if index+1 < len(stack) {
			child = stack[index+1]
		}
		switch statement := ancestor.(type) {
		case *ast.IfStmt:
			condition, known := ps6088Boolean(pass, statement.Cond)
			if !known {
				continue
			}
			if (!condition && child == statement.Body) ||
				(condition && statement.Else != nil && child == statement.Else) {
				return false
			}
		case *ast.ForStmt:
			if statement.Cond == nil {
				continue
			}
			condition, known := ps6088Boolean(pass, statement.Cond)
			if known && !condition && (child == statement.Body || child == statement.Post) {
				return false
			}
		case *ast.RangeStmt:
			if child != statement.Body {
				continue
			}
			empty, known := ps6088LoopAtMost(pass, statement, 0)
			if known && empty {
				return false
			}
		case *ast.BinaryExpr:
			if child != statement.Y {
				continue
			}
			left, known := ps6088Boolean(pass, statement.X)
			if known && ((statement.Op == token.LAND && !left) || (statement.Op == token.LOR && left)) {
				return false
			}
		case *ast.SwitchStmt:
			if child == statement.Body && !ps6088SwitchPathLive(pass, statement, stack[index+1:], node) {
				return false
			}
		case *ast.CommClause:
			if ps6088CommClauseBodyChild(statement, child) {
				if ps6088NilCommClause(pass, statement) ||
					ps6088ClosedSendComm(pass, statement.Comm) {
					return false
				}
				if statement.Comm == nil {
					for stackIndex := index - 1; stackIndex >= 0; stackIndex-- {
						selection, ok := stack[stackIndex].(*ast.SelectStmt)
						if ok && ps6088SelectHasClosedSend(pass, selection) {
							return false
						}
					}
				}
			}
		case *ast.AssignStmt:
			if index > 0 {
				clause, ok := stack[index-1].(*ast.CommClause)
				if ok && clause.Comm == statement && ps6088NilCommClause(pass, clause) &&
					ps6088ExpressionListContains(statement.Lhs, child) {
					return false
				}
			}
		}
	}
	return true
}

func ps6088ExpressionListContains(expressions []ast.Expr, node ast.Node) bool {
	for _, expression := range expressions {
		if expression == node || expression.Pos() <= node.Pos() && node.End() <= expression.End() {
			return true
		}
	}
	return false
}

func ps6088CommClauseBodyChild(clause *ast.CommClause, child ast.Node) bool {
	for _, statement := range clause.Body {
		if statement == child {
			return true
		}
	}
	return false
}

func ps6088NilCommClause(pass *analysis.Pass, clause *ast.CommClause) bool {
	var channel ast.Expr
	switch communication := clause.Comm.(type) {
	case *ast.SendStmt:
		channel = communication.Chan
	case *ast.ExprStmt:
		if receive, ok := ps2110Unparen(communication.X).(*ast.UnaryExpr); ok && receive.Op == token.ARROW {
			channel = receive.X
		}
	case *ast.AssignStmt:
		if len(communication.Rhs) == 1 {
			if receive, ok := ps2110Unparen(communication.Rhs[0]).(*ast.UnaryExpr); ok && receive.Op == token.ARROW {
				channel = receive.X
			}
		}
	}
	return channel != nil &&
		ps6088NilRangeExpression(pass, channel, pass.TypesInfo.TypeOf(channel))
}

func ps6088ClosedSendComm(pass *analysis.Pass, communication ast.Stmt) bool {
	send, ok := communication.(*ast.SendStmt)
	return ok && ps6088DefinitelyClosedChannel(pass, send, send.Chan)
}

func ps6088SelectHasClosedSend(pass *analysis.Pass, statement *ast.SelectStmt) bool {
	for _, item := range statement.Body.List {
		clause, ok := item.(*ast.CommClause)
		if ok && ps6088ClosedSendComm(pass, clause.Comm) {
			return true
		}
	}
	return false
}

func ps6088PriorStatementsReturn(pass *analysis.Pass, node ast.Node, stack []ast.Node) bool {
	for index, ancestor := range stack {
		block, ok := ancestor.(*ast.BlockStmt)
		if !ok {
			continue
		}
		childNode := node
		if index+1 < len(stack) {
			childNode = stack[index+1]
		}
		child, ok := childNode.(ast.Stmt)
		if !ok {
			continue
		}
		for statementIndex, statement := range block.List {
			if statement != child {
				continue
			}
			if ps6088PriorGotoTargetsStatement(pass, block.List[:statementIndex], child) {
				break
			}
			if !ps6088StatementOutcomes(pass, block.List[:statementIndex], "").fallthroughPossible {
				return false
			}
			break
		}
	}
	return true
}

func ps6088PriorGotoTargetsStatement(
	pass *analysis.Pass,
	prior []ast.Stmt,
	statement ast.Stmt,
) bool {
	labels := make(map[string]int)
	for index, candidate := range prior {
		if labeled, ok := candidate.(*ast.LabeledStmt); ok {
			labels[labeled.Label.Name] = index
		}
	}
	if labeled, ok := statement.(*ast.LabeledStmt); ok {
		labels[labeled.Label.Name] = len(prior)
	}
	if len(labels) == 0 {
		return false
	}
	found := false
	for candidateIndex, candidate := range prior {
		astutil.WithStack(candidate, func(node ast.Node, stack []ast.Node) bool {
			if found || node == nil {
				return false
			}
			if _, literal := node.(*ast.FuncLit); literal {
				return false
			}
			branch, ok := node.(*ast.BranchStmt)
			targetIndex := -1
			if ok && branch.Label != nil {
				if index, exists := labels[branch.Label.Name]; exists {
					targetIndex = index
				}
			}
			if ok && branch.Tok == token.GOTO && branch.Label != nil &&
				targetIndex > candidateIndex &&
				ps6088StaticPathLive(pass, branch, stack) &&
				ps6088PriorStatementsReturn(pass, branch, stack) &&
				ps6088EnclosingStatementHeadersReturn(pass, stack) &&
				ps6088StatementOutcomes(pass, prior[:candidateIndex], "").fallthroughPossible &&
				(targetIndex == len(prior) ||
					ps6088StatementOutcomes(pass, prior[targetIndex:], "").fallthroughPossible) {
				found = true
				return false
			}
			return true
		})
	}
	return found
}

func ps6088EnclosingStatementHeadersReturn(pass *analysis.Pass, stack []ast.Node) bool {
	for _, ancestor := range stack {
		switch statement := ancestor.(type) {
		case *ast.IfStmt:
			if !ps6088StatementHeaderReturns(pass, statement) {
				return false
			}
		case *ast.SwitchStmt:
			if !ps6088StatementHeaderReturns(pass, statement) {
				return false
			}
		case *ast.TypeSwitchStmt:
			if !ps6088StatementHeaderReturns(pass, statement) {
				return false
			}
		case *ast.ForStmt:
			if !ps6088StatementHeaderReturns(pass, statement) {
				return false
			}
		case *ast.RangeStmt:
			if !ps6088StatementHeaderReturns(pass, statement) {
				return false
			}
		case *ast.SelectStmt:
			if !ps6088StatementHeaderReturns(pass, statement) {
				return false
			}
		}
	}
	return true
}

func ps6088SwitchPathLive(
	pass *analysis.Pass,
	statement *ast.SwitchStmt,
	descendants []ast.Node,
	node ast.Node,
) bool {
	var targetClause *ast.CaseClause
	targetExpression := -1
	for index, descendant := range descendants {
		clause, ok := descendant.(*ast.CaseClause)
		if !ok {
			continue
		}
		child := node
		if index+1 < len(descendants) {
			child = descendants[index+1]
		}
		if !ps6088CaseBodyChild(clause, child) {
			for expressionIndex, expression := range clause.List {
				if expression == child {
					targetExpression = expressionIndex
					break
				}
			}
			if targetExpression < 0 {
				continue
			}
		}
		targetClause = clause
		break
	}
	if targetClause == nil {
		return true
	}
	tag := constant.MakeBool(true)
	if statement.Tag != nil {
		tag = pass.TypesInfo.Types[statement.Tag].Value
		if tag == nil {
			return true
		}
	}
	selected, fallback, targetIndex := -1, -1, -1
	clauses := make([]*ast.CaseClause, 0, len(statement.Body.List))
	for _, item := range statement.Body.List {
		clause, ok := item.(*ast.CaseClause)
		if !ok {
			return true
		}
		clauses = append(clauses, clause)
		clauseIndex := len(clauses) - 1
		if clause == targetClause {
			targetIndex = clauseIndex
		}
		if len(clause.List) == 0 {
			fallback = clauseIndex
		}
	}
selection:
	for clauseIndex, clause := range clauses {
		for expressionIndex, expression := range clause.List {
			if clauseIndex == targetIndex && expressionIndex == targetExpression {
				return true
			}
			value := pass.TypesInfo.Types[expression].Value
			if value == nil {
				return true
			}
			if constant.Compare(tag, token.EQL, value) {
				selected = clauseIndex
				break selection
			}
		}
	}
	if targetExpression >= 0 {
		return false
	}
	if selected < 0 {
		selected = fallback
	}
	if selected < 0 || targetIndex < selected {
		return false
	}
	for index := selected; index < targetIndex; index++ {
		if !ps6088FallsThrough(clauses[index]) {
			return false
		}
	}
	return true
}

func ps6088CaseBodyChild(clause *ast.CaseClause, child ast.Node) bool {
	for _, statement := range clause.Body {
		if statement == child {
			return true
		}
	}
	return false
}

func ps6088FallsThrough(clause *ast.CaseClause) bool {
	if len(clause.Body) == 0 {
		return false
	}
	branch, ok := clause.Body[len(clause.Body)-1].(*ast.BranchStmt)
	return ok && branch.Tok == token.FALLTHROUGH
}

func ps6088RepeatedLoop(
	pass *analysis.Pass,
	positionCall *ast.CallExpr,
	ignoredCall *ast.CallExpr,
	stack []ast.Node,
	flow *cfg.CFG,
	parameters map[types.Object]struct{},
) bool {
	if !ps6088EagerStatementSuffixReturns(pass, positionCall, stack) {
		return false
	}
	for _, ancestor := range stack {
		if _, typeSwitch := ancestor.(*ast.TypeSwitchStmt); typeSwitch {
			return false
		}
	}
	sourceMayRepeat := false
	for index := len(stack) - 1; index >= 1; index-- {
		if _, boundary := stack[index-1].(*ast.FuncLit); boundary {
			return false
		}
		loop := stack[index-1]
		if !astutil.IsLoop(loop) || astutil.LoopBody(loop) != stack[index] {
			continue
		}
		if !ps6088CallerLoopDomainSupported(pass, loop) {
			continue
		}
		mayRepeat, intraBodyCycle := ps6088CallerLoopMayRepeat(
			pass, loop, positionCall, ignoredCall, stack, index-1, parameters,
		)
		atMostOne, known := ps6088LoopAtMost(pass, loop, 1)
		if intraBodyCycle || ((!known || !atMostOne) && mayRepeat) {
			sourceMayRepeat = true
		}
	}
	return sourceMayRepeat && ps6088PositionRepeats(pass, flow, positionCall.Pos())
}

func ps6088CallOperandsReturn(pass *analysis.Pass, call *ast.CallExpr) bool {
	if ps6088NodeHasSynchronousNonreturnIIFE(pass, call.Fun) ||
		ps6088DirectExpressionPanics(pass, call.Fun) ||
		ps6088NodeHasSynchronousInvocationPanic(pass, call.Fun) {
		return false
	}
	for _, operand := range call.Args {
		if ps6088NodeHasSynchronousNonreturnIIFE(pass, operand) ||
			ps6088DirectExpressionPanics(pass, operand) ||
			ps6088NodeHasSynchronousInvocationPanic(pass, operand) {
			return false
		}
	}
	return true
}

func ps6088NodeHasSynchronousInvocationPanic(pass *analysis.Pass, node ast.Node) bool {
	panics := false
	astutil.WithStack(node, func(current ast.Node, stack []ast.Node) bool {
		if panics || current == nil {
			return false
		}
		if _, literal := current.(*ast.FuncLit); literal {
			return false
		}
		call, ok := current.(*ast.CallExpr)
		if !ok {
			return true
		}
		if !ps6088StaticPathLive(pass, call, stack) || ps6088CallOperandUnevaluated(pass, call) {
			return false
		}
		if !ps6088AsyncCall(call, stack, false) && ps6088CallInvocationPanics(pass, call) {
			panics = true
			return false
		}
		return true
	})
	return panics
}

func ps6088DirectExpressionPanics(pass *analysis.Pass, expression ast.Expr) bool {
	if expression == nil {
		return false
	}
	expression = ps2110Unparen(expression)
	passIndex, ok := ps6088DirectPanics.Load(pass)
	if !ok {
		passIndex, _ = ps6088DirectPanics.LoadOrStore(pass, &ps6088DirectPanicPassIndex{})
	}
	index := passIndex.(*ps6088DirectPanicPassIndex)
	entry, ok := index.expressions.Load(expression)
	if !ok {
		entry, _ = index.expressions.LoadOrStore(expression, &ps6088DirectPanicEntry{})
	}
	result := entry.(*ps6088DirectPanicEntry)
	result.once.Do(func() {
		result.panics = ps6088ComputeDirectExpressionPanics(pass, expression)
	})
	return result.panics
}

func ps6088ComputeDirectExpressionPanics(pass *analysis.Pass, expression ast.Expr) bool {
	expression = ps2110Unparen(expression)
	switch value := expression.(type) {
	case *ast.SelectorExpr:
		return ps6088DirectExpressionPanics(pass, value.X) ||
			ps6088DirectNilSelectorPanics(pass, value)
	case *ast.IndexExpr:
		if ps6088DirectExpressionPanics(pass, value.X) ||
			ps6088DirectExpressionPanics(pass, value.Index) {
			return true
		}
		if ps6088UnhashableMapIndex(pass, value, token.NoPos, token.NoPos) {
			return true
		}
		if ps6088NilPointerOrSliceExpression(pass, value.X, pass.TypesInfo.TypeOf(value.X)) {
			return true
		}
		if ps6088IndexDefinitelyOutOfBounds(pass, value.X, value.Index) {
			return true
		}
		actual, ok := ps6088DomainExpressionActual(
			pass, value, nil, nil, false,
		)
		return ok && actual.noExecution
	case *ast.IndexListExpr:
		if ps6088DirectExpressionPanics(pass, value.X) {
			return true
		}
		for _, index := range value.Indices {
			if ps6088DirectExpressionPanics(pass, index) {
				return true
			}
		}
	case *ast.StarExpr:
		return ps6088DirectExpressionPanics(pass, value.X) ||
			ps6088NilPointerOrSliceExpression(pass, value.X, pass.TypesInfo.TypeOf(value.X))
	case *ast.CallExpr:
		if ps6088CallOperandUnevaluated(pass, value) {
			return false
		}
		if ps6088DirectExpressionPanics(pass, value.Fun) {
			return true
		}
		for _, argument := range value.Args {
			if ps6088DirectExpressionPanics(pass, argument) {
				return true
			}
		}
		if typedBuiltinName(pass, value.Fun, "delete") && len(value.Args) == 2 &&
			!value.Ellipsis.IsValid() && ps6088UnhashableMapKey(
			pass, value.Args[0], value.Args[1], token.NoPos, token.NoPos,
		) {
			return true
		}
		if ps6088InvalidMakeCall(pass, value) {
			return true
		}
		if targetLength, conversion := ps6088SliceToArrayConversion(pass, value); conversion {
			length, known := ps6088ActualSliceLength(pass, value.Args[0], nil, nil)
			return known && length < targetLength
		}
	case *ast.UnaryExpr:
		return ps6088DirectExpressionPanics(pass, value.X) ||
			(value.Op == token.ARROW &&
				ps6088NilRangeExpression(pass, value.X, pass.TypesInfo.TypeOf(value.X)))
	case *ast.BinaryExpr:
		if ps6088DirectExpressionPanics(pass, value.X) {
			return true
		}
		if left, known := ps6088Boolean(pass, value.X); known &&
			((value.Op == token.LAND && !left) || (value.Op == token.LOR && left)) {
			return false
		}
		if ps6088DirectExpressionPanics(pass, value.Y) {
			return true
		}
		if ps6088InterfaceComparisonPanics(pass, value) {
			return true
		}
		operand := ps6088SliceBound(pass, value.Y, 0, false)
		if (value.Op == token.QUO || value.Op == token.REM) && operand != nil {
			return ps6088TypeSetAll(pass.TypesInfo.TypeOf(value.Y), func(underlying types.Type) bool {
				basic, ok := underlying.(*types.Basic)
				return ok && basic.Info()&types.IsInteger != 0
			}) && constant.Sign(operand) == 0
		}
		if (value.Op == token.SHL || value.Op == token.SHR) && operand != nil {
			return constant.Sign(operand) < 0
		}
		return false
	case *ast.SliceExpr:
		for _, part := range []ast.Expr{value.X, value.Low, value.High, value.Max} {
			if part != nil && ps6088DirectExpressionPanics(pass, part) {
				return true
			}
		}
		if ps6088SliceBoundsPanic(pass, value) {
			return true
		}
		containerType := pass.TypesInfo.TypeOf(value.X)
		if containerType != nil {
			pointer, ok := types.Unalias(containerType).Underlying().(*types.Pointer)
			if ok {
				_, arrayOK := types.Unalias(pointer.Elem()).Underlying().(*types.Array)
				if arrayOK && ps6088NilPointerOrSliceExpression(pass, value.X, containerType) {
					return true
				}
			}
		}
		if !ps6088NilPointerOrSliceExpression(pass, value.X, containerType) {
			return false
		}
		for _, bound := range []ast.Expr{value.Low, value.High, value.Max} {
			if bound == nil {
				continue
			}
			integer := pass.TypesInfo.Types[bound].Value
			if integer == nil || integer.Kind() != constant.Int {
				continue
			}
			limit, exact := constant.Int64Val(integer)
			if exact && limit != 0 {
				return true
			}
		}
	case *ast.TypeAssertExpr:
		return ps6088DirectExpressionPanics(pass, value.X) ||
			ps6088DirectTypeAssertionPanics(pass, value)
	case *ast.CompositeLit:
		for _, element := range value.Elts {
			if keyed, ok := element.(*ast.KeyValueExpr); ok {
				if ps6088DirectExpressionPanics(pass, keyed.Key) ||
					ps6088DirectExpressionPanics(pass, keyed.Value) {
					return true
				}
				continue
			}
			if ps6088DirectExpressionPanics(pass, element) {
				return true
			}
		}
	}
	return false
}

func ps6088InterfaceComparisonPanics(pass *analysis.Pass, expression *ast.BinaryExpr) bool {
	if expression.Op != token.EQL && expression.Op != token.NEQ {
		return false
	}
	for _, operand := range []ast.Expr{expression.X, expression.Y} {
		typeOf := pass.TypesInfo.TypeOf(operand)
		if typeOf == nil {
			return false
		}
		if _, ok := types.Unalias(typeOf).Underlying().(*types.Interface); !ok {
			return false
		}
	}
	left, leftNil, leftKnown := ps6088StableDynamicTypeForComparison(
		pass, expression.X, expression, make(map[ast.Expr]bool),
	)
	right, rightNil, rightKnown := ps6088StableDynamicTypeForComparison(
		pass, expression.Y, expression, make(map[ast.Expr]bool),
	)
	return leftKnown && rightKnown && !leftNil && !rightNil &&
		types.Identical(left, right) && !types.Comparable(left)
}

func ps6088InvalidMakeCall(pass *analysis.Pass, call *ast.CallExpr) bool {
	if !typedBuiltinName(pass, call.Fun, "make") || len(call.Args) < 2 ||
		call.Ellipsis.IsValid() {
		return false
	}
	typeOf := pass.TypesInfo.TypeOf(call)
	if typeOf == nil {
		return false
	}
	bound := func(index int) constant.Value {
		if index >= len(call.Args) {
			return nil
		}
		return ps6088SliceBound(pass, call.Args[index], 0, false)
	}
	if ps6088TypeSetAll(typeOf, func(underlying types.Type) bool {
		_, slice := underlying.(*types.Slice)
		return slice
	}) {
		length := bound(1)
		capacity := length
		if len(call.Args) > 2 {
			capacity = bound(2)
		}
		if ps6088MakeBoundInvalid(pass, length) || ps6088MakeBoundInvalid(pass, capacity) {
			return true
		}
		return length != nil && capacity != nil &&
			constant.Compare(length, token.GTR, capacity)
	}
	if ps6088TypeSetAll(typeOf, func(underlying types.Type) bool {
		_, channel := underlying.(*types.Chan)
		return channel
	}) {
		capacity := bound(1)
		return ps6088MakeBoundInvalid(pass, capacity)
	}
	return false
}

func ps6088MakeBoundInvalid(pass *analysis.Pass, bound constant.Value) bool {
	if bound == nil {
		return false
	}
	if constant.Sign(bound) < 0 {
		return true
	}
	if pass.TypesSizes == nil {
		return false
	}
	bits := pass.TypesSizes.Sizeof(types.Typ[types.Int]) * 8
	if bits <= 1 {
		return false
	}
	maximum := constant.BinaryOp(
		constant.Shift(constant.MakeInt64(1), token.SHL, uint(bits-1)),
		token.SUB,
		constant.MakeInt64(1),
	)
	return constant.Compare(bound, token.GTR, maximum)
}

func ps6088TypeSetAll(value types.Type, matches func(types.Type) bool) bool {
	terms, universal, ok := ps6088TypeTerms(value, make(map[types.Type]bool))
	if !ok || universal || len(terms) == 0 {
		return false
	}
	for _, term := range terms {
		if !matches(types.Unalias(term.value).Underlying()) {
			return false
		}
	}
	return true
}

type ps6088TypeTerm struct {
	value types.Type
	tilde bool
}

func ps6088TypeTerms(value types.Type, seen map[types.Type]bool) ([]ps6088TypeTerm, bool, bool) {
	if value == nil {
		return nil, false, false
	}
	value = types.Unalias(value)
	if seen[value] {
		return nil, false, false
	}
	seen[value] = true
	defer delete(seen, value)
	switch typed := value.(type) {
	case *types.TypeParam:
		return ps6088TypeTerms(typed.Constraint(), seen)
	case *types.Union:
		if typed.Len() == 0 {
			return nil, false, false
		}
		terms := make([]ps6088TypeTerm, 0, typed.Len())
		for index := range typed.Len() {
			term := typed.Term(index)
			terms = ps6088AppendTypeTerm(terms, ps6088TypeTerm{
				value: types.Unalias(term.Type()),
				tilde: term.Tilde(),
			})
		}
		return terms, false, true
	}
	underlying := value.Underlying()
	if constraint, ok := underlying.(*types.Interface); ok {
		constraint.Complete()
		var terms []ps6088TypeTerm
		universal := true
		for index := range constraint.NumEmbeddeds() {
			embedded, embeddedUniversal, valid := ps6088TypeTerms(
				constraint.EmbeddedType(index), seen,
			)
			if !valid {
				return nil, false, false
			}
			terms, universal = ps6088IntersectTypeTerms(
				terms, universal, embedded, embeddedUniversal,
			)
		}
		filtered := terms[:0]
		for _, term := range terms {
			if constraint.IsComparable() && !types.Comparable(term.value) {
				continue
			}
			if !term.tilde && constraint.NumMethods() > 0 &&
				!types.Satisfies(term.value, constraint) {
				continue
			}
			filtered = append(filtered, term)
		}
		return filtered, universal, true
	}
	return []ps6088TypeTerm{{value: value}}, false, true
}

func ps6088IntersectTypeTerms(
	left []ps6088TypeTerm,
	leftUniversal bool,
	right []ps6088TypeTerm,
	rightUniversal bool,
) ([]ps6088TypeTerm, bool) {
	if leftUniversal {
		return right, rightUniversal
	}
	if rightUniversal {
		return left, false
	}
	intersection := make([]ps6088TypeTerm, 0, len(left)*len(right))
	for _, leftTerm := range left {
		for _, rightTerm := range right {
			if term, ok := ps6088IntersectTypeTerm(leftTerm, rightTerm); ok {
				intersection = ps6088AppendTypeTerm(intersection, term)
			}
		}
	}
	return intersection, false
}

func ps6088IntersectTypeTerm(left, right ps6088TypeTerm) (ps6088TypeTerm, bool) {
	left.value = types.Unalias(left.value)
	right.value = types.Unalias(right.value)
	if left.tilde && right.tilde {
		if types.Identical(left.value.Underlying(), right.value.Underlying()) {
			return left, true
		}
		return ps6088TypeTerm{}, false
	}
	if left.tilde {
		if types.Identical(left.value.Underlying(), right.value.Underlying()) {
			return right, true
		}
		return ps6088TypeTerm{}, false
	}
	if right.tilde {
		if types.Identical(left.value.Underlying(), right.value.Underlying()) {
			return left, true
		}
		return ps6088TypeTerm{}, false
	}
	return left, types.Identical(left.value, right.value)
}

func ps6088AppendTypeTerm(terms []ps6088TypeTerm, term ps6088TypeTerm) []ps6088TypeTerm {
	for _, existing := range terms {
		if existing.tilde == term.tilde && types.Identical(existing.value, term.value) {
			return terms
		}
	}
	return append(terms, term)
}

func ps6088SliceBoundsPanic(pass *analysis.Pass, expression *ast.SliceExpr) bool {
	length, lengthKnown, capacity, capacityKnown :=
		ps6088SliceContainerSizes(pass, expression.X)
	low := ps6088SliceBound(pass, expression.Low, 0, true)
	high := ps6088SliceBound(pass, expression.High, length, lengthKnown)
	maximum := ps6088SliceBound(pass, expression.Max, capacity, capacityKnown)
	for _, value := range []constant.Value{low, high, maximum} {
		if value != nil && constant.Sign(value) < 0 {
			return true
		}
	}
	if low != nil && high != nil && constant.Compare(low, token.GTR, high) {
		return true
	}
	if expression.Max != nil && high != nil && maximum != nil &&
		constant.Compare(high, token.GTR, maximum) {
		return true
	}
	if expression.Max != nil && low != nil && maximum != nil &&
		constant.Compare(low, token.GTR, maximum) {
		return true
	}
	if capacityKnown {
		limit := constant.MakeInt64(capacity)
		if low != nil && constant.Compare(low, token.GTR, limit) {
			return true
		}
		if expression.High != nil && high != nil && constant.Compare(high, token.GTR, limit) {
			return true
		}
		if expression.Max != nil && maximum != nil && constant.Compare(maximum, token.GTR, limit) {
			return true
		}
	}
	return false
}

func ps6088SliceBound(
	pass *analysis.Pass,
	expression ast.Expr,
	fallback int64,
	fallbackKnown bool,
) constant.Value {
	if expression == nil {
		if fallbackKnown {
			return constant.MakeInt64(fallback)
		}
		return nil
	}
	resolved, zero, safe := ps6088StableExpression(pass, expression)
	if !safe {
		return nil
	}
	if zero {
		return constant.MakeInt64(0)
	}
	expression = resolved
	integer := pass.TypesInfo.Types[expression].Value
	if integer == nil || integer.Kind() != constant.Int {
		return nil
	}
	return integer
}

func ps6088SliceContainerSizes(
	pass *analysis.Pass,
	expression ast.Expr,
) (int64, bool, int64, bool) {
	resolved, zero, safe := ps6088StableExpression(pass, expression)
	if safe {
		expression = resolved
	}
	typeOf := pass.TypesInfo.TypeOf(expression)
	if typeOf == nil {
		return 0, false, 0, false
	}
	underlying := types.Unalias(typeOf).Underlying()
	if pointer, ok := underlying.(*types.Pointer); ok {
		underlying = types.Unalias(pointer.Elem()).Underlying()
	}
	if _, slice := underlying.(*types.Slice); slice && zero {
		return 0, true, 0, true
	}
	if sliced, ok := ps2110Unparen(expression).(*ast.SliceExpr); ok {
		length, lengthKnown, capacity, capacityKnown :=
			ps6088SliceContainerSizes(pass, sliced.X)
		low := ps6088SliceBound(pass, sliced.Low, 0, true)
		high := ps6088SliceBound(pass, sliced.High, length, lengthKnown)
		maximum := ps6088SliceBound(pass, sliced.Max, capacity, capacityKnown)
		resultLength, resultLengthKnown := ps6088SliceSizeDifference(high, low)
		if basic, ok := underlying.(*types.Basic); ok && basic.Info()&types.IsString != 0 {
			return resultLength, resultLengthKnown, resultLength, resultLengthKnown
		}
		capacityBound := constant.Value(nil)
		if sliced.Max != nil {
			capacityBound = maximum
		} else if capacityKnown {
			capacityBound = constant.MakeInt64(capacity)
		}
		resultCapacity, resultCapacityKnown := ps6088SliceSizeDifference(capacityBound, low)
		return resultLength, resultLengthKnown, resultCapacity, resultCapacityKnown
	}
	switch value := underlying.(type) {
	case *types.Array:
		return value.Len(), true, value.Len(), true
	case *types.Basic:
		if value.Info()&types.IsString == 0 {
			return 0, false, 0, false
		}
		text := pass.TypesInfo.Types[expression].Value
		if text == nil || text.Kind() != constant.String {
			return 0, false, 0, false
		}
		length := int64(len(constant.StringVal(text)))
		return length, true, length, true
	case *types.Slice:
		length, lengthKnown := ps6088ActualSliceLength(pass, expression, nil, nil)
		capacity, capacityKnown := ps6088ActualSliceCapacity(pass, expression)
		return length, lengthKnown, capacity, capacityKnown
	}
	return 0, false, 0, false
}

func ps6088SliceSizeDifference(high, low constant.Value) (int64, bool) {
	if high == nil || low == nil {
		return 0, false
	}
	difference := constant.BinaryOp(high, token.SUB, low)
	if difference.Kind() != constant.Int || constant.Sign(difference) < 0 {
		return 0, false
	}
	return constant.Int64Val(difference)
}

func ps6088ActualSliceCapacity(pass *analysis.Pass, expression ast.Expr) (int64, bool) {
	expression = ps6088UnwrapCollectionConversions(pass, expression)
	if ps6088NilRangeExpression(pass, expression, pass.TypesInfo.TypeOf(expression)) {
		return 0, true
	}
	if call, ok := ps2110Unparen(expression).(*ast.CallExpr); ok &&
		typedBuiltinName(pass, call.Fun, "make") && len(call.Args) >= 2 {
		argument := call.Args[1]
		if len(call.Args) > 2 {
			argument = call.Args[2]
		}
		value := ps6088SliceBound(pass, argument, 0, false)
		if value != nil {
			return constant.Int64Val(value)
		}
	}
	literal := ps6088CompositeExpression(pass, expression)
	if literal == nil {
		return 0, false
	}
	return ps6088CompositeLength(pass, literal)
}

func ps6088DirectTypeAssertionPanics(pass *analysis.Pass, assertion *ast.TypeAssertExpr) bool {
	if assertion.Type == nil {
		return false
	}
	interfaceType := pass.TypesInfo.TypeOf(assertion.X)
	if interfaceType == nil {
		return false
	}
	if _, ok := types.Unalias(interfaceType).Underlying().(*types.Interface); !ok {
		return false
	}
	dynamicType, nilInterface, known := ps6088StableDynamicType(
		pass, assertion.X, make(map[ast.Expr]bool),
	)
	if !known {
		return false
	}
	if nilInterface {
		return true
	}
	assertedType := pass.TypesInfo.TypeOf(assertion.Type)
	if dynamicType == nil || assertedType == nil {
		return false
	}
	if ps6088ContainsTypeParameter(assertedType, make(map[types.Type]bool)) ||
		ps6088ContainsTypeParameter(dynamicType, make(map[types.Type]bool)) {
		return false
	}
	if _, uncertain := types.Unalias(dynamicType).Underlying().(*types.Interface); uncertain {
		return false
	}
	dynamicType = types.Default(dynamicType)
	if dynamicType == nil {
		return false
	}
	if assertedInterface, ok := types.Unalias(assertedType).Underlying().(*types.Interface); ok {
		return !types.Implements(dynamicType, assertedInterface)
	}
	return !types.Identical(dynamicType, assertedType)
}

func ps6088ContainsTypeParameter(value types.Type, seen map[types.Type]bool) bool {
	if value == nil || seen[value] {
		return false
	}
	seen[value] = true
	switch typed := value.(type) {
	case *types.TypeParam:
		return true
	case *types.Alias:
		return ps6088ContainsTypeParameter(types.Unalias(typed), seen)
	case *types.Named:
		if arguments := typed.TypeArgs(); arguments != nil {
			for index := range arguments.Len() {
				if ps6088ContainsTypeParameter(arguments.At(index), seen) {
					return true
				}
			}
		}
		return ps6088ContainsTypeParameter(typed.Underlying(), seen)
	case *types.Pointer:
		return ps6088ContainsTypeParameter(typed.Elem(), seen)
	case *types.Slice:
		return ps6088ContainsTypeParameter(typed.Elem(), seen)
	case *types.Array:
		return ps6088ContainsTypeParameter(typed.Elem(), seen)
	case *types.Map:
		return ps6088ContainsTypeParameter(typed.Key(), seen) ||
			ps6088ContainsTypeParameter(typed.Elem(), seen)
	case *types.Chan:
		return ps6088ContainsTypeParameter(typed.Elem(), seen)
	case *types.Struct:
		for index := range typed.NumFields() {
			if ps6088ContainsTypeParameter(typed.Field(index).Type(), seen) {
				return true
			}
		}
	case *types.Tuple:
		for index := range typed.Len() {
			if ps6088ContainsTypeParameter(typed.At(index).Type(), seen) {
				return true
			}
		}
	case *types.Signature:
		if typed.Recv() != nil &&
			ps6088ContainsTypeParameter(typed.Recv().Type(), seen) {
			return true
		}
		return ps6088ContainsTypeParameter(typed.Params(), seen) ||
			ps6088ContainsTypeParameter(typed.Results(), seen)
	case *types.Interface:
		typed.Complete()
		for index := range typed.NumMethods() {
			if ps6088ContainsTypeParameter(typed.Method(index).Type(), seen) {
				return true
			}
		}
		for index := range typed.NumEmbeddeds() {
			if ps6088ContainsTypeParameter(typed.EmbeddedType(index), seen) {
				return true
			}
		}
	case *types.Union:
		for index := range typed.Len() {
			if ps6088ContainsTypeParameter(typed.Term(index).Type(), seen) {
				return true
			}
		}
	}
	return false
}

func ps6088StableDynamicType(
	pass *analysis.Pass,
	expression ast.Expr,
	seen map[ast.Expr]bool,
) (types.Type, bool, bool) {
	return ps6088StableDynamicTypeAt(pass, expression, seen, expression.Pos())
}

func ps6088StableDynamicTypeAt(
	pass *analysis.Pass,
	expression ast.Expr,
	seen map[ast.Expr]bool,
	position token.Pos,
) (types.Type, bool, bool) {
	if typeOf, known := ps6088ConcreteDynamicType(pass, expression); known {
		return typeOf, false, true
	}
	resolved, zero, safe := ps6088StableExpressionAt(pass, expression, position)
	return ps6088ResolvedDynamicType(
		pass, resolved, zero, safe, seen, nil, token.NoPos, false,
		false, token.NoPos, token.NoPos,
	)
}

func ps6088StableDynamicTypeForComparison(
	pass *analysis.Pass,
	expression ast.Expr,
	comparison *ast.BinaryExpr,
	seen map[ast.Expr]bool,
) (types.Type, bool, bool) {
	return ps6088StableDynamicTypeForComparisonAt(
		pass, expression, comparison, seen, comparison.Pos(), true,
	)
}

func ps6088StableDynamicTypeForComparisonAt(
	pass *analysis.Pass,
	expression ast.Expr,
	comparison *ast.BinaryExpr,
	seen map[ast.Expr]bool,
	position token.Pos,
	wholeRuntime bool,
) (types.Type, bool, bool) {
	if typeOf, known := ps6088ConcreteDynamicType(pass, expression); known {
		return typeOf, false, true
	}
	resolved, zero, safe, resolvedPosition, resolvedWholeRuntime := ps6088StableExpressionForComparisonAt(
		pass, expression, position, wholeRuntime,
	)
	return ps6088ResolvedDynamicType(
		pass, resolved, zero, safe, seen, comparison, resolvedPosition, resolvedWholeRuntime,
		true, token.NoPos, token.NoPos,
	)
}

func ps6088StableDynamicTypeForSnapshot(
	pass *analysis.Pass,
	expression ast.Expr,
	seen map[ast.Expr]bool,
	ignoredStart token.Pos,
	ignoredEnd token.Pos,
) (types.Type, bool, bool) {
	return ps6088StableDynamicTypeForRuntimeAt(
		pass, expression, nil, seen, expression.Pos(), true, ignoredStart, ignoredEnd,
	)
}

func ps6088StableDynamicTypeForRuntimeAt(
	pass *analysis.Pass,
	expression ast.Expr,
	comparison *ast.BinaryExpr,
	seen map[ast.Expr]bool,
	position token.Pos,
	wholeRuntime bool,
	ignoredStart token.Pos,
	ignoredEnd token.Pos,
) (types.Type, bool, bool) {
	if typeOf, known := ps6088ConcreteDynamicType(pass, expression); known {
		return typeOf, false, true
	}
	resolved, zero, safe, resolvedPosition, resolvedWholeRuntime := ps6088StableExpressionForRuntimeAt(
		pass, expression, position, wholeRuntime, ignoredStart, ignoredEnd,
	)
	return ps6088ResolvedDynamicType(
		pass, resolved, zero, safe, seen, comparison, resolvedPosition, resolvedWholeRuntime,
		true, ignoredStart, ignoredEnd,
	)
}

func ps6088ConcreteDynamicType(pass *analysis.Pass, expression ast.Expr) (types.Type, bool) {
	typeOf := pass.TypesInfo.TypeOf(expression)
	if typeOf == nil || ps6088ContainsTypeParameter(typeOf, make(map[types.Type]bool)) {
		return nil, false
	}
	underlying := types.Unalias(typeOf).Underlying()
	if _, ok := underlying.(*types.Interface); ok {
		return nil, false
	}
	if basic, ok := underlying.(*types.Basic); ok && basic.Kind() == types.UntypedNil {
		return nil, false
	}
	return typeOf, true
}

func ps6088ResolvedDynamicType(
	pass *analysis.Pass,
	resolved ast.Expr,
	zero, safe bool,
	seen map[ast.Expr]bool,
	comparison *ast.BinaryExpr,
	comparisonPosition token.Pos,
	comparisonWholeRuntime bool,
	runtimeAware bool,
	ignoredStart token.Pos,
	ignoredEnd token.Pos,
) (types.Type, bool, bool) {
	resolved = ps2110Unparen(resolved)
	if !safe || seen[resolved] {
		return nil, false, false
	}
	seen[resolved] = true
	typeOf := pass.TypesInfo.TypeOf(resolved)
	if typeOf == nil {
		return nil, false, false
	}
	if ps6088ContainsTypeParameter(typeOf, make(map[types.Type]bool)) {
		return nil, false, false
	}
	if zero {
		if basic, ok := types.Unalias(typeOf).Underlying().(*types.Basic); ok &&
			basic.Kind() == types.UntypedNil {
			return nil, true, true
		}
	}
	if _, ok := types.Unalias(typeOf).Underlying().(*types.Interface); !ok {
		return typeOf, false, true
	}
	if zero {
		return nil, true, true
	}
	conversion, ok := resolved.(*ast.CallExpr)
	if !ok || len(conversion.Args) != 1 || conversion.Ellipsis.IsValid() ||
		!pass.TypesInfo.Types[conversion.Fun].IsType() {
		return nil, false, false
	}
	if runtimeAware {
		insideComparison := comparison != nil &&
			comparison.Pos() <= resolved.Pos() && resolved.End() <= comparison.End()
		position := comparisonPosition
		wholeRuntime := comparisonWholeRuntime
		if comparison != nil && insideComparison {
			position = comparison.Pos()
			wholeRuntime = true
		}
		return ps6088StableDynamicTypeForRuntimeAt(
			pass, conversion.Args[0], comparison, seen, position, wholeRuntime,
			ignoredStart, ignoredEnd,
		)
	}
	return ps6088StableDynamicType(pass, conversion.Args[0], seen)
}

func ps6088CallInvocationPanics(pass *analysis.Pass, call *ast.CallExpr) bool {
	return ps6088NilFunctionExpression(pass, call.Fun, pass.TypesInfo.TypeOf(call.Fun)) ||
		(typedBuiltinName(pass, call.Fun, "close") && len(call.Args) == 1 &&
			!call.Ellipsis.IsValid() &&
			(ps6088NilRangeExpression(pass, call.Args[0], pass.TypesInfo.TypeOf(call.Args[0])) ||
				ps6088DefinitelyClosedChannel(pass, call, call.Args[0])))
}

func ps6088DefinitelyClosedChannel(
	pass *analysis.Pass,
	consumer ast.Node,
	expression ast.Expr,
) bool {
	identifier, ok := ps2110Unparen(expression).(*ast.Ident)
	if !ok {
		return false
	}
	object := pass.TypesInfo.ObjectOf(identifier)
	if object == nil || ps6088PackageVariable(object) {
		return false
	}
	typeOf := object.Type()
	if typeOf == nil {
		return false
	}
	if _, channel := types.Unalias(typeOf).Underlying().(*types.Chan); !channel {
		return false
	}
	fn := ps6088LocalObjectOwner(pass, object)
	if fn == nil {
		return false
	}
	index := ps6088FunctionLocalDomainIndex(pass, fn)
	path := index.closePaths[consumer]
	if len(path) == 0 {
		return false
	}
	if index.hasGoto {
		return false
	}
	for _, write := range index.dynamicTypeWrites[object] {
		send, sendStatement := consumer.(*ast.SendStmt)
		if sendStatement && send.Value.Pos() <= write.node && write.node < send.Value.End() {
			// A send snapshots its channel operand before evaluating its value.
			// Rebinding the identifier while evaluating the value therefore cannot
			// change which (already closed) channel this send uses.
			continue
		}
		if sendStatement {
			ignored := false
			for _, snapshotRange := range index.sendSnapshotRanges[send] {
				if snapshotRange.start <= write.node && write.node < snapshotRange.end {
					ignored = true
					break
				}
			}
			if ignored {
				continue
			}
		}
		return false
	}
	for _, position := range path {
		closes := index.directCloses[position.block][object]
		closeIndex, _ := slices.BinarySearchFunc(closes, position.index, func(
			close ps6088DirectClose,
			statementIndex int,
		) int {
			return close.index - statementIndex
		})
		if closeIndex > 0 {
			return true
		}
	}
	return false
}

func ps6088DirectCloseCall(
	pass *analysis.Pass,
	statement ast.Stmt,
) *ast.CallExpr {
	expression, ok := statement.(*ast.ExprStmt)
	if !ok {
		return nil
	}
	call, ok := ps2110Unparen(expression.X).(*ast.CallExpr)
	if !ok || !typedBuiltinName(pass, call.Fun, "close") || len(call.Args) != 1 ||
		call.Ellipsis.IsValid() {
		return nil
	}
	return call
}

func ps6088DirectNilSelectorPanics(pass *analysis.Pass, selector *ast.SelectorExpr) bool {
	typeOf := pass.TypesInfo.TypeOf(selector.X)
	if !ps6088NilPointerOrSliceExpression(pass, selector.X, typeOf) {
		return false
	}
	selection := pass.TypesInfo.Selections[selector]
	if selection == nil {
		return false
	}
	switch selection.Kind() {
	case types.FieldVal:
		return true
	case types.MethodVal:
		method, ok := selection.Obj().(*types.Func)
		if !ok {
			return false
		}
		signature, ok := method.Type().(*types.Signature)
		if !ok || signature.Recv() == nil {
			return false
		}
		_, pointerReceiver := types.Unalias(signature.Recv().Type()).Underlying().(*types.Pointer)
		return !pointerReceiver
	}
	return false
}

func ps6088EagerStatementSuffixReturns(
	pass *analysis.Pass,
	positionCall *ast.CallExpr,
	stack []ast.Node,
) bool {
	for index := len(stack) - 1; index >= 0; index-- {
		if _, boundary := stack[index].(*ast.FuncLit); boundary {
			return false
		}
		statement, ok := stack[index].(ast.Stmt)
		if !ok {
			continue
		}
		if !ps6088StatementReturnsNormally(pass, statement, ps6088SelectCommStatement(statement, stack)) {
			return false
		}
		break
	}
	for index := 0; index+1 < len(stack); index++ {
		statement, ok := stack[index].(ast.Stmt)
		if !ok || !astutil.IsLoop(statement) || astutil.LoopBody(statement) != stack[index+1] {
			continue
		}
		outcomes, found := ps6088TargetSuffixOutcomes(
			pass, astutil.LoopBody(statement).List, positionCall,
			ps6088StatementLabel(pass, statement), statement, statement,
		)
		atMostOne, known := ps6088LoopAtMost(pass, statement, 1)
		loopMayRepeat := !known || !atMostOne
		if found && (outcomes.next || outcomes.intraBodyCycle ||
			loopMayRepeat && (outcomes.fallthroughPossible || outcomes.localNext)) {
			return true
		}
	}
	return false
}

func ps6088TargetSuffixOutcomes(
	pass *analysis.Pass,
	statements []ast.Stmt,
	target ast.Node,
	loopLabel string,
	breakTarget ast.Stmt,
	continueTarget ast.Stmt,
) (ps6088LoopOutcomes, bool) {
	for index, statement := range statements {
		if statement.Pos() > target.Pos() || target.End() > statement.End() {
			continue
		}
		inside := ps6088TargetStatementOutcome(
			pass, statement, target, loopLabel, breakTarget, continueTarget,
		)
		if inside.fallthroughPossible {
			suffix := ps6088TargetControlOutcomes(
				pass, statements[index+1:], loopLabel, breakTarget, continueTarget,
			)
			inside.fallthroughPossible = suffix.fallthroughPossible
			inside.next = inside.next || suffix.next
			inside.intraBodyCycle = inside.intraBodyCycle || suffix.intraBodyCycle
			inside.breaks = inside.breaks || suffix.breaks
			ps6088MergeTargetExits(&inside, suffix)
			inside.localNext = inside.localNext || suffix.localNext
		}
		return inside, true
	}
	return ps6088LoopOutcomes{}, false
}

func ps6088TargetStatementOutcome(
	pass *analysis.Pass,
	statement ast.Stmt,
	target ast.Node,
	loopLabel string,
	breakTarget ast.Stmt,
	continueTarget ast.Stmt,
) ps6088LoopOutcomes {
	switch value := statement.(type) {
	case *ast.BlockStmt:
		if outcomes, found := ps6088TargetSuffixOutcomes(
			pass, value.List, target, loopLabel, breakTarget, continueTarget,
		); found {
			return outcomes
		}
	case *ast.LabeledStmt:
		return ps6088TargetStatementOutcome(
			pass, value.Stmt, target, loopLabel, breakTarget, continueTarget,
		)
	case *ast.IfStmt:
		if value.Body.Pos() <= target.Pos() && target.End() <= value.Body.End() {
			if outcomes, found := ps6088TargetSuffixOutcomes(
				pass, value.Body.List, target, loopLabel, breakTarget, continueTarget,
			); found {
				return outcomes
			}
		}
		if value.Else != nil && value.Else.Pos() <= target.Pos() && target.End() <= value.Else.End() {
			return ps6088TargetStatementOutcome(
				pass, value.Else, target, loopLabel, breakTarget, continueTarget,
			)
		}
	case *ast.SwitchStmt:
		for index, item := range value.Body.List {
			clause, ok := item.(*ast.CaseClause)
			if !ok {
				break
			}
			outcomes, found := ps6088TargetSuffixOutcomes(
				pass, clause.Body, target, loopLabel, value, continueTarget,
			)
			if !found {
				continue
			}
			if outcomes.fallthroughPossible && ps6088ClauseFallsThrough(clause) && index+1 < len(value.Body.List) {
				clauses := make([]*ast.CaseClause, 0, len(value.Body.List))
				for _, candidate := range value.Body.List {
					selected, ok := candidate.(*ast.CaseClause)
					if !ok {
						return ps6088LoopOutcomes{fallthroughPossible: true}
					}
					clauses = append(clauses, selected)
				}
				next := ps6088TargetSelectedClauseOutcome(
					pass, value, clauses, index+1, loopLabel, continueTarget,
				)
				outcomes.fallthroughPossible = next.fallthroughPossible
				outcomes.next = outcomes.next || next.next
				outcomes.intraBodyCycle = outcomes.intraBodyCycle || next.intraBodyCycle
				ps6088MergeTargetExits(&outcomes, next)
				outcomes.localNext = outcomes.localNext || next.localNext
			}
			exit := ps6088ConsumeTargetExit(&outcomes, value)
			outcomes.fallthroughPossible = outcomes.fallthroughPossible || outcomes.breaks || exit
			outcomes.breaks = false
			return outcomes
		}
	case *ast.TypeSwitchStmt:
		for _, item := range value.Body.List {
			clause, ok := item.(*ast.CaseClause)
			if !ok {
				break
			}
			if outcomes, found := ps6088TargetSuffixOutcomes(
				pass, clause.Body, target, loopLabel, value, continueTarget,
			); found {
				exit := ps6088ConsumeTargetExit(&outcomes, value)
				outcomes.fallthroughPossible = outcomes.fallthroughPossible || outcomes.breaks || exit
				outcomes.breaks = false
				return outcomes
			}
		}
	case *ast.SelectStmt:
		for _, item := range value.Body.List {
			clause, ok := item.(*ast.CommClause)
			if !ok {
				break
			}
			if outcomes, found := ps6088TargetSuffixOutcomes(
				pass, clause.Body, target, loopLabel, value, continueTarget,
			); found {
				exit := ps6088ConsumeTargetExit(&outcomes, value)
				outcomes.fallthroughPossible = outcomes.fallthroughPossible || outcomes.breaks || exit
				outcomes.breaks = false
				return outcomes
			}
			if clause.Comm == nil || clause.Comm.Pos() > target.Pos() || target.End() > clause.Comm.End() {
				continue
			}
			if assignment, ok := clause.Comm.(*ast.AssignStmt); ok {
				for index, destination := range assignment.Lhs {
					if destination.Pos() > target.Pos() || target.End() > destination.End() {
						continue
					}
					for _, remaining := range assignment.Lhs[index+1:] {
						if !ps6088AssignmentDestinationReturns(pass, remaining) {
							return ps6088LoopOutcomes{}
						}
					}
					outcomes := ps6088TargetControlOutcomes(
						pass, clause.Body, loopLabel, value, continueTarget,
					)
					exit := ps6088ConsumeTargetExit(&outcomes, value)
					outcomes.fallthroughPossible = outcomes.fallthroughPossible || outcomes.breaks || exit
					outcomes.breaks = false
					return outcomes
				}
			}
			return ps6088TargetSelectOutcome(pass, value, loopLabel, continueTarget)
		}
	case *ast.ForStmt, *ast.RangeStmt:
		body := astutil.LoopBody(value)
		if body == nil || body.Pos() > target.Pos() || target.End() > body.End() {
			break
		}
		outcomes, found := ps6088TargetSuffixOutcomes(
			pass, body.List, target, ps6088StatementLabel(pass, value), value, value,
		)
		if !found {
			break
		}
		exit := ps6088ConsumeTargetExit(&outcomes, value)
		completed := outcomes.breaks || exit
		outcomes.breaks = false
		normal := outcomes.fallthroughPossible || outcomes.localNext
		infinite := false
		if loop, ok := value.(*ast.ForStmt); ok {
			if !ps6088OptionalSimpleStatementReturns(pass, loop.Post) {
				normal = false
			}
			infinite = ps6086ProvablyInfiniteLoop(pass, loop)
		}
		outcomes.fallthroughPossible = completed || normal && !infinite
		outcomes.localNext = false
		atMostOne, known := ps6088LoopAtMost(pass, value, 1)
		outcomes.intraBodyCycle = outcomes.intraBodyCycle || normal && (!known || !atMostOne)
		return outcomes
	}
	return ps6088StatementOutcome(pass, statement, loopLabel)
}

func ps6088TargetControlOutcomes(
	pass *analysis.Pass,
	statements []ast.Stmt,
	loopLabel string,
	breakTarget ast.Stmt,
	continueTarget ast.Stmt,
) ps6088LoopOutcomes {
	outcomes := ps6088LoopOutcomes{fallthroughPossible: true}
	for _, statement := range statements {
		if !outcomes.fallthroughPossible {
			break
		}
		current := ps6088TargetControlOutcome(pass, statement, loopLabel, breakTarget, continueTarget)
		outcomes.next = outcomes.next || current.next
		outcomes.intraBodyCycle = outcomes.intraBodyCycle || current.intraBodyCycle
		outcomes.breaks = outcomes.breaks || current.breaks
		ps6088MergeTargetExits(&outcomes, current)
		outcomes.localNext = outcomes.localNext || current.localNext
		outcomes.fallthroughPossible = current.fallthroughPossible
	}
	return outcomes
}

func ps6088TargetControlOutcome(
	pass *analysis.Pass,
	statement ast.Stmt,
	loopLabel string,
	breakTarget ast.Stmt,
	continueTarget ast.Stmt,
) ps6088LoopOutcomes {
	if !ps6088TargetStatementHeaderReturns(pass, statement) {
		return ps6088LoopOutcomes{}
	}
	switch statement.(type) {
	case *ast.ExprStmt, *ast.AssignStmt, *ast.DeclStmt, *ast.IncDecStmt, *ast.SendStmt,
		*ast.GoStmt, *ast.DeferStmt:
		if !ps6088StatementReturnsNormally(pass, statement, false) {
			return ps6088LoopOutcomes{}
		}
	}
	switch value := statement.(type) {
	case *ast.BlockStmt:
		return ps6088TargetControlOutcomes(pass, value.List, loopLabel, breakTarget, continueTarget)
	case *ast.LabeledStmt:
		return ps6088TargetControlOutcome(pass, value.Stmt, loopLabel, breakTarget, continueTarget)
	case *ast.BranchStmt:
		switch value.Tok {
		case token.CONTINUE:
			if continueTarget != nil &&
				(value.Label == nil || ps6088LabeledBreakTargets(pass, value, continueTarget)) {
				return ps6088LoopOutcomes{localNext: true}
			}
			// An external continue can only target another enclosing loop. The
			// final CFG cycle proof determines whether it reaches this call.
			return ps6088LoopOutcomes{next: true}
		case token.GOTO:
			return ps6088LoopOutcomes{intraBodyCycle: true}
		case token.BREAK:
			if value.Label == nil || ps6088LabeledBreakTargets(pass, value, breakTarget) {
				return ps6088LoopOutcomes{breaks: true}
			}
			// Preserve a labeled exit from an enclosing construct. Its exact CFG
			// edge must still return to the call before repetition is accepted.
			target := ps6088LabeledBranchTarget(pass, value)
			if target == nil {
				return ps6088LoopOutcomes{}
			}
			return ps6088LoopOutcomes{exitTargets: map[ast.Stmt]struct{}{target: {}}}
		}
	case *ast.ReturnStmt:
		return ps6088LoopOutcomes{}
	case *ast.IfStmt:
		if condition, known := ps6088Boolean(pass, value.Cond); known {
			if condition {
				return ps6088TargetControlOutcomes(
					pass, value.Body.List, loopLabel, breakTarget, continueTarget,
				)
			}
			return ps6088TargetElseOutcome(pass, value.Else, loopLabel, breakTarget, continueTarget)
		}
		body := ps6088TargetControlOutcomes(
			pass, value.Body.List, loopLabel, breakTarget, continueTarget,
		)
		other := ps6088TargetElseOutcome(pass, value.Else, loopLabel, breakTarget, continueTarget)
		outcomes := ps6088LoopOutcomes{
			fallthroughPossible: body.fallthroughPossible || other.fallthroughPossible,
			next:                body.next || other.next,
			intraBodyCycle:      body.intraBodyCycle || other.intraBodyCycle,
			breaks:              body.breaks || other.breaks,
			localNext:           body.localNext || other.localNext,
		}
		ps6088MergeTargetExits(&outcomes, body)
		ps6088MergeTargetExits(&outcomes, other)
		return outcomes
	case *ast.SwitchStmt:
		return ps6088TargetSwitchOutcome(pass, value, loopLabel, continueTarget)
	case *ast.TypeSwitchStmt:
		return ps6088TargetTypeSwitchOutcome(pass, value, loopLabel, continueTarget)
	case *ast.SelectStmt:
		return ps6088TargetSelectOutcome(pass, value, loopLabel, continueTarget)
	case *ast.ForStmt, *ast.RangeStmt:
		return ps6088TargetLoopOutcome(pass, statement, loopLabel)
	}
	return ps6088StatementOutcome(pass, statement, loopLabel)
}

func ps6088TargetStatementHeaderReturns(pass *analysis.Pass, statement ast.Stmt) bool {
	switch value := statement.(type) {
	case *ast.ForStmt:
		return ps6088OptionalSimpleStatementReturns(pass, value.Init) &&
			(value.Cond == nil || ps6088ExpressionReturnsNormally(pass, value.Cond))
	case *ast.RangeStmt:
		return ps6088RangeHeaderReturns(pass, value)
	default:
		return ps6088StatementHeaderReturns(pass, statement)
	}
}

func ps6088TargetElseOutcome(
	pass *analysis.Pass,
	statement ast.Stmt,
	loopLabel string,
	breakTarget ast.Stmt,
	continueTarget ast.Stmt,
) ps6088LoopOutcomes {
	if statement == nil {
		return ps6088LoopOutcomes{fallthroughPossible: true}
	}
	return ps6088TargetControlOutcome(pass, statement, loopLabel, breakTarget, continueTarget)
}

func ps6088TargetLoopOutcome(
	pass *analysis.Pass,
	statement ast.Stmt,
	loopLabel string,
) ps6088LoopOutcomes {
	if empty, known := ps6088LoopAtMost(pass, statement, 0); known && empty {
		return ps6088LoopOutcomes{fallthroughPossible: true}
	}
	if loop, ok := statement.(*ast.RangeStmt); ok && loop.Tok == token.ASSIGN &&
		ps6088LoopDefinitelyNonempty(pass, loop) {
		for _, destination := range []ast.Expr{loop.Key, loop.Value} {
			if !ps6088AssignmentDestinationReturns(pass, destination) {
				return ps6088LoopOutcomes{}
			}
		}
	}
	body := astutil.LoopBody(statement)
	if body == nil {
		return ps6088LoopOutcomes{fallthroughPossible: true}
	}
	current := ps6088TargetControlOutcomes(
		pass, body.List, ps6088StatementLabel(pass, statement), statement, statement,
	)
	exit := ps6088ConsumeTargetExit(&current, statement)
	current.fallthroughPossible = current.fallthroughPossible || current.breaks || exit
	current.breaks = false
	if loop, ok := statement.(*ast.ForStmt); ok &&
		!ps6088OptionalSimpleStatementReturns(pass, loop.Post) {
		current.fallthroughPossible = false
		current.localNext = false
	}
	if ps6088LoopDefinitelyNonempty(pass, statement) && !ps6088LoopHasReachableBreak(pass, statement) {
		if !current.fallthroughPossible && !current.localNext && !current.next &&
			len(current.exitTargets) == 0 && !current.intraBodyCycle {
			return current
		}
		if current.fallthroughPossible || current.localNext {
			current.intraBodyCycle = true
		}
		current.fallthroughPossible = false
		current.localNext = false
		return current
	}
	current.fallthroughPossible = true
	current.localNext = false
	return current
}

func ps6088TargetSwitchOutcome(
	pass *analysis.Pass,
	statement *ast.SwitchStmt,
	loopLabel string,
	continueTarget ast.Stmt,
) ps6088LoopOutcomes {
	clauses := make([]*ast.CaseClause, 0, len(statement.Body.List))
	defaultIndex := -1
	for _, item := range statement.Body.List {
		clause, ok := item.(*ast.CaseClause)
		if !ok {
			return ps6088LoopOutcomes{fallthroughPossible: true}
		}
		clauses = append(clauses, clause)
		if len(clause.List) == 0 {
			defaultIndex = len(clauses) - 1
		}
	}
	tag := constant.MakeBool(true)
	if statement.Tag != nil {
		tag = pass.TypesInfo.Types[statement.Tag].Value
	}
	if tag == nil {
		return ps6088TargetPossibleClauseOutcomes(
			pass, statement, clauses, defaultIndex, loopLabel, continueTarget,
		)
	}
	for index, clause := range clauses {
		for _, expression := range clause.List {
			value := pass.TypesInfo.Types[expression].Value
			if value == nil {
				return ps6088TargetPossibleClauseOutcomes(
					pass, statement, clauses, defaultIndex, loopLabel, continueTarget,
				)
			}
			if constant.Compare(tag, token.EQL, value) {
				return ps6088TargetSelectedClauseOutcome(
					pass, statement, clauses, index, loopLabel, continueTarget,
				)
			}
		}
	}
	if defaultIndex < 0 {
		return ps6088LoopOutcomes{fallthroughPossible: true}
	}
	return ps6088TargetSelectedClauseOutcome(
		pass, statement, clauses, defaultIndex, loopLabel, continueTarget,
	)
}

func ps6088TargetTypeSwitchOutcome(
	pass *analysis.Pass,
	statement *ast.TypeSwitchStmt,
	loopLabel string,
	continueTarget ast.Stmt,
) ps6088LoopOutcomes {
	clauses := make([]*ast.CaseClause, 0, len(statement.Body.List))
	defaultIndex := -1
	for _, item := range statement.Body.List {
		clause, ok := item.(*ast.CaseClause)
		if !ok {
			return ps6088LoopOutcomes{fallthroughPossible: true}
		}
		clauses = append(clauses, clause)
		if len(clause.List) == 0 {
			defaultIndex = len(clauses) - 1
		}
	}
	return ps6088TargetPossibleClauseOutcomes(
		pass, statement, clauses, defaultIndex, loopLabel, continueTarget,
	)
}

func ps6088TargetPossibleClauseOutcomes(
	pass *analysis.Pass,
	statement ast.Stmt,
	clauses []*ast.CaseClause,
	defaultIndex int,
	loopLabel string,
	continueTarget ast.Stmt,
) ps6088LoopOutcomes {
	outcome := ps6088LoopOutcomes{fallthroughPossible: defaultIndex < 0}
	for index := range clauses {
		current := ps6088TargetSelectedClauseOutcome(
			pass, statement, clauses, index, loopLabel, continueTarget,
		)
		outcome.fallthroughPossible = outcome.fallthroughPossible || current.fallthroughPossible
		outcome.next = outcome.next || current.next
		outcome.intraBodyCycle = outcome.intraBodyCycle || current.intraBodyCycle
		outcome.breaks = outcome.breaks || current.breaks
		ps6088MergeTargetExits(&outcome, current)
		outcome.localNext = outcome.localNext || current.localNext
	}
	return outcome
}

func ps6088TargetSelectedClauseOutcome(
	pass *analysis.Pass,
	statement ast.Stmt,
	clauses []*ast.CaseClause,
	index int,
	loopLabel string,
	continueTarget ast.Stmt,
) ps6088LoopOutcomes {
	result := ps6088TargetControlOutcomes(
		pass, clauses[index].Body, loopLabel, statement, continueTarget,
	)
	exit := ps6088ConsumeTargetExit(&result, statement)
	breaks := result.breaks || exit
	result.breaks = false
	if ps6088ClauseFallsThrough(clauses[index]) && result.fallthroughPossible && index+1 < len(clauses) {
		next := ps6088TargetSelectedClauseOutcome(
			pass, statement, clauses, index+1, loopLabel, continueTarget,
		)
		next.fallthroughPossible = breaks || next.fallthroughPossible
		next.next = result.next || next.next
		next.intraBodyCycle = result.intraBodyCycle || next.intraBodyCycle
		ps6088MergeTargetExits(&next, result)
		next.localNext = result.localNext || next.localNext
		return next
	}
	result.fallthroughPossible = breaks || result.fallthroughPossible
	return result
}

func ps6088TargetSelectOutcome(
	pass *analysis.Pass,
	statement *ast.SelectStmt,
	loopLabel string,
	continueTarget ast.Stmt,
) ps6088LoopOutcomes {
	result := ps6088LoopOutcomes{}
	closedSendReady := ps6088SelectHasClosedSend(pass, statement)
	for _, item := range statement.Body.List {
		clause, ok := item.(*ast.CommClause)
		if !ok {
			return ps6088LoopOutcomes{fallthroughPossible: true}
		}
		if clause.Comm == nil && closedSendReady ||
			clause.Comm != nil && (ps6088NilCommClause(pass, clause) ||
				ps6088ClosedSendComm(pass, clause.Comm)) {
			continue
		}
		assignment, assigned := clause.Comm.(*ast.AssignStmt)
		lhsReturns := true
		if assigned {
			for _, destination := range assignment.Lhs {
				lhsReturns = lhsReturns && ps6088AssignmentDestinationReturns(pass, destination)
			}
		}
		if !lhsReturns {
			continue
		}
		current := ps6088TargetControlOutcomes(
			pass, clause.Body, loopLabel, statement, continueTarget,
		)
		exit := ps6088ConsumeTargetExit(&current, statement)
		current.fallthroughPossible = current.fallthroughPossible || current.breaks || exit
		current.breaks = false
		result.fallthroughPossible = result.fallthroughPossible || current.fallthroughPossible
		result.next = result.next || current.next
		result.intraBodyCycle = result.intraBodyCycle || current.intraBodyCycle
		ps6088MergeTargetExits(&result, current)
		result.localNext = result.localNext || current.localNext
	}
	return result
}

func ps6088StatementReturnsNormally(pass *analysis.Pass, statement ast.Stmt, selectComm bool) bool {
	entry := ps6088StatementReturnIndexEntry(pass, statement)
	index := 0
	if selectComm {
		index = 1
	}
	entry.once[index].Do(func() {
		entry.returns[index] = ps6088ComputeStatementReturns(pass, statement, selectComm)
	})
	return entry.returns[index]
}

func ps6088StatementReturnIndexEntry(
	pass *analysis.Pass,
	statement ast.Stmt,
) *ps6088StatementReturnEntry {
	passIndex, ok := ps6088StatementReturns.Load(pass)
	if !ok {
		passIndex, _ = ps6088StatementReturns.LoadOrStore(pass, &ps6088StatementReturnPassIndex{})
	}
	index := passIndex.(*ps6088StatementReturnPassIndex)
	entry, ok := index.statements.Load(statement)
	if !ok {
		entry, _ = index.statements.LoadOrStore(statement, &ps6088StatementReturnEntry{})
	}
	return entry.(*ps6088StatementReturnEntry)
}

func ps6088ComputeStatementReturns(pass *analysis.Pass, statement ast.Stmt, selectComm bool) bool {
	if !selectComm && !ps6088StatementCompletionReturns(pass, statement) {
		return false
	}
	nonreturn := false
	astutil.WithStack(statement, func(node ast.Node, nodeStack []ast.Node) bool {
		if nonreturn || node == nil {
			return false
		}
		if literal, ok := node.(*ast.FuncLit); ok {
			invocation := ps6088ImmediateFuncLit(pass, literal, nodeStack)
			return invocation != nil && !ps6088AsyncCall(invocation, nodeStack, false)
		}
		expression, ok := node.(ast.Expr)
		if !ok {
			return true
		}
		if !ps6088StaticPathLive(pass, expression, nodeStack) {
			return false
		}
		if !(selectComm && ps6088SelectCommReceiveExpression(statement, expression)) &&
			!ps6088CommaOkTypeAssertion(expression, nodeStack) &&
			ps6088DirectExpressionPanics(pass, expression) {
			nonreturn = true
			return false
		}
		call, ok := expression.(*ast.CallExpr)
		if !ok {
			return true
		}
		if ps6088CallOperandUnevaluated(pass, call) {
			return false
		}
		if ps6088SynchronousCallPreventsReturn(pass, call, nodeStack) {
			nonreturn = true
			return false
		}
		invoked := ps6088InvokedFuncLit(pass, call)
		return invoked == nil
	})
	return !nonreturn
}

func ps6088SelectCommReceiveExpression(statement ast.Stmt, expression ast.Expr) bool {
	var receive ast.Expr
	switch communication := statement.(type) {
	case *ast.ExprStmt:
		receive = communication.X
	case *ast.AssignStmt:
		if len(communication.Rhs) == 1 {
			receive = communication.Rhs[0]
		}
	}
	unary, ok := ps2110Unparen(receive).(*ast.UnaryExpr)
	return ok && unary.Op == token.ARROW && ps2110Unparen(expression) == unary
}

func ps6088SelectCommOperandExpressions(statement ast.Stmt) []ast.Expr {
	if statement == nil {
		return nil
	}
	switch communication := statement.(type) {
	case *ast.SendStmt:
		return []ast.Expr{communication.Chan, communication.Value}
	case *ast.ExprStmt:
		receive, ok := ps2110Unparen(communication.X).(*ast.UnaryExpr)
		if ok && receive.Op == token.ARROW {
			return []ast.Expr{receive.X}
		}
	case *ast.AssignStmt:
		if len(communication.Rhs) != 1 {
			return nil
		}
		receive, ok := ps2110Unparen(communication.Rhs[0]).(*ast.UnaryExpr)
		if ok && receive.Op == token.ARROW {
			return []ast.Expr{receive.X}
		}
	}
	return nil
}

func ps6088SelectCommOperandsReturn(pass *analysis.Pass, statement ast.Stmt) bool {
	if statement == nil {
		return true
	}
	switch communication := statement.(type) {
	case *ast.SendStmt:
		return ps6088ExpressionReturnsNormally(pass, communication.Chan) &&
			ps6088ExpressionReturnsNormally(pass, communication.Value)
	case *ast.ExprStmt:
		receive, ok := ps2110Unparen(communication.X).(*ast.UnaryExpr)
		return ok && receive.Op == token.ARROW &&
			ps6088ExpressionReturnsNormally(pass, receive.X)
	case *ast.AssignStmt:
		if len(communication.Rhs) != 1 {
			return false
		}
		receive, ok := ps2110Unparen(communication.Rhs[0]).(*ast.UnaryExpr)
		return ok && receive.Op == token.ARROW &&
			ps6088ExpressionReturnsNormally(pass, receive.X)
	}
	return false
}

func ps6088SelectCommStatement(statement ast.Stmt, stack []ast.Node) bool {
	for index := len(stack) - 1; index >= 0; index-- {
		clause, ok := stack[index].(*ast.CommClause)
		if ok {
			return clause.Comm == statement
		}
		if stack[index] == statement {
			continue
		}
		if _, boundary := stack[index].(ast.Stmt); boundary {
			return false
		}
	}
	return false
}

func ps6088StatementCompletionReturns(pass *analysis.Pass, statement ast.Stmt) bool {
	switch value := statement.(type) {
	case *ast.SendStmt:
		return !ps6088NilRangeExpression(pass, value.Chan, pass.TypesInfo.TypeOf(value.Chan)) &&
			!ps6088DefinitelyClosedChannel(pass, value, value.Chan)
	case *ast.AssignStmt:
		for _, destination := range value.Lhs {
			if ps6088NilMapDestination(pass, destination) ||
				ps6088UnhashableMapDestination(pass, destination, value) {
				return false
			}
		}
	case *ast.IncDecStmt:
		return !ps6088NilMapDestination(pass, value.X) &&
			!ps6088UnhashableMapDestination(pass, value.X, value)
	}
	return true
}

func ps6088NilMapDestination(pass *analysis.Pass, destination ast.Expr) bool {
	index, ok := ps2110Unparen(destination).(*ast.IndexExpr)
	if !ok {
		return false
	}
	containerType := pass.TypesInfo.TypeOf(index.X)
	if containerType == nil {
		return false
	}
	_, mapType := types.Unalias(containerType).Underlying().(*types.Map)
	return mapType && ps6088NilRangeExpression(pass, index.X, containerType)
}

func ps6088UnhashableMapDestination(
	pass *analysis.Pass,
	destination ast.Expr,
	statement ast.Stmt,
) bool {
	index, ok := ps2110Unparen(destination).(*ast.IndexExpr)
	if !ok {
		return false
	}
	ignoredStart, ignoredEnd := token.NoPos, token.NoPos
	if assignment, ok := statement.(*ast.AssignStmt); ok {
		// Assignment destination operands are evaluated before the right-hand
		// side and before any stores. The index value is therefore already
		// snapshotted when later operands can rebind its source identifier.
		ignoredStart, ignoredEnd = destination.End(), assignment.End()
	}
	return ps6088UnhashableMapIndex(pass, index, ignoredStart, ignoredEnd)
}

func ps6088UnhashableMapIndex(
	pass *analysis.Pass,
	index *ast.IndexExpr,
	ignoredStart token.Pos,
	ignoredEnd token.Pos,
) bool {
	if !ignoredStart.IsValid() {
		if domain := ps6088ExpressionLocalDomainIndex(pass, index.Index); domain != nil {
			if evaluationEnd := domain.evaluationEnds[index]; evaluationEnd.IsValid() {
				ignoredStart, ignoredEnd = index.End(), evaluationEnd
			}
		}
	}
	return ps6088UnhashableMapKey(
		pass, index.X, index.Index, ignoredStart, ignoredEnd,
	)
}

func ps6088ExpressionLocalDomainIndex(
	pass *analysis.Pass,
	expression ast.Expr,
) *ps6088LocalDomainIndex {
	var result *ps6088LocalDomainIndex
	ast.Inspect(expression, func(node ast.Node) bool {
		if result != nil {
			return false
		}
		identifier, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		owner := ps6088LocalObjectOwner(pass, pass.TypesInfo.ObjectOf(identifier))
		if owner != nil {
			result = ps6088FunctionLocalDomainIndex(pass, owner)
		}
		return result == nil
	})
	return result
}

func ps6088UnhashableMapKey(
	pass *analysis.Pass,
	container ast.Expr,
	key ast.Expr,
	ignoredStart token.Pos,
	ignoredEnd token.Pos,
) bool {
	containerType := pass.TypesInfo.TypeOf(container)
	if containerType == nil {
		return false
	}
	mapType, ok := types.Unalias(containerType).Underlying().(*types.Map)
	if !ok {
		return false
	}
	if _, interfaceKey := types.Unalias(mapType.Key()).Underlying().(*types.Interface); !interfaceKey {
		return false
	}
	dynamicType, nilInterface, known := ps6088StableDynamicTypeForSnapshot(
		pass, key, make(map[ast.Expr]bool), ignoredStart, ignoredEnd,
	)
	return known && !nilInterface && dynamicType != nil && !types.Comparable(dynamicType)
}

func ps6088CommaOkTypeAssertion(expression ast.Expr, stack []ast.Node) bool {
	assertion, ok := ps2110Unparen(expression).(*ast.TypeAssertExpr)
	if !ok || assertion.Type == nil {
		return false
	}
	for index := len(stack) - 1; index >= 0; index-- {
		switch parent := stack[index].(type) {
		case *ast.ParenExpr:
			continue
		case *ast.AssignStmt:
			return len(parent.Lhs) == 2 && len(parent.Rhs) == 1 &&
				ps2110Unparen(parent.Rhs[0]) == assertion
		case *ast.ValueSpec:
			return len(parent.Names) == 2 && len(parent.Values) == 1 &&
				ps2110Unparen(parent.Values[0]) == assertion
		default:
			return false
		}
	}
	return false
}

func ps6088SynchronousCallPreventsReturn(
	pass *analysis.Pass,
	call *ast.CallExpr,
	stack []ast.Node,
) bool {
	if ps6088AsyncCall(call, stack, false) {
		return false
	}
	if ps6088CallInvocationPanics(pass, call) {
		return true
	}
	if ps6088NonreturnCall(pass, call) {
		return true
	}
	invoked := ps6088InvokedFuncLit(pass, call)
	if invoked == nil {
		return false
	}
	flow := cfg.New(
		invoked.Body,
		func(call *ast.CallExpr) bool { return !ps6088NonreturnCall(pass, call) },
	)
	return len(flow.Blocks) == 0 || !ps6088LiteralCFGBlockCanReachReturn(pass, invoked, flow.Blocks[0]) ||
		ps6088LiteralMayPreventReturn(pass, invoked)
}

func ps6088CallerLoopDomainSupported(pass *analysis.Pass, loop ast.Node) bool {
	if statement, ok := loop.(*ast.ForStmt); ok {
		if statement.Cond == nil {
			return true
		}
		if condition, known := ps6088Boolean(pass, statement.Cond); known {
			return condition
		}
	}
	return ps6088LoopDomainSupported(pass, loop)
}

func ps6088PositionRepeats(pass *analysis.Pass, flow *cfg.CFG, position token.Pos) bool {
	start := ps6079CFGBlockAt(flow, position)
	if start == nil || !start.Live {
		return false
	}
	seen := map[*cfg.Block]bool{start: true}
	queue := ps6088BlockSuccessors(pass, start)
	for len(queue) > 0 {
		block := queue[0]
		queue = queue[1:]
		if block == start {
			return true
		}
		if seen[block] {
			continue
		}
		seen[block] = true
		queue = append(queue, ps6088BlockSuccessors(pass, block)...)
	}
	return false
}

func ps6088CallerLoopMayRepeat(
	pass *analysis.Pass,
	loop ast.Node,
	positionCall *ast.CallExpr,
	ignoredCall *ast.CallExpr,
	stack []ast.Node,
	loopIndex int,
	parameters map[types.Object]struct{},
) (mayRepeat, intraBodyCycle bool) {
	body := astutil.LoopBody(loop)
	if body == nil || loopIndex+2 >= len(stack) || stack[loopIndex+1] != body {
		return true, false
	}
	statement, ok := stack[loopIndex+2].(ast.Stmt)
	if !ok {
		return true, false
	}
	statementIndex := -1
	for index, candidate := range body.List {
		if candidate == statement {
			statementIndex = index
			break
		}
	}
	if statementIndex < 0 {
		return true, false
	}
	if ps6088CallerLoopControlWritten(
		pass, loop, body.List, ps6088LoopPrefix(loop, stack[:loopIndex]), ignoredCall, parameters,
	) {
		return false, false
	}
	switch ps6088CallerStatementKind(pass, statement, positionCall) {
	case ps6088CallerStatementExits:
		return false, false
	case ps6088CallerStatementStraightLine:
		outcomes := ps6088StatementOutcomes(pass, body.List[statementIndex+1:], ps6088LoopLabel(stack, loop, loopIndex))
		return outcomes.fallthroughPossible || outcomes.next, outcomes.intraBodyCycle
	default:
		return true, false
	}
}

func ps6088LoopLabel(stack []ast.Node, loop ast.Node, loopIndex int) string {
	if loopIndex == 0 {
		return ""
	}
	labeled, ok := stack[loopIndex-1].(*ast.LabeledStmt)
	if !ok || labeled.Stmt != loop {
		return ""
	}
	return labeled.Label.Name
}

type ps6088CallerStatement uint8

const (
	ps6088CallerStatementUnknown ps6088CallerStatement = iota
	ps6088CallerStatementStraightLine
	ps6088CallerStatementExits
)

func ps6088CallerStatementKind(
	pass *analysis.Pass,
	statement ast.Stmt,
	call *ast.CallExpr,
) ps6088CallerStatement {
	if labeled, ok := statement.(*ast.LabeledStmt); ok {
		return ps6088CallerStatementKind(pass, labeled.Stmt, call)
	}
	contains := false
	ast.Inspect(statement, func(node ast.Node) bool {
		if contains || node == nil {
			return false
		}
		if node == call {
			contains = true
			return false
		}
		if _, literal := node.(*ast.FuncLit); literal {
			return false
		}
		return true
	})
	if !contains {
		return ps6088CallerStatementUnknown
	}
	if !ps6088StatementHeaderReturns(pass, statement) {
		return ps6088CallerStatementExits
	}
	switch value := statement.(type) {
	case *ast.ExprStmt:
		if outer, ok := ps2110Unparen(value.X).(*ast.CallExpr); ok && ps6088NonreturnCall(pass, outer) {
			return ps6088CallerStatementExits
		}
		return ps6088CallerStatementStraightLine
	case *ast.AssignStmt, *ast.DeclStmt, *ast.GoStmt, *ast.DeferStmt, *ast.SendStmt:
		return ps6088CallerStatementStraightLine
	case *ast.ReturnStmt:
		return ps6088CallerStatementExits
	default:
		return ps6088CallerStatementUnknown
	}
}

func ps6088LoopMayFanout(
	pass *analysis.Pass,
	loop ast.Node,
	launch *ast.GoStmt,
	prefix []ast.Stmt,
) bool {
	if !ps6088LoopDomainSupported(pass, loop) {
		return false
	}
	atMostOne, known := ps6088LoopAtMost(pass, loop, 1)
	if known {
		return !atMostOne && ps6088MayReachNextIteration(pass, loop, launch, prefix)
	}
	return ps6088MayReachNextIteration(pass, loop, launch, prefix)
}

func ps6088LoopDomainSupported(pass *analysis.Pass, loop ast.Node) bool {
	var expression ast.Expr
	if rangeLoop, ok := loop.(*ast.RangeStmt); ok {
		expression = rangeLoop.X
	} else {
		var domainOK bool
		expression, domainOK = ps6086LoopDomainExpression(pass, loop)
		if !domainOK {
			return false
		}
	}
	return ps6088DomainShapeSupported(pass, expression)
}

func ps6088DomainShapeSupported(pass *analysis.Pass, expression ast.Expr) bool {
	switch value := ps2110Unparen(expression).(type) {
	case *ast.Ident, *ast.BasicLit:
		return true
	case *ast.SelectorExpr:
		return ps6088DomainShapeSupported(pass, value.X)
	case *ast.IndexExpr:
		return ps6088DomainShapeSupported(pass, value.X) &&
			ps6088DomainShapeSupported(pass, value.Index)
	case *ast.IndexListExpr:
		if !ps6088DomainShapeSupported(pass, value.X) {
			return false
		}
		for _, index := range value.Indices {
			if !ps6088DomainShapeSupported(pass, index) {
				return false
			}
		}
		return true
	case *ast.StarExpr:
		return ps6088DomainShapeSupported(pass, value.X)
	case *ast.CallExpr:
		supported := pass.TypesInfo.Types[value.Fun].IsType()
		for _, name := range []string{"len", "cap", "min", "max"} {
			supported = supported || typedBuiltinName(pass, value.Fun, name)
		}
		if callee, _, ok := typedCallee(pass, value.Fun); ok &&
			callee.Pkg() != nil && callee.Pkg().Path() == "runtime" && callee.Name() == "GOMAXPROCS" {
			supported = true
		}
		if !supported {
			return false
		}
		for _, argument := range value.Args {
			if !ps6088DomainShapeSupported(pass, argument) {
				return false
			}
		}
		return true
	}
	return false
}

type ps6088LoopOutcomes struct {
	fallthroughPossible bool
	next                bool
	intraBodyCycle      bool
	breaks              bool
	localNext           bool
	exitTargets         map[ast.Stmt]struct{}
}

func ps6088MergeTargetExits(target *ps6088LoopOutcomes, source ps6088LoopOutcomes) {
	if len(source.exitTargets) == 0 {
		return
	}
	if target.exitTargets == nil {
		target.exitTargets = make(map[ast.Stmt]struct{}, len(source.exitTargets))
	}
	for statement := range source.exitTargets {
		target.exitTargets[statement] = struct{}{}
	}
}

func ps6088ConsumeTargetExit(outcomes *ps6088LoopOutcomes, target ast.Stmt) bool {
	if _, found := outcomes.exitTargets[target]; !found {
		return false
	}
	delete(outcomes.exitTargets, target)
	return true
}

func ps6088MayReachNextIteration(
	pass *analysis.Pass,
	loop ast.Node,
	launch *ast.GoStmt,
	prefix []ast.Stmt,
) bool {
	body := astutil.LoopBody(loop)
	if body == nil {
		return false
	}
	for index, statement := range body.List {
		if statement == launch {
			suffix := body.List[index+1:]
			if ps6088LoopControlWritten(pass, loop, body.List, prefix) {
				return false
			}
			outcomes := ps6088StatementOutcomes(pass, suffix, "")
			return outcomes.fallthroughPossible || outcomes.next
		}
	}
	return false
}

type ps6088LoopControls struct {
	objects  map[types.Object]struct{}
	paths    [][]ps6088StorageStep
	aliases  map[types.Object]struct{}
	params   map[types.Object]struct{}
	unknown  bool
	global   bool
	external bool
}

type ps6088StorageStep struct {
	kind       byte
	object     types.Object
	indexValue string
	indexKnown bool
}

func ps6088LoopControlWritten(
	pass *analysis.Pass,
	loop ast.Node,
	statements, prefix []ast.Stmt,
) bool {
	return ps6088LoopControlWrittenExcept(pass, loop, statements, prefix, nil, nil)
}

func ps6088CallerLoopControlWritten(
	pass *analysis.Pass,
	loop ast.Node,
	statements, prefix []ast.Stmt,
	call *ast.CallExpr,
	parameters map[types.Object]struct{},
) bool {
	return ps6088LoopControlWrittenExcept(pass, loop, statements, prefix, call, parameters)
}

func ps6088LoopControlWrittenExcept(
	pass *analysis.Pass,
	loop ast.Node,
	statements, prefix []ast.Stmt,
	call *ast.CallExpr,
	parameters map[types.Object]struct{},
) bool {
	var control ast.Expr
	switch value := loop.(type) {
	case *ast.ForStmt:
		control = value.Cond
	case *ast.RangeStmt:
		if !ps6088MutableRangeDomain(pass.TypesInfo.TypeOf(value.X)) {
			return false
		}
		if _, knownStorage := ps6088StoragePath(pass, value.X); !knownStorage {
			return true
		}
		control = value.X
	}
	if control == nil {
		return false
	}
	controls := &ps6088LoopControls{
		objects: make(map[types.Object]struct{}),
		aliases: make(map[types.Object]struct{}),
		params:  parameters,
	}
	ast.Inspect(control, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.SelectorExpr:
			if path, ok := ps6088StoragePath(pass, value); ok {
				controls.paths = append(controls.paths, path)
				for _, step := range path {
					controls.global = controls.global || ps6088PackageVariable(step.object)
					_, parameter := parameters[step.object]
					controls.external = controls.external || parameter && step.object != nil &&
						ps6088ContainsReference(step.object.Type(), make(map[types.Type]bool))
					if step.kind == 'i' && step.object != nil {
						controls.objects[step.object] = struct{}{}
					}
				}
			} else {
				controls.unknown = true
			}
			return false
		case *ast.Ident:
			identifier := value
			if object := pass.TypesInfo.ObjectOf(identifier); object != nil {
				controls.objects[object] = struct{}{}
				controls.global = controls.global || ps6088PackageVariable(object)
				_, parameter := parameters[object]
				controls.external = controls.external || parameter &&
					ps6088ContainsReference(object.Type(), make(map[types.Type]bool))
			}
		}
		return true
	})
	if controls.unknown {
		return true
	}
	ps6088SeedAliases(pass, prefix, controls)
	if ps6088StatementsWriteControl(pass, prefix, ps6088CloneControls(controls), call, false) {
		return true
	}
	return ps6088StatementsWriteControl(pass, statements, controls, call, false)
}

func ps6088MutableRangeDomain(value types.Type) bool {
	return ps6088MayBeMap(value) || ps6088MayBeChan(value)
}

func ps6088PackageVariable(object types.Object) bool {
	variable, ok := object.(*types.Var)
	return ok && variable.Pkg() != nil && variable.Parent() == variable.Pkg().Scope()
}

func ps6088SeedAliases(
	pass *analysis.Pass,
	statements []ast.Stmt,
	controls *ps6088LoopControls,
) {
	for _, statement := range statements {
		switch value := statement.(type) {
		case *ast.AssignStmt:
			ps6088RecordControlAliases(pass, value.Lhs, value.Rhs, controls, true)
		case *ast.DeclStmt:
			declaration, ok := value.Decl.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, specification := range declaration.Specs {
				value, ok := specification.(*ast.ValueSpec)
				if !ok {
					continue
				}
				left := make([]ast.Expr, 0, len(value.Names))
				for _, name := range value.Names {
					left = append(left, name)
				}
				ps6088RecordControlAliases(pass, left, value.Values, controls, true)
			}
		case *ast.SendStmt:
			if ps6088ExpressionAliasesControl(pass, value.Value, controls) {
				controls.global = true
			}
		case *ast.IncDecStmt:
			ps6088RecordControlIndexAlias(pass, value.X, controls)
		case *ast.RangeStmt:
			if ps6088ExpressionAliasesControl(pass, value.X, controls) {
				for _, target := range []ast.Expr{value.Key, value.Value} {
					if target != nil {
						ps6088RecordControlAliases(
							pass, []ast.Expr{target}, []ast.Expr{value.X}, controls, false,
						)
					}
				}
			}
			ps6088DiscoverAliases(pass, statement, controls)
		case *ast.BlockStmt:
			ps6088SeedAliases(pass, value.List, controls)
		case *ast.IfStmt:
			if value.Init != nil {
				ps6088SeedAliases(pass, []ast.Stmt{value.Init}, controls)
			}
			if condition, known := ps6088CFGBoolean(pass, value.Cond); known {
				if condition {
					ps6088SeedAliases(pass, value.Body.List, controls)
				} else {
					ps6088SeedAliasElse(pass, value.Else, controls)
				}
				continue
			}
			body := ps6088CloneControls(controls)
			ps6088SeedAliases(pass, value.Body.List, body)
			other := ps6088CloneControls(controls)
			ps6088SeedAliasElse(pass, value.Else, other)
			ps6088MergeAliases(controls, body, other)
		default:
			ps6088DiscoverAliases(pass, statement, controls)
		}
	}
}

func ps6088SeedAliasElse(pass *analysis.Pass, statement ast.Stmt, controls *ps6088LoopControls) {
	switch value := statement.(type) {
	case nil:
	case *ast.BlockStmt:
		ps6088SeedAliases(pass, value.List, controls)
	default:
		ps6088SeedAliases(pass, []ast.Stmt{statement}, controls)
	}
}

func ps6088CloneControls(controls *ps6088LoopControls) *ps6088LoopControls {
	clone := &ps6088LoopControls{
		objects:  controls.objects,
		paths:    controls.paths,
		aliases:  make(map[types.Object]struct{}, len(controls.aliases)),
		params:   controls.params,
		unknown:  controls.unknown,
		global:   controls.global,
		external: controls.external,
	}
	for object := range controls.aliases {
		clone.aliases[object] = struct{}{}
	}
	return clone
}

func ps6088MergeAliases(target *ps6088LoopControls, sources ...*ps6088LoopControls) {
	clear(target.aliases)
	for _, source := range sources {
		for object := range source.aliases {
			target.aliases[object] = struct{}{}
		}
		target.unknown = target.unknown || source.unknown
		target.global = target.global || source.global
		target.external = target.external || source.external
	}
}

func ps6088DiscoverAliases(pass *analysis.Pass, statement ast.Stmt, controls *ps6088LoopControls) {
	ast.Inspect(statement, func(node ast.Node) bool {
		if _, literal := node.(*ast.FuncLit); literal {
			return false
		}
		switch value := node.(type) {
		case *ast.AssignStmt:
			ps6088RecordControlAliases(pass, value.Lhs, value.Rhs, controls, false)
		case *ast.ValueSpec:
			left := make([]ast.Expr, 0, len(value.Names))
			for _, name := range value.Names {
				left = append(left, name)
			}
			ps6088RecordControlAliases(pass, left, value.Values, controls, false)
		}
		return true
	})
}

func ps6088StatementsWriteControl(
	pass *analysis.Pass,
	statements []ast.Stmt,
	controls *ps6088LoopControls,
	ignored *ast.CallExpr,
	deferredCompletes bool,
) bool {
	written := false
	for _, statement := range statements {
		astutil.WithStack(statement, func(node ast.Node, stack []ast.Node) bool {
			if written || node == nil {
				return false
			}
			if _, literal := node.(*ast.FuncLit); literal {
				return false
			}
			switch value := node.(type) {
			case *ast.AssignStmt:
				ps6088RecordControlAliases(pass, value.Lhs, value.Rhs, controls, false)
				for _, left := range value.Lhs {
					if ps6088WritesControl(pass, left, controls) {
						written = true
						return false
					}
				}
			case *ast.IncDecStmt:
				wasGlobal := controls.global
				ps6088RecordControlIndexAlias(pass, value.X, controls)
				written = !wasGlobal && controls.global || ps6088WritesControl(pass, value.X, controls)
			case *ast.RangeStmt:
				wasGlobal := controls.global
				aliasCount := len(controls.aliases)
				if ps6088ExpressionAliasesControl(pass, value.X, controls) {
					for _, target := range []ast.Expr{value.Key, value.Value} {
						if target != nil {
							ps6088RecordControlAliases(
								pass, []ast.Expr{target}, []ast.Expr{value.X}, controls, false,
							)
						}
					}
				}
				written = len(controls.aliases) > aliasCount || !wasGlobal && controls.global ||
					ps6088WritesControl(pass, value.Key, controls) ||
					ps6088WritesControl(pass, value.Value, controls)
			case *ast.ValueSpec:
				left := make([]ast.Expr, 0, len(value.Names))
				for _, name := range value.Names {
					left = append(left, name)
				}
				ps6088RecordControlAliases(pass, left, value.Values, controls, false)
			case *ast.SendStmt:
				if ps6088ExpressionAliasesControl(pass, value.Value, controls) {
					controls.global = true
					written = true
				}
			case *ast.UnaryExpr:
				if value.Op == token.ARROW {
					written = ps6088WritesControl(pass, value.X, controls)
				}
			case *ast.CallExpr:
				if value == ignored {
					written = ps6088KnownCallMayWriteControl(pass, value, controls)
					return !written
				}
				literal := ps6088InvokedFuncLit(pass, value)
				asynchronous := ps6088AsyncCall(value, stack, deferredCompletes)
				if literal != nil && !asynchronous {
					ps6088RecordIIFEAliases(pass, literal, value, controls)
					written = ps6088StatementsWriteControl(pass, literal.Body.List, controls, ignored, true)
				} else if literal != nil && controls.params != nil && ps6088GoCall(value, stack) {
					asyncControls := ps6088CloneControls(controls)
					ps6088RecordIIFEAliases(pass, literal, value, asyncControls)
					written = ps6088StatementsWriteControl(
						pass, literal.Body.List, asyncControls, ignored, true,
					)
				} else if !asynchronous || controls.params != nil && ps6088GoCall(value, stack) {
					written = ps6088CallMayWriteControl(pass, value, controls)
				}
			}
			return !written
		})
		if written {
			return true
		}
	}
	return false
}

func ps6088InvokedFuncLit(pass *analysis.Pass, call *ast.CallExpr) *ast.FuncLit {
	expression := ps2110Unparen(call.Fun)
	for {
		if literal, ok := expression.(*ast.FuncLit); ok {
			return literal
		}
		conversion, ok := expression.(*ast.CallExpr)
		if !ok || len(conversion.Args) != 1 || !ps6088FunctionConversion(pass, conversion) {
			return nil
		}
		expression = ps2110Unparen(conversion.Args[0])
	}
}

func ps6088KnownCallMayWriteControl(
	pass *analysis.Pass,
	call *ast.CallExpr,
	controls *ps6088LoopControls,
) bool {
	if controls.global || controls.external {
		return true
	}
	for _, operand := range ps6088CallOperands(call) {
		if ps6088OpaqueFunctionValue(pass, operand) {
			return true
		}
		if ps6088ExpressionHasReferenceParameter(pass, operand, controls.params) {
			return true
		}
		if ps6088ExpressionAliasesControl(pass, operand, controls) {
			return true
		}
	}
	return false
}

func ps6088ExpressionHasReferenceParameter(
	pass *analysis.Pass,
	expression ast.Expr,
	parameters map[types.Object]struct{},
) bool {
	found := false
	ast.Inspect(expression, func(node ast.Node) bool {
		if found || node == nil {
			return false
		}
		identifier, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		object := pass.TypesInfo.ObjectOf(identifier)
		_, parameter := parameters[object]
		if parameter && ps6088ContainsReference(object.Type(), make(map[types.Type]bool)) {
			found = true
			return false
		}
		return true
	})
	return found
}

func ps6088Parameters(pass *analysis.Pass) map[types.Object]struct{} {
	parameters := make(map[types.Object]struct{})
	add := func(list *ast.FieldList) {
		if list == nil {
			return
		}
		for _, field := range list.List {
			for _, name := range field.Names {
				if object := pass.TypesInfo.Defs[name]; object != nil {
					parameters[object] = struct{}{}
				}
			}
		}
	}
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok {
				continue
			}
			add(function.Recv)
			add(function.Type.Params)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			if literal, ok := node.(*ast.FuncLit); ok {
				add(literal.Type.Params)
			}
			return true
		})
	}
	return parameters
}

func ps6088OpaqueFunctionValue(pass *analysis.Pass, expression ast.Expr) bool {
	value := pass.TypesInfo.TypeOf(expression)
	if value == nil {
		return false
	}
	if _, signature := types.Unalias(value).Underlying().(*types.Signature); !signature {
		return false
	}
	expression = ps2110Unparen(expression)
	if _, literal := expression.(*ast.FuncLit); literal {
		return false
	}
	if identifier, ok := expression.(*ast.Ident); ok {
		_, declaredFunction := pass.TypesInfo.ObjectOf(identifier).(*types.Func)
		return !declaredFunction
	}
	if selector, ok := expression.(*ast.SelectorExpr); ok && pass.TypesInfo.Selections[selector] == nil {
		_, packageFunction := pass.TypesInfo.ObjectOf(selector.Sel).(*types.Func)
		return !packageFunction
	}
	return true
}

func ps6088CallMayWriteControl(
	pass *analysis.Pass,
	call *ast.CallExpr,
	controls *ps6088LoopControls,
) bool {
	if ps6088PureCall(pass, call) {
		return false
	}
	for _, operand := range ps6088CallOperands(call) {
		if ps6088ExpressionMayDesignateControl(pass, operand, controls) {
			return true
		}
	}
	if controls.global {
		return true
	}
	_, _, resolved := typedCallee(pass, call.Fun)
	return !resolved
}

func ps6088PureCall(pass *analysis.Pass, call *ast.CallExpr) bool {
	if pass.TypesInfo.Types[call.Fun].IsType() {
		return true
	}
	for _, name := range []string{
		"cap", "complex", "imag", "len", "make", "max", "min", "new", "real", "recover",
	} {
		if typedBuiltinName(pass, call.Fun, name) {
			return true
		}
	}
	return false
}

func ps6088CallOperands(call *ast.CallExpr) []ast.Expr {
	operands := make([]ast.Expr, 0, len(call.Args)+1)
	if selector, ok := ps2110Unparen(call.Fun).(*ast.SelectorExpr); ok {
		operands = append(operands, selector.X)
	}
	return append(operands, call.Args...)
}

func ps6088ExpressionMayDesignateControl(
	pass *analysis.Pass,
	expression ast.Expr,
	controls *ps6088LoopControls,
) bool {
	expression = ps2110Unparen(expression)
	if address, ok := expression.(*ast.UnaryExpr); ok && address.Op == token.AND {
		if ps6088WritesControl(pass, address.X, controls) {
			return true
		}
	}
	if path, ok := ps6088StoragePath(pass, expression); ok &&
		ps6088StorageWritesControl(path, controls) {
		return true
	}
	return ps6088ContainsReference(pass.TypesInfo.TypeOf(expression), make(map[types.Type]bool))
}

func ps6088ContainsReference(value types.Type, seen map[types.Type]bool) bool {
	if value == nil {
		return false
	}
	value = types.Unalias(value)
	if seen[value] {
		return false
	}
	seen[value] = true
	if parameter, ok := value.(*types.TypeParam); ok {
		return ps6088ConstraintContainsReference(parameter.Constraint(), seen)
	}
	switch underlying := value.Underlying().(type) {
	case *types.Basic:
		return underlying.Kind() == types.UnsafePointer
	case *types.Pointer, *types.Slice, *types.Map, *types.Chan, *types.Signature, *types.Interface:
		return true
	case *types.Array:
		return ps6088ContainsReference(underlying.Elem(), seen)
	case *types.Struct:
		for index := range underlying.NumFields() {
			if ps6088ContainsReference(underlying.Field(index).Type(), seen) {
				return true
			}
		}
	}
	return false
}

func ps6088ConstraintContainsReference(value types.Type, seen map[types.Type]bool) bool {
	if value == nil {
		return false
	}
	value = types.Unalias(value)
	if union, ok := value.(*types.Union); ok {
		for index := range union.Len() {
			if ps6088ContainsReference(union.Term(index).Type(), seen) {
				return true
			}
		}
		return false
	}
	constraint, ok := value.Underlying().(*types.Interface)
	if !ok || constraint.NumEmbeddeds() == 0 {
		return true
	}
	for index := range constraint.NumEmbeddeds() {
		if ps6088ConstraintContainsReference(constraint.EmbeddedType(index), seen) {
			return true
		}
	}
	return false
}

func ps6088RecordControlAliases(
	pass *analysis.Pass,
	left, right []ast.Expr,
	controls *ps6088LoopControls,
	kill bool,
) {
	if len(left) != len(right) {
		if len(right) == 1 {
			aliases := ps6088MultiValueAliases(pass, right[0], len(left), controls)
			for index, destination := range left {
				ps6088RecordControlAliasDestination(pass, destination, aliases[index], controls, kill)
			}
		}
		return
	}
	aliases := make([]bool, len(right))
	for index, expression := range right {
		aliases[index] = ps6088ExpressionAliasesControl(pass, expression, controls)
	}
	for index, destination := range left {
		ps6088RecordControlAliasDestination(pass, destination, aliases[index], controls, kill)
	}
}

func ps6088MultiValueAliases(
	pass *analysis.Pass,
	expression ast.Expr,
	count int,
	controls *ps6088LoopControls,
) []bool {
	result := make([]bool, count)
	value := ps2110Unparen(expression)
	aliases := false
	switch source := value.(type) {
	case *ast.IndexExpr:
		aliases = ps6088ExpressionAliasesControl(pass, source.X, controls)
	case *ast.UnaryExpr:
		aliases = source.Op == token.ARROW && ps6088ExpressionAliasesControl(pass, source.X, controls)
	case *ast.TypeAssertExpr:
		aliases = ps6088ExpressionAliasesControl(pass, source.X, controls)
	case *ast.CallExpr:
		aliases = ps6088ExpressionTreeAliasesControl(pass, source, controls)
		if _, _, resolved := typedCallee(pass, source.Fun); !resolved {
			aliases = true
		}
	}
	if !aliases {
		return result
	}
	valueType := pass.TypesInfo.TypeOf(expression)
	if tuple, ok := valueType.(*types.Tuple); ok {
		for index := 0; index < count && index < tuple.Len(); index++ {
			result[index] = ps6088ContainsReference(tuple.At(index).Type(), make(map[types.Type]bool))
		}
		return result
	}
	if count > 0 {
		result[0] = ps6088ContainsReference(valueType, make(map[types.Type]bool))
	}
	return result
}

func ps6088RecordControlAliasDestination(
	pass *analysis.Pass,
	destination ast.Expr,
	alias bool,
	controls *ps6088LoopControls,
	kill bool,
) {
	ps6088RecordControlIndexAlias(pass, destination, controls)
	identifier, identifierOK := ps2110Unparen(destination).(*ast.Ident)
	var object types.Object
	if identifierOK {
		object = pass.TypesInfo.ObjectOf(identifier)
	}
	if identifierOK && kill {
		delete(controls.aliases, object)
	}
	if !alias {
		return
	}
	if !identifierOK {
		path, ok := ps6088StoragePath(pass, destination)
		if !ok || len(path) == 0 {
			controls.global = true
			return
		}
		object = path[0].object
	}
	if object != nil {
		controls.aliases[object] = struct{}{}
		controls.global = controls.global || ps6088PackageVariable(object)
	}
}

func ps6088RecordControlIndexAlias(
	pass *analysis.Pass,
	destination ast.Expr,
	controls *ps6088LoopControls,
) {
	indexed, ok := ps2110Unparen(destination).(*ast.IndexExpr)
	if !ok || !ps6088MayBeMap(pass.TypesInfo.TypeOf(indexed.X)) ||
		!ps6088ExpressionAliasesControl(pass, indexed.Index, controls) {
		return
	}
	path, ok := ps6088StoragePath(pass, indexed.X)
	if !ok || len(path) == 0 || path[0].object == nil {
		controls.global = true
		return
	}
	root := path[0].object
	controls.aliases[root] = struct{}{}
	controls.global = controls.global || ps6088PackageVariable(root)
}

func ps6088MayBeMap(value types.Type) bool {
	if value == nil {
		return false
	}
	value = types.Unalias(value)
	switch typed := value.(type) {
	case *types.TypeParam:
		return ps6088ConstraintMayBeMap(typed.Constraint(), make(map[types.Type]bool))
	case *types.Union:
		for index := range typed.Len() {
			if ps6088ConstraintMayBeMap(typed.Term(index).Type(), make(map[types.Type]bool)) {
				return true
			}
		}
		return false
	}
	_, mapped := value.Underlying().(*types.Map)
	return mapped
}

func ps6088ConstraintMayBeMap(value types.Type, seen map[types.Type]bool) bool {
	if value == nil {
		return false
	}
	value = types.Unalias(value)
	if seen[value] {
		return false
	}
	seen[value] = true
	if _, mapped := value.Underlying().(*types.Map); mapped {
		return true
	}
	if union, ok := value.(*types.Union); ok {
		for index := range union.Len() {
			if ps6088ConstraintMayBeMap(union.Term(index).Type(), seen) {
				return true
			}
		}
		return false
	}
	constraint, ok := value.Underlying().(*types.Interface)
	if !ok {
		return false
	}
	if constraint.NumEmbeddeds() == 0 {
		return true
	}
	for index := range constraint.NumEmbeddeds() {
		if ps6088ConstraintMayBeMap(constraint.EmbeddedType(index), seen) {
			return true
		}
	}
	return false
}

func ps6088MayBeChan(value types.Type) bool {
	if value == nil {
		return false
	}
	value = types.Unalias(value)
	switch typed := value.(type) {
	case *types.TypeParam:
		return ps6088ConstraintMayBeChan(typed.Constraint(), make(map[types.Type]bool))
	case *types.Union:
		for index := range typed.Len() {
			if ps6088ConstraintMayBeChan(typed.Term(index).Type(), make(map[types.Type]bool)) {
				return true
			}
		}
		return false
	}
	_, channel := value.Underlying().(*types.Chan)
	return channel
}

func ps6088ConstraintMayBeChan(value types.Type, seen map[types.Type]bool) bool {
	if value == nil {
		return false
	}
	value = types.Unalias(value)
	if seen[value] {
		return false
	}
	seen[value] = true
	if _, channel := value.Underlying().(*types.Chan); channel {
		return true
	}
	if union, ok := value.(*types.Union); ok {
		for index := range union.Len() {
			if ps6088ConstraintMayBeChan(union.Term(index).Type(), seen) {
				return true
			}
		}
		return false
	}
	constraint, ok := value.Underlying().(*types.Interface)
	if !ok {
		return false
	}
	if constraint.NumEmbeddeds() == 0 {
		return true
	}
	for index := range constraint.NumEmbeddeds() {
		if ps6088ConstraintMayBeChan(constraint.EmbeddedType(index), seen) {
			return true
		}
	}
	return false
}

func ps6088RecordIIFEAliases(
	pass *analysis.Pass,
	literal *ast.FuncLit,
	call *ast.CallExpr,
	controls *ps6088LoopControls,
) {
	parameter := 0
	for _, field := range literal.Type.Params.List {
		if len(field.Names) == 0 {
			parameter++
			continue
		}
		for _, name := range field.Names {
			if parameter >= len(call.Args) {
				return
			}
			_, variadic := field.Type.(*ast.Ellipsis)
			aliasesControl := ps6088ExpressionAliasesControl(pass, call.Args[parameter], controls)
			if variadic {
				for _, argument := range call.Args[parameter:] {
					aliasesControl = aliasesControl ||
						ps6088ExpressionAliasesControl(pass, argument, controls)
				}
				aliasesControl = aliasesControl || call.Ellipsis.IsValid()
			}
			if aliasesControl {
				if object := pass.TypesInfo.ObjectOf(name); object != nil {
					controls.aliases[object] = struct{}{}
				}
			}
			if variadic {
				return
			}
			parameter++
		}
	}
}

func ps6088ExpressionAliasesControl(
	pass *analysis.Pass,
	expression ast.Expr,
	controls *ps6088LoopControls,
) bool {
	expression = ps2110Unparen(expression)
	if literal, ok := expression.(*ast.FuncLit); ok {
		return ps6088StatementsWriteControl(pass, literal.Body.List, ps6088CloneControls(controls), nil, true)
	}
	if address, ok := expression.(*ast.UnaryExpr); ok && address.Op == token.AND {
		if ps6088WritesControl(pass, address.X, controls) {
			return true
		}
	}
	if identifier, ok := expression.(*ast.Ident); ok {
		if _, alias := controls.aliases[pass.TypesInfo.ObjectOf(identifier)]; alias {
			return true
		}
	}
	typeOf := pass.TypesInfo.TypeOf(expression)
	if !ps6088ContainsReference(typeOf, make(map[types.Type]bool)) {
		return false
	}
	return ps6088ExpressionTreeAliasesControl(pass, expression, controls)
}

func ps6088ExpressionTreeAliasesControl(
	pass *analysis.Pass,
	expression ast.Expr,
	controls *ps6088LoopControls,
) bool {
	aliased := false
	ast.Inspect(expression, func(node ast.Node) bool {
		if aliased || node == nil {
			return false
		}
		current, ok := node.(ast.Expr)
		if !ok {
			return true
		}
		current = ps2110Unparen(current)
		if address, ok := current.(*ast.UnaryExpr); ok && address.Op == token.AND &&
			ps6088WritesControl(pass, address.X, controls) {
			aliased = true
			return false
		}
		if identifier, ok := current.(*ast.Ident); ok {
			if _, alias := controls.aliases[pass.TypesInfo.ObjectOf(identifier)]; alias {
				aliased = true
				return false
			}
		}
		if path, ok := ps6088StoragePath(pass, current); ok &&
			ps6088StorageWritesControl(path, controls) {
			aliased = true
			return false
		}
		return true
	})
	return aliased
}

func ps6088AsyncCall(call *ast.CallExpr, stack []ast.Node, deferredCompletes bool) bool {
	for index := len(stack) - 1; index >= 0; index-- {
		switch statement := stack[index].(type) {
		case *ast.GoStmt:
			return statement.Call == call
		case *ast.DeferStmt:
			return statement.Call == call && !deferredCompletes
		}
	}
	return false
}

func ps6088GoCall(call *ast.CallExpr, stack []ast.Node) bool {
	for index := len(stack) - 1; index >= 0; index-- {
		if statement, ok := stack[index].(*ast.GoStmt); ok {
			return statement.Call == call
		}
	}
	return false
}

func ps6088DeferredCall(call *ast.CallExpr, stack []ast.Node) bool {
	for index := len(stack) - 1; index >= 0; index-- {
		if statement, ok := stack[index].(*ast.DeferStmt); ok {
			return statement.Call == call
		}
	}
	return false
}

func ps6088WritesControl(pass *analysis.Pass, expression ast.Expr, controls *ps6088LoopControls) bool {
	if expression == nil {
		return false
	}
	switch value := ps2110Unparen(expression).(type) {
	case *ast.Ident:
		object := pass.TypesInfo.ObjectOf(value)
		if _, written := controls.objects[object]; written {
			return true
		}
		for _, path := range controls.paths {
			if len(path) > 0 && path[0].object == object {
				return true
			}
		}
		return false
	case *ast.SelectorExpr:
		path, ok := ps6088StoragePath(pass, value)
		if !ok {
			return ps6088ContainsReference(pass.TypesInfo.TypeOf(value.X), make(map[types.Type]bool))
		}
		return ps6088StorageWritesControl(path, controls)
	case *ast.IndexExpr:
		if path, ok := ps6088StoragePath(pass, value); ok {
			if ps6088StorageWritesControl(path, controls) {
				return true
			}
		} else if ps6088ContainsReference(pass.TypesInfo.TypeOf(value.X), make(map[types.Type]bool)) {
			return true
		}
		return ps6088WritesControl(pass, value.X, controls)
	case *ast.IndexListExpr:
		return ps6088WritesControl(pass, value.X, controls)
	case *ast.SliceExpr:
		return ps6088WritesControl(pass, value.X, controls)
	case *ast.StarExpr:
		if path, ok := ps6088StoragePath(pass, value); ok {
			if ps6088StorageWritesControl(path, controls) {
				return true
			}
		} else if ps6088ContainsReference(pass.TypesInfo.TypeOf(value.X), make(map[types.Type]bool)) {
			return true
		}
		return ps6088WritesControl(pass, value.X, controls)
	}
	return false
}

func ps6088StorageWritesControl(path []ps6088StorageStep, controls *ps6088LoopControls) bool {
	if len(path) == 0 {
		return false
	}
	root := path[0].object
	if _, alias := controls.aliases[root]; alias {
		return true
	}
	if _, wholeObject := controls.objects[root]; wholeObject {
		return true
	}
	for _, control := range controls.paths {
		if ps6088SameStoragePath(path, control) {
			return true
		}
	}
	return false
}

func ps6088StoragePath(pass *analysis.Pass, expression ast.Expr) ([]ps6088StorageStep, bool) {
	switch value := ps2110Unparen(expression).(type) {
	case *ast.Ident:
		object := pass.TypesInfo.ObjectOf(value)
		return []ps6088StorageStep{{kind: 'r', object: object}}, object != nil
	case *ast.SelectorExpr:
		path, ok := ps6088StoragePath(pass, value.X)
		steps, stepsOK := ps6088SelectionSteps(pass, value)
		if !ok || !stepsOK {
			return nil, false
		}
		return append(path, steps...), true
	case *ast.StarExpr:
		path, ok := ps6088StoragePath(pass, value.X)
		return append(path, ps6088StorageStep{kind: 'd'}), ok
	case *ast.IndexExpr:
		path, ok := ps6088StoragePath(pass, value.X)
		if !ok {
			return nil, false
		}
		if ps6088IndexDereferencesPointer(pass.TypesInfo.TypeOf(value.X)) {
			path = append(path, ps6088StorageStep{kind: 'd'})
		}
		indexValue, indexObject, indexKnown := ps6088IndexIdentity(pass, value.Index)
		return append(path, ps6088StorageStep{
			kind:       'i',
			object:     indexObject,
			indexValue: indexValue,
			indexKnown: indexKnown,
		}), true
	case *ast.IndexListExpr:
		return ps6088StoragePath(pass, value.X)
	case *ast.SliceExpr:
		return nil, false
	}
	return nil, false
}

func ps6088SelectionSteps(pass *analysis.Pass, selector *ast.SelectorExpr) ([]ps6088StorageStep, bool) {
	selection := pass.TypesInfo.Selections[selector]
	if selection == nil {
		object := pass.TypesInfo.ObjectOf(selector.Sel)
		return []ps6088StorageStep{{kind: 's', object: object}}, object != nil
	}
	if selection.Kind() != types.FieldVal {
		return nil, false
	}
	current := selection.Recv()
	steps := make([]ps6088StorageStep, 0, len(selection.Index())+1)
	for _, index := range selection.Index() {
		current = types.Unalias(current)
		if pointer, ok := current.Underlying().(*types.Pointer); ok {
			steps = append(steps, ps6088StorageStep{kind: 'd'})
			current = types.Unalias(pointer.Elem())
		}
		structure, ok := current.Underlying().(*types.Struct)
		if !ok || index < 0 || index >= structure.NumFields() {
			return nil, false
		}
		field := structure.Field(index)
		steps = append(steps, ps6088StorageStep{kind: 's', object: field})
		current = field.Type()
	}
	return steps, len(steps) > 0
}

func ps6088IndexDereferencesPointer(value types.Type) bool {
	if value == nil {
		return false
	}
	_, pointer := types.Unalias(value).Underlying().(*types.Pointer)
	return pointer
}

func ps6088IndexIdentity(pass *analysis.Pass, expression ast.Expr) (string, types.Object, bool) {
	if value := pass.TypesInfo.Types[expression].Value; value != nil {
		return value.ExactString(), nil, true
	}
	identifier, ok := ps2110Unparen(expression).(*ast.Ident)
	if !ok {
		return "", nil, false
	}
	object := pass.TypesInfo.ObjectOf(identifier)
	if object == nil {
		return "", nil, false
	}
	return "", object, true
}

func ps6088SameStoragePath(left, right []ps6088StorageStep) bool {
	if len(left) == 0 || len(right) == 0 {
		return false
	}
	referenceRoots := false
	separated := false
	length := min(len(left), len(right))
	for index := range length {
		if left[index].kind != right[index].kind {
			return true
		}
		switch left[index].kind {
		case 'r':
			if left[index].object != right[index].object {
				if !ps6088ReferenceRootsMayAlias(left[index].object, right[index].object) {
					separated = true
					continue
				}
				referenceRoots = true
			}
		case 's':
			if left[index].object != right[index].object {
				separated = true
			}
		case 'i':
			if referenceRoots {
				continue
			}
			leftConstant := left[index].indexKnown && left[index].object == nil
			rightConstant := right[index].indexKnown && right[index].object == nil
			if leftConstant && rightConstant && left[index].indexValue != right[index].indexValue {
				separated = true
			}
		case 'd':
			separated = false
			referenceRoots = false
		}
	}
	return !separated
}

func ps6088ReferenceRootsMayAlias(left, right types.Object) bool {
	if left == nil || right == nil || left.Type() == nil || right.Type() == nil {
		return false
	}
	leftValue := types.Unalias(left.Type())
	rightValue := types.Unalias(right.Type())
	if _, parameter := leftValue.(*types.TypeParam); parameter {
		return true
	}
	if _, parameter := rightValue.(*types.TypeParam); parameter {
		return true
	}
	leftType := leftValue.Underlying()
	rightType := rightValue.Underlying()
	if leftBasic, ok := leftType.(*types.Basic); ok && leftBasic.Kind() == types.UnsafePointer {
		return true
	}
	if rightBasic, ok := rightType.(*types.Basic); ok && rightBasic.Kind() == types.UnsafePointer {
		return true
	}
	switch leftReference := leftType.(type) {
	case *types.Pointer:
		rightReference, ok := rightType.(*types.Pointer)
		return ok && types.Identical(leftReference.Elem(), rightReference.Elem())
	case *types.Slice:
		rightReference, ok := rightType.(*types.Slice)
		return ok && types.Identical(leftReference.Elem(), rightReference.Elem())
	case *types.Map:
		rightReference, ok := rightType.(*types.Map)
		return ok && types.Identical(leftReference.Key(), rightReference.Key()) &&
			types.Identical(leftReference.Elem(), rightReference.Elem())
	}
	return false
}

func ps6088StatementOutcomes(
	pass *analysis.Pass,
	statements []ast.Stmt,
	loopLabel string,
) ps6088LoopOutcomes {
	outcomes := ps6088LoopOutcomes{fallthroughPossible: true}
	for _, statement := range statements {
		if !outcomes.fallthroughPossible {
			break
		}
		current := ps6088StatementOutcome(pass, statement, loopLabel)
		outcomes.next = outcomes.next || current.next
		outcomes.intraBodyCycle = outcomes.intraBodyCycle || current.intraBodyCycle
		outcomes.fallthroughPossible = current.fallthroughPossible
	}
	return outcomes
}

func ps6088StatementOutcome(
	pass *analysis.Pass,
	statement ast.Stmt,
	loopLabel string,
) ps6088LoopOutcomes {
	if !ps6088StatementHeaderReturns(pass, statement) {
		return ps6088LoopOutcomes{}
	}
	switch statement.(type) {
	case *ast.ExprStmt, *ast.AssignStmt, *ast.DeclStmt, *ast.IncDecStmt, *ast.SendStmt,
		*ast.GoStmt, *ast.DeferStmt:
		if !ps6088StatementReturnsNormally(pass, statement, false) {
			return ps6088LoopOutcomes{}
		}
	}
	switch value := statement.(type) {
	case *ast.BlockStmt:
		return ps6088StatementOutcomes(pass, value.List, loopLabel)
	case *ast.LabeledStmt:
		return ps6088StatementOutcome(pass, value.Stmt, loopLabel)
	case *ast.BranchStmt:
		switch value.Tok {
		case token.CONTINUE:
			if value.Label == nil || value.Label.Name == loopLabel {
				return ps6088LoopOutcomes{next: true}
			}
			return ps6088LoopOutcomes{}
		case token.GOTO:
			return ps6088LoopOutcomes{intraBodyCycle: true}
		case token.BREAK:
			return ps6088LoopOutcomes{}
		}
	case *ast.ReturnStmt:
		return ps6088LoopOutcomes{}
	case *ast.IfStmt:
		if condition, known := ps6088Boolean(pass, value.Cond); known {
			if condition {
				return ps6088StatementOutcomes(pass, value.Body.List, loopLabel)
			}
			return ps6088ElseOutcome(pass, value.Else, loopLabel)
		}
		body := ps6088StatementOutcomes(pass, value.Body.List, loopLabel)
		other := ps6088ElseOutcome(pass, value.Else, loopLabel)
		return ps6088LoopOutcomes{
			fallthroughPossible: body.fallthroughPossible || other.fallthroughPossible,
			next:                body.next || other.next,
			intraBodyCycle:      body.intraBodyCycle || other.intraBodyCycle,
		}
	case *ast.ExprStmt:
		if call, ok := ps2110Unparen(value.X).(*ast.CallExpr); ok && ps6088NonreturnCall(pass, call) {
			return ps6088LoopOutcomes{}
		}
	case *ast.SwitchStmt:
		return ps6088SwitchStatementOutcome(pass, value, loopLabel)
	case *ast.TypeSwitchStmt:
		return ps6088TypeSwitchStatementOutcome(pass, value, loopLabel)
	case *ast.SelectStmt:
		return ps6088SelectStatementOutcome(pass, value, loopLabel)
	case *ast.ForStmt:
		if ps6086ProvablyInfiniteLoop(pass, value) {
			return ps6088LoopOutcomes{}
		}
		if ps6088LoopDefinitelyNonempty(pass, value) && !ps6088LoopHasReachableBreak(pass, value) {
			body := ps6088StatementOutcomes(pass, value.Body.List, loopLabel)
			if !body.fallthroughPossible && !body.next && !body.intraBodyCycle {
				return body
			}
		}
		return ps6088LoopOutcomes{
			fallthroughPossible: true,
			intraBodyCycle:      ps6088ReachableGoto(pass, statement),
		}
	case *ast.RangeStmt:
		if ps6088LoopDefinitelyNonempty(pass, value) && !ps6088LoopHasReachableBreak(pass, value) {
			body := ps6088StatementOutcomes(pass, value.Body.List, loopLabel)
			if !body.fallthroughPossible && !body.next && !body.intraBodyCycle {
				return body
			}
		}
		return ps6088LoopOutcomes{
			fallthroughPossible: true,
			intraBodyCycle:      ps6088ReachableGoto(pass, statement),
		}
	}
	return ps6088LoopOutcomes{fallthroughPossible: true}
}

func ps6088SwitchStatementOutcome(
	pass *analysis.Pass,
	statement *ast.SwitchStmt,
	loopLabel string,
) ps6088LoopOutcomes {
	clauses := make([]*ast.CaseClause, 0, len(statement.Body.List))
	defaultIndex := -1
	for _, item := range statement.Body.List {
		clause, ok := item.(*ast.CaseClause)
		if !ok {
			return ps6088LoopOutcomes{fallthroughPossible: true}
		}
		clauses = append(clauses, clause)
		if len(clause.List) == 0 {
			defaultIndex = len(clauses) - 1
		}
	}
	tag := constant.MakeBool(true)
	if statement.Tag != nil {
		tag = pass.TypesInfo.Types[statement.Tag].Value
	}
	if tag == nil {
		return ps6088PossibleClauseOutcomes(pass, statement, clauses, defaultIndex, loopLabel)
	}
	for index, clause := range clauses {
		for _, expression := range clause.List {
			value := pass.TypesInfo.Types[expression].Value
			if value == nil {
				return ps6088PossibleClauseOutcomes(pass, statement, clauses, defaultIndex, loopLabel)
			}
			if constant.Compare(tag, token.EQL, value) {
				return ps6088SelectedClauseOutcome(pass, statement, clauses, index, loopLabel)
			}
		}
	}
	if defaultIndex < 0 {
		return ps6088LoopOutcomes{fallthroughPossible: true}
	}
	return ps6088SelectedClauseOutcome(pass, statement, clauses, defaultIndex, loopLabel)
}

func ps6088TypeSwitchStatementOutcome(
	pass *analysis.Pass,
	statement *ast.TypeSwitchStmt,
	loopLabel string,
) ps6088LoopOutcomes {
	clauses := make([]*ast.CaseClause, 0, len(statement.Body.List))
	defaultIndex := -1
	for _, item := range statement.Body.List {
		clause, ok := item.(*ast.CaseClause)
		if !ok {
			return ps6088LoopOutcomes{fallthroughPossible: true}
		}
		clauses = append(clauses, clause)
		if len(clause.List) == 0 {
			defaultIndex = len(clauses) - 1
		}
	}
	return ps6088PossibleClauseOutcomes(pass, statement, clauses, defaultIndex, loopLabel)
}

func ps6088PossibleClauseOutcomes(
	pass *analysis.Pass,
	statement ast.Stmt,
	clauses []*ast.CaseClause,
	defaultIndex int,
	loopLabel string,
) ps6088LoopOutcomes {
	outcome := ps6088LoopOutcomes{fallthroughPossible: defaultIndex < 0}
	for index := range clauses {
		current := ps6088SelectedClauseOutcome(pass, statement, clauses, index, loopLabel)
		outcome.fallthroughPossible = outcome.fallthroughPossible || current.fallthroughPossible
		outcome.next = outcome.next || current.next
		outcome.intraBodyCycle = outcome.intraBodyCycle || current.intraBodyCycle
	}
	return outcome
}

func ps6088SelectedClauseOutcome(
	pass *analysis.Pass,
	statement ast.Stmt,
	clauses []*ast.CaseClause,
	index int,
	loopLabel string,
) ps6088LoopOutcomes {
	result := ps6088StatementOutcomes(pass, clauses[index].Body, loopLabel)
	breaks := ps6088BreakTargetsStatement(pass, statement, clauses[index])
	if ps6088ClauseFallsThrough(clauses[index]) && result.fallthroughPossible && index+1 < len(clauses) {
		next := ps6088SelectedClauseOutcome(pass, statement, clauses, index+1, loopLabel)
		next.fallthroughPossible = breaks || next.fallthroughPossible
		next.next = result.next || next.next
		next.intraBodyCycle = result.intraBodyCycle || next.intraBodyCycle
		return next
	}
	result.fallthroughPossible = breaks || result.fallthroughPossible
	return result
}

func ps6088SelectStatementOutcome(
	pass *analysis.Pass,
	statement *ast.SelectStmt,
	loopLabel string,
) ps6088LoopOutcomes {
	result := ps6088LoopOutcomes{}
	closedSendReady := ps6088SelectHasClosedSend(pass, statement)
	for _, item := range statement.Body.List {
		clause, ok := item.(*ast.CommClause)
		if !ok {
			return ps6088LoopOutcomes{fallthroughPossible: true}
		}
		if clause.Comm == nil && closedSendReady ||
			clause.Comm != nil && (ps6088NilCommClause(pass, clause) ||
				ps6088ClosedSendComm(pass, clause.Comm)) {
			continue
		}
		current := ps6088LoopOutcomes{}
		assignment, assigned := clause.Comm.(*ast.AssignStmt)
		lhsReturns := true
		if assigned {
			for _, destination := range assignment.Lhs {
				lhsReturns = lhsReturns && ps6088AssignmentDestinationReturns(pass, destination)
			}
		}
		if lhsReturns {
			current = ps6088StatementOutcomes(pass, clause.Body, loopLabel)
		}
		if ps6088BreakTargetsStatement(pass, statement, clause) {
			current.fallthroughPossible = true
		}
		result.fallthroughPossible = result.fallthroughPossible || current.fallthroughPossible
		result.next = result.next || current.next
		result.intraBodyCycle = result.intraBodyCycle || current.intraBodyCycle
	}
	return result
}

func ps6088ClauseFallsThrough(clause *ast.CaseClause) bool {
	if len(clause.Body) == 0 {
		return false
	}
	branch, ok := clause.Body[len(clause.Body)-1].(*ast.BranchStmt)
	return ok && branch.Tok == token.FALLTHROUGH
}

func ps6088BreakTargetsStatement(
	pass *analysis.Pass,
	target ast.Stmt,
	body ast.Node,
) bool {
	found := false
	astutil.WithStack(body, func(node ast.Node, stack []ast.Node) bool {
		if found || node == nil {
			return false
		}
		if _, literal := node.(*ast.FuncLit); literal {
			return false
		}
		branch, ok := node.(*ast.BranchStmt)
		if !ok || branch.Tok != token.BREAK || !ps6088StaticPathLive(pass, branch, stack) {
			return true
		}
		if branch.Label != nil {
			found = ps6088LabeledBreakTargets(pass, branch, target)
			return !found
		}
		for index := len(stack) - 1; index >= 0; index-- {
			switch nested := stack[index].(type) {
			case *ast.ForStmt, *ast.RangeStmt, *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt:
				found = nested == target
				return false
			}
		}
		found = true
		return false
	})
	return found
}

func ps6088StatementHeaderReturns(pass *analysis.Pass, statement ast.Stmt) bool {
	switch value := statement.(type) {
	case *ast.LabeledStmt:
		return ps6088StatementHeaderReturns(pass, value.Stmt)
	case *ast.IfStmt:
		return ps6088OptionalSimpleStatementReturns(pass, value.Init) &&
			ps6088ExpressionReturnsNormally(pass, value.Cond)
	case *ast.SwitchStmt:
		return ps6088OptionalSimpleStatementReturns(pass, value.Init) &&
			(value.Tag == nil || ps6088ExpressionReturnsNormally(pass, value.Tag)) &&
			ps6088SwitchCaseExpressionsReturn(pass, value)
	case *ast.TypeSwitchStmt:
		return ps6088OptionalSimpleStatementReturns(pass, value.Init) &&
			ps6088OptionalSimpleStatementReturns(pass, value.Assign)
	case *ast.ForStmt:
		if !ps6088OptionalSimpleStatementReturns(pass, value.Init) ||
			(value.Cond != nil && !ps6088ExpressionReturnsNormally(pass, value.Cond)) {
			return false
		}
		return !ps6088LoopDefinitelyNonempty(pass, value) || ps6088LoopHasReachableBreak(pass, value) ||
			ps6088OptionalSimpleStatementReturns(pass, value.Post)
	case *ast.RangeStmt:
		return ps6088RangeHeaderReturns(pass, value)
	case *ast.SelectStmt:
		viable := false
		for _, item := range value.Body.List {
			clause, ok := item.(*ast.CommClause)
			if !ok {
				return false
			}
			if clause.Comm == nil {
				viable = true
				continue
			}
			if !ps6088SelectCommOperandsReturn(pass, clause.Comm) {
				return false
			}
			if !ps6088NilCommClause(pass, clause) {
				viable = true
			}
		}
		return viable
	}
	return true
}

func ps6088RangeHeaderReturns(pass *analysis.Pass, statement *ast.RangeStmt) bool {
	rangeValueEvaluated := statement.Value != nil
	constantArrayRange := !rangeValueEvaluated &&
		ps6088ArrayOrPointerArray(pass.TypesInfo.TypeOf(statement.X)) &&
		ps6088ConstantArrayRangeExpression(pass, statement.X)
	if (!constantArrayRange && !ps6088ExpressionReturnsNormally(pass, statement.X)) ||
		(rangeValueEvaluated && ps6088NilPointerToArrayExpression(
			pass, statement.X, pass.TypesInfo.TypeOf(statement.X),
		)) {
		return false
	}
	rangeType := pass.TypesInfo.TypeOf(statement.X)
	if rangeType == nil {
		return true
	}
	if _, iterator := types.Unalias(rangeType).Underlying().(*types.Signature); iterator &&
		ps6088NilFunctionExpression(pass, statement.X, rangeType) {
		return false
	}
	_, channel := types.Unalias(rangeType).Underlying().(*types.Chan)
	return !channel || !ps6088NilRangeExpression(pass, statement.X, rangeType)
}

func ps6088SwitchCaseExpressionsReturn(pass *analysis.Pass, statement *ast.SwitchStmt) bool {
	tag := constant.MakeBool(true)
	if statement.Tag != nil {
		tag = pass.TypesInfo.Types[statement.Tag].Value
	}
	for _, item := range statement.Body.List {
		clause, ok := item.(*ast.CaseClause)
		if !ok {
			return false
		}
		for _, expression := range clause.List {
			if !ps6088ExpressionReturnsNormally(pass, expression) {
				return false
			}
			value := pass.TypesInfo.Types[expression].Value
			if tag != nil && value != nil && constant.Compare(tag, token.EQL, value) {
				return true
			}
		}
	}
	return true
}

func ps6088LoopDefinitelyNonempty(pass *analysis.Pass, loop ast.Node) bool {
	empty, known := ps6088LoopAtMost(pass, loop, 0)
	return known && !empty
}

func ps6088LoopHasReachableBreak(pass *analysis.Pass, loop ast.Node) bool {
	found := false
	body := astutil.LoopBody(loop)
	astutil.WithStack(body, func(current ast.Node, stack []ast.Node) bool {
		if found || current == nil {
			return false
		}
		if _, literal := current.(*ast.FuncLit); literal {
			return false
		}
		branch, ok := current.(*ast.BranchStmt)
		if !ok || branch.Tok != token.BREAK || !ps6088StaticPathLive(pass, branch, stack) {
			return true
		}
		if branch.Label != nil {
			found = ps6088LabeledBreakTargets(pass, branch, loop)
			return false
		}
		for index := len(stack) - 1; index >= 0; index-- {
			switch stack[index].(type) {
			case *ast.ForStmt, *ast.RangeStmt, *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt:
				return false
			}
		}
		found = true
		return false
	})
	return found
}

func ps6088LabeledBreakTargets(
	pass *analysis.Pass,
	branch *ast.BranchStmt,
	target ast.Node,
) bool {
	return ps6088LabeledBranchTarget(pass, branch) == target
}

func ps6088LabeledBranchTarget(pass *analysis.Pass, branch *ast.BranchStmt) ast.Stmt {
	if branch.Label == nil {
		return nil
	}
	label := pass.TypesInfo.Uses[branch.Label]
	if label == nil {
		return nil
	}
	return ps6088LabelTargetIndex(pass).targets[label]
}

func ps6088LabelTargetIndex(pass *analysis.Pass) *ps6088LabelTargetPassIndex {
	passIndex, ok := ps6088LabelTargets.Load(pass)
	if !ok {
		passIndex, _ = ps6088LabelTargets.LoadOrStore(pass, &ps6088LabelTargetPassIndex{})
	}
	index := passIndex.(*ps6088LabelTargetPassIndex)
	index.once.Do(func() {
		index.targets = make(map[types.Object]ast.Stmt)
		index.labels = make(map[ast.Stmt]string)
		for _, file := range pass.Files {
			ast.Inspect(file, func(node ast.Node) bool {
				statement, ok := node.(*ast.LabeledStmt)
				if !ok {
					return true
				}
				if object := pass.TypesInfo.Defs[statement.Label]; object != nil {
					index.targets[object] = statement.Stmt
					index.labels[statement.Stmt] = object.Name()
				}
				return true
			})
		}
	})
	return index
}

func ps6088StatementLabel(pass *analysis.Pass, target ast.Stmt) string {
	return ps6088LabelTargetIndex(pass).labels[target]
}

func ps6088NilPointerToArrayExpression(
	pass *analysis.Pass,
	expression ast.Expr,
	valueType types.Type,
) bool {
	if valueType == nil {
		return false
	}
	pointer, ok := types.Unalias(valueType).Underlying().(*types.Pointer)
	if !ok {
		return false
	}
	if _, ok := types.Unalias(pointer.Elem()).Underlying().(*types.Array); !ok {
		return false
	}
	return ps6088NilPointerOrSliceExpression(pass, expression, valueType)
}

func ps6088OptionalSimpleStatementReturns(pass *analysis.Pass, statement ast.Stmt) bool {
	return statement == nil || ps6088StatementReturnsNormally(pass, statement, false)
}

func ps6088ExpressionReturnsNormally(pass *analysis.Pass, expression ast.Expr) bool {
	if expression == nil {
		return true
	}
	return ps6088ExpressionReturnIndexEntry(pass, expression).returns
}

func ps6088ExpressionCallReached(
	pass *analysis.Pass,
	expression ast.Expr,
	call *ast.CallExpr,
	stack []ast.Node,
) bool {
	for index, parent := range stack {
		child := ast.Node(call)
		if index+1 < len(stack) {
			child = stack[index+1]
		}
		if !ps6088EvaluationPrefixReturns(pass, parent, child, call) {
			return false
		}
	}
	return expression == call || expression.Pos() <= call.Pos() && call.End() <= expression.End()
}

func ps6088NodeEntryReturns(pass *analysis.Pass, target ast.Node, stack []ast.Node) bool {
	for index, parent := range stack {
		child := target
		if index+1 < len(stack) {
			child = stack[index+1]
		}
		if !ps6088EvaluationPrefixReturns(pass, parent, child, target) {
			return false
		}
	}
	return true
}

func ps6088EvaluationPrefixReturns(
	pass *analysis.Pass,
	parent ast.Node,
	child ast.Node,
	target ast.Node,
) bool {
	returns := func(expressions ...ast.Expr) bool {
		for _, expression := range expressions {
			if expression != nil && !ps6088ExpressionReturnsNormally(pass, expression) {
				return false
			}
		}
		return true
	}
	containsTarget := func(node ast.Node) bool {
		return node != nil && node.Pos() <= target.Pos() && target.End() <= node.End()
	}
	switch value := parent.(type) {
	case *ast.CallExpr:
		if invoked := ps6088InvokedFuncLit(pass, value); invoked != nil && containsTarget(invoked.Body) {
			return returns(value.Fun) && returns(value.Args...)
		}
		if containsTarget(value.Fun) {
			return true
		}
		if !returns(value.Fun) {
			return false
		}
		for _, argument := range value.Args {
			if containsTarget(argument) {
				return true
			}
			if !returns(argument) {
				return false
			}
		}
	case *ast.BinaryExpr:
		return child == value.X || returns(value.X)
	case *ast.IndexExpr:
		return child == value.X || returns(value.X)
	case *ast.IndexListExpr:
		return child == value.X || returns(value.X)
	case *ast.SliceExpr:
		for _, operand := range []ast.Expr{value.X, value.Low, value.High, value.Max} {
			if operand == child || containsTarget(operand) {
				return true
			}
			if !returns(operand) {
				return false
			}
		}
	case *ast.CompositeLit:
		for _, element := range value.Elts {
			if element == child || containsTarget(element) {
				return true
			}
			if !returns(element) {
				return false
			}
		}
	case *ast.KeyValueExpr:
		return child == value.Key || returns(value.Key)
	case *ast.SendStmt:
		return child == value.Chan || returns(value.Chan)
	case *ast.AssignStmt:
		for _, operand := range value.Lhs {
			if operand == child || containsTarget(operand) {
				return true
			}
			if !returns(operand) {
				return false
			}
		}
		for _, operand := range value.Rhs {
			if operand == child || containsTarget(operand) {
				return true
			}
			if !returns(operand) {
				return false
			}
		}
	case *ast.ReturnStmt:
		for _, result := range value.Results {
			if result == child || containsTarget(result) {
				return true
			}
			if !returns(result) {
				return false
			}
		}
	case *ast.ValueSpec:
		for _, operand := range value.Values {
			if containsTarget(operand) {
				return true
			}
			if !returns(operand) {
				return false
			}
		}
	case *ast.GenDecl:
		for _, specification := range value.Specs {
			if specification.Pos() <= target.Pos() && target.End() <= specification.End() {
				return true
			}
			values, ok := specification.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, operand := range values.Values {
				if !returns(operand) {
					return false
				}
			}
		}
	case *ast.IfStmt:
		if child != value.Init && !ps6088OptionalSimpleStatementReturns(pass, value.Init) {
			return false
		}
		return child == value.Init || child == value.Cond || returns(value.Cond)
	case *ast.SwitchStmt:
		if child != value.Init && !ps6088OptionalSimpleStatementReturns(pass, value.Init) {
			return false
		}
		if child == value.Init || child == value.Tag {
			return true
		}
		return (value.Tag == nil || returns(value.Tag)) &&
			ps6088SwitchEvaluationPrefixReturns(pass, value, target)
	case *ast.SelectStmt:
		return ps6088SelectEvaluationPrefixReturns(pass, value, target)
	case *ast.ForStmt:
		if child != value.Init && !ps6088OptionalSimpleStatementReturns(pass, value.Init) {
			return false
		}
		if child == value.Init || child == value.Cond {
			return true
		}
		if value.Cond != nil && !returns(value.Cond) {
			return false
		}
		if child != value.Post {
			return true
		}
		outcomes := ps6088StatementOutcomes(
			pass, value.Body.List, ps6088StatementLabel(pass, value),
		)
		return outcomes.fallthroughPossible || outcomes.next
	case *ast.RangeStmt:
		if child == value.X {
			return true
		}
		if !ps6088RangeHeaderReturns(pass, value) {
			return false
		}
		empty, known := ps6088LoopAtMost(pass, value, 0)
		if known && empty {
			return false
		}
		if child == value.Key {
			return true
		}
		if child == value.Value {
			return value.Key == nil || returns(value.Key)
		}
		if value.Tok == token.ASSIGN {
			for _, destination := range []ast.Expr{value.Key, value.Value} {
				if !ps6088AssignmentDestinationReturns(pass, destination) {
					return false
				}
			}
		}
		return true
	case *ast.TypeSwitchStmt:
		if child != value.Init && !ps6088OptionalSimpleStatementReturns(pass, value.Init) {
			return false
		}
		return child == value.Init || child == value.Assign ||
			ps6088OptionalSimpleStatementReturns(pass, value.Assign)
	}
	return true
}

func ps6088AssignmentDestinationReturns(pass *analysis.Pass, destination ast.Expr) bool {
	return destination == nil || ps6088ExpressionReturnsNormally(pass, destination) &&
		!ps6088NilMapDestination(pass, destination)
}

func ps6088SwitchEvaluationPrefixReturns(
	pass *analysis.Pass,
	statement *ast.SwitchStmt,
	target ast.Node,
) bool {
	for _, item := range statement.Body.List {
		clause, ok := item.(*ast.CaseClause)
		if !ok {
			return false
		}
		for _, expression := range clause.List {
			if expression.Pos() <= target.Pos() && target.End() <= expression.End() {
				return true
			}
			if !ps6088ExpressionReturnsNormally(pass, expression) {
				return false
			}
		}
		for _, body := range clause.Body {
			if body.Pos() <= target.Pos() && target.End() <= body.End() {
				return true
			}
		}
	}
	return true
}

func ps6088SelectEvaluationPrefixReturns(
	pass *analysis.Pass,
	statement *ast.SelectStmt,
	target ast.Node,
) bool {
	var targetClause *ast.CommClause
	var targetAssignment *ast.AssignStmt
	targetCommunication := false
	for _, item := range statement.Body.List {
		clause, ok := item.(*ast.CommClause)
		if !ok {
			return false
		}
		for _, body := range clause.Body {
			if body.Pos() <= target.Pos() && target.End() <= body.End() {
				targetClause = clause
			}
		}
		if assignment, ok := clause.Comm.(*ast.AssignStmt); ok {
			for _, destination := range assignment.Lhs {
				if destination.Pos() <= target.Pos() && target.End() <= destination.End() {
					targetAssignment = assignment
				}
			}
		}
	}
	for _, item := range statement.Body.List {
		clause := item.(*ast.CommClause)
		if targetClause == nil && targetAssignment == nil &&
			ps6088SelectCommPrefixReturns(pass, clause.Comm, target) {
			targetCommunication = true
		}
		if !ps6088SelectCommOperandsReturn(pass, clause.Comm) {
			return false
		}
	}
	if targetCommunication {
		return true
	}
	if targetAssignment != nil {
		for _, destination := range targetAssignment.Lhs {
			if destination.Pos() <= target.Pos() && target.End() <= destination.End() {
				return true
			}
			if !ps6088ExpressionReturnsNormally(pass, destination) {
				return false
			}
		}
	}
	if targetClause != nil {
		if assignment, ok := targetClause.Comm.(*ast.AssignStmt); ok {
			for _, destination := range assignment.Lhs {
				if !ps6088AssignmentDestinationReturns(pass, destination) {
					return false
				}
			}
		}
	}
	return true
}

func ps6088SelectCommPrefixReturns(
	pass *analysis.Pass,
	statement ast.Stmt,
	target ast.Node,
) bool {
	contains := func(expression ast.Expr) bool {
		return expression != nil && expression.Pos() <= target.Pos() && target.End() <= expression.End()
	}
	switch communication := statement.(type) {
	case nil:
		return false
	case *ast.SendStmt:
		if contains(communication.Chan) {
			return true
		}
		if !ps6088ExpressionReturnsNormally(pass, communication.Chan) {
			return false
		}
		return contains(communication.Value)
	case *ast.ExprStmt:
		receive, ok := ps2110Unparen(communication.X).(*ast.UnaryExpr)
		return ok && receive.Op == token.ARROW && contains(receive.X)
	case *ast.AssignStmt:
		if len(communication.Rhs) != 1 {
			return false
		}
		receive, ok := ps2110Unparen(communication.Rhs[0]).(*ast.UnaryExpr)
		return ok && receive.Op == token.ARROW && contains(receive.X)
	}
	return false
}

func ps6088ExpressionReturnIndexEntry(
	pass *analysis.Pass,
	expression ast.Expr,
) *ps6088ExpressionReturnEntry {
	passIndex, ok := ps6088ExpressionReturns.Load(pass)
	if !ok {
		passIndex, _ = ps6088ExpressionReturns.LoadOrStore(pass, &ps6088ExpressionReturnPassIndex{})
	}
	index := passIndex.(*ps6088ExpressionReturnPassIndex)
	entry, ok := index.expressions.Load(expression)
	if !ok {
		entry, _ = index.expressions.LoadOrStore(expression, &ps6088ExpressionReturnEntry{})
	}
	result := entry.(*ps6088ExpressionReturnEntry)
	result.once.Do(func() {
		result.returns = ps6088ComputeExpressionReturnsNormally(pass, expression)
	})
	return result
}

func ps6088ComputeExpressionReturnsNormally(pass *analysis.Pass, expression ast.Expr) bool {
	nonreturn := false
	astutil.WithStack(expression, func(node ast.Node, stack []ast.Node) bool {
		if nonreturn || node == nil {
			return false
		}
		if literal, ok := node.(*ast.FuncLit); ok {
			invocation := ps6088ImmediateFuncLit(pass, literal, stack)
			return invocation != nil && !ps6088AsyncCall(invocation, stack, false)
		}
		current, ok := node.(ast.Expr)
		if !ok {
			return true
		}
		if !ps6088StaticPathLive(pass, current, stack) {
			return false
		}
		preventsReturn := !ps6088CommaOkTypeAssertion(current, stack) &&
			ps6088DirectExpressionPanics(pass, current)
		if call, ok := current.(*ast.CallExpr); ok {
			if ps6088CallOperandUnevaluated(pass, call) {
				return false
			}
			preventsReturn = preventsReturn || ps6088SynchronousCallPreventsReturn(pass, call, stack)
		}
		if preventsReturn {
			nonreturn = true
			return false
		}
		return true
	})
	return !nonreturn
}

func ps6088ReachableGoto(pass *analysis.Pass, statement ast.Stmt) bool {
	found := false
	astutil.WithStack(statement, func(node ast.Node, stack []ast.Node) bool {
		if found {
			return false
		}
		if _, literal := node.(*ast.FuncLit); literal {
			return false
		}
		branch, ok := node.(*ast.BranchStmt)
		if ok && branch.Tok == token.GOTO && ps6088StaticPathLive(pass, branch, stack) {
			found = true
			return false
		}
		return true
	})
	return found
}

func ps6088ElseOutcome(pass *analysis.Pass, statement ast.Stmt, loopLabel string) ps6088LoopOutcomes {
	if statement == nil {
		return ps6088LoopOutcomes{fallthroughPossible: true}
	}
	return ps6088StatementOutcome(pass, statement, loopLabel)
}

func ps6088LoopAtMost(pass *analysis.Pass, loop ast.Node, limit int64) (bool, bool) {
	if rangeLoop, ok := loop.(*ast.RangeStmt); ok {
		resolved, zero, safe := ps6088StableExpression(pass, rangeLoop.X)
		if !safe {
			return false, false
		}
		if zero {
			typeOf := pass.TypesInfo.TypeOf(rangeLoop.X)
			if typeOf == nil {
				return false, false
			}
			switch underlying := types.Unalias(typeOf).Underlying().(type) {
			case *types.Basic, *types.Slice, *types.Map, *types.Chan, *types.Signature:
				return 0 <= limit, true
			case *types.Array:
				return underlying.Len() <= limit, true
			case *types.Pointer:
				array, ok := types.Unalias(underlying.Elem()).Underlying().(*types.Array)
				if ok {
					return array.Len() <= limit, true
				}
			}
			return false, false
		}
		return ps6088DomainAtMost(pass, resolved, limit)
	}
	if forLoop, ok := loop.(*ast.ForStmt); ok {
		if domain, found := ps6088ForLoopRawDomain(pass, forLoop); found {
			return ps6088DomainAtMost(pass, domain, limit)
		}
	}
	domain, ok := ps6086LoopDomainExpression(pass, loop)
	if !ok {
		return false, false
	}
	return ps6088DomainAtMost(pass, domain, limit)
}

func ps6088ForLoopRawDomain(pass *analysis.Pass, loop *ast.ForStmt) (ast.Expr, bool) {
	assignment, ok := loop.Init.(*ast.AssignStmt)
	if !ok || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 ||
		assignment.Tok != token.DEFINE || !ps6086ConstantZero(pass, assignment.Rhs[0]) {
		return nil, false
	}
	indexIdentifier, ok := assignment.Lhs[0].(*ast.Ident)
	if !ok {
		return nil, false
	}
	index := pass.TypesInfo.ObjectOf(indexIdentifier)
	comparison, ok := loop.Cond.(*ast.BinaryExpr)
	if !ok {
		return nil, false
	}
	if identifier, ok := comparison.X.(*ast.Ident); ok &&
		pass.TypesInfo.ObjectOf(identifier) == index && comparison.Op == token.LSS {
		return comparison.Y, true
	}
	if identifier, ok := comparison.Y.(*ast.Ident); ok &&
		pass.TypesInfo.ObjectOf(identifier) == index && comparison.Op == token.GTR {
		return comparison.X, true
	}
	return nil, false
}

func ps6088DomainAtMost(pass *analysis.Pass, expression ast.Expr, limit int64) (bool, bool) {
	if value := pass.TypesInfo.Types[expression].Value; value != nil {
		switch value.Kind() {
		case constant.Int:
			return constant.Compare(value, token.LEQ, constant.MakeInt64(limit)), true
		case constant.String:
			count := int64(utf8.RuneCountInString(constant.StringVal(value)))
			return count <= limit, true
		}
	}
	if call, ok := ps2110Unparen(expression).(*ast.CallExpr); ok {
		if typedBuiltinName(pass, call.Fun, "min") && len(call.Args) > 0 && !call.Ellipsis.IsValid() {
			for _, argument := range call.Args {
				if atMost, known := ps6088DomainAtMost(pass, argument, limit); known && atMost {
					return true, true
				}
			}
			return false, false
		}
		if argument, conversion := ps6088SameBasicConversion(pass, call); conversion {
			return ps6088DomainAtMost(pass, argument, limit)
		}
	}
	typeOf := pass.TypesInfo.TypeOf(expression)
	if typeOf == nil {
		return false, false
	}
	switch underlying := types.Unalias(typeOf).Underlying().(type) {
	case *types.Array:
		return underlying.Len() <= limit, true
	case *types.Pointer:
		array, ok := types.Unalias(underlying.Elem()).Underlying().(*types.Array)
		if ok {
			return array.Len() <= limit, true
		}
	case *types.Slice:
		if ps6088NilRangeExpression(pass, expression, typeOf) {
			return true, true
		}
		literal, ok := ps2110Unparen(expression).(*ast.CompositeLit)
		if !ok {
			return false, false
		}
		length, ok := ps6088CompositeLength(pass, literal)
		return length <= limit, ok
	case *types.Map:
		if ps6088NilRangeExpression(pass, expression, typeOf) {
			return true, true
		}
		literal, ok := ps2110Unparen(expression).(*ast.CompositeLit)
		if !ok || int64(len(literal.Elts)) > limit {
			return false, ok
		}
		return true, true
	}
	return false, false
}

func ps6088SameBasicConversion(pass *analysis.Pass, call *ast.CallExpr) (ast.Expr, bool) {
	if len(call.Args) != 1 || call.Ellipsis.IsValid() || !pass.TypesInfo.Types[call.Fun].IsType() {
		return nil, false
	}
	from, to := pass.TypesInfo.TypeOf(call.Args[0]), pass.TypesInfo.TypeOf(call)
	if from == nil || to == nil {
		return nil, false
	}
	fromBasic, fromOK := types.Unalias(from).Underlying().(*types.Basic)
	toBasic, toOK := types.Unalias(to).Underlying().(*types.Basic)
	return call.Args[0], fromOK && toOK && fromBasic.Kind() == toBasic.Kind()
}

func ps6088CompositeLength(pass *analysis.Pass, literal *ast.CompositeLit) (int64, bool) {
	var length, next int64
	for _, element := range literal.Elts {
		if keyed, ok := element.(*ast.KeyValueExpr); ok {
			value := pass.TypesInfo.Types[keyed.Key].Value
			if value == nil || value.Kind() != constant.Int {
				return 0, false
			}
			index, exact := constant.Int64Val(value)
			if !exact || index < 0 {
				return 0, false
			}
			next = index
		}
		next++
		if next > length {
			length = next
		}
	}
	return length, true
}

func ps6088Boolean(pass *analysis.Pass, expression ast.Expr) (bool, bool) {
	value := pass.TypesInfo.Types[expression].Value
	if value == nil || value.Kind() != constant.Bool {
		return false, false
	}
	return constant.BoolVal(value), true
}

func ps6088PositionLive(pass *analysis.Pass, body *ast.BlockStmt, position token.Pos) bool {
	flow := cfg.New(body, func(call *ast.CallExpr) bool { return !ps6088NonreturnCall(pass, call) })
	return ps6088CFGPositionLive(pass, flow, position)
}

func ps6088CFGPositionLive(pass *analysis.Pass, flow *cfg.CFG, position token.Pos) bool {
	if len(flow.Blocks) == 0 {
		return false
	}
	target := ps6079CFGBlockAt(flow, position)
	if target == nil || !target.Live {
		return false
	}
	seen := make(map[*cfg.Block]bool)
	queue := []*cfg.Block{flow.Blocks[0]}
	for len(queue) > 0 {
		block := queue[0]
		queue = queue[1:]
		if seen[block] {
			continue
		}
		seen[block] = true
		if block == target {
			return !ps6088BlockHasNonreturnBefore(pass, block, position)
		}
		queue = append(queue, ps6088BlockSuccessors(pass, block)...)
	}
	return false
}

func ps6088BlockSuccessors(pass *analysis.Pass, block *cfg.Block) []*cfg.Block {
	if ps6088BlockHasNonreturnBefore(pass, block, token.NoPos) {
		return nil
	}
	if len(block.Succs) == 2 {
		if condition, ok := ps6088CFGCondition(pass, block); ok {
			if condition {
				return block.Succs[:1]
			}
			return block.Succs[1:]
		}
	}
	return block.Succs
}

func ps6088CFGCondition(pass *analysis.Pass, block *cfg.Block) (bool, bool) {
	if len(block.Nodes) == 0 {
		return false, false
	}
	expression, ok := block.Nodes[len(block.Nodes)-1].(ast.Expr)
	if !ok {
		return false, false
	}
	condition, conditionKnown := ps6088CFGBoolean(pass, expression)
	value := pass.TypesInfo.Types[expression].Value
	if !conditionKnown && value == nil {
		return false, false
	}
	switchStatement := ps6088ConditionSwitch(pass, expression)
	if switchStatement == nil || switchStatement.Tag == nil {
		return condition, conditionKnown
	}
	tag := pass.TypesInfo.Types[switchStatement.Tag].Value
	if tag == nil {
		return false, false
	}
	if tag.Kind() == constant.Bool && conditionKnown {
		return constant.BoolVal(tag) == condition, true
	}
	if value == nil {
		return false, false
	}
	return constant.Compare(tag, token.EQL, value), true
}

func ps6088CFGBoolean(pass *analysis.Pass, expression ast.Expr) (bool, bool) {
	if condition, known := ps6088Boolean(pass, expression); known {
		return condition, true
	}
	unparenthesized := ps2110Unparen(expression)
	if unary, ok := unparenthesized.(*ast.UnaryExpr); ok && unary.Op == token.NOT {
		condition, known := ps6088CFGBoolean(pass, unary.X)
		return !condition, known
	}
	binary, ok := unparenthesized.(*ast.BinaryExpr)
	if !ok || binary.Op != token.LAND && binary.Op != token.LOR {
		return false, false
	}
	left, leftKnown := ps6088CFGBoolean(pass, binary.X)
	right, rightKnown := ps6088CFGBoolean(pass, binary.Y)
	switch binary.Op {
	case token.LAND:
		if leftKnown && !left || rightKnown && !right {
			return false, true
		}
		if leftKnown && left {
			return right, rightKnown
		}
	case token.LOR:
		if leftKnown && left || rightKnown && right {
			return true, true
		}
		if leftKnown && !left {
			return right, rightKnown
		}
	}
	return false, false
}

func ps6088ConditionSwitch(pass *analysis.Pass, expression ast.Expr) *ast.SwitchStmt {
	entry, ok := ps6088SwitchOwners.Load(pass)
	if !ok {
		entry, _ = ps6088SwitchOwners.LoadOrStore(pass, new(ps6088SwitchOwnerIndex))
	}
	index := entry.(*ps6088SwitchOwnerIndex)
	index.once.Do(func() {
		index.owners = ps6088IndexSwitchOwners(pass)
	})
	return index.owners[expression]
}

func ps6088IndexSwitchOwners(pass *analysis.Pass) map[ast.Expr]*ast.SwitchStmt {
	owners := make(map[ast.Expr]*ast.SwitchStmt)
	for _, file := range pass.Files {
		ast.Inspect(file, func(node ast.Node) bool {
			statement, ok := node.(*ast.SwitchStmt)
			if !ok {
				return true
			}
			for _, item := range statement.Body.List {
				clause, ok := item.(*ast.CaseClause)
				if !ok {
					continue
				}
				for _, condition := range clause.List {
					owners[condition] = statement
				}
			}
			return true
		})
	}
	return owners
}

func ps6088BlockHasNonreturnBefore(pass *analysis.Pass, block *cfg.Block, position token.Pos) bool {
	for _, node := range block.Nodes {
		statement, ok := node.(*ast.ExprStmt)
		if !ok || position.IsValid() && statement.End() > position {
			continue
		}
		call, ok := ps2110Unparen(statement.X).(*ast.CallExpr)
		if ok && (ps6088NonreturnCall(pass, call) || ps6088CallInvocationPanics(pass, call)) {
			return true
		}
	}
	return false
}

func ps6088NonreturnCall(pass *analysis.Pass, call *ast.CallExpr) bool {
	if ps6079PanicCall(pass, call) {
		return true
	}
	function, _, ok := typedCallee(pass, call.Fun)
	if !ok || function.Pkg() == nil {
		return false
	}
	path := function.Pkg().Path()
	name := function.Name()
	return (path == "os" && name == "Exit") || (path == "runtime" && name == "Goexit")
}

func ps6088TestFile(pass *analysis.Pass, file *ast.File) bool {
	position := pass.Fset.PositionFor(file.Pos(), false)
	return strings.HasSuffix(position.Filename, "_test.go")
}
