package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/cfg"

	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS6099 implements owner issue #899. It finds one-scalar-transcendental-per-
// output loops whose exact call input can be staged in the destination before
// an already present vector/batched leaf consumes the completed band.
var PS6099 = register(&lint.Check{
	ID:       "PS6099",
	Category: "vector",
	Slug:     "scalar-transcendental-output-staging",
	Level:    lint.LevelAggressive,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "a scalar transcendental per independent output can stage into that output for a known batched leaf",
		Text: `A per-output loop often finishes cheap indexed arithmetic and then
calls math.Exp, math.Log, math.Pow, or another expensive scalar transcendental.
When the package already exposes an operation- and precision-compatible SIMD,
vector, batch, or assembly leaf, a two-pass candidate can write the exact
transcendental input into the final destination and consume that completed
band in place. The extra memory pass can be much cheaper than scalar libm.
Pow exponents handled by square root or at most three successive-squaring
steps, including their reciprocal forms, are excluded as cheap scalar work.

PS6099 reports a deliberately narrow source-proved shape. The destination is
written at the canonical range/counting-loop index, the assignment is
unconditional, exactly one scalar transcendental lies on the element path,
its argument depends on that index (possibly through local accumulator
assignments) on every reaching path; maybe-empty nested loops retain their
zero-trip path unless entry is source-proved, while fixed-point transfer covers
every reachable iteration state. The destination and any aliases
formed before the loop have no other read, rebind, escape, capture, or opaque
use on the candidate path.
Package-local generic and nongeneric single-return wrappers are followed
through multiple layers, but every live nested call and effect is counted: a
wrapper must return on every path and cannot hide a panic hazard, allocation,
mutation, another scalar transcendental, or an opaque runtime call. A package
function or ignored architecture sibling must independently provide matching
vector/batch/assembly evidence; delegated vector work must run synchronously on
every path, including through an immediately invoked closure; a leaf
and any vector leaf it calls must have result-free, consistently float32 or
float64 sequence signatures. The destination must itself be that exact float
sequence precision and be assignable, or representation-preservingly
convertible, to a leaf slice formal for zero-copy in-place reuse. Names,
signatures, and lane access patterns must agree on the operation and precision;
an operation-free leaf name must either delegate through the sequence to one
reachable compatible vector call or write one operation's results to at least
two distinct lanes of that sequence;
an interface method declaration alone is not a concrete SIMD implementation,
and repeated access to one lane is not multi-lane evidence. Leaf and scalar
calls must be control-flow reachable after returns, branches, builtin panic,
constant and short-circuit conditions, gotos, and fallthrough. Syntax-only
ignored siblings honor lexical bindings instead of treating shadowed callback
or qualifier spellings as package or import evidence.

There is NO automatic fix. Separating the passes changes when approximation
and special-value behavior occurs, can expose aliasing between destination and
inputs, changes partial destination contents if a later element panics, and
may need a scalar tail, alignment guard, feature gate, or minimum band
crossover. Stage the exact pre-transcendental value, preserve all cheap
arithmetic and traversal order, test panic/partial-output behavior where it is
observable, and compare a separately selectable candidate against the
historical path in alternating same-binary campaigns.

When the output feeds an iterative solver, an error tolerance is not an
acceptance gate: tiny transcendental differences can change the search path
non-monotonically. Require matching iteration counts, termination, support or
active sets, decision signs, repeatability, and complete end-to-end benchmarks
before promotion.`,
		Before: `for row := range output {
	var squaredDistance float64
	for column := range features[row] {
		delta := features[row][column] - query[column]
		squaredDistance += delta * delta
	}
	output[row] = math.Exp(-gamma * squaredDistance)
}`,
		After: `// Separately selectable candidate; keep the scalar path.
for row := range output {
	var squaredDistance float64
	for column := range features[row] {
		delta := features[row][column] - query[column]
		squaredDistance += delta * delta // identical accumulation order
	}
	output[row] = -gamma * squaredDistance
}
ExpScaledF64(output, output) // known in-place batched leaf; preserve tail/gates`,
		MeasuredWin: `Owner issue #899 measured GoAI's 4000x20 RBF SVC fit on an
Apple M2 Pro with GOEXPERIMENT=simd. Seven order-alternated GOMAXPROCS=1 pairs
improved the median by 1.4856x (candidate/base 0.673122), with every pair
winning and 182 allocs/op on both arms. Scalar and SIMD controls both required
79 SMO steps and retained 42 support vectors; decision signs matched, repeated
SIMD fits were bit-deterministic, and the maximum decision-value delta was
3.33e-15. A loaded 12-core campaign also won all seven pairs at 1.2218x but was
not used as the publication campaign because unrelated tests occupied cores.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6099",
		Doc:  "scalar transcendental output loop can stage into its destination for a known batched leaf",
		Run:  runPS6099,
	},
})

var ps6099Operations = map[string]bool{
	"Acos": true, "Asin": true, "Atan": true, "Cbrt": true,
	"Cos": true, "Cosh": true, "Erf": true, "Erfc": true,
	"Exp": true, "Exp2": true, "Expm1": true, "Gamma": true,
	"Lgamma": true, "Log": true, "Log10": true, "Log1p": true,
	"Log2": true, "Pow": true, "Sin": true, "Sinh": true,
	"Tan": true, "Tanh": true,
}

var ps6099OperationOrder = []string{
	"Expm1", "Exp2", "Log10", "Log1p", "Log2", "Lgamma",
	"Acos", "Asin", "Atan", "Cbrt", "Cosh", "Erfc", "Gamma",
	"Sinh", "Tanh", "Exp", "Log", "Cos", "Erf", "Pow", "Sin", "Tan",
}

type ps6099Helper struct {
	operation       string
	parameterInputs []int
	call            *ast.CallExpr
}

type ps6099Leaf struct {
	operation     string
	name          string
	precision     string
	kind          string
	score         int
	node          ast.Node
	sequences     []ps6099LeafSequence
	syntaxTypes   map[string]ast.Expr
	syntaxAliases map[string]bool
}

type ps6099LeafSequence struct {
	typ    types.Type
	syntax ast.Expr
}

type ps6099Loop struct {
	node       ast.Node
	body       *ast.BlockStmt
	index      types.Object
	rangeValue types.Object
	sequence   ast.Expr
}

type ps6099Storage struct {
	key  string
	name string
	typ  types.Type
	root types.Object
}

type ps6099SequenceParameters struct {
	objects map[types.Object]string
	names   map[string]string
}

func runPS6099(pass *analysis.Pass) (any, error) {
	helpers := ps6099Helpers(pass)
	leaves := ps6099Leaves(pass)
	if len(leaves) == 0 {
		return nil, nil
	}
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			ps6099Function(pass, function, helpers, leaves)
		}
	}
	return nil, nil
}

func ps6099Helpers(pass *analysis.Pass) map[*types.Func]ps6099Helper {
	declarations := ps6099LocalFunctionDeclarations(pass)
	functions := make(map[*types.Func]*ast.FuncDecl, len(declarations))
	for object, function := range declarations {
		if function.Body == nil || function.Recv != nil || len(function.Body.List) != 1 {
			continue
		}
		functions[object] = function
	}
	result := make(map[*types.Func]ps6099Helper)
	scalarMemo := make(map[*types.Func]ps6099ScalarFunctionSummary)
	for changed := true; changed; {
		changed = false
		for object, function := range functions {
			if _, known := result[object]; known {
				continue
			}
			returned, ok := function.Body.List[0].(*ast.ReturnStmt)
			if !ok || len(returned.Results) != 1 {
				continue
			}
			call, ok := ps2110Unparen(returned.Results[0]).(*ast.CallExpr)
			if !ok {
				continue
			}
			operation, ok := ps6099DirectMathCall(pass, call)
			var parameterInputs []int
			if !ok {
				callee, _, resolved := typedCallee(pass, call.Fun)
				if !resolved {
					continue
				}
				calleeSummary := result[callee.Origin()]
				operation = calleeSummary.operation
				if operation == "" {
					continue
				}
				parameterInputs = ps6099ForwardedParameters(pass, function, call, calleeSummary.parameterInputs)
			} else {
				parameterInputs = ps6099ForwardedParameters(pass, function, call, ps6099ArgumentPositions(call.Args))
			}
			scalar := ps6099ExpressionScalarSummary(pass, call, result, declarations, scalarMemo)
			if !scalar.valid || scalar.count != 1 || scalar.operation != operation {
				continue
			}
			result[object] = ps6099Helper{operation: operation, parameterInputs: parameterInputs, call: call}
			changed = true
		}
	}
	return result
}

func ps6099LocalFunctionDeclarations(pass *analysis.Pass) map[*types.Func]*ast.FuncDecl {
	result := make(map[*types.Func]*ast.FuncDecl)
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			if object, ok := pass.TypesInfo.Defs[function.Name].(*types.Func); ok {
				result[object.Origin()] = function
			}
		}
	}
	return result
}

func ps6099ArgumentPositions(arguments []ast.Expr) []int {
	positions := make([]int, len(arguments))
	for index := range arguments {
		positions[index] = index
	}
	return positions
}

func ps6099ForwardedParameters(pass *analysis.Pass, function *ast.FuncDecl, call *ast.CallExpr, calleeInputs []int) []int {
	parameters := ps6099FunctionParameters(pass, function)
	seen := make([]bool, len(parameters))
	var result []int
	for _, calleePosition := range calleeInputs {
		if calleePosition < 0 || calleePosition >= len(call.Args) {
			continue
		}
		for position, parameter := range parameters {
			if !seen[position] && ps6099MentionsObject(pass, call.Args[calleePosition], parameter) {
				seen[position] = true
				result = append(result, position)
			}
		}
	}
	slices.Sort(result)
	return result
}

func ps6099FunctionParameters(pass *analysis.Pass, function *ast.FuncDecl) []types.Object {
	if function == nil || function.Type.Params == nil {
		return nil
	}
	var result []types.Object
	for _, field := range function.Type.Params.List {
		for _, name := range field.Names {
			result = append(result, pass.TypesInfo.ObjectOf(name))
		}
	}
	return result
}

func ps6099MentionsObject(pass *analysis.Pass, expression ast.Expr, target types.Object) bool {
	if target == nil {
		return false
	}
	found := false
	ast.Inspect(expression, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if ok && pass.TypesInfo.ObjectOf(identifier) == target {
			found = true
			return false
		}
		return !found
	})
	return found
}

type ps6099ScalarExpressionSummary struct {
	operation string
	count     int
	valid     bool
}

func ps6099ExpressionScalarSummary(pass *analysis.Pass, expression ast.Expr, helpers map[*types.Func]ps6099Helper, functions map[*types.Func]*ast.FuncDecl, memo map[*types.Func]ps6099ScalarFunctionSummary) ps6099ScalarExpressionSummary {
	result := ps6099ScalarExpressionSummary{valid: true}
	var inspect func(ast.Expr)
	inspect = func(candidate ast.Expr) {
		if candidate == nil || !result.valid {
			return
		}
		ast.Inspect(candidate, func(node ast.Node) bool {
			if !result.valid {
				return false
			}
			switch value := node.(type) {
			case *ast.FuncLit:
				// Merely creating a closure does not execute its body. An immediate
				// invocation is rejected as an opaque call by the CallExpr case.
				return false
			case *ast.IndexExpr:
				if ps6099RuntimeIndex(pass, value) {
					result.valid = false
					return false
				}
			case *ast.SliceExpr, *ast.TypeAssertExpr, *ast.CompositeLit:
				result.valid = false
				return false
			case *ast.UnaryExpr:
				if value.Op == token.AND || value.Op == token.ARROW || value.Op == token.MUL {
					result.valid = false
					return false
				}
			case *ast.SelectorExpr:
				if ps6099PointerFieldSelection(pass, value) {
					result.valid = false
					return false
				}
			case *ast.BinaryExpr:
				if ps6099RiskyIntegerOperation(pass, value) {
					result.valid = false
					return false
				}
				if value.Op != token.LAND && value.Op != token.LOR {
					return true
				}
				inspect(value.X)
				left, known := ps6099BooleanCondition(pass, value.X)
				if known && (value.Op == token.LAND && !left || value.Op == token.LOR && left) {
					return false
				}
				inspect(value.Y)
				return false
			case *ast.CallExpr:
				// Go evaluates the function value and every argument before invoking
				// the outer call. Inspect in that order so a non-returning or opaque
				// argument cannot leave the outer scalar call counted as reachable.
				inspect(value.Fun)
				for _, argument := range value.Args {
					inspect(argument)
				}
				if !result.valid {
					return false
				}
				operation, scalar := ps6099DirectMathCall(pass, value)
				var callee *types.Func
				if !scalar {
					if resolved, _, ok := typedCallee(pass, value.Fun); ok {
						callee = resolved.Origin()
						operation = helpers[callee.Origin()].operation
						scalar = operation != ""
					}
				}
				if scalar {
					if result.count == 0 {
						result.operation = operation
					}
					result.count = min(2, result.count+1)
				} else if !ps6099PureCall(pass, value) {
					summary := ps6099LocalScalarSummary(pass, callee, functions, memo, make(map[*types.Func]bool))
					if !summary.valid {
						result.valid = false
						return false
					}
					if !summary.returns {
						result.valid = false
						return false
					}
					ps6099AddScalarSummary(&result, summary.operation, summary.count)
				}
				return false
			}
			return true
		})
	}
	inspect(expression)
	return result
}

type ps6099ScalarFunctionSummary struct {
	operation string
	count     int
	valid     bool
	returns   bool
}

func ps6099AddScalarSummary(destination *ps6099ScalarExpressionSummary, operation string, count int) {
	if destination == nil || count <= 0 {
		return
	}
	if destination.count == 0 {
		destination.operation = operation
	}
	destination.count = min(2, destination.count+count)
}

func ps6099LocalScalarSummary(pass *analysis.Pass, function *types.Func, functions map[*types.Func]*ast.FuncDecl, memo map[*types.Func]ps6099ScalarFunctionSummary, active map[*types.Func]bool) ps6099ScalarFunctionSummary {
	if function == nil {
		return ps6099ScalarFunctionSummary{}
	}
	function = function.Origin()
	if state, known := memo[function]; known {
		return state
	}
	declaration := functions[function]
	if declaration == nil || declaration.Body == nil || active[function] {
		return ps6099ScalarFunctionSummary{}
	}
	active[function] = true
	defer delete(active, function)
	parents := ps6087Parents(declaration.Body)
	reachable := ps6099ReachableNodesInBlock(pass, declaration.Body, parents)
	result := ps6099ScalarExpressionSummary{valid: true}
	returns := ps6099AllPathsReturn(pass, declaration.Body, parents)
	ast.Inspect(declaration.Body, func(node ast.Node) bool {
		if !result.valid {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		if node == nil || !ps6099NodeReachable(pass, node, parents, reachable[node]) {
			return true
		}
		switch value := node.(type) {
		case *ast.CallExpr:
			if operation, scalar := ps6099DirectMathCall(pass, value); scalar {
				ps6099AddScalarSummary(&result, operation, 1)
				return true
			}
			if ps6099PureCall(pass, value) {
				return true
			}
			callee, _, resolved := typedCallee(pass, value.Fun)
			if !resolved {
				result.valid = false
				return false
			}
			nested := ps6099LocalScalarSummary(pass, callee, functions, memo, active)
			if !nested.valid || !nested.returns {
				result.valid = false
				return false
			}
			ps6099AddScalarSummary(&result, nested.operation, nested.count)
		case *ast.AssignStmt:
			for _, left := range value.Lhs {
				if !ps6099LocalMutation(pass, declaration, left) {
					result.valid = false
					return false
				}
			}
		case *ast.IncDecStmt:
			if !ps6099LocalMutation(pass, declaration, value.X) {
				result.valid = false
				return false
			}
		case *ast.SendStmt, *ast.GoStmt, *ast.DeferStmt, *ast.CompositeLit,
			*ast.TypeAssertExpr, *ast.SliceExpr:
			result.valid = false
			return false
		case *ast.IndexExpr:
			if ps6099RuntimeIndex(pass, value) {
				result.valid = false
				return false
			}
		case *ast.UnaryExpr:
			if value.Op == token.AND || value.Op == token.ARROW || value.Op == token.MUL {
				result.valid = false
				return false
			}
		case *ast.SelectorExpr:
			if ps6099PointerFieldSelection(pass, value) {
				result.valid = false
				return false
			}
		case *ast.BinaryExpr:
			if ps6099RiskyIntegerOperation(pass, value) {
				result.valid = false
				return false
			}
		}
		return true
	})
	summary := ps6099ScalarFunctionSummary{
		operation: result.operation,
		count:     result.count,
		valid:     result.valid,
		returns:   returns,
	}
	memo[function] = summary
	return summary
}

func ps6099LocalMutation(pass *analysis.Pass, function *ast.FuncDecl, expression ast.Expr) bool {
	identifier, ok := ps2110Unparen(expression).(*ast.Ident)
	if !ok || identifier.Name == "_" {
		return identifier != nil && identifier.Name == "_"
	}
	object, ok := pass.TypesInfo.ObjectOf(identifier).(*types.Var)
	if !ok || object.Parent() == pass.Pkg.Scope() {
		return false
	}
	return object.Pos() >= function.Pos() && object.Pos() <= function.End()
}

func ps6099PointerFieldSelection(pass *analysis.Pass, selector *ast.SelectorExpr) bool {
	selection := pass.TypesInfo.Selections[selector]
	if selection == nil || selection.Kind() != types.FieldVal && selection.Kind() != types.MethodVal {
		return false
	}
	if selection.Indirect() {
		return true
	}
	if selection.Kind() != types.FieldVal {
		return false
	}
	_, pointer := types.Unalias(selection.Recv()).(*types.Pointer)
	return pointer
}

func ps6099RiskyIntegerOperation(pass *analysis.Pass, expression *ast.BinaryExpr) bool {
	if expression == nil || expression.Op != token.QUO && expression.Op != token.REM && expression.Op != token.SHL && expression.Op != token.SHR {
		return false
	}
	typ := pass.TypesInfo.TypeOf(expression)
	if typ == nil {
		return true
	}
	typ = types.Unalias(typ)
	basic, ok := typ.Underlying().(*types.Basic)
	return ok && basic.Info()&types.IsInteger != 0
}

func ps6099RuntimeIndex(pass *analysis.Pass, expression *ast.IndexExpr) bool {
	return expression == nil || !pass.TypesInfo.Types[expression.Index].IsType()
}

func ps6099PureCall(pass *analysis.Pass, call *ast.CallExpr) bool {
	if call == nil {
		return false
	}
	if pass.TypesInfo.Types[call.Fun].IsType() {
		typ := types.Unalias(pass.TypesInfo.TypeOf(call))
		basic, ok := typ.Underlying().(*types.Basic)
		return ok && basic.Info()&types.IsNumeric != 0
	}
	if operation, direct := ps6099DirectMathIdentity(pass, call); direct {
		return operation == "Pow" && ps6099CheapPow(pass, call)
	}
	identifier, ok := ps2110Unparen(call.Fun).(*ast.Ident)
	if !ok {
		return false
	}
	builtin, ok := pass.TypesInfo.ObjectOf(identifier).(*types.Builtin)
	if !ok {
		return false
	}
	switch builtin.Name() {
	case "cap", "complex", "imag", "len", "max", "min", "real":
		return true
	}
	return false
}

func ps6099DirectMathCall(pass *analysis.Pass, call *ast.CallExpr) (string, bool) {
	operation, ok := ps6099DirectMathIdentity(pass, call)
	if !ok || operation == "Pow" && ps6099CheapPow(pass, call) {
		return "", false
	}
	return operation, true
}

func ps6099DirectMathIdentity(pass *analysis.Pass, call *ast.CallExpr) (string, bool) {
	if call == nil || call.Ellipsis.IsValid() {
		return "", false
	}
	function, signature, ok := typedCallee(pass, call.Fun)
	if !ok || function.Pkg() == nil || function.Pkg().Path() != "math" || signature.Recv() != nil ||
		signature.Variadic() || len(call.Args) != signature.Params().Len() || !ps6099Operations[function.Name()] {
		return "", false
	}
	return function.Name(), true
}

func ps6099CheapPow(pass *analysis.Pass, call *ast.CallExpr) bool {
	if len(call.Args) != 2 {
		return false
	}
	value := pass.TypesInfo.Types[call.Args[1]].Value
	if value == nil || value.Kind() != constant.Int && value.Kind() != constant.Float {
		return false
	}
	exponent, exact := constant.Float64Val(value)
	return exact && (exponent == 0 || exponent == 0.5 || exponent == -0.5 || exponent == 1 || exponent == -1 ||
		exponent == float64(int64(exponent)) && exponent >= -8 && exponent <= 8)
}

func ps6099CallOperation(pass *analysis.Pass, call *ast.CallExpr, helpers map[*types.Func]ps6099Helper) (string, bool) {
	if operation, ok := ps6099DirectMathCall(pass, call); ok {
		return operation, true
	}
	callee, signature, ok := typedCallee(pass, call.Fun)
	if !ok || signature.Recv() != nil || signature.Variadic() || call.Ellipsis.IsValid() || len(call.Args) != signature.Params().Len() {
		return "", false
	}
	operation := helpers[callee.Origin()].operation
	return operation, operation != ""
}

func ps6099Leaves(pass *analysis.Pass) map[string][]ps6099Leaf {
	result := make(map[string][]ps6099Leaf)
	sources := ps6077PackageSources(pass)
	typesByName := ps6099SyntaxTypes(sources)
	typeAliases := ps6099SyntaxTypeAliases(sources)
	functionsByName := ps6099SyntaxFunctions(sources)
	for _, source := range sources {
		for _, declaration := range source.file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok {
				continue
			}
			precision, ok := ps6099BatchPrecision(pass, function, typesByName)
			if !ok {
				continue
			}
			operation := ps6099OperationInName(function.Name.Name)
			operation, kind, score := ps6099VectorEvidence(pass, source.file, function, operation, precision, typesByName, functionsByName)
			if operation == "" || score == 0 {
				continue
			}
			result[operation] = append(result[operation], ps6099Leaf{
				operation: operation, name: function.Name.Name,
				precision: precision, sequences: ps6099LeafSequences(pass, function, typesByName),
				kind: kind, score: score, node: function.Name,
				syntaxTypes: typesByName, syntaxAliases: typeAliases,
			})
		}
	}
	for operation, leaves := range result {
		slices.SortFunc(leaves, func(left, right ps6099Leaf) int {
			if left.score != right.score {
				return right.score - left.score
			}
			return strings.Compare(left.name, right.name)
		})
		result[operation] = leaves
	}
	return result
}

func ps6099VectorEvidence(pass *analysis.Pass, file *ast.File, function *ast.FuncDecl, operation, precision string, typesByName map[string]ast.Expr, functionsByName map[string][]*ast.FuncDecl) (string, string, int) {
	if file == nil || function == nil {
		return "", "", 0
	}
	if function.Body == nil {
		if operation == "" {
			return "", "", 0
		}
		return operation, "an external assembly implementation", 3
	}
	imports := ps6077Imports(file)
	sequenceParameters := ps6099SequenceParameters{
		objects: make(map[types.Object]string),
		names:   make(map[string]string),
	}
	for _, field := range function.Type.Params.List {
		fieldPrecision := ps6099SequencePrecision(pass.TypesInfo.TypeOf(field.Type), field.Type, typesByName)
		if fieldPrecision == "" {
			continue
		}
		for _, name := range field.Names {
			if object := pass.TypesInfo.ObjectOf(name); object != nil {
				sequenceParameters.objects[object] = fieldPrecision
			} else {
				sequenceParameters.names[name.Name] = fieldPrecision
			}
		}
	}
	parents := ps6087Parents(function.Body)
	reachableCalls := ps6099ReachableCalls(pass, function, parents)
	type vectorCall struct {
		operation string
		display   string
		site      ast.Node
	}
	var vectorCalls []vectorCall
	ast.Inspect(function.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		name := ps6074CalledName(call.Fun)
		callOperation := ps6099OperationInName(name)
		if callOperation == "" || operation != "" && callOperation != operation ||
			!ps6099CalledLeafCompatible(pass, call, name, precision, imports, typesByName, functionsByName, sequenceParameters, function, parents) {
			return true
		}
		if calledPrecision := ps6099NamePrecision(name); calledPrecision != "" && calledPrecision != precision {
			return true
		}
		vector := ps6074VectorName(strings.ToLower(name))
		displayName := name
		if selector, ok := ps2110Unparen(call.Fun).(*ast.SelectorExpr); ok {
			if qualifier, ok := ps2110Unparen(selector.X).(*ast.Ident); ok {
				path := imports[qualifier.Name]
				vector = vector || path != "" && ps6074VectorName(strings.ToLower(qualifier.Name+"/"+path))
				displayName = qualifier.Name + "." + selector.Sel.Name
			}
		}
		if !vector {
			return true
		}
		site, synchronous := ps6099SynchronousCallSite(function.Body, call, parents)
		if !synchronous {
			return true
		}
		vectorCalls = append(vectorCalls, vectorCall{operation: callOperation, display: displayName, site: site})
		return true
	})
	grouped := make(map[string][]ast.Node, len(vectorCalls))
	display := make(map[string]string, len(vectorCalls))
	for _, candidate := range vectorCalls {
		grouped[candidate.operation] = append(grouped[candidate.operation], candidate.site)
		if display[candidate.operation] == "" {
			display[candidate.operation] = candidate.display
		}
	}
	if operation != "" {
		if ps6099ExecutionSitesCoverAllPaths(pass, function.Body, grouped[operation], parents) {
			return operation, "SIMD/vector-backed via " + display[operation], 2
		}
	} else if len(grouped) == 1 {
		for candidateOperation, sites := range grouped {
			if ps6099ExecutionSitesCoverAllPaths(pass, function.Body, sites, parents) {
				return candidateOperation, "SIMD/vector-backed via " + display[candidateOperation], 2
			}
		}
	}
	if operation != "" && ps6099DistinctLaneLoop(pass, function.Body, sequenceParameters) {
		return operation, "a multi-lane vector-width loop", 1
	}
	if operation == "" {
		if inferred := ps6099DistinctLaneOperation(pass, function.Body, sequenceParameters, parents, reachableCalls); inferred != "" {
			return inferred, "a multi-lane vector-width loop", 1
		}
	}
	return "", "", 0
}

func ps6099SynchronousCallSite(body *ast.BlockStmt, call *ast.CallExpr, parents map[ast.Node]ast.Node) (ast.Node, bool) {
	if body == nil || call == nil {
		return nil, false
	}
	site := ast.Node(call)
	for node := site; parents[node] != nil && parents[node] != body; {
		parent := parents[node]
		switch value := parent.(type) {
		case *ast.GoStmt, *ast.DeferStmt:
			return nil, false
		case *ast.FuncLit:
			container := parents[value]
			for {
				if _, ok := container.(*ast.ParenExpr); !ok {
					break
				}
				container = parents[container]
			}
			invocation, ok := container.(*ast.CallExpr)
			if !ok || ps2110Unparen(invocation.Fun) != value {
				return nil, false
			}
			node = invocation
			continue
		}
		node = parent
	}
	return site, true
}

func ps6099ExecutionSitesCoverAllPaths(pass *analysis.Pass, body *ast.BlockStmt, sites []ast.Node, parents map[ast.Node]ast.Node) bool {
	if body == nil || len(sites) == 0 {
		return false
	}
	graph := cfg.New(body, func(call *ast.CallExpr) bool {
		return !typedBuiltinName(pass, call.Fun, "panic")
	})
	if len(graph.Blocks) == 0 {
		return false
	}
	visiting := make(map[*cfg.Block]bool, len(graph.Blocks))
	covered := make(map[*cfg.Block]bool, len(graph.Blocks))
	var covers func(*cfg.Block) bool
	covers = func(block *cfg.Block) bool {
		if block == nil {
			return false
		}
		if result, known := covered[block]; known {
			return result
		}
		if visiting[block] {
			return false
		}
		visiting[block] = true
		defer delete(visiting, block)
		for _, root := range block.Nodes {
			for _, site := range sites {
				if ps6099NodeWithin(site, root) && ps6099AlwaysEvaluatedWithin(pass, site, root, sites, parents) {
					covered[block] = true
					return true
				}
			}
		}
		successors := ps6099ReachableSuccessors(pass, block, parents)
		if len(successors) == 0 {
			covered[block] = false
			return false
		}
		for _, successor := range successors {
			if !covers(successor) {
				covered[block] = false
				return false
			}
		}
		covered[block] = true
		return true
	}
	return covers(graph.Blocks[0])
}

func ps6099AlwaysEvaluatedWithin(pass *analysis.Pass, candidate, root ast.Node, sites []ast.Node, parents map[ast.Node]ast.Node) bool {
	for node := candidate; parents[node] != nil && parents[node] != root; {
		parent := parents[node]
		switch value := parent.(type) {
		case *ast.GoStmt, *ast.DeferStmt:
			return false
		case *ast.FuncLit:
			container := parents[value]
			for {
				if _, ok := container.(*ast.ParenExpr); !ok {
					break
				}
				container = parents[container]
			}
			invocation, ok := container.(*ast.CallExpr)
			if !ok || ps2110Unparen(invocation.Fun) != value {
				return false
			}
			var nested []ast.Node
			for _, site := range sites {
				if ps6099NodeWithin(site, value.Body) {
					nested = append(nested, site)
				}
			}
			if !ps6099ExecutionSitesCoverAllPaths(pass, value.Body, nested, parents) {
				return false
			}
			node = invocation
			continue
		case *ast.BinaryExpr:
			if value.Op != token.LAND && value.Op != token.LOR || !ps6099NodeWithin(node, value.Y) {
				continue
			}
			left, known := ps6099BooleanCondition(pass, value.X)
			if !known || value.Op == token.LAND && !left || value.Op == token.LOR && left {
				return false
			}
		}
		node = parent
	}
	return true
}

func ps6099CalledLeafCompatible(pass *analysis.Pass, call *ast.CallExpr, name, precision string, imports map[string]string, typesByName map[string]ast.Expr, functionsByName map[string][]*ast.FuncDecl, parameters ps6099SequenceParameters, enclosing *ast.FuncDecl, parents map[ast.Node]ast.Node) bool {
	if calledPrecision := ps6099NamePrecision(name); calledPrecision != "" && calledPrecision != precision {
		return false
	}
	if function, signature, ok := typedCallee(pass, call.Fun); ok {
		if !ps6099ConcreteLeafCallee(pass, call, function, signature) {
			return false
		}
		calledPrecision, valid := ps6099SignaturePrecision(signature)
		return valid && calledPrecision == precision && ps6099CallHasSequenceFormal(pass, call, signature, parameters, precision)
	}
	if selector, ok := ps2110Unparen(call.Fun).(*ast.SelectorExpr); ok {
		qualifier, qualified := ps2110Unparen(selector.X).(*ast.Ident)
		if !qualified || pass.TypesInfo.ObjectOf(qualifier) != nil || ps6099SyntaxIdentifierBound(enclosing, qualifier, parents) {
			return false
		}
		path := imports[qualifier.Name]
		if imported := ps6099ImportedPackage(pass.Pkg, path); imported != nil {
			if function, ok := imported.Scope().Lookup(selector.Sel.Name).(*types.Func); ok {
				signature, ok := function.Type().(*types.Signature)
				if !ok {
					return false
				}
				if _, valid := ps6099CallSignatureOffset(pass, call, signature); !valid {
					return false
				}
				calledPrecision, valid := ps6099SignaturePrecision(signature)
				return valid && calledPrecision == precision && ps6099CallHasSequenceFormal(pass, call, signature, parameters, precision)
			}
		}
		return false
	}
	identifier, ok := ps2110Unparen(call.Fun).(*ast.Ident)
	if !ok {
		return false
	}
	// A typed identifier has an authoritative lexical binding. In particular,
	// a callback variable deliberately named ExpSIMDF64 is not evidence for a
	// package leaf merely because an ignored sibling has that spelling.
	if pass.TypesInfo.ObjectOf(identifier) != nil || ps6099SyntaxIdentifierBound(enclosing, identifier, parents) {
		return false
	}
	for _, function := range functionsByName[identifier.Name] {
		if call.Ellipsis.IsValid() || ps6099SyntaxParameterCount(function.Type.Params) != len(call.Args) {
			continue
		}
		calledPrecision, valid := ps6099BatchPrecision(pass, function, typesByName)
		if valid && calledPrecision == precision && ps6099SyntaxCallHasSequenceFormal(pass, call, function, parameters, precision, typesByName) {
			return true
		}
	}
	return false
}

func ps6099SyntaxIdentifierBound(function *ast.FuncDecl, identifier *ast.Ident, parents map[ast.Node]ast.Node) bool {
	if function == nil || identifier == nil || identifier.Name == "" {
		return false
	}
	name := identifier.Name
	if ps6099SyntaxFieldsBind(function.Recv, name) || ps6099SyntaxFieldsBind(function.Type.Params, name) ||
		ps6099SyntaxFieldsBind(function.Type.Results, name) {
		return true
	}
	before := identifier.Pos()
	for node, parent := ast.Node(identifier), parents[identifier]; parent != nil; node, parent = parent, parents[parent] {
		switch value := parent.(type) {
		case *ast.BlockStmt:
			if ps6099SyntaxStatementsBindBefore(value.List, before, name) {
				return true
			}
		case *ast.CaseClause:
			if ps6099SyntaxStatementsBindBefore(value.Body, before, name) {
				return true
			}
		case *ast.CommClause:
			if value.Comm != nil && value.Comm.End() < before && ps6099SyntaxStatementBinds(value.Comm, name) ||
				ps6099SyntaxStatementsBindBefore(value.Body, before, name) {
				return true
			}
		case *ast.IfStmt:
			if value.Init != nil && value.Init.End() < before && ps6099SyntaxStatementBinds(value.Init, name) {
				return true
			}
		case *ast.ForStmt:
			if value.Init != nil && value.Init.End() < before && ps6099SyntaxStatementBinds(value.Init, name) {
				return true
			}
		case *ast.RangeStmt:
			if value.Tok == token.DEFINE && ps6099NodeWithin(node, value.Body) &&
				(ps6099SyntaxExpressionBinds(value.Key, name) || ps6099SyntaxExpressionBinds(value.Value, name)) {
				return true
			}
		case *ast.SwitchStmt:
			if value.Init != nil && value.Init.End() < before && ps6099SyntaxStatementBinds(value.Init, name) {
				return true
			}
		case *ast.TypeSwitchStmt:
			if value.Init != nil && value.Init.End() < before && ps6099SyntaxStatementBinds(value.Init, name) {
				return true
			}
			if value.Assign != nil && value.Assign.End() < before && ps6099SyntaxStatementBinds(value.Assign, name) {
				return true
			}
		}
	}
	return false
}

func ps6099SyntaxFieldsBind(fields *ast.FieldList, name string) bool {
	if fields == nil {
		return false
	}
	for _, field := range fields.List {
		for _, candidate := range field.Names {
			if candidate.Name == name {
				return true
			}
		}
	}
	return false
}

func ps6099SyntaxStatementsBindBefore(statements []ast.Stmt, before token.Pos, name string) bool {
	for _, statement := range statements {
		if statement.Pos() >= before {
			break
		}
		if statement.End() < before && ps6099SyntaxStatementBinds(statement, name) {
			return true
		}
	}
	return false
}

func ps6099SyntaxStatementBinds(statement ast.Stmt, name string) bool {
	switch value := statement.(type) {
	case *ast.AssignStmt:
		if value.Tok != token.DEFINE {
			return false
		}
		for _, left := range value.Lhs {
			if ps6099SyntaxExpressionBinds(left, name) {
				return true
			}
		}
	case *ast.DeclStmt:
		declaration, ok := value.Decl.(*ast.GenDecl)
		if !ok {
			return false
		}
		for _, specification := range declaration.Specs {
			switch item := specification.(type) {
			case *ast.ValueSpec:
				for _, candidate := range item.Names {
					if candidate.Name == name {
						return true
					}
				}
			case *ast.TypeSpec:
				if item.Name.Name == name {
					return true
				}
			}
		}
	case *ast.LabeledStmt:
		return ps6099SyntaxStatementBinds(value.Stmt, name)
	}
	return false
}

func ps6099SyntaxExpressionBinds(expression ast.Expr, name string) bool {
	identifier, ok := ps2110Unparen(expression).(*ast.Ident)
	return ok && identifier.Name != "_" && identifier.Name == name
}

func ps6099ReachableCalls(pass *analysis.Pass, function *ast.FuncDecl, parents map[ast.Node]ast.Node) map[*ast.CallExpr]bool {
	if function == nil || function.Body == nil {
		return make(map[*ast.CallExpr]bool)
	}
	return ps6099ReachableCallsInBlock(pass, function.Body, parents)
}

func ps6099ReachableCallsInBlock(pass *analysis.Pass, body *ast.BlockStmt, parents map[ast.Node]ast.Node) map[*ast.CallExpr]bool {
	result := make(map[*ast.CallExpr]bool)
	for node := range ps6099ReachableNodesInBlock(pass, body, parents) {
		if call, ok := node.(*ast.CallExpr); ok {
			result[call] = true
		}
	}
	return result
}

func ps6099AllPathsReturn(pass *analysis.Pass, body *ast.BlockStmt, parents map[ast.Node]ast.Node) bool {
	if body == nil {
		return false
	}
	graph := cfg.New(body, func(call *ast.CallExpr) bool {
		return !typedBuiltinName(pass, call.Fun, "panic")
	})
	if len(graph.Blocks) == 0 {
		return false
	}
	seen := make(map[*cfg.Block]bool, len(graph.Blocks))
	liveStatements := make(map[ast.Stmt]bool)
	pending := []*cfg.Block{graph.Blocks[0]}
	for len(pending) > 0 {
		block := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if block == nil || seen[block] {
			continue
		}
		seen[block] = true
		if block.Stmt != nil {
			liveStatements[block.Stmt] = true
		}
		if len(block.Succs) == 0 && block.Return() == nil {
			return false
		}
		pending = append(pending, ps6099ReachableSuccessors(pass, block, parents)...)
	}
	allFinite := true
	ast.Inspect(body, func(node ast.Node) bool {
		if !allFinite {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		switch loop := node.(type) {
		case *ast.ForStmt:
			if !liveStatements[loop] {
				return false
			}
			if loop.Cond != nil {
				if _, known := ps6099ForExactIterations(pass, loop); known {
					return true
				}
			}
			allFinite = false
			return false
		case *ast.RangeStmt:
			if !liveStatements[loop] {
				return false
			}
			typ := pass.TypesInfo.TypeOf(loop.X)
			if typ != nil {
				switch underlying := types.Unalias(typ).Underlying().(type) {
				case *types.Array, *types.Slice, *types.Map:
					return true
				case *types.Basic:
					if underlying.Info()&types.IsInteger != 0 || underlying.Info()&types.IsString != 0 {
						return true
					}
				}
			}
			allFinite = false
			return false
		}
		return true
	})
	return allFinite
}

func ps6099ReachableNodesInBlock(pass *analysis.Pass, body *ast.BlockStmt, parents map[ast.Node]ast.Node) map[ast.Node]bool {
	result := make(map[ast.Node]bool)
	if body == nil {
		return result
	}
	graph := cfg.New(body, func(call *ast.CallExpr) bool {
		return !typedBuiltinName(pass, call.Fun, "panic")
	})
	if len(graph.Blocks) == 0 {
		return result
	}
	seen := make(map[*cfg.Block]bool, len(graph.Blocks))
	pending := []*cfg.Block{graph.Blocks[0]}
	for len(pending) > 0 {
		block := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if block == nil || seen[block] {
			continue
		}
		seen[block] = true
		for _, root := range block.Nodes {
			ast.Inspect(root, func(node ast.Node) bool {
				if node == nil {
					return false
				}
				if _, nested := node.(*ast.FuncLit); nested {
					return false
				}
				result[node] = true
				return true
			})
		}
		pending = append(pending, ps6099ReachableSuccessors(pass, block, parents)...)
	}
	return result
}

func ps6099ReachableSuccessors(pass *analysis.Pass, block *cfg.Block, parents map[ast.Node]ast.Node) []*cfg.Block {
	if block == nil || len(block.Succs) != 2 || len(block.Nodes) == 0 {
		return block.Succs
	}
	condition, ok := block.Nodes[len(block.Nodes)-1].(ast.Expr)
	if !ok {
		return block.Succs
	}
	truth, known := ps6099ControlCondition(pass, condition, parents)
	if !known {
		return block.Succs
	}
	if truth {
		return block.Succs[:1]
	}
	return block.Succs[1:]
}

func ps6099ControlCondition(pass *analysis.Pass, expression ast.Expr, parents map[ast.Node]ast.Node) (bool, bool) {
	if loop, ok := parents[expression].(*ast.ForStmt); ok && loop.Cond == expression && ps6099ForNeverTerminates(pass, loop) {
		return true, true
	}
	if clause, caseExpression := parents[expression].(*ast.CaseClause); caseExpression {
		statement, ok := parents[parents[clause]].(*ast.SwitchStmt)
		if !ok {
			return false, false
		}
		return ps6099SwitchCaseCondition(pass, statement, expression)
	}
	return ps6099BooleanCondition(pass, expression)
}

func ps6099SwitchCaseCondition(pass *analysis.Pass, statement *ast.SwitchStmt, candidateExpression ast.Expr) (bool, bool) {
	if statement == nil {
		return false, false
	}
	interfaceTag := statement.Tag != nil && ps6099InterfaceType(pass.TypesInfo.TypeOf(statement.Tag))
	tag, tagType, tagKnown := constant.MakeBool(true), types.Type(nil), true
	if statement.Tag != nil {
		tag, tagType, tagKnown = ps6099SwitchValue(pass, statement.Tag, interfaceTag)
	}
	candidate, candidateType, candidateKnown := ps6099SwitchValue(pass, candidateExpression, interfaceTag)
	if !tagKnown || !candidateKnown {
		return false, false
	}
	if interfaceTag && !types.Identical(tagType, candidateType) {
		return false, true
	}
	return constant.Compare(tag, token.EQL, candidate), true
}

func ps6099SwitchValue(pass *analysis.Pass, expression ast.Expr, interfaceValue bool) (constant.Value, types.Type, bool) {
	if expression == nil {
		return nil, nil, false
	}
	expression = ps2110Unparen(expression)
	if interfaceValue {
		if conversion, ok := expression.(*ast.CallExpr); ok && len(conversion.Args) == 1 &&
			pass.TypesInfo.Types[conversion.Fun].IsType() && ps6099InterfaceType(pass.TypesInfo.TypeOf(conversion)) {
			return ps6099SwitchValue(pass, conversion.Args[0], true)
		}
	}
	value := pass.TypesInfo.Types[expression].Value
	if value == nil {
		if boolean, known := ps6099BooleanCondition(pass, expression); known {
			value = constant.MakeBool(boolean)
		} else {
			return nil, nil, false
		}
	}
	if !interfaceValue {
		return value, nil, true
	}
	dynamicType := pass.TypesInfo.TypeOf(expression)
	if dynamicType == nil || ps6099InterfaceType(dynamicType) {
		return nil, nil, false
	}
	return value, types.Default(dynamicType), true
}

func ps6099InterfaceType(value types.Type) bool {
	if value == nil {
		return false
	}
	_, ok := types.Unalias(value).Underlying().(*types.Interface)
	return ok
}

func ps6099BooleanCondition(pass *analysis.Pass, expression ast.Expr) (bool, bool) {
	if expression == nil {
		return false, false
	}
	expression = ps2110Unparen(expression)
	if value := pass.TypesInfo.Types[expression].Value; value != nil && value.Kind() == constant.Bool {
		return constant.BoolVal(value), true
	}
	if unary, ok := expression.(*ast.UnaryExpr); ok && unary.Op == token.NOT {
		value, known := ps6099BooleanCondition(pass, unary.X)
		return !value, known
	}
	binary, ok := expression.(*ast.BinaryExpr)
	if !ok || binary.Op != token.LAND && binary.Op != token.LOR {
		return false, false
	}
	left, leftKnown := ps6099BooleanCondition(pass, binary.X)
	right, rightKnown := ps6099BooleanCondition(pass, binary.Y)
	switch binary.Op {
	case token.LAND:
		if leftKnown && !left || rightKnown && !right {
			return false, true
		}
		if leftKnown && left {
			return right, rightKnown
		}
	case token.LOR:
		if leftKnown && left || rightKnown && right {
			return true, true
		}
		if leftKnown && !left {
			return right, rightKnown
		}
	}
	return false, false
}

func ps6099CallReachable(pass *analysis.Pass, call *ast.CallExpr, parents map[ast.Node]ast.Node, controlFlowReachable bool) bool {
	return ps6099NodeReachable(pass, call, parents, controlFlowReachable)
}

func ps6099NodeReachable(pass *analysis.Pass, candidate ast.Node, parents map[ast.Node]ast.Node, controlFlowReachable bool) bool {
	if candidate == nil || !controlFlowReachable {
		return false
	}
	for node, parent := candidate, parents[candidate]; parent != nil; node, parent = parent, parents[parent] {
		switch value := parent.(type) {
		case *ast.BinaryExpr:
			if node == value.Y || ps6099NodeWithin(node, value.Y) {
				left, known := ps6099BooleanCondition(pass, value.X)
				if known && (value.Op == token.LAND && !left || value.Op == token.LOR && left) {
					return false
				}
			}
		case *ast.ForStmt:
			if ps6099NodeWithin(node, value.Body) {
				if iterations, known := ps6099ForExactIterations(pass, value); known && iterations == 0 {
					return false
				}
			}
		case *ast.RangeStmt:
			if ps6099NodeWithin(node, value.Body) {
				if iterations, known := ps6099RangeExactIterations(pass, value.X); known && iterations == 0 {
					return false
				}
			}
		}
	}
	return true
}

func ps6099NodeWithin(node, container ast.Node) bool {
	return node != nil && container != nil && node.Pos() >= container.Pos() && node.End() <= container.End()
}

func ps6099CallHasSequenceFormal(pass *analysis.Pass, call *ast.CallExpr, signature *types.Signature, parameters ps6099SequenceParameters, precision string) bool {
	offset, ok := ps6099CallSignatureOffset(pass, call, signature)
	if !ok {
		return false
	}
	if receiver := signature.Recv(); receiver != nil {
		candidate, sequence := ps6099TypeSequencePrecision(receiver.Type())
		if sequence && candidate == precision {
			if expression := ps6099CallReceiverExpression(pass, call); expression != nil &&
				ps6099ArgumentMentionsSequenceParameter(pass, expression, parameters, precision) {
				return true
			}
		}
	}
	for index := 0; index < signature.Params().Len() && index+offset < len(call.Args); index++ {
		candidate, sequence := ps6099TypeSequencePrecision(signature.Params().At(index).Type())
		if sequence && candidate == precision && ps6099ArgumentMentionsSequenceParameter(pass, call.Args[index+offset], parameters, precision) {
			return true
		}
	}
	return false
}

func ps6099ConcreteLeafCallee(pass *analysis.Pass, call *ast.CallExpr, function *types.Func, signature *types.Signature) bool {
	if function == nil || signature == nil {
		return false
	}
	if signature.Recv() == nil {
		_, valid := ps6099CallSignatureOffset(pass, call, signature)
		return valid
	}
	selector := ps6099CallSelector(call.Fun)
	selection := pass.TypesInfo.Selections[selector]
	if selection == nil {
		return false
	}
	receiver := types.Unalias(selection.Recv())
	if pointer, ok := receiver.(*types.Pointer); ok {
		receiver = types.Unalias(pointer.Elem())
	}
	if _, ok := receiver.(*types.TypeParam); ok {
		return false
	}
	if _, dynamic := receiver.Underlying().(*types.Interface); dynamic {
		return false
	}
	_, valid := ps6099CallSignatureOffset(pass, call, signature)
	return valid
}

func ps6099CallSignatureOffset(pass *analysis.Pass, call *ast.CallExpr, signature *types.Signature) (int, bool) {
	if call == nil || signature == nil || call.Ellipsis.IsValid() {
		return 0, false
	}
	offset := 0
	if signature.Recv() != nil {
		selector := ps6099CallSelector(call.Fun)
		selection := pass.TypesInfo.Selections[selector]
		if selection == nil {
			return 0, false
		}
		if selection.Kind() == types.MethodExpr {
			offset = 1
		}
	}
	return offset, len(call.Args) == signature.Params().Len()+offset
}

func ps6099CallReceiverExpression(pass *analysis.Pass, call *ast.CallExpr) ast.Expr {
	if call == nil {
		return nil
	}
	selector := ps6099CallSelector(call.Fun)
	selection := pass.TypesInfo.Selections[selector]
	if selection == nil {
		return nil
	}
	if selection.Kind() == types.MethodExpr {
		if len(call.Args) == 0 {
			return nil
		}
		return call.Args[0]
	}
	return selector.X
}

func ps6099CallSelector(expression ast.Expr) *ast.SelectorExpr {
	expression = ps2110Unparen(expression)
	switch value := expression.(type) {
	case *ast.IndexExpr:
		return ps6099CallSelector(value.X)
	case *ast.IndexListExpr:
		return ps6099CallSelector(value.X)
	case *ast.SelectorExpr:
		return value
	}
	return nil
}

func ps6099SyntaxCallHasSequenceFormal(pass *analysis.Pass, call *ast.CallExpr, function *ast.FuncDecl, parameters ps6099SequenceParameters, precision string, typesByName map[string]ast.Expr) bool {
	position := 0
	for _, field := range function.Type.Params.List {
		candidate, sequence := ps6099SequenceKindPrecision(pass.TypesInfo.TypeOf(field.Type), field.Type, typesByName, make(map[string]bool))
		count := max(1, len(field.Names))
		for range count {
			if position < len(call.Args) && sequence && candidate == precision &&
				ps6099ArgumentMentionsSequenceParameter(pass, call.Args[position], parameters, precision) {
				return true
			}
			position++
		}
	}
	return false
}

func ps6099ArgumentMentionsSequenceParameter(pass *analysis.Pass, argument ast.Expr, parameters ps6099SequenceParameters, precision string) bool {
	found := false
	ast.Inspect(argument, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if !ok {
			return !found
		}
		object := pass.TypesInfo.ObjectOf(identifier)
		if object != nil && parameters.objects[object] == precision || object == nil && parameters.names[identifier.Name] == precision {
			found = true
			return false
		}
		return !found
	})
	return found
}

func ps6099SyntaxParameterCount(parameters *ast.FieldList) int {
	if parameters == nil {
		return 0
	}
	count := 0
	for _, field := range parameters.List {
		if len(field.Names) == 0 {
			count++
		} else {
			count += len(field.Names)
		}
	}
	return count
}

func ps6099ImportedPackage(root *types.Package, path string) *types.Package {
	if root == nil || path == "" {
		return nil
	}
	for _, imported := range root.Imports() {
		if imported.Path() == path {
			return imported
		}
	}
	return nil
}

func ps6099SignaturePrecision(signature *types.Signature) (string, bool) {
	if signature == nil || signature.Variadic() || signature.Results().Len() != 0 {
		return "", false
	}
	precision := ""
	found := false
	if receiver := signature.Recv(); receiver != nil {
		candidate, sequence := ps6099TypeSequencePrecision(receiver.Type())
		if sequence {
			found = true
			if candidate == "" {
				return "", false
			}
			precision = candidate
		}
	}
	for index := 0; index < signature.Params().Len(); index++ {
		candidate, sequence := ps6099TypeSequencePrecision(signature.Params().At(index).Type())
		if !sequence {
			continue
		}
		found = true
		if candidate == "" || precision != "" && precision != candidate {
			return "", false
		}
		precision = candidate
	}
	return precision, found && precision != ""
}

func ps6099TypeSequencePrecision(typ types.Type) (string, bool) {
	if typ == nil {
		return "", false
	}
	switch value := types.Unalias(typ).Underlying().(type) {
	case *types.Pointer:
		return ps6099TypeSequencePrecision(value.Elem())
	case *types.Slice:
		return ps6099Precision(value.Elem()), true
	case *types.Array:
		return ps6099Precision(value.Elem()), true
	}
	return "", false
}

func ps6099SyntaxFunctions(sources []ps6077Source) map[string][]*ast.FuncDecl {
	result := make(map[string][]*ast.FuncDecl)
	for _, source := range sources {
		for _, declaration := range source.file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if ok && function.Recv == nil {
				result[function.Name.Name] = append(result[function.Name.Name], function)
			}
		}
	}
	return result
}

func ps6099DistinctLaneLoop(pass *analysis.Pass, body *ast.BlockStmt, parameters ps6099SequenceParameters) bool {
	found := false
	parents := ps6087Parents(body)
	reachable := ps6099ReachableNodesInBlock(pass, body, parents)
	ast.Inspect(body, func(node ast.Node) bool {
		if found {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		loop, ok := node.(*ast.ForStmt)
		if !ok {
			return true
		}
		if loop.Cond == nil || !ps6099ExecutionSitesCoverAllPaths(pass, body, []ast.Node{loop.Cond}, parents) {
			return true
		}
		step := ps6077LiteralStep(loop.Post)
		indexName := ps6077LoopIndexName(loop)
		if step < 2 || indexName == "" {
			return true
		}
		aliases := ps6099SequenceAliases(pass, body, loop.Pos(), parents, parameters)
		offsets := make(map[string]map[int64]bool)
		ast.Inspect(loop.Body, func(candidate ast.Node) bool {
			if _, nested := candidate.(*ast.FuncLit); nested {
				return false
			}
			index, ok := candidate.(*ast.IndexExpr)
			if !ok || !reachable[index] {
				return true
			}
			sequence := ps6099SequenceBaseKey(pass, index.X, aliases)
			if sequence == "" {
				return true
			}
			offset, ok := ps6099LaneOffset(index.Index, indexName)
			if ok && offset >= 0 && offset < step {
				if offsets[sequence] == nil {
					offsets[sequence] = make(map[int64]bool, step)
				}
				offsets[sequence][offset] = true
			}
			return len(offsets[sequence]) < 2
		})
		for _, lanes := range offsets {
			found = found || len(lanes) >= 2
		}
		return !found
	})
	return found
}

func ps6099DistinctLaneOperation(pass *analysis.Pass, body *ast.BlockStmt, parameters ps6099SequenceParameters, parents map[ast.Node]ast.Node, reachableCalls map[*ast.CallExpr]bool) string {
	operations := make(map[string]bool)
	linkedOperations := make(map[string]bool)
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		loop, ok := node.(*ast.ForStmt)
		if !ok {
			return true
		}
		if loop.Cond == nil || !ps6099ExecutionSitesCoverAllPaths(pass, body, []ast.Node{loop.Cond}, parents) {
			return true
		}
		step := ps6077LiteralStep(loop.Post)
		indexName := ps6077LoopIndexName(loop)
		if step < 2 || indexName == "" {
			return true
		}
		aliases := ps6099SequenceAliases(pass, body, loop.Pos(), parents, parameters)
		offsets := make(map[string]map[string]map[int64]bool)
		astutil.WithStack(loop.Body, func(candidate ast.Node, stack []ast.Node) bool {
			if _, nested := candidate.(*ast.FuncLit); nested {
				return false
			}
			call, ok := candidate.(*ast.CallExpr)
			if !ok || !ps6099CallReachable(pass, call, parents, reachableCalls[call]) || ps6099Precision(pass.TypesInfo.TypeOf(call)) == "" {
				return true
			}
			operation, direct := ps6099DirectMathIdentity(pass, call)
			if !direct {
				operation = ps6099OperationInName(ps6074CalledName(call.Fun))
			}
			if operation == "" {
				return true
			}
			_, output := ps6099CallAssignment(call, stack)
			if output == nil {
				return true
			}
			sequence := ps6099SequenceBaseKey(pass, output.X, aliases)
			offset, valid := ps6099LaneOffset(output.Index, indexName)
			if sequence == "" || !valid || offset < 0 || offset >= step {
				return true
			}
			linkedOperations[operation] = true
			operationOffsets := offsets[operation]
			if operationOffsets == nil {
				operationOffsets = make(map[string]map[int64]bool)
				offsets[operation] = operationOffsets
			}
			lanes := operationOffsets[sequence]
			if lanes == nil {
				lanes = make(map[int64]bool, step)
				operationOffsets[sequence] = lanes
			}
			lanes[offset] = true
			return true
		})
		for operation, sequences := range offsets {
			for _, lanes := range sequences {
				if len(lanes) >= 2 {
					operations[operation] = true
				}
			}
		}
		return true
	})
	if len(operations) != 1 || len(linkedOperations) != 1 {
		return ""
	}
	for operation := range operations {
		return operation
	}
	return ""
}

func ps6099SequenceAliases(pass *analysis.Pass, body *ast.BlockStmt, before token.Pos, parents map[ast.Node]ast.Node, parameters ps6099SequenceParameters) map[string]string {
	aliases := make(map[string]string, len(parameters.objects)+len(parameters.names))
	for object := range parameters.objects {
		key := ps6099ObjectKey(object)
		aliases[key] = key
	}
	for name := range parameters.names {
		key := "syntax:" + name
		aliases[key] = key
	}
	ast.Inspect(body, func(node ast.Node) bool {
		if node == nil || node.Pos() >= before {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		if !ps6099StraightLineBefore(node, body, parents) {
			switch value := node.(type) {
			case *ast.AssignStmt:
				for _, left := range value.Lhs {
					ps6099InvalidateSequenceAlias(pass, left, aliases)
				}
			case *ast.ValueSpec:
				for _, name := range value.Names {
					ps6099InvalidateSequenceAlias(pass, name, aliases)
				}
			}
			return true
		}
		switch value := node.(type) {
		case *ast.AssignStmt:
			if len(value.Lhs) != len(value.Rhs) {
				return true
			}
			for index := range value.Lhs {
				ps6099UpdateSequenceAlias(pass, value.Lhs[index], value.Rhs[index], aliases)
			}
		case *ast.ValueSpec:
			if len(value.Names) != len(value.Values) {
				return true
			}
			for index, name := range value.Names {
				ps6099UpdateSequenceAlias(pass, name, value.Values[index], aliases)
			}
		}
		return true
	})
	return aliases
}

func ps6099InvalidateSequenceAlias(pass *analysis.Pass, expression ast.Expr, aliases map[string]string) {
	if storage, ok := ps6099StorageOf(pass, expression); ok {
		ps6099ClearSequenceAlias(aliases, storage.key)
	}
}

func ps6099ClearSequenceAlias(aliases map[string]string, key string) {
	delete(aliases, key)
	for candidate := range aliases {
		if strings.HasPrefix(candidate, key+"/") {
			delete(aliases, candidate)
		}
	}
}

func ps6099StraightLineBefore(node ast.Node, body *ast.BlockStmt, parents map[ast.Node]ast.Node) bool {
	for parent := parents[node]; parent != nil && parent != body; parent = parents[parent] {
		switch parent.(type) {
		case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt, *ast.SwitchStmt, *ast.TypeSwitchStmt,
			*ast.SelectStmt, *ast.CaseClause, *ast.CommClause:
			return false
		}
	}
	return true
}

func ps6099UpdateSequenceAlias(pass *analysis.Pass, left, right ast.Expr, aliases map[string]string) {
	storage, ok := ps6099StorageOf(pass, left)
	if !ok {
		return
	}
	canonical := ps6099SequenceBaseKey(pass, right, aliases)
	ps6099ClearSequenceAlias(aliases, storage.key)
	if canonical != "" {
		aliases[storage.key] = canonical
	}
}

func ps6099SequenceBaseKey(pass *analysis.Pass, expression ast.Expr, aliases map[string]string) string {
	switch value := ps2110Unparen(expression).(type) {
	case *ast.SliceExpr:
		return ps6099SequenceBaseKey(pass, value.X, aliases)
	case *ast.IndexListExpr, *ast.IndexExpr:
		return ""
	}
	if storage, ok := ps6099StorageOf(pass, expression); ok {
		return aliases[storage.key]
	}
	if identifier, ok := ps2110Unparen(expression).(*ast.Ident); ok && pass.TypesInfo.ObjectOf(identifier) == nil {
		return aliases["syntax:"+identifier.Name]
	}
	return ""
}

func ps6099LaneOffset(expression ast.Expr, indexName string) (int64, bool) {
	switch value := ps2110Unparen(expression).(type) {
	case *ast.Ident:
		return 0, value.Name == indexName
	case *ast.BinaryExpr:
		leftOffset, leftIndex := ps6099LaneOffset(value.X, indexName)
		rightOffset, rightIndex := ps6099LaneOffset(value.Y, indexName)
		switch value.Op {
		case token.ADD:
			if leftIndex {
				constantOffset, ok := ps6099SyntaxInteger(value.Y)
				return leftOffset + constantOffset, ok
			}
			if rightIndex {
				constantOffset, ok := ps6099SyntaxInteger(value.X)
				return rightOffset + constantOffset, ok
			}
		case token.SUB:
			if leftIndex {
				constantOffset, ok := ps6099SyntaxInteger(value.Y)
				return leftOffset - constantOffset, ok
			}
		}
	}
	return 0, false
}

func ps6099SyntaxInteger(expression ast.Expr) (int64, bool) {
	literal, ok := ps2110Unparen(expression).(*ast.BasicLit)
	if !ok || literal.Kind != token.INT {
		return 0, false
	}
	value, err := strconv.ParseInt(strings.ReplaceAll(literal.Value, "_", ""), 0, 64)
	return value, err == nil
}

func ps6099SyntaxTypes(sources []ps6077Source) map[string]ast.Expr {
	result := make(map[string]ast.Expr)
	for _, source := range sources {
		for _, declaration := range source.file.Decls {
			generic, ok := declaration.(*ast.GenDecl)
			if !ok || generic.Tok != token.TYPE {
				continue
			}
			for _, specification := range generic.Specs {
				if named, ok := specification.(*ast.TypeSpec); ok {
					result[named.Name.Name] = named.Type
				}
			}
		}
	}
	return result
}

func ps6099SyntaxTypeAliases(sources []ps6077Source) map[string]bool {
	result := make(map[string]bool)
	for _, source := range sources {
		for _, declaration := range source.file.Decls {
			generic, ok := declaration.(*ast.GenDecl)
			if !ok || generic.Tok != token.TYPE {
				continue
			}
			for _, specification := range generic.Specs {
				if named, ok := specification.(*ast.TypeSpec); ok {
					result[named.Name.Name] = named.Assign.IsValid()
				}
			}
		}
	}
	return result
}

func ps6099LeafSequences(pass *analysis.Pass, function *ast.FuncDecl, typesByName map[string]ast.Expr) []ps6099LeafSequence {
	if function == nil || function.Type == nil || function.Type.Params == nil {
		return nil
	}
	var result []ps6099LeafSequence
	for _, field := range function.Type.Params.List {
		if precision, sequence := ps6099SequenceKindPrecision(pass.TypesInfo.TypeOf(field.Type), field.Type, typesByName, make(map[string]bool)); sequence && precision != "" {
			result = append(result, ps6099LeafSequence{typ: pass.TypesInfo.TypeOf(field.Type), syntax: field.Type})
		}
	}
	return result
}

// ps6099BatchPrecision accepts only a result-free batch signature whose every
// sequence is a float sequence of one consistent precision. Requiring known
// precision is intentional: an untyped ignored sibling or []int must never be
// wildcard evidence for math's float64 operations.
func ps6099BatchPrecision(pass *analysis.Pass, function *ast.FuncDecl, typesByName map[string]ast.Expr) (string, bool) {
	if function == nil || function.Recv != nil || function.Type == nil || function.Type.Params == nil ||
		function.Type.Results != nil && len(function.Type.Results.List) != 0 {
		return "", false
	}
	precision := ""
	found := false
	for _, field := range function.Type.Params.List {
		if _, variadic := ps2110Unparen(field.Type).(*ast.Ellipsis); variadic {
			return "", false
		}
		fieldPrecision, sequence := ps6099SequenceKindPrecision(pass.TypesInfo.TypeOf(field.Type), field.Type, typesByName, make(map[string]bool))
		if !sequence {
			continue
		}
		found = true
		if fieldPrecision == "" || precision != "" && precision != fieldPrecision {
			return "", false
		}
		precision = fieldPrecision
	}
	namePrecision := ps6099NamePrecision(function.Name.Name)
	if !found || precision == "" || namePrecision == "mixed" || namePrecision != "" && namePrecision != precision {
		return "", false
	}
	return precision, true
}

func ps6099SequencePrecision(typ types.Type, syntax ast.Expr, typesByName map[string]ast.Expr) string {
	precision, sequence := ps6099SequenceKindPrecision(typ, syntax, typesByName, make(map[string]bool))
	if !sequence {
		return ""
	}
	return precision
}

func ps6099SequenceKindPrecision(typ types.Type, syntax ast.Expr, typesByName map[string]ast.Expr, active map[string]bool) (string, bool) {
	if typ != nil {
		switch sequence := types.Unalias(typ).Underlying().(type) {
		case *types.Slice:
			return ps6099Precision(sequence.Elem()), true
		case *types.Array:
			return ps6099Precision(sequence.Elem()), true
		}
	}
	switch value := ps2110Unparen(syntax).(type) {
	case *ast.StarExpr:
		return ps6099SequenceKindPrecision(nil, value.X, typesByName, active)
	case *ast.ArrayType:
		return ps6099SyntaxScalarPrecision(value.Elt, typesByName, active), true
	case *ast.Ident:
		if active[value.Name] || typesByName == nil || typesByName[value.Name] == nil {
			return "", false
		}
		active[value.Name] = true
		precision, sequence := ps6099SequenceKindPrecision(nil, typesByName[value.Name], typesByName, active)
		delete(active, value.Name)
		return precision, sequence
	}
	return "", false
}

func ps6099SyntaxScalarPrecision(expression ast.Expr, typesByName map[string]ast.Expr, active map[string]bool) string {
	switch value := ps2110Unparen(expression).(type) {
	case *ast.Ident:
		if value.Name == "float32" || value.Name == "float64" {
			return value.Name
		}
		if active[value.Name] || typesByName == nil || typesByName[value.Name] == nil {
			return ""
		}
		active[value.Name] = true
		precision := ps6099SyntaxScalarPrecision(typesByName[value.Name], typesByName, active)
		delete(active, value.Name)
		return precision
	case *ast.UnaryExpr:
		if value.Op == token.TILDE {
			return ps6099SyntaxScalarPrecision(value.X, typesByName, active)
		}
	}
	return ""
}

func ps6099OperationInName(name string) string {
	for _, operation := range ps6099OperationOrder {
		for offset := 0; ; {
			index := strings.Index(name[offset:], operation)
			if index < 0 {
				break
			}
			index += offset
			after := index + len(operation)
			if (after == len(name) || !unicode.IsLower(rune(name[after]))) &&
				ps6099OperationAffixes(name[:index], name[after:]) {
				return operation
			}
			offset = index + 1
		}
		lower, target := strings.ToLower(name), strings.ToLower(operation)
		for _, prefix := range []string{target, "v" + target, "simd" + target, "vector" + target} {
			if !strings.HasPrefix(lower, prefix) {
				continue
			}
			rest := strings.TrimPrefix(lower, prefix)
			if ps6099OperationSuffix(rest) {
				return operation
			}
		}
	}
	return ""
}

func ps6099OperationAffixes(prefix, suffix string) bool {
	prefix = strings.Trim(strings.ToLower(prefix), "_")
	switch prefix {
	case "", "v", "simd", "vector", "batch", "batched", "neon", "avx", "sse", "asm", "native", "apply", "fast":
	default:
		return false
	}
	return ps6099OperationSuffix(strings.TrimLeft(strings.ToLower(suffix), "_"))
}

func ps6099OperationSuffix(suffix string) bool {
	suffix = strings.TrimLeft(suffix, "_")
	for suffix != "" {
		matched := false
		for _, token := range []string{
			"float32", "float64", "batched", "inplace", "scaled", "vector", "native",
			"float", "batch", "simd", "neon", "avx512", "avx2", "avx", "sse4",
			"sse", "into", "asm", "f32", "f64", "x4", "x2", "x",
		} {
			if strings.HasPrefix(suffix, token) {
				suffix = strings.TrimLeft(strings.TrimPrefix(suffix, token), "_")
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

func ps6099NamePrecision(name string) string {
	lower := strings.ToLower(name)
	f32 := strings.Contains(lower, "float32") || strings.Contains(lower, "f32")
	f64 := strings.Contains(lower, "float64") || strings.Contains(lower, "f64")
	switch {
	case f32 && f64:
		return "mixed"
	case f32:
		return "float32"
	case f64:
		return "float64"
	default:
		return ""
	}
}

func ps6099Function(pass *analysis.Pass, function *ast.FuncDecl, helpers map[*types.Func]ps6099Helper, leaves map[string][]ps6099Leaf) {
	reported := make(map[ast.Node]bool)
	var parents map[ast.Node]ast.Node
	var reachableCalls map[*ast.CallExpr]bool
	astutil.WithStack(function.Body, func(node ast.Node, stack []ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		operation, ok := ps6099CallOperation(pass, call, helpers)
		if !ok {
			return true
		}
		if parents == nil {
			parents = ps6087Parents(function.Body)
			reachableCalls = ps6099ReachableCalls(pass, function, parents)
		}
		if !ps6099CallReachable(pass, call, parents, reachableCalls[call]) {
			return true
		}
		assignment, left := ps6099CallAssignment(call, stack)
		if assignment == nil {
			return true
		}
		destination, ok := ps6099StorageOf(pass, left.X)
		if !ok || !ps6099Sequence(destination.typ) {
			return true
		}
		loop := ps6099EnclosingLoop(pass, stack, left.Index)
		if loop == nil || reported[loop.node] || !ps6099Unconditional(stack, loop.node) ||
			!ps6099OuterControlSafe(pass, function.Body, loop) || !ps6099CallEvaluationSafe(pass, call, loop) ||
			!ps6099DestinationSafe(pass, function.Body, loop, call, left, destination) ||
			ps6099ScalarCallCount(pass, loop.body, helpers) != 1 {
			return true
		}
		definitions := ps6099Definitions(pass, loop.body, call.Pos(), loop)
		if !ps6099ValidCallInputs(pass, operation, call, helpers, loop, definitions) {
			return true
		}
		precision := ps6099Precision(pass.TypesInfo.TypeOf(call))
		leaf, ok := ps6099BestLeaf(leaves[operation], precision, destination.typ)
		if !ok {
			return true
		}
		reported[loop.node] = true
		position := pass.Fset.Position(leaf.node.Pos())
		message := "loop calls scalar math." + operation + " exactly once per independent " + destination.name + " element and the package exposes " + leaf.name + " (" + leaf.kind + ") at " + filepath.Base(position.Filename) + ":" + strconv.Itoa(position.Line) + "; measure a two-pass candidate that stages the exact pre-transcendental value into " + destination.name + " and invokes the compatible batched leaf in place; prove destination/input aliases, precision, special values, panic/partial-output behavior, tails, alignment, feature gates, and the profitable band crossover"
		message += "; if values feed an iterative solver, tolerance is not an acceptance gate—require matching iteration counts, termination, active/support sets, decision signs, repeatability, and alternating same-binary end-to-end benchmarks (advisory, no automatic fix)"
		pass.Report(analysis.Diagnostic{
			Pos: loop.node.Pos(), End: loop.node.End(), Message: message,
			Related: []analysis.RelatedInformation{
				{Pos: call.Pos(), End: call.End(), Message: "scalar " + operation + " on the per-element path"},
				{Pos: leaf.node.Pos(), End: leaf.node.End(), Message: "known compatible batched/vector leaf " + leaf.name},
			},
		})
		return true
	})
}

func ps6099ValidCallInputs(pass *analysis.Pass, operation string, call *ast.CallExpr, helpers map[*types.Func]ps6099Helper, loop *ps6099Loop, definitions map[types.Object]bool) bool {
	inputs := ps6099CallInputs(pass, call, helpers)
	if operation != "Pow" {
		return ps6099DependsOnIteration(pass, inputs, loop, definitions)
	}
	if _, direct := ps6099DirectMathCall(pass, call); !direct || len(inputs) != 2 {
		return false
	}
	return ps6099DependsOnIteration(pass, inputs[:1], loop, definitions) &&
		!ps6099DependsOnIteration(pass, inputs[1:], loop, definitions) &&
		ps6099LoopStableExpression(pass, inputs[1], loop.body)
}

func ps6099LoopStableExpression(pass *analysis.Pass, expression ast.Expr, body *ast.BlockStmt) bool {
	written := make(map[types.Object]bool)
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		switch value := node.(type) {
		case *ast.AssignStmt:
			for _, left := range value.Lhs {
				ps6099MarkExpressionObjects(pass, left, written)
			}
		case *ast.IncDecStmt:
			ps6099MarkExpressionObjects(pass, value.X, written)
		case *ast.ValueSpec:
			for _, name := range value.Names {
				if object := pass.TypesInfo.ObjectOf(name); object != nil {
					written[object] = true
				}
			}
		case *ast.RangeStmt:
			ps6099MarkExpressionObjects(pass, value.Key, written)
			ps6099MarkExpressionObjects(pass, value.Value, written)
		}
		return true
	})
	stable := true
	ast.Inspect(expression, func(node ast.Node) bool {
		if !stable {
			return false
		}
		switch value := node.(type) {
		case *ast.CallExpr:
			stable = false
			return false
		case *ast.UnaryExpr:
			if value.Op == token.ARROW {
				stable = false
				return false
			}
		case *ast.Ident:
			if written[pass.TypesInfo.ObjectOf(value)] {
				stable = false
				return false
			}
		case *ast.SelectorExpr:
			if written[pass.TypesInfo.ObjectOf(value.Sel)] {
				stable = false
				return false
			}
		}
		return true
	})
	return stable
}

func ps6099MarkExpressionObjects(pass *analysis.Pass, expression ast.Expr, objects map[types.Object]bool) {
	if expression == nil {
		return
	}
	ast.Inspect(expression, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.Ident:
			if object := pass.TypesInfo.ObjectOf(value); object != nil {
				objects[object] = true
			}
		case *ast.SelectorExpr:
			if object := pass.TypesInfo.ObjectOf(value.Sel); object != nil {
				objects[object] = true
			}
		}
		return true
	})
}

func ps6099CallInputs(pass *analysis.Pass, call *ast.CallExpr, helpers map[*types.Func]ps6099Helper) []ast.Expr {
	if _, direct := ps6099DirectMathCall(pass, call); direct {
		return call.Args
	}
	callee, _, ok := typedCallee(pass, call.Fun)
	if !ok {
		return nil
	}
	positions := helpers[callee.Origin()].parameterInputs
	result := make([]ast.Expr, 0, len(positions))
	for _, position := range positions {
		if position >= 0 && position < len(call.Args) {
			result = append(result, call.Args[position])
		}
	}
	return result
}

func ps6099CallEvaluationSafe(pass *analysis.Pass, call *ast.CallExpr, loop *ps6099Loop) bool {
	if call == nil || loop == nil {
		return false
	}
	safe := true
	ast.Inspect(call, func(node ast.Node) bool {
		if !safe || node == nil {
			return false
		}
		switch value := node.(type) {
		case *ast.FuncLit:
			safe = false
			return false
		case *ast.CallExpr:
			if value != call && !ps6099PureCall(pass, value) {
				safe = false
				return false
			}
		case *ast.IndexExpr:
			if ps6099RuntimeIndex(pass, value) && !ps6099LoopSafeIndex(pass, value, loop) {
				safe = false
				return false
			}
		case *ast.SliceExpr, *ast.TypeAssertExpr, *ast.CompositeLit:
			safe = false
			return false
		case *ast.UnaryExpr:
			if value.Op == token.AND || value.Op == token.ARROW || value.Op == token.MUL {
				safe = false
				return false
			}
		case *ast.SelectorExpr:
			if ps6099PointerFieldSelection(pass, value) {
				safe = false
				return false
			}
		case *ast.BinaryExpr:
			if ps6099RiskyIntegerOperation(pass, value) {
				safe = false
				return false
			}
		}
		return true
	})
	return safe
}

func ps6099LoopSafeIndex(pass *analysis.Pass, index *ast.IndexExpr, loop *ps6099Loop) bool {
	if index == nil || loop == nil || loop.sequence == nil || !ps6099Object(pass, index.Index, loop.index) {
		return false
	}
	indexed, indexedOK := ps6099StorageOf(pass, index.X)
	sequence, sequenceOK := ps6099StorageOf(pass, loop.sequence)
	return indexedOK && sequenceOK && indexed.key == sequence.key
}

func ps6099CallAssignment(call *ast.CallExpr, stack []ast.Node) (*ast.AssignStmt, *ast.IndexExpr) {
	for position := len(stack) - 1; position >= 0; position-- {
		assignment, ok := stack[position].(*ast.AssignStmt)
		if !ok {
			continue
		}
		if assignment.Tok != token.ASSIGN || len(assignment.Lhs) != len(assignment.Rhs) {
			return nil, nil
		}
		for index, right := range assignment.Rhs {
			if ps2110Unparen(right) != call {
				continue
			}
			left, ok := ps2110Unparen(assignment.Lhs[index]).(*ast.IndexExpr)
			if ok {
				return assignment, left
			}
			return nil, nil
		}
		return nil, nil
	}
	return nil, nil
}

func ps6099EnclosingLoop(pass *analysis.Pass, stack []ast.Node, index ast.Expr) *ps6099Loop {
	identifier, ok := ps2110Unparen(index).(*ast.Ident)
	if !ok {
		return nil
	}
	indexObject := pass.TypesInfo.ObjectOf(identifier)
	for position := len(stack) - 1; position >= 0; position-- {
		loop := ps6099CanonicalLoop(pass, stack[position])
		if loop != nil && loop.index == indexObject {
			return loop
		}
	}
	return nil
}

func ps6099CanonicalLoop(pass *analysis.Pass, node ast.Node) *ps6099Loop {
	switch loop := node.(type) {
	case *ast.RangeStmt:
		identifier, ok := ps2110Unparen(loop.Key).(*ast.Ident)
		if !ok || identifier.Name == "_" || loop.Body == nil || !ps6099Sequence(pass.TypesInfo.TypeOf(loop.X)) {
			return nil
		}
		index := ps6099AssignedObject(pass, identifier, loop.Tok)
		if index == nil {
			return nil
		}
		var value types.Object
		if identifier, ok := ps2110Unparen(loop.Value).(*ast.Ident); ok && identifier.Name != "_" {
			value = ps6099AssignedObject(pass, identifier, loop.Tok)
		}
		return &ps6099Loop{node: loop, body: loop.Body, index: index, rangeValue: value, sequence: loop.X}
	case *ast.ForStmt:
		initializer, ok := loop.Init.(*ast.AssignStmt)
		if !ok || len(initializer.Lhs) != 1 || len(initializer.Rhs) != 1 || !ps6099Integer(pass, initializer.Rhs[0], 0) {
			return nil
		}
		identifier, ok := ps2110Unparen(initializer.Lhs[0]).(*ast.Ident)
		if !ok {
			return nil
		}
		index := ps6099AssignedObject(pass, identifier, initializer.Tok)
		condition, ok := ps2110Unparen(loop.Cond).(*ast.BinaryExpr)
		if index == nil || !ok || condition.Op != token.LSS || !ps6099Object(pass, condition.X, index) {
			return nil
		}
		length, ok := ps2110Unparen(condition.Y).(*ast.CallExpr)
		if !ok || len(length.Args) != 1 || !typedBuiltinName(pass, length.Fun, "len") || !ps6099Sequence(pass.TypesInfo.TypeOf(length.Args[0])) || !ps6099Increment(pass, loop.Post, index) {
			return nil
		}
		return &ps6099Loop{node: loop, body: loop.Body, index: index, sequence: length.Args[0]}
	}
	return nil
}

func ps6099AssignedObject(pass *analysis.Pass, identifier *ast.Ident, assignment token.Token) types.Object {
	if identifier == nil {
		return nil
	}
	if assignment == token.DEFINE {
		return pass.TypesInfo.Defs[identifier]
	}
	return pass.TypesInfo.Uses[identifier]
}

func ps6099Integer(pass *analysis.Pass, expression ast.Expr, expected int64) bool {
	value := pass.TypesInfo.Types[expression].Value
	return value != nil && value.Kind() == constant.Int && constant.Compare(value, token.EQL, constant.MakeInt64(expected))
}

func ps6099Object(pass *analysis.Pass, expression ast.Expr, object types.Object) bool {
	identifier, ok := ps2110Unparen(expression).(*ast.Ident)
	return ok && pass.TypesInfo.ObjectOf(identifier) == object
}

func ps6099Increment(pass *analysis.Pass, statement ast.Stmt, object types.Object) bool {
	switch value := statement.(type) {
	case *ast.IncDecStmt:
		return value.Tok == token.INC && ps6099Object(pass, value.X, object)
	case *ast.AssignStmt:
		return value.Tok == token.ADD_ASSIGN && len(value.Lhs) == 1 && len(value.Rhs) == 1 &&
			ps6099Object(pass, value.Lhs[0], object) && ps6099Integer(pass, value.Rhs[0], 1)
	}
	return false
}

func ps6099Sequence(typ types.Type) bool {
	if typ == nil {
		return false
	}
	switch types.Unalias(typ).Underlying().(type) {
	case *types.Array, *types.Slice:
		return true
	}
	return false
}

func ps6099Unconditional(stack []ast.Node, loop ast.Node) bool {
	seenLoop := false
	for _, node := range stack {
		if node == loop {
			seenLoop = true
			continue
		}
		if !seenLoop {
			continue
		}
		switch node.(type) {
		case *ast.IfStmt, *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt,
			*ast.CaseClause, *ast.CommClause, *ast.ForStmt, *ast.RangeStmt,
			*ast.FuncLit, *ast.GoStmt, *ast.DeferStmt:
			return false
		}
	}
	return seenLoop
}

func ps6099OuterControlSafe(pass *analysis.Pass, scope *ast.BlockStmt, loop *ps6099Loop) bool {
	if loop == nil || loop.body == nil {
		return false
	}
	if ps6099LoopAddressTaken(pass, scope, loop) {
		return false
	}
	safe := true
	aliases := make(map[types.Object]bool)
	parents := ps6087Parents(scope)
	reachable := ps6099ReachableNodesInBlock(pass, loop.body, ps6087Parents(loop.body))
	astutil.WithStack(loop.body, func(node ast.Node, _ []ast.Node) bool {
		if !safe {
			return false
		}
		if literal, ok := node.(*ast.FuncLit); ok && literal != nil {
			return false
		}
		if branch, ok := node.(*ast.BranchStmt); ok {
			if !ps6099BranchReachable(pass, branch, parents) {
				return true
			}
			safe = !ps6099BranchAffectsLoop(pass, scope, branch, loop, parents)
			return safe
		}
		if !reachable[node] {
			return true
		}
		switch value := node.(type) {
		case *ast.ReturnStmt:
			safe = false
		case *ast.AssignStmt:
			ps6099RecordLoopAliases(pass, value, loop, aliases)
			for _, left := range value.Lhs {
				if ps6099LoopVariable(pass, left, loop) || ps6099WritesLoopAlias(pass, left, aliases) {
					safe = false
					break
				}
			}
		case *ast.IncDecStmt:
			safe = !ps6099LoopVariable(pass, value.X, loop) && !ps6099WritesLoopAlias(pass, value.X, aliases)
		case *ast.RangeStmt:
			safe = !ps6099LoopVariable(pass, value.Key, loop) && !ps6099LoopVariable(pass, value.Value, loop)
		case *ast.UnaryExpr:
			if value.Op == token.AND && ps6099LoopVariable(pass, value.X, loop) {
				safe = false
			}
		}
		return safe
	})
	return safe
}

func ps6099BranchReachable(pass *analysis.Pass, branch *ast.BranchStmt, parents map[ast.Node]ast.Node) bool {
	for node, parent := ast.Node(branch), parents[branch]; parent != nil; node, parent = parent, parents[parent] {
		switch value := parent.(type) {
		case *ast.IfStmt:
			truth, known := ps6099BooleanCondition(pass, value.Cond)
			if !known {
				continue
			}
			inThen := ps6099NodeWithin(node, value.Body)
			if truth != inThen {
				return false
			}
		case *ast.ForStmt:
			if ps6099NodeWithin(node, value.Body) {
				if iterations, known := ps6099ForExactIterations(pass, value); known && iterations == 0 {
					return false
				}
			}
		case *ast.RangeStmt:
			if ps6099NodeWithin(node, value.Body) {
				if iterations, known := ps6099RangeExactIterations(pass, value.X); known && iterations == 0 {
					return false
				}
			}
		case *ast.SwitchStmt:
			selected, known := ps6099SelectedSwitchClause(pass, value)
			if !known {
				continue
			}
			if selected < 0 {
				return false
			}
			var clause *ast.CaseClause
			for candidate := parents[branch]; candidate != nil && candidate != value; candidate = parents[candidate] {
				if current, ok := candidate.(*ast.CaseClause); ok {
					clause = current
					break
				}
			}
			if clause == nil {
				continue
			}
			selectedClause := -1
			for index, candidate := range value.Body.List {
				if candidate == clause {
					selectedClause = index
					break
				}
			}
			if selectedClause < selected {
				return false
			}
			for index := selected; index < selectedClause; index++ {
				candidate, ok := value.Body.List[index].(*ast.CaseClause)
				if !ok || !ps6099CaseFallsThrough(candidate) {
					return false
				}
			}
		}
	}
	return true
}

func ps6099BranchAffectsLoop(pass *analysis.Pass, scope *ast.BlockStmt, branch *ast.BranchStmt, loop *ps6099Loop, parents map[ast.Node]ast.Node) bool {
	if branch == nil || loop == nil {
		return true
	}
	switch branch.Tok {
	case token.FALLTHROUGH:
		return false
	case token.GOTO:
		return true
	case token.BREAK, token.CONTINUE:
	default:
		return true
	}
	target := ps6099BranchTarget(pass, scope, branch, parents)
	if target == nil || target == loop.node || ps6099NodeWithin(loop.node, target) {
		return true
	}
	switch target.(type) {
	case *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt:
		return false
	}
	// A nested-loop branch changes the iteration transfer and remains outside
	// the deliberately narrow flow proof until break/continue states are modeled.
	return true
}

func ps6099BranchTarget(pass *analysis.Pass, scope *ast.BlockStmt, branch *ast.BranchStmt, parents map[ast.Node]ast.Node) ast.Node {
	if branch == nil {
		return nil
	}
	if branch.Label != nil {
		label := pass.TypesInfo.ObjectOf(branch.Label)
		var target ast.Node
		ast.Inspect(scope, func(node ast.Node) bool {
			if target != nil {
				return false
			}
			statement, ok := node.(*ast.LabeledStmt)
			if ok && (label != nil && pass.TypesInfo.ObjectOf(statement.Label) == label ||
				label == nil && statement.Label.Name == branch.Label.Name) {
				target = statement.Stmt
				return false
			}
			return true
		})
		return target
	}
	for parent := parents[branch]; parent != nil; parent = parents[parent] {
		switch branch.Tok {
		case token.BREAK:
			switch parent.(type) {
			case *ast.ForStmt, *ast.RangeStmt, *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt:
				return parent
			}
		case token.CONTINUE:
			switch parent.(type) {
			case *ast.ForStmt, *ast.RangeStmt:
				return parent
			}
		}
	}
	return nil
}

func ps6099LoopAddressTaken(pass *analysis.Pass, scope *ast.BlockStmt, loop *ps6099Loop) bool {
	taken := false
	parents := ps6087Parents(scope)
	reachable := ps6099ReachableNodesInBlock(pass, scope, parents)
	ast.Inspect(scope, func(node ast.Node) bool {
		if taken {
			return false
		}
		unary, ok := node.(*ast.UnaryExpr)
		if ok && reachable[unary] && unary.Op == token.AND && ps6099LoopVariable(pass, unary.X, loop) {
			taken = true
			return false
		}
		return true
	})
	return taken
}

func ps6099LoopVariable(pass *analysis.Pass, expression ast.Expr, loop *ps6099Loop) bool {
	identifier, ok := ps2110Unparen(expression).(*ast.Ident)
	if !ok {
		return false
	}
	object := pass.TypesInfo.ObjectOf(identifier)
	return object != nil && (object == loop.index || object == loop.rangeValue)
}

func ps6099RecordLoopAliases(pass *analysis.Pass, assignment *ast.AssignStmt, loop *ps6099Loop, aliases map[types.Object]bool) {
	if assignment == nil || len(assignment.Lhs) != len(assignment.Rhs) {
		return
	}
	for index, right := range assignment.Rhs {
		unary, ok := ps2110Unparen(right).(*ast.UnaryExpr)
		if !ok || unary.Op != token.AND || !ps6099LoopVariable(pass, unary.X, loop) {
			continue
		}
		identifier, ok := ps2110Unparen(assignment.Lhs[index]).(*ast.Ident)
		if ok {
			if object := pass.TypesInfo.ObjectOf(identifier); object != nil {
				aliases[object] = true
			}
		}
	}
}

func ps6099WritesLoopAlias(pass *analysis.Pass, expression ast.Expr, aliases map[types.Object]bool) bool {
	star, ok := ps2110Unparen(expression).(*ast.StarExpr)
	if !ok {
		return false
	}
	identifier, ok := ps2110Unparen(star.X).(*ast.Ident)
	return ok && aliases[pass.TypesInfo.ObjectOf(identifier)]
}

func ps6099StorageOf(pass *analysis.Pass, expression ast.Expr) (ps6099Storage, bool) {
	expression = ps2110Unparen(expression)
	switch value := expression.(type) {
	case *ast.Ident:
		object := pass.TypesInfo.ObjectOf(value)
		if object == nil {
			return ps6099Storage{}, false
		}
		return ps6099Storage{key: ps6099ObjectKey(object), name: value.Name, typ: object.Type(), root: object}, true
	case *ast.SelectorExpr:
		base, ok := ps6099StorageOf(pass, value.X)
		field := pass.TypesInfo.Uses[value.Sel]
		if !ok || field == nil {
			return ps6099Storage{}, false
		}
		return ps6099Storage{
			key: base.key + "/" + ps6099ObjectKey(field), name: exprTextRendered(value),
			typ: pass.TypesInfo.TypeOf(value), root: base.root,
		}, true
	case *ast.StarExpr:
		return ps6099StorageOf(pass, value.X)
	}
	return ps6099Storage{}, false
}

func ps6099ObjectKey(object types.Object) string {
	path := "local"
	if object.Pkg() != nil {
		path = object.Pkg().Path()
	}
	return path + ":" + strconv.Itoa(int(object.Pos())) + ":" + object.Name()
}

func ps6099DestinationSafe(pass *analysis.Pass, scope *ast.BlockStmt, loop *ps6099Loop, scalarCall *ast.CallExpr, left *ast.IndexExpr, destination ps6099Storage) bool {
	if destination.root == nil || destination.root.Pos() > loop.node.Pos() && destination.root.Pos() < loop.node.End() {
		return false
	}
	aliases, escaped := ps6099DestinationAliases(pass, scope, loop.node.Pos(), destination)
	if escaped {
		return false
	}
	parents := ps6087Parents(loop.body)
	reachableCalls := ps6099ReachableCallsInBlock(pass, loop.body, parents)
	allowedRoot := make(map[ast.Node]bool)
	ast.Inspect(left.X, func(node ast.Node) bool {
		if node != nil {
			allowedRoot[node] = true
		}
		return true
	})
	safe := true
	ast.Inspect(loop.body, func(node ast.Node) bool {
		if !safe {
			return false
		}
		switch value := node.(type) {
		case *ast.CallExpr:
			if value != scalarCall && ps6099CallReachable(pass, value, parents, reachableCalls[value]) && !ps6099PureCall(pass, value) {
				safe = false
				return false
			}
		case *ast.AssignStmt:
			for _, target := range value.Lhs {
				if ps6099RebindsStorage(pass, target, destination) {
					safe = false
					return false
				}
			}
		case *ast.RangeStmt:
			if ps6099RebindsStorage(pass, value.Key, destination) || ps6099RebindsStorage(pass, value.Value, destination) {
				safe = false
				return false
			}
		case *ast.UnaryExpr:
			if value.Op == token.AND && ps6099RebindsStorage(pass, value.X, destination) {
				safe = false
				return false
			}
		case *ast.Ident:
			// Every occurrence of the destination root outside the one canonical
			// write is an observable read, rebind, capture, receiver, or escape.
			// This also descends into closures instead of assuming captures are safe.
			object := pass.TypesInfo.ObjectOf(value)
			if aliases[object] && (object != destination.root || !allowedRoot[value]) {
				safe = false
				return false
			}
		}
		expression, ok := node.(ast.Expr)
		if !ok {
			return true
		}
		storage, ok := ps6099StorageOf(pass, expression)
		if !ok || storage.key != destination.key {
			return true
		}
		parent := parents[node]
		if selector, ok := parent.(*ast.SelectorExpr); ok && selector.X == expression {
			return true
		}
		if index, ok := parent.(*ast.IndexExpr); ok && index.X == expression {
			if index == left {
				return true
			}
			safe = false
			return false
		}
		if expression == left.X {
			return true
		}
		safe = false
		return false
	})
	return safe
}

type ps6099AliasEquation struct {
	left  types.Object
	right []types.Object
}

// ps6099DestinationAliases builds a conservative, bidirectional may-alias
// closure from assignments before the candidate loop. This catches aliases
// whether the canonical write uses the original destination or an alias of it,
// and follows slice/pointer aliases through tuples, structs, and alias chains.
func ps6099DestinationAliases(pass *analysis.Pass, scope *ast.BlockStmt, before token.Pos, destination ps6099Storage) (map[types.Object]bool, bool) {
	aliases := map[types.Object]bool{destination.root: true}
	var equations []ps6099AliasEquation
	unknown := make(map[types.Object]bool)
	ast.Inspect(scope, func(node ast.Node) bool {
		if node == nil || node.Pos() >= before {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		switch value := node.(type) {
		case *ast.AssignStmt:
			if len(value.Lhs) != len(value.Rhs) {
				ps6099TupleAliasAssignments(pass, value.Lhs, value.Rhs, &equations, unknown)
				return true
			}
			for index := range value.Lhs {
				if equation, ok := ps6099AliasAssignment(pass, value.Lhs[index], value.Rhs[index]); ok {
					equations = append(equations, equation)
				}
			}
		case *ast.ValueSpec:
			if len(value.Names) != len(value.Values) {
				left := make([]ast.Expr, len(value.Names))
				for index, name := range value.Names {
					left[index] = name
				}
				ps6099TupleAliasAssignments(pass, left, value.Values, &equations, unknown)
				return true
			}
			for index, name := range value.Names {
				if equation, ok := ps6099AliasAssignment(pass, name, value.Values[index]); ok {
					equations = append(equations, equation)
				}
			}
		}
		return true
	})
	for changed := true; changed; {
		changed = false
		for _, equation := range equations {
			leftAlias := aliases[equation.left]
			rightAlias := false
			for _, object := range equation.right {
				rightAlias = rightAlias || aliases[object]
			}
			if rightAlias && !aliases[equation.left] {
				aliases[equation.left] = true
				changed = true
			}
			if leftAlias {
				for _, object := range equation.right {
					if !aliases[object] {
						aliases[object] = true
						changed = true
					}
				}
			}
		}
	}
	for object := range unknown {
		if aliases[object] {
			return aliases, true
		}
	}
	return aliases, ps6099DestinationEscapedBefore(pass, scope, before, aliases)
}

type ps6099ResultSource struct {
	receiver  bool
	parameter int
}

type ps6099ResultSummary struct {
	known   bool
	sources []ps6099ResultSource
}

func ps6099TupleAliasAssignments(pass *analysis.Pass, left, right []ast.Expr, equations *[]ps6099AliasEquation, unknown map[types.Object]bool) {
	if len(right) != 1 || len(left) < 2 {
		ps6099MarkUnknownAliasRoots(pass, left, unknown)
		return
	}
	call, ok := ps2110Unparen(right[0]).(*ast.CallExpr)
	if !ok {
		ps6099MarkUnknownAliasRoots(pass, left, unknown)
		return
	}
	function, signature, ok := typedCallee(pass, call.Fun)
	if !ok || signature.Results().Len() != len(left) {
		ps6099MarkUnknownAliasRoots(pass, left, unknown)
		return
	}
	for index, expression := range left {
		root := ps6099ExpressionRoot(pass, expression)
		if root == nil || !ps6099MayCarryAlias(root.Type(), make(map[types.Type]bool)) {
			continue
		}
		summary := ps6099FunctionResultSummary(pass, function, index, make(map[*types.Func]bool))
		if !summary.known {
			unknown[root] = true
			continue
		}
		objects := make(map[types.Object]bool)
		for _, source := range summary.sources {
			actual, ok := ps6099CallSourceExpression(pass, call, source)
			if !ok {
				unknown[root] = true
				objects = nil
				break
			}
			actualObjects, known := ps6099AliasObjects(pass, actual)
			if !known {
				unknown[root] = true
				objects = nil
				break
			}
			for _, object := range actualObjects {
				objects[object] = true
			}
		}
		if len(objects) == 0 {
			continue
		}
		rightObjects := make([]types.Object, 0, len(objects))
		for object := range objects {
			rightObjects = append(rightObjects, object)
		}
		*equations = append(*equations, ps6099AliasEquation{left: root, right: rightObjects})
	}
}

func ps6099MarkUnknownAliasRoots(pass *analysis.Pass, expressions []ast.Expr, unknown map[types.Object]bool) {
	for _, expression := range expressions {
		root := ps6099ExpressionRoot(pass, expression)
		if root != nil && ps6099MayCarryAlias(root.Type(), make(map[types.Type]bool)) {
			unknown[root] = true
		}
	}
}

func ps6099FunctionResultSummary(pass *analysis.Pass, function *types.Func, result int, active map[*types.Func]bool) ps6099ResultSummary {
	if function == nil || active[function] {
		return ps6099ResultSummary{}
	}
	declaration := ps6099FunctionDeclaration(pass, function)
	if declaration == nil || declaration.Body == nil {
		return ps6099ResultSummary{}
	}
	active[function] = true
	defer delete(active, function)
	signature, ok := function.Type().(*types.Signature)
	if !ok || result < 0 || result >= signature.Results().Len() {
		return ps6099ResultSummary{}
	}
	combined := ps6099ResultSummary{known: true}
	found := false
	valid := true
	ast.Inspect(declaration.Body, func(node ast.Node) bool {
		if !valid {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		statement, ok := node.(*ast.ReturnStmt)
		if !ok {
			return true
		}
		found = true
		var summary ps6099ResultSummary
		switch {
		case len(statement.Results) == signature.Results().Len():
			summary = ps6099FormalExpressionSummary(pass, declaration, statement.Results[result], active)
		case len(statement.Results) == 1:
			call, ok := ps2110Unparen(statement.Results[0]).(*ast.CallExpr)
			if !ok {
				valid = false
				return false
			}
			summary = ps6099NestedResultSummary(pass, declaration, call, result, active)
		default:
			valid = false
			return false
		}
		if !summary.known {
			valid = false
			return false
		}
		combined.sources = ps6099AppendResultSources(combined.sources, summary.sources...)
		return true
	})
	if !valid || !found {
		return ps6099ResultSummary{}
	}
	return combined
}

func ps6099FunctionDeclaration(pass *analysis.Pass, function *types.Func) *ast.FuncDecl {
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			candidate, ok := declaration.(*ast.FuncDecl)
			if !ok || candidate.Name.Name != function.Name() {
				continue
			}
			if pass.TypesInfo.ObjectOf(candidate.Name) == function || candidate.Name.Pos() == function.Pos() {
				return candidate
			}
		}
	}
	return nil
}

func ps6099FormalExpressionSummary(pass *analysis.Pass, declaration *ast.FuncDecl, expression ast.Expr, active map[*types.Func]bool) ps6099ResultSummary {
	expression = ps2110Unparen(expression)
	switch value := expression.(type) {
	case *ast.SliceExpr:
		return ps6099FormalExpressionSummary(pass, declaration, value.X, active)
	case *ast.StarExpr:
		return ps6099FormalExpressionSummary(pass, declaration, value.X, active)
	case *ast.CompositeLit:
		return ps6099ResultSummary{}
	case *ast.CallExpr:
		if typedBuiltinName(pass, value.Fun, "make") || typedBuiltinName(pass, value.Fun, "new") {
			return ps6099ResultSummary{known: true}
		}
		if pass.TypesInfo.Types[value.Fun].IsType() && len(value.Args) == 1 {
			return ps6099FormalExpressionSummary(pass, declaration, value.Args[0], active)
		}
		return ps6099NestedResultSummary(pass, declaration, value, 0, active)
	case *ast.Ident:
		if value.Name == "nil" {
			return ps6099ResultSummary{known: true}
		}
	}
	root := ps6099ExpressionRoot(pass, expression)
	if root == nil {
		return ps6099ResultSummary{}
	}
	if ps6099ReceiverObject(pass, declaration) == root {
		return ps6099ResultSummary{known: true, sources: []ps6099ResultSource{{receiver: true}}}
	}
	if parameter, ok := ps6099ParameterIndex(pass, declaration, root); ok {
		return ps6099ResultSummary{known: true, sources: []ps6099ResultSource{{parameter: parameter}}}
	}
	return ps6099ResultSummary{}
}

func ps6099NestedResultSummary(pass *analysis.Pass, outer *ast.FuncDecl, call *ast.CallExpr, result int, active map[*types.Func]bool) ps6099ResultSummary {
	function, _, ok := typedCallee(pass, call.Fun)
	if !ok {
		return ps6099ResultSummary{}
	}
	nested := ps6099FunctionResultSummary(pass, function, result, active)
	if !nested.known {
		return nested
	}
	resultSummary := ps6099ResultSummary{known: true}
	for _, source := range nested.sources {
		expression, ok := ps6099CallSourceExpression(pass, call, source)
		if !ok {
			return ps6099ResultSummary{}
		}
		summary := ps6099FormalExpressionSummary(pass, outer, expression, active)
		if !summary.known {
			return ps6099ResultSummary{}
		}
		resultSummary.sources = ps6099AppendResultSources(resultSummary.sources, summary.sources...)
	}
	return resultSummary
}

func ps6099CallSourceExpression(pass *analysis.Pass, call *ast.CallExpr, source ps6099ResultSource) (ast.Expr, bool) {
	if source.receiver {
		selector, ok := ps2110Unparen(call.Fun).(*ast.SelectorExpr)
		if !ok {
			return nil, false
		}
		if selection := pass.TypesInfo.Selections[selector]; selection != nil && selection.Kind() == types.MethodVal {
			return selector.X, true
		}
		return nil, false
	}
	if source.parameter < 0 || source.parameter >= len(call.Args) {
		return nil, false
	}
	return call.Args[source.parameter], true
}

func ps6099ReceiverObject(pass *analysis.Pass, declaration *ast.FuncDecl) types.Object {
	if declaration == nil || declaration.Recv == nil {
		return nil
	}
	for _, field := range declaration.Recv.List {
		for _, name := range field.Names {
			return pass.TypesInfo.ObjectOf(name)
		}
	}
	return nil
}

func ps6099ParameterIndex(pass *analysis.Pass, declaration *ast.FuncDecl, target types.Object) (int, bool) {
	position := 0
	if declaration == nil || declaration.Type.Params == nil {
		return 0, false
	}
	for _, field := range declaration.Type.Params.List {
		if len(field.Names) == 0 {
			position++
			continue
		}
		for _, name := range field.Names {
			if pass.TypesInfo.ObjectOf(name) == target {
				return position, true
			}
			position++
		}
	}
	return 0, false
}

func ps6099AppendResultSources(destination []ps6099ResultSource, sources ...ps6099ResultSource) []ps6099ResultSource {
	for _, source := range sources {
		found := false
		for _, existing := range destination {
			found = found || existing == source
		}
		if !found {
			destination = append(destination, source)
		}
	}
	return destination
}

func ps6099AliasObjects(pass *analysis.Pass, expression ast.Expr) ([]types.Object, bool) {
	expression = ps2110Unparen(expression)
	switch value := expression.(type) {
	case *ast.SliceExpr:
		return ps6099AliasObjects(pass, value.X)
	case *ast.StarExpr:
		return ps6099AliasObjects(pass, value.X)
	case *ast.CallExpr:
		if typedBuiltinName(pass, value.Fun, "make") || typedBuiltinName(pass, value.Fun, "new") {
			return nil, true
		}
		if pass.TypesInfo.Types[value.Fun].IsType() && len(value.Args) == 1 {
			return ps6099AliasObjects(pass, value.Args[0])
		}
		return nil, false
	case *ast.Ident:
		if value.Name == "nil" {
			return nil, true
		}
	}
	root := ps6099ExpressionRoot(pass, expression)
	if root != nil && ps6099MayCarryAlias(root.Type(), make(map[types.Type]bool)) {
		return []types.Object{root}, true
	}
	if !ps6099MayCarryAlias(pass.TypesInfo.TypeOf(expression), make(map[types.Type]bool)) {
		return nil, true
	}
	return nil, false
}

func ps6099AliasAssignment(pass *analysis.Pass, left, right ast.Expr) (ps6099AliasEquation, bool) {
	root := ps6099ExpressionRoot(pass, left)
	if root == nil || !ps6099MayCarryAlias(root.Type(), make(map[types.Type]bool)) {
		return ps6099AliasEquation{}, false
	}
	seen := make(map[types.Object]bool)
	var objects []types.Object
	ast.Inspect(right, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		object, ok := pass.TypesInfo.ObjectOf(identifier).(*types.Var)
		if ok && !seen[object] && ps6099MayCarryAlias(object.Type(), make(map[types.Type]bool)) {
			seen[object] = true
			objects = append(objects, object)
		}
		return true
	})
	return ps6099AliasEquation{left: root, right: objects}, len(objects) != 0
}

func ps6099ExpressionRoot(pass *analysis.Pass, expression ast.Expr) types.Object {
	expression = ps2110Unparen(expression)
	switch value := expression.(type) {
	case *ast.Ident:
		return pass.TypesInfo.ObjectOf(value)
	case *ast.SelectorExpr:
		return ps6099ExpressionRoot(pass, value.X)
	case *ast.IndexExpr:
		return ps6099ExpressionRoot(pass, value.X)
	case *ast.IndexListExpr:
		return ps6099ExpressionRoot(pass, value.X)
	case *ast.StarExpr:
		return ps6099ExpressionRoot(pass, value.X)
	}
	return nil
}

func ps6099MayCarryAlias(typ types.Type, active map[types.Type]bool) bool {
	if typ == nil || active[typ] {
		return false
	}
	active[typ] = true
	defer delete(active, typ)
	switch value := types.Unalias(typ).Underlying().(type) {
	case *types.Pointer, *types.Slice, *types.Map, *types.Interface, *types.Chan, *types.Signature:
		return true
	case *types.Array:
		return ps6099MayCarryAlias(value.Elem(), active)
	case *types.Struct:
		for index := 0; index < value.NumFields(); index++ {
			if ps6099MayCarryAlias(value.Field(index).Type(), active) {
				return true
			}
		}
	}
	return false
}

func ps6099DestinationEscapedBefore(pass *analysis.Pass, scope *ast.BlockStmt, before token.Pos, aliases map[types.Object]bool) bool {
	for object := range aliases {
		if variable, ok := object.(*types.Var); ok && variable.Parent() == pass.Pkg.Scope() {
			return true
		}
	}
	parents := ps6087Parents(scope)
	escaped := false
	ast.Inspect(scope, func(node ast.Node) bool {
		if escaped || node == nil || node.Pos() >= before {
			return false
		}
		identifier, ok := node.(*ast.Ident)
		if !ok || !aliases[pass.TypesInfo.ObjectOf(identifier)] {
			return true
		}
		for parent := parents[node]; parent != nil; parent = parents[parent] {
			switch value := parent.(type) {
			case *ast.CallExpr, *ast.FuncLit, *ast.SendStmt, *ast.GoStmt, *ast.DeferStmt:
				escaped = true
				return false
			case *ast.UnaryExpr:
				if value.Op == token.AND {
					escaped = true
					return false
				}
			case *ast.AssignStmt, *ast.ValueSpec:
				return true
			}
		}
		return true
	})
	return escaped
}

func ps6099RebindsStorage(pass *analysis.Pass, expression ast.Expr, destination ps6099Storage) bool {
	if expression == nil || destination.key == "" {
		return false
	}
	storage, ok := ps6099StorageOf(pass, expression)
	return ok && (storage.key == destination.key || strings.HasPrefix(destination.key, storage.key+"/"))
}

func ps6099ScalarCallCount(pass *analysis.Pass, body *ast.BlockStmt, helpers map[*types.Func]ps6099Helper) int {
	count := 0
	parents := ps6087Parents(body)
	reachable := ps6099ReachableCallsInBlock(pass, body, parents)
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if ok && ps6099CallReachable(pass, call, parents, reachable[call]) {
			if _, scalar := ps6099CallOperation(pass, call, helpers); scalar {
				count++
			}
		}
		return true
	})
	return count
}

// ps6099Definitions computes a must-depend state at the scalar call. Branches
// merge with AND, so a value is index-dependent only when every reaching path
// defines it from the iteration. Straight-line assignments still kill earlier
// definitions, and two dependent branches retain dependency.
func ps6099Definitions(pass *analysis.Pass, body *ast.BlockStmt, before token.Pos, loop *ps6099Loop) map[types.Object]bool {
	state := make(map[types.Object]bool)
	ps6099FlowBlock(pass, body, before, loop, state)
	return state
}

func ps6099FlowBlock(pass *analysis.Pass, body *ast.BlockStmt, before token.Pos, loop *ps6099Loop, state map[types.Object]bool) bool {
	if body == nil {
		return false
	}
	for _, statement := range body.List {
		if before.IsValid() && statement.Pos() >= before {
			return true
		}
		if before.IsValid() && statement.Pos() < before && before <= statement.End() {
			ps6099FlowStmt(pass, statement, before, loop, state)
			return true
		}
		ps6099FlowStmt(pass, statement, token.NoPos, loop, state)
	}
	return false
}

func ps6099FlowStmt(pass *analysis.Pass, statement ast.Stmt, before token.Pos, loop *ps6099Loop, state map[types.Object]bool) {
	if statement == nil || before.IsValid() && statement.Pos() >= before {
		return
	}
	switch value := statement.(type) {
	case *ast.AssignStmt:
		if before.IsValid() && before <= value.End() {
			return
		}
		ps6099FlowAssignment(pass, value, loop, state)
	case *ast.IncDecStmt:
		if _, ok := ps2110Unparen(value.X).(*ast.Ident); !ok {
			ps6099InvalidateDependence(state)
		}
	case *ast.DeclStmt:
		declaration, ok := value.Decl.(*ast.GenDecl)
		if !ok {
			return
		}
		for _, specification := range declaration.Specs {
			named, ok := specification.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for index, name := range named.Names {
				object := pass.TypesInfo.ObjectOf(name)
				state[object] = index < len(named.Values) && ps6099ExpressionDepends(pass, named.Values[index], loop, state)
			}
		}
	case *ast.IfStmt:
		if value.Init != nil {
			ps6099FlowStmt(pass, value.Init, token.NoPos, loop, state)
		}
		if truth, known := ps6099BooleanCondition(pass, value.Cond); known {
			if truth {
				ps6099FlowBlock(pass, value.Body, before, loop, state)
			} else if value.Else != nil {
				ps6099FlowStmt(pass, value.Else, before, loop, state)
			}
			return
		}
		thenState := ps6099CloneDependence(state)
		ps6099FlowBlock(pass, value.Body, before, loop, thenState)
		elseState := ps6099CloneDependence(state)
		if value.Else != nil {
			ps6099FlowStmt(pass, value.Else, before, loop, elseState)
		}
		ps6099MergeDependence(state, thenState, elseState)
	case *ast.BlockStmt:
		ps6099FlowBlock(pass, value, before, loop, state)
	case *ast.LabeledStmt:
		ps6099FlowStmt(pass, value.Stmt, before, loop, state)
	case *ast.ForStmt:
		if value.Init != nil {
			ps6099FlowStmt(pass, value.Init, token.NoPos, loop, state)
		}
		exact, known := ps6099ForExactIterations(pass, value)
		minimum := uint64(0)
		if ps6099ForGuaranteedEntry(pass, value) {
			minimum = 1
		}
		ps6099FlowIterations(state, exact, known, minimum, func(iteration map[types.Object]bool) {
			ps6099FlowBlock(pass, value.Body, before, loop, iteration)
			if value.Post != nil {
				ps6099FlowStmt(pass, value.Post, token.NoPos, loop, iteration)
			}
		})
	case *ast.RangeStmt:
		rangeDependent := ps6099ExpressionDepends(pass, value.X, loop, state)
		exact, known := ps6099RangeExactIterations(pass, value.X)
		minimum := uint64(0)
		if ps6099RangeGuaranteedEntry(pass, value.X) {
			minimum = 1
		}
		ps6099FlowIterations(state, exact, known, minimum, func(iteration map[types.Object]bool) {
			if identifier, ok := ps2110Unparen(value.Key).(*ast.Ident); ok && identifier.Name != "_" {
				iteration[pass.TypesInfo.ObjectOf(identifier)] = rangeDependent
			}
			if identifier, ok := ps2110Unparen(value.Value).(*ast.Ident); ok && identifier.Name != "_" {
				iteration[pass.TypesInfo.ObjectOf(identifier)] = rangeDependent
			}
			ps6099FlowBlock(pass, value.Body, before, loop, iteration)
		})
	case *ast.SwitchStmt:
		if value.Init != nil {
			ps6099FlowStmt(pass, value.Init, token.NoPos, loop, state)
		}
		if clause, known := ps6099SelectedSwitchClause(pass, value); known {
			ps6099FlowSelectedSwitchClauses(pass, value.Body.List, clause, before, loop, state)
			return
		}
		ps6099FlowClauses(pass, value.Body.List, before, loop, state)
	case *ast.TypeSwitchStmt:
		if value.Init != nil {
			ps6099FlowStmt(pass, value.Init, token.NoPos, loop, state)
		}
		if value.Assign != nil {
			ps6099FlowStmt(pass, value.Assign, token.NoPos, loop, state)
		}
		ps6099FlowClauses(pass, value.Body.List, before, loop, state)
	case *ast.SelectStmt:
		ps6099FlowCommClauses(pass, value.Body.List, before, loop, state)
	}
}

// ps6099SelectedSwitchClause returns the statically selected case, or -1 for
// a switch that has no matching case and no default. Unknown comparisons retain
// the conservative all-clauses flow join.
func ps6099SelectedSwitchClause(pass *analysis.Pass, statement *ast.SwitchStmt) (int, bool) {
	if statement == nil || statement.Body == nil {
		return -1, false
	}
	defaultClause := -1
	for index, item := range statement.Body.List {
		clause, ok := item.(*ast.CaseClause)
		if !ok {
			continue
		}
		if len(clause.List) == 0 {
			defaultClause = index
			continue
		}
		for _, expression := range clause.List {
			matches, known := ps6099SwitchCaseCondition(pass, statement, expression)
			if !known {
				return -1, false
			}
			if matches {
				return index, true
			}
		}
	}
	return defaultClause, true
}

// ps6099FlowSelectedSwitchClauses follows a source-proved switch selection.
// It only proceeds into a later clause when the selected clause's direct last
// statement is fallthrough; normal case completion exits the switch.
func ps6099FlowSelectedSwitchClauses(pass *analysis.Pass, clauses []ast.Stmt, selected int, before token.Pos, loop *ps6099Loop, state map[types.Object]bool) {
	for index := selected; index >= 0 && index < len(clauses); index++ {
		clause, ok := clauses[index].(*ast.CaseClause)
		if !ok {
			return
		}
		ps6099FlowBlock(pass, &ast.BlockStmt{List: clause.Body}, before, loop, state)
		if before.IsValid() && before <= clause.End() || !ps6099CaseFallsThrough(clause) {
			return
		}
	}
}

func ps6099CaseFallsThrough(clause *ast.CaseClause) bool {
	if clause == nil || len(clause.Body) == 0 {
		return false
	}
	branch, ok := clause.Body[len(clause.Body)-1].(*ast.BranchStmt)
	return ok && branch.Tok == token.FALLTHROUGH
}

func ps6099FlowAssignment(pass *analysis.Pass, assignment *ast.AssignStmt, loop *ps6099Loop, state map[types.Object]bool) {
	if len(assignment.Lhs) != len(assignment.Rhs) {
		for _, left := range assignment.Lhs {
			if identifier, ok := ps2110Unparen(left).(*ast.Ident); ok {
				state[pass.TypesInfo.ObjectOf(identifier)] = false
			}
		}
		return
	}
	values := make([]bool, len(assignment.Rhs))
	for index, right := range assignment.Rhs {
		values[index] = ps6099ExpressionDepends(pass, right, loop, state)
	}
	for index, left := range assignment.Lhs {
		identifier, ok := ps2110Unparen(left).(*ast.Ident)
		if !ok {
			ps6099InvalidateDependence(state)
			continue
		}
		object := pass.TypesInfo.ObjectOf(identifier)
		switch assignment.Tok {
		case token.ASSIGN, token.DEFINE:
			state[object] = values[index]
		default:
			state[object] = state[object] || values[index]
		}
	}
}

func ps6099InvalidateDependence(state map[types.Object]bool) {
	for object := range state {
		state[object] = false
	}
}

func ps6099FlowClauses(pass *analysis.Pass, clauses []ast.Stmt, before token.Pos, loop *ps6099Loop, state map[types.Object]bool) {
	var branches []map[types.Object]bool
	hasDefault := false
	for _, statement := range clauses {
		clause, ok := statement.(*ast.CaseClause)
		if !ok {
			continue
		}
		hasDefault = hasDefault || len(clause.List) == 0
		branch := ps6099CloneDependence(state)
		ps6099FlowBlock(pass, &ast.BlockStmt{List: clause.Body}, before, loop, branch)
		branches = append(branches, branch)
	}
	if !hasDefault {
		branches = append(branches, ps6099CloneDependence(state))
	}
	ps6099MergeDependence(state, branches...)
}

func ps6099FlowCommClauses(pass *analysis.Pass, clauses []ast.Stmt, before token.Pos, loop *ps6099Loop, state map[types.Object]bool) {
	var branches []map[types.Object]bool
	for _, statement := range clauses {
		clause, ok := statement.(*ast.CommClause)
		if !ok {
			continue
		}
		branch := ps6099CloneDependence(state)
		if clause.Comm != nil {
			ps6099FlowStmt(pass, clause.Comm, token.NoPos, loop, branch)
		}
		ps6099FlowBlock(pass, &ast.BlockStmt{List: clause.Body}, before, loop, branch)
		branches = append(branches, branch)
	}
	ps6099MergeDependence(state, branches...)
}

func ps6099CloneDependence(state map[types.Object]bool) map[types.Object]bool {
	result := make(map[types.Object]bool, len(state))
	for object, dependent := range state {
		result[object] = dependent
	}
	return result
}

func ps6099CopyDependence(destination, source map[types.Object]bool) {
	clear(destination)
	for object, dependent := range source {
		destination[object] = dependent
	}
}

const ps6099FlowStateCap = 64

func ps6099FlowIterations(state map[types.Object]bool, exact uint64, known bool, minimum uint64, transfer func(map[types.Object]bool)) {
	if known {
		ps6099FlowExactIterations(state, exact, transfer)
		return
	}
	current := ps6099CloneDependence(state)
	var history []map[types.Object]bool
	var meet map[types.Object]bool
	for iteration := uint64(0); ; iteration++ {
		if iteration >= minimum {
			if meet == nil {
				meet = ps6099CloneDependence(current)
			} else {
				ps6099MeetDependence(meet, current)
			}
		}
		if ps6099DependenceSeen(history, current) {
			if meet == nil {
				ps6099InvalidateDependence(state)
			} else {
				ps6099CopyDependence(state, meet)
			}
			return
		}
		if len(history) >= ps6099FlowStateCap {
			ps6099InvalidateDependence(state)
			return
		}
		history = append(history, ps6099CloneDependence(current))
		transfer(current)
	}
}

func ps6099FlowExactIterations(state map[types.Object]bool, exact uint64, transfer func(map[types.Object]bool)) {
	current := ps6099CloneDependence(state)
	var history []map[types.Object]bool
	for iteration := uint64(0); iteration < exact; iteration++ {
		if previous := ps6099DependenceIndex(history, current); previous >= 0 {
			cycle := uint64(len(history) - previous)
			target := uint64(previous) + (exact-uint64(previous))%cycle
			ps6099CopyDependence(state, history[target])
			return
		}
		if len(history) >= ps6099FlowStateCap {
			ps6099InvalidateDependence(state)
			return
		}
		history = append(history, ps6099CloneDependence(current))
		transfer(current)
	}
	ps6099CopyDependence(state, current)
}

func ps6099DependenceSeen(history []map[types.Object]bool, state map[types.Object]bool) bool {
	return ps6099DependenceIndex(history, state) >= 0
}

func ps6099DependenceIndex(history []map[types.Object]bool, state map[types.Object]bool) int {
	for index, candidate := range history {
		if ps6099SameDependence(candidate, state) {
			return index
		}
	}
	return -1
}

func ps6099SameDependence(left, right map[types.Object]bool) bool {
	for object, dependent := range left {
		if right[object] != dependent {
			return false
		}
	}
	for object, dependent := range right {
		if left[object] != dependent {
			return false
		}
	}
	return true
}

func ps6099MeetDependence(destination, source map[types.Object]bool) {
	objects := make(map[types.Object]bool, len(destination))
	for object := range destination {
		objects[object] = true
	}
	for object := range source {
		objects[object] = true
	}
	for object := range objects {
		destination[object] = destination[object] && source[object]
	}
}

func ps6099ForGuaranteedEntry(pass *analysis.Pass, loop *ast.ForStmt) bool {
	if loop == nil || loop.Cond == nil {
		return false
	}
	if value := pass.TypesInfo.Types[loop.Cond].Value; value != nil && value.Kind() == constant.Bool {
		return constant.BoolVal(value)
	}
	initializer, ok := loop.Init.(*ast.AssignStmt)
	if !ok || initializer.Tok != token.ASSIGN && initializer.Tok != token.DEFINE ||
		len(initializer.Lhs) != 1 || len(initializer.Rhs) != 1 {
		return false
	}
	identifier, ok := ps2110Unparen(initializer.Lhs[0]).(*ast.Ident)
	if !ok {
		return false
	}
	object := ps6099AssignedObject(pass, identifier, initializer.Tok)
	initial := ps6099StaticNumber(pass, initializer.Rhs[0])
	condition, ok := ps2110Unparen(loop.Cond).(*ast.BinaryExpr)
	if object == nil || initial == nil || !ok || !ps6099Comparison(condition.Op) {
		return false
	}
	if ps6099Object(pass, condition.X, object) {
		bound := ps6099StaticNumber(pass, condition.Y)
		return bound != nil && constant.Compare(initial, condition.Op, bound)
	}
	if ps6099Object(pass, condition.Y, object) {
		bound := ps6099StaticNumber(pass, condition.X)
		return bound != nil && constant.Compare(bound, condition.Op, initial)
	}
	return false
}

func ps6099ForNeverTerminates(pass *analysis.Pass, loop *ast.ForStmt) bool {
	if loop == nil || !ps6099ForGuaranteedEntry(pass, loop) {
		return false
	}
	initializer, ok := loop.Init.(*ast.AssignStmt)
	if !ok || initializer.Tok != token.ASSIGN && initializer.Tok != token.DEFINE || len(initializer.Lhs) != 1 {
		return false
	}
	identifier, ok := ps2110Unparen(initializer.Lhs[0]).(*ast.Ident)
	if !ok {
		return false
	}
	object := ps6099AssignedObject(pass, identifier, initializer.Tok)
	if object == nil || ps6099LoopObjectEscapes(pass, loop, object) {
		return false
	}
	if loop.Post != nil {
		if !ps6099ZeroStep(pass, loop.Post, object) {
			return false
		}
	}
	stable := true
	parents := ps6087Parents(loop)
	reachable := ps6099ReachableNodesInBlock(pass, loop.Body, parents)
	ast.Inspect(loop.Body, func(node ast.Node) bool {
		if !stable {
			return false
		}
		if nested, ok := node.(*ast.FuncLit); ok {
			if ps6099NodeMentionsObject(pass, nested.Body, object) {
				stable = false
			}
			return false
		}
		if node == nil || !ps6099NodeReachable(pass, node, parents, reachable[node]) {
			return true
		}
		switch value := node.(type) {
		case *ast.AssignStmt:
			for _, left := range value.Lhs {
				if ps6099Object(pass, left, object) {
					stable = false
					return false
				}
			}
		case *ast.IncDecStmt:
			if ps6099Object(pass, value.X, object) {
				stable = false
				return false
			}
		case *ast.RangeStmt:
			if ps6099Object(pass, value.Key, object) || ps6099Object(pass, value.Value, object) {
				stable = false
				return false
			}
		case *ast.UnaryExpr:
			if value.Op == token.AND && ps6099Object(pass, value.X, object) {
				stable = false
				return false
			}
		case *ast.BranchStmt:
			target := ps6099LoopBranchTarget(pass, loop, value, parents)
			exitsLoop := target == loop || target != nil && ps6099NodeWithin(loop, target)
			if value.Tok == token.BREAK && exitsLoop || value.Tok == token.CONTINUE && target != loop && exitsLoop ||
				value.Tok == token.GOTO && !ps6099NodeWithin(target, loop.Body) {
				stable = false
				return false
			}
		}
		return true
	})
	return stable
}

func ps6099LoopObjectEscapes(pass *analysis.Pass, loop *ast.ForStmt, object types.Object) bool {
	if loop == nil || object == nil {
		return true
	}
	if variable, ok := object.(*types.Var); !ok || variable.Parent() == pass.Pkg.Scope() {
		return true
	}
	var scope *ast.BlockStmt
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if ok && function.Body != nil && ps6099NodeWithin(loop, function.Body) {
				scope = function.Body
				break
			}
		}
		if scope != nil {
			break
		}
	}
	if scope == nil {
		return true
	}
	parents := ps6087Parents(scope)
	escaped := false
	ast.Inspect(scope, func(node ast.Node) bool {
		if escaped || node == nil || node.Pos() >= loop.Pos() {
			return false
		}
		identifier, ok := node.(*ast.Ident)
		if !ok || pass.TypesInfo.ObjectOf(identifier) != object {
			return true
		}
		for parent := parents[identifier]; parent != nil && parent != scope; parent = parents[parent] {
			switch value := parent.(type) {
			case *ast.FuncLit:
				escaped = true
				return false
			case *ast.UnaryExpr:
				if value.Op == token.AND {
					escaped = true
					return false
				}
			}
		}
		return true
	})
	return escaped
}

func ps6099NodeMentionsObject(pass *analysis.Pass, node ast.Node, object types.Object) bool {
	mentioned := false
	ast.Inspect(node, func(candidate ast.Node) bool {
		identifier, ok := candidate.(*ast.Ident)
		if ok && pass.TypesInfo.ObjectOf(identifier) == object {
			mentioned = true
			return false
		}
		return !mentioned
	})
	return mentioned
}

func ps6099ZeroStep(pass *analysis.Pass, statement ast.Stmt, object types.Object) bool {
	assignment, ok := statement.(*ast.AssignStmt)
	if !ok || assignment.Tok != token.ADD_ASSIGN && assignment.Tok != token.SUB_ASSIGN ||
		len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 || !ps6099Object(pass, assignment.Lhs[0], object) {
		return false
	}
	value := pass.TypesInfo.Types[assignment.Rhs[0]].Value
	return value != nil && (value.Kind() == constant.Int || value.Kind() == constant.Float) && constant.Sign(value) == 0
}

func ps6099LoopBranchTarget(pass *analysis.Pass, loop *ast.ForStmt, branch *ast.BranchStmt, parents map[ast.Node]ast.Node) ast.Node {
	if branch == nil {
		return nil
	}
	if branch.Label == nil {
		for parent := parents[branch]; parent != nil; parent = parents[parent] {
			switch branch.Tok {
			case token.BREAK:
				switch parent.(type) {
				case *ast.ForStmt, *ast.RangeStmt, *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt:
					return parent
				}
			case token.CONTINUE:
				switch parent.(type) {
				case *ast.ForStmt, *ast.RangeStmt:
					return parent
				}
			}
		}
		return nil
	}
	label := pass.TypesInfo.ObjectOf(branch.Label)
	for _, file := range pass.Files {
		var target ast.Node
		ast.Inspect(file, func(node ast.Node) bool {
			if target != nil {
				return false
			}
			statement, ok := node.(*ast.LabeledStmt)
			if ok && (label != nil && pass.TypesInfo.ObjectOf(statement.Label) == label ||
				label == nil && statement.Label.Name == branch.Label.Name) {
				target = statement.Stmt
				return false
			}
			return true
		})
		if target != nil {
			return target
		}
	}
	return nil
}

func ps6099ForExactIterations(pass *analysis.Pass, loop *ast.ForStmt) (uint64, bool) {
	if loop == nil || loop.Cond == nil {
		return 0, false
	}
	if value := pass.TypesInfo.Types[loop.Cond].Value; value != nil && value.Kind() == constant.Bool {
		if !constant.BoolVal(value) {
			return 0, true
		}
		return 0, false
	}
	initializer, ok := loop.Init.(*ast.AssignStmt)
	if !ok || initializer.Tok != token.ASSIGN && initializer.Tok != token.DEFINE ||
		len(initializer.Lhs) != 1 || len(initializer.Rhs) != 1 {
		return 0, false
	}
	identifier, ok := ps2110Unparen(initializer.Lhs[0]).(*ast.Ident)
	if !ok {
		return 0, false
	}
	object := ps6099AssignedObject(pass, identifier, initializer.Tok)
	initial, initialOK := ps6099Integer64(ps6099StaticInteger(pass, initializer.Rhs[0]))
	condition, ok := ps2110Unparen(loop.Cond).(*ast.BinaryExpr)
	if object == nil || !ps6099PlainInt(object.Type()) || !initialOK || !ok || !ps6099Comparison(condition.Op) {
		return 0, false
	}
	operation := condition.Op
	var bound int64
	var boundOK bool
	if ps6099Object(pass, condition.X, object) {
		bound, boundOK = ps6099Integer64(ps6099StaticInteger(pass, condition.Y))
	} else if ps6099Object(pass, condition.Y, object) {
		bound, boundOK = ps6099Integer64(ps6099StaticInteger(pass, condition.X))
		operation = ps6099ReverseComparison(operation)
	} else {
		return 0, false
	}
	step, stepOK := ps6099ForStep(pass, loop.Post, object)
	if !boundOK || !stepOK || step == 0 {
		return 0, false
	}
	iterations, known := ps6099IterationCount(initial, bound, step, operation)
	if !known || !ps6099IntPostRepresentable(pass, initial, step, iterations) {
		return 0, false
	}
	return iterations, true
}

func ps6099PlainInt(typ types.Type) bool {
	basic, ok := types.Unalias(typ).Underlying().(*types.Basic)
	return ok && basic.Kind() == types.Int
}

func ps6099IntPostRepresentable(pass *analysis.Pass, initial, step int64, iterations uint64) bool {
	if pass.TypesSizes == nil {
		return false
	}
	value := constant.BinaryOp(
		constant.MakeInt64(initial),
		token.ADD,
		constant.BinaryOp(constant.MakeUint64(iterations), token.MUL, constant.MakeInt64(step)),
	)
	final, ok := constant.Int64Val(value)
	if !ok {
		return false
	}
	bits := uint(pass.TypesSizes.Sizeof(types.Typ[types.Int]) * 8)
	if bits == 0 || bits > 64 {
		return false
	}
	if bits == 64 {
		return true
	}
	minimum := -int64(1) << (bits - 1)
	maximum := int64(1)<<(bits-1) - 1
	return final >= minimum && final <= maximum
}

func ps6099ForStep(pass *analysis.Pass, statement ast.Stmt, object types.Object) (int64, bool) {
	switch value := statement.(type) {
	case *ast.IncDecStmt:
		if !ps6099Object(pass, value.X, object) {
			return 0, false
		}
		if value.Tok == token.INC {
			return 1, true
		}
		if value.Tok == token.DEC {
			return -1, true
		}
	case *ast.AssignStmt:
		if len(value.Lhs) != 1 || len(value.Rhs) != 1 || !ps6099Object(pass, value.Lhs[0], object) {
			return 0, false
		}
		step, ok := ps6099Integer64(ps6099StaticInteger(pass, value.Rhs[0]))
		if !ok {
			return 0, false
		}
		switch value.Tok {
		case token.ADD_ASSIGN:
			return step, true
		case token.SUB_ASSIGN:
			if step == -1<<63 {
				return 0, false
			}
			return -step, true
		}
	}
	return 0, false
}

func ps6099IterationCount(initial, bound, step int64, operation token.Token) (uint64, bool) {
	switch operation {
	case token.EQL:
		if initial == bound {
			return 1, true
		}
		return 0, true
	case token.NEQ:
		if initial == bound {
			return 0, true
		}
		if initial < bound && step <= 0 || initial > bound && step >= 0 {
			return 0, false
		}
		distance := ps6099UnsignedDistance(initial, bound)
		magnitude := ps6099UnsignedMagnitude(step)
		if magnitude == 0 || distance%magnitude != 0 {
			return 0, false
		}
		return distance / magnitude, true
	case token.LSS:
		if initial >= bound {
			return 0, true
		}
		if step <= 0 {
			return 0, false
		}
		distance := ps6099UnsignedDistance(initial, bound)
		magnitude := uint64(step)
		return 1 + (distance-1)/magnitude, true
	case token.LEQ:
		if initial > bound {
			return 0, true
		}
		if step <= 0 {
			return 0, false
		}
		return ps6099UnsignedDistance(initial, bound)/uint64(step) + 1, true
	case token.GTR:
		if initial <= bound {
			return 0, true
		}
		if step >= 0 {
			return 0, false
		}
		distance := ps6099UnsignedDistance(initial, bound)
		magnitude := ps6099UnsignedMagnitude(step)
		return 1 + (distance-1)/magnitude, true
	case token.GEQ:
		if initial < bound {
			return 0, true
		}
		if step >= 0 {
			return 0, false
		}
		return ps6099UnsignedDistance(initial, bound)/ps6099UnsignedMagnitude(step) + 1, true
	}
	return 0, false
}

func ps6099Integer64(value constant.Value) (int64, bool) {
	if value == nil || value.Kind() != constant.Int {
		return 0, false
	}
	return constant.Int64Val(value)
}

func ps6099UnsignedDistance(left, right int64) uint64 {
	if left < right {
		return uint64(right) - uint64(left)
	}
	return uint64(left) - uint64(right)
}

func ps6099UnsignedMagnitude(value int64) uint64 {
	if value < 0 {
		return uint64(-(value + 1)) + 1
	}
	return uint64(value)
}

func ps6099ReverseComparison(operation token.Token) token.Token {
	switch operation {
	case token.LSS:
		return token.GTR
	case token.LEQ:
		return token.GEQ
	case token.GTR:
		return token.LSS
	case token.GEQ:
		return token.LEQ
	}
	return operation
}

func ps6099StaticInteger(pass *analysis.Pass, expression ast.Expr) constant.Value {
	if value := pass.TypesInfo.Types[expression].Value; value != nil && value.Kind() == constant.Int {
		return value
	}
	call, ok := ps2110Unparen(expression).(*ast.CallExpr)
	if !ok || len(call.Args) != 1 || !typedBuiltinName(pass, call.Fun, "len") {
		return nil
	}
	typ := types.Unalias(pass.TypesInfo.TypeOf(call.Args[0]))
	if typ == nil {
		return nil
	}
	if array, ok := typ.Underlying().(*types.Array); ok {
		return constant.MakeInt64(array.Len())
	}
	return nil
}

func ps6099StaticNumber(pass *analysis.Pass, expression ast.Expr) constant.Value {
	value := pass.TypesInfo.Types[expression].Value
	if value != nil && (value.Kind() == constant.Int || value.Kind() == constant.Float) {
		return value
	}
	return ps6099StaticInteger(pass, expression)
}

func ps6099Comparison(operation token.Token) bool {
	switch operation {
	case token.EQL, token.NEQ, token.LSS, token.LEQ, token.GTR, token.GEQ:
		return true
	}
	return false
}

func ps6099RangeGuaranteedEntry(pass *analysis.Pass, expression ast.Expr) bool {
	typ := types.Unalias(pass.TypesInfo.TypeOf(expression))
	if typ != nil {
		switch value := typ.Underlying().(type) {
		case *types.Array:
			return value.Len() > 0
		case *types.Pointer:
			array, ok := types.Unalias(value.Elem()).Underlying().(*types.Array)
			return ok && array.Len() > 0
		}
	}
	value := pass.TypesInfo.Types[expression].Value
	if value == nil {
		return false
	}
	switch value.Kind() {
	case constant.String:
		return len(constant.StringVal(value)) > 0
	case constant.Int:
		return constant.Sign(value) > 0
	}
	return false
}

func ps6099RangeExactIterations(pass *analysis.Pass, expression ast.Expr) (uint64, bool) {
	typ := types.Unalias(pass.TypesInfo.TypeOf(expression))
	if typ != nil {
		switch value := typ.Underlying().(type) {
		case *types.Array:
			return uint64(value.Len()), true
		case *types.Pointer:
			if array, ok := types.Unalias(value.Elem()).Underlying().(*types.Array); ok {
				return uint64(array.Len()), true
			}
		}
	}
	value := pass.TypesInfo.Types[expression].Value
	if value == nil {
		return 0, false
	}
	switch value.Kind() {
	case constant.String:
		return uint64(utf8.RuneCountInString(constant.StringVal(value))), true
	case constant.Int:
		if constant.Sign(value) <= 0 {
			return 0, true
		}
		return constant.Uint64Val(value)
	}
	return 0, false
}

func ps6099MergeDependence(destination map[types.Object]bool, branches ...map[types.Object]bool) {
	objects := make(map[types.Object]bool, len(destination))
	for object := range destination {
		objects[object] = true
	}
	for _, branch := range branches {
		for object := range branch {
			objects[object] = true
		}
	}
	for object := range objects {
		dependent := len(branches) != 0
		for _, branch := range branches {
			dependent = dependent && branch[object]
		}
		destination[object] = dependent
	}
}

func ps6099DependsOnIteration(pass *analysis.Pass, arguments []ast.Expr, loop *ps6099Loop, definitions map[types.Object]bool) bool {
	for _, argument := range arguments {
		if ps6099ExpressionDepends(pass, argument, loop, definitions) {
			return true
		}
	}
	return false
}

func ps6099ExpressionDepends(pass *analysis.Pass, expression ast.Expr, loop *ps6099Loop, definitions map[types.Object]bool) bool {
	found := false
	ast.Inspect(expression, func(node ast.Node) bool {
		if found {
			return false
		}
		identifier, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		object := pass.TypesInfo.ObjectOf(identifier)
		if object == loop.index || object != nil && object == loop.rangeValue {
			found = true
			return false
		}
		found = object != nil && definitions[object]
		return !found
	})
	return found
}

func ps6099Precision(typ types.Type) string {
	if typ == nil {
		return ""
	}
	if parameter, ok := types.Unalias(typ).(*types.TypeParam); ok {
		return ps6099TypeParameterPrecision(parameter)
	}
	basic, ok := types.Unalias(typ).Underlying().(*types.Basic)
	if !ok {
		return ""
	}
	switch basic.Kind() {
	case types.Float32:
		return "float32"
	case types.Float64:
		return "float64"
	}
	return ""
}

func ps6099TypeParameterPrecision(parameter *types.TypeParam) string {
	if parameter == nil {
		return ""
	}
	constraint, ok := parameter.Constraint().Underlying().(*types.Interface)
	if !ok {
		return ""
	}
	constraint.Complete()
	precision := ""
	for index := 0; index < constraint.NumEmbeddeds(); index++ {
		embedded := types.Unalias(constraint.EmbeddedType(index))
		union, ok := embedded.(*types.Union)
		if !ok {
			candidate := ps6099Precision(embedded)
			if candidate == "" || precision != "" && precision != candidate {
				return ""
			}
			precision = candidate
			continue
		}
		for termIndex := 0; termIndex < union.Len(); termIndex++ {
			candidate := ps6099Precision(union.Term(termIndex).Type())
			if candidate == "" || precision != "" && precision != candidate {
				return ""
			}
			precision = candidate
		}
	}
	return precision
}

func ps6099BestLeaf(leaves []ps6099Leaf, precision string, destination types.Type) (ps6099Leaf, bool) {
	if precision == "" || ps6099SequencePrecision(destination, nil, nil) != precision {
		return ps6099Leaf{}, false
	}
	for _, leaf := range leaves {
		if leaf.precision == precision && ps6099LeafAcceptsDestination(leaf, destination) {
			return leaf, true
		}
	}
	return ps6099Leaf{}, false
}

func ps6099LeafAcceptsDestination(leaf ps6099Leaf, destination types.Type) bool {
	actual := ps6099DestinationSliceType(destination)
	if actual == nil {
		return false
	}
	for _, sequence := range leaf.sequences {
		if sequence.typ != nil {
			if ps6099TypedSequenceAccepts(actual, sequence.typ) {
				return true
			}
			continue
		}
		if ps6099SyntaxSequenceAccepts(actual, sequence.syntax, leaf.syntaxTypes, leaf.syntaxAliases, make(map[string]bool)) {
			return true
		}
	}
	return false
}

// ps6099DestinationSliceType returns the exact slice value that can be handed
// to a batch leaf without copying. An array destination can be sliced in place;
// a by-value array formal is deliberately never accepted as an in-place leaf.
func ps6099DestinationSliceType(destination types.Type) types.Type {
	if destination == nil {
		return nil
	}
	underlying := types.Unalias(destination).Underlying()
	switch value := underlying.(type) {
	case *types.Slice:
		return destination
	case *types.Array:
		return types.NewSlice(value.Elem())
	}
	return nil
}

func ps6099TypedSequenceAccepts(actual, formal types.Type) bool {
	if actual == nil || formal == nil {
		return false
	}
	if _, ok := types.Unalias(formal).Underlying().(*types.Slice); !ok {
		return false
	}
	if types.AssignableTo(actual, formal) || types.ConvertibleTo(actual, formal) {
		return true
	}
	actualSlice, actualOK := types.Unalias(actual).Underlying().(*types.Slice)
	formalSlice, formalOK := types.Unalias(formal).Underlying().(*types.Slice)
	if !actualOK || !formalOK {
		return false
	}
	return ps6099GenericElementAccepts(actualSlice.Elem(), formalSlice.Elem())
}

func ps6099GenericElementAccepts(actual, formal types.Type) bool {
	formal = types.Unalias(formal)
	if parameter, ok := formal.(*types.TypeParam); ok {
		constraint, ok := parameter.Constraint().Underlying().(*types.Interface)
		return ok && types.Satisfies(actual, constraint)
	}
	return types.Identical(actual, formal)
}

func ps6099SyntaxSequenceAccepts(actual types.Type, syntax ast.Expr, typesByName map[string]ast.Expr, aliases map[string]bool, active map[string]bool) bool {
	actualSlice, ok := types.Unalias(actual).Underlying().(*types.Slice)
	if !ok {
		return false
	}
	syntax = ps2110Unparen(syntax)
	switch value := syntax.(type) {
	case *ast.Ident:
		if active[value.Name] || typesByName[value.Name] == nil {
			return false
		}
		active[value.Name] = true
		accepted := ps6099SyntaxSequenceAccepts(actual, typesByName[value.Name], typesByName, aliases, active)
		delete(active, value.Name)
		return accepted
	case *ast.ArrayType:
		if value.Len != nil {
			return false
		}
		return ps6099SyntaxElementMatches(actualSlice.Elem(), value.Elt, typesByName, aliases, make(map[string]bool))
	}
	return false
}

func ps6099SyntaxElementMatches(actual types.Type, syntax ast.Expr, typesByName map[string]ast.Expr, aliases map[string]bool, active map[string]bool) bool {
	identifier, ok := ps2110Unparen(syntax).(*ast.Ident)
	if !ok {
		return false
	}
	if identifier.Name == "float32" || identifier.Name == "float64" {
		basic, ok := types.Unalias(actual).(*types.Basic)
		return ok && basic.Name() == identifier.Name
	}
	definition := typesByName[identifier.Name]
	if definition == nil || active[identifier.Name] {
		return false
	}
	if !aliases[identifier.Name] {
		named, ok := types.Unalias(actual).(*types.Named)
		return ok && named.Obj() != nil && named.Obj().Name() == identifier.Name
	}
	active[identifier.Name] = true
	accepted := ps6099SyntaxElementMatches(actual, definition, typesByName, aliases, active)
	delete(active, identifier.Name)
	return accepted
}
