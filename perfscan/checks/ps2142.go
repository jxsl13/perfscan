package checks

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS2142 implements the low-noise intra-function form of owner issue #754:
// os.Open -> io.ReadAll -> decode, where the staged []byte is not visibly
// mutated, aliased, or returned. The remedy is advisory because proving a
// regular file, mapping lifetime, and decoder ownership is non-local.
var PS2142 = register(&lint.Check{
	ID:       "PS2142",
	Category: "alloc",
	Slug:     "regular-file-readall-before-decode",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "an os.Open file is fully heap-staged by io.ReadAll before decode",
		Text: `Opening a model, archive, image, or other immutable input with
os.Open and then feeding the same untouched file to io.ReadAll allocates and
copies the complete encoded payload into a heap slice before decoding it into
separately owned output. For large regular files that staging copy can dominate
load time and peak heap. A read-only mapping can instead expose the file bytes
directly to the decoder, avoiding the whole-file allocation and copy.

This check implements a deliberately conservative intra-function data-flow
shape from owner issue #754. It requires:

  - a local *os.File produced directly by the standard library's os.Open;
  - the same file, with no intervening read/seek or unknown use, passed to the
    standard library's io.ReadAll (directly or through bufio.NewReader);
  - the returned []byte bound to a local and consumed by a later decode-style
    call or read-only indexing; and
  - no visible mutation, local slice alias, channel send, asynchronous use, or
    direct/derived return of that staging slice.

Type information pins every recognized standard-library call, so aliases work
and shadowed os/io/bufio names do not match. The no-escape screen is purposely
one-function and fail-closed. Passing []byte to a function is not a proof that
the callee neither mutates nor retains it; the finding therefore says
"candidate" and requires that ownership audit before any change.

There is NO automatic fix. A safe implementation must first prove the opened
object is a regular file, preserve a buffered fallback when mapping is
unsupported or fails, keep the mapping alive until the last consumer is done,
and verify every returned value owns its storage. It must also document the
concurrent truncation/SIGBUS risk and close/unmap ordering. Files that are
written, streamed, returned as bytes, or intentionally retained in memory are
not mapping candidates.

This is a semantic materialization finding, not a blanket mmap recommendation:
small files and one-shot decoders may not benefit, and direct ReadAt can beat a
whole mapping for selected ranges (the distinct issue #755 shape).`,
		Before: `f, err := os.Open(path)
if err != nil { return Model{}, err }
defer f.Close()
data, err := io.ReadAll(f)
if err != nil { return Model{}, err }
return decode(data)`,
		After: `// after proving a regular file and independently owned decode output:
data, unmap, err := mapReadOnly(path)
if err != nil { return decodeBuffered(path) } // portable fallback
defer unmap()
return decode(data)`,
		MeasuredWin: `GoAI GGUF, 638 MiB TinyLlama Q4_K_M on Apple M2 Pro
(10 fresh-process order-alternating pairs): buffered whole-file staging
182.00 ms -> read-only mapping 97.12 ms (1.87x, -46.64%); heap
4.735 GiB/op -> 4.113 GiB/op (-13.14%, about 668 MiB/op).`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2142",
		Doc:  "os.Open file fully heap-staged by io.ReadAll before an immutable decode",
		Run:  runPS2142,
	},
})

var ps2142BufferedReadChain = []typedCallStep{
	{PkgPath: "io", Name: "ReadAll", Kind: typedPackageFunc, Arity: 1, NextArg: 0},
	{PkgPath: "bufio", Name: "NewReader", Kind: typedPackageFunc, Arity: 1, NextArg: -1},
}

func runPS2142(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ps2142ScanBody(pass, fn.Body)
		}
	}
	return nil, nil
}

func ps2142ScanBody(pass *analysis.Pass, body *ast.BlockStmt) {
	opened := ps2142OpenedFiles(pass, body)
	if len(opened) == 0 {
		return
	}

	astutil.WithStack(body, func(node ast.Node, stack []ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		fileObj, ok := ps2142ReadAllFile(pass, call)
		openPos, wasOpened := opened[fileObj]
		if !ok || !wasOpened || openPos >= call.Pos() || !ps2142FileUntouched(pass, body, fileObj, call) {
			return true
		}
		dataObj, ok := ps2142ResultObject(pass, call, stack)
		if !ok || !ps2142DecodeOnly(pass, body, dataObj, call) {
			return true
		}
		pass.Report(analysis.Diagnostic{
			Pos:     call.Pos(),
			End:     call.End(),
			Message: "os.Open file is fully heap-staged by io.ReadAll before decode; for a large immutable regular file whose decoded outputs own their storage, evaluate a read-only mapping with a buffered fallback (no auto-fix: regular-file proof, mapping lifetime, truncation risk, and alias ownership require review)",
		})
		return true
	})
}

