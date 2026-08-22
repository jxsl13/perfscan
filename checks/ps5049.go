package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS5049 reports append(dst, fmt.Sprintf(f, a...)...) — and its verbless
// Sprint/Sprintln twins — where the formatted text is materialized as an
// intermediate string only to be immediately copied into dst by a spread
// append. fmt.Appendf/Append/Appendln(dst, ...) format the identical bytes
// straight into dst's backing array, skipping the throwaway string allocation.
//
// The spread-append sibling of PS2109 ([]byte(fmt.Sprintf(...)) ->
// fmt.Appendf(nil, ...)): PS2109 rewrites the fresh-slice conversion form,
// PS5049 the append-onto-an-existing-buffer form. It reuses PS2109's verified
// fmt-formatter detection and Sprint*->Append* twin table.
var PS5049 = register(&lint.Check{
	ID:       "PS5049",
	Category: "alloc",
	Slug:     "append-sprintf-to-appendf",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "append(dst, fmt.Sprintf(...)...) allocates an intermediate string then copies it; fmt.Appendf formats into dst directly",
		Text: `append(dst, fmt.Sprintf(format, a...)...) runs fmt's formatter to build a
string on the heap, then a spread append copies that string's bytes into dst.
fmt.Appendf(dst, format, a...) formats those exact bytes directly into dst's
backing array (growing it as append would), so the throwaway string — and the
allocation and copy it costs — never exist. The verbless twins map the same
way: fmt.Sprint -> fmt.Append and fmt.Sprintln -> fmt.Appendln.

The rewrite is BYTE-IDENTICAL: fmt.Append{f,,ln} drive the identical format
machinery as fmt.Sprint{f,,ln} over the identical operands, and a spread append
of the resulting string appends exactly those bytes to dst — verified equal for
the plain, verbless, and newline forms, and even when a formatted operand
aliases dst (the argument's contents are read before the tail bytes it points
past are grown, so the result matches).

The match is deliberately narrow — it is the whole safety story:
  - the callee is the predeclared append builtin, confirmed through type
    information, so a shadowing local named append never matches, and the call
    is the two-argument spread form append(dst, x...);
  - the spread argument is a direct call (no wrapping parens) of the
    package-level fmt.Sprintf / fmt.Sprint / fmt.Sprintln — pinned by type
    information, so a shadowed fmt, an aliased import (the alias is carried
    through verbatim), or a method of the same name never matches;
  - the destination's type is an UNNAMED []byte. fmt.Append* returns []byte, so
    a named byte-slice destination (whose append would return that named type)
    would change the expression's static type and is reported without a fix.
    Because append(dst, aString...) only compiles when dst is a byte slice, the
    destination is always some []byte; the guard only narrows it to the unnamed
    case.
No import is added or dropped — fmt survives the rewrite (Sprintf becomes
Appendf). A comment inside the rewritten scaffolding (the append wrapper or the
fmt.Sprint* call head) keeps the report advisory.`,
		Before: `dst = append(dst, fmt.Sprintf("user=%d", id)...)`,
		After:  `dst = fmt.Appendf(dst, "user=%d", id)`,
		MeasuredWin: `append(dst, fmt.Sprintf("%d-%s", n, s)...) ~54 ns/op, 8 B/op, 1 alloc/op vs ` +
			`fmt.Appendf(dst, "%d-%s", n, s) ~44 ns/op, 0 B/op, 0 allocs/op (Apple M2 Pro, go1.26) — ` +
			`the eliminated intermediate string is one heap allocation and one copy per call.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5049",
		Doc:  "append(dst, fmt.Sprintf(...)...) instead of fmt.Appendf(dst, ...)",
		Run:  runPS5049,
	},
})

func runPS5049(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			outer, ok := n.(*ast.CallExpr)
			if !ok || len(outer.Args) != 2 || !outer.Ellipsis.IsValid() {
				return true
			}
			// The callee must be the predeclared append builtin, not a
			// shadowing local.
			id, ok := outer.Fun.(*ast.Ident)
			if !ok {
				return true
			}
			if b, ok := pass.TypesInfo.Uses[id].(*types.Builtin); !ok || b.Name() != "append" {
				return true
			}
			// The spread argument is a direct call of a package-level fmt
			// formatter with an Append* twin (reusing PS2109's detection).
			inner, ok := outer.Args[1].(*ast.CallExpr)
			if !ok {
				return true
			}
			name := ps2109FmtSprintName(pass, inner)
			if name == "" {
				return true
			}
			appendName := ps2109Append[name]
			dst := outer.Args[0]

			diag := analysis.Diagnostic{
				Pos: outer.Pos(),
				End: outer.End(),
				Message: "append(dst, fmt." + name + "(...)...) allocates an intermediate string and copies it; fmt." +
					appendName + "(dst, ...) formats into dst directly",
			}
			// Fix only when the destination is an unnamed []byte (fmt.Append*
			// returns []byte; a named byte-slice destination would change the
			// expression's static type). The rewritten scaffolding — the append
			// wrapper before dst, the ", fmt.Sprint*(" head, and the ")...)"
			// tail — must not swallow a comment.
			midEnd := inner.Rparen // zero-arg Sprint/Sprintln: nothing between dst and the call's ")"
			if len(inner.Args) > 0 {
				midEnd = inner.Args[0].Pos()
			}
			if ps2141ByteSliceUnnamed(pass.TypesInfo.TypeOf(dst)) &&
				!ps2109CommentBetween(f, outer.Pos(), dst.Pos()) &&
				!ps2109CommentBetween(f, dst.End(), midEnd) &&
				!ps2109CommentBetween(f, inner.Rparen, outer.End()) {
				sel := inner.Fun.(*ast.SelectorExpr)
				qual := sel.X.(*ast.Ident).Name // fmt, or its import alias — carried through verbatim

				// Replace "append(" with "<qual>.<Append*>(" ...
				edits := []analysis.TextEdit{
					{Pos: outer.Pos(), End: dst.Pos(), NewText: []byte(qual + "." + appendName + "(")},
				}
				// ... splice dst as the first argument, dropping the inner
				// "fmt.Sprint*(" head (has-args) or the whole head incl. its
				// "(" (zero-arg) ...
				if len(inner.Args) > 0 {
					edits = append(edits, analysis.TextEdit{Pos: dst.End(), End: midEnd, NewText: []byte(", ")})
				} else {
					edits = append(edits, analysis.TextEdit{Pos: dst.End(), End: inner.Rparen})
				}
				// ... and collapse the ")...)" tail to a single ")".
				edits = append(edits, analysis.TextEdit{Pos: inner.Rparen, End: outer.End(), NewText: []byte(")")})

				diag.SuggestedFixes = []analysis.SuggestedFix{{
					Message:   "replace with " + qual + "." + appendName + "(dst, ...)",
					TextEdits: edits,
				}}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}
