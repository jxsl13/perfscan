package checks

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS5057 reports []byte(enc.EncodeToString(b)) where enc is an encoding/base64
// or encoding/base32 *Encoding — encoding a byte slice into a throwaway STRING
// and then copying that string into a fresh []byte — where
// enc.AppendEncode([]byte{}, b) (go1.22) encodes the bytes directly into a
// []byte, with no intermediate string and one allocation instead of two. The
// base64/base32 sibling of PS5056 (the encoding/hex form).
var PS5057 = register(&lint.Check{
	ID:       "PS5057",
	Category: "alloc",
	Slug:     "base-encodetostring-conv-to-appendencode",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "[]byte(enc.EncodeToString(b)) (base64/base32) allocates an encoded STRING then copies it into a []byte; enc.AppendEncode encodes straight into the []byte",
		Text: `[]byte(enc.EncodeToString(b)), for a base64 or base32 Encoding enc,
allocates an EncodedLen(len(b))-byte string, encodes b into it, and then the
[]byte(...) conversion allocates a SECOND buffer and copies the string across —
two allocations for one result. enc.AppendEncode([]byte{}, b) (go1.22) encodes
b's digits straight into a fresh []byte, so the intermediate string and the copy
never exist.

The rewrite is BYTE-IDENTICAL, nil-ness included. EncodeToString and AppendEncode
produce the same encoding (same alphabet and padding — the encoder receiver is
kept verbatim, so both use the identical enc), and the destination is []byte{} —
a non-nil empty slice — precisely so an empty b matches: []byte("") from the
conversion is non-nil, and AppendEncode([]byte{}, empty) returns that same
non-nil empty slice (a nil destination would return nil and break code that tests
the result == nil). Verified equal, digits and nil-ness, across Std/URL/Raw
base64 and Std/Hex base32 over nil, empty, and randomized inputs. b is evaluated
once, and the encoder receiver is evaluated once (kept verbatim).

The match is deliberately narrow — it is the whole safety story:
  - the outer expression is a conversion to the unnamed type []byte (a named
    byte-slice type would change the static type — not matched);
  - its operand is a single-argument call of the EncodeToString METHOD of an
    encoding/base64 or encoding/base32 *Encoding, pinned by type information to
    the exact Encoding type (a wrapper embedding *Encoding, whose own EncodedLen
    could shadow the promoted one, is rejected — reused from PS2043's callee
    check; the package-level encoding/hex form is PS5056's).
The fix drops the []byte(...) conversion, renames EncodeToString to AppendEncode,
and inserts the []byte{} destination, keeping the encoder receiver and b verbatim;
it touches no imports. A comment inside the removed conversion scaffolding
withholds the fix.`,
		Before: `token := []byte(base64.StdEncoding.EncodeToString(b))`,
		After:  `token := base64.StdEncoding.AppendEncode([]byte{}, b)`,
		MeasuredWin: `On a 48-byte slice, base64.StdEncoding (Apple M2 Pro, go1.26): ` +
			`[]byte(enc.EncodeToString(b)) ~73 ns/op, 192 B/op, 3 allocs/op vs ` +
			`enc.AppendEncode([]byte{}, b) ~48 ns/op, 64 B/op, 1 alloc/op (~1.5x, the throwaway ` +
			`encoded string and its copy eliminated).`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5057",
		Doc:  "[]byte(base64/base32 enc.EncodeToString(b)) instead of enc.AppendEncode([]byte{}, b)",
		Run:  runPS5057,
	},
})

func runPS5057(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			// Outer: []byte(<one arg>) conversion to the unnamed []byte type.
			outer, ok := n.(*ast.CallExpr)
			if !ok || len(outer.Args) != 1 || outer.Ellipsis.IsValid() {
				return true
			}
			if !ps2109IsByteSliceConv(pass, outer) {
				return true
			}
			// Inner: enc.EncodeToString(b) — a base64/base32 Encoding method.
			inner, ok := ps2109Unparen(outer.Args[0]).(*ast.CallExpr)
			if !ok || len(inner.Args) != 1 || inner.Ellipsis.IsValid() {
				return true
			}
			sel, ok := inner.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			kind, ok := ps2043Callee(pass, sel)
			if !ok || (kind != "base64" && kind != "base32") {
				return true
			}

			diag := analysis.Diagnostic{
				Pos:     outer.Pos(),
				End:     outer.End(),
				Message: "[]byte(enc.EncodeToString(b)) allocates a throwaway encoded string and copies it into a []byte; enc.AppendEncode([]byte{}, b) encodes straight into the []byte",
			}
			// Drop the []byte(...) conversion, rename EncodeToString ->
			// AppendEncode, and insert the []byte{} destination. A comment inside
			// the removed conversion scaffolding withholds the fix.
			if !ps2109CommentBetween(f, outer.Pos(), inner.Pos()) &&
				!ps2109CommentBetween(f, inner.End(), outer.End()) {
				diag.SuggestedFixes = []analysis.SuggestedFix{{
					Message: "encode straight into a []byte with enc.AppendEncode([]byte{}, b)",
					TextEdits: []analysis.TextEdit{
						{Pos: outer.Pos(), End: inner.Pos()},                                          // drop "[]byte("
						{Pos: sel.Sel.Pos(), End: sel.Sel.End(), NewText: []byte("AppendEncode")},     // rename
						{Pos: inner.Lparen + 1, End: inner.Lparen + 1, NewText: []byte("[]byte{}, ")}, // insert dst
						{Pos: inner.End(), End: outer.End()},                                          // drop conversion's ")"
					},
				}}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}
