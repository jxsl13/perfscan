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

// PS2031 reports fmt.Errorf("%s", s) / fmt.Errorf("%v", s) with s
// statically a plain string — the whole printf machinery (format scan,
// interface boxing, a throwaway copy of every byte of s) run just to
// hand s verbatim to errors.New, which errors.New(s) does directly.
// This is PS2126's sibling: PS2126 handles the verbless constant
// message, PS2031 the one-verb pass-through of a string operand.
var PS2031 = register(&lint.Check{
	ID:       "PS2031",
	Category: "alloc",
	Slug:     "errorf-string-passthrough",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: `fmt.Errorf("%s", s) on a plain string is errors.New(s) behind the printf machinery`,
		Text: `fmt.Errorf("%s", s) gets a printer from fmt's sync.Pool, scans the
format for verbs in doPrintf, boxes s into an any (one heap allocation
for the string-to-interface conversion), materializes a fresh result via
string(p.buf) (a second allocation plus a copy of every byte of s), and
then — because there is no %w verb — returns errors.New of that copy
(the errorString allocation). errors.New(s) performs ONLY that last
allocation: no pool traffic, no format scan, no boxing, no throwaway
copy. Roughly three allocations and a format walk collapse to one
allocation. fmt.Errorf("%v", s) is the same pattern — %v on a string
operand is %s.

The rewrite is BIT-IDENTICAL. With no %w verb fmt.Errorf returns
errors.New(formatted) — the same *errorString dynamic type, no Unwrap
method — so the only question is whether formatted equals s. For a
format that is EXACTLY "%s" or EXACTLY "%v" (one verb, no width, flags
or index) applied to an operand STATICALLY of the predeclared type
string, doPrintf writes s's bytes verbatim into the buffer: both %s and
%v copy a string operand unchanged. Any '%' inside s is DATA under
%s/%v, never re-parsed — the vanishing one-verb format itself was the
only interpreter. Invalid UTF-8 and NUL bytes ride through both
spellings unchanged, and s cannot be nil (it is a non-pointer string
value). s is evaluated exactly once, in its original position, on both
sides. The one boundary is the unsafe package: errors.New(s) shares s's
backing array where the fmt path stored a fresh copy — observable only
via unsafe.StringData, indistinguishable under safe Go semantics (the
same boundary PS2130 documents for the Sprintf identity).

The match is deliberately strict; everything else is out of scope. The
callee is pinned by type information to the package-level fmt.Errorf (an
aliased fmt import matches; a shadowed fmt, a same-named third-party
package or a method named Errorf does not). The call passes exactly two
arguments with no variadic spread; the format is a string LITERAL whose
entire constant value is "%s" or "%v" — "%s\n", "%q", "%d", "%+v",
"%5s", "%[1]s", "%%s" and friends all format differently and never
match, and a variable or named constant holding "%s" proves nothing at
this call site. The operand must be STATICALLY the predeclared string:
a defined string type (type S string), a []byte, a fmt.Stringer or an
error operand routes through String()/Error() or formats differently —
excluded. %w with a string operand does not compile, so wrapping is
structurally out of the picture.

The fix deletes the fmt.Errorf wrapper and the format literal around the
operand, whose text stays byte-verbatim in place (already inside the new
call's parentheses, so no parenthesization question arises), and
qualifies errors.New with the file's existing "errors" import (honoring
an alias) or adds one in sorted position when absent. The fix is
withheld — the report stays advisory — when "errors" is shadowed or
dot/blank-imported at the call site, when a comment overlaps the deleted
wrapper text (a comment inside the operand itself survives verbatim), or
in a cgo file (import "C") whose import block must never be edited, when
that file would need the errors import added or would have its last fmt
reference removed (the fix pipeline prunes the orphaned fmt import in
ordinary files, exactly as for PS2126).`,
		Before: `return fmt.Errorf("%s", msg)`,
		After:  `return errors.New(msg)`,
		MeasuredWin: `BenchmarkPS2031 (a 26-byte message string, Apple M2
Pro, go1.26, -count=3): fmt.Errorf("%s", msg) ~66.0 ns/op, 64 B/op,
3 allocs/op vs errors.New(msg) ~14.8 ns/op, 16 B/op, 1 alloc/op (~4.5x
faster, 3 allocs down to 1: the interface boxing, the formatted copy
and the format walk all vanish; only the errorString allocation
remains, and it no longer copies the message bytes).`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2031",
		Doc:  `fmt.Errorf("%s", s) / fmt.Errorf("%v", s) on a plain string runs the printf machinery to hand s verbatim to errors.New; errors.New(s) builds the identical error directly`,
		Run:  runPS2031,
	},
})

