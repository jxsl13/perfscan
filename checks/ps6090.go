package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/cfg"

	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS6090 implements owner issue #900. It finds configured pure-compute calls
// whose primary result is structurally dead in a real Go benchmark loop. The
// check follows direct local helpers so benchmark bodies do not have to contain
// the vulnerable call themselves.
var PS6090 = register(&lint.Check{
	ID:          "PS6090",
	Category:    "verify",
	Slug:        "benchmark-discards-compute-result",
	Level:       lint.LevelStructured,
	AutoFix:     false,
	NeedsConfig: true,
	Vocab:       []string{"pureComputeFuncs"},
	Doc: lint.Documentation{
		Title: "a benchmark loop discards the primary result of a pure compute call",
		Text: `A benchmark can call a substantial compute API, check its error, and
discard the returned tensor, buffer, digest, or scalar. Current compiler and
callee complexity may keep the work in place, but the benchmark contract then
depends on incidental effects rather than an observable result. Refactoring or
a future optimizer can remove some or all of the work without making the
benchmark fail.

PS6090 reports this shape only for functions or methods explicitly listed in
pureComputeFuncs. Entries may be a legacy short function name or, preferably,
an exact typed identity such as example.com/math.QMatMul or
example.com/backend.Engine.Execute. This opt-in is the purity proof: ordinary
APIs such as io.Reader.Read and database calls are not guessed from names.

The check starts only at a Go-registered top-level benchmark in a _test.go file:
Benchmark itself or BenchmarkX where the first suffix rune is not lowercase,
with the exact func(*testing.B) shape. It recognizes only exact testing.B.N,
testing.B.Loop, and testing.PB.Next repetition, and follows direct typed calls
into package-local functions and methods. This covers a benchmark loop that
calls a helper and a benchmark that calls a helper which owns its loop. Direct
b.Run and b.RunParallel closures and immediately invoked closures are included.
The helper call graph does not speculate through opaque function values or
interface dispatch, and unrelated setup loops are deliberately excluded; an
explicitly configured interface method call itself can still be audited at its
source site. Per-body control-flow reachability excludes calls after return,
branch statements, and known no-return calls, while constant-condition pruning
removes statically dead if, short-circuit, loop, and switch paths.

A finding requires the configured call to return at least one non-error value
and its first non-error result to be explicitly blank, discarded as an
expression statement, or bound to a fresh local whose only reads assign it to
the blank identifier. A sink assignment, return, assertion, comparison, or
other semantic use keeps the check silent.

There is NO automatic fix. A package-level typed sink is appropriate for many
serial benchmarks, while parallel benchmarks need a race-free per-worker sink
or a semantic oracle. Add the observation without changing the timed operation
and keep validation outside the timed region where practical. APIs intentionally
benchmarked only for documented effects can use
//perfscan:ignore PS6090 <reason>; do not list an effectful API as pure compute
merely to obtain broad matching.`,
		Before: `func BenchmarkQMatMul(b *testing.B) {
	for b.Loop() {
		if _, err := QMatMul(a, w); err != nil {
			b.Fatal(err)
		}
	}
}`,
		After: `var benchmarkQMatMulSink *Tensor

func BenchmarkQMatMul(b *testing.B) {
	for b.Loop() {
		out, err := QMatMul(a, w)
		if err != nil {
			b.Fatal(err)
		}
		benchmarkQMatMulSink = out
	}
}`,
		MeasuredWin: `The GoAI validation behind owner issue #900 measured seven
fresh-process, order-alternated 10,000-iteration pairs on Apple M2 Pro. The
discarded-result control had a 7,628 ns/op median and the typed-sink benchmark
had a 7,634 ns/op median (candidate/control 1.00079), with 592 B/op and 4
allocs/op on both sides. The sink is intentionally performance-neutral
benchmark hardening.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6090",
		Doc:  "benchmark loop discards the primary result of a configured pure compute call",
		Run:  runPS6090,
	},
})

type ps6090NamedWork struct {
	function *ast.FuncDecl
	repeated bool
}

type ps6090LiteralWork struct {
	literal  *ast.FuncLit
	repeated bool
}

type ps6090NamedKey struct {
	function *ast.FuncDecl
	repeated bool
}

type ps6090LiteralKey struct {
	literal  *ast.FuncLit
	repeated bool
}

func runPS6090(pass *analysis.Pass) (any, error) {
	return runPS6090WithCompute(pass, config.Current().PureComputeFuncs)
}

func runPS6090WithCompute(pass *analysis.Pass, configured map[string]bool) (any, error) {
	if len(configured) == 0 {
		return nil, nil
	}

	declarations := make(map[*types.Func]*ast.FuncDecl)
	var named []ps6090NamedWork
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
			declarations[object.Origin()] = function
			if ps6090RunnableBenchmark(pass, file, function) {
				named = append(named, ps6090NamedWork{function: function})
			}
		}
	}

	reported := make(map[*ast.CallExpr]bool)
	seenNamed := make(map[ps6090NamedKey]bool)
	seenLiterals := make(map[ps6090LiteralKey]bool)
	var literals []ps6090LiteralWork
	for len(named) > 0 || len(literals) > 0 {
		if len(named) > 0 {
			work := named[0]
			named = named[1:]
			key := ps6090NamedKey(work)
			if seenNamed[key] {
				continue
			}
			seenNamed[key] = true
			ps6090WalkBody(pass, work.function.Body, work.repeated, configured, declarations, reported, &named, &literals)
			continue
		}

		work := literals[0]
		literals = literals[1:]
		key := ps6090LiteralKey(work)
		if seenLiterals[key] {
			continue
		}
		seenLiterals[key] = true
		ps6090WalkBody(pass, work.literal.Body, work.repeated, configured, declarations, reported, &named, &literals)
	}
	return nil, nil
}

func ps6090RunnableBenchmark(pass *analysis.Pass, file *ast.File, function *ast.FuncDecl) bool {
	if !ps6006Benchmark(pass, function) || pass.Fset == nil || file == nil {
		return false
	}
	if function.Type.TypeParams != nil && function.Type.TypeParams.NumFields() != 0 {
		return false
	}
	filename := pass.Fset.PositionFor(file.Pos(), false).Filename
	if !strings.HasSuffix(filename, "_test.go") {
		return false
	}
	name := function.Name.Name
	if len(name) == len("Benchmark") {
		return true
	}
	r, _ := utf8.DecodeRuneInString(name[len("Benchmark"):])
	return !unicode.IsLower(r)
}

func ps6090WalkBody(
	pass *analysis.Pass,
	body *ast.BlockStmt,
	inheritedRepeated bool,
	configured map[string]bool,
	declarations map[*types.Func]*ast.FuncDecl,
	reported map[*ast.CallExpr]bool,
	named *[]ps6090NamedWork,
	literals *[]ps6090LiteralWork,
) {
	parents := ps6090Parents(body)
	graph := ps6090ControlFlow(pass, body)
	reachable := ps6090ReachableNodes(graph)
	astutil.WithStack(body, func(node ast.Node, stack []ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if !reachable[call] || !ps6090StaticPathLive(pass, call, stack) {
			return true
		}

		repeated := inheritedRepeated || ps6090InsideBenchmarkLoop(pass, call, stack)
		if repeated && !reported[call] {
			if function, signature, matched := ps6090ConfiguredCompute(pass, call, configured); matched &&
				ps6090PrimaryResultDiscarded(pass, graph, call, signature, parents, reachable) {
				reported[call] = true
				pass.Reportf(call.Pos(), "benchmark repetition discards the primary result of configured pure compute call %s; keep it observably live with a typed sink or a race-free semantic oracle (document intentional effect-only measurement with //perfscan:ignore PS6090 <reason>)", ps6090FunctionID(function))
			}
		}

		if function, _, resolved := typedCallee(pass, call.Fun); resolved {
			if declaration := declarations[function.Origin()]; declaration != nil {
				*named = append(*named, ps6090NamedWork{function: declaration, repeated: repeated})
			}
		}
		if literal, ok := ps2110Unparen(call.Fun).(*ast.FuncLit); ok {
			*literals = append(*literals, ps6090LiteralWork{literal: literal, repeated: repeated})
		}
		if ps6090TestingCallback(pass, call) {
			for _, argument := range call.Args {
				ps6090QueueCallback(pass, argument, repeated, declarations, named, literals)
			}
		}
		return true
	})
}

func ps6090ControlFlow(pass *analysis.Pass, body *ast.BlockStmt) *cfg.CFG {
	return cfg.New(body, func(call *ast.CallExpr) bool {
		return !ps6088NonreturnCall(pass, call) && !ps6090TestingTerminator(pass, call)
	})
}

func ps6090ReachableNodes(graph *cfg.CFG) map[ast.Node]bool {
	reachable := make(map[ast.Node]bool)
	for _, block := range graph.Blocks {
		if !block.Live {
			continue
		}
		for _, node := range block.Nodes {
			ast.Inspect(node, func(descendant ast.Node) bool {
				if descendant == nil {
					return true
				}
				reachable[descendant] = true
				_, nested := descendant.(*ast.FuncLit)
				return !nested
			})
		}
	}
	return reachable
}

func ps6090TestingTerminator(pass *analysis.Pass, call *ast.CallExpr) bool {
	function, signature, ok := typedCallee(pass, call.Fun)
	if !ok || function.Pkg() == nil || function.Pkg().Path() != "testing" || signature.Recv() == nil {
		return false
	}
	switch function.Name() {
	case "FailNow", "Fatal", "Fatalf", "Skip", "Skipf", "SkipNow":
		return true
	default:
		return false
	}
}

func ps6090Parents(root ast.Node) map[ast.Node]ast.Node {
	parents := make(map[ast.Node]ast.Node)
	astutil.WithStack(root, func(node ast.Node, stack []ast.Node) bool {
		if len(stack) > 0 {
			parents[node] = stack[len(stack)-1]
		}
		return true
	})
	return parents
}

func ps6090InsideBenchmarkLoop(pass *analysis.Pass, node ast.Node, stack []ast.Node) bool {
	for index, ancestor := range stack {
		child := node
		if index+1 < len(stack) {
			child = stack[index+1]
		}
		switch loop := ancestor.(type) {
		case *ast.ForStmt:
			if (child == loop.Body || child == loop.Post) && ps6090BenchmarkFor(pass, loop) {
				return true
			}
		case *ast.RangeStmt:
			if child == loop.Body && ps6090TestingBN(pass, loop.X) {
				return true
			}
		}
	}
	return false
}

func ps6090BenchmarkFor(pass *analysis.Pass, loop *ast.ForStmt) bool {
	if loop == nil || loop.Cond == nil {
		return false
	}
	if ps6090TestingIterationCall(pass, loop.Cond) {
		return true
	}

	initialization, ok := loop.Init.(*ast.AssignStmt)
	if !ok || len(initialization.Lhs) != 1 || len(initialization.Rhs) != 1 ||
		(initialization.Tok != token.DEFINE && initialization.Tok != token.ASSIGN) ||
		!ps6090IntegerConstant(pass, initialization.Rhs[0], 0) {
		return false
	}
	index, ok := ps2110Unparen(initialization.Lhs[0]).(*ast.Ident)
	if !ok || index.Name == "_" {
		return false
	}
	indexObject := identObject(pass, index)
	post, ok := loop.Post.(*ast.IncDecStmt)
	if !ok || post.Tok != token.INC || !ps6090ObjectExpression(pass, post.X, indexObject) {
		return false
	}
	condition, ok := ps2110Unparen(loop.Cond).(*ast.BinaryExpr)
	if !ok {
		return false
	}
	switch condition.Op {
	case token.LSS, token.NEQ:
		return ps6090ObjectExpression(pass, condition.X, indexObject) && ps6090TestingBN(pass, condition.Y)
	case token.GTR:
		return ps6090TestingBN(pass, condition.X) && ps6090ObjectExpression(pass, condition.Y, indexObject)
	default:
		return false
	}
}

func ps6090TestingIterationCall(pass *analysis.Pass, expression ast.Expr) bool {
	call, ok := ps2110Unparen(expression).(*ast.CallExpr)
	if !ok || len(call.Args) != 0 {
		return false
	}
	function, signature, ok := typedCallee(pass, call.Fun)
	return ok && function.Pkg() != nil && function.Pkg().Path() == "testing" &&
		((function.Name() == "Loop" && typedReceiverNamed(signature, "testing", "B")) ||
			(function.Name() == "Next" && typedReceiverNamed(signature, "testing", "PB")))
}

func ps6090IntegerConstant(pass *analysis.Pass, expression ast.Expr, want int64) bool {
	value := pass.TypesInfo.Types[ps2110Unparen(expression)].Value
	if value == nil || value.Kind() != constant.Int {
		return false
	}
	got, exact := constant.Int64Val(value)
	return exact && got == want
}

func ps6090ObjectExpression(pass *analysis.Pass, expression ast.Expr, object types.Object) bool {
	identifier, ok := ps2110Unparen(expression).(*ast.Ident)
	return ok && object != nil && identObject(pass, identifier) == object
}

func ps6090TestingBN(pass *analysis.Pass, expression ast.Expr) bool {
	selector, ok := ps2110Unparen(expression).(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "N" {
		return false
	}
	selection := pass.TypesInfo.Selections[selector]
	if selection == nil || selection.Kind() != types.FieldVal {
		return false
	}
	field, ok := selection.Obj().(*types.Var)
	if !ok || field.Pkg() == nil || field.Pkg().Path() != "testing" || field.Name() != "N" {
		return false
	}
	receiver := types.Unalias(selection.Recv())
	if pointer, ok := receiver.(*types.Pointer); ok {
		receiver = types.Unalias(pointer.Elem())
	}
	named, ok := receiver.(*types.Named)
	return ok && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == "testing" && named.Obj().Name() == "B"
}

func ps6090StaticPathLive(pass *analysis.Pass, node ast.Node, stack []ast.Node) bool {
	for index, ancestor := range stack {
		child := node
		if index+1 < len(stack) {
			child = stack[index+1]
		}
		switch statement := ancestor.(type) {
		case *ast.IfStmt:
			condition, known := ps6090Boolean(pass, statement.Cond)
			if known && ((!condition && child == statement.Body) ||
				(condition && statement.Else != nil && child == statement.Else)) {
				return false
			}
		case *ast.ForStmt:
			condition, known := ps6090Boolean(pass, statement.Cond)
			if known && !condition && (child == statement.Body || child == statement.Post) {
				return false
			}
		case *ast.BinaryExpr:
			if child != statement.Y {
				continue
			}
			left, known := ps6090Boolean(pass, statement.X)
			if known && ((statement.Op == token.LAND && !left) || (statement.Op == token.LOR && left)) {
				return false
			}
		case *ast.SwitchStmt:
			clause := ps6090StackCaseClause(stack[index+1:])
			if clause != nil && !ps6090SwitchClauseLive(pass, statement, clause) {
				return false
			}
		}
	}
	return true
}

func ps6090Boolean(pass *analysis.Pass, expression ast.Expr) (bool, bool) {
	if expression == nil {
		return false, false
	}
	value := pass.TypesInfo.Types[ps2110Unparen(expression)].Value
	if value == nil || value.Kind() != constant.Bool {
		return false, false
	}
	return constant.BoolVal(value), true
}

func ps6090StackCaseClause(stack []ast.Node) *ast.CaseClause {
	for _, node := range stack {
		if clause, ok := node.(*ast.CaseClause); ok {
			return clause
		}
	}
	return nil
}

func ps6090SwitchClauseLive(pass *analysis.Pass, statement *ast.SwitchStmt, target *ast.CaseClause) bool {
	tag := constant.MakeBool(true)
	if statement.Tag != nil {
		tag = pass.TypesInfo.Types[ps2110Unparen(statement.Tag)].Value
		if tag == nil {
			return true
		}
	}

	clauses := make([]*ast.CaseClause, 0, len(statement.Body.List))
	selected := -1
	defaultIndex := -1
	for _, node := range statement.Body.List {
		clause, ok := node.(*ast.CaseClause)
		if !ok {
			return true
		}
		clauses = append(clauses, clause)
		index := len(clauses) - 1
		if len(clause.List) == 0 {
			defaultIndex = index
			continue
		}
		for _, expression := range clause.List {
			if selected >= 0 {
				break
			}
			value := pass.TypesInfo.Types[ps2110Unparen(expression)].Value
			if value == nil {
				return true
			}
			if constant.Compare(tag, token.EQL, value) {
				selected = index
			}
		}
	}
	if selected < 0 {
		selected = defaultIndex
	}
	targetIndex := -1
	for index, clause := range clauses {
		if clause == target {
			targetIndex = index
			break
		}
	}
	if selected < 0 || targetIndex < selected {
		return false
	}
	for index := selected; index < targetIndex; index++ {
		body := clauses[index].Body
		if len(body) == 0 {
			return false
		}
		branch, ok := body[len(body)-1].(*ast.BranchStmt)
		if !ok || branch.Tok != token.FALLTHROUGH {
			return false
		}
	}
	return true
}

func ps6090TestingCallback(pass *analysis.Pass, call *ast.CallExpr) bool {
	function, signature, ok := typedCallee(pass, call.Fun)
	if !ok || function.Pkg() == nil || function.Pkg().Path() != "testing" {
		return false
	}
	return typedReceiverNamed(signature, "testing", "B") &&
		(function.Name() == "Run" || function.Name() == "RunParallel")
}

func ps6090QueueCallback(
	pass *analysis.Pass,
	expression ast.Expr,
	repeated bool,
	declarations map[*types.Func]*ast.FuncDecl,
	named *[]ps6090NamedWork,
	literals *[]ps6090LiteralWork,
) {
	if literal, ok := ps2110Unparen(expression).(*ast.FuncLit); ok {
		*literals = append(*literals, ps6090LiteralWork{literal: literal, repeated: repeated})
		return
	}
	function, _, ok := typedCallee(pass, expression)
	if !ok {
		return
	}
	if declaration := declarations[function.Origin()]; declaration != nil {
		*named = append(*named, ps6090NamedWork{function: declaration, repeated: repeated})
	}
}

func ps6090ConfiguredCompute(pass *analysis.Pass, call *ast.CallExpr, configured map[string]bool) (*types.Func, *types.Signature, bool) {
	function, signature, ok := typedCallee(pass, call.Fun)
	if !ok || function.Pkg() == nil {
		return nil, nil, false
	}
	return function, signature, configured[function.Name()] || configured[ps6090FunctionID(function)]
}

func ps6090FunctionID(function *types.Func) string {
	if function == nil || function.Pkg() == nil {
		return "compute"
	}
	origin := function.Origin()
	signature, _ := origin.Type().(*types.Signature)
	if signature == nil || signature.Recv() == nil {
		return origin.Pkg().Path() + "." + origin.Name()
	}
	receiver := types.Unalias(signature.Recv().Type())
	if pointer, ok := receiver.(*types.Pointer); ok {
		receiver = types.Unalias(pointer.Elem())
	}
	if named, ok := receiver.(*types.Named); ok && named.Obj() != nil {
		return origin.Pkg().Path() + "." + named.Obj().Name() + "." + origin.Name()
	}
	return origin.Pkg().Path() + "." + origin.Name()
}

func ps6090PrimaryResultDiscarded(
	pass *analysis.Pass,
	graph *cfg.CFG,
	call *ast.CallExpr,
	signature *types.Signature,
	parents map[ast.Node]ast.Node,
	reachable map[ast.Node]bool,
) bool {
	primary := ps6090PrimaryResult(signature)
	if primary < 0 {
		return false
	}
	parent := ps6090NonParenParent(call, parents)
	switch value := parent.(type) {
	case *ast.ExprStmt:
		return ps2110Unparen(value.X) == call
	case *ast.GoStmt:
		return value.Call == call
	case *ast.DeferStmt:
		return value.Call == call
	case *ast.AssignStmt:
		target, ok := ps6090AssignmentResultTarget(call, signature, primary, value.Lhs, value.Rhs)
		if !ok {
			return false
		}
		return ps6090DiscardedTarget(pass, graph, value, target, value.Tok == token.DEFINE, parents, reachable)
	case *ast.ValueSpec:
		target, ok := ps6090ValueSpecResultTarget(call, signature, primary, value.Names, value.Values)
		if !ok {
			return false
		}
		return ps6090DiscardedTarget(pass, graph, value, target, true, parents, reachable)
	default:
		return false
	}
}

func ps6090AssignmentResultTarget(
	call *ast.CallExpr,
	signature *types.Signature,
	primary int,
	lhs, rhs []ast.Expr,
) (ast.Expr, bool) {
	if len(rhs) == 1 {
		if ps2110Unparen(rhs[0]) != call || primary >= len(lhs) {
			return nil, false
		}
		return lhs[primary], true
	}
	if signature == nil || signature.Results().Len() != 1 || primary != 0 || len(lhs) != len(rhs) {
		return nil, false
	}
	for index, expression := range rhs {
		if ps2110Unparen(expression) == call {
			return lhs[index], true
		}
	}
	return nil, false
}

func ps6090ValueSpecResultTarget(
	call *ast.CallExpr,
	signature *types.Signature,
	primary int,
	names []*ast.Ident,
	values []ast.Expr,
) (ast.Expr, bool) {
	if len(values) == 1 {
		if ps2110Unparen(values[0]) != call || primary >= len(names) {
			return nil, false
		}
		return names[primary], true
	}
	if signature == nil || signature.Results().Len() != 1 || primary != 0 || len(names) != len(values) {
		return nil, false
	}
	for index, expression := range values {
		if ps2110Unparen(expression) == call {
			return names[index], true
		}
	}
	return nil, false
}

func ps6090PrimaryResult(signature *types.Signature) int {
	if signature == nil {
		return -1
	}
	errorType := types.Universe.Lookup("error").Type()
	for index := 0; index < signature.Results().Len(); index++ {
		if !types.AssignableTo(signature.Results().At(index).Type(), errorType) {
			return index
		}
	}
	return -1
}

func ps6090DiscardedTarget(
	pass *analysis.Pass,
	graph *cfg.CFG,
	origin ast.Node,
	target ast.Expr,
	fresh bool,
	parents map[ast.Node]ast.Node,
	reachable map[ast.Node]bool,
) bool {
	identifier, ok := ps2110Unparen(target).(*ast.Ident)
	if !ok {
		return false
	}
	if identifier.Name == "_" {
		return true
	}
	if !fresh {
		return false
	}
	object := pass.TypesInfo.Defs[identifier]
	if object == nil {
		return false
	}
	return ps6090FreshValueDiscarded(pass, graph, origin, object, parents, reachable)
}

func ps6090FreshValueDiscarded(
	pass *analysis.Pass,
	graph *cfg.CFG,
	origin ast.Node,
	object types.Object,
	parents map[ast.Node]ast.Node,
	reachable map[ast.Node]bool,
) bool {
	discarded, live, _ := ps6090FlowFreshObject(pass, graph, origin, object, false, parents, reachable)
	return discarded && !live
}

type ps6090FlowStates uint8

type ps6090FlowPoint struct {
	block       *cfg.Block
	predecessor *cfg.Block
}

const (
	ps6090FlowEmpty ps6090FlowStates = 1 << iota
	ps6090FlowOld
	ps6090FlowDeferred
	ps6090FlowOldDeferred
)

func ps6090FlowState(oldValue, deferred bool) ps6090FlowStates {
	switch {
	case oldValue && deferred:
		return ps6090FlowOldDeferred
	case oldValue:
		return ps6090FlowOld
	case deferred:
		return ps6090FlowDeferred
	default:
		return ps6090FlowEmpty
	}
}

func ps6090FlowStateOld(state ps6090FlowStates) bool {
	return state == ps6090FlowOld || state == ps6090FlowOldDeferred
}

func ps6090FlowStateDeferred(state ps6090FlowStates) bool {
	return state == ps6090FlowDeferred || state == ps6090FlowOldDeferred
}

func ps6090FlowFreshObject(
	pass *analysis.Pass,
	graph *cfg.CFG,
	origin ast.Node,
	object types.Object,
	entryOldValue bool,
	parents map[ast.Node]ast.Node,
	reachable map[ast.Node]bool,
) (bool, bool, bool) {
	if len(graph.Blocks) == 0 {
		return false, false, entryOldValue
	}

	entry := ps6090FlowPoint{block: graph.Blocks[0]}
	statesIn := map[ps6090FlowPoint]ps6090FlowStates{
		entry: ps6090FlowState(entryOldValue, false),
	}
	work := []ps6090FlowPoint{entry}
	sawDiscard := false
	exitOldValue := false
	for len(work) > 0 {
		point := work[0]
		work = work[1:]
		block := point.block
		if !block.Live {
			continue
		}
		states := statesIn[point]
		for _, node := range block.Nodes {
			nextStates := ps6090FlowStates(0)
			for state := ps6090FlowEmpty; state <= ps6090FlowOldDeferred; state <<= 1 {
				if states&state == 0 {
					continue
				}
				oldValue, discarded, live := ps6090FlowFreshValue(
					pass, node, origin, object, ps6090FlowStateOld(state), parents, reachable,
				)
				sawDiscard = sawDiscard || discarded
				if live {
					return sawDiscard, true, oldValue
				}
				deferred := ps6090FlowStateDeferred(state)
				if _, ok := node.(*ast.DeferStmt); ok {
					deferred = true
				}
				nextStates |= ps6090FlowState(oldValue, deferred)
			}
			states = nextStates
		}
		successors := ps6090FlowSuccessors(pass, block, point.predecessor, parents)
		// A zero-successor block is either a normal return or an abnormal
		// exit such as panic, os.Exit, or a testing terminator. Only the
		// former propagates a captured definition back to an invoked
		// closure's caller. A preceding defer remains conservative because
		// it may recover a panic and turn that exit into a normal return.
		if len(successors) == 0 {
			for state := ps6090FlowEmpty; state <= ps6090FlowOldDeferred; state <<= 1 {
				if states&state == 0 || !ps6090FlowStateOld(state) {
					continue
				}
				if block.Return() != nil ||
					(ps6090FlowStateDeferred(state) && ps6090RecoverablePanicExit(pass, block)) {
					exitOldValue = true
				}
			}
		}
		for _, successor := range successors {
			successorStates := ps6090FlowStates(0)
			for state := ps6090FlowEmpty; state <= ps6090FlowOldDeferred; state <<= 1 {
				if states&state == 0 {
					continue
				}
				successorOldValue, discarded, live := ps6090ControlBodyFreshValue(
					pass, successor, object, ps6090FlowStateOld(state), parents, reachable,
				)
				sawDiscard = sawDiscard || discarded
				if live {
					return sawDiscard, true, successorOldValue
				}
				successorStates |= ps6090FlowState(successorOldValue, ps6090FlowStateDeferred(state))
			}
			successorPoint := ps6090FlowPoint{block: successor, predecessor: block}
			if !successor.Live || successorStates&^statesIn[successorPoint] == 0 {
				continue
			}
			statesIn[successorPoint] |= successorStates
			work = append(work, successorPoint)
		}
	}
	return sawDiscard, false, exitOldValue
}

func ps6090RecoverablePanicExit(pass *analysis.Pass, block *cfg.Block) bool {
	for _, node := range block.Nodes {
		statement, ok := node.(*ast.ExprStmt)
		if !ok {
			continue
		}
		call, ok := ps2110Unparen(statement.X).(*ast.CallExpr)
		if ok && ps6079PanicCall(pass, call) {
			return true
		}
	}
	return false
}

func ps6090ControlBodyFreshValue(
	pass *analysis.Pass,
	block *cfg.Block,
	object types.Object,
	oldValue bool,
	parents map[ast.Node]ast.Node,
	reachable map[ast.Node]bool,
) (bool, bool, bool) {
	var targets []ast.Expr
	switch block.Kind {
	case cfg.KindRangeBody:
		statement, ok := block.Stmt.(*ast.RangeStmt)
		if !ok {
			return oldValue, false, false
		}
		targets = []ast.Expr{statement.Key, statement.Value}
	case cfg.KindSelectCaseBody:
		clause, ok := block.Stmt.(*ast.CommClause)
		if !ok {
			return oldValue, false, false
		}
		assignment, ok := clause.Comm.(*ast.AssignStmt)
		if !ok {
			return oldValue, false, false
		}
		targets = assignment.Lhs
	default:
		return oldValue, false, false
	}
	discarded := false
	writesObject := false
	for _, target := range targets {
		if target == nil {
			continue
		}
		if ps6090ExpressionObject(pass, target, object) {
			writesObject = true
			continue
		}
		seenDiscard, seenLive, afterTarget := ps6090ObserveFreshValue(
			pass, []ast.Expr{target}, object, oldValue, parents, reachable,
		)
		discarded = discarded || seenDiscard
		if seenLive {
			return oldValue, discarded, true
		}
		oldValue = afterTarget
	}
	if writesObject {
		oldValue = false
	}
	return oldValue, discarded, false
}

func ps6090FlowSuccessors(
	pass *analysis.Pass,
	block, predecessor *cfg.Block,
	parents map[ast.Node]ast.Node,
) []*cfg.Block {
	successors := block.Succs
	if block.Kind == cfg.KindRangeLoop && ps6090RangeDefinitelyEmpty(pass, block.Stmt) && len(successors) == 2 {
		successors = successors[1:]
	}
	if block.Kind == cfg.KindRangeLoop && ps6090RangeDefinitelyNonempty(pass, block.Stmt) && len(successors) == 2 &&
		!ps6090RangeBackedge(predecessor, block.Stmt) {
		successors = successors[:1]
	}
	if len(successors) == 2 && successors[0].Kind == cfg.KindSelectCaseBody &&
		ps6090SelectCaseDefinitelyDisabled(pass, successors[0].Stmt) {
		successors = successors[1:]
	}
	if len(successors) != 2 || len(block.Nodes) == 0 {
		return successors
	}
	condition, ok := block.Nodes[len(block.Nodes)-1].(ast.Expr)
	if !ok {
		return successors
	}
	value, known := ps6090FlowCondition(pass, condition, parents)
	if !known {
		return successors
	}
	if value {
		return successors[:1]
	}
	return successors[1:]
}

func ps6090RangeBackedge(predecessor *cfg.Block, statement ast.Stmt) bool {
	rangeStatement, ok := statement.(*ast.RangeStmt)
	if !ok || predecessor == nil {
		return false
	}
	if predecessor.Kind == cfg.KindRangeBody && predecessor.Stmt == statement {
		return true
	}
	insideBody := func(node ast.Node) bool {
		return node != nil && node.Pos() >= rangeStatement.Body.Pos() && node.End() <= rangeStatement.Body.End()
	}
	for _, node := range predecessor.Nodes {
		if insideBody(node) {
			return true
		}
	}
	return predecessor.Stmt != statement && insideBody(predecessor.Stmt)
}

func ps6090RangeDefinitelyEmpty(pass *analysis.Pass, statement ast.Stmt) bool {
	rangeStatement, ok := statement.(*ast.RangeStmt)
	if !ok {
		return false
	}
	expression := ps2110Unparen(rangeStatement.X)
	if ps6090ZeroLengthArrayRange(pass.TypesInfo.TypeOf(expression)) {
		return true
	}
	if constantValue := pass.TypesInfo.Types[expression].Value; constantValue != nil {
		switch constantValue.Kind() {
		case constant.String:
			return constant.StringVal(constantValue) == ""
		case constant.Int:
			return constant.Sign(constantValue) <= 0
		}
	}
	switch value := expression.(type) {
	case *ast.CompositeLit:
		typ := pass.TypesInfo.TypeOf(value)
		if typ == nil {
			return false
		}
		switch ranged := typ.Underlying().(type) {
		case *types.Array:
			return ranged.Len() == 0
		case *types.Map, *types.Slice:
			return len(value.Elts) == 0
		}
	case *ast.CallExpr:
		return ps6090FreshEmptyRange(pass, value)
	}
	return false
}

func ps6090RangeDefinitelyNonempty(pass *analysis.Pass, statement ast.Stmt) bool {
	rangeStatement, ok := statement.(*ast.RangeStmt)
	if !ok {
		return false
	}
	expression := ps2110Unparen(rangeStatement.X)
	if length, pointer, ok := ps6090ArrayRangeLength(pass.TypesInfo.TypeOf(expression)); ok {
		return length > 0 && (!pointer || !ps6090RangeHasEffectiveValue(rangeStatement) ||
			ps6090DefinitelyNonNilArrayPointer(pass, expression))
	}
	if constantValue := pass.TypesInfo.Types[expression].Value; constantValue != nil {
		switch constantValue.Kind() {
		case constant.String:
			return constant.StringVal(constantValue) != ""
		case constant.Int:
			return constant.Sign(constantValue) > 0
		}
	}
	switch value := expression.(type) {
	case *ast.CompositeLit:
		underlying := pass.TypesInfo.TypeOf(value)
		if underlying == nil {
			return false
		}
		switch underlying.Underlying().(type) {
		case *types.Map, *types.Slice:
			return len(value.Elts) > 0
		}
	case *ast.CallExpr:
		return ps6090FreshNonemptyRange(pass, value)
	}
	return false
}

func ps6090ZeroLengthArrayRange(typ types.Type) bool {
	length, _, ok := ps6090ArrayRangeLength(typ)
	return ok && length == 0
}

func ps6090ArrayRangeLength(typ types.Type) (length int64, pointer, ok bool) {
	if typ == nil {
		return 0, false, false
	}
	typ = types.Unalias(typ).Underlying()
	if pointerType, isPointer := typ.(*types.Pointer); isPointer {
		pointer = true
		typ = types.Unalias(pointerType.Elem()).Underlying()
	}
	array, ok := typ.(*types.Array)
	if !ok {
		return 0, false, false
	}
	return array.Len(), pointer, true
}

func ps6090RangeHasEffectiveValue(statement *ast.RangeStmt) bool {
	return statement.Value != nil && !ps6090BlankIdentifier(statement.Value)
}

func ps6090DefinitelyNonNilArrayPointer(pass *analysis.Pass, expression ast.Expr) bool {
	switch value := ps2110Unparen(expression).(type) {
	case *ast.UnaryExpr:
		if value.Op != token.AND {
			return false
		}
		_, addressOfDereference := ps2110Unparen(value.X).(*ast.StarExpr)
		return !addressOfDereference
	case *ast.CallExpr:
		identifier, ok := ps2110Unparen(value.Fun).(*ast.Ident)
		return ok && pass.TypesInfo.Uses[identifier] == types.Universe.Lookup("new")
	default:
		return false
	}
}

func ps6090FreshEmptyRange(pass *analysis.Pass, call *ast.CallExpr) bool {
	if call == nil {
		return false
	}
	underlying := pass.TypesInfo.TypeOf(call)
	if underlying == nil {
		return false
	}
	underlying = types.Unalias(underlying).Underlying()

	function := ps2110Unparen(call.Fun)
	if identifier, ok := function.(*ast.Ident); ok &&
		pass.TypesInfo.Uses[identifier] == types.Universe.Lookup("make") {
		switch underlying.(type) {
		case *types.Map:
			return true
		case *types.Slice:
			return len(call.Args) >= 2 && ps6090IntegerConstant(pass, call.Args[1], 0)
		default:
			return false
		}
	}

	if len(call.Args) != 1 || !pass.TypesInfo.Types[function].IsType() {
		return false
	}
	nilIdentifier, ok := ps2110Unparen(call.Args[0]).(*ast.Ident)
	if !ok || pass.TypesInfo.Uses[nilIdentifier] != types.Universe.Lookup("nil") {
		return false
	}
	switch underlying.(type) {
	case *types.Map, *types.Slice:
		return true
	default:
		return false
	}
}

func ps6090FreshNonemptyRange(pass *analysis.Pass, call *ast.CallExpr) bool {
	if call == nil {
		return false
	}
	underlying := pass.TypesInfo.TypeOf(call)
	if underlying == nil {
		return false
	}
	if _, ok := types.Unalias(underlying).Underlying().(*types.Slice); !ok {
		return false
	}
	function, ok := ps2110Unparen(call.Fun).(*ast.Ident)
	return ok && pass.TypesInfo.Uses[function] == types.Universe.Lookup("make") &&
		len(call.Args) >= 2 && ps6090PositiveIntegerConstant(pass, call.Args[1])
}

func ps6090PositiveIntegerConstant(pass *analysis.Pass, expression ast.Expr) bool {
	value := pass.TypesInfo.Types[ps2110Unparen(expression)].Value
	return value != nil && value.Kind() == constant.Int && constant.Sign(value) > 0
}

func ps6090SelectCaseDefinitelyDisabled(pass *analysis.Pass, statement ast.Stmt) bool {
	clause, ok := statement.(*ast.CommClause)
	if !ok || clause.Comm == nil {
		return false
	}
	var channel ast.Expr
	switch communication := clause.Comm.(type) {
	case *ast.AssignStmt:
		if len(communication.Rhs) != 1 {
			return false
		}
		receive, ok := ps2110Unparen(communication.Rhs[0]).(*ast.UnaryExpr)
		if !ok || receive.Op != token.ARROW {
			return false
		}
		channel = receive.X
	case *ast.ExprStmt:
		receive, ok := ps2110Unparen(communication.X).(*ast.UnaryExpr)
		if !ok || receive.Op != token.ARROW {
			return false
		}
		channel = receive.X
	case *ast.SendStmt:
		channel = communication.Chan
	default:
		return false
	}
	conversion, ok := ps2110Unparen(channel).(*ast.CallExpr)
	if !ok || len(conversion.Args) != 1 || !pass.TypesInfo.Types[ps2110Unparen(conversion.Fun)].IsType() {
		return false
	}
	nilIdentifier, ok := ps2110Unparen(conversion.Args[0]).(*ast.Ident)
	return ok && pass.TypesInfo.Uses[nilIdentifier] == types.Universe.Lookup("nil")
}

func ps6090FlowCondition(
	pass *analysis.Pass,
	condition ast.Expr,
	parents map[ast.Node]ast.Node,
) (bool, bool) {
	switch statement := parents[condition].(type) {
	case *ast.IfStmt:
		if statement.Cond == condition {
			return ps6090Boolean(pass, condition)
		}
	case *ast.ForStmt:
		if statement.Cond == condition {
			return ps6090Boolean(pass, condition)
		}
	case *ast.CaseClause:
		switchStatement := ps6090EnclosingSwitch(statement, parents)
		if switchStatement == nil {
			return false, false
		}
		tag := constant.MakeBool(true)
		if switchStatement.Tag != nil {
			tag = pass.TypesInfo.Types[ps2110Unparen(switchStatement.Tag)].Value
			if tag == nil {
				return false, false
			}
		}
		caseValue := pass.TypesInfo.Types[ps2110Unparen(condition)].Value
		if caseValue == nil {
			return false, false
		}
		return constant.Compare(tag, token.EQL, caseValue), true
	}
	return false, false
}

func ps6090EnclosingSwitch(clause *ast.CaseClause, parents map[ast.Node]ast.Node) *ast.SwitchStmt {
	for ancestor := parents[clause]; ancestor != nil; ancestor = parents[ancestor] {
		if statement, ok := ancestor.(*ast.SwitchStmt); ok {
			return statement
		}
	}
	return nil
}

func ps6090FlowFreshValue(
	pass *analysis.Pass,
	node ast.Node,
	origin ast.Node,
	object types.Object,
	oldValue bool,
	parents map[ast.Node]ast.Node,
	reachable map[ast.Node]bool,
) (bool, bool, bool) {
	if node == origin {
		return true, false, false
	}
	if ps6090ControlAssignmentTarget(node, parents) {
		return oldValue, false, false
	}

	switch value := node.(type) {
	case *ast.AssignStmt:
		if _, selectReceive := parents[value].(*ast.CommClause); selectReceive {
			discarded, live, afterRHS := ps6090ObserveFreshValue(
				pass, value.Rhs, object, oldValue, parents, reachable,
			)
			return afterRHS, discarded, live
		}
		discarded := false
		// Go evaluates index/pointer operands on the left before the right
		// side, then performs the actual assignments left-to-right. Observe
		// complex target addresses in that first phase; direct identifiers
		// are writes only for = and := and are killed after RHS evaluation.
		for _, target := range value.Lhs {
			identifier, direct := ps2110Unparen(target).(*ast.Ident)
			writesObject := direct && pass.TypesInfo.Uses[identifier] == object
			if writesObject && (value.Tok == token.ASSIGN || value.Tok == token.DEFINE) {
				continue
			}
			seenDiscard, seenLive, afterTarget := ps6090ObserveFreshValue(pass, []ast.Expr{target}, object, oldValue, parents, reachable)
			discarded = discarded || seenDiscard
			if seenLive {
				return oldValue, discarded, true
			}
			oldValue = afterTarget
		}
		seenDiscard, live, afterRHS := ps6090ObserveFreshValue(pass, value.Rhs, object, oldValue, parents, reachable)
		discarded = discarded || seenDiscard
		oldValue = afterRHS
		if live {
			return oldValue, discarded, true
		}
		if (value.Tok == token.ASSIGN || value.Tok == token.DEFINE) && ps6090AssignmentWritesObject(pass, value, object) {
			oldValue = false
		}
		return oldValue, discarded, false
	case *ast.IncDecStmt:
		discarded, live, oldValue := ps6090ObserveFreshValue(pass, []ast.Expr{value.X}, object, oldValue, parents, reachable)
		if ps6090ExpressionObject(pass, value.X, object) {
			oldValue = false
		}
		return oldValue, discarded, live
	case *ast.Ident:
		// The CFG also represents range and selected receive targets as
		// standalone identifiers. Their path-specific writes are handled on
		// the edge into the corresponding body block above.
		return oldValue, false, false
	default:
		discarded, live, oldValue := ps6090ObserveFreshValue(pass, []ast.Node{node}, object, oldValue, parents, reachable)
		return oldValue, discarded, live
	}
}

func ps6090ControlAssignmentTarget(node ast.Node, parents map[ast.Node]ast.Node) bool {
	expression, ok := node.(ast.Expr)
	if !ok {
		return false
	}
	target := ast.Expr(expression)
	for {
		parent := parents[target]
		parenthesized, ok := parent.(*ast.ParenExpr)
		if !ok {
			break
		}
		target = parenthesized
	}
	switch parent := parents[target].(type) {
	case *ast.RangeStmt:
		return parent.Key != nil && ps2110Unparen(parent.Key) == ps2110Unparen(target) ||
			parent.Value != nil && ps2110Unparen(parent.Value) == ps2110Unparen(target)
	case *ast.AssignStmt:
		if _, selectedReceive := parents[parent].(*ast.CommClause); !selectedReceive {
			return false
		}
		for _, assignmentTarget := range parent.Lhs {
			if ps2110Unparen(assignmentTarget) == ps2110Unparen(target) {
				return true
			}
		}
	}
	return false
}

func ps6090AssignmentWritesObject(pass *analysis.Pass, assignment *ast.AssignStmt, object types.Object) bool {
	for _, target := range assignment.Lhs {
		if ps6090ExpressionObject(pass, target, object) {
			return true
		}
	}
	return false
}

func ps6090ExpressionObject(pass *analysis.Pass, expression ast.Expr, object types.Object) bool {
	identifier, ok := ps2110Unparen(expression).(*ast.Ident)
	return ok && pass.TypesInfo.Uses[identifier] == object
}

func ps6090ObserveFreshValue[T ast.Node](
	pass *analysis.Pass,
	roots []T,
	object types.Object,
	oldValue bool,
	parents map[ast.Node]ast.Node,
	reachable map[ast.Node]bool,
) (bool, bool, bool) {
	if !oldValue {
		return false, false, false
	}
	discarded := false
	live := false
	for _, root := range roots {
		ast.Inspect(root, func(node ast.Node) bool {
			if node == nil || live || !oldValue {
				return false
			}
			if call, ok := node.(*ast.CallExpr); ok {
				if literal := ps6090InvokedLiteral(pass, call, parents); literal != nil {
					if !ps6090ExecutableNode(pass, call, parents, reachable) {
						return false
					}
					entryOldValue := oldValue
					maySkip := ps6090MaySkipNode(pass, call, parents)
					for _, argument := range call.Args {
						seenDiscard, seenLive, afterArgument := ps6090ObserveFreshValue(pass, []ast.Expr{argument}, object, oldValue, parents, reachable)
						discarded = discarded || seenDiscard
						if seenLive {
							live = true
							return false
						}
						oldValue = afterArgument
					}
					if oldValue {
						seenDiscard, seenLive, afterCall := ps6090LiteralObservesFreshValue(pass, literal, object, parents, reachable)
						discarded = discarded || seenDiscard
						live = seenLive
						oldValue = afterCall
					}
					if maySkip {
						oldValue = oldValue || entryOldValue
					}
					return false
				}
			}
			if literal, ok := node.(*ast.FuncLit); ok {
				if !ps6090ExecutableNode(pass, literal, parents, reachable) {
					return false
				}
				if !ps6090DeferLiteralObservation(pass, literal, parents) {
					_, seenLive, _ := ps6090LiteralObservesFreshValue(pass, literal, object, parents, reachable)
					live = seenLive
				}
				return false
			}
			use, ok := node.(*ast.Ident)
			if !ok || pass.TypesInfo.Uses[use] != object || !ps6090ExecutableUse(pass, use, parents, reachable) {
				return true
			}
			if ps6090BlankRead(use, parents) {
				discarded = true
				return true
			}
			live = true
			return false
		})
		if live {
			break
		}
	}
	return discarded, live, oldValue
}

func ps6090MaySkipNode(pass *analysis.Pass, node ast.Node, parents map[ast.Node]ast.Node) bool {
	child := node
	for parent := parents[child]; parent != nil; parent = parents[parent] {
		binary, ok := parent.(*ast.BinaryExpr)
		if ok && child == binary.Y && (binary.Op == token.LAND || binary.Op == token.LOR) {
			left, known := ps6090Boolean(pass, binary.X)
			if !known {
				return true
			}
			if binary.Op == token.LAND && !left || binary.Op == token.LOR && left {
				return true
			}
		}
		child = parent
	}
	return false
}

func ps6090InvokedLiteral(pass *analysis.Pass, call *ast.CallExpr, parents map[ast.Node]ast.Node) *ast.FuncLit {
	switch function := ps2110Unparen(call.Fun).(type) {
	case *ast.FuncLit:
		switch parents[call].(type) {
		case *ast.DeferStmt, *ast.GoStmt:
			return nil
		}
		return function
	case *ast.Ident:
		object := pass.TypesInfo.Uses[function]
		if object == nil {
			return nil
		}
		body := ps6090EnclosingFunctionBody(call, parents)
		if body == nil {
			return nil
		}
		var literal *ast.FuncLit
		ast.Inspect(body, func(node ast.Node) bool {
			if node == nil || literal != nil {
				return false
			}
			if _, nested := node.(*ast.FuncLit); nested {
				return false
			}
			identifier, ok := node.(*ast.Ident)
			if !ok || pass.TypesInfo.Defs[identifier] != object {
				return true
			}
			candidate := ps6090BoundLiteral(identifier, parents)
			if candidate != nil && ps6090DeferLiteralObservation(pass, candidate, parents) {
				literal = candidate
			}
			return false
		})
		return literal
	}
	return nil
}

func ps6090DeferLiteralObservation(pass *analysis.Pass, literal *ast.FuncLit, parents map[ast.Node]ast.Node) bool {
	identifier, object := ps6090LiteralBinding(pass, literal, parents)
	if identifier == nil || object == nil {
		return false
	}
	body := ps6090EnclosingFunctionBody(literal, parents)
	if body == nil {
		return false
	}
	safe := true
	ast.Inspect(body, func(node ast.Node) bool {
		if node == nil || !safe {
			return false
		}
		if node == literal {
			return false
		}
		use, ok := node.(*ast.Ident)
		if !ok || pass.TypesInfo.Uses[use] != object {
			return true
		}
		if ps6090EnclosingFunctionBody(use, parents) == body &&
			(ps6090BlankRead(use, parents) || ps6090DirectCallee(use, parents)) {
			return true
		}
		safe = false
		return false
	})
	return safe
}

func ps6090LiteralBinding(pass *analysis.Pass, literal *ast.FuncLit, parents map[ast.Node]ast.Node) (*ast.Ident, types.Object) {
	parent := ps6090NonParenParent(literal, parents)
	switch value := parent.(type) {
	case *ast.AssignStmt:
		if len(value.Lhs) != len(value.Rhs) {
			return nil, nil
		}
		for index, expression := range value.Rhs {
			if ps2110Unparen(expression) != literal {
				continue
			}
			identifier, ok := ps2110Unparen(value.Lhs[index]).(*ast.Ident)
			if !ok {
				return nil, nil
			}
			return identifier, pass.TypesInfo.Defs[identifier]
		}
	case *ast.ValueSpec:
		if len(value.Names) != len(value.Values) {
			return nil, nil
		}
		for index, expression := range value.Values {
			if ps2110Unparen(expression) == literal {
				identifier := value.Names[index]
				return identifier, pass.TypesInfo.Defs[identifier]
			}
		}
	}
	return nil, nil
}

func ps6090BoundLiteral(identifier *ast.Ident, parents map[ast.Node]ast.Node) *ast.FuncLit {
	parent := ps6090NonParenParent(identifier, parents)
	switch value := parent.(type) {
	case *ast.AssignStmt:
		if len(value.Lhs) != len(value.Rhs) {
			return nil
		}
		for index, target := range value.Lhs {
			if ps2110Unparen(target) == identifier {
				literal, _ := ps2110Unparen(value.Rhs[index]).(*ast.FuncLit)
				return literal
			}
		}
	case *ast.ValueSpec:
		if len(value.Names) != len(value.Values) {
			return nil
		}
		for index, name := range value.Names {
			if name == identifier {
				literal, _ := ps2110Unparen(value.Values[index]).(*ast.FuncLit)
				return literal
			}
		}
	}
	return nil
}

func ps6090DirectCallee(identifier *ast.Ident, parents map[ast.Node]ast.Node) bool {
	call, ok := ps6090NonParenParent(identifier, parents).(*ast.CallExpr)
	if !ok || ps2110Unparen(call.Fun) != identifier {
		return false
	}
	switch parents[call].(type) {
	case *ast.DeferStmt, *ast.GoStmt:
		return false
	}
	return true
}

func ps6090EnclosingFunctionBody(node ast.Node, parents map[ast.Node]ast.Node) *ast.BlockStmt {
	var outermost *ast.BlockStmt
	for ancestor := node; ancestor != nil; ancestor = parents[ancestor] {
		body, ok := ancestor.(*ast.BlockStmt)
		if !ok {
			continue
		}
		outermost = body
		switch parents[body].(type) {
		case *ast.FuncDecl, *ast.FuncLit:
			return body
		}
	}
	return outermost
}

func ps6090LiteralObservesFreshValue(
	pass *analysis.Pass,
	literal *ast.FuncLit,
	object types.Object,
	parents map[ast.Node]ast.Node,
	enclosingReachable map[ast.Node]bool,
) (bool, bool, bool) {
	if !enclosingReachable[literal] {
		return false, false, true
	}
	graph := ps6090ControlFlow(pass, literal.Body)
	reachable := ps6090ReachableNodes(graph)
	return ps6090FlowFreshObject(pass, graph, nil, object, true, parents, reachable)
}

func ps6090ExecutableUse(
	pass *analysis.Pass,
	use *ast.Ident,
	parents map[ast.Node]ast.Node,
	reachable map[ast.Node]bool,
) bool {
	return ps6090ExecutableNode(pass, use, parents, reachable)
}

func ps6090ExecutableNode(
	pass *analysis.Pass,
	node ast.Node,
	parents map[ast.Node]ast.Node,
	reachable map[ast.Node]bool,
) bool {
	stack := ps6090AncestorStack(node, parents)
	return reachable[node] && ps6090StaticPathLive(pass, node, stack)
}

func ps6090AncestorStack(node ast.Node, parents map[ast.Node]ast.Node) []ast.Node {
	stack := make([]ast.Node, 0, 8)
	for ancestor := parents[node]; ancestor != nil; ancestor = parents[ancestor] {
		stack = append(stack, ancestor)
	}
	for left, right := 0, len(stack)-1; left < right; left, right = left+1, right-1 {
		stack[left], stack[right] = stack[right], stack[left]
	}
	return stack
}

func ps6090BlankRead(identifier *ast.Ident, parents map[ast.Node]ast.Node) bool {
	parent := ps6090NonParenParent(identifier, parents)
	switch value := parent.(type) {
	case *ast.AssignStmt:
		for index, expression := range value.Rhs {
			if ps2110Unparen(expression) == identifier && len(value.Rhs) == len(value.Lhs) &&
				index < len(value.Lhs) && ps6090BlankIdentifier(value.Lhs[index]) {
				return true
			}
		}
	case *ast.ValueSpec:
		for index, expression := range value.Values {
			if ps2110Unparen(expression) == identifier && len(value.Values) == len(value.Names) &&
				index < len(value.Names) && value.Names[index].Name == "_" {
				return true
			}
		}
	}
	return false
}

func ps6090BlankIdentifier(expression ast.Expr) bool {
	identifier, ok := ps2110Unparen(expression).(*ast.Ident)
	return ok && identifier.Name == "_"
}

func ps6090NonParenParent(node ast.Node, parents map[ast.Node]ast.Node) ast.Node {
	parent := parents[node]
	for {
		parentheses, ok := parent.(*ast.ParenExpr)
		if !ok {
			return parent
		}
		parent = parents[parentheses]
	}
}
