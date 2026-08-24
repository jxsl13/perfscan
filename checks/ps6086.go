package checks

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"go/version"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS6086 implements owner issue #829. It recognizes the narrow all-goroutine
// fan-out shape where a loop launches inline workers against a function-local
// sync.WaitGroup and the launching goroutine contributes no equivalent work
// before waiting. Moving one existing chunk onto the caller removes one launch,
// but the issue's production result showed why that is benchmark-required
// guidance rather than an automatic performance fix.
var PS6086 = register(&lint.Check{
	ID:       "PS6086",
	Category: "verify",
	Slug:     "caller-participation-needs-latency-benchmark",
	Level:    lint.LevelAggressive,
	Doc: lint.Documentation{
		Title: "all-goroutine fan-out leaves the caller waiting; caller participation needs a latency benchmark, not an allocation-only rewrite",
		Text: `A fan-out helper can launch every chunk as a goroutine and leave its
caller doing no useful work before sync.WaitGroup.Wait. Running one existing
chunk on the caller removes exactly one goroutine launch per dispatch, which can
reduce allocations, but it also changes scheduler placement. Fewer launches and
allocations therefore do not prove lower latency.

PS6086 reports the deliberately narrow source shape it can prove:
  - a for loop or value-domain range loop has one direct, unconditional inline
    launch and cannot return, panic, block forever, or jump around the launch
    or matched join; channel operations and range-over-function iteration are
    declined because synchronous work can block send/receive/yield progression;
  - the goroutine registers defer wg.Done() as its first statement and does not
    otherwise change that barrier;
  - wg is a fresh local zero value, new(sync.WaitGroup), or composite literal
    that is neither copied nor passed elsewhere before the join; exactly one
    direct per-iteration Add(1) is the only barrier operation outside the
    matched worker across the whole loop, no earlier function-scope generation
    or enclosing-loop backedge generation is active, and Wait immediately
    follows the loop; and
  - the launching goroutine does not execute a proven equivalent chunk before
    waiting. That suppression is available only for a call-shaped worker
    (defer Done plus one stable call), with the same method receiver when the
    worker is a method (including side-effect-free canonical selector/index
    receiver identity and launch-time inline-parameter snapshots whose source
    is a direct binding or recursively value-only expression and remains stable
    and unexposed through argument evaluation; reference-derived sources,
    address-sensitive copies, pre-launch aliases, receives, and other effectful
    receivers are declined, as are generic receiver alias paths whose type-set
    storage identity would require a wider ownership proof),
    exactly matching normalized worker arguments built from iteration values
    and constants, the same generic instantiation and variadic expansion, no
    variadic inline-parameter remapping, and a directly reached synchronous
    caller call, including a single direct call whose result is assigned or
    declared. Mutable or address-exposed function
    bindings—including implicit address-taking by pointer-receiver methods—do
    not qualify as stable workers, including mutations after a join that can
    race the next iteration of an enclosing loop.

Helpers with an existing terminating serial fallback behind an integer
work/parallelism threshold stay silent only when that branch loops over the
exact same domain, directly invokes the same proven worker with exactly
matching normalized arguments, and ends in an unconditionally reached return.
The fallback must dominate the fan-out, and the compared domain expression and
callable binding must remain stable through fan-out and enclosing-loop
backedges. Referential domains,
including reference-bearing selector results, are declined because an
intervening call can mutate them without rebinding the base object. Value-only
selector paths remain eligible even when unrelated fields in the containing
struct are reference-bearing.
Else-if fallbacks qualify only when every preceding arm terminates. Their
crossover is already tuned, so a generic caller-participation suggestion would
be especially weak. Lookalike Add/Done/Wait methods, aliases of shared or
caller-owned barriers, completion registered after an early-exit path,
    captured reused or explicitly reassigned loop variables, non-value snapshots,
    implicitly addressed method-value snapshots, array-backed slice aliases,
    conditional/deferred caller work, and ambiguous worker bodies are excluded from
    the corresponding proof. Loop variables declared by the loop are treated as
    per-iteration values only when the analyzed file's language version is Go
    1.22 or newer; older or unknown language semantics remain conservative.

This is advisory only. Compare the unchanged all-goroutine control with a
work-first candidate in the SAME production-shaped harness. Preserve worker
count, chunk boundaries, arithmetic, and an exact output digest; record
allocs/op separately from wall time; use independent order-alternating samples.
Keep caller participation only when latency improves beyond the campaign's
retained gate.`,
		Before: `var wg sync.WaitGroup
for chunk := range chunks {
	wg.Add(1)
	go func(chunk int) {
		defer wg.Done()
		work(chunk)
	}(chunk)
}
wg.Wait() // caller only waits`,
		After: `// Benchmark-required candidate, not an automatic rewrite:
// launch N-1 unchanged chunks, run one unchanged chunk on the caller,
// then wait. Retain only after allocation AND production-latency evidence.`,
		MeasuredWin: `GoAI QMatMul on Apple M2 Pro, GOMAXPROCS=8: caller
participation removed 2 allocs/op at the Q4_K leaf and reduced 64-step
TinyLlama allocations by 7.06%, yet latency regressed from 1.980 s to 2.092 s
(+5.66%, p=0.019) with the exact digest preserved. The candidate was rejected.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6086",
		Doc:  "all-goroutine fan-out whose caller only waits; require latency evidence before caller participation",
		Run:  runPS6086,
	},
})

type ps6086CallableKey struct {
	object             types.Object
	receiverExpression string
	receiverObjects    [4]types.Object
	receiverCount      int
	receiverSnapshots  [4]types.Object
	snapshotCount      int
	snapshotAt         token.Pos
	literalAt          token.Pos
	callableType       string
	instantiation      string
	arguments          string
}

type ps6086ExpressionFingerprint struct {
	key       string
	iteration bool
}

func runPS6086(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil || strings.HasPrefix(function.Name.Name, "Benchmark") {
				continue
			}
			ast.Inspect(function.Body, func(node ast.Node) bool {
				if literal, ok := node.(*ast.FuncLit); ok && literal.Body != function.Body {
					return false
				}
				block, ok := node.(*ast.BlockStmt)
				if !ok {
					return true
				}
				ps6086InspectBlock(pass, function, block)
				return true
			})
		}
	}
	return nil, nil
}

func ps6086InspectBlock(pass *analysis.Pass, function *ast.FuncDecl, block *ast.BlockStmt) {
	for loopIndex, statement := range block.List {
		loop := ps6086Loop(statement)
		if loop == nil {
			continue
		}
		launches := ps6086GoStatements(loop)
		if len(launches) != 1 || !ps6086FanoutDomainSafe(pass, loop) {
			continue
		}
		for waitIndex := loopIndex + 1; waitIndex < len(block.List); waitIndex++ {
			waitObject, ok := ps6086DirectWait(pass, block.List[waitIndex])
			if !ok || !ps6086FreshLocalWaitGroup(pass, function, waitObject, block.List[waitIndex].Pos()) ||
				!ps6086NoBackedgeBarrierUse(
					pass, function.Body, waitObject, loop, block.List[waitIndex].End()) {
				continue
			}
			if !ps6086LaunchUnconditional(pass, loop, launches[0]) ||
				!ps6086JoinReached(pass, loop) ||
				!ps6086IterationCaptureSafe(pass, function.Body, loop, launches[0]) ||
				!ps6086NoPriorBarrierMethods(pass, function.Body, waitObject, loop.Pos()) {
				continue
			}
			matching := ps6086MatchingLaunches(pass, launches, waitObject)
			if len(matching) != 1 || !ps6086HasExactUnitAdd(pass, loop, waitObject, matching[0]) {
				continue
			}
			worker, workerKnown := ps6086WorkObject(pass, loop, matching[0], waitObject)
			workerKnown = workerKnown && ps6086CallableStable(
				pass, function.Body, &worker, function.Body.Pos(), block.List[waitIndex].End()) &&
				ps6086CallableBackedgeStable(
					pass, function.Body, &worker, loop, block.List[waitIndex].End())
			if ps6086CallerParticipates(pass, block.List[loopIndex:waitIndex], loop, &worker, workerKnown) ||
				ps6086HasThresholdFallback(
					pass, function.Body, block.List[:loopIndex], statement, loop, &worker, workerKnown,
					block.List[waitIndex].End()) ||
				waitIndex != loopIndex+1 {
				continue
			}
			pass.Reportf(matching[0].Go,
				"all-goroutine fan-out launches every chunk and the caller only waits on a function-local sync.WaitGroup; caller participation can avoid one goroutine launch per dispatch, but allocation reduction does not prove a latency win — benchmark the unchanged control and work-first candidate with separate allocs/op and production-shaped order-alternating wall-time gates (advisory, no automatic fix)")
			break
		}
	}
}

func ps6086NoBackedgeBarrierUse(
	pass *analysis.Pass,
	body *ast.BlockStmt,
	object *types.Var,
	matchedLoop ast.Node,
	after token.Pos,
) bool {
	safe := true
	ast.Inspect(body, func(node ast.Node) bool {
		if !safe {
			return false
		}
		var enclosing ast.Node
		var enclosingBody *ast.BlockStmt
		switch value := node.(type) {
		case *ast.ForStmt:
			enclosing = node
			enclosingBody = value.Body
		case *ast.RangeStmt:
			enclosing = node
			enclosingBody = value.Body
		}
		if enclosing == nil || enclosing == matchedLoop || object.Pos() >= enclosingBody.Pos() ||
			enclosing.Pos() >= matchedLoop.Pos() || enclosing.End() <= after {
			return true
		}
		ast.Inspect(enclosing, func(child ast.Node) bool {
			if !safe || child == nil {
				return false
			}
			if child.Pos() <= after {
				return true
			}
			if identifier, ok := child.(*ast.Ident); ok && pass.TypesInfo.Uses[identifier] == object {
				safe = false
				return false
			}
			call, ok := child.(*ast.CallExpr)
			if !ok {
				return true
			}
			for _, method := range []string{"Add", "Done", "Wait"} {
				receiver, matched := ps6086WaitGroupMethod(pass, call, method)
				if matched && receiver == object {
					safe = false
					return false
				}
			}
			return true
		})
		return safe
	})
	return safe
}

func ps6086Loop(statement ast.Stmt) ast.Node {
	if labeled, ok := statement.(*ast.LabeledStmt); ok {
		statement = labeled.Stmt
	}
	switch loop := statement.(type) {
	case *ast.ForStmt:
		return loop
	case *ast.RangeStmt:
		return loop
	default:
		return nil
	}
}

func ps6086FanoutDomainSafe(pass *analysis.Pass, loop ast.Node) bool {
	if !ps6086ChannelControlFree(pass, loop) {
		return false
	}
	rangeLoop, ok := loop.(*ast.RangeStmt)
	if !ok {
		return true
	}
	domain := pass.TypesInfo.TypeOf(rangeLoop.X)
	if domain == nil {
		return false
	}
	switch underlying := types.Unalias(domain).Underlying().(type) {
	case *types.Basic, *types.Array, *types.Slice, *types.Map:
		return true
	case *types.Pointer:
		_, array := types.Unalias(underlying.Elem()).Underlying().(*types.Array)
		return array
	default:
		return false
	}
}

func ps6086ChannelControlFree(pass *analysis.Pass, loop ast.Node) bool {
	safe := true
	ast.Inspect(loop, func(node ast.Node) bool {
		if !safe {
			return false
		}
		switch value := node.(type) {
		case *ast.CallExpr:
			function := value.Fun
			for {
				parenthesized, ok := function.(*ast.ParenExpr)
				if !ok {
					break
				}
				function = parenthesized.X
			}
			identifier, identifierOK := function.(*ast.Ident)
			if !identifierOK {
				break
			}
			builtin, builtinOK := pass.TypesInfo.ObjectOf(identifier).(*types.Builtin)
			if identifierOK && builtinOK && builtin.Name() == "close" {
				safe = false
				return false
			}
		case *ast.UnaryExpr:
			if value.Op == token.ARROW {
				safe = false
				return false
			}
		case *ast.SendStmt:
			safe = false
			return false
		case *ast.RangeStmt:
			domain := pass.TypesInfo.TypeOf(value.X)
			if domain == nil {
				safe = false
				return false
			}
			unaliased := types.Unalias(domain)
			if _, parameter := unaliased.(*types.TypeParam); parameter {
				safe = false
				return false
			}
			switch unaliased.Underlying().(type) {
			case *types.Chan, *types.Signature, *types.Interface:
				safe = false
				return false
			}
		}
		return true
	})
	return safe
}

func ps6086GoStatements(loop ast.Node) []*ast.GoStmt {
	var result []*ast.GoStmt
	var body *ast.BlockStmt
	switch value := loop.(type) {
	case *ast.ForStmt:
		body = value.Body
	case *ast.RangeStmt:
		body = value.Body
	}
	if body == nil {
		return nil
	}
	for _, statement := range body.List {
		if launch, ok := statement.(*ast.GoStmt); ok {
			result = append(result, launch)
		}
	}
	return result
}

func ps6086DirectWait(pass *analysis.Pass, statement ast.Stmt) (*types.Var, bool) {
	expression, ok := statement.(*ast.ExprStmt)
	if !ok {
		return nil, false
	}
	call, ok := expression.X.(*ast.CallExpr)
	if !ok {
		return nil, false
	}
	return ps6086WaitGroupMethod(pass, call, "Wait")
}

func ps6086WaitGroupMethod(pass *analysis.Pass, call *ast.CallExpr, method string) (*types.Var, bool) {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != method {
		return nil, false
	}
	function, ok := pass.TypesInfo.ObjectOf(selector.Sel).(*types.Func)
	if !ok || function.Pkg() == nil || function.Pkg().Path() != "sync" || function.Name() != method {
		return nil, false
	}
	receiver, ok := ps6086ReceiverObject(pass, selector.X).(*types.Var)
	return receiver, ok
}

func ps6086ReceiverObject(pass *analysis.Pass, expression ast.Expr) types.Object {
	switch value := expression.(type) {
	case *ast.Ident:
		return pass.TypesInfo.ObjectOf(value)
	case *ast.ParenExpr:
		return ps6086ReceiverObject(pass, value.X)
	case *ast.StarExpr:
		return ps6086ReceiverObject(pass, value.X)
	case *ast.UnaryExpr:
		if value.Op == token.AND {
			return ps6086ReceiverObject(pass, value.X)
		}
	}
	return nil
}

func ps6086FreshLocalWaitGroup(
	pass *analysis.Pass,
	function *ast.FuncDecl,
	object *types.Var,
	before token.Pos,
) bool {
	if object == nil || object.Pos() <= function.Body.Pos() || object.Pos() >= function.Body.End() {
		return false
	}
	fresh := false
	reassigned := false
	ast.Inspect(function.Body, func(node ast.Node) bool {
		if node == nil || node.Pos() >= before {
			return false
		}
		switch value := node.(type) {
		case *ast.AssignStmt:
			for index, left := range value.Lhs {
				identifier, ok := left.(*ast.Ident)
				if !ok {
					continue
				}
				if pass.TypesInfo.Defs[identifier] == object {
					fresh = len(value.Lhs) == len(value.Rhs) && index < len(value.Rhs) &&
						ps6086FreshWaitGroupExpression(pass, value.Rhs[index])
				} else if pass.TypesInfo.Uses[identifier] == object {
					reassigned = true
				}
			}
		case *ast.ValueSpec:
			for index, name := range value.Names {
				if pass.TypesInfo.Defs[name] != object {
					continue
				}
				if len(value.Values) == 0 {
					fresh = ps6086WaitGroupValue(object.Type())
				} else if len(value.Names) == len(value.Values) && index < len(value.Values) {
					fresh = ps6086FreshWaitGroupExpression(pass, value.Values[index])
				}
			}
		case *ast.UnaryExpr:
			if value.Op == token.AND {
				if identifier, ok := value.X.(*ast.Ident); ok && pass.TypesInfo.ObjectOf(identifier) == object {
					reassigned = true
				}
			}
		}
		return true
	})
	return fresh && !reassigned && ps6086BarrierUnescaped(pass, function.Body, object, before)
}

// A WaitGroup generation cannot be copied safely after first use, and handing
// its pointer to an unknown callee makes ownership impossible to prove. Permit
// only the three sync.WaitGroup protocol calls on the local binding before the
// matched join.
func ps6086BarrierUnescaped(
	pass *analysis.Pass,
	body *ast.BlockStmt,
	object *types.Var,
	before token.Pos,
) bool {
	allowed := make(map[*ast.Ident]bool)
	ast.Inspect(body, func(node ast.Node) bool {
		if node == nil || node.Pos() >= before {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		for _, method := range []string{"Add", "Done", "Wait"} {
			receiver, matched := ps6086WaitGroupMethod(pass, call, method)
			if !matched || receiver != object {
				continue
			}
			selector := call.Fun.(*ast.SelectorExpr)
			if identifier := ps6086ReceiverIdentifier(selector.X); identifier != nil {
				allowed[identifier] = true
			}
			break
		}
		return true
	})
	escaped := false
	ast.Inspect(body, func(node ast.Node) bool {
		if escaped || node == nil || node.Pos() >= before {
			return false
		}
		identifier, ok := node.(*ast.Ident)
		if ok && pass.TypesInfo.Uses[identifier] == object && !allowed[identifier] {
			escaped = true
			return false
		}
		return true
	})
	return !escaped
}

func ps6086ReceiverIdentifier(expression ast.Expr) *ast.Ident {
	switch value := expression.(type) {
	case *ast.Ident:
		return value
	case *ast.ParenExpr:
		return ps6086ReceiverIdentifier(value.X)
	case *ast.StarExpr:
		return ps6086ReceiverIdentifier(value.X)
	}
	return nil
}

func ps6086FreshWaitGroupExpression(pass *analysis.Pass, expression ast.Expr) bool {
	switch value := expression.(type) {
	case *ast.ParenExpr:
		return ps6086FreshWaitGroupExpression(pass, value.X)
	case *ast.CompositeLit:
		return ps6086WaitGroupValue(pass.TypesInfo.TypeOf(value))
	case *ast.UnaryExpr:
		return value.Op == token.AND && ps6086FreshWaitGroupExpression(pass, value.X)
	case *ast.CallExpr:
		identifier, ok := value.Fun.(*ast.Ident)
		if !ok {
			return false
		}
		builtin, builtinOK := pass.TypesInfo.ObjectOf(identifier).(*types.Builtin)
		return builtinOK && builtin.Name() == "new" && len(value.Args) == 1 &&
			ps6086WaitGroupValue(pass.TypesInfo.TypeOf(value.Args[0]))
	}
	return false
}

func ps6086WaitGroupValue(value types.Type) bool {
	named, ok := types.Unalias(value).(*types.Named)
	return ok && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == "sync" && named.Obj().Name() == "WaitGroup"
}

func ps6086CallableStable(
	pass *analysis.Pass,
	body *ast.BlockStmt,
	callable *ps6086CallableKey,
	from token.Pos,
	through token.Pos,
) bool {
	objects := []types.Object{callable.object}
	objects = append(objects, callable.receiverObjects[:callable.receiverCount]...)
	for _, object := range objects {
		variable, ok := object.(*types.Var)
		if !ok {
			continue
		}
		if variable.Parent() == pass.Pkg.Scope() {
			return false
		}
		if object == callable.object && variable.Pos() > body.Pos() &&
			!ps6086StableCallableInitializer(pass, body, variable) {
			return false
		}
		stable := true
		ast.Inspect(body, func(node ast.Node) bool {
			if !stable {
				return false
			}
			if node == nil || node.End() < from || node.Pos() > through {
				return false
			}
			switch value := node.(type) {
			case *ast.AssignStmt:
				for _, left := range value.Lhs {
					if ps6086WritesTrackedObject(
						pass, left, map[types.Object]bool{variable: true}) {
						stable = false
						return false
					}
				}
			case *ast.RangeStmt:
				for _, target := range []ast.Expr{value.Key, value.Value} {
					if target != nil && value.Tok != token.DEFINE && ps6086WritesTrackedObject(
						pass, target, map[types.Object]bool{variable: true}) {
						stable = false
						return false
					}
				}
			case *ast.UnaryExpr:
				if value.Op == token.AND {
					if ps6086WritesTrackedObject(
						pass, value.X, map[types.Object]bool{variable: true}) {
						stable = false
						return false
					}
				}
			case *ast.SelectorExpr:
				if object == callable.object && ps6086ImplicitlyAddressesObjects(
					pass, value, map[types.Object]bool{variable: true}) {
					stable = false
					return false
				}
			case *ast.SliceExpr:
				if ps6086SliceAliasesObjects(
					pass, value, map[types.Object]bool{variable: true}) {
					stable = false
					return false
				}
			}
			return true
		})
		if !stable {
			return false
		}
	}
	return true
}

func ps6086CallableBackedgeStable(
	pass *analysis.Pass,
	body *ast.BlockStmt,
	callable *ps6086CallableKey,
	matchedLoop ast.Node,
	after token.Pos,
) bool {
	callableObjects := make(map[types.Object]bool)
	receiverObjects := make(map[types.Object]bool)
	if _, variable := callable.object.(*types.Var); variable {
		callableObjects[callable.object] = true
	}
	for _, object := range callable.receiverObjects[:callable.receiverCount] {
		if _, variable := object.(*types.Var); variable {
			receiverObjects[object] = true
		}
	}
	if len(callableObjects) == 0 && len(receiverObjects) == 0 {
		return true
	}
	stable := true
	ast.Inspect(body, func(node ast.Node) bool {
		if !stable {
			return false
		}
		var enclosing ast.Node
		var enclosingBody *ast.BlockStmt
		switch value := node.(type) {
		case *ast.ForStmt:
			enclosing = node
			enclosingBody = value.Body
		case *ast.RangeStmt:
			enclosing = node
			enclosingBody = value.Body
		}
		if enclosing == nil || enclosing == matchedLoop || enclosing.Pos() >= matchedLoop.Pos() ||
			enclosing.End() <= after {
			return true
		}
		groups := []struct {
			objects         map[types.Object]bool
			implicitMethods bool
		}{{callableObjects, true}, {receiverObjects, false}}
		for _, group := range groups {
			relevant := make(map[types.Object]bool)
			for object := range group.objects {
				if object.Pos() < enclosingBody.Pos() {
					relevant[object] = true
				}
			}
			if len(relevant) > 0 && !ps6086ObjectsStable(
				pass, enclosingBody, relevant, after, enclosing.End(), group.implicitMethods) {
				stable = false
				return false
			}
		}
		return true
	})
	return stable
}

func ps6086ImplicitlyAddressedExpression(pass *analysis.Pass, selector *ast.SelectorExpr) ast.Expr {
	selection := pass.TypesInfo.Selections[selector]
	if selection == nil || selection.Kind() != types.MethodVal {
		return nil
	}
	function, ok := selection.Obj().(*types.Func)
	if !ok {
		return nil
	}
	signature, ok := function.Type().(*types.Signature)
	if !ok || signature.Recv() == nil {
		return nil
	}
	if _, pointerReceiver := signature.Recv().Type().(*types.Pointer); !pointerReceiver {
		return nil
	}
	receiverType := pass.TypesInfo.TypeOf(selector.X)
	if receiverType == nil {
		return nil
	}
	if _, alreadyPointer := receiverType.Underlying().(*types.Pointer); alreadyPointer {
		return nil
	}
	return selector.X
}

func ps6086ImplicitlyAddressesObjects(
	pass *analysis.Pass,
	selector *ast.SelectorExpr,
	objects map[types.Object]bool,
) bool {
	expression := ps6086ImplicitlyAddressedExpression(pass, selector)
	if expression == nil {
		return false
	}
	for object := range ps6086WrittenObjects(pass, expression) {
		if objects[object] {
			return true
		}
	}
	return false
}

func ps6086WrittenObjects(pass *analysis.Pass, expression ast.Expr) map[types.Object]bool {
	result := make(map[types.Object]bool)
	var collect func(ast.Expr)
	collect = func(current ast.Expr) {
		switch value := current.(type) {
		case *ast.Ident:
			if object := pass.TypesInfo.Uses[value]; object != nil {
				result[object] = true
			}
		case *ast.ParenExpr:
			collect(value.X)
		case *ast.SelectorExpr:
			collect(value.X)
		case *ast.IndexExpr:
			collect(value.X)
		case *ast.StarExpr:
			collect(value.X)
		}
	}
	collect(expression)
	return result
}

func ps6086WritesTrackedObject(
	pass *analysis.Pass,
	expression ast.Expr,
	objects map[types.Object]bool,
) bool {
	for object := range ps6086WrittenObjects(pass, expression) {
		if objects[object] {
			return true
		}
	}
	return false
}

func ps6086SliceAliasesObjects(
	pass *analysis.Pass,
	expression *ast.SliceExpr,
	objects map[types.Object]bool,
) bool {
	return ps6086ArrayStorage(pass.TypesInfo.TypeOf(expression.X)) &&
		ps6086WritesTrackedObject(pass, expression.X, objects)
}

// Local function variables are accepted only when their definition is a
// simple snapshot of another callable. In particular, a closure can mutate its
// own binding when the caller invokes it before launch, so function literals
// deliberately do not establish stable provenance here.
func ps6086StableCallableInitializer(pass *analysis.Pass, body *ast.BlockStmt, object *types.Var) bool {
	stable := false
	ast.Inspect(body, func(node ast.Node) bool {
		if stable {
			return false
		}
		switch value := node.(type) {
		case *ast.AssignStmt:
			if len(value.Lhs) != len(value.Rhs) {
				return true
			}
			for index, left := range value.Lhs {
				identifier, ok := left.(*ast.Ident)
				if ok && pass.TypesInfo.Defs[identifier] == object {
					stable = ps6086SimpleCallableAlias(pass, value.Rhs[index])
					return false
				}
			}
		case *ast.ValueSpec:
			if len(value.Names) != len(value.Values) {
				return true
			}
			for index, name := range value.Names {
				if pass.TypesInfo.Defs[name] == object {
					stable = ps6086SimpleCallableAlias(pass, value.Values[index])
					return false
				}
			}
		}
		return true
	})
	return stable
}

func ps6086SimpleCallableAlias(pass *analysis.Pass, expression ast.Expr) bool {
	for {
		parenthesized, ok := expression.(*ast.ParenExpr)
		if !ok {
			break
		}
		expression = parenthesized.X
	}
	identifier, ok := expression.(*ast.Ident)
	if !ok {
		return false
	}
	switch object := pass.TypesInfo.ObjectOf(identifier).(type) {
	case *types.Func:
		return true
	case *types.Var:
		_, callable := object.Type().Underlying().(*types.Signature)
		return callable
	default:
		return false
	}
}

func ps6086MatchingLaunches(pass *analysis.Pass, launches []*ast.GoStmt, waitObject *types.Var) []*ast.GoStmt {
	result := make([]*ast.GoStmt, 0, len(launches))
	for _, launch := range launches {
		literal, ok := ps6086InlineLiteral(launch.Call.Fun)
		if !ok {
			continue
		}
		if ps6086DirectCompletion(pass, literal.Body, waitObject) &&
			ps6086OnlyDeferredCompletion(pass, literal.Body, waitObject) {
			result = append(result, launch)
		}
	}
	return result
}

func ps6086LaunchUnconditional(pass *analysis.Pass, loop ast.Node, launch *ast.GoStmt) bool {
	var body *ast.BlockStmt
	switch value := loop.(type) {
	case *ast.ForStmt:
		body = value.Body
	case *ast.RangeStmt:
		body = value.Body
	}
	if body == nil {
		return false
	}
	for _, statement := range body.List {
		if statement.Pos() >= launch.Go {
			break
		}
		if precedingLoop, ok := statement.(*ast.ForStmt); ok &&
			!ps6086ProvablyInfiniteLoop(pass, precedingLoop) {
			continue
		}
		exits := false
		ast.Inspect(statement, func(node ast.Node) bool {
			if exits {
				return false
			}
			if _, literal := node.(*ast.FuncLit); literal {
				return false
			}
			switch value := node.(type) {
			case *ast.BranchStmt, *ast.ReturnStmt:
				exits = true
				return false
			case *ast.SelectStmt:
				if len(value.Body.List) == 0 {
					exits = true
					return false
				}
			case *ast.ForStmt:
				if ps6086ProvablyInfiniteLoop(pass, value) {
					exits = true
					return false
				}
			case *ast.CallExpr:
				if ps6086BuiltinPanic(pass, value) {
					exits = true
					return false
				}
			}
			return true
		})
		if exits {
			return false
		}
	}
	return true
}

func ps6086JoinReached(pass *analysis.Pass, loop ast.Node) bool {
	joined := true
	ast.Inspect(loop, func(node ast.Node) bool {
		if !joined {
			return false
		}
		if _, ok := node.(*ast.FuncLit); ok {
			return false
		}
		switch value := node.(type) {
		case *ast.ReturnStmt:
			joined = false
			return false
		case *ast.BranchStmt:
			if value.Tok == token.GOTO || value.Label != nil {
				joined = false
				return false
			}
		case *ast.SelectStmt:
			if len(value.Body.List) == 0 {
				joined = false
				return false
			}
		case *ast.ForStmt:
			if ps6086ProvablyInfiniteLoop(pass, value) {
				joined = false
				return false
			}
		case *ast.CallExpr:
			if ps6086BuiltinPanic(pass, value) {
				joined = false
				return false
			}
		}
		return true
	})
	return joined
}

func ps6086ProvablyInfiniteLoop(pass *analysis.Pass, loop *ast.ForStmt) bool {
	conditionTrue := loop.Cond == nil
	if loop.Cond != nil {
		value := pass.TypesInfo.Types[loop.Cond].Value
		conditionTrue = value != nil && value.Kind() == constant.Bool && constant.BoolVal(value)
	}
	if !conditionTrue {
		return false
	}
	for index, statement := range loop.Body.List {
		branch, ok := statement.(*ast.BranchStmt)
		if ok && branch.Tok == token.BREAK && branch.Label == nil &&
			ps6086FollowingStatementReached(pass, loop.Body.List[:index]) {
			return false
		}
	}
	return true
}

func ps6086BuiltinPanic(pass *analysis.Pass, call *ast.CallExpr) bool {
	identifier, ok := call.Fun.(*ast.Ident)
	if !ok {
		return false
	}
	builtin, ok := pass.TypesInfo.ObjectOf(identifier).(*types.Builtin)
	return ok && builtin.Name() == "panic"
}

func ps6086IterationCaptureSafe(
	pass *analysis.Pass,
	body *ast.BlockStmt,
	loop ast.Node,
	launch *ast.GoStmt,
) bool {
	unsafe := ps6086UnsafeLoopControlObjects(pass, loop)
	if len(unsafe) == 0 {
		return true
	}
	addressExposed := false
	ast.Inspect(body, func(node ast.Node) bool {
		if addressExposed {
			return false
		}
		switch value := node.(type) {
		case *ast.UnaryExpr:
			if value.Op == token.AND && ps6086ExpressionUsesObject(pass, value.X, unsafe) {
				addressExposed = true
				return false
			}
		case *ast.SliceExpr:
			if ps6086SliceAliasesObjects(pass, value, unsafe) {
				addressExposed = true
				return false
			}
		}
		return true
	})
	if addressExposed {
		return false
	}
	literal, ok := ps6086InlineLiteral(launch.Call.Fun)
	if !ok {
		return false
	}
	captured := false
	ast.Inspect(literal.Body, func(node ast.Node) bool {
		if captured {
			return false
		}
		identifier, ok := node.(*ast.Ident)
		if ok && unsafe[pass.TypesInfo.ObjectOf(identifier)] {
			captured = true
			return false
		}
		return true
	})
	if captured {
		return false
	}
	for _, argument := range launch.Call.Args {
		if ps6086ExpressionUsesObject(pass, argument, unsafe) &&
			!ps6086ValueSnapshotExpression(pass, argument, unsafe) {
			return false
		}
	}
	return true
}

func ps6086UnsafeLoopControlObjects(pass *analysis.Pass, loop ast.Node) map[types.Object]bool {
	candidates := make(map[types.Object]bool)
	fresh := make(map[types.Object]bool)
	perIteration := ps6086PerIterationLoopVariables(pass, loop)
	collect := func(node ast.Node) {
		if node == nil {
			return
		}
		ast.Inspect(node, func(child ast.Node) bool {
			identifier, ok := child.(*ast.Ident)
			if !ok {
				return true
			}
			if variable, ok := pass.TypesInfo.ObjectOf(identifier).(*types.Var); ok && variable.Name() != "_" {
				candidates[variable] = true
			}
			return true
		})
	}
	var loopBody *ast.BlockStmt
	switch value := loop.(type) {
	case *ast.ForStmt:
		loopBody = value.Body
		if assignment, ok := value.Init.(*ast.AssignStmt); ok {
			for _, left := range assignment.Lhs {
				collect(left)
				if perIteration && assignment.Tok == token.DEFINE {
					if identifier, ok := left.(*ast.Ident); ok {
						fresh[pass.TypesInfo.ObjectOf(identifier)] = true
					}
				}
			}
		}
		collect(value.Cond)
		collect(value.Post)
	case *ast.RangeStmt:
		loopBody = value.Body
		for _, target := range []ast.Expr{value.Key, value.Value} {
			collect(target)
			if perIteration && value.Tok == token.DEFINE {
				if identifier, ok := target.(*ast.Ident); ok {
					fresh[pass.TypesInfo.ObjectOf(identifier)] = true
				}
			}
		}
	}
	unsafe := make(map[types.Object]bool)
	for object := range candidates {
		if !fresh[object] {
			unsafe[object] = true
		}
	}
	if loopBody != nil {
		ast.Inspect(loopBody, func(node ast.Node) bool {
			var targets []ast.Expr
			switch value := node.(type) {
			case *ast.AssignStmt:
				targets = value.Lhs
			case *ast.IncDecStmt:
				targets = []ast.Expr{value.X}
			case *ast.RangeStmt:
				if value.Tok != token.DEFINE {
					targets = []ast.Expr{value.Key, value.Value}
				}
			case *ast.SelectorExpr:
				if ps6086ImplicitlyAddressesObjects(pass, value, candidates) {
					for object := range ps6086WrittenObjects(pass, value.X) {
						if candidates[object] {
							unsafe[object] = true
						}
					}
				}
			case *ast.SliceExpr:
				if ps6086SliceAliasesObjects(pass, value, candidates) {
					for object := range ps6086WrittenObjects(pass, value.X) {
						if candidates[object] {
							unsafe[object] = true
						}
					}
				}
			}
			for _, target := range targets {
				if target == nil {
					continue
				}
				for object := range ps6086WrittenObjects(pass, target) {
					if candidates[object] {
						unsafe[object] = true
					}
				}
			}
			return true
		})
	}
	return unsafe
}

func ps6086PerIterationLoopVariables(pass *analysis.Pass, loop ast.Node) bool {
	var file *ast.File
	for _, candidate := range pass.Files {
		if candidate.Pos() <= loop.Pos() && loop.End() <= candidate.End() {
			file = candidate
			break
		}
	}
	language := ""
	if file != nil && pass.TypesInfo.FileVersions != nil {
		language = pass.TypesInfo.FileVersions[file]
	}
	if language == "" && pass.Pkg != nil {
		language = pass.Pkg.GoVersion()
	}
	language = version.Lang(language)
	return language != "" && version.Compare(language, "go1.22") >= 0
}

func ps6086ValueSnapshotExpression(
	pass *analysis.Pass,
	expression ast.Expr,
	objects map[types.Object]bool,
) bool {
	safe := true
	ast.Inspect(expression, func(node ast.Node) bool {
		if !safe {
			return false
		}
		switch value := node.(type) {
		case *ast.FuncLit:
			if ps6086ExpressionUsesObject(pass, value.Body, objects) {
				safe = false
			}
			return false
		case *ast.UnaryExpr:
			if value.Op == token.AND && ps6086ExpressionUsesObject(pass, value.X, objects) {
				safe = false
				return false
			}
		case *ast.SelectorExpr:
			if ps6086ImplicitlyAddressesObjects(pass, value, objects) {
				safe = false
				return false
			}
		case *ast.SliceExpr:
			if ps6086SliceAliasesObjects(pass, value, objects) {
				safe = false
				return false
			}
		}
		return true
	})
	return safe
}

func ps6086ArrayStorage(value types.Type) bool {
	if value == nil {
		return false
	}
	if _, array := value.Underlying().(*types.Array); array {
		return true
	}
	pointer, ok := value.Underlying().(*types.Pointer)
	if !ok {
		return false
	}
	_, array := pointer.Elem().Underlying().(*types.Array)
	return array
}

func ps6086OnlyDeferredCompletion(pass *analysis.Pass, body *ast.BlockStmt, object *types.Var) bool {
	doneCalls := 0
	invalid := false
	ast.Inspect(body, func(node ast.Node) bool {
		if invalid {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		for _, method := range []string{"Add", "Done", "Wait"} {
			receiver, matched := ps6086WaitGroupMethod(pass, call, method)
			if !matched || receiver != object {
				continue
			}
			if method == "Done" {
				doneCalls++
			} else {
				invalid = true
			}
			return !invalid
		}
		return true
	})
	return !invalid && doneCalls == 1
}

func ps6086InlineLiteral(expression ast.Expr) (*ast.FuncLit, bool) {
	for {
		parenthesized, ok := expression.(*ast.ParenExpr)
		if !ok {
			break
		}
		expression = parenthesized.X
	}
	literal, ok := expression.(*ast.FuncLit)
	return literal, ok
}

func ps6086HasExactUnitAdd(pass *analysis.Pass, loop ast.Node, waitObject *types.Var, launch *ast.GoStmt) bool {
	var body *ast.BlockStmt
	switch value := loop.(type) {
	case *ast.ForStmt:
		body = value.Body
	case *ast.RangeStmt:
		body = value.Body
	}
	if body == nil {
		return false
	}
	worker, _ := ps6086InlineLiteral(launch.Call.Fun)
	protocolCalls := 0
	directUnitAdd := false
	ast.Inspect(loop, func(node ast.Node) bool {
		if literal, ok := node.(*ast.FuncLit); ok && literal == worker {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		for _, method := range []string{"Add", "Done", "Wait"} {
			receiver, matched := ps6086WaitGroupMethod(pass, call, method)
			if matched && receiver == waitObject {
				protocolCalls++
			}
		}
		return true
	})
	for _, statement := range body.List {
		if statement.Pos() >= launch.Go {
			continue
		}
		expression, ok := statement.(*ast.ExprStmt)
		if !ok {
			continue
		}
		call, ok := expression.X.(*ast.CallExpr)
		if !ok || len(call.Args) != 1 {
			continue
		}
		receiver, matched := ps6086WaitGroupMethod(pass, call, "Add")
		directUnitAdd = directUnitAdd ||
			matched && receiver == waitObject && ps6086ConstantOne(pass, call.Args[0])
	}
	return protocolCalls == 1 && directUnitAdd
}

func ps6086NoPriorBarrierMethods(
	pass *analysis.Pass,
	body *ast.BlockStmt,
	object *types.Var,
	before token.Pos,
) bool {
	found := false
	ast.Inspect(body, func(node ast.Node) bool {
		if node == nil || node.Pos() >= before {
			return false
		}
		if found {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		for _, method := range []string{"Add", "Done", "Wait"} {
			receiver, matched := ps6086WaitGroupMethod(pass, call, method)
			if matched && receiver == object {
				found = true
				return false
			}
		}
		return true
	})
	return !found
}

func ps6086ConstantOne(pass *analysis.Pass, expression ast.Expr) bool {
	value := pass.TypesInfo.Types[expression].Value
	return value != nil && value.Kind() == constant.Int &&
		constant.Compare(value, token.EQL, constant.MakeInt64(1))
}

func ps6086DirectCompletion(pass *analysis.Pass, body *ast.BlockStmt, object *types.Var) bool {
	if len(body.List) == 0 {
		return false
	}
	deferred, ok := body.List[0].(*ast.DeferStmt)
	if !ok {
		return false
	}
	receiver, matched := ps6086WaitGroupMethod(pass, deferred.Call, "Done")
	return matched && receiver == object
}

func ps6086WorkObject(
	pass *analysis.Pass,
	loop ast.Node,
	launch *ast.GoStmt,
	barrier *types.Var,
) (ps6086CallableKey, bool) {
	literal, _ := ps6086InlineLiteral(launch.Call.Fun)
	if len(literal.Body.List) != 2 || !ps6086DirectCompletion(pass, literal.Body, barrier) {
		return ps6086CallableKey{}, false
	}
	expression, ok := literal.Body.List[1].(*ast.ExprStmt)
	if !ok {
		return ps6086CallableKey{}, false
	}
	call, ok := expression.X.(*ast.CallExpr)
	if !ok {
		return ps6086CallableKey{}, false
	}
	arguments, receiverSubstitutions, relevant := ps6086WorkerCallArguments(
		pass, loop, launch, literal, call)
	if !relevant {
		return ps6086CallableKey{}, false
	}
	worker, callable := ps6086CallableObject(pass, call.Fun, receiverSubstitutions)
	worker.snapshotAt = launch.Call.Pos()
	worker.literalAt = literal.Pos()
	worker.arguments = arguments
	return worker, callable
}

func ps6086WorkerCallArguments(
	pass *analysis.Pass,
	loop ast.Node,
	launch *ast.GoStmt,
	literal *ast.FuncLit,
	call *ast.CallExpr,
) (string, map[types.Object]ast.Expr, bool) {
	signature, ok := pass.TypesInfo.TypeOf(literal).Underlying().(*types.Signature)
	if !ok || signature.Variadic() {
		return "", nil, false
	}
	parameterSources := make(map[types.Object]ps6086ExpressionFingerprint)
	receiverSubstitutions := make(map[types.Object]ast.Expr)
	var parameters []types.Object
	if literal.Type.Params != nil {
		for _, field := range literal.Type.Params.List {
			if len(field.Names) == 0 {
				parameters = append(parameters, nil)
				continue
			}
			for _, name := range field.Names {
				parameters = append(parameters, pass.TypesInfo.Defs[name])
			}
		}
	}
	if len(parameters) != len(launch.Call.Args) {
		return "", nil, false
	}
	for index, parameter := range parameters {
		fingerprint, ok := ps6086ArgumentFingerprint(pass, launch.Call.Args[index], loop, nil)
		if !ok {
			return "", nil, false
		}
		if parameter != nil {
			parameterSources[parameter] = fingerprint
			if ps6086ReceiverSubstitutionSourceSafe(pass, loop, launch.Call.Args[index]) {
				receiverSubstitutions[parameter] = launch.Call.Args[index]
			}
		}
	}
	arguments, relevant := ps6086ArgumentSignature(pass, call, loop, parameterSources)
	return arguments, receiverSubstitutions, relevant
}

func ps6086ReceiverSubstitutionSourceSafe(
	pass *analysis.Pass,
	loop ast.Node,
	expression ast.Expr,
) bool {
	if !ps6086PerIterationLoopVariables(pass, loop) {
		return false
	}
	unparenthesized := ps6086Unparen(expression)
	if _, directBinding := unparenthesized.(*ast.Ident); directBinding {
		return true
	}
	valueOnly := true
	ast.Inspect(expression, func(node ast.Node) bool {
		if !valueOnly {
			return false
		}
		current, ok := node.(ast.Expr)
		if ok && !ps6086ValueDomainType(pass.TypesInfo.TypeOf(current)) {
			valueOnly = false
			return false
		}
		return true
	})
	return valueOnly
}

func ps6086CallableObject(
	pass *analysis.Pass,
	expression ast.Expr,
	receiverSubstitutions map[types.Object]ast.Expr,
) (ps6086CallableKey, bool) {
	switch value := expression.(type) {
	case *ast.Ident:
		object := pass.TypesInfo.ObjectOf(value)
		switch candidate := object.(type) {
		case *types.Func:
			return ps6086CallableKey{
				object:        candidate,
				callableType:  ps6086TypeKey(pass.TypesInfo.TypeOf(value)),
				instantiation: ps6086InstantiationKey(pass, value),
			}, true
		case *types.Var:
			if _, ok := candidate.Type().Underlying().(*types.Signature); ok {
				return ps6086CallableKey{
					object:        candidate,
					callableType:  ps6086TypeKey(pass.TypesInfo.TypeOf(value)),
					instantiation: ps6086InstantiationKey(pass, value),
				}, true
			}
		}
	case *ast.SelectorExpr:
		function, ok := pass.TypesInfo.ObjectOf(value.Sel).(*types.Func)
		if !ok {
			return ps6086CallableKey{}, false
		}
		selection := pass.TypesInfo.Selections[value]
		if selection == nil {
			return ps6086CallableKey{
				object:        function,
				callableType:  ps6086TypeKey(pass.TypesInfo.TypeOf(value)),
				instantiation: ps6086InstantiationKey(pass, value),
			}, true
		}
		if selection.Kind() != types.MethodVal {
			return ps6086CallableKey{}, false
		}
		if ps6086AddressSensitiveReceiverSubstitution(pass, value, receiverSubstitutions) {
			return ps6086CallableKey{}, false
		}
		receiverExpression, receiverObjects, receiverCount,
			receiverSnapshots, snapshotCount, receiverKnown :=
			ps6086ReceiverIdentity(pass, value.X, receiverSubstitutions)
		if receiverKnown && receiverCount+snapshotCount > 0 {
			return ps6086CallableKey{
				object:             function,
				receiverExpression: receiverExpression,
				receiverObjects:    receiverObjects,
				receiverCount:      receiverCount,
				receiverSnapshots:  receiverSnapshots,
				snapshotCount:      snapshotCount,
				callableType:       ps6086TypeKey(pass.TypesInfo.TypeOf(value)),
				instantiation:      ps6086InstantiationKey(pass, value),
			}, true
		}
	case *ast.IndexExpr:
		callable, ok := ps6086CallableObject(pass, value.X, receiverSubstitutions)
		callable.callableType = ps6086TypeKey(pass.TypesInfo.TypeOf(value))
		callable.instantiation = ps6086InstantiationKey(pass, value)
		return callable, ok
	case *ast.IndexListExpr:
		callable, ok := ps6086CallableObject(pass, value.X, receiverSubstitutions)
		callable.callableType = ps6086TypeKey(pass.TypesInfo.TypeOf(value))
		callable.instantiation = ps6086InstantiationKey(pass, value)
		return callable, ok
	case *ast.ParenExpr:
		return ps6086CallableObject(pass, value.X, receiverSubstitutions)
	}
	return ps6086CallableKey{}, false
}

func ps6086AddressSensitiveReceiverSubstitution(
	pass *analysis.Pass,
	selector *ast.SelectorExpr,
	substitutions map[types.Object]ast.Expr,
) bool {
	if len(substitutions) == 0 {
		return false
	}
	substitutedObjects := make(map[types.Object]bool, len(substitutions))
	for object := range substitutions {
		substitutedObjects[object] = true
	}
	if addressed := ps6086ImplicitlyAddressedExpression(pass, selector); addressed != nil &&
		ps6086WritesTrackedObject(pass, addressed, substitutedObjects) &&
		!ps6086SubstitutedReceiverStorageAliases(pass, addressed, substitutedObjects) {
		return true
	}
	unsafe := false
	ast.Inspect(selector.X, func(node ast.Node) bool {
		if unsafe {
			return false
		}
		unary, ok := node.(*ast.UnaryExpr)
		if ok && unary.Op == token.AND &&
			ps6086WritesTrackedObject(pass, unary.X, substitutedObjects) &&
			!ps6086SubstitutedReceiverStorageAliases(pass, unary.X, substitutedObjects) {
			unsafe = true
			return false
		}
		return true
	})
	return unsafe
}

func ps6086SubstitutedReceiverStorageAliases(
	pass *analysis.Pass,
	expression ast.Expr,
	substitutions map[types.Object]bool,
) bool {
	var aliases func(ast.Expr) (bool, bool)
	aliases = func(current ast.Expr) (bool, bool) {
		switch value := current.(type) {
		case *ast.ParenExpr:
			return aliases(value.X)
		case *ast.Ident:
			return substitutions[pass.TypesInfo.ObjectOf(value)], false
		case *ast.StarExpr:
			uses, preserved := aliases(value.X)
			if uses {
				if valueType := pass.TypesInfo.TypeOf(value.X); valueType != nil {
					_, pointer := valueType.Underlying().(*types.Pointer)
					preserved = preserved || pointer
				}
			}
			return uses, preserved
		case *ast.SelectorExpr:
			uses, preserved := aliases(value.X)
			if uses {
				if valueType := pass.TypesInfo.TypeOf(value.X); valueType != nil {
					_, pointer := valueType.Underlying().(*types.Pointer)
					preserved = preserved || pointer
				}
			}
			return uses, preserved
		case *ast.IndexExpr:
			uses, preserved := aliases(value.X)
			if !uses {
				return false, false
			}
			valueType := pass.TypesInfo.TypeOf(value.X)
			if valueType == nil {
				return true, preserved
			}
			switch domain := valueType.Underlying().(type) {
			case *types.Slice:
				preserved = true
			case *types.Pointer:
				_, array := domain.Elem().Underlying().(*types.Array)
				preserved = preserved || array
			}
			return true, preserved
		}
		return false, false
	}
	uses, preserved := aliases(expression)
	return uses && preserved
}

func ps6086TypeKey(value types.Type) string {
	if value == nil {
		return ""
	}
	return types.TypeString(value, func(pkg *types.Package) string { return pkg.Path() })
}

func ps6086ReceiverIdentity(
	pass *analysis.Pass,
	expression ast.Expr,
	substitutions map[types.Object]ast.Expr,
) (string, [4]types.Object, int, [4]types.Object, int, bool) {
	var objects [4]types.Object
	var snapshots [4]types.Object
	count := 0
	snapshotCount := 0
	addObject := func(object types.Object, target *[4]types.Object, targetCount *int) bool {
		if _, variable := object.(*types.Var); !variable {
			return true
		}
		for index := 0; index < *targetCount; index++ {
			if target[index] == object {
				return true
			}
		}
		if *targetCount == len(target) {
			return false
		}
		target[*targetCount] = object
		(*targetCount)++
		return true
	}
	var key func(ast.Expr, bool) (string, bool)
	key = func(current ast.Expr, snapshot bool) (string, bool) {
		switch value := current.(type) {
		case *ast.ParenExpr:
			return key(value.X, snapshot)
		case *ast.Ident:
			object := pass.TypesInfo.ObjectOf(value)
			if !snapshot {
				if source, substituted := substitutions[object]; substituted {
					return key(source, true)
				}
			}
			if snapshot {
				return "object:" + ps6086ObjectKey(object), object != nil &&
					addObject(object, &snapshots, &snapshotCount)
			}
			return "object:" + ps6086ObjectKey(object), object != nil &&
				addObject(object, &objects, &count)
		case *ast.BasicLit:
			return "literal:" + value.Kind.String() + ":" + value.Value, true
		case *ast.SelectorExpr:
			base, ok := key(value.X, snapshot)
			return "selector:" + ps6086ObjectKey(pass.TypesInfo.ObjectOf(value.Sel)) + "(" + base + ")", ok
		case *ast.IndexExpr:
			base, baseOK := key(value.X, snapshot)
			index, indexOK := key(value.Index, snapshot)
			return "index(" + base + ";" + index + ")", baseOK && indexOK
		case *ast.IndexListExpr:
			base, ok := key(value.X, snapshot)
			var result strings.Builder
			result.Grow(len(base) + len(value.Indices)*16 + len("index-list()"))
			result.WriteString("index-list(")
			result.WriteString(base)
			for _, indexExpression := range value.Indices {
				index, indexOK := key(indexExpression, snapshot)
				result.WriteByte(';')
				result.WriteString(index)
				ok = ok && indexOK
			}
			result.WriteByte(')')
			return result.String(), ok
		case *ast.StarExpr:
			base, ok := key(value.X, snapshot)
			return "star(" + base + ")", ok
		case *ast.UnaryExpr:
			if value.Op == token.ARROW {
				return "", false
			}
			base, ok := key(value.X, snapshot)
			return "unary:" + value.Op.String() + "(" + base + ")", ok
		case *ast.BinaryExpr:
			left, leftOK := key(value.X, snapshot)
			right, rightOK := key(value.Y, snapshot)
			return "binary:" + value.Op.String() + "(" + left + ";" + right + ")", leftOK && rightOK
		}
		return "", false
	}
	identity, ok := key(expression, false)
	return identity, objects, count, snapshots, snapshotCount, ok
}

func ps6086InstantiationKey(pass *analysis.Pass, expression ast.Expr) string {
	var identifier *ast.Ident
	switch value := expression.(type) {
	case *ast.Ident:
		identifier = value
	case *ast.SelectorExpr:
		identifier = value.Sel
	case *ast.IndexExpr:
		return ps6086InstantiationKey(pass, value.X)
	case *ast.IndexListExpr:
		return ps6086InstantiationKey(pass, value.X)
	case *ast.ParenExpr:
		return ps6086InstantiationKey(pass, value.X)
	}
	instance, ok := pass.TypesInfo.Instances[identifier]
	if !ok {
		return ""
	}
	var result strings.Builder
	for index := range instance.TypeArgs.Len() {
		argument := ps6086TypeKey(instance.TypeArgs.At(index))
		fmt.Fprintf(&result, "%d:%s;", len(argument), argument)
	}
	return result.String()
}

func ps6086CallerParticipates(
	pass *analysis.Pass,
	statements []ast.Stmt,
	loop ast.Node,
	worker *ps6086CallableKey,
	workerKnown bool,
) bool {
	if !workerKnown {
		return false
	}
	for _, statement := range statements {
		if ps6086DirectWorkerCall(pass, statement, worker, loop) {
			return true
		}
	}
	return false
}

func ps6086DirectWorkerCall(
	pass *analysis.Pass,
	statement ast.Stmt,
	worker *ps6086CallableKey,
	domainLoop ast.Node,
) bool {
	if call := ps6086DirectStatementCall(statement); call != nil {
		return ps6086CallMatchesWorker(pass, call, worker, domainLoop)
	}
	loop := ps6086Loop(statement)
	var body *ast.BlockStmt
	switch value := loop.(type) {
	case *ast.ForStmt:
		body = value.Body
	case *ast.RangeStmt:
		body = value.Body
	}
	if body == nil {
		return false
	}
	for index, inner := range body.List {
		if ps6086FollowingStatementReached(pass, body.List[:index]) &&
			ps6086CallMatchesWorker(
				pass, ps6086DirectStatementCall(inner), worker, domainLoop) {
			return true
		}
	}
	return false
}

func ps6086CallMatchesWorker(
	pass *analysis.Pass,
	call *ast.CallExpr,
	worker *ps6086CallableKey,
	domainLoop ast.Node,
) bool {
	if call == nil {
		return false
	}
	candidate, matched := ps6086CallableObject(pass, call.Fun, nil)
	arguments, relevant := ps6086ArgumentSignature(pass, call, domainLoop, nil)
	candidate.arguments = arguments
	return matched && relevant && ps6086SameCallable(&candidate, worker) &&
		ps6086ReceiverSnapshotsStable(pass, call, domainLoop, worker)
}

func ps6086SameCallable(left, right *ps6086CallableKey) bool {
	return left.object == right.object &&
		left.receiverExpression == right.receiverExpression &&
		left.callableType == right.callableType &&
		left.instantiation == right.instantiation &&
		left.arguments == right.arguments
}

func ps6086ReceiverSnapshotsStable(
	pass *analysis.Pass,
	call *ast.CallExpr,
	loop ast.Node,
	worker *ps6086CallableKey,
) bool {
	if worker.snapshotCount == 0 {
		return true
	}
	var body *ast.BlockStmt
	switch value := loop.(type) {
	case *ast.ForStmt:
		body = value.Body
	case *ast.RangeStmt:
		body = value.Body
	}
	if body == nil {
		return false
	}
	objects := make(map[types.Object]bool, worker.snapshotCount)
	for _, object := range worker.receiverSnapshots[:worker.snapshotCount] {
		objects[object] = true
	}
	from, through := call.Pos(), worker.snapshotAt
	if from > through {
		from, through = through, from
	}
	return ps6086SnapshotSourcesUnexposed(
		pass, body, objects, worker.snapshotAt, worker.literalAt) &&
		ps6086SnapshotObjectsStable(pass, body, objects, from, through)
}

func ps6086SnapshotSourcesUnexposed(
	pass *analysis.Pass,
	body *ast.BlockStmt,
	objects map[types.Object]bool,
	through token.Pos,
	immediateWorker token.Pos,
) bool {
	safe := true
	ast.Inspect(body, func(node ast.Node) bool {
		if !safe || node == nil || node.Pos() > through {
			return false
		}
		switch value := node.(type) {
		case *ast.UnaryExpr:
			if value.Op == token.AND && ps6086WritesTrackedObject(pass, value.X, objects) {
				safe = false
				return false
			}
		case *ast.SelectorExpr:
			addressed := ps6086ImplicitlyAddressedExpression(pass, value)
			if addressed != nil && ps6086WritesTrackedObject(pass, addressed, objects) &&
				!ps6086SubstitutedReceiverStorageAliases(pass, addressed, objects) {
				safe = false
				return false
			}
		case *ast.SliceExpr:
			if ps6086SliceAliasesObjects(pass, value, objects) {
				safe = false
				return false
			}
		case *ast.FuncLit:
			if value.Pos() == immediateWorker {
				return false
			}
			if ps6086ExpressionUsesObject(pass, value.Body, objects) {
				safe = false
			}
			return false
		}
		return true
	})
	return safe
}

func ps6086SnapshotObjectsStable(
	pass *analysis.Pass,
	body *ast.BlockStmt,
	objects map[types.Object]bool,
	from token.Pos,
	through token.Pos,
) bool {
	stable := true
	ast.Inspect(body, func(node ast.Node) bool {
		if !stable || node == nil || node.End() < from || node.Pos() > through {
			return false
		}
		switch value := node.(type) {
		case *ast.AssignStmt:
			for _, left := range value.Lhs {
				if ps6086WritesTrackedObject(pass, left, objects) {
					stable = false
					return false
				}
			}
		case *ast.RangeStmt:
			for _, target := range []ast.Expr{value.Key, value.Value} {
				if target != nil && value.Tok != token.DEFINE &&
					ps6086WritesTrackedObject(pass, target, objects) {
					stable = false
					return false
				}
			}
		case *ast.IncDecStmt:
			if ps6086WritesTrackedObject(pass, value.X, objects) {
				stable = false
				return false
			}
		case *ast.UnaryExpr:
			if value.Op == token.AND && ps6086WritesTrackedObject(pass, value.X, objects) {
				stable = false
				return false
			}
		case *ast.SelectorExpr:
			addressed := ps6086ImplicitlyAddressedExpression(pass, value)
			if addressed != nil && ps6086WritesTrackedObject(pass, addressed, objects) &&
				!ps6086SubstitutedReceiverStorageAliases(pass, addressed, objects) {
				stable = false
				return false
			}
		case *ast.SliceExpr:
			if ps6086SliceAliasesObjects(pass, value, objects) {
				stable = false
				return false
			}
		}
		return true
	})
	return stable
}

func ps6086DirectStatementCall(statement ast.Stmt) *ast.CallExpr {
	var expression ast.Expr
	switch value := statement.(type) {
	case *ast.ExprStmt:
		expression = value.X
	case *ast.AssignStmt:
		if len(value.Rhs) != 1 {
			return nil
		}
		expression = value.Rhs[0]
	case *ast.DeclStmt:
		declaration, ok := value.Decl.(*ast.GenDecl)
		if !ok || len(declaration.Specs) != 1 {
			return nil
		}
		specification, ok := declaration.Specs[0].(*ast.ValueSpec)
		if !ok || len(specification.Values) != 1 {
			return nil
		}
		expression = specification.Values[0]
	default:
		return nil
	}
	call, _ := expression.(*ast.CallExpr)
	return call
}

func ps6086FollowingStatementReached(pass *analysis.Pass, statements []ast.Stmt) bool {
	for _, statement := range statements {
		if precedingLoop, ok := statement.(*ast.ForStmt); ok &&
			!ps6086ProvablyInfiniteLoop(pass, precedingLoop) {
			continue
		}
		bypass := false
		ast.Inspect(statement, func(node ast.Node) bool {
			if bypass {
				return false
			}
			if _, literal := node.(*ast.FuncLit); literal {
				return false
			}
			switch value := node.(type) {
			case *ast.BranchStmt, *ast.ReturnStmt:
				bypass = true
				return false
			case *ast.SelectStmt:
				if len(value.Body.List) == 0 {
					bypass = true
					return false
				}
			case *ast.ForStmt:
				if ps6086ProvablyInfiniteLoop(pass, value) {
					bypass = true
					return false
				}
			case *ast.CallExpr:
				if ps6086BuiltinPanic(pass, value) {
					bypass = true
					return false
				}
			}
			return true
		})
		if bypass {
			return false
		}
	}
	return true
}

func ps6086ArgumentSignature(
	pass *analysis.Pass,
	call *ast.CallExpr,
	loop ast.Node,
	substitutions map[types.Object]ps6086ExpressionFingerprint,
) (string, bool) {
	var signature strings.Builder
	fmt.Fprintf(&signature, "ellipsis:%t;", call.Ellipsis.IsValid())
	relevant := false
	for _, argument := range call.Args {
		fingerprint, ok := ps6086ArgumentFingerprint(pass, argument, loop, substitutions)
		if !ok {
			return "", false
		}
		fmt.Fprintf(&signature, "%d:%s;", len(fingerprint.key), fingerprint.key)
		relevant = relevant || fingerprint.iteration
	}
	return signature.String(), relevant
}

func ps6086ArgumentFingerprint(
	pass *analysis.Pass,
	expression ast.Expr,
	loop ast.Node,
	substitutions map[types.Object]ps6086ExpressionFingerprint,
) (ps6086ExpressionFingerprint, bool) {
	if ps6086TerminalIterationArgument(pass, loop, expression) {
		return ps6086ExpressionFingerprint{key: "iteration:0", iteration: true}, true
	}
	iterations := ps6086LoopIterationObjects(pass, loop)
	return ps6086ExpressionKey(pass, expression, iterations, substitutions)
}

func ps6086ExpressionKey(
	pass *analysis.Pass,
	expression ast.Expr,
	iterations map[types.Object]string,
	substitutions map[types.Object]ps6086ExpressionFingerprint,
) (ps6086ExpressionFingerprint, bool) {
	combine := func(kind string, expressions ...ast.Expr) (ps6086ExpressionFingerprint, bool) {
		var key strings.Builder
		key.Grow(len(kind) + len(expressions)*16 + 2)
		key.WriteString(kind)
		key.WriteByte('(')
		iteration := false
		for _, child := range expressions {
			fingerprint, ok := ps6086ExpressionKey(pass, child, iterations, substitutions)
			if !ok {
				return ps6086ExpressionFingerprint{}, false
			}
			key.WriteString(strconv.Itoa(len(fingerprint.key)))
			key.WriteByte(':')
			key.WriteString(fingerprint.key)
			key.WriteByte(';')
			iteration = iteration || fingerprint.iteration
		}
		key.WriteByte(')')
		return ps6086ExpressionFingerprint{key: key.String(), iteration: iteration}, true
	}
	switch value := expression.(type) {
	case *ast.ParenExpr:
		return ps6086ExpressionKey(pass, value.X, iterations, substitutions)
	case *ast.Ident:
		object := pass.TypesInfo.ObjectOf(value)
		if source, ok := substitutions[object]; ok {
			return source, true
		}
		if role, ok := iterations[object]; ok {
			return ps6086ExpressionFingerprint{key: role, iteration: true}, true
		}
		if object == nil {
			return ps6086ExpressionFingerprint{}, false
		}
		switch object.(type) {
		case *types.Const, *types.Nil:
			return ps6086ExpressionFingerprint{key: "constant:" + ps6086ObjectKey(object)}, true
		default:
			return ps6086ExpressionFingerprint{}, false
		}
	case *ast.BasicLit:
		return ps6086ExpressionFingerprint{key: "literal:" + value.Kind.String() + ":" + value.Value}, true
	case *ast.UnaryExpr:
		return combine("unary:"+value.Op.String(), value.X)
	case *ast.BinaryExpr:
		return combine("binary:"+value.Op.String(), value.X, value.Y)
	case *ast.SelectorExpr:
		if _, constant := pass.TypesInfo.ObjectOf(value.Sel).(*types.Const); constant &&
			pass.TypesInfo.Selections[value] == nil {
			return ps6086ExpressionFingerprint{
				key: "constant:" + ps6086ObjectKey(pass.TypesInfo.ObjectOf(value.Sel)),
			}, true
		}
		fingerprint, ok := combine("selector:"+ps6086ObjectKey(pass.TypesInfo.ObjectOf(value.Sel)), value.X)
		return fingerprint, ok
	case *ast.IndexExpr:
		return combine("index", value.X, value.Index)
	case *ast.IndexListExpr:
		expressions := make([]ast.Expr, 0, len(value.Indices)+1)
		expressions = append(expressions, value.X)
		expressions = append(expressions, value.Indices...)
		return combine("index-list", expressions...)
	case *ast.SliceExpr:
		result, ok := combine("slice:"+strconv.FormatBool(value.Slice3), value.X)
		if !ok {
			return ps6086ExpressionFingerprint{}, false
		}
		var key strings.Builder
		key.Grow(len(result.key) + 3*16)
		key.WriteString(result.key)
		for _, bound := range []ast.Expr{value.Low, value.High, value.Max} {
			if bound == nil {
				key.WriteString("missing;")
				continue
			}
			fingerprint, boundOK := ps6086ExpressionKey(pass, bound, iterations, substitutions)
			if !boundOK {
				return ps6086ExpressionFingerprint{}, false
			}
			key.WriteString(strconv.Itoa(len(fingerprint.key)))
			key.WriteByte(':')
			key.WriteString(fingerprint.key)
			key.WriteByte(';')
			result.iteration = result.iteration || fingerprint.iteration
		}
		result.key = key.String()
		return result, true
	case *ast.StarExpr:
		return combine("star", value.X)
	}
	return ps6086ExpressionFingerprint{}, false
}

func ps6086ObjectKey(object types.Object) string {
	if object == nil {
		return "<nil>"
	}
	path := "<universe>"
	if object.Pkg() != nil {
		path = object.Pkg().Path()
	}
	return fmt.Sprintf("%s:%s:%d", path, object.Name(), object.Pos())
}

func ps6086HasThresholdFallback(
	pass *analysis.Pass,
	body *ast.BlockStmt,
	statements []ast.Stmt,
	fanoutStatement ast.Stmt,
	loop ast.Node,
	worker *ps6086CallableKey,
	workerKnown bool,
	after token.Pos,
) bool {
	if !workerKnown {
		return false
	}
	if _, labeled := fanoutStatement.(*ast.LabeledStmt); labeled || !ps6086ThresholdControlFlowSafe(statements) {
		return false
	}
	domain := ps6086LoopDomainObjects(pass, loop)
	domainExpression, domainKnown := ps6086LoopDomainExpression(pass, loop)
	if !domainKnown || !ps6086StableThresholdDomain(pass, domain, domainExpression) ||
		!ps6086ObjectsStable(pass, body, domain, body.Pos(), loop.End(), true) ||
		!ps6086DomainBackedgeStable(pass, body, domain, loop, after) {
		return false
	}
	for _, statement := range statements {
		conditional, ok := statement.(*ast.IfStmt)
		if ok && ps6086ThresholdIf(pass, conditional, loop, domain, worker) {
			return true
		}
	}
	return false
}

func ps6086DomainBackedgeStable(
	pass *analysis.Pass,
	body *ast.BlockStmt,
	domain map[types.Object]bool,
	matchedLoop ast.Node,
	after token.Pos,
) bool {
	stable := true
	ast.Inspect(body, func(node ast.Node) bool {
		if !stable {
			return false
		}
		var enclosing ast.Node
		var enclosingBody *ast.BlockStmt
		switch value := node.(type) {
		case *ast.ForStmt:
			enclosing = node
			enclosingBody = value.Body
		case *ast.RangeStmt:
			enclosing = node
			enclosingBody = value.Body
		}
		if enclosing == nil || enclosing == matchedLoop || enclosing.Pos() >= matchedLoop.Pos() ||
			enclosing.End() <= after {
			return true
		}
		relevant := make(map[types.Object]bool)
		for object := range domain {
			if object.Pos() < enclosingBody.Pos() {
				relevant[object] = true
			}
		}
		if len(relevant) > 0 && !ps6086ObjectsStable(
			pass, enclosingBody, relevant, after, enclosing.End(), true) {
			stable = false
			return false
		}
		return true
	})
	return stable
}

func ps6086ThresholdControlFlowSafe(statements []ast.Stmt) bool {
	safe := true
	for _, statement := range statements {
		ast.Inspect(statement, func(node ast.Node) bool {
			if !safe {
				return false
			}
			if _, literal := node.(*ast.FuncLit); literal {
				return false
			}
			switch value := node.(type) {
			case *ast.LabeledStmt:
				safe = false
				return false
			case *ast.BranchStmt:
				if value.Tok == token.GOTO {
					safe = false
					return false
				}
			}
			return true
		})
	}
	return safe
}

func ps6086StableThresholdDomain(
	pass *analysis.Pass,
	domain map[types.Object]bool,
	expression ast.Expr,
) bool {
	valueDomain := len(domain) > 0
	ast.Inspect(expression, func(node ast.Node) bool {
		if !valueDomain {
			return false
		}
		expressionNode, ok := node.(ast.Expr)
		if ok && !ps6086ValueDomainType(pass.TypesInfo.TypeOf(expressionNode)) {
			valueDomain = false
			return false
		}
		return true
	})
	return valueDomain
}

func ps6086ValueDomainType(value types.Type) bool {
	if value == nil {
		return false
	}
	switch value.Underlying().(type) {
	case *types.Basic, *types.Array, *types.Struct:
		return true
	default:
		return false
	}
}

func ps6086ObjectsStable(
	pass *analysis.Pass,
	body *ast.BlockStmt,
	objects map[types.Object]bool,
	from token.Pos,
	through token.Pos,
	implicitMethods bool,
) bool {
	stable := true
	ast.Inspect(body, func(node ast.Node) bool {
		if !stable || node == nil || node.End() < from || node.Pos() > through {
			return false
		}
		switch value := node.(type) {
		case *ast.AssignStmt:
			for _, left := range value.Lhs {
				if ps6086WritesTrackedObject(pass, left, objects) {
					stable = false
					return false
				}
			}
		case *ast.RangeStmt:
			for _, target := range []ast.Expr{value.Key, value.Value} {
				if target != nil && value.Tok != token.DEFINE &&
					ps6086WritesTrackedObject(pass, target, objects) {
					stable = false
					return false
				}
			}
		case *ast.IncDecStmt:
			if ps6086WritesTrackedObject(pass, value.X, objects) {
				stable = false
				return false
			}
		case *ast.UnaryExpr:
			if value.Op == token.AND && ps6086WritesTrackedObject(pass, value.X, objects) {
				stable = false
				return false
			}
		case *ast.SelectorExpr:
			if implicitMethods && ps6086ImplicitlyAddressesObjects(pass, value, objects) {
				stable = false
				return false
			}
		case *ast.SliceExpr:
			if ps6086SliceAliasesObjects(pass, value, objects) {
				stable = false
				return false
			}
		}
		return true
	})
	return stable
}

func ps6086ThresholdIf(
	pass *analysis.Pass,
	conditional *ast.IfStmt,
	fanoutLoop ast.Node,
	domain map[types.Object]bool,
	worker *ps6086CallableKey,
) bool {
	matchedDomain, threshold := ps6086IntegerThreshold(pass, conditional.Cond, domain)
	if threshold {
		if ps6086TerminatingWorkBranch(pass, conditional.Body, fanoutLoop, matchedDomain, worker) {
			return true
		}
		if alternative, ok := conditional.Else.(*ast.BlockStmt); ok &&
			ps6086TerminatingWorkBranch(pass, alternative, fanoutLoop, matchedDomain, worker) {
			return true
		}
	}
	if alternative, ok := conditional.Else.(*ast.IfStmt); ok && ps6086BlockTerminates(conditional.Body) {
		return ps6086ThresholdIf(pass, alternative, fanoutLoop, domain, worker)
	}
	return false
}

func ps6086LoopDomainObjects(pass *analysis.Pass, loop ast.Node) map[types.Object]bool {
	result := make(map[types.Object]bool)
	expression, ok := ps6086LoopDomainExpression(pass, loop)
	if !ok {
		return result
	}
	ast.Inspect(expression, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		if variable, ok := pass.TypesInfo.ObjectOf(identifier).(*types.Var); ok && !variable.IsField() {
			result[variable] = true
		}
		return true
	})
	return result
}

func ps6086LoopIterationObjects(pass *analysis.Pass, loop ast.Node) map[types.Object]string {
	result := make(map[types.Object]string)
	add := func(expression ast.Expr, role string) {
		identifier, ok := expression.(*ast.Ident)
		if !ok {
			return
		}
		if variable, ok := pass.TypesInfo.ObjectOf(identifier).(*types.Var); ok && variable.Name() != "_" {
			result[variable] = role
		}
	}
	switch value := loop.(type) {
	case *ast.ForStmt:
		if assignment, ok := value.Init.(*ast.AssignStmt); ok && assignment.Tok == token.DEFINE {
			for index, left := range assignment.Lhs {
				add(left, "iteration:"+strconv.Itoa(index))
			}
		}
	case *ast.RangeStmt:
		if value.Tok == token.DEFINE {
			add(value.Key, "iteration:0")
			add(value.Value, "iteration:1")
		}
	}
	return result
}

// Recognize the common work-first split where a canonical loop launches
// [0,n-1) and the caller executes n-1. This keeps exact argument matching while
// avoiding a false advisory for an already-participating caller.
func ps6086TerminalIterationArgument(pass *analysis.Pass, loop ast.Node, argument ast.Expr) bool {
	classic, ok := loop.(*ast.ForStmt)
	if !ok {
		return false
	}
	assignment, ok := classic.Init.(*ast.AssignStmt)
	if !ok || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 ||
		assignment.Tok != token.DEFINE || !ps6086ConstantZero(pass, assignment.Rhs[0]) {
		return false
	}
	indexIdentifier, ok := assignment.Lhs[0].(*ast.Ident)
	if !ok {
		return false
	}
	index := pass.TypesInfo.ObjectOf(indexIdentifier)
	increment, ok := classic.Post.(*ast.IncDecStmt)
	if !ok || increment.Tok != token.INC {
		return false
	}
	incremented, ok := increment.X.(*ast.Ident)
	if !ok || pass.TypesInfo.ObjectOf(incremented) != index {
		return false
	}
	condition, ok := ps6086Unparen(classic.Cond).(*ast.BinaryExpr)
	if !ok || condition.Op != token.LSS {
		return false
	}
	offsetIndex, ok := ps6086Unparen(condition.X).(*ast.BinaryExpr)
	if !ok || offsetIndex.Op != token.ADD || !ps6086ConstantOne(pass, offsetIndex.Y) {
		return false
	}
	identifier, ok := ps6086Unparen(offsetIndex.X).(*ast.Ident)
	if !ok || pass.TypesInfo.ObjectOf(identifier) != index {
		return false
	}
	terminal, ok := ps6086Unparen(argument).(*ast.BinaryExpr)
	return ok && terminal.Op == token.SUB && ps6086ConstantOne(pass, terminal.Y) &&
		ps6086SameDomainExpression(pass, condition.Y, terminal.X)
}

func ps6086Unparen(expression ast.Expr) ast.Expr {
	for {
		parenthesized, ok := expression.(*ast.ParenExpr)
		if !ok {
			return expression
		}
		expression = parenthesized.X
	}
}

func ps6086IntegerThreshold(
	pass *analysis.Pass,
	condition ast.Expr,
	domain map[types.Object]bool,
) (map[types.Object]bool, bool) {
	comparison, ok := ps6086ComparisonExpression(condition)
	if !ok || !ps6086Comparison(comparison.Op) ||
		!ps6086Integer(pass.TypesInfo.TypeOf(comparison.X)) ||
		!ps6086Integer(pass.TypesInfo.TypeOf(comparison.Y)) {
		return nil, false
	}
	matched := make(map[types.Object]bool)
	ast.Inspect(comparison, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if ok {
			object := pass.TypesInfo.ObjectOf(identifier)
			if domain[object] {
				matched[object] = true
			}
		}
		return true
	})
	return matched, len(matched) > 0
}

func ps6086ComparisonExpression(expression ast.Expr) (*ast.BinaryExpr, bool) {
	for {
		parenthesized, ok := expression.(*ast.ParenExpr)
		if !ok {
			break
		}
		expression = parenthesized.X
	}
	comparison, ok := expression.(*ast.BinaryExpr)
	return comparison, ok
}

func ps6086ExpressionUsesObject(pass *analysis.Pass, expression ast.Node, objects map[types.Object]bool) bool {
	found := false
	ast.Inspect(expression, func(node ast.Node) bool {
		if found {
			return false
		}
		identifier, ok := node.(*ast.Ident)
		found = ok && objects[pass.TypesInfo.ObjectOf(identifier)]
		return !found
	})
	return found
}

func ps6086Comparison(operation token.Token) bool {
	switch operation {
	case token.EQL, token.NEQ, token.LSS, token.LEQ, token.GTR, token.GEQ:
		return true
	default:
		return false
	}
}

func ps6086Integer(value types.Type) bool {
	if value == nil {
		return false
	}
	basic, ok := value.Underlying().(*types.Basic)
	return ok && basic.Info()&types.IsInteger != 0
}

func ps6086BlockTerminates(block *ast.BlockStmt) bool {
	if len(block.List) == 0 {
		return false
	}
	_, ok := block.List[len(block.List)-1].(*ast.ReturnStmt)
	if !ok {
		return false
	}
	escapes := false
	ast.Inspect(block, func(node ast.Node) bool {
		if escapes {
			return false
		}
		if _, literal := node.(*ast.FuncLit); literal {
			return false
		}
		if _, branch := node.(*ast.BranchStmt); branch {
			escapes = true
			return false
		}
		return true
	})
	return !escapes
}

func ps6086TerminatingWorkBranch(
	pass *analysis.Pass,
	block *ast.BlockStmt,
	fanoutLoop ast.Node,
	matchedDomain map[types.Object]bool,
	worker *ps6086CallableKey,
) bool {
	if len(block.List) < 2 || !ps6086BlockTerminates(block) {
		return false
	}
	for _, statement := range block.List[:len(block.List)-1] {
		loop := ps6086Loop(statement)
		domain, hasDomain := ps6086LoopDomainExpression(pass, loop)
		if loop != nil && hasDomain && ps6086ExpressionUsesObject(pass, domain, matchedDomain) &&
			ps6086SameLoopDomain(pass, fanoutLoop, loop) &&
			ps6086DirectWorkerCall(pass, statement, worker, loop) {
			return true
		}
	}
	return false
}

func ps6086SameLoopDomain(pass *analysis.Pass, left, right ast.Node) bool {
	leftExpression, leftOK := ps6086LoopDomainExpression(pass, left)
	rightExpression, rightOK := ps6086LoopDomainExpression(pass, right)
	return leftOK && rightOK && ps6086SameDomainExpression(pass, leftExpression, rightExpression)
}

func ps6086LoopDomainExpression(pass *analysis.Pass, loop ast.Node) (ast.Expr, bool) {
	switch value := loop.(type) {
	case *ast.RangeStmt:
		if (value.Key != nil || value.Value != nil) && value.Tok != token.DEFINE {
			return nil, false
		}
		return ps6086NormalizeDomainExpression(pass, value.X), true
	case *ast.ForStmt:
		assignment, ok := value.Init.(*ast.AssignStmt)
		if !ok || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 ||
			assignment.Tok != token.DEFINE || !ps6086ConstantZero(pass, assignment.Rhs[0]) {
			return nil, false
		}
		indexIdentifier, ok := assignment.Lhs[0].(*ast.Ident)
		if !ok {
			return nil, false
		}
		index := pass.TypesInfo.ObjectOf(indexIdentifier)
		increment, ok := value.Post.(*ast.IncDecStmt)
		if !ok || increment.Tok != token.INC {
			return nil, false
		}
		incremented, ok := increment.X.(*ast.Ident)
		if !ok || pass.TypesInfo.ObjectOf(incremented) != index {
			return nil, false
		}
		comparison, ok := value.Cond.(*ast.BinaryExpr)
		if !ok {
			return nil, false
		}
		if identifier, ok := comparison.X.(*ast.Ident); ok &&
			pass.TypesInfo.ObjectOf(identifier) == index && comparison.Op == token.LSS {
			return ps6086NormalizeDomainExpression(pass, comparison.Y), true
		}
		if identifier, ok := comparison.Y.(*ast.Ident); ok &&
			pass.TypesInfo.ObjectOf(identifier) == index && comparison.Op == token.GTR {
			return ps6086NormalizeDomainExpression(pass, comparison.X), true
		}
	}
	return nil, false
}

func ps6086NormalizeDomainExpression(pass *analysis.Pass, expression ast.Expr) ast.Expr {
	for {
		parenthesized, ok := expression.(*ast.ParenExpr)
		if !ok {
			break
		}
		expression = parenthesized.X
	}
	call, ok := expression.(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return expression
	}
	identifier, ok := call.Fun.(*ast.Ident)
	if !ok {
		return expression
	}
	builtin, builtinOK := pass.TypesInfo.ObjectOf(identifier).(*types.Builtin)
	if builtinOK && builtin.Name() == "len" {
		return ps6086NormalizeDomainExpression(pass, call.Args[0])
	}
	return expression
}

func ps6086SameDomainExpression(pass *analysis.Pass, left, right ast.Expr) bool {
	left = ps6086NormalizeDomainExpression(pass, left)
	right = ps6086NormalizeDomainExpression(pass, right)
	switch leftValue := left.(type) {
	case *ast.Ident:
		rightValue, ok := right.(*ast.Ident)
		return ok && pass.TypesInfo.ObjectOf(leftValue) == pass.TypesInfo.ObjectOf(rightValue)
	case *ast.SelectorExpr:
		rightValue, ok := right.(*ast.SelectorExpr)
		return ok && pass.TypesInfo.ObjectOf(leftValue.Sel) == pass.TypesInfo.ObjectOf(rightValue.Sel) &&
			ps6086SameDomainExpression(pass, leftValue.X, rightValue.X)
	case *ast.BinaryExpr:
		rightValue, ok := right.(*ast.BinaryExpr)
		return ok && leftValue.Op == rightValue.Op &&
			ps6086SameDomainExpression(pass, leftValue.X, rightValue.X) &&
			ps6086SameDomainExpression(pass, leftValue.Y, rightValue.Y)
	case *ast.UnaryExpr:
		rightValue, ok := right.(*ast.UnaryExpr)
		return ok && leftValue.Op == rightValue.Op &&
			ps6086SameDomainExpression(pass, leftValue.X, rightValue.X)
	case *ast.BasicLit:
		rightValue, ok := right.(*ast.BasicLit)
		return ok && leftValue.Kind == rightValue.Kind && leftValue.Value == rightValue.Value
	}
	return false
}

func ps6086ConstantZero(pass *analysis.Pass, expression ast.Expr) bool {
	value := pass.TypesInfo.Types[expression].Value
	return value != nil && value.Kind() == constant.Int && constant.Sign(value) == 0
}
