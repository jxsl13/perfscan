package checks

import (
	"go/ast"
	"go/token"
	"go/types"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS2044 reports fmt.Appendf(buf, "%s=%s;", k, v) — a format that splices
// plain strings into literal text through bare %s verbs — where the nested
// builtin chain append(append(append(append(buf, k...), "="...), v...), ";"...)
// writes the identical bytes with no fmt machinery. The interleaved-literal
// sibling of PS2033 (which owns the pure ("%s")^k format) and the
// Appendf-destination twin of PS2034 (Sprintf splice -> + concatenation);
// the lone bare %s belongs to PS2141, the verbless constant format to PS3025.
var PS2044 = register(&lint.Check{
	ID:       "PS2044",
	Category: "alloc",
	Slug:     "appendf-splice-append",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "fmt.Appendf splicing plain strings into literal text runs fmt's formatter; a nested append chain writes the same bytes directly",
		Text: `fmt.Appendf parses its format string, boxes EACH operand into an
interface (each box is a heap allocation when it escapes into the printer),
walks fmt's formatter state machine through a pooled pp buffer, and finally
copies that buffer into buf. When the format is nothing but literal text
spliced with bare %s verbs over plain strings, the result is byte-for-byte
the nested builtin chain that appends the literal segments and the operands
in order: append(append(append(append(buf, k...), "="...), v...), ";"...).
The builtin append has a dedicated string->[]byte special case, so the chain
needs no fmt, no pool, no reflection and no allocation beyond buf's own
growth — literal segments are appended as constant-string spreads. PS2033
owns the pure ("%s")^k format with no literal text, PS2141 the lone bare
"%s", PS3025 the verbless constant format, and PS5015 the single scalar
verb; this check owns the interleaved-literal %s splice.

The match is deliberately narrow — it is the whole safety story. The callee
must resolve via type information to the package-level fmt.Appendf, the call
must not spread its arguments (no ...), and the format must be a string
literal consisting ONLY of literal text and bare %s verbs, with the verb
count matching the operand count exactly: any flag, width, precision, %% or
any other verb disqualifies it. At least one literal segment must be
non-empty (the all-verbs format is PS2033's or PS2141's). Three further
gates keep the fix bit-identical, each pinned by a divergence this check's
equivalence suite reproduces:

  - Every operand's type must be EXACTLY the predeclared string (untyped
    constants included). A NAMED string type could carry String()/Format()
    that %s honors and append would not. A []byte operand — although %s
    accepts it and single-operand PS2141 fixes it — is EXCLUDED even
    unnamed: if its bytes alias buf's spare capacity, an earlier append in
    the chain overwrites them before a later append reads them, while
    fmt.Appendf reads every operand before touching buf (measured
    divergence: "abXY=cd" vs "abXY=XY").
  - Operands evaluated AFTER the chain's first write to buf must be inert —
    an identifier, a string literal, or a package-qualified identifier —
    because the chain writes into buf's capacity between operand
    evaluations, while fmt.Appendf writes only after evaluating everything.
    A later operand that runs user code (any call) could observe buf's
    array through another slice mid-rewrite (measured divergence: "XXa=XX"
    vs "XXa==X"). With an EMPTY leading segment the chain's first write
    happens after the first operand is evaluated, so the FIRST operand may
    be any expression of type string (exactly PS2033's rule); with a
    NON-EMPTY leading segment that literal is written BEFORE any operand is
    evaluated, so EVERY operand must be inert (measured divergence: "XXLX"
    vs "XXLL" for fmt.Appendf(buf, "L%s", f()) when f observes buf's
    array). Evaluation order and count of buf and every operand are
    otherwise identical (left-to-right, exactly once).
  - buf must be an unnamed []byte so the chain reproduces fmt.Appendf's
    []byte return type exactly. The resulting slice's length and bytes are
    identical; only its unobserved spare CAPACITY may differ (the chain may
    grow more than once), the same standard PS2141/PS2033 ship under.

Shapes outside the gates — named operand or destination types, []byte
operands, non-inert operands after the first write — keep an advisory
report with no fix. A NIL-LITERAL destination is not reported at all:
the builtin append requires a typed slice as its first argument, so
append(nil, ...) does not compile and the chain advice is unspellable
there — fmt.Appendf(nil, splice) wants a different rewrite (a + concat
converted once), which is not this check's shape. (This differs from
PS5015, whose strconv.Append*(nil, ...) rewrite accepts nil at its typed
[]byte parameter and therefore keeps the nil-dest advisory.) %s writes a
plain string verbatim (invalid UTF-8 and
empties included; a string is never nil), matching append byte for byte,
and each literal segment is re-emitted with strconv.Quote of its DECODED
value, so escape sequences, raw-string formats and non-ASCII text all keep
an identical runtime value. The replacement is a call expression — a
primary — so it never needs parentheses in any context. Each rewrite
removes the file's fmt.Appendf selector; the fix applies even when that
removes the file's last fmt reference — the fix pipeline prunes the
now-unused fmt import — EXCEPT in cgo files (import "C"), whose import
block is never edited: there the fixes are withheld and the report stays
advisory. A comment inside the rewritten scaffolding is never silently
dropped: it suppresses the fix and keeps the report.`,
		Before: `buf = fmt.Appendf(buf, "%s=%s;", k, v)`,
		After:  `buf = append(append(append(append(buf, k...), "="...), v...), ";"...)`,
		MeasuredWin: `BenchmarkPS2044 (a "%s=%s;" splice of a 12-byte key and a
15-byte value into a preallocated buffer, Apple M2 Pro):
fmt.Appendf(buf, "%s=%s;", k, v) ~72.4 ns/op, 32 B/op, 2 allocs/op vs
append(append(append(append(buf, k...), "="...), v...), ";"...)
~3.8 ns/op, 0 B/op, 0 allocs/op (~19x faster, and the two allocations —
one interface box per string operand — disappear; both write into the
buffer's existing capacity).`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2044",
		Doc:  "fmt.Appendf splicing plain strings into literal text with bare %s verbs; a nested append chain is identical and cheaper",
		Run:  runPS2044,
	},
})

