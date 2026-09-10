package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/cfg"

	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/lint"
)

// PS6121 implements owner issue #963. It identifies a narrowly proven
// source-monomorphic function parameter that is still called inside a
// configured parallel callback's element loop.
var PS6121 = register(&lint.Check{
	ID: "PS6121", Category: "verify", Slug: "monomorphic-loop-callback-dispatch",
	Level: lint.LevelAggressive, AutoFix: false, NeedsConfig: true,
	Vocab: []string{"fanOutHelpers"},
	Doc: lint.Documentation{
		Title: "a source-monomorphic function value is invoked inside a parallel element loop",
		Text: `A function parameter can remain an indirect machine-code call after it is
captured by a parallel band callback and invoked for every element. PS6121
finds a deliberately narrow source-level measurement opportunity: an
unexported, non-generic package helper; one function-typed parameter; a literal
callback passed directly to a configured fanOutHelpers entry point; and a live
for loop whose initial index and condition derive from callback band
parameters. The function value must be invoked in that loop.

Every package-visible reference to the helper must be a direct call and every
caller must pass the identical concrete, non-generic function at the relevant
argument. Reassignment, aliases, storage, address-taking, opaque forwarding,
dynamic/interface/method values, go/defer, nested or uninvoked closures,
constant-dead paths, immediate loop exits, unconfigured or shadowed fan-out
helpers, ambiguous literal callbacks, nested loop-body control flow, and
unsupported or source-proven non-fallthrough enclosing prefixes stay silent,
as do //perfscan:monomorphic-loop-callback-validated functions.

This source proof does NOT prove that an indirect call survives in machine
code and does not claim a speedup. Inspect the exact instantiated callback
symbol in generated machine code first. If indirect dispatch survives, keep
the original scalar formula and function boundary for the attribution
experiment. Freeze identical whole-operation harnesses and verify finite
result bits, signed zeros, nonfinite classes, views, input immutability, and
that a deliberate one-ULP mutation fails the independent exact oracle.

There is NO automatic fix. Consider a direct specialized helper or one-time
dispatch outside the loop only after three alternating count-seven campaigns
on the declared complete-operation target each reach at least 1.05x with
p<0.05, no reproducible greater-than-3% regression in small or parallel
controls, and no allocation increase. Preserve the generic fallback and
reject the candidate when any numerical or measurement gate fails.`,
		Before: `func activationBackward(values, gradients, output []float64,
	grad func(float64, float64) float64) {
	parallel(len(output), func(start, end int) {
		for i := start; i < end; i++ { output[i] = grad(values[i], gradients[i]) }
	})
}
func geluBackward(x, g, out []float64) { activationBackward(x, g, out, geluGrad) }`,
		After: `// Candidate only after machine-code, numerical, and complete-operation gates:
// retain the scalar formula/function boundary and route the proven concrete
// function to a direct specialized helper outside the element loop.`,
		MeasuredWin: `The owner Go 1.27.1 darwin/arm64 experiment confirmed that
the indirect call disappeared, but rejected the runtime candidate. Three
complete 262144-element CPU GELU-backward campaigns at GOMAXPROCS=1 measured
only 1.0114x (p=0.382867), 1.0049x (p=0.710373), and 1.0400x (p=0.620047).
Every campaign failed both the predeclared 1.05x and p<0.05 gates. The result
proves neither a universal neutral effect nor a compiler defect, and authorizes
no rewrite or performance-win claim; scalar transcendental work dominated the
confirmed dispatch site.`,
	},
	Analyzer: &analysis.Analyzer{Name: "PS6121", Doc: "source-monomorphic function parameter called in a parallel element loop", Run: runPS6121},
})

type ps6121Site struct {
	parameter *types.Var
	call      *ast.CallExpr
	helper    string
	index     int
}

func runPS6121(pass *analysis.Pass) (any, error) {
	return runPS6121WithFanout(pass, config.Current().FanOutHelpers)
}

func runPS6121WithFanout(pass *analysis.Pass, fanout map[string]bool) (any, error) {
	if len(fanout) == 0 {
		return nil, nil
	}
	decls := map[*types.Func]*ast.FuncDecl{}
	executions := map[*ast.BlockStmt]*ps6121Execution{}
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			if fn, ok := declaration.(*ast.FuncDecl); ok && fn.Body != nil {
				if object, ok := pass.TypesInfo.Defs[fn.Name].(*types.Func); ok {
					decls[object] = fn
				}
			}
		}
	}
	for object, declaration := range decls {
		signature, _ := object.Type().(*types.Signature)
		if object.Exported() || declaration.Recv != nil || signature == nil || signature.TypeParams().Len() != 0 || ps6121Validated(declaration) {
			continue
		}
		for i := 0; i < signature.Params().Len(); i++ {
			parameter := signature.Params().At(i)
			if !ps6121FunctionType(parameter.Type()) {
				continue
			}
			site, ok := ps6121LoopSite(pass, declaration, parameter, i, fanout, executions)
			if !ok || !ps6121MonomorphicCallers(pass, object, i, decls) || !ps6121ParameterUses(pass, declaration, parameter, site.call) {
				continue
			}
			pass.Reportf(site.call.Pos(), "%s captures source-monomorphic function parameter %s and invokes it in a callback-band-derived element loop; inspect the exact callback machine code before assuming indirect dispatch survives, then preserve the scalar boundary and apply finite-bit, signed-zero, nonfinite, view, immutability, mutation-oracle, allocation, and three complete-operation >=1.05x/p<0.05 gates before considering specialization (PS6121 advisory, no automatic fix; the owner candidate was rejected)", site.helper, parameter.Name())
		}
	}
	return nil, nil
}

