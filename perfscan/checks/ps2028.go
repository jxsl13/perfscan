package checks

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS2028 reports len(strings.Fields(s)) compared against the literal 0
// or 1 to answer a blank-string question — "is there at least one
// field?" / "is s nothing but white space?" — where the Fields call
// allocates the entire []string of fields and always scans the whole
// input just to have its length compared and the slice thrown away.
// strings.TrimSpace(s) == "" (resp. != "") is the same boolean with
// zero allocations, and for the common non-blank input it stops at the
// first non-space byte instead of tallying every field.
var PS2028 = register(&lint.Check{
	ID:       "PS2028",
	Category: "alloc",
	Slug:     "len-fields-blank-test",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: `len(strings.Fields(s)) == 0 allocates every field just to test blankness; strings.TrimSpace(s) == "" is allocation-free`,
		Text: `strings.Fields allocates a []string holding EVERY
whitespace-delimited field of the input (one slice allocation plus a
string header per field) and always scans the entire string. Comparing
only its len against 0 or 1 throws that slice away to answer a single
boolean: "does s contain at least one non-space rune?".
strings.TrimSpace answers the identical question with zero allocations
— it returns a re-slice of s — and for the common non-blank input it
short-circuits at the FIRST non-space byte from the left instead of
walking every field.

Exactly six comparison shapes are blankness-equivalent, and only those
are matched (with the literal on either side of the operator):

	len(strings.Fields(s)) == 0  →  strings.TrimSpace(s) == ""
	len(strings.Fields(s)) <  1  →  strings.TrimSpace(s) == ""
	len(strings.Fields(s)) <= 0  →  strings.TrimSpace(s) == ""
	len(strings.Fields(s)) != 0  →  strings.TrimSpace(s) != ""
	len(strings.Fields(s)) >  0  →  strings.TrimSpace(s) != ""
	len(strings.Fields(s)) >= 1  →  strings.TrimSpace(s) != ""

The rewrite is BIT-IDENTICAL for every input. len(strings.Fields(s))
is 0 exactly when s has no field, i.e. every rune of s is white space
(or s is empty); strings.TrimSpace(s) is "" under exactly the same
condition. Both functions classify runes with the IDENTICAL whitespace
set: each has an ASCII fast path driven by the same asciiSpace table
('\t', '\n', '\v', '\f', '\r', ' ') and each falls back to
unicode.IsSpace for non-ASCII input (Fields via FieldsFunc, TrimSpace
via TrimFunc), and asciiSpace equals unicode.IsSpace restricted to
r < utf8.RuneSelf — the two can never disagree on any rune. The edges
hold: the empty string (Fields yields an empty slice, TrimSpace
returns ""), all-ASCII white space, non-ASCII spaces such as NBSP
U+00A0 and NEL U+0085 (IsSpace-true in both), and invalid UTF-8 (a bad
byte decodes to utf8.RuneError, which is not a space in either
function, so a run of invalid bytes is a non-empty field in both).
Both sides evaluate s exactly once in the same position, take and
return plain string values (a named string type cannot reach
strings.Fields without an explicit conversion, and that conversion
carries over verbatim), neither can panic, and both comparisons are
untyped-bool binary expressions, so any context that accepted the
Before shape accepts the After shape unchanged.

The automatic fix keeps the argument expression byte-verbatim — same
text, same evaluation order — and rewrites only the scaffolding around
it: the len( and Fields( prefix collapses to TrimSpace( (reusing the
original, possibly aliased, package qualifier), and the integer
comparison becomes == "" or != "". TrimSpace lives in the same package
as Fields, so the import can never be orphaned. A comparison replaces
a comparison of the same precedence, so every position that parsed
before parses identically after and no parentheses are ever needed. A
comment inside the rewritten scaffolding withholds the fix (the report
stays advisory) rather than destroying the comment.

Comparisons that genuinely need the field count are left alone:
len(Fields(s)) == 1, > 1, >= 2 and friends, a Fields result stored in
a variable or used for anything but this comparison, and any
comparison against a non-literal (a variable or named constant holding
0 is not matched). The callee is pinned with type information to the
package-level strings.Fields and the len to the predeclared builtin —
a shadowed strings or len, or a local function or method named Fields,
never matches. bytes.Fields is out of scope: []byte has no == ""
comparison, so its blank test has no drop-in TrimSpace spelling.`,
		Before: `if len(strings.Fields(s)) == 0 {
	return errBlank
}`,
		After: `if strings.TrimSpace(s) == "" {
	return errBlank
}`,
		MeasuredWin: `BenchmarkPS2028 (Apple M2 Pro, go1.26): a ~1.2 KB
line of 64 space-separated fields drops from 1531 ns/op, 1152 B/op,
1 alloc/op to 2.7 ns/op, 0 B/op, 0 allocs/op (~570x) — TrimSpace
stops at the first non-space byte while Fields materializes all 64
fields. An all-whitespace 256-byte input drops from 215 ns/op to
123 ns/op (~1.7x, both alloc-free: Fields' zero-length make does not
allocate); both sides must scan a blank input fully, so the remaining
win there is Fields' per-byte counting work.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2028",
		Doc:  `len(strings.Fields(s)) compared against 0 or 1 allocates every field just to test blankness; strings.TrimSpace(s) == "" answers it with zero allocations`,
		Run:  runPS2028,
	},
})

func runPS2028(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			bin, ok := n.(*ast.BinaryExpr)
			if !ok {
				return true
			}
			m := ps2028Match(pass, bin)
			if m == nil {
				return true
			}
			msg := m.beforeText + " allocates every whitespace-separated field just to test blankness; " +
				m.afterText + " is the identical boolean with zero allocations and stops at the first non-space byte"
			diag := analysis.Diagnostic{
				Pos:     bin.Pos(),
				End:     bin.End(),
				Message: msg,
			}
			if ps2111CommentIn(f, bin.Pos(), m.arg.Pos()) || ps2111CommentIn(f, m.arg.End(), bin.End()) {
				// A comment sits inside the scaffolding the fix would
				// delete: withhold the fix rather than destroy the comment.
				diag.Message = msg + "; a comment inside the rewritten syntax withholds the automatic fix — rewrite by hand"
			} else {
				// The argument expression carries over byte-verbatim; only
				// the scaffolding on either side of it is rewritten. The
				// replacement comparison has the same precedence as the
				// replaced one, so no position ever needs parentheses.
				diag.SuggestedFixes = []analysis.SuggestedFix{{
					Message: "test blankness with " + m.qual + ".TrimSpace instead of counting fields",
					TextEdits: []analysis.TextEdit{
						{Pos: bin.Pos(), End: m.arg.Pos(), NewText: []byte(m.qual + ".TrimSpace(")},
						{Pos: m.arg.End(), End: bin.End(), NewText: []byte(") " + m.cmpOp + ` ""`)},
					},
				}}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}

// ps2028Site is a matched blank-string test: the verbatim argument of
// the Fields call, the (possibly aliased) package qualifier, the
// comparison operator of the rewrite ("==" for the is-blank shapes,
// "!=" for has-a-field), and the message's before/after renderings.
type ps2028Site struct {
	arg        ast.Expr
	qual       string
	cmpOp      string
	beforeText string
	afterText  string
}

// ps2028Match matches a binary comparison whose one operand is
// len(strings.Fields(s)) — len the predeclared builtin, Fields the
// package-level strings.Fields pinned by type information — and whose
// other operand is the literal integer constant 0 or 1, in one of the
// six blankness-equivalent shapes. Both operand orders are matched.
func ps2028Match(pass *analysis.Pass, bin *ast.BinaryExpr) *ps2028Site {
	switch bin.Op {
	case token.EQL, token.NEQ, token.LSS, token.LEQ, token.GTR, token.GEQ:
	default:
		return nil
	}
	arg, qual, sideOK := ps2028LenFields(pass, bin.X)
	litOnLeft := false
	if !sideOK {
		arg, qual, sideOK = ps2028LenFields(pass, bin.Y)
		if !sideOK {
			return nil
		}
		litOnLeft = true
	}
	var litSide ast.Expr
	if litOnLeft {
		litSide = bin.X
	} else {
		litSide = bin.Y
	}
	litVal, isLit := ps5104ZeroOneLit(pass, litSide)
	if !isLit {
		return nil
	}
	// Normalize to the call-on-left spelling: `0 < len(...)` means
	// `len(...) > 0`, so mirror the operator when the literal is on the
	// left, then classify against the six membership shapes — exactly
	// PS5104's mapping, with "has at least one field" standing in for
	// membership. Any other (op, literal) pair genuinely needs the count.
	op := bin.Op
	if litOnLeft {
		op = ps5104Mirror(op)
	}
	negate, ok := ps5104Membership(op, litVal)
	if !ok {
		return nil
	}
	cmpOp := "!=" // has at least one field
	if negate {
		cmpOp = "==" // blank: no fields at all
	}
	beforeText, okText := ps5004ExprText(bin)
	if !okText {
		return nil
	}
	argText, okText := ps5004ExprText(arg)
	if !okText {
		return nil
	}
	return &ps2028Site{
		arg:        arg,
		qual:       qual,
		cmpOp:      cmpOp,
		beforeText: beforeText,
		afterText:  qual + ".TrimSpace(" + argText + ") " + cmpOp + ` ""`,
	}
}

// ps2028LenFields matches e (modulo parentheses) as
// len(strings.Fields(arg)), returning the argument expression and the
// source spelling of the package qualifier (which reuses an import
// alias verbatim). The len identifier must resolve to the predeclared
// builtin and the Fields selector to the receiver-less package-level
// strings.Fields — a shadowed len or strings, or a local function,
// variable, field or method spelled Fields, resolves elsewhere and is
// rejected. The argument must feed the comparison DIRECTLY: a Fields
// slice stored in a variable first may have other consumers and is out
// of scope.
func ps2028LenFields(pass *analysis.Pass, e ast.Expr) (arg ast.Expr, qual string, ok bool) {
	lenCall, isCall := ps2108Unparen(e).(*ast.CallExpr)
	if !isCall || len(lenCall.Args) != 1 || lenCall.Ellipsis.IsValid() {
		return nil, "", false
	}
	lenIdent, isIdent := ps2108Unparen(lenCall.Fun).(*ast.Ident)
	if !isIdent || pass.TypesInfo.Uses[lenIdent] != types.Universe.Lookup("len") {
		return nil, "", false
	}
	inner, isCall := ps2108Unparen(lenCall.Args[0]).(*ast.CallExpr)
	if !isCall || len(inner.Args) != 1 || inner.Ellipsis.IsValid() {
		return nil, "", false
	}
	sel, isSel := ps2108Unparen(inner.Fun).(*ast.SelectorExpr)
	if !isSel {
		return nil, "", false
	}
	fn, isFunc := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !isFunc || fn.Name() != "Fields" || fn.Pkg() == nil || fn.Pkg().Path() != "strings" {
		return nil, "", false
	}
	if sig, isSig := fn.Type().(*types.Signature); !isSig || sig.Recv() != nil {
		return nil, "", false
	}
	pkgIdent, isIdent := ps2108Unparen(sel.X).(*ast.Ident)
	if !isIdent {
		return nil, "", false
	}
	return inner.Args[0], pkgIdent.Name, true
}
