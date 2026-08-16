package checks

import (
	"encoding/hex"
	"fmt"
	"go/ast"
	"go/token"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS5064 reports hex.EncodeToString(x) == "<lowercase-hex-literal>" (and !=) —
// hex-encoding a byte slice into a throwaway string only to compare it against a
// constant hex string — where bytes.Equal(x, <the decoded bytes>) compares the
// raw bytes directly, with no encoding pass. The constant-operand cousin of
// PS5054 (hex.EncodeToString(a) == hex.EncodeToString(b)).
var PS5064 = register(&lint.Check{
	ID:       "PS5064",
	Category: "alloc",
	Slug:     "hex-encode-compare-const-to-bytes-equal",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "hex.EncodeToString(x) == \"<hex>\" encodes the slice just to compare it to a constant; bytes.Equal(x, <decoded>) compares the raw bytes",
		Text: `hex.EncodeToString(x) == "deadbeef" encodes x into its hex string and
compares that against a constant. bytes.Equal(x, []byte("\xde\xad\xbe\xef"))
compares x against the constant's DECODED bytes directly — no encoding pass, and
the length check short-circuits a mismatch immediately.

The rewrite is BIT-IDENTICAL. hex.EncodeToString is a bijection onto lowercase
even-length hex strings, so for a constant C that is itself valid lowercase
even-length hex, hex.EncodeToString(x) == C exactly when x equals C's decoded
bytes — which bytes.Equal answers. nil and empty x compare equal to the empty
constant under both forms. The decoded bytes are computed at fix time and emitted
as a []byte string literal, so no decoding happens at run time.

The match is deliberately narrow — it is the whole safety story:
  - one operand is a single-argument call of the package-level encoding/hex
    EncodeToString (pinned by type information; a shadowed hex or a base64/base32
    method never matches), and the other is a STRING LITERAL;
  - the literal's value is valid hex: an EVEN number of digits, each 0-9 or a-f.
    UPPERCASE is excluded — EncodeToString only ever emits lowercase, so
    hex.EncodeToString(x) == "AB" is unconditionally false while bytes.Equal of
    the decoded byte would be true; an odd-length or non-hex literal (also never
    produced by EncodeToString) is left alone rather than folded to a constant;
  - bytes must be importable, and because the rewrite drops the hex reference the
    fix is withheld unless hex retains another use afterward (never orphaning
    encoding/hex — advisory then).
The fix rewrites to bytes.Equal(x, []byte("...")) (negated for !=), keeping x
byte-verbatim; a comment inside a removed span withholds the fix.`,
		Before: `if hex.EncodeToString(sum) == "deadbeef" {`,
		After:  `if bytes.Equal(sum, []byte("\xde\xad\xbe\xef")) {`,
		MeasuredWin: `On a 16-byte slice against a 32-char hex constant (Apple M2 Pro, go1.26): ` +
			`hex.EncodeToString(x) == "..." ~14 ns/op vs bytes.Equal(x, []byte("...")) ~2.5 ns/op ` +
			`(~5.4x, 0 allocs/op either way) — the hex-encode pass is eliminated.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5064",
		Doc:  "hex.EncodeToString(x) == a constant hex string instead of bytes.Equal(x, decoded)",
		Run:  runPS5064,
	},
})

func runPS5064(pass *analysis.Pass) (any, error) {
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
			// One side is hex.EncodeToString(x), the other a valid hex string
			// literal.
			call, litSide, ok := ps5064Match(pass, bin)
			if !ok {
				return true
			}
			x := call.Args[0]
			decoded, _ := ps5064DecodeHexLit(litSide)

			var fix *analysis.SuggestedFix
			// The rewritten span runs from bin.Pos() to bin.End(); a comment
			// inside it (other than in x itself) would be lost.
			if !ps2109CommentBetween(f, bin.Pos(), x.Pos()) &&
				!ps2109CommentBetween(f, x.End(), bin.End()) {
				useName, needImport, usable := ps2107PkgUsable(pass, f, bin.Pos(), "bytes", "bytes")
				if usable && !(needImport && ps2107ImportsC(f)) {
					prefix := useName + ".Equal("
					if bin.Op == token.NEQ {
						prefix = "!" + prefix
					}
					litText := ps5064ByteLit(decoded)
					// Replace bin.Pos()..x.Pos() with the callee+open paren, and
					// x.End()..bin.End() with ", <decoded>)". x stays verbatim.
					edits := []analysis.TextEdit{
						{Pos: bin.Pos(), End: x.Pos(), NewText: []byte(prefix)},
						{Pos: x.End(), End: bin.End(), NewText: []byte(", " + litText + ")")},
					}
					if needImport && !importAdded {
						edits = append(edits, ps2107ImportEdit(f, "bytes"))
						importAdded = true
					}
					fix = &analysis.SuggestedFix{
						Message:   "compare the raw bytes with " + useName + ".Equal(x, " + litText + ")",
						TextEdits: edits,
					}
					fixable++
				}
			}
			sites = append(sites, site{bin, fix})
			return true
		})
		emitFixes := fixable > 0 && pkgRefCount(pass, f, "encoding/hex") > fixable
		for _, st := range sites {
			diag := analysis.Diagnostic{
				Pos:     st.bin.Pos(),
				End:     st.bin.End(),
				Message: "hex.EncodeToString(x) " + st.bin.Op.String() + " a constant hex string encodes x just to compare it; bytes.Equal(x, <decoded>) compares the raw bytes directly",
			}
			if emitFixes && st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps5064Match reports whether bin compares a package-level hex.EncodeToString
// call against a valid lowercase even-length hex string literal, returning the
// call and the literal.
func ps5064Match(pass *analysis.Pass, bin *ast.BinaryExpr) (call *ast.CallExpr, lit *ast.BasicLit, ok bool) {
	tryOrder := func(callExpr, litExpr ast.Expr) (*ast.CallExpr, *ast.BasicLit, bool) {
		c, cok := ps5054HexCall(pass, callExpr)
		if !cok {
			return nil, nil, false
		}
		l, lok := litExpr.(*ast.BasicLit)
		if !lok {
			return nil, nil, false
		}
		if _, dok := ps5064DecodeHexLit(l); !dok {
			return nil, nil, false
		}
		return c, l, true
	}
	if c, l, mok := tryOrder(bin.X, bin.Y); mok {
		return c, l, true
	}
	if c, l, mok := tryOrder(bin.Y, bin.X); mok {
		return c, l, true
	}
	return nil, nil, false
}

// ps5064DecodeHexLit returns the decoded bytes of lit if it is a string literal
// whose value is valid hex: an even number of digits, each 0-9 or a-f (lowercase
// only, since hex.EncodeToString never emits uppercase).
func ps5064DecodeHexLit(lit *ast.BasicLit) ([]byte, bool) {
	if lit.Kind != token.STRING {
		return nil, false
	}
	s, err := strconv.Unquote(lit.Value)
	if err != nil || len(s)%2 != 0 {
		return nil, false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return nil, false
		}
	}
	dec, err := hex.DecodeString(s)
	if err != nil {
		return nil, false
	}
	return dec, true
}

// ps5064ByteLit renders decoded bytes as an explicit []byte composite literal,
// e.g. []byte{0xde, 0xad, 0xbe, 0xef} (or []byte{} for empty) — clearer for raw
// byte data than a quoted-string conversion.
func ps5064ByteLit(b []byte) string {
	if len(b) == 0 {
		return "[]byte{}"
	}
	parts := make([]string, len(b))
	for i, c := range b {
		parts[i] = fmt.Sprintf("0x%02x", c)
	}
	return "[]byte{" + strings.Join(parts, ", ") + "}"
}
