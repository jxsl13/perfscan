package checks

import (
	"go/ast"
	"go/token"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS5054 reports hex.EncodeToString(a) == hex.EncodeToString(b) (and !=) —
// encoding two byte slices to throwaway hex strings only to compare them —
// where bytes.Equal(a, b) compares the bytes directly, with no encoding pass and
// no allocation. The byte-slice sibling of the compare-collapse family PS5048
// (Itoa), PS5051 (FormatInt) and PS5053 (Quote): hex encoding is injective, so
// equal hex strings appear exactly when the byte slices are equal.
var PS5054 = register(&lint.Check{
	ID:       "PS5054",
	Category: "alloc",
	Slug:     "hex-encode-compare-to-bytes-equal",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "hex.EncodeToString(a) == hex.EncodeToString(b) hex-encodes both slices just to compare them; bytes.Equal(a, b) compares the bytes directly",
		Text: `hex.EncodeToString(a) == hex.EncodeToString(b) allocates a
2*len-byte string for each slice, encodes every byte to two lowercase hex
digits, and then compares the two strings. Because hex encoding is injective —
each byte maps to a fixed two-character pair, so slices of equal length and
content produce identical hex and any difference in length or content produces
different hex — the encoded comparison answers exactly bytes.Equal(a, b), which
scans the bytes directly with no allocation and an early length check. The == and
!= forms both collapse: == becomes bytes.Equal(a, b), != becomes
!bytes.Equal(a, b).

The rewrite is BIT-IDENTICAL, verified over adversarial slices (nil, empty,
NUL bytes, differing lengths, invalid UTF-8) — hex.EncodeToString(a) ==
hex.EncodeToString(b) equals bytes.Equal(a, b) for every pair. Both operands are
evaluated exactly once, and nil and empty slices compare equal under both forms.

The match is deliberately narrow — it is the whole safety story:
  - an == or != comparison whose BOTH operands are a single-argument call of the
    package-level encoding/hex EncodeToString (a shadowed hex, or the
    EncodeToString METHOD of a base64/base32 Encoding — whose alphabet the two
    sides could differ in — never matches);
  - the byte-slice arguments carry over verbatim as bytes.Equal's arguments
    (they are already []byte, exactly bytes.Equal's parameters);
  - bytes must be importable at the site, and because the rewrite drops these
    hex references the fix is withheld unless hex retains another use afterward
    (never orphaning the encoding/hex import — that residual case is advisory).
The fix rewrites the comparison to bytes.Equal(a, b) (negated for !=), keeping
a and b byte-verbatim; it is withheld (advisory) on a comment inside a removed
span.`,
		Before: `if hex.EncodeToString(a) == hex.EncodeToString(b) {`,
		After:  `if bytes.Equal(a, b) {`,
		MeasuredWin: `On two 32-byte slices (Apple M2 Pro, go1.26): hex.EncodeToString(a) == ` +
			`hex.EncodeToString(b) ~103 ns/op, 288 B/op, 4 allocs/op vs bytes.Equal(a, b) ` +
			`~0.7 ns/op, 0 B/op, 0 allocs/op (~150x) — two 2*len-byte encoding allocations and ` +
			`two encoding passes replaced by a direct byte scan with an early length check.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5054",
		Doc:  "hex.EncodeToString(a) == / != hex.EncodeToString(b) instead of bytes.Equal(a, b)",
		Run:  runPS5054,
	},
})

func runPS5054(pass *analysis.Pass) (any, error) {
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
			if !ok || (bin.Op != token.EQL && bin.Op != token.NEQ) {
				return true
			}
			left, lok := ps5054HexCall(pass, bin.X)
			right, rok := ps5054HexCall(pass, bin.Y)
			if !lok || !rok {
				return true
			}
			a, b := left.Args[0], right.Args[0]

			var fix *analysis.SuggestedFix
			// The rewritten scaffolding — "hex.EncodeToString(" before a and
			// ") <op> hex.EncodeToString(" between a and b — must not swallow a
			// comment.
			if !ps2109CommentBetween(f, bin.Pos(), a.Pos()) &&
				!ps2109CommentBetween(f, a.End(), b.Pos()) {
				useName, needImport, usable := ps2107PkgUsable(pass, f, bin.Pos(), "bytes", "bytes")
				if usable && !(needImport && ps2107ImportsC(f)) {
					prefix := useName + ".Equal("
					if bin.Op == token.NEQ {
						prefix = "!" + prefix
					}
					edits := []analysis.TextEdit{
						// "hex.EncodeToString(" -> "bytes.Equal(" (or "!bytes.Equal(")
						{Pos: bin.Pos(), End: a.Pos(), NewText: []byte(prefix)},
						// ") <op> hex.EncodeToString(" -> ", "; the right call's
						// closing ")" becomes bytes.Equal's.
						{Pos: a.End(), End: b.Pos(), NewText: []byte(", ")},
					}
					if needImport && !importAdded {
						edits = append(edits, ps2107ImportEdit(f, "bytes"))
						importAdded = true
					}
					fix = &analysis.SuggestedFix{
						Message:   "compare the bytes directly with " + useName + ".Equal",
						TextEdits: edits,
					}
					fixable++
				}
			}
			sites = append(sites, site{bin, fix})
			return true
		})
		// Each fixable comparison removes TWO hex references; withhold all fixes
		// if that would orphan the encoding/hex import.
		emitFixes := fixable > 0 && pkgRefCount(pass, f, "encoding/hex") > 2*fixable
		for _, st := range sites {
			diag := analysis.Diagnostic{
				Pos:     st.bin.Pos(),
				End:     st.bin.End(),
				Message: "hex.EncodeToString(a) " + st.bin.Op.String() + " hex.EncodeToString(b) hex-encodes two slices just to compare them; bytes.Equal(a, b) compares the bytes directly",
			}
			if emitFixes && st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps5054HexCall reports whether e is a single-argument call of the package-level
// encoding/hex EncodeToString, returning the call. It reuses ps2043Callee (which
// pins the callee via type information) and accepts only the "hex" flavor — the
// base64/base32 EncodeToString methods are excluded because their alphabet could
// differ between the two operands.
func ps5054HexCall(pass *analysis.Pass, e ast.Expr) (*ast.CallExpr, bool) {
	call, ok := ps2109Unparen(e).(*ast.CallExpr)
	if !ok || len(call.Args) != 1 || call.Ellipsis.IsValid() {
		return nil, false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil, false
	}
	if kind, ok := ps2043Callee(pass, sel); !ok || kind != "hex" {
		return nil, false
	}
	return call, true
}
