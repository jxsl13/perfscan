package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS6060 implements owner issue #777. It recognizes the exact scalar ReLU
// contract that needs ordered compare+select rather than floating maximum.
var PS6060 = register(&lint.Check{
	ID:       "PS6060",
	Category: "arith",
	Slug:     "exact-relu-loop-is-not-vectorized",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "an exact float ReLU loop remains scalar despite a SIMD-safe ordered form",
		Text: `A positive-only float store can express an exact ReLU contract:
positive finite values and +Inf retain their bits, while negative values,
NaNs, +0, and -0 leave the zeroed destination as +0. Current Go arm64 code
generation may leave this hot leaf scalar.

This check implements owner issue #777. It reports only a canonical range loop
over flat []float32 or []float64 slices when:

  - source and destination use the same induction variable and element type;
  - they are distinct typed slice objects;
  - the loop body is exactly one if statement with no else;
  - the condition is src[i] > 0 or 0 < src[i], optionally through an if-init
    temporary; and
  - the only store is dst[i] = src[i] (or that exact temporary).

Integer slices, >= comparisons, in-place loops, mismatched indices/types,
extra statements, transformed stores, else branches, and nonzero thresholds
stay silent. A //perfscan:measured-fallback annotation on the function or loop
suppresses a deliberately retained scalar site. A package that already exposes
a ReLU sibling named with SIMD/NEON/SVE/AVX/assembly/native vocabulary for the
same dtype also stays silent.

The safe native implementation is an ordered floating compare against +0
followed by an integer/bit select. Do NOT substitute an unconditional FMAX or
math.Max: NaN propagation and signed-zero behavior can differ from Go's
ordered > comparison and zero-destination contract. Validate unaligned slices,
lengths around every vector tail, random raw bit patterns, quiet/signaling NaNs,
infinities, subnormals, and both zero signs; inspect emitted instructions and
benchmark the complete production operation boundary.

There is NO automatic fix because Go has no portable ordered SIMD-select
primitive, architecture dispatch/registration is project-specific, and a
floating maximum is not bit-identical.`,
		Before: `for i := range dst {
	if src[i] > 0 {
		dst[i] = src[i]
	}
}`,
		After: `// Architecture-specific candidate, not portable Go pseudocode:
mask := orderedCompareGreaterThan(srcVector, positiveZero)
dstVector := bitSelect(mask, srcVector, positiveZero)
// Keep the scalar loop as the tail/fallback. Do not replace with FMAX.`,
		MeasuredWin: `Go 1.26/arm64 on Apple M2 Pro did not vectorize the loop
behind issue #777. A 16-lane NEON FCMGT-plus-bit-select leaf improved the
complete GoAI CPU operation boundary by 2.89x to 6.20x across 2,048 to
4,194,304 float32 elements with unchanged allocations. Evidence used 20
untimed warmups and three independent -benchtime=100x -count=7 campaigns per
revision, plus bit-for-bit gates over unaligned lengths 0..129 and raw float32
patterns and go tool objdump inspection.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6060",
		Doc:  "exact scalar ReLU loop has an ordered compare-and-select SIMD form",
		Run:  runPS6060,
	},
})

type ps6060Match struct {
	loop     *ast.RangeStmt
	source   types.Object
	dest     types.Object
	dtype    types.BasicKind
	srcName  string
	destName string
}

func runPS6060(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				if _, nested := node.(*ast.FuncLit); nested {
					return false
				}
				loop, ok := node.(*ast.RangeStmt)
				if !ok {
					return true
				}
				match, ok := ps6060ExactReLU(pass, loop)
				if !ok || ps6060MeasuredFallback(file, fn, loop) || ps6060NativeSibling(pass, fn, match.dtype) {
					return true
				}
				dtype := "float64"
				if match.dtype == types.Float32 {
					dtype = "float32"
				}
				pass.Reportf(loop.For, "exact %s ReLU loop %s[i] > 0 -> %s[i] remains scalar; the bit-identical native form is ordered compare against +0 followed by bit select, not FMAX/math.Max (NaNs and -0 differ) — add or register a measured SIMD sibling, preserve the scalar tail/fallback, gate raw-bit edge cases and allocations, inspect instructions, and benchmark the complete operation boundary (advisory, no automatic fix)", dtype, match.srcName, match.destName)
				return true
			})
		}
	}
	return nil, nil
}

func ps6060ExactReLU(pass *analysis.Pass, loop *ast.RangeStmt) (ps6060Match, bool) {
	index, ok := loop.Key.(*ast.Ident)
	if !ok || index.Name == "_" || loop.Value != nil || len(loop.Body.List) != 1 {
		return ps6060Match{}, false
	}
	indexObject := identObject(pass, index)
	if indexObject == nil {
		return ps6060Match{}, false
	}
	ifStmt, ok := loop.Body.List[0].(*ast.IfStmt)
	if !ok || ifStmt.Else != nil || len(ifStmt.Body.List) != 1 {
		return ps6060Match{}, false
	}
	store, ok := ifStmt.Body.List[0].(*ast.AssignStmt)
	if !ok || store.Tok != token.ASSIGN || len(store.Lhs) != 1 || len(store.Rhs) != 1 {
		return ps6060Match{}, false
	}
	dest, destKind, destName, ok := ps6060IndexedSlice(pass, store.Lhs[0], indexObject)
	if !ok {
		return ps6060Match{}, false
	}

	positive := ps6060PositiveOperand(pass, ifStmt.Cond)
	if positive == nil {
		return ps6060Match{}, false
	}
	var source types.Object
	var sourceKind types.BasicKind
	var sourceName string
	if ifStmt.Init == nil {
		source, sourceKind, sourceName, ok = ps6060IndexedSlice(pass, positive, indexObject)
		if !ok || !ps6060SameIndexedValue(pass, store.Rhs[0], source, indexObject) {
			return ps6060Match{}, false
		}
	} else {
		init, initOK := ifStmt.Init.(*ast.AssignStmt)
		if !initOK || init.Tok != token.DEFINE || len(init.Lhs) != 1 || len(init.Rhs) != 1 {
			return ps6060Match{}, false
		}
		temp, tempOK := init.Lhs[0].(*ast.Ident)
		if !tempOK || temp.Name == "_" {
			return ps6060Match{}, false
		}
		tempObject := identObject(pass, temp)
		positiveID, positiveOK := ps2110Unparen(positive).(*ast.Ident)
		storedID, storedOK := ps2110Unparen(store.Rhs[0]).(*ast.Ident)
		if tempObject == nil || !positiveOK || !storedOK || identObject(pass, positiveID) != tempObject || identObject(pass, storedID) != tempObject {
			return ps6060Match{}, false
		}
		source, sourceKind, sourceName, ok = ps6060IndexedSlice(pass, init.Rhs[0], indexObject)
		if !ok {
			return ps6060Match{}, false
		}
	}
	if source == dest || sourceKind != destKind || sourceKind != types.Float32 && sourceKind != types.Float64 {
		return ps6060Match{}, false
	}
	rangeID, ok := ps2110Unparen(loop.X).(*ast.Ident)
	if !ok {
		return ps6060Match{}, false
	}
	rangeObject := identObject(pass, rangeID)
	if rangeObject != source && rangeObject != dest {
		return ps6060Match{}, false
	}
	return ps6060Match{loop: loop, source: source, dest: dest, dtype: sourceKind, srcName: sourceName, destName: destName}, true
}

func ps6060PositiveOperand(pass *analysis.Pass, expression ast.Expr) ast.Expr {
	binary, ok := ps2110Unparen(expression).(*ast.BinaryExpr)
	if !ok {
		return nil
	}
	switch {
	case binary.Op == token.GTR && ps6060Zero(pass, binary.Y):
		return binary.X
	case binary.Op == token.LSS && ps6060Zero(pass, binary.X):
		return binary.Y
	default:
		return nil
	}
}

func ps6060Zero(pass *analysis.Pass, expression ast.Expr) bool {
	value := pass.TypesInfo.Types[ps2110Unparen(expression)].Value
	return value != nil && constant.Sign(value) == 0
}

func ps6060IndexedSlice(pass *analysis.Pass, expression ast.Expr, index types.Object) (types.Object, types.BasicKind, string, bool) {
	indexed, ok := ps2110Unparen(expression).(*ast.IndexExpr)
	if !ok {
		return nil, types.Invalid, "", false
	}
	indexID, ok := ps2110Unparen(indexed.Index).(*ast.Ident)
	if !ok || identObject(pass, indexID) != index {
		return nil, types.Invalid, "", false
	}
	base, ok := ps2110Unparen(indexed.X).(*ast.Ident)
	if !ok {
		return nil, types.Invalid, "", false
	}
	object := identObject(pass, base)
	if object == nil {
		return nil, types.Invalid, "", false
	}
	slice, ok := object.Type().Underlying().(*types.Slice)
	if !ok {
		return nil, types.Invalid, "", false
	}
	basic, ok := slice.Elem().Underlying().(*types.Basic)
	if !ok || basic.Kind() != types.Float32 && basic.Kind() != types.Float64 {
		return nil, types.Invalid, "", false
	}
	return object, basic.Kind(), base.Name, true
}

func ps6060SameIndexedValue(pass *analysis.Pass, expression ast.Expr, source, index types.Object) bool {
	object, _, _, ok := ps6060IndexedSlice(pass, expression, index)
	return ok && object == source
}

func ps6060MeasuredFallback(file *ast.File, fn *ast.FuncDecl, loop *ast.RangeStmt) bool {
	for _, group := range file.Comments {
		adjacentToFunction := group.End() <= fn.Pos() && fn.Pos()-group.End() <= 3
		adjacentToLoop := group.End() <= loop.Pos() && loop.Pos()-group.End() <= 3
		insideFunction := group == fn.Doc || adjacentToFunction || adjacentToLoop || group.Pos() >= fn.Body.Pos() && group.End() <= fn.Body.End()
		if !insideFunction {
			continue
		}
		for _, comment := range group.List {
			text := ps6058Compact(comment.Text)
			if strings.Contains(text, "perfscanmeasuredfallback") || strings.Contains(text, "perfscanmeasuredrelufallback") {
				return true
			}
		}
	}
	return false
}

func ps6060NativeSibling(pass *analysis.Pass, current *ast.FuncDecl, dtype types.BasicKind) bool {
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn == current || !ps6060NativeReLUName(fn.Name.Name) {
				continue
			}
			if ps6060NameDType(fn.Name.Name, dtype) || ps6060SignatureDType(pass, fn, dtype) {
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
			found = strings.Contains(text, "relu") && ps6060NativeMarker(text) && ps6060NameDType(text, dtype)
			return !found
		})
		if found {
			return true
		}
	}
	return false
}

func ps6060NativeReLUName(name string) bool {
	name = ps6007NormalizeName(name)
	return strings.Contains(name, "relu") && ps6060NativeMarker(name)
}

func ps6060NativeMarker(name string) bool {
	return ps6007ContainsAny(name, "simd", "neon", "sve", "avx", "assembly", "asm", "native", "vectorized")
}

func ps6060NameDType(name string, dtype types.BasicKind) bool {
	name = ps6007NormalizeName(name)
	if dtype == types.Float32 {
		return ps6007ContainsAny(name, "float32", "f32")
	}
	return ps6007ContainsAny(name, "float64", "f64", "double")
}

func ps6060SignatureDType(pass *analysis.Pass, fn *ast.FuncDecl, dtype types.BasicKind) bool {
	object, ok := pass.TypesInfo.Defs[fn.Name].(*types.Func)
	if !ok {
		return false
	}
	signature, ok := object.Type().(*types.Signature)
	if !ok {
		return false
	}
	for index := 0; index < signature.Params().Len(); index++ {
		slice, ok := signature.Params().At(index).Type().Underlying().(*types.Slice)
		if !ok {
			continue
		}
		basic, ok := slice.Elem().Underlying().(*types.Basic)
		if ok && basic.Kind() == dtype {
			return true
		}
	}
	return false
}
