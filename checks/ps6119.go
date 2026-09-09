package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"slices"
	"strings"

	"github.com/jxsl13/perfscan/internal/astutil"
	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/lint"
)

// PS6119 implements owner issue #820. It deliberately proves only repetition
// of one source handle; cache residency remains a measured runtime fact.
var PS6119 = register(&lint.Check{
	ID: "PS6119", Category: "verify", Slug: "single-resident-weight-benchmark",
	Level: lint.LevelAggressive, AutoFix: false, NeedsConfig: true,
	Vocab: []string{"residentWeightBenchmarkContracts"},
	Doc: lint.Documentation{
		Title: "one invariant resident weight can make accelerator evidence unrepresentatively cache-hot",
		Text: `A benchmark loop that repeatedly launches an accelerator operation with
one immutable weight handle measures a single-handle, potentially cache-hot
cell. Source syntax proves neither physical residency nor a cache hit, but this
cell alone can hide streaming-weight leverage that appears in a parent graph.

PS6119 is config-gated by residentWeightBenchmarkContracts. Each contract names
an exact typed accelerator callable and its weight argument or receiver. The
check inspects real BenchmarkX(*testing.B) b.N and b.Loop loops, including
b.Run bodies and bounded straight-through local functions or closures. It
reports only a handle selected outside the timed loop and rejects dynamic
indexes, rebinding, mutation, address-taking, capture by another closure, or
opaque uses. A constant weights[0] is still one invariant selection; a loop-
varying weights[i%len(weights)] is rotating evidence and stays silent.

Classify the finding as single-invariant/cache-hot evidence, declare total
working-set bytes against the relevant cache capacity, and add a rotating
multi-weight campaign matching the parent graph. Retain GPU, encoder, and wall
decomposition. Require both a whole-workload gate and an equal host/device
boundary gate before rejecting or promoting a route. An exact
//perfscan:ignore PS6119 <reason> may document why one weight matches production.

There is NO automatic fix. The diagnostic does not claim actual cache
residency, streaming production behavior, a kernel win, or a routing result.`,
		Before: `weight := uploadWeight()
for range b.N { recorder.Launch(input, weight, output) }`,
		After: `// Keep the single-weight cell as cache-hot evidence, then add:
// rotating parent-shaped weights; GPU/encoder/wall timing; whole-workload and
// equal host/device-boundary gates; working-set bytes versus cache bytes.`,
		MeasuredWin: `On Apple M2 Pro, the Q4_1 single-weight wall cell showed
about 1.2x while 16 resident weights produced 2.745x-8.487x GPU gains and a
six-layer decoder improved 2.535x. Independent IQ4_NL and IQ4_XS campaigns
reproduced the direction, while equal host/device boundaries favored ARM64;
therefore neither the cache-hot cell nor the rotating cell alone selects a route.`,
	},
	Analyzer: &analysis.Analyzer{Name: "PS6119", Doc: "find benchmark loops repeating one configured invariant accelerator weight", Run: runPS6119},
})

type ps6119Contract struct {
	config.ResidentWeightBenchmarkContract
}
type ps6119Candidate struct {
	call   *ast.CallExpr
	weight ast.Expr
}

func runPS6119(pass *analysis.Pass) (any, error) {
	return runPS6119WithContracts(pass, config.Current().ResidentWeightBenchmarkContracts)
}

func runPS6119WithContracts(pass *analysis.Pass, configured []config.ResidentWeightBenchmarkContract) (any, error) {
	contracts := ps6119Contracts(configured)
	if len(contracts) == 0 {
		return nil, nil
	}
	wrappers := ps6119Wrappers(pass, contracts)
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			fn, ok := declaration.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !ps6090RunnableBenchmark(pass, file, fn) {
				continue
			}
			closures := ps6119Closures(pass, file, fn.Body, contracts)
			ps6119WalkBenchmarkBodies(pass, fn.Body, func(scope *ast.BlockStmt, node ast.Node) {
				loop, body := ps6119BenchmarkLoop(pass, node)
				if loop == nil {
					return
				}
				if ps2144PositionIn(loop.Pos(), ps2144Unreachable(pass, scope)) {
					return
				}
				candidates := ps6119LoopCandidates(pass, file, body, contracts, closures, wrappers)
				if len(candidates) == 0 {
					return
				}
				root, allowed, ok := ps6119CommonInvariant(pass, loop, candidates)
				if !ok || ps6119UnsafeWindow(pass, fn.Body, root, loop.End(), allowed) {
					return
				}
				pass.Reportf(candidates[0].call.Pos(), "accelerator benchmark repeats one loop-invariant weight handle: classify this only as single-invariant/cache-hot evidence (not proof of cache residency); record working-set versus cache bytes, rotate parent-graph weights, decompose GPU/encoder/wall time, and require whole-workload plus equal host/device-boundary gates before route promotion or rejection (PS6119 advisory, no automatic fix)")
			})
		}
	}
	return nil, nil
}

