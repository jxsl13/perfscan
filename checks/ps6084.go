package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"slices"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS6084 implements owner issue #810. It finds a fixed, small scalar scratch
// plane that a Go loop completely builds immediately before one noescape
// assembly/native leaf consumes both the scratch and its packed input.
var PS6084 = register(&lint.Check{
	ID:       "PS6084",
	Category: "verify",
	Slug:     "fixed-scratch-before-noescape-leaf",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "fixed scalar scratch is built immediately before a noescape native leaf",
		Text: `A coarse native leaf can still pay avoidable Go work at every ABI
crossing. A wrapper may unpack a fixed number of fields, combine them with a
block scale or lookup table, store a stack array, and immediately pass both
that array and the original packed metadata to assembly. Moving only that
block-local arithmetic into the existing leaf lets it reuse metadata that it
already loads and removes the Go preprocessing loop and scratch plane.

This check implements owner issue #810. It reports only a local numeric array
of 2 through 64 elements with zero initialization and a complete fixed-count
fill. The fill must be one side-effect-free scalar assignment per element,
indexed by an exact zero-based range or for loop. The immediately following
statement must contain exactly one direct call to a same-package bodyless
function or method carrying the compiler-recognized //go:noescape directive.
The scratch must reach that call as a direct pointer or full slice, have no
other use in the function, and the leaf must independently receive integer or
integer-container metadata read by the fill. Packed fields selected from a
record qualify only when the leaf receives the same typed record/field index
path; another element of the same aggregate is not evidence. Calls through function values,
Go-bodied helpers, partial or conditional fills, dynamic slices, non-zero
initializers, transformed scratch pointers, additional scratch consumers, and
packed inputs visible only through len/cap stay silent.

For owner issue #839 the check also recognizes a grouped straight-line fill:
two through sixteen plain stores may completely overwrite one or two fixed
arrays through source-proven affine indices. Tuple locals are tracked at their
statement versions, and every packed dependency must resolve through stable
slice subpaths to the corresponding typed native argument. A nonzero payload
pointer qualifies only at the exact end of the proven header subpath; another
offset or aggregate stays silent. Numeric conversions, exact
binary.LittleEndian Uint16 reads, and local single-return helpers whose used
parameters and source-visible region-stable tables are proved may carry
provenance. Runtime writes, address escapes, dynamic ByteOrder receivers,
ignored arguments, mixed roots, and unmodeled calls stay silent. Package tables
may be populated only by source-visible init functions. A direct table pointer
argument to a recognized noescape leaf is treated only as a nonretaining,
possibly writing borrow: it disqualifies wrappers that may execute that borrow,
while an unreachable borrow in a separate wrapper does not establish native
purity and does not globally poison the table. Callbacks and values whose types
may carry callbacks passed to opaque, imported, or bodyless callees are
conservatively included in that regional reachability proof, including when
the callee is noescape. Dynamic interface method calls are likewise unknown.
Other writes and address escapes stay silent. The grouped path additionally
requires a supported
typed induction/bound shape and a feasible recurring edge; unknown, wrapping,
mutated, blocking, asynchronous, or uninvoked regions stay silent.

When an enclosing Go loop repeats the proven leaf call, the diagnostic also
asks whether the native boundary can cover the complete row. That is a
benchmark question, not a static rewrite: a larger leaf may increase native
extraction cost, register pressure, code size, or numerical drift. There is NO
automatic fix. Preserve the exact scalar dtype and operation order, bounds,
alignment, aliases, tails, feature gates, fallback behavior, and the noescape
contract. Validate arbitrary packed rows against a scalar oracle, inspect the
native code, keep the candidate separately selectable, and retain it only
after same-binary alternating-order complete-operation benchmarks. A reviewed
wrapper can use //perfscan:native-scratch-validated on its function declaration.

Owner issue #843 covers the independent-row form of this same repeated-call
pattern, not a second overlapping rule. Keep exact block iteration order and
per-block reduction boundaries (including float32 subtotals widened to ordered
float64 accumulation). Preserve a zero-length fast path before forming element
pointers, and change only architecture-specific routing while retaining other
architectures and scalar fallbacks. Build with the minimum supported compiler
as well as the current one. Retain arbitrary-header parity, allocation checks,
and end-to-end benchmarks in addition to the isolated row benchmark.

Prioritize a candidate when a retained profile confirms meaningful wrapper/ABI
cost and measurements confirm an allocation-free native arithmetic leaf.
Noescape proves nonretention, not allocation freedom or absence of side effects;
neither native purity nor arithmetic ownership is inferred from its name or
directive. The source detector deliberately requires the fixed-array staging
and packed-input proofs above; scalar-local-only or unmodeled orchestration
stays outside its supported subset.`,
		Before: `//go:noescape
func blockLeaf(dst, packed *byte, coefficients *float32)

func runBlock(dst, packed []byte, scale float32, table []float32) {
	var coefficients [8]float32
	for field := range coefficients {
		coefficients[field] = scale * table[packed[field]&15]
	}
	blockLeaf(&dst[0], &packed[0], &coefficients[0])
}`,
		After: `// Pass scale/table across the existing noescape boundary.
// Let blockLeaf reuse its packed metadata load to form each coefficient.
// Preserve float32 multiply order and keep the scalar wrapper as fallback.
blockLeaf(&dst[0], &packed[0], scale, &table[0])`,
		MeasuredWin: `Owner issue #810 measured this boundary change on Apple M2
Pro after all other IQ1_S optimizations. Five alternating fresh-process pairs
at 500 ms reduced the K4096 leaf from 679.6 ns to 656.9 ns (-3.34%, p=0.008)
and M1/N64/K1024 from 11.38 us to 10.78 us (-5.26%, p=0.008), with unchanged
scalar controls and allocations. Extending the same boundary to the complete
row later reduced K4096 from 657.0 ns to 612.2 ns (-6.82%, p=0.008) and
M1/N64/K1024 from 10.78 us to 10.14 us (-5.87%, p=0.008). Maximum reported
scalar-relative error was 3.452027636539311e-14. These measurements motivate
the diagnostic shape; they do not predict a win for another leaf or target.

Owner issue #839 extends the same advisory to a straight-line grouped fill of
one or two fixed arrays. Its accepted Q4_K pair moved two fully overwritten
16-float staging arrays and packed-header decode across the existing noescape
boundary. Seven alternating row pairs improved 545.5 to 510.3 ns (1.069x),
and seven complete pair-apply pairs improved 561.721 to 509.870 us (1.102x),
with allocations unchanged. Five production pairs had a 1.042x aggregate
median and three wins; final numerical digests were identical. An enlarged Go
staging alternative was rejected after growing the frame from 224 to 1216
bytes, zeroing 1088 bytes, and regressing. These owner-specific measurements
require the pinned emitted-code and end-to-end evidence; source analysis alone
does not prove zeroing, a speedup, or native purity.

Owner issue #843's independent Q4_K row moved 16-float metadata staging and
eight block calls at K=2048 into one ARM64 row call. On Apple M2 Pro the owner
reported 283.5 to 245.4 ns (1.155x, 7/7 row wins, zero allocations),
157.548 to 147.343 us at M1/N=4096 (1.069x, 5/7, 29 allocations unchanged),
and 1.788325 to 1.651625 s for 64-token production decode (1.083x, 7/7).
Every retained production pair had final-logit digest ea3df5516f17df83.
The pinned owner campaign compares baseline source
9f1801c82f91330c79c4d02ab9e4755f635a4ada with candidate
68f4018355d7de48c6e2b0fb5fbe1bef51044ca1; evidence is at
https://github.com/jxsl13/goai/tree/8d1d26d4e3fb239d7c727f2dec6bb398335da46b/internal/benchcompare/leadership/evidence/m2-cpu-q4k-single-row-asm-20260823.
These are the owner's separate-binary, alternating fresh-process results, not
a new perfscan benchmark run or a general speedup guarantee. The campaign
records Go 1.26.6/1.27.0 package checks and cross-builds, and separately records
unrelated full-repository test failures; it does not claim that run was green.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6084",
		Doc:  "fixed scalar scratch is immediately consumed with its packed input by a noescape native leaf",
		Run:  runPS6084,
	},
})

const (
	ps6084MinimumScratch     = int64(2)
	ps6084MaximumScratch     = int64(64)
	ps6084WholeRowGuardrails = " Preserve exact block iteration order and per-block reduction boundaries, a zero-length fast path before element pointers, and architecture-specific routing; require minimum-supported-compiler builds and retained end-to-end benchmarks. Noescape alone does not prove allocation freedom or native purity."
)

type ps6084Fill struct {
	loop        ast.Stmt
	scratch     *types.Var
	scratchName string
	index       *types.Var
	assignment  *ast.AssignStmt
	rhs         ast.Expr
	length      int64
	element     types.Type
	allowedUses map[*ast.Ident]bool
}

type ps6084State struct {
	pass                  *analysis.Pass
	function              *ast.FuncDecl
	parents               map[ast.Node]ast.Node
	leaves                map[*types.Func]*ast.FuncDecl
	writes                map[types.Object]int
	definitions           map[types.Object]int
	addressTaken          map[types.Object]bool
	aliasParent           map[types.Object]types.Object
	groupedFlow           *ps6122Flow
	groupedDecls          map[*types.Func]*ast.FuncDecl
	groupedTables         map[*types.Var]bool
	groupedRegions        map[ast.Node]bool
	groupedRegionsUnknown bool
	groupedRegionsReady   bool
}

type ps6084Match struct {
	fill       ps6084Fill
	call       *ast.CallExpr
	leaf       *types.Func
	sources    []types.Object
	repeated   bool
	callUseIDs map[*ast.Ident]bool
}

type ps6084PackedSource struct {
	object  types.Object
	direct  bool
	anchors []ast.Expr
}

func runPS6084(pass *analysis.Pass) (any, error) {
	leaves := ps6084NoescapeLeaves(pass)
	if len(leaves) == 0 {
		return nil, nil
	}
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil || ps6084Validated(function) {
				continue
			}
			state := ps6084NewState(pass, function, leaves)
			ast.Inspect(function.Body, func(node ast.Node) bool {
				if _, nested := node.(*ast.FuncLit); nested {
					return false
				}
				block, ok := node.(*ast.BlockStmt)
				if !ok {
					return true
				}
				for index := 0; index+1 < len(block.List); index++ {
					match, ok := state.match(block.List[index], block.List[index+1])
					if ok {
						ps6084Report(pass, &match)
					}
					grouped, ok := state.groupedMatch(block.List[index], block.List[index+1])
					if ok {
						ps6084ReportGrouped(pass, &grouped)
					}
				}
				return true
			})
		}
	}
	return nil, nil
}

func ps6084NoescapeLeaves(pass *analysis.Pass) map[*types.Func]*ast.FuncDecl {
	result := make(map[*types.Func]*ast.FuncDecl)
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body != nil || !ps6084Directive(function.Doc, "go:noescape") {
				continue
			}
			object, _ := pass.TypesInfo.Defs[function.Name].(*types.Func)
			if object != nil {
				result[object] = function
			}
		}
	}
	return result
}

func ps6084Directive(comments *ast.CommentGroup, directive string) bool {
	if comments == nil {
		return false
	}
	for _, comment := range comments.List {
		if comment.Text == "//"+directive {
			return true
		}
	}
	return false
}

func ps6084Validated(function *ast.FuncDecl) bool {
	return ps6084Directive(function.Doc, "perfscan:native-scratch-validated")
}

func ps6084NewState(pass *analysis.Pass, function *ast.FuncDecl, leaves map[*types.Func]*ast.FuncDecl) *ps6084State {
	state := &ps6084State{
		pass: pass, function: function, parents: ps6071Parents(function.Body), leaves: leaves,
		writes: make(map[types.Object]int), definitions: make(map[types.Object]int),
		addressTaken: make(map[types.Object]bool),
		aliasParent:  make(map[types.Object]types.Object),
	}
	type aliasPair struct{ left, right types.Object }
	var pairs []aliasPair
	ast.Inspect(function.Body, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.AssignStmt:
			for index, left := range value.Lhs {
				identifier, ok := ps2110Unparen(left).(*ast.Ident)
				if !ok {
					continue
				}
				object := pass.TypesInfo.Defs[identifier]
				if object != nil {
					state.definitions[object]++
				}
				if object == nil {
					object = pass.TypesInfo.Uses[identifier]
				}
				if object == nil {
					continue
				}
				state.writes[object]++
				if len(value.Lhs) == len(value.Rhs) && index < len(value.Rhs) {
					if right := ps6084IdentifierObject(pass, value.Rhs[index]); right != nil {
						pairs = append(pairs, aliasPair{left: object, right: right})
					}
				}
			}
		case *ast.ValueSpec:
			for index, name := range value.Names {
				object := pass.TypesInfo.Defs[name]
				if object == nil {
					continue
				}
				state.definitions[object]++
				state.writes[object]++
				if len(value.Names) == len(value.Values) && index < len(value.Values) {
					if right := ps6084IdentifierObject(pass, value.Values[index]); right != nil {
						pairs = append(pairs, aliasPair{left: object, right: right})
					}
				}
			}
		case *ast.RangeStmt:
			for _, expression := range []ast.Expr{value.Key, value.Value} {
				identifier, ok := ps2110Unparen(expression).(*ast.Ident)
				if !ok || identifier.Name == "_" {
					continue
				}
				object := pass.TypesInfo.Defs[identifier]
				if object != nil {
					state.definitions[object]++
				}
				if object == nil {
					object = pass.TypesInfo.Uses[identifier]
				}
				if object != nil {
					state.writes[object]++
				}
			}
		case *ast.IncDecStmt:
			if object := ps6084IdentifierObject(pass, value.X); object != nil {
				state.writes[object]++
			}
		case *ast.UnaryExpr:
			if value.Op == token.AND {
				if object := ps6084IdentifierObject(pass, value.X); object != nil {
					state.addressTaken[object] = true
				}
			}
		}
		return true
	})
	for _, pair := range pairs {
		left, leftOK := pair.left.(*types.Var)
		right, rightOK := pair.right.(*types.Var)
		if !leftOK || !rightOK || left.IsField() || right.IsField() ||
			!ps6084IdentityAliasType(left.Type()) || !ps6084IdentityAliasType(right.Type()) ||
			state.definitions[left] != 1 || state.writes[left] != state.definitions[left] ||
			state.writes[right] != state.definitions[right] ||
			state.addressTaken[left] || state.addressTaken[right] {
			continue
		}
		state.aliasUnion(left, right)
	}
	return state
}

// Assignment copies arrays and structs, so their subobjects can diverge even
// when both variables initially contain equal values. Only carriers whose Go
// assignment preserves the referenced storage participate in identity unions.
func ps6084IdentityAliasType(t types.Type) bool {
	if t == nil {
		return false
	}
	switch types.Unalias(t).Underlying().(type) {
	case *types.Pointer, *types.Slice:
		return true
	default:
		return false
	}
}

func ps6084IdentifierObject(pass *analysis.Pass, expression ast.Expr) types.Object {
	identifier, ok := ps2110Unparen(expression).(*ast.Ident)
	if !ok {
		return nil
	}
	if object := pass.TypesInfo.Uses[identifier]; object != nil {
		return object
	}
	return pass.TypesInfo.Defs[identifier]
}

func (state *ps6084State) aliasFind(object types.Object) types.Object {
	if object == nil {
		return nil
	}
	parent := state.aliasParent[object]
	if parent == nil {
		state.aliasParent[object] = object
		return object
	}
	if parent == object {
		return object
	}
	root := state.aliasFind(parent)
	state.aliasParent[object] = root
	return root
}

func (state *ps6084State) aliasUnion(left, right types.Object) {
	left = state.aliasFind(left)
	right = state.aliasFind(right)
	if left != right {
		state.aliasParent[left] = right
	}
}

func (state *ps6084State) match(preprocess, consumer ast.Stmt) (ps6084Match, bool) {
	fill, ok := state.fixedFill(preprocess)
	if !ok || !state.zeroInitialized(fill.scratch, fill.loop.Pos()) ||
		ps6084ExpressionHasEffects(state.pass, fill.rhs) {
		return ps6084Match{}, false
	}
	sources := ps6084PackedSources(state, fill.rhs, fill.scratch, fill.index)
	if len(sources) == 0 {
		return ps6084Match{}, false
	}
	call, leaf, ok := state.nativeConsumer(consumer)
	if !ok {
		return ps6084Match{}, false
	}
	callUses, scratchArgument, ok := ps6084ConsumerScratch(state.pass, call, fill.scratch)
	if !ok {
		return ps6084Match{}, false
	}
	matchedSources := state.consumerSources(call, scratchArgument, sources)
	if len(matchedSources) == 0 {
		return ps6084Match{}, false
	}
	allowed := make(map[*ast.Ident]bool, len(fill.allowedUses)+len(callUses))
	for identifier := range fill.allowedUses {
		allowed[identifier] = true
	}
	for identifier := range callUses {
		allowed[identifier] = true
	}
	if !ps6084OnlyScratchUses(state.pass, state.function.Body, fill.scratch, allowed) {
		return ps6084Match{}, false
	}
	return ps6084Match{
		fill: fill, call: call, leaf: leaf, sources: matchedSources,
		repeated: ps6084EnclosingLoop(call, state.parents), callUseIDs: callUses,
	}, true
}

func (state *ps6084State) fixedFill(statement ast.Stmt) (ps6084Fill, bool) {
	var loopBody *ast.BlockStmt
	switch loop := statement.(type) {
	case *ast.ForStmt:
		loopBody = loop.Body
	case *ast.RangeStmt:
		loopBody = loop.Body
	default:
		return ps6084Fill{}, false
	}
	if len(loopBody.List) != 1 {
		return ps6084Fill{}, false
	}
	assignment, ok := loopBody.List[0].(*ast.AssignStmt)
	if !ok || assignment.Tok != token.ASSIGN || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
		return ps6084Fill{}, false
	}
	indexExpression, ok := ps2110Unparen(assignment.Lhs[0]).(*ast.IndexExpr)
	if !ok {
		return ps6084Fill{}, false
	}
	scratchIdentifier, ok := ps2110Unparen(indexExpression.X).(*ast.Ident)
	if !ok {
		return ps6084Fill{}, false
	}
	scratch, ok := state.pass.TypesInfo.Uses[scratchIdentifier].(*types.Var)
	if !ok || scratch.IsField() || scratch.Parent() == state.pass.Pkg.Scope() {
		return ps6084Fill{}, false
	}
	array, ok := types.Unalias(scratch.Type()).Underlying().(*types.Array)
	if !ok || array.Len() < ps6084MinimumScratch || array.Len() > ps6084MaximumScratch ||
		!ps6084NumericType(array.Elem()) {
		return ps6084Fill{}, false
	}
	indexIdentifier, ok := ps2110Unparen(indexExpression.Index).(*ast.Ident)
	if !ok {
		return ps6084Fill{}, false
	}
	index, ok := state.pass.TypesInfo.Uses[indexIdentifier].(*types.Var)
	if !ok {
		return ps6084Fill{}, false
	}
	allowed := map[*ast.Ident]bool{scratchIdentifier: true}
	if !state.fixedLoop(statement, scratch, index, array.Len(), allowed) {
		return ps6084Fill{}, false
	}
	return ps6084Fill{
		loop: statement, scratch: scratch, scratchName: scratchIdentifier.Name,
		index: index, assignment: assignment, rhs: assignment.Rhs[0],
		length: array.Len(), element: array.Elem(), allowedUses: allowed,
	}, true
}

func (state *ps6084State) fixedLoop(statement ast.Stmt, scratch, index *types.Var, length int64, allowed map[*ast.Ident]bool) bool {
	switch loop := statement.(type) {
	case *ast.RangeStmt:
		if loop.Tok != token.DEFINE || loop.Value != nil {
			return false
		}
		key, ok := ps2110Unparen(loop.Key).(*ast.Ident)
		if !ok || state.pass.TypesInfo.Defs[key] != index {
			return false
		}
		if identifier, ok := ps2110Unparen(loop.X).(*ast.Ident); ok && state.pass.TypesInfo.Uses[identifier] == scratch {
			allowed[identifier] = true
			return true
		}
		bound, boundUses, ok := ps6084ExactBound(state.pass, loop.X, scratch, length)
		if !ok || bound != length {
			return false
		}
		for identifier := range boundUses {
			allowed[identifier] = true
		}
		return true
	case *ast.ForStmt:
		initialization, ok := loop.Init.(*ast.AssignStmt)
		if !ok || initialization.Tok != token.DEFINE || len(initialization.Lhs) != 1 || len(initialization.Rhs) != 1 ||
			ps6084ConstantInt(state.pass, initialization.Rhs[0]) != 0 {
			return false
		}
		initialIdentifier, ok := ps2110Unparen(initialization.Lhs[0]).(*ast.Ident)
		if !ok || state.pass.TypesInfo.Defs[initialIdentifier] != index {
			return false
		}
		condition, ok := ps2110Unparen(loop.Cond).(*ast.BinaryExpr)
		if !ok || condition.Op != token.LSS {
			return false
		}
		conditionIndex, ok := ps2110Unparen(condition.X).(*ast.Ident)
		if !ok || state.pass.TypesInfo.Uses[conditionIndex] != index {
			return false
		}
		bound, boundUses, ok := ps6084ExactBound(state.pass, condition.Y, scratch, length)
		if !ok || bound != length {
			return false
		}
		for identifier := range boundUses {
			allowed[identifier] = true
		}
		increment, ok := loop.Post.(*ast.IncDecStmt)
		if !ok || increment.Tok != token.INC || ps6084IdentifierObject(state.pass, increment.X) != index {
			return false
		}
		return true
	default:
		return false
	}
}

func ps6084ExactBound(pass *analysis.Pass, expression ast.Expr, scratch *types.Var, length int64) (int64, map[*ast.Ident]bool, bool) {
	uses := make(map[*ast.Ident]bool)
	call, ok := ps2110Unparen(expression).(*ast.CallExpr)
	if ok && len(call.Args) == 1 {
		identifier, identifierOK := ps2110Unparen(call.Fun).(*ast.Ident)
		argument, argumentOK := ps2110Unparen(call.Args[0]).(*ast.Ident)
		if identifierOK && argumentOK && pass.TypesInfo.Uses[identifier] == types.Universe.Lookup("len") &&
			pass.TypesInfo.Uses[argument] == scratch {
			uses[argument] = true
			return length, uses, true
		}
	}
	value := ps6084ConstantInt(pass, expression)
	return value, uses, value == length
}

func ps6084ConstantInt(pass *analysis.Pass, expression ast.Expr) int64 {
	value := pass.TypesInfo.Types[expression].Value
	if value == nil {
		value = pass.TypesInfo.Types[ps2110Unparen(expression)].Value
	}
	if value == nil || value.Kind() != constant.Int {
		return -1
	}
	integer, exact := constant.Int64Val(value)
	if !exact {
		return -1
	}
	return integer
}

func ps6084NumericType(t types.Type) bool {
	basic, ok := types.Unalias(t).Underlying().(*types.Basic)
	return ok && basic.Info()&types.IsNumeric != 0
}

func (state *ps6084State) zeroInitialized(scratch *types.Var, before token.Pos) bool {
	found := false
	ast.Inspect(state.function.Body, func(node ast.Node) bool {
		if found || node == nil || node.Pos() >= before {
			return !found
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		switch value := node.(type) {
		case *ast.ValueSpec:
			for index, name := range value.Names {
				if state.pass.TypesInfo.Defs[name] != scratch {
					continue
				}
				if len(value.Values) == 0 {
					found = true
					return false
				}
				if len(value.Values) == len(value.Names) && ps6084ZeroArrayLiteral(state.pass, value.Values[index], scratch.Type()) {
					found = true
					return false
				}
			}
		case *ast.AssignStmt:
			if value.Tok != token.DEFINE || len(value.Lhs) != 1 || len(value.Rhs) != 1 {
				return true
			}
			identifier, ok := ps2110Unparen(value.Lhs[0]).(*ast.Ident)
			if ok && state.pass.TypesInfo.Defs[identifier] == scratch && ps6084ZeroArrayLiteral(state.pass, value.Rhs[0], scratch.Type()) {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

func ps6084ZeroArrayLiteral(pass *analysis.Pass, expression ast.Expr, target types.Type) bool {
	literal, ok := ps2110Unparen(expression).(*ast.CompositeLit)
	return ok && len(literal.Elts) == 0 && types.Identical(pass.TypesInfo.TypeOf(literal), target)
}

func ps6084ExpressionHasEffects(pass *analysis.Pass, expression ast.Expr) bool {
	effect := false
	ast.Inspect(expression, func(node ast.Node) bool {
		if effect {
			return false
		}
		switch value := node.(type) {
		case *ast.UnaryExpr:
			if value.Op == token.ARROW {
				effect = true
				return false
			}
			return true
		case *ast.TypeAssertExpr:
			effect = true
			return false
		case *ast.IndexExpr:
			if indexedType := pass.TypesInfo.TypeOf(value.X); indexedType != nil {
				if _, mapIndex := types.Unalias(indexedType).Underlying().(*types.Map); mapIndex {
					effect = true
					return false
				}
			}
			return true
		case *ast.CallExpr:
			if pass.TypesInfo.Types[value.Fun].IsType() {
				return true
			}
			effect = true
			return false
		default:
			return true
		}
	})
	return effect
}

func ps6084PackedSources(state *ps6084State, expression ast.Expr, scratch, index *types.Var) map[types.Object]*ps6084PackedSource {
	result := make(map[types.Object]*ps6084PackedSource)
	parents := ps6071Parents(expression)
	ast.Inspect(expression, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.Ident:
			object, ok := state.pass.TypesInfo.Uses[value].(*types.Var)
			if !ok || object == scratch || object == index || object.IsField() ||
				!ps6084PackedSourceUse(object.Type(), value, parents) {
				return true
			}
			root := state.aliasFind(object)
			source := result[root]
			if source == nil {
				source = &ps6084PackedSource{object: root}
				result[root] = source
			}
			source.direct = true
		case *ast.SelectorExpr:
			if !ps6084PackedType(state.pass.TypesInfo.TypeOf(value)) {
				return true
			}
			for _, object := range ps6084AggregateRoots(state.pass, value.X, scratch, index) {
				root := state.aliasFind(object)
				source := result[root]
				if source == nil {
					source = &ps6084PackedSource{object: root}
					result[root] = source
				}
				source.anchors = append(source.anchors, value.X, value)
			}
		}
		return true
	})
	return result
}

func ps6084AggregateRoots(pass *analysis.Pass, expression ast.Expr, scratch, index *types.Var) []types.Object {
	seen := make(map[types.Object]bool)
	var result []types.Object
	ast.Inspect(expression, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		object, ok := pass.TypesInfo.Uses[identifier].(*types.Var)
		if !ok || object == scratch || object == index || object.IsField() || seen[object] ||
			!ps6084AggregateType(object.Type(), 0) {
			return true
		}
		seen[object] = true
		result = append(result, object)
		return true
	})
	return result
}

func ps6084AggregateType(t types.Type, depth int) bool {
	if t == nil || depth > 4 {
		return false
	}
	switch value := types.Unalias(t).Underlying().(type) {
	case *types.Struct:
		return true
	case *types.Array:
		return ps6084AggregateType(value.Elem(), depth+1)
	case *types.Slice:
		return ps6084AggregateType(value.Elem(), depth+1)
	case *types.Pointer:
		return ps6084AggregateType(value.Elem(), depth+1)
	default:
		return false
	}
}

func ps6084PackedSourceUse(t types.Type, identifier *ast.Ident, parents map[ast.Node]ast.Node) bool {
	underlying := types.Unalias(t).Underlying()
	if basic, ok := underlying.(*types.Basic); ok {
		if basic.Kind() == types.String {
			return true
		}
		if basic.Info()&types.IsInteger == 0 {
			return false
		}
		for parent := parents[identifier]; parent != nil; parent = parents[parent] {
			switch value := parent.(type) {
			case *ast.IndexExpr:
				if ps6084ContainsNode(value.Index, identifier) {
					return false
				}
			case *ast.BinaryExpr:
				if (value.Op == token.AND || value.Op == token.OR || value.Op == token.XOR ||
					value.Op == token.SHL || value.Op == token.SHR) && ps6084ContainsNode(value.X, identifier) {
					return true
				}
			case *ast.UnaryExpr:
				if value.Op == token.XOR {
					return true
				}
			}
		}
		return false
	}
	return ps6084PackedType(t)
}

func ps6084ContainsNode(root, target ast.Node) bool {
	found := false
	ast.Inspect(root, func(node ast.Node) bool {
		if node == target {
			found = true
			return false
		}
		return !found
	})
	return found
}

func ps6084PackedType(t types.Type) bool {
	underlying := types.Unalias(t).Underlying()
	switch value := underlying.(type) {
	case *types.Basic:
		return value.Info()&types.IsInteger != 0 || value.Kind() == types.String
	case *types.Array:
		return ps6084IntegerElement(value.Elem())
	case *types.Slice:
		return ps6084IntegerElement(value.Elem())
	case *types.Pointer:
		switch element := types.Unalias(value.Elem()).Underlying().(type) {
		case *types.Array:
			return ps6084IntegerElement(element.Elem())
		case *types.Basic:
			return element.Info()&types.IsInteger != 0
		}
	}
	return false
}

func ps6084IntegerElement(t types.Type) bool {
	basic, ok := types.Unalias(t).Underlying().(*types.Basic)
	return ok && basic.Info()&types.IsInteger != 0
}

func (state *ps6084State) nativeConsumer(statement ast.Stmt) (*ast.CallExpr, *types.Func, bool) {
	switch statement.(type) {
	case *ast.ExprStmt, *ast.AssignStmt, *ast.ReturnStmt:
	default:
		return nil, nil, false
	}
	var native *ast.CallExpr
	var leaf *types.Func
	invalid := false
	ast.Inspect(statement, func(node ast.Node) bool {
		if invalid {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			invalid = true
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if state.pass.TypesInfo.Types[call.Fun].IsType() || ps6084HarmlessBuiltin(state.pass, call) {
			return true
		}
		function := ps6071CalledFunction(state.pass, call)
		if function == nil || state.leaves[function] == nil || native != nil {
			invalid = true
			return false
		}
		native, leaf = call, function
		return true
	})
	if !invalid && native != nil && !state.consumerCallUnconditional(native, statement) {
		invalid = true
	}
	return native, leaf, !invalid && native != nil
}

func (state *ps6084State) consumerCallUnconditional(call *ast.CallExpr, statement ast.Stmt) bool {
	for parent := state.parents[call]; parent != nil && parent != statement; parent = state.parents[parent] {
		if binary, ok := parent.(*ast.BinaryExpr); ok && (binary.Op == token.LAND || binary.Op == token.LOR) {
			return false
		}
	}
	return true
}

func ps6084HarmlessBuiltin(pass *analysis.Pass, call *ast.CallExpr) bool {
	identifier, ok := ps2110Unparen(call.Fun).(*ast.Ident)
	if !ok {
		return false
	}
	builtin, ok := pass.TypesInfo.Uses[identifier].(*types.Builtin)
	return ok && (builtin.Name() == "len" || builtin.Name() == "cap")
}

func ps6084ConsumerScratch(pass *analysis.Pass, call *ast.CallExpr, scratch *types.Var) (map[*ast.Ident]bool, int, bool) {
	array, ok := types.Unalias(scratch.Type()).Underlying().(*types.Array)
	if !ok {
		return nil, -1, false
	}
	matched := -1
	uses := make(map[*ast.Ident]bool)
	for index, argument := range call.Args {
		if !ps6084ReferenceCarrier(pass.TypesInfo.TypeOf(argument), array.Elem()) {
			if identifier, ok := ps6084ScratchObservation(pass, argument, scratch); ok {
				uses[identifier] = true
			}
			continue
		}
		argumentUses, ok := ps6084ScratchArgument(pass, argument, scratch)
		if !ok {
			continue
		}
		if matched >= 0 {
			return nil, -1, false
		}
		matched = index
		for identifier := range argumentUses {
			uses[identifier] = true
		}
	}
	return uses, matched, matched >= 0
}

func ps6084ScratchObservation(pass *analysis.Pass, expression ast.Expr, scratch *types.Var) (*ast.Ident, bool) {
	call, ok := ps2110Unparen(expression).(*ast.CallExpr)
	if !ok || len(call.Args) != 1 || !ps6084HarmlessBuiltin(pass, call) {
		return nil, false
	}
	identifier, ok := ps2110Unparen(call.Args[0]).(*ast.Ident)
	return identifier, ok && pass.TypesInfo.Uses[identifier] == scratch
}

func ps6084ReferenceCarrier(t, element types.Type) bool {
	if t == nil {
		return false
	}
	switch value := types.Unalias(t).Underlying().(type) {
	case *types.Pointer:
		pointee := types.Unalias(value.Elem()).Underlying()
		if array, ok := pointee.(*types.Array); ok {
			return types.Identical(array.Elem(), element)
		}
		return types.Identical(value.Elem(), element)
	case *types.Slice:
		return types.Identical(value.Elem(), element)
	case *types.Basic:
		return value.Kind() == types.UnsafePointer
	default:
		return false
	}
}

func ps6084ScratchArgument(pass *analysis.Pass, expression ast.Expr, scratch *types.Var) (map[*ast.Ident]bool, bool) {
	uses := make(map[*ast.Ident]bool)
	var storage func(ast.Expr, bool) bool
	storage = func(raw ast.Expr, addressed bool) bool {
		expression := ps2110Unparen(raw)
		switch value := expression.(type) {
		case *ast.Ident:
			if pass.TypesInfo.Uses[value] != scratch || !addressed {
				return false
			}
			uses[value] = true
			return true
		case *ast.UnaryExpr:
			if value.Op != token.AND {
				return false
			}
			return storage(value.X, true)
		case *ast.IndexExpr:
			if ps6084ConstantInt(pass, value.Index) != 0 {
				return false
			}
			return storage(value.X, addressed)
		case *ast.SliceExpr:
			if value.Slice3 || !ps6084ZeroOrNil(pass, value.Low) || value.High != nil || value.Max != nil {
				return false
			}
			return storage(value.X, true)
		case *ast.CallExpr:
			if len(value.Args) != 1 || !pass.TypesInfo.Types[value.Fun].IsType() {
				return false
			}
			return storage(value.Args[0], addressed)
		default:
			return false
		}
	}
	return uses, storage(expression, false)
}

func ps6084ZeroOrNil(pass *analysis.Pass, expression ast.Expr) bool {
	return expression == nil || ps6084ConstantInt(pass, expression) == 0
}

func (state *ps6084State) consumerSources(call *ast.CallExpr, scratchArgument int, sources map[types.Object]*ps6084PackedSource) []types.Object {
	matched := make(map[types.Object]types.Object)
	for argumentIndex, argument := range call.Args {
		if argumentIndex == scratchArgument {
			continue
		}
		for root, source := range sources {
			if source.direct && ps6084PassesSource(state, argument, root) {
				matched[root] = ps6084RepresentativeSource(state, argument, root)
				continue
			}
			for _, anchor := range source.anchors {
				if ps6084PassesAggregateSource(state, argument, anchor) {
					matched[root] = source.object
					break
				}
			}
		}
	}
	result := make([]types.Object, 0, len(matched))
	for source, representative := range matched {
		if representative == nil {
			representative = source
		}
		result = append(result, representative)
	}
	slices.SortFunc(result, func(left, right types.Object) int { return strings.Compare(left.Name(), right.Name()) })
	return result
}

func ps6084PassesAggregateSource(state *ps6084State, expression, anchor ast.Expr) bool {
	expression = ps2110Unparen(expression)
	if ps6084SameExpression(state, expression, anchor) {
		return true
	}
	switch value := expression.(type) {
	case *ast.UnaryExpr:
		return (value.Op == token.AND || value.Op == token.MUL) &&
			ps6084PassesAggregateSource(state, value.X, anchor)
	case *ast.IndexExpr:
		return ps6084ConstantInt(state.pass, value.Index) == 0 &&
			ps6084PassesAggregateSource(state, value.X, anchor)
	case *ast.SliceExpr:
		return !value.Slice3 && ps6084ZeroOrNil(state.pass, value.Low) && value.High == nil &&
			ps6084PassesAggregateSource(state, value.X, anchor)
	case *ast.CallExpr:
		return ps6084CarrierPreservingConversion(state.pass, value) &&
			ps6084PassesAggregateSource(state, value.Args[0], anchor)
	default:
		return false
	}
}

func ps6084SameExpression(state *ps6084State, left, right ast.Expr) bool {
	left = ps2110Unparen(left)
	right = ps2110Unparen(right)
	switch leftValue := left.(type) {
	case *ast.Ident:
		rightValue, ok := right.(*ast.Ident)
		if !ok {
			return false
		}
		leftObject := state.pass.TypesInfo.ObjectOf(leftValue)
		rightObject := state.pass.TypesInfo.ObjectOf(rightValue)
		return leftObject != nil && rightObject != nil && state.aliasFind(leftObject) == state.aliasFind(rightObject)
	case *ast.BasicLit:
		rightValue, ok := right.(*ast.BasicLit)
		if !ok {
			return false
		}
		leftConstant := state.pass.TypesInfo.Types[leftValue].Value
		rightConstant := state.pass.TypesInfo.Types[rightValue].Value
		return leftConstant != nil && rightConstant != nil && constant.Compare(leftConstant, token.EQL, rightConstant)
	case *ast.IndexExpr:
		rightValue, ok := right.(*ast.IndexExpr)
		return ok && ps6084SameExpression(state, leftValue.X, rightValue.X) &&
			ps6084SameExpression(state, leftValue.Index, rightValue.Index)
	case *ast.SelectorExpr:
		rightValue, ok := right.(*ast.SelectorExpr)
		return ok && state.pass.TypesInfo.ObjectOf(leftValue.Sel) == state.pass.TypesInfo.ObjectOf(rightValue.Sel) &&
			ps6084SameExpression(state, leftValue.X, rightValue.X)
	case *ast.BinaryExpr:
		rightValue, ok := right.(*ast.BinaryExpr)
		return ok && leftValue.Op == rightValue.Op && ps6084SameExpression(state, leftValue.X, rightValue.X) &&
			ps6084SameExpression(state, leftValue.Y, rightValue.Y)
	case *ast.UnaryExpr:
		rightValue, ok := right.(*ast.UnaryExpr)
		return ok && leftValue.Op == rightValue.Op && ps6084SameExpression(state, leftValue.X, rightValue.X)
	case *ast.CallExpr:
		rightValue, ok := right.(*ast.CallExpr)
		return ok && len(leftValue.Args) == 1 && len(rightValue.Args) == 1 &&
			state.pass.TypesInfo.Types[leftValue.Fun].IsType() && state.pass.TypesInfo.Types[rightValue.Fun].IsType() &&
			types.Identical(state.pass.TypesInfo.TypeOf(leftValue), state.pass.TypesInfo.TypeOf(rightValue)) &&
			ps6084SameExpression(state, leftValue.Args[0], rightValue.Args[0])
	default:
		return false
	}
}

func ps6084PassesSource(state *ps6084State, expression ast.Expr, source types.Object) bool {
	return ps6084PassesSourceCarrier(state, expression, source, false)
}

func ps6084PassesSourceCarrier(state *ps6084State, expression ast.Expr, source types.Object, addressed bool) bool {
	expression = ps2110Unparen(expression)
	switch value := expression.(type) {
	case *ast.Ident:
		object := state.pass.TypesInfo.Uses[value]
		return object != nil && state.aliasFind(object) == source
	case *ast.UnaryExpr:
		if value.Op == token.AND {
			return ps6084PassesSourceCarrier(state, value.X, source, true)
		}
		return value.Op == token.MUL && addressed && ps6084PassesSourceCarrier(state, value.X, source, false)
	case *ast.IndexExpr:
		return addressed && ps6084ConstantInt(state.pass, value.Index) == 0 &&
			ps6084PassesSourceCarrier(state, value.X, source, false)
	case *ast.SliceExpr:
		return !value.Slice3 && ps6084ZeroOrNil(state.pass, value.Low) && value.High == nil && value.Max == nil &&
			ps6084PassesSourceCarrier(state, value.X, source, false)
	case *ast.CallExpr:
		return ps6084CarrierPreservingConversion(state.pass, value) &&
			ps6084PassesSourceCarrier(state, value.Args[0], source, addressed)
	default:
		return false
	}
}

func ps6084CarrierPreservingConversion(pass *analysis.Pass, call *ast.CallExpr) bool {
	if len(call.Args) != 1 || call.Ellipsis.IsValid() || !pass.TypesInfo.Types[call.Fun].IsType() {
		return false
	}
	from, to := pass.TypesInfo.TypeOf(call.Args[0]), pass.TypesInfo.TypeOf(call)
	if from == nil || to == nil {
		return false
	}
	fromUnderlying := types.Unalias(from).Underlying()
	toUnderlying := types.Unalias(to).Underlying()
	if types.Identical(fromUnderlying, toUnderlying) {
		return true
	}
	if pass.TypesSizes == nil || pass.TypesSizes.Sizeof(from) != pass.TypesSizes.Sizeof(to) {
		return false
	}
	fromBasic, fromIsBasic := fromUnderlying.(*types.Basic)
	toBasic, toIsBasic := toUnderlying.(*types.Basic)
	if fromIsBasic && toIsBasic && fromBasic.Info()&types.IsInteger != 0 && toBasic.Info()&types.IsInteger != 0 {
		return true
	}
	return ps6084PointerRepresentation(fromUnderlying) && ps6084PointerRepresentation(toUnderlying)
}

func ps6084PointerRepresentation(t types.Type) bool {
	if _, ok := t.(*types.Pointer); ok {
		return true
	}
	basic, ok := t.(*types.Basic)
	return ok && basic.Kind() == types.UnsafePointer
}

func ps6084RepresentativeSource(state *ps6084State, expression ast.Expr, source types.Object) types.Object {
	var result types.Object
	ast.Inspect(expression, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		object := state.pass.TypesInfo.Uses[identifier]
		if object != nil && state.aliasFind(object) == source {
			result = object
			return false
		}
		return true
	})
	return result
}

func ps6084OnlyScratchUses(pass *analysis.Pass, body *ast.BlockStmt, scratch *types.Var, allowed map[*ast.Ident]bool) bool {
	valid := true
	ast.Inspect(body, func(node ast.Node) bool {
		if !valid {
			return false
		}
		identifier, ok := node.(*ast.Ident)
		if ok && pass.TypesInfo.Uses[identifier] == scratch && !allowed[identifier] {
			valid = false
			return false
		}
		return true
	})
	return valid
}

func ps6084EnclosingLoop(node ast.Node, parents map[ast.Node]ast.Node) bool {
	for parent := parents[node]; parent != nil; parent = parents[parent] {
		switch parent.(type) {
		case *ast.FuncDecl, *ast.FuncLit:
			return false
		case *ast.ForStmt, *ast.RangeStmt:
			return true
		}
	}
	return false
}

func ps6084Report(pass *analysis.Pass, match *ps6084Match) {
	element := types.TypeString(match.fill.element, func(pkg *types.Package) string {
		if pkg == pass.Pkg {
			return ""
		}
		return pkg.Name()
	})
	sourceNames := make([]string, 0, len(match.sources))
	for _, source := range match.sources {
		sourceNames = append(sourceNames, source.Name())
	}
	repeated := ""
	if match.repeated {
		repeated = " The enclosing Go loop also repeats this noescape ABI crossing; benchmark whether the native boundary can cover the complete row while retaining live partials." + ps6084WholeRowGuardrails
	}
	pass.Reportf(match.fill.loop.Pos(), "fixed %d-element %s scratch %s is completely built by this Go loop, used nowhere else, and immediately consumed by noescape native/assembly leaf %s, which independently receives packed source %s; move only the block-local coefficient construction across the existing ABI so the leaf can reuse already-loaded metadata.%s Preserve exact dtype and operation order, alignment, bounds, aliases, tails, feature gates, fallback behavior, and the noescape contract; validate arbitrary packed rows against a scalar oracle, inspect native code, and require same-binary alternating-order complete-operation benchmarks (advisory, no automatic fix)",
		match.fill.length, element, match.fill.scratchName, match.leaf.Name(), strings.Join(sourceNames, ", "), repeated)
}