func ps6121LoopSite(pass *analysis.Pass, fn *ast.FuncDecl, parameter *types.Var, index int, fanout map[string]bool, executions map[*ast.BlockStmt]*ps6121Execution) (ps6121Site, bool) {
	var found []ps6121Site
	functionExecution := ps6121ExecutionFor(pass, fn.Body, executions)
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		if literal, ok := node.(*ast.FuncLit); ok {
			_ = literal
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		helper, ok := ps6076FanoutHelper(pass, call, fanout)
		if !ok || !functionExecution.Reachable(call) || ps6121AsyncCall(call, functionExecution.parents) {
			return true
		}
		var callbacks []*ast.FuncLit
		for _, arg := range call.Args {
			if lit, ok := ps2110Unparen(arg).(*ast.FuncLit); ok {
				callbacks = append(callbacks, lit)
			}
		}
		if len(callbacks) != 1 || !ps6121OnlyCallbackArgument(pass, call, callbacks[0]) {
			return true
		}
		callback := callbacks[0]
		if ps6121ExtraFunctionCapture(pass, callback, parameter) {
			return true
		}
		callbackExecution := ps6121ExecutionFor(pass, callback.Body, executions)
		parents := callbackExecution.parents
		ast.Inspect(callback.Body, func(candidate ast.Node) bool {
			if literal, ok := candidate.(*ast.FuncLit); ok && literal != callback {
				return false
			}
			loop, ok := candidate.(*ast.ForStmt)
			if !ok {
				return true
			}
			if loop.Init == nil || !callbackExecution.Reachable(loop.Init) || !ps6121BandLoop(pass, callback, loop) || ps6121DeadOrTrivial(pass, loop) {
				return true
			}
			var calls []*ast.CallExpr
			ast.Inspect(loop.Body, func(inner ast.Node) bool {
				if lit, ok := inner.(*ast.FuncLit); ok {
					_ = lit
					return false
				}
				if invocation, ok := inner.(*ast.CallExpr); ok {
					if id, ok := ps2110Unparen(invocation.Fun).(*ast.Ident); ok && pass.TypesInfo.ObjectOf(id) == parameter && callbackExecution.Reachable(invocation) {
						calls = append(calls, invocation)
					}
				}
				return true
			})
			if len(calls) == 1 && !callbackExecution.CycleThrough(calls[0], loop.Post) {
				calls = nil
			}
			if len(calls) == 1 {
				if parent := parents[calls[0]]; parent != nil {
					switch parent.(type) {
					case *ast.GoStmt, *ast.DeferStmt:
						calls = nil
					}
				}
			}
			if len(calls) == 1 {
				found = append(found, ps6121Site{parameter: parameter, call: calls[0], helper: helper, index: index})
			}
			return true
		})
		return true
	})
	if len(found) != 1 {
		return ps6121Site{}, false
	}
	return found[0], true
}

type ps6121Execution struct {
	pass      *analysis.Pass
	body      *ast.BlockStmt
	graph     *cfg.CFG
	parents   map[ast.Node]ast.Node
	blocks    map[ast.Node]*cfg.Block
	reachable map[*cfg.Block]bool
	completes map[*cfg.Block]bool
}

func ps6121ExecutionFor(pass *analysis.Pass, body *ast.BlockStmt, cache map[*ast.BlockStmt]*ps6121Execution) *ps6121Execution {
	if execution := cache[body]; execution != nil {
		return execution
	}
	execution := &ps6121Execution{pass: pass, body: body, graph: ps6090ControlFlow(pass, body), parents: ps6071Parents(body), blocks: map[ast.Node]*cfg.Block{}, reachable: map[*cfg.Block]bool{}, completes: map[*cfg.Block]bool{}}
	for _, block := range execution.graph.Blocks {
		execution.completes[block] = true
		for _, root := range block.Nodes {
			if ps6121ExecutionNodeBlocks(pass, root, execution.parents) {
				execution.completes[block] = false
			}
			ast.Inspect(root, func(node ast.Node) bool {
				if node == nil {
					return false
				}
				execution.blocks[node] = block
				_, nested := node.(*ast.FuncLit)
				return !nested
			})
		}
	}
	if len(execution.graph.Blocks) != 0 {
		pending := []*cfg.Block{execution.graph.Blocks[0]}
		for len(pending) != 0 {
			block := pending[len(pending)-1]
			pending = pending[:len(pending)-1]
			if block == nil || execution.reachable[block] {
				continue
			}
			execution.reachable[block] = true
			pending = append(pending, execution.successors(block)...)
		}
	}
	cache[body] = execution
	return execution
}

func ps6121ExecutionNodeBlocks(pass *analysis.Pass, node ast.Node, parents map[ast.Node]ast.Node) bool {
	original := node
	for node != nil {
		if clause, ok := node.(*ast.CommClause); ok {
			if clause.Comm == original {
				return ps6121SelectOperandsBlock(pass, clause.Comm)
			}
			if expression, ok := original.(*ast.UnaryExpr); ok && expression.Op == token.ARROW && parents[expression] == clause.Comm {
				return ps6121ExpressionBlocks(pass, expression.X)
			}
			break
		}
		node = parents[node]
	}
	return ps6121NodeBlocks(pass, original)
}

