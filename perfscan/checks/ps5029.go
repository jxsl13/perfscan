package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS5029 reports fmt.Sprintln(a, b, ..., z) where EVERY operand is a
// plain string — the shape where Sprintln's UNCONDITIONAL formatting
// rule (exactly one ' ' between adjacent operands, exactly one trailing
// '\n', no format-string interpretation) makes the result byte-for-byte
// a + " " + b + ... + z + "\n", without fmt's reflection machinery or
// its pooled pp buffer. The Sprintln sibling of PS2123 (fmt.Sprint over
// plain strings, which inserts NO separators).
var PS5029 = register(&lint.Check{
	ID:       "PS5029",
	Category: "alloc",
	Slug:     "sprintln-concat",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: `fmt.Sprintln over plain strings is a reflection-priced + with " " and "\n"; concatenate directly`,
		Text: `fmt.Sprintln takes a pp printer from fmt's sync.Pool, boxes every
operand into an interface (a string box is a heap allocation), walks
doPrintln's per-operand default formatter through the pooled buffer and
heap-allocates the result via string(p.buf). Unlike Sprint's
conditional spacing rule, doPrintln is UNCONDITIONAL: it writes exactly
one ' ' between adjacent operands and exactly one trailing '\n', with
no format-string interpretation (a '%' in an operand is printed
literally). So when EVERY operand is a plain string the result is
byte-for-byte a + " " + b + ... + z + "\n" — and that + chain lowers to
a single runtime.concatstrings call producing ONE allocation of exactly
the final length, with zero interface boxing, zero pool traffic and
zero format dispatch. The single-operand fmt.Sprintln(a) rewrites to
a + "\n". Both the call and the + chain evaluate each operand exactly
once, left to right, so side-effect count and order are preserved.

The match is deliberately narrow — it is the whole safety story. The
callee must resolve via type information to the package-level
fmt.Sprintln (a shadowed fmt or a method named Sprintln does not), the
call must not spread its arguments (no ...), and there must be at least
one argument (the zero-argument fmt.Sprintln() is out of scope).
fmt.Sprint (no separators — PS2123's territory) and fmt.Sprintf
(PS2122's) never match. Every argument must have type EXACTLY the
predeclared string (untyped constant strings default to it) — a NAMED
string type could implement fmt.Stringer or fmt.Formatter, which
Sprintln's %v formatting honors and + would not, so named types,
[]byte, error, interfaces, numbers and bools all stay out. For plain
strings the default %v formatting emits the value verbatim — empty
strings and invalid UTF-8 included — making the rewrite bit-identical.

The fix keeps every argument byte-verbatim in place and edits only the
scaffolding: the fmt.Sprintln( prefix is dropped, each inter-argument
comma becomes a + " " + join and the closing parenthesis becomes
+ "\n". String-typed operands are primaries or + chains, so the
operands never need parentheses; the WHOLE replacement is parenthesized
when the surrounding context binds tighter than + (an index, a slice, a
binary operand), exactly as PS2121 decides it — and unlike PS2123's
identity case the replacement always ends in + "\n", so there is
no parenthesis-free single-operand shortcut. Each rewrite removes the
file's fmt.Sprintln selector; the fix applies even when that removes
the file's last fmt reference — the fix pipeline prunes the now-unused
fmt import — EXCEPT in cgo files (import "C"), whose import block is
never edited: there the fixes are withheld and the report stays
advisory. A comment inside the rewritten scaffolding is never silently
dropped: it suppresses the fix and keeps the report.`,
		Before: `s := fmt.Sprintln(host, port)`,
		After:  `s := host + " " + port + "\n"`,
		MeasuredWin: `BenchmarkPS5029 (a host + " " + port join of two plain
strings, 24/5 bytes, once per op, Apple M2 Pro, go1.26):
fmt.Sprintln(host, port) 67.1 ns/op, 64 B/op, 3 allocs/op vs
host + " " + port + "\n" 28.4 ns/op, 32 B/op, 1 alloc/op (~2.4x faster,
one allocation instead of three — the two interface boxings and the
fmt pp buffer round-trip disappear, leaving only the result string).`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5029",
		Doc:  "fmt.Sprintln over plain strings writes exactly one space between operands and one trailing newline; direct + concatenation is identical and cheaper",
		Run:  runPS5029,
	},
})

// ps5029Msg is the diagnostic text (shared by the fixed and advisory paths).
const ps5029Msg = `fmt.Sprintln over only plain strings writes exactly one space between operands and one trailing newline, boxing every operand through fmt's reflection machinery; direct + concatenation with " " and "\n" builds the identical string`

