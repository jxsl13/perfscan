package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS2047 reports append(dst, strconv.Itoa(n)...) and its Format*/Quote*
// siblings — a freshly formatted string spread-appended into a byte
// slice — when the matching strconv.Append* function formats straight
// into dst. The append-spread analog of PS2136 (whose []byte-conversion
// form covers the same formatter family), sharing its verb table.
var PS2047 = register(&lint.Check{
	ID:       "PS2047",
	Category: "alloc",
	Slug:     "strconv-append-spread",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "append(dst, strconv.Itoa(n)...) formats a throwaway string then copies it; strconv.AppendInt writes the bytes into dst directly",
		Text: `strconv.Itoa and every strconv.Format*/Quote* function build their
result in a scratch buffer and return it as a STRING — a heap allocation
plus a copy for any value outside strconv's tiny small-int cache — and
append(dst, s...) then copies those same bytes a SECOND time into dst's
tail. The go1.22 zero-copy append optimization that neutralizes the
[]byte(s) spread does NOT help here: the formatter allocates its string
internally regardless of what the caller does with it. Each formatter
has an Append* twin — in the standard library the string form IS the
string of the Append* form's bytes — that runs the identical formatting
code straight into dst's backing array: same bytes, no intermediate
string, no second copy.

  append(dst, strconv.Itoa(n)...)             -> strconv.AppendInt(dst, int64(n), 10)
  append(dst, strconv.FormatInt(x, b)...)     -> strconv.AppendInt(dst, x, b)
  append(dst, strconv.FormatUint(x, b)...)    -> strconv.AppendUint(dst, x, b)
  append(dst, strconv.FormatFloat(f, m, p, s)...) -> strconv.AppendFloat(dst, f, m, p, s)
  append(dst, strconv.FormatBool(v)...)       -> strconv.AppendBool(dst, v)
  append(dst, strconv.Quote(s)...)            -> strconv.AppendQuote(dst, s)   (and the
                                                 QuoteToASCII/QuoteToGraphic/QuoteRune*
                                                 variants likewise)

The rewrite preserves every observable value the project's differential
pins (see equiv_PS2047_test.go):

  - BYTES and LENGTH: strconv documents each Format*/Quote*/Itoa result
    as the string of its Append* twin's bytes; both spellings append
    byte-for-byte the same k bytes over every value, base, float verb
    (including unrecognized verbs), precision, bitSize, NaN/±Inf/-0.0,
    denormals, bool, and invalid-UTF-8 quote input.
  - NIL-NESS: every matched formatter renders at least one byte (a
    digit, "true"/"false", or the opening quote), so a nil dst becomes
    non-nil in both spellings and there is no nil-vs-empty corner.
  - EVALUATION: dst is evaluated once, then the formatter's arguments
    once, in the same order, in both spellings — the fix applies even
    when an argument is a side-effecting call. An illegal integer base
    or float bitSize panics with the identical strconv panic in both.
  - IN-PLACE: when dst has spare capacity for the whole output both
    spellings write the same backing array in place and preserve its
    capacity exactly.

When append must GROW, the fresh slice's capacity is an unspecified
implementation detail (the sign/float/quote formatters append into dst
piecewise, so the growth steps — and on some paths a scratch byte
beyond len(dst) in the abandoned old array — can differ from the single
grow-and-copy of the spread). That is the accepted PS2112 class this
project's dst-position rewrites already carry (PS2035/PS2036/PS5015
rewrite fmt.Appendf(buf, ...) to these same strconv.Append* calls, and
PS2044 emits piecewise append chains): contents, length, nil-ness, and
aliasing with live slices are preserved always; no correct program
relies on the rounding of — or the scratch bytes behind — a freshly
grown backing array.

Only the predeclared append builtin with exactly two arguments, a
spread argument that is (up to parentheses) a call type-pinned to the
standard library's strconv, and a dst whose static type is exactly the
unnamed []byte are rewritten. A NAMED byte-slice dst compiles with
append — which returns dst's own type — but strconv.Append* returns
[]byte, so the rewrite would change the expression's static type (and
break a method call or %T on the result): it keeps an advisory report,
as does a generic dst typed by a ~[]byte type parameter. The fix keeps
dst and every formatter argument byte-verbatim in place and edits only
the scaffolding, so an aliased strconv import keeps its qualifier and
no import is ever added or orphaned (Append* lives in strconv too).
Itoa is the one special case: it has no direct Append twin, so the
argument is wrapped in a value-preserving int64(...) conversion and the
explicit base 10 is appended — strconv.Itoa is defined as
FormatInt(int64(i), 10). A comment inside the rewritten scaffolding
suppresses the fix and keeps the advisory report.`,
		Before: `dst = append(dst, strconv.Itoa(n)...)`,
		After:  `dst = strconv.AppendInt(dst, int64(n), 10)`,
		MeasuredWin: `BenchmarkPS2047 (13-digit int64 appended into a reset
preallocated []byte, Apple M2 Pro, go1.26): 24.5 ns/op, 16 B/op,
1 alloc/op -> 12.1 ns/op, 0 B/op, 0 allocs/op (~2x, alloc-free) — the
intermediate string allocation and its copy removed; the gap widens
with output length since the saved copy is O(len).`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2047",
		Doc:  "append(dst, strconv.Itoa/Format*/Quote*(...)...) instead of strconv.Append*(dst, ...)",
		Run:  runPS2047,
	},
})