func ps6121SelectOperandsBlock(pass *analysis.Pass, comm ast.Stmt) bool {
	switch value := comm.(type) {
	case *ast.SendStmt:
		return ps6121ExpressionBlocks(pass, value.Chan) || ps6121ExpressionBlocks(pass, value.Value)
	case *ast.ExprStmt:
		if receive, ok := ps2110Unparen(value.X).(*ast.UnaryExpr); ok && receive.Op == token.ARROW {
			return ps6121ExpressionBlocks(pass, receive.X)
		}
	case *ast.AssignStmt:
		for _, expression := range value.Rhs {
			if receive, ok := ps2110Unparen(expression).(*ast.UnaryExpr); ok && receive.Op == token.ARROW {
				if ps6121ExpressionBlocks(pass, receive.X) {
					return true
				}
			} else if ps6121ExpressionBlocks(pass, expression) {
				return true
			}
		}
	}
	return false
}

func (execution *ps6121Execution) successors(block *cfg.Block) []*cfg.Block {
	if block != nil && !execution.completes[block] {
		return nil
	}
	if block == nil || len(block.Succs) != 2 || len(block.Nodes) == 0 {
		return block.Succs
	}
	condition, ok := block.Nodes[len(block.Nodes)-1].(ast.Expr)
	if !ok {
		return block.Succs
	}
	truth, known := ps6121ControlCondition(execution.pass, condition, execution.parents)
	if !known {
		return block.Succs
	}
	if truth {
		return block.Succs[:1]
	}
	return block.Succs[1:]
}

func ps6121ControlCondition(pass *analysis.Pass, expression ast.Expr, parents map[ast.Node]ast.Node) (bool, bool) {
	if clause, ok := parents[expression].(*ast.CaseClause); ok {
		block := parents[clause]
		statement, _ := parents[block].(*ast.SwitchStmt)
		if statement == nil {
			return false, false
		}
		tag, tagKnown := ps6121ExpressionConstant(pass, statement.Tag)
		candidate, candidateKnown := ps6121ExpressionConstant(pass, expression)
		if !tagKnown || !candidateKnown {
			return false, false
		}
		return constant.Compare(tag, token.EQL, candidate), true
	}
	return ps6121BoolConstant(pass, expression)
}

func (execution *ps6121Execution) Reachable(node ast.Node) bool {
	block := execution.blocks[node]
	return block != nil && execution.reachable[block] && execution.blockPrefixCompletes(block, node) && ps6121SourceFeasible(execution.pass, execution.body, node, execution.parents)
}

func (execution *ps6121Execution) CycleThrough(from, through ast.Node) bool {
	start, target := execution.blocks[from], execution.blocks[through]
	return start != nil && target != nil && execution.Reachable(from) && execution.Reachable(through) &&
		execution.blockSuffixCompletes(start, from) && execution.path(start, target) && execution.path(target, start)
}

func (execution *ps6121Execution) blockPrefixCompletes(block *cfg.Block, target ast.Node) bool {
	for _, node := range block.Nodes {
		if node.End() <= target.Pos() && ps6121ExecutionNodeBlocks(execution.pass, node, execution.parents) {
			return false
		}
	}
	return true
}

func (execution *ps6121Execution) blockSuffixCompletes(block *cfg.Block, start ast.Node) bool {
	for _, node := range block.Nodes {
		if node.Pos() >= start.End() && ps6121ExecutionNodeBlocks(execution.pass, node, execution.parents) {
			return false
		}
	}
	return true
}

func (execution *ps6121Execution) path(start, target *cfg.Block) bool {
	seen := map[*cfg.Block]bool{}
	pending := []*cfg.Block{start}
	for len(pending) != 0 {
		block := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if block == target {
			return true
		}
		if block == nil || seen[block] {
			continue
		}
		seen[block] = true
		pending = append(pending, execution.successors(block)...)
	}
	return false
}

func ps6121SourceFeasible(pass *analysis.Pass, root *ast.BlockStmt, target ast.Node, parents map[ast.Node]ast.Node) bool {
	for node := target; node != nil && node != root; {
		parent := parents[node]
		switch enclosing := parent.(type) {
		case *ast.IfStmt:
			if truth, known := ps6121BoolConstant(pass, enclosing.Cond); known {
				if (node == enclosing.Body && !truth) || (node == enclosing.Else && truth) {
					return false
				}
			}
		case *ast.ForStmt:
			if node == enclosing.Body && ps6121ForCardinality(pass, enclosing) == ps6121Zero {
				return false
			}
		case *ast.RangeStmt:
			if node == enclosing.Body && ps6121RangeCardinality(pass, enclosing) == ps6121Zero {
				return false
			}
		case *ast.CaseClause:
			if selected, known := ps6121SelectedSwitchClause(pass, enclosing, parents); known && !selected {
				return false
			}
		case *ast.CommClause:
			if enclosing.Comm != nil && ps6121NilChannelComm(pass, enclosing.Comm) {
				return false
			}
		}
		block, ok := parent.(*ast.BlockStmt)
		if !ok {
			node = parent
			continue
		}
		statement, ok := node.(ast.Stmt)
		if !ok {
			return false
		}
		for index, sibling := range block.List {
			if sibling == statement {
				break
			}
			if branch, ok := sibling.(*ast.BranchStmt); ok && branch.Tok == token.GOTO && ps6121GotoTargets(pass, branch, target, block.List[index+1:]) {
				break
			}
			if ps6121NilChannelComm(pass, sibling) {
				return false
			}
			if !ps6121FallsThrough(pass, sibling) {
				return false
			}
		}
		node = block
	}
	return true
}

