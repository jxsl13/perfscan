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
	"golang.org/x/tools/go/cfg"

	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS6079 implements owner issue #796. It traces simple semantic sign guards
// from optimized/scalar branches back through plain wrappers to benchmark
// fixtures and requires either constrained inputs or observed-route evidence.
var PS6079 = register(&lint.Check{
	ID:       "PS6079",
	Category: "verify",
	Slug:     "benchmark-fixture-misses-guarded-fast-path",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "a benchmark fixture does not prove that its guarded optimized route is entered",
		Text: `A benchmark can call the intended operation and still measure only
fallback code when its generated inputs do not satisfy the optimized route's
semantic domain. Naming the benchmark after the operation is not route
coverage.

This check implements owner issue #796. It finds a same-package branch that
selects a fast/optimized/SIMD/NEON/AVX/SSE/native/kernel target against a
scalar/fallback/generic/reference/slow target and extracts simple zero-sign
requirements from the optimized arm. Supported guards include direct
comparisons, conjunctions for a true fast arm, disjunctions for an inverted
early-fallback arm, and semantic predicates named nonnegative, positive,
nonpositive, or negative. Unrelated capability terms may accompany the
semantic terms without being mistaken for fixture evidence.
Indexed comparisons stay unmodeled because one element cannot prove a
whole-slice sign domain.

Requirements propagate backward through direct same-package calls when a
wrapper passes plain parameters unchanged. Exact func BenchmarkX(*testing.B)
functions are then checked at routed call sites inside b.Loop or b.N repetition
regions; setup and warmup calls are not measured workloads. One-time for-loop
initializers and range expressions are excluded even when their enclosing loop
contains b.Loop or b.N. The fixture flow
recognizes
numeric constants, zero-filled make/literals, unary sign changes, math.Abs only
for proven non-NaN inputs, clearly named sign-constraining constructors,
unrestricted random/normal/
uniform constructors, scalar/array assignments, random/fill mutators, indexed
writes, and conservative copy/alias barriers. Unknown or unrestricted
provenance does not prove a universal sign requirement. make and copy must
resolve to the actual predeclared builtins; same-named user functions are not
trusted. clear establishes a zero fixture only for a proven whole destination;
bounded or indexed slices stay unknown. A constraining provenance event must
dominate the routed workload; lexical position alone is insufficient.
Supported unexported package initializers establish synthetic pre-benchmark
fixture provenance only when package-wide analysis proves that the object and
its aliases are never mutated or escaped; this accounts for benchmark
calibration re-entry and other tests or benchmarks sharing package state.
Conditional/loop-local fixture
mutations and keyed literals with
possible implicit zero holes also do not establish an unconditional strict-sign
domain. Slice-to-slice assignment, partial copy, discarded append calls,
compound updates, and calls that may mutate reference inputs likewise stay
unknown, including mutations through a derived slice or an address-of
expression. Mutating map builtins such as delete and clear likewise invalidate
predicate provenance. A recognized fill state applies only to a receiver or
first reference argument that is an unambiguous whole-fixture destination.
Indexed and bounded derived slices, aliases originating from them, and
aggregates that capture such slices in literals or later member assignments
stay unknown. Aggregate member fills are trusted only when the containing value
has one reference slot; otherwise field-insensitive aliases make sibling state
ambiguous. Parallel reference rebindings, conditional rebindings, and range-
clause assignments remain conservative, and reference sources are followed
through derived selector, index, slice, assertion, and dereference expressions.
A mutator launched with go is
asynchronous and therefore cannot constrain a fixture before the measured
workload; this also applies to clear. Capturing or passing a fixture to a
goroutine, or sending it through a channel, persistently escapes the fixture so
later fills cannot restore provenance without a proven ownership transfer.
That persistent escape follows aliases created later and calls are recursively
audited for helpers that launch goroutines or otherwise retain reference inputs.
Reference values received from channels, including through range statements,
are likewise persistently escaped, including values returned through recursively
called same-package helpers. Range-over-function iterators and function-carrying
arguments or receivers invalidate captured fixtures; because an iterator or
callback can launch work, those fixtures remain persistently escaped. Calls or
direct benchmark assignments that may retain references in package or aggregate
storage likewise keep those fixtures persistently escaped. Reference results
returned from same-package helpers stay linked to package storage exposed by
direct, nested, aggregate, local-alias, or named-result return flows, so later
package mutations invalidate the benchmark alias.
Other reference arguments stay unknown.
Mutations after a routed call inside an enclosing loop are treated as loop-
carried input state for later iterations. Constructor sign inference uses the
typed callee, never argument or receiver variable names, and rejects sign states
that the constructor's static result type cannot represent. Creating a slice alias
invalidates both names unless stronger ownership evidence exists. A routed
same-package call is also checked for direct or downstream mutation of a guarded
reference parameter before its initializer is trusted across benchmark-loop iterations. Reference-valued
key/value aliases introduced by range clauses are included in that mutation
proof. Reference parameters captured inside aggregate literals conservatively
count as mutable. Conditional and loop-local rebindings retain every possible
reference alias. Address-taken scalar and array fixtures remain linked to their
pointer aliases, so later indirect writes invalidate the rooted fixture. Conversions
through unsafe.Pointer and uintptr retain the originating fixture component across
calls, sends, retention, and later indirect writes. Unsafe-derived storage remains
sticky-unknown, so a later fill cannot turn pointer bits into fixture evidence. Conditional
pointer rebindings retain every possible target. Representation-preserving
conversions and type assertions do not hide reference aliases. Slice and other
reference aliases remain linked across later constraining fills until an
unconditional rebind proves that the names no longer share storage. Reference-
bearing results from a multi-result call conservatively retain aliases to the
call's reference inputs. Calling an unresolved function value also invalidates
established fixture provenance because a closure can mutate captured storage
without receiving it as an explicit argument. A resolved same-package helper
invalidates tracked package-level fixtures because it can mutate global storage
without receiving it as an explicit argument.

A benchmark may instead prove the selected route with an unconditional reset
of a clearly named fast-path/optimized-route/branch counter before the routed
workload and a direct missing-route failure afterward. A
//perfscan:benchmark-fast-path-validated annotation records equivalent external
profile evidence. Merely assigning a force-enable or capability boolean is not
route evidence: the real fixture must model a valid workload, and the route
must be observed after warmup. No other function capable of contributing to
that route counter may run between the reset and final assertion; calls with
function-valued arguments, including values nested under conversions, are
treated as potential callback invokers. Same-package helpers are recursively
checked, and an unresolved indirect call such as a package-level function
variable also invalidates the proof. Direct writes to the asserted counter are
not route evidence, including writes through pointer aliases captured directly
or inside aggregate values before the reset. Workload wrappers and the guarded
contributor are audited: the asserted counter must be updated inside the
guard's proven optimized arm, or by same-package code reachable exclusively
from that arm, so unconditional and sibling writes cannot manufacture route
evidence. A lexical tail that any early-fallback goto, including one nested in
control flow before a return, can enter is not considered optimized-only counter
evidence. Custom counter observations must be a single
read-only field return or a trusted sync/atomic load, and their reset must
use a pointer receiver and write zero to that exact storage; name-only Load or
Store methods are not trusted. Every intervening Load, Value, or Count call on
the counter must pass the same read-only audit. Returning the asserted counter
by reference from a helper also invalidates the proof. The reset must dominate the
measured workload, and reset and assertion receivers must resolve to the same
typed counter object. Route
names must contain every non-generic operation component shared by the
optimized and fallback targets; a common family prefix alone is insufficient.
Every control-flow path through a direct counter branch must mark
the current benchmark receiver failed on the missing route (for example
ssmFastPathHits.Load() == 0); reversed-polarity failures, conditional failures,
else-only failures, fallback counters, and counters named for another operation
are not fast-route proof. A goroutine launched before the counter reset can
still manufacture a later hit, so any outstanding asynchronous counter writer
invalidates the proof, including a writer launched inside a recursively called
same-package helper. Counter references returned before reset, sends of counter
references to existing workers, range-clause counter aliases, and writers launched by package initialization
are included in that audit. Reassigning the benchmark receiver before the failure
branch also invalidates the proof; taking its address or assigning it from a
nested closure is treated as a possible reassignment. Route assertions nested
under a conditional or loop, or bypassable by an intervening return or goto,
are not unconditional evidence. Skip, Skipf, and SkipNow terminate a benchmark
without failing it, so a later lexical failure call does not prove the missing
route is rejected; direct method calls and method-expression invocations are
both recognized. Unresolved calls and resolved same-package helpers in the
failure branch are rejected because they can hide a terminal skip. Opaque
cross-package calls are rejected for the same reason.
Generic assert/require/verify APIs are declined because their polarity is not
implied by their names.

Indirect calls, cross-package bodies, receiver/field reshaping, opaque semantic
predicates, OR alternatives on a positive fast arm, AND alternatives on an
inverted fast arm, nested benchmark closures, and guards without distinct
optimized and fallback targets stay silent. These exclusions avoid claiming a
constraint that source does not prove.

There is NO automatic fix. Taking absolute values, negating a tensor, changing
a shape, or forcing a capability can change the benchmarked workload. Build a
representative semantic fixture, assert a route counter after warmup, and keep
the fallback visible as a separately named control/profile cell.`,
		Before: `func BenchmarkSSMF64(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	for b.Loop() { SSMF64(delta, decay) }
}`,
		After: `func BenchmarkSSMF64FastPath(b *testing.B) {
	delta := positiveMambaDelta()
	decay := negativeMambaDecay()
	ssmFastPathHits.Store(0)
	for b.Loop() { SSMF64(delta, decay) }
	if ssmFastPathHits.Load() == 0 { b.Fatal("fast path not entered") }
}`,
		MeasuredWin: `In owner issue #796, BenchmarkSSMF64_512x1024x16_cpu used
unrestricted random F64 delta and A tensors even though the arm64 route required
delta >= 0 and A <= 0. The nominal candidate remained near 19 ms/op and profiles
attributed 82.32% cumulative CPU to ssmScanDRangeScalar and 53.04% flat CPU to
math.archExp. Changing only the shared fixture to delta > 0 and A < 0 produced
16.90-17.91 ms/op controls and 2.878-2.898 ms/op fused NEON candidates on Apple
M2 Pro, exposing an approximately 83% end-to-end latency reduction.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6079",
		Doc:  "benchmark fixtures do not prove entry into reachable guarded optimized branches",
		Run:  runPS6079,
	},
})

type ps6079Relation uint8

const (
	ps6079AtLeastZero ps6079Relation = iota + 1
	ps6079AboveZero
	ps6079AtMostZero
	ps6079BelowZero
	ps6079ExactlyZero
)

type ps6079Requirement struct {
	parameter int
	relation  ps6079Relation
	input     string
}

type ps6079Gate struct {
	function       *ast.FuncDecl
	object         *types.Func
	condition      ast.Expr
	requires       []ps6079Requirement
	optimized      string
	fallback       string
	optimizedNodes []ast.Node
}

type ps6079ReachableGate struct {
	gate         *ps6079Gate
	contributors map[*types.Func]bool
}

type ps6079Route struct {
	target            *types.Func
	requires          []ps6079Requirement
	gate              *ps6079Gate
	path              []string
	contributors      map[*types.Func]bool
	uniqueContributor bool
	reachableGates    []ps6079ReachableGate
}

type ps6079CallSite struct {
	caller       *ast.FuncDecl
	callerObject *types.Func
	call         *ast.CallExpr
}

type ps6079FixtureState uint8

const (
	ps6079UnknownFixture ps6079FixtureState = iota
	ps6079UnrestrictedFixture
	ps6079ZeroFixture
	ps6079NonnegativeFixture
	ps6079PositiveFixture
	ps6079NonpositiveFixture
	ps6079NegativeFixture
)

type ps6079FixtureEvent struct {
	position token.Pos
	state    ps6079FixtureState
	source   string
	loops    []ps6079LoopRange
}

type ps6079LoopRange struct {
	start token.Pos
	end   token.Pos
}

type ps6079ParameterKey struct {
	function  *types.Func
	parameter int
}

type ps6079CounterReference struct {
	root   types.Object
	fields []types.Object
}

type ps6079CounterObservation struct {
	reference ps6079CounterReference
	storage   []types.Object
	atomic    bool
}

func runPS6079(pass *analysis.Pass) (any, error) {
	functions, sites, declarations := ps6079PackageFunctions(pass)
	if !ps6079PackageHasBenchmark(pass, functions) {
		// PS6079 only reports routed calls made by a benchmark in this
		// package. Avoid the substantially more expensive gate, alias, and
		// route analysis when no such reporting site can exist.
		return nil, nil
	}
	var gates []ps6079Gate
	for _, function := range functions {
		gates = append(gates, ps6079FunctionGates(pass, function)...)
	}
	reachableGates := make([]ps6079ReachableGate, len(gates))
	for index := range gates {
		reachableGates[index] = ps6079ReachableGate{
			gate: &gates[index], contributors: ps6079RouteContributors(gates[index].object, sites),
		}
	}
	routes := make([]ps6079Route, 0, len(reachableGates))
	for _, reachable := range reachableGates {
		gate := reachable.gate
		routes = append(routes, ps6079Route{
			target: gate.object, requires: gate.requires, gate: gate,
			path: []string{gate.function.Name.Name}, contributors: reachable.contributors,
			uniqueContributor: ps6079RouteContributorCallCount(pass, gate.function, reachable.contributors) == 0,
			reachableGates:    reachableGates,
		})
	}

	type routeKey struct {
		target *types.Func
		gate   *ps6079Gate
		shape  string
	}
	seenRoutes := make(map[routeKey]bool, len(routes))
	for _, route := range routes {
		seenRoutes[routeKey{target: route.target, gate: route.gate, shape: ps6079RequirementShape(route.requires)}] = true
	}
	reported := make(map[string]bool)
	for len(routes) > 0 {
		route := routes[0]
		routes = routes[1:]
		for _, site := range sites[route.target] {
			if ps6006Benchmark(pass, site.caller) {
				if !ps6079MeasuredBenchmarkCall(pass, site.caller, site.call) {
					continue
				}
				key := site.caller.Name.Name + "|" + strconv.FormatInt(int64(route.gate.condition.Pos()), 10)
				if reported[key] || ps6079Validated(site.caller) ||
					ps6079HasRouteProof(pass, declarations, site.caller, site.call, route) {
					continue
				}
				if ps6079ReportBenchmark(pass, site.caller, site.call, route, declarations) {
					reported[key] = true
				}
				continue
			}
			mapped, ok := ps6079MapRequirements(pass, site.callerObject, site.call, route.requires)
			if !ok {
				continue
			}
			next := ps6079Route{
				target: site.callerObject, requires: mapped, gate: route.gate,
				path: append([]string{site.caller.Name.Name}, route.path...), contributors: route.contributors,
				uniqueContributor: route.uniqueContributor &&
					ps6079RouteContributorCallCount(pass, site.caller, route.contributors) == 1,
				reachableGates: route.reachableGates,
			}
			key := routeKey{target: next.target, gate: next.gate, shape: ps6079RequirementShape(next.requires)}
			if !seenRoutes[key] {
				seenRoutes[key] = true
				routes = append(routes, next)
			}
		}
	}
	return nil, nil
}

func ps6079PackageHasBenchmark(pass *analysis.Pass, functions []*ast.FuncDecl) bool {
	return slices.ContainsFunc(functions, func(function *ast.FuncDecl) bool {
		return ps6006Benchmark(pass, function)
	})
}

func ps6079MeasuredBenchmarkCall(pass *analysis.Pass, benchmark *ast.FuncDecl, call *ast.CallExpr) bool {
	receiver := ps6079BenchmarkParameter(pass, benchmark)
	if receiver == nil || benchmark.Body == nil {
		return false
	}
	parents := ps6071Parents(benchmark.Body)
	for node := ast.Node(call); node != nil; node = parents[node] {
		switch loop := node.(type) {
		case *ast.ForStmt:
			if ps6079BenchmarkForLoop(pass, loop, receiver) && ps6079RepeatedForRegion(call, loop, parents) {
				return true
			}
		case *ast.RangeStmt:
			if ps6079ContainsBenchmarkN(pass, loop.X, receiver) && ps6079RepeatedRangeRegion(call, loop, parents) {
				return true
			}
		}
	}
	return false
}

func ps6079RepeatedForRegion(call *ast.CallExpr, loop *ast.ForStmt, parents map[ast.Node]ast.Node) bool {
	child := ast.Node(call)
	for child != nil && parents[child] != loop {
		child = parents[child]
	}
	return child != nil && child != loop.Init
}

func ps6079RepeatedRangeRegion(call *ast.CallExpr, loop *ast.RangeStmt, parents map[ast.Node]ast.Node) bool {
	child := ast.Node(call)
	for child != nil && parents[child] != loop {
		child = parents[child]
	}
	return child == loop.Body
}

func ps6079BenchmarkForLoop(pass *analysis.Pass, loop *ast.ForStmt, receiver types.Object) bool {
	if ps6079ContainsBenchmarkLoopCall(pass, loop.Cond, receiver) {
		return true
	}
	for _, node := range []ast.Node{loop.Init, loop.Cond, loop.Post} {
		if ps6079ContainsBenchmarkN(pass, node, receiver) {
			return true
		}
	}
	return false
}

func ps6079ContainsBenchmarkLoopCall(pass *analysis.Pass, node ast.Node, receiver types.Object) bool {
	found := false
	ast.Inspect(node, func(child ast.Node) bool {
		if found {
			return false
		}
		call, ok := child.(*ast.CallExpr)
		if !ok || len(call.Args) != 0 {
			return true
		}
		selector, ok := ps2110Unparen(call.Fun).(*ast.SelectorExpr)
		found = ok && selector.Sel.Name == "Loop" && ps6079RootObject(pass, selector.X) == receiver
		return !found
	})
	return found
}

func ps6079ContainsBenchmarkN(pass *analysis.Pass, node ast.Node, receiver types.Object) bool {
	found := false
	ast.Inspect(node, func(child ast.Node) bool {
		if found {
			return false
		}
		selector, ok := child.(*ast.SelectorExpr)
		found = ok && selector.Sel.Name == "N" && ps6079RootObject(pass, selector.X) == receiver
		return !found
	})
	return found
}

func ps6079PackageFunctions(pass *analysis.Pass) ([]*ast.FuncDecl, map[*types.Func][]ps6079CallSite, map[*types.Func]*ast.FuncDecl) {
	var functions []*ast.FuncDecl
	sites := make(map[*types.Func][]ps6079CallSite)
	declarations := make(map[*types.Func]*ast.FuncDecl)
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
			functions = append(functions, function)
			declarations[object] = function
			ast.Inspect(function.Body, func(node ast.Node) bool {
				if _, nested := node.(*ast.FuncLit); nested {
					return false
				}
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				callee, _, ok := typedCallee(pass, call.Fun)
				if !ok || callee.Pkg() == nil || callee.Pkg().Path() != pass.Pkg.Path() {
					return true
				}
				sites[callee] = append(sites[callee], ps6079CallSite{
					caller: function, callerObject: object, call: call,
				})
				return true
			})
		}
	}
	return functions, sites, declarations
}

func ps6079RouteContributors(target *types.Func, sites map[*types.Func][]ps6079CallSite) map[*types.Func]bool {
	result := map[*types.Func]bool{target: true}
	queue := []*types.Func{target}
	for len(queue) > 0 {
		callee := queue[0]
		queue = queue[1:]
		for _, site := range sites[callee] {
			if site.callerObject != nil && !result[site.callerObject] {
				result[site.callerObject] = true
				queue = append(queue, site.callerObject)
			}
		}
	}
	return result
}

func ps6079RouteContributorCallCount(
	pass *analysis.Pass,
	function *ast.FuncDecl,
	contributors map[*types.Func]bool,
) int {
	count := 0
	ast.Inspect(function.Body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		callee, _, known := typedCallee(pass, call.Fun)
		if known && contributors[callee] {
			count++
		}
		return count < 2
	})
	return count
}

func ps6079FunctionGates(pass *analysis.Pass, function *ast.FuncDecl) []ps6079Gate {
	if function.Body == nil || ps6006Benchmark(pass, function) {
		return nil
	}
	object, ok := pass.TypesInfo.Defs[function.Name].(*types.Func)
	if !ok {
		return nil
	}
	parents := ps6071Parents(function.Body)
	vectorNames := ps6079VectorNames(pass)
	functionValues := ps6079ImmutableFunctionValues(pass, function.Body)
	var result []ps6079Gate
	ast.Inspect(function.Body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		statement, ok := node.(*ast.IfStmt)
		if !ok || statement.Init != nil {
			return true
		}
		thenFast, thenFallback := ps6079BranchTargets(pass, []ast.Node{statement.Body}, vectorNames)
		var other []ast.Node
		if statement.Else != nil {
			other = append(other, statement.Else)
		} else if ps6078Terminates(statement.Body) {
			other = append(other, ps6078Following(statement, parents)...)
		}
		otherFast, otherFallback := ps6079BranchTargets(pass, other, vectorNames)
		fastWhenTrue := false
		optimized, fallback := "", ""
		var optimizedNodes []ast.Node
		switch {
		case thenFast != "" && thenFallback == "" && otherFallback != "" && otherFast == "":
			fastWhenTrue, optimized, fallback = true, thenFast, otherFallback
			optimizedNodes = []ast.Node{statement.Body}
		case thenFallback != "" && thenFast == "" && otherFast != "" && otherFallback == "":
			optimized, fallback = otherFast, thenFallback
			optimizedNodes = other
			if statement.Else == nil && ps6079ContainsGoto(statement.Body) {
				// The lexical tail still identifies the optimized target, but a
				// goto anywhere in the fallback can re-enter that tail, including
				// from nested control flow before a final return. Do not treat any
				// of the tail as exclusive route-counter evidence.
				optimizedNodes = nil
			}
		default:
			return true
		}
		requirements := ps6079ConditionRequirements(pass, object, statement.Cond, fastWhenTrue, functionValues)
		if len(requirements) != 0 {
			result = append(result, ps6079Gate{
				function: function, object: object, condition: statement.Cond,
				requires: requirements, optimized: optimized, fallback: fallback, optimizedNodes: optimizedNodes,
			})
		}
		return true
	})
	return result
}

func ps6079ContainsGoto(block *ast.BlockStmt) bool {
	found := false
	ast.Inspect(block, func(node ast.Node) bool {
		if found {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		branch, ok := node.(*ast.BranchStmt)
		found = ok && branch.Tok == token.GOTO
		return !found
	})
	return found
}

func ps6079VectorNames(pass *analysis.Pass) map[string]bool {
	result := make(map[string]bool)
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok {
				continue
			}
			_, rank := ps6077VectorEvidence(function)
			if rank > 0 {
				result[function.Name.Name] = true
			}
		}
	}
	return result
}

func ps6079BranchTargets(pass *analysis.Pass, nodes []ast.Node, vectorNames map[string]bool) (string, string) {
	optimized, fallback := "", ""
	for _, node := range nodes {
		ast.Inspect(node, func(candidate ast.Node) bool {
			if _, nested := candidate.(*ast.FuncLit); nested {
				return false
			}
			call, ok := candidate.(*ast.CallExpr)
			if !ok {
				return true
			}
			callee, _, ok := typedCallee(pass, call.Fun)
			if !ok || callee.Pkg() == nil || callee.Pkg().Path() != pass.Pkg.Path() {
				return true
			}
			name := callee.Name()
			if optimized == "" && (ps6078OptimizedName(name) || vectorNames[name]) {
				optimized = name
			}
			if fallback == "" && ps6079FallbackName(name) {
				fallback = name
			}
			return true
		})
	}
	return optimized, fallback
}

func ps6079FallbackName(name string) bool {
	return ps6007ContainsAny(ps6007NormalizeName(name), "scalar", "fallback", "generic", "reference", "slow")
}

func ps6079ConditionRequirements(
	pass *analysis.Pass,
	function *types.Func,
	expression ast.Expr,
	want bool,
	functionValues map[types.Object][]ast.Expr,
) []ps6079Requirement {
	expression = ps2110Unparen(expression)
	switch candidate := expression.(type) {
	case *ast.UnaryExpr:
		if candidate.Op == token.NOT {
			return ps6079ConditionRequirements(pass, function, candidate.X, !want, functionValues)
		}
	case *ast.BinaryExpr:
		if candidate.Op == token.LAND && want || candidate.Op == token.LOR && !want {
			left := ps6079ConditionRequirements(pass, function, candidate.X, want, functionValues)
			right := ps6079ConditionRequirements(pass, function, candidate.Y, want, functionValues)
			return ps6079MergeRequirements(left, right)
		}
		if requirement, ok := ps6079ZeroRequirement(pass, function, candidate, want); ok {
			return []ps6079Requirement{requirement}
		}
	case *ast.CallExpr:
		if !want || len(candidate.Args) == 0 {
			return nil
		}
		relation, ok := ps6079SemanticPredicateRelation(pass, candidate.Fun, functionValues)
		if !ok {
			return nil
		}
		parameter, name, ok := ps6079ParameterExpression(pass, function, candidate.Args[0])
		if ok {
			return []ps6079Requirement{{parameter: parameter, relation: relation, input: name}}
		}
	}
	return nil
}

func ps6079ZeroRequirement(pass *analysis.Pass, function *types.Func, comparison *ast.BinaryExpr, want bool) (ps6079Requirement, bool) {
	operation := comparison.Op
	value := comparison.X
	if ps6079Zero(pass, comparison.X) {
		value = comparison.Y
		operation = ps6079ReverseComparison(operation)
	} else if !ps6079Zero(pass, comparison.Y) {
		return ps6079Requirement{}, false
	}
	if !want {
		operation = ps6079NegateComparison(operation)
	}
	relation, ok := ps6079ComparisonRelation(operation)
	if !ok {
		return ps6079Requirement{}, false
	}
	parameter, name, ok := ps6079ParameterExpression(pass, function, value)
	if !ok {
		return ps6079Requirement{}, false
	}
	return ps6079Requirement{parameter: parameter, relation: relation, input: name}, true
}

func ps6079Zero(pass *analysis.Pass, expression ast.Expr) bool {
	value := pass.TypesInfo.Types[ps2110Unparen(expression)].Value
	sign, ok := ps6079NumericSign(value)
	return ok && sign == 0
}

func ps6079ReverseComparison(operation token.Token) token.Token {
	switch operation {
	case token.GTR:
		return token.LSS
	case token.GEQ:
		return token.LEQ
	case token.LSS:
		return token.GTR
	case token.LEQ:
		return token.GEQ
	default:
		return operation
	}
}

func ps6079NegateComparison(operation token.Token) token.Token {
	switch operation {
	case token.GTR:
		return token.LEQ
	case token.GEQ:
		return token.LSS
	case token.LSS:
		return token.GEQ
	case token.LEQ:
		return token.GTR
	case token.EQL:
		return token.NEQ
	case token.NEQ:
		return token.EQL
	default:
		return token.ILLEGAL
	}
}

func ps6079ComparisonRelation(operation token.Token) (ps6079Relation, bool) {
	switch operation {
	case token.GEQ:
		return ps6079AtLeastZero, true
	case token.GTR:
		return ps6079AboveZero, true
	case token.LEQ:
		return ps6079AtMostZero, true
	case token.LSS:
		return ps6079BelowZero, true
	case token.EQL:
		return ps6079ExactlyZero, true
	default:
		return 0, false
	}
}

func ps6079PredicateRelation(name string) (ps6079Relation, bool) {
	name = ps6007NormalizeName(name)
	switch {
	case ps6007ContainsAny(name, "nonnegative", "nonneg", "atleastzero", "greaterorequalzero"):
		return ps6079AtLeastZero, true
	case ps6007ContainsAny(name, "nonpositive", "nonpos", "atmostzero", "lessorequalzero"):
		return ps6079AtMostZero, true
	case strings.Contains(name, "positive"):
		return ps6079AboveZero, true
	case strings.Contains(name, "negative"):
		return ps6079BelowZero, true
	default:
		return 0, false
	}
}

func ps6079SemanticPredicateRelation(
	pass *analysis.Pass,
	function ast.Expr,
	functionValues map[types.Object][]ast.Expr,
) (ps6079Relation, bool) {
	callables, resolved := ps6079SemanticCallables(pass, function, functionValues)
	if !resolved {
		return 0, false
	}
	var relation ps6079Relation
	for index, callable := range callables {
		callee, _, _ := typedCallee(pass, callable)
		current, ok := ps6079PredicateRelation(callee.Name())
		if !ok || index > 0 && current != relation {
			return 0, false
		}
		relation = current
	}
	return relation, true
}

func ps6079ParameterExpression(pass *analysis.Pass, function *types.Func, expression ast.Expr) (int, string, bool) {
	expression = ps2110Unparen(expression)
	identifier, ok := expression.(*ast.Ident)
	if !ok {
		return 0, "", false
	}
	object := pass.TypesInfo.ObjectOf(identifier)
	signature, ok := function.Type().(*types.Signature)
	if !ok {
		return 0, "", false
	}
	for parameter := range signature.Params().Len() {
		if signature.Params().At(parameter) == object {
			return parameter, identifier.Name, true
		}
	}
	return 0, "", false
}

func ps6079MergeRequirements(left, right []ps6079Requirement) []ps6079Requirement {
	result := slices.Concat(left, right)
	slices.SortFunc(result, func(a, b ps6079Requirement) int {
		if a.parameter != b.parameter {
			return a.parameter - b.parameter
		}
		return int(a.relation) - int(b.relation)
	})
	return slices.CompactFunc(result, func(a, b ps6079Requirement) bool {
		return a.parameter == b.parameter && a.relation == b.relation
	})
}

func ps6079MapRequirements(pass *analysis.Pass, caller *types.Func, call *ast.CallExpr, requirements []ps6079Requirement) ([]ps6079Requirement, bool) {
	mapped := make([]ps6079Requirement, 0, len(requirements))
	for _, requirement := range requirements {
		if requirement.parameter >= len(call.Args) {
			return nil, false
		}
		parameter, _, ok := ps6079ParameterExpression(pass, caller, call.Args[requirement.parameter])
		if !ok {
			return nil, false
		}
		requirement.parameter = parameter
		mapped = append(mapped, requirement)
	}
	return ps6079MergeRequirements(mapped, nil), true
}

func ps6079RequirementShape(requirements []ps6079Requirement) string {
	var builder strings.Builder
	builder.Grow(len(requirements) * 8)
	for _, requirement := range requirements {
		builder.WriteString(strconv.Itoa(requirement.parameter))
		builder.WriteByte(':')
		builder.WriteString(strconv.Itoa(int(requirement.relation)))
		builder.WriteByte(';')
	}
	return builder.String()
}

func ps6079ReportBenchmark(
	pass *analysis.Pass,
	benchmark *ast.FuncDecl,
	call *ast.CallExpr,
	route ps6079Route,
	declarations map[*types.Func]*ast.FuncDecl,
) bool {
	events := ps6079FixtureEvents(pass, declarations, benchmark.Body)
	functionValues := ps6079ImmutableFunctionValues(pass, benchmark.Body)
	flow := cfg.New(benchmark.Body, func(*ast.CallExpr) bool { return true })
	var missing []string
	related := []analysis.RelatedInformation{{
		Pos: route.gate.condition.Pos(), End: route.gate.condition.End(),
		Message: "semantic guard selects " + route.gate.optimized + " instead of " + route.gate.fallback,
	}}
	var seenRelated []token.Pos
	for _, requirement := range route.requires {
		if requirement.parameter >= len(call.Args) {
			return false
		}
		argument := call.Args[requirement.parameter]
		state, event := ps6079FixtureExpression(pass, declarations, argument, events, functionValues, call.Pos())
		mayReturnAsynchronously := ps6079ExpressionMayReturnAsynchronousReference(pass, declarations, argument)
		if argumentCall, ok := ps2110Unparen(argument).(*ast.CallExpr); ok &&
			ps6079SynchronousSamePackageCall(pass, declarations, argumentCall, functionValues) {
			mayReturnAsynchronously = false
		}
		if mayReturnAsynchronously {
			state = ps6079UnknownFixture
			event = ps6079FixtureEvent{
				position: argument.Pos(), state: state,
				source: "reference-returning call may mutate its result asynchronously",
			}
		}
		if event.position.IsValid() && event.position < call.Pos() &&
			ps6079FixtureSatisfies(state, requirement.relation) &&
			!ps6079GraphPositionDominates(flow, event.position, call.Pos()) {
			state = ps6079UnknownFixture
			event.state = state
			event.source = "non-dominating " + event.source
		}
		if ps6079PositionInLoop(benchmark.Body, call.Pos()) &&
			ps6079ParameterMayMutate(pass, declarations, route.target, requirement.parameter, make(map[ps6079ParameterKey]bool)) {
			state = ps6079UnknownFixture
			event = ps6079FixtureEvent{
				position: call.Pos(), state: state, source: "loop-carried routed call may mutate fixture",
			}
		}
		if ps6079FixtureSatisfies(state, requirement.relation) {
			continue
		}
		source := exprTextRendered(argument)
		if event.source != "" {
			source = event.source
		}
		missing = append(missing, requirement.input+" ("+exprTextRendered(argument)+") requires "+
			ps6079RelationText(requirement.relation)+" but "+source+" is "+ps6079FixtureStateText(state))
		if event.position.IsValid() && !slices.Contains(seenRelated, event.position) {
			seenRelated = append(seenRelated, event.position)
			related = append(related, analysis.RelatedInformation{
				Pos:     event.position,
				Message: "fixture provenance: " + source + " is " + ps6079FixtureStateText(state),
			})
		}
	}
	if len(missing) == 0 {
		return false
	}
	pass.Report(analysis.Diagnostic{
		Pos: call.Pos(), End: call.End(),
		Message: "benchmark " + benchmark.Name.Name + " reaches " + route.gate.function.Name.Name +
			" through " + strings.Join(route.path, " -> ") +
			" but its fixture does not prove guarded fast-path condition " + exprTextRendered(route.gate.condition) +
			": " + strings.Join(missing, "; ") + ". The optimized target is " + route.gate.optimized +
			" and the observed alternative can be " + route.gate.fallback +
			"; construct a representative semantic workload and reset then assert a post-warmup route counter (or validated profile) instead of force-enabling the route (advisory, no automatic fix)",
		Related: related,
	})
	return true
}

func ps6079FixtureEvents(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	body *ast.BlockStmt,
) map[types.Object][]ps6079FixtureEvent {
	events := make(map[types.Object][]ps6079FixtureEvent)
	addressAliases := make(map[types.Object]map[types.Object]bool)
	referenceAliases := make(map[types.Object]map[types.Object]bool)
	asynchronousEscapes := make(map[types.Object]bool)
	initializationInvalidations := make(map[types.Object]bool)
	ps6079BootstrapPackageReferenceAliases(
		pass, declarations, referenceAliases, asynchronousEscapes, initializationInvalidations,
	)
	partialReferenceAliases := make(map[types.Object]bool)
	functionValues := ps6079PackageFunctionValues(pass)
	semanticFunctionValues := ps6079ImmutableFunctionValues(pass, body)
	stableReferenceObjects := ps6079StableReferenceObjects(pass, body)
	ps6079SeedPackageFixtureEvents(
		pass, declarations, events, semanticFunctionValues, referenceAliases,
		initializationInvalidations, asynchronousEscapes,
	)
	recordFunctionValue := func(left ast.Expr, right ast.Expr) {
		typeOf := pass.TypesInfo.TypeOf(right)
		if !ps6079TypeCanCarryFunction(typeOf, make(map[types.Type]bool)) {
			return
		}
		if object := ps6079RootObject(pass, left); object != nil {
			for _, existing := range functionValues[object] {
				if existing == right {
					return
				}
			}
			functionValues[object] = append(functionValues[object], right)
		}
	}
	astutil.WithStack(body, func(node ast.Node, stack []ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		loops := ps6079FixtureEventLoops(stack)
		conditional := ps6079ConditionalFixtureEvent(stack)
		aliasConditional := ps6079ConditionalControlFlow(node, stack)
		recordOne := func(object types.Object, position token.Pos, state ps6079FixtureState, source string) {
			if object == nil || object.Name() == "_" {
				return
			}
			if asynchronousEscapes[object] && !strings.HasPrefix(source, "asynchronously escaped fixture") {
				state = ps6079UnknownFixture
				source = "asynchronously escaped fixture remains unknown after " + source
			}
			if conditional {
				state = ps6079UnknownFixture
				source = "conditional " + source
			}
			events[object] = append(events[object], ps6079FixtureEvent{
				position: position, state: state, source: source, loops: loops,
			})
		}
		record := func(object types.Object, position token.Pos, state ps6079FixtureState, source string) {
			seen := make(map[types.Object]bool)
			queue := []types.Object{object}
			var component []types.Object
			for len(queue) > 0 {
				object = queue[0]
				queue = queue[1:]
				if object == nil || seen[object] {
					continue
				}
				seen[object] = true
				component = append(component, object)
				for target := range addressAliases[object] {
					queue = append(queue, target)
				}
				for target := range referenceAliases[object] {
					queue = append(queue, target)
				}
			}
			escaped := false
			for _, object := range component {
				escaped = escaped || asynchronousEscapes[object]
			}
			if escaped {
				state = ps6079UnknownFixture
				source = "asynchronously escaped fixture remains unknown after " + source
			}
			for _, object := range component {
				recordOne(object, position, state, source)
			}
		}
		markAsynchronousEscape := func(root types.Object) {
			seen := make(map[types.Object]bool)
			queue := []types.Object{root}
			for len(queue) > 0 {
				object := queue[0]
				queue = queue[1:]
				if object == nil || seen[object] {
					continue
				}
				seen[object] = true
				asynchronousEscapes[object] = true
				for target := range addressAliases[object] {
					queue = append(queue, target)
				}
				for target := range referenceAliases[object] {
					queue = append(queue, target)
				}
			}
		}
		propagateAsynchronousEscape := func(destination types.Object, component map[types.Object]bool) {
			if destination == nil || asynchronousEscapes[destination] {
				return
			}
			for object := range component {
				if asynchronousEscapes[object] {
					asynchronousEscapes[destination] = true
					return
				}
			}
			for object := range addressAliases[destination] {
				if asynchronousEscapes[object] {
					asynchronousEscapes[destination] = true
					return
				}
			}
		}
		markUnsafeDerivedCarrier := func(
			destination types.Object,
			expression ast.Expr,
			component map[types.Object]bool,
		) {
			if !ps6079UnsafeDerivedExpression(
				pass, declarations, expression, semanticFunctionValues, referenceAliases,
			) {
				return
			}
			if component == nil {
				component = make(map[types.Object]bool)
			}
			for object := range ps6079UnsafeDerivedSourceComponent(
				pass, declarations, expression, semanticFunctionValues, referenceAliases,
			) {
				component[object] = true
			}
			if _, ok := ps2110Unparen(expression).(*ast.CallExpr); ok {
				ps6079UpdateReferenceAliasComponent(destination, component, true, referenceAliases)
			}
			markAsynchronousEscape(destination)
			if destination != nil {
				record(destination, expression.Pos(), ps6079UnknownFixture,
					"unsafe-derived fixture alias")
			}
			for object := range component {
				markAsynchronousEscape(object)
				record(object, expression.Pos(), ps6079UnknownFixture,
					"fixture storage exposed through unsafe representation")
			}
		}
		markAsynchronousReceive := func(destination types.Object, expression ast.Expr) {
			mayReceive := ps6079ExpressionMayReceiveReference(
				pass, declarations, expression, make(map[*types.Func]bool),
			)
			if call, ok := ps2110Unparen(expression).(*ast.CallExpr); ok {
				if callee, _, known := typedCallee(pass, call.Fun); known && callee != nil &&
					callee.Pkg() != nil && pass.Pkg != nil && callee.Pkg().Path() == pass.Pkg.Path() {
					mayReceive = ps6079CallHasConcreteReferenceReceive(
						pass, declarations, call, make(map[*types.Func]bool),
					)
				}
			}
			if destination != nil && mayReceive {
				asynchronousEscapes[destination] = true
			}
		}
		switch value := node.(type) {
		case *ast.AssignStmt:
			if len(value.Lhs) == len(value.Rhs) {
				for index, left := range value.Lhs {
					recordFunctionValue(left, value.Rhs[index])
				}
			} else if len(value.Rhs) == 1 {
				for _, left := range value.Lhs {
					recordFunctionValue(left, value.Rhs[0])
				}
			}
			if ps6079AssignmentMayRetainReference(pass, value) {
				retainedFunction := false
				for _, expression := range value.Rhs {
					retainedFunction = retainedFunction || ps6079TypeCanCarryFunction(
						pass.TypesInfo.TypeOf(expression), make(map[types.Type]bool),
					)
					for object := range ps6079ReferenceAliasComponentWithReturns(
						pass, declarations, expression, referenceAliases,
					) {
						markAsynchronousEscape(object)
						record(object, value.Pos(), ps6079UnknownFixture, "fixture retained by assignment")
					}
				}
				if retainedFunction {
					for _, expression := range value.Rhs {
						for object := range ps6079FunctionReferenceCaptures(
							pass, declarations, expression, functionValues,
						) {
							markAsynchronousEscape(object)
							record(object, value.Pos(), ps6079UnknownFixture,
								"retained function may access captured fixture")
						}
					}
					for object := range events {
						markAsynchronousEscape(object)
						record(object, value.Pos(), ps6079UnknownFixture,
							"retained function may access captured fixture")
					}
				}
			}
			if value.Tok != token.ASSIGN && value.Tok != token.DEFINE {
				for _, left := range value.Lhs {
					ps6079UnknownFixtureTarget(
						pass, declarations, left, value.Pos(), "compound assignment", referenceAliases, record,
					)
				}
				break
			}
			if len(value.Lhs) != len(value.Rhs) {
				if len(value.Rhs) == 1 {
					component, referenceResults := ps6079MultiResultReferenceComponent(
						pass, declarations, value.Rhs[0], len(value.Lhs), referenceAliases,
					)
					for index, left := range value.Lhs {
						if !referenceResults[index] {
							continue
						}
						identifier, direct := ps2110Unparen(left).(*ast.Ident)
						ps6079UpdateReferenceAliasComponent(
							ps6079RootObject(pass, left), component, aliasConditional || !direct || identifier.Name == "_",
							referenceAliases,
						)
						markUnsafeDerivedCarrier(ps6079RootObject(pass, left), value.Rhs[0], component)
						propagateAsynchronousEscape(ps6079RootObject(pass, left), component)
						markAsynchronousReceive(ps6079RootObject(pass, left), value.Rhs[0])
						ps6079MarkPartialReferenceAlias(
							ps6079RootObject(pass, left), true, aliasConditional || !direct || identifier.Name == "_",
							partialReferenceAliases,
						)
					}
				}
				for _, left := range value.Lhs {
					ps6079UnknownFixtureTarget(
						pass, declarations, left, value.Pos(), "multi-value assignment", referenceAliases, record,
					)
				}
				break
			}
			if len(value.Lhs) == len(value.Rhs) {
				referenceComponents := make([]map[types.Object]bool, len(value.Rhs))
				for index, expression := range value.Rhs {
					referenceComponents[index] = ps6079ReferenceAliasComponentWithReturns(
						pass, declarations, expression, referenceAliases,
					)
				}
				ambiguousParallelReferences := make(map[types.Object]bool)
				if len(value.Lhs) > 1 {
					for _, left := range value.Lhs {
						identifier, direct := ps2110Unparen(left).(*ast.Ident)
						if !direct {
							continue
						}
						object := pass.TypesInfo.ObjectOf(identifier)
						if object == nil || !ps6079AliasCarrierType(object.Type()) {
							continue
						}
						if value.Tok == token.DEFINE && pass.TypesInfo.Defs[identifier] != nil {
							continue
						}
						ambiguousParallelReferences[object] = true
						ps6079UpdateReferenceAliasComponent(object, nil, false, referenceAliases)
						delete(addressAliases, object)
						ps6079MarkPartialReferenceAlias(object, true, false, partialReferenceAliases)
					}
				}
				for index, left := range value.Lhs {
					if identifier, ok := ps2110Unparen(left).(*ast.Ident); ok {
						object := pass.TypesInfo.ObjectOf(identifier)
						if !ambiguousParallelReferences[object] {
							ps6079UpdateAddressAlias(pass, object, value.Rhs[index], conditional, addressAliases)
							ps6079UpdateReferenceAliasComponent(object, referenceComponents[index], aliasConditional, referenceAliases)
							markUnsafeDerivedCarrier(object, value.Rhs[index], referenceComponents[index])
							propagateAsynchronousEscape(object, referenceComponents[index])
						}
						markAsynchronousReceive(object, value.Rhs[index])
						if !ambiguousParallelReferences[object] {
							ps6079UpdatePartialReferenceAlias(
								pass, object, value.Rhs[index], aliasConditional, partialReferenceAliases,
							)
						}
						state, event := ps6079AssignedFixtureExpression(
							pass, declarations, value.Rhs[index], events, semanticFunctionValues, value.Pos(),
						)
						source := exprTextRendered(value.Rhs[index])
						if event.source != "" {
							source = event.source
						}
						if ambiguousParallelReferences[object] {
							state = ps6079UnknownFixture
							source = "parallel reference assignment"
						}
						recordOne(object, value.Pos(), state, source)
						for returned := range referenceComponents[index] {
							if pass.Pkg != nil && returned.Parent() == pass.Pkg.Scope() {
								recordOne(returned, value.Pos(), state, source)
							}
						}
						ps6079InvalidateAliasSource(pass, value.Rhs[index], value.Pos(), record)
						continue
					}
					// A selector or index assignment can store a reference inside an
					// existing aggregate. Keep the aggregate root connected to every
					// source component. Do not disconnect older components here: another
					// member may still retain them.
					root := ps6079RootObject(pass, left)
					if len(value.Lhs) == 1 {
						ps6079UpdateReferenceAliasComponent(root, referenceComponents[index], true, referenceAliases)
						markUnsafeDerivedCarrier(root, value.Rhs[index], referenceComponents[index])
						propagateAsynchronousEscape(root, referenceComponents[index])
					}
					markAsynchronousReceive(root, value.Rhs[index])
					if len(value.Lhs) > 1 {
						ps6079MarkPartialReferenceAlias(root, true, false, partialReferenceAliases)
					} else {
						ps6079UpdatePartialReferenceAlias(
							pass, root, value.Rhs[index], true, partialReferenceAliases,
						)
					}
					if indexed, ok := ps2110Unparen(left).(*ast.IndexExpr); ok {
						if object := ps6079RootObject(pass, indexed.X); object != nil {
							record(object, value.Pos(), ps6079UnknownFixture, "indexed fixture write")
						} else {
							ps6079UnknownFixtureTarget(
								pass, declarations, left, value.Pos(), "indexed fixture write", referenceAliases, record,
							)
						}
					} else {
						ps6079UnknownFixtureTarget(
							pass, declarations, left, value.Pos(), "indirect fixture write", referenceAliases, record,
						)
					}
					ps6079InvalidateAliasSource(pass, value.Rhs[index], value.Pos(), record)
				}
			}
		case *ast.ValueSpec:
			if len(value.Names) == len(value.Values) {
				for index, name := range value.Names {
					recordFunctionValue(name, value.Values[index])
				}
			} else if len(value.Values) == 1 {
				for _, name := range value.Names {
					recordFunctionValue(name, value.Values[0])
				}
			}
			if len(value.Names) != len(value.Values) && len(value.Values) == 1 {
				component, referenceResults := ps6079MultiResultReferenceComponent(
					pass, declarations, value.Values[0], len(value.Names), referenceAliases,
				)
				for index, name := range value.Names {
					if !referenceResults[index] || name.Name == "_" {
						continue
					}
					object := pass.TypesInfo.Defs[name]
					ps6079UpdateReferenceAliasComponent(object, component, aliasConditional, referenceAliases)
					markUnsafeDerivedCarrier(object, value.Values[0], component)
					propagateAsynchronousEscape(object, component)
					markAsynchronousReceive(object, value.Values[0])
					ps6079MarkPartialReferenceAlias(object, true, aliasConditional, partialReferenceAliases)
				}
				for _, name := range value.Names {
					record(pass.TypesInfo.Defs[name], value.Pos(), ps6079UnknownFixture, "multi-value declaration")
				}
			}
			if len(value.Names) == len(value.Values) {
				referenceComponents := make([]map[types.Object]bool, len(value.Values))
				for index, expression := range value.Values {
					referenceComponents[index] = ps6079ReferenceAliasComponentWithReturns(
						pass, declarations, expression, referenceAliases,
					)
				}
				for index, name := range value.Names {
					object := pass.TypesInfo.Defs[name]
					ps6079UpdateAddressAlias(pass, object, value.Values[index], conditional, addressAliases)
					ps6079UpdateReferenceAliasComponent(object, referenceComponents[index], aliasConditional, referenceAliases)
					markUnsafeDerivedCarrier(object, value.Values[index], referenceComponents[index])
					propagateAsynchronousEscape(object, referenceComponents[index])
					markAsynchronousReceive(object, value.Values[index])
					ps6079UpdatePartialReferenceAlias(
						pass, object, value.Values[index], aliasConditional, partialReferenceAliases,
					)
					state, event := ps6079AssignedFixtureExpression(
						pass, declarations, value.Values[index], events, semanticFunctionValues, value.Pos(),
					)
					source := exprTextRendered(value.Values[index])
					if event.source != "" {
						source = event.source
					}
					recordOne(object, value.Pos(), state, source)
					for returned := range referenceComponents[index] {
						if pass.Pkg != nil && returned.Parent() == pass.Pkg.Scope() {
							recordOne(returned, value.Pos(), state, source)
						}
					}
					ps6079InvalidateAliasSource(pass, value.Values[index], value.Pos(), record)
				}
			}
		case *ast.CallExpr:
			ps6079RecordBuiltinFunctionFlow(pass, value, recordFunctionValue)
			markUnsafeDerivedCarrier(nil, value, nil)
			if callee, _, known := typedCallee(pass, value.Fun); known && callee != nil &&
				ps6079DynamicDispatch(pass, value.Fun, callee) {
				for object := range ps6079AllPackageStorage(pass) {
					markAsynchronousEscape(object)
				}
				for object := range events {
					markAsynchronousEscape(object)
					record(object, value.Pos(), ps6079UnknownFixture,
						"dynamic call may retain or mutate fixture storage")
				}
			}
			if len(stack) == 0 {
				break
			}
			var protectedPackageObjects map[types.Object]bool
			packageStorageReadOnly := !ps6079CallMayMutatePackageStorageWithContext(
				pass, declarations, value, nil, semanticFunctionValues,
				make(map[*types.Func]bool), make(map[*ast.FuncLit]bool),
			)
			if packageStorageReadOnly {
				protectedPackageObjects = ps6079AllPackageStorage(pass)
			}
			synchronousSamePackageRead := ps6079SynchronousSamePackageCall(
				pass, declarations, value, semanticFunctionValues,
			)
			switch stack[len(stack)-1].(type) {
			case *ast.DeferStmt:
				// Deferred calls run after the benchmark body returns, so they do not
				// constrain the measured fixture.
			case *ast.GoStmt:
				for _, object := range ps6079AsynchronousFixtureRoots(
					pass, declarations, value, events, functionValues, referenceAliases,
				) {
					markAsynchronousEscape(object)
					record(object, value.Pos(), ps6079UnknownFixture, "asynchronous fixture escape")
				}
				ps6079MutationEvents(
					pass, declarations, value, events, semanticFunctionValues, partialReferenceAliases,
					referenceAliases, stableReferenceObjects, protectedPackageObjects, true, record,
				)
			default:
				if ps6079FunctionArgument(pass, value) {
					for object := range events {
						record(object, value.Pos(), ps6079UnknownFixture,
							"function-valued argument may access captured fixture")
					}
				}
				if !synchronousSamePackageRead &&
					ps6079CallMayLaunchAsynchronousWork(pass, declarations, value, make(map[*types.Func]bool)) {
					for _, object := range ps6079AsynchronousFixtureRoots(
						pass, declarations, value, events, functionValues, referenceAliases,
					) {
						markAsynchronousEscape(object)
						record(object, value.Pos(), ps6079UnknownFixture, "fixture may escape asynchronously through "+exprTextRendered(value.Fun))
					}
					for _, object := range ps6079CallResultFixtureRoots(pass, value, stack) {
						markAsynchronousEscape(object)
						record(object, value.Pos(), ps6079UnknownFixture, "asynchronously mutable call result")
					}
				}
				ps6079MutationEvents(
					pass, declarations, value, events, semanticFunctionValues, partialReferenceAliases,
					referenceAliases, stableReferenceObjects, protectedPackageObjects, false, record,
				)
			}
			if _, deferred := stack[len(stack)-1].(*ast.DeferStmt); !deferred {
				ps6079LinkCallReferenceAliases(
					pass, declarations, value, semanticFunctionValues, referenceAliases, markAsynchronousEscape,
				)
			}
		case *ast.IncDecStmt:
			ps6079UnknownFixtureTarget(
				pass, declarations, value.X, value.Pos(), "increment or decrement", referenceAliases, record,
			)
		case *ast.RangeStmt:
			for _, target := range []ast.Expr{value.Key, value.Value} {
				if target != nil && ps6079TypeCanCarryFunction(
					pass.TypesInfo.TypeOf(target), make(map[types.Type]bool),
				) {
					recordFunctionValue(target, value.X)
				}
			}
			channelReference := ps6079ReferenceChannelRange(pass, value)
			functionRange := ps6079FunctionRange(pass, value)
			functionComponent := make(map[types.Object]bool)
			if functionRange {
				for object := range ps6079FunctionReferenceCaptures(
					pass, declarations, value.X, functionValues,
				) {
					markAsynchronousEscape(object)
					record(object, value.Pos(), ps6079UnknownFixture,
						"range-over-function iterator may access fixture")
					if ps6079AliasCarrierType(object.Type()) {
						functionComponent[object] = true
					}
				}
				for object := range events {
					markAsynchronousEscape(object)
					record(object, value.Pos(), ps6079UnknownFixture,
						"range-over-function iterator may access fixture")
					if ps6079AliasCarrierType(object.Type()) {
						functionComponent[object] = true
					}
				}
			}
			for index, target := range []ast.Expr{value.Key, value.Value} {
				if target == nil {
					continue
				}
				object := ps6079RootObject(pass, target)
				if object == nil || object.Name() == "_" {
					continue
				}
				if ps6079AliasCarrierType(object.Type()) {
					component := ps6079ReferenceAliasComponentWithReturns(
						pass, declarations, value.X, referenceAliases,
					)
					for source := range functionComponent {
						component[source] = true
					}
					ps6079UpdateReferenceAliasComponent(object, component, true, referenceAliases)
					ps6079MarkPartialReferenceAlias(object, true, true, partialReferenceAliases)
				}
				source := "range assignment"
				if index == 0 && channelReference {
					asynchronousEscapes[object] = true
					source = "reference value received from channel range"
				}
				record(object, value.Pos(), ps6079UnknownFixture, source)
			}
		case *ast.SendStmt:
			if ps6079TypeCanCarryFunction(pass.TypesInfo.TypeOf(value.Value), make(map[types.Type]bool)) {
				for object := range ps6079FunctionReferenceCaptures(
					pass, declarations, value.Value, functionValues,
				) {
					markAsynchronousEscape(object)
					record(object, value.Pos(), ps6079UnknownFixture,
						"function sent through channel may access captured fixture")
				}
			}
			for object := range ps6079ReferenceAliasComponentWithReturns(
				pass, declarations, value.Value, referenceAliases,
			) {
				markAsynchronousEscape(object)
				record(object, value.Pos(), ps6079UnknownFixture, "channel send may escape fixture")
			}
		}
		return true
	})
	return events
}

func ps6079CallResultFixtureRoots(
	pass *analysis.Pass,
	call *ast.CallExpr,
	stack []ast.Node,
) []types.Object {
	containsCall := func(expression ast.Expr) bool {
		return expression != nil && expression.Pos() <= call.Pos() && call.End() <= expression.End()
	}
	roots := make(map[types.Object]bool)
	for index := len(stack) - 1; index >= 0; index-- {
		switch ancestor := stack[index].(type) {
		case *ast.AssignStmt:
			if len(ancestor.Lhs) == len(ancestor.Rhs) {
				for expressionIndex, expression := range ancestor.Rhs {
					if !containsCall(expression) {
						continue
					}
					object := ps6079RootObject(pass, ancestor.Lhs[expressionIndex])
					if object != nil && ps6079ContainsReferenceFixture(object.Type()) {
						roots[object] = true
					}
				}
			} else if len(ancestor.Rhs) == 1 && containsCall(ancestor.Rhs[0]) {
				for _, left := range ancestor.Lhs {
					object := ps6079RootObject(pass, left)
					if object != nil && ps6079ContainsReferenceFixture(object.Type()) {
						roots[object] = true
					}
				}
			}
			index = -1
		case *ast.ValueSpec:
			if len(ancestor.Names) == len(ancestor.Values) {
				for expressionIndex, expression := range ancestor.Values {
					if containsCall(expression) {
						object := pass.TypesInfo.Defs[ancestor.Names[expressionIndex]]
						if object != nil && ps6079ContainsReferenceFixture(object.Type()) {
							roots[object] = true
						}
					}
				}
			} else if len(ancestor.Values) == 1 && containsCall(ancestor.Values[0]) {
				for _, name := range ancestor.Names {
					object := pass.TypesInfo.Defs[name]
					if object != nil && ps6079ContainsReferenceFixture(object.Type()) {
						roots[object] = true
					}
				}
			}
			index = -1
		case ast.Stmt:
			index = -1
		}
	}
	result := make([]types.Object, 0, len(roots))
	for object := range roots {
		result = append(result, object)
	}
	return result
}

func ps6079ExpressionMayReturnAsynchronousReference(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	expression ast.Expr,
) bool {
	mayReturn := false
	ast.Inspect(expression, func(node ast.Node) bool {
		if mayReturn {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok || !ps6079ContainsReferenceFixture(pass.TypesInfo.TypeOf(call)) {
			return true
		}
		mayReturn = ps6079CallMayLaunchAsynchronousWork(
			pass, declarations, call, make(map[*types.Func]bool),
		)
		return !mayReturn
	})
	return mayReturn
}

func ps6079FunctionRange(pass *analysis.Pass, statement *ast.RangeStmt) bool {
	if statement == nil {
		return false
	}
	typeOf := pass.TypesInfo.TypeOf(statement.X)
	if typeOf == nil {
		return false
	}
	_, ok := types.Unalias(typeOf).Underlying().(*types.Signature)
	return ok
}

func ps6079ReferenceChannelRange(pass *analysis.Pass, statement *ast.RangeStmt) bool {
	if statement == nil || statement.Key == nil || statement.Value != nil {
		return false
	}
	typeOf := pass.TypesInfo.TypeOf(statement.X)
	if typeOf == nil {
		return false
	}
	channel, ok := types.Unalias(typeOf).Underlying().(*types.Chan)
	return ok && ps6079ContainsReferenceFixture(channel.Elem())
}

func ps6079ReferenceReceiveExpression(pass *analysis.Pass, expression ast.Expr) bool {
	found := false
	ast.Inspect(expression, func(node ast.Node) bool {
		if found {
			return false
		}
		unary, ok := node.(*ast.UnaryExpr)
		if !ok || unary.Op != token.ARROW {
			return true
		}
		typeOf := pass.TypesInfo.TypeOf(unary)
		if typeOf == nil {
			return true
		}
		if tuple, ok := types.Unalias(typeOf).Underlying().(*types.Tuple); ok {
			for index := range tuple.Len() {
				found = found || ps6079ContainsReferenceFixture(tuple.At(index).Type())
			}
		} else {
			found = ps6079ContainsReferenceFixture(typeOf)
		}
		return !found
	})
	return found
}

func ps6079ExpressionMayReceiveReference(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	expression ast.Expr,
	visiting map[*types.Func]bool,
) bool {
	if ps6079ReferenceReceiveExpression(pass, expression) {
		return true
	}
	expression = ps2110Unparen(expression)
	switch value := expression.(type) {
	case *ast.SelectorExpr:
		return ps6079ExpressionMayReceiveReference(pass, declarations, value.X, visiting)
	case *ast.IndexExpr:
		return ps6079ExpressionMayReceiveReference(pass, declarations, value.X, visiting)
	case *ast.IndexListExpr:
		return ps6079ExpressionMayReceiveReference(pass, declarations, value.X, visiting)
	case *ast.SliceExpr:
		return ps6079ExpressionMayReceiveReference(pass, declarations, value.X, visiting)
	case *ast.TypeAssertExpr:
		return ps6079ExpressionMayReceiveReference(pass, declarations, value.X, visiting)
	case *ast.StarExpr:
		return ps6079ExpressionMayReceiveReference(pass, declarations, value.X, visiting)
	}
	call, ok := expression.(*ast.CallExpr)
	if !ok || !ps6079ContainsReferenceFixture(pass.TypesInfo.TypeOf(call)) {
		return false
	}
	if selector, ok := ps2110Unparen(call.Fun).(*ast.SelectorExpr); ok &&
		ps6079ExpressionMayReceiveReference(pass, declarations, selector.X, visiting) {
		return true
	}
	for _, argument := range call.Args {
		if ps6079ExpressionMayReceiveReference(pass, declarations, argument, visiting) {
			return true
		}
	}
	if ps6079BuiltinOrConversion(pass, call.Fun) {
		return false
	}
	if literal, ok := ps2110Unparen(call.Fun).(*ast.FuncLit); ok {
		return ps6079BodyMayReceiveReference(pass, declarations, literal.Body, visiting)
	}
	callee, _, known := typedCallee(pass, call.Fun)
	if !known {
		return true
	}
	if callee.Pkg() == nil || pass.Pkg == nil || callee.Pkg().Path() != pass.Pkg.Path() {
		if selector, ok := ps2110Unparen(call.Fun).(*ast.SelectorExpr); ok &&
			ps6079TypeCanCarryReferenceChannel(pass.TypesInfo.TypeOf(selector.X), make(map[types.Type]bool)) {
			return true
		}
		for _, argument := range call.Args {
			if ps6079TypeCanCarryReferenceChannel(pass.TypesInfo.TypeOf(argument), make(map[types.Type]bool)) {
				return true
			}
		}
		return false
	}
	declaration := declarations[callee]
	if declaration == nil {
		return true
	}
	if visiting[callee] {
		return false
	}
	visiting[callee] = true
	defer delete(visiting, callee)
	return ps6079BodyMayReceiveReference(pass, declarations, declaration.Body, visiting)
}

func ps6079BodyMayReceiveReference(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	body *ast.BlockStmt,
	visiting map[*types.Func]bool,
) bool {
	receives := false
	astutil.WithStack(body, func(node ast.Node, _ []ast.Node) bool {
		if receives {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		switch value := node.(type) {
		case *ast.RangeStmt:
			receives = ps6079ReferenceChannelRange(pass, value)
		case *ast.UnaryExpr:
			receives = value.Op == token.ARROW &&
				ps6079ContainsReferenceFixture(pass.TypesInfo.TypeOf(value))
		case *ast.CallExpr:
			receives = ps6079ContainsReferenceFixture(pass.TypesInfo.TypeOf(value)) &&
				ps6079ExpressionMayReceiveReference(pass, declarations, value, visiting)
		}
		return !receives
	})
	return receives
}

func ps6079CallHasConcreteReferenceReceive(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	call *ast.CallExpr,
	visiting map[*types.Func]bool,
) bool {
	if selector, ok := ps2110Unparen(call.Fun).(*ast.SelectorExpr); ok &&
		ps6079ExpressionHasConcreteReferenceReceive(pass, declarations, selector.X, visiting) {
		return true
	}
	for _, argument := range call.Args {
		if ps6079ExpressionHasConcreteReferenceReceive(pass, declarations, argument, visiting) {
			return true
		}
	}
	if literal, ok := ps2110Unparen(call.Fun).(*ast.FuncLit); ok {
		return ps6079BodyHasConcreteReferenceReceive(pass, declarations, literal.Body, visiting)
	}
	callee, _, known := typedCallee(pass, call.Fun)
	if !known || callee == nil || callee.Pkg() == nil || pass.Pkg == nil ||
		callee.Pkg().Path() != pass.Pkg.Path() {
		if selector, ok := ps2110Unparen(call.Fun).(*ast.SelectorExpr); ok &&
			ps6079TypeCanCarryReferenceChannel(pass.TypesInfo.TypeOf(selector.X), make(map[types.Type]bool)) {
			return true
		}
		for _, argument := range call.Args {
			if ps6079TypeCanCarryReferenceChannel(pass.TypesInfo.TypeOf(argument), make(map[types.Type]bool)) {
				return true
			}
		}
		return false
	}
	if visiting[callee] {
		return true
	}
	declaration := declarations[callee]
	if declaration == nil || declaration.Body == nil {
		return true
	}
	visiting[callee] = true
	receives := ps6079BodyHasConcreteReferenceReceive(pass, declarations, declaration.Body, visiting)
	delete(visiting, callee)
	return receives
}

func ps6079BodyHasConcreteReferenceReceive(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	body *ast.BlockStmt,
	visiting map[*types.Func]bool,
) bool {
	receives := false
	astutil.WithStack(body, func(node ast.Node, _ []ast.Node) bool {
		if receives {
			return false
		}
		switch value := node.(type) {
		case *ast.RangeStmt:
			receives = ps6079ReferenceChannelRange(pass, value)
		case *ast.UnaryExpr:
			receives = value.Op == token.ARROW &&
				ps6079ContainsReferenceFixture(pass.TypesInfo.TypeOf(value))
		case *ast.CallExpr:
			receives = ps6079CallHasConcreteReferenceReceive(pass, declarations, value, visiting)
		}
		return !receives
	})
	return receives
}

func ps6079ExpressionHasConcreteReferenceReceive(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	expression ast.Expr,
	visiting map[*types.Func]bool,
) bool {
	receives := false
	ast.Inspect(expression, func(node ast.Node) bool {
		if receives {
			return false
		}
		switch value := node.(type) {
		case *ast.UnaryExpr:
			receives = value.Op == token.ARROW &&
				ps6079ContainsReferenceFixture(pass.TypesInfo.TypeOf(value))
		case *ast.CallExpr:
			receives = ps6079CallHasConcreteReferenceReceive(pass, declarations, value, visiting)
			return false
		}
		return !receives
	})
	return receives
}

func ps6079TypeCanCarryReferenceChannel(typeOf types.Type, visiting map[types.Type]bool) bool {
	if typeOf == nil || visiting[typeOf] {
		return false
	}
	visiting[typeOf] = true
	defer delete(visiting, typeOf)
	switch underlying := types.Unalias(typeOf).Underlying().(type) {
	case *types.Chan:
		return ps6079ContainsReferenceFixture(underlying.Elem())
	case *types.Array:
		return ps6079TypeCanCarryReferenceChannel(underlying.Elem(), visiting)
	case *types.Slice:
		return ps6079TypeCanCarryReferenceChannel(underlying.Elem(), visiting)
	case *types.Map:
		return ps6079TypeCanCarryReferenceChannel(underlying.Key(), visiting) ||
			ps6079TypeCanCarryReferenceChannel(underlying.Elem(), visiting)
	case *types.Pointer:
		return ps6079TypeCanCarryReferenceChannel(underlying.Elem(), visiting)
	case *types.Struct:
		for index := range underlying.NumFields() {
			if ps6079TypeCanCarryReferenceChannel(underlying.Field(index).Type(), visiting) {
				return true
			}
		}
	case *types.Tuple:
		for index := range underlying.Len() {
			if ps6079TypeCanCarryReferenceChannel(underlying.At(index).Type(), visiting) {
				return true
			}
		}
	}
	return false
}

func ps6079CallMayLaunchAsynchronousWork(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	call *ast.CallExpr,
	visiting map[*types.Func]bool,
) bool {
	if literal, ok := ps2110Unparen(call.Fun).(*ast.FuncLit); ok {
		return ps6079BodyMayLaunchAsynchronousWork(pass, declarations, literal.Body, visiting)
	}
	callee, _, known := typedCallee(pass, call.Fun)
	if !known {
		return !ps6079BuiltinOrConversion(pass, call.Fun) && !ps6079UnsafeCall(pass, call.Fun)
	}
	if ps6079HarmlessExternalRouteProofCall(callee) {
		return false
	}
	if callee.Pkg() == nil || pass.Pkg == nil || callee.Pkg().Path() != pass.Pkg.Path() {
		if ps6079ContainsReferenceFixture(pass.TypesInfo.TypeOf(call)) {
			return true
		}
		if ps6079FunctionArgument(pass, call) {
			return true
		}
		if selector, ok := ps2110Unparen(call.Fun).(*ast.SelectorExpr); ok &&
			ps6079ContainsReferenceFixture(pass.TypesInfo.TypeOf(selector.X)) {
			return true
		}
		for _, argument := range call.Args {
			if ps6079ContainsReferenceFixture(pass.TypesInfo.TypeOf(argument)) {
				return true
			}
		}
		return false
	}
	declaration := declarations[callee]
	if declaration == nil {
		return true
	}
	if visiting[callee] {
		return false
	}
	visiting[callee] = true
	defer delete(visiting, callee)
	return ps6079BodyMayLaunchAsynchronousWork(pass, declarations, declaration.Body, visiting)
}

func ps6079SynchronousSamePackageCall(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	call *ast.CallExpr,
	functionValues map[types.Object][]ast.Expr,
) bool {
	callables, resolved := ps6079SemanticCallables(pass, call.Fun, functionValues)
	if !resolved || pass.Pkg == nil {
		return false
	}
	for _, callable := range callables {
		callee, _, _ := typedCallee(pass, callable)
		if callee.Pkg() == nil || callee.Pkg().Path() != pass.Pkg.Path() {
			return false
		}
	}
	return !ps6079CallMayPersistReferenceEffectsWithValues(
		pass, declarations, call, functionValues, make(map[*types.Func]bool), make(map[*ast.FuncLit]bool),
	)
}

func ps6079CallMayPersistReferenceEffectsWithValues(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	call *ast.CallExpr,
	functionValues map[types.Object][]ast.Expr,
	visitingFunctions map[*types.Func]bool,
	visitingLiterals map[*ast.FuncLit]bool,
) bool {
	if ps6079BuiltinOrConversion(pass, call.Fun) {
		return false
	}
	callables := ps6079ResolveFunctionExpressions(
		pass, call.Fun, functionValues, make(map[types.Object]bool),
	)
	if len(callables) == 0 {
		return true
	}
	for _, callable := range callables {
		if literal, ok := ps2110Unparen(callable).(*ast.FuncLit); ok {
			if visitingLiterals[literal] {
				return true
			}
			visitingLiterals[literal] = true
			persists := ps6079BodyMayPersistReferenceEffects(
				pass, declarations, literal.Body, functionValues, visitingFunctions, visitingLiterals,
			)
			delete(visitingLiterals, literal)
			if persists {
				return true
			}
			continue
		}
		callee, _, known := typedCallee(pass, callable)
		if !known || callee == nil {
			return true
		}
		if callee.Pkg() == nil || pass.Pkg == nil || callee.Pkg().Path() != pass.Pkg.Path() {
			if ps6079ContainsReferenceFixture(pass.TypesInfo.TypeOf(call)) {
				return true
			}
			if ps6079FunctionArgument(pass, call) {
				return true
			}
			if selector, ok := ps2110Unparen(call.Fun).(*ast.SelectorExpr); ok &&
				ps6079ContainsReferenceFixture(pass.TypesInfo.TypeOf(selector.X)) {
				return true
			}
			for _, argument := range call.Args {
				if ps6079ContainsReferenceFixture(pass.TypesInfo.TypeOf(argument)) {
					return true
				}
			}
			continue
		}
		if visitingFunctions[callee] {
			return true
		}
		declaration := declarations[callee]
		if declaration == nil || declaration.Body == nil {
			return true
		}
		visitingFunctions[callee] = true
		persists := ps6079BodyMayPersistReferenceEffects(
			pass, declarations, declaration.Body, nil, visitingFunctions, visitingLiterals,
		)
		delete(visitingFunctions, callee)
		if persists {
			return true
		}
	}
	return false
}

func ps6079BodyMayPersistReferenceEffects(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	body *ast.BlockStmt,
	initialFunctionValues map[types.Object][]ast.Expr,
	visitingFunctions map[*types.Func]bool,
	visitingLiterals map[*ast.FuncLit]bool,
) bool {
	functionValues := make(map[types.Object][]ast.Expr, len(initialFunctionValues))
	for object, values := range initialFunctionValues {
		functionValues[object] = slices.Clone(values)
	}
	recordFunctionValue := func(left ast.Expr, right ast.Expr) {
		typeOf := pass.TypesInfo.TypeOf(right)
		if typeOf == nil {
			return
		}
		if _, ok := types.Unalias(typeOf).Underlying().(*types.Signature); !ok {
			return
		}
		if object := ps6079RootObject(pass, left); object != nil {
			functionValues[object] = append(functionValues[object], right)
		}
	}
	persists := false
	astutil.WithStack(body, func(node ast.Node, _ []ast.Node) bool {
		if persists {
			return false
		}
		switch value := node.(type) {
		case *ast.GoStmt, *ast.SendStmt:
			persists = true
			return false
		case *ast.AssignStmt:
			if len(value.Lhs) == len(value.Rhs) {
				for index, left := range value.Lhs {
					recordFunctionValue(left, value.Rhs[index])
				}
			} else if len(value.Rhs) == 1 {
				for _, left := range value.Lhs {
					recordFunctionValue(left, value.Rhs[0])
				}
			}
			if !ps6079AssignmentMayRetainReference(pass, value) {
				return true
			}
			for _, left := range value.Lhs {
				if ps6079PackageStorageObject(pass, ps6079RootObject(pass, left)) {
					persists = true
					return false
				}
			}
		case *ast.ValueSpec:
			if len(value.Names) == len(value.Values) {
				for index, name := range value.Names {
					recordFunctionValue(name, value.Values[index])
				}
			} else if len(value.Values) == 1 {
				for _, name := range value.Names {
					recordFunctionValue(name, value.Values[0])
				}
			}
		case *ast.CallExpr:
			persists = ps6079CallMayPersistReferenceEffectsWithValues(
				pass, declarations, value, functionValues, visitingFunctions, visitingLiterals,
			)
			return !persists
		case *ast.ReturnStmt:
			if len(value.Results) == 0 {
				persists = len(functionValues) > 0
				return !persists
			}
			for _, result := range value.Results {
				persists = persists || ps6079TypeCanCarryFunction(
					pass.TypesInfo.TypeOf(result), make(map[types.Type]bool),
				)
			}
			return !persists
		}
		return true
	})
	return persists
}

func ps6079BodyMayLaunchAsynchronousWork(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	body *ast.BlockStmt,
	visiting map[*types.Func]bool,
) bool {
	launches := false
	astutil.WithStack(body, func(node ast.Node, stack []ast.Node) bool {
		if launches {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		switch value := node.(type) {
		case *ast.GoStmt, *ast.SendStmt:
			launches = true
			return false
		case *ast.AssignStmt:
			launches = ps6079AssignmentMayRetainReference(pass, value)
			return !launches
		case *ast.ReturnStmt:
			for _, result := range value.Results {
				if ps6079TypeCanCarryFunction(pass.TypesInfo.TypeOf(result), make(map[types.Type]bool)) {
					launches = true
					return false
				}
			}
		case *ast.CallExpr:
			for _, ancestor := range stack {
				if _, asynchronous := ancestor.(*ast.GoStmt); asynchronous {
					return false
				}
			}
			launches = ps6079CallMayLaunchAsynchronousWork(pass, declarations, value, visiting)
			return !launches
		}
		return true
	})
	return launches
}

func ps6079AssignmentMayRetainReference(pass *analysis.Pass, assignment *ast.AssignStmt) bool {
	retainedValue := false
	for _, expression := range assignment.Rhs {
		typeOf := pass.TypesInfo.TypeOf(expression)
		retainedValue = retainedValue || ps6079AliasCarrierType(typeOf)
	}
	if !retainedValue {
		return false
	}
	for _, expression := range assignment.Lhs {
		identifier, direct := ps2110Unparen(expression).(*ast.Ident)
		if !direct {
			return true
		}
		object := pass.TypesInfo.ObjectOf(identifier)
		if object != nil && pass.Pkg != nil && object.Parent() == pass.Pkg.Scope() {
			return true
		}
	}
	return false
}

func ps6079FunctionReferenceCaptures(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	expression ast.Expr,
	functionValues map[types.Object][]ast.Expr,
) map[types.Object]bool {
	captures := make(map[types.Object]bool)
	seenValues := make(map[types.Object]bool)
	seenLiterals := make(map[*ast.FuncLit]bool)
	var visitExpression func(ast.Expr)
	visitExpression = func(current ast.Expr) {
		if current == nil {
			return
		}
		ast.Inspect(current, func(node ast.Node) bool {
			switch value := node.(type) {
			case *ast.FuncLit:
				if seenLiterals[value] {
					return false
				}
				seenLiterals[value] = true
				locals := make(map[types.Object]bool)
				ast.Inspect(value, func(node ast.Node) bool {
					identifier, ok := node.(*ast.Ident)
					if ok {
						locals[pass.TypesInfo.Defs[identifier]] = true
					}
					return true
				})
				ast.Inspect(value.Body, func(node ast.Node) bool {
					identifier, ok := node.(*ast.Ident)
					if !ok {
						return true
					}
					object := pass.TypesInfo.ObjectOf(identifier)
					if _, variable := object.(*types.Var); variable && !locals[object] {
						captures[object] = true
					}
					return true
				})
				return false
			case *ast.Ident:
				object := pass.TypesInfo.ObjectOf(value)
				if len(functionValues[object]) > 0 && !seenValues[object] {
					seenValues[object] = true
					for _, assigned := range functionValues[object] {
						visitExpression(assigned)
					}
					delete(seenValues, object)
					return false
				}
				typeOf := pass.TypesInfo.TypeOf(value)
				if typeOf != nil {
					_, ok := types.Unalias(typeOf).Underlying().(*types.Signature)
					if !ok {
						break
					}
					for object := range ps6079CallablePackageReferenceSources(
						pass, declarations, value, make(map[*types.Func]bool),
					) {
						captures[object] = true
					}
				}
			case *ast.SelectorExpr:
				typeOf := pass.TypesInfo.TypeOf(value)
				if typeOf != nil {
					_, ok := types.Unalias(typeOf).Underlying().(*types.Signature)
					if !ok {
						break
					}
					if object := ps6079RootObject(pass, value.X); object != nil {
						if _, variable := object.(*types.Var); variable {
							captures[object] = true
						}
					}
					for _, object := range ps6079ReferenceAliasSources(pass, value.X) {
						captures[object] = true
					}
				}
			}
			return true
		})
	}
	visitExpression(expression)
	return captures
}

func ps6079RecordBuiltinFunctionFlow(
	pass *analysis.Pass,
	call *ast.CallExpr,
	record func(ast.Expr, ast.Expr),
) {
	identifier, ok := ps2110Unparen(call.Fun).(*ast.Ident)
	if !ok {
		return
	}
	if _, builtin := pass.TypesInfo.ObjectOf(identifier).(*types.Builtin); !builtin {
		return
	}
	switch identifier.Name {
	case "copy":
		if len(call.Args) == 2 {
			record(call.Args[0], call.Args[1])
		}
	case "append":
		if len(call.Args) >= 2 {
			record(call.Args[0], call)
		}
	}
}

func ps6079AsynchronousFixtureRoots(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	call *ast.CallExpr,
	events map[types.Object][]ps6079FixtureEvent,
	functionValues map[types.Object][]ast.Expr,
	referenceAliases map[types.Object]map[types.Object]bool,
) []types.Object {
	roots := make(map[types.Object]bool)
	addExpression := func(expression ast.Expr) {
		for object := range ps6079ReferenceAliasComponentWithReturns(
			pass, declarations, expression, referenceAliases,
		) {
			roots[object] = true
		}
	}
	if selector, ok := ps2110Unparen(call.Fun).(*ast.SelectorExpr); ok {
		addExpression(selector.X)
		callee, _, known := typedCallee(pass, call.Fun)
		if (!known || !ps6079HarmlessExternalRouteProofCall(callee)) &&
			ps6079TypeCanCarryFunction(pass.TypesInfo.TypeOf(selector.X), make(map[types.Type]bool)) {
			for object := range events {
				roots[object] = true
			}
		}
	}
	for _, argument := range call.Args {
		addExpression(argument)
		if ps6079TypeCanCarryFunction(pass.TypesInfo.TypeOf(argument), make(map[types.Type]bool)) {
			for object := range ps6079FunctionReferenceCaptures(
				pass, declarations, argument, functionValues,
			) {
				roots[object] = true
			}
			for object := range events {
				roots[object] = true
			}
		}
	}
	for object := range ps6079FunctionReferenceCaptures(pass, declarations, call.Fun, functionValues) {
		roots[object] = true
	}
	if callee, _, resolved := typedCallee(pass, call.Fun); resolved && callee.Pkg() != nil &&
		pass.Pkg != nil && callee.Pkg().Path() == pass.Pkg.Path() {
		for object := range ps6079CallablePackageReferenceSources(
			pass, declarations, call.Fun, make(map[*types.Func]bool),
		) {
			roots[object] = true
		}
	} else if !resolved && !ps6079BuiltinOrConversion(pass, call.Fun) {
		for object := range events {
			roots[object] = true
		}
	}
	result := make([]types.Object, 0, len(roots))
	for object := range roots {
		result = append(result, object)
	}
	return result
}

func ps6079UpdateReferenceAliasComponent(
	destination types.Object,
	component map[types.Object]bool,
	conditional bool,
	aliases map[types.Object]map[types.Object]bool,
) {
	if destination == nil {
		return
	}
	if len(component) == 1 && component[destination] {
		return
	}
	if !conditional {
		for other := range aliases[destination] {
			delete(aliases[other], destination)
		}
		delete(aliases, destination)
	}
	if len(component) == 0 {
		return
	}
	for other := range component {
		if other == destination {
			continue
		}
		if aliases[destination] == nil {
			aliases[destination] = make(map[types.Object]bool)
		}
		if aliases[other] == nil {
			aliases[other] = make(map[types.Object]bool)
		}
		aliases[destination][other] = true
		aliases[other][destination] = true
	}
}

func ps6079MarkPartialReferenceAlias(
	destination types.Object,
	partial bool,
	conditional bool,
	aliases map[types.Object]bool,
) {
	if destination == nil || !ps6079ContainsReferenceFixture(destination.Type()) {
		return
	}
	if conditional {
		aliases[destination] = aliases[destination] || partial
		return
	}
	aliases[destination] = partial
}

func ps6079UpdatePartialReferenceAlias(
	pass *analysis.Pass,
	destination types.Object,
	expression ast.Expr,
	conditional bool,
	aliases map[types.Object]bool,
) {
	ps6079MarkPartialReferenceAlias(
		destination, conditional || ps6079PartialReferenceExpression(pass, expression, aliases), conditional, aliases,
	)
}

func ps6079PartialReferenceExpression(
	pass *analysis.Pass,
	expression ast.Expr,
	aliases map[types.Object]bool,
) bool {
	expression = ps2110Unparen(expression)
	switch value := expression.(type) {
	case *ast.Ident:
		return aliases[pass.TypesInfo.ObjectOf(value)]
	case *ast.SliceExpr:
		return value.Low != nil || value.High != nil || value.Max != nil ||
			ps6079PartialReferenceExpression(pass, value.X, aliases)
	case *ast.IndexExpr:
		return true
	case *ast.CompositeLit:
		for _, element := range value.Elts {
			if ps6079PartialReferenceExpression(pass, element, aliases) {
				return true
			}
		}
		return false
	case *ast.KeyValueExpr:
		return ps6079PartialReferenceExpression(pass, value.Key, aliases) ||
			ps6079PartialReferenceExpression(pass, value.Value, aliases)
	case *ast.SelectorExpr:
		return ps6079ContainsReferenceFixture(pass.TypesInfo.TypeOf(value)) ||
			ps6079PartialReferenceExpression(pass, value.X, aliases)
	case *ast.TypeAssertExpr:
		return ps6079PartialReferenceExpression(pass, value.X, aliases)
	case *ast.UnaryExpr:
		return value.Op == token.AND && ps6079PartialReferenceExpression(pass, value.X, aliases)
	case *ast.CallExpr:
		if len(value.Args) == 1 && ps6079ConversionPreservesReference(pass, value) {
			return ps6079PartialReferenceExpression(pass, value.Args[0], aliases)
		}
		return len(ps6079ReferenceAliasSources(pass, value)) > 0
	default:
		return false
	}
}

func ps6079ReferenceAliasComponent(
	pass *analysis.Pass,
	expression ast.Expr,
	aliases map[types.Object]map[types.Object]bool,
) map[types.Object]bool {
	component := make(map[types.Object]bool)
	carrierResult := ps6079AliasCarrierResultType(pass, expression)
	var queue []types.Object
	if carrierResult {
		queue = ps6079ReferenceAliasSources(pass, expression)
	}
	queue = append(queue, ps6079ReferenceAliasCarrierSources(pass, expression, aliases)...)
	if !carrierResult && len(queue) == 0 {
		return component
	}
	if root := ps6079RootObject(pass, expression); carrierResult && root != nil && len(aliases[root]) > 0 {
		queue = append(queue, root)
	}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if current == nil || component[current] {
			continue
		}
		component[current] = true
		for other := range aliases[current] {
			queue = append(queue, other)
		}
	}
	return component
}

func ps6079AliasCarrierResultType(pass *analysis.Pass, expression ast.Expr) bool {
	return ps6079AliasCarrierType(pass.TypesInfo.TypeOf(ps2110Unparen(expression)))
}

func ps6079AliasCarrierType(typeOf types.Type) bool {
	return ps6079ContainsReferenceFixture(typeOf) ||
		ps6079TypeCanCarryUnsafeAlias(typeOf, make(map[types.Type]bool)) ||
		ps6079TypeCanCarryFunction(typeOf, make(map[types.Type]bool))
}

func ps6079UnsafeDerivedSourceComponent(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	expression ast.Expr,
	functionValues map[types.Object][]ast.Expr,
	aliases map[types.Object]map[types.Object]bool,
) map[types.Object]bool {
	component := ps6079ReferenceAliasTargetComponentWithReturns(
		pass, declarations, expression, aliases,
	)
	call, ok := ps2110Unparen(expression).(*ast.CallExpr)
	if !ok {
		return component
	}
	for _, argument := range call.Args {
		for object := range ps6079ReferenceAliasComponentWithReturns(
			pass, declarations, argument, aliases,
		) {
			component[object] = true
		}
	}
	if selector, selected := ps2110Unparen(call.Fun).(*ast.SelectorExpr); selected {
		for object := range ps6079ReferenceAliasComponentWithReturns(
			pass, declarations, selector.X, aliases,
		) {
			component[object] = true
		}
	}
	for object := range ps6079FunctionReferenceCaptures(
		pass, declarations, call.Fun, functionValues,
	) {
		component[object] = true
	}
	callables, complete := ps6079ImmutableCallables(pass, call.Fun, functionValues)
	opaque := !complete || len(callables) == 0
	dynamic := false
	namedCallable := false
	explicitProvenance := ps6079BuiltinOrConversion(pass, call.Fun) || ps6079UnsafeCall(pass, call.Fun)
	for _, callable := range callables {
		for object := range ps6079CallablePackageReferenceSources(
			pass, declarations, callable, make(map[*types.Func]bool),
		) {
			component[object] = true
		}
		callee, _, known := typedCallee(pass, callable)
		if _, literal := ps2110Unparen(callable).(*ast.FuncLit); !literal {
			namedCallable = true
		}
		opaque = opaque || !known || callee == nil
		dynamic = dynamic || known && callee != nil && ps6079DynamicDispatch(pass, callable, callee)
	}
	hasStorageSource := false
	for object := range component {
		hasStorageSource = hasStorageSource || ps6079ContainsReferenceFixture(object.Type()) ||
			ps6079TypeCanCarryUnsafeAlias(object.Type(), make(map[types.Type]bool))
	}
	if !explicitProvenance && (dynamic || (!hasStorageSource && (opaque || namedCallable))) {
		for object := range ps6079AllPackageStorage(pass) {
			component[object] = true
		}
	}
	return component
}

func ps6079UnsafeDerivedExpression(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	expression ast.Expr,
	functionValues map[types.Object][]ast.Expr,
	aliases map[types.Object]map[types.Object]bool,
) bool {
	if call, ok := ps2110Unparen(expression).(*ast.CallExpr); ok &&
		(ps6079UnsafeFunction(pass, call.Fun, "Sizeof") ||
			ps6079UnsafeFunction(pass, call.Fun, "Alignof") ||
			ps6079UnsafeFunction(pass, call.Fun, "Offsetof")) {
		return false
	}
	typeOf := pass.TypesInfo.TypeOf(ps2110Unparen(expression))
	if ps6079TypeContainsUnsafeBits(typeOf, make(map[types.Type]bool)) {
		return true
	}
	nestedUnsafe := false
	ast.Inspect(expression, func(node ast.Node) bool {
		if nestedUnsafe {
			return false
		}
		child, ok := node.(ast.Expr)
		if ok && child != expression && ps6079TypeContainsUnsafeBits(
			pass.TypesInfo.TypeOf(child), make(map[types.Type]bool),
		) {
			nestedUnsafe = true
			return false
		}
		return true
	})
	if nestedUnsafe {
		return true
	}
	if call, ok := ps2110Unparen(expression).(*ast.CallExpr); ok &&
		ps6079CallBodyMayProduceUnsafeDerivedValue(
			pass, declarations, call, functionValues, make(map[*types.Func]bool),
		) {
		return true
	}
	sources := ps6079ReferenceAliasCarrierSources(pass, expression, aliases)
	if len(sources) == 0 {
		return false
	}
	if ps6079IntegerType(typeOf) {
		return true
	}
	call, conversion := ps2110Unparen(expression).(*ast.CallExpr)
	return conversion && ps6079TypeConversion(pass, call.Fun)
}

func ps6079CallBodyMayProduceUnsafeDerivedValue(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	call *ast.CallExpr,
	functionValues map[types.Object][]ast.Expr,
	visiting map[*types.Func]bool,
) bool {
	if ps6079BuiltinOrConversion(pass, call.Fun) && !ps6079UnsafeAliasCall(pass, call.Fun) {
		return false
	}
	callables, complete := ps6079ImmutableCallables(pass, call.Fun, functionValues)
	if !complete || len(callables) == 0 {
		return ps6079OpaqueCallMayReturnUnsafeDerivedValue(pass, call)
	}
	bodyMayProduce := func(body *ast.BlockStmt) bool {
		unsafeDerived := false
		bodyFunctionValues := ps6079PackageFunctionValues(pass)
		for object, values := range ps6079ImmutableFunctionValues(pass, body) {
			bodyFunctionValues[object] = values
		}
		ast.Inspect(body, func(node ast.Node) bool {
			if unsafeDerived {
				return false
			}
			expression, ok := node.(ast.Expr)
			if ok && ps6079TypeContainsUnsafeBits(
				pass.TypesInfo.TypeOf(expression), make(map[types.Type]bool),
			) {
				unsafeDerived = true
				return false
			}
			nestedCall, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			if ps6079UnsafeAliasCall(pass, nestedCall.Fun) ||
				ps6079CallBodyMayProduceUnsafeDerivedValue(
					pass, declarations, nestedCall, bodyFunctionValues, visiting,
				) {
				unsafeDerived = true
				return false
			}
			return true
		})
		return unsafeDerived
	}
	for _, callable := range callables {
		if literal, ok := ps2110Unparen(callable).(*ast.FuncLit); ok {
			if bodyMayProduce(literal.Body) {
				return true
			}
			continue
		}
		callee, _, known := typedCallee(pass, callable)
		if !known || callee == nil || ps6079DynamicDispatch(pass, callable, callee) ||
			callee.Pkg() == nil || pass.Pkg == nil ||
			callee.Pkg().Path() != pass.Pkg.Path() {
			if ps6079OpaqueCallMayReturnUnsafeDerivedValue(pass, call) {
				return true
			}
			continue
		}
		if visiting[callee] {
			return true
		}
		declaration := declarations[callee]
		if declaration == nil || declaration.Body == nil {
			return ps6079OpaqueCallMayReturnUnsafeDerivedValue(pass, call)
		}
		visiting[callee] = true
		unsafeDerived := bodyMayProduce(declaration.Body)
		delete(visiting, callee)
		if unsafeDerived {
			return true
		}
	}
	return false
}

func ps6079OpaqueCallMayReturnUnsafeDerivedValue(pass *analysis.Pass, call *ast.CallExpr) bool {
	typeOf := pass.TypesInfo.TypeOf(call)
	if typeOf == nil {
		return false
	}
	if !ps6079OpaqueUnsafeResultType(typeOf, make(map[types.Type]bool)) {
		return false
	}
	for _, argument := range call.Args {
		if ps6079AliasCarrierType(pass.TypesInfo.TypeOf(argument)) {
			return true
		}
	}
	if selector, ok := ps2110Unparen(call.Fun).(*ast.SelectorExpr); ok {
		return ps6079AliasCarrierType(pass.TypesInfo.TypeOf(selector.X))
	}
	return false
}

func ps6079OpaqueUnsafeResultType(typeOf types.Type, visiting map[types.Type]bool) bool {
	if typeOf == nil {
		return false
	}
	typeOf = types.Unalias(typeOf)
	if visiting[typeOf] {
		return false
	}
	visiting[typeOf] = true
	defer delete(visiting, typeOf)
	switch underlying := typeOf.Underlying().(type) {
	case *types.Basic:
		return underlying.Info()&(types.IsInteger|types.IsFloat|types.IsString) != 0
	case *types.Interface:
		return true
	case *types.Array:
		return ps6079OpaqueUnsafeResultType(underlying.Elem(), visiting)
	case *types.Struct:
		for index := range underlying.NumFields() {
			if ps6079OpaqueUnsafeResultType(underlying.Field(index).Type(), visiting) {
				return true
			}
		}
	case *types.Tuple:
		for index := range underlying.Len() {
			if ps6079OpaqueUnsafeResultType(underlying.At(index).Type(), visiting) {
				return true
			}
		}
	}
	return false
}

func ps6079TypeContainsUnsafeBits(typeOf types.Type, visiting map[types.Type]bool) bool {
	if typeOf == nil {
		return false
	}
	typeOf = types.Unalias(typeOf)
	if visiting[typeOf] {
		return false
	}
	visiting[typeOf] = true
	defer delete(visiting, typeOf)
	switch underlying := typeOf.Underlying().(type) {
	case *types.Basic:
		return underlying.Kind() == types.UnsafePointer || underlying.Kind() == types.Uintptr
	case *types.Pointer:
		return ps6079TypeContainsUnsafeBits(underlying.Elem(), visiting)
	case *types.Array:
		return ps6079TypeContainsUnsafeBits(underlying.Elem(), visiting)
	case *types.Slice:
		return ps6079TypeContainsUnsafeBits(underlying.Elem(), visiting)
	case *types.Map:
		return ps6079TypeContainsUnsafeBits(underlying.Key(), visiting) ||
			ps6079TypeContainsUnsafeBits(underlying.Elem(), visiting)
	case *types.Struct:
		for index := range underlying.NumFields() {
			if ps6079TypeContainsUnsafeBits(underlying.Field(index).Type(), visiting) {
				return true
			}
		}
	case *types.Tuple:
		for index := range underlying.Len() {
			if ps6079TypeContainsUnsafeBits(underlying.At(index).Type(), visiting) {
				return true
			}
		}
	}
	return false
}

func ps6079ReferenceAliasCarrierSources(
	pass *analysis.Pass,
	expression ast.Expr,
	aliases map[types.Object]map[types.Object]bool,
) []types.Object {
	seen := make(map[types.Object]bool)
	var visit func(ast.Expr)
	visit = func(expression ast.Expr) {
		expression = ps2110Unparen(expression)
		switch value := expression.(type) {
		case *ast.Ident:
			if object := pass.TypesInfo.ObjectOf(value); object != nil && len(aliases[object]) > 0 {
				seen[object] = true
			}
		case *ast.BinaryExpr:
			if ps6079IntegerType(pass.TypesInfo.TypeOf(value)) {
				visit(value.X)
				visit(value.Y)
			}
		case *ast.UnaryExpr:
			if ps6079IntegerType(pass.TypesInfo.TypeOf(value)) {
				visit(value.X)
			}
		case *ast.CallExpr:
			carrierResult := ps6079AliasCarrierType(pass.TypesInfo.TypeOf(value))
			if ps6079UnsafeFunction(pass, value.Fun, "String") && len(value.Args) > 0 {
				for _, object := range ps6079ReferenceAliasSources(pass, value.Args[0]) {
					seen[object] = true
				}
				visit(value.Args[0])
			} else if len(value.Args) == 1 && (ps6079TypeConversion(pass, value.Fun) ||
				ps6079ConversionPreservesReference(pass, value)) {
				for _, object := range ps6079ReferenceAliasSources(pass, value.Args[0]) {
					seen[object] = true
				}
				visit(value.Args[0])
			} else if carrierResult {
				for _, argument := range value.Args {
					visit(argument)
				}
				if selector, ok := ps2110Unparen(value.Fun).(*ast.SelectorExpr); ok {
					visit(selector.X)
				}
			}
			if carrierResult {
				visit(value.Fun)
			}
		case *ast.FuncLit:
			ast.Inspect(value.Body, func(node ast.Node) bool {
				if nested, ok := node.(*ast.FuncLit); ok && nested != value {
					return false
				}
				expression, ok := node.(ast.Expr)
				if !ok {
					return true
				}
				for _, object := range ps6079ReferenceAliasSources(pass, expression) {
					seen[object] = true
				}
				if identifier, ok := expression.(*ast.Ident); ok {
					if object := pass.TypesInfo.ObjectOf(identifier); object != nil && len(aliases[object]) > 0 {
						seen[object] = true
					}
				}
				return true
			})
		case *ast.CompositeLit:
			for _, element := range value.Elts {
				if keyed, ok := element.(*ast.KeyValueExpr); ok {
					visit(keyed.Value)
					continue
				}
				visit(element)
			}
		case *ast.KeyValueExpr:
			visit(value.Value)
		case *ast.SelectorExpr:
			if ps6079AliasCarrierResultType(pass, value) {
				if root := ps6079RootObject(pass, value); root != nil && len(aliases[root]) > 0 {
					seen[root] = true
				}
			}
		case *ast.IndexExpr:
			if root := ps6079RootObject(pass, value.X); root != nil && len(aliases[root]) > 0 {
				seen[root] = true
			}
		case *ast.SliceExpr:
			visit(value.X)
		case *ast.TypeAssertExpr:
			if ps6079AliasCarrierResultType(pass, value) {
				visit(value.X)
			}
		case *ast.StarExpr:
			visit(value.X)
		}
	}
	visit(expression)
	result := make([]types.Object, 0, len(seen))
	for object := range seen {
		result = append(result, object)
	}
	return result
}

func ps6079UnsafeFunction(pass *analysis.Pass, expression ast.Expr, name string) bool {
	selector, ok := ps2110Unparen(expression).(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != name {
		return false
	}
	return ps6079UnsafeSelector(pass, selector)
}

func ps6079UnsafeCall(pass *analysis.Pass, expression ast.Expr) bool {
	selector, ok := ps2110Unparen(expression).(*ast.SelectorExpr)
	return ok && ps6079UnsafeSelector(pass, selector)
}

func ps6079UnsafeAliasCall(pass *analysis.Pass, expression ast.Expr) bool {
	selector, ok := ps2110Unparen(expression).(*ast.SelectorExpr)
	if !ok || !ps6079UnsafeSelector(pass, selector) {
		return false
	}
	switch selector.Sel.Name {
	case "Add", "Pointer", "Slice", "SliceData", "String", "StringData":
		return true
	default:
		return false
	}
}

func ps6079UnsafeSelector(pass *analysis.Pass, selector *ast.SelectorExpr) bool {
	identifier, ok := ps2110Unparen(selector.X).(*ast.Ident)
	if !ok {
		return false
	}
	packageName, ok := pass.TypesInfo.ObjectOf(identifier).(*types.PkgName)
	return ok && packageName.Imported() != nil && packageName.Imported().Path() == "unsafe"
}

func ps6079LinkCallReferenceAliases(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	call *ast.CallExpr,
	functionValues map[types.Object][]ast.Expr,
	aliases map[types.Object]map[types.Object]bool,
	markEscape func(types.Object),
) {
	if typedBuiltinName(pass, call.Fun, "copy") && len(call.Args) >= 2 &&
		ps6079TypeCanCarryUnsafeAlias(ps6079SliceElementType(pass.TypesInfo.TypeOf(call.Args[0])),
			make(map[types.Type]bool)) {
		for object := range ps6079ReferenceAliasTargetComponentWithReturns(
			pass, declarations, call.Args[0], aliases,
		) {
			markEscape(object)
		}
		for object := range ps6079ReferenceAliasComponentWithReturns(pass, declarations, call.Args[1], aliases) {
			markEscape(object)
		}
		return
	}
	if ps6079BuiltinOrConversion(pass, call.Fun) {
		return
	}
	callables, complete := ps6079SemanticCallables(pass, call.Fun, functionValues)
	if selector, ok := ps2110Unparen(call.Fun).(*ast.SelectorExpr); ok &&
		ps6079TypeCanCarryUnsafeAlias(pass.TypesInfo.TypeOf(selector.X), make(map[types.Type]bool)) {
		receiverComponent := ps6079ReferenceAliasTargetComponentWithReturns(
			pass, declarations, selector.X, aliases,
		)
		argumentComponent := make(map[types.Object]bool)
		for _, argument := range call.Args {
			for object := range ps6079ReferenceAliasComponentWithReturns(pass, declarations, argument, aliases) {
				argumentComponent[object] = true
			}
		}
		if len(argumentComponent) > 0 && ps6079MethodMayRetainUnsafeAlias(
			pass, declarations, callables, complete,
		) {
			for object := range receiverComponent {
				markEscape(object)
			}
			for object := range argumentComponent {
				markEscape(object)
			}
		}
	}
	for index, argument := range call.Args {
		pointer, ok := types.Unalias(pass.TypesInfo.TypeOf(argument)).Underlying().(*types.Pointer)
		if !ok || !ps6079TypeCanCarryUnsafeAlias(pointer.Elem(), make(map[types.Type]bool)) {
			continue
		}
		destinations := ps6079ReferenceAliasTargetComponentWithReturns(
			pass, declarations, argument, aliases,
		)
		if len(destinations) == 0 {
			continue
		}
		mayMutate := !complete
		for _, callable := range callables {
			callee, _, known := typedCallee(pass, callable)
			if !known || callee == nil || callee.Pkg() == nil || pass.Pkg == nil ||
				callee.Pkg().Path() != pass.Pkg.Path() || declarations[callee] == nil {
				mayMutate = true
				continue
			}
			signature, ok := callee.Type().(*types.Signature)
			if !ok || index >= signature.Params().Len() ||
				ps6079ParameterMayMutate(pass, declarations, callee, index, make(map[ps6079ParameterKey]bool)) {
				mayMutate = true
			}
		}
		if !mayMutate {
			continue
		}
		if sourceIndex, exact := ps6079SimpleOutParameterSource(
			pass, declarations, callables, complete, index,
		); exact && sourceIndex >= 0 && sourceIndex < len(call.Args) {
			component := ps6079ReferenceAliasComponentWithReturns(
				pass, declarations, call.Args[sourceIndex], aliases,
			)
			for destination := range destinations {
				delete(component, destination)
			}
			for destination := range destinations {
				ps6079UpdateReferenceAliasComponent(destination, component, true, aliases)
			}
			continue
		}
		for destination := range destinations {
			markEscape(destination)
		}
		for otherIndex, other := range call.Args {
			if otherIndex == index {
				continue
			}
			for object := range ps6079ReferenceAliasComponentWithReturns(pass, declarations, other, aliases) {
				markEscape(object)
			}
		}
	}
}

func ps6079MethodMayRetainUnsafeAlias(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	callables []ast.Expr,
	complete bool,
) bool {
	if !complete || len(callables) == 0 {
		return true
	}
	for _, callable := range callables {
		callee, _, known := typedCallee(pass, callable)
		if !known || callee == nil || callee.Pkg() == nil || pass.Pkg == nil ||
			callee.Pkg().Path() != pass.Pkg.Path() {
			return true
		}
		declaration := declarations[callee]
		if declaration == nil || declaration.Body == nil {
			return true
		}
		receiver := ps6079FunctionReceiverObject(pass, declaration)
		retains := false
		ast.Inspect(declaration.Body, func(node ast.Node) bool {
			if retains {
				return false
			}
			switch value := node.(type) {
			case *ast.AssignStmt:
				for _, left := range value.Lhs {
					if ps6079RootObject(pass, left) == receiver {
						retains = true
						return false
					}
				}
			case *ast.GoStmt, *ast.SendStmt, *ast.CallExpr:
				retains = true
				return false
			}
			return true
		})
		if retains {
			return true
		}
	}
	return false
}

func ps6079SimpleOutParameterSource(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	callables []ast.Expr,
	complete bool,
	destination int,
) (int, bool) {
	if !complete || len(callables) == 0 {
		return -1, false
	}
	commonSource := -1
	for _, callable := range callables {
		callee, _, known := typedCallee(pass, callable)
		if !known || callee == nil {
			return -1, false
		}
		declaration := declarations[callee]
		signature, signatureOK := callee.Type().(*types.Signature)
		if declaration == nil || declaration.Body == nil || !signatureOK ||
			destination >= signature.Params().Len() || len(declaration.Body.List) != 1 {
			return -1, false
		}
		assignment, ok := declaration.Body.List[0].(*ast.AssignStmt)
		if !ok || assignment.Tok != token.ASSIGN || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 ||
			ps6079RootObject(pass, assignment.Lhs[0]) != signature.Params().At(destination) {
			return -1, false
		}
		sourceObject := ps6079RootObject(pass, assignment.Rhs[0])
		source := -1
		for index := range signature.Params().Len() {
			if signature.Params().At(index) == sourceObject {
				source = index
				break
			}
		}
		if source < 0 || source == destination || commonSource >= 0 && commonSource != source {
			return -1, false
		}
		commonSource = source
	}
	return commonSource, commonSource >= 0
}

func ps6079SliceElementType(typeOf types.Type) types.Type {
	if typeOf == nil {
		return nil
	}
	slice, ok := types.Unalias(typeOf).Underlying().(*types.Slice)
	if !ok {
		return nil
	}
	return slice.Elem()
}

func ps6079TypeCanCarryUnsafeAlias(typeOf types.Type, visiting map[types.Type]bool) bool {
	if typeOf == nil {
		return false
	}
	typeOf = types.Unalias(typeOf)
	if visiting[typeOf] {
		return false
	}
	visiting[typeOf] = true
	defer delete(visiting, typeOf)
	switch underlying := typeOf.Underlying().(type) {
	case *types.Basic:
		return underlying.Kind() == types.UnsafePointer || underlying.Kind() == types.Uintptr
	case *types.Pointer:
		return true
	case *types.Interface:
		return true
	case *types.Array, *types.Slice:
		var element types.Type
		if array, ok := underlying.(*types.Array); ok {
			element = array.Elem()
		} else {
			element = underlying.(*types.Slice).Elem()
		}
		return ps6079TypeCanCarryUnsafeAlias(element, visiting)
	case *types.Map:
		return ps6079TypeCanCarryUnsafeAlias(underlying.Key(), visiting) ||
			ps6079TypeCanCarryUnsafeAlias(underlying.Elem(), visiting)
	case *types.Struct:
		for index := range underlying.NumFields() {
			if ps6079TypeCanCarryUnsafeAlias(underlying.Field(index).Type(), visiting) {
				return true
			}
		}
	case *types.Tuple:
		for index := range underlying.Len() {
			if ps6079TypeCanCarryUnsafeAlias(underlying.At(index).Type(), visiting) {
				return true
			}
		}
	}
	return false
}

func ps6079PackageFunctionValues(pass *analysis.Pass) map[types.Object][]ast.Expr {
	functionValues := make(map[types.Object][]ast.Expr)
	declarations := make(map[*types.Func]*ast.FuncDecl)
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok {
				continue
			}
			if object, ok := pass.TypesInfo.Defs[function.Name].(*types.Func); ok {
				declarations[object] = function
			}
		}
	}
	record := func(left ast.Expr, right ast.Expr) {
		typeOf := pass.TypesInfo.TypeOf(right)
		if !ps6079TypeCanCarryFunction(typeOf, make(map[types.Type]bool)) {
			return
		}
		if object := ps6079RootObject(pass, left); object != nil {
			functionValues[object] = append(functionValues[object], right)
		}
	}
	visitingFunctions := make(map[*types.Func]bool)
	visitingLiterals := make(map[*ast.FuncLit]bool)
	var scanBody func(*ast.BlockStmt)
	scanCall := func(call *ast.CallExpr) {
		for _, callable := range ps6079ResolveFunctionExpressions(
			pass, call.Fun, functionValues, make(map[types.Object]bool),
		) {
			if literal, ok := ps2110Unparen(callable).(*ast.FuncLit); ok {
				if !visitingLiterals[literal] {
					visitingLiterals[literal] = true
					scanBody(literal.Body)
					delete(visitingLiterals, literal)
				}
				continue
			}
			callee, _, known := typedCallee(pass, callable)
			if !known || callee == nil || visitingFunctions[callee] {
				continue
			}
			declaration := declarations[callee]
			if declaration == nil || declaration.Body == nil {
				continue
			}
			visitingFunctions[callee] = true
			scanBody(declaration.Body)
			delete(visitingFunctions, callee)
		}
	}
	scanBody = func(body *ast.BlockStmt) {
		astutil.WithStack(body, func(node ast.Node, _ []ast.Node) bool {
			if _, nested := node.(*ast.FuncLit); nested {
				return false
			}
			switch value := node.(type) {
			case *ast.AssignStmt:
				if len(value.Lhs) == len(value.Rhs) {
					for index, left := range value.Lhs {
						record(left, value.Rhs[index])
					}
				} else if len(value.Rhs) == 1 {
					for _, left := range value.Lhs {
						record(left, value.Rhs[0])
					}
				}
			case *ast.ValueSpec:
				if len(value.Names) == len(value.Values) {
					for index, name := range value.Names {
						record(name, value.Values[index])
					}
				} else if len(value.Values) == 1 {
					for _, name := range value.Names {
						record(name, value.Values[0])
					}
				}
			case *ast.RangeStmt:
				for _, target := range []ast.Expr{value.Key, value.Value} {
					if target != nil {
						record(target, value.X)
					}
				}
			case *ast.CallExpr:
				ps6079RecordBuiltinFunctionFlow(pass, value, record)
				scanCall(value)
				if _, literal := ps2110Unparen(value.Fun).(*ast.FuncLit); literal {
					return false
				}
			}
			return true
		})
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
				if len(value.Names) == len(value.Values) {
					for index, name := range value.Names {
						record(name, value.Values[index])
					}
				}
			}
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
				for _, expression := range value.Values {
					ast.Inspect(expression, func(node ast.Node) bool {
						if _, literal := node.(*ast.FuncLit); literal {
							return false
						}
						call, ok := node.(*ast.CallExpr)
						if !ok {
							return true
						}
						scanCall(call)
						if _, literal := ps2110Unparen(call.Fun).(*ast.FuncLit); literal {
							return false
						}
						return true
					})
				}
			}
		}
	}
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Name.Name != "init" || function.Body == nil {
				continue
			}
			if object, ok := pass.TypesInfo.Defs[function.Name].(*types.Func); ok {
				visitingFunctions[object] = true
			}
			scanBody(function.Body)
		}
	}
	return functionValues
}

func ps6079SeedPackageFixtureEvents(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	events map[types.Object][]ps6079FixtureEvent,
	functionValues map[types.Object][]ast.Expr,
	referenceAliases map[types.Object]map[types.Object]bool,
	initializationInvalidations map[types.Object]bool,
	asynchronousEscapes map[types.Object]bool,
) {
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok || general.Tok != token.VAR {
				continue
			}
			for _, specification := range general.Specs {
				value, ok := specification.(*ast.ValueSpec)
				if !ok || len(value.Names) != len(value.Values) {
					continue
				}
				for index, name := range value.Names {
					object := pass.TypesInfo.Defs[name]
					if name.Name == "_" || !ps6079PackageStorageObject(pass, object) {
						continue
					}
					stable := ps6079PackageFixtureSeedStable(pass, declarations, object, referenceAliases)
					if asynchronousEscapes[object] || initializationInvalidations[object] || !stable {
						continue
					}
					expression := value.Values[index]
					state, event := ps6079AssignedFixtureExpression(
						pass, declarations, expression, events, functionValues, expression.End()+1,
					)
					source := "package initializer " + exprTextRendered(expression)
					if event.source != "" {
						source = "package initializer " + event.source
					}
					events[object] = append(events[object], ps6079FixtureEvent{
						position: token.NoPos,
						state:    state,
						source:   source,
					})
				}
			}
		}
	}
}

func ps6079PackageFixtureSeedStable(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	object types.Object,
	referenceAliases map[types.Object]map[types.Object]bool,
) bool {
	if object == nil {
		return false
	}
	component := make(map[types.Object]bool)
	queue := []types.Object{object}
	for len(queue) > 0 {
		candidate := queue[0]
		queue = queue[1:]
		if candidate == nil || component[candidate] {
			continue
		}
		component[candidate] = true
		for alias := range referenceAliases[candidate] {
			queue = append(queue, alias)
		}
	}
	for candidate := range component {
		if candidate.Exported() {
			return false
		}
	}
	for _, file := range pass.Files {
		for _, packageDeclaration := range file.Decls {
			switch value := packageDeclaration.(type) {
			case *ast.FuncDecl:
				if value.Body == nil {
					return false
				}
			case *ast.GenDecl:
				if value.Tok != token.VAR {
					continue
				}
				for _, specification := range value.Specs {
					values, ok := specification.(*ast.ValueSpec)
					if !ok {
						continue
					}
					for index, expression := range values.Values {
						ownInitializer := len(values.Names) == len(values.Values) &&
							pass.TypesInfo.Defs[values.Names[index]] == object
						if !ownInitializer && ps6079NodeUsesAlias(pass, expression, component) {
							return false
						}
					}
				}
			}
		}
	}
	for _, declaration := range declarations {
		if declaration == nil || declaration.Body == nil {
			continue
		}
		aliases := make(map[types.Object]bool, len(component))
		for candidate := range component {
			aliases[candidate] = true
		}
		unstable := false
		astutil.WithStack(declaration.Body, func(node ast.Node, stack []ast.Node) bool {
			if unstable {
				return false
			}
			if literal, nested := node.(*ast.FuncLit); nested {
				unstable = ps6079NodeUsesAlias(pass, literal.Body, aliases)
				return false
			}
			switch value := node.(type) {
			case *ast.AssignStmt:
				for _, left := range value.Lhs {
					identifier, direct := ps2110Unparen(left).(*ast.Ident)
					if direct && aliases[pass.TypesInfo.ObjectOf(identifier)] {
						unstable = true
						return false
					}
				}
				unstable = ps6079AssignmentMutatesAlias(
					pass, value, ps6079ConditionalAliasAssignment(value, stack), aliases,
				)
			case *ast.ValueSpec:
				unstable = ps6079ValueSpecMutatesAlias(pass, value, aliases)
			case *ast.RangeStmt:
				for _, left := range []ast.Expr{value.Key, value.Value} {
					identifier, direct := ps2110Unparen(left).(*ast.Ident)
					if direct && aliases[pass.TypesInfo.ObjectOf(identifier)] {
						unstable = true
						return false
					}
				}
				ps6079RangeAliases(pass, value, aliases)
			case *ast.IncDecStmt:
				unstable = aliases[ps6079RootObject(pass, value.X)] ||
					ps6079WritesThroughAlias(pass, value.X, aliases)
			case *ast.SendStmt:
				unstable = ps6079NodeUsesAlias(pass, value.Value, aliases)
			case *ast.UnaryExpr:
				unstable = value.Op == token.AND && ps6079NodeUsesAlias(pass, value.X, aliases)
			case *ast.ReturnStmt:
				for _, result := range value.Results {
					unstable = unstable || ps6079NodeUsesAlias(pass, result, aliases)
				}
			case *ast.CallExpr:
				unstable = ps6079CallMayMutateAlias(
					pass, declarations, value, aliases, make(map[ps6079ParameterKey]bool),
				)
			}
			return !unstable
		})
		if unstable {
			return false
		}
	}
	return true
}

func ps6079BootstrapPackageReferenceAliases(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	aliases map[types.Object]map[types.Object]bool,
	asynchronousEscapes map[types.Object]bool,
	initializationInvalidations map[types.Object]bool,
) {
	packageFunctionValues := ps6079PackageFunctionValues(pass)
	componentFor := func(expression ast.Expr) map[types.Object]bool {
		component := ps6079ReferenceAliasComponentWithReturns(pass, declarations, expression, aliases)
		root := ps6079RootObject(pass, expression)
		if receive, ok := ps2110Unparen(expression).(*ast.UnaryExpr); ok && receive.Op == token.ARROW {
			root = ps6079RootObject(pass, receive.X)
		}
		if root == nil || component[root] {
			return component
		}
		queue := []types.Object{root}
		for len(queue) > 0 {
			object := queue[0]
			queue = queue[1:]
			if object == nil || component[object] {
				continue
			}
			component[object] = true
			for other := range aliases[object] {
				queue = append(queue, other)
			}
		}
		return component
	}
	invalidateComponent := func(component map[types.Object]bool) {
		for object := range component {
			initializationInvalidations[object] = true
		}
	}
	invalidateTarget := func(expression ast.Expr) {
		identifier, direct := ps2110Unparen(expression).(*ast.Ident)
		if direct {
			object := pass.TypesInfo.ObjectOf(identifier)
			if ps6079PackageStorageObject(pass, object) {
				initializationInvalidations[object] = true
			}
			return
		}
		invalidateComponent(componentFor(expression))
	}
	expressionMayReceiveReference := func(expression ast.Expr) bool {
		mayReceive := ps6079ExpressionMayReceiveReference(
			pass, declarations, expression, make(map[*types.Func]bool),
		)
		if call, ok := ps2110Unparen(expression).(*ast.CallExpr); ok {
			if callee, _, known := typedCallee(pass, call.Fun); known && callee != nil && callee.Pkg() != nil &&
				pass.Pkg != nil && callee.Pkg().Path() == pass.Pkg.Path() {
				mayReceive = ps6079CallHasConcreteReferenceReceive(
					pass, declarations, call, make(map[*types.Func]bool),
				)
			}
		}
		return mayReceive
	}
	markUnsafeDerived := func(destination types.Object, expression ast.Expr, component map[types.Object]bool) {
		if !ps6079UnsafeDerivedExpression(
			pass, declarations, expression, packageFunctionValues, aliases,
		) {
			return
		}
		for object := range ps6079UnsafeDerivedSourceComponent(
			pass, declarations, expression, packageFunctionValues, aliases,
		) {
			component[object] = true
		}
		if destination != nil {
			asynchronousEscapes[destination] = true
		}
		for object := range component {
			asynchronousEscapes[object] = true
		}
	}
	link := func(left ast.Expr, right ast.Expr) {
		object := ps6079RootObject(pass, left)
		component := componentFor(right)
		markUnsafeDerived(object, right, component)
		if object == nil || !ps6079ContainsReferenceFixture(object.Type()) {
			return
		}
		ps6079UpdateReferenceAliasComponent(object, component, true, aliases)
		asynchronous := expressionMayReceiveReference(right) ||
			ps6079ExpressionMayReturnAsynchronousReference(pass, declarations, right)
		for source := range component {
			asynchronous = asynchronous || asynchronousEscapes[source]
		}
		asynchronousEscapes[object] = asynchronousEscapes[object] || asynchronous
	}
	linkMultiResults := func(left []ast.Expr, right ast.Expr) {
		component, referenceResults := ps6079MultiResultReferenceComponent(
			pass, declarations, right, len(left), aliases,
		)
		if component == nil {
			component = make(map[types.Object]bool)
		}
		for _, destination := range left {
			markUnsafeDerived(ps6079RootObject(pass, destination), right, component)
		}
		asynchronous := expressionMayReceiveReference(right) ||
			ps6079ExpressionMayReturnAsynchronousReference(pass, declarations, right)
		for source := range component {
			asynchronous = asynchronous || asynchronousEscapes[source]
		}
		for index, destination := range left {
			if !referenceResults[index] {
				continue
			}
			object := ps6079RootObject(pass, destination)
			ps6079UpdateReferenceAliasComponent(object, component, true, aliases)
			if object != nil {
				asynchronousEscapes[object] = asynchronousEscapes[object] || asynchronous
			}
		}
	}
	linkValues := func(names []*ast.Ident, values []ast.Expr) {
		if len(names) == len(values) {
			for index, name := range names {
				link(name, values[index])
			}
			return
		}
		if len(values) != 1 {
			return
		}
		left := make([]ast.Expr, len(names))
		for index, name := range names {
			left[index] = name
		}
		linkMultiResults(left, values[0])
	}
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok || general.Tok != token.VAR {
				continue
			}
			for _, specification := range general.Specs {
				if value, ok := specification.(*ast.ValueSpec); ok {
					linkValues(value.Names, value.Values)
				}
			}
		}
	}
	connectAllPackageStorage := func() {
		component := ps6079AllPackageStorage(pass)
		for object := range component {
			ps6079UpdateReferenceAliasComponent(object, component, true, aliases)
		}
	}
	visiting := make(map[*types.Func]bool)
	linkCallEffects := func(
		call *ast.CallExpr,
		functionValues map[types.Object][]ast.Expr,
	) map[types.Object]bool {
		captures := ps6079FunctionReferenceCaptures(
			pass, declarations, call.Fun, functionValues,
		)
		component := make(map[types.Object]bool, len(captures))
		for object := range captures {
			component[object] = true
		}
		callables := ps6079ResolveFunctionExpressions(
			pass, call.Fun, functionValues, make(map[types.Object]bool),
		)
		opaqueCallable := len(callables) == 0
		for _, callable := range callables {
			for object := range ps6079CallablePackageReferenceSources(
				pass, declarations, callable, make(map[*types.Func]bool),
			) {
				component[object] = true
			}
			if _, literal := ps2110Unparen(callable).(*ast.FuncLit); literal {
				continue
			}
			callee, _, known := typedCallee(pass, callable)
			opaqueCallable = opaqueCallable || !known || callee == nil ||
				(callee.Pkg() != nil && pass.Pkg != nil && callee.Pkg().Path() == pass.Pkg.Path() &&
					declarations[callee] == nil)
		}
		for _, argument := range call.Args {
			for object := range componentFor(argument) {
				component[object] = true
			}
			if ps6079TypeCanCarryFunction(pass.TypesInfo.TypeOf(argument), make(map[types.Type]bool)) {
				for object := range ps6079FunctionReferenceCaptures(
					pass, declarations, argument, functionValues,
				) {
					component[object] = true
				}
				for object := range ps6079CallablePackageReferenceSources(
					pass, declarations, argument, make(map[*types.Func]bool),
				) {
					component[object] = true
				}
				if _, literal := ps2110Unparen(argument).(*ast.FuncLit); !literal {
					callee, _, known := typedCallee(pass, argument)
					if !known || callee == nil {
						connectAllPackageStorage()
					}
				}
			}
		}
		if opaqueCallable && !ps6079BuiltinOrConversion(pass, call.Fun) &&
			!ps6079UnsafeCall(pass, call.Fun) {
			for object := range ps6079AllPackageStorage(pass) {
				component[object] = true
			}
			connectAllPackageStorage()
		}
		for _, argument := range call.Args {
			typeOf := pass.TypesInfo.TypeOf(argument)
			if typeOf != nil {
				if pointer, ok := types.Unalias(typeOf).Underlying().(*types.Pointer); ok &&
					ps6079ContainsReferenceFixture(pointer.Elem()) {
					ps6079UpdateReferenceAliasComponent(
						ps6079RootObject(pass, argument), component, true, aliases,
					)
				}
			}
			if literal, ok := ps2110Unparen(argument).(*ast.FuncLit); ok {
				ast.Inspect(literal.Body, func(node ast.Node) bool {
					identifier, ok := node.(*ast.Ident)
					if ok {
						object := pass.TypesInfo.ObjectOf(identifier)
						if object != nil && ps6079ContainsReferenceFixture(object.Type()) {
							ps6079UpdateReferenceAliasComponent(object, component, true, aliases)
						}
					}
					return true
				})
			}
		}
		if selector, ok := ps2110Unparen(call.Fun).(*ast.SelectorExpr); ok {
			for source := range componentFor(selector.X) {
				component[source] = true
			}
			object := ps6079RootObject(pass, selector.X)
			if object != nil && ps6079ContainsReferenceFixture(object.Type()) {
				ps6079UpdateReferenceAliasComponent(object, component, true, aliases)
			}
		}
		return component
	}
	var scanBody func(*ast.BlockStmt, map[types.Object][]ast.Expr)
	visitingLiterals := make(map[*ast.FuncLit]bool)
	scanLiteral := func(literal *ast.FuncLit, functionValues map[types.Object][]ast.Expr) {
		if literal == nil || visitingLiterals[literal] {
			return
		}
		visitingLiterals[literal] = true
		scanBody(literal.Body, functionValues)
		delete(visitingLiterals, literal)
	}
	scanBody = func(body *ast.BlockStmt, initialFunctionValues map[types.Object][]ast.Expr) {
		functionValues := make(map[types.Object][]ast.Expr, len(initialFunctionValues))
		for object, values := range initialFunctionValues {
			functionValues[object] = slices.Clone(values)
		}
		recordFunctionValue := func(left ast.Expr, right ast.Expr) {
			typeOf := pass.TypesInfo.TypeOf(right)
			if !ps6079TypeCanCarryFunction(typeOf, make(map[types.Type]bool)) {
				return
			}
			if object := ps6079RootObject(pass, left); object != nil {
				functionValues[object] = append(functionValues[object], right)
			}
		}
		astutil.WithStack(body, func(node ast.Node, stack []ast.Node) bool {
			if _, nested := node.(*ast.FuncLit); nested {
				return false
			}
			switch value := node.(type) {
			case *ast.AssignStmt:
				if value.Tok != token.DEFINE {
					for _, left := range value.Lhs {
						invalidateTarget(left)
					}
				}
				if value.Tok != token.ASSIGN && value.Tok != token.DEFINE {
					return true
				}
				if len(value.Lhs) == len(value.Rhs) {
					for index, left := range value.Lhs {
						recordFunctionValue(left, value.Rhs[index])
						link(left, value.Rhs[index])
					}
				} else if len(value.Rhs) == 1 {
					for _, left := range value.Lhs {
						recordFunctionValue(left, value.Rhs[0])
					}
					linkMultiResults(value.Lhs, value.Rhs[0])
				}
			case *ast.ValueSpec:
				if len(value.Names) == len(value.Values) {
					for index, name := range value.Names {
						recordFunctionValue(name, value.Values[index])
					}
				} else if len(value.Values) == 1 {
					for _, name := range value.Names {
						recordFunctionValue(name, value.Values[0])
					}
				}
				linkValues(value.Names, value.Values)
			case *ast.RangeStmt:
				if value.Tok != token.DEFINE {
					for _, left := range []ast.Expr{value.Key, value.Value} {
						if left != nil {
							invalidateTarget(left)
						}
					}
				}
				for _, target := range []ast.Expr{value.Key, value.Value} {
					if target != nil {
						recordFunctionValue(target, value.X)
					}
				}
				for _, left := range []ast.Expr{value.Key, value.Value} {
					if left != nil {
						link(left, value.X)
						if ps6079ReferenceChannelRange(pass, value) {
							asynchronousEscapes[ps6079RootObject(pass, left)] = true
						}
					}
				}
				if ps6079FunctionRange(pass, value) {
					component := make(map[types.Object]bool)
					callables := ps6079ResolveFunctionExpressions(
						pass, value.X, functionValues, make(map[types.Object]bool),
					)
					opaqueIterator := len(callables) == 0
					persistentIterator := false
					for _, callable := range callables {
						for object := range ps6079CallablePackageReferenceSources(
							pass, declarations, callable, make(map[*types.Func]bool),
						) {
							component[object] = true
						}
						if literal, ok := ps2110Unparen(callable).(*ast.FuncLit); ok {
							persistentIterator = persistentIterator || ps6079BodyMayPersistReferenceEffects(
								pass, declarations, literal.Body, functionValues, make(map[*types.Func]bool),
								make(map[*ast.FuncLit]bool),
							)
							continue
						}
						callee, _, known := typedCallee(pass, callable)
						if !known || callee == nil {
							opaqueIterator = true
							persistentIterator = true
							continue
						}
						if callee.Pkg() == nil || pass.Pkg == nil || callee.Pkg().Path() != pass.Pkg.Path() {
							persistentIterator = true
							continue
						}
						declaration := declarations[callee]
						if declaration == nil || declaration.Body == nil {
							opaqueIterator = true
							persistentIterator = true
							continue
						}
						persistentIterator = persistentIterator || ps6079BodyMayPersistReferenceEffects(
							pass, declarations, declaration.Body, nil, make(map[*types.Func]bool),
							make(map[*ast.FuncLit]bool),
						)
					}
					if len(component) == 0 && opaqueIterator {
						component = ps6079AllPackageStorage(pass)
					}
					if persistentIterator {
						for object := range component {
							asynchronousEscapes[object] = true
						}
					}
					for _, left := range []ast.Expr{value.Key, value.Value} {
						object := ps6079RootObject(pass, left)
						if object != nil && ps6079ContainsReferenceFixture(object.Type()) {
							ps6079UpdateReferenceAliasComponent(object, component, true, aliases)
							asynchronousEscapes[object] = asynchronousEscapes[object] || persistentIterator
						}
					}
				}
			case *ast.SendStmt:
				invalidateComponent(componentFor(value.Value))
				if ps6079TypeCanCarryFunction(pass.TypesInfo.TypeOf(value.Value), make(map[types.Type]bool)) {
					for object := range ps6079FunctionReferenceCaptures(
						pass, declarations, value.Value, functionValues,
					) {
						asynchronousEscapes[object] = true
					}
				}
				channel := ps6079RootObject(pass, value.Chan)
				if channel == nil {
					connectAllPackageStorage()
					return true
				}
				ps6079UpdateReferenceAliasComponent(
					channel, componentFor(value.Value), true, aliases,
				)
				for object := range componentFor(value.Value) {
					asynchronousEscapes[object] = true
				}
			case *ast.IncDecStmt:
				invalidateTarget(value.X)
			case *ast.CallExpr:
				ps6079RecordBuiltinFunctionFlow(pass, value, recordFunctionValue)
				callComponent := linkCallEffects(value, functionValues)
				invalidateComponent(callComponent)
				callables := ps6079ResolveFunctionExpressions(
					pass, value.Fun, functionValues, make(map[types.Object]bool),
				)
				analyzableCall := len(callables) > 0
				for _, callable := range callables {
					if _, literal := ps2110Unparen(callable).(*ast.FuncLit); literal {
						continue
					}
					callee, _, known := typedCallee(pass, callable)
					analyzableCall = analyzableCall && known && callee != nil && callee.Pkg() != nil &&
						pass.Pkg != nil && callee.Pkg().Path() == pass.Pkg.Path() && declarations[callee] != nil
				}
				synchronousAnalyzableCall := analyzableCall &&
					!ps6079CallMayPersistReferenceEffectsWithValues(
						pass, declarations, value, functionValues, make(map[*types.Func]bool),
						make(map[*ast.FuncLit]bool),
					)
				explicitGo := len(stack) > 0
				if explicitGo {
					_, explicitGo = stack[len(stack)-1].(*ast.GoStmt)
				}
				if explicitGo || (!synchronousAnalyzableCall && ps6079CallMayLaunchAsynchronousWork(
					pass, declarations, value, make(map[*types.Func]bool),
				)) {
					for object := range callComponent {
						asynchronousEscapes[object] = true
					}
					for _, argument := range value.Args {
						for object := range componentFor(argument) {
							asynchronousEscapes[object] = true
						}
						if literal, ok := ps2110Unparen(argument).(*ast.FuncLit); ok {
							ast.Inspect(literal.Body, func(node ast.Node) bool {
								identifier, ok := node.(*ast.Ident)
								if ok {
									object := pass.TypesInfo.ObjectOf(identifier)
									if object != nil && ps6079ContainsReferenceFixture(object.Type()) {
										asynchronousEscapes[object] = true
									}
								}
								return true
							})
						}
					}
				}
				for _, callable := range callables {
					if literal, ok := ps2110Unparen(callable).(*ast.FuncLit); ok {
						scanLiteral(literal, functionValues)
						continue
					}
					callee, _, known := typedCallee(pass, callable)
					if !known || callee == nil || callee.Pkg() == nil || pass.Pkg == nil ||
						callee.Pkg().Path() != pass.Pkg.Path() || visiting[callee] {
						continue
					}
					declaration := declarations[callee]
					if declaration == nil || declaration.Body == nil {
						continue
					}
					visiting[callee] = true
					scanBody(declaration.Body, packageFunctionValues)
					delete(visiting, callee)
				}
				if _, directLiteral := ps2110Unparen(value.Fun).(*ast.FuncLit); directLiteral {
					return false
				}
			}
			return true
		})
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
				for _, expression := range value.Values {
					ast.Inspect(expression, func(node ast.Node) bool {
						if _, literal := node.(*ast.FuncLit); literal {
							return false
						}
						call, ok := node.(*ast.CallExpr)
						if !ok {
							return true
						}
						callComponent := linkCallEffects(call, packageFunctionValues)
						invalidateComponent(callComponent)
						callables := ps6079ResolveFunctionExpressions(
							pass, call.Fun, packageFunctionValues, make(map[types.Object]bool),
						)
						analyzableCall := len(callables) > 0
						for _, callable := range callables {
							if _, literal := ps2110Unparen(callable).(*ast.FuncLit); literal {
								continue
							}
							callee, _, known := typedCallee(pass, callable)
							analyzableCall = analyzableCall && known && callee != nil && callee.Pkg() != nil &&
								pass.Pkg != nil && callee.Pkg().Path() == pass.Pkg.Path() && declarations[callee] != nil
						}
						synchronousAnalyzableCall := analyzableCall &&
							!ps6079CallMayPersistReferenceEffectsWithValues(
								pass, declarations, call, packageFunctionValues, make(map[*types.Func]bool),
								make(map[*ast.FuncLit]bool),
							)
						if !synchronousAnalyzableCall && ps6079CallMayLaunchAsynchronousWork(
							pass, declarations, call, make(map[*types.Func]bool),
						) {
							for object := range callComponent {
								asynchronousEscapes[object] = true
							}
						}
						for _, callable := range callables {
							if literal, ok := ps2110Unparen(callable).(*ast.FuncLit); ok {
								scanLiteral(literal, packageFunctionValues)
								continue
							}
							callee, _, known := typedCallee(pass, callable)
							if !known || callee == nil || callee.Pkg() == nil || pass.Pkg == nil ||
								callee.Pkg().Path() != pass.Pkg.Path() || visiting[callee] {
								continue
							}
							called := declarations[callee]
							if called == nil || called.Body == nil {
								continue
							}
							visiting[callee] = true
							scanBody(called.Body, packageFunctionValues)
							delete(visiting, callee)
						}
						if _, directLiteral := ps2110Unparen(call.Fun).(*ast.FuncLit); directLiteral {
							return false
						}
						return true
					})
				}
			}
		}
	}
	for function, declaration := range declarations {
		if function.Name() != "init" || declaration == nil || declaration.Body == nil {
			continue
		}
		visiting[function] = true
		scanBody(declaration.Body, packageFunctionValues)
		delete(visiting, function)
	}
	changed := true
	for changed {
		changed = false
		for object := range initializationInvalidations {
			for alias := range aliases[object] {
				if !initializationInvalidations[alias] {
					initializationInvalidations[alias] = true
					changed = true
				}
			}
		}
	}
}

func ps6079ReferenceAliasComponentWithReturns(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	expression ast.Expr,
	aliases map[types.Object]map[types.Object]bool,
) map[types.Object]bool {
	return ps6079ReferenceAliasComponentWithReturnsExcluding(
		pass, declarations, expression, aliases, nil, true,
	)
}

func ps6079ReferenceAliasTargetComponentWithReturns(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	expression ast.Expr,
	aliases map[types.Object]map[types.Object]bool,
) map[types.Object]bool {
	return ps6079ReferenceAliasComponentWithReturnsExcluding(
		pass, declarations, expression, aliases, nil, false,
	)
}

func ps6079ReferenceAliasComponentWithReturnsExcluding(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	expression ast.Expr,
	aliases map[types.Object]map[types.Object]bool,
	excluded map[types.Object]bool,
	requireCarrierResult bool,
) map[types.Object]bool {
	component := make(map[types.Object]bool)
	carrierResult := ps6079AliasCarrierResultType(pass, expression)
	var queue []types.Object
	if !requireCarrierResult || carrierResult {
		queue = ps6079ReferenceAliasSources(pass, expression)
	}
	queue = append(queue, ps6079ReferenceAliasCarrierSources(pass, expression, aliases)...)
	if requireCarrierResult && !carrierResult && len(queue) == 0 {
		return component
	}
	if root := ps6079RootObject(pass, expression); (!requireCarrierResult || carrierResult) &&
		root != nil && len(aliases[root]) > 0 {
		queue = append(queue, root)
	}
	if !requireCarrierResult || carrierResult {
		returned := ps6079ReferenceReturnSources(
			pass, declarations, expression, make(map[*types.Func]bool),
		)
		for object := range returned {
			queue = append(queue, object)
		}
	}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if current == nil || component[current] || excluded[current] {
			continue
		}
		component[current] = true
		for other := range aliases[current] {
			queue = append(queue, other)
		}
	}
	return component
}

func ps6079MultiResultReferenceComponent(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	expression ast.Expr,
	results int,
	aliases map[types.Object]map[types.Object]bool,
) (map[types.Object]bool, []bool) {
	referenceResults := make([]bool, results)
	typeOf := pass.TypesInfo.TypeOf(ps2110Unparen(expression))
	if typeOf == nil {
		return nil, referenceResults
	}
	tuple, ok := types.Unalias(typeOf).Underlying().(*types.Tuple)
	if !ok || tuple.Len() != results {
		return nil, referenceResults
	}
	containsReference := false
	for index := range results {
		referenceResults[index] = ps6079AliasCarrierType(tuple.At(index).Type())
		containsReference = containsReference || referenceResults[index]
	}
	if !containsReference {
		return nil, referenceResults
	}
	return ps6079ReferenceAliasComponentWithReturns(pass, declarations, expression, aliases), referenceResults
}

func ps6079ReferenceAliasSources(pass *analysis.Pass, expression ast.Expr) []types.Object {
	seen := make(map[types.Object]bool)
	var visit func(ast.Expr)
	visit = func(expression ast.Expr) {
		expression = ps2110Unparen(expression)
		if ps6079ContainsReferenceFixture(pass.TypesInfo.TypeOf(expression)) {
			if object := ps6079RootObject(pass, expression); object != nil {
				seen[object] = true
				return
			}
			if call, ok := expression.(*ast.CallExpr); ok {
				if selector, ok := ps2110Unparen(call.Fun).(*ast.SelectorExpr); ok &&
					ps6079ContainsReferenceFixture(pass.TypesInfo.TypeOf(selector.X)) {
					visit(selector.X)
				}
				for _, argument := range call.Args {
					if ps6079ContainsReferenceFixture(pass.TypesInfo.TypeOf(argument)) {
						visit(argument)
					}
				}
			}
		}
		switch value := expression.(type) {
		case *ast.CompositeLit:
			for _, element := range value.Elts {
				if keyed, ok := element.(*ast.KeyValueExpr); ok {
					visit(keyed.Key)
					visit(keyed.Value)
					continue
				}
				visit(element)
			}
		case *ast.UnaryExpr:
			if value.Op == token.AND {
				visit(value.X)
			}
		case *ast.SelectorExpr:
			visit(value.X)
		case *ast.IndexExpr:
			visit(value.X)
		case *ast.IndexListExpr:
			visit(value.X)
		case *ast.SliceExpr:
			visit(value.X)
		case *ast.TypeAssertExpr:
			visit(value.X)
		case *ast.StarExpr:
			visit(value.X)
		case *ast.CallExpr:
			if len(value.Args) == 1 && ps6079ConversionPreservesReference(pass, value) {
				visit(value.Args[0])
			}
		}
	}
	visit(expression)
	result := make([]types.Object, 0, len(seen))
	for object := range seen {
		result = append(result, object)
	}
	return result
}

func ps6079ReferenceReturnSources(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	expression ast.Expr,
	visiting map[*types.Func]bool,
) map[types.Object]bool {
	sources := make(map[types.Object]bool)
	ast.Inspect(expression, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || !ps6079AliasCarrierType(pass.TypesInfo.TypeOf(call)) {
			return true
		}
		for object := range ps6079ReferenceCallReturnSources(pass, declarations, call, visiting) {
			sources[object] = true
		}
		return true
	})
	return sources
}

func ps6079ReferenceCallReturnSources(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	call *ast.CallExpr,
	visiting map[*types.Func]bool,
) map[types.Object]bool {
	return ps6079ReferenceCallableReturnSources(pass, declarations, call, call.Fun, visiting)
}

func ps6079ReferenceCallableReturnSources(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	call *ast.CallExpr,
	callable ast.Expr,
	visiting map[*types.Func]bool,
) map[types.Object]bool {
	sources := make(map[types.Object]bool)
	if call == nil || !ps6079AliasCarrierType(pass.TypesInfo.TypeOf(call)) {
		return sources
	}
	if literal, ok := ps2110Unparen(callable).(*ast.FuncLit); ok {
		return ps6079BodyReferenceReturnSources(
			pass, declarations, literal.Body, ps6079NamedResultObjects(pass, literal.Type.Results), visiting,
		)
	}
	callee, _, known := typedCallee(pass, callable)
	if !known {
		if ps6079BuiltinOrConversion(pass, callable) || ps6079UnsafeCall(pass, callable) {
			return sources
		}
		return ps6079AllPackageStorage(pass)
	}
	if callee.Pkg() == nil || pass.Pkg == nil || callee.Pkg().Path() != pass.Pkg.Path() {
		// A known external function cannot capture unpassed storage from this
		// package. Its opaque reference result is tracked as an asynchronous
		// result at the destination instead of aliasing every package variable.
		// Reference arguments and receivers are collected separately by
		// ps6079ReferenceAliasSources.
		return sources
	}
	if visiting[callee] {
		return ps6079AllPackageStorage(pass)
	}
	declaration := declarations[callee]
	if declaration == nil || declaration.Body == nil {
		return ps6079AllPackageStorage(pass)
	}
	visiting[callee] = true
	for object := range ps6079BodyReferenceReturnSources(
		pass, declarations, declaration.Body,
		ps6079NamedResultObjects(pass, declaration.Type.Results), visiting,
	) {
		sources[object] = true
	}
	delete(visiting, callee)
	return sources
}

func ps6079FixtureCallReturnSources(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	call *ast.CallExpr,
	functionValues map[types.Object][]ast.Expr,
) map[types.Object]bool {
	sources := make(map[types.Object]bool)
	callables, complete := ps6079ImmutableCallables(pass, call.Fun, functionValues)
	if !complete {
		return sources
	}
	for _, callable := range callables {
		if literal, literalCall := ps2110Unparen(callable).(*ast.FuncLit); literalCall {
			if !ps6079LiteralReferenceReturnsHaveSources(pass, declarations, literal) {
				return make(map[types.Object]bool)
			}
		} else {
			callee, _, resolved := typedCallee(pass, callable)
			if !resolved || callee == nil || ps6079DynamicDispatch(pass, callable, callee) {
				return make(map[types.Object]bool)
			}
		}
		for object := range ps6079ReferenceCallableReturnSources(
			pass, declarations, call, callable, make(map[*types.Func]bool),
		) {
			sources[object] = true
		}
	}
	return sources
}

func ps6079LiteralReferenceReturnsHaveSources(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	literal *ast.FuncLit,
) bool {
	complete := true
	returns := 0
	ast.Inspect(literal.Body, func(node ast.Node) bool {
		if !complete {
			return false
		}
		if nested, ok := node.(*ast.FuncLit); ok && nested != literal {
			return false
		}
		statement, ok := node.(*ast.ReturnStmt)
		if !ok {
			return true
		}
		returns++
		if len(statement.Results) == 0 {
			complete = false
			return false
		}
		for _, result := range statement.Results {
			if !ps6079ContainsReferenceFixture(pass.TypesInfo.TypeOf(result)) {
				continue
			}
			hasSource := ps6079ReturnPreservesFixtureAlias(pass, result) &&
				(len(ps6079ReferenceAliasSources(pass, result)) > 0 ||
					len(ps6079ReferenceReturnSources(pass, declarations, result, make(map[*types.Func]bool))) > 0)
			if !hasSource {
				complete = false
				return false
			}
		}
		return true
	})
	return complete && returns > 0
}

func ps6079ReturnPreservesFixtureAlias(pass *analysis.Pass, expression ast.Expr) bool {
	expression = ps2110Unparen(expression)
	switch value := expression.(type) {
	case *ast.Ident:
		return ps6079PackageStorageObject(pass, pass.TypesInfo.ObjectOf(value))
	case *ast.SliceExpr:
		return ps6079ReturnPreservesFixtureAlias(pass, value.X)
	case *ast.StarExpr:
		return ps6079ReturnPreservesFixtureAlias(pass, value.X)
	case *ast.TypeAssertExpr:
		return ps6079ReturnPreservesFixtureAlias(pass, value.X)
	case *ast.UnaryExpr:
		return value.Op == token.AND && ps6079ReturnPreservesFixtureAlias(pass, value.X)
	case *ast.CallExpr:
		return len(value.Args) == 1 && ps6079ConversionPreservesReference(pass, value) &&
			ps6079ReturnPreservesFixtureAlias(pass, value.Args[0])
	default:
		return false
	}
}

func ps6079CallablePackageReferenceSources(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	callable ast.Expr,
	visiting map[*types.Func]bool,
) map[types.Object]bool {
	if literal, ok := ps2110Unparen(callable).(*ast.FuncLit); ok {
		return ps6079BodyPackageReferenceSources(pass, declarations, literal.Body, visiting)
	}
	callee, _, known := typedCallee(pass, callable)
	if !known || callee.Pkg() == nil || pass.Pkg == nil || callee.Pkg().Path() != pass.Pkg.Path() ||
		visiting[callee] {
		return nil
	}
	declaration := declarations[callee]
	if declaration == nil || declaration.Body == nil {
		return nil
	}
	visiting[callee] = true
	sources := ps6079BodyPackageReferenceSources(pass, declarations, declaration.Body, visiting)
	delete(visiting, callee)
	return sources
}

func ps6079BodyPackageReferenceSources(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	body *ast.BlockStmt,
	visiting map[*types.Func]bool,
) map[types.Object]bool {
	sources := make(map[types.Object]bool)
	ast.Inspect(body, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.Ident:
			object := pass.TypesInfo.ObjectOf(value)
			if ps6079PackageStorageObject(pass, object) {
				sources[object] = true
			}
		case *ast.CallExpr:
			for object := range ps6079CallablePackageReferenceSources(
				pass, declarations, value.Fun, visiting,
			) {
				sources[object] = true
			}
		}
		return true
	})
	return sources
}

func ps6079BodyReferenceReturnSources(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	body *ast.BlockStmt,
	namedResults []types.Object,
	visiting map[*types.Func]bool,
) map[types.Object]bool {
	sources := make(map[types.Object]bool)
	aliases := make(map[types.Object]map[types.Object]bool)
	functionValues := make(map[types.Object][]ast.Expr)
	var resolveFunctionValues func(ast.Expr, map[types.Object]bool) []ast.Expr
	resolveFunctionValues = func(expression ast.Expr, seen map[types.Object]bool) []ast.Expr {
		identifier, ok := ps2110Unparen(expression).(*ast.Ident)
		if !ok {
			return []ast.Expr{expression}
		}
		object := pass.TypesInfo.ObjectOf(identifier)
		if object == nil || len(functionValues[object]) == 0 {
			return []ast.Expr{expression}
		}
		if seen[object] {
			return nil
		}
		seen[object] = true
		var resolved []ast.Expr
		for _, value := range functionValues[object] {
			resolved = append(resolved, resolveFunctionValues(value, seen)...)
		}
		delete(seen, object)
		return resolved
	}
	component := func(expression ast.Expr) map[types.Object]bool {
		result := ps6079ReferenceAliasComponent(pass, expression, aliases)
		queue := make([]types.Object, 0)
		ast.Inspect(expression, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || !ps6079AliasCarrierType(pass.TypesInfo.TypeOf(call)) {
				return true
			}
			identifier, localFunctionValue := ps2110Unparen(call.Fun).(*ast.Ident)
			localFunctionValue = localFunctionValue && len(functionValues[pass.TypesInfo.ObjectOf(identifier)]) > 0
			if !localFunctionValue {
				for object := range ps6079ReferenceCallReturnSources(pass, declarations, call, visiting) {
					queue = append(queue, object)
				}
				return true
			}
			for _, resolved := range resolveFunctionValues(identifier, make(map[types.Object]bool)) {
				for object := range ps6079CallablePackageReferenceSources(
					pass, declarations, resolved, visiting,
				) {
					queue = append(queue, object)
				}
			}
			return true
		})
		for len(queue) > 0 {
			object := queue[0]
			queue = queue[1:]
			if object == nil || result[object] {
				continue
			}
			result[object] = true
			for other := range aliases[object] {
				queue = append(queue, other)
			}
		}
		return result
	}
	addReturnedComponent := func(returned map[types.Object]bool) {
		queue := make([]types.Object, 0, len(returned))
		for object := range returned {
			queue = append(queue, object)
		}
		seen := make(map[types.Object]bool)
		for len(queue) > 0 {
			object := queue[0]
			queue = queue[1:]
			if object == nil || seen[object] {
				continue
			}
			seen[object] = true
			if ps6079PackageStorageObject(pass, object) {
				sources[object] = true
			}
			for other := range aliases[object] {
				queue = append(queue, other)
			}
		}
	}
	learn := func(left ast.Expr, right ast.Expr) {
		object := ps6079RootObject(pass, left)
		if object == nil || !ps6079AliasCarrierType(object.Type()) {
			return
		}
		ps6079UpdateReferenceAliasComponent(object, component(right), true, aliases)
	}
	recordFunctionValue := func(left ast.Expr, right ast.Expr) {
		typeOf := pass.TypesInfo.TypeOf(right)
		if typeOf == nil {
			return
		}
		if _, ok := types.Unalias(typeOf).Underlying().(*types.Signature); !ok {
			return
		}
		if object := ps6079RootObject(pass, left); object != nil {
			for _, existing := range functionValues[object] {
				if existing == right {
					return
				}
			}
			functionValues[object] = append(functionValues[object], right)
		}
	}
	processedLiterals := make(map[*ast.FuncLit]bool)
	var applyCallEffects func(*ast.CallExpr)
	applyCallEffects = func(call *ast.CallExpr) {
		callables := resolveFunctionValues(call.Fun, make(map[types.Object]bool))
		packageSources := make(map[types.Object]bool)
		opaqueCallable := false
		for _, callable := range callables {
			for object := range ps6079CallablePackageReferenceSources(
				pass, declarations, callable, visiting,
			) {
				packageSources[object] = true
			}
			if _, literal := ps2110Unparen(callable).(*ast.FuncLit); literal {
				continue
			}
			callee, _, known := typedCallee(pass, callable)
			opaqueCallable = opaqueCallable || !known ||
				(callee != nil && callee.Pkg() != nil && pass.Pkg != nil &&
					callee.Pkg().Path() == pass.Pkg.Path() && declarations[callee] == nil)
		}
		if opaqueCallable && len(packageSources) == 0 {
			for object := range ps6079AllPackageStorage(pass) {
				packageSources[object] = true
			}
		}
		effectSources := make(map[types.Object]bool, len(packageSources))
		for object := range packageSources {
			effectSources[object] = true
		}
		for _, argument := range call.Args {
			if ps6079ContainsReferenceFixture(pass.TypesInfo.TypeOf(argument)) {
				for object := range component(argument) {
					effectSources[object] = true
				}
			}
		}
		for _, argument := range call.Args {
			typeOf := pass.TypesInfo.TypeOf(argument)
			if typeOf == nil {
				continue
			}
			pointer, ok := types.Unalias(typeOf).Underlying().(*types.Pointer)
			if !ok || !ps6079ContainsReferenceFixture(pointer.Elem()) {
				continue
			}
			object := ps6079RootObject(pass, argument)
			if object != nil {
				ps6079UpdateReferenceAliasComponent(object, effectSources, true, aliases)
			}
		}
		for _, argument := range call.Args {
			literal, ok := ps2110Unparen(argument).(*ast.FuncLit)
			if !ok {
				continue
			}
			ast.Inspect(literal.Body, func(node ast.Node) bool {
				identifier, ok := node.(*ast.Ident)
				if !ok {
					return true
				}
				object := pass.TypesInfo.ObjectOf(identifier)
				if object != nil && ps6079ContainsReferenceFixture(object.Type()) {
					ps6079UpdateReferenceAliasComponent(object, effectSources, true, aliases)
				}
				return true
			})
		}
		if selector, ok := ps2110Unparen(call.Fun).(*ast.SelectorExpr); ok {
			receiver := ps6079RootObject(pass, selector.X)
			if receiver != nil && ps6079ContainsReferenceFixture(receiver.Type()) {
				ps6079UpdateReferenceAliasComponent(receiver, effectSources, true, aliases)
			}
		}
		for _, callable := range callables {
			literal, ok := ps2110Unparen(callable).(*ast.FuncLit)
			if !ok || processedLiterals[literal] {
				continue
			}
			processedLiterals[literal] = true
			astutil.WithStack(literal.Body, func(node ast.Node, _ []ast.Node) bool {
				if _, nested := node.(*ast.FuncLit); nested {
					return false
				}
				switch value := node.(type) {
				case *ast.AssignStmt:
					if value.Tok != token.ASSIGN && value.Tok != token.DEFINE {
						return true
					}
					if len(value.Lhs) == len(value.Rhs) {
						for index, left := range value.Lhs {
							recordFunctionValue(left, value.Rhs[index])
							learn(left, value.Rhs[index])
						}
					} else if len(value.Rhs) == 1 {
						for _, left := range value.Lhs {
							recordFunctionValue(left, value.Rhs[0])
							learn(left, value.Rhs[0])
						}
					}
				case *ast.ValueSpec:
					if len(value.Names) == len(value.Values) {
						for index, name := range value.Names {
							recordFunctionValue(name, value.Values[index])
							learn(name, value.Values[index])
						}
					} else if len(value.Values) == 1 {
						for _, name := range value.Names {
							recordFunctionValue(name, value.Values[0])
							learn(name, value.Values[0])
						}
					}
				case *ast.CallExpr:
					applyCallEffects(value)
				}
				return true
			})
		}
	}
	astutil.WithStack(body, func(node ast.Node, _ []ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		switch value := node.(type) {
		case *ast.AssignStmt:
			if value.Tok != token.ASSIGN && value.Tok != token.DEFINE {
				return true
			}
			if len(value.Lhs) == len(value.Rhs) {
				for index, left := range value.Lhs {
					recordFunctionValue(left, value.Rhs[index])
					learn(left, value.Rhs[index])
				}
			} else if len(value.Rhs) == 1 {
				for _, left := range value.Lhs {
					recordFunctionValue(left, value.Rhs[0])
					learn(left, value.Rhs[0])
				}
			}
		case *ast.ValueSpec:
			if len(value.Names) == len(value.Values) {
				for index, name := range value.Names {
					recordFunctionValue(name, value.Values[index])
					learn(name, value.Values[index])
				}
			} else if len(value.Values) == 1 {
				for _, name := range value.Names {
					recordFunctionValue(name, value.Values[0])
					learn(name, value.Values[0])
				}
			}
		case *ast.RangeStmt:
			for _, target := range []ast.Expr{value.Key, value.Value} {
				if target != nil {
					learn(target, value.X)
				}
			}
			if ps6079FunctionRange(pass, value) {
				packageSources := make(map[types.Object]bool)
				for _, callable := range resolveFunctionValues(value.X, make(map[types.Object]bool)) {
					for object := range ps6079CallablePackageReferenceSources(
						pass, declarations, callable, visiting,
					) {
						packageSources[object] = true
					}
				}
				if len(packageSources) == 0 && pass.Pkg != nil {
					for object := range ps6079AllPackageStorage(pass) {
						packageSources[object] = true
					}
				}
				for _, target := range []ast.Expr{value.Key, value.Value} {
					object := ps6079RootObject(pass, target)
					if object != nil && ps6079AliasCarrierType(object.Type()) {
						ps6079UpdateReferenceAliasComponent(object, packageSources, true, aliases)
					}
				}
			}
		case *ast.CallExpr:
			applyCallEffects(value)
		case *ast.ReturnStmt:
			if len(value.Results) == 0 {
				returned := make(map[types.Object]bool, len(namedResults))
				for _, object := range namedResults {
					returned[object] = true
				}
				addReturnedComponent(returned)
				return false
			}
			for _, result := range value.Results {
				addReturnedComponent(component(result))
			}
			return false
		}
		return true
	})
	return sources
}

func ps6079UpdateAddressAlias(
	pass *analysis.Pass,
	destination types.Object,
	expression ast.Expr,
	conditional bool,
	aliases map[types.Object]map[types.Object]bool,
) {
	if destination == nil || !ps6079PointerType(destination.Type()) &&
		!ps6079UnsafePointerType(destination.Type()) {
		return
	}
	targets := make(map[types.Object]bool)
	expression = ps2110Unparen(expression)
	if address, ok := expression.(*ast.UnaryExpr); ok && address.Op == token.AND {
		if target := ps6079RootObject(pass, address.X); target != nil {
			targets[target] = true
		}
	}
	for target := range aliases[ps6079RootObject(pass, expression)] {
		targets[target] = true
	}
	if conditional {
		if aliases[destination] == nil {
			aliases[destination] = make(map[types.Object]bool)
		}
		for target := range targets {
			aliases[destination][target] = true
		}
		return
	}
	if len(targets) == 0 {
		delete(aliases, destination)
	} else {
		aliases[destination] = targets
	}
}

func ps6079PointerType(typeOf types.Type) bool {
	if typeOf == nil {
		return false
	}
	_, ok := types.Unalias(typeOf).Underlying().(*types.Pointer)
	return ok
}

func ps6079InvalidateAliasSource(
	pass *analysis.Pass,
	expression ast.Expr,
	position token.Pos,
	add func(types.Object, token.Pos, ps6079FixtureState, string),
) {
	typeOf := pass.TypesInfo.TypeOf(ps2110Unparen(expression))
	if !ps6079ReferenceFixture(typeOf) {
		return
	}
	if object := ps6079RootObject(pass, expression); object != nil {
		source := "reference alias creation"
		switch types.Unalias(typeOf).Underlying().(type) {
		case *types.Slice:
			source = "slice alias creation"
		case *types.Pointer:
			source = "pointer alias creation"
		}
		add(object, position, ps6079UnknownFixture, source)
	}
}

func ps6079UnknownFixtureTarget(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	expression ast.Expr,
	position token.Pos,
	source string,
	referenceAliases map[types.Object]map[types.Object]bool,
	add func(types.Object, token.Pos, ps6079FixtureState, string),
) {
	component := ps6079ReferenceAliasTargetComponentWithReturns(
		pass, declarations, expression, referenceAliases,
	)
	for object := range component {
		add(object, position, ps6079UnknownFixture, source)
	}
	if object := ps6079RootObject(pass, expression); object != nil && !component[object] {
		add(object, position, ps6079UnknownFixture, source)
	}
}

func ps6079AssignedFixtureExpression(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	expression ast.Expr,
	events map[types.Object][]ps6079FixtureEvent,
	functionValues map[types.Object][]ast.Expr,
	before token.Pos,
) (ps6079FixtureState, ps6079FixtureEvent) {
	state, event := ps6079FixtureExpression(pass, declarations, expression, events, functionValues, before)
	identifier, ok := ps2110Unparen(expression).(*ast.Ident)
	if !ok {
		return state, event
	}
	typeOf := pass.TypesInfo.TypeOf(identifier)
	if typeOf == nil {
		return ps6079UnknownFixture, ps6079FixtureEvent{state: ps6079UnknownFixture, source: "assignment with unknown type"}
	}
	if _, aliases := types.Unalias(typeOf).Underlying().(*types.Slice); aliases {
		return ps6079UnknownFixture, ps6079FixtureEvent{
			state: ps6079UnknownFixture, source: "slice alias from " + identifier.Name,
		}
	}
	return state, event
}

func ps6079ConditionalFixtureEvent(stack []ast.Node) bool {
	for _, ancestor := range stack {
		switch ancestor.(type) {
		case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt, *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt:
			return true
		}
	}
	return false
}

func ps6079FixtureEventLoops(stack []ast.Node) []ps6079LoopRange {
	var result []ps6079LoopRange
	for _, ancestor := range stack {
		switch loop := ancestor.(type) {
		case *ast.ForStmt:
			result = append(result, ps6079LoopRange{start: loop.Pos(), end: loop.End()})
		case *ast.RangeStmt:
			result = append(result, ps6079LoopRange{start: loop.Pos(), end: loop.End()})
		}
	}
	return result
}

func ps6079CallMayMutatePackageStorageWithContext(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	call *ast.CallExpr,
	packageAliases map[types.Object]bool,
	functionValues map[types.Object][]ast.Expr,
	visitingFunctions map[*types.Func]bool,
	visitingLiterals map[*ast.FuncLit]bool,
) bool {
	if typeAndValue, ok := pass.TypesInfo.Types[call.Fun]; ok && typeAndValue.IsType() {
		return false
	}
	for _, name := range []string{
		"cap", "complex", "imag", "len", "make", "max", "min", "new", "panic", "print", "println", "real", "recover",
	} {
		if typedBuiltinName(pass, call.Fun, name) {
			return false
		}
	}
	for _, name := range []string{"append", "clear", "copy", "delete"} {
		if typedBuiltinName(pass, call.Fun, name) {
			return len(call.Args) > 0 && ps6079ExpressionAliasesPackageStorage(
				pass, declarations, call.Args[0], packageAliases,
			)
		}
	}
	callables := ps6079ResolveFunctionExpressions(
		pass, call.Fun, functionValues, make(map[types.Object]bool),
	)
	for _, callable := range callables {
		if ps6079CallableMayMutatePackageStorage(
			pass, declarations, call, callable, packageAliases, functionValues,
			visitingFunctions, visitingLiterals,
		) {
			return true
		}
	}
	return false
}

func ps6079CallableMayMutatePackageStorage(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	call *ast.CallExpr,
	callable ast.Expr,
	packageAliases map[types.Object]bool,
	functionValues map[types.Object][]ast.Expr,
	visitingFunctions map[*types.Func]bool,
	visitingLiterals map[*ast.FuncLit]bool,
) bool {
	if literal, ok := ps2110Unparen(callable).(*ast.FuncLit); ok {
		if visitingLiterals[literal] {
			return true
		}
		aliases, functions := ps6079CallPackageMutationBindings(
			pass, declarations, call, pass.TypesInfo.TypeOf(literal), packageAliases, functionValues,
		)
		visitingLiterals[literal] = true
		mutates := ps6079BodyMayMutatePackageStorage(
			pass, declarations, literal.Body, aliases, functions, visitingFunctions, visitingLiterals,
		)
		delete(visitingLiterals, literal)
		return mutates
	}
	callee, _, known := typedCallee(pass, callable)
	if !known || callee == nil || callee.Pkg() == nil || pass.Pkg == nil ||
		callee.Pkg().Path() != pass.Pkg.Path() {
		if selector, ok := ps2110Unparen(call.Fun).(*ast.SelectorExpr); ok &&
			ps6079ExpressionAliasesPackageStorage(pass, declarations, selector.X, packageAliases) {
			return true
		}
		for _, argument := range call.Args {
			if ps6079ExpressionAliasesPackageStorage(pass, declarations, argument, packageAliases) {
				return true
			}
			if literal, ok := ps2110Unparen(argument).(*ast.FuncLit); ok &&
				len(ps6079BodyPackageReferenceSources(pass, declarations, literal.Body, make(map[*types.Func]bool))) > 0 {
				return true
			}
		}
		return false
	}
	if visitingFunctions[callee] {
		return true
	}
	declaration := declarations[callee]
	if declaration == nil || declaration.Body == nil {
		return true
	}
	aliases, functions := ps6079CallPackageMutationBindings(
		pass, declarations, call, callee.Type(), packageAliases, functionValues,
	)
	visitingFunctions[callee] = true
	mutates := ps6079BodyMayMutatePackageStorage(
		pass, declarations, declaration.Body, aliases, functions, visitingFunctions, visitingLiterals,
	)
	delete(visitingFunctions, callee)
	return mutates
}

func ps6079CallPackageMutationBindings(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	call *ast.CallExpr,
	callableType types.Type,
	packageAliases map[types.Object]bool,
	functionValues map[types.Object][]ast.Expr,
) (map[types.Object]bool, map[types.Object][]ast.Expr) {
	aliases := make(map[types.Object]bool, len(packageAliases))
	for object, alias := range packageAliases {
		aliases[object] = alias
	}
	functions := make(map[types.Object][]ast.Expr, len(functionValues))
	for object, values := range functionValues {
		functions[object] = slices.Clone(values)
	}
	if callableType == nil {
		return aliases, functions
	}
	signature, ok := types.Unalias(callableType).Underlying().(*types.Signature)
	if !ok {
		return aliases, functions
	}
	for index, argument := range call.Args {
		parameterIndex := index
		if parameterIndex >= signature.Params().Len() {
			if !signature.Variadic() || signature.Params().Len() == 0 {
				break
			}
			parameterIndex = signature.Params().Len() - 1
		}
		parameter := signature.Params().At(parameterIndex)
		if ps6079ExpressionAliasesPackageStorage(pass, declarations, argument, packageAliases) {
			aliases[parameter] = true
		}
		if ps6079TypeCanCarryFunction(parameter.Type(), make(map[types.Type]bool)) {
			functions[parameter] = append(functions[parameter], ps6079ResolveFunctionExpressions(
				pass, argument, functionValues, make(map[types.Object]bool),
			)...)
		}
	}
	if signature.Recv() != nil {
		if selector, ok := ps2110Unparen(call.Fun).(*ast.SelectorExpr); ok &&
			ps6079ExpressionAliasesPackageStorage(pass, declarations, selector.X, packageAliases) {
			aliases[signature.Recv()] = true
		}
	}
	return aliases, functions
}

func ps6079ResolveFunctionExpressions(
	pass *analysis.Pass,
	expression ast.Expr,
	functionValues map[types.Object][]ast.Expr,
	seen map[types.Object]bool,
) []ast.Expr {
	identifier, ok := ps2110Unparen(expression).(*ast.Ident)
	if !ok {
		return []ast.Expr{expression}
	}
	object := pass.TypesInfo.ObjectOf(identifier)
	if object == nil || len(functionValues[object]) == 0 {
		return []ast.Expr{expression}
	}
	if seen[object] {
		return nil
	}
	seen[object] = true
	var resolved []ast.Expr
	for _, value := range functionValues[object] {
		resolved = append(resolved, ps6079ResolveFunctionExpressions(pass, value, functionValues, seen)...)
	}
	delete(seen, object)
	return resolved
}

func ps6079ImmutableFunctionValues(pass *analysis.Pass, body *ast.BlockStmt) map[types.Object][]ast.Expr {
	writes := make(map[types.Object]int)
	values := make(map[types.Object]ast.Expr)
	invalid := make(map[types.Object]bool)
	initialized := make(map[types.Object]bool)
	functionObject := func(expression ast.Expr) types.Object {
		object := ps6079RootObject(pass, expression)
		if object == nil || ps6079PackageStorageObject(pass, object) ||
			!ps6079TypeCanCarryFunction(object.Type(), make(map[types.Type]bool)) {
			return nil
		}
		return object
	}
	record := func(left, right ast.Expr, definition bool) {
		object := functionObject(left)
		if object == nil {
			return
		}
		if definition {
			initialized[object] = true
		} else if !initialized[object] {
			invalid[object] = true
		}
		writes[object]++
		values[object] = right
	}
	ast.Inspect(body, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.AssignStmt:
			if len(value.Lhs) == len(value.Rhs) {
				for index, left := range value.Lhs {
					identifier, _ := ps2110Unparen(left).(*ast.Ident)
					record(left, value.Rhs[index], value.Tok == token.DEFINE &&
						identifier != nil && pass.TypesInfo.Defs[identifier] != nil)
				}
			} else if len(value.Rhs) == 1 {
				for _, left := range value.Lhs {
					identifier, _ := ps2110Unparen(left).(*ast.Ident)
					record(left, value.Rhs[0], value.Tok == token.DEFINE &&
						identifier != nil && pass.TypesInfo.Defs[identifier] != nil)
				}
			}
		case *ast.ValueSpec:
			if len(value.Values) == 0 {
				for _, name := range value.Names {
					if object := functionObject(name); object != nil {
						invalid[object] = true
					}
				}
				break
			}
			if len(value.Names) == len(value.Values) {
				for index, name := range value.Names {
					record(name, value.Values[index], true)
				}
			} else if len(value.Values) == 1 {
				for _, name := range value.Names {
					record(name, value.Values[0], true)
				}
			}
		case *ast.RangeStmt:
			for _, target := range []ast.Expr{value.Key, value.Value} {
				if target != nil {
					identifier, _ := ps2110Unparen(target).(*ast.Ident)
					record(target, value.X, value.Tok == token.DEFINE &&
						identifier != nil && pass.TypesInfo.Defs[identifier] != nil)
				}
			}
		case *ast.UnaryExpr:
			if value.Op == token.AND {
				if object := functionObject(value.X); object != nil {
					invalid[object] = true
				}
			}
		}
		return true
	})
	result := make(map[types.Object][]ast.Expr)
	for object, value := range values {
		if writes[object] == 1 && !invalid[object] {
			result[object] = []ast.Expr{value}
		}
	}
	return result
}

func ps6079StableReferenceObjects(pass *analysis.Pass, body *ast.BlockStmt) map[types.Object]bool {
	writes := make(map[types.Object]int)
	invalid := make(map[types.Object]bool)
	referenceObject := func(expression ast.Expr) types.Object {
		identifier, identified := ps2110Unparen(expression).(*ast.Ident)
		if !identified {
			return nil
		}
		object := pass.TypesInfo.ObjectOf(identifier)
		if object == nil || ps6079PackageStorageObject(pass, object) ||
			!ps6079ReferenceFixture(object.Type()) {
			return nil
		}
		return object
	}
	write := func(expression ast.Expr) {
		if object := referenceObject(expression); object != nil {
			writes[object]++
		}
	}
	ast.Inspect(body, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.AssignStmt:
			for _, left := range value.Lhs {
				write(left)
			}
		case *ast.ValueSpec:
			if len(value.Values) == 0 {
				for _, name := range value.Names {
					if object := referenceObject(name); object != nil {
						invalid[object] = true
					}
				}
				break
			}
			for _, name := range value.Names {
				write(name)
			}
		case *ast.RangeStmt:
			write(value.Key)
			write(value.Value)
		case *ast.UnaryExpr:
			if value.Op == token.AND {
				if object := referenceObject(value.X); object != nil {
					invalid[object] = true
				}
			}
		case *ast.SelectorExpr:
			selection := pass.TypesInfo.Selections[value]
			if selection != nil {
				if signature, ok := selection.Obj().Type().(*types.Signature); ok && signature.Recv() != nil {
					if _, pointerReceiver := types.Unalias(signature.Recv().Type()).(*types.Pointer); pointerReceiver {
						if object := referenceObject(value.X); object != nil {
							invalid[object] = true
						}
					}
				}
			}
		}
		return true
	})
	stable := make(map[types.Object]bool, len(writes))
	for object, count := range writes {
		stable[object] = count == 1 && !invalid[object]
	}
	return stable
}

func ps6079ImmutableCallables(
	pass *analysis.Pass,
	expression ast.Expr,
	functionValues map[types.Object][]ast.Expr,
) ([]ast.Expr, bool) {
	var resolve func(ast.Expr, map[types.Object]bool) ([]ast.Expr, bool)
	resolve = func(current ast.Expr, seen map[types.Object]bool) ([]ast.Expr, bool) {
		current = ps2110Unparen(current)
		if conversion, called := current.(*ast.CallExpr); called && len(conversion.Args) == 1 {
			if typeAndValue, ok := pass.TypesInfo.Types[conversion.Fun]; ok && typeAndValue.IsType() {
				destination := pass.TypesInfo.TypeOf(conversion)
				source := pass.TypesInfo.TypeOf(conversion.Args[0])
				if destination != nil && source != nil {
					destinationSignature, destinationFunction := types.Unalias(destination).Underlying().(*types.Signature)
					sourceSignature, sourceFunction := types.Unalias(source).Underlying().(*types.Signature)
					if destinationFunction && sourceFunction && types.Identical(destinationSignature, sourceSignature) {
						return resolve(conversion.Args[0], seen)
					}
				}
			}
		}
		identifier, identified := current.(*ast.Ident)
		if !identified {
			return []ast.Expr{current}, true
		}
		object := pass.TypesInfo.ObjectOf(identifier)
		assigned := functionValues[object]
		if len(assigned) == 0 {
			return []ast.Expr{current}, true
		}
		if len(assigned) != 1 || seen[object] || !assigned[0].Pos().IsValid() || assigned[0].Pos() >= current.Pos() {
			return nil, false
		}
		seen[object] = true
		resolved, ok := resolve(assigned[0], seen)
		delete(seen, object)
		return resolved, ok
	}
	callables, complete := resolve(expression, make(map[types.Object]bool))
	if !complete || len(callables) == 0 {
		return nil, false
	}
	return callables, true
}

func ps6079SemanticCallables(
	pass *analysis.Pass,
	expression ast.Expr,
	functionValues map[types.Object][]ast.Expr,
) ([]ast.Expr, bool) {
	callables, complete := ps6079ImmutableCallables(pass, expression, functionValues)
	if !complete {
		return nil, false
	}
	for _, callable := range callables {
		callee, _, resolved := typedCallee(pass, callable)
		if !resolved || callee == nil || ps6079DynamicDispatch(pass, callable, callee) {
			return nil, false
		}
	}
	return callables, true
}

func ps6079DynamicDispatch(pass *analysis.Pass, callable ast.Expr, callee *types.Func) bool {
	dynamicType := func(typeOf types.Type) bool {
		if typeOf == nil {
			return false
		}
		typeOf = types.Unalias(typeOf)
		if _, parameter := typeOf.(*types.TypeParam); parameter {
			return true
		}
		_, interfaceType := typeOf.Underlying().(*types.Interface)
		return interfaceType
	}
	if signature, ok := callee.Type().(*types.Signature); ok && signature.Recv() != nil &&
		dynamicType(signature.Recv().Type()) {
		return true
	}
	callable = ps2110Unparen(callable)
	switch value := callable.(type) {
	case *ast.IndexExpr:
		callable = ps2110Unparen(value.X)
	case *ast.IndexListExpr:
		callable = ps2110Unparen(value.X)
	}
	selector, selected := callable.(*ast.SelectorExpr)
	if !selected {
		return false
	}
	selection := pass.TypesInfo.Selections[selector]
	return selection != nil && dynamicType(selection.Recv())
}

func ps6079ExpressionAliasesPackageStorage(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	expression ast.Expr,
	packageAliases map[types.Object]bool,
) bool {
	for _, object := range ps6079ReferenceAliasSources(pass, expression) {
		if ps6079PackageStorageObject(pass, object) || packageAliases[object] {
			return true
		}
	}
	if declarations != nil {
		for object := range ps6079ReferenceReturnSources(
			pass, declarations, expression, make(map[*types.Func]bool),
		) {
			if ps6079PackageStorageObject(pass, object) || packageAliases[object] {
				return true
			}
		}
	}
	return false
}

func ps6079ApplyPackageAliasCallEffects(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	call *ast.CallExpr,
	packageAliases map[types.Object]bool,
	functionValues map[types.Object][]ast.Expr,
) {
	effectAliasesPackage := false
	callables := ps6079ResolveFunctionExpressions(
		pass, call.Fun, functionValues, make(map[types.Object]bool),
	)
	for _, callable := range callables {
		sources := ps6079CallablePackageReferenceSources(
			pass, declarations, callable, make(map[*types.Func]bool),
		)
		effectAliasesPackage = effectAliasesPackage || len(sources) > 0
		if _, literal := ps2110Unparen(callable).(*ast.FuncLit); literal {
			continue
		}
		callee, _, known := typedCallee(pass, callable)
		if !known || (callee != nil && callee.Pkg() != nil && pass.Pkg != nil &&
			callee.Pkg().Path() == pass.Pkg.Path() && declarations[callee] == nil) {
			effectAliasesPackage = true
		}
	}
	for _, argument := range call.Args {
		effectAliasesPackage = effectAliasesPackage || ps6079ExpressionAliasesPackageStorage(
			pass, declarations, argument, packageAliases,
		)
	}
	if !effectAliasesPackage {
		return
	}
	for _, argument := range call.Args {
		typeOf := pass.TypesInfo.TypeOf(argument)
		if typeOf != nil {
			if pointer, ok := types.Unalias(typeOf).Underlying().(*types.Pointer); ok &&
				ps6079ContainsReferenceFixture(pointer.Elem()) {
				if object := ps6079RootObject(pass, argument); object != nil {
					packageAliases[object] = true
				}
			}
		}
		if literal, ok := ps2110Unparen(argument).(*ast.FuncLit); ok {
			ast.Inspect(literal.Body, func(node ast.Node) bool {
				identifier, ok := node.(*ast.Ident)
				if ok {
					object := pass.TypesInfo.ObjectOf(identifier)
					if object != nil && ps6079ContainsReferenceFixture(object.Type()) {
						packageAliases[object] = true
					}
				}
				return true
			})
		}
	}
	if selector, ok := ps2110Unparen(call.Fun).(*ast.SelectorExpr); ok {
		object := ps6079RootObject(pass, selector.X)
		if object != nil && ps6079ContainsReferenceFixture(object.Type()) {
			packageAliases[object] = true
		}
	}
}

func ps6079BodyMayMutatePackageStorage(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	body *ast.BlockStmt,
	initialPackageAliases map[types.Object]bool,
	initialFunctionValues map[types.Object][]ast.Expr,
	visitingFunctions map[*types.Func]bool,
	visitingLiterals map[*ast.FuncLit]bool,
) bool {
	packageAliases := make(map[types.Object]bool, len(initialPackageAliases))
	for object, alias := range initialPackageAliases {
		packageAliases[object] = alias
	}
	functionValues := make(map[types.Object][]ast.Expr, len(initialFunctionValues))
	for object, values := range initialFunctionValues {
		functionValues[object] = slices.Clone(values)
	}
	targetMutates := func(expression ast.Expr) bool {
		identifier, direct := ps2110Unparen(expression).(*ast.Ident)
		object := ps6079RootObject(pass, expression)
		if direct {
			object = pass.TypesInfo.ObjectOf(identifier)
			return ps6079PackageStorageObject(pass, object)
		}
		return ps6079PackageStorageObject(pass, object) || packageAliases[object]
	}
	recordFunctionValue := func(left ast.Expr, right ast.Expr) {
		typeOf := pass.TypesInfo.TypeOf(right)
		if typeOf == nil {
			return
		}
		if _, ok := types.Unalias(typeOf).Underlying().(*types.Signature); !ok {
			return
		}
		object := ps6079RootObject(pass, left)
		if object != nil {
			functionValues[object] = append(functionValues[object], right)
		}
	}
	recordAlias := func(left ast.Expr, right ast.Expr) {
		identifier, direct := ps2110Unparen(left).(*ast.Ident)
		if !direct {
			return
		}
		object := pass.TypesInfo.ObjectOf(identifier)
		if object != nil && ps6079ExpressionAliasesPackageStorage(pass, declarations, right, packageAliases) {
			packageAliases[object] = true
		}
	}
	mutates := false
	astutil.WithStack(body, func(node ast.Node, _ []ast.Node) bool {
		if mutates {
			return false
		}
		switch value := node.(type) {
		case *ast.FuncLit:
			return false
		case *ast.AssignStmt:
			for _, left := range value.Lhs {
				if targetMutates(left) {
					mutates = true
					return false
				}
			}
			if len(value.Lhs) == len(value.Rhs) {
				for index, left := range value.Lhs {
					recordFunctionValue(left, value.Rhs[index])
					recordAlias(left, value.Rhs[index])
				}
			} else if len(value.Rhs) == 1 {
				for _, left := range value.Lhs {
					recordFunctionValue(left, value.Rhs[0])
					recordAlias(left, value.Rhs[0])
				}
			}
		case *ast.ValueSpec:
			if len(value.Names) == len(value.Values) {
				for index, name := range value.Names {
					recordFunctionValue(name, value.Values[index])
					recordAlias(name, value.Values[index])
				}
			} else if len(value.Values) == 1 {
				for _, name := range value.Names {
					recordFunctionValue(name, value.Values[0])
					recordAlias(name, value.Values[0])
				}
			}
		case *ast.RangeStmt:
			for _, left := range []ast.Expr{value.Key, value.Value} {
				if left != nil && targetMutates(left) {
					mutates = true
					return false
				}
				if left != nil {
					recordAlias(left, value.X)
				}
			}
			if ps6079FunctionRange(pass, value) {
				mutates = ps6079CallMayMutatePackageStorageWithContext(
					pass, declarations, &ast.CallExpr{Fun: value.X}, packageAliases, functionValues,
					visitingFunctions, visitingLiterals,
				)
				if mutates {
					return false
				}
			}
		case *ast.IncDecStmt:
			mutates = targetMutates(value.X)
			return !mutates
		case *ast.SendStmt:
			mutates = ps6079ExpressionAliasesPackageStorage(pass, declarations, value.Value, packageAliases)
			return !mutates
		case *ast.CallExpr:
			mutates = ps6079CallMayMutatePackageStorageWithContext(
				pass, declarations, value, packageAliases, functionValues,
				visitingFunctions, visitingLiterals,
			)
			if !mutates {
				ps6079ApplyPackageAliasCallEffects(
					pass, declarations, value, packageAliases, functionValues,
				)
			}
			return !mutates
		}
		return true
	})
	return mutates
}

func ps6079MutationEvents(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	call *ast.CallExpr,
	events map[types.Object][]ps6079FixtureEvent,
	functionValues map[types.Object][]ast.Expr,
	partialReferenceAliases map[types.Object]bool,
	referenceAliases map[types.Object]map[types.Object]bool,
	stableReferenceObjects map[types.Object]bool,
	protectedPackageObjects map[types.Object]bool,
	asynchronous bool,
	add func(types.Object, token.Pos, ps6079FixtureState, string),
) {
	semanticCallables, semanticallyResolved := ps6079SemanticCallables(pass, call.Fun, functionValues)
	allRootObjects := func(expression ast.Expr) map[types.Object]bool {
		return ps6079ReferenceAliasComponentWithReturns(
			pass, declarations, expression, referenceAliases,
		)
	}
	rootObjects := func(expression ast.Expr) map[types.Object]bool {
		return ps6079ReferenceAliasComponentWithReturnsExcluding(
			pass, declarations, expression, referenceAliases, protectedPackageObjects, true,
		)
	}
	samePackageCall := false
	for _, callable := range semanticCallables {
		callee, _, _ := typedCallee(pass, callable)
		samePackageCall = samePackageCall || callee.Pkg() != nil && pass.Pkg != nil &&
			callee.Pkg().Path() == pass.Pkg.Path()
	}
	if samePackageCall {
		// A same-package helper can write package storage without exposing that
		// storage as a receiver or argument. Keep precise local fixture evidence,
		// but invalidate every already observed package fixture and its aliases.
		for object := range ps6079AllPackageStorage(pass) {
			if !protectedPackageObjects[object] {
				add(object, call.Pos(), ps6079UnknownFixture,
					"call to "+exprTextRendered(call.Fun)+" may mutate package fixture")
			}
		}
	}
	if typedBuiltinName(pass, call.Fun, "copy") && len(call.Args) >= 2 {
		for object := range allRootObjects(call.Args[0]) {
			add(object, call.Pos(), ps6079UnknownFixture, "copy with unproven full destination coverage")
		}
		return
	}
	if typedBuiltinName(pass, call.Fun, "clear") && len(call.Args) == 1 {
		switch {
		case ps6079SliceType(pass.TypesInfo.TypeOf(call.Args[0])):
			state, source := ps6079ZeroFixture, "clear"
			if asynchronous {
				state, source = ps6079UnknownFixture, "asynchronous clear"
			} else if !ps6079WholeFixtureDestination(pass, call.Args[0], partialReferenceAliases) {
				state, source = ps6079UnknownFixture, "partial clear with unproven full destination coverage"
			}
			for object := range allRootObjects(call.Args[0]) {
				add(object, call.Pos(), state, source)
			}
		case ps6079MapType(pass.TypesInfo.TypeOf(call.Args[0])):
			for object := range allRootObjects(call.Args[0]) {
				add(object, call.Pos(), ps6079UnknownFixture, "clear may change map predicate")
			}
		}
		return
	}
	if typedBuiltinName(pass, call.Fun, "delete") && len(call.Args) >= 1 &&
		ps6079MapType(pass.TypesInfo.TypeOf(call.Args[0])) {
		for object := range allRootObjects(call.Args[0]) {
			add(object, call.Pos(), ps6079UnknownFixture, "delete may change map predicate")
		}
		return
	}
	if typedBuiltinName(pass, call.Fun, "append") && len(call.Args) > 0 {
		// append can reuse the first argument's backing array even when its
		// returned slice is discarded, so the rooted fixture and its aliases no
		// longer have trustworthy element provenance.
		for object := range allRootObjects(call.Args[0]) {
			add(object, call.Pos(), ps6079UnknownFixture, "append may mutate fixture backing storage")
		}
		return
	}
	state, mutates := ps6079SemanticMutatorState(pass, call.Fun, functionValues, stableReferenceObjects)
	if mutates {
		var destination ast.Expr
		if selector, ok := ps2110Unparen(call.Fun).(*ast.SelectorExpr); ok {
			if ps6079ReferenceFixture(pass.TypesInfo.TypeOf(selector.X)) {
				destination = selector.X
			}
		}
		if destination == nil {
			for _, callable := range semanticCallables {
				if receiver, bound := ps6079BoundMethodReceiver(pass, callable); bound &&
					ps6079ReferenceFixture(pass.TypesInfo.TypeOf(receiver)) {
					destination = receiver
					break
				}
			}
		}
		if destination == nil && len(call.Args) > 0 && ps6079ReferenceFixture(pass.TypesInfo.TypeOf(call.Args[0])) {
			destination = call.Args[0]
		}
		if destination != nil {
			source := exprTextRendered(call.Fun)
			if asynchronous {
				state = ps6079UnknownFixture
				source = "asynchronous fixture mutation by " + source
			} else if !ps6079FixtureStatePossibleForType(pass.TypesInfo.TypeOf(destination), state) {
				state = ps6079UnknownFixture
				source = "fixture mutation by " + source + " has a sign incompatible with its destination type"
			} else if !ps6079WholeFixtureDestination(pass, destination, partialReferenceAliases) {
				state = ps6079UnknownFixture
				source = "partial fixture mutation by " + source
			}
			for object := range allRootObjects(destination) {
				add(object, call.Pos(), state, source)
			}
		}
		if selector, ok := ps2110Unparen(call.Fun).(*ast.SelectorExpr); ok && selector.X != destination {
			for object := range rootObjects(selector.X) {
				add(object, call.Pos(), ps6079UnknownFixture, "non-destination receiver of "+exprTextRendered(call.Fun))
			}
		}
		for _, argument := range call.Args {
			if argument == destination {
				continue
			}
			for object := range rootObjects(argument) {
				add(object, call.Pos(), ps6079UnknownFixture, "non-destination argument to "+exprTextRendered(call.Fun))
			}
		}
		return
	}
	if ps6079BuiltinOrConversion(pass, call.Fun) {
		return
	}
	if !semanticallyResolved && !ps6079BuiltinOrConversion(pass, call.Fun) {
		// A function value can close over a fixture without exposing that fixture
		// as a receiver or argument at the call site. Until the invoked value is
		// resolved to a declaration and its captures are audited, preserve soundness
		// by invalidating every fixture whose provenance has already been observed.
		for object := range events {
			add(object, call.Pos(), ps6079UnknownFixture,
				"indirect call to "+exprTextRendered(call.Fun)+" may mutate captured fixture")
		}
	}
	if selector, ok := ps2110Unparen(call.Fun).(*ast.SelectorExpr); ok {
		for object := range rootObjects(selector.X) {
			add(object, call.Pos(), ps6079UnknownFixture, "call to "+exprTextRendered(call.Fun)+" may mutate fixture")
		}
	}
	for _, callable := range semanticCallables {
		if receiver, bound := ps6079BoundMethodReceiver(pass, callable); bound {
			for object := range rootObjects(receiver) {
				add(object, call.Pos(), ps6079UnknownFixture,
					"bound method call to "+exprTextRendered(call.Fun)+" may mutate fixture")
			}
		}
	}
	for _, argument := range call.Args {
		for object := range rootObjects(argument) {
			add(object, call.Pos(), ps6079UnknownFixture, "call to "+exprTextRendered(call.Fun)+" may mutate fixture")
		}
	}
}

func ps6079BoundMethodReceiver(pass *analysis.Pass, callable ast.Expr) (ast.Expr, bool) {
	selector, selected := ps2110Unparen(callable).(*ast.SelectorExpr)
	if !selected {
		return nil, false
	}
	selection := pass.TypesInfo.Selections[selector]
	return selector.X, selection != nil && selection.Kind() == types.MethodVal
}

func ps6079WholeFixtureDestination(
	pass *analysis.Pass,
	expression ast.Expr,
	partialReferenceAliases map[types.Object]bool,
) bool {
	expression = ps2110Unparen(expression)
	switch value := expression.(type) {
	case *ast.Ident:
		return !partialReferenceAliases[pass.TypesInfo.ObjectOf(value)]
	case *ast.SelectorExpr:
		// Aggregate aliases are intentionally field-insensitive. Treating one
		// selected member as the whole connected component could transfer a fill
		// state to unrelated sibling reference fields.
		return ps6079AggregateReferenceSlotCount(pass.TypesInfo.TypeOf(value.X), 2) == 1 &&
			ps6079WholeFixtureDestination(pass, value.X, partialReferenceAliases)
	case *ast.SliceExpr:
		return value.Low == nil && value.High == nil && value.Max == nil &&
			ps6079WholeFixtureDestination(pass, value.X, partialReferenceAliases)
	case *ast.TypeAssertExpr:
		return ps6079WholeFixtureDestination(pass, value.X, partialReferenceAliases)
	case *ast.CallExpr:
		return len(value.Args) == 1 && ps6079ConversionPreservesReference(pass, value) &&
			ps6079WholeFixtureDestination(pass, value.Args[0], partialReferenceAliases)
	default:
		return false
	}
}

func ps6079AggregateReferenceSlotCount(typeOf types.Type, limit int) int {
	if typeOf == nil {
		return 0
	}
	if pointer, ok := types.Unalias(typeOf).Underlying().(*types.Pointer); ok {
		typeOf = pointer.Elem()
	}
	return ps6079ReferenceSlotCount(typeOf, limit)
}

func ps6079ReferenceSlotCount(typeOf types.Type, limit int) int {
	if typeOf == nil || limit <= 0 {
		return 0
	}
	switch underlying := types.Unalias(typeOf).Underlying().(type) {
	case *types.Slice, *types.Map:
		return 1
	case *types.Interface:
		return limit
	case *types.Pointer:
		return 1
	case *types.Array:
		count := 0
		for range min(int64(limit), underlying.Len()) {
			count += ps6079ReferenceSlotCount(underlying.Elem(), limit-count)
			if count >= limit {
				return limit
			}
		}
		return count
	case *types.Struct:
		count := 0
		for index := range underlying.NumFields() {
			count += ps6079ReferenceSlotCount(underlying.Field(index).Type(), limit-count)
			if count >= limit {
				return limit
			}
		}
		return count
	}
	return 0
}

func ps6079SliceType(typeOf types.Type) bool {
	if typeOf == nil {
		return false
	}
	_, ok := types.Unalias(typeOf).Underlying().(*types.Slice)
	return ok
}

func ps6079MapType(typeOf types.Type) bool {
	if typeOf == nil {
		return false
	}
	_, ok := types.Unalias(typeOf).Underlying().(*types.Map)
	return ok
}

func ps6079ReferenceFixture(typeOf types.Type) bool {
	if typeOf == nil {
		return false
	}
	switch types.Unalias(typeOf).Underlying().(type) {
	case *types.Slice, *types.Map, *types.Pointer, *types.Interface:
		return true
	default:
		return ps6079UnsafePointerType(typeOf)
	}
}

func ps6079ContainsReferenceFixture(typeOf types.Type) bool {
	if typeOf == nil {
		return false
	}
	switch underlying := types.Unalias(typeOf).Underlying().(type) {
	case *types.Slice, *types.Map, *types.Pointer, *types.Interface:
		return true
	case *types.Basic:
		return underlying.Kind() == types.UnsafePointer
	case *types.Array:
		return ps6079ContainsReferenceFixture(underlying.Elem())
	case *types.Struct:
		for index := range underlying.NumFields() {
			if ps6079ContainsReferenceFixture(underlying.Field(index).Type()) {
				return true
			}
		}
	case *types.Tuple:
		for index := range underlying.Len() {
			if ps6079ContainsReferenceFixture(underlying.At(index).Type()) {
				return true
			}
		}
	}
	return false
}

func ps6079PackageStorageObject(pass *analysis.Pass, object types.Object) bool {
	if object == nil || pass.Pkg == nil || object.Parent() != pass.Pkg.Scope() {
		return false
	}
	_, variable := object.(*types.Var)
	return variable
}

func ps6079AllPackageStorage(pass *analysis.Pass) map[types.Object]bool {
	storage := make(map[types.Object]bool)
	if pass.Pkg == nil {
		return storage
	}
	for _, name := range pass.Pkg.Scope().Names() {
		object := pass.Pkg.Scope().Lookup(name)
		if ps6079PackageStorageObject(pass, object) {
			storage[object] = true
		}
	}
	return storage
}

func ps6079MutatorState(name string) (ps6079FixtureState, bool) {
	mutatorVerb := strings.HasPrefix(name, "fill") || strings.HasPrefix(name, "populate") ||
		strings.HasPrefix(name, "randomize") || strings.HasPrefix(name, "initialize") || strings.HasPrefix(name, "init")
	if state, ok := ps6079NamedFixtureState(name); ok && mutatorVerb {
		return state, true
	}
	if ps6007ContainsAny(name, "randread", "randomize", "fillrandom", "fillnormal", "filluniform", "populaterandom") {
		return ps6079UnrestrictedFixture, true
	}
	return ps6079UnknownFixture, false
}

func ps6079FixtureExpression(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	expression ast.Expr,
	events map[types.Object][]ps6079FixtureEvent,
	functionValues map[types.Object][]ast.Expr,
	before token.Pos,
) (ps6079FixtureState, ps6079FixtureEvent) {
	expression = ps2110Unparen(expression)
	if value := pass.TypesInfo.Types[expression].Value; value != nil {
		sign, numeric := ps6079NumericSign(value)
		if numeric {
			switch sign {
			case -1:
				return ps6079NegativeFixture, ps6079FixtureEvent{state: ps6079NegativeFixture, source: value.ExactString()}
			case 0:
				return ps6079ZeroFixture, ps6079FixtureEvent{state: ps6079ZeroFixture, source: value.ExactString()}
			case 1:
				return ps6079PositiveFixture, ps6079FixtureEvent{state: ps6079PositiveFixture, source: value.ExactString()}
			}
		}
	}
	switch value := expression.(type) {
	case *ast.Ident:
		event := ps6079LatestEvent(events[pass.TypesInfo.ObjectOf(value)], before)
		return event.state, event
	case *ast.UnaryExpr:
		if value.Op == token.SUB {
			state, event := ps6079FixtureExpression(pass, declarations, value.X, events, functionValues, before)
			return ps6079NegatedFixture(pass, value.X, state, event)
		}
	case *ast.CallExpr:
		var returnedEvent ps6079FixtureEvent
		returnedState := ps6079UnknownFixture
		returnedObserved := false
		for object := range ps6079FixtureCallReturnSources(pass, declarations, value, functionValues) {
			event := ps6079LatestEvent(events[object], before)
			if event.source == "" {
				continue
			}
			if !returnedObserved {
				returnedState, returnedEvent, returnedObserved = event.state, event, true
				continue
			}
			if event.state != returnedState {
				return ps6079UnknownFixture, ps6079FixtureEvent{
					state: ps6079UnknownFixture, source: "returned package fixtures have conflicting provenance",
				}
			}
			if event.position > returnedEvent.position {
				returnedEvent = event
			}
		}
		if returnedObserved {
			return returnedState, returnedEvent
		}
		if typedBuiltinName(pass, value.Fun, "make") && ps6079SliceType(pass.TypesInfo.TypeOf(value)) {
			return ps6079ZeroFixture, ps6079FixtureEvent{state: ps6079ZeroFixture, source: "make"}
		}
		if ps6079MathAbsCall(pass, value) {
			if len(value.Args) == 1 {
				state, _ := ps6079FixtureExpression(
					pass, declarations, value.Args[0], events, functionValues, before,
				)
				absolute := ps6079UnknownFixture
				switch state {
				case ps6079PositiveFixture, ps6079NegativeFixture:
					absolute = ps6079PositiveFixture
				case ps6079ZeroFixture:
					absolute = ps6079ZeroFixture
				case ps6079NonnegativeFixture, ps6079NonpositiveFixture:
					absolute = ps6079NonnegativeFixture
				}
				if absolute != ps6079UnknownFixture {
					return absolute, ps6079FixtureEvent{state: absolute, source: exprTextRendered(value.Fun)}
				}
			}
			return ps6079UnknownFixture, ps6079FixtureEvent{
				state: ps6079UnknownFixture, source: "math.Abs with unproven non-NaN input",
			}
		}
		if state, ok := ps6079SemanticFixtureState(pass, value.Fun, functionValues); ok {
			return state, ps6079FixtureEvent{state: state, source: exprTextRendered(value.Fun)}
		}
	case *ast.CompositeLit:
		state, ok := ps6079LiteralState(pass, value)
		if ok {
			return state, ps6079FixtureEvent{state: state, source: "numeric composite literal"}
		}
	}
	return ps6079UnknownFixture, ps6079FixtureEvent{state: ps6079UnknownFixture, source: exprTextRendered(expression)}
}

func ps6079FixtureCallName(pass *analysis.Pass, function ast.Expr) string {
	if callee, _, ok := typedCallee(pass, function); ok && callee != nil {
		name := callee.Name()
		if selector, selected := ps2110Unparen(function).(*ast.SelectorExpr); selected {
			if identifier, packageName := ps2110Unparen(selector.X).(*ast.Ident); packageName {
				if _, imported := pass.TypesInfo.ObjectOf(identifier).(*types.PkgName); imported {
					name = identifier.Name + name
				}
			}
		}
		return ps6007NormalizeName(name)
	}
	return ""
}

func ps6079SemanticFixtureState(
	pass *analysis.Pass,
	function ast.Expr,
	functionValues map[types.Object][]ast.Expr,
) (ps6079FixtureState, bool) {
	callables, resolved := ps6079SemanticCallables(pass, function, functionValues)
	if !resolved {
		return ps6079UnknownFixture, false
	}
	state := ps6079UnknownFixture
	for index, callable := range callables {
		current, ok := ps6079NamedFixtureState(ps6079FixtureCallName(pass, callable))
		if !ok || !ps6079FixtureStatePossibleForCallable(pass, callable, current) || index > 0 && current != state {
			return ps6079UnknownFixture, false
		}
		state = current
	}
	return state, true
}

func ps6079FixtureStatePossibleForCallable(
	pass *analysis.Pass,
	callable ast.Expr,
	state ps6079FixtureState,
) bool {
	if state != ps6079NegativeFixture {
		return true
	}
	typeOf := pass.TypesInfo.TypeOf(callable)
	if typeOf == nil {
		return false
	}
	signature, ok := types.Unalias(typeOf).Underlying().(*types.Signature)
	if !ok || signature.Results().Len() != 1 {
		return false
	}
	return ps6079FixtureStatePossibleForType(signature.Results().At(0).Type(), state)
}

func ps6079FixtureStatePossibleForType(typeOf types.Type, state ps6079FixtureState) bool {
	return state != ps6079NegativeFixture || ps6079FixtureTypeCanBeNegative(typeOf)
}

func ps6079FixtureTypeCanBeNegative(typeOf types.Type) bool {
	if typeOf == nil {
		return false
	}
	switch underlying := types.Unalias(typeOf).Underlying().(type) {
	case *types.Basic:
		return underlying.Info()&(types.IsInteger|types.IsFloat) != 0 && underlying.Info()&types.IsUnsigned == 0
	case *types.Array:
		return ps6079FixtureTypeCanBeNegative(underlying.Elem())
	case *types.Slice:
		return ps6079FixtureTypeCanBeNegative(underlying.Elem())
	case *types.Map:
		return ps6079FixtureTypeCanBeNegative(underlying.Elem())
	case *types.Pointer:
		return ps6079FixtureTypeCanBeNegative(underlying.Elem())
	default:
		return false
	}
}

func ps6079SemanticMutatorState(
	pass *analysis.Pass,
	function ast.Expr,
	functionValues map[types.Object][]ast.Expr,
	stableReferenceObjects map[types.Object]bool,
) (ps6079FixtureState, bool) {
	callables, resolved := ps6079SemanticCallables(pass, function, functionValues)
	if !resolved {
		return ps6079UnknownFixture, false
	}
	originalSelector, directMethodCall := ps2110Unparen(function).(*ast.SelectorExpr)
	state := ps6079UnknownFixture
	for index, callable := range callables {
		if receiver, bound := ps6079BoundMethodReceiver(pass, callable); bound {
			selector, _ := ps2110Unparen(callable).(*ast.SelectorExpr)
			if !directMethodCall || selector != originalSelector {
				identifier, identified := ps2110Unparen(receiver).(*ast.Ident)
				if !identified || !stableReferenceObjects[pass.TypesInfo.ObjectOf(identifier)] {
					return ps6079UnknownFixture, false
				}
			}
			if directMethodCall && selector != originalSelector {
				return ps6079UnknownFixture, false
			}
		}
		current, ok := ps6079MutatorState(ps6079FixtureCallName(pass, callable))
		if !ok || index > 0 && current != state {
			return ps6079UnknownFixture, false
		}
		state = current
	}
	return state, true
}

func ps6079MathAbsCall(pass *analysis.Pass, call *ast.CallExpr) bool {
	callee, _, ok := typedCallee(pass, call.Fun)
	return ok && callee.Pkg() != nil && callee.Pkg().Path() == "math" && callee.Name() == "Abs"
}

func ps6079NamedFixtureState(name string) (ps6079FixtureState, bool) {
	name = ps6007NormalizeName(name)
	switch {
	case ps6007ContainsAny(name, "nonnegative", "nonneg"):
		return ps6079NonnegativeFixture, true
	case ps6007ContainsAny(name, "nonpositive", "nonpos"):
		return ps6079NonpositiveFixture, true
	case ps6007ContainsAny(name, "zerotensor", "zeroslike", "newzeros"):
		return ps6079ZeroFixture, true
	case ps6007ContainsAny(name, "randnorm", "normfloat", "gaussian", "normal", "random", "uniform", "randtensor"):
		return ps6079UnrestrictedFixture, true
	case ps6007ContainsAny(name, "randfloat32", "randfloat64"):
		return ps6079NonnegativeFixture, true
	case strings.Contains(name, "positive"):
		return ps6079PositiveFixture, true
	case strings.Contains(name, "negative"):
		return ps6079NegativeFixture, true
	default:
		return ps6079UnknownFixture, false
	}
}

func ps6079LiteralState(pass *analysis.Pass, literal *ast.CompositeLit) (ps6079FixtureState, bool) {
	typeOf := pass.TypesInfo.TypeOf(literal)
	if typeOf == nil {
		return ps6079UnknownFixture, false
	}
	implicitZero := false
	switch underlying := types.Unalias(typeOf).Underlying().(type) {
	case *types.Slice:
	case *types.Array:
		implicitZero = underlying.Len() > int64(len(literal.Elts))
	default:
		return ps6079UnknownFixture, false
	}
	if len(literal.Elts) == 0 {
		return ps6079ZeroFixture, true
	}
	allNonnegative, allPositive := true, true
	allNonpositive, allNegative := true, true
	for _, element := range literal.Elts {
		expression := element
		if _, keyed := element.(*ast.KeyValueExpr); keyed {
			return ps6079UnknownFixture, false
		}
		value := pass.TypesInfo.Types[ps2110Unparen(expression)].Value
		if value == nil {
			return ps6079UnknownFixture, false
		}
		sign, numeric := ps6079NumericSign(value)
		if !numeric {
			return ps6079UnknownFixture, false
		}
		allNonnegative = allNonnegative && sign >= 0
		allPositive = allPositive && sign > 0
		allNonpositive = allNonpositive && sign <= 0
		allNegative = allNegative && sign < 0
	}
	if implicitZero {
		allPositive, allNegative = false, false
	}
	switch {
	case allPositive:
		return ps6079PositiveFixture, true
	case allNegative:
		return ps6079NegativeFixture, true
	case allNonnegative && allNonpositive:
		return ps6079ZeroFixture, true
	case allNonnegative:
		return ps6079NonnegativeFixture, true
	case allNonpositive:
		return ps6079NonpositiveFixture, true
	default:
		return ps6079UnrestrictedFixture, true
	}
}

func ps6079LatestEvent(events []ps6079FixtureEvent, before token.Pos) ps6079FixtureEvent {
	result := ps6079FixtureEvent{state: ps6079UnknownFixture}
	for _, event := range events {
		if event.position < before && event.position >= result.position {
			result = event
		}
	}
	for _, event := range events {
		if event.position <= before || !ps6079LoopCarriesEvent(event, before) {
			continue
		}
		event.state = ps6079UnknownFixture
		event.source = "loop-carried " + strings.TrimPrefix(event.source, "conditional ")
		return event
	}
	return result
}

func ps6079LoopCarriesEvent(event ps6079FixtureEvent, before token.Pos) bool {
	for _, loop := range event.loops {
		if loop.start < before && before < loop.end {
			return true
		}
	}
	return false
}

func ps6079PositionInLoop(body *ast.BlockStmt, position token.Pos) bool {
	inside := false
	ast.Inspect(body, func(node ast.Node) bool {
		if inside {
			return false
		}
		switch loop := node.(type) {
		case *ast.ForStmt:
			inside = loop.Pos() < position && position < loop.End()
		case *ast.RangeStmt:
			inside = loop.Pos() < position && position < loop.End()
		}
		return !inside
	})
	return inside
}

func ps6079ParameterMayMutate(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	function *types.Func,
	parameter int,
	visiting map[ps6079ParameterKey]bool,
) bool {
	declaration := declarations[function]
	signature, ok := function.Type().(*types.Signature)
	if declaration == nil || !ok || parameter < 0 || parameter >= signature.Params().Len() ||
		!ps6079ContainsReferenceFixture(signature.Params().At(parameter).Type()) {
		return declaration == nil
	}
	key := ps6079ParameterKey{function: function, parameter: parameter}
	if visiting[key] {
		return true
	}
	visiting[key] = true
	defer delete(visiting, key)
	object := signature.Params().At(parameter)
	aliases := map[types.Object]bool{object: true}
	mutates := false
	astutil.WithStack(declaration.Body, func(node ast.Node, stack []ast.Node) bool {
		if mutates {
			return false
		}
		if literal, nested := node.(*ast.FuncLit); nested {
			mutates = ps6079NodeUsesAlias(pass, literal.Body, aliases)
			return false
		}
		switch value := node.(type) {
		case *ast.AssignStmt:
			mutates = ps6079AssignmentMutatesAlias(pass, value, ps6079ConditionalAliasAssignment(value, stack), aliases)
		case *ast.ValueSpec:
			mutates = ps6079ValueSpecMutatesAlias(pass, value, aliases)
		case *ast.RangeStmt:
			ps6079RangeAliases(pass, value, aliases)
		case *ast.IncDecStmt:
			mutates = ps6079WritesThroughAlias(pass, value.X, aliases)
		case *ast.SendStmt:
			mutates = ps6079NodeUsesAlias(pass, value.Value, aliases)
		case *ast.CallExpr:
			mutates = ps6079CallMayMutateAlias(pass, declarations, value, aliases, visiting)
		}
		return !mutates
	})
	return mutates
}

func ps6079ConditionalAliasAssignment(assignment *ast.AssignStmt, stack []ast.Node) bool {
	return ps6079ConditionalControlFlow(assignment, stack)
}

func ps6079ConditionalControlFlow(node ast.Node, stack []ast.Node) bool {
	child := node
	for index := len(stack) - 1; index >= 0; index-- {
		switch control := stack[index].(type) {
		case *ast.IfStmt:
			if child == control.Body || child == control.Else {
				return true
			}
		case *ast.ForStmt:
			if child == control.Body || child == control.Post {
				return true
			}
		case *ast.RangeStmt:
			if child == control.Body {
				return true
			}
		case *ast.SwitchStmt:
			if child == control.Body {
				return true
			}
		case *ast.TypeSwitchStmt:
			if child == control.Body {
				return true
			}
		case *ast.SelectStmt:
			if child == control.Body {
				return true
			}
		}
		child = stack[index]
	}
	return false
}

func ps6079RangeAliases(pass *analysis.Pass, statement *ast.RangeStmt, aliases map[types.Object]bool) {
	if !ps6079NodeUsesAlias(pass, statement.X, aliases) {
		return
	}
	for _, expression := range []ast.Expr{statement.Key, statement.Value} {
		identifier, ok := ps2110Unparen(expression).(*ast.Ident)
		if !ok || identifier.Name == "_" || !ps6079ReferenceFixture(pass.TypesInfo.TypeOf(identifier)) {
			continue
		}
		if object := pass.TypesInfo.ObjectOf(identifier); object != nil {
			aliases[object] = true
		}
	}
}

func ps6079AssignmentMutatesAlias(
	pass *analysis.Pass,
	assignment *ast.AssignStmt,
	conditional bool,
	aliases map[types.Object]bool,
) bool {
	if len(assignment.Lhs) != len(assignment.Rhs) {
		for _, expression := range slices.Concat(assignment.Lhs, assignment.Rhs) {
			if ps6079NodeUsesAlias(pass, expression, aliases) {
				return true
			}
		}
		return false
	}
	rightAliases := make([]bool, len(assignment.Rhs))
	for index, right := range assignment.Rhs {
		if ps6079CompositeCapturesAlias(pass, right, aliases) {
			return true
		}
		rightAliases[index] = aliases[ps6079RootObject(pass, right)] && ps6079ReferenceFixture(pass.TypesInfo.TypeOf(right))
	}
	for index, left := range assignment.Lhs {
		right := assignment.Rhs[index]
		if identifier, ok := ps2110Unparen(left).(*ast.Ident); ok {
			leftObject := pass.TypesInfo.ObjectOf(identifier)
			if identifier.Name == "_" || leftObject == nil {
				continue
			}
			if rightAliases[index] {
				if leftObject.Parent() == pass.Pkg.Scope() {
					return true
				}
				aliases[leftObject] = true
			} else if !conditional && leftObject != ps6079RootObject(pass, right) {
				delete(aliases, leftObject)
			}
			continue
		}
		if rightAliases[index] || ps6079WritesThroughAlias(pass, left, aliases) {
			return true
		}
	}
	return false
}

func ps6079ValueSpecMutatesAlias(pass *analysis.Pass, specification *ast.ValueSpec, aliases map[types.Object]bool) bool {
	if len(specification.Names) != len(specification.Values) {
		return false
	}
	for index, name := range specification.Names {
		if name.Name == "_" {
			continue
		}
		value := specification.Values[index]
		if ps6079CompositeCapturesAlias(pass, value, aliases) {
			return true
		}
		if aliases[ps6079RootObject(pass, value)] && ps6079ReferenceFixture(pass.TypesInfo.TypeOf(value)) {
			aliases[pass.TypesInfo.Defs[name]] = true
		}
	}
	return false
}

func ps6079CompositeCapturesAlias(pass *analysis.Pass, expression ast.Expr, aliases map[types.Object]bool) bool {
	captures := false
	ast.Inspect(expression, func(node ast.Node) bool {
		if captures {
			return false
		}
		literal, ok := node.(*ast.CompositeLit)
		if !ok {
			return true
		}
		captures = ps6079NodeUsesAlias(pass, literal, aliases)
		return !captures
	})
	return captures
}

func ps6079WritesThroughAlias(pass *analysis.Pass, expression ast.Expr, aliases map[types.Object]bool) bool {
	expression = ps2110Unparen(expression)
	switch value := expression.(type) {
	case *ast.IndexExpr:
		return aliases[ps6079RootObject(pass, value.X)]
	case *ast.StarExpr:
		return aliases[ps6079RootObject(pass, value.X)]
	case *ast.SelectorExpr:
		return aliases[ps6079RootObject(pass, value.X)]
	default:
		return false
	}
}

func ps6079CallMayMutateAlias(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	call *ast.CallExpr,
	aliases map[types.Object]bool,
	visiting map[ps6079ParameterKey]bool,
) bool {
	if identifier, ok := ps2110Unparen(call.Fun).(*ast.Ident); ok {
		if builtin, ok := pass.TypesInfo.ObjectOf(identifier).(*types.Builtin); ok {
			return ps6079BuiltinMayMutateAlias(pass, builtin.Name(), call.Args, aliases)
		}
		if _, conversion := pass.TypesInfo.ObjectOf(identifier).(*types.TypeName); conversion {
			return false
		}
	}
	if selector, ok := ps2110Unparen(call.Fun).(*ast.SelectorExpr); ok {
		if _, conversion := pass.TypesInfo.ObjectOf(selector.Sel).(*types.TypeName); conversion {
			return false
		}
		if ps6079NodeUsesAlias(pass, selector.X, aliases) {
			return true
		}
	}
	callee, _, known := typedCallee(pass, call.Fun)
	for index, argument := range call.Args {
		if !ps6079NodeUsesAlias(pass, argument, aliases) {
			continue
		}
		// A reference-bearing result may be the argument itself (possibly nested
		// in an aggregate or tuple). Without an interprocedural return-alias proof,
		// treating it as fresh would miss writes through aliases returned by
		// helpers such as identity.
		if ps6079ContainsReferenceFixture(pass.TypesInfo.TypeOf(call)) {
			return true
		}
		if !known || callee.Pkg() == nil || callee.Pkg().Path() != pass.Pkg.Path() || declarations[callee] == nil {
			return true
		}
		signature, ok := callee.Type().(*types.Signature)
		if !ok || index >= signature.Params().Len() {
			return true
		}
		if ps6079ParameterMayMutate(pass, declarations, callee, index, visiting) {
			return true
		}
	}
	return false
}

func ps6079BuiltinMayMutateAlias(pass *analysis.Pass, name string, arguments []ast.Expr, aliases map[types.Object]bool) bool {
	for index, argument := range arguments {
		if !ps6079NodeUsesAlias(pass, argument, aliases) {
			continue
		}
		switch name {
		case "len", "cap", "min", "max", "real", "imag", "complex":
			continue
		case "copy":
			return index == 0
		default:
			return true
		}
	}
	return false
}

func ps6079NodeUsesAlias(pass *analysis.Pass, node ast.Node, aliases map[types.Object]bool) bool {
	uses := false
	ast.Inspect(node, func(child ast.Node) bool {
		if uses {
			return false
		}
		identifier, ok := child.(*ast.Ident)
		if ok && aliases[pass.TypesInfo.ObjectOf(identifier)] {
			uses = true
		}
		return !uses
	})
	return uses
}

func ps6079RootObject(pass *analysis.Pass, expression ast.Expr) types.Object {
	expression = ps2110Unparen(expression)
	switch value := expression.(type) {
	case *ast.Ident:
		return pass.TypesInfo.ObjectOf(value)
	case *ast.IndexExpr:
		return ps6079RootObject(pass, value.X)
	case *ast.SliceExpr:
		return ps6079RootObject(pass, value.X)
	case *ast.SelectorExpr:
		return ps6079RootObject(pass, value.X)
	case *ast.TypeAssertExpr:
		return ps6079RootObject(pass, value.X)
	case *ast.CallExpr:
		if len(value.Args) == 1 && ps6079ConversionPreservesReference(pass, value) {
			return ps6079RootObject(pass, value.Args[0])
		}
	case *ast.UnaryExpr:
		if value.Op == token.AND {
			return ps6079RootObject(pass, value.X)
		}
	case *ast.StarExpr:
		return ps6079RootObject(pass, value.X)
	}
	return nil
}

func ps6079ConversionPreservesReference(pass *analysis.Pass, call *ast.CallExpr) bool {
	if len(call.Args) != 1 || !ps6079TypeConversion(pass, call.Fun) {
		return false
	}
	destination := pass.TypesInfo.TypeOf(call)
	source := pass.TypesInfo.TypeOf(call.Args[0])
	if ps6079ReferenceFixture(destination) && ps6079ReferenceFixture(source) {
		return true
	}
	return ps6079UnsafePointerType(destination) &&
		(ps6079PointerType(source) || ps6079UintptrType(source)) ||
		ps6079UnsafePointerType(source) &&
			(ps6079PointerType(destination) || ps6079UintptrType(destination))
}

func ps6079TypeConversion(pass *analysis.Pass, expression ast.Expr) bool {
	if typeAndValue, ok := pass.TypesInfo.Types[expression]; ok && typeAndValue.IsType() {
		return true
	}
	switch function := ps2110Unparen(expression).(type) {
	case *ast.Ident:
		_, conversion := pass.TypesInfo.ObjectOf(function).(*types.TypeName)
		return conversion
	case *ast.SelectorExpr:
		_, conversion := pass.TypesInfo.ObjectOf(function.Sel).(*types.TypeName)
		return conversion
	default:
		return false
	}
}

func ps6079UnsafePointerType(typeOf types.Type) bool {
	if typeOf == nil {
		return false
	}
	basic, ok := types.Unalias(typeOf).Underlying().(*types.Basic)
	return ok && basic.Kind() == types.UnsafePointer
}

func ps6079UintptrType(typeOf types.Type) bool {
	if typeOf == nil {
		return false
	}
	basic, ok := types.Unalias(typeOf).Underlying().(*types.Basic)
	return ok && basic.Kind() == types.Uintptr
}

func ps6079IntegerType(typeOf types.Type) bool {
	if typeOf == nil {
		return false
	}
	basic, ok := types.Unalias(typeOf).Underlying().(*types.Basic)
	return ok && basic.Info()&types.IsInteger != 0
}

func ps6079NumericSign(value constant.Value) (int, bool) {
	if value == nil || value.Kind() != constant.Int && value.Kind() != constant.Float {
		return 0, false
	}
	return constant.Sign(value), true
}

func ps6079FlipFixture(state ps6079FixtureState) ps6079FixtureState {
	switch state {
	case ps6079NonnegativeFixture:
		return ps6079NonpositiveFixture
	case ps6079PositiveFixture:
		return ps6079NegativeFixture
	case ps6079NonpositiveFixture:
		return ps6079NonnegativeFixture
	case ps6079NegativeFixture:
		return ps6079PositiveFixture
	default:
		return state
	}
}

func ps6079NegatedFixture(pass *analysis.Pass, operand ast.Expr, state ps6079FixtureState, event ps6079FixtureEvent) (ps6079FixtureState, ps6079FixtureEvent) {
	typeOf := pass.TypesInfo.TypeOf(ps2110Unparen(operand))
	if typeOf == nil {
		return ps6079UnknownFixture, ps6079FixtureEvent{state: ps6079UnknownFixture, source: "unproven unary negation"}
	}
	basic, ok := types.Unalias(typeOf).Underlying().(*types.Basic)
	if !ok || basic.Info()&(types.IsInteger|types.IsFloat) == 0 || basic.Info()&types.IsUnsigned != 0 ||
		basic.Info()&types.IsInteger != 0 && (state == ps6079NegativeFixture || state == ps6079NonpositiveFixture) {
		return ps6079UnknownFixture, ps6079FixtureEvent{state: ps6079UnknownFixture, source: "unproven unary negation"}
	}
	event.state = ps6079FlipFixture(state)
	if event.source != "" {
		event.source = "-(" + event.source + ")"
	}
	return event.state, event
}

func ps6079FixtureSatisfies(state ps6079FixtureState, relation ps6079Relation) bool {
	switch relation {
	case ps6079AtLeastZero:
		return state == ps6079ZeroFixture || state == ps6079NonnegativeFixture || state == ps6079PositiveFixture
	case ps6079AboveZero:
		return state == ps6079PositiveFixture
	case ps6079AtMostZero:
		return state == ps6079ZeroFixture || state == ps6079NonpositiveFixture || state == ps6079NegativeFixture
	case ps6079BelowZero:
		return state == ps6079NegativeFixture
	case ps6079ExactlyZero:
		return state == ps6079ZeroFixture
	default:
		return false
	}
}

func ps6079RelationText(relation ps6079Relation) string {
	switch relation {
	case ps6079AtLeastZero:
		return ">= 0"
	case ps6079AboveZero:
		return "> 0"
	case ps6079AtMostZero:
		return "<= 0"
	case ps6079BelowZero:
		return "< 0"
	case ps6079ExactlyZero:
		return "== 0"
	default:
		return "a guarded sign domain"
	}
}

func ps6079FixtureStateText(state ps6079FixtureState) string {
	switch state {
	case ps6079UnrestrictedFixture:
		return "unrestricted"
	case ps6079ZeroFixture:
		return "provably zero"
	case ps6079NonnegativeFixture:
		return "provably nonnegative"
	case ps6079PositiveFixture:
		return "provably positive"
	case ps6079NonpositiveFixture:
		return "provably nonpositive"
	case ps6079NegativeFixture:
		return "provably negative"
	default:
		return "of unknown sign"
	}
}

func ps6079HasRouteProof(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	benchmark *ast.FuncDecl,
	workload *ast.CallExpr,
	route ps6079Route,
) bool {
	if !route.uniqueContributor {
		return false
	}
	proved := false
	astutil.WithStack(benchmark.Body, func(node ast.Node, stack []ast.Node) bool {
		if proved {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		conditional := ps6079ConditionalFixtureEvent(stack)
		switch value := node.(type) {
		case *ast.IfStmt:
			observation, counterText, missing := ps6079MissingRouteCounter(pass, declarations, value.Cond, route)
			if !conditional && value.Pos() > workload.Pos() && missing &&
				ps6079CounterMatchesSingleGate(counterText, route) &&
				ps6079AssertionMustExecute(pass, benchmark, workload.Pos(), value.Cond.Pos()) && ps6079BenchmarkFailureBranch(pass, benchmark, value) &&
				ps6079RouteCounterReset(pass, declarations, benchmark, workload, value.Body.Pos(), observation, route) {
				proved = true
				return false
			}
		}
		return true
	})
	return proved
}

func ps6079AssertionMustExecute(pass *analysis.Pass, benchmark *ast.FuncDecl, workload, assertion token.Pos) bool {
	return ps6079PositionPostDominates(pass, benchmark.Body, workload, assertion)
}

func ps6079MissingRouteCounter(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	expression ast.Expr,
	route ps6079Route,
) (ps6079CounterObservation, string, bool) {
	expression = ps2110Unparen(expression)
	comparison, ok := expression.(*ast.BinaryExpr)
	if !ok {
		return ps6079CounterObservation{}, "", false
	}
	var counter ast.Expr
	switch comparison.Op {
	case token.EQL:
		if ps6079Zero(pass, comparison.Y) {
			counter = comparison.X
		} else if ps6079Zero(pass, comparison.X) {
			counter = comparison.Y
		}
	case token.LEQ:
		if ps6079Zero(pass, comparison.Y) {
			counter = comparison.X
		}
	case token.GEQ:
		if ps6079Zero(pass, comparison.X) {
			counter = comparison.Y
		}
	}
	if counter == nil {
		return ps6079CounterObservation{}, "", false
	}
	observation, observed, resolved := ps6079ResolveCounterObservation(pass, declarations, counter)
	if !resolved {
		return ps6079CounterObservation{}, "", false
	}
	text := exprTextRendered(observed)
	if !ps6079RouteEvidenceForRoute(text, route) {
		return ps6079CounterObservation{}, "", false
	}
	return observation, text, true
}

func ps6079CounterMatchesSingleGate(text string, route ps6079Route) bool {
	matches := 0
	for _, reachable := range route.reachableGates {
		if !reachable.contributors[route.target] {
			continue
		}
		candidate := route
		candidate.gate = reachable.gate
		if ps6079RouteEvidenceForRoute(text, candidate) {
			matches++
		}
	}
	return matches == 1
}

func ps6079ResolveCounterObservation(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	expression ast.Expr,
) (ps6079CounterObservation, ast.Expr, bool) {
	expression = ps2110Unparen(expression)
	call, called := expression.(*ast.CallExpr)
	if !called {
		reference, ok := ps6079CounterObject(pass, expression)
		return ps6079CounterObservation{reference: reference}, expression, ok
	}
	selector, selected := ps2110Unparen(call.Fun).(*ast.SelectorExpr)
	if !selected || !ps6007ContainsAny(ps6007NormalizeName(selector.Sel.Name), "load", "value", "count") {
		return ps6079CounterObservation{}, nil, false
	}
	callee, _, known := typedCallee(pass, call.Fun)
	if !known || callee.Pkg() == nil {
		return ps6079CounterObservation{}, nil, false
	}
	if callee.Pkg().Path() == "sync/atomic" {
		storage, ok := ps6079AtomicStorageExpression(pass, call, "load")
		if !ok {
			return ps6079CounterObservation{}, nil, false
		}
		reference, resolved := ps6079CounterObject(pass, storage)
		return ps6079CounterObservation{reference: reference, atomic: true}, storage, resolved
	}
	if callee.Pkg().Path() != pass.Pkg.Path() || len(call.Args) != 0 {
		return ps6079CounterObservation{}, nil, false
	}
	declaration := declarations[callee]
	storage, ok := ps6079CustomCounterReadStorage(pass, declaration)
	if !ok {
		return ps6079CounterObservation{}, nil, false
	}
	reference, resolved := ps6079CounterObject(pass, selector.X)
	return ps6079CounterObservation{reference: reference, storage: storage}, selector.X, resolved
}

func ps6079CustomCounterReadStorage(pass *analysis.Pass, declaration *ast.FuncDecl) ([]types.Object, bool) {
	if declaration == nil || declaration.Body == nil || len(declaration.Body.List) != 1 {
		return nil, false
	}
	returned, ok := declaration.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(returned.Results) != 1 {
		return nil, false
	}
	receiver := ps6079FunctionReceiverObject(pass, declaration)
	if receiver == nil {
		return nil, false
	}
	result := ps2110Unparen(returned.Results[0])
	if call, ok := result.(*ast.CallExpr); ok {
		storage, atomic := ps6079AtomicStorageExpression(pass, call, "load")
		if !atomic {
			return nil, false
		}
		return ps6079RelativeCounterFields(pass, storage, receiver)
	}
	return ps6079RelativeCounterFields(pass, result, receiver)
}

func ps6079AtomicStorageExpression(pass *analysis.Pass, call *ast.CallExpr, operation string) (ast.Expr, bool) {
	callee, _, known := typedCallee(pass, call.Fun)
	name := ""
	if known {
		name = ps6007NormalizeName(callee.Name())
	}
	if !known || callee.Pkg() == nil || callee.Pkg().Path() != "sync/atomic" ||
		!strings.Contains(name, operation) || operation == "swap" && strings.Contains(name, "compare") {
		return nil, false
	}
	selector, ok := ps2110Unparen(call.Fun).(*ast.SelectorExpr)
	if !ok {
		return nil, false
	}
	if packageName, ok := ps2110Unparen(selector.X).(*ast.Ident); ok {
		if _, imported := pass.TypesInfo.ObjectOf(packageName).(*types.PkgName); imported {
			if len(call.Args) == 0 {
				return nil, false
			}
			return call.Args[0], true
		}
	}
	if operation == "load" && len(call.Args) != 0 {
		return nil, false
	}
	return selector.X, true
}

func ps6079FunctionReceiverObject(pass *analysis.Pass, declaration *ast.FuncDecl) types.Object {
	if declaration == nil || declaration.Recv == nil || len(declaration.Recv.List) != 1 ||
		len(declaration.Recv.List[0].Names) != 1 {
		return nil
	}
	return pass.TypesInfo.Defs[declaration.Recv.List[0].Names[0]]
}

func ps6079RelativeCounterFields(
	pass *analysis.Pass,
	expression ast.Expr,
	root types.Object,
) ([]types.Object, bool) {
	expression = ps2110Unparen(expression)
	switch value := expression.(type) {
	case *ast.Ident:
		return nil, pass.TypesInfo.ObjectOf(value) == root
	case *ast.SelectorExpr:
		fields, ok := ps6079RelativeCounterFields(pass, value.X, root)
		selection := pass.TypesInfo.Selections[value]
		if !ok || selection == nil || selection.Kind() != types.FieldVal {
			return nil, false
		}
		return append(fields, selection.Obj()), true
	case *ast.StarExpr:
		return ps6079RelativeCounterFields(pass, value.X, root)
	case *ast.UnaryExpr:
		if value.Op == token.AND {
			return ps6079RelativeCounterFields(pass, value.X, root)
		}
	}
	return nil, false
}

func ps6079SameCounterFields(left, right []types.Object) bool {
	return slices.Equal(left, right)
}

func ps6079CounterObject(pass *analysis.Pass, expression ast.Expr) (ps6079CounterReference, bool) {
	expression = ps2110Unparen(expression)
	switch value := expression.(type) {
	case *ast.Ident:
		object := pass.TypesInfo.ObjectOf(value)
		return ps6079CounterReference{root: object}, object != nil
	case *ast.SelectorExpr:
		if packageName, ok := ps2110Unparen(value.X).(*ast.Ident); ok {
			if _, imported := pass.TypesInfo.ObjectOf(packageName).(*types.PkgName); imported {
				object := pass.TypesInfo.ObjectOf(value.Sel)
				return ps6079CounterReference{root: object}, object != nil
			}
		}
		reference, ok := ps6079CounterObject(pass, value.X)
		selection := pass.TypesInfo.Selections[value]
		if !ok || selection == nil || selection.Kind() != types.FieldVal {
			return ps6079CounterReference{}, false
		}
		reference.fields = append(reference.fields, selection.Obj())
		return reference, true
	case *ast.StarExpr:
		return ps6079CounterObject(pass, value.X)
	case *ast.UnaryExpr:
		if value.Op == token.AND {
			return ps6079CounterObject(pass, value.X)
		}
	default:
		return ps6079CounterReference{}, false
	}
	return ps6079CounterReference{}, false
}

func ps6079SameCounter(left, right ps6079CounterReference) bool {
	if left.root == nil || left.root != right.root || len(left.fields) != len(right.fields) {
		return false
	}
	for index := range left.fields {
		if left.fields[index] != right.fields[index] {
			return false
		}
	}
	return true
}

func ps6079RouteCounterReset(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	benchmark *ast.FuncDecl,
	workload *ast.CallExpr,
	assertion token.Pos,
	observation ps6079CounterObservation,
	route ps6079Route,
) bool {
	counter := observation.reference
	resetPosition := token.NoPos
	astutil.WithStack(benchmark.Body, func(node ast.Node, stack []ast.Node) bool {
		if node.Pos() >= workload.Pos() {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		if ps6079ConditionalFixtureEvent(stack) {
			return true
		}
		for _, ancestor := range stack {
			switch ancestor.(type) {
			case *ast.GoStmt, *ast.DeferStmt:
				return true
			}
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if ps6079ResetClearsObservation(pass, declarations, call, observation) {
			resetPosition = call.Pos()
		}
		return true
	})
	if !resetPosition.IsValid() {
		return false
	}
	if !ps6079PositionDominates(benchmark.Body, resetPosition, workload.Pos()) {
		return false
	}
	counterAliases := ps6079CounterAliasesBeforeReset(pass, declarations, benchmark, resetPosition, counter)
	if ps6079PackageMayLaunchCounterInvalidator(pass, declarations, counter, counterAliases) {
		return false
	}
	if ps6079AsynchronousCounterMayInvalidateBefore(
		pass, declarations, benchmark.Body, assertion, counter, counterAliases,
	) {
		return false
	}
	clean := true
	ast.Inspect(benchmark.Body, func(node ast.Node) bool {
		if !clean {
			return false
		}
		if node == nil {
			return true
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		if node.Pos() <= resetPosition || node.Pos() >= assertion {
			return true
		}
		if ps6079NodeMutatesCounter(pass, declarations, node, counter, counterAliases) {
			clean = false
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if call == workload {
			observed := false
			clean = !ps6079WorkloadMayInvalidateRouteProof(
				pass, declarations, route.target, route.gate, counter, counterAliases,
				false, &observed, make(map[*types.Func]bool),
			) && observed
			return clean
		}
		if ps6079CounterCallReadOnly(pass, declarations, call) {
			return true
		}
		if ps6079FunctionArgument(pass, call) {
			clean = false
			return false
		}
		callee, _, known := typedCallee(pass, call.Fun)
		if known {
			clean = !route.contributors[callee] &&
				!ps6079FunctionMayInvalidateRouteProof(pass, declarations, callee, counter, counterAliases, make(map[*types.Func]bool))
			return clean
		}
		if !ps6079BuiltinOrConversion(pass, call.Fun) {
			clean = false
		}
		return true
	})
	return clean
}

func ps6079PackageMayLaunchCounterInvalidator(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	counter ps6079CounterReference,
	aliases map[types.Object]bool,
) bool {
	for function, declaration := range declarations {
		if function.Name() == "init" && ps6079CounterBodyMayLaunchInvalidator(
			pass, declarations, declaration.Body, token.NoPos, counter, aliases, make(map[*types.Func]bool),
		) {
			return true
		}
	}
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok || general.Tok != token.VAR {
				continue
			}
			for _, specification := range general.Specs {
				values, ok := specification.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for _, expression := range values.Values {
					if ps6079InitializerMayLaunchCounterInvalidator(
						pass, declarations, expression, counter, aliases,
					) {
						return true
					}
				}
			}
		}
	}
	return false
}

func ps6079InitializerMayLaunchCounterInvalidator(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	expression ast.Expr,
	counter ps6079CounterReference,
	aliases map[types.Object]bool,
) bool {
	uncertain := false
	ast.Inspect(expression, func(node ast.Node) bool {
		if uncertain {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		uncertain = ps6079CallMayLaunchCounterInvalidator(
			pass, declarations, call, counter, aliases, make(map[*types.Func]bool),
		)
		return !uncertain
	})
	return uncertain
}

func ps6079AsynchronousCounterMayInvalidateBefore(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	body *ast.BlockStmt,
	before token.Pos,
	counter ps6079CounterReference,
	aliases map[types.Object]bool,
) bool {
	return ps6079CounterBodyMayLaunchInvalidator(
		pass, declarations, body, before, counter, aliases, make(map[*types.Func]bool),
	)
}

func ps6079CounterBodyMayLaunchInvalidator(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	body *ast.BlockStmt,
	before token.Pos,
	counter ps6079CounterReference,
	aliases map[types.Object]bool,
	visiting map[*types.Func]bool,
) bool {
	uncertain := false
	astutil.WithStack(body, func(node ast.Node, stack []ast.Node) bool {
		if uncertain {
			return false
		}
		if before.IsValid() && node.Pos() >= before {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		switch value := node.(type) {
		case *ast.GoStmt:
			uncertain = ps6079AsynchronousCallMayInvalidateCounter(
				pass, declarations, value.Call, counter, aliases,
			)
			return false
		case *ast.SendStmt:
			uncertain = ps6079ExpressionReturnsCounterAlias(
				pass, declarations, value.Value, counter, aliases, make(map[*types.Func]bool),
			)
			return !uncertain
		case *ast.AssignStmt:
			if !before.IsValid() {
				ps6079LearnCounterAssignmentAliases(pass, declarations, value, counter, aliases, visiting)
			}
		case *ast.ValueSpec:
			if !before.IsValid() {
				ps6079LearnCounterValueSpecAliases(pass, declarations, value, counter, aliases, visiting)
			}
		case *ast.RangeStmt:
			if !before.IsValid() {
				ps6079LearnCounterRangeAliases(pass, declarations, value, counter, aliases, visiting)
			}
		case *ast.CallExpr:
			for _, ancestor := range stack {
				if _, asynchronous := ancestor.(*ast.GoStmt); asynchronous {
					return false
				}
			}
			uncertain = ps6079CallMayLaunchCounterInvalidator(
				pass, declarations, value, counter, aliases, visiting,
			)
			return !uncertain
		}
		return true
	})
	return uncertain
}

func ps6079CallMayLaunchCounterInvalidator(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	call *ast.CallExpr,
	counter ps6079CounterReference,
	aliases map[types.Object]bool,
	visiting map[*types.Func]bool,
) bool {
	if literal, ok := ps2110Unparen(call.Fun).(*ast.FuncLit); ok {
		return ps6079CounterBodyMayLaunchInvalidator(
			pass, declarations, literal.Body, token.NoPos, counter, aliases, visiting,
		)
	}
	if ps6079FunctionArgument(pass, call) {
		return true
	}
	callee, _, known := typedCallee(pass, call.Fun)
	if !known {
		return !ps6079BuiltinOrConversion(pass, call.Fun) && !ps6079UnsafeCall(pass, call.Fun)
	}
	if callee.Pkg() != nil && callee.Pkg().Path() == "sync/atomic" {
		// Atomic operations are synchronous. Their writes are still audited by
		// the clean-window mutation pass, but they cannot launch a later writer.
		return false
	}
	if callee.Pkg() == nil || pass.Pkg == nil || callee.Pkg().Path() != pass.Pkg.Path() {
		return ps6079NodeMutatesCounter(pass, declarations, call, counter, aliases)
	}
	declaration := declarations[callee]
	if declaration == nil {
		return ps6079NodeMutatesCounter(pass, declarations, call, counter, aliases)
	}
	if visiting[callee] {
		return false
	}
	visiting[callee] = true
	defer delete(visiting, callee)
	if ps6079CounterBodyMayLaunchInvalidator(
		pass, declarations, declaration.Body, token.NoPos, counter, aliases, visiting,
	) {
		return true
	}
	return ps6079NodeMutatesCounter(pass, declarations, call, counter, aliases) &&
		ps6079BodyMayLaunchAsynchronousWork(pass, declarations, declaration.Body, make(map[*types.Func]bool))
}

func ps6079AsynchronousCallMayInvalidateCounter(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	call *ast.CallExpr,
	counter ps6079CounterReference,
	aliases map[types.Object]bool,
) bool {
	if literal, ok := ps2110Unparen(call.Fun).(*ast.FuncLit); ok {
		return ps6079CounterBodyMayInvalidate(pass, declarations, literal.Body, counter, aliases)
	}
	if ps6079NodeMutatesCounter(pass, declarations, call, counter, aliases) || ps6079FunctionArgument(pass, call) {
		return true
	}
	callee, _, known := typedCallee(pass, call.Fun)
	if !known {
		return !ps6079BuiltinOrConversion(pass, call.Fun) && !ps6079UnsafeCall(pass, call.Fun)
	}
	return ps6079FunctionMayInvalidateRouteProof(
		pass, declarations, callee, counter, aliases, make(map[*types.Func]bool),
	)
}

func ps6079CounterBodyMayInvalidate(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	body *ast.BlockStmt,
	counter ps6079CounterReference,
	aliases map[types.Object]bool,
) bool {
	uncertain := false
	astutil.WithStack(body, func(node ast.Node, _ []ast.Node) bool {
		if uncertain {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		switch value := node.(type) {
		case *ast.AssignStmt:
			ps6079LearnCounterAssignmentAliases(
				pass, declarations, value, counter, aliases, make(map[*types.Func]bool),
			)
		case *ast.ValueSpec:
			ps6079LearnCounterValueSpecAliases(
				pass, declarations, value, counter, aliases, make(map[*types.Func]bool),
			)
		case *ast.RangeStmt:
			ps6079LearnCounterRangeAliases(
				pass, declarations, value, counter, aliases, make(map[*types.Func]bool),
			)
		case *ast.SendStmt:
			if ps6079ExpressionReturnsCounterAlias(
				pass, declarations, value.Value, counter, aliases, make(map[*types.Func]bool),
			) {
				uncertain = true
				return false
			}
		}
		if ps6079NodeMutatesCounter(pass, declarations, node, counter, aliases) {
			uncertain = true
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if ps6079CounterCallReadOnly(pass, declarations, call) {
			return true
		}
		if ps6079PanicCall(pass, call) || ps6079FunctionArgument(pass, call) {
			uncertain = true
			return false
		}
		callee, _, known := typedCallee(pass, call.Fun)
		if !known {
			uncertain = !ps6079BuiltinOrConversion(pass, call.Fun)
			return !uncertain
		}
		uncertain = ps6079FunctionMayInvalidateRouteProof(
			pass, declarations, callee, counter, aliases, make(map[*types.Func]bool),
		)
		return !uncertain
	})
	return uncertain
}

func ps6079ResetClearsObservation(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	call *ast.CallExpr,
	observation ps6079CounterObservation,
) bool {
	selector, ok := ps2110Unparen(call.Fun).(*ast.SelectorExpr)
	if !ok {
		return false
	}
	name := ps6007NormalizeName(selector.Sel.Name)
	if !ps6007ContainsAny(name, "store", "swap", "reset") || len(call.Args) == 0 ||
		!ps6079Zero(pass, call.Args[len(call.Args)-1]) {
		return false
	}
	callee, _, known := typedCallee(pass, call.Fun)
	if !known || callee.Pkg() == nil {
		return false
	}
	if callee.Pkg().Path() == "sync/atomic" {
		// CompareAndSwap is conditional: even a zero replacement does not prove
		// that stale route evidence was cleared. Only unconditional Store and
		// Swap operations can establish the reset.
		if !observation.atomic || strings.Contains(name, "compare") {
			return false
		}
		operation := "store"
		if strings.Contains(name, "swap") {
			operation = "swap"
		}
		storage, ok := ps6079AtomicStorageExpression(pass, call, operation)
		if !ok {
			return false
		}
		reference, resolved := ps6079CounterObject(pass, storage)
		return resolved && ps6079SameCounter(reference, observation.reference)
	}
	if observation.atomic || callee.Pkg().Path() != pass.Pkg.Path() || len(call.Args) != 1 {
		return false
	}
	reference, resolved := ps6079CounterObject(pass, selector.X)
	if !resolved || !ps6079SameCounter(reference, observation.reference) {
		return false
	}
	storage, ok := ps6079CustomCounterResetStorage(pass, declarations[callee])
	return ok && ps6079SameCounterFields(storage, observation.storage)
}

func ps6079CustomCounterResetStorage(pass *analysis.Pass, declaration *ast.FuncDecl) ([]types.Object, bool) {
	if declaration == nil || declaration.Body == nil || len(declaration.Body.List) != 1 {
		return nil, false
	}
	receiver := ps6079FunctionReceiverObject(pass, declaration)
	function, _ := pass.TypesInfo.Defs[declaration.Name].(*types.Func)
	if receiver == nil || function == nil {
		return nil, false
	}
	signature, ok := function.Type().(*types.Signature)
	if !ok || signature.Recv() == nil || !ps6079PointerType(signature.Recv().Type()) || signature.Params().Len() != 1 {
		return nil, false
	}
	parameter := signature.Params().At(0)
	switch statement := declaration.Body.List[0].(type) {
	case *ast.AssignStmt:
		if statement.Tok != token.ASSIGN || len(statement.Lhs) != 1 || len(statement.Rhs) != 1 {
			return nil, false
		}
		identifier, ok := ps2110Unparen(statement.Rhs[0]).(*ast.Ident)
		if !ok || pass.TypesInfo.ObjectOf(identifier) != parameter {
			return nil, false
		}
		return ps6079RelativeCounterFields(pass, statement.Lhs[0], receiver)
	case *ast.ExprStmt:
		call, ok := ps2110Unparen(statement.X).(*ast.CallExpr)
		if !ok || len(call.Args) == 0 {
			return nil, false
		}
		identifier, ok := ps2110Unparen(call.Args[len(call.Args)-1]).(*ast.Ident)
		if !ok || pass.TypesInfo.ObjectOf(identifier) != parameter {
			return nil, false
		}
		name := ps6007NormalizeName(ps6074CalledName(call.Fun))
		operation := "store"
		if strings.Contains(name, "swap") {
			operation = "swap"
		}
		storage, ok := ps6079AtomicStorageExpression(pass, call, operation)
		if !ok {
			return nil, false
		}
		return ps6079RelativeCounterFields(pass, storage, receiver)
	}
	return nil, false
}

func ps6079NodeMutatesCounter(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	node ast.Node,
	counter ps6079CounterReference,
	aliases map[types.Object]bool,
) bool {
	overlaps := func(expression ast.Expr) bool {
		reference, ok := ps6079CounterObject(pass, expression)
		return ok && ps6079CounterReferencesOverlap(reference, counter) || aliases[ps6079RootObject(pass, expression)]
	}
	switch value := node.(type) {
	case *ast.AssignStmt:
		for _, expression := range value.Lhs {
			if overlaps(expression) {
				return true
			}
		}
		for _, expression := range value.Rhs {
			if ps6079ExpressionAliasesCounter(pass, expression, counter, aliases) {
				return true
			}
		}
	case *ast.ValueSpec:
		for _, expression := range value.Values {
			if ps6079ExpressionAliasesCounter(pass, expression, counter, aliases) {
				return true
			}
		}
	case *ast.IncDecStmt:
		return overlaps(value.X)
	case *ast.SendStmt:
		return ps6079ExpressionAliasesCounter(pass, value.Value, counter, aliases)
	case *ast.ReturnStmt:
		for _, expression := range value.Results {
			if ps6079ExpressionAliasesCounter(pass, expression, counter, aliases) {
				return true
			}
		}
	case *ast.CallExpr:
		if selector, ok := ps2110Unparen(value.Fun).(*ast.SelectorExpr); ok && overlaps(selector.X) {
			name := ps6007NormalizeName(selector.Sel.Name)
			if ps6007ContainsAny(name, "load", "value", "count") {
				return !ps6079CounterCallReadOnly(pass, declarations, value)
			}
			return true
		}
		for _, argument := range value.Args {
			if ps6079ExpressionAliasesCounter(pass, argument, counter, aliases) {
				return true
			}
		}
	}
	return false
}

func ps6079CounterCallReadOnly(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	call *ast.CallExpr,
) bool {
	callee, _, known := typedCallee(pass, call.Fun)
	if !known || callee.Pkg() == nil {
		return false
	}
	if callee.Pkg().Path() == "sync/atomic" {
		_, ok := ps6079AtomicStorageExpression(pass, call, "load")
		return ok
	}
	if callee.Pkg().Path() != pass.Pkg.Path() || len(call.Args) != 0 {
		return false
	}
	_, ok := ps6079CustomCounterReadStorage(pass, declarations[callee])
	return ok
}

func ps6079CounterReferencesOverlap(left, right ps6079CounterReference) bool {
	if left.root == nil || left.root != right.root {
		return false
	}
	shared := min(len(left.fields), len(right.fields))
	for index := range shared {
		if left.fields[index] != right.fields[index] {
			return false
		}
	}
	return true
}

func ps6079CounterAliasesBeforeReset(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	benchmark *ast.FuncDecl,
	reset token.Pos,
	counter ps6079CounterReference,
) map[types.Object]bool {
	aliases := make(map[types.Object]bool)
	changed := true
	for changed {
		changed = false
		for _, file := range pass.Files {
			for _, declaration := range file.Decls {
				general, ok := declaration.(*ast.GenDecl)
				if !ok || general.Tok != token.VAR {
					continue
				}
				for _, specification := range general.Specs {
					values, ok := specification.(*ast.ValueSpec)
					if !ok {
						continue
					}
					if len(values.Names) != len(values.Values) {
						if len(values.Values) == 1 && ps6079ExpressionReturnsCounterAlias(
							pass, declarations, values.Values[0], counter, aliases, make(map[*types.Func]bool),
						) {
							for _, name := range values.Names {
								object := pass.TypesInfo.Defs[name]
								if object != nil && ps6079ContainsReferenceFixture(object.Type()) && !aliases[object] {
									aliases[object] = true
									changed = true
								}
							}
						}
						continue
					}
					for index, name := range values.Names {
						object := pass.TypesInfo.Defs[name]
						if object != nil && !aliases[object] &&
							ps6079ExpressionReturnsCounterAlias(
								pass, declarations, values.Values[index], counter, aliases, make(map[*types.Func]bool),
							) {
							aliases[object] = true
							changed = true
						}
					}
				}
			}
		}
	}
	astutil.WithStack(benchmark.Body, func(node ast.Node, stack []ast.Node) bool {
		if node.Pos() >= reset {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		switch value := node.(type) {
		case *ast.AssignStmt:
			if len(value.Lhs) != len(value.Rhs) {
				if len(value.Rhs) == 1 && ps6079ExpressionReturnsCounterAlias(
					pass, declarations, value.Rhs[0], counter, aliases, make(map[*types.Func]bool),
				) {
					for _, expression := range value.Lhs {
						if object := ps6079RootObject(pass, expression); object != nil &&
							ps6079ContainsReferenceFixture(object.Type()) {
							aliases[object] = true
						}
					}
				}
				return true
			}
			rightAliases := make([]bool, len(value.Rhs))
			for index, expression := range value.Rhs {
				rightAliases[index] = ps6079ExpressionReturnsCounterAlias(
					pass, declarations, expression, counter, aliases, make(map[*types.Func]bool),
				)
			}
			conditional := ps6079ConditionalControlFlow(value, stack)
			for index, expression := range value.Lhs {
				identifier, ok := ps2110Unparen(expression).(*ast.Ident)
				if !ok {
					// A counter pointer stored into an existing aggregate remains
					// reachable through its root. Retain that fact conservatively:
					// overwriting one member cannot prove that every other member no
					// longer aliases the observed counter.
					if root := ps6079RootObject(pass, expression); root != nil && rightAliases[index] {
						aliases[root] = true
					}
					continue
				}
				if identifier.Name == "_" {
					continue
				}
				object := pass.TypesInfo.ObjectOf(identifier)
				if object == nil {
					continue
				}
				if rightAliases[index] {
					aliases[object] = true
				} else if !conditional {
					delete(aliases, object)
				}
			}
		case *ast.ValueSpec:
			if len(value.Names) != len(value.Values) {
				if len(value.Values) == 1 && ps6079ExpressionReturnsCounterAlias(
					pass, declarations, value.Values[0], counter, aliases, make(map[*types.Func]bool),
				) {
					for _, name := range value.Names {
						if object := pass.TypesInfo.Defs[name]; object != nil &&
							ps6079ContainsReferenceFixture(object.Type()) {
							aliases[object] = true
						}
					}
				}
				return true
			}
			for index, name := range value.Names {
				object := pass.TypesInfo.Defs[name]
				if object != nil && ps6079ExpressionReturnsCounterAlias(
					pass, declarations, value.Values[index], counter, aliases, make(map[*types.Func]bool),
				) {
					aliases[object] = true
				}
			}
		case *ast.RangeStmt:
			ps6079LearnCounterRangeAliases(
				pass, declarations, value, counter, aliases, make(map[*types.Func]bool),
			)
		}
		return true
	})
	return aliases
}

func ps6079ExpressionAliasesCounter(
	pass *analysis.Pass,
	expression ast.Expr,
	counter ps6079CounterReference,
	aliases map[types.Object]bool,
) bool {
	found := false
	ast.Inspect(expression, func(node ast.Node) bool {
		if found {
			return false
		}
		candidate, ok := node.(ast.Expr)
		if !ok || !ps6079ReferenceFixture(pass.TypesInfo.TypeOf(candidate)) {
			return true
		}
		if aliases[ps6079RootObject(pass, candidate)] {
			found = true
			return false
		}
		reference, ok := ps6079CounterObject(pass, candidate)
		found = ok && ps6079CounterReferencesOverlap(reference, counter)
		return !found
	})
	return found
}

func ps6079LearnCounterAssignmentAliases(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	assignment *ast.AssignStmt,
	counter ps6079CounterReference,
	aliases map[types.Object]bool,
	visiting map[*types.Func]bool,
) {
	if assignment == nil || len(assignment.Rhs) == 0 {
		return
	}
	if len(assignment.Lhs) != len(assignment.Rhs) {
		if len(assignment.Rhs) == 1 && ps6079ExpressionReturnsCounterAlias(
			pass, declarations, assignment.Rhs[0], counter, aliases, visiting,
		) {
			for _, left := range assignment.Lhs {
				if object := ps6079RootObject(pass, left); object != nil &&
					ps6079ContainsReferenceFixture(object.Type()) {
					aliases[object] = true
				}
			}
		}
		return
	}
	for index, left := range assignment.Lhs {
		if !ps6079ExpressionReturnsCounterAlias(
			pass, declarations, assignment.Rhs[index], counter, aliases, visiting,
		) {
			continue
		}
		if object := ps6079RootObject(pass, left); object != nil {
			aliases[object] = true
		}
	}
}

func ps6079LearnCounterValueSpecAliases(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	values *ast.ValueSpec,
	counter ps6079CounterReference,
	aliases map[types.Object]bool,
	visiting map[*types.Func]bool,
) {
	if values == nil || len(values.Values) == 0 {
		return
	}
	if len(values.Names) != len(values.Values) {
		if len(values.Values) == 1 && ps6079ExpressionReturnsCounterAlias(
			pass, declarations, values.Values[0], counter, aliases, visiting,
		) {
			for _, name := range values.Names {
				if object := pass.TypesInfo.Defs[name]; object != nil &&
					ps6079ContainsReferenceFixture(object.Type()) {
					aliases[object] = true
				}
			}
		}
		return
	}
	for index, name := range values.Names {
		if ps6079ExpressionReturnsCounterAlias(
			pass, declarations, values.Values[index], counter, aliases, visiting,
		) {
			if object := pass.TypesInfo.Defs[name]; object != nil {
				aliases[object] = true
			}
		}
	}
}

func ps6079LearnCounterRangeAliases(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	statement *ast.RangeStmt,
	counter ps6079CounterReference,
	aliases map[types.Object]bool,
	visiting map[*types.Func]bool,
) {
	if statement == nil {
		return
	}
	yieldsAlias := ps6079ExpressionReturnsCounterAlias(
		pass, declarations, statement.X, counter, aliases, visiting,
	)
	typeOf := pass.TypesInfo.TypeOf(statement.X)
	if typeOf != nil {
		switch underlying := types.Unalias(typeOf).Underlying().(type) {
		case *types.Chan:
			yieldsAlias = yieldsAlias || ps6079ContainsReferenceFixture(underlying.Elem())
		case *types.Signature:
			if underlying.Params().Len() == 1 {
				if yield, ok := types.Unalias(underlying.Params().At(0).Type()).Underlying().(*types.Signature); ok {
					for index := range yield.Params().Len() {
						yieldsAlias = yieldsAlias || ps6079ContainsReferenceFixture(yield.Params().At(index).Type())
					}
				}
			}
		}
	}
	if !yieldsAlias {
		return
	}
	for _, target := range []ast.Expr{statement.Key, statement.Value} {
		if object := ps6079RootObject(pass, target); object != nil &&
			ps6079ContainsReferenceFixture(object.Type()) {
			aliases[object] = true
		}
	}
}

func ps6079ExpressionReturnsCounterAlias(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	expression ast.Expr,
	counter ps6079CounterReference,
	aliases map[types.Object]bool,
	visiting map[*types.Func]bool,
) bool {
	if ps6079ExpressionAliasesCounter(pass, expression, counter, aliases) {
		return true
	}
	call, ok := ps2110Unparen(expression).(*ast.CallExpr)
	if !ok {
		return false
	}
	if ps6079BuiltinOrConversion(pass, call.Fun) {
		for _, argument := range call.Args {
			if ps6079ExpressionReturnsCounterAlias(pass, declarations, argument, counter, aliases, visiting) {
				return true
			}
		}
		return false
	}
	if literal, ok := ps2110Unparen(call.Fun).(*ast.FuncLit); ok {
		return ps6079BodyReturnsCounterAlias(
			pass, declarations, literal.Body, ps6079NamedResultObjects(pass, literal.Type.Results),
			counter, aliases, visiting,
		)
	}
	callee, _, known := typedCallee(pass, call.Fun)
	if !known || callee.Pkg() == nil || pass.Pkg == nil || callee.Pkg().Path() != pass.Pkg.Path() {
		return false
	}
	declaration := declarations[callee]
	if declaration == nil || visiting[callee] {
		return declaration == nil
	}
	visiting[callee] = true
	defer delete(visiting, callee)
	return ps6079BodyReturnsCounterAlias(
		pass, declarations, declaration.Body, ps6079NamedResultObjects(pass, declaration.Type.Results),
		counter, aliases, visiting,
	)
}

func ps6079BodyReturnsCounterAlias(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	body *ast.BlockStmt,
	namedResults []types.Object,
	counter ps6079CounterReference,
	aliases map[types.Object]bool,
	visiting map[*types.Func]bool,
) bool {
	returnsAlias := false
	astutil.WithStack(body, func(node ast.Node, _ []ast.Node) bool {
		if returnsAlias {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		switch value := node.(type) {
		case *ast.AssignStmt:
			ps6079LearnCounterAssignmentAliases(pass, declarations, value, counter, aliases, visiting)
		case *ast.ValueSpec:
			ps6079LearnCounterValueSpecAliases(pass, declarations, value, counter, aliases, visiting)
		case *ast.RangeStmt:
			ps6079LearnCounterRangeAliases(pass, declarations, value, counter, aliases, visiting)
		case *ast.ReturnStmt:
			if len(value.Results) == 0 {
				for _, result := range namedResults {
					returnsAlias = returnsAlias || aliases[result]
				}
				return false
			}
			for _, result := range value.Results {
				if ps6079ExpressionReturnsCounterAlias(pass, declarations, result, counter, aliases, visiting) {
					returnsAlias = true
					break
				}
			}
			return false
		}
		return true
	})
	return returnsAlias
}

func ps6079NamedResultObjects(pass *analysis.Pass, results *ast.FieldList) []types.Object {
	if results == nil {
		return nil
	}
	var objects []types.Object
	for _, field := range results.List {
		for _, name := range field.Names {
			if object := pass.TypesInfo.Defs[name]; object != nil {
				objects = append(objects, object)
			}
		}
	}
	return objects
}

func ps6079FunctionMayInvalidateRouteProof(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	function *types.Func,
	counter ps6079CounterReference,
	aliases map[types.Object]bool,
	visiting map[*types.Func]bool,
) bool {
	if function == nil {
		return true
	}
	if function.Pkg() == nil || function.Pkg().Path() != pass.Pkg.Path() {
		// The proof cannot inspect another package's implementation. Keep the one
		// framework primitive that defines the measured b.Loop region, but treat
		// every other opaque call as capable of invoking registered callbacks or
		// otherwise manufacturing route evidence.
		return !ps6079HarmlessExternalRouteProofCall(function)
	}
	declaration := declarations[function]
	if declaration == nil {
		return true
	}
	if visiting[function] {
		return false
	}
	visiting[function] = true
	defer delete(visiting, function)

	uncertain := false
	ast.Inspect(declaration.Body, func(node ast.Node) bool {
		if uncertain {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		if ps6079NodeMutatesCounter(pass, declarations, node, counter, aliases) {
			uncertain = true
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if ps6079PanicCall(pass, call) {
			uncertain = true
			return false
		}
		if ps6079FunctionArgument(pass, call) {
			uncertain = true
			return false
		}
		callee, _, known := typedCallee(pass, call.Fun)
		if !known {
			uncertain = !ps6079BuiltinOrConversion(pass, call.Fun)
			return !uncertain
		}
		uncertain = ps6079FunctionMayInvalidateRouteProof(pass, declarations, callee, counter, aliases, visiting)
		return !uncertain
	})
	return uncertain
}

func ps6079WorkloadMayInvalidateRouteProof(
	pass *analysis.Pass,
	declarations map[*types.Func]*ast.FuncDecl,
	function *types.Func,
	gate *ps6079Gate,
	counter ps6079CounterReference,
	aliases map[types.Object]bool,
	allowCounterWrites bool,
	observed *bool,
	visiting map[*types.Func]bool,
) bool {
	if function == nil || function.Pkg() == nil || function.Pkg().Path() != pass.Pkg.Path() {
		return true
	}
	declaration := declarations[function]
	if declaration == nil {
		return true
	}
	if visiting[function] {
		return false
	}
	visiting[function] = true
	defer delete(visiting, function)

	uncertain := false
	ast.Inspect(declaration.Body, func(node ast.Node) bool {
		if uncertain {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		if ps6079NodeMutatesCounter(pass, declarations, node, counter, aliases) {
			// Only code reachable exclusively from the exact guarded
			// contributor's optimized arm may produce route evidence. Skip the
			// allowed mutation's subtree so its Add/Store helper is not
			// reclassified as an unrelated counter writer.
			if (allowCounterWrites || function == gate.object && ps6079InsideAny(node, gate.optimizedNodes)) &&
				ps6079ProvenCounterWrite(pass, node, counter, aliases) {
				*observed = true
				return false
			}
			uncertain = true
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if ps6079PanicCall(pass, call) || ps6079FunctionArgument(pass, call) {
			uncertain = true
			return false
		}
		callee, _, known := typedCallee(pass, call.Fun)
		if !known {
			uncertain = !ps6079BuiltinOrConversion(pass, call.Fun)
			return !uncertain
		}
		childAllowsCounter := allowCounterWrites || function == gate.object && ps6079InsideAny(call, gate.optimizedNodes)
		uncertain = ps6079WorkloadMayInvalidateRouteProof(
			pass, declarations, callee, gate, counter, aliases,
			childAllowsCounter, observed, visiting,
		)
		return !uncertain
	})
	return uncertain
}

func ps6079ProvenCounterWrite(
	pass *analysis.Pass,
	node ast.Node,
	counter ps6079CounterReference,
	aliases map[types.Object]bool,
) bool {
	overlaps := func(expression ast.Expr) bool {
		reference, ok := ps6079CounterObject(pass, expression)
		return ok && ps6079CounterReferencesOverlap(reference, counter) || aliases[ps6079RootObject(pass, expression)]
	}
	nonzero := func(expression ast.Expr) bool {
		value := pass.TypesInfo.Types[ps2110Unparen(expression)].Value
		sign, known := ps6079NumericSign(value)
		return !known || sign != 0
	}
	switch value := node.(type) {
	case *ast.AssignStmt:
		for index, expression := range value.Lhs {
			if !overlaps(expression) {
				continue
			}
			if value.Tok != token.ASSIGN && value.Tok != token.DEFINE {
				return true
			}
			return index < len(value.Rhs) && nonzero(value.Rhs[index])
		}
	case *ast.IncDecStmt:
		return overlaps(value.X)
	case *ast.CallExpr:
		name := ps6007NormalizeName(ps6074CalledName(value.Fun))
		selector, selected := ps2110Unparen(value.Fun).(*ast.SelectorExpr)
		directReceiver := selected && overlaps(selector.X)
		atomicFunction := false
		if callee, _, known := typedCallee(pass, value.Fun); known && callee.Pkg() != nil {
			atomicFunction = callee.Pkg().Path() == "sync/atomic"
		}
		if !directReceiver && !atomicFunction {
			return false
		}
		target := ast.Expr(nil)
		if directReceiver {
			target = selector.X
		} else {
			for _, argument := range value.Args {
				if overlaps(argument) {
					target = argument
					break
				}
			}
		}
		if target == nil {
			return false
		}
		switch {
		case ps6007ContainsAny(name, "add", "increment", "inc"):
			return len(value.Args) > 0 && nonzero(value.Args[len(value.Args)-1])
		case ps6007ContainsAny(name, "store", "swap", "set"):
			return len(value.Args) > 0 && nonzero(value.Args[len(value.Args)-1])
		}
	}
	return false
}

func ps6079InsideAny(node ast.Node, regions []ast.Node) bool {
	if node == nil {
		return false
	}
	for _, region := range regions {
		if region != nil && region.Pos() <= node.Pos() && node.End() <= region.End() {
			return true
		}
	}
	return false
}

func ps6079HarmlessExternalRouteProofCall(function *types.Func) bool {
	return function != nil && function.Pkg() != nil && function.Pkg().Path() == "testing" && function.Name() == "Loop"
}

func ps6079PanicCall(pass *analysis.Pass, call *ast.CallExpr) bool {
	identifier, ok := ps2110Unparen(call.Fun).(*ast.Ident)
	if !ok {
		return false
	}
	builtin, ok := pass.TypesInfo.ObjectOf(identifier).(*types.Builtin)
	return ok && builtin.Name() == "panic"
}

func ps6079PositionDominates(body *ast.BlockStmt, before, after token.Pos) bool {
	graph := cfg.New(body, func(*ast.CallExpr) bool { return true })
	return ps6079GraphPositionDominates(graph, before, after)
}

func ps6079PositionPostDominates(pass *analysis.Pass, body *ast.BlockStmt, before, after token.Pos) bool {
	graph := cfg.New(body, func(call *ast.CallExpr) bool { return !ps6079PanicCall(pass, call) })
	beforeBlock := ps6079CFGBlockAt(graph, before)
	afterBlock := ps6079CFGBlockAt(graph, after)
	if beforeBlock == nil || afterBlock == nil || !beforeBlock.Live || !afterBlock.Live {
		return false
	}
	if beforeBlock == afterBlock {
		return before < after && !ps6079CFGBlockHasTerminatingExpression(pass, beforeBlock, before, after)
	}
	seen := map[*cfg.Block]bool{beforeBlock: true}
	queue := []*cfg.Block{beforeBlock}
	for len(queue) > 0 {
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
		if ps6079CFGBlockHasTerminatingExpression(pass, block, minimum, maximum) {
			return false
		}
		if block == afterBlock {
			continue
		}
		if len(block.Succs) == 0 {
			return false
		}
		for _, successor := range block.Succs {
			if !seen[successor] {
				seen[successor] = true
				queue = append(queue, successor)
			}
		}
	}
	return true
}

func ps6079CFGBlockHasTerminatingExpression(pass *analysis.Pass, block *cfg.Block, minimum, maximum token.Pos) bool {
	for _, node := range block.Nodes {
		statement, ok := node.(*ast.ExprStmt)
		if !ok || statement.Pos() <= minimum || maximum.IsValid() && statement.Pos() >= maximum {
			continue
		}
		call, ok := ps2110Unparen(statement.X).(*ast.CallExpr)
		if ok && ps6079PanicCall(pass, call) {
			return true
		}
	}
	return false
}

func ps6079GraphPositionDominates(graph *cfg.CFG, before, after token.Pos) bool {
	beforeBlock := ps6079CFGBlockAt(graph, before)
	afterBlock := ps6079CFGBlockAt(graph, after)
	if beforeBlock == nil || afterBlock == nil {
		return false
	}
	if !beforeBlock.Live || !afterBlock.Live {
		return false
	}
	if beforeBlock == afterBlock {
		return before < after
	}
	seen := map[*cfg.Block]bool{graph.Blocks[0]: true}
	queue := []*cfg.Block{graph.Blocks[0]}
	for len(queue) > 0 {
		block := queue[0]
		queue = queue[1:]
		if block == beforeBlock {
			continue
		}
		if block == afterBlock {
			return false
		}
		for _, successor := range block.Succs {
			if !seen[successor] {
				seen[successor] = true
				queue = append(queue, successor)
			}
		}
	}
	return true
}

func ps6079CFGBlockAt(graph *cfg.CFG, position token.Pos) *cfg.Block {
	for _, block := range graph.Blocks {
		for _, node := range block.Nodes {
			found := false
			ast.Inspect(node, func(child ast.Node) bool {
				if found {
					return false
				}
				if child != nil && child.Pos() == position {
					found = true
				}
				return !found
			})
			if found {
				return block
			}
		}
	}
	return nil
}

func ps6079FunctionArgument(pass *analysis.Pass, call *ast.CallExpr) bool {
	if selector, ok := ps2110Unparen(call.Fun).(*ast.SelectorExpr); ok {
		callee, _, known := typedCallee(pass, call.Fun)
		if (!known || !ps6079HarmlessExternalRouteProofCall(callee)) &&
			ps6079TypeCanCarryFunction(pass.TypesInfo.TypeOf(selector.X), make(map[types.Type]bool)) {
			return true
		}
	}
	for _, argument := range call.Args {
		if ps6079TypeCanCarryFunction(pass.TypesInfo.TypeOf(argument), make(map[types.Type]bool)) {
			return true
		}
	}
	return false
}

func ps6079TypeCanCarryFunction(typeOf types.Type, visiting map[types.Type]bool) bool {
	if typeOf == nil || visiting[typeOf] {
		return false
	}
	visiting[typeOf] = true
	defer delete(visiting, typeOf)
	switch underlying := types.Unalias(typeOf).Underlying().(type) {
	case *types.Signature, *types.Interface:
		return true
	case *types.Array:
		return ps6079TypeCanCarryFunction(underlying.Elem(), visiting)
	case *types.Slice:
		return ps6079TypeCanCarryFunction(underlying.Elem(), visiting)
	case *types.Map:
		return ps6079TypeCanCarryFunction(underlying.Key(), visiting) ||
			ps6079TypeCanCarryFunction(underlying.Elem(), visiting)
	case *types.Pointer:
		return ps6079TypeCanCarryFunction(underlying.Elem(), visiting)
	case *types.Chan:
		return ps6079TypeCanCarryFunction(underlying.Elem(), visiting)
	case *types.Struct:
		for index := range underlying.NumFields() {
			if ps6079TypeCanCarryFunction(underlying.Field(index).Type(), visiting) {
				return true
			}
		}
	case *types.Tuple:
		for index := range underlying.Len() {
			if ps6079TypeCanCarryFunction(underlying.At(index).Type(), visiting) {
				return true
			}
		}
	}
	return false
}

func ps6079BuiltinOrConversion(pass *analysis.Pass, function ast.Expr) bool {
	if typeAndValue, ok := pass.TypesInfo.Types[function]; ok && typeAndValue.IsType() {
		return true
	}
	switch value := ps2110Unparen(function).(type) {
	case *ast.Ident:
		switch pass.TypesInfo.ObjectOf(value).(type) {
		case *types.Builtin, *types.TypeName:
			return true
		}
	case *ast.SelectorExpr:
		_, conversion := pass.TypesInfo.ObjectOf(value.Sel).(*types.TypeName)
		return conversion
	}
	return false
}

func ps6079RouteEvidenceForRoute(text string, route ps6079Route) bool {
	if !ps6079RouteEvidenceText(text) || route.gate == nil {
		return false
	}
	tokens := ps6079IdentifierTokens(text)
	components := ps6079SharedOperationComponents(route.gate.optimized, route.gate.fallback)
	if len(components) == 0 && route.gate.function != nil {
		components = ps6079OperationComponents(route.gate.function.Name.Name)
	}
	if len(components) == 0 {
		return false
	}
	for _, component := range components {
		if !slices.Contains(tokens, component) {
			return false
		}
	}
	return true
}

func ps6079SharedOperationComponents(left, right string) []string {
	rightComponents := ps6079OperationComponents(right)
	var result []string
	for _, component := range ps6079OperationComponents(left) {
		if slices.Contains(rightComponents, component) {
			result = append(result, component)
		}
	}
	return result
}

func ps6079OperationComponents(name string) []string {
	var result []string
	for _, component := range ps6079IdentifierTokens(name) {
		if len(component) >= 3 && !ps6079GenericRouteComponent(component) && !slices.Contains(result, component) {
			result = append(result, component)
		}
	}
	return result
}

func ps6079IdentifierTokens(text string) []string {
	var result []string
	start := -1
	runes := []rune(text)
	flush := func(end int) {
		if start >= 0 && start < end {
			result = append(result, strings.ToLower(string(runes[start:end])))
		}
		start = -1
	}
	for index, current := range runes {
		if !('a' <= current && current <= 'z' || 'A' <= current && current <= 'Z' || '0' <= current && current <= '9') {
			flush(index)
			continue
		}
		if start < 0 {
			start = index
			continue
		}
		previous := runes[index-1]
		nextLower := index+1 < len(runes) && 'a' <= runes[index+1] && runes[index+1] <= 'z'
		if 'A' <= current && current <= 'Z' && ('a' <= previous && previous <= 'z' || '0' <= previous && previous <= '9' ||
			'A' <= previous && previous <= 'Z' && nextLower) {
			flush(index)
			start = index
		}
	}
	flush(len(runes))
	return result
}

func ps6079GenericRouteComponent(component string) bool {
	return slices.Contains([]string{
		"fast", "path", "optimized", "route", "branch", "vector", "simd", "neon", "avx", "sse",
		"native", "kernel", "scalar", "fallback", "generic", "reference", "slow", "public", "wrapper",
	}, component)
}

func ps6079RouteEvidenceText(text string) bool {
	text = ps6007NormalizeName(text)
	route := ps6007ContainsAny(text, "fastpath", "optimizedroute", "vectorroute", "simdroute", "neonroute", "fastbranch")
	observed := ps6007ContainsAny(text, "hit", "count", "taken", "entered", "sample", "profile")
	return route && observed
}

func ps6079BenchmarkFailureBranch(pass *analysis.Pass, benchmark *ast.FuncDecl, conditional *ast.IfStmt) bool {
	receiver := ps6079BenchmarkParameter(pass, benchmark)
	if receiver == nil || ps6079BenchmarkReceiverReassigned(pass, benchmark, receiver, conditional.End()) ||
		ps6079BenchmarkBranchMayBypassFailure(pass, conditional.Body, receiver) {
		return false
	}
	graph := cfg.New(conditional.Body, func(*ast.CallExpr) bool { return true })
	type state struct {
		block  *cfg.Block
		failed bool
	}
	queue := []state{{block: graph.Blocks[0]}}
	seen := make(map[state]bool)
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if seen[current] {
			continue
		}
		seen[current] = true
		failed := current.failed || ps6079BlockFailsBenchmark(pass, current.block, receiver)
		if len(current.block.Succs) == 0 {
			if !failed {
				return false
			}
			continue
		}
		for _, successor := range current.block.Succs {
			queue = append(queue, state{block: successor, failed: failed})
		}
	}
	return true
}

func ps6079BenchmarkBranchMayBypassFailure(pass *analysis.Pass, body *ast.BlockStmt, receiver types.Object) bool {
	graph := cfg.New(body, func(*ast.CallExpr) bool { return true })
	type state struct {
		block  *cfg.Block
		failed bool
	}
	queue := []state{{block: graph.Blocks[0]}}
	seen := make(map[state]bool)
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if seen[current] {
			continue
		}
		seen[current] = true
		failed := current.failed
		for _, node := range current.block.Nodes {
			if failed {
				break
			}
			if ps6079FailureEvaluationMayBypass(pass, node, receiver, &failed) {
				return true
			}
		}
		for _, successor := range current.block.Succs {
			queue = append(queue, state{block: successor, failed: failed})
		}
	}
	return false
}

func ps6079FailureEvaluationMayBypass(pass *analysis.Pass, node ast.Node, receiver types.Object, failed *bool) bool {
	if node == nil || *failed {
		return false
	}
	switch value := node.(type) {
	case *ast.FuncLit:
		return false
	case *ast.DeferStmt:
		return ps6079ScheduledCallEvaluationMayBypass(pass, value.Call, receiver, failed)
	case *ast.GoStmt:
		if ps6079ScheduledCallEvaluationMayBypass(pass, value.Call, receiver, failed) {
			return true
		}
		// An asynchronous call is not an immediate failure marker and may
		// terminate the process or otherwise prevent a later marker.
		return true
	}
	if call, ok := node.(*ast.CallExpr); ok {
		if ps6079FailureEvaluationMayBypass(pass, call.Fun, receiver, failed) {
			return true
		}
		for _, argument := range call.Args {
			if ps6079FailureEvaluationMayBypass(pass, argument, receiver, failed) {
				return true
			}
		}
		if ps6079FailureCall(pass, call, receiver) {
			*failed = true
			return false
		}
		return ps6079BenchmarkSkipCall(pass, call, receiver) || ps6079PanicCall(pass, call) ||
			!ps6079BuiltinOrConversion(pass, call.Fun)
	}
	bypasses := false
	root := true
	ast.Inspect(node, func(child ast.Node) bool {
		if root {
			root = false
			return true
		}
		if child == nil || bypasses || *failed {
			return false
		}
		bypasses = ps6079FailureEvaluationMayBypass(pass, child, receiver, failed)
		return false
	})
	return bypasses
}

func ps6079ScheduledCallEvaluationMayBypass(
	pass *analysis.Pass,
	call *ast.CallExpr,
	receiver types.Object,
	failed *bool,
) bool {
	if ps6079FailureEvaluationMayBypass(pass, call.Fun, receiver, failed) {
		return true
	}
	for _, argument := range call.Args {
		if ps6079FailureEvaluationMayBypass(pass, argument, receiver, failed) {
			return true
		}
	}
	return false
}

func ps6079BenchmarkSkipCall(pass *analysis.Pass, call *ast.CallExpr, receiver types.Object) bool {
	selector, ok := ps2110Unparen(call.Fun).(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "Skip" && selector.Sel.Name != "Skipf" && selector.Sel.Name != "SkipNow" {
		return false
	}
	if ps6079RootObject(pass, selector.X) == receiver {
		return true
	}
	selection := pass.TypesInfo.Selections[selector]
	return selection != nil && selection.Kind() == types.MethodExpr && len(call.Args) > 0 &&
		ps6079RootObject(pass, call.Args[0]) == receiver
}

func ps6079BenchmarkReceiverReassigned(
	pass *analysis.Pass,
	benchmark *ast.FuncDecl,
	receiver types.Object,
	before token.Pos,
) bool {
	reassigned := false
	ast.Inspect(benchmark.Body, func(node ast.Node) bool {
		if reassigned {
			return false
		}
		if node == nil || node.Pos() >= before {
			return true
		}
		assigned := func(expression ast.Expr) bool {
			identifier, ok := ps2110Unparen(expression).(*ast.Ident)
			return ok && pass.TypesInfo.ObjectOf(identifier) == receiver
		}
		switch value := node.(type) {
		case *ast.AssignStmt:
			for _, expression := range value.Lhs {
				if assigned(expression) {
					reassigned = true
					break
				}
			}
		case *ast.RangeStmt:
			reassigned = assigned(value.Key) || assigned(value.Value)
		case *ast.UnaryExpr:
			// Once &b escapes, a later indirect write can replace the benchmark
			// receiver without an identifier assignment that this proof can see.
			// Conservatively reject receiver-based failure evidence from that point.
			identifier, ok := ps2110Unparen(value.X).(*ast.Ident)
			reassigned = value.Op == token.AND && ok && pass.TypesInfo.ObjectOf(identifier) == receiver
		}
		return !reassigned
	})
	return reassigned
}

func ps6079BenchmarkParameter(pass *analysis.Pass, benchmark *ast.FuncDecl) types.Object {
	if benchmark == nil || benchmark.Type == nil || benchmark.Type.Params == nil || len(benchmark.Type.Params.List) != 1 ||
		len(benchmark.Type.Params.List[0].Names) != 1 {
		return nil
	}
	return pass.TypesInfo.ObjectOf(benchmark.Type.Params.List[0].Names[0])
}

func ps6079BlockFailsBenchmark(pass *analysis.Pass, block *cfg.Block, receiver types.Object) bool {
	for _, node := range block.Nodes {
		switch node.(type) {
		case *ast.GoStmt, *ast.DeferStmt:
			continue
		}
		failed := false
		ast.Inspect(node, func(child ast.Node) bool {
			if failed {
				return false
			}
			if _, nested := child.(*ast.FuncLit); nested {
				return false
			}
			call, ok := child.(*ast.CallExpr)
			if ok && ps6079FailureCall(pass, call, receiver) {
				failed = true
			}
			return !failed
		})
		if failed {
			return true
		}
	}
	return false
}

func ps6079FailureCall(pass *analysis.Pass, call *ast.CallExpr, receiver types.Object) bool {
	selector, ok := ps2110Unparen(call.Fun).(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "Fail" && selector.Sel.Name != "FailNow" &&
		selector.Sel.Name != "Fatal" && selector.Sel.Name != "Fatalf" &&
		selector.Sel.Name != "Error" && selector.Sel.Name != "Errorf" {
		return false
	}
	selection := pass.TypesInfo.Selections[selector]
	method, methodSelected := pass.TypesInfo.ObjectOf(selector.Sel).(*types.Func)
	if selection == nil || !methodSelected || method.Pkg() == nil || method.Pkg().Path() != "testing" ||
		receiver == nil || !types.Identical(selection.Recv(), receiver.Type()) {
		return false
	}
	if selection.Kind() == types.MethodVal {
		return ps6079FailureReceiverIdentity(pass, selector.X, receiver)
	}
	return selection.Kind() == types.MethodExpr && len(call.Args) > 0 &&
		ps6079FailureReceiverIdentity(pass, call.Args[0], receiver)
}

func ps6079FailureReceiverIdentity(pass *analysis.Pass, expression ast.Expr, receiver types.Object) bool {
	expression = ps2110Unparen(expression)
	switch value := expression.(type) {
	case *ast.Ident:
		return pass.TypesInfo.ObjectOf(value) == receiver
	case *ast.UnaryExpr:
		star, ok := ps2110Unparen(value.X).(*ast.StarExpr)
		return value.Op == token.AND && ok && ps6079FailureReceiverIdentity(pass, star.X, receiver)
	case *ast.StarExpr:
		address, ok := ps2110Unparen(value.X).(*ast.UnaryExpr)
		return ok && address.Op == token.AND && ps6079FailureReceiverIdentity(pass, address.X, receiver)
	case *ast.CallExpr:
		if len(value.Args) != 1 {
			return false
		}
		typeAndValue, conversion := pass.TypesInfo.Types[value.Fun]
		if !conversion || !typeAndValue.IsType() {
			return false
		}
		destination, destinationPointer := types.Unalias(pass.TypesInfo.TypeOf(value)).Underlying().(*types.Pointer)
		source, sourcePointer := types.Unalias(pass.TypesInfo.TypeOf(value.Args[0])).Underlying().(*types.Pointer)
		return destinationPointer && sourcePointer &&
			types.Identical(types.Unalias(destination.Elem()), types.Unalias(source.Elem())) &&
			ps6079FailureReceiverIdentity(pass, value.Args[0], receiver)
	default:
		return false
	}
}

func ps6079Validated(function *ast.FuncDecl) bool {
	if function.Doc == nil {
		return false
	}
	for _, comment := range function.Doc.List {
		if strings.Contains(strings.ToLower(comment.Text), "perfscan:benchmark-fast-path-validated") {
			return true
		}
	}
	return false
}
