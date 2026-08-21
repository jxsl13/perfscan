package checks

import (
	"go/ast"
	"go/token"
	"go/types"
	"strconv"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS5040 reports fmt.Appendf(buf, "%c", r) — a lone "%c" verb encoding one
// rune-valued integer into a byte buffer — where utf8.AppendRune(buf, r)
// appends the identical UTF-8 bytes without fmt's format parse or interface
// boxing. The "%c" analog of PS5039 (append(dst, string(r)...) ->
// utf8.AppendRune) carried to the fmt.Appendf destination, and a sibling of
// PS2035 (the "%v" scalar arm) and PS2141 (the lone "%s").
var PS5040 = register(&lint.Check{
	ID:       "PS5040",
	Category: "alloc",
	Slug:     "appendf-c-rune",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: `fmt.Appendf(buf, "%c", r) runs fmt's formatter to UTF-8-encode one rune; utf8.AppendRune(buf, r) encodes it straight into buf`,
		Text: `fmt.Appendf(buf, "%c", r) parses the format string, boxes r into an
interface and drives fmt's formatter through a pooled buffer — all to append
the UTF-8 encoding of one code point.
utf8.AppendRune(buf, r) runs the identical encoder directly into buf's
backing array: no format parse, no boxing.

The rewrite is BIT-IDENTICAL. fmt's "%c" verb formats an integer operand as
"the character represented by the corresponding Unicode code point": a value
in 0..0x10FFFF that is not a surrogate is UTF-8-encoded, and every other value
— negative, above utf8.MaxRune, or in the surrogate range 0xD800..0xDFFF —
becomes U+FFFD (0xEF 0xBF 0xBD). utf8.AppendRune(buf, r) applies exactly that
mapping. The two therefore append the same bytes for every value of r.

The match is deliberately narrow — it is the whole safety story:
  - the callee is pinned by type information to the package-level fmt.Appendf
    (a shadowed fmt or a method named Appendf never matches) and must not
    spread its arguments;
  - the format is a string literal that is EXACTLY "%c" — any width, flag, or
    other verb disqualifies it;
  - the operand is a NON-CONSTANT UNNAMED predeclared integer whose type is
    lossless in rune (int32/rune, int8, int16, uint8, uint16). Wider kinds
    (int, int64, uint, uint32, uint64, uintptr) are excluded: a value outside
    int32's range makes fmt emit U+FFFD while rune(r) would TRUNCATE to a
    different code point, so they can diverge. A NAMED integer type is excluded
    too — "%c" consults fmt.Formatter for every verb, so a named operand could
    hijack the verb; an unnamed predeclared type carries no methods. Constants
    are excluded (rune(const) could overflow int32 at compile time). The
    operand is rune-wrapped only when its type is not already assignable to
    rune (the int8/int16/uint8/uint16 widths), keeping rune(r) safe;
  - the destination is an unnamed []byte, so utf8.AppendRune's []byte result
    matches Appendf's exactly (a named byte-slice destination would change the
    expression's static type — advisory), and the fix is withheld unless the
    file keeps another fmt reference (so dropping this call never orphans the
    fmt import) and unicode/utf8 is importable (no cgo file, no shadow).
A comment inside the rewritten scaffolding keeps the report advisory. Named
destinations, wider or constant operands, and non-"%c" formats are reported
without a fix.`,
		Before: `buf = fmt.Appendf(buf, "%c", r)`,
		After:  `buf = utf8.AppendRune(buf, r)`,
		MeasuredWin: `fmt.Appendf(buf, "%c", r) ~24.7 ns/op vs utf8.AppendRune(buf, r) ` +
			`~1.8 ns/op (~14x, Apple M2 Pro, go1.26) — the win is the eliminated format ` +
			`parse and interface dispatch; both are 0 allocs when buf is reused, and the ` +
			`fmt path additionally boxes r (a heap allocation when the call escapes).`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5040",
		Doc:  "fmt.Appendf(buf, \"%c\", r) of a rune instead of utf8.AppendRune(buf, r)",
		Run:  runPS5040,
	},
})

