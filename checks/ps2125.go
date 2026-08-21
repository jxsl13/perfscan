package checks

import (
	"go/ast"
	"go/printer"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS2125 reports len of a throwaway string conversion: len([]rune(s))
// allocates and decodes the ENTIRE rune slice just to count it, where
// utf8.RuneCountInString(s) computes the identical integer in one
// allocation-free pass; len([]byte(s)) allocates a byte copy of the
// string just to take its length, which is len(s) directly.
//
// PS2125 is a perfscan-original check (the x1xx block per category is
// reserved for checks that did not originate in the upstream reference
// registry).
var PS2125 = register(&lint.Check{
	ID:       "PS2125",
	Category: "alloc",
	Slug:     "len-of-conversion",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "len([]rune(s)) / len([]byte(s)) take a length through a throwaway conversion; count the string directly",
		Text: `[]rune(s) decodes every rune of s into a rune slice; taking only
its len discards that slice immediately. utf8.RuneCountInString(s)
counts the runes in a single pass with zero allocation. The two are
bit-identical for EVERY input, including invalid UTF-8: the []rune
conversion yields one U+FFFD replacement rune per invalid byte, and
utf8.RuneCountInString likewise counts one rune per invalid byte, so the
integers agree exactly on all strings.

[]byte(s) copies the string's bytes; its len is by definition the byte
length of s, so len(s) is the identical integer with no conversion at
all.

An honest caveat: cmd/compile already special-cases EXACTLY these two
compositions — len([]rune(s)) is lowered to an allocation-free
runtime.countrunes call and len([]byte(s)) to len(s) — so on current gc
the measured difference is nil (see below). The rewrite moves that
guarantee from a compiler special case into the source: it survives
compilers and build modes without the peephole (gccgo, tinygo, older
toolchains), keeps holding when the expression is later refactored out
of the exact len(conversion) shape the peephole matches (a stored
[]rune(s) allocates for real), and states the intent — "count runes" /
"byte length" — in the standard vocabulary.

Both forms are matched only when they are provably the builtin len over
a genuine conversion of a plain string:

  - the outer callee must be the predeclared len (a shadowed len is
    rejected via type information);
  - the argument must be DIRECTLY the conversion []rune(x) / []byte(x)
    — the callee is a slice type expression whose element is the
    predeclared rune/byte (int32/uint8 spell the identical types), so a
    named slice type or a function call that merely looks like a
    conversion never matches; a conversion result stored in a variable
    first may have other consumers and is out of scope;
  - the operand x must have type EXACTLY the predeclared string (an
    untyped string constant defaults to string). A named string type is
    skipped deliberately: requiring exactly string keeps the rewrite
    trivially bit-identical and free of surprises.

The fix keeps x verbatim in place — same text, same single evaluation —
and edits only the scaffolding. len([]rune(x)) becomes
utf8.RuneCountInString(x): a call is a primary expression, so no outer
parentheses are ever needed; the unicode/utf8 import is added when
missing (reusing an existing import's alias when present), and the
vanishing []rune spelling references only predeclared identifiers, so no
import can be orphaned. len([]byte(x)) becomes len(x): only the
conversion around x is dropped, so no import changes at all. A constant
byte-form operand remains advisory because len([]byte("x")) is not a constant
while len("x") is; changing that compile-time property can create a duplicate
switch case. A comment inside the rewritten scaffolding likewise downgrades
the fix to an advisory report.`,
		Before: `n := len([]rune(name))
m := len([]byte(name))`,
		After: `n := utf8.RuneCountInString(name)
m := len(name)`,
		MeasuredWin: `BenchmarkPS2125 (a ~1.1KB mixed-width line of 896 runes,
counted once per op, Apple M2 Pro, gc 1.26): len([]rune(s)) 778 ns/op,
0 B/op vs utf8.RuneCountInString(s) 779 ns/op, 0 B/op — identical,
because cmd/compile lowers this exact composition to runtime.countrunes;
len([]byte(s)) 0.29 ns/op vs len(s) 0.29 ns/op — the peephole reduces
both to a length read. The win on current gc is source-level (intent and
robustness: a []rune(s) that drifts out of the peephole's exact shape
allocates 4096 B/op for this line); on toolchains without the peephole
the rewrite removes a real O(len) allocation per evaluation.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2125",
		Doc:  "len over a throwaway []rune/[]byte conversion of a string; count or measure it directly instead",
		Run:  runPS2125,
	},
})

func runPS2125(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		// All fixes of a run are applied together, so only the first fix
		// needing the unicode/utf8 import carries the import edit.
		importAdded := false
		ast.Inspect(f, func(n ast.Node) bool {
			lenCall, ok := n.(*ast.CallExpr)
			if !ok || len(lenCall.Args) != 1 || lenCall.Ellipsis.IsValid() {
				return true
			}
			// The outer callee must be the predeclared builtin len — a
			// shadowed len resolves to some other object and is rejected.
			id, ok := lenCall.Fun.(*ast.Ident)
			if !ok || id.Name != "len" {
				return true
			}
			if pass.TypesInfo.Uses[id] != types.Universe.Lookup("len") {
				return true
			}
			// The argument must be DIRECTLY the conversion: a result
			// stored in a variable first may have other consumers.
			inner, isRune := ps2125Conversion(pass, lenCall.Args[0])
			if inner == nil {
				return true
			}
			// The operand must be EXACTLY the predeclared string; an
			// untyped string constant defaults to string. Named string
			// types are skipped deliberately (see the Doc).
			x := inner.Args[0]
			xt := pass.TypesInfo.TypeOf(x)
			if xt == nil || !types.Identical(types.Default(xt), types.Typ[types.String]) {
				return true
			}
			convText := ps2125ExprText(inner)
			xText := ps2125ExprText(x)
			if isRune {
				ps2125ReportRune(pass, f, lenCall, inner, convText, xText, &importAdded)
			} else {
				ps2125ReportByte(pass, f, lenCall, inner, convText, xText)
			}
			return true
		})
	}
	return nil, nil
}

// ps2125ReportRune reports len([]rune(x)) and, when possible, attaches
// the utf8.RuneCountInString(x) rewrite. x stays verbatim in place; the
// replacement is a call — a primary expression — so no outer parentheses
// are ever needed. The unicode/utf8 import is added when missing.
func ps2125ReportRune(pass *analysis.Pass, f *ast.File, lenCall, inner *ast.CallExpr, convText, xText string, importAdded *bool) {
	x := inner.Args[0]
	diag := analysis.Diagnostic{
		Pos:     lenCall.Pos(),
		End:     lenCall.End(),
		Message: "len(" + convText + ") spells a rune count as a throwaway []rune conversion; utf8.RuneCountInString(" + xText + ") is the direct, bit-identical count (allocation-free on every toolchain)",
	}
	// A comment inside the rewritten scaffolding would be destroyed —
	// the fix is withheld then and the report stays advisory.
	if fix := ps2125RuneFix(pass, f, lenCall, x, importAdded); fix != nil {
		diag.SuggestedFixes = []analysis.SuggestedFix{*fix}
	}
	pass.Report(diag)
}

// ps2125RuneFix builds the rewrite to <utf8>.RuneCountInString(x), or
// returns nil when any guard fails and the report must stay advisory.
func ps2125RuneFix(pass *analysis.Pass, f *ast.File, lenCall *ast.CallExpr, x ast.Expr, importAdded *bool) *analysis.SuggestedFix {
	if ps2111CommentIn(f, lenCall.Pos(), x.Pos()) || ps2111CommentIn(f, x.End(), lenCall.End()) {
		return nil
	}
	// Reuse the file's existing unicode/utf8 import name (possibly an
	// alias); dot and blank imports leave no usable qualifier.
	name, ok := ps2125Utf8Name(f)
	if !ok {
		return nil
	}
	// The name must actually reference unicode/utf8 at the call site —
	// a shadowing local would capture the qualifier.
	needImport, usable := ps2110PkgUsable(pass, lenCall.Pos(), name, "unicode/utf8")
	if !usable {
		return nil
	}
	if needImport && ps2110ImportsC(f) {
		return nil
	}
	// x's text is untouched in place: same expression, same single
	// evaluation. Only the surrounding scaffolding changes.
	edits := []analysis.TextEdit{
		{Pos: lenCall.Pos(), End: x.Pos(), NewText: []byte(name + ".RuneCountInString(")},
		{Pos: x.End(), End: lenCall.End(), NewText: []byte(")")},
	}
	if needImport && !*importAdded {
		edits = append(edits, ps2110ImportEdit(f, "unicode/utf8"))
		*importAdded = true
	}
	return &analysis.SuggestedFix{
		Message:   "replace with " + name + ".RuneCountInString(...)",
		TextEdits: edits,
	}
}

// ps2125ReportByte reports len([]byte(x)) and, when possible, attaches
// the trivial rewrite dropping the conversion: only []byte( and its
// closing parenthesis vanish, the outer len(...) and x stay in place, so
// no import is ever touched.
func ps2125ReportByte(pass *analysis.Pass, f *ast.File, lenCall, inner *ast.CallExpr, convText, xText string) {
	x := inner.Args[0]
	diag := analysis.Diagnostic{
		Pos:     lenCall.Pos(),
		End:     lenCall.End(),
		Message: "len(" + convText + ") spells a byte length as a throwaway []byte conversion; len(" + xText + ") is the direct, bit-identical byte count (allocation-free on every toolchain)",
	}
	// Do not turn a runtime len(slice-conversion) into a compile-time string
	// length constant: that can make an existing switch fail with duplicate
	// cases even though the integer value is unchanged.
	typedValue := pass.TypesInfo.Types[x]
	if typedValue.Value == nil && !ps2111CommentIn(f, inner.Pos(), x.Pos()) && !ps2111CommentIn(f, x.End(), inner.End()) {
		diag.SuggestedFixes = []analysis.SuggestedFix{{
			Message: "drop the []byte conversion",
			TextEdits: []analysis.TextEdit{
				{Pos: inner.Pos(), End: x.Pos(), NewText: nil},
				{Pos: x.End(), End: inner.End(), NewText: nil},
			},
		}}
	}
	pass.Report(diag)
}

// ps2125Conversion matches e as the direct conversion []rune(x) (isRune
// true) or []byte(x) (isRune false) with exactly one operand and no
// spread. The callee must be a slice type expression — confirmed as a
// type via go/types, which rejects function calls that merely look like
// conversions — whose element is spelled with a predeclared identifier
// (rune/int32 and byte/uint8 are the identical types); a named slice
// type or a package-qualified element never matches, so the vanishing
// spelling can never orphan an import.
func ps2125Conversion(pass *analysis.Pass, e ast.Expr) (conv *ast.CallExpr, isRune bool) {
	call, ok := e.(*ast.CallExpr)
	if !ok || len(call.Args) != 1 || call.Ellipsis.IsValid() {
		return nil, false
	}
	at, ok := call.Fun.(*ast.ArrayType)
	if !ok || at.Len != nil {
		return nil, false
	}
	elt, ok := at.Elt.(*ast.Ident)
	if !ok {
		return nil, false
	}
	if obj := pass.TypesInfo.Uses[elt]; obj == nil || obj != types.Universe.Lookup(elt.Name) {
		return nil, false
	}
	tv, ok := pass.TypesInfo.Types[call.Fun]
	if !ok || !tv.IsType() {
		return nil, false
	}
	sl, ok := tv.Type.(*types.Slice)
	if !ok {
		return nil, false
	}
	eb, ok := sl.Elem().(*types.Basic)
	if !ok {
		return nil, false
	}
	switch eb.Kind() {
	case types.Int32:
		return call, true
	case types.Uint8:
		return call, false
	}
	return nil, false
}

// ps2125Utf8Name returns the qualifier that references unicode/utf8 in
// f: an existing import's local name (alias-aware) or the default utf8
// when the file does not import it yet. Dot and blank imports leave no
// usable qualifier — ok is false then.
func ps2125Utf8Name(f *ast.File) (name string, ok bool) {
	for _, imp := range f.Imports {
		if imp.Path.Value != `"unicode/utf8"` {
			continue
		}
		if imp.Name == nil {
			return "utf8", true
		}
		if imp.Name.Name == "." || imp.Name.Name == "_" {
			return "", false
		}
		return imp.Name.Name, true
	}
	return "utf8", true
}

// ps2125ExprText renders e for use in diagnostic messages.
func ps2125ExprText(e ast.Expr) string {
	var b strings.Builder
	if err := printer.Fprint(&b, token.NewFileSet(), e); err != nil {
		return "..."
	}
	return b.String()
}