func ps6119Contracts(in []config.ResidentWeightBenchmarkContract) []ps6119Contract {
	counts := map[string]int{}
	for _, c := range in {
		if c.Valid() {
			counts[c.AcceleratorCallable]++
		}
	}
	var out []ps6119Contract
	for _, c := range in {
		if c.Valid() && counts[c.AcceleratorCallable] == 1 {
			out = append(out, ps6119Contract{c})
		}
	}
	slices.SortFunc(out, func(a, b ps6119Contract) int { return strings.Compare(a.AcceleratorCallable, b.AcceleratorCallable) })
	return out
}

func ps6119BenchmarkLoop(pass *analysis.Pass, node ast.Node) (ast.Node, *ast.BlockStmt) {
	switch loop := node.(type) {
	case *ast.RangeStmt:
		if ps6090TestingBN(pass, loop.X) {
			return loop, loop.Body
		}
	case *ast.ForStmt:
		if ps6090BenchmarkFor(pass, loop) {
			return loop, loop.Body
		}
	}
	return nil, nil
}

func ps6119WalkBenchmarkBodies(pass *analysis.Pass, body *ast.BlockStmt, visit func(*ast.BlockStmt, ast.Node)) {
	var walk func(*ast.BlockStmt)
	walk = func(scope *ast.BlockStmt) {
		graph := ps6090ControlFlow(pass, scope)
		reachable := ps6090ReachableNodes(graph)
		astutil.WithStack(scope, func(node ast.Node, stack []ast.Node) bool {
			if literal, nested := node.(*ast.FuncLit); nested && literal.Body != scope {
				return false
			}
			visit(scope, node)
			call, ok := node.(*ast.CallExpr)
			if !ok || !reachable[call] || !ps6090StaticPathLive(pass, call, stack) || ps6119Deferred(stack) || !ps6119TestingRun(pass, call) {
				return true
			}
			for _, argument := range call.Args {
				if literal, ok := ps2110Unparen(argument).(*ast.FuncLit); ok {
					walk(literal.Body)
				}
			}
			return true
		})
	}
	walk(body)
}

func ps6119Deferred(stack []ast.Node) bool {
	for _, node := range stack {
		if _, ok := node.(*ast.DeferStmt); ok {
			return true
		}
		if _, ok := node.(*ast.GoStmt); ok {
			return true
		}
	}
	return false
}

func ps6119LiveCalls(pass *analysis.Pass, body *ast.BlockStmt, visit func(*ast.CallExpr)) {
	graph := ps6090ControlFlow(pass, body)
	reachable := ps6090ReachableNodes(graph)
	astutil.WithStack(body, func(node ast.Node, stack []ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if ok && reachable[call] && ps6090StaticPathLive(pass, call, stack) && !ps6119Deferred(stack) {
			visit(call)
		}
		return true
	})
}

func ps6119TestingRun(pass *analysis.Pass, call *ast.CallExpr) bool {
	function, signature, ok := typedCallee(pass, call.Fun)
	return ok && function.Pkg() != nil && function.Pkg().Path() == "testing" && function.Name() == "Run" && typedReceiverNamed(signature, "testing", "B")
}

func ps6119Weight(pass *analysis.Pass, call *ast.CallExpr, contract ps6119Contract) ast.Expr {
	if contract.WeightReceiver {
		sel, ok := ps2110Unparen(call.Fun).(*ast.SelectorExpr)
		if ok {
			return sel.X
		}
		return nil
	}
	i := contract.WeightArgument - 1
	if i < 0 || i >= len(call.Args) {
		return nil
	}
	return call.Args[i]
}

func ps6119Direct(pass *analysis.Pass, file *ast.File, call *ast.CallExpr, contracts []ps6119Contract) ast.Expr {
	for _, c := range contracts {
		if ps6115DirectCall(pass, file, call, c.AcceleratorCallable) {
			return ps6119Weight(pass, call, c)
		}
	}
	return nil
}

type ps6119Closure struct {
	weight ast.Expr
}

type ps6119Wrapper struct{ argument int }