// ps2142OpenedFiles returns locals assigned the first result of os.Open.
func ps2142OpenedFiles(pass *analysis.Pass, body *ast.BlockStmt) map[types.Object]token.Pos {
	opened := make(map[types.Object]token.Pos)
	astutil.WithStack(body, func(node ast.Node, _ []ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		switch stmt := node.(type) {
		case *ast.AssignStmt:
			if len(stmt.Rhs) != 1 || len(stmt.Lhs) == 0 || !ps2142OpenCall(pass, stmt.Rhs[0]) {
				return true
			}
			if id, ok := ps2110Unparen(stmt.Lhs[0]).(*ast.Ident); ok {
				if obj := identObject(pass, id); obj != nil {
					opened[obj] = stmt.Pos()
				}
			}
		case *ast.ValueSpec:
			if len(stmt.Values) != 1 || len(stmt.Names) == 0 || !ps2142OpenCall(pass, stmt.Values[0]) {
				return true
			}
			if obj := identObject(pass, stmt.Names[0]); obj != nil {
				opened[obj] = stmt.Pos()
			}
		}
		return true
	})
	return opened
}

func ps2142OpenCall(pass *analysis.Pass, expr ast.Expr) bool {
	call, ok := ps2110Unparen(expr).(*ast.CallExpr)
	if !ok || len(call.Args) != 1 || call.Ellipsis.IsValid() {
		return false
	}
	fn, sig, ok := typedCallee(pass, call.Fun)
	return ok && sig.Recv() == nil && fn.Pkg() != nil && fn.Pkg().Path() == "os" && fn.Name() == "Open"
}

// ps2142ReadAllFile recognizes io.ReadAll(f) and
// io.ReadAll(bufio.NewReader(f)), returning f's object.
func ps2142ReadAllFile(pass *analysis.Pass, call *ast.CallExpr) (types.Object, bool) {
	fn, sig, ok := typedCallee(pass, call.Fun)
	if !ok || sig.Recv() != nil || fn.Pkg() == nil || fn.Pkg().Path() != "io" || fn.Name() != "ReadAll" ||
		len(call.Args) != 1 || call.Ellipsis.IsValid() {
		return nil, false
	}
	arg := ps2110Unparen(call.Args[0])
	if id, ok := arg.(*ast.Ident); ok {
		obj := pass.TypesInfo.Uses[id]
		return obj, obj != nil
	}
	chain, ok := matchTypedCallChain(pass, call, ps2142BufferedReadChain...)
	if !ok {
		return nil, false
	}
	inner := chain[1]
	id, ok := ps2110Unparen(inner.Args[0]).(*ast.Ident)
	if !ok {
		return nil, false
	}
	obj := pass.TypesInfo.Uses[id]
	return obj, obj != nil
}