func ps6121GotoTargets(pass *analysis.Pass, branch *ast.BranchStmt, target ast.Node, following []ast.Stmt) bool {
	if branch.Label == nil {
		return false
	}
	label := pass.TypesInfo.ObjectOf(branch.Label)
	for _, statement := range following {
		labeled, ok := statement.(*ast.LabeledStmt)
		if ok && pass.TypesInfo.ObjectOf(labeled.Label) == label && labeled.Pos() <= target.Pos() && target.End() <= labeled.End() {
			return true
		}
	}
	return false
}

type ps6121Cardinality uint8

const (
	ps6121Unknown ps6121Cardinality = iota
	ps6121Zero
	ps6121Finite
	ps6121Blocking
)

func ps6121ForCardinality(pass *analysis.Pass, loop *ast.ForStmt) ps6121Cardinality {
	if loop.Cond == nil {
		return ps6121Unknown
	}
	if truth, known := ps6121BoolConstant(pass, loop.Cond); known {
		if !truth {
			return ps6121Zero
		}
		return ps6121Unknown
	}
	init, initOK := loop.Init.(*ast.AssignStmt)
	condition, conditionOK := ps2110Unparen(loop.Cond).(*ast.BinaryExpr)
	if !initOK || !conditionOK || len(init.Lhs) != 1 || len(init.Rhs) != 1 {
		return ps6121Unknown
	}
	left, leftOK := ps2110Unparen(condition.X).(*ast.Ident)
	index, indexOK := ps2110Unparen(init.Lhs[0]).(*ast.Ident)
	start, end := pass.TypesInfo.Types[init.Rhs[0]].Value, pass.TypesInfo.Types[condition.Y].Value
	if !leftOK || !indexOK || pass.TypesInfo.ObjectOf(left) != pass.TypesInfo.ObjectOf(index) || start == nil || end == nil || start.Kind() != constant.Int || end.Kind() != constant.Int {
		return ps6121Unknown
	}
	if !constant.Compare(start, condition.Op, end) {
		return ps6121Zero
	}
	return ps6121Finite
}

func ps6121RangeCardinality(pass *analysis.Pass, loop *ast.RangeStmt) ps6121Cardinality {
	value := ps2110Unparen(loop.X)
	if ps6121ProvablyNilChannel(pass, value) {
		return ps6121Blocking
	}
	typeValue := types.Unalias(pass.TypesInfo.TypeOf(value))
	if pointer, ok := typeValue.(*types.Pointer); ok {
		typeValue = types.Unalias(pointer.Elem())
	}
	if array, ok := typeValue.Underlying().(*types.Array); ok {
		if array.Len() == 0 {
			return ps6121Zero
		}
		return ps6121Finite
	}
	if constantValue := pass.TypesInfo.Types[value].Value; constantValue != nil {
		switch constantValue.Kind() {
		case constant.Int:
			if constant.Sign(constantValue) <= 0 {
				return ps6121Zero
			}
			return ps6121Finite
		case constant.String:
			if constant.StringVal(constantValue) == "" {
				return ps6121Zero
			}
			return ps6121Finite
		}
	}
	if literal, ok := value.(*ast.CompositeLit); ok {
		switch typeValue.Underlying().(type) {
		case *types.Slice, *types.Map:
			if len(literal.Elts) == 0 {
				return ps6121Zero
			}
			return ps6121Finite
		}
	}
	return ps6121Unknown
}

func ps6121LoopBodyFallsThrough(pass *analysis.Pass, body *ast.BlockStmt, label string) bool {
	if ps6121Breaks(pass, body, label) {
		return true
	}
	return ps6121StatementsFallThrough(pass, body.List)
}

func ps6121ProvablyNilChannel(pass *analysis.Pass, expression ast.Expr) bool {
	typeValue := pass.TypesInfo.TypeOf(expression)
	if typeValue == nil {
		return false
	}
	if _, ok := types.Unalias(typeValue).Underlying().(*types.Chan); !ok {
		return false
	}
	call, ok := ps2110Unparen(expression).(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return false
	}
	id, ok := ps2110Unparen(call.Args[0]).(*ast.Ident)
	return ok && id.Name == "nil"
}

func ps6121NilChannelComm(pass *analysis.Pass, statement ast.Stmt) bool {
	switch value := statement.(type) {
	case *ast.SendStmt:
		return ps6121ProvablyNilChannel(pass, value.Chan)
	case *ast.ExprStmt:
		unary, ok := ps2110Unparen(value.X).(*ast.UnaryExpr)
		return ok && unary.Op == token.ARROW && ps6121ProvablyNilChannel(pass, unary.X)
	case *ast.AssignStmt:
		if len(value.Rhs) != 1 {
			return false
		}
		unary, ok := ps2110Unparen(value.Rhs[0]).(*ast.UnaryExpr)
		return ok && unary.Op == token.ARROW && ps6121ProvablyNilChannel(pass, unary.X)
	}
	return false
}

