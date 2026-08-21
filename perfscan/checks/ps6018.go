package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"slices"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS6018 implements owner issue #774: fixed-count control/candidate route
// benchmarks need more than one untimed warmup unless initialization is proven.
var PS6018 = register(&lint.Check{
	ID:       "PS6018",
	Category: "verify",
	Slug:     "underwarmed-fixed-count-route-benchmark",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "a fixed-count control/candidate route benchmark warms an arm only once",
		Text: `A control/candidate route benchmark can pass semantic checks and
show an overwhelming median signal while failing its own stability audit. One
untimed call before b.ResetTimer does not necessarily stabilize pipeline
compilation, allocator state, caches, command submission, or other lazy work in
a microsecond-scale candidate.

This check implements owner issue #774. It reports a real
func BenchmarkX(*testing.B) only when all of these source-auditable facts hold:

  - the benchmark has route, leadership, crossover, or promotion context;
  - it defines explicit control/candidate arms, paired b.Run arms, or calls a
    shared helper with two distinct arm functions;
  - an affected arm is invoked exactly once before testing.B.ResetTimer; and
  - the benchmark documents fixed-count evidence (for example
    -benchtime=100x/-count=7) or a max/min, spread, ceiling, or stability gate.

The detector follows three common organizations: direct named control and
candidate calls in one benchmark, two b.Run closures, and a helper whose
signature includes *testing.B plus an arm func parameter. Calls inside an
untimed loop count as repeated warmup. A nearby comment or call documenting an
initialization barrier, eager/preinitialized state, or absence of lazy
initialization suppresses the finding.

There is NO automatic fix. Some kernels have no lazy initialization, one call
is enough after an external barrier, and choosing a warmup count is measured
benchmark policy. Add a short repeated untimed warmup or document the concrete
initialization barrier. Preserve per-cell max/min as a disturbance diagnostic,
but select routes from repeated independent campaign medians and re-run an
excursion before classifying it as recurrent instability. Neither a median-only
policy nor a single max/min rejection is a robust decision statistic.`,
		Before: `// Run with -benchtime=100x -count=7; max/min must stay < 3x.
control()
candidate() // one untimed call each
b.ResetTimer()`,
		After: `for range 10 {
	control()
	candidate()
}
	b.ResetTimer() // retain spread diagnostics and repeated campaign medians`,
		MeasuredWin: `In the issue #774 Apple-M2 route audit, one untimed CPU
candidate warmup produced 823.8 ns-3737 ns across seven fixed-count samples, a
4.54x spread that invalidated the campaign. Ten untimed warmups narrowed the
same arm to 722.1 ns-952.9 ns (1.32x); every control and candidate series then
stayed below the frozen 3.0x stability ceiling.

The production-selector follow-up kept all 84 medians on the same CPU winner
across three isolated 100x7 campaigns (floors 1.743x-1.777x). One campaign had
a non-recurring GELU-backward 7.339x max/min excursion; an immediate unchanged
diagnostic campaign kept every affected cell at <=1.193x spread. The excursion
was useful evidence of disturbance, not by itself proof of recurrent cell
instability.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6018",
		Doc:  "fixed-count route benchmark invokes a control or candidate arm only once before ResetTimer",
		Run:  runPS6018,
	},
})

type ps6018CallKey struct {
	object types.Object
	name   string
}

type ps6018Stats struct {
	direct   int
	repeated bool
	pos      ast.Node
}

type ps6018Helper struct {
	fn       *ast.FuncDecl
	object   types.Object
	arm      types.Object
	armIndex int
	reset    *ast.CallExpr
	under    bool
}

func runPS6018(pass *analysis.Pass) (any, error) {
	helpers := ps6018Helpers(pass)
	reportedHelpers := make(map[types.Object]bool)
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !ps6006Benchmark(pass, fn) {
				continue
			}
			text := ps6015FunctionText(pass, file, fn)
			if !ps6018RouteContext(text) || !ps6018EvidenceContext(text) {
				continue
			}
			if reset, arms, ok := ps6018DirectArms(pass, file, fn); ok {
				pass.Reportf(reset.Pos(), "fixed-count route benchmark warms %s only once before ResetTimer; use a short repeated untimed warmup or document a concrete initialization barrier; retain per-cell max/min as a disturbance diagnostic, select with repeated campaign medians, and re-run excursions before classifying recurrent instability", strings.Join(arms, " and "))
			}
			if reset, arms, ok := ps6018SubbenchArms(pass, file, fn); ok {
				pass.Reportf(reset.Pos(), "fixed-count route benchmark warms b.Run arm(s) %s only once before ResetTimer; use a short repeated untimed warmup or document a concrete initialization barrier; retain per-cell max/min as a disturbance diagnostic, select with repeated campaign medians, and re-run excursions before classifying recurrent instability", strings.Join(arms, ", "))
			}
			for helper, armNames := range ps6018CalledHelpers(pass, fn, helpers) {
				if reportedHelpers[helper.object] || !helper.under || len(armNames) < 2 {
					continue
				}
				reportedHelpers[helper.object] = true
				pass.Reportf(helper.reset.Pos(), "fixed-count route benchmark helper %s invokes its selected arm only once before ResetTimer (paired arms: %s); use a short repeated untimed warmup or document a concrete initialization barrier; retain per-cell max/min as a disturbance diagnostic, select with repeated campaign medians, and re-run excursions before classifying recurrent instability", helper.fn.Name.Name, strings.Join(armNames, ", "))
			}
		}
	}
	return nil, nil
}

func ps6018DirectArms(pass *analysis.Pass, file *ast.File, fn *ast.FuncDecl) (*ast.CallExpr, []string, bool) {
	reset := ps6018FirstReset(pass, fn.Body)
	if reset == nil || ps6018HasBarrier(pass, file, ps6018FunctionStart(fn), reset.Pos()) {
		return nil, nil, false
	}
	stats := ps6018CallStats(pass, fn.Body, reset.Pos(), nil)
	classes := map[string][]ps6018CallKey{"control": nil, "candidate": nil}
	for key := range stats {
		if class := ps6018ArmClass(key.name); class != "" {
			classes[class] = append(classes[class], key)
		}
	}
	if len(classes["control"]) == 0 || len(classes["candidate"]) == 0 {
		return nil, nil, false
	}
	var under []string
	for _, class := range []string{"control", "candidate"} {
		for _, key := range classes[class] {
			stat := stats[key]
			if stat.direct == 1 && !stat.repeated {
				under = append(under, key.name)
			}
		}
	}
	slices.Sort(under)
	return reset, slices.Compact(under), len(under) > 0
}

func ps6018SubbenchArms(pass *analysis.Pass, file *ast.File, fn *ast.FuncDecl) (*ast.CallExpr, []string, bool) {
	type arm struct {
		label string
		body  *ast.BlockStmt
	}
	var arms []arm
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || !ps6018BRun(pass, call) || len(call.Args) != 2 {
			return true
		}
		label, ok := ps6018ConstantString(pass, call.Args[0])
		closure, closureOK := ps2110Unparen(call.Args[1]).(*ast.FuncLit)
		if !ok || !closureOK || closure.Body == nil {
			return true
		}
		arms = append(arms, arm{label: label, body: closure.Body})
		return false
	})
	labels := make([]string, 0, len(arms))
	for _, arm := range arms {
		labels = append(labels, arm.label)
	}
	if !ps6018ExplicitPairLabels(labels) {
		return nil, nil, false
	}
	var firstReset *ast.CallExpr
	var under []string
	for _, arm := range arms {
		reset := ps6018FirstReset(pass, arm.body)
		if reset == nil || ps6018HasBarrier(pass, file, arm.body.Pos(), reset.Pos()) {
			continue
		}
		if ps6018ClosureUnderwarmed(pass, arm.body, reset.Pos(), arm.label) {
			if firstReset == nil {
				firstReset = reset
			}
			under = append(under, arm.label)
		}
	}
	slices.Sort(under)
	return firstReset, under, firstReset != nil
}

func ps6018ClosureUnderwarmed(pass *analysis.Pass, body *ast.BlockStmt, before tokenPos, label string) bool {
	stats := ps6018CallStats(pass, body, before, nil)
	if len(stats) == 0 {
		return false
	}
	class := ps6018ArmClass(label)
	var classed []ps6018CallKey
	var last ps6018CallKey
	lastPos := tokenPos(0)
	for key, stat := range stats {
		if stat.repeated {
			return false
		}
		if class != "" && ps6018ArmClass(key.name) == class {
			classed = append(classed, key)
		}
		if stat.pos != nil && tokenPos(stat.pos.Pos()) > lastPos {
			last, lastPos = key, tokenPos(stat.pos.Pos())
		}
	}
	if len(classed) > 0 {
		for _, key := range classed {
			if stats[key].direct == 1 {
				return true
			}
		}
		return false
	}
	return last.name != "" && stats[last].direct == 1
}

// tokenPos is a local alias that keeps the helper signatures compact without
// mixing source offsets with integer evidence.
type tokenPos = token.Pos

func ps6018Helpers(pass *analysis.Pass) map[types.Object]ps6018Helper {
	result := make(map[types.Object]ps6018Helper)
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || fn.Recv != nil || !ps6018HasTestingBParam(pass, fn) {
				continue
			}
			fnObj, ok := pass.TypesInfo.Defs[fn.Name].(*types.Func)
			if !ok {
				continue
			}
			sig, ok := fnObj.Type().(*types.Signature)
			if !ok {
				continue
			}
			reset := ps6018FirstReset(pass, fn.Body)
			if reset == nil || ps6018HasBarrier(pass, file, ps6018FunctionStart(fn), reset.Pos()) {
				continue
			}
			for index := 0; index < sig.Params().Len(); index++ {
				param := sig.Params().At(index)
				if _, ok := types.Unalias(param.Type()).Underlying().(*types.Signature); !ok {
					continue
				}
				stats := ps6018CallStats(pass, fn.Body, reset.Pos(), param)
				key := ps6018CallKey{object: param, name: param.Name()}
				stat := stats[key]
				result[fnObj] = ps6018Helper{
					fn: fn, object: fnObj, arm: param, armIndex: index, reset: reset,
					under: stat.direct == 1 && !stat.repeated,
				}
				break
			}
		}
	}
	return result
}

func ps6018CalledHelpers(pass *analysis.Pass, fn *ast.FuncDecl, helpers map[types.Object]ps6018Helper) map[ps6018Helper][]string {
	seen := make(map[types.Object]map[string]bool)
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		object, _ := ps6018CallIdentity(pass, call)
		helper, ok := helpers[object]
		if !ok || helper.armIndex >= len(call.Args) {
			return true
		}
		name := ps6018ExprName(call.Args[helper.armIndex])
		if name == "" {
			return true
		}
		if seen[object] == nil {
			seen[object] = make(map[string]bool)
		}
		seen[object][name] = true
		return true
	})
	result := make(map[ps6018Helper][]string)
	for object, names := range seen {
		helper := helpers[object]
		list := make([]string, 0, len(names))
		control, candidate := false, false
		for name := range names {
			list = append(list, name)
			control = control || ps6018ArmClass(name) == "control"
			candidate = candidate || ps6018ArmClass(name) == "candidate"
		}
		slices.Sort(list)
		if len(list) == 2 || control && candidate {
			result[helper] = list
		}
	}
	return result
}

func ps6018CallStats(pass *analysis.Pass, body *ast.BlockStmt, before token.Pos, only types.Object) map[ps6018CallKey]ps6018Stats {
	result := make(map[ps6018CallKey]ps6018Stats)
	astutil.WithStack(body, func(node ast.Node, stack []ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok || call.Pos() >= before || ps6018ResetCall(pass, call) || ps6018TestingCall(pass, call) {
			return true
		}
		object, name := ps6018CallIdentity(pass, call)
		if object == nil || only != nil && object != only {
			return true
		}
		key := ps6018CallKey{object: object, name: name}
		stat := result[key]
		stat.pos = call
		if loop := ps6018EnclosingLoop(stack); loop != nil && ps6018LoopRepeats(pass, loop) {
			stat.repeated = true
		} else {
			stat.direct++
		}
		result[key] = stat
		return true
	})
	return result
}

func ps6018FirstReset(pass *analysis.Pass, body *ast.BlockStmt) *ast.CallExpr {
	var reset *ast.CallExpr
	ast.Inspect(body, func(node ast.Node) bool {
		if reset != nil {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if ok && ps6018ResetCall(pass, call) {
			reset = call
			return false
		}
		return true
	})
	return reset
}

func ps6018ResetCall(pass *analysis.Pass, call *ast.CallExpr) bool {
	fn, sig, ok := typedCallee(pass, call.Fun)
	return ok && sig.Recv() != nil && fn.Pkg() != nil && fn.Pkg().Path() == "testing" && fn.Name() == "ResetTimer"
}

func ps6018BRun(pass *analysis.Pass, call *ast.CallExpr) bool {
	fn, sig, ok := typedCallee(pass, call.Fun)
	return ok && sig.Recv() != nil && fn.Pkg() != nil && fn.Pkg().Path() == "testing" && fn.Name() == "Run"
}

func ps6018TestingCall(pass *analysis.Pass, call *ast.CallExpr) bool {
	fn, _, ok := typedCallee(pass, call.Fun)
	return ok && fn.Pkg() != nil && fn.Pkg().Path() == "testing"
}

func ps6018CallIdentity(pass *analysis.Pass, call *ast.CallExpr) (types.Object, string) {
	if fn, _, ok := typedCallee(pass, call.Fun); ok {
		return fn, fn.Name()
	}
	expr := ps2110Unparen(call.Fun)
	switch value := expr.(type) {
	case *ast.Ident:
		object := pass.TypesInfo.Uses[value]
		return object, value.Name
	case *ast.SelectorExpr:
		object := pass.TypesInfo.Uses[value.Sel]
		return object, value.Sel.Name
	}
	return nil, ""
}

func ps6018FunctionStart(fn *ast.FuncDecl) token.Pos {
	if fn.Doc != nil {
		return fn.Doc.Pos()
	}
	return fn.Pos()
}

func ps6018ExprName(expr ast.Expr) string {
	switch value := ps2110Unparen(expr).(type) {
	case *ast.Ident:
		return value.Name
	case *ast.SelectorExpr:
		return value.Sel.Name
	}
	return ""
}

func ps6018EnclosingLoop(stack []ast.Node) ast.Node {
	for index := len(stack) - 1; index >= 0; index-- {
		switch stack[index].(type) {
		case *ast.ForStmt, *ast.RangeStmt:
			return stack[index]
		}
	}
	return nil
}

func ps6018LoopRepeats(pass *analysis.Pass, loop ast.Node) bool {
	if rangeStmt, ok := loop.(*ast.RangeStmt); ok {
		tv, known := pass.TypesInfo.Types[ps2110Unparen(rangeStmt.X)]
		if known && tv.Value != nil && tv.Value.Kind() == constant.Int {
			value, exact := constant.Int64Val(tv.Value)
			return !exact || value > 1
		}
	}
	return true
}

func ps6018HasTestingBParam(pass *analysis.Pass, fn *ast.FuncDecl) bool {
	object, ok := pass.TypesInfo.Defs[fn.Name].(*types.Func)
	if !ok {
		return false
	}
	sig, ok := object.Type().(*types.Signature)
	if !ok {
		return false
	}
	for index := 0; index < sig.Params().Len(); index++ {
		ptr, ok := types.Unalias(sig.Params().At(index).Type()).(*types.Pointer)
		if !ok {
			continue
		}
		named, ok := types.Unalias(ptr.Elem()).(*types.Named)
		if ok && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == "testing" && named.Obj().Name() == "B" {
			return true
		}
	}
	return false
}

func ps6018HasBarrier(pass *analysis.Pass, file *ast.File, start, end token.Pos) bool {
	for _, group := range file.Comments {
		if group.End() < start || group.Pos() > end {
			continue
		}
		text := strings.ToLower(group.Text())
		if ps6007ContainsAny(text, "initialization barrier", "initialisation barrier", "no lazy initialization", "no lazy initialisation", "eager initialization", "eager initialisation", "preinitialized", "pre-initialized", "fully initialized") {
			return true
		}
	}
	found := false
	ast.Inspect(file, func(node ast.Node) bool {
		if found || node == nil || node.Pos() < start || node.Pos() > end {
			return !found
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		_, name := ps6018CallIdentity(pass, call)
		name = ps6007NormalizeName(name)
		found = ps6007ContainsAny(name, "initializationbarrier", "initialisationbarrier", "ensureinitialized", "ensureinitialised", "preinitialize", "preinitialise", "precompile", "compilepipeline", "warmupmany")
		return !found
	})
	return found
}

func ps6018ConstantString(pass *analysis.Pass, expr ast.Expr) (string, bool) {
	tv, ok := pass.TypesInfo.Types[ps2110Unparen(expr)]
	if !ok || tv.Value == nil || tv.Value.Kind() != constant.String {
		return "", false
	}
	return constant.StringVal(tv.Value), true
}

func ps6018ExplicitPairLabels(labels []string) bool {
	if len(labels) < 2 {
		return false
	}
	control, candidate := false, false
	for _, label := range labels {
		control = control || ps6018ArmClass(label) == "control"
		candidate = candidate || ps6018ArmClass(label) == "candidate"
	}
	if control && candidate {
		return true
	}
	return len(labels) == 2 && labels[0] != labels[1] &&
		ps6018BackendRole(labels[0]) && ps6018BackendRole(labels[1])
}

func ps6018BackendRole(name string) bool {
	name = ps6007NormalizeName(name)
	return ps6007ContainsAny(name, "cpu", "host", "reference", "ref", "gpu", "device", "metal", "vulkan", "cuda", "mps")
}

func ps6018ArmClass(name string) string {
	name = ps6007NormalizeName(name)
	switch {
	case ps6007ContainsAny(name, "control", "baseline", "reference", "incumbent"):
		return "control"
	case ps6007ContainsAny(name, "candidate", "optimized", "optimised", "experiment", "challenger", "newroute"):
		return "candidate"
	}
	return ""
}

func ps6018RouteContext(text string) bool {
	return ps6007ContainsAny(text, "route", "routing", "leadership", "crossover", "promotion")
}

func ps6018EvidenceContext(text string) bool {
	return ps6007ContainsAny(text,
		"benchtime", "fixedcount", "fixed count", "fixed-count", "-count", "max/min", "maxmin",
		"stability", "spread", "ceiling")
}
