package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS3071 reports methods that declare a local fixed-size byte array and pass
// a slice of it to an interface-taking call — the array escapes and is
// heap-allocated on every invocation.
var PS3071 = register(&lint.Check{
	ID:       "PS3071",
	Category: "indirect",
	Slug:     "local-buffer-escapes-per-call",
	Level:    lint.LevelStructured,
	Doc: lint.Documentation{
		Title: "a method-local fixed-size buffer sliced into an interface-taking call",
		Text: `A method that declares var buf [8]byte and hands buf[:] to a call
whose parameter is an interface (io.ReadFull and friends) forces the array
to escape: one heap allocation per invocation, which a reader primitive pays
once per scalar it decodes. Hang the buffer on the receiver — already on the
heap — and the per-call allocation disappears.

Safe only if nothing keeps the buffer beyond the call (string(b) copies, so
the string case is fine). Check every caller before sharing one scratch, and
check the type is not used concurrently.`,
		Before: `func (r *reader) u32() uint32 {
	var buf [4]byte
	io.ReadFull(r.src, buf[:])
	return binary.LittleEndian.Uint32(buf[:])
}`,
		After: `type reader struct {
	src io.Reader
	scratch [8]byte // reused across calls; reader is not used concurrently
}

func (r *reader) u32() uint32 {
	io.ReadFull(r.src, r.scratch[:4])
	return binary.LittleEndian.Uint32(r.scratch[:4])
}`,
		MeasuredWin: "reference corpus: GGUF header reader allocations -57.2% (223892→95804) and -17.3% time on the header-heavy cell",
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS3071",
		Doc:  "method-local buffer escaping through an interface call",
		Run:  runPS3071,
	},
})

func runPS3071(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || fn.Body == nil {
				continue
			}
			checkEscapingBuffer(pass, fn)
		}
	}
	return nil, nil
}

func checkEscapingBuffer(pass *analysis.Pass, fn *ast.FuncDecl) {
	// Local fixed-size array declarations: var buf [N]byte.
	arrays := map[string]bool{}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		ds, ok := n.(*ast.DeclStmt)
		if !ok {
			return true
		}
		gd, ok := ds.Decl.(*ast.GenDecl)
		if !ok {
			return true
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			at, ok := vs.Type.(*ast.ArrayType)
			if !ok || at.Len == nil {
				continue
			}
			for _, name := range vs.Names {
				arrays[name.Name] = true
			}
		}
		return true
	})
	if len(arrays) == 0 {
		return
	}

	reported := map[string]bool{}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		for i, arg := range call.Args {
			se, ok := arg.(*ast.SliceExpr)
			if !ok {
				continue
			}
			id, ok := se.X.(*ast.Ident)
			if !ok || !arrays[id.Name] || reported[id.Name] {
				continue
			}
			if !callMayEscapeBuffer(pass, call, i) {
				continue
			}
			reported[id.Name] = true
			pass.Report(analysis.Diagnostic{
				Pos:     se.Pos(),
				End:     se.End(),
				Message: "local array " + id.Name + " sliced into an interface parameter escapes and is heap-allocated on every call; hang the buffer on the receiver (check the type is not used concurrently)",
			})
		}
		return true
	})
}

// callMayEscapeBuffer reports whether passing a buffer slice as the i-th
// argument of call plausibly forces the backing array to escape: the buffer
// lands in an interface parameter (boxed), the call is a method on an
// interface value (r.Read(buf)), or an interface-typed argument travels
// alongside it (io.ReadFull(src, buf) — the callee hands buf through the
// interface, and escape analysis cannot devirtualize).
func callMayEscapeBuffer(pass *analysis.Pass, call *ast.CallExpr, i int) bool {
	// Method called on an interface value.
	if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
		if rt := pass.TypesInfo.TypeOf(sel.X); rt != nil && types.IsInterface(rt) {
			return true
		}
	}
	sig, ok := pass.TypesInfo.TypeOf(call.Fun).(*types.Signature)
	if !ok {
		return false
	}
	params := sig.Params()
	paramType := func(idx int) types.Type {
		if sig.Variadic() && idx >= params.Len()-1 {
			if sl, ok := params.At(params.Len() - 1).Type().(*types.Slice); ok {
				return sl.Elem()
			}
		}
		if idx < params.Len() {
			return params.At(idx).Type()
		}
		return nil
	}
	for j := range call.Args {
		t := paramType(j)
		if t == nil {
			continue
		}
		if j == i && types.IsInterface(t) {
			return true // the buffer itself is boxed
		}
		if j != i && types.IsInterface(t) {
			return true // an interface travels alongside the buffer
		}
	}
	return false
}