func ps6121NodeBlocks(pass *analysis.Pass, node ast.Node) bool {
	blocked := false
	ast.Inspect(node, func(child ast.Node) bool {
		if child == nil || blocked {
			return false
		}
		if _, ok := child.(*ast.FuncLit); ok {
			return false
		}
		// Select evaluates channels but a nil case does not block when another
		// case (notably default) can proceed; select flow is modeled separately.
		if _, ok := child.(*ast.SelectStmt); ok {
			return false
		}
		if send, ok := child.(*ast.SendStmt); ok && ps6121ProvablyNilChannel(pass, send.Chan) {
			blocked = true
			return false
		}
		if expression, ok := child.(ast.Expr); ok {
			blocked = ps6121ExpressionBlocks(pass, expression)
			return false
		}
		return true
	})
	return blocked
}

func ps6121ExpressionBlocks(pass *analysis.Pass, expression ast.Expr) bool {
	expression = ps2110Unparen(expression)
	switch value := expression.(type) {
	case *ast.FuncLit:
		return false
	case *ast.UnaryExpr:
		return value.Op == token.ARROW && ps6121ProvablyNilChannel(pass, value.X) || ps6121ExpressionBlocks(pass, value.X)
	case *ast.BinaryExpr:
		if ps6121ExpressionBlocks(pass, value.X) {
			return true
		}
		if value.Op == token.LAND || value.Op == token.LOR {
			if truth, known := ps6121BoolConstant(pass, value.X); known && (value.Op == token.LAND && !truth || value.Op == token.LOR && truth) {
				return false
			}
		}
		return ps6121ExpressionBlocks(pass, value.Y)
	}
	blocked := false
	ast.Inspect(expression, func(child ast.Node) bool {
		if child == nil || child == expression || blocked {
			return child != nil && !blocked
		}
		if _, ok := child.(*ast.FuncLit); ok {
			return false
		}
		if nested, ok := child.(ast.Expr); ok {
			blocked = ps6121ExpressionBlocks(pass, nested)
			return false
		}
		return true
	})
	return blocked
}

func ps6121SelectedSwitchClause(pass *analysis.Pass, clause *ast.CaseClause, parents map[ast.Node]ast.Node) (bool, bool) {
	var statement *ast.SwitchStmt
	for node := ast.Node(clause); node != nil; node = parents[node] {
		if candidate, ok := node.(*ast.SwitchStmt); ok {
			statement = candidate
			break
		}
	}
	if statement == nil {
		return false, false
	}
	tag, known := ps6121ExpressionConstant(pass, statement.Tag)
	if !known {
		return false, false
	}
	var selected, fallback *ast.CaseClause
	for _, item := range statement.Body.List {
		candidate := item.(*ast.CaseClause)
		if len(candidate.List) == 0 {
			fallback = candidate
			continue
		}
		for _, expression := range candidate.List {
			caseValue, caseKnown := ps6121ExpressionConstant(pass, expression)
			if !caseKnown {
				return false, false
			}
			if constant.Compare(tag, token.EQL, caseValue) {
				selected = candidate
				break
			}
		}
		if selected != nil {
			break
		}
	}
	if selected == nil {
		selected = fallback
	}
	return selected == clause, true
}

func ps6121ExpressionConstant(pass *analysis.Pass, expression ast.Expr) (constant.Value, bool) {
	if expression == nil {
		return constant.MakeBool(true), true
	}
	if value := pass.TypesInfo.Types[expression].Value; value != nil {
		return value, true
	}
	if truth, known := ps6121BoolConstant(pass, expression); known {
		return constant.MakeBool(truth), true
	}
	return nil, false
}

func ps6121FallsThrough(pass *analysis.Pass, statement ast.Stmt) bool {
	switch value := statement.(type) {
	case *ast.BlockStmt:
		for _, child := range value.List {
			if !ps6121FallsThrough(pass, child) {
				return false
			}
		}
		return true
	case *ast.IfStmt:
		if truth, known := ps6121BoolConstant(pass, value.Cond); known {
			if truth {
				return ps6121FallsThrough(pass, value.Body)
			}
			if value.Else == nil {
				return true
			}
			return ps6121FallsThrough(pass, value.Else)
		}
		if ps6121FallsThrough(pass, value.Body) {
			return true
		}
		return value.Else == nil || ps6121FallsThrough(pass, value.Else)
	case *ast.ForStmt:
		cardinality := ps6121ForCardinality(pass, value)
		if cardinality == ps6121Zero {
			return true
		}
		if cardinality == ps6121Finite {
			return ps6121LoopBodyFallsThrough(pass, value.Body, "")
		}
		if value.Cond != nil {
			if truth, known := ps6121BoolConstant(pass, value.Cond); known {
				return !truth || ps6121Breaks(pass, value.Body, "")
			}
			return true
		}
		return ps6121Breaks(pass, value.Body, "")
	case *ast.RangeStmt:
		switch ps6121RangeCardinality(pass, value) {
		case ps6121Zero:
			return true
		case ps6121Finite:
			return ps6121LoopBodyFallsThrough(pass, value.Body, "")
		case ps6121Blocking:
			return false
		default:
			return true
		}
	case *ast.LabeledStmt:
		if loop, ok := value.Stmt.(*ast.ForStmt); ok {
			if loop.Cond == nil {
				return ps6121Breaks(pass, loop.Body, value.Label.Name)
			}
			if truth, known := ps6121BoolConstant(pass, loop.Cond); known && truth {
				return ps6121Breaks(pass, loop.Body, value.Label.Name)
			}
		}
		return ps6121FallsThrough(pass, value.Stmt)
	case *ast.SwitchStmt:
		return ps6121SwitchFallsThrough(pass, value)
	case *ast.SelectStmt:
		if len(value.Body.List) == 0 {
			return false
		}
		for _, statement := range value.Body.List {
			clause := statement.(*ast.CommClause)
			if clause.Comm != nil && ps6121NilChannelComm(pass, clause.Comm) {
				continue
			}
			if ps6121StatementsFallThrough(pass, clause.Body) {
				return true
			}
		}
		return false
	case *ast.ReturnStmt, *ast.BranchStmt, *ast.TypeSwitchStmt:
		return false
	case *ast.ExprStmt:
		call, ok := ps2110Unparen(value.X).(*ast.CallExpr)
		return !ok || !ps6088NonreturnCall(pass, call)
	}
	return true
}