func runPS2031(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		// Collect first: applying all fixes may rewrite the file's last
		// fmt reference and orphan the import. The fix pipeline prunes
		// such an orphan afterwards — except in a cgo file, whose import
		// block is never edited, so there the fixes are withheld and the
		// reports stay advisory (same rule as PS2126/PS2107).
		type site struct {
			call *ast.CallExpr
			verb string
			fix  *analysis.SuggestedFix
		}
		var sites []site
		fixable := 0
		// All fixes of a run apply together, so only the first fix that
		// needs the errors import carries the import edit.
		importAdded := false
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			verb, s, ok := ps2031Match(pass, call)
			if !ok {
				return true
			}
			var fix *analysis.SuggestedFix
			// The deleted spans are the wrapper around the operand (the
			// format literal included); a comment there would be silently
			// destroyed — advisory then. A comment INSIDE the operand
			// survives: s's bytes are not touched.
			if !ps2111CommentIn(f, call.Pos(), s.Pos()) && !ps2111CommentIn(f, s.End(), call.End()) {
				// errors.New needs the "errors" package usable at the call
				// site: reuse an existing import (honoring its alias) or add
				// one, unless the name is shadowed or the file is cgo (import
				// block untouched).
				useName, needImport, usable := ps2107PkgUsable(pass, f, call.Pos(), "errors", "errors")
				if usable && !(needImport && ps2107ImportsC(f)) {
					// The operand text stays byte-verbatim in place — evaluated
					// once, in the original position; only the wrapper around it
					// is rewritten. Inside errors.New's parentheses no operand
					// shape ever needs extra parentheses.
					edits := []analysis.TextEdit{
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
		// Each fixable call removes exactly one fmt reference (its
		// qualifier); when those are ALL of the file's fmt references, the
		// rewrites orphan the import, which is fine in an ordinary file
		// (the fix pipeline prunes it) but not in a cgo file.
		emitFixes := fixable > 0 && (pkgRefCount(pass, f, "fmt") > fixable || !ps2107ImportsC(f))
		for _, st := range sites {
			diag := analysis.Diagnostic{
				Pos:     st.call.Pos(),
				End:     st.call.End(),
				Message: "fmt.Errorf with a bare " + st.verb + " verb on a plain string runs the printf machinery — format scan, interface boxing and a throwaway copy — just to hand the string verbatim to errors.New; errors.New(s) builds the identical error with one allocation",
			}
			if emitFixes && st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps2031Match matches call as fmt.Errorf("%s", s) or fmt.Errorf("%v", s)
// with s statically the predeclared string type. The callee is pinned by
// type information to the package-level fmt.Errorf (aliased fmt imports
// included), the format must be a string LITERAL whose entire constant
// value is exactly the one verb, and there must be no variadic spread.
func ps2031Match(pass *analysis.Pass, call *ast.CallExpr) (verb string, s ast.Expr, ok bool) {
	if len(call.Args) != 2 || call.Ellipsis.IsValid() {
		return "", nil, false
	}
	if _, isErrorf := astutil.PkgFuncCall(pass.TypesInfo, call.Fun, "fmt", map[string]bool{"Errorf": true}); !isErrorf {
		return "", nil, false
	}
	// The format must be a string LITERAL that is exactly "%s" or "%v" —
	// one verb, nothing else: "%s\n", "%q", "%5s", "%[1]s", "%%s" and
	// friends all format differently. A variable or named constant
	// holding "%s" is not matched: the literal is what makes the
	// vanishing format locally evident.
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
	// The operand must be STATICALLY the predeclared string (aliases of
	// it included): a defined string type, []byte, fmt.Stringer or error
	// operand routes through String()/Error() or formats differently —
	// never a verbatim pass-through. Untyped string constants default to
	// string and match.
	s = call.Args[1]
	st := pass.TypesInfo.TypeOf(s)
	if st == nil || !types.Identical(st, types.Typ[types.String]) {
		return "", nil, false
	}
	return verb, s, true
}
