package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS5066 reports buf.String()[i] on a bytes.Buffer — copying the ENTIRE buffer
// into a fresh string just to read one byte — where buf.Bytes()[i] reads that
// byte from the buffer's backing array with no copy. bytes.Buffer.String()
// allocates (string(b.buf[b.off:])); Bytes() returns the internal slice.
var PS5066 = register(&lint.Check{
	ID:       "PS5066",
	Category: "alloc",
	Slug:     "buffer-string-index-to-bytes",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "indexing bytes.Buffer.String() copies the whole buffer to read one byte; buf.Bytes()[i] reads it in place",
		Text: `bytes.Buffer.String() returns the unread portion as a freshly allocated
string (string(b.buf[b.off:])); indexing it, buf.String()[i], then throws that
whole copy away after reading a single byte. buf.Bytes() returns the buffer's
internal slice directly — no allocation — and buf.Bytes()[i] reads the same byte.

The rewrite is BIT-IDENTICAL: String() and Bytes() both expose the same unread
region in the same order, so buf.String()[i] and buf.Bytes()[i] are the same
byte value and share the same bounds — index i panics for exactly the same i
(>= buf.Len()) in both. The single index read happens immediately, before any
buffer mutation, so the slice Bytes() returns is read at the same moment
String()'s copy would have been.

The match is deliberately narrow — it is the whole safety story:
  - the expression is an INDEX buf.String()[i], not a slice buf.String()[i:j]
    (slicing a string yields a string while slicing Bytes() yields []byte — a
    different type);
  - the callee is bytes.Buffer.String() with no arguments, pinned by type
    information to a receiver whose type is exactly bytes.Buffer or *bytes.Buffer
    (a wrapper type embedding bytes.Buffer is excluded — it could override
    Bytes() with a different method — and strings.Builder.String() is left alone:
    its String() is already zero-copy, so there is nothing to save).
The fix renames only the method (String -> Bytes); the receiver and the index
carry over verbatim and no import changes. A comment inside the renamed method
selector withholds the fix.`,
		Before: `c := buf.String()[0]`,
		After:  `c := buf.Bytes()[0]`,
		MeasuredWin: `On a 1024-byte bytes.Buffer (Apple M2 Pro, go1.26): buf.String()[i] ` +
			`~120 ns/op, 1024 B/op, 1 alloc/op vs buf.Bytes()[i] ~0.8 ns/op, 0 B/op, 0 allocs/op ` +
			`(~150x) — the whole-buffer string copy is eliminated; the saving grows with the buffer size.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5066",
		Doc:  "indexing bytes.Buffer.String() instead of buf.Bytes()[i]",
		Run:  runPS5066,
	},
})

func runPS5066(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			// buf.String()[i] — an index (not a slice) of a String() call.
			idx, ok := n.(*ast.IndexExpr)
			if !ok {
				return true
			}
			call, ok := idx.X.(*ast.CallExpr)
			if !ok || len(call.Args) != 0 || call.Ellipsis.IsValid() {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
			if !ok || fn.Name() != "String" || fn.Pkg() == nil || fn.Pkg().Path() != "bytes" {
				return true
			}
			// The receiver's type must be exactly bytes.Buffer / *bytes.Buffer,
			// so buf.Bytes() resolves to bytes.Buffer.Bytes and not a wrapper's
			// override.
			if !ps5066RecvIsBuffer(pass, sel.X) {
				return true
			}

			diag := analysis.Diagnostic{
				Pos:     idx.Pos(),
				End:     idx.End(),
				Message: "indexing bytes.Buffer.String() copies the whole buffer to read one byte; buf.Bytes()[i] reads it from the backing array with no copy",
			}
			if !ps2111CommentIn(f, sel.Sel.Pos(), sel.Sel.End()) {
				diag.SuggestedFixes = []analysis.SuggestedFix{{
					Message: "read the byte in place with buf.Bytes()[i]",
					TextEdits: []analysis.TextEdit{
						{Pos: sel.Sel.Pos(), End: sel.Sel.End(), NewText: []byte("Bytes")},
					},
				}}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}

// ps5066RecvIsBuffer reports whether x's type is exactly bytes.Buffer or a
// pointer to it (not a wrapper type that embeds it).
func ps5066RecvIsBuffer(pass *analysis.Pass, x ast.Expr) bool {
	t := pass.TypesInfo.TypeOf(x)
	if t == nil {
		return false
	}
	if p, isPtr := t.(*types.Pointer); isPtr {
		t = p.Elem()
	}
	named, ok := t.(*types.Named)
	if !ok {
		return false
	}
	obj := named.Obj()
	return obj.Name() == "Buffer" && obj.Pkg() != nil && obj.Pkg().Path() == "bytes"
}
