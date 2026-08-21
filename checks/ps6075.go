package checks

import (
	"cmp"
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"slices"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS6075 implements owner issue #792. It detects store-once accumulator tiles
// whose reduction loop walks a flat row-major source by the full row stride
// while consuming at most one quarter of each cache line.
var PS6075 = register(&lint.Check{
	ID:       "PS6075",
	Category: "verify",
	Slug:     "accumulator-hoist-strided-cache-line-underuse",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "a store-once accumulator tile destroys row-contiguous source traversal",
		Text: `Keeping several reduction accumulators in registers can remove repeated
destination stores while making a different operand dramatically less local.
The dangerous loop order puts a narrow output-column tile outside the complete
reduction:

  for col := 0; col < n; col += 2 {
      var c0, c1 float64
      for k := 0; k < depth; k++ {
          c0 += a[k] * b[k*n+col]
          c1 += a[k] * b[k*n+col+1]
      }
      dst[col], dst[col+1] = c0, c1
  }

The two b values are contiguous, but consecutive k iterations advance by a
whole n-element row. On a target with 128-byte cache lines, two float64 values
use 16 bytes and can fetch 128. The remaining line is not revisited until the
outer column tile advances after a full depth traversal. A row-contiguous k-j
order, cache-line-width tile, or packed panel can use the line before eviction.

This check implements owner issue #792. It type-checks a deliberately narrow,
self-contained proof:

  - a counted output-column loop directly encloses a counted reduction loop;
  - one or more local numeric accumulators are declared before that reduction,
    updated with += inside it, and stored to an indexed destination afterward;
  - the updates read a flat numeric slice at the affine row-major index
    reduction*outputBound + outputColumn + constantOffset;
  - the outer loop step covers the distinct contiguous offsets, so reuse of
    the rest of the source line waits for the complete reduction; and
  - the unique contiguous source width is at most one quarter of the configured
    cache-line size.

The row-stride expression must be the exact output-loop bound. That gives a
local proof that interchanging or strip-mining the output loop inside the
reduction produces unit-stride source traversal; no repository-specific names
or unavailable VCS history are guessed. Nested [][]T storage, maps, pointer
arithmetic, non-affine indexes, holes in the offset tile, accumulators declared
inside the reduction, missing post-reduction stores, and tiles wider than the
quarter-line threshold stay silent. A
//perfscan:cache-locality-validated function annotation suppresses a layout
already retained by complete shape campaigns.

Set cacheLineBytes in perfscan.yaml for the target architecture. Zero or an
omitted field uses a conservative 64-byte default; Apple M-series campaigns
should normally set 128. The diagnostic prints both sides of the tradeoff: the
maximum repeated destination-store bytes the hoist can avoid and the source
cache-line bytes fetched versus useful bytes under the strided traversal.

There is NO automatic fix. Bare loop interchange can change floating-point
reduction order, alias timing, bounds behavior, and vectorization. Prefer
cache-line-width tiling or packing that retains store-once accumulators while
restoring row-contiguous source use, then compare the complete direct kernels
in the same binary across square, wide, odd, and tail shapes.`,
		Before: `for col := 0; col < n; col += 2 {
	var acc0, acc1 float64
	for k := 0; k < depth; k++ {
		acc0 += a[k] * b[k*n+col]
		acc1 += a[k] * b[k*n+col+1]
	}
	dst[col], dst[col+1] = acc0, acc1
}`,
		After: `// Tile or pack b so a cache line is consumed before advancing k.
// Keep candidates separately selectable: loop changes may alter FP order.
// Retain only after same-binary square, wide, odd, and tail campaigns pass.`,
		MeasuredWin: `The owner campaign rejected the store-once candidate on
Apple M2 Pro: control/candidate medians were 2.248/4.171 ms for 512 cubed
(candidate 0.54x), 16.789/37.894 ms for 1024 cubed (0.44x), 39.686/350.426 ms
for 512x2048x2048 (0.11x), and 2.358/4.450 ms for 511x513x515 (0.53x).
The wide case consumed only 16 bytes of each 128-byte line before a full
reduction-length reuse distance, approaching the predicted 8x line-traffic
penalty. Five 300 ms samples used identical GOEXPERIMENT=simd binaries;
control was goai f33b9ead and the rejected candidate 6086beb1. A later
four-shape campaign measured the rejected interchange 1.431x-7.968x slower
with p=.001 in every cell. The operation-scoped packing plus 4x8 NEON design
merged in GoAI PR #1126 at 58a2fa4e3f1716a81326e2093100cffe70e2ab6b after
two complete 15-of-15 CI matrices.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6075",
		Doc:  "store-once reduction accumulators trade destination writes for severe strided source cache-line underuse",
		Run:  runPS6075,
	},
})

const ps6075DefaultCacheLineBytes int64 = 64

type ps6075Loop struct {
	statement *ast.ForStmt
	variable  *types.Var
	bound     ast.Expr
	step      int64
}

type ps6075Group struct {
	source       types.Object
	sourceName   string
	index        *ast.IndexExpr
	stride       ast.Expr
	elementBytes int64
	offsets      map[int64]bool
	accumulators map[types.Object]int64
}

type ps6075Finding struct {
	output         ps6075Loop
	reduction      ps6075Loop
	sourceName     string
	strideText     string
	elementBytes   int64
	usefulBytes    int64
	cacheLineBytes int64
	accumulators   int
	accumulatorB   int64
}

func runPS6075(pass *analysis.Pass) (any, error) {
	cacheLineBytes := int64(config.Current().CacheLineBytes)
	if cacheLineBytes <= 0 {
		cacheLineBytes = ps6075DefaultCacheLineBytes
	}
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil || ps6075Validated(function) {
				continue
			}
			reported := make(map[*ast.ForStmt]bool)
			astutil.WithStack(function.Body, func(node ast.Node, stack []ast.Node) bool {
				reductionStatement, ok := node.(*ast.ForStmt)
				if !ok || reported[reductionStatement] {
					return true
				}
				finding, ok := ps6075Match(pass, reductionStatement, stack, cacheLineBytes)
				if !ok {
					return true
				}
				if reported[finding.output.statement] {
					return false
				}
				reported[finding.output.statement] = true
				pass.Reportf(finding.output.statement.For, "%s hoists %d reduction accumulators across %s while source %s advances by %s*%d bytes per %s iteration and uses only %d of each configured %d-byte cache line (%.1f%%); reuse of the remaining line waits until the outer %s tile advances after the full reduction. Estimated tradeoff per tile: up to %s destination store bytes avoided versus %s source line bytes fetched for %s useful source bytes. Prefer cache-line-width tiling or packing that restores unit-stride source traversal, then gate complete same-binary square/wide/odd/tail kernels (advisory, no automatic fix)",
					function.Name.Name,
					finding.accumulators,
					exprTextRendered(finding.reduction.bound),
					finding.sourceName,
					finding.strideText,
					finding.elementBytes,
					finding.reduction.variable.Name(),
					finding.usefulBytes,
					finding.cacheLineBytes,
					100*float64(finding.usefulBytes)/float64(finding.cacheLineBytes),
					finding.output.variable.Name(),
					ps6075DestinationTraffic(pass, &finding),
					ps6075SourceTraffic(pass, &finding, finding.cacheLineBytes),
					ps6075SourceTraffic(pass, &finding, finding.usefulBytes),
				)
				return false
			})
		}
	}
	return nil, nil
}

func ps6075Match(pass *analysis.Pass, reductionStatement *ast.ForStmt, stack []ast.Node, cacheLineBytes int64) (ps6075Finding, bool) {
	reduction, ok := ps6075CountedLoop(pass, reductionStatement)
	if !ok || reduction.step != 1 || containsLoop(reductionStatement.Body) || ps6075ContainsFunctionLiteral(reductionStatement.Body) {
		return ps6075Finding{}, false
	}
	output, ok := ps6075EnclosingOutputLoop(pass, reductionStatement, stack)
	if !ok || output.variable == reduction.variable {
		return ps6075Finding{}, false
	}
	reductionIndex := -1
	for index, statement := range output.statement.Body.List {
		if statement == reductionStatement {
			reductionIndex = index
			break
		}
	}
	if reductionIndex < 0 {
		return ps6075Finding{}, false
	}

	groups := make(map[types.Object]*ps6075Group)
	ast.Inspect(reductionStatement.Body, func(node ast.Node) bool {
		assignment, ok := node.(*ast.AssignStmt)
		if !ok || assignment.Tok != token.ADD_ASSIGN || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
			return true
		}
		accumulator, ok := ps2110Unparen(assignment.Lhs[0]).(*ast.Ident)
		if !ok {
			return true
		}
		accumulatorObject := pass.TypesInfo.ObjectOf(accumulator)
		accumulatorBytes, ok := ps6075AccumulatorBytes(pass, accumulatorObject)
		if !ok || !ps6075HoistedAndStored(pass, output, reductionIndex, reductionStatement, accumulatorObject) {
			return true
		}
		ast.Inspect(assignment.Rhs[0], func(candidate ast.Node) bool {
			index, ok := candidate.(*ast.IndexExpr)
			if !ok {
				return true
			}
			source, sourceName, stride, offset, elementBytes, ok := ps6075AffineSource(pass, index, reduction, output)
			if !ok {
				return true
			}
			group := groups[source]
			if group == nil {
				group = &ps6075Group{
					source: source, sourceName: sourceName, index: index, stride: stride,
					elementBytes: elementBytes, offsets: make(map[int64]bool),
					accumulators: make(map[types.Object]int64),
				}
				groups[source] = group
			}
			if group.elementBytes == elementBytes {
				group.offsets[offset] = true
				group.accumulators[accumulatorObject] = accumulatorBytes
			}
			return false
		})
		return true
	})

	candidates := make([]ps6075Finding, 0, len(groups))
	for _, group := range groups {
		minimum, maximum, contiguous := ps6075ContiguousOffsets(group.offsets)
		if !contiguous {
			continue
		}
		span := maximum - minimum + 1
		if output.step < span || len(group.accumulators) < len(group.offsets) {
			continue
		}
		usefulBytes := span * group.elementBytes
		if usefulBytes <= 0 || usefulBytes*4 > cacheLineBytes || !ps6075NonUnitStride(pass, output.bound, group.elementBytes, usefulBytes) {
			continue
		}
		accumulatorBytes := int64(0)
		for _, size := range group.accumulators {
			accumulatorBytes += size
		}
		candidates = append(candidates, ps6075Finding{
			output: output, reduction: reduction, sourceName: group.sourceName,
			strideText: exprTextRendered(group.stride), elementBytes: group.elementBytes,
			usefulBytes: usefulBytes, cacheLineBytes: cacheLineBytes,
			accumulators: len(group.accumulators), accumulatorB: accumulatorBytes,
		})
	}
	if len(candidates) == 0 {
		return ps6075Finding{}, false
	}
	slices.SortFunc(candidates, func(left, right ps6075Finding) int {
		if byWidth := cmp.Compare(left.usefulBytes, right.usefulBytes); byWidth != 0 {
			return byWidth
		}
		return cmp.Compare(left.sourceName, right.sourceName)
	})
	return candidates[0], true
}

func ps6075CountedLoop(pass *analysis.Pass, statement *ast.ForStmt) (ps6075Loop, bool) {
	if statement == nil || statement.Init == nil || statement.Cond == nil || statement.Post == nil {
		return ps6075Loop{}, false
	}
	initializer, ok := statement.Init.(*ast.AssignStmt)
	if !ok || initializer.Tok != token.DEFINE || len(initializer.Lhs) != 1 || len(initializer.Rhs) != 1 {
		return ps6075Loop{}, false
	}
	identifier, ok := ps2110Unparen(initializer.Lhs[0]).(*ast.Ident)
	if !ok || !ps6075IntegerValue(pass, initializer.Rhs[0], 0) {
		return ps6075Loop{}, false
	}
	variable, ok := pass.TypesInfo.ObjectOf(identifier).(*types.Var)
	if !ok {
		return ps6075Loop{}, false
	}
	condition, ok := ps2110Unparen(statement.Cond).(*ast.BinaryExpr)
	if !ok || condition.Op != token.LSS || !ps6075ObjectExpression(pass, condition.X, variable) {
		return ps6075Loop{}, false
	}
	step := int64(0)
	switch post := statement.Post.(type) {
	case *ast.IncDecStmt:
		if post.Tok == token.INC && ps6075ObjectExpression(pass, post.X, variable) {
			step = 1
		}
	case *ast.AssignStmt:
		if post.Tok == token.ADD_ASSIGN && len(post.Lhs) == 1 && len(post.Rhs) == 1 &&
			ps6075ObjectExpression(pass, post.Lhs[0], variable) {
			if value, ok := ps6075ConstantInt(pass, post.Rhs[0]); ok && value > 0 {
				step = value
			}
		}
	}
	if step == 0 || !ps2008SimpleLen(condition.Y) || ps6075MentionsObject(pass, condition.Y, variable) {
		return ps6075Loop{}, false
	}
	return ps6075Loop{statement: statement, variable: variable, bound: condition.Y, step: step}, true
}

func ps6075EnclosingOutputLoop(pass *analysis.Pass, reduction *ast.ForStmt, stack []ast.Node) (ps6075Loop, bool) {
	for index := len(stack) - 1; index >= 1; index-- {
		candidate, ok := stack[index-1].(*ast.ForStmt)
		if !ok || stack[index] != ast.Node(candidate.Body) {
			continue
		}
		output, ok := ps6075CountedLoop(pass, candidate)
		if ok {
			return output, true
		}
	}
	return ps6075Loop{}, false
}

func ps6075AffineSource(pass *analysis.Pass, index *ast.IndexExpr, reduction, output ps6075Loop) (types.Object, string, ast.Expr, int64, int64, bool) {
	source, ok := ps2110Unparen(index.X).(*ast.Ident)
	if !ok {
		return nil, "", nil, 0, 0, false
	}
	sourceObject := pass.TypesInfo.ObjectOf(source)
	slice, ok := types.Unalias(pass.TypesInfo.TypeOf(source)).Underlying().(*types.Slice)
	if sourceObject == nil || !ok || !ps6075Numeric(slice.Elem()) || pass.TypesSizes == nil {
		return nil, "", nil, 0, 0, false
	}
	elementBytes := pass.TypesSizes.Sizeof(slice.Elem())
	if elementBytes <= 0 {
		return nil, "", nil, 0, 0, false
	}
	type term struct {
		expression ast.Expr
		sign       int64
	}
	var terms []term
	var flatten func(ast.Expr, int64)
	flatten = func(expression ast.Expr, sign int64) {
		expression = ps2110Unparen(expression)
		if binary, ok := expression.(*ast.BinaryExpr); ok && (binary.Op == token.ADD || binary.Op == token.SUB) {
			flatten(binary.X, sign)
			rightSign := sign
			if binary.Op == token.SUB {
				rightSign = -sign
			}
			flatten(binary.Y, rightSign)
			return
		}
		terms = append(terms, term{expression: expression, sign: sign})
	}
	flatten(index.Index, 1)

	var stride ast.Expr
	offset := int64(0)
	outputSeen := false
	for _, candidate := range terms {
		if value, ok := ps6075ConstantInt(pass, candidate.expression); ok {
			offset += candidate.sign * value
			continue
		}
		if candidate.sign == 1 && ps6075ObjectExpression(pass, candidate.expression, output.variable) {
			if outputSeen {
				return nil, "", nil, 0, 0, false
			}
			outputSeen = true
			continue
		}
		if candidate.sign == 1 && stride == nil {
			if factor, ok := ps6075ReductionStride(pass, candidate.expression, reduction.variable); ok &&
				ps6075SameTypedExpression(pass, factor, output.bound) &&
				!ps6075MentionsObject(pass, factor, reduction.variable) &&
				!ps6075MentionsObject(pass, factor, output.variable) {
				stride = factor
				continue
			}
		}
		return nil, "", nil, 0, 0, false
	}
	if stride == nil || !outputSeen {
		return nil, "", nil, 0, 0, false
	}
	return sourceObject, source.Name, stride, offset, elementBytes, true
}

func ps6075ReductionStride(pass *analysis.Pass, expression ast.Expr, reduction types.Object) (ast.Expr, bool) {
	binary, ok := ps2110Unparen(expression).(*ast.BinaryExpr)
	if !ok || binary.Op != token.MUL {
		return nil, false
	}
	if ps6075ObjectExpression(pass, binary.X, reduction) {
		return binary.Y, true
	}
	if ps6075ObjectExpression(pass, binary.Y, reduction) {
		return binary.X, true
	}
	return nil, false
}

func ps6075HoistedAndStored(pass *analysis.Pass, output ps6075Loop, reductionIndex int, reduction *ast.ForStmt, accumulator types.Object) bool {
	if accumulator == nil || accumulator.Pos() <= output.statement.Body.Lbrace || accumulator.Pos() >= reduction.Pos() {
		return false
	}
	for _, statement := range output.statement.Body.List[reductionIndex+1:] {
		stored := false
		ast.Inspect(statement, func(node ast.Node) bool {
			assignment, ok := node.(*ast.AssignStmt)
			if !ok || assignment.Tok != token.ASSIGN {
				return !stored
			}
			hasDestination := false
			for _, left := range assignment.Lhs {
				if _, ok := ps2110Unparen(left).(*ast.IndexExpr); ok {
					hasDestination = true
					break
				}
			}
			if !hasDestination {
				return true
			}
			for _, right := range assignment.Rhs {
				ast.Inspect(right, func(candidate ast.Node) bool {
					identifier, ok := candidate.(*ast.Ident)
					if ok && pass.TypesInfo.ObjectOf(identifier) == accumulator {
						stored = true
						return false
					}
					return !stored
				})
			}
			return !stored
		})
		if stored {
			return true
		}
	}
	return false
}

func ps6075AccumulatorBytes(pass *analysis.Pass, object types.Object) (int64, bool) {
	if object == nil || pass.TypesSizes == nil || !ps6075Numeric(object.Type()) {
		return 0, false
	}
	size := pass.TypesSizes.Sizeof(object.Type())
	return size, size > 0
}

func ps6075Numeric(value types.Type) bool {
	basic, ok := types.Unalias(value).Underlying().(*types.Basic)
	return ok && basic.Info()&types.IsNumeric != 0
}

func ps6075ContiguousOffsets(offsets map[int64]bool) (int64, int64, bool) {
	if len(offsets) == 0 {
		return 0, 0, false
	}
	first := true
	minimum, maximum := int64(0), int64(0)
	for offset := range offsets {
		if first || offset < minimum {
			minimum = offset
		}
		if first || offset > maximum {
			maximum = offset
		}
		first = false
	}
	return minimum, maximum, maximum-minimum+1 == int64(len(offsets))
}

func ps6075NonUnitStride(pass *analysis.Pass, bound ast.Expr, elementBytes, usefulBytes int64) bool {
	if value, ok := ps6075ConstantInt(pass, bound); ok {
		return value > 1 && value*elementBytes > usefulBytes
	}
	return true
}

func ps6075DestinationTraffic(pass *analysis.Pass, finding *ps6075Finding) string {
	if count, ok := ps6075ConstantInt(pass, finding.reduction.bound); ok && count > 0 {
		return fmt.Sprintf("%d B", (count-1)*finding.accumulatorB)
	}
	return fmt.Sprintf("(%s-1)*%d B", exprTextRendered(finding.reduction.bound), finding.accumulatorB)
}

func ps6075SourceTraffic(pass *analysis.Pass, finding *ps6075Finding, bytesPerIteration int64) string {
	if count, ok := ps6075ConstantInt(pass, finding.reduction.bound); ok && count > 0 {
		return fmt.Sprintf("%d B", count*bytesPerIteration)
	}
	return fmt.Sprintf("%s*%d B", exprTextRendered(finding.reduction.bound), bytesPerIteration)
}

func ps6075IntegerValue(pass *analysis.Pass, expression ast.Expr, want int64) bool {
	value, ok := ps6075ConstantInt(pass, expression)
	return ok && value == want
}

func ps6075ConstantInt(pass *analysis.Pass, expression ast.Expr) (int64, bool) {
	typed, ok := pass.TypesInfo.Types[ps2110Unparen(expression)]
	if !ok || typed.Value == nil {
		return 0, false
	}
	return constant.Int64Val(constant.ToInt(typed.Value))
}

func ps6075ObjectExpression(pass *analysis.Pass, expression ast.Expr, object types.Object) bool {
	identifier, ok := ps2110Unparen(expression).(*ast.Ident)
	return ok && object != nil && pass.TypesInfo.ObjectOf(identifier) == object
}

func ps6075MentionsObject(pass *analysis.Pass, expression ast.Expr, object types.Object) bool {
	found := false
	ast.Inspect(expression, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if ok && pass.TypesInfo.ObjectOf(identifier) == object {
			found = true
			return false
		}
		return !found
	})
	return found
}

func ps6075SameTypedExpression(pass *analysis.Pass, left, right ast.Expr) bool {
	left = ps2110Unparen(left)
	right = ps2110Unparen(right)
	switch leftValue := left.(type) {
	case *ast.Ident:
		rightValue, ok := right.(*ast.Ident)
		return ok && pass.TypesInfo.ObjectOf(leftValue) != nil &&
			pass.TypesInfo.ObjectOf(leftValue) == pass.TypesInfo.ObjectOf(rightValue)
	case *ast.BasicLit:
		rightValue, ok := right.(*ast.BasicLit)
		return ok && leftValue.Kind == rightValue.Kind && leftValue.Value == rightValue.Value
	case *ast.SelectorExpr:
		rightValue, ok := right.(*ast.SelectorExpr)
		return ok && pass.TypesInfo.ObjectOf(leftValue.Sel) != nil &&
			pass.TypesInfo.ObjectOf(leftValue.Sel) == pass.TypesInfo.ObjectOf(rightValue.Sel) &&
			ps6075SameTypedExpression(pass, leftValue.X, rightValue.X)
	case *ast.BinaryExpr:
		rightValue, ok := right.(*ast.BinaryExpr)
		return ok && leftValue.Op == rightValue.Op &&
			ps6075SameTypedExpression(pass, leftValue.X, rightValue.X) &&
			ps6075SameTypedExpression(pass, leftValue.Y, rightValue.Y)
	default:
		return false
	}
}

func ps6075ContainsFunctionLiteral(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(node ast.Node) bool {
		if _, ok := node.(*ast.FuncLit); ok {
			found = true
			return false
		}
		return !found
	})
	return found
}

func ps6075Validated(function *ast.FuncDecl) bool {
	if function.Doc == nil {
		return false
	}
	for _, comment := range function.Doc.List {
		if strings.Contains(strings.ToLower(comment.Text), "perfscan:cache-locality-validated") {
			return true
		}
	}
	return false
}
