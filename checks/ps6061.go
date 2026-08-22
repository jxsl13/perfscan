package checks

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS6061 implements owner issue #778. It recognizes the float32 -> float64 ->
// math.Abs -> float32 scalar shape whose signaling-NaN encoding has changed
// across Go toolchains and must be measured before a native SIMD replacement.
var PS6061 = register(&lint.Check{
	ID:       "PS6061",
	Category: "arith",
	Slug:     "exact-f32-abs-loop-needs-bit-checked-simd",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "a widened float32 Abs loop needs toolchain-specific raw-bit validation before SIMD replacement",
		Text: `The canonical Go float32 absolute-value leaf often widens each lane
to float64 for math.Abs and narrows the result again:

  dst[i] = float32(math.Abs(float64(src[i])))

Finite values, zero, and infinities have their sign bit cleared. Signaling-NaN
encoding, however, is not a portable raw-bit contract of Go's numeric
conversions. Go 1.26.6/darwin-arm64 quieted float32 signaling NaNs during the
widen/narrow round trip, while Go 1.27/darwin-arm64 preserves the signaling
payload for the same source expression. Consequently neither "always OR the
quiet bit" nor "plain FABS/sign-mask is always exact" is a valid universal
recipe. A native implementation must match a scalar golden compiled with every
supported toolchain/architecture combination.

This check implements owner issue #778. It reports only a canonical range loop
whose body is exactly one assignment, whose source and destination are flat
float32 slices indexed by the loop induction variable, and whose right-hand
side is the typed float32(math.Abs(float64(src[i]))) chain. math.Abs and both
conversions are resolved through go/types, so aliases work while shadowed
functions, float64 inputs, missing widen/narrow steps, changed indices,
additional statements, and other math calls stay silent. In-place loops are
valid candidates too.

Start with sign clearing, then derive signaling-NaN handling from same-binary
scalar results rather than a hard-coded quieting rule. Validate raw bits for
positive/negative finite values, both zero signs, both infinities, positive and
negative quiet/signaling NaNs, sign clearing, payload handling, and any
toolchain-specific quieting. Include unaligned slices and lengths around every
vector tail, and run that differential suite on every supported CI target.

After vectorizing the serial leaf, remeasure an Abs-specific serial/parallel
crossover at the complete operation boundary. A global worker threshold tuned
for the old scalar leaf can add dispatch and closure cost below the new
crossover. Use order-alternating same-binary campaigns, preserve the scalar
tail/fallback, inspect instructions, and gate allocations. A
//perfscan:measured-abs-fallback annotation or a same-dtype registered/native
SIMD Abs sibling suppresses an intentionally handled site.

There is NO automatic fix: portable Go has no primitive expressing a stable
cross-toolchain SIMD raw-bit contract, while assembly registration and tails
are project-specific. A substitution is safe only after it matches the scalar
golden for the actual support matrix.`,
		Before: `for i := range dst {
	dst[i] = float32(math.Abs(float64(src[i])))
}`,
		After: `// Architecture/toolchain-specific candidate, not portable Go pseudocode:
mag := bits & 0x7fffffff
result := applyObservedScalarNaNContract(mag)
// Keep the scalar Go loop as the tail/fallback and retune its parallel gate.`,
		MeasuredWin: `On Go 1.26.6/darwin-arm64 on Apple M2 Pro, the exact
NEON implementation improved complete-operation medians by 1.176x-2.608x
across 2K-8M float32 elements in three independent -benchtime=100x -count=7
campaigns. Vectorizing the serial leaf moved the worker crossover: an
Abs-specific 1<<18 serial-vector threshold beat parallel dispatch below
262,144 elements and removed one small-shape closure allocation.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6061",
		Doc:  "widened float32 math.Abs loop needs toolchain-specific raw-bit validation before native SIMD",
		Run:  runPS6061,
	},
})

type ps6061Match struct {
	loop     *ast.RangeStmt
	srcName  string
	destName string
}

func runPS6061(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			ast.Inspect(function.Body, func(node ast.Node) bool {
				if _, nested := node.(*ast.FuncLit); nested {
					return false
				}
				loop, ok := node.(*ast.RangeStmt)
				if !ok {
					return true
				}
				match, ok := ps6061ExactAbs(pass, loop)
				if !ok || ps6061MeasuredFallback(file, function, loop) || ps6061NativeSibling(pass, function) {
					return true
				}
				pass.Reportf(loop.For, "exact float32 Abs loop %s[i] -> float64 -> math.Abs -> float32 -> %s[i] remains scalar; native SIMD must match a same-toolchain scalar raw-bit golden because signaling-NaN quieting changed between Go 1.26 and Go 1.27 — gate signed zero, infinities, quiet/signaling NaNs, sign clearing, payload handling, unaligned tails, instructions, and allocations across every supported target; remeasure the Abs-specific serial/parallel crossover with order-alternating same-binary complete-operation campaigns (advisory, no automatic fix)", match.srcName, match.destName)
				return true
			})
		}
	}
	return nil, nil
}

func ps6061ExactAbs(pass *analysis.Pass, loop *ast.RangeStmt) (ps6061Match, bool) {
	index, ok := loop.Key.(*ast.Ident)
	if !ok || index.Name == "_" || loop.Value != nil || len(loop.Body.List) != 1 {
		return ps6061Match{}, false
	}
	indexObject := identObject(pass, index)
	if indexObject == nil {
		return ps6061Match{}, false
	}
	assignment, ok := loop.Body.List[0].(*ast.AssignStmt)
	if !ok || assignment.Tok != token.ASSIGN || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
		return ps6061Match{}, false
	}
	destination, destinationKind, destinationName, ok := ps6060IndexedSlice(pass, assignment.Lhs[0], indexObject)
	if !ok || destinationKind != types.Float32 {
		return ps6061Match{}, false
	}

	narrow, ok := ps6061Conversion(pass, assignment.Rhs[0], types.Float32)
	if !ok {
		return ps6061Match{}, false
	}
	absCall, ok := ps2110Unparen(narrow.Args[0]).(*ast.CallExpr)
	if !ok || len(absCall.Args) != 1 || absCall.Ellipsis.IsValid() {
		return ps6061Match{}, false
	}
	absFunction, signature, ok := typedCallee(pass, absCall.Fun)
	if !ok || signature.Recv() != nil || absFunction.Pkg() == nil || absFunction.Pkg().Path() != "math" || absFunction.Name() != "Abs" {
		return ps6061Match{}, false
	}
	widen, ok := ps6061Conversion(pass, absCall.Args[0], types.Float64)
	if !ok {
		return ps6061Match{}, false
	}
	source, sourceKind, sourceName, ok := ps6060IndexedSlice(pass, widen.Args[0], indexObject)
	if !ok || sourceKind != types.Float32 {
		return ps6061Match{}, false
	}
	rangeID, ok := ps2110Unparen(loop.X).(*ast.Ident)
	if !ok {
		return ps6061Match{}, false
	}
	rangeObject := identObject(pass, rangeID)
	if rangeObject != source && rangeObject != destination {
		return ps6061Match{}, false
	}
	return ps6061Match{loop: loop, srcName: sourceName, destName: destinationName}, true
}

func ps6061Conversion(pass *analysis.Pass, expression ast.Expr, kind types.BasicKind) (*ast.CallExpr, bool) {
	call, ok := ps2110Unparen(expression).(*ast.CallExpr)
	if !ok || len(call.Args) != 1 || call.Ellipsis.IsValid() {
		return nil, false
	}
	typeValue, ok := pass.TypesInfo.Types[ps2110Unparen(call.Fun)]
	if !ok || !typeValue.IsType() {
		return nil, false
	}
	basic, ok := types.Unalias(typeValue.Type).(*types.Basic)
	return call, ok && basic.Kind() == kind
}

func ps6061MeasuredFallback(file *ast.File, function *ast.FuncDecl, loop *ast.RangeStmt) bool {
	for _, group := range file.Comments {
		adjacentToFunction := group.End() <= function.Pos() && function.Pos()-group.End() <= 3
		adjacentToLoop := group.End() <= loop.Pos() && loop.Pos()-group.End() <= 3
		insideFunction := group == function.Doc || adjacentToFunction || adjacentToLoop || group.Pos() >= function.Body.Pos() && group.End() <= function.Body.End()
		if !insideFunction {
			continue
		}
		for _, comment := range group.List {
			text := ps6058Compact(comment.Text)
			if strings.Contains(text, "perfscanmeasuredfallback") || strings.Contains(text, "perfscanmeasuredabsfallback") {
				return true
			}
		}
	}
	return false
}

func ps6061NativeSibling(pass *analysis.Pass, current *ast.FuncDecl) bool {
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function == current || !ps6061NativeAbsName(function.Name.Name) {
				continue
			}
			if ps6060NameDType(function.Name.Name, types.Float32) || ps6060SignatureDType(pass, function, types.Float32) {
				return true
			}
		}
		found := false
		ast.Inspect(file, func(node ast.Node) bool {
			if found {
				return false
			}
			call, ok := node.(*ast.CallExpr)
			if !ok || !ps6015RegistrationCall(pass, call) {
				return true
			}
			text := ps6007NormalizeName(ps6015NodeText(pass, call))
			found = strings.Contains(text, "abs") && ps6060NativeMarker(text) && ps6060NameDType(text, types.Float32)
			return !found
		})
		if found {
			return true
		}
	}
	return false
}

func ps6061NativeAbsName(name string) bool {
	name = ps6007NormalizeName(name)
	return strings.Contains(name, "abs") && ps6060NativeMarker(name)
}
