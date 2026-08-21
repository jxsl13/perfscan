package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS6024 implements owner issue #762: a newly allocated large destination
// should not be borrowed as sparse scratch before a later full overwrite, and
// benchmarks of that rewrite need a fresh-allocation/demand-zero cell.
var PS6024 = register(&lint.Check{
	ID:       "PS6024",
	Category: "verify",
	Slug:     "output-scratch-pretouch",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "a fresh destination is partially pre-touched as scratch before a later full overwrite",
		Text: `Borrowing a newly allocated output as temporary scratch can turn a
two-pass leaf optimization into extra demand-zero and page traffic. A warm
benchmark that reuses the destination hides the first-touch cost, while a real
bulk load allocates the full result, sparsely faults/writes its pages in the
prepass, then revisits and overwrites them in the decode pass.

This check implements owner issue #762 in two source-auditable forms.

For production code it links one local slice object through:

  - make([]T, runtimeSize) whose size derives from a function parameter;
  - an earlier loop that writes the destination through a strided, offset,
    tail, or otherwise partial index; and
  - a later unconditional full-range loop that assigns dst[i] for every i and
    then returns the same destination.

The full overwrite must be a canonical "for i := range dst" or
"for i := 0; i < len(dst); i++" with an unconditional plain assignment. A
separate scratch allocation, a conditional or partial second pass, reversed
pass order, and constant-size local buffers stay silent.

For BenchmarkX(*testing.B), the check also reports a runtime-sized destination
allocated before the b.N loop and passed to a decode/dequantize/transform/
compression/serialization call on every iteration when the benchmark has no
documented fresh-allocation or demand-zero comparison cell.

There is NO automatic fix. Moving scratch to a block-local buffer, recomputing
cheap coefficients, or changing pass order can alter aliasing and arithmetic.
Validate with alternating warm-leaf and fresh end-to-end measurements, and
retain exact output/parity gates.`,
		Before: `out := make([]float32, blocks*blockSize)
for i := range blocks {
	out[len(out)-blocks+i] = coefficient(i) // pre-touch output tail
}
for i := range out {
	out[i] = decode(i, out[len(out)-blocks+i/blockSize])
}
return out`,
		After: `out := make([]float32, blocks*blockSize)
for block := range blocks {
	coeff := coefficient(block) // block-local scratch/recompute
	decodeBlock(out[block*blockSize:], coeff)
}
return out
// Benchmark both reused/warm and fresh demand-zero destinations.`,
		MeasuredWin: `In the Apple-M2 GGUF campaign behind issue #762, an output-
tail coefficient prepass improved warm Q6_K/Q4_K leaves to about 23.9/31.1 us
but regressed fresh real-model loads (for example 129.6 ms vs 99.6 ms and
232.0 ms vs 109.5 ms) while pre-touching roughly 4.4 GB of f32 output. Removing
the pre-touch restored a directional ten-run model-load median of 104.06 ms
versus 113.11 ms control.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6024",
		Doc:  "fresh large output is used as partial scratch before full overwrite, or benchmark hides first-touch cost",
		Run:  runPS6024,
	},
})

type ps6024Allocation struct {
	object types.Object
	call   *ast.CallExpr
}

type ps6024Writes struct {
	partialPos token.Pos
	partial    *ast.IndexExpr
	fullPos    token.Pos
}

func runPS6024(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			if ps6006Benchmark(pass, fn) {
				ps6024Benchmark(pass, file, fn)
				continue
			}
			params := ps2140ParamObjs(pass, fn)
			for _, allocation := range ps6024Allocations(pass, fn.Body, params) {
				writes := ps6024ClassifyWrites(pass, fn.Body, allocation.object)
				if writes.partial == nil || writes.fullPos <= writes.partialPos || !ps6024Returned(pass, fn.Body, allocation.object, writes.fullPos) {
					continue
				}
				pass.Reportf(writes.partial.Pos(), "freshly allocated destination %s is partially/strided pre-touched as scratch in one pass and then fully overwritten in a later pass; avoid borrowing large output pages as scratch and benchmark alternating fresh demand-zero versus reused/warm destinations", allocation.object.Name())
			}
		}
	}
	return nil, nil
}

func ps6024Allocations(pass *analysis.Pass, body *ast.BlockStmt, params map[types.Object]bool) []ps6024Allocation {
	var allocations []ps6024Allocation
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		assign, ok := node.(*ast.AssignStmt)
		if !ok || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
			return true
		}
		id, ok := ps2110Unparen(assign.Lhs[0]).(*ast.Ident)
		if !ok {
			return true
		}
		call, ok := ps2110Unparen(assign.Rhs[0]).(*ast.CallExpr)
		if !ok || !ps2140IsMake(pass, call) || len(call.Args) < 2 {
			return true
		}
		if _, ok := pass.TypesInfo.TypeOf(call).(*types.Slice); !ok || !ps2140SizeFromParam(pass, call.Args[1], params) {
			return true
		}
		object := identObject(pass, id)
		if object != nil {
			allocations = append(allocations, ps6024Allocation{object: object, call: call})
		}
		return true
	})
	return allocations
}

func ps6024ClassifyWrites(pass *analysis.Pass, body *ast.BlockStmt, object types.Object) ps6024Writes {
	var result ps6024Writes
	astutil.WithStack(body, func(node ast.Node, stack []ast.Node) bool {
		assign, ok := node.(*ast.AssignStmt)
		if !ok || assign.Tok != token.ASSIGN {
			return true
		}
		loop, inLoop := astutil.InLoop(stack)
		if !inLoop {
			return true
		}
		for _, lhs := range assign.Lhs {
			index, ok := ps2110Unparen(lhs).(*ast.IndexExpr)
			if !ok || ps6024BaseObject(pass, index.X) != object {
				continue
			}
			if ps2140IndexMatchesLoop(pass, ps2110Unparen(index.Index), loop, object) && ps6024Unconditional(stack, loop) {
				if result.fullPos == token.NoPos || assign.Pos() < result.fullPos {
					result.fullPos = assign.Pos()
				}
				continue
			}
			if result.partial == nil && ps6024ScratchIndex(pass, index.Index, loop, object) {
				result.partial, result.partialPos = index, assign.Pos()
			}
		}
		return true
	})
	return result
}

func ps6024BaseObject(pass *analysis.Pass, expr ast.Expr) types.Object {
	id, ok := ps2110Unparen(expr).(*ast.Ident)
	if !ok {
		return nil
	}
	return identObject(pass, id)
}

func ps6024Unconditional(stack []ast.Node, loop ast.Node) bool {
	seenLoop := false
	for _, ancestor := range stack {
		if ancestor == loop {
			seenLoop = true
			continue
		}
		if !seenLoop {
			continue
		}
		switch ancestor.(type) {
		case *ast.IfStmt, *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt:
			return false
		}
	}
	return seenLoop
}

func ps6024ScratchIndex(pass *analysis.Pass, index ast.Expr, loop ast.Node, object types.Object) bool {
	loopVars := ps6024LoopVars(pass, loop)
	if len(loopVars) == 0 || !ps6024MentionsObject(pass, index, loopVars) {
		return false
	}
	interesting := false
	ast.Inspect(index, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.BinaryExpr:
			if value.Op == token.ADD || value.Op == token.SUB || value.Op == token.MUL || value.Op == token.QUO || value.Op == token.REM || value.Op == token.SHL || value.Op == token.SHR {
				interesting = true
			}
		case *ast.CallExpr:
			if id, ok := ps2110Unparen(value.Fun).(*ast.Ident); ok && (id.Name == "len" || id.Name == "cap") && ps6024MentionsObject(pass, value, map[types.Object]bool{object: true}) {
				interesting = true
			}
		}
		return !interesting
	})
	if interesting {
		return true
	}
	// A plain loop variable over some block/chunk/tile collection is partial
	// with respect to dst even though its index expression is just i.
	return !ps2140IndexMatchesLoop(pass, ps2110Unparen(index), loop, object) && ps6024PartialLoopLabel(loop)
}

func ps6024LoopVars(pass *analysis.Pass, loop ast.Node) map[types.Object]bool {
	objects := make(map[types.Object]bool)
	switch value := loop.(type) {
	case *ast.RangeStmt:
		for _, expr := range []ast.Expr{value.Key, value.Value} {
			if id, ok := ps2110Unparen(expr).(*ast.Ident); ok {
				objects[identObject(pass, id)] = true
			}
		}
	case *ast.ForStmt:
		if init, ok := value.Init.(*ast.AssignStmt); ok {
			for _, lhs := range init.Lhs {
				if id, ok := ps2110Unparen(lhs).(*ast.Ident); ok {
					objects[identObject(pass, id)] = true
				}
			}
		}
	}
	delete(objects, nil)
	return objects
}

func ps6024MentionsObject(pass *analysis.Pass, node ast.Node, objects map[types.Object]bool) bool {
	found := false
	ast.Inspect(node, func(child ast.Node) bool {
		if found {
			return false
		}
		id, ok := child.(*ast.Ident)
		if ok && objects[pass.TypesInfo.ObjectOf(id)] {
			found = true
			return false
		}
		return true
	})
	return found
}

func ps6024PartialLoopLabel(loop ast.Node) bool {
	text := ps6007NormalizeName(ps6024RenderedLoopText(loop))
	return ps6007ContainsAny(text, "block", "chunk", "page", "tile", "tail", "stride", "scratch", "coefficient", "coeff")
}

func ps6024RenderedLoopText(loop ast.Node) string {
	switch value := loop.(type) {
	case *ast.RangeStmt:
		return exprTextRendered(value.X)
	case *ast.ForStmt:
		if value.Cond != nil {
			return exprTextRendered(value.Cond)
		}
	}
	return ""
}

func ps6024Returned(pass *analysis.Pass, body *ast.BlockStmt, object types.Object, after token.Pos) bool {
	returned := false
	ast.Inspect(body, func(node ast.Node) bool {
		if node == nil {
			return true
		}
		if returned || node.Pos() <= after {
			return !returned
		}
		statement, ok := node.(*ast.ReturnStmt)
		if !ok {
			return true
		}
		for _, result := range statement.Results {
			id, ok := ps2110Unparen(result).(*ast.Ident)
			if ok && identObject(pass, id) == object {
				returned = true
				return false
			}
		}
		return true
	})
	return returned
}

func ps6024Benchmark(pass *analysis.Pass, file *ast.File, fn *ast.FuncDecl) {
	text := ps6015FunctionText(pass, file, fn)
	normalized := ps6007NormalizeName(text)
	context := ps6007ContainsAny(normalized, "decode", "dequant", "transform", "compress", "serial", "image", "audio")
	if !context || ps6007ContainsAny(normalized, "freshallocationarm", "freshdestinationarm", "demandzeroarm", "allocateperiteration", "coldallocationarm") {
		return
	}
	loop := ps6024BenchmarkLoop(pass, fn)
	if loop == nil {
		return
	}
	for _, allocation := range ps6024BenchmarkAllocations(pass, fn.Body) {
		if allocation.call.Pos() >= loop.Pos() || !ps6024DestinationName(allocation.object.Name()) || !ps6024PassedInside(pass, loop, allocation.object) {
			continue
		}
		pass.Reportf(allocation.call.Pos(), "benchmark reuses runtime-sized destination %s across b.N for a bulk decode/transform path but has no fresh-allocation/demand-zero comparison cell; add an alternating fresh destination arm before accepting a multi-pass output-scratch rewrite", allocation.object.Name())
	}
}

func ps6024BenchmarkAllocations(pass *analysis.Pass, body *ast.BlockStmt) []ps6024Allocation {
	var allocations []ps6024Allocation
	ast.Inspect(body, func(node ast.Node) bool {
		assign, ok := node.(*ast.AssignStmt)
		if !ok || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
			return true
		}
		id, ok := ps2110Unparen(assign.Lhs[0]).(*ast.Ident)
		if !ok {
			return true
		}
		call, ok := ps2110Unparen(assign.Rhs[0]).(*ast.CallExpr)
		if !ok || !ps2140IsMake(pass, call) || len(call.Args) < 2 {
			return true
		}
		if _, ok := pass.TypesInfo.TypeOf(call).(*types.Slice); !ok || !ps6024BenchmarkSize(pass, call.Args[1]) {
			return true
		}
		object := identObject(pass, id)
		if object != nil {
			allocations = append(allocations, ps6024Allocation{object: object, call: call})
		}
		return true
	})
	return allocations
}

func ps6024BenchmarkSize(pass *analysis.Pass, expr ast.Expr) bool {
	tv, ok := pass.TypesInfo.Types[ps2110Unparen(expr)]
	if !ok || tv.Value == nil {
		return true
	}
	if tv.Value.Kind() != constant.Int {
		return false
	}
	value, ok := constant.Int64Val(tv.Value)
	return ok && value >= 64<<10
}

func ps6024BenchmarkLoop(pass *analysis.Pass, fn *ast.FuncDecl) ast.Node {
	var found ast.Node
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		if found != nil {
			return false
		}
		switch value := node.(type) {
		case *ast.RangeStmt:
			if ps6024TestingN(pass, value.X) {
				found = value
				return false
			}
		case *ast.ForStmt:
			if value.Cond != nil && ps6024ContainsTestingN(pass, value.Cond) {
				found = value
				return false
			}
		}
		return true
	})
	return found
}

func ps6024TestingN(pass *analysis.Pass, expr ast.Expr) bool {
	selector, ok := ps2110Unparen(expr).(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "N" {
		return false
	}
	t := pass.TypesInfo.TypeOf(selector.X)
	if t == nil {
		return false
	}
	pointer, ok := types.Unalias(t).(*types.Pointer)
	if !ok {
		return false
	}
	named, ok := types.Unalias(pointer.Elem()).(*types.Named)
	return ok && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == "testing" && named.Obj().Name() == "B"
}

func ps6024ContainsTestingN(pass *analysis.Pass, node ast.Node) bool {
	found := false
	ast.Inspect(node, func(child ast.Node) bool {
		if found {
			return false
		}
		if expr, ok := child.(ast.Expr); ok && ps6024TestingN(pass, expr) {
			found = true
			return false
		}
		return true
	})
	return found
}

func ps6024DestinationName(name string) bool {
	name = ps6007NormalizeName(name)
	return ps6007ContainsAny(name, "dst", "dest", "out", "output", "scratch", "buffer", "buf")
}

func ps6024PassedInside(pass *analysis.Pass, loop ast.Node, object types.Object) bool {
	body := astutil.LoopBody(loop)
	if body == nil {
		return false
	}
	passed := false
	ast.Inspect(body, func(node ast.Node) bool {
		if passed {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if id, ok := ps2110Unparen(call.Fun).(*ast.Ident); ok && (id.Name == "len" || id.Name == "cap") {
			return true
		}
		for _, arg := range call.Args {
			if ps6024MentionsObject(pass, arg, map[types.Object]bool{object: true}) {
				passed = true
				return false
			}
		}
		return true
	})
	return passed
}
