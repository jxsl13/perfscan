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

// PS6066 implements owner issue #783. It finds byte-wise packed decode loads
// whose offset arithmetic proves a fixed alignment relative to the source.
var PS6066 = register(&lint.Check{
	ID:       "PS6066",
	Category: "verify",
	Slug:     "nested-byte-decode-packed-load-candidate",
	Level:    lint.LevelAggressive,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "a nested byte-wise decode loop has an aligned packed-load candidate",
		Text: `Scalar byte accesses can leave a packed decode load fragmented even
when every hot-loop iteration reads one adjacent 4-, 8-, or 16-byte region.
A target-appropriate packed load followed by register extraction may reduce
load and address-generation work, but the result depends on native alignment,
endianness, compiler lowering, target ISA, and the complete workload shape.

This check implements owner issue #783. It reports only a straight-line
assignment or declaration inside at least two nested loops in a function whose
name identifies decode, unpack, or dequant work. Every index in the expression
must read the same byte slice or byte array, the distinct affine indices must
cover exactly one adjacent 4-, 8-, or 16-byte interval, and every coefficient
and the interval base must be divisible by that width. Immutable local offsets
are expanded before the congruence proof. Calls other than type conversions,
mutable offset aliases, mixed buffers, gaps, non-byte elements, and results that
feed an if/for/switch/select decision stay silent.

The congruence proof is relative to element zero. Go does not in general expose
enough information to prove the native address alignment of an arbitrary byte
slice, and an aligned source allocation can still be passed as an unaligned
subslice. Before changing the code, establish the actual base and stride
alignment, preserve bounds and byte order, inspect the generated native code,
and compare the same binary. Plant odd tails and boundary offsets in the parity
suite. Gate the complete production shape and retain a measured threshold when
small shapes do not pay back fixed costs. Benchmark coalesced per-lane loads
against any shared-loader/shuffle design, and price workgroup geometry changes
separately.

A //perfscan:packed-decode-load-validated annotation suppresses a function
whose alignment, parity, native-code, and end-to-end benchmark contract is
recorded externally. There is NO automatic fix: selecting a packed-load API or
unsafe/native implementation without those facts can introduce alignment,
bounds, endian, aliasing, or performance regressions.`,
		Before: `for block := 0; block < blocks; block++ {
	for lane := 0; lane < lanes; lane++ {
		base := block*16 + lane*4
		word := uint32(src[base]) |
			uint32(src[base+1])<<8 |
			uint32(src[base+2])<<16 |
			uint32(src[base+3])<<24
		decode(word)
	}
}`,
		After: `// Only after proving native alignment, endian/bounds parity, and a shape gate:
word := loadAlignedPacked32(src, base)
decode(word)`,
		MeasuredWin: `In the owner Apple M2 Metal Q5_K M=1 campaigns, aligned
wide loads improved three independent count-7 medians by 1.102x-1.152x at
2048x3072, 1.216x-1.223x at 4096x2048, 1.243x-1.255x at 2048x5632, and
1.394x-1.404x at 5632x2048. The 2048x2048 shape improved only about 1.05x,
so production retained a measured K*N threshold. Sharing one loader through
extra SIMD shuffles lost to duplicated coalesced wide loads, and a wider
threadgroup was also rejected.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6066",
		Doc:  "nested byte-wise decode loop has a benchmark-gated packed-load candidate",
		Run:  runPS6066,
	},
})

type ps6066Affine struct {
	coeff    map[types.Object]int64
	constant int64
}

func runPS6066(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil || !ps6066DecodeContext(function.Name.Name) || ps6066ValidatedAnnotation(file, function) {
				continue
			}
			flow := ps6064FunctionFlow(pass, function)
			astutil.WithStack(function.Body, func(node ast.Node, stack []ast.Node) bool {
				if _, nested := node.(*ast.FuncLit); nested {
					return false
				}
				if ps6066LoopDepth(stack) < 2 {
					return true
				}
				switch statement := node.(type) {
				case *ast.AssignStmt:
					if len(statement.Lhs) != len(statement.Rhs) {
						return true
					}
					for index, expression := range statement.Rhs {
						ps6066ReportCandidate(pass, function, flow, expression, ps6066AssignedObject(pass, statement.Lhs[index]))
					}
				case *ast.ValueSpec:
					if len(statement.Names) != len(statement.Values) {
						return true
					}
					for index, expression := range statement.Values {
						ps6066ReportCandidate(pass, function, flow, expression, pass.TypesInfo.Defs[statement.Names[index]])
					}
				}
				return true
			})
		}
	}
	return nil, nil
}

func ps6066ReportCandidate(pass *analysis.Pass, function *ast.FuncDecl, flow ps6064Flow, expression ast.Expr, result types.Object) {
	width, source, ok := ps6066PackedCluster(pass, flow, expression)
	if !ok || result != nil && ps6066ControlsFlow(pass, function.Body, result) {
		return
	}
	pass.Reportf(expression.Pos(), "%d adjacent byte loads from %s in nested decode loops start at an offset provably divisible by %d relative to element zero; consider one target-appropriate packed load plus register extraction only after proving actual base/stride alignment, bounds, aliasing, and endianness, then validate same-binary parity/native lowering and gate the complete production shape (small shapes, loader-sharing shuffles, and workgroup changes may lose; advisory, no automatic fix)", width, source.Name(), width)
}

func ps6066PackedCluster(pass *analysis.Pass, flow ps6064Flow, expression ast.Expr) (int, types.Object, bool) {
	var source types.Object
	var indices []ps6066Affine
	valid := true
	ast.Inspect(expression, func(node ast.Node) bool {
		if !valid || node == nil {
			return valid
		}
		switch value := node.(type) {
		case *ast.FuncLit:
			valid = false
			return false
		case *ast.CallExpr:
			if typeAndValue, ok := pass.TypesInfo.Types[value.Fun]; !ok || !typeAndValue.IsType() {
				valid = false
				return false
			}
		case *ast.UnaryExpr:
			if value.Op == token.ARROW {
				valid = false
				return false
			}
		case *ast.IndexExpr:
			identifier, ok := ps2110Unparen(value.X).(*ast.Ident)
			if !ok || !ps6066ByteSequence(pass.TypesInfo.TypeOf(identifier)) {
				valid = false
				return false
			}
			object := pass.TypesInfo.ObjectOf(identifier)
			if object == nil || source != nil && source != object {
				valid = false
				return false
			}
			index, ok := ps6066AffineExpression(pass, flow, value.Index, map[types.Object]bool{})
			if !ok {
				valid = false
				return false
			}
			source = object
			indices = append(indices, index)
		}
		return true
	})
	if !valid || source == nil || !ps6066SupportedWidth(len(indices)) {
		return 0, nil, false
	}
	slices.SortFunc(indices, func(left, right ps6066Affine) int {
		switch {
		case left.constant < right.constant:
			return -1
		case left.constant > right.constant:
			return 1
		default:
			return 0
		}
	})
	base := indices[0]
	width := int64(len(indices))
	for index, current := range indices {
		expected, exact := ps6066SafeAdd(base.constant, int64(index))
		if !exact || !ps6066SameCoefficients(base, current) || current.constant != expected {
			return 0, nil, false
		}
	}
	if base.constant%width != 0 {
		return 0, nil, false
	}
	for _, coefficient := range base.coeff {
		if coefficient%width != 0 {
			return 0, nil, false
		}
	}
	return int(width), source, true
}

func ps6066SupportedWidth(width int) bool {
	return width == 4 || width == 8 || width == 16
}

func ps6066ByteSequence(value types.Type) bool {
	if value == nil {
		return false
	}
	var element types.Type
	switch sequence := types.Unalias(value).Underlying().(type) {
	case *types.Array:
		element = sequence.Elem()
	case *types.Slice:
		element = sequence.Elem()
	default:
		return false
	}
	basic, ok := types.Unalias(element).Underlying().(*types.Basic)
	return ok && basic.Kind() == types.Uint8
}

func ps6066AffineExpression(pass *analysis.Pass, flow ps6064Flow, expression ast.Expr, seen map[types.Object]bool) (ps6066Affine, bool) {
	expression = ps2110Unparen(expression)
	if value := pass.TypesInfo.Types[expression].Value; value != nil {
		integer, ok := constant.Int64Val(value)
		if !ok {
			return ps6066Affine{}, false
		}
		return ps6066Affine{coeff: map[types.Object]int64{}, constant: integer}, true
	}
	switch value := expression.(type) {
	case *ast.Ident:
		object := pass.TypesInfo.ObjectOf(value)
		if object == nil || !ps6066IntegerType(object.Type()) {
			return ps6066Affine{}, false
		}
		if initializer := flow.initializers[object]; initializer != nil && flow.writes[object] == 0 && !seen[object] {
			seen[object] = true
			result, ok := ps6066AffineExpression(pass, flow, initializer, seen)
			delete(seen, object)
			return result, ok
		}
		return ps6066Affine{coeff: map[types.Object]int64{object: 1}}, true
	case *ast.UnaryExpr:
		if value.Op != token.ADD && value.Op != token.SUB {
			return ps6066Affine{}, false
		}
		result, ok := ps6066AffineExpression(pass, flow, value.X, seen)
		if !ok || value.Op == token.ADD {
			return result, ok
		}
		return ps6066ScaleAffine(result, -1)
	case *ast.BinaryExpr:
		switch value.Op {
		case token.ADD, token.SUB:
			left, leftOK := ps6066AffineExpression(pass, flow, value.X, seen)
			right, rightOK := ps6066AffineExpression(pass, flow, value.Y, seen)
			if !leftOK || !rightOK {
				return ps6066Affine{}, false
			}
			if value.Op == token.SUB {
				var ok bool
				right, ok = ps6066ScaleAffine(right, -1)
				if !ok {
					return ps6066Affine{}, false
				}
			}
			return ps6066AddAffine(left, right)
		case token.MUL:
			if factor, ok := ps6066IntegerConstant(pass, value.X); ok {
				term, termOK := ps6066AffineExpression(pass, flow, value.Y, seen)
				if !termOK {
					return ps6066Affine{}, false
				}
				return ps6066ScaleAffine(term, factor)
			}
			if factor, ok := ps6066IntegerConstant(pass, value.Y); ok {
				term, termOK := ps6066AffineExpression(pass, flow, value.X, seen)
				if !termOK {
					return ps6066Affine{}, false
				}
				return ps6066ScaleAffine(term, factor)
			}
		}
	}
	return ps6066Affine{}, false
}

func ps6066IntegerConstant(pass *analysis.Pass, expression ast.Expr) (int64, bool) {
	value := pass.TypesInfo.Types[ps2110Unparen(expression)].Value
	if value == nil {
		return 0, false
	}
	return constant.Int64Val(value)
}

func ps6066IntegerType(value types.Type) bool {
	if value == nil {
		return false
	}
	basic, ok := types.Unalias(value).Underlying().(*types.Basic)
	return ok && basic.Info()&types.IsInteger != 0
}

func ps6066AddAffine(left, right ps6066Affine) (ps6066Affine, bool) {
	constantValue, ok := ps6066SafeAdd(left.constant, right.constant)
	if !ok {
		return ps6066Affine{}, false
	}
	result := ps6066Affine{coeff: make(map[types.Object]int64, len(left.coeff)+len(right.coeff)), constant: constantValue}
	for object, coefficient := range left.coeff {
		result.coeff[object] = coefficient
	}
	for object, coefficient := range right.coeff {
		combined, ok := ps6066SafeAdd(result.coeff[object], coefficient)
		if !ok {
			return ps6066Affine{}, false
		}
		if combined == 0 {
			delete(result.coeff, object)
		} else {
			result.coeff[object] = combined
		}
	}
	return result, true
}

func ps6066ScaleAffine(value ps6066Affine, factor int64) (ps6066Affine, bool) {
	constantValue, ok := ps6066SafeMul(value.constant, factor)
	if !ok {
		return ps6066Affine{}, false
	}
	result := ps6066Affine{coeff: make(map[types.Object]int64, len(value.coeff)), constant: constantValue}
	for object, coefficient := range value.coeff {
		scaled, ok := ps6066SafeMul(coefficient, factor)
		if !ok {
			return ps6066Affine{}, false
		}
		if scaled != 0 {
			result.coeff[object] = scaled
		}
	}
	return result, true
}

func ps6066SafeAdd(left, right int64) (int64, bool) {
	result := left + right
	if right > 0 && result < left || right < 0 && result > left {
		return 0, false
	}
	return result, true
}

func ps6066SafeMul(left, right int64) (int64, bool) {
	if left == 0 || right == 0 {
		return 0, true
	}
	const minInt64 = -1 << 63
	if left == minInt64 && right == -1 || right == minInt64 && left == -1 {
		return 0, false
	}
	result := left * right
	if result/right != left {
		return 0, false
	}
	return result, true
}

func ps6066SameCoefficients(left, right ps6066Affine) bool {
	if len(left.coeff) != len(right.coeff) {
		return false
	}
	for object, coefficient := range left.coeff {
		if right.coeff[object] != coefficient {
			return false
		}
	}
	return true
}

func ps6066LoopDepth(stack []ast.Node) int {
	depth := 0
	for index := 0; index+1 < len(stack); index++ {
		body := astutil.LoopBody(stack[index])
		if body != nil && stack[index+1] == ast.Node(body) {
			depth++
		}
	}
	return depth
}

func ps6066AssignedObject(pass *analysis.Pass, expression ast.Expr) types.Object {
	identifier, ok := ps2110Unparen(expression).(*ast.Ident)
	if !ok || identifier.Name == "_" {
		return nil
	}
	if object := pass.TypesInfo.Defs[identifier]; object != nil {
		return object
	}
	return pass.TypesInfo.Uses[identifier]
}

func ps6066ControlsFlow(pass *analysis.Pass, body *ast.BlockStmt, object types.Object) bool {
	controlled := false
	uses := func(expression ast.Expr) bool {
		if expression == nil {
			return false
		}
		found := false
		ast.Inspect(expression, func(node ast.Node) bool {
			identifier, ok := node.(*ast.Ident)
			if ok && pass.TypesInfo.Uses[identifier] == object {
				found = true
				return false
			}
			return !found
		})
		return found
	}
	ast.Inspect(body, func(node ast.Node) bool {
		if controlled || node == nil {
			return !controlled
		}
		switch value := node.(type) {
		case *ast.IfStmt:
			controlled = uses(value.Cond)
		case *ast.ForStmt:
			controlled = uses(value.Cond)
		case *ast.SwitchStmt:
			controlled = uses(value.Tag)
		case *ast.CaseClause:
			for _, expression := range value.List {
				if uses(expression) {
					controlled = true
					break
				}
			}
		case *ast.CommClause:
			if value.Comm != nil {
				ast.Inspect(value.Comm, func(inner ast.Node) bool {
					identifier, ok := inner.(*ast.Ident)
					if ok && pass.TypesInfo.Uses[identifier] == object {
						controlled = true
						return false
					}
					return !controlled
				})
			}
		}
		return !controlled
	})
	return controlled
}

func ps6066DecodeContext(name string) bool {
	name = ps6007NormalizeName(name)
	return ps6007ContainsAny(name, "decode", "unpack", "dequant")
}

func ps6066ValidatedAnnotation(file *ast.File, function *ast.FuncDecl) bool {
	for _, group := range file.Comments {
		if group != function.Doc && !(group.End() <= function.Pos() && function.Pos()-group.End() <= 3) && !(group.Pos() >= function.Body.Pos() && group.End() <= function.Body.End()) {
			continue
		}
		for _, comment := range group.List {
			text := ps6058Compact(comment.Text)
			if strings.Contains(text, "perfscanpackeddecodeloadvalidated") || strings.Contains(text, "perfscanpackedloadvalidated") {
				return true
			}
		}
	}
	return false
}
