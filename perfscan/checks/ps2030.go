package checks

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS2030 reports len(bytes.Fields(b)) compared against the literal 0
// or 1 to answer a blank-slice question — "is there at least one
// field?" / "is b nothing but white space?" — where the Fields call
// allocates the entire [][]byte of fields and always scans the whole
// input just to have its length compared and the slice thrown away.
// len(bytes.TrimSpace(b)) == 0 (resp. != 0) is the same boolean with
// zero allocations, and for the common non-blank input it stops at the
// first non-space byte. This is the []byte twin of PS2028
// (strings.Fields): []byte has no == "" comparison, so the blank test
// keeps its len() wrapper and only the Fields identifier changes.
var PS2030 = register(&lint.Check{
	ID:       "PS2030",
	Category: "alloc",
	Slug:     "len-bytes-fields-blank-test",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: `len(bytes.Fields(b)) == 0 allocates every field just to test blankness; len(bytes.TrimSpace(b)) == 0 is allocation-free`,
		Text: `bytes.Fields allocates a [][]byte holding EVERY
whitespace-delimited field of the input (one slice allocation carrying
a three-word header per field) and always scans the entire slice.
Comparing only its len against 0 or 1 throws that slice away to answer
a single boolean: "does b contain at least one non-space rune?".
bytes.TrimSpace answers the identical question with zero allocations —
it returns a re-slice of b — and for the common non-blank input it
short-circuits at the FIRST non-space byte from the left instead of
walking every field.

Exactly six comparison shapes are blankness-equivalent, and only those
are matched (with the literal on either side of the operator):

	len(bytes.Fields(b)) == 0  →  len(bytes.TrimSpace(b)) == 0
	len(bytes.Fields(b)) <  1  →  len(bytes.TrimSpace(b)) <  1
	len(bytes.Fields(b)) <= 0  →  len(bytes.TrimSpace(b)) <= 0
	len(bytes.Fields(b)) != 0  →  len(bytes.TrimSpace(b)) != 0
	len(bytes.Fields(b)) >  0  →  len(bytes.TrimSpace(b)) >  0
	len(bytes.Fields(b)) >= 1  →  len(bytes.TrimSpace(b)) >= 1

The rewrite is BIT-IDENTICAL for every input. len(bytes.Fields(b)) is
0 exactly when b has no field, i.e. every rune of b is white space (or
b is empty or nil); len(bytes.TrimSpace(b)) is 0 under exactly the
same condition. Both functions classify runes with the IDENTICAL
whitespace set: each has an ASCII fast path driven by the same
asciiSpace table ('\t', '\n', '\v', '\f', '\r', ' ') and each falls
back to unicode.IsSpace for non-ASCII input (Fields via FieldsFunc,
TrimSpace via TrimFunc), and asciiSpace equals unicode.IsSpace
restricted to r < utf8.RuneSelf — the two can never disagree on any
rune. The edges hold: nil and the empty slice (Fields yields an empty
slice, TrimSpace a zero-length re-slice — len 0 either way), all-ASCII
white space, non-ASCII spaces such as NBSP U+00A0 and NEL U+0085
(IsSpace-true in both), and invalid UTF-8 (a bad byte decodes to
utf8.RuneError, which is not a space in either function, so a run of
invalid bytes is a non-empty field in both — both lens nonzero). The
crucial restriction is the six zero/nonzero shapes above: only there
is the nonzero MAGNITUDE irrelevant, since len(Fields) counts fields
while len(TrimSpace) counts bytes — len == 2, > 1 and friends
genuinely need the field count and are never matched.

The automatic fix is a single-identifier rename: the sole edited token
is the Fields selector, which becomes TrimSpace. The argument b, the
len() wrapper, the comparison operator and the literal all stay
byte-verbatim, so b is evaluated exactly once in the same position
either way, both parameters are the identical []byte (a named type
assignable to bytes.Fields' parameter is assignable to TrimSpace's
unchanged), len applies to the [][]byte before and the []byte after,
neither side can panic, and no parentheses are ever needed. TrimSpace
lives in the same package as Fields (the original, possibly aliased,
qualifier is untouched), so the import can never be orphaned, and a
single-identifier edit can never contain — let alone destroy — a
comment.

Comparisons that genuinely need the field count are left alone:
len(Fields(b)) == 1, > 1, >= 2 and friends, a Fields result stored in
a variable or used for anything but this comparison, and any
comparison against a non-literal (a variable or named constant holding
0 is not matched). The callee is pinned with type information to the
package-level bytes.Fields and the len to the predeclared builtin — a
shadowed bytes or len, or a local function or method named Fields,
never matches. strings.Fields is PS2028's territory, where the whole
len scaffolding collapses to a == "" comparison instead.`,
		Before: `if len(bytes.Fields(b)) == 0 {
	return errBlank
}`,
		After: `if len(bytes.TrimSpace(b)) == 0 {
	return errBlank
}`,
		MeasuredWin: `BenchmarkPS2030 (Apple M2 Pro, go1.26): a ~1.2 KB
line of 64 space-separated fields drops from 1586 ns/op, 1792 B/op,
1 alloc/op to 3.1 ns/op, 0 B/op, 0 allocs/op (~510x) — TrimSpace
stops at the first non-space byte while Fields materializes all 64
field headers. An all-whitespace 256-byte input drops from 222 ns/op
to 126 ns/op (~1.8x, both alloc-free: Fields' zero-length make does
not allocate); both sides must scan a blank input fully, so the
remaining win there is Fields' per-byte field-boundary counting.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2030",
		Doc:  `len(bytes.Fields(b)) compared against 0 or 1 allocates every field just to test blankness; len(bytes.TrimSpace(b)) answers it with zero allocations`,
		Run:  runPS2030,
	},
})

func runPS2030(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			bin, ok := n.(*ast.BinaryExpr)
			if !ok {
				return true
			}
			m := ps2030Match(pass, bin)
			if m == nil {
				return true
			}
			// The fix renames the single Fields identifier and nothing
			// else: a one-identifier span cannot contain a comment, the
			// argument and all scaffolding stay byte-verbatim, and a
			// single-ident replacement never needs parentheses — so the
			// fix is never withheld.
			pass.Report(analysis.Diagnostic{
				Pos: bin.Pos(),
				End: bin.End(),
				Message: m.beforeText + " allocates a [][]byte of every whitespace-separated field just to test blankness; " +
					m.afterText + " is the identical zero test with zero allocations and stops at the first non-space byte",
				SuggestedFixes: []analysis.SuggestedFix{{
					Message: "test blankness with " + m.qual + ".TrimSpace instead of counting fields",
					TextEdits: []analysis.TextEdit{
						{Pos: m.fields.Pos(), End: m.fields.End(), NewText: []byte("TrimSpace")},
					},
				}},
			})
			return true
		})
	}
	return nil, nil
}

// ps2030Site is a matched blank-slice test: the Fields selector
// identifier the fix renames, the (possibly aliased) package
// qualifier, and the message's before/after renderings.
type ps2030Site struct {
	fields     *ast.Ident
	qual       string
	beforeText string
	afterText  string
}

// ps2030Match matches a binary comparison whose one operand is
// len(bytes.Fields(b)) — len the predeclared builtin, Fields the
// package-level bytes.Fields pinned by type information — and whose
// other operand is the literal integer constant 0 or 1, in one of the
// six blankness-equivalent shapes. Both operand orders are matched.
func ps2030Match(pass *analysis.Pass, bin *ast.BinaryExpr) *ps2030Site {
	switch bin.Op {
	case token.EQL, token.NEQ, token.LSS, token.LEQ, token.GTR, token.GEQ:
	default:
		return nil
	}
	arg, fields, qual, sideOK := ps2030LenFields(pass, bin.X)
	litOnLeft := false
	if !sideOK {
		arg, fields, qual, sideOK = ps2030LenFields(pass, bin.Y)
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
	// membership. Any other (op, literal) pair genuinely needs the
	// count: len(TrimSpace) counts bytes, not fields, so only the
	// zero/nonzero shapes are equivalent.
	op := bin.Op
	if litOnLeft {
		op = ps5104Mirror(op)
	}
	if _, ok := ps5104Membership(op, litVal); !ok {
		return nil
	}
	beforeText, okText := ps5004ExprText(bin)
	if !okText {
		return nil
	}
	argText, okText := ps5004ExprText(arg)
	if !okText {
		return nil
	}
	litText, okText := ps5004ExprText(ps2108Unparen(litSide))
	if !okText {
		return nil
	}
	lenAfter := "len(" + qual + ".TrimSpace(" + argText + "))"
	var afterText string
	if litOnLeft {
		afterText = litText + " " + bin.Op.String() + " " + lenAfter
	} else {
		afterText = lenAfter + " " + bin.Op.String() + " " + litText
	}
	return &ps2030Site{
		fields:     fields,
		qual:       qual,
		beforeText: beforeText,
		afterText:  afterText,
	}
}

// ps2030LenFields matches e (modulo parentheses) as
// len(bytes.Fields(arg)), returning the argument expression, the
// Fields selector identifier (the sole token the fix rewrites) and the
// source spelling of the package qualifier (which reuses an import
// alias verbatim, for the message only). The len identifier must
// resolve to the predeclared builtin and the Fields selector to the
// receiver-less package-level bytes.Fields — a shadowed len or bytes,
// or a local function, variable, field or method spelled Fields,
// resolves elsewhere and is rejected. The argument must feed the
// comparison DIRECTLY: a Fields slice stored in a variable first may
// have other consumers and is out of scope.
func ps2030LenFields(pass *analysis.Pass, e ast.Expr) (arg ast.Expr, fields *ast.Ident, qual string, ok bool) {
	lenCall, isCall := ps2108Unparen(e).(*ast.CallExpr)
	if !isCall || len(lenCall.Args) != 1 || lenCall.Ellipsis.IsValid() {
		return nil, nil, "", false
	}
	lenIdent, isIdent := ps2108Unparen(lenCall.Fun).(*ast.Ident)
	if !isIdent || pass.TypesInfo.Uses[lenIdent] != types.Universe.Lookup("len") {
		return nil, nil, "", false
	}
	inner, isCall := ps2108Unparen(lenCall.Args[0]).(*ast.CallExpr)
	if !isCall || len(inner.Args) != 1 || inner.Ellipsis.IsValid() {
		return nil, nil, "", false
	}
	sel, isSel := ps2108Unparen(inner.Fun).(*ast.SelectorExpr)
	if !isSel {
		return nil, nil, "", false
	}
	fn, isFunc := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !isFunc || fn.Name() != "Fields" || fn.Pkg() == nil || fn.Pkg().Path() != "bytes" {
		return nil, nil, "", false
	}
	if sig, isSig := fn.Type().(*types.Signature); !isSig || sig.Recv() != nil {
		return nil, nil, "", false
	}
	pkgIdent, isIdent := ps2108Unparen(sel.X).(*ast.Ident)
	if !isIdent {
		return nil, nil, "", false
	}
	return inner.Args[0], sel.Sel, pkgIdent.Name, true
}
