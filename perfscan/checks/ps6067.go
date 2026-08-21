package checks

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS6067 implements owner issue #784. It finds ordinary tests that turn one
// absolute timing sample into a hard failure without a controlled-runner gate.
var PS6067 = register(&lint.Check{
	ID:       "PS6067",
	Category: "verify",
	Slug:     "absolute-performance-ceiling-in-shared-test",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "an ordinary test enforces an absolute performance ceiling on a potentially shared runner",
		Text: `An ordinary unit test cannot distinguish a product regression from
shared-runner load, clock changes, OS scheduling, device contention, or thermal
state when it fails on one absolute timing sample. A hard 40-microsecond ceiling
can therefore turn an unrelated source-only change red even though the complete
product path is unchanged.

This check implements owner issue #784. It analyzes exact func TestX(*testing.T)
functions and requires all three links before reporting:

  - a metric derived from time.Now/time.Since/time.Time.Sub,
    testing.Benchmark/BenchmarkResult timing or reported metrics, or a numeric
    elapsed/latency/device-duration conversion;
  - an ordered comparison with a compile-time absolute duration or throughput
    constant, including an immutable local initialized from constants; and
  - a branch controlled by that comparison that invokes testing.T Fatal/Fatalf,
    Error/Errorf, or the predeclared panic builtin.

Timing provenance follows immutable locals through conversions, duration-unit
methods, arithmetic, selectors, and indexes, so both elapsed > 40*time.Microsecond
and items/elapsed.Seconds() < 1e6 are covered. Callees and testing receivers are
resolved through go/types; same-named user functions and methods stay silent.
Reachable local function literals are analyzed as part of the test. Numeric
constant arguments propagate through their parameter objects to a fixed point,
so an invoked slope(name, 40) helper is covered even when the comparison uses a
ceiling parameter; unused helpers and helpers invoked only with dynamic stored
thresholds stay silent.

Actual Go Benchmark functions, metric-only tests, nonconstant stored thresholds,
relative expressions combining candidate/control timings, and expressions whose
baseline/confidence/interval/percentile provenance identifies a statistical
comparison are excluded.

An earlier top-level or lexically enclosing opt-in guard suppresses a finding when it reads an
environment variable or a clearly named dedicated-runner/performance-test flag
and exits or calls testing.T.Skip/Skipf/SkipNow on the uncontrolled path. A
same-condition environment opt-in and a recognized require-dedicated-runner
helper are accepted too. Platform checks such as runtime.GOOS alone are not a
runner-quality gate. A //perfscan:absolute-performance-ceiling-validated
annotation suppresses a function with an externally enforced dedicated-runner
contract.

Replace the hard unit-test ceiling with benchmark distributions analyzed by
benchstat or another confidence-aware method, or compare candidate and control
in the same process with alternating order. If a timing assertion is genuinely
required, make it an explicit controlled-hardware opt-in and record runner,
clock, thermal, warmup, and repetition policy. There is NO automatic fix:
perfscan cannot invent a representative baseline distribution, same-process
control, hardware gate, or acceptable regression policy.`,
		Before: `func TestResidualCost(t *testing.T) {
	start := time.Now()
	runResidual()
	elapsed := time.Since(start)
	if elapsed > 40*time.Microsecond {
		t.Fatalf("residual took %v", elapsed)
	}
}`,
		After: `func BenchmarkResidualCost(b *testing.B) {
	for b.Loop() {
		runResidual()
	}
}
// Compare repeated candidate/control distributions with benchstat,
// or gate any hard assertion behind an explicit controlled-runner opt-in.`,
		MeasuredWin: `The macOS Metal shared-runner failure behind issue #784
observed residual add at 44.87 us against a hard 40 us ceiling in
backend/metal/TestPrefillOpCosts. The pull request had no product-code delta
for that path and the other 14 jobs passed; the absolute test converted runner
noise into a false-red pipeline.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6067",
		Doc:  "ordinary test fails on an absolute timing or throughput ceiling without a controlled-runner gate",
		Run:  runPS6067,
	},
})

type ps6067TimingKind uint8

const (
	ps6067NoTiming ps6067TimingKind = iota
	ps6067ClockPoint
	ps6067Measurement
)

type ps6067TimingInfo struct {
	kind        ps6067TimingKind
	source      string
	relative    bool
	statistical bool
}

type ps6067Comparison struct {
	expression *ast.BinaryExpr
	timing     ps6067TimingInfo
}

type ps6067InvokedClosure struct {
	literal            *ast.FuncLit
	calls              []*ast.CallExpr
	absoluteParameters map[types.Object]bool
}

func runPS6067(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil || !ps6067OrdinaryTest(pass, function) || ps6067ValidatedAnnotation(file, function) {
				continue
			}
			rootFlow := ps6064FunctionFlow(pass, function)
			rootEnvironment := ps6067EnvironmentObjects(pass, function.Body)
			ps6067AnalyzeBody(pass, function.Body, rootFlow, nil, rootEnvironment, function.Body)
			closures := ps6067InvokedClosures(pass, function.Body, rootFlow)
			closureFlow := rootFlow
			for _, closure := range closures {
				closureFlow = ps6067MergeFlows(closureFlow, ps6067BodyFlow(pass, closure.literal.Body))
			}
			for _, closure := range closures {
				environment := ps6067MergeObjectSets(rootEnvironment, ps6067EnvironmentObjects(pass, closure.literal.Body))
				ps6067AnalyzeBody(pass, closure.literal.Body, closureFlow, closure.absoluteParameters, environment, function.Body)
			}
		}
	}
	return nil, nil
}

func ps6067AnalyzeBody(
	pass *analysis.Pass,
	body *ast.BlockStmt,
	flow ps6064Flow,
	absoluteObjects map[types.Object]bool,
	environment map[types.Object]bool,
	rootBody *ast.BlockStmt,
) {
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		conditional, ok := node.(*ast.IfStmt)
		if !ok || !ps6067FailureBranch(pass, conditional) || ps6067GuardCondition(pass, conditional.Cond, environment) {
			return true
		}
		comparison, ok := ps6067AbsoluteComparison(pass, flow, absoluteObjects, conditional.Cond)
		if !ok || ps6067GuardedBefore(pass, body, conditional.Pos(), environment) ||
			body != rootBody && ps6067GuardedBefore(pass, rootBody, conditional.Pos(), environment) {
			return true
		}
		source := comparison.timing.source
		if source == "" {
			source = "elapsed/device timing"
		}
		pass.Reportf(comparison.expression.Pos(), "%s is compared with an absolute performance threshold and directly fails an ordinary test; absolute ceilings are unstable on shared CI, so use repeated benchmark distributions/benchstat, a relative same-process control, or an explicit controlled-hardware opt-in guard (advisory, no automatic fix)", source)
		return true
	})
}

// ps6067InvokedClosures finds local function literals reachable from direct
// calls in the test and propagates absolute call-site arguments onto their
// parameter objects. Reachability prevents an unused helper declaration from
// producing a finding, while the fixed point handles one local helper passing
// a threshold through to another.
func ps6067InvokedClosures(pass *analysis.Pass, root *ast.BlockStmt, rootFlow ps6064Flow) []ps6067InvokedClosure {
	bindings := make(map[types.Object]*ast.FuncLit)
	bindingWrites := make(map[types.Object]int)
	ast.Inspect(root, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.AssignStmt:
			for index, target := range value.Lhs {
				identifier, ok := ps2110Unparen(target).(*ast.Ident)
				if !ok || identifier.Name == "_" {
					continue
				}
				object := identObject(pass, identifier)
				if object == nil {
					continue
				}
				bindingWrites[object]++
				if len(value.Lhs) == len(value.Rhs) {
					if literal, ok := ps2110Unparen(value.Rhs[index]).(*ast.FuncLit); ok {
						bindings[object] = literal
					}
				}
			}
		case *ast.ValueSpec:
			if len(value.Values) == 0 {
				return true
			}
			for index, name := range value.Names {
				object := pass.TypesInfo.Defs[name]
				if object == nil {
					continue
				}
				bindingWrites[object]++
				if len(value.Names) == len(value.Values) {
					if literal, ok := ps2110Unparen(value.Values[index]).(*ast.FuncLit); ok {
						bindings[object] = literal
					}
				}
			}
		}
		return true
	})
	for object, writes := range bindingWrites {
		if writes != 1 {
			delete(bindings, object)
		}
	}

	byLiteral := make(map[*ast.FuncLit]*ps6067InvokedClosure)
	var ordered []*ps6067InvokedClosure
	queue := []*ast.BlockStmt{root}
	visitedBodies := map[*ast.BlockStmt]bool{root: true}
	for len(queue) != 0 {
		body := queue[0]
		queue = queue[1:]
		ast.Inspect(body, func(node ast.Node) bool {
			if _, nested := node.(*ast.FuncLit); nested {
				return false
			}
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			var literal *ast.FuncLit
			switch callee := ps2110Unparen(call.Fun).(type) {
			case *ast.FuncLit:
				literal = callee
			case *ast.Ident:
				literal = bindings[pass.TypesInfo.ObjectOf(callee)]
			}
			if literal == nil {
				return true
			}
			closure := byLiteral[literal]
			if closure == nil {
				closure = &ps6067InvokedClosure{literal: literal, absoluteParameters: make(map[types.Object]bool)}
				byLiteral[literal] = closure
				ordered = append(ordered, closure)
			}
			closure.calls = append(closure.calls, call)
			if !visitedBodies[literal.Body] {
				visitedBodies[literal.Body] = true
				queue = append(queue, literal.Body)
			}
			return true
		})
	}

	closures := make([]ps6067InvokedClosure, 0, len(ordered))
	flow := rootFlow
	for _, closure := range ordered {
		closures = append(closures, *closure)
		flow = ps6067MergeFlows(flow, ps6067BodyFlow(pass, closure.literal.Body))
	}
	abs := make(map[types.Object]bool)
	changed := true
	for changed {
		changed = false
		for index := range closures {
			closure := &closures[index]
			signature, ok := pass.TypesInfo.TypeOf(closure.literal).(*types.Signature)
			if !ok {
				continue
			}
			for _, call := range closure.calls {
				limit := min(signature.Params().Len(), len(call.Args))
				for parameterIndex := 0; parameterIndex < limit; parameterIndex++ {
					parameter := signature.Params().At(parameterIndex)
					if abs[parameter] || !ps6067AbsoluteConstant(pass, flow, abs, call.Args[parameterIndex], map[types.Object]bool{}) {
						continue
					}
					abs[parameter] = true
					changed = true
				}
			}
		}
	}
	for index := range closures {
		closure := &closures[index]
		signature, ok := pass.TypesInfo.TypeOf(closure.literal).(*types.Signature)
		if !ok {
			continue
		}
		for parameterIndex := 0; parameterIndex < signature.Params().Len(); parameterIndex++ {
			parameter := signature.Params().At(parameterIndex)
			if abs[parameter] {
				closure.absoluteParameters[parameter] = true
			}
		}
	}
	return closures
}

func ps6067BodyFlow(pass *analysis.Pass, body *ast.BlockStmt) ps6064Flow {
	flow := ps6064Flow{initializers: map[types.Object]ast.Expr{}, writes: map[types.Object]int{}}
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		switch value := node.(type) {
		case *ast.ValueSpec:
			if len(value.Names) == len(value.Values) {
				for index, name := range value.Names {
					if object := pass.TypesInfo.Defs[name]; object != nil {
						flow.initializers[object] = value.Values[index]
					}
				}
			}
		case *ast.AssignStmt:
			for index, target := range value.Lhs {
				identifier, ok := ps2110Unparen(target).(*ast.Ident)
				if !ok || identifier.Name == "_" {
					continue
				}
				if value.Tok == token.DEFINE {
					if object := pass.TypesInfo.Defs[identifier]; object != nil && len(value.Lhs) == len(value.Rhs) {
						flow.initializers[object] = value.Rhs[index]
					}
					continue
				}
				if object := pass.TypesInfo.ObjectOf(identifier); object != nil {
					flow.writes[object]++
				}
			}
		case *ast.IncDecStmt:
			if identifier, ok := ps2110Unparen(value.X).(*ast.Ident); ok {
				if object := pass.TypesInfo.ObjectOf(identifier); object != nil {
					flow.writes[object]++
				}
			}
		}
		return true
	})
	return flow
}

func ps6067MergeFlows(left, right ps6064Flow) ps6064Flow {
	result := ps6064Flow{initializers: make(map[types.Object]ast.Expr), writes: make(map[types.Object]int)}
	for object, initializer := range left.initializers {
		result.initializers[object] = initializer
	}
	for object, initializer := range right.initializers {
		result.initializers[object] = initializer
	}
	for object, writes := range left.writes {
		result.writes[object] += writes
	}
	for object, writes := range right.writes {
		result.writes[object] += writes
	}
	return result
}

func ps6067MergeObjectSets(sets ...map[types.Object]bool) map[types.Object]bool {
	result := make(map[types.Object]bool)
	for _, set := range sets {
		for object := range set {
			result[object] = true
		}
	}
	return result
}

func ps6067OrdinaryTest(pass *analysis.Pass, function *ast.FuncDecl) bool {
	if function.Recv != nil || !strings.HasPrefix(function.Name.Name, "Test") {
		return false
	}
	object, ok := pass.TypesInfo.Defs[function.Name].(*types.Func)
	if !ok {
		return false
	}
	signature, ok := object.Type().(*types.Signature)
	if !ok || signature.Params().Len() != 1 || signature.Results().Len() != 0 || signature.Variadic() {
		return false
	}
	pointer, ok := types.Unalias(signature.Params().At(0).Type()).(*types.Pointer)
	if !ok {
		return false
	}
	named, ok := types.Unalias(pointer.Elem()).(*types.Named)
	return ok && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == "testing" && named.Obj().Name() == "T"
}

func ps6067AbsoluteComparison(pass *analysis.Pass, flow ps6064Flow, absoluteObjects map[types.Object]bool, condition ast.Expr) (ps6067Comparison, bool) {
	var result ps6067Comparison
	ast.Inspect(condition, func(node ast.Node) bool {
		if result.expression != nil {
			return false
		}
		comparison, ok := node.(*ast.BinaryExpr)
		if !ok || !ps6067OrderedComparison(comparison.Op) {
			return true
		}
		left := ps6067TimingExpression(pass, flow, comparison.X, map[types.Object]bool{})
		right := ps6067TimingExpression(pass, flow, comparison.Y, map[types.Object]bool{})
		leftConstant := ps6067AbsoluteConstant(pass, flow, absoluteObjects, comparison.X, map[types.Object]bool{})
		rightConstant := ps6067AbsoluteConstant(pass, flow, absoluteObjects, comparison.Y, map[types.Object]bool{})
		switch {
		case left.kind == ps6067Measurement && rightConstant && !left.relative && !left.statistical:
			result = ps6067Comparison{expression: comparison, timing: left}
		case right.kind == ps6067Measurement && leftConstant && !right.relative && !right.statistical:
			result = ps6067Comparison{expression: comparison, timing: right}
		}
		return result.expression == nil
	})
	return result, result.expression != nil
}

func ps6067OrderedComparison(operator token.Token) bool {
	return operator == token.LSS || operator == token.LEQ || operator == token.GTR || operator == token.GEQ
}

func ps6067TimingExpression(pass *analysis.Pass, flow ps6064Flow, expression ast.Expr, seen map[types.Object]bool) ps6067TimingInfo {
	expression = ps2110Unparen(expression)
	switch value := expression.(type) {
	case *ast.Ident:
		object := pass.TypesInfo.ObjectOf(value)
		info := ps6067TimingInfo{statistical: ps6067StatisticalName(value.Name)}
		if object == nil {
			return info
		}
		if initializer := flow.initializers[object]; initializer != nil && flow.writes[object] == 0 && !seen[object] {
			seen[object] = true
			derived := ps6067TimingExpression(pass, flow, initializer, seen)
			delete(seen, object)
			derived.statistical = derived.statistical || info.statistical
			if derived.kind != ps6067NoTiming {
				return derived
			}
		}
		if ps6067TimingName(value.Name) && ps6067NumericType(object.Type()) {
			info.kind = ps6067Measurement
			info.source = "named elapsed/device timing"
		}
		return info

	case *ast.CallExpr:
		return ps6067TimingCall(pass, flow, value, seen)

	case *ast.BinaryExpr:
		left := ps6067TimingExpression(pass, flow, value.X, seen)
		right := ps6067TimingExpression(pass, flow, value.Y, seen)
		result := ps6067MergeTiming(left, right)
		if value.Op == token.SUB && left.kind == ps6067ClockPoint && right.kind == ps6067ClockPoint {
			result.kind = ps6067Measurement
			result.source = ps6067FirstSource(left.source, right.source, "clock delta")
		}
		if left.kind == ps6067Measurement && right.kind == ps6067Measurement {
			result.relative = true
		}
		return result

	case *ast.UnaryExpr:
		return ps6067TimingExpression(pass, flow, value.X, seen)

	case *ast.SelectorExpr:
		result := ps6067TimingExpression(pass, flow, value.X, seen)
		result.statistical = result.statistical || ps6067StatisticalName(value.Sel.Name)
		return result

	case *ast.IndexExpr:
		return ps6067MergeTiming(
			ps6067TimingExpression(pass, flow, value.X, seen),
			ps6067TimingExpression(pass, flow, value.Index, seen),
		)

	case *ast.SliceExpr:
		result := ps6067TimingExpression(pass, flow, value.X, seen)
		for _, bound := range []ast.Expr{value.Low, value.High, value.Max} {
			if bound != nil {
				result = ps6067MergeTiming(result, ps6067TimingExpression(pass, flow, bound, seen))
			}
		}
		return result

	case *ast.ParenExpr:
		return ps6067TimingExpression(pass, flow, value.X, seen)
	}
	return ps6067TimingInfo{}
}

func ps6067TimingCall(pass *analysis.Pass, flow ps6064Flow, call *ast.CallExpr, seen map[types.Object]bool) ps6067TimingInfo {
	if ps6067PackageCall(pass, call, "time", "Now") {
		return ps6067TimingInfo{kind: ps6067ClockPoint, source: "time.Now-derived elapsed time"}
	}
	if ps6067PackageCall(pass, call, "time", "Since") {
		return ps6067TimingInfo{kind: ps6067Measurement, source: "time.Since-derived elapsed time"}
	}
	if ps6067PackageCall(pass, call, "testing", "Benchmark") {
		return ps6067TimingInfo{kind: ps6067Measurement, source: "testing.Benchmark result"}
	}

	var result ps6067TimingInfo
	if selector, ok := ps2110Unparen(call.Fun).(*ast.SelectorExpr); ok {
		result = ps6067TimingExpression(pass, flow, selector.X, seen)
		result.statistical = result.statistical || ps6067StatisticalName(selector.Sel.Name)
	}
	for _, argument := range call.Args {
		result = ps6067MergeTiming(result, ps6067TimingExpression(pass, flow, argument, seen))
	}

	if typed, ok := pass.TypesInfo.Types[ps2110Unparen(call.Fun)]; ok && typed.IsType() {
		if ps6067TimeDurationType(pass.TypesInfo.TypeOf(call)) &&
			(result.kind != ps6067NoTiming || ps6067ExpressionTimingName(call.Args)) {
			result.kind = ps6067Measurement
			result.source = ps6067FirstSource(result.source, "duration conversion")
		}
		return result
	}

	function, signature, ok := typedCallee(pass, call.Fun)
	if !ok {
		return result
	}
	name := function.Name()
	if signature.Recv() != nil && typedReceiverNamed(signature, "time", "Time") {
		switch name {
		case "Sub":
			result.kind = ps6067Measurement
			result.source = "time.Time.Sub-derived elapsed time"
		case "Unix", "UnixMilli", "UnixMicro", "UnixNano":
			result.kind = ps6067ClockPoint
			result.source = "time.Now-derived clock delta"
		}
	}
	if signature.Recv() != nil && typedReceiverNamed(signature, "testing", "BenchmarkResult") {
		if name == "NsPerOp" {
			result.kind = ps6067Measurement
			result.source = "testing.Benchmark NsPerOp"
		}
	}
	if signature.Recv() != nil && typedReceiverNamed(signature, "time", "Duration") &&
		ps6007ContainsAny(ps6007NormalizeName(name), "hours", "minutes", "seconds", "milliseconds", "microseconds", "nanoseconds") {
		if result.kind != ps6067NoTiming {
			result.kind = ps6067Measurement
		}
	}
	if ps6067NumericType(pass.TypesInfo.TypeOf(call)) {
		switch ps6067TimingCallKind(name) {
		case ps6067ClockPoint:
			result.kind = ps6067ClockPoint
			result.source = ps6067FirstSource(result.source, "device clock delta")
		case ps6067Measurement:
			result.kind = ps6067Measurement
			result.source = ps6067FirstSource(result.source, "device elapsed/latency timing")
		}
	}
	result.statistical = result.statistical || ps6067StatisticalName(name)
	return result
}

func ps6067PackageCall(pass *analysis.Pass, call *ast.CallExpr, pkgPath, name string) bool {
	function, signature, ok := typedCallee(pass, call.Fun)
	return ok && signature.Recv() == nil && function.Pkg() != nil && function.Pkg().Path() == pkgPath && function.Name() == name
}

func ps6067TimingCallKind(name string) ps6067TimingKind {
	name = ps6007NormalizeName(name)
	if ps6007ContainsAny(name, "starttime", "endtime", "timestamp", "clocktick", "gputick", "devicetick") {
		return ps6067ClockPoint
	}
	if ps6007ContainsAny(name,
		"elapsed", "duration", "latency", "nanoseconds", "microseconds",
		"gpustime", "devicetime", "gpuseconds", "deviceseconds", "wallseconds") {
		return ps6067Measurement
	}
	return ps6067NoTiming
}

func ps6067TimingName(name string) bool {
	name = ps6007NormalizeName(name)
	if name == "ns" || name == "us" || name == "ms" {
		return true
	}
	return ps6007ContainsAny(name,
		"elapsed", "duration", "latency", "nanosecond", "microsecond", "millisecond",
		"gpustime", "devicetime", "walltime", "nsperop", "opspersecond", "throughput")
}

func ps6067StatisticalName(name string) bool {
	name = ps6007NormalizeName(name)
	return ps6007ContainsAny(name,
		"baseline", "reference", "control", "incumbent", "confidence", "interval",
		"percentile", "median", "variance", "stddev", "pvalue", "distribution", "benchstat", "samples")
}

func ps6067ExpressionTimingName(expressions []ast.Expr) bool {
	found := false
	for _, expression := range expressions {
		ast.Inspect(expression, func(node ast.Node) bool {
			identifier, ok := node.(*ast.Ident)
			if ok && ps6067TimingName(identifier.Name) {
				found = true
				return false
			}
			return !found
		})
		if found {
			return true
		}
	}
	return false
}

func ps6067MergeTiming(left, right ps6067TimingInfo) ps6067TimingInfo {
	result := left
	if right.kind > result.kind {
		result.kind = right.kind
	}
	result.source = ps6067FirstSource(left.source, right.source)
	result.relative = left.relative || right.relative
	result.statistical = left.statistical || right.statistical
	return result
}

func ps6067FirstSource(sources ...string) string {
	for _, source := range sources {
		if source != "" {
			return source
		}
	}
	return ""
}

func ps6067NumericType(value types.Type) bool {
	if value == nil {
		return false
	}
	basic, ok := types.Unalias(value).Underlying().(*types.Basic)
	return ok && basic.Info()&(types.IsInteger|types.IsFloat) != 0
}

func ps6067TimeDurationType(value types.Type) bool {
	named, ok := types.Unalias(value).(*types.Named)
	return ok && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == "time" && named.Obj().Name() == "Duration"
}

func ps6067AbsoluteConstant(pass *analysis.Pass, flow ps6064Flow, absoluteObjects map[types.Object]bool, expression ast.Expr, seen map[types.Object]bool) bool {
	expression = ps2110Unparen(expression)
	if typed, ok := pass.TypesInfo.Types[expression]; ok && typed.Value != nil {
		return typed.Value.Kind() == constant.Int || typed.Value.Kind() == constant.Float
	}
	identifier, ok := expression.(*ast.Ident)
	if !ok {
		return false
	}
	object := pass.TypesInfo.ObjectOf(identifier)
	if absoluteObjects[object] {
		return true
	}
	if object == nil || seen[object] || flow.writes[object] != 0 {
		return false
	}
	initializer := flow.initializers[object]
	if initializer == nil {
		return false
	}
	seen[object] = true
	result := ps6067AbsoluteConstant(pass, flow, absoluteObjects, initializer, seen)
	delete(seen, object)
	return result
}

func ps6067FailureBranch(pass *analysis.Pass, conditional *ast.IfStmt) bool {
	if conditional == nil {
		return false
	}
	return ps6067HasFailure(pass, conditional.Body) || conditional.Else != nil && ps6067HasFailure(pass, conditional.Else)
}

func ps6067HasFailure(pass *analysis.Pass, node ast.Node) bool {
	found := false
	ast.Inspect(node, func(current ast.Node) bool {
		if found {
			return false
		}
		if _, nested := current.(*ast.FuncLit); nested {
			return false
		}
		call, ok := current.(*ast.CallExpr)
		if !ok {
			return true
		}
		if typedBuiltinName(pass, call.Fun, "panic") {
			found = true
			return false
		}
		if ps6067TestingTMethod(pass, call, "Fatal", "Fatalf", "Error", "Errorf") {
			found = true
			return false
		}
		return true
	})
	return found
}

func ps6067EnvironmentObjects(pass *analysis.Pass, body *ast.BlockStmt) map[types.Object]bool {
	objects := make(map[types.Object]bool)
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		switch statement := node.(type) {
		case *ast.AssignStmt:
			if len(statement.Rhs) == 1 && ps6067EnvironmentExpression(pass, statement.Rhs[0]) {
				for _, target := range statement.Lhs {
					if identifier, ok := ps2110Unparen(target).(*ast.Ident); ok {
						if object := identObject(pass, identifier); object != nil {
							objects[object] = true
						}
					}
				}
			}
		case *ast.ValueSpec:
			if len(statement.Values) == 1 && ps6067EnvironmentExpression(pass, statement.Values[0]) {
				for _, name := range statement.Names {
					if object := pass.TypesInfo.Defs[name]; object != nil {
						objects[object] = true
					}
				}
			}
		}
		return true
	})
	return objects
}

func ps6067EnvironmentExpression(pass *analysis.Pass, expression ast.Expr) bool {
	found := false
	ast.Inspect(expression, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if ok && (ps6067PackageCall(pass, call, "os", "Getenv") || ps6067PackageCall(pass, call, "os", "LookupEnv")) {
			found = true
			return false
		}
		return !found
	})
	return found
}

func ps6067GuardCondition(pass *analysis.Pass, condition ast.Expr, environment map[types.Object]bool) bool {
	guarded := false
	ast.Inspect(condition, func(node ast.Node) bool {
		if guarded {
			return false
		}
		switch value := node.(type) {
		case *ast.CallExpr:
			if ps6067PackageCall(pass, value, "os", "Getenv") || ps6067PackageCall(pass, value, "os", "LookupEnv") {
				guarded = true
				return false
			}
			if ps6067BooleanType(pass.TypesInfo.TypeOf(value)) && ps6067DedicatedGuardCall(pass, value) {
				guarded = true
				return false
			}
		case *ast.Ident:
			object := pass.TypesInfo.ObjectOf(value)
			if environment[object] || ps6067BooleanType(ps6067ObjectType(object)) && ps6067DedicatedGuardName(value.Name) {
				guarded = true
				return false
			}
		}
		return true
	})
	return guarded
}

func ps6067GuardedBefore(pass *analysis.Pass, body *ast.BlockStmt, position token.Pos, environment map[types.Object]bool) bool {
	if ps6067EnclosingGuard(pass, body, position, environment) {
		return true
	}
	for _, statement := range body.List {
		if statement.Pos() >= position {
			break
		}
		switch value := statement.(type) {
		case *ast.IfStmt:
			if ps6067GuardCondition(pass, value.Cond, environment) &&
				(ps6067GuardExit(pass, value.Body) || value.Else != nil && ps6067GuardExit(pass, value.Else)) {
				return true
			}
		case *ast.ExprStmt:
			call, ok := ps2110Unparen(value.X).(*ast.CallExpr)
			if ok && ps6067DedicatedGuardCall(pass, call) {
				return true
			}
		}
	}
	return false
}

func ps6067EnclosingGuard(pass *analysis.Pass, body *ast.BlockStmt, position token.Pos, environment map[types.Object]bool) bool {
	guarded := false
	ast.Inspect(body, func(node ast.Node) bool {
		if guarded || node == nil {
			return !guarded
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		conditional, ok := node.(*ast.IfStmt)
		if !ok || conditional.Pos() >= position || position >= conditional.End() {
			return true
		}
		if ps6067GuardCondition(pass, conditional.Cond, environment) {
			guarded = true
			return false
		}
		return true
	})
	return guarded
}

func ps6067GuardExit(pass *analysis.Pass, node ast.Node) bool {
	block, ok := node.(*ast.BlockStmt)
	if !ok {
		return false
	}
	for _, statement := range block.List {
		if _, ok := statement.(*ast.ReturnStmt); ok {
			return true
		}
		expression, ok := statement.(*ast.ExprStmt)
		if !ok {
			continue
		}
		call, ok := ps2110Unparen(expression.X).(*ast.CallExpr)
		if !ok {
			continue
		}
		if ps6067TestingTMethod(pass, call, "Skip", "Skipf", "SkipNow") {
			return true
		}
	}
	return false
}

func ps6067TestingTMethod(pass *analysis.Pass, call *ast.CallExpr, names ...string) bool {
	selector, ok := ps2110Unparen(call.Fun).(*ast.SelectorExpr)
	if !ok {
		return false
	}
	receiver := types.Unalias(pass.TypesInfo.TypeOf(selector.X))
	pointer, ok := receiver.(*types.Pointer)
	if !ok {
		return false
	}
	named, ok := types.Unalias(pointer.Elem()).(*types.Named)
	if !ok || named.Obj().Pkg() == nil || named.Obj().Pkg().Path() != "testing" || named.Obj().Name() != "T" {
		return false
	}
	for _, name := range names {
		if selector.Sel.Name == name {
			return true
		}
	}
	return false
}

func ps6067DedicatedGuardCall(pass *analysis.Pass, call *ast.CallExpr) bool {
	function, _, ok := typedCallee(pass, call.Fun)
	if ok {
		return ps6067DedicatedGuardHelperName(function.Name())
	}
	if identifier, ok := ps2110Unparen(call.Fun).(*ast.Ident); ok {
		return ps6067DedicatedGuardHelperName(identifier.Name)
	}
	return false
}

func ps6067DedicatedGuardName(name string) bool {
	name = ps6007NormalizeName(name)
	return ps6007ContainsAny(name,
		"dedicatedrunner", "controlledrunner", "controlledhardware", "performancetest",
		"perftest", "perfoptin", "perfenabled", "requireperf", "timingoptin", "allowtiming", "timingassert")
}

func ps6067DedicatedGuardHelperName(name string) bool {
	name = ps6007NormalizeName(name)
	return ps6007ContainsAny(name,
		"dedicatedrunner", "controlledrunner", "controlledhardware", "requireperf",
		"skipunlessperf", "performanceoptin", "perftestoptin", "timingoptin", "allowtiming", "timingassert")
}

func ps6067BooleanType(value types.Type) bool {
	if value == nil {
		return false
	}
	basic, ok := types.Unalias(value).Underlying().(*types.Basic)
	return ok && basic.Kind() == types.Bool
}

func ps6067ObjectType(object types.Object) types.Type {
	if object == nil {
		return nil
	}
	return object.Type()
}

func ps6067ValidatedAnnotation(file *ast.File, function *ast.FuncDecl) bool {
	for _, group := range file.Comments {
		if group != function.Doc && !(group.End() <= function.Pos() && function.Pos()-group.End() <= 3) && !(group.Pos() >= function.Body.Pos() && group.End() <= function.Body.End()) {
			continue
		}
		for _, comment := range group.List {
			text := ps6058Compact(comment.Text)
			if strings.Contains(text, "perfscanabsoluteperformanceceilingvalidated") || strings.Contains(text, "perfscancontrolledtimingassertion") {
				return true
			}
		}
	}
	return false
}

func ps6067DebugTiming(info ps6067TimingInfo) string {
	return fmt.Sprintf("kind=%d source=%q relative=%t statistical=%t", info.kind, info.source, info.relative, info.statistical)
}
