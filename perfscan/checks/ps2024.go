package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS2024 reports utf8.RuneCount([]byte(s)) with s a plain string — a
// full byte-by-byte copy of s made purely to hand RuneCount a []byte —
// and rewrites it to utf8.RuneCountInString(s), which scans the string's
// bytes in place. The symmetric shape utf8.RuneCountInString(string(b))
// with b a plain []byte allocates a throwaway string copy of b and is
// rewritten to utf8.RuneCount(b). This is the sibling-call twin of
// PS2125 (len([]rune(s)) -> utf8.RuneCountInString(s)): same count, the
// entire saving is data movement.
var PS2024 = register(&lint.Check{
	ID:       "PS2024",
	Category: "alloc",
	Slug:     "runecount-throwaway-conversion",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "utf8.RuneCount([]byte(s)) copies the whole string just to count runes; utf8.RuneCountInString(s) counts it in place",
		Text: `[]byte(s) forces a full byte-by-byte copy of s — a heap
allocation once the string exceeds the compiler's small stack-temporary
budget (~32 bytes), or whenever escape analysis cannot prove the slice
does not escape — purely to hand utf8.RuneCount a []byte.
utf8.RuneCountInString(s) scans the string's bytes directly: zero copy,
zero allocation, same decode loop. The count itself is identical work in
both spellings; the entire saving is data movement. The symmetric
reverse holds too: utf8.RuneCountInString(string(b)) with b a []byte
allocates a string copy of b and becomes utf8.RuneCount(b) with the
same win.

An honest nuance on current gc (verified on Go 1.26): RuneCount's own
non-ASCII slow path re-materializes string(p[n:]) — a general call
argument sits outside the compiler's zero-copy conversion contexts — so
RuneCount itself pays a heap copy of the tail from the FIRST non-ASCII
byte once that tail exceeds the 32-byte stack temporary. For the string
direction this only strengthens the case: the Before pays the explicit
[]byte(s) copy and the After none. For the mirror it means the
allocation win is realized from the first non-ASCII byte position —
zero allocations on pure-ASCII and ASCII-dominated inputs (the common
case, where the byte-wise ASCII fast path is also ~2x faster than the
rune-decoding loop), a tail copy strictly smaller than the Before's
full copy in between, and parity, never a regression, when the input
starts with a non-ASCII byte (the tail IS the whole input).

The rewrite is bit-identical on EVERY input, including empty and invalid
UTF-8: unicode/utf8's RuneCount and RuneCountInString share one counting
core — the standard library implements RuneCount's non-ASCII path
directly in terms of RuneCountInString over the remaining bytes, and
both count exactly one rune per byte of any invalid sequence — and
[]byte(s) has byte content identical to s (resp. string(b) to b, with
string(nil []byte) == ""), so the integer result agrees on all inputs. Both functions are pure; the single operand is
evaluated exactly once in both forms, so side effects are preserved in
count and order. The replacement is itself a call — a primary
expression — so it is legal in every context the original occupies
(including bare statement, go, and defer) and never needs parentheses.

The automatic fix applies only when type information proves the shape:

  - the callee resolves to the standard library's unicode/utf8
    RuneCount (resp. RuneCountInString) — through any import alias or a
    dot import; a shadowed identifier or a same-named local function
    never matches;
  - the argument is DIRECTLY the conversion []byte(x) (spelled with the
    predeclared byte or uint8 — a named or package-qualified slice type
    never matches, so the vanishing spelling can never orphan an
    import) whose operand x has type EXACTLY the predeclared string
    (an untyped string constant defaults to string) — resp. DIRECTLY
    string(b) with the predeclared string and b exactly the predeclared
    []byte. A NAMED string or byte-slice operand is skipped entirely:
    the sibling call would need a kept no-op conversion, so the
    conversion would not truly vanish;
  - a conversion result stored in a variable first may have other
    consumers and is out of scope.

The fix keeps the operand verbatim in place — same text, same single
evaluation — renames the callee to its sibling, and deletes only the
conversion scaffolding. The unicode/utf8 import stays referenced by the
sibling call, so no import is ever added or orphaned. Under a dot
import the sibling name must itself resolve to unicode/utf8 at the call
site — where a local declaration shadows it, the report stays advisory.
A comment inside the deleted scaffolding downgrades the fix to an
advisory report.`,
		Before: `n := utf8.RuneCount([]byte(s))
m := utf8.RuneCountInString(string(b))`,
		After: `n := utf8.RuneCountInString(s)
m := utf8.RuneCount(b)`,
		MeasuredWin: `BenchmarkPS2024 (~1.1KB operands, Apple M2 Pro, gc 1.26):
string direction, a mixed-width line of 896 runes —
utf8.RuneCount([]byte(s)) 1196 ns/op, 1152 B/op, 1 allocs/op ->
utf8.RuneCountInString(s) 907 ns/op, 0 B/op, 0 allocs/op. Mirror on
ASCII-dominated bytes (non-ASCII only in the final rune) —
utf8.RuneCountInString(string(b)) 757 ns/op, 1152 B/op, 1 allocs/op ->
utf8.RuneCount(b) 406 ns/op, 0 B/op, 0 allocs/op (~1.9x). Mirror on the
mixed line, whose second byte is already non-ASCII — 1287 ns/op,
1152 B/op -> 1286 ns/op, 1152 B/op: parity, the honest worst case,
because RuneCount internally re-copies the non-ASCII tail on current gc
(see above); the After's copy is never larger than the Before's.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2024",
		Doc:  "utf8.RuneCount/RuneCountInString fed through a throwaway []byte/string conversion; call the sibling on the operand directly",
		Run:  runPS2024,
	},
})

func runPS2024(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 1 || call.Ellipsis.IsValid() {
				return true
			}
			nameID := ps2024Utf8Callee(pass, call)
			if nameID == nil {
				return true
			}
			switch nameID.Name {
			case "RuneCount":
				// utf8.RuneCount([]byte(x)) with x a plain string.
				conv, isRune := ps2125Conversion(pass, ps2108Unparen(call.Args[0]))
				if conv == nil || isRune {
					return true
				}
				x := conv.Args[0]
				xt := pass.TypesInfo.TypeOf(x)
				if xt == nil || !types.Identical(types.Default(xt), types.Typ[types.String]) {
					return true
				}
				ps2024Report(pass, f, call, nameID, conv, x, "RuneCountInString",
					"utf8.RuneCount("+ps2125ExprText(conv)+") copies "+ps2125ExprText(x)+" into a throwaway []byte just to count its runes; utf8.RuneCountInString("+ps2125ExprText(x)+") is the bit-identical, zero-copy count")
			case "RuneCountInString":
				// utf8.RuneCountInString(string(b)) with b a plain []byte.
				conv := ps2024StringConv(pass, ps2108Unparen(call.Args[0]))
				if conv == nil {
					return true
				}
				b := conv.Args[0]
				if !ps2022IsPlainByteSlice(pass, b) {
					return true
				}
				ps2024Report(pass, f, call, nameID, conv, b, "RuneCount",
					"utf8.RuneCountInString("+ps2125ExprText(conv)+") copies "+ps2125ExprText(b)+" into a throwaway string just to count its runes; utf8.RuneCount("+ps2125ExprText(b)+") is the bit-identical, zero-copy count")
			}
			return true
		})
	}
	return nil, nil
}

// ps2024Report emits the diagnostic for one matched call and, when every
// guard holds, attaches the sibling rewrite: the callee name ident is
// renamed to sibling and only the conversion scaffolding around x is
// deleted — x stays byte-verbatim in place, so it is evaluated exactly
// once in exactly the original spelling.
func ps2024Report(pass *analysis.Pass, f *ast.File, call *ast.CallExpr, nameID *ast.Ident, conv *ast.CallExpr, x ast.Expr, sibling, msg string) {
	diag := analysis.Diagnostic{
		Pos:     call.Pos(),
		End:     call.End(),
		Message: msg,
	}
	// A comment inside the deleted scaffolding would be destroyed by the
	// edits — the fix is withheld then and the report stays advisory.
	fixOK := !ps2111CommentIn(f, conv.Pos(), x.Pos()) && !ps2111CommentIn(f, x.End(), conv.End())
	// Under a dot import the callee is a bare ident and the rewrite
	// spells the bare sibling name; a local declaration shadowing that
	// name at the call site would capture it — withhold the fix there.
	// (A selector callee keeps its qualifier, and the sibling is exported
	// from the same package, so nothing can shadow it.)
	if fixOK {
		if _, bare := ps2108Unparen(call.Fun).(*ast.Ident); bare {
			fixOK = ps2024SiblingUsable(pass, call, sibling)
		}
	}
	if fixOK {
		diag.SuggestedFixes = []analysis.SuggestedFix{{
			Message: "replace with utf8." + sibling + "(...) on the unconverted operand",
			TextEdits: []analysis.TextEdit{
				{Pos: nameID.Pos(), End: nameID.End(), NewText: []byte(sibling)},
				{Pos: conv.Pos(), End: x.Pos(), NewText: nil},
				{Pos: x.End(), End: conv.End(), NewText: nil},
			},
		}}
	}
	pass.Report(diag)
}

// ps2024SiblingUsable reports whether the bare identifier name resolves
// to unicode/utf8's package-level function of that name at pos — true
// exactly when the dot-imported sibling is not shadowed at the call
// site.
func ps2024SiblingUsable(pass *analysis.Pass, call *ast.CallExpr, name string) bool {
	scope := pass.Pkg.Scope().Innermost(call.Pos())
	if scope == nil {
		return false
	}
	_, obj := scope.LookupParent(name, call.Pos())
	fn, ok := obj.(*types.Func)
	return ok && fn.Pkg() != nil && fn.Pkg().Path() == "unicode/utf8"
}

// ps2024Utf8Callee resolves call's callee to a package-level function of
// the standard library's unicode/utf8 — through a selector with any
// import alias, or a bare ident under a dot import — and returns the
// name ident to be renamed by the fix. A shadowed identifier, a
// same-named local function, or a function value stored in a variable
// resolves to a different object and returns nil.
func ps2024Utf8Callee(pass *analysis.Pass, call *ast.CallExpr) *ast.Ident {
	var id *ast.Ident
	switch fun := ps2108Unparen(call.Fun).(type) {
	case *ast.SelectorExpr:
		id = fun.Sel
	case *ast.Ident:
		id = fun
	default:
		return nil
	}
	fn, ok := pass.TypesInfo.Uses[id].(*types.Func)
	if !ok || fn.Pkg() == nil || fn.Pkg().Path() != "unicode/utf8" {
		return nil
	}
	return id
}

// ps2024StringConv matches e as the direct conversion string(x) spelled
// with the predeclared (unshadowed) string identifier. Function calls
// that merely look like conversions are rejected via type information.
func ps2024StringConv(pass *analysis.Pass, e ast.Expr) *ast.CallExpr {
	call, ok := e.(*ast.CallExpr)
	if !ok || len(call.Args) != 1 || call.Ellipsis.IsValid() {
		return nil
	}
	id, ok := ps2108Unparen(call.Fun).(*ast.Ident)
	if !ok || pass.TypesInfo.Uses[id] != types.Universe.Lookup("string") {
		return nil
	}
	if tv, ok := pass.TypesInfo.Types[call.Fun]; !ok || !tv.IsType() {
		return nil
	}
	return call
}