func ps6121BoolConstant(pass *analysis.Pass, expression ast.Expr) (bool, bool) {
	if truth, known := ps2144BoolConstant(pass, expression); known {
		return truth, true
	}
	switch value := ps2110Unparen(expression).(type) {
	case *ast.UnaryExpr:
		if value.Op == token.NOT {
			if truth, known := ps6121BoolConstant(pass, value.X); known {
				return !truth, true
			}
		}
	case *ast.BinaryExpr:
		if value.Op == token.EQL || value.Op == token.NEQ {
			left, leftKnown := ps6121BoolConstant(pass, value.X)
			right, rightKnown := ps6121BoolConstant(pass, value.Y)
			if leftKnown && rightKnown {
				if value.Op == token.EQL {
					return left == right, true
				}
				return left != right, true
			}
		}
	}
	binary, ok := ps2110Unparen(expression).(*ast.BinaryExpr)
	if !ok || (binary.Op != token.LOR && binary.Op != token.LAND) {
		return false, false
	}
	left, leftKnown := ps6121BoolConstant(pass, binary.X)
	if leftKnown && ((binary.Op == token.LOR && left) || (binary.Op == token.LAND && !left)) {
		return left, true
	}
	right, rightKnown := ps6121BoolConstant(pass, binary.Y)
	if rightKnown && ((binary.Op == token.LOR && right) || (binary.Op == token.LAND && !right)) {
		return right, true
	}
	if leftKnown && rightKnown {
		if binary.Op == token.LOR {
			return left || right, true
		}
		return left && right, true
	}
	return false, false
}

func ps6121Breaks(pass *analysis.Pass, block *ast.BlockStmt, label string) bool {
	for _, statement := range block.List {
		switch value := statement.(type) {
		case *ast.BranchStmt:
			if value.Tok == token.BREAK && ((label == "" && value.Label == nil) || (value.Label != nil && value.Label.Name == label)) {
				return true
			}
		case *ast.IfStmt:
			if truth, known := ps6121BoolConstant(pass, value.Cond); known {
				if truth && ps6121Breaks(pass, value.Body, label) {
					return true
				}
				if !truth && value.Else != nil && ps6121StatementBreaks(pass, value.Else, label) {
					return true
				}
			} else if ps6121Breaks(pass, value.Body, label) || (value.Else != nil && ps6121StatementBreaks(pass, value.Else, label)) {
				return true
			}
		case *ast.BlockStmt:
			if ps6121Breaks(pass, value, label) {
				return true
			}
		case *ast.ForStmt:
			if label != "" && ps6121ForCardinality(pass, value) != ps6121Zero && ps6121Breaks(pass, value.Body, label) {
				return true
			}
		}
		if !ps6121FallsThrough(pass, statement) {
			return false
		}
	}
	return false
}

func ps6121StatementBreaks(pass *analysis.Pass, statement ast.Stmt, label string) bool {
	if block, ok := statement.(*ast.BlockStmt); ok {
		return ps6121Breaks(pass, block, label)
	}
	if nested, ok := statement.(*ast.IfStmt); ok {
		return ps6121Breaks(pass, &ast.BlockStmt{List: []ast.Stmt{nested}}, label)
	}
	return false
}

func ps6121StatementsFallThrough(pass *analysis.Pass, statements []ast.Stmt) bool {
	for _, statement := range statements {
		if branch, ok := statement.(*ast.BranchStmt); ok && branch.Tok == token.BREAK && branch.Label == nil {
			return true
		}
		if !ps6121FallsThrough(pass, statement) {
			return false
		}
	}
	return true
}

func ps6121SwitchFallsThrough(pass *analysis.Pass, statement *ast.SwitchStmt) bool {
	defaultIndex := -1
	clauses := make([]*ast.CaseClause, len(statement.Body.List))
	for index, item := range statement.Body.List {
		clauses[index] = item.(*ast.CaseClause)
		if len(clauses[index].List) == 0 {
			defaultIndex = index
		}
	}
	tag, tagKnown := ps6121ExpressionConstant(pass, statement.Tag)
	possible := make([]int, 0, len(clauses))
	if !tagKnown {
		for index, clause := range clauses {
			if len(clause.List) != 0 {
				possible = append(possible, index)
			}
		}
		if defaultIndex >= 0 {
			possible = append(possible, defaultIndex)
		} else {
			return true
		}
	} else {
		definite := -1
		for index, clause := range clauses {
			if len(clause.List) == 0 {
				continue
			}
			unknown := false
			for _, expression := range clause.List {
				candidate, candidateKnown := ps6121ExpressionConstant(pass, expression)
				if !candidateKnown {
					unknown = true
					continue
				}
				if constant.Compare(tag, token.EQL, candidate) {
					definite = index
					break
				}
			}
			if definite >= 0 {
				possible = append(possible, definite)
				break
			}
			if unknown {
				possible = append(possible, index)
			}
		}
		if definite < 0 {
			if defaultIndex >= 0 {
				possible = append(possible, defaultIndex)
			} else {
				return true
			}
		}
	}
	for _, selected := range possible {
		if ps6121SwitchPathFallsThrough(pass, clauses, selected) {
			return true
		}
	}
	return false
}

