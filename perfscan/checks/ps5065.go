package checks

import (
	"go/ast"
	"go/token"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS5065 reports hex.EncodeToString(a) < hex.EncodeToString(b) (and <=, >, >=) —
// hex-encoding two byte slices to throwaway strings only to ORDER them — where
// bytes.Compare(a, b) < 0 (etc.) orders the raw bytes directly, with no encoding
// pass and no allocation. The ordering sibling of PS5054 (which handles == / !=
// to bytes.Equal): hex encoding is order-preserving, so the string ordering is
// exactly the byte ordering.
var PS5065 = register(&lint.Check{
	ID:       "PS5065",
	Category: "alloc",
	Slug:     "hex-encode-order-to-bytes-compare",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "hex.EncodeToString(a) < hex.EncodeToString(b) encodes both slices just to order them; bytes.Compare(a, b) < 0 orders the raw bytes",
		Text: `hex.EncodeToString(a) < hex.EncodeToString(b) allocates a hex string for
each slice and compares the two lexicographically. Because hex encoding is
ORDER-PRESERVING — each byte maps to two lowercase hex digits whose ASCII order
matches the byte's value (00 < 01 < ... < 0a < ... < ff), and both strings run
two chars per byte — the string comparison yields exactly the byte comparison.
bytes.Compare(a, b) < 0 answers it directly: no encoding pass, no allocation, and
the length rule (a proper prefix is less) agrees because a proper byte-prefix
encodes to a proper hex-prefix.

The rewrite is BIT-IDENTICAL for <, <=, >, >=: hex.EncodeToString(a) OP
hex.EncodeToString(b) equals bytes.Compare(a, b) OP 0 for every pair — empty or
nil, one a prefix of the other, first difference anywhere — verified over
randomized slices across all four operators. Both operands are evaluated once.

The match is deliberately narrow — it is the whole safety story:
  - the comparison operator is <, <=, >, or >= (equality is PS5054's, to
    bytes.Equal);
  - BOTH operands are a single-argument call of the package-level encoding/hex
    EncodeToString (pinned by type information; a shadowed hex or the
    base64/base32 methods — whose encodings are NOT order-preserving relative to
    each other in the same alphabet edge cases — never match);
  - the byte-slice arguments carry over verbatim as bytes.Compare's arguments;
  - bytes must be importable, and because the rewrite drops these hex references
    the fix is withheld unless hex retains another use afterward (never orphaning
    encoding/hex — advisory then).
The fix rewrites to bytes.Compare(a, b) OP 0, keeping a and b byte-verbatim; a
comment inside a removed span withholds the fix.`,
		Before: `if hex.EncodeToString(a) < hex.EncodeToString(b) {`,
		After:  `if bytes.Compare(a, b) < 0 {`,
		MeasuredWin: `On two 32-byte slices (Apple M2 Pro, go1.26): hex.EncodeToString(a) < ` +
			`hex.EncodeToString(b) ~106 ns/op, 288 B/op, 4 allocs/op vs bytes.Compare(a, b) < 0 ` +
			`~2.4 ns/op, 0 B/op, 0 allocs/op (~44x) — two hex-string allocations and two encoding ` +
			`passes replaced by a direct byte comparison.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5065",
		Doc:  "hex.EncodeToString(a) < / <= / > / >= hex.EncodeToString(b) instead of bytes.Compare(a, b) OP 0",
		Run:  runPS5065,
	},
})

func runPS5065(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		type site struct {
			bin *ast.BinaryExpr
			fix *analysis.SuggestedFix
		}
		var sites []site
		fixable := 0
		importAdded := false
		ast.Inspect(f, func(n ast.Node) bool {
			bin, ok := n.(*ast.BinaryExpr)
			if !ok {
				return true
			}
			switch bin.Op {
			case token.LSS, token.LEQ, token.GTR, token.GEQ:
			default:
				return true
			}
			left, lok := ps5054HexCall(pass, bin.X)
			right, rok := ps5054HexCall(pass, bin.Y)
			if !lok || !rok {
				return true
			}
			a, b := left.Args[0], right.Args[0]

			var fix *analysis.SuggestedFix
			if !ps2109CommentBetween(f, bin.Pos(), a.Pos()) &&
				!ps2109CommentBetween(f, a.End(), b.Pos()) {
				useName, needImport, usable := ps2107PkgUsable(pass, f, bin.Pos(), "bytes", "bytes")
				if usable && !(needImport && ps2107ImportsC(f)) {
					edits := []analysis.TextEdit{
						// "hex.EncodeToString(" -> "bytes.Compare("
						{Pos: bin.Pos(), End: a.Pos(), NewText: []byte(useName + ".Compare(")},
						// ") OP hex.EncodeToString(" -> ", "; the right call's ")"
						// becomes bytes.Compare's.
						{Pos: a.End(), End: b.Pos(), NewText: []byte(", ")},
						// append the ordering against 0
						{Pos: bin.End(), End: bin.End(), NewText: []byte(" " + bin.Op.String() + " 0")},
					}
					if needImport && !importAdded {
						edits = append(edits, ps2107ImportEdit(f, "bytes"))
						importAdded = true
					}
					fix = &analysis.SuggestedFix{
						Message:   "order the bytes directly with " + useName + ".Compare(a, b) " + bin.Op.String() + " 0",
						TextEdits: edits,
					}
					fixable++
				}
			}
			sites = append(sites, site{bin, fix})
			return true
		})
		emitFixes := fixable > 0 && pkgRefCount(pass, f, "encoding/hex") > 2*fixable
		for _, st := range sites {
			diag := analysis.Diagnostic{
				Pos:     st.bin.Pos(),
				End:     st.bin.End(),
				Message: "hex.EncodeToString(a) " + st.bin.Op.String() + " hex.EncodeToString(b) hex-encodes two slices just to order them; bytes.Compare(a, b) " + st.bin.Op.String() + " 0 orders the raw bytes directly",
			}
			if emitFixes && st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}
