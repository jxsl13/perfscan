package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS6022 implements owner issue #764 by grouping consecutive sibling
// conversion dispatches with identical geometry and independent buffer pairs.
var PS6022 = register(&lint.Check{
	ID:       "PS6022",
	Category: "verify",
	Slug:     "sibling-conversion-dispatches",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "consecutive sibling dtype-conversion dispatches differ only by source/destination buffers",
		Text: `Two or more structurally identical accelerator conversions can pay
separate encoder and launch floors even when they share geometry, dtype,
lifetime, and command-buffer placement. Key/value cache conversion is a common
case: independent f32-to-f16 K and V dispatches perform the same indexed work
over different buffer pairs and can often be encoded as one paired stream.

This check implements owner issue #764 with typed multi-call matching. It
reports a group only when:

  - every call resolves to the same conversion function or method;
  - the callee or surrounding context establishes f32/f16, bf16, or another
    dtype cast/pack/conversion on an accelerator;
  - method calls use the same recorder, encoder, graph, stream, command-buffer,
    or device receiver;
  - all scalar geometry/offset/length arguments are syntactically identical;
  - exactly two data-buffer arguments differ, yielding independent source and
    destination pairs; and
  - no other command on that receiver appears between the sibling calls.

An already Paired/Dual/MultiStream conversion API stays silent. Calls with
different row geometry, offsets, flags, or lifetimes also stay silent, as do
ordinary CPU conversion helpers without an accelerator/command context.

There is NO automatic fix. A paired kernel must preserve the original
conversion semantics and both streams' alignment. Vectorize (for example,
half2) only when every participating stream is aligned, retain a scalar path
for unaligned values and odd tails, and gate exact IEEE conversion bytes,
finite downstream values, and end-to-end semantic quality before promotion.`,
		Before: `rec.F32ToF16(kSource, kCache, rows, width)
rec.F32ToF16(vSource, vCache, rows, width)`,
		After: `rec.PairedF32ToF16(
	kSource, kCache,
	vSource, vCache,
	rows, width,
) // half2 only when both streams align; scalar odd-tail fallback`,
		MeasuredWin: `In the Apple-M2 f16 K/V-cache campaign behind issue #764,
two conversion dispatches per layer cost about 143.6 us across the model and
left ctx512 at only 1.019x-1.023x. A paired dispatch with aligned half2 and a
scalar odd-tail path reduced the conversion family to roughly 95-108 us; three
trained-model campaigns then measured ctx512 at 1.0310x-1.0358x, ctx1024 at
1.0503x-1.0634x, and ctx1536 at 1.0624x-1.0691x.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6022",
		Doc:  "consecutive accelerator dtype conversions share geometry and differ only by buffer pairs",
		Run:  runPS6022,
	},
})

type ps6022Call struct {
	call     *ast.CallExpr
	function *types.Func
	block    *ast.BlockStmt
	receiver string
	recvType types.Type
}

func runPS6022(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			text := ps6015FunctionText(pass, file, fn)
			parents := ps6019Parents(fn.Body)
			calls := ps6022Calls(pass, fn.Body, parents)
			consumed := make(map[*ast.CallExpr]bool)
			for i, first := range calls {
				if consumed[first.call] || !ps6022ConversionContext(first, text) {
					continue
				}
				group := []ps6022Call{first}
				for _, candidate := range calls[i+1:] {
					if candidate.block != first.block {
						continue
					}
					if !ps6022SameCommandReceiver(first, candidate) {
						continue
					}
					if candidate.function != first.function {
						break
					}
					if !ps6022SiblingArgs(pass, first.call, candidate.call) {
						break
					}
					group = append(group, candidate)
				}
				if len(group) < 2 {
					continue
				}
				for _, call := range group {
					consumed[call.call] = true
				}
				pass.Reportf(first.call.Pos(), "%d consecutive %s conversion dispatches use the same command context and scalar geometry but independent source/destination buffer pairs; evaluate one paired/N-stream dispatch, preserving all-stream alignment checks, scalar odd tails, exact conversion bytes, and end-to-end semantic gates", len(group), first.function.Name())
			}
		}
	}
	return nil, nil
}

func ps6022Calls(pass *analysis.Pass, body *ast.BlockStmt, parents map[ast.Node]ast.Node) []ps6022Call {
	base := ps6020Calls(pass, body, parents)
	calls := make([]ps6022Call, 0, len(base))
	for _, item := range base {
		function, _, ok := typedCallee(pass, item.call.Fun)
		if !ok {
			continue
		}
		call := ps6022Call{call: item.call, function: function, block: item.block}
		if selector, ok := ps2110Unparen(item.call.Fun).(*ast.SelectorExpr); ok {
			call.receiver = exprTextRendered(selector.X)
			call.recvType = pass.TypesInfo.TypeOf(selector.X)
		}
		calls = append(calls, call)
	}
	return calls
}

func ps6022ConversionContext(call ps6022Call, enclosing string) bool {
	name := ps6007NormalizeName(call.function.Name())
	if ps6007ContainsAny(name, "paired", "pair", "dual", "multistream", "multibuffer", "interleaved") {
		return false
	}
	dtypes := ps6007ContainsAny(name,
		"f32tof16", "float32tofloat16", "f16tof32", "float16tofloat32",
		"f32tobf16", "bf16tof32", "float32bfloat16", "bfloat16float32",
	) || ps6007ContainsAny(name, "f32f16", "f16f32", "f32bf16", "bf16f32")
	conversion := ps6007ContainsAny(name, "convert", "conversion", "cast", "pack", "store", "tohalf")
	context := ps6007NormalizeName(enclosing)
	if !dtypes {
		dtypes = ps6007ContainsAny(context, "f32tof16", "float32tofloat16", "f16tof32", "float16tofloat32", "f32tobf16", "bf16tof32")
	}
	if !conversion {
		conversion = dtypes
	}
	accelerator := ps6022CommandType(call.recvType) || ps6007ContainsAny(context, "gpu", "metal", "mps", "cuda", "vulkan", "accelerator", "commandbuffer", "graphstep")
	return dtypes && conversion && accelerator
}

func ps6022CommandType(t types.Type) bool {
	if t == nil {
		return false
	}
	name := ps6007NormalizeName(types.TypeString(t, func(*types.Package) string { return "" }))
	return ps6007ContainsAny(name, "recorder", "encoder", "commandbuffer", "command", "graph", "stream", "device", "gpu")
}

func ps6022SameCommandReceiver(left, right ps6022Call) bool {
	if left.block == nil || right.block != left.block {
		return false
	}
	if left.receiver == "" {
		return right.receiver == ""
	}
	return right.receiver == left.receiver
}

func ps6022SiblingArgs(pass *analysis.Pass, left, right *ast.CallExpr) bool {
	if len(left.Args) != len(right.Args) || len(left.Args) < 2 {
		return false
	}
	var leftData, rightData []string
	for i := range left.Args {
		leftText := exprTextRendered(left.Args[i])
		rightText := exprTextRendered(right.Args[i])
		if leftText == rightText {
			continue
		}
		if !ps6022DataType(pass.TypesInfo.TypeOf(left.Args[i])) || !ps6022DataType(pass.TypesInfo.TypeOf(right.Args[i])) {
			return false
		}
		leftData = append(leftData, leftText)
		rightData = append(rightData, rightText)
	}
	if len(leftData) != 2 {
		return false
	}
	seen := make(map[string]bool, 4)
	for _, name := range append(leftData, rightData...) {
		if name == "" || seen[name] {
			return false
		}
		seen[name] = true
	}
	return true
}

func ps6022DataType(t types.Type) bool {
	if t == nil {
		return false
	}
	switch types.Unalias(t).Underlying().(type) {
	case *types.Slice, *types.Array:
		return true
	}
	name := ps6007NormalizeName(types.TypeString(t, func(*types.Package) string { return "" }))
	return ps6007ContainsAny(name, "buffer", "buf", "tensor", "storage", "vector", "matrix")
}