func ps6121SwitchPathFallsThrough(pass *analysis.Pass, clauses []*ast.CaseClause, selected int) bool {
	for index := selected; index < len(clauses); index++ {
		body := clauses[index].Body
		fallsNext := false
		if len(body) != 0 {
			if branch, ok := body[len(body)-1].(*ast.BranchStmt); ok && branch.Tok == token.FALLTHROUGH {
				fallsNext = true
				body = body[:len(body)-1]
			}
		}
		if !ps6121StatementsFallThrough(pass, body) {
			return false
		}
		if !fallsNext {
			return true
		}
	}
	return true
}

func ps6121OnlyCallbackArgument(pass *analysis.Pass, call *ast.CallExpr, callback *ast.FuncLit) bool {
	_, signature, ok := typedCallee(pass, call.Fun)
	if !ok || signature == nil {
		return false
	}
	seen := 0
	for argumentIndex, argument := range call.Args {
		parameterIndex := argumentIndex
		if signature.Variadic() && parameterIndex >= signature.Params().Len()-1 {
			parameterIndex = signature.Params().Len() - 1
		}
		if parameterIndex < 0 || parameterIndex >= signature.Params().Len() {
			return false
		}
		parameterType := types.Unalias(signature.Params().At(parameterIndex).Type())
		if signature.Variadic() && parameterIndex == signature.Params().Len()-1 {
			if slice, ok := parameterType.(*types.Slice); ok {
				parameterType = types.Unalias(slice.Elem())
			}
		}
		if ps6121FunctionType(parameterType) {
			seen++
			if ps2110Unparen(argument) != callback {
				return false
			}
		}
	}
	return seen == 1
}

func ps6121AsyncCall(call *ast.CallExpr, parents map[ast.Node]ast.Node) bool {
	switch parents[call].(type) {
	case *ast.GoStmt, *ast.DeferStmt:
		return true
	}
	return false
}

func ps6121BandLoop(pass *analysis.Pass, callback *ast.FuncLit, loop *ast.ForStmt) bool {
	if loop.Init == nil || loop.Cond == nil || loop.Post == nil {
		return false
	}
	band := map[types.Object]bool{}
	if callback.Type.Params != nil {
		for _, field := range callback.Type.Params.List {
			for _, name := range field.Names {
				if object := pass.TypesInfo.ObjectOf(name); object != nil {
					band[object] = true
				}
			}
		}
	}
	init, ok := loop.Init.(*ast.AssignStmt)
	if !ok || len(init.Lhs) != 1 || len(init.Rhs) != 1 {
		return false
	}
	indexID, ok := ps2110Unparen(init.Lhs[0]).(*ast.Ident)
	if !ok {
		return false
	}
	index := pass.TypesInfo.ObjectOf(indexID)
	startID, ok := ps2110Unparen(init.Rhs[0]).(*ast.Ident)
	if index == nil || !ok || !band[pass.TypesInfo.ObjectOf(startID)] {
		return false
	}
	condition, ok := ps2110Unparen(loop.Cond).(*ast.BinaryExpr)
	if !ok || (condition.Op != token.LSS && condition.Op != token.NEQ) {
		return false
	}
	left, leftOK := ps2110Unparen(condition.X).(*ast.Ident)
	right, rightOK := ps2110Unparen(condition.Y).(*ast.Ident)
	if !leftOK || !rightOK || pass.TypesInfo.ObjectOf(left) != index || !band[pass.TypesInfo.ObjectOf(right)] || pass.TypesInfo.ObjectOf(right) == pass.TypesInfo.ObjectOf(startID) {
		return false
	}
	post, ok := loop.Post.(*ast.IncDecStmt)
	postID, postOK := func() (*ast.Ident, bool) {
		if !ok || post.Tok != token.INC {
			return nil, false
		}
		id, yes := ps2110Unparen(post.X).(*ast.Ident)
		return id, yes
	}()
	if !postOK || pass.TypesInfo.ObjectOf(postID) != index {
		return false
	}
	return ps6121StableLoopObjects(pass, callback, loop, index, band)
}

func ps6121StableLoopObjects(pass *analysis.Pass, callback *ast.FuncLit, loop *ast.ForStmt, index types.Object, band map[types.Object]bool) bool {
	stable := true
	writesProtected := func(expression ast.Expr) bool {
		identifier, ok := ps2110Unparen(expression).(*ast.Ident)
		if !ok {
			return false
		}
		object := pass.TypesInfo.ObjectOf(identifier)
		return object == index || band[object]
	}
	ast.Inspect(callback.Body, func(node ast.Node) bool {
		if !stable {
			return false
		}
		switch value := node.(type) {
		case *ast.AssignStmt:
			if value == loop.Init {
				return true
			}
			for _, left := range value.Lhs {
				if writesProtected(left) {
					stable = false
					return false
				}
			}
		case *ast.RangeStmt:
			if (value.Key != nil && writesProtected(value.Key)) || (value.Value != nil && writesProtected(value.Value)) {
				stable = false
				return false
			}
		case *ast.IncDecStmt:
			if value != loop.Post && writesProtected(value.X) {
				stable = false
				return false
			}
		case *ast.UnaryExpr:
			if value.Op == token.AND && writesProtected(value.X) {
				stable = false
				return false
			}
		}
		return true
	})
	return stable
}

