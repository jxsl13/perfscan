package checks

import (
	"go/ast"
	"go/token"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS5058 reports enc.EncodeToString(a) == enc.EncodeToString(b) (and !=), where
// enc is the SAME encoding/base64 or encoding/base32 Encoding on both sides —
// encoding two byte slices to throwaway strings only to compare them — where
// bytes.Equal(a, b) compares the bytes directly, with no encoding pass and no
// allocation. The base64/base32 sibling of PS5054 (the encoding/hex form): a
// fixed encoder is injective, so equal encoded strings appear exactly when the
// byte slices are equal.
var PS5058 = register(&lint.Check{
	ID:       "PS5058",
	Category: "alloc",
	Slug:     "base-encode-compare-to-bytes-equal",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "enc.EncodeToString(a) == enc.EncodeToString(b) (base64/base32, same encoder) encodes both slices just to compare them; bytes.Equal(a, b) compares the bytes directly",
		Text: `enc.EncodeToString(a) == enc.EncodeToString(b), for one base64 or base32
Encoding enc, allocates an encoded string for each slice and compares the two
strings. A fixed Encoding is injective (Decode inverts it), so equal encoded
strings appear exactly when the inputs are equal: the comparison answers exactly
bytes.Equal(a, b), a direct byte scan with an early length check and no
allocation. The == and != forms both collapse: == becomes bytes.Equal(a, b), !=
becomes !bytes.Equal(a, b).

The rewrite is BIT-IDENTICAL, verified over randomized slices across Std/URL/Raw
base64 and base32. It requires the SAME encoder on BOTH sides: a std-vs-URL
base64 mix maps the byte 0xFB to "+/" versus "-_", so equal inputs could produce
unequal strings — the two receiver expressions must be textually identical and
side-effect-free, so they denote one encoder evaluated the same way. Both slice
operands are evaluated once.

The match is deliberately narrow — it is the whole safety story:
  - an == or != comparison whose BOTH operands are a single-argument call of the
    EncodeToString METHOD of an encoding/base64 or encoding/base32 *Encoding,
    pinned by type information to the exact Encoding type (reused from PS2043's
    callee check; the package-level encoding/hex form is PS5054's);
  - both calls use the SAME Encoding kind, and their receiver expressions are
    identical text and side-effect-free (an identifier or a package/field
    selector — never a call), so both name one encoder;
  - bytes must be importable, and because the rewrite drops these encoder
    references the fix is withheld unless the encoder's package retains another
    use afterward (never orphaning encoding/base64 or encoding/base32 — that
    residual case is advisory).
The fix rewrites the comparison to bytes.Equal(a, b) (negated for !=), keeping a
and b byte-verbatim; a comment inside a removed span withholds the fix.`,
		Before: `if base64.StdEncoding.EncodeToString(a) == base64.StdEncoding.EncodeToString(b) {`,
		After:  `if bytes.Equal(a, b) {`,
		MeasuredWin: `On two 32-byte slices, base64.StdEncoding (Apple M2 Pro, go1.26): ` +
			`enc.EncodeToString(a) == enc.EncodeToString(b) ~96 ns/op, 192 B/op, 4 allocs/op vs ` +
			`bytes.Equal(a, b) ~0.7 ns/op, 0 B/op, 0 allocs/op (~137x) — two encoding allocations ` +
			`and two encoding passes replaced by a direct byte scan.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5058",
		Doc:  "base64/base32 enc.EncodeToString(a) == / != enc.EncodeToString(b) instead of bytes.Equal(a, b)",
		Run:  runPS5058,
	},
})

var ps5058KindPath = map[string]string{
	"base64": "encoding/base64",
	"base32": "encoding/base32",
}

func runPS5058(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		type site struct {
			bin     *ast.BinaryExpr
			a, b    ast.Expr
			encPath string
			fixable bool // comment-clean and bytes importable
		}
		var sites []site
		fixableByPkg := map[string]int{}
		ast.Inspect(f, func(n ast.Node) bool {
			bin, ok := n.(*ast.BinaryExpr)
			if !ok || (bin.Op != token.EQL && bin.Op != token.NEQ) {
				return true
			}
			lcall, lsel, lkind, lok := ps5058EncCall(pass, bin.X)
			rcall, rsel, rkind, rok := ps5058EncCall(pass, bin.Y)
			if !lok || !rok || lkind != rkind {
				return true
			}
			// Same encoder: identical, side-effect-free receiver expressions.
			if !ps3021PureKey(lsel.X) || !ps3021PureKey(rsel.X) ||
				exprTextRendered(lsel.X) != exprTextRendered(rsel.X) {
				return true
			}
			a, b := lcall.Args[0], rcall.Args[0]
			s := site{bin: bin, a: a, b: b, encPath: ps5058KindPath[lkind]}
			// Comment-clean and bytes importable -> eligible for a fix.
			if !ps2109CommentBetween(f, bin.Pos(), a.Pos()) &&
				!ps2109CommentBetween(f, a.End(), b.Pos()) {
				_, needImport, usable := ps2107PkgUsable(pass, f, bin.Pos(), "bytes", "bytes")
				if usable && !(needImport && ps2107ImportsC(f)) {
					s.fixable = true
					fixableByPkg[s.encPath]++
				}
			}
			sites = append(sites, s)
			return true
		})

		importAdded := false
		for _, s := range sites {
			diag := analysis.Diagnostic{
				Pos:     s.bin.Pos(),
				End:     s.bin.End(),
				Message: "enc.EncodeToString(a) " + s.bin.Op.String() + " enc.EncodeToString(b) encodes two slices just to compare them; bytes.Equal(a, b) compares the bytes directly",
			}
			// Emit the fix only if it will not orphan the encoder's package:
			// each fixed comparison removes two encoder references.
			if s.fixable && pkgRefCount(pass, f, s.encPath) > 2*fixableByPkg[s.encPath] {
				useName, needImport, _ := ps2107PkgUsable(pass, f, s.bin.Pos(), "bytes", "bytes")
				prefix := useName + ".Equal("
				if s.bin.Op == token.NEQ {
					prefix = "!" + prefix
				}
				edits := []analysis.TextEdit{
					{Pos: s.bin.Pos(), End: s.a.Pos(), NewText: []byte(prefix)},
					{Pos: s.a.End(), End: s.b.Pos(), NewText: []byte(", ")},
				}
				if needImport && !importAdded {
					edits = append(edits, ps2107ImportEdit(f, "bytes"))
					importAdded = true
				}
				diag.SuggestedFixes = []analysis.SuggestedFix{{
					Message:   "compare the bytes directly with " + useName + ".Equal",
					TextEdits: edits,
				}}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps5058EncCall reports whether e is a single-argument call of the EncodeToString
// method of an encoding/base64 or encoding/base32 *Encoding, returning the call,
// its selector, and the encoder kind ("base64" or "base32").
func ps5058EncCall(pass *analysis.Pass, e ast.Expr) (call *ast.CallExpr, sel *ast.SelectorExpr, kind string, ok bool) {
	c, isCall := ps2109Unparen(e).(*ast.CallExpr)
	if !isCall || len(c.Args) != 1 || c.Ellipsis.IsValid() {
		return nil, nil, "", false
	}
	s, isSel := c.Fun.(*ast.SelectorExpr)
	if !isSel {
		return nil, nil, "", false
	}
	k, matched := ps2043Callee(pass, s)
	if !matched || (k != "base64" && k != "base32") {
		return nil, nil, "", false
	}
	return c, s, k, true
}