func runPS5040(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		type site struct {
			call *ast.CallExpr
			fix  *analysis.SuggestedFix
		}
		var sites []site
		fixable := 0
		// All fixes of a run are applied together, so only the first fix
		// needing the unicode/utf8 import carries the import edit.
		importAdded := false
		astutil.WithStack(f, func(n ast.Node, stack []ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 3 || call.Ellipsis.IsValid() {
				return true
			}
			// Type info pins the callee to the package-level fmt.Appendf; a
			// shadowed fmt or a method named Appendf never matches.
			if _, ok := astutil.PkgFuncCall(pass.TypesInfo, call.Fun, "fmt", map[string]bool{"Appendf": true}); !ok {
				return true
			}
			lit, ok := call.Args[1].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			if v, err := strconv.Unquote(lit.Value); err != nil || v != "%c" {
				return true
			}
			needsWrap, ok := ps5040RuneArg(pass, call.Args[2])
			if !ok {
				return true
			}
			var fix *analysis.SuggestedFix
			// Destination guard: an unnamed []byte, so the rewrite's []byte
			// result matches Appendf's exactly. (An untyped nil destination
			// fails this too and stays advisory.)
			if ps2141ByteSliceUnnamed(pass.TypesInfo.TypeOf(call.Args[0])) &&
				!ps2111CommentIn(f, call.Args[0].End(), call.Args[2].Pos()) &&
				!ps2111CommentIn(f, call.Args[2].End(), call.End()) {
				useName, needImport, usable := ps2107PkgUsable(pass, f, call.Pos(), "utf8", "unicode/utf8")
				if usable && !(needImport && ps2107ImportsC(f)) {
					// PkgFuncCall matched, so call.Fun is a SelectorExpr.
					sel := call.Fun.(*ast.SelectorExpr)
					prefix, suffix := ", ", ")"
					if needsWrap {
						prefix, suffix = ", rune(", "))"
					}
					edits := []analysis.TextEdit{
						{Pos: sel.Pos(), End: sel.End(), NewText: []byte(useName + ".AppendRune")},
						{Pos: call.Args[0].End(), End: call.Args[2].Pos(), NewText: []byte(prefix)},
						{Pos: call.Args[2].End(), End: call.End(), NewText: []byte(suffix)},
					}
					if needImport && !importAdded {
						edits = append(edits, ps2107ImportEdit(f, "unicode/utf8"))
						importAdded = true
					}
					fix = &analysis.SuggestedFix{
						Message:   "replace fmt.Appendf(buf, \"%c\", r) with " + useName + ".AppendRune",
						TextEdits: edits,
					}
					fixable++
				}
			}
			sites = append(sites, site{call, fix})
			return true
		})
		// Withhold every fix when applying them would strip the file's last
		// fmt reference and orphan the import (the fix adds no import-drop).
		emitFixes := fixable > 0 && pkgRefCount(pass, f, "fmt") > fixable
		for _, st := range sites {
			diag := analysis.Diagnostic{
				Pos:     st.call.Pos(),
				End:     st.call.End(),
				Message: "fmt.Appendf(buf, \"%c\", r) parses the format and boxes r to UTF-8-encode one rune; utf8.AppendRune(buf, r) appends the identical bytes directly",
			}
			if emitFixes && st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps5040RuneArg reports whether arg is a non-constant integer operand whose
// type is lossless in rune (int32) — the whole width-safety story, shared in
// spirit with PS5039/PS2139's rune-conversion gate. needsWrap is true when the
// operand's type is not already assignable to rune, so the fix must wrap it as
// rune(arg); ok is false for constants, wider integer kinds, and non-integers.
func ps5040RuneArg(pass *analysis.Pass, arg ast.Expr) (needsWrap, ok bool) {
	for {
		p, isP := arg.(*ast.ParenExpr)
		if !isP {
			break
		}
		arg = p.X
	}
	tv, exists := pass.TypesInfo.Types[arg]
	if !exists || tv.Value != nil {
		// A constant operand: fmt still runs at format time, but the rune
		// conversion could overflow int32 at compile time, so stay out.
		return false, false
	}
	t := pass.TypesInfo.TypeOf(arg)
	if t == nil {
		return false, false
	}
	// The type must be an UNNAMED predeclared integer (types.Unalias yields a
	// *types.Basic directly, never a *types.Named). A named integer type is
	// excluded: unlike PS5039's string(x) language conversion, fmt's "%c"
	// consults fmt.Formatter for every verb, so a named operand implementing
	// Format would hijack the verb and diverge from utf8.AppendRune. An
	// unnamed predeclared type cannot carry methods, so no Formatter can arise.
	b, isB := types.Unalias(t).(*types.Basic)
	if !isB {
		return false, false
	}
	switch b.Kind() {
	case types.Int32, types.Int8, types.Int16, types.Uint8, types.Uint16:
		// Lossless in int32: rune(x) preserves the exact value, so fmt's "%c"
		// code-point interpretation and utf8.AppendRune agree on every value.
	default:
		return false, false
	}
	return !types.AssignableTo(t, types.Typ[types.Int32]), true
}
