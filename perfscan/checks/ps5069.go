package checks

import (
	"go/ast"
	"go/constant"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS5069 reports the string-predicate functions from package strings applied
// to buf.String() over a bytes.Buffer against a compile-time string constant:
// strings.HasPrefix(buf.String(), "OK"), strings.Contains, strings.Index,
// strings.Count and their kin. buf.String() heap-allocates and byte-copies
// the buffer's entire unread contents on every call; the strings predicate
// then answers a question the equivalent bytes predicate answers over the
// zero-copy Bytes() view with a stack-allocated []byte(constant). The
// buffer-copy allocation is pure waste. The byte-slice twin of the
// buf.String()-vs-constant family (PS2031's bytes.Equal companion).
var PS5069 = register(&lint.Check{
	ID:       "PS5069",
	Category: "alloc",
	Slug:     "strings-pred-over-buffer-string",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "strings.HasPrefix/Contains/Index/... over buf.String() copies the whole bytes.Buffer; the bytes twin over buf.Bytes() reads it with no copy",
		Text: `bytes.Buffer.String() heap-allocates a fresh string and byte-copies
the buffer's entire unread contents into it. Feeding that string to a
strings predicate against a short compile-time constant —
strings.HasPrefix(buf.String(), "OK"), strings.Contains(buf.String(),
"\n\n"), strings.Index(buf.String(), "sep"), and so on — pays a
full-buffer copy (and its allocation) on every evaluation. The package
bytes twin computes the identical answer over Bytes() with none of it:
Bytes() returns the zero-copy b.buf[b.off:] slice header, and the tiny
constant's []byte conversion is stack-allocated (its operand is a
constant and the argument does not escape into the read-only predicate).

The recognized functions each have a bytes counterpart with the same
name and an identical byte-level algorithm:

  strings.HasPrefix(buf.String(), c) -> bytes.HasPrefix(buf.Bytes(), []byte(c))
  strings.HasSuffix(buf.String(), c) -> bytes.HasSuffix(buf.Bytes(), []byte(c))
  strings.Contains (buf.String(), c) -> bytes.Contains (buf.Bytes(), []byte(c))
  strings.EqualFold(buf.String(), c) -> bytes.EqualFold(buf.Bytes(), []byte(c))
  strings.Index    (buf.String(), c) -> bytes.Index    (buf.Bytes(), []byte(c))
  strings.LastIndex(buf.String(), c) -> bytes.LastIndex(buf.Bytes(), []byte(c))
  strings.Count    (buf.String(), c) -> bytes.Count    (buf.Bytes(), []byte(c))

The rewrite is BIT-IDENTICAL under the fix's gates. For a non-nil
receiver, String() returns string(b.buf[b.off:]) and Bytes() returns
b.buf[b.off:] — the same bytes. The strings and bytes functions share
the same byte-level implementation (Index/Count perform the identical
byte search and count non-overlapping matches; HasPrefix/HasSuffix/
Contains test raw byte spans; EqualFold applies the identical Unicode
simple-fold to the same bytes), and each returns the same concrete type
(bool or int) as its strings original, so the value and its type are
untouched. The predicate is read-only and the receiver is evaluated
exactly once in the same position, so side effects and evaluation order
are preserved, for every buffer state (fresh, written, partially or
fully drained, Reset, Truncate, arbitrary — including invalid-UTF-8 —
bytes) and every constant. The one divergent input is a nil
*bytes.Buffer: String() returns the sentinel "<nil>" while Bytes()
dereferences and panics — so the fix applies only when the receiver is
provably non-nil (a value-typed bytes.Buffer, &x, or new(bytes.Buffer)).

The needle is restricted to a NON-EMPTY compile-time string constant:
its []byte conversion is stack-allocated and free, so the buffer copy is
eliminated outright with nothing traded for it. A non-constant needle
would swap the buffer copy for an operand copy of unknown size and is
left as a plain advisory report. The empty string is excluded too — it
makes every predicate a degenerate constant (HasPrefix "" is always
true, Index "" is always 0).

The automatic fix applies only when type information proves the shape:
the callee resolves to the standard library's strings.HasPrefix (etc.),
the first argument is a (*bytes.Buffer).String call whose receiver's
static type is exactly bytes.Buffer or *bytes.Buffer, and the second is
a non-empty string constant. It renames the strings selector to the
bytes one, swaps buf.String() for buf.Bytes(), and wraps the constant in
[]byte(...). The fix requires package bytes to be importable at the site
(added when missing, except in a cgo file whose import block must not be
touched) and is withheld file-wide unless package strings retains
another use afterward, so it never orphans the strings import. A comment
inside the rewritten call withholds the fix rather than destroying it.`,
		Before: `var buf bytes.Buffer
// ...
if strings.HasPrefix(buf.String(), "GET ") {
	return
}`,
		After: `var buf bytes.Buffer
// ...
if bytes.HasPrefix(buf.Bytes(), []byte("GET ")) {
	return
}`,
		MeasuredWin: `bytes.Buffer holding 4 KiB (Apple M2 Pro, go1.26):
strings.HasPrefix(buf.String(), "xy") 387 ns/op, 4864 B/op, 1 alloc/op
-> bytes.HasPrefix(buf.Bytes(), []byte("xy")) 0.85 ns/op, 0 B/op,
0 allocs/op (~450x, allocation-free). strings.Contains/Index/Count show
the same 4864 B/op -> 0 B/op collapse (~5-7x on CPU); the saving grows
linearly with the buffered bytes.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5069",
		Doc:  "strings predicate over buf.String() against a constant copies the whole bytes.Buffer; the bytes twin over buf.Bytes() reads it with no copy",
		Run:  runPS5069,
	},
})

// ps5069Funcs are the strings functions whose bytes counterpart has the same
// name and takes the needle as a []byte (so buf.String() -> buf.Bytes() and
// the string constant -> []byte(constant) is a byte-identical rewrite).
var ps5069Funcs = map[string]bool{
	"HasPrefix": true,
	"HasSuffix": true,
	"Contains":  true,
	"EqualFold": true,
	"Index":     true,
	"LastIndex": true,
	"Count":     true,
}

func runPS5069(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		type site struct {
			diag analysis.Diagnostic
			fix  *analysis.SuggestedFix
		}
		var sites []site
		fixable := 0
		importAdded := false
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 2 || call.Ellipsis.IsValid() {
				return true
			}
			sel, isSel := call.Fun.(*ast.SelectorExpr)
			if !isSel {
				return true
			}
			fn, isFunc := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
			if !isFunc || fn.Pkg() == nil || fn.Pkg().Path() != "strings" || !ps5069Funcs[fn.Name()] {
				return true
			}
			if sig, isSig := fn.Type().(*types.Signature); !isSig || sig.Recv() != nil {
				return true
			}

			// First argument must be buf.String() over a bytes.Buffer.
			arg0Call, isCall := ps2108Unparen(call.Args[0]).(*ast.CallExpr)
			if !isCall {
				return true
			}
			_, recvText, nonNil, isBuf := ps2027BufferStringCall(pass, arg0Call)
			if !isBuf {
				return true
			}

			// Second argument must be a non-empty compile-time string constant.
			farTV, found := pass.TypesInfo.Types[call.Args[1]]
			if !found || farTV.Value == nil || farTV.Value.Kind() != constant.String ||
				constant.StringVal(farTV.Value) == "" {
				return true
			}
			farText, okFar := ps5004ExprText(call.Args[1])
			if !okFar {
				return true
			}

			name := fn.Name()
			after := "bytes." + name + "(" + recvText + ".Bytes(), []byte(" + farText + "))"
			msg := "strings." + name + " over buf.String() copies the whole bytes.Buffer just to test it against a constant; " +
				after + " reads the same bytes with no copy and no allocation"

			s := site{diag: analysis.Diagnostic{Pos: call.Pos(), End: call.End(), Message: msg}}
			switch {
			case !nonNil:
				// A *bytes.Buffer we cannot prove non-nil:
				// (*bytes.Buffer)(nil).String() is "<nil>" while Bytes panics.
				s.diag.Message = msg + `; the *bytes.Buffer receiver is not provably non-nil ((*bytes.Buffer)(nil).String() is "<nil>" while Bytes panics) — the automatic fix is withheld; rewrite by hand once the pointer is known non-nil`
			case ps2111CommentIn(f, call.Pos(), call.End()):
				// A comment sits inside the syntax the fix would replace.
				s.diag.Message = msg + "; a comment inside the rewritten call withholds the automatic fix — rewrite by hand"
			default:
				useName, needImport, usable := ps2107PkgUsable(pass, f, call.Pos(), "bytes", "bytes")
				if usable && !(needImport && ps2107ImportsC(f)) {
					edits := []analysis.TextEdit{
						// Rename strings.<Fn> -> <bytes>.<Fn>.
						{Pos: sel.Pos(), End: sel.End(), NewText: []byte(useName + "." + name)},
						// buf.String() -> buf.Bytes().
						{Pos: call.Args[0].Pos(), End: call.Args[0].End(), NewText: []byte(recvText + ".Bytes()")},
						// constant -> []byte(constant).
						{Pos: call.Args[1].Pos(), End: call.Args[1].Pos(), NewText: []byte("[]byte(")},
						{Pos: call.Args[1].End(), End: call.Args[1].End(), NewText: []byte(")")},
					}
					if needImport && !importAdded {
						edits = append(edits, ps2107ImportEdit(f, "bytes"))
						importAdded = true
					}
					// Recompute after with the file's actual bytes qualifier.
					s.diag.Message = "strings." + name + " over buf.String() copies the whole bytes.Buffer just to test it against a constant; " +
						useName + "." + name + "(" + recvText + ".Bytes(), []byte(" + farText + ")) reads the same bytes with no copy and no allocation"
					s.fix = &analysis.SuggestedFix{
						Message:   "read the raw bytes with " + useName + "." + name + " over buf.Bytes() instead of materializing the contents",
						TextEdits: edits,
					}
					fixable++
				} else {
					// bytes is not importable at this site.
					s.diag.Message = msg + "; this file has no usable import of package bytes at this position (missing under a blank/dot import, shadowed, or a cgo file whose import block cannot be edited) — the automatic fix is withheld; rewrite by hand"
				}
			}
			sites = append(sites, s)
			return true
		})

		// Withhold every fix in the file unless strings retains a use after the
		// rewrite drops these references — never orphan the strings import.
		emitFixes := fixable > 0 && pkgRefCount(pass, f, "strings") > fixable
		for _, s := range sites {
			d := s.diag
			if emitFixes && s.fix != nil {
				d.SuggestedFixes = []analysis.SuggestedFix{*s.fix}
			}
			pass.Report(d)
		}
	}
	return nil, nil
}