func runPS2047(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 2 || !call.Ellipsis.IsValid() {
				return true
			}
			// The callee must be the predeclared append builtin, not a
			// shadowing local.
			fn, ok := call.Fun.(*ast.Ident)
			if !ok {
				return true
			}
			if b, ok := pass.TypesInfo.Uses[fn].(*types.Builtin); !ok || b.Name() != "append" {
				return true
			}
			// The spread argument must be (up to parentheses) a call of a
			// strconv formatter with an Append* twin — the same verb table
			// as PS2136.
			inner, ok := ps2109Unparen(call.Args[1]).(*ast.CallExpr)
			if !ok || inner.Ellipsis.IsValid() {
				return true
			}
			name, ok := astutil.PkgFuncCall(pass.TypesInfo, inner.Fun, "strconv", nil)
			if !ok {
				return true
			}
			verb, known := ps2136Append[name]
			if !known || len(inner.Args) != verb.argc {
				return true
			}
			dst := call.Args[0]
			dstType := pass.TypesInfo.TypeOf(dst)
			if dstType == nil {
				return true
			}
			msg := "append(dst, strconv." + name + "(...)...) formats a throwaway string that append copies a second time; strconv." +
				verb.appendName + "(dst, ...) formats straight into dst"
			diag := analysis.Diagnostic{
				Pos:     call.Pos(),
				End:     call.End(),
				Message: msg,
			}
			if !types.Identical(dstType, types.NewSlice(types.Typ[types.Uint8])) {
				// append returns dst's own (named or generic) type;
				// strconv.Append* returns []byte — the rewrite would change
				// the expression's static type, so it stays advisory.
				diag.Message = msg + "; dst's static type is not the plain []byte and strconv." +
					verb.appendName + " returns []byte, so the rewrite would change the expression's static type — rewrite by hand if nothing relies on it"
			} else if fix := ps2047Fix(f, call, dst, inner, name, verb.appendName); fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*fix}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}

// ps2047Fix builds the strconv.Append*(dst, ...) rewrite, or returns nil
// when a comment sits inside the rewritten scaffolding and the report must
// stay advisory. dst and every formatter argument are kept byte-verbatim
// in place — same expressions, same single evaluation, same order — and
// only the scaffolding around them is edited: "append(" becomes
// "<strconv>.AppendX(", the ", <strconv>.FormatX(" span becomes ", " (or
// ", int64(" for Itoa), and the closing ")...)" span becomes ")" (or
// "), 10)" for Itoa). The qualifier is rendered from the source selector,
// so an aliased strconv import keeps its alias; no import is ever added
// (Append* lives in strconv, which the matched call already uses) and
// none can be orphaned.
func ps2047Fix(f *ast.File, call *ast.CallExpr, dst ast.Expr, inner *ast.CallExpr, name, appendName string) *analysis.SuggestedFix {
	// PkgFuncCall matched, so inner.Fun is a SelectorExpr whose qualifier
	// is an Ident resolving to strconv.
	qual := inner.Fun.(*ast.SelectorExpr).X.(*ast.Ident).Name
	firstArg := inner.Args[0]
	lastArg := inner.Args[len(inner.Args)-1]
	// A comment anywhere inside the rewritten scaffolding would be
	// silently destroyed — advisory then. (Comments inside dst or the
	// formatter arguments survive verbatim.)
	if ps2109CommentBetween(f, call.Pos(), dst.Pos()) ||
		ps2109CommentBetween(f, dst.End(), firstArg.Pos()) ||
		ps2109CommentBetween(f, lastArg.End(), call.End()) {
		return nil
	}
	sep, closing := ", ", ")"
	if name == "Itoa" {
		// Itoa(n) -> AppendInt(dst, int64(n), 10): wrap the original
		// argument text in the value-preserving int64(...) and add the
		// explicit base 10 (strconv.Itoa is FormatInt(int64(i), 10)).
		sep, closing = ", int64(", "), 10)"
	}
	return &analysis.SuggestedFix{
		Message: "replace with " + qual + "." + appendName + "(dst, ...)",
		TextEdits: []analysis.TextEdit{
			{Pos: call.Pos(), End: dst.Pos(), NewText: []byte(qual + "." + appendName + "(")},
			{Pos: dst.End(), End: firstArg.Pos(), NewText: []byte(sep)},
			{Pos: lastArg.End(), End: call.End(), NewText: []byte(closing)},
		},
	}
}
