package checks

import (
	"bytes"
	"go/ast"
	"go/constant"
	"go/format"
	"go/token"
	"go/types"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/cfg"

	"github.com/jxsl13/perfscan/lint"
)

// PS6095 implements owner issue #909. It finds an exact floating-point
// quotient that is recomputed for every output/dot element even though all of
// its inputs are invariant in that loop.
var PS6095 = register(&lint.Check{
	ID:       "PS6095",
	Category: "verify",
	Slug:     "exact-output-invariant-float-quotient",
	Level:    lint.LevelStructured,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "an exact floating-point quotient is recomputed for every output element",
		Text: `A reduction or combine kernel can evaluate the same floating-point
division for every output element even though neither operand depends on the
output index. Computing the original division once and reusing that rounded
result is bit-identical and can remove repeated high-latency divisions when
the compiler has not already hoisted them. A source finding does not prove
repeated machine instructions: the compiler may already hoist a simple scalar
quotient. Inspect optimized code and benchmark the real caller before treating
the rewrite as a speedup; deterministic source equivalence is not a runtime
performance guarantee.

Replacing x/y with x*(1/y) is not the same transformation. The reciprocal is
rounded before the multiplication and can change finite result bits, NaN
behavior, and signed zero. PS6095 therefore recommends and, where safe,
automatically applies only an exact cache of the original division expression.

The check requires a canonical integer range or zero-based counted loop with a
material dynamic or at-least-four-element domain. A type-identified loop index
must reach the output subscript or the other factor of a dot-product update,
while no quotient input may depend on that index. Findings must be on a live
panic-aware CFG path.

Dependencies are tracked by go/types objects rather than identifier spelling.
Package variables, locals captured by another closure, address-taken locals,
and inputs written in the loop are rejected. A function literal may use a
captured scalar only when no enclosing or sibling closure can mutate or escape
it. This ownership tracking includes the implicit variable declared for each
type-switch case. Indexed slice operands are
reported only when every loop store is proven disjoint (for example, a local
fresh make result that stays fresh across every enclosing-loop re-entry), and
any unknown call, indirect write, possible slice alias, or mutation of operand
storage suppresses the finding. Taking an element address or constructing an
unsafe alias ends that fresh-allocation proof. So does selecting or invoking a
pointer-receiver method on a fresh root or one of its elements, because Go may
implicitly take its address. Slicing an array-valued element or descendant
likewise exposes the fresh backing storage. Capturing a fresh destination in
any function literal also ends the proof, including nested and read-only
closures, because the slice can escape or be rebound outside the local CFG.

A direct package-local helper is recognized across a function boundary only
when its body is exactly one return of a floating-point division over all of
its scalar numeric value parameters and constants. Reference parameters,
unused parameters, methods, closures, globals, calls, extra statements, and
effectful wrappers receive no purity inference.

Canonical loops inside function literals are analyzed in the literal's own
control-flow graph and lexical fix scope. A counted-loop bound must also remain
stable and unescaped throughout every iteration. Parent, name, control-flow,
write, capture, and freshness facts are prepared once per function context;
nested loop effects are queried without repeatedly descending their subtrees.

The automatic fix is narrower still: all operands must be pure scalar value
expressions already in scope before the loop. This avoids moving a possible
index or pointer panic onto a zero-trip path. The fix introduces one local per
unchanged division (or proven exact helper call) and replaces each in-loop
expression with that local; it never synthesizes a reciprocal multiplication.
Storage-backed findings remain advisory.

Loop effects are summarized once, and at most the first 64 quotient candidates
in one loop are evaluated. This keeps analyzer work bounded on generated code
while retaining bundled multi-quotient fixes for ordinary kernels.`,
		Before: `for d := range output {
	output[d] += gradient[d] * (weight / denominator)
}`,
		After: `quotient := weight / denominator // original IEEE division, rounded once
for d := range output {
	output[d] += gradient[d] * quotient
}`,
		MeasuredWin: `Owner issue #909 measured the exact cached quotient in the
typed MoE combine backward kernel on Apple M2 Pro. Nine order-alternated frozen
binary pairs improved F64 median latency from 13.416355 ms to 10.386168 ms
(1.292x, 8/9 wins) and F32 from 11.755059 ms to 10.988136 ms (1.070x, 7/9
wins), without an allocation-count regression. Serial and parallel mutation
tests remained bit-exact; the reciprocal-multiply alternative changed F64
output bits.

The repository's simple scalar benchmark was neutral under Go 1.27.0 on
darwin/arm64, Apple M2 Pro, GOMAXPROCS=3: six alternating fresh-process pairs
of two-second arms measured medians of 89.68 ns/op before and 89.405 ns/op
after, with zero bytes and allocations in both arms. Both compiled functions
already execute one division before the loop, so this microbenchmark proves
no additional speedup. Do not generalize the owner's indexed production gain
to scalar expressions that the compiler already optimizes.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6095",
		Doc:  "exact floating-point quotient recomputed per output element",
		Run:  runPS6095,
	},
})

type ps6095Helper struct {
	reciprocal bool
}

type ps6095Loop struct {
	statement  ast.Stmt
	body       *ast.BlockStmt
	index      types.Object
	condition  ast.Expr
	bound      map[types.Object]bool
	reentryEnd token.Pos
}

type ps6095Candidate struct {
	expression ast.Expr
	operands   []ast.Expr
	reciprocal bool
}

type ps6095Dependencies struct {
	objects map[types.Object]bool
	storage map[types.Object]bool
	scalar  bool
}

type ps6095Finding struct {
	candidate    ps6095Candidate
	dependencies ps6095Dependencies
}

type ps6095FreshInfo struct {
	makePosition token.Pos
	unsafeUse    token.Pos
}

// ps6095MaxCandidatesPerLoop bounds all candidate-dependent work. Candidate
// discovery and the loop effect summary remain linear in the loop body; the
// parent-chain and dependency checks below therefore have a fixed upper bound
// even for generated kernels containing thousands of divisions.
const ps6095MaxCandidatesPerLoop = 64

type ps6095LoopFacts struct {
	straight       bool
	written        map[types.Object]bool
	stored         map[types.Object]bool
	storageBarrier bool
}

// ps6095EffectIndex records writes once per function body. Loop-local queries
// are then logarithmic in the number of writes to an object, including writes
// in nested loops that direct loop scans deliberately do not revisit.
type ps6095EffectIndex struct {
	written map[types.Object][]token.Pos
}

type ps6095Stats struct {
	preparedNodes atomic.Uint64
	directNodes   atomic.Uint64
	functions     atomic.Uint64
	diagnostics   atomic.Uint64
}

type ps6095FunctionContext struct {
	body         *ast.BlockStmt
	ownerNames   []*ast.Ident
	parents      map[ast.Node]ast.Node
	nodes        []ast.Node
	loops        []ps6095Loop
	blocks       map[ast.Node]*cfg.Block
	cyclicBlocks map[*cfg.Block]bool
	effects      ps6095EffectIndex
	freshObjects map[types.Object]ps6095FreshInfo
	captured     map[types.Object]bool
	foreign      map[types.Object]bool
	usedNames    map[string]bool
	hasGoto      bool
	stats        *ps6095Stats
	comments     []*ast.CommentGroup
}

type ps6095TreeFacts struct {
	mutated     map[types.Object]bool
	unsafe      map[types.Object]bool
	sliceExpose map[types.Object]bool
}

func runPS6095(pass *analysis.Pass) (any, error) {
	return ps6095Run(pass, nil)
}

func ps6095Run(pass *analysis.Pass, stats *ps6095Stats) (any, error) {
	helpers := ps6095Helpers(pass)
	for _, file := range pass.Files {
		analyzeRoot := func(body *ast.BlockStmt, ownerNames []*ast.Ident) {
			var contexts []*ps6095FunctionContext
			ps6095PrepareContext(pass, file, body, ownerNames, stats, &contexts)
			facts := ps6095PrepareTreeFacts(pass, contexts)
			for _, context := range contexts {
				ps6095Function(pass, context, helpers, facts)
			}
		}
		for _, declaration := range file.Decls {
			switch declaration := declaration.(type) {
			case *ast.FuncDecl:
				if declaration.Body == nil {
					continue
				}
				ownerNames := ps6095FunctionOwnerNames(declaration)
				analyzeRoot(declaration.Body, ownerNames)
			case *ast.GenDecl:
				ast.Inspect(declaration, func(node ast.Node) bool {
					literal, ok := node.(*ast.FuncLit)
					if !ok {
						return true
					}
					ownerNames := ps6095FieldNames(literal.Type.Params, literal.Type.Results)
					analyzeRoot(literal.Body, ownerNames)
					return false
				})
			}
		}
	}
	return nil, nil
}

func ps6095FieldNames(lists ...*ast.FieldList) []*ast.Ident {
	var names []*ast.Ident
	for _, list := range lists {
		if list == nil {
			continue
		}
		for _, field := range list.List {
			names = append(names, field.Names...)
		}
	}
	return names
}

func ps6095FunctionOwnerNames(function *ast.FuncDecl) []*ast.Ident {
	// Function type parameters share the function body's declaration scope
	// with parameters and results. A generated short declaration at the top
	// of that body therefore cannot reuse one of their names. Generic receiver
	// type arguments similarly declare the receiver type parameters whose scope
	// includes the method body; unlike the receiver variable, those identifiers
	// are nested in the receiver type rather than stored in Field.Names.
	names := ps6095FieldNames(function.Recv, function.Type.TypeParams, function.Type.Params, function.Type.Results)
	if function.Recv == nil {
		return names
	}
	for _, field := range function.Recv.List {
		receiver := ps6095Unparen(field.Type)
		if pointer, ok := receiver.(*ast.StarExpr); ok {
			receiver = ps6095Unparen(pointer.X)
		}
		switch receiver := receiver.(type) {
		case *ast.IndexExpr:
			if identifier, ok := ps6095Unparen(receiver.Index).(*ast.Ident); ok {
				names = append(names, identifier)
			}
		case *ast.IndexListExpr:
			for _, argument := range receiver.Indices {
				if identifier, ok := ps6095Unparen(argument).(*ast.Ident); ok {
					names = append(names, identifier)
				}
			}
		}
	}
	return names
}

func ps6095Helpers(pass *analysis.Pass) map[*types.Func]ps6095Helper {
	helpers := make(map[*types.Func]ps6095Helper)
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Recv != nil || function.Body == nil || len(function.Body.List) != 1 {
				continue
			}
			object, ok := pass.TypesInfo.Defs[function.Name].(*types.Func)
			if !ok {
				continue
			}
			signature, ok := object.Type().(*types.Signature)
			if !ok || signature.Results().Len() != 1 || !ps6095Float(signature.Results().At(0).Type()) {
				continue
			}
			returned, ok := function.Body.List[0].(*ast.ReturnStmt)
			if !ok || len(returned.Results) != 1 {
				continue
			}
			division, ok := ps6095Unparen(returned.Results[0]).(*ast.BinaryExpr)
			if !ok || division.Op != token.QUO || !ps6095Float(pass.TypesInfo.TypeOf(division)) || pass.TypesInfo.Types[division].Value != nil {
				continue
			}
			parameters := make(map[types.Object]bool, signature.Params().Len())
			parametersSafe := true
			for index := range signature.Params().Len() {
				parameter := signature.Params().At(index)
				parameters[parameter] = true
				parametersSafe = parametersSafe && ps6095Numeric(parameter.Type())
			}
			used := make(map[types.Object]bool, len(parameters))
			if !parametersSafe || !ps6095HelperOperand(pass, division.X, parameters, used) ||
				!ps6095HelperOperand(pass, division.Y, parameters, used) || len(used) != len(parameters) {
				continue
			}
			helpers[object] = ps6095Helper{reciprocal: ps6095One(pass, division.X)}
		}
	}
	return helpers
}

func ps6095HelperOperand(pass *analysis.Pass, expression ast.Expr, parameters, used map[types.Object]bool) bool {
	switch value := ps6095Unparen(expression).(type) {
	case *ast.BasicLit:
		return true
	case *ast.Ident:
		object := identObject(pass, value)
		if object == nil {
			return false
		}
		if _, ok := object.(*types.Const); ok {
			return true
		}
		if parameters[object] {
			used[object] = true
			return true
		}
		return false
	case *ast.UnaryExpr:
		return (value.Op == token.ADD || value.Op == token.SUB) && ps6095HelperOperand(pass, value.X, parameters, used)
	case *ast.BinaryExpr:
		return value.Op != token.QUO && value.Op != token.REM && value.Op != token.SHL && value.Op != token.SHR &&
			ps6095HelperOperand(pass, value.X, parameters, used) && ps6095HelperOperand(pass, value.Y, parameters, used)
	case *ast.CallExpr:
		return len(value.Args) == 1 && !value.Ellipsis.IsValid() && pass.TypesInfo.Types[value.Fun].IsType() &&
			ps6095HelperOperand(pass, value.Args[0], parameters, used)
	}
	return false
}

func ps6095PrepareContext(
	pass *analysis.Pass,
	file *ast.File,
	body *ast.BlockStmt,
	ownerNames []*ast.Ident,
	stats *ps6095Stats,
	contexts *[]*ps6095FunctionContext,
) {
	context := &ps6095FunctionContext{
		body:         body,
		ownerNames:   ownerNames,
		parents:      make(map[ast.Node]ast.Node),
		effects:      ps6095EffectIndex{written: make(map[types.Object][]token.Pos)},
		freshObjects: make(map[types.Object]ps6095FreshInfo),
		captured:     make(map[types.Object]bool),
		foreign:      make(map[types.Object]bool),
		usedNames:    make(map[string]bool),
		stats:        stats,
		comments:     file.Comments,
	}
	*contexts = append(*contexts, context)
	if stats != nil {
		stats.functions.Add(1)
	}

	var stack []ast.Node
	var literals []*ast.FuncLit
	ast.Inspect(body, func(node ast.Node) bool {
		if node == nil {
			stack = stack[:len(stack)-1]
			return false
		}
		if len(stack) != 0 {
			context.parents[node] = stack[len(stack)-1]
		}
		context.nodes = append(context.nodes, node)
		if stats != nil {
			stats.preparedNodes.Add(1)
		}
		if literal, ok := node.(*ast.FuncLit); ok {
			literals = append(literals, literal)
			return false
		}
		stack = append(stack, node)
		return true
	})

	for _, name := range ownerNames {
		context.usedNames[name.Name] = true
	}
	for _, node := range context.nodes {
		if identifier, ok := node.(*ast.Ident); ok {
			context.usedNames[identifier.Name] = true
		}
		if branch, ok := node.(*ast.BranchStmt); ok && branch.Tok == token.GOTO {
			context.hasGoto = true
		}
		if loop, ok := ps6095LoopHeader(pass, node); ok {
			context.loops = append(context.loops, loop)
		}
		ps6095PrepareWrite(pass, context.effects, node)
		ps6095PrepareFreshDeclaration(pass, context.freshObjects, node)
	}
	loopByStatement := make(map[ast.Stmt]int, len(context.loops))
	for index := range context.loops {
		loopByStatement[context.loops[index].statement] = index
	}
	var enclosingEnds []token.Pos
	for _, node := range context.nodes {
		for len(enclosingEnds) != 0 && enclosingEnds[len(enclosingEnds)-1] <= node.Pos() {
			enclosingEnds = enclosingEnds[:len(enclosingEnds)-1]
		}
		statement, nested := node.(ast.Stmt)
		if !nested {
			continue
		}
		switch statement.(type) {
		case *ast.ForStmt, *ast.RangeStmt:
			if index, canonical := loopByStatement[statement]; canonical {
				context.loops[index].reentryEnd = statement.End()
				if len(enclosingEnds) != 0 {
					context.loops[index].reentryEnd = enclosingEnds[0]
				}
			}
			enclosingEnds = append(enclosingEnds, statement.End())
		}
	}
	for _, node := range context.nodes {
		identifier, ok := node.(*ast.Ident)
		if !ok {
			continue
		}
		object := identObject(pass, identifier)
		info, ok := context.freshObjects[object]
		if !ok || ps6095SafeFreshUse(pass, context.parents, identifier, object) {
			continue
		}
		if info.unsafeUse == token.NoPos || identifier.Pos() < info.unsafeUse {
			info.unsafeUse = identifier.Pos()
			context.freshObjects[object] = info
		}
	}
	if context.hasGoto {
		// A backward goto can re-enter a syntactically completed loop after a
		// later aliasing use. Source-order ancestry cannot prove freshness.
		context.freshObjects = nil
	}

	graph := cfg.New(body, func(call *ast.CallExpr) bool { return !ps6088NonreturnCall(pass, call) })
	blocks := make(map[ast.Node]*cfg.Block)
	for _, block := range graph.Blocks {
		for _, node := range block.Nodes {
			blocks[node] = block
		}
	}
	context.blocks = blocks
	context.cyclicBlocks = ps6095CyclicBlocks(graph.Blocks)

	for _, literal := range literals {
		childNames := ps6095FieldNames(literal.Type.Params, literal.Type.Results)
		ps6095PrepareContext(pass, file, literal.Body, childNames, stats, contexts)
	}
}

func ps6095PrepareWrite(pass *analysis.Pass, effects ps6095EffectIndex, node ast.Node) {
	record := func(expression ast.Expr) {
		identifier, ok := ps6095Unparen(expression).(*ast.Ident)
		if !ok {
			return
		}
		if object := identObject(pass, identifier); object != nil {
			effects.written[object] = append(effects.written[object], identifier.Pos())
		}
	}
	switch value := node.(type) {
	case *ast.AssignStmt:
		for _, left := range value.Lhs {
			record(left)
		}
	case *ast.IncDecStmt:
		record(value.X)
	case *ast.RangeStmt:
		if value.Tok == token.ASSIGN {
			record(value.Key)
			record(value.Value)
		}
	}
}

func ps6095PrepareFreshDeclaration(pass *analysis.Pass, fresh map[types.Object]ps6095FreshInfo, node ast.Node) {
	assignment, ok := node.(*ast.AssignStmt)
	if !ok || assignment.Tok != token.DEFINE || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
		return
	}
	identifier, ok := assignment.Lhs[0].(*ast.Ident)
	if !ok {
		return
	}
	object := pass.TypesInfo.Defs[identifier]
	call, ok := ps6095Unparen(assignment.Rhs[0]).(*ast.CallExpr)
	if object == nil || !ok || len(call.Args) == 0 {
		return
	}
	builtin, ok := ps6095Unparen(call.Fun).(*ast.Ident)
	if !ok || builtin.Name != "make" {
		return
	}
	if _, ok := identObject(pass, builtin).(*types.Builtin); !ok {
		return
	}
	if _, ok := types.Unalias(object.Type()).Underlying().(*types.Slice); ok {
		fresh[object] = ps6095FreshInfo{makePosition: assignment.Pos()}
	}
}

func ps6095SafeFreshUse(pass *analysis.Pass, parents map[ast.Node]ast.Node, identifier *ast.Ident, object types.Object) bool {
	parent := parents[identifier]
	switch value := parent.(type) {
	case *ast.IndexExpr:
		if value.X != identifier {
			return false
		}
		var node ast.Node = value
		for {
			parent = parents[node]
			parenthesized, ok := parent.(*ast.ParenExpr)
			if !ok || parenthesized.X != node {
				break
			}
			node = parenthesized
		}
		unary, addressed := parent.(*ast.UnaryExpr)
		return !addressed || unary.Op != token.AND || unary.X != node
	case *ast.RangeStmt:
		return value.X == identifier
	case *ast.CallExpr:
		function, ok := ps6095Unparen(value.Fun).(*ast.Ident)
		if !ok {
			return false
		}
		builtin, ok := identObject(pass, function).(*types.Builtin)
		return ok && (builtin.Name() == "len" || builtin.Name() == "cap")
	case *ast.AssignStmt:
		for _, left := range value.Lhs {
			if left == identifier && pass.TypesInfo.Defs[identifier] == object {
				return true
			}
		}
	}
	return false
}

type ps6095Interval struct {
	start token.Pos
	end   token.Pos
}

func ps6095PrepareTreeFacts(pass *analysis.Pass, contexts []*ps6095FunctionContext) ps6095TreeFacts {
	facts := ps6095TreeFacts{
		mutated:     make(map[types.Object]bool),
		unsafe:      make(map[types.Object]bool),
		sliceExpose: make(map[types.Object]bool),
	}
	owners := make(map[types.Object]*ps6095FunctionContext)
	canonicalPosts := make(map[ast.Node]bool)
	for _, context := range contexts {
		for _, loop := range context.loops {
			if counted, ok := loop.statement.(*ast.ForStmt); ok {
				canonicalPosts[counted.Post] = true
			}
		}
		for _, identifier := range context.ownerNames {
			if object := pass.TypesInfo.Defs[identifier]; object != nil {
				owners[object] = context
			}
		}
		for _, node := range context.nodes {
			if object := ps6095OwnedImplicit(pass, node); object != nil {
				owners[object] = context
			}
			identifier, ok := node.(*ast.Ident)
			if !ok {
				continue
			}
			if object := pass.TypesInfo.Defs[identifier]; object != nil {
				owners[object] = context
			}
		}
	}

	recordMutation := func(expression ast.Expr) {
		identifier, ok := ps6095Unparen(expression).(*ast.Ident)
		if !ok {
			return
		}
		if object := pass.TypesInfo.Uses[identifier]; object != nil {
			facts.mutated[object] = true
		}
	}
	for _, context := range contexts {
		var intervals, sliceIntervals []ps6095Interval
		for _, node := range context.nodes {
			switch value := node.(type) {
			case *ast.AssignStmt:
				for _, left := range value.Lhs {
					recordMutation(left)
				}
			case *ast.IncDecStmt:
				if !canonicalPosts[value] {
					recordMutation(value.X)
				}
			case *ast.RangeStmt:
				if value.Tok == token.ASSIGN {
					recordMutation(value.Key)
					recordMutation(value.Value)
				}
			case *ast.UnaryExpr:
				if value.Op == token.AND {
					intervals = ps6095AppendInterval(intervals, value.X.Pos(), value.X.End())
				}
			case *ast.SelectorExpr:
				if ps6095PointerReceiverUses(pass, value) {
					intervals = ps6095AppendInterval(intervals, value.X.Pos(), value.X.End())
				}
			case *ast.SliceExpr:
				// Slicing an addressable array element implicitly exposes its
				// storage. Keep this separate from general unsafe-object facts:
				// scalar index/bound objects inside X are not themselves unsafe.
				sliceIntervals = ps6095AppendInterval(sliceIntervals, value.X.Pos(), value.X.End())
			}
			identifier, ok := node.(*ast.Ident)
			if !ok {
				continue
			}
			object := pass.TypesInfo.Uses[identifier]
			if owner := owners[object]; object != nil && owner != nil && owner != context {
				owner.captured[object] = true
				context.foreign[object] = true
			}
		}

		interval, sliceInterval := 0, 0
		for _, node := range context.nodes {
			identifier, ok := node.(*ast.Ident)
			if !ok {
				continue
			}
			for interval < len(intervals) && intervals[interval].end <= identifier.Pos() {
				interval++
			}
			if interval < len(intervals) && intervals[interval].start <= identifier.Pos() && identifier.End() <= intervals[interval].end {
				if object := identObject(pass, identifier); object != nil {
					facts.unsafe[object] = true
				}
			}
			for sliceInterval < len(sliceIntervals) && sliceIntervals[sliceInterval].end <= identifier.Pos() {
				sliceInterval++
			}
			if sliceInterval < len(sliceIntervals) && sliceIntervals[sliceInterval].start <= identifier.Pos() && identifier.End() <= sliceIntervals[sliceInterval].end {
				if object := identObject(pass, identifier); object != nil {
					facts.sliceExpose[object] = true
				}
			}
		}
	}
	for _, context := range contexts {
		for object := range context.freshObjects {
			if context.captured[object] || facts.unsafe[object] || facts.sliceExpose[object] {
				// A captured slice header may be returned, stored, or rebound by
				// code whose dynamic order is outside the declaring context's CFG.
				// Explicit address-taking and an implicit address through a
				// pointer-receiver selection can expose the backing array too.
				// Slicing an array descendant exposes the same storage. These
				// precomputed tree facts invalidate freshness globally.
				delete(context.freshObjects, object)
			}
		}
	}
	return facts
}

func ps6095OwnedImplicit(pass *analysis.Pass, node ast.Node) types.Object {
	// Info.Implicits also contains PkgNames for unrenamed imports and Vars for
	// anonymous parameters/results. Imports are package-scope noise here, while
	// anonymous fields have no identifier that a closure could reference. The
	// type-specific case Var is the only referencable mutable implicit object.
	if _, ok := node.(*ast.CaseClause); !ok {
		return nil
	}
	variable, _ := pass.TypesInfo.Implicits[node].(*types.Var)
	return variable
}

func ps6095AppendInterval(intervals []ps6095Interval, start, end token.Pos) []ps6095Interval {
	if len(intervals) != 0 && start <= intervals[len(intervals)-1].end {
		intervals[len(intervals)-1].end = max(intervals[len(intervals)-1].end, end)
		return intervals
	}
	return append(intervals, ps6095Interval{start: start, end: end})
}

func ps6095Function(pass *analysis.Pass, context *ps6095FunctionContext, helpers map[*types.Func]ps6095Helper, tree ps6095TreeFacts) {
	for _, loop := range context.loops {
		ps6095InspectLoop(pass, context, loop, helpers, tree)
	}
}

func (effects ps6095EffectIndex) writesIn(loop ps6095Loop, object types.Object) bool {
	positions := effects.written[object]
	first := sort.Search(len(positions), func(index int) bool {
		return positions[index] >= loop.body.Pos()
	})
	return first < len(positions) && positions[first] < loop.body.End()
}

// ps6095InspectDirect visits a loop body's own statements and expressions but
// does not descend into nested loops or function literals. Across all loops in
// a function, each ordinary AST node is therefore visited only at its nearest
// loop level instead of once per enclosing loop.
func ps6095InspectDirect(root ast.Node, inspect func(ast.Node) bool) {
	ps6095InspectDirectStats(root, nil, inspect)
}

func ps6095InspectDirectStats(root ast.Node, stats *ps6095Stats, inspect func(ast.Node) bool) {
	ast.Inspect(root, func(node ast.Node) bool {
		if node == nil {
			return false
		}
		if stats != nil {
			stats.directNodes.Add(1)
		}
		descend := inspect(node)
		if node != root {
			switch node.(type) {
			case *ast.FuncLit, *ast.ForStmt, *ast.RangeStmt:
				return false
			}
		}
		return descend
	})
}

func ps6095LoopHeader(pass *analysis.Pass, node ast.Node) (ps6095Loop, bool) {
	switch loop := node.(type) {
	case *ast.RangeStmt:
		identifier, ok := loop.Key.(*ast.Ident)
		if !ok || identifier.Name == "_" || loop.Value != nil {
			return ps6095Loop{}, false
		}
		index := pass.TypesInfo.Defs[identifier]
		if index == nil || !ps6095MaterialRange(pass, loop.X) {
			return ps6095Loop{}, false
		}
		return ps6095Loop{statement: loop, body: loop.Body, index: index}, true
	case *ast.ForStmt:
		initialization, ok := loop.Init.(*ast.AssignStmt)
		if !ok || initialization.Tok != token.DEFINE || len(initialization.Lhs) != 1 || len(initialization.Rhs) != 1 {
			return ps6095Loop{}, false
		}
		identifier, ok := initialization.Lhs[0].(*ast.Ident)
		if !ok || !ps6095Zero(pass, initialization.Rhs[0]) {
			return ps6095Loop{}, false
		}
		index := pass.TypesInfo.Defs[identifier]
		condition, ok := loop.Cond.(*ast.BinaryExpr)
		if index == nil || !ok || condition.Op != token.LSS || !ps6095Object(pass, condition.X, index) || ps6095Mentions(pass, condition.Y, index) {
			return ps6095Loop{}, false
		}
		post, ok := loop.Post.(*ast.IncDecStmt)
		if !ok || post.Tok != token.INC || !ps6095Object(pass, post.X, index) || !ps6095MaterialBound(pass, condition.Y) {
			return ps6095Loop{}, false
		}
		bound, ok := ps6095BoundObjects(pass, condition.Y)
		if !ok {
			return ps6095Loop{}, false
		}
		return ps6095Loop{statement: loop, body: loop.Body, index: index, condition: loop.Cond, bound: bound}, true
	}
	return ps6095Loop{}, false
}

func ps6095BoundObjects(pass *analysis.Pass, expression ast.Expr) (map[types.Object]bool, bool) {
	objects := make(map[types.Object]bool)
	var collect func(ast.Expr) bool
	collect = func(expression ast.Expr) bool {
		switch value := ps6095Unparen(expression).(type) {
		case *ast.BasicLit:
			return true
		case *ast.Ident:
			object := identObject(pass, value)
			if object == nil {
				return false
			}
			switch object.(type) {
			case *types.Const, *types.Builtin:
				return true
			case *types.Var:
				if object.Pkg() != nil && object.Parent() == object.Pkg().Scope() {
					return false
				}
				objects[object] = true
				return true
			}
			return false
		case *ast.UnaryExpr:
			return value.Op != token.ARROW && value.Op != token.AND && collect(value.X)
		case *ast.BinaryExpr:
			return collect(value.X) && collect(value.Y)
		case *ast.CallExpr:
			if len(value.Args) != 1 || value.Ellipsis.IsValid() {
				return false
			}
			if pass.TypesInfo.Types[value.Fun].IsType() {
				return collect(value.Args[0])
			}
			identifier, ok := ps6095Unparen(value.Fun).(*ast.Ident)
			if !ok {
				return false
			}
			builtin, ok := identObject(pass, identifier).(*types.Builtin)
			return ok && (builtin.Name() == "len" || builtin.Name() == "cap") && collect(value.Args[0])
		}
		return false
	}
	return objects, collect(expression)
}

func ps6095MaterialRange(pass *analysis.Pass, expression ast.Expr) bool {
	if value := pass.TypesInfo.Types[expression].Value; value != nil {
		if value.Kind() == constant.Int {
			bound, ok := constant.Int64Val(value)
			return ok && bound >= 4
		}
		return false
	}
	typeOf := pass.TypesInfo.TypeOf(expression)
	if typeOf == nil {
		return false
	}
	typeOf = types.Unalias(typeOf)
	switch underlying := typeOf.Underlying().(type) {
	case *types.Array:
		return underlying.Len() >= 4
	case *types.Slice, *types.Basic:
		return true
	}
	return false
}

func ps6095MaterialBound(pass *analysis.Pass, expression ast.Expr) bool {
	value := pass.TypesInfo.Types[expression].Value
	if value == nil || value.Kind() != constant.Int {
		return true
	}
	bound, ok := constant.Int64Val(value)
	return ok && bound >= 4
}

func ps6095Zero(pass *analysis.Pass, expression ast.Expr) bool {
	value := pass.TypesInfo.Types[expression].Value
	return value != nil && value.Kind() == constant.Int && constant.Sign(value) == 0
}

func ps6095One(pass *analysis.Pass, expression ast.Expr) bool {
	value := pass.TypesInfo.Types[expression].Value
	return value != nil && constant.Compare(value, token.EQL, constant.MakeInt64(1))
}

func ps6095InspectLoop(
	pass *analysis.Pass,
	context *ps6095FunctionContext,
	loop ps6095Loop,
	helpers map[*types.Func]ps6095Helper,
	tree ps6095TreeFacts,
) {
	var candidates []ps6095Candidate
	ps6095InspectDirectStats(loop.body, context.stats, func(node ast.Node) bool {
		expression, ok := node.(ast.Expr)
		if !ok {
			return true
		}
		if len(candidates) >= ps6095MaxCandidatesPerLoop {
			return false
		}
		if division, ok := ps6095Unparen(expression).(*ast.BinaryExpr); ok && division.Op == token.QUO && ps6095Float(pass.TypesInfo.TypeOf(division)) && pass.TypesInfo.Types[division].Value == nil {
			ps6095AddCandidate(&candidates, ps6095Candidate{expression: expression, operands: []ast.Expr{division.X, division.Y}, reciprocal: ps6095One(pass, division.X)})
			return false
		}
		call, ok := ps6095Unparen(expression).(*ast.CallExpr)
		if !ok || call.Ellipsis.IsValid() {
			return true
		}
		function, _, ok := typedCallee(pass, call.Fun)
		if !ok {
			return true
		}
		if helper, ok := helpers[function]; ok && ps6095Float(pass.TypesInfo.TypeOf(call)) {
			ps6095AddCandidate(&candidates, ps6095Candidate{expression: expression, operands: call.Args, reciprocal: helper.reciprocal})
			return false
		}
		return true
	})
	if len(candidates) == 0 {
		return
	}
	facts := ps6095SummarizeLoop(pass, loop, helpers, context.freshObjects, context.stats)
	// The canonical post is intentionally outside loop.body. Any write to the
	// same induction object inside the body (including a nested-loop post), or
	// any address-taking/pointer-receiver escape, can make the candidate execute
	// fewer times than the material loop bound and invalidates both report/fix.
	if facts.written[loop.index] || context.effects.writesIn(loop, loop.index) || ps6095Unstable(context, tree, loop.index) {
		return
	}
	for object := range loop.bound {
		if facts.written[object] || context.effects.writesIn(loop, object) || ps6095Unstable(context, tree, object) {
			return
		}
	}
	var findings []ps6095Finding
	for _, candidate := range candidates {
		if !ps6095Repeated(context.parents, context.blocks, context.cyclicBlocks, candidate.expression) || !ps6095OutputUse(pass, candidate.expression, loop, context.parents) ||
			!facts.straight || !ps6095Unconditional(candidate.expression, loop, context.parents) ||
			candidate.reciprocal && ps6095Multiplied(pass, candidate.expression, loop, context.parents) {
			continue
		}
		dependencies, ok := ps6095CollectDependencies(pass, candidate.operands)
		if !ok || len(dependencies.objects) == 0 || dependencies.objects[loop.index] || !ps6095InputsPrecedeLoop(loop, dependencies) {
			continue
		}
		if !ps6095Stable(context, tree, loop, facts, dependencies) {
			continue
		}
		findings = append(findings, ps6095Finding{candidate: candidate, dependencies: dependencies})
	}
	if len(findings) == 0 {
		return
	}
	selected := findings[0]
	var fixable []ps6095Candidate
	if !context.hasGoto {
		for _, finding := range findings {
			if finding.dependencies.scalar && !ps6095CommentsOverlap(context.comments, finding.candidate.expression.Pos(), finding.candidate.expression.End()) {
				if len(fixable) == 0 {
					selected = finding
				}
				fixable = append(fixable, finding.candidate)
			}
		}
	}
	diagnostic := analysis.Diagnostic{
		Pos:     selected.candidate.expression.Pos(),
		End:     selected.candidate.expression.End(),
		Message: "this floating-point quotient is invariant in output index " + loop.index.Name() + "; compute the original division once and reuse its rounded result (never replace it with reciprocal multiplication, which is not bit-equivalent); the compiler may already hoist this quotient, so inspect optimized code and benchmark the caller before claiming a speedup",
	}
	if fix := ps6095Fix(pass, context.parents, context.usedNames, context.hasGoto, context.comments, loop, fixable); fix != nil {
		diagnostic.SuggestedFixes = []analysis.SuggestedFix{*fix}
	}
	if context.stats != nil {
		context.stats.diagnostics.Add(1)
	}
	pass.Report(diagnostic)
}

func ps6095Unstable(context *ps6095FunctionContext, tree ps6095TreeFacts, object types.Object) bool {
	return tree.unsafe[object] || context.captured[object] || context.foreign[object] && tree.mutated[object]
}

func ps6095AddCandidate(candidates *[]ps6095Candidate, candidate ps6095Candidate) bool {
	if len(*candidates) >= ps6095MaxCandidatesPerLoop {
		return false
	}
	*candidates = append(*candidates, candidate)
	return true
}

func ps6095CollectDependencies(pass *analysis.Pass, operands []ast.Expr) (ps6095Dependencies, bool) {
	dependencies := ps6095Dependencies{
		objects: make(map[types.Object]bool),
		storage: make(map[types.Object]bool),
		scalar:  true,
	}
	for _, operand := range operands {
		if !ps6095CollectExpression(pass, operand, &dependencies) {
			return ps6095Dependencies{}, false
		}
	}
	return dependencies, true
}

func ps6095CollectExpression(pass *analysis.Pass, expression ast.Expr, dependencies *ps6095Dependencies) bool {
	switch value := ps6095Unparen(expression).(type) {
	case *ast.BasicLit:
		return true
	case *ast.Ident:
		object := identObject(pass, value)
		if object == nil {
			return false
		}
		switch object.(type) {
		case *types.Const, *types.Builtin, *types.TypeName:
			return true
		case *types.Var:
			if object.Pkg() != nil && object.Parent() == object.Pkg().Scope() {
				return false
			}
			dependencies.objects[object] = true
			if !ps6095Numeric(object.Type()) {
				dependencies.scalar = false
			}
			return true
		}
		return false
	case *ast.UnaryExpr:
		return (value.Op == token.ADD || value.Op == token.SUB || value.Op == token.XOR || value.Op == token.NOT) &&
			ps6095CollectExpression(pass, value.X, dependencies)
	case *ast.BinaryExpr:
		if value.Op == token.SHL || value.Op == token.SHR ||
			(value.Op == token.QUO || value.Op == token.REM) && ps6095Integer(pass.TypesInfo.TypeOf(value)) {
			return false
		}
		return ps6095CollectExpression(pass, value.X, dependencies) && ps6095CollectExpression(pass, value.Y, dependencies)
	case *ast.IndexExpr:
		root, ok := value.X.(*ast.Ident)
		if !ok {
			return false
		}
		object := identObject(pass, root)
		if object == nil || object.Pkg() != nil && object.Parent() == object.Pkg().Scope() {
			return false
		}
		switch types.Unalias(object.Type()).Underlying().(type) {
		case *types.Slice, *types.Array:
		default:
			return false
		}
		dependencies.objects[object] = true
		dependencies.storage[object] = true
		dependencies.scalar = false
		return ps6095CollectExpression(pass, value.Index, dependencies)
	case *ast.CallExpr:
		return len(value.Args) == 1 && !value.Ellipsis.IsValid() && pass.TypesInfo.Types[value.Fun].IsType() &&
			ps6095CollectExpression(pass, value.Args[0], dependencies)
	}
	return false
}

func ps6095InputsPrecedeLoop(loop ps6095Loop, dependencies ps6095Dependencies) bool {
	for object := range dependencies.objects {
		if object.Pos() >= loop.statement.Pos() {
			return false
		}
	}
	return true
}

func ps6095Stable(context *ps6095FunctionContext, tree ps6095TreeFacts, loop ps6095Loop, facts ps6095LoopFacts, dependencies ps6095Dependencies) bool {
	for object := range dependencies.objects {
		if ps6095Unstable(context, tree, object) || facts.written[object] || context.effects.writesIn(loop, object) {
			return false
		}
		if dependencies.storage[object] && context.foreign[object] {
			// A captured slice/array can have aliases and mutators in sibling
			// closures whose dynamic call order is outside this literal's CFG.
			return false
		}
	}
	if len(dependencies.storage) == 0 {
		return true
	}
	if facts.storageBarrier {
		return false
	}
	for object := range dependencies.storage {
		if facts.stored[object] {
			return false
		}
	}
	return true
}

// ps6095SummarizeLoop scans loop effects exactly once. Findings can then be
// checked by object-set intersection instead of rescanning the body for every
// division. Unknown storage effects deliberately poison all storage-backed
// candidates; scalar numeric values remain protected by unsafeObjects and
// direct-write tracking.
func ps6095SummarizeLoop(
	pass *analysis.Pass,
	loop ps6095Loop,
	helpers map[*types.Func]ps6095Helper,
	freshObjects map[types.Object]ps6095FreshInfo,
	stats *ps6095Stats,
) ps6095LoopFacts {
	facts := ps6095LoopFacts{
		straight: true,
		written:  make(map[types.Object]bool),
		stored:   make(map[types.Object]bool),
	}
	straightDependencies := map[types.Object]bool{loop.index: true}
	ps6095InspectDirectStats(loop.body, stats, func(node ast.Node) bool {
		if !facts.straight || node == nil {
			return false
		}
		if node != loop.body {
			switch value := node.(type) {
			case *ast.FuncLit, *ast.ForStmt, *ast.RangeStmt:
				return true
			case *ast.BranchStmt, *ast.ReturnStmt:
				facts.straight = false
				return false
			case *ast.AssignStmt:
				for _, left := range value.Lhs {
					if ps6095WritesDependency(pass, left, straightDependencies) {
						facts.straight = false
						return false
					}
				}
			case *ast.IncDecStmt:
				if ps6095WritesDependency(pass, value.X, straightDependencies) {
					facts.straight = false
					return false
				}
			}
		}
		return true
	})

	inspectEffects := func(root ast.Node) {
		ps6095InspectDirectStats(root, stats, func(node ast.Node) bool {
			switch value := node.(type) {
			case *ast.ForStmt, *ast.RangeStmt:
				if node != root {
					// Raw writes are retained by the per-function effect index.
					// Storage provenance is loop-relative, so conservatively block
					// storage-backed outer findings rather than rescan the subtree.
					facts.storageBarrier = true
				}
			case *ast.AssignStmt:
				for _, left := range value.Lhs {
					ps6095RecordWrite(pass, left, facts.written)
					ps6095RecordStore(pass, left, loop, freshObjects, &facts)
				}
			case *ast.IncDecStmt:
				ps6095RecordWrite(pass, value.X, facts.written)
				ps6095RecordStore(pass, value.X, loop, freshObjects, &facts)
			case *ast.SendStmt:
				facts.storageBarrier = true
			case *ast.UnaryExpr:
				if value.Op == token.ARROW {
					facts.storageBarrier = true
				}
			case *ast.CallExpr:
				if !ps6095CallSafe(pass, value, helpers) {
					facts.storageBarrier = true
				}
			}
			return true
		})
	}
	inspectEffects(loop.body)
	if loop.condition != nil {
		inspectEffects(loop.condition)
	}
	return facts
}

func ps6095RecordWrite(pass *analysis.Pass, expression ast.Expr, written map[types.Object]bool) {
	identifier, ok := ps6095Unparen(expression).(*ast.Ident)
	if !ok {
		return
	}
	if object := identObject(pass, identifier); object != nil {
		written[object] = true
	}
}

func ps6095RecordStore(
	pass *analysis.Pass,
	expression ast.Expr,
	loop ps6095Loop,
	fresh map[types.Object]ps6095FreshInfo,
	facts *ps6095LoopFacts,
) {
	if _, ok := ps6095Unparen(expression).(*ast.Ident); ok {
		return
	}
	indexed, ok := ps6095Unparen(expression).(*ast.IndexExpr)
	if !ok {
		facts.storageBarrier = true
		return
	}
	root, ok := ps6095Unparen(indexed.X).(*ast.Ident)
	if !ok {
		facts.storageBarrier = true
		return
	}
	object := identObject(pass, root)
	info, ok := fresh[object]
	if object == nil || !ok || !ps6095FreshForLoop(info, loop) {
		facts.storageBarrier = true
		return
	}
	facts.stored[object] = true
}

// ps6095FreshForLoop requires the make to dominate the loop and rejects an
// unsafe use before the precomputed end of its outermost re-entry region. An
// assignment lexically after an inner loop can execute before that inner
// loop's next dynamic entry, so the inner loop's End alone is insufficient.
func ps6095FreshForLoop(info ps6095FreshInfo, loop ps6095Loop) bool {
	if info.makePosition >= loop.statement.Pos() {
		return false
	}
	if info.unsafeUse == token.NoPos {
		return true
	}
	return info.unsafeUse >= loop.reentryEnd
}

func ps6095CallSafe(pass *analysis.Pass, call *ast.CallExpr, helpers map[*types.Func]ps6095Helper) bool {
	if pass.TypesInfo.Types[call.Fun].IsType() {
		return true
	}
	if function, _, ok := typedCallee(pass, call.Fun); ok {
		if _, pure := helpers[function]; pure {
			return true
		}
	}
	if identifier, ok := ps6095Unparen(call.Fun).(*ast.Ident); ok {
		if builtin, ok := identObject(pass, identifier).(*types.Builtin); ok && (builtin.Name() == "len" || builtin.Name() == "cap") {
			return true
		}
	}
	return false
}

func ps6095Unconditional(candidate ast.Expr, loop ps6095Loop, parents map[ast.Node]ast.Node) bool {
	for node := ast.Node(candidate); node != nil && node != loop.body; node = parents[node] {
		switch parent := parents[node].(type) {
		case *ast.IfStmt, *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt, *ast.CaseClause, *ast.CommClause:
			return false
		case *ast.BinaryExpr:
			if (parent.Op == token.LAND || parent.Op == token.LOR) && node == parent.Y {
				return false
			}
		case *ast.BlockStmt:
			if parent != loop.body {
				return false
			}
		}
	}
	return true
}

func ps6095PointerReceiverUses(pass *analysis.Pass, expression ast.Expr) bool {
	selector, ok := ps6095Unparen(expression).(*ast.SelectorExpr)
	if !ok {
		return false
	}
	selection := pass.TypesInfo.Selections[selector]
	if selection == nil {
		return false
	}
	function, ok := selection.Obj().(*types.Func)
	if !ok {
		return false
	}
	signature, ok := function.Type().(*types.Signature)
	if !ok || signature.Recv() == nil {
		return false
	}
	if _, pointer := types.Unalias(signature.Recv().Type()).(*types.Pointer); !pointer {
		return false
	}
	return true
}

func ps6095WritesDependency(pass *analysis.Pass, expression ast.Expr, dependencies map[types.Object]bool) bool {
	identifier, ok := ps6095Unparen(expression).(*ast.Ident)
	return ok && dependencies[identObject(pass, identifier)]
}

func ps6095OutputUse(pass *analysis.Pass, candidate ast.Expr, loop ps6095Loop, parents map[ast.Node]ast.Node) bool {
	productDependsOnOutput := false
	for node := ast.Node(candidate); node != nil && node != loop.body; node = parents[node] {
		switch value := parents[node].(type) {
		case *ast.BinaryExpr:
			if value.Op == token.MUL {
				other := value.X
				if node == value.X {
					other = value.Y
				}
				if ps6095Mentions(pass, other, loop.index) {
					productDependsOnOutput = true
				}
			}
		case *ast.AssignStmt:
			if len(value.Lhs) != 1 || len(value.Rhs) != 1 {
				return false
			}
			for _, left := range value.Lhs {
				if ps6095IndexedBy(pass, left, loop.index) {
					return true
				}
			}
			return productDependsOnOutput && (value.Tok == token.ADD_ASSIGN || value.Tok == token.SUB_ASSIGN || value.Tok == token.ASSIGN || value.Tok == token.DEFINE)
		}
	}
	return false
}

func ps6095Multiplied(pass *analysis.Pass, candidate ast.Expr, loop ps6095Loop, parents map[ast.Node]ast.Node) bool {
	for node := ast.Node(candidate); node != nil && node != loop.body; node = parents[node] {
		switch parent := parents[node].(type) {
		case *ast.BinaryExpr:
			return parent.Op == token.MUL
		case *ast.ParenExpr:
			continue
		case *ast.UnaryExpr:
			if (parent.Op == token.ADD || parent.Op == token.SUB) && parent.X == node {
				continue
			}
			return false
		case *ast.CallExpr:
			if len(parent.Args) == 1 && parent.Args[0] == node && !parent.Ellipsis.IsValid() && pass.TypesInfo.Types[parent.Fun].IsType() {
				continue
			}
			return false
		default:
			return false
		}
	}
	return false
}

func ps6095IndexedBy(pass *analysis.Pass, expression ast.Expr, index types.Object) bool {
	found := false
	ast.Inspect(expression, func(node ast.Node) bool {
		indexed, ok := node.(*ast.IndexExpr)
		if ok && ps6095Mentions(pass, indexed.Index, index) {
			found = true
			return false
		}
		return !found
	})
	return found
}

func ps6095CommentsOverlap(comments []*ast.CommentGroup, pos, end token.Pos) bool {
	first := sort.Search(len(comments), func(index int) bool {
		return comments[index].End() > pos
	})
	return first < len(comments) && comments[first].Pos() < end
}

func ps6095Fix(pass *analysis.Pass, parents map[ast.Node]ast.Node, used map[string]bool, hasGoto bool, comments []*ast.CommentGroup, loop ps6095Loop, candidates []ps6095Candidate) *analysis.SuggestedFix {
	if len(candidates) == 0 {
		return nil
	}
	if _, labeled := parents[loop.statement].(*ast.LabeledStmt); labeled || hasGoto {
		return nil
	}
	position := pass.Fset.Position(loop.statement.Pos())
	indent := strings.Repeat("\t", max(position.Column-1, 0))
	var declaration strings.Builder
	declaration.Grow(len(candidates) * (64 + len(indent)))
	edits := make([]analysis.TextEdit, 0, len(candidates)+1)
	for _, candidate := range candidates {
		if ps6095CommentsOverlap(comments, candidate.expression.Pos(), candidate.expression.End()) {
			return nil
		}
		var rendered bytes.Buffer
		if err := format.Node(&rendered, pass.Fset, ps6095Unparen(candidate.expression)); err != nil {
			return nil
		}
		candidatePosition := pass.Fset.Position(candidate.expression.Pos())
		base := "psQuotient" + strconv.Itoa(candidatePosition.Line) + "_" + strconv.Itoa(candidatePosition.Column)
		name := base
		for suffix := 2; used[name]; suffix++ {
			name = base + "_" + strconv.Itoa(suffix)
		}
		used[name] = true
		declaration.WriteString(name)
		declaration.WriteString(" := ")
		declaration.WriteString(rendered.String())
		declaration.WriteByte('\n')
		declaration.WriteString(indent)
		edits = append(edits, analysis.TextEdit{Pos: candidate.expression.Pos(), End: candidate.expression.End(), NewText: []byte(name)})
	}
	edits = append([]analysis.TextEdit{{Pos: loop.statement.Pos(), End: loop.statement.Pos(), NewText: []byte(declaration.String())}}, edits...)
	return &analysis.SuggestedFix{
		Message:   "compute each original floating-point division once and reuse its rounded result",
		TextEdits: edits,
	}
}

func ps6095Repeated(parents map[ast.Node]ast.Node, blocks map[ast.Node]*cfg.Block, cyclicBlocks map[*cfg.Block]bool, expression ast.Expr) bool {
	var candidate *cfg.Block
	for node := ast.Node(expression); node != nil; node = parents[node] {
		if block := blocks[node]; block != nil {
			candidate = block
			break
		}
	}
	return candidate != nil && candidate.Live && cyclicBlocks[candidate]
}

// ps6095CyclicBlocks classifies all live CFG blocks once per function. A
// candidate repeats exactly when its block belongs to a non-trivial strongly
// connected component (or has a self-edge). This keeps functions containing
// many sequential loops linear instead of searching the remaining CFG once
// per quotient.
func ps6095CyclicBlocks(blocks []*cfg.Block) map[*cfg.Block]bool {
	nextIndex := 1
	indices := make(map[*cfg.Block]int, len(blocks))
	lowlinks := make(map[*cfg.Block]int, len(blocks))
	onStack := make(map[*cfg.Block]bool, len(blocks))
	stack := make([]*cfg.Block, 0, len(blocks))
	cyclic := make(map[*cfg.Block]bool)

	var visit func(*cfg.Block)
	visit = func(block *cfg.Block) {
		indices[block] = nextIndex
		lowlinks[block] = nextIndex
		nextIndex++
		stack = append(stack, block)
		onStack[block] = true

		for _, successor := range block.Succs {
			if successor == nil || !successor.Live {
				continue
			}
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

		component := make([]*cfg.Block, 0, 2)
		for {
			last := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			onStack[last] = false
			component = append(component, last)
			if last == block {
				break
			}
		}
		isCycle := len(component) > 1
		if !isCycle {
			for _, successor := range block.Succs {
				if successor == block {
					isCycle = true
					break
				}
			}
		}
		if isCycle {
			for _, member := range component {
				cyclic[member] = true
			}
		}
	}

	for _, block := range blocks {
		if block != nil && block.Live && indices[block] == 0 {
			visit(block)
		}
	}
	return cyclic
}

func ps6095Mentions(pass *analysis.Pass, node ast.Node, object types.Object) bool {
	return ps6095MentionsAny(pass, node, map[types.Object]bool{object: true})
}

func ps6095MentionsAny(pass *analysis.Pass, node ast.Node, objects map[types.Object]bool) bool {
	found := false
	ast.Inspect(node, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if ok && objects[identObject(pass, identifier)] {
			found = true
			return false
		}
		return !found
	})
	return found
}

func ps6095Object(pass *analysis.Pass, expression ast.Expr, object types.Object) bool {
	identifier, ok := ps6095Unparen(expression).(*ast.Ident)
	return ok && identObject(pass, identifier) == object
}

func ps6095Float(typeOf types.Type) bool {
	return ps6095BasicInfo(typeOf, types.IsFloat)
}

func ps6095Numeric(typeOf types.Type) bool {
	return ps6095BasicInfo(typeOf, types.IsNumeric)
}

func ps6095Integer(typeOf types.Type) bool {
	return ps6095BasicInfo(typeOf, types.IsInteger)
}

// ps6095BasicInfo proves that every feasible member of a type parameter's
// type set has the requested basic property. A single mixed or unknown member
// keeps the detector conservative: in particular, a quotient over
// ~int|~float64 must not be treated as unconditionally floating-point.
func ps6095BasicInfo(typeOf types.Type, required types.BasicInfo) bool {
	if typeOf == nil {
		return false
	}
	typeOf = types.Unalias(typeOf)
	if basic, ok := typeOf.Underlying().(*types.Basic); ok {
		return basic.Info()&required != 0
	}
	typeParameter, ok := typeOf.(*types.TypeParam)
	if !ok {
		return false
	}
	constraint, ok := types.Unalias(typeParameter.Constraint()).Underlying().(*types.Interface)
	if !ok {
		return false
	}
	set, ok := ps6091ConstraintTypeSet(constraint, make(map[types.Type]bool))
	if !ok || set.unrestricted || len(set.terms) == 0 {
		return false
	}
	feasible := 0
	for _, term := range set.terms {
		termFeasible, known := ps6091TypeTermFeasible(term, constraint, typeParameter)
		if !known {
			return false
		}
		if !termFeasible {
			continue
		}
		feasible++
		basic, ok := types.Unalias(term.value).Underlying().(*types.Basic)
		if !ok || basic.Info()&required == 0 {
			return false
		}
	}
	return feasible > 0
}

func ps6095Unparen(expression ast.Expr) ast.Expr {
	for {
		parenthesized, ok := expression.(*ast.ParenExpr)
		if !ok {
			return expression
		}
		expression = parenthesized.X
	}
}
