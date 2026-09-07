package checks

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/constant"
	"go/format"
	"go/token"
	"go/types"
	"slices"
	"strconv"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS6102 implements owner issue #868. It finds performance assertions that
// ordinary tests and real subtests can still execute in testing.Short mode.
var PS6102 = register(&lint.Check{
	ID:       "PS6102",
	Category: "verify",
	Slug:     "performance-assertion-reachable-in-short-test",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "a performance pass/fail assertion remains reachable under testing.Short",
		Text: `Short mode is commonly used on shared or constrained runners, where wall
clock, throughput, allocation, and relative-speed thresholds are least stable.
An ordinary test that still enforces such a threshold under testing.Short can
turn a correct change into a red build because of runner load rather than a
functional regression.

This check implements owner issue #868. It analyzes exact func TestX(*testing.T)
roots, real testing.T.Run callbacks, and directly called same-package helpers or
local closures. It reports an ordered or equality comparison when one operand is
derived from time.Now/time.Since/time.Time.Sub, testing.Benchmark metrics,
testing.AllocsPerRun, BenchmarkResult allocation methods, or a numeric name that
identifies elapsed time, throughput, allocation count, bandwidth, speedup, or a
performance ratio; and a reachable branch can fail through testing.T
Fatal/Fatalf/Error/Errorf/Fail/FailNow or the predeclared panic builtin.
Callees and testing receivers are resolved through go/types, so aliased imports
work while same-named user functions and fake testing methods stay silent.

Reachability is evaluated with testing.Short() fixed to true. Constant Boolean
expressions, source-order &&/|| short circuiting, immutable aliases, early
returns, typed Skip/Skipf/SkipNow calls, guarded branches, and inverse else
branches are followed. An overwritten alias becomes unknown: a possible
short-mode failure is intentionally not suppressed. A Short call after the
assertion, the wrong guard polarity, a log-only guard, or a guard combined with
an unknown condition also cannot prove the assertion unreachable.

The analysis is deliberately bounded rather than a full Go interpreter. It
follows structured if/loop/switch blocks plus direct calls to immutable local
closures and same-package functions to a fixed depth. Dynamic function values,
interface dispatch, goroutines, reflection, and interprocedural mutation are
conservative boundaries; when their behavior is needed to prove a skip, the
assertion remains reportable. Only actual Benchmark roots are excluded; an
ordinary test that calls testing.Benchmark is still an ordinary short-mode gate.
Metric-only logging remains silent.

Move only the performance measurement and threshold behind an early
testing.Short skip or a !testing.Short branch. Keep correctness assertions
outside that skipped block, or put them in a separate unconditional test. There
is NO automatic fix because inserting a function-wide skip could silently remove
correctness coverage.`,
		Before: `func TestCampaign(t *testing.T) {
	got := runCorrectnessChecks(t)
	start := time.Now()
	runCampaign(got)
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("campaign took %v", elapsed)
	}
}`,
		After: `func TestCampaign(t *testing.T) {
	got := runCorrectnessChecks(t) // always retained
	if testing.Short() {
		return
	}
	start := time.Now()
	runCampaign(got)
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("campaign took %v", elapsed)
	}
}`,
		MeasuredWin: `The macOS Metal failure behind owner issue #868 ran a 1.20x
speedup assertion in a -short lane. Correctness passed, but three hosted-runner
campaigns measured 1.1042x, 1.0645x, and 1.1188x (1.0755x aggregate), making an
otherwise green matrix fail on environment-sensitive timing.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6102",
		Doc:  "performance assertion remains reachable when testing.Short is true",
		Run:  runPS6102,
	},
})

const (
	ps6102False = 1 << iota
	ps6102True
	ps6102CallDepth = 12
)

type ps6102Bool struct {
	mask       uint8
	thresholds []*ast.BinaryExpr
}

type ps6102Outcome struct {
	continues    bool
	returned     bool
	failed       bool
	returnStates []ps6102State
}

type ps6102State struct {
	mutations map[types.Object]bool
	aliases   map[types.Object]types.Object
}

type ps6102Context struct {
	flow      ps6064Flow
	closures  map[types.Object]*ast.FuncLit
	mutations map[types.Object]bool
	aliases   map[types.Object]types.Object
	testName  string
	depth     int
	callStack map[ast.Node]bool
}

type ps6102Runner struct {
	pass      *analysis.Pass
	functions map[types.Object]*ast.FuncDecl
	reported  map[string]bool
}

func runPS6102(pass *analysis.Pass) (any, error) {
	runner := &ps6102Runner{
		pass:      pass,
		functions: make(map[types.Object]*ast.FuncDecl),
		reported:  make(map[string]bool),
	}
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			if object := pass.TypesInfo.Defs[function.Name]; object != nil {
				runner.functions[object] = function
			}
		}
	}
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil || !ps6067OrdinaryTest(pass, function) {
				continue
			}
			closures := ps6102ClosureBindings(pass, function.Body)
			context := ps6102Context{
				flow:      ps6064FunctionFlow(pass, function),
				closures:  closures,
				mutations: make(map[types.Object]bool),
				aliases:   make(map[types.Object]types.Object),
				testName:  function.Name.Name,
				callStack: map[ast.Node]bool{function: true},
			}
			runner.block(function.Body.List, context, true)
		}
	}
	return nil, nil
}

// block executes structured statements with testing.Short fixed to true. Its
// outcome distinguishes a local return from an aborting Skip/Fatal call so a
// helper's return does not incorrectly terminate its caller.
func (runner *ps6102Runner) block(statements []ast.Stmt, context ps6102Context, report bool) ps6102Outcome {
	result := ps6102Outcome{continues: true}
	for _, statement := range statements {
		if !result.continues {
			break
		}
		current := runner.statement(statement, context, report)
		result.continues = current.continues
		result.returned = result.returned || current.returned
		result.failed = result.failed || current.failed
		result.returnStates = append(result.returnStates, current.returnStates...)
	}
	return result
}

func (runner *ps6102Runner) statement(statement ast.Stmt, context ps6102Context, report bool) ps6102Outcome {
	switch value := statement.(type) {
	case *ast.BlockStmt:
		return runner.block(value.List, context, report)
	case *ast.ReturnStmt:
		return ps6102Outcome{returned: true, returnStates: []ps6102State{ps6102Snapshot(context)}}
	case *ast.IncDecStmt:
		if object := runner.pointerTarget(value.X, context, map[types.Object]bool{}); object != nil {
			context.mutations[object] = true
		} else if identifier, ok := ps2110Unparen(value.X).(*ast.Ident); ok {
			if object := runner.pass.TypesInfo.ObjectOf(identifier); object != nil {
				context.mutations[object] = true
			}
		}
		return ps6102Outcome{continues: true}
	case *ast.AssignStmt:
		result := runner.expressionStatements(value.Rhs, context, report)
		if result.continues {
			runner.recordAssignment(value, context)
		}
		return result
	case *ast.DeclStmt:
		declaration, ok := value.Decl.(*ast.GenDecl)
		if !ok {
			return ps6102Outcome{continues: true}
		}
		result := ps6102Outcome{continues: true}
		for _, specification := range declaration.Specs {
			if values, ok := specification.(*ast.ValueSpec); ok {
				current := runner.expressionStatements(values.Values, context, report)
				result.failed = result.failed || current.failed
				if !current.continues {
					result.continues = false
					break
				}
				runner.recordValueSpec(values, context)
			}
		}
		return result
	case *ast.ExprStmt:
		call, ok := ps2110Unparen(value.X).(*ast.CallExpr)
		if !ok {
			return ps6102Outcome{continues: true}
		}
		return runner.call(call, context, report)
	case *ast.IfStmt:
		if value.Init != nil {
			initialized := runner.statement(value.Init, context, report)
			if !initialized.continues {
				return initialized
			}
		}
		return runner.ifStatement(value, context, report)
	case *ast.ForStmt:
		condition := ps6102Bool{mask: ps6102True}
		if value.Cond != nil {
			condition = runner.boolean(value.Cond, context, map[types.Object]bool{})
		}
		body := ps6102Outcome{}
		if condition.mask&ps6102True != 0 {
			bodyContext := ps6102CloneContext(context)
			body = runner.block(value.Body.List, bodyContext, report)
			var continuing []ps6102State
			if condition.mask&ps6102False != 0 {
				continuing = append(continuing, ps6102Snapshot(context))
			}
			if body.continues {
				continuing = append(continuing, ps6102Snapshot(bodyContext))
			}
			ps6102MergeStates(context, continuing)
		}
		if condition.mask == ps6102True && !body.continues {
			return body
		}
		return ps6102Outcome{continues: true, returned: body.returned, failed: body.failed, returnStates: body.returnStates}
	case *ast.RangeStmt:
		if ps6102DefinitelyEmptyRange(runner.pass, value.X) {
			return ps6102Outcome{continues: true}
		}
		bodyContext := ps6102CloneContext(context)
		body := runner.block(value.Body.List, bodyContext, report)
		continuing := []ps6102State{ps6102Snapshot(context)}
		if body.continues {
			continuing = append(continuing, ps6102Snapshot(bodyContext))
		}
		ps6102MergeStates(context, continuing)
		return ps6102Outcome{continues: true, returned: body.returned, failed: body.failed, returnStates: body.returnStates}
	case *ast.SwitchStmt:
		if value.Init != nil {
			initialized := runner.statement(value.Init, context, report)
			if !initialized.continues {
				return initialized
			}
		}
		return runner.switchStatement(value, context, report)
	case *ast.TypeSwitchStmt:
		return runner.clauses(value.Body.List, context, report)
	case *ast.SelectStmt:
		return runner.clauses(value.Body.List, context, report)
	case *ast.LabeledStmt:
		return runner.statement(value.Stmt, context, report)
	default:
		return ps6102Outcome{continues: true}
	}
}

func (runner *ps6102Runner) switchStatement(statement *ast.SwitchStmt, context ps6102Context, report bool) ps6102Outcome {
	if ps6102HasFallthrough(statement.Body) {
		return runner.clauses(statement.Body.List, context, report)
	}
	tag := ps6102Bool{mask: ps6102True}
	if statement.Tag != nil {
		tag = runner.boolean(statement.Tag, context, map[types.Object]bool{})
	}
	if tag.mask != ps6102False && tag.mask != ps6102True {
		return runner.clauses(statement.Body.List, context, report)
	}
	var defaultClause *ast.CaseClause
	for _, node := range statement.Body.List {
		clause, ok := node.(*ast.CaseClause)
		if !ok {
			continue
		}
		if len(clause.List) == 0 {
			defaultClause = clause
			continue
		}
		known := true
		matched := false
		for _, expression := range clause.List {
			candidate := runner.boolean(expression, context, map[types.Object]bool{})
			if candidate.mask != ps6102False && candidate.mask != ps6102True {
				known = false
				break
			}
			if candidate.mask == tag.mask {
				matched = true
			}
		}
		if !known {
			return runner.clauses(statement.Body.List, context, report)
		}
		if matched {
			return runner.block(clause.Body, context, report)
		}
	}
	if defaultClause != nil {
		return runner.block(defaultClause.Body, context, report)
	}
	return ps6102Outcome{continues: true}
}

func ps6102HasFallthrough(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(node ast.Node) bool {
		if found {
			return false
		}
		if branch, ok := node.(*ast.BranchStmt); ok && branch.Tok == token.FALLTHROUGH {
			found = true
			return false
		}
		return true
	})
	return found
}

func ps6102DefinitelyEmptyRange(pass *analysis.Pass, expression ast.Expr) bool {
	valueType := pass.TypesInfo.TypeOf(expression)
	if valueType != nil {
		switch value := types.Unalias(valueType).Underlying().(type) {
		case *types.Array:
			return value.Len() == 0
		case *types.Basic:
			if value.Kind() == types.String {
				if typed := pass.TypesInfo.Types[ps2110Unparen(expression)].Value; typed != nil && typed.Kind() == constant.String {
					return constant.StringVal(typed) == ""
				}
			}
		}
	}
	if literal, ok := ps2110Unparen(expression).(*ast.CompositeLit); ok && len(literal.Elts) == 0 {
		_, slice := types.Unalias(pass.TypesInfo.TypeOf(literal)).Underlying().(*types.Slice)
		return slice
	}
	return false
}

func (runner *ps6102Runner) expressionStatements(expressions []ast.Expr, context ps6102Context, report bool) ps6102Outcome {
	result := ps6102Outcome{continues: true}
	for _, expression := range expressions {
		call, ok := ps2110Unparen(expression).(*ast.CallExpr)
		if !ok {
			continue
		}
		current := runner.call(call, context, report)
		result.failed = result.failed || current.failed
		if !current.continues {
			result.continues = false
			break
		}
	}
	return result
}

func (runner *ps6102Runner) ifStatement(statement *ast.IfStmt, context ps6102Context, report bool) ps6102Outcome {
	condition := runner.boolean(statement.Cond, context, map[types.Object]bool{})
	trueOutcome := ps6102Outcome{}
	falseOutcome := ps6102Outcome{continues: true}
	if condition.mask&ps6102True != 0 {
		trueOutcome = runner.block(statement.Body.List, ps6102CloneContext(context), false)
	}
	if condition.mask&ps6102False != 0 && statement.Else != nil {
		falseOutcome = runner.statement(statement.Else, ps6102CloneContext(context), false)
	}
	if report && (condition.mask&ps6102True != 0 && trueOutcome.failed ||
		condition.mask&ps6102False != 0 && falseOutcome.failed) {
		for _, comparison := range condition.thresholds {
			falseCondition := runner.booleanOverride(statement.Cond, context, comparison, ps6102False)
			trueCondition := runner.booleanOverride(statement.Cond, context, comparison, ps6102True)
			if falseCondition.mask != trueCondition.mask && trueOutcome.failed != falseOutcome.failed {
				runner.report(comparison, context.testName)
			}
		}
	}

	var continuing []ps6102State
	if condition.mask&ps6102True != 0 {
		trueContext := ps6102CloneContext(context)
		trueOutcome = runner.block(statement.Body.List, trueContext, report)
		if trueOutcome.continues {
			continuing = append(continuing, ps6102Snapshot(trueContext))
		}
	}
	if condition.mask&ps6102False != 0 {
		falseContext := ps6102CloneContext(context)
		if statement.Else != nil {
			falseOutcome = runner.statement(statement.Else, falseContext, report)
		}
		if falseOutcome.continues {
			continuing = append(continuing, ps6102Snapshot(falseContext))
		}
	}
	ps6102MergeStates(context, continuing)
	return ps6102Outcome{
		continues: condition.mask&ps6102True != 0 && trueOutcome.continues ||
			condition.mask&ps6102False != 0 && falseOutcome.continues,
		returned: condition.mask&ps6102True != 0 && trueOutcome.returned ||
			condition.mask&ps6102False != 0 && falseOutcome.returned,
		failed: condition.mask&ps6102True != 0 && trueOutcome.failed ||
			condition.mask&ps6102False != 0 && falseOutcome.failed,
		returnStates: slices.Concat(trueOutcome.returnStates, falseOutcome.returnStates),
	}
}

func (runner *ps6102Runner) clauses(nodes []ast.Stmt, context ps6102Context, report bool) ps6102Outcome {
	result := ps6102Outcome{}
	var continuing []ps6102State
	hasDefault := false
	for _, node := range nodes {
		var body []ast.Stmt
		switch clause := node.(type) {
		case *ast.CaseClause:
			body = clause.Body
			hasDefault = hasDefault || len(clause.List) == 0
		case *ast.CommClause:
			body = clause.Body
			hasDefault = hasDefault || clause.Comm == nil
		default:
			continue
		}
		branchContext := ps6102CloneContext(context)
		outcome := runner.block(body, branchContext, report)
		result.returned = result.returned || outcome.returned
		result.failed = result.failed || outcome.failed
		result.returnStates = append(result.returnStates, outcome.returnStates...)
		if outcome.continues {
			continuing = append(continuing, ps6102Snapshot(branchContext))
		}
	}
	if !hasDefault {
		continuing = append(continuing, ps6102Snapshot(context))
	}
	result.continues = len(continuing) != 0
	ps6102MergeStates(context, continuing)
	return result
}

func (runner *ps6102Runner) call(call *ast.CallExpr, context ps6102Context, report bool) ps6102Outcome {
	if typedBuiltinName(runner.pass, call.Fun, "panic") {
		return ps6102Outcome{failed: true}
	}
	if ps6067TestingTMethod(runner.pass, call, "Error", "Errorf", "Fail") {
		return ps6102Outcome{continues: true, failed: true}
	}
	if ps6067TestingTMethod(runner.pass, call, "Fatal", "Fatalf", "FailNow") {
		return ps6102Outcome{failed: true}
	}
	if ps6067TestingTMethod(runner.pass, call, "Skip", "Skipf", "SkipNow") {
		return ps6102Outcome{}
	}
	if runner.testingRun(call) {
		runner.subtest(call, context, report)
		return ps6102Outcome{continues: true}
	}
	outcome, ok := runner.invoke(call.Fun, call.Args, context, report, context.testName)
	if !ok {
		for _, argument := range call.Args {
			if object := runner.pointerTarget(argument, context, map[types.Object]bool{}); object != nil {
				context.mutations[object] = true
			}
		}
		return ps6102Outcome{continues: true}
	}
	return ps6102Outcome{
		continues: outcome.continues || outcome.returned,
		failed:    outcome.failed,
	}
}

func (runner *ps6102Runner) testingRun(call *ast.CallExpr) bool {
	function, signature, ok := typedCallee(runner.pass, call.Fun)
	return ok && function.Name() == "Run" && typedReceiverNamed(signature, "testing", "T") && len(call.Args) == 2
}

func (runner *ps6102Runner) subtest(call *ast.CallExpr, context ps6102Context, report bool) {
	name := ps6102Expression(runner.pass, call.Args[0])
	if value := runner.pass.TypesInfo.Types[ps2110Unparen(call.Args[0])].Value; value != nil && value.Kind() == constant.String {
		name = constant.StringVal(value)
	}
	testName := context.testName + "/" + name
	runner.invoke(call.Args[1], nil, context, report, testName)
}

func (runner *ps6102Runner) invoke(
	callee ast.Expr,
	arguments []ast.Expr,
	context ps6102Context,
	report bool,
	testName string,
) (ps6102Outcome, bool) {
	if context.depth >= ps6102CallDepth {
		return ps6102Outcome{}, false
	}
	var node ast.Node
	var body *ast.BlockStmt
	var signature *types.Signature
	switch value := ps2110Unparen(callee).(type) {
	case *ast.FuncLit:
		node, body = value, value.Body
		signature, _ = runner.pass.TypesInfo.TypeOf(value).(*types.Signature)
	case *ast.Ident:
		object := runner.pass.TypesInfo.ObjectOf(value)
		if literal := context.closures[object]; literal != nil {
			node, body = literal, literal.Body
			signature, _ = runner.pass.TypesInfo.TypeOf(literal).(*types.Signature)
		} else if function := runner.functions[object]; function != nil {
			node, body = function, function.Body
			if typed, ok := runner.pass.TypesInfo.Defs[function.Name].(*types.Func); ok {
				signature, _ = typed.Type().(*types.Signature)
			}
		}
	case *ast.SelectorExpr:
		if function := runner.functions[runner.pass.TypesInfo.ObjectOf(value.Sel)]; function != nil {
			node, body = function, function.Body
			if typed, ok := runner.pass.TypesInfo.Defs[function.Name].(*types.Func); ok {
				signature, _ = typed.Type().(*types.Signature)
			}
		}
	}
	if node == nil || body == nil || context.callStack[node] {
		return ps6102Outcome{}, false
	}

	flow := ps6067MergeFlows(context.flow, ps6067BodyFlow(runner.pass, body))
	if signature != nil {
		limit := min(signature.Params().Len(), len(arguments))
		for index := 0; index < limit; index++ {
			flow.initializers[signature.Params().At(index)] = arguments[index]
		}
	}
	closures := make(map[types.Object]*ast.FuncLit, len(context.closures))
	for object, literal := range context.closures {
		closures[object] = literal
	}
	for object, literal := range ps6102ClosureBindings(runner.pass, body) {
		closures[object] = literal
	}
	stack := make(map[ast.Node]bool, len(context.callStack)+1)
	for callNode := range context.callStack {
		stack[callNode] = true
	}
	stack[node] = true
	invoked := ps6102Context{
		flow:      flow,
		closures:  closures,
		mutations: ps6102CopyMutations(context.mutations),
		aliases:   ps6102CopyAliases(context.aliases),
		testName:  testName,
		depth:     context.depth + 1,
		callStack: stack,
	}
	if signature != nil {
		limit := min(signature.Params().Len(), len(arguments))
		for index := 0; index < limit; index++ {
			if target := runner.pointerTarget(arguments[index], context, map[types.Object]bool{}); target != nil {
				invoked.aliases[signature.Params().At(index)] = target
			}
		}
	}
	outcome := runner.block(body.List, invoked, report)
	normalStates := slices.Clone(outcome.returnStates)
	if outcome.continues {
		normalStates = append(normalStates, ps6102Snapshot(invoked))
	}
	ps6102MergeStates(context, normalStates)
	return outcome, true
}

func (runner *ps6102Runner) boolean(expression ast.Expr, context ps6102Context, seen map[types.Object]bool) ps6102Bool {
	return runner.booleanValue(expression, context, seen, nil, 0)
}

func (runner *ps6102Runner) booleanOverride(expression ast.Expr, context ps6102Context, target *ast.BinaryExpr, mask uint8) ps6102Bool {
	return runner.booleanValue(expression, context, map[types.Object]bool{}, target, mask)
}

func (runner *ps6102Runner) booleanValue(
	expression ast.Expr,
	context ps6102Context,
	seen map[types.Object]bool,
	target *ast.BinaryExpr,
	targetMask uint8,
) ps6102Bool {
	expression = ps2110Unparen(expression)
	if typed, ok := runner.pass.TypesInfo.Types[expression]; ok && typed.Value != nil && typed.Value.Kind() == constant.Bool {
		if constant.BoolVal(typed.Value) {
			return ps6102Bool{mask: ps6102True}
		}
		return ps6102Bool{mask: ps6102False}
	}
	switch value := expression.(type) {
	case *ast.Ident:
		object := runner.pass.TypesInfo.ObjectOf(value)
		if object != nil && !seen[object] && !context.mutations[object] {
			if initializer := context.flow.initializers[object]; initializer != nil {
				seen[object] = true
				result := runner.booleanValue(initializer, context, seen, target, targetMask)
				delete(seen, object)
				return result
			}
		}
	case *ast.CallExpr:
		if ps6067PackageCall(runner.pass, value, "testing", "Short") && len(value.Args) == 0 {
			return ps6102Bool{mask: ps6102True}
		}
	case *ast.UnaryExpr:
		if value.Op == token.NOT {
			result := runner.booleanValue(value.X, context, seen, target, targetMask)
			result.mask = ps6102Negate(result.mask)
			return result
		}
	case *ast.BinaryExpr:
		if value == target {
			return ps6102Bool{mask: targetMask}
		}
		switch value.Op {
		case token.LAND:
			left := runner.booleanValue(value.X, context, seen, target, targetMask)
			result := ps6102Bool{thresholds: left.thresholds}
			if left.mask&ps6102False != 0 {
				result.mask |= ps6102False
			}
			if left.mask&ps6102True != 0 {
				right := runner.booleanValue(value.Y, context, seen, target, targetMask)
				result.mask |= right.mask
				result.thresholds = ps6102AppendThresholds(result.thresholds, right.thresholds...)
			}
			return result
		case token.LOR:
			left := runner.booleanValue(value.X, context, seen, target, targetMask)
			result := ps6102Bool{thresholds: left.thresholds}
			if left.mask&ps6102True != 0 {
				result.mask |= ps6102True
			}
			if left.mask&ps6102False != 0 {
				right := runner.booleanValue(value.Y, context, seen, target, targetMask)
				result.mask |= right.mask
				result.thresholds = ps6102AppendThresholds(result.thresholds, right.thresholds...)
			}
			return result
		default:
			if (value.Op == token.EQL || value.Op == token.NEQ) &&
				ps6067BooleanType(runner.pass.TypesInfo.TypeOf(value.X)) &&
				ps6067BooleanType(runner.pass.TypesInfo.TypeOf(value.Y)) {
				left := runner.booleanValue(value.X, context, seen, target, targetMask)
				right := runner.booleanValue(value.Y, context, seen, target, targetMask)
				return ps6102BooleanEquality(left, right, value.Op == token.NEQ)
			}
			if ps6102Comparison(value.Op) && runner.performanceComparison(value, context.flow) {
				return ps6102Bool{mask: ps6102False | ps6102True, thresholds: []*ast.BinaryExpr{value}}
			}
		}
	}
	return ps6102Bool{mask: ps6102False | ps6102True}
}

func ps6102BooleanEquality(left, right ps6102Bool, negate bool) ps6102Bool {
	result := ps6102Bool{thresholds: ps6102AppendThresholds(left.thresholds, right.thresholds...)}
	for _, leftValue := range []uint8{ps6102False, ps6102True} {
		if left.mask&leftValue == 0 {
			continue
		}
		for _, rightValue := range []uint8{ps6102False, ps6102True} {
			if right.mask&rightValue == 0 {
				continue
			}
			equal := leftValue == rightValue
			if equal != negate {
				result.mask |= ps6102True
			} else {
				result.mask |= ps6102False
			}
		}
	}
	return result
}

func ps6102Negate(mask uint8) uint8 {
	var result uint8
	if mask&ps6102False != 0 {
		result |= ps6102True
	}
	if mask&ps6102True != 0 {
		result |= ps6102False
	}
	return result
}

func ps6102Comparison(operator token.Token) bool {
	return ps6067OrderedComparison(operator) || operator == token.EQL || operator == token.NEQ
}

func ps6102AppendThresholds(existing []*ast.BinaryExpr, values ...*ast.BinaryExpr) []*ast.BinaryExpr {
	for _, value := range values {
		found := false
		for _, current := range existing {
			if current == value {
				found = true
				break
			}
		}
		if !found {
			existing = append(existing, value)
		}
	}
	return existing
}

func (runner *ps6102Runner) performanceComparison(comparison *ast.BinaryExpr, flow ps6064Flow) bool {
	return runner.performanceExpression(comparison.X, flow, map[types.Object]bool{}) ||
		runner.performanceExpression(comparison.Y, flow, map[types.Object]bool{})
}

func (runner *ps6102Runner) performanceExpression(expression ast.Expr, flow ps6064Flow, seen map[types.Object]bool) bool {
	if runner.timingExpression(expression, flow, map[types.Object]bool{}).kind == ps6067Measurement {
		return true
	}
	expression = ps2110Unparen(expression)
	switch value := expression.(type) {
	case *ast.Ident:
		object := runner.pass.TypesInfo.ObjectOf(value)
		if ps6102PerformanceName(value.Name) && object != nil && ps6067NumericType(object.Type()) {
			return true
		}
		if object != nil && !seen[object] && flow.writes[object] == 0 {
			if initializer := flow.initializers[object]; initializer != nil {
				seen[object] = true
				result := runner.performanceExpression(initializer, flow, seen)
				delete(seen, object)
				return result
			}
		}
	case *ast.CallExpr:
		if ps6067PackageCall(runner.pass, value, "testing", "AllocsPerRun") {
			return true
		}
		if function, signature, ok := typedCallee(runner.pass, value.Fun); ok &&
			typedReceiverNamed(signature, "testing", "BenchmarkResult") &&
			(function.Name() == "AllocsPerOp" || function.Name() == "AllocedBytesPerOp") {
			return true
		}
	case *ast.BinaryExpr:
		return runner.performanceExpression(value.X, flow, seen) || runner.performanceExpression(value.Y, flow, seen)
	case *ast.UnaryExpr:
		return runner.performanceExpression(value.X, flow, seen)
	case *ast.SelectorExpr:
		return ps6102PerformanceName(value.Sel.Name) && ps6067NumericType(runner.pass.TypesInfo.TypeOf(value)) ||
			runner.performanceExpression(value.X, flow, seen)
	case *ast.IndexExpr:
		return runner.performanceExpression(value.X, flow, seen)
	}
	return false
}

// timingExpression retains PS6067's typed timing vocabulary without inheriting
// its policy of propagating metric provenance through every arbitrary call
// argument. A result is a metric only when its own operation or immutable name
// proves that dependency.
func (runner *ps6102Runner) timingExpression(expression ast.Expr, flow ps6064Flow, seen map[types.Object]bool) ps6067TimingInfo {
	expression = ps2110Unparen(expression)
	switch value := expression.(type) {
	case *ast.Ident:
		object := runner.pass.TypesInfo.ObjectOf(value)
		if object == nil {
			return ps6067TimingInfo{}
		}
		if initializer := flow.initializers[object]; initializer != nil && flow.writes[object] == 0 && !seen[object] {
			seen[object] = true
			derived := runner.timingExpression(initializer, flow, seen)
			delete(seen, object)
			if derived.kind != ps6067NoTiming {
				return derived
			}
		}
		if ps6067TimingName(value.Name) && ps6067NumericType(object.Type()) {
			return ps6067TimingInfo{kind: ps6067Measurement, source: "named elapsed/device timing"}
		}
	case *ast.CallExpr:
		return runner.timingCall(value, flow, seen)
	case *ast.BinaryExpr:
		left := runner.timingExpression(value.X, flow, seen)
		right := runner.timingExpression(value.Y, flow, seen)
		result := ps6067MergeTiming(left, right)
		if value.Op == token.SUB && left.kind == ps6067ClockPoint && right.kind == ps6067ClockPoint {
			result.kind = ps6067Measurement
			result.source = ps6067FirstSource(left.source, right.source, "clock delta")
		}
		return result
	case *ast.UnaryExpr:
		return runner.timingExpression(value.X, flow, seen)
	case *ast.SelectorExpr:
		return runner.timingExpression(value.X, flow, seen)
	case *ast.IndexExpr:
		return runner.timingExpression(value.X, flow, seen)
	case *ast.SliceExpr:
		return runner.timingExpression(value.X, flow, seen)
	case *ast.StarExpr:
		return runner.timingExpression(value.X, flow, seen)
	}
	return ps6067TimingInfo{}
}

func (runner *ps6102Runner) timingCall(call *ast.CallExpr, flow ps6064Flow, seen map[types.Object]bool) ps6067TimingInfo {
	if ps6067PackageCall(runner.pass, call, "time", "Now") {
		return ps6067TimingInfo{kind: ps6067ClockPoint, source: "time.Now-derived elapsed time"}
	}
	if ps6067PackageCall(runner.pass, call, "time", "Since") {
		return ps6067TimingInfo{kind: ps6067Measurement, source: "time.Since-derived elapsed time"}
	}
	if ps6067PackageCall(runner.pass, call, "testing", "Benchmark") {
		return ps6067TimingInfo{kind: ps6067Measurement, source: "testing.Benchmark result"}
	}
	if ps6067PackageCall(runner.pass, call, "testing", "AllocsPerRun") {
		return ps6067TimingInfo{kind: ps6067Measurement, source: "testing.AllocsPerRun result"}
	}

	if typed, ok := runner.pass.TypesInfo.Types[ps2110Unparen(call.Fun)]; ok && typed.IsType() {
		if len(call.Args) != 1 {
			return ps6067TimingInfo{}
		}
		return runner.timingExpression(call.Args[0], flow, seen)
	}

	function, signature, ok := typedCallee(runner.pass, call.Fun)
	if !ok {
		return ps6067TimingInfo{}
	}
	name := function.Name()
	var receiver ps6067TimingInfo
	if selector, ok := ps2110Unparen(call.Fun).(*ast.SelectorExpr); ok {
		receiver = runner.timingExpression(selector.X, flow, seen)
	}
	if typedReceiverNamed(signature, "time", "Time") {
		switch name {
		case "Sub":
			return ps6067TimingInfo{kind: ps6067Measurement, source: "time.Time.Sub-derived elapsed time"}
		case "Unix", "UnixMilli", "UnixMicro", "UnixNano":
			return ps6067TimingInfo{kind: ps6067ClockPoint, source: "time.Time clock value"}
		}
	}
	if typedReceiverNamed(signature, "testing", "BenchmarkResult") {
		switch name {
		case "NsPerOp", "AllocsPerOp", "AllocedBytesPerOp":
			return ps6067TimingInfo{kind: ps6067Measurement, source: "testing.Benchmark result metric"}
		}
	}
	if typedReceiverNamed(signature, "time", "Duration") &&
		ps6007ContainsAny(ps6007NormalizeName(name), "hours", "minutes", "seconds", "milliseconds", "microseconds", "nanoseconds") {
		return receiver
	}
	if ps6067NumericType(runner.pass.TypesInfo.TypeOf(call)) {
		switch ps6067TimingCallKind(name) {
		case ps6067ClockPoint:
			return ps6067TimingInfo{kind: ps6067ClockPoint, source: "device clock value"}
		case ps6067Measurement:
			return ps6067TimingInfo{kind: ps6067Measurement, source: "device elapsed/latency timing"}
		}
		if ps6102PerformanceName(name) {
			return ps6067TimingInfo{kind: ps6067Measurement, source: "named performance metric"}
		}
	}
	return ps6067TimingInfo{}
}

func ps6102PerformanceName(name string) bool {
	name = ps6007NormalizeName(name)
	if name == "allocs" {
		return true
	}
	return ps6067TimingName(name) || ps6007ContainsAny(name,
		"allocation", "alloccount", "allocsperop", "allocatedbytes", "bytesperop",
		"bandwidth", "itemspersecond", "speedup", "slowdown", "performanceratio",
		"perfratio", "throughputratio", "latencyratio", "timeratio")
}

func (runner *ps6102Runner) report(comparison *ast.BinaryExpr, testName string) {
	key := fmt.Sprintf("%d:%s", comparison.Pos(), testName)
	if runner.reported[key] {
		return
	}
	runner.reported[key] = true
	expression := ps6102Expression(runner.pass, comparison)
	runner.pass.Reportf(comparison.Pos(), "performance threshold %s in %s remains reachable when testing.Short() is true; skip only the performance assertion or measurement block and keep correctness checks unconditional (advisory, no automatic fix)", strconv.Quote(expression), testName)
}

func ps6102Expression(pass *analysis.Pass, expression ast.Expr) string {
	var output bytes.Buffer
	if err := format.Node(&output, pass.Fset, expression); err == nil {
		return output.String()
	}
	return "performance comparison"
}

func ps6102ClosureBindings(pass *analysis.Pass, body *ast.BlockStmt) map[types.Object]*ast.FuncLit {
	bindings := make(map[types.Object]*ast.FuncLit)
	writes := make(map[types.Object]int)
	ast.Inspect(body, func(node ast.Node) bool {
		if node != body {
			if _, nested := node.(*ast.FuncLit); nested {
				return false
			}
		}
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
				writes[object]++
				if len(value.Lhs) == len(value.Rhs) {
					if literal, ok := ps2110Unparen(value.Rhs[index]).(*ast.FuncLit); ok {
						bindings[object] = literal
					}
				}
			}
		case *ast.ValueSpec:
			for index, name := range value.Names {
				object := pass.TypesInfo.Defs[name]
				if object == nil {
					continue
				}
				writes[object]++
				if len(value.Names) == len(value.Values) {
					if literal, ok := ps2110Unparen(value.Values[index]).(*ast.FuncLit); ok {
						bindings[object] = literal
					}
				}
			}
		}
		return true
	})
	for object, count := range writes {
		if count != 1 {
			delete(bindings, object)
		}
	}
	return bindings
}

func ps6102CloneContext(context ps6102Context) ps6102Context {
	context.mutations = ps6102CopyMutations(context.mutations)
	context.aliases = ps6102CopyAliases(context.aliases)
	return context
}

func ps6102Snapshot(context ps6102Context) ps6102State {
	return ps6102State{
		mutations: ps6102CopyMutations(context.mutations),
		aliases:   ps6102CopyAliases(context.aliases),
	}
}

func ps6102CopyMutations(source map[types.Object]bool) map[types.Object]bool {
	result := make(map[types.Object]bool, len(source))
	for object, mutated := range source {
		if mutated {
			result[object] = true
		}
	}
	return result
}

func ps6102CopyAliases(source map[types.Object]types.Object) map[types.Object]types.Object {
	result := make(map[types.Object]types.Object, len(source))
	for object, target := range source {
		result[object] = target
	}
	return result
}

func ps6102MergeStates(context ps6102Context, states []ps6102State) {
	if len(states) == 0 {
		return
	}
	mutations := make(map[types.Object]bool)
	aliases := make(map[types.Object]types.Object)
	pointers := make(map[types.Object]bool)
	for _, state := range states {
		for object, mutated := range state.mutations {
			if mutated {
				mutations[object] = true
			}
		}
		for pointer := range state.aliases {
			pointers[pointer] = true
		}
	}
	for pointer := range pointers {
		var target types.Object
		consistent := true
		for _, state := range states {
			current, present := state.aliases[pointer]
			if !present || target != nil && current != target {
				consistent = false
			}
			if current != nil {
				if target == nil {
					target = current
				}
				if !consistent {
					mutations[current] = true
				}
			}
		}
		if consistent && target != nil {
			aliases[pointer] = target
		} else if target != nil {
			mutations[target] = true
		}
	}
	clear(context.mutations)
	for object := range mutations {
		context.mutations[object] = true
	}
	clear(context.aliases)
	for object, target := range aliases {
		context.aliases[object] = target
	}
}

func (runner *ps6102Runner) recordAssignment(statement *ast.AssignStmt, context ps6102Context) {
	for index, target := range statement.Lhs {
		if identifier, ok := ps2110Unparen(target).(*ast.Ident); ok {
			object := identObject(runner.pass, identifier)
			if object == nil || identifier.Name == "_" {
				continue
			}
			if statement.Tok != token.DEFINE {
				context.mutations[object] = true
			}
			delete(context.aliases, object)
			if len(statement.Lhs) == len(statement.Rhs) {
				if pointed := runner.pointerTarget(statement.Rhs[index], context, map[types.Object]bool{}); pointed != nil {
					context.aliases[object] = pointed
				}
			}
			continue
		}
		if object := runner.pointerTarget(target, context, map[types.Object]bool{}); object != nil {
			context.mutations[object] = true
		}
	}
}

func (runner *ps6102Runner) recordValueSpec(specification *ast.ValueSpec, context ps6102Context) {
	if len(specification.Names) != len(specification.Values) {
		return
	}
	for index, name := range specification.Names {
		object := runner.pass.TypesInfo.Defs[name]
		if object == nil {
			continue
		}
		if pointed := runner.pointerTarget(specification.Values[index], context, map[types.Object]bool{}); pointed != nil {
			context.aliases[object] = pointed
		}
	}
}

// pointerTarget resolves the bounded pointer identities needed for Short alias
// invalidation: &local, immutable local aliases, and forwarded helper
// parameters. It deliberately does not attempt heap, field, or interface
// points-to analysis.
func (runner *ps6102Runner) pointerTarget(expression ast.Expr, context ps6102Context, seen map[types.Object]bool) types.Object {
	expression = ps2110Unparen(expression)
	if unary, ok := expression.(*ast.UnaryExpr); ok && unary.Op == token.AND {
		identifier, ok := ps2110Unparen(unary.X).(*ast.Ident)
		if !ok {
			return nil
		}
		return runner.pass.TypesInfo.ObjectOf(identifier)
	}
	if star, ok := expression.(*ast.StarExpr); ok {
		return runner.pointerTarget(star.X, context, seen)
	}
	identifier, ok := expression.(*ast.Ident)
	if !ok {
		return nil
	}
	object := runner.pass.TypesInfo.ObjectOf(identifier)
	if object == nil || seen[object] {
		return nil
	}
	if pointed := context.aliases[object]; pointed != nil {
		return pointed
	}
	if context.mutations[object] {
		return nil
	}
	initializer := context.flow.initializers[object]
	if initializer == nil {
		return nil
	}
	seen[object] = true
	return runner.pointerTarget(initializer, context, seen)
}