func ps6119Closures(pass *analysis.Pass, file *ast.File, body *ast.BlockStmt, contracts []ps6119Contract) map[types.Object]ps6119Closure {
	out := map[types.Object]ps6119Closure{}
	writes := map[types.Object]int{}
	ast.Inspect(body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.AssignStmt:
			for _, lhs := range node.Lhs {
				if id, ok := ps2110Unparen(lhs).(*ast.Ident); ok {
					writes[pass.TypesInfo.ObjectOf(id)]++
				}
			}
		case *ast.ValueSpec:
			for _, id := range node.Names {
				writes[pass.TypesInfo.ObjectOf(id)]++
			}
		}
		return true
	})
	ast.Inspect(body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
			return true
		}
		id, ok := assign.Lhs[0].(*ast.Ident)
		if !ok {
			return true
		}
		lit, ok := ps2110Unparen(assign.Rhs[0]).(*ast.FuncLit)
		if !ok {
			return true
		}
		var found *ast.CallExpr
		var weight ast.Expr
		matches := 0
		ps6119LiveCalls(pass, lit.Body, func(c *ast.CallExpr) {
			if w := ps6119Direct(pass, file, c, contracts); w != nil {
				matches++
				found, weight = c, w
			}
		})
		if found != nil && matches == 1 {
			if obj := pass.TypesInfo.ObjectOf(id); obj != nil {
				out[obj] = ps6119Closure{weight: weight}
			}
		}
		return false
	})
	parents := ps6090Parents(body)
	for object := range out {
		validUses := true
		ast.Inspect(body, func(node ast.Node) bool {
			id, ok := node.(*ast.Ident)
			if !ok || pass.TypesInfo.ObjectOf(id) != object || id.Pos() == object.Pos() {
				return true
			}
			call, direct := parents[id].(*ast.CallExpr)
			if !direct || ps2110Unparen(call.Fun) != id {
				validUses = false
			}
			return true
		})
		if writes[object] != 1 || !validUses {
			delete(out, object)
		}
	}
	return out
}

func ps6119LoopCandidates(pass *analysis.Pass, file *ast.File, body *ast.BlockStmt, contracts []ps6119Contract, closures map[types.Object]ps6119Closure, wrappers map[*types.Func]ps6119Wrapper) []ps6119Candidate {
	var out []ps6119Candidate
	graph := ps6090ControlFlow(pass, body)
	reachable := ps6090ReachableNodes(graph)
	astutil.WithStack(body, func(n ast.Node, stack []ast.Node) bool {
		if _, nested := n.(*ast.FuncLit); nested {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok || !reachable[call] || !ps6090StaticPathLive(pass, call, stack) || ps6119Deferred(stack) {
			return true
		}
		if weight := ps6119Direct(pass, file, call, contracts); weight != nil {
			out = append(out, ps6119Candidate{call, weight})
			return false
		}
		if id, ok := ps2110Unparen(call.Fun).(*ast.Ident); ok {
			if closure, exists := closures[pass.TypesInfo.ObjectOf(id)]; exists {
				out = append(out, ps6119Candidate{call, closure.weight})
				return false
			}
		}
		if function, signature, ok := typedCallee(pass, call.Fun); ok && function != nil && signature != nil {
			if wrapper, exists := wrappers[function.Origin()]; exists && wrapper.argument < len(call.Args) {
				out = append(out, ps6119Candidate{call, call.Args[wrapper.argument]})
				return false
			}
		}
		return true
	})
	return out
}

func ps6119Wrappers(pass *analysis.Pass, contracts []ps6119Contract) map[*types.Func]ps6119Wrapper {
	out := map[*types.Func]ps6119Wrapper{}
	for _, source := range pass.Files {
		for _, declaration := range source.Decls {
			fn, ok := declaration.(*ast.FuncDecl)
			if !ok || fn.Body == nil || fn.Recv != nil {
				continue
			}
			object, ok := pass.TypesInfo.Defs[fn.Name].(*types.Func)
			if !ok {
				continue
			}
			signature, ok := object.Type().(*types.Signature)
			if !ok || signature.TypeParams().Len() != 0 || signature.Variadic() {
				continue
			}
			matches, argument := 0, -1
			uses := make(map[types.Object]int)
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				if id, ok := node.(*ast.Ident); ok {
					uses[pass.TypesInfo.ObjectOf(id)]++
				}
				return true
			})
			graph := ps6090ControlFlow(pass, fn.Body)
			reachable := ps6090ReachableNodes(graph)
			astutil.WithStack(fn.Body, func(node ast.Node, stack []ast.Node) bool {
				if _, nested := node.(*ast.FuncLit); nested {
					return false
				}
				call, ok := node.(*ast.CallExpr)
				if !ok || !reachable[call] || !ps6090StaticPathLive(pass, call, stack) || ps6119Deferred(stack) {
					return true
				}
				weight := ps6119Direct(pass, source, call, contracts)
				id, ok := ps2110Unparen(weight).(*ast.Ident)
				if !ok {
					return true
				}
				for i := 0; i < signature.Params().Len(); i++ {
					if pass.TypesInfo.ObjectOf(id) == signature.Params().At(i) {
						argument, matches = i, matches+1
					}
				}
				return true
			})
			if matches == 1 && argument >= 0 && uses[signature.Params().At(argument)] == 1 {
				out[object.Origin()] = ps6119Wrapper{argument: argument}
			}
		}
	}
	return out
}