func runPS2044(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		// Collect first: applying all fixes may rewrite the file's last fmt
		// reference and orphan the import. The fix pipeline prunes such an
		// orphan afterwards — except in a cgo file (import "C"), whose import
		// block is never edited, so there the fixes are withheld and the
		// reports stay advisory (same guard as PS2034/PS2035/PS2036).
		type site struct {
			call *ast.CallExpr
			fix  *analysis.SuggestedFix
		}
		var sites []site
		fixable := 0
		astutil.WithStack(f, func(n ast.Node, stack []ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) < 3 || call.Ellipsis.IsValid() {
				return true
			}
			if _, ok := astutil.PkgFuncCall(pass.TypesInfo, call.Fun, "fmt", map[string]bool{"Appendf": true}); !ok {
				return true
			}
			// The format must be a string LITERAL made only of literal text
			// and bare %s verbs — anything else (%%, a flag, a width, another
			// verb, a variable format) disqualifies. ps2034Split scans the
			// DECODED text, so a "\x25d" smuggling a %d is rejected too.
			lit, ok := call.Args[1].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			format, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			operands := call.Args[2:]
			// len(segs)-1 is the verb count; it must equal the operand count
			// (>= 1 here since len(call.Args) >= 3).
			segs, ok := ps2034Split(format)
			if !ok || len(segs) != len(operands)+1 {
				return true
			}
			// At least one literal segment must be non-empty: the pure
			// ("%s")^k format is PS2033's and the lone bare "%s" PS2141's.
			hasText := false
			for _, seg := range segs {
				if seg != "" {
					hasText = true
					break
				}
			}
			if !hasText {
				return true
			}
			// Only report %s over strings/byte slices — the shape append can
			// reproduce at all. %s over any other type (an int, a
			// Stringer-less struct) is different formatting, not this pattern.
			for _, arg := range operands {
				if !ps2141Stringish(pass.TypesInfo.TypeOf(arg)) {
					return true
				}
			}
			// The destination must be a []byte-underlying expression (named
			// stays advisory below). A NIL literal is silent, not advisory:
			// the builtin append cannot take an untyped nil first argument,
			// so the advised chain is unspellable for that shape.
			if !ps2044ByteSliceUnderlying(pass.TypesInfo.TypeOf(call.Args[0])) {
				return true
			}
			// The fix demands the provably bit-identical subset: unnamed
			// []byte destination, every operand EXACTLY the predeclared
			// string, and every operand evaluated after the chain's first
			// write to buf inert.
			fix := (*analysis.SuggestedFix)(nil)
			if ps2044FixableShape(pass.TypesInfo, call.Args[0], operands, segs) {
				fix = ps2044Fix(f, call, segs)
				if fix != nil {
					fixable++
				}
			}
			sites = append(sites, site{call, fix})
			return true
		})
		// Each fixable call holds exactly one fmt reference (its selector's
		// package identifier); if those are ALL of the file's fmt references,
		// the fixes orphan the import — fine in a non-cgo file (the fix
		// pipeline prunes it), advisory only in a cgo file.
		emitFixes := fixable > 0 && (pkgRefCount(pass, f, "fmt") > fixable || !ps2110ImportsC(f))
		for _, st := range sites {
			diag := analysis.Diagnostic{
				Pos:     st.call.Pos(),
				End:     st.call.End(),
				Message: "fmt.Appendf splicing plain strings into literal text boxes every operand and walks fmt's formatter state machine; a nested append chain with the literal segments writes the identical bytes directly",
			}
			if emitFixes && st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps2044ByteSliceUnderlying reports whether t's underlying type is a slice of
// a byte-kinded element — the destinations for which the append chain is
// spellable at all. An untyped nil is not (append(nil, ...) does not
// compile); a NAMED []byte or a []NamedByte is, but stays advisory (the
// unnamed gate lives in ps2044FixableShape).
func ps2044ByteSliceUnderlying(t types.Type) bool {
	if t == nil {
		return false
	}
	sl, ok := t.Underlying().(*types.Slice)
	if !ok {
		return false
	}
	b, ok := sl.Elem().Underlying().(*types.Basic)
	return ok && b.Kind() == types.Byte
}

// ps2044FixableShape reports whether the destination and operands are inside
// the bit-identical subset: unnamed []byte destination, every operand exactly
// the predeclared string, and every operand evaluated after the chain's first
// write to buf inert. With an empty leading segment the first write happens
// after the FIRST operand's evaluation, so that operand may be any string
// expression (PS2033's rule); with a non-empty leading segment the constant
// is appended before any operand is evaluated, so EVERY operand must be inert.
func ps2044FixableShape(info *types.Info, dest ast.Expr, operands []ast.Expr, segs []string) bool {
	if !ps2141ByteSliceUnnamed(info.TypeOf(dest)) {
		return false
	}
	leadingWrite := segs[0] != ""
	for i, arg := range operands {
		if !ps2033PlainString(info.TypeOf(arg)) {
			return false
		}
		if (i > 0 || leadingWrite) && !ps2033Inert(info, arg) {
			return false
		}
	}
	return true
}

// ps2044Fix rewrites fmt.Appendf(buf, "%s=%s;", k, v) to
// append(append(append(append(buf, k...), "="...), v...), ";"...): buf and
// every operand stay byte-verbatim in place; only the scaffolding is edited.
// The chain nests one append per operand and per NON-EMPTY literal segment,
// in the format's interleaved order; each non-empty segment is re-emitted
// with strconv.Quote of its decoded value, which always denotes the identical
// runtime string. A comment inside a rewritten scaffolding span would be
// destroyed — the fix is withheld then and the report stays advisory.
func ps2044Fix(f *ast.File, call *ast.CallExpr, segs []string) *analysis.SuggestedFix {
	args := call.Args
	last := len(args) - 1
	// The edited spans: the callee itself, buf..firstOperand (the format
	// literal and its commas), each inter-operand comma (which becomes a
	// chain link, possibly carrying a literal-segment append), and
	// lastOperand..) (where the final spreads land).
	sel := call.Fun.(*ast.SelectorExpr)
	if ps2111CommentIn(f, sel.Pos(), sel.End()) ||
		ps2111CommentIn(f, args[0].End(), args[2].Pos()) ||
		ps2111CommentIn(f, args[last].End(), call.End()) {
		return nil
	}
	for i := 2; i < last; i++ {
		if ps2111CommentIn(f, args[i].End(), args[i+1].Pos()) {
			return nil
		}
	}
	// One nested append per operand and per non-empty literal segment; the
	// innermost level reuses the call's own parentheses.
	appends := len(args) - 2
	for _, seg := range segs {
		if seg != "" {
			appends++
		}
	}
	edits := make([]analysis.TextEdit, 0, len(args))
	edits = append(edits, analysis.TextEdit{
		Pos: sel.Pos(), End: sel.End(),
		NewText: []byte(strings.Repeat("append(", appends-1) + "append"),
	})
	// buf..firstOperand: the leading literal segment (if any) becomes the
	// innermost append's second argument, closing that level before the
	// first operand opens the next.
	lead := ", "
	if segs[0] != "" {
		lead = ", " + strconv.Quote(segs[0]) + "...), "
	}
	edits = append(edits, analysis.TextEdit{Pos: args[0].End(), End: args[2].Pos(), NewText: []byte(lead)})
	for i := 2; i < last; i++ {
		mid := "...), "
		if seg := segs[i-1]; seg != "" {
			mid = "...), " + strconv.Quote(seg) + "...), "
		}
		edits = append(edits, analysis.TextEdit{Pos: args[i].End(), End: args[i+1].Pos(), NewText: []byte(mid)})
	}
	trail := "...)"
	if seg := segs[len(segs)-1]; seg != "" {
		trail = "...), " + strconv.Quote(seg) + "...)"
	}
	edits = append(edits, analysis.TextEdit{Pos: args[last].End(), End: call.End(), NewText: []byte(trail)})
	return &analysis.SuggestedFix{
		Message:   "replace fmt.Appendf with a nested append chain over the literal segments and operands",
		TextEdits: edits,
	}
}
