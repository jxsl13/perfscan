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

// PS2143 implements the conservative intra-function form of owner issue #755:
// a selected ReadAt payload and marshaled JSON header are rebuilt in a fresh
// bytes.Buffer, fed to a full collection loader, then immediately indexed.
var PS2143 = register(&lint.Check{
	ID:       "PS2143",
	Category: "alloc",
	Slug:     "partial-read-rebuilt-for-full-parser",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "a partial ReadAt payload is rebuilt with a synthetic JSON header for a full collection parser, then one item is selected",
		Text: `A partial loader can read one selected file range, then erase that
advantage by marshaling a miniature header, copying header and payload into a
fresh in-memory container, invoking the full collection parser, and selecting
one item from its result. The path pays for synthetic serialization, container
growth, a second payload copy, generic parser setup, and collection creation
even though the caller requested one entry.

This check implements a deliberately conservative intra-function neighborhood
from owner issue #755. It requires all of the following in one function:

  - standard-library os.File.ReadAt or io.ReaderAt.ReadAt fills a local []byte
    using a positive or nonconstant offset;
  - encoding/json.Marshal produces a local synthetic-header []byte;
  - a locally created zero bytes.Buffer receives both that header and the
    ReadAt payload through bytes.Buffer.Write;
  - a later call consumes that buffer's Bytes result, directly or through a
    nested adapter such as bytes.NewReader; and
  - the call's first result is a map, slice, or array local whose only later use
    is one index operation.

Every recognized standard-library operation is resolved through go/types, so
import aliases work and shadowed functions or user-defined ReadAt/Buffer types
do not match. Requiring the same local objects to flow through both writes and
into the indexed collection keeps the diagnostic focused on synthetic
single-item reconstruction rather than ordinary JSON or buffer use.

There is NO automatic fix. The repair is architectural: factor shared entry
validation and decoding primitives, validate the selected metadata directly,
and ReadAt into independently owned final storage. Preserve bounds, alignment,
checksum, dtype, shape, overflow, short-read, and error behavior from the full
parser. Do not replace the path with an aliased mmap slice unless the public
ownership and mapping lifetime explicitly permit that contract.

A regression fixture for the repaired loader should place one small selected
item beside a much larger unselected item, assert exact output/error parity,
verify that the result owns its storage, and report ns/op, B/op, and allocs/op.
The detector is advisory because proving those format-specific invariants is
interprocedural and cannot be synthesized safely from this AST neighborhood.`,
		Before: `payload := make([]byte, entry.Size)
_, err = f.ReadAt(payload, entry.Offset)
header, err := json.Marshal(singleEntryHeader(entry))
var synthetic bytes.Buffer
_, _ = synthetic.Write(header)
_, _ = synthetic.Write(payload)
items, err := parseAll(synthetic.Bytes())
return items[name], err`,
		After: `// factor the full parser's validation/decode primitive first
out := make([]byte, entry.DecodedSize)
if err := readAndDecodeEntryAt(f, entry, out); err != nil { return Tensor{}, err }
return Tensor{Data: out, Shape: entry.Shape}, nil`,
		MeasuredWin: `GoAI safetensors.LoadTensor, 4 MiB selected from a 64 MiB
file on Apple M2 Pro: synthetic-container full-parser path 614468 ns/op,
12602318 B/op, 124 allocs/op -> shared validation plus direct ReadAt into final
storage 280561 ns/op, 4200899 B/op, 80 allocs/op (2.19x, -66.67% heap).
A whole-file mmap was separately 31.18% slower than direct ReadAt.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2143",
		Doc:  "partial ReadAt payload rebuilt with a synthetic header for a full parser before selecting one item",
		Run:  runPS2143,
	},
})

type ps2143BufferState struct {
	defined      token.Pos
	headerWrite  token.Pos
	payloadWrite token.Pos
	interference []token.Pos
}

func runPS2143(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ps2143ScanBody(pass, fn.Body)
		}
	}
	return nil, nil
}

func ps2143ScanBody(pass *analysis.Pass, body *ast.BlockStmt) {
	partials, headers := ps2143Sources(pass, body)
	if len(partials) == 0 || len(headers) == 0 {
		return
	}
	buffers := ps2143FreshBuffers(pass, body)
	if len(buffers) == 0 {
		return
	}
	ps2143TrackWrites(pass, body, partials, headers, buffers)

	astutil.WithStack(body, func(node ast.Node, stack []ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		buffer, ok := ps2143BufferInput(pass, call, buffers)
		if !ok || !ps2143BufferReadyAt(buffer, call.Pos()) {
			return true
		}
		collection, ok := ps2143ResultObject(pass, call, stack)
		if !ok || !ps2143CollectionType(collection.Type()) || !ps2143OnlyIndexed(pass, body, collection, call) {
			return true
		}
		pass.Report(analysis.Diagnostic{
			Pos:     call.Pos(),
			End:     call.End(),
			Message: "partial ReadAt payload is copied with a marshaled JSON header into a fresh bytes.Buffer, passed through a full collection parser, then only one item is indexed; factor shared entry validation and decode the selected range directly into independently owned final storage",
		})
		return false
	})
}

// ps2143Sources returns local byte slices filled by a partial-looking ReadAt
// and local byte slices returned as json.Marshal's first result.
func ps2143Sources(pass *analysis.Pass, body *ast.BlockStmt) (map[types.Object]token.Pos, map[types.Object]token.Pos) {
	partials := make(map[types.Object]token.Pos)
	headers := make(map[types.Object]token.Pos)
	astutil.WithStack(body, func(node ast.Node, stack []ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if payload, ok := ps2143ReadAtPayload(pass, call); ok {
			partials[payload] = call.Pos()
		}
		if ps2143JSONMarshal(pass, call) {
			if header, ok := ps2143ResultObject(pass, call, stack); ok && ps2143ByteSlice(header.Type()) {
				headers[header] = call.Pos()
			}
		}
		return true
	})
	return partials, headers
}

func ps2143ReadAtPayload(pass *analysis.Pass, call *ast.CallExpr) (types.Object, bool) {
	fn, sig, ok := typedCallee(pass, call.Fun)
	if !ok || sig.Recv() == nil || fn.Pkg() == nil || fn.Name() != "ReadAt" ||
		(fn.Pkg().Path() != "os" && fn.Pkg().Path() != "io") || len(call.Args) != 2 || call.Ellipsis.IsValid() {
		return nil, false
	}
	if value := pass.TypesInfo.Types[ps2110Unparen(call.Args[1])].Value; value != nil && value.Kind() == constant.Int && constant.Sign(value) <= 0 {
		return nil, false
	}
	id := ps2143BaseIdent(call.Args[0])
	if id == nil || !ps2143ByteSlice(pass.TypesInfo.TypeOf(call.Args[0])) {
		return nil, false
	}
	obj := pass.TypesInfo.Uses[id]
	return obj, obj != nil
}

func ps2143JSONMarshal(pass *analysis.Pass, call *ast.CallExpr) bool {
	fn, sig, ok := typedCallee(pass, call.Fun)
	return ok && sig.Recv() == nil && fn.Pkg() != nil && fn.Pkg().Path() == "encoding/json" &&
		fn.Name() == "Marshal" && len(call.Args) == 1 && !call.Ellipsis.IsValid()
}

func ps2143FreshBuffers(pass *analysis.Pass, body *ast.BlockStmt) map[types.Object]*ps2143BufferState {
	buffers := make(map[types.Object]*ps2143BufferState)
	astutil.WithStack(body, func(node ast.Node, _ []ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		switch n := node.(type) {
		case *ast.ValueSpec:
			if len(n.Values) != 0 {
				return true
			}
			for _, name := range n.Names {
				if obj := pass.TypesInfo.Defs[name]; obj != nil && ps2143BufferType(obj.Type()) {
					buffers[obj] = &ps2143BufferState{defined: name.Pos()}
				}
			}
		case *ast.AssignStmt:
			if n.Tok != token.DEFINE || len(n.Lhs) != len(n.Rhs) {
				return true
			}
			for i, lhs := range n.Lhs {
				id, ok := ps2110Unparen(lhs).(*ast.Ident)
				if !ok || !ps2143FreshBufferExpr(pass, n.Rhs[i]) {
					continue
				}
				if obj := pass.TypesInfo.Defs[id]; obj != nil && ps2143BufferType(obj.Type()) {
					buffers[obj] = &ps2143BufferState{defined: id.Pos()}
				}
			}
		}
		return true
	})
	return buffers
}

func ps2143FreshBufferExpr(pass *analysis.Pass, expr ast.Expr) bool {
	if !ps2143BufferType(pass.TypesInfo.TypeOf(expr)) {
		return false
	}
	switch x := ps2110Unparen(expr).(type) {
	case *ast.UnaryExpr:
		_, composite := ps2110Unparen(x.X).(*ast.CompositeLit)
		return x.Op == token.AND && composite
	case *ast.CallExpr:
		id, ok := ps2110Unparen(x.Fun).(*ast.Ident)
		builtin, isBuiltin := pass.TypesInfo.Uses[id].(*types.Builtin)
		return ok && isBuiltin && builtin.Name() == "new" && len(x.Args) == 1
	}
	return false
}

func ps2143TrackWrites(pass *analysis.Pass, body *ast.BlockStmt, partials, headers map[types.Object]token.Pos, buffers map[types.Object]*ps2143BufferState) {
	astutil.WithStack(body, func(node ast.Node, _ []ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		if assign, ok := node.(*ast.AssignStmt); ok {
			for _, lhs := range assign.Lhs {
				id, isID := ps2110Unparen(lhs).(*ast.Ident)
				if !isID {
					continue
				}
				if state := buffers[pass.TypesInfo.Uses[id]]; state != nil {
					state.interference = append(state.interference, assign.Pos())
				}
			}
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		fn, sig, ok := typedCallee(pass, call.Fun)
		if !ok || sig.Recv() == nil || fn.Pkg() == nil || fn.Pkg().Path() != "bytes" {
			return true
		}
		selector, ok := ps2110Unparen(call.Fun).(*ast.SelectorExpr)
		if !ok {
			return true
		}
		receiver := ps2143BaseIdent(selector.X)
		if receiver == nil {
			return true
		}
		state := buffers[pass.TypesInfo.Uses[receiver]]
		if state == nil || state.defined >= call.Pos() {
			return true
		}
		if fn.Name() != "Write" {
			switch fn.Name() {
			case "Bytes", "String", "Len", "Cap", "Available", "Grow":
				// Observation and capacity reservation do not alter the
				// reconstructed byte sequence.
			default:
				state.interference = append(state.interference, call.Pos())
			}
			return true
		}
		if len(call.Args) != 1 || call.Ellipsis.IsValid() {
			return true
		}
		source := ps2143BaseIdent(call.Args[0])
		if source == nil {
			return true
		}
		object := pass.TypesInfo.Uses[source]
		if object == nil {
			return true
		}
		if pos, ok := headers[object]; ok && pos < call.Pos() && state.headerWrite == token.NoPos &&
			!ps2143Reassigned(pass, body, object, pos, call.Pos()) {
			state.headerWrite = call.Pos()
		}
		if pos, ok := partials[object]; ok && pos < call.Pos() && state.payloadWrite == token.NoPos &&
			!ps2143Reassigned(pass, body, object, pos, call.Pos()) {
			state.payloadWrite = call.Pos()
		}
		return true
	})
}

func ps2143BufferReadyAt(state *ps2143BufferState, loader token.Pos) bool {
	if state == nil || state.headerWrite == token.NoPos || state.payloadWrite == token.NoPos ||
		state.headerWrite == state.payloadWrite || state.headerWrite >= loader || state.payloadWrite >= loader {
		return false
	}
	start := state.headerWrite
	if state.payloadWrite < start {
		start = state.payloadWrite
	}
	for _, pos := range state.interference {
		if pos > start && pos < loader {
			return false
		}
	}
	return true
}

func ps2143Reassigned(pass *analysis.Pass, body *ast.BlockStmt, object types.Object, after, before token.Pos) bool {
	reassigned := false
	astutil.WithStack(body, func(node ast.Node, _ []ast.Node) bool {
		if reassigned {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		assign, ok := node.(*ast.AssignStmt)
		if !ok || assign.Pos() <= after || assign.Pos() >= before {
			return true
		}
		for _, lhs := range assign.Lhs {
			id, isID := ps2110Unparen(lhs).(*ast.Ident)
			if isID && pass.TypesInfo.Uses[id] == object {
				reassigned = true
				return false
			}
		}
		return true
	})
	return reassigned
}

// ps2143BufferInput finds a Buffer.Bytes call nested in one of call's
// arguments and returns the corresponding fully tracked local buffer.
func ps2143BufferInput(pass *analysis.Pass, call *ast.CallExpr, buffers map[types.Object]*ps2143BufferState) (*ps2143BufferState, bool) {
	var found *ps2143BufferState
	for _, arg := range call.Args {
		ast.Inspect(arg, func(node ast.Node) bool {
			if found != nil {
				return false
			}
			inner, ok := node.(*ast.CallExpr)
			if !ok || len(inner.Args) != 0 {
				return true
			}
			fn, sig, ok := typedCallee(pass, inner.Fun)
			if !ok || sig.Recv() == nil || fn.Pkg() == nil || fn.Pkg().Path() != "bytes" || fn.Name() != "Bytes" {
				return true
			}
			selector, ok := ps2110Unparen(inner.Fun).(*ast.SelectorExpr)
			if !ok {
				return true
			}
			id := ps2143BaseIdent(selector.X)
			if id != nil {
				found = buffers[pass.TypesInfo.Uses[id]]
			}
			return found == nil
		})
	}
	return found, found != nil
}

func ps2143ResultObject(pass *analysis.Pass, call *ast.CallExpr, stack []ast.Node) (types.Object, bool) {
	for i := len(stack) - 1; i >= 0; i-- {
		switch parent := stack[i].(type) {
		case *ast.AssignStmt:
			if len(parent.Rhs) != 1 || ps2110Unparen(parent.Rhs[0]) != ast.Expr(call) || len(parent.Lhs) == 0 {
				return nil, false
			}
			id, ok := ps2110Unparen(parent.Lhs[0]).(*ast.Ident)
			if !ok || id.Name == "_" {
				return nil, false
			}
			obj := identObject(pass, id)
			return obj, obj != nil
		case *ast.ValueSpec:
			if len(parent.Values) != 1 || ps2110Unparen(parent.Values[0]) != ast.Expr(call) || len(parent.Names) == 0 || parent.Names[0].Name == "_" {
				return nil, false
			}
			obj := identObject(pass, parent.Names[0])
			return obj, obj != nil
		case *ast.ReturnStmt:
			return nil, false
		}
	}
	return nil, false
}

func ps2143OnlyIndexed(pass *analysis.Pass, body *ast.BlockStmt, object types.Object, loader *ast.CallExpr) bool {
	safe := true
	indexes := make(map[token.Pos]bool)
	astutil.WithStack(body, func(node ast.Node, stack []ast.Node) bool {
		if !safe {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		id, ok := node.(*ast.Ident)
		if !ok || pass.TypesInfo.Uses[id] != object || id.Pos() <= loader.End() {
			return true
		}
		if len(stack) == 0 {
			safe = false
			return false
		}
		index, ok := stack[len(stack)-1].(*ast.IndexExpr)
		if !ok || index.X != ast.Expr(id) {
			safe = false
			return false
		}
		indexes[index.Pos()] = true
		return true
	})
	return safe && len(indexes) == 1
}

func ps2143BaseIdent(expr ast.Expr) *ast.Ident {
	switch x := ps2110Unparen(expr).(type) {
	case *ast.Ident:
		return x
	case *ast.SliceExpr:
		return ps2143BaseIdent(x.X)
	}
	return nil
}

func ps2143ByteSlice(t types.Type) bool {
	if t == nil {
		return false
	}
	t = types.Unalias(t)
	if named, ok := t.(*types.Named); ok {
		t = named.Underlying()
	}
	slice, ok := t.(*types.Slice)
	if !ok {
		return false
	}
	basic, ok := types.Unalias(slice.Elem()).(*types.Basic)
	return ok && basic.Kind() == types.Byte
}

func ps2143BufferType(t types.Type) bool {
	if t == nil {
		return false
	}
	t = types.Unalias(t)
	if pointer, ok := t.(*types.Pointer); ok {
		t = types.Unalias(pointer.Elem())
	}
	named, ok := t.(*types.Named)
	return ok && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == "bytes" && named.Obj().Name() == "Buffer"
}

func ps2143CollectionType(t types.Type) bool {
	if t == nil {
		return false
	}
	t = types.Unalias(t)
	if named, ok := t.(*types.Named); ok {
		t = named.Underlying()
	}
	switch t.(type) {
	case *types.Map, *types.Slice, *types.Array:
		return true
	}
	return false
}
