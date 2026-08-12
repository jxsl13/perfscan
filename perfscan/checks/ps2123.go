package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS2123 reports fmt.Sprint(a, b, ..., z) where EVERY operand is a plain
// string — the shape where Sprint's spacing rule inserts nothing and the
// result is byte-for-byte the concatenation a + b + ... + z, without
// fmt's reflection machinery or its pp buffer. The single-operand
// fmt.Sprint(a) is the identity on a and rewrites to a itself.
var PS2123 = register(&lint.Check{
	ID:       "PS2123",
	Category: "alloc",
	Slug:     "sprint-concat",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: `fmt.Sprint over plain strings is a reflection-priced +; concatenate directly`,
		Text: `fmt.Sprint boxes every operand into an interface and walks fmt's
reflection-based default formatter through a pooled pp buffer. Its spec
adds a space between operands only "when neither is a string" — so when
EVERY operand is a string no separator is ever inserted, and the result
is byte-for-byte the operands concatenated. a + b + ... + z is a direct
compiler-emitted concatenation: one length computation, one allocation,
one copy per operand, no fmt at all. A single string operand makes
fmt.Sprint(a) the identity on a.

The match is deliberately narrow — it is the whole safety story. The
callee must resolve via type information to the package-level fmt.Sprint
(a shadowed fmt or a method named Sprint does not), the call must not
spread its arguments (no ...), and there must be at least one argument.
fmt.Sprintln (ALWAYS space-separated plus a trailing newline) and
fmt.Sprintf (PS2122's territory) never match. Every argument must have
type EXACTLY the predeclared string (untyped constant strings default to
it) — a NAMED string type could implement fmt.Stringer or fmt.Formatter,
which Sprint's %v formatting honors and + would not, so named types,
[]byte, error, interfaces, numbers and bools all stay out; even one
non-string operand would also re-engage the spacing rule against its
neighbors. For plain strings the default %v formatting emits the value
verbatim, making the rewrite bit-identical. Argument evaluation order is
unchanged: both the call and the + chain evaluate left to right.

The fix keeps every argument byte-verbatim in place and edits only the
scaffolding: the fmt.Sprint( prefix is dropped and each inter-argument
comma becomes " + ". String-typed operands are primaries or + chains, so
the operands never need parentheses; the WHOLE replacement is
parenthesized when the surrounding context binds tighter than + (an
index, a slice, a binary operand), exactly as PS2121 decides it — except
that a single-operand rewrite whose operand is a primary needs none in
any context. Each rewrite removes the file's fmt.Sprint selector; the
fix applies even when that removes the file's last fmt reference — the
fix pipeline prunes the now-unused fmt import — EXCEPT in cgo files
(import "C"), whose import block is never edited: there the fixes are
withheld and the report stays advisory. A comment inside the rewritten
scaffolding is never silently dropped: it suppresses the fix and keeps
the report.`,
		Before: `s := fmt.Sprint(host, ":", port)`,
		After:  `s := host + ":" + port`,
		MeasuredWin: `BenchmarkPS2123 (a host + ":" + port join of three
plain strings, 24/1/5 bytes, once per op, Apple M2 Pro):
fmt.Sprint(a, b, c) 88.5 ns/op, 80 B/op, 4 allocs/op vs
a + b + c 25.0 ns/op, 32 B/op, 1 alloc/op (~3.5x faster, one
allocation instead of four — the three interface boxings and the fmt
pp buffer round-trip disappear, leaving only the result string).`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2123",
		Doc:  "fmt.Sprint over plain strings inserts no separators; direct + concatenation is identical and cheaper",
		Run:  runPS2123,
	},
})

func runPS2123(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		// Collect first: applying all fixes may rewrite the file's last
		// fmt reference and orphan the import. The fix pipeline prunes
		// such an orphan afterwards — except in a cgo file (import "C"),
		// whose import block is never edited, so there the fixes are
		// withheld and the reports stay advisory (same guard as
		// PS2107/PS2118/PS2122).
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
			// Type info pins the callee to the package-level fmt.Sprint:
			// a shadowed fmt resolves sel.Sel to some other object, and a
			// method named Sprint carries a receiver. Sprintln and Sprintf
			// fail the name test.
			fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
			if !ok || fn.Name() != "Sprint" || fn.Pkg() == nil || fn.Pkg().Path() != "fmt" {
				return true
			}
			if sig, isSig := fn.Type().(*types.Signature); !isSig || sig.Recv() != nil {
				return true
			}
			// Every argument must be EXACTLY the predeclared string
			// (untyped constant strings default to it). A named string
			// type could implement fmt.Stringer/fmt.Formatter, which
			// Sprint's %v formatting honors and + would not, and any
			// non-string operand re-engages the spacing rule —
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
			needParens := ps2121NeedsParens(parent, call)
			if len(call.Args) == 1 {
				// The identity rewrite substitutes the lone operand
				// verbatim: a primary (anything but a + chain) is
				// self-delimiting in every context.
				if _, isBinary := call.Args[0].(*ast.BinaryExpr); !isBinary {
					needParens = false
				}
			}
			fix := ps2123Fix(f, call, needParens)
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
				Message: "fmt.Sprint over only plain strings inserts no separators and boxes every operand through fmt's reflection machinery; direct + concatenation builds the identical string",
			}
			if emitFixes && st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps2123Fix builds the a + b + ... + z replacement (or the bare a for a
// single operand). Every argument stays byte-verbatim in place; only the
// scaffolding is edited — the fmt.Sprint( prefix becomes the (optional)
// opening parenthesis, each inter-argument comma becomes " + ", and the
// closing parenthesis of the call becomes the (optional) closing one.
// String-typed operands are primaries or + chains (the only
// string-producing binary operator is the associative + itself), so the
// operands themselves never need parentheses. A comment inside any
// scaffolding span would be destroyed by the edits — the fix is withheld
// then and the report stays advisory.
func ps2123Fix(f *ast.File, call *ast.CallExpr, needParens bool) *analysis.SuggestedFix {
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
		edits = append(edits, analysis.TextEdit{Pos: args[i].End(), End: args[i+1].Pos(), NewText: []byte(" + ")})
	}
	edits = append(edits, analysis.TextEdit{Pos: args[len(args)-1].End(), End: call.End(), NewText: []byte(closing)})
	return &analysis.SuggestedFix{
		Message:   "replace fmt.Sprint with direct string concatenation",
		TextEdits: edits,
	}
}
