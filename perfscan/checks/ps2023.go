package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS2023 reports fmt.Errorf("%s", s) / fmt.Errorf("%v", s) with s
// statically a plain string — PS2130's identity formatting landing on an
// error value: fmt boxes s into an interface, parses the one-verb format
// and copies the printer buffer into a fresh string, only to hand the
// very same bytes to errors.New. errors.New(s) builds the identical
// error directly.
var PS2023 = register(&lint.Check{
	ID:       "PS2023",
	Category: "alloc",
	Slug:     "errorf-string-identity",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: `fmt.Errorf("%s", s) on a plain string formats the identity and hands it to errors.New; errors.New(s) is the same error without the printer`,
		Text: `fmt.Errorf spins up a pooled printer, parses the format string,
boxes the operand into an interface{} (one heap allocation), walks
doPrintf, and copies the pp buffer into a fresh string — and then, for a
format with no %w wrap verb, returns errors.New of that string (this is
fmt.Errorf's documented and implemented behavior). When the format is
exactly "%s" or "%v" and the operand s is STATICALLY the predeclared
type string, the formatted string IS s verbatim — %s and %v write a
plain string operand byte-for-byte — so the whole call computes
errors.New(s) the slow way. The rewrite calls errors.New(s) directly:
the printer get/put, the format parse, the interface boxing and the
intermediate result copy all vanish, and both forms allocate exactly the
one *errorString.

The result is BIT-IDENTICAL. Same dynamic type (*errors.errorString),
same message bytes — including an empty s, invalid UTF-8 and NUL bytes,
which %s/%v pass through untouched. The rewrite is safe against verb
injection: any '%' lives in the ARGUMENT s, which %s/%v copy as data and
never re-parse — the single vanishing verb is the format literal itself.
With no %w in the format the wrapError branches of Errorf never apply,
so neither form implements Unwrap. Both return a fresh pointer on every
call, so == identity behavior is preserved. The operand is kept
byte-verbatim in place — evaluated exactly once, in its original
position; only the fmt wrapper around it changes, and inside
errors.New(...) the operand needs no parenthesization.

The match is deliberately strict; everything else is out of scope. The
callee is resolved with type information — only the standard library's
package function fmt.Errorf matches, never a shadowed fmt, a same-named
third-party package or a method. Exactly two arguments, no variadic
spread. The format must be a string LITERAL whose entire constant value
is the one verb "%s" or "%v" — "%s\n", "%q", "%d", "%+v", "%5s",
"%[1]s", "%%s" and friends all format differently and never match. The
operand must be STATICALLY the predeclared type string (untyped string
constants default to string and match): a defined string type may carry
a String() method fmt would call, a fmt.Stringer, error or []byte
operand is formatted through String()/Error()/fmt's own printing — none
of which is a verbatim copy — and errors.New would also reject them.

The fix rewrites the call to errors.New(s), reusing the file's existing
"errors" import (honoring an alias) or adding one in sorted position
when absent; the import edit is carried once per file. Like PS2126, the
fix still applies when it removes the file's last fmt reference — the
fix pipeline prunes the now-unused fmt import — except in a cgo file
(import "C"), whose import block must never be edited, so there the
report stays advisory. The fix is also withheld when "errors" is
shadowed at the call site or when a comment sits in the replaced
scaffolding around the operand (the rewrite would drop it).`,
		Before: `return fmt.Errorf("%s", msg)`,
		After:  `return errors.New(msg)`,
		MeasuredWin: `BenchmarkPS2023 (a 46-byte runtime string, Apple M2 Pro,
-count=5): fmt.Errorf("%s", msg) ~62 ns/op, 80 B/op, 3 allocs/op vs
errors.New(msg) ~13 ns/op, 16 B/op, 1 alloc/op (~4.8x faster, one
allocation instead of three: the interface boxing of msg and the
printer's result-string copy vanish along with the printer get/put and
the format walk; only the *errorString itself remains).`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2023",
		Doc:  `fmt.Errorf("%s", s) / fmt.Errorf("%v", s) on a plain string computes errors.New(s) through the printf machinery; errors.New(s) builds the identical error directly`,
		Run:  runPS2023,
	},
})

