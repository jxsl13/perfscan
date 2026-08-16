package checks

import (
	"go/ast"
	"go/token"
	"go/types"
	"strconv"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS5044 reports fmt.Appendf(buf, "%v", s) — a lone "%v" over a plain string —
// where the direct append(buf, s...) writes the identical bytes without fmt's
// formatter machinery. The "%v"-over-string cell that PS2141 ("%s") and PS2035
// ("%v" over a scalar, strings out of scope) both leave uncovered. Unlike
// PS2141, a []byte operand is NOT matched: "%v" of a []byte prints the decimal
// element list "[104 105]", not the bytes.
var PS5044 = register(&lint.Check{
	ID:       "PS5044",
	Category: "alloc",
	Slug:     "appendf-v-string",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "fmt.Appendf(buf, \"%v\", s) over a plain string runs fmt's formatter to append bytes append writes directly",
		Text: `fmt.Appendf parses its format string, boxes the argument into an
interface (a heap allocation), and walks fmt's formatter state machine through a
pooled pp buffer — even when the format is a lone "%v" splicing one string onto
the buffer. For an operand whose static type is EXACTLY the predeclared string,
"%v" writes the string's bytes verbatim (no quoting, no spacing, invalid UTF-8
and the empty string included) — byte-for-byte append(buf, s...), which hits the
builtin's string-to-[]byte fast path with no fmt and no interface box — the same
fast path PS2141 rewrites the "%s" twin onto.

The match is deliberately narrow — it is the whole safety story:
  - the callee is pinned by type information to the package-level fmt.Appendf (a
    shadowed fmt or a method named Appendf never matches) and must not spread its
    arguments;
  - the format is a string literal that is EXACTLY "%v" — any literal text,
    flag, width, or other verb disqualifies it;
  - the operand's default type is EXACTLY the predeclared string. A NAMED string
    type is excluded: "%v" honors fmt.Stringer/fmt.Formatter, so a named type
    with a String()/Format() method would print via that method, not the raw
    bytes. An unnamed predeclared string carries no methods, so the two are
    provably identical. A []byte operand ("%v" prints its element list), and
    every non-string operand, are not this shape and never match;
  - the destination is an unnamed []byte, so append(buf, s...) reproduces
    fmt.Appendf's []byte return type exactly (a named byte-slice destination
    keeps the advisory report), and the fix is withheld unless the file keeps
    another fmt reference (so dropping this call never orphans the fmt import).
A comment inside the rewritten scaffolding keeps the report advisory. Named
destinations, named or non-string operands, and non-"%v" formats are reported
without a fix.`,
		Before: `buf = fmt.Appendf(buf, "%v", name)`,
		After:  `buf = append(buf, name...)`,
		MeasuredWin: `fmt.Appendf(buf, "%v", s) ~21.6 ns/op vs append(buf, s...) ~0.5 ns/op ` +
			`(~40x, Apple M2 Pro, go1.26) — append hits the string-to-[]byte fast path; both ` +
			`are 0 allocs when buf is reused, and the fmt path additionally boxes s (a heap ` +
			`allocation when the call escapes). The "%v" twin of PS2141's "%s" profile.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5044",
		Doc:  "fmt.Appendf with a lone %v over a plain string; append(buf, s...) is identical and cheaper",
		Run:  runPS5044,
	},
})

func runPS5044(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		type site struct {
			call *ast.CallExpr
			fix  *analysis.SuggestedFix
		}
		var sites []site
		fixable := 0
		astutil.WithStack(f, func(n ast.Node, stack []ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 3 || call.Ellipsis.IsValid() {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if _, ok := astutil.PkgFuncCall(pass.TypesInfo, call.Fun, "fmt", map[string]bool{"Appendf": true}); !ok {
				return true
			}
			lit, ok := call.Args[1].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			if format, err := strconv.Unquote(lit.Value); err != nil || format != "%v" {
				return true
			}
			// Only report "%v" over a string — the shape append reproduces.
			// "%v" over a []byte prints the element list, over any other type
			// prints something else entirely; neither is this pattern.
			if !ps5044Stringish(pass.TypesInfo.TypeOf(call.Args[2])) {
				return true
			}
			// Fix only when the destination is an unnamed []byte and the operand
			// is EXACTLY the predeclared string (a named string may implement
			// Stringer/Formatter that "%v" honors and append does not).
			fix := (*analysis.SuggestedFix)(nil)
			if ps2141ByteSliceUnnamed(pass.TypesInfo.TypeOf(call.Args[0])) &&
				ps5044PlainString(pass.TypesInfo.TypeOf(call.Args[2])) {
				fix = ps5044Fix(f, call, sel)
				if fix != nil {
					fixable++
				}
			}
			sites = append(sites, site{call, fix})
			return true
		})
		emitFixes := fixable > 0 && pkgRefCount(pass, f, "fmt") > fixable
		for _, st := range sites {
			diag := analysis.Diagnostic{
				Pos:     st.call.Pos(),
				End:     st.call.End(),
				Message: "fmt.Appendf with a lone %v over a plain string boxes the argument and walks fmt's formatter state machine; append(buf, s...) writes the identical bytes directly",
			}
			if emitFixes && st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps5044Stringish reports whether "%v" over a value of type t is a string append
// in spirit: its default underlying type is the predeclared string. Covers named
// and unnamed strings — the fix guard narrows to the provably bit-identical
// unnamed subset. A []byte is deliberately NOT stringish: "%v" over a byte slice
// prints the decimal element list.
func ps5044Stringish(t types.Type) bool {
	if t == nil {
		return false
	}
	b, ok := types.Default(t).Underlying().(*types.Basic)
	return ok && b.Info()&types.IsString != 0
}

// ps5044PlainString reports whether t is EXACTLY the predeclared string type (an
// untyped string constant defaults to it). A named string type may carry a
// String()/Format() method that "%v" honors and append does not, so it is out —
// types.Default of a named type is the named type itself, never a *types.Basic.
func ps5044PlainString(t types.Type) bool {
	if t == nil {
		return false
	}
	basic, ok := types.Default(t).(*types.Basic)
	return ok && basic.Kind() == types.String
}

// ps5044Fix rewrites fmt.Appendf(buf, "%v", s) to append(buf, s...): buf and the
// value stay byte-verbatim in place; only the scaffolding is edited. A comment
// inside a rewritten scaffolding span would be destroyed — the fix is withheld
// then and the report stays advisory.
func ps5044Fix(f *ast.File, call *ast.CallExpr, sel *ast.SelectorExpr) *analysis.SuggestedFix {
	if ps2111CommentIn(f, call.Args[0].End(), call.Args[2].Pos()) ||
		ps2111CommentIn(f, call.Args[2].End(), call.End()) {
		return nil
	}
	return &analysis.SuggestedFix{
		Message: "replace fmt.Appendf with append",
		TextEdits: []analysis.TextEdit{
			{Pos: sel.Pos(), End: sel.End(), NewText: []byte("append")},
			{Pos: call.Args[0].End(), End: call.Args[2].Pos(), NewText: []byte(", ")},
			{Pos: call.Args[2].End(), End: call.End(), NewText: []byte("...)")},
		},
	}
}