func runPS5029(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		// Collect first: applying all fixes may rewrite the file's last
		// fmt reference and orphan the import. The fix pipeline prunes
		// such an orphan afterwards — except in a cgo file (import "C"),
		// whose import block is never edited, so there the fixes are
		// withheld and the reports stay advisory (same guard as
		// PS2107/PS2118/PS2123).
		type site struct {
			call *ast.CallExpr
			fix  *analysis.SuggestedFix
		}
		var sites []site
		fixable := 0
		astutil.WithStack(f, func(n ast.Node, stack []ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) < 1 || call.Ellipsis.IsValid() {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			// Type info pins the callee to the package-level fmt.Sprintln:
			// a shadowed fmt resolves sel.Sel to some other object, and a
			// method named Sprintln carries a receiver. Sprint and Sprintf
			// fail the name test.
			fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
			if !ok || fn.Name() != "Sprintln" || fn.Pkg() == nil || fn.Pkg().Path() != "fmt" {
				return true
			}
			if sig, isSig := fn.Type().(*types.Signature); !isSig || sig.Recv() != nil {
				return true
			}
			// Every argument must be EXACTLY the predeclared string
			// (untyped constant strings default to it). A named string
			// type could implement fmt.Stringer/fmt.Formatter, which
			// Sprintln's %v formatting honors and + would not —
			// bit-identity requires plain string throughout.
			for _, a := range call.Args {
				t := pass.TypesInfo.TypeOf(a)
				if t == nil || !types.Identical(types.Default(t), types.Typ[types.String]) {
					return true
				}
			}
			var parent ast.Node
			if len(stack) > 0 {
				parent = stack[len(stack)-1]
			}
			// The replacement ALWAYS ends in + "\n", so it is always a +
			// chain — no single-operand parenthesis-free shortcut exists
			// (unlike PS2123's identity rewrite).
			fix := ps5029Fix(f, call, ps2121NeedsParens(parent, call))
			if fix != nil {
				fixable++
			}
			sites = append(sites, site{call, fix})
			return true
		})
		// Each fixable call holds exactly one fmt reference (its selector's
		// package identifier); if those are ALL of the file's fmt
		// references, the fixes orphan the import — fine in a non-cgo file
		// (the fix pipeline prunes it), advisory only in a cgo file.
		emitFixes := fixable > 0 && (pkgRefCount(pass, f, "fmt") > fixable || !ps2110ImportsC(f))
		for _, st := range sites {
			diag := analysis.Diagnostic{
				Pos:     st.call.Pos(),
				End:     st.call.End(),
				Message: ps5029Msg,
			}
			if emitFixes && st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps5029Fix builds the a + " " + b + ... + z + "\n" replacement (or
// a + "\n" for a single operand). Every argument stays byte-verbatim in
// place; only the scaffolding is edited — the fmt.Sprintln( prefix
// becomes the (optional) opening parenthesis, each inter-argument comma
// becomes ` + " " + ` and the closing parenthesis of the call becomes
// ` + "\n"` plus the (optional) closing one. String-typed operands are
// primaries or + chains (the only string-producing binary operator is
// the associative + itself), so the operands themselves never need
// parentheses. A comment inside any scaffolding span would be destroyed
// by the edits — the fix is withheld then and the report stays advisory.
func ps5029Fix(f *ast.File, call *ast.CallExpr, needParens bool) *analysis.SuggestedFix {
	args := call.Args
	if ps2111CommentIn(f, call.Pos(), args[0].Pos()) ||
		ps2111CommentIn(f, args[len(args)-1].End(), call.End()) {
		return nil
	}
	for i := 0; i+1 < len(args); i++ {
		if ps2111CommentIn(f, args[i].End(), args[i+1].Pos()) {
			return nil
		}
	}
	open, closing := "", ""
	if needParens {
		open, closing = "(", ")"
	}
	edits := make([]analysis.TextEdit, 0, len(args)+1)
	edits = append(edits, analysis.TextEdit{Pos: call.Pos(), End: args[0].Pos(), NewText: []byte(open)})
	for i := 0; i+1 < len(args); i++ {
		edits = append(edits, analysis.TextEdit{Pos: args[i].End(), End: args[i+1].Pos(), NewText: []byte(` + " " + `)})
	}
	edits = append(edits, analysis.TextEdit{Pos: args[len(args)-1].End(), End: call.End(), NewText: []byte(` + "\n"` + closing)})
	return &analysis.SuggestedFix{
		Message:   `replace fmt.Sprintln with direct string concatenation ending in "\n"`,
		TextEdits: edits,
	}
}