func runPS2023(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		// Collect first: applying all fixes may rewrite the file's last fmt
		// reference and orphan the import. The fix pipeline prunes such an
		// orphan afterwards — except in a cgo file, whose import block is
		// never edited, so there the fixes are withheld and the reports stay
		// advisory (same rule as PS2126/PS2107).
		type site struct {
			call *ast.CallExpr
			verb string
			fix  *analysis.SuggestedFix
		}
		var sites []site
		fixable := 0
		// All fixes of a run apply together, so only the first fix that needs
		// the errors import carries the import edit.
		importAdded := false
		astutil.WithStack(f, func(n ast.Node, stack []ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			verb, s, ok := ps2023Match(pass, call)
			if !ok {
				return true
			}
			var fix *analysis.SuggestedFix
			// The replaced spans are the call text around the operand (the
			// format literal included); a comment there would be silently
			// destroyed — advisory then. A comment INSIDE the operand
			// survives, because the operand's bytes stay untouched in place.
			if !ps2111CommentIn(f, call.Pos(), s.Pos()) && !ps2111CommentIn(f, s.End(), call.End()) {
				// errors.New needs the "errors" package usable at the call site:
				// reuse an existing import (honoring its alias) or add one, unless
				// the name is shadowed or the file is cgo (import block untouched).
				useName, needImport, usable := ps2107PkgUsable(pass, f, call.Pos(), "errors", "errors")
				if usable && !(needImport && ps2107ImportsC(f)) {
					edits := []analysis.TextEdit{
						// The operand stays byte-verbatim in place; only the
						// fmt wrapper around it is rewritten. Inside
						// errors.New(...)'s parens no operand ever needs
						// parenthesization.
						{Pos: call.Pos(), End: s.Pos(), NewText: []byte(useName + ".New(")},
						{Pos: s.End(), End: call.End(), NewText: []byte(")")},
					}
					if needImport && !importAdded {
						edits = append(edits, ps2107ImportEdit(f, "errors"))
						importAdded = true
					}
					fix = &analysis.SuggestedFix{
						Message:   "replace fmt.Errorf(\"" + verb + "\", s) with errors.New(s)",
						TextEdits: edits,
					}
					fixable++
				}
			}
			sites = append(sites, site{call, verb, fix})
			return true
		})
		// Each fixable call removes exactly one fmt reference (its qualifier;
		// the format literal holds none, and the operand's references — even
		// a nested fmt call returning string — stay in place). When those are
		// ALL of the file's fmt references the rewrites orphan the import,
		// which the fix pipeline prunes — impossible only in a cgo file, so
		// there the reports stay advisory (same accounting as PS2126).
		emitFixes := fixable > 0 && (pkgRefCount(pass, f, "fmt") > fixable || !ps2107ImportsC(f))
		for _, st := range sites {
			diag := analysis.Diagnostic{
				Pos:     st.call.Pos(),
				End:     st.call.End(),
				Message: "fmt.Errorf(\"" + st.verb + "\", s) on a plain string boxes s, parses the format and copies the printer result just to hand the identical bytes to errors.New; errors.New(s) builds the identical error directly",
			}
			if emitFixes && st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps2023Match matches call as fmt.Errorf("%s", s) or fmt.Errorf("%v", s)
// with s statically the predeclared string type. The callee is pinned by
// type information to the standard library fmt.Errorf, so a shadowed
// fmt, a same-named third-party package or a method never matches. The
// format must be a string literal whose entire constant value is exactly
// the one verb.
func ps2023Match(pass *analysis.Pass, call *ast.CallExpr) (verb string, s ast.Expr, ok bool) {
	// Exactly (format, s), not spread: a spread passes an unknown number of
	// operands and a third operand would append %!(EXTRA ...).
	if len(call.Args) != 2 || call.Ellipsis.IsValid() {
		return "", nil, false
	}
	if _, isErrorf := astutil.PkgFuncCall(pass.TypesInfo, call.Fun, "fmt", map[string]bool{"Errorf": true}); !isErrorf {
		return "", nil, false
	}
	// The format must be a string LITERAL (possibly parenthesized) whose
	// entire constant value is the one verb "%s" or "%v" — anything else
	// ("%s\n", "%q", "%5s", "%w", …) formats differently.
	lit, isLit := ps2110Unparen(call.Args[0]).(*ast.BasicLit)
	if !isLit || lit.Kind != token.STRING {
		return "", nil, false
	}
	tv, has := pass.TypesInfo.Types[call.Args[0]]
	if !has || tv.Value == nil || tv.Value.Kind() != constant.String {
		return "", nil, false
	}
	verb = constant.StringVal(tv.Value)
	if verb != "%s" && verb != "%v" {
		return "", nil, false
	}
	s = call.Args[1]
	// s must be STATICALLY the predeclared string (aliases included): a
	// defined string type may carry a String() method fmt would call, and a
	// []byte, fmt.Stringer or error operand goes through fmt's own
	// formatting, not a verbatim copy. Untyped string constants default to
	// string and match.
	st := pass.TypesInfo.TypeOf(s)
	if st == nil || !types.Identical(st, types.Typ[types.String]) {
		return "", nil, false
	}
	return verb, s, true
}