func ps6119Root(pass *analysis.Pass, expr ast.Expr) (types.Object, string, map[*ast.Ident]bool, bool) {
	allowed := map[*ast.Ident]bool{}
	switch value := ps2110Unparen(expr).(type) {
	case *ast.Ident:
		obj := pass.TypesInfo.ObjectOf(value)
		if obj == nil {
			return nil, "", nil, false
		}
		allowed[value] = true
		return obj, "root", allowed, true
	case *ast.IndexExpr:
		id, ok := ps2110Unparen(value.X).(*ast.Ident)
		if !ok {
			return nil, "", nil, false
		}
		v := pass.TypesInfo.Types[value.Index].Value
		if v == nil || v.Kind() != constant.Int {
			return nil, "", nil, false
		}
		obj := pass.TypesInfo.ObjectOf(id)
		if obj == nil {
			return nil, "", nil, false
		}
		allowed[id] = true
		return obj, "index:" + v.ExactString(), allowed, true
	}
	return nil, "", nil, false
}

func ps6119CommonInvariant(pass *analysis.Pass, loop ast.Node, candidates []ps6119Candidate) (types.Object, map[*ast.Ident]bool, bool) {
	breaks := false
	ast.Inspect(ps6119LoopBody(loop), func(node ast.Node) bool {
		if branch, ok := node.(*ast.BranchStmt); ok && branch.Tok == token.BREAK {
			breaks = true
		}
		return true
	})
	if breaks {
		return nil, nil, false
	}
	var root types.Object
	var selection string
	allowed := map[*ast.Ident]bool{}
	for _, candidate := range candidates {
		object, selected, ids, ok := ps6119Root(pass, candidate.weight)
		if !ok || object.Pos() >= loop.Pos() || root != nil && (root != object || selection != selected) {
			return nil, nil, false
		}
		root = object
		selection = selected
		for id := range ids {
			allowed[id] = true
		}
	}
	variable, local := root.(*types.Var)
	return root, allowed, local && variable.Parent() != pass.Pkg.Scope()
}

func ps6119LoopBody(loop ast.Node) *ast.BlockStmt {
	switch value := loop.(type) {
	case *ast.ForStmt:
		return value.Body
	case *ast.RangeStmt:
		return value.Body
	}
	return nil
}

func ps6119UnsafeWindow(pass *analysis.Pass, body *ast.BlockStmt, root types.Object, end token.Pos, allowed map[*ast.Ident]bool) bool {
	unsafe := false
	astutil.WithStack(body, func(n ast.Node, stack []ast.Node) bool {
		if unsafe {
			return false
		}
		id, ok := n.(*ast.Ident)
		if !ok || id.Pos() <= root.Pos() || id.Pos() >= end || pass.TypesInfo.ObjectOf(id) != root {
			return true
		}
		if ps6119DeferredReceiver(id, stack) {
			return true
		}
		if !allowed[id] {
			unsafe = true
		}
		return true
	})
	return unsafe
}

func ps6119DeferredReceiver(id *ast.Ident, stack []ast.Node) bool {
	var deferred *ast.DeferStmt
	for _, ancestor := range stack {
		if _, nested := ancestor.(*ast.FuncLit); nested {
			return false
		}
		if value, ok := ancestor.(*ast.DeferStmt); ok {
			deferred = value
		}
	}
	if deferred == nil {
		return false
	}
	selector, ok := ps2110Unparen(deferred.Call.Fun).(*ast.SelectorExpr)
	return ok && ps2110Unparen(selector.X) == id
}