func ps6121ExtraFunctionCapture(pass *analysis.Pass, callback *ast.FuncLit, wanted *types.Var) bool {
	callbackObjects := map[types.Object]bool{}
	ast.Inspect(callback.Type, func(node ast.Node) bool {
		if id, ok := node.(*ast.Ident); ok {
			if object := pass.TypesInfo.ObjectOf(id); object != nil {
				callbackObjects[object] = true
			}
		}
		return true
	})
	extra := false
	ast.Inspect(callback.Body, func(node ast.Node) bool {
		if literal, ok := node.(*ast.FuncLit); ok && literal != callback {
			return false
		}
		id, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		object := pass.TypesInfo.ObjectOf(id)
		if object == nil || object == wanted || callbackObjects[object] {
			return true
		}
		if ps6121FunctionType(object.Type()) {
			extra = true
			return false
		}
		return true
	})
	return extra
}

func ps6121FunctionType(value types.Type) bool {
	if value == nil {
		return false
	}
	_, ok := types.Unalias(value).Underlying().(*types.Signature)
	return ok
}

func ps6121DeadOrTrivial(pass *analysis.Pass, loop *ast.ForStmt) bool {
	if len(loop.Body.List) == 0 {
		return true
	}
	if !ps6121StraightLineLoop(loop) {
		return true
	}
	switch loop.Body.List[0].(type) {
	case *ast.BranchStmt, *ast.ReturnStmt:
		return true
	}
	terminal := false
	ast.Inspect(loop.Body, func(node ast.Node) bool {
		if literal, ok := node.(*ast.FuncLit); ok {
			_ = literal
			return false
		}
		switch value := node.(type) {
		case *ast.ReturnStmt:
			terminal = true
		case *ast.BranchStmt:
			if value.Tok == token.BREAK || value.Tok == token.GOTO {
				terminal = true
			}
		case *ast.CallExpr:
			if identifier, ok := ps2110Unparen(value.Fun).(*ast.Ident); ok {
				if builtin, ok := pass.TypesInfo.ObjectOf(identifier).(*types.Builtin); ok && builtin.Name() == "panic" {
					terminal = true
				}
			}
		}
		return !terminal
	})
	return terminal
}

func ps6121StraightLineLoop(loop *ast.ForStmt) bool {
	straight := true
	ast.Inspect(loop.Body, func(node ast.Node) bool {
		if !straight {
			return false
		}
		if literal, ok := node.(*ast.FuncLit); ok {
			_ = literal
			return false
		}
		switch value := node.(type) {
		case *ast.IfStmt, *ast.RangeStmt, *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt:
			straight = false
			return false
		case *ast.ForStmt:
			if value != loop {
				straight = false
				return false
			}
		}
		return true
	})
	return straight
}

func ps6121MonomorphicCallers(pass *analysis.Pass, helper *types.Func, parameterIndex int, declarations map[*types.Func]*ast.FuncDecl) bool {
	var concrete *types.Func
	callers := 0
	valid := true
	for _, file := range pass.Files {
		parents := ps6071Parents(file)
		ast.Inspect(file, func(node ast.Node) bool {
			id, ok := node.(*ast.Ident)
			if !ok || pass.TypesInfo.Uses[id] != helper {
				return true
			}
			parent := parents[id]
			call, ok := parent.(*ast.CallExpr)
			if !ok || ps2110Unparen(call.Fun) != id || parameterIndex >= len(call.Args) {
				valid = false
				return true
			}
			if ps6121AsyncCall(call, parents) {
				valid = false
				return true
			}
			argument := ps2110Unparen(call.Args[parameterIndex])
			argID, ok := argument.(*ast.Ident)
			if !ok {
				valid = false
				return true
			}
			function, ok := pass.TypesInfo.ObjectOf(argID).(*types.Func)
			if !ok || function.Pkg() != pass.Pkg {
				valid = false
				return true
			}
			sig, _ := function.Type().(*types.Signature)
			if sig == nil || sig.Recv() != nil || sig.TypeParams().Len() != 0 || declarations[function] == nil {
				valid = false
				return true
			}
			if concrete == nil {
				concrete = function
			} else if concrete != function {
				valid = false
			}
			callers++
			return true
		})
	}
	return valid && callers > 0 && concrete != nil
}

func ps6121ParameterUses(pass *analysis.Pass, fn *ast.FuncDecl, parameter *types.Var, wanted *ast.CallExpr) bool {
	valid := true
	parents := ps6071Parents(fn.Body)
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		id, ok := node.(*ast.Ident)
		if !ok || pass.TypesInfo.ObjectOf(id) != parameter {
			return true
		}
		parent := parents[id]
		call, ok := parent.(*ast.CallExpr)
		if !ok || call != wanted || ps2110Unparen(call.Fun) != id {
			valid = false
		}
		return true
	})
	return valid
}

func ps6121Validated(fn *ast.FuncDecl) bool {
	if fn.Doc == nil {
		return false
	}
	for _, comment := range fn.Doc.List {
		if strings.Contains(comment.Text, "perfscan:monomorphic-loop-callback-validated") {
			return true
		}
	}
	return false
}
