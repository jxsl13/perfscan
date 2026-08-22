package checks

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS5056 reports []byte(hex.EncodeToString(b)) — hex-encoding a byte slice into
// a throwaway STRING and then copying that string into a fresh []byte — where
// hex.AppendEncode([]byte{}, b) encodes the bytes directly into a []byte, with
// no intermediate string and one allocation instead of two. The []byte-result
// sibling of PS5054 (hex.EncodeToString compare) and the hex analog of PS2109
// ([]byte(fmt.Sprintf(...)) -> fmt.Appendf).
var PS5056 = register(&lint.Check{
	ID:       "PS5056",
	Category: "alloc",
	Slug:     "hex-encodetostring-conv-to-appendencode",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "[]byte(hex.EncodeToString(b)) allocates a hex STRING then copies it into a []byte; hex.AppendEncode encodes straight into the []byte",
		Text: `[]byte(hex.EncodeToString(b)) allocates a 2*len(b)-byte string, encodes
b into it, and then the []byte(...) conversion allocates a SECOND 2*len(b)-byte
buffer and copies the string across — two allocations for one result.
hex.AppendEncode([]byte{}, b) (go1.22) encodes b's hex digits straight into a
fresh []byte, so the intermediate string and the copy never exist.

The rewrite is BYTE-IDENTICAL, nil-ness included. hex.EncodeToString(b) and
hex.AppendEncode's digits are the same for every input, and the destination is
[]byte{} — a non-nil empty slice — precisely so an empty b matches: []byte("")
from the conversion is non-nil, and AppendEncode([]byte{}, empty) returns that
same non-nil empty slice (a nil destination would return nil and break code that
tests the result == nil). Verified equal, digits and nil-ness, over nil, empty,
and randomized inputs. b is evaluated exactly once in both forms.

The match is deliberately narrow — it is the whole safety story:
  - the outer expression is a conversion to the unnamed type []byte (a named
    byte-slice type would change the static type — not matched);
  - its operand is a single-argument call of the package-level encoding/hex
    EncodeToString, pinned by type information (a shadowed hex, or the
    base64/base32 EncodeToString METHODS, never match — reused from PS2043's
    callee check).
The fix drops the []byte(...) conversion, renames EncodeToString to AppendEncode
(an aliased hex import keeps its qualifier), and inserts the []byte{}
destination, keeping b verbatim; it touches no imports (hex stays imported). A
comment inside the removed conversion scaffolding withholds the fix.`,
		Before: `sum := []byte(hex.EncodeToString(b))`,
		After:  `sum := hex.AppendEncode([]byte{}, b)`,
		MeasuredWin: `On a 64-byte slice (Apple M2 Pro, go1.26): []byte(hex.EncodeToString(b)) ` +
			`~105 ns/op, 384 B/op, 3 allocs/op vs hex.AppendEncode([]byte{}, b) ~71 ns/op, ` +
			`128 B/op, 1 alloc/op (~1.5x, the throwaway hex string and its copy eliminated).`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5056",
		Doc:  "[]byte(hex.EncodeToString(b)) instead of hex.AppendEncode([]byte{}, b)",
		Run:  runPS5056,
	},
})

func runPS5056(pass *analysis.Pass) (any, error) {
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
			// Inner: hex.EncodeToString(b) — package-level encoding/hex.
			inner, ok := ps2109Unparen(outer.Args[0]).(*ast.CallExpr)
			if !ok || len(inner.Args) != 1 || inner.Ellipsis.IsValid() {
				return true
			}
			sel, ok := inner.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if kind, ok := ps2043Callee(pass, sel); !ok || kind != "hex" {
				return true
			}

			diag := analysis.Diagnostic{
				Pos:     outer.Pos(),
				End:     outer.End(),
				Message: "[]byte(hex.EncodeToString(b)) allocates a throwaway hex string and copies it into a []byte; hex.AppendEncode([]byte{}, b) encodes straight into the []byte",
			}
			// Drop the []byte(...) conversion, rename EncodeToString ->
			// AppendEncode, and insert the []byte{} destination. A comment inside
			// the removed conversion scaffolding withholds the fix.
			if !ps2109CommentBetween(f, outer.Pos(), inner.Pos()) &&
				!ps2109CommentBetween(f, inner.End(), outer.End()) {
				diag.SuggestedFixes = []analysis.SuggestedFix{{
					Message: "encode straight into a []byte with hex.AppendEncode([]byte{}, b)",
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