func ps2142ResultObject(pass *analysis.Pass, call *ast.CallExpr, stack []ast.Node) (types.Object, bool) {
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

// ps2142FileUntouched rejects an intervening use of f other than Close or
// Stat. The f reference inside the ReadAll expression itself is ignored.
func ps2142FileUntouched(pass *analysis.Pass, body *ast.BlockStmt, fileObj types.Object, read *ast.CallExpr) bool {
	ok := true
	astutil.WithStack(body, func(node ast.Node, stack []ast.Node) bool {
		if !ok {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		id, isID := node.(*ast.Ident)
		if !isID || pass.TypesInfo.Uses[id] != fileObj || id.Pos() >= read.Pos() {
			return true
		}
		if len(stack) > 1 {
			if sel, isSel := stack[len(stack)-1].(*ast.SelectorExpr); isSel && sel.X == ast.Expr(id) {
				if call, isCall := stack[len(stack)-2].(*ast.CallExpr); isCall && call.Fun == ast.Expr(sel) {
					if sel.Sel.Name == "Stat" {
						return true
					}
					// Close is harmless before ReadAll only when its execution is
					// deferred. An immediate or asynchronous Close invalidates the
					// file-backed staging shape.
					if sel.Sel.Name == "Close" && len(stack) > 2 {
						if deferStmt, isDefer := stack[len(stack)-3].(*ast.DeferStmt); isDefer && deferStmt.Call == call {
							return true
						}
					}
				}
			}
		}
		ok = false
		return false
	})
	return ok
}

// ps2142DecodeOnly accepts a local staging slice that reaches a later call or
// read-only index, while rejecting visible mutation, aliases, and escape.
func ps2142DecodeOnly(pass *analysis.Pass, body *ast.BlockStmt, dataObj types.Object, read *ast.CallExpr) bool {
	safe := true
	consumed := false
	astutil.WithStack(body, func(node ast.Node, stack []ast.Node) bool {
		if !safe {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		id, isID := node.(*ast.Ident)
		if !isID || pass.TypesInfo.Uses[id] != dataObj || id.Pos() <= read.End() {
			return true
		}
		for _, ancestor := range stack {
			switch ancestor.(type) {
			case *ast.GoStmt, *ast.SendStmt:
				safe = false
				return false
			}
		}

		for i := len(stack) - 1; i >= 0; i-- {
			switch parent := stack[i].(type) {
			case *ast.GoStmt, *ast.SendStmt:
				safe = false
				return false
			case *ast.UnaryExpr:
				if parent.Op.String() == "&" {
					safe = false
					return false
				}
			case *ast.ReturnStmt:
				for _, result := range parent.Results {
					if ps2142ReturnAliases(pass, result, dataObj) {
						safe = false
						return false
					}
				}
				consumed = true // return decode(data) is a normal terminal consumer.
				return true
			case *ast.AssignStmt:
				for _, lhs := range parent.Lhs {
					if ps2142ContainsObject(pass, lhs, dataObj) {
						safe = false // data = ..., data[i] = ..., copy target via assignment.
						return false
					}
				}
				for _, rhs := range parent.Rhs {
					if ps2142ContainsObject(pass, rhs, dataObj) && ps2142AliasExpr(pass, rhs, dataObj) {
						safe = false // alias := data or data[...].
						return false
					}
				}
			case *ast.CallExpr:
				if ps2142MutatingBuiltin(pass, parent, dataObj) {
					safe = false
					return false
				}
				if !ps2142ObservationBuiltin(pass, parent) {
					consumed = true
				}
				return true
			case *ast.IndexExpr, *ast.SliceExpr:
				consumed = true
			}
		}
		return true
	})
	return safe && consumed
}

func ps2142ContainsObject(pass *analysis.Pass, node ast.Node, object types.Object) bool {
	found := false
	ast.Inspect(node, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && pass.TypesInfo.Uses[id] == object {
			found = true
			return false
		}
		return !found
	})
	return found
}

func ps2142AliasExpr(pass *analysis.Pass, expr ast.Expr, object types.Object) bool {
	switch x := ps2110Unparen(expr).(type) {
	case *ast.Ident:
		return pass.TypesInfo.Uses[x] == object
	case *ast.SliceExpr:
		return ps2142ContainsObject(pass, x.X, object)
	}
	return false
}

// ps2142ReturnAliases conservatively treats a non-call result containing the
// staging object as an escape. A call such as decode(data) is the candidate
// consumer this advisory is looking for; its ownership still requires review.
func ps2142ReturnAliases(pass *analysis.Pass, expr ast.Expr, object types.Object) bool {
	if _, isCall := ps2110Unparen(expr).(*ast.CallExpr); isCall {
		return false
	}
	return ps2142ContainsObject(pass, expr, object)
}

func ps2142MutatingBuiltin(pass *analysis.Pass, call *ast.CallExpr, object types.Object) bool {
	id, ok := ps2110Unparen(call.Fun).(*ast.Ident)
	if !ok || len(call.Args) == 0 {
		return false
	}
	obj, ok := pass.TypesInfo.Uses[id].(*types.Builtin)
	if !ok {
		return false
	}
	switch obj.Name() {
	case "append", "clear":
		return ps2142ContainsObject(pass, call.Args[0], object)
	case "copy":
		return ps2142ContainsObject(pass, call.Args[0], object)
	}
	return false
}

func ps2142ObservationBuiltin(pass *analysis.Pass, call *ast.CallExpr) bool {
	id, ok := ps2110Unparen(call.Fun).(*ast.Ident)
	if !ok {
		return false
	}
	obj, ok := pass.TypesInfo.Uses[id].(*types.Builtin)
	return ok && (obj.Name() == "len" || obj.Name() == "cap")
}
