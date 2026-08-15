package checks

import (
	"go/ast"
	"go/token"
	"go/types"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS2034 reports fmt.Sprintf("host=%s;port=%s", host, port) — a format that
// splices plain strings into literal text through bare %s verbs, outside a
// loop — where the direct concatenation "host=" + host + ";port=" + port
// produces the identical string without fmt's boxing or formatter machinery.
// The interleaved-literal sibling of PS2122 (which owns the pure ("%s")^k
// format); the in-loop shape belongs to PS2103, the lone bare verb with no
// literal text to PS2107/PS2130.
var PS2034 = register(&lint.Check{
	ID:       "PS2034",
	Category: "alloc",
	Slug:     "sprintf-splice-concat",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: `fmt.Sprintf splicing plain strings into literal text is a reflection-priced +; concatenate directly`,
		Text: `fmt.Sprintf parses its format string, boxes every argument into
an interface, and walks fmt's formatter state machine through a pooled pp
buffer — even when the format is nothing but literal text spliced with
bare %s verbs over plain strings. For that shape the result is
byte-for-byte the concatenation of the literal segments and the
arguments, and "host=" + host + ";port=" + port is a direct
compiler-emitted concatenation: the literal runs are compile-time
constants (adjacent constants fold), so the whole expression lowers to
one concatenation with a single result allocation — no format parse, no
boxing, no fmt at all.

The match is deliberately narrow — it is the whole safety story. The
callee must resolve via type information to the package-level
fmt.Sprintf (a shadowed fmt or a method named Sprintf does not), the
call must not spread its arguments (no ...), and the format must be a
string literal consisting ONLY of literal text and bare %s verbs, with
the verb count matching the argument count exactly: any flag, width,
precision, %% or any other verb disqualifies it (numeric and float
verbs genuinely need fmt). At least one literal segment must be
non-empty — the pure ("%s")^k format belongs to PS2122 and the lone
bare "%s" to PS2130. Calls inside a loop body belong to PS2103
(sprintf-concat-in-loop) and are excluded here to avoid double
reporting. Every value argument must have type EXACTLY the predeclared
string — a NAMED string type could implement fmt.Stringer or
fmt.Formatter, which %s would honor and + would not, so named types,
[]byte, error, interfaces and numbers all stay out. For plain strings
%s emits the value verbatim (invalid UTF-8 and empty preserved; a
string is never nil), making the rewrite bit-identical. Argument
evaluation order is unchanged: both the call and the + chain evaluate
left to right, each argument exactly once.

The fix keeps every argument byte-verbatim in place and edits only the
scaffolding: the fmt.Sprintf( prefix and the format literal become the
(optionally parenthesized) leading literal segment, each inter-argument
comma becomes " + " around the quoted literal segment between the two
verbs (or a bare " + " when that segment is empty), and the closing
parenthesis becomes the trailing segment. Each literal segment is
re-emitted with strconv.Quote of its DECODED value, so escape
sequences, raw-string formats and non-ASCII text all keep an identical
runtime value. String-typed operands are primaries or + chains, so the
operands never need parentheses; the WHOLE replacement is parenthesized
when the surrounding context binds tighter than + (an index, a slice, a
binary operand), exactly as PS2121 decides it. Each rewrite removes the
file's fmt.Sprintf selector; the fix applies even when that removes the
file's last fmt reference — the fix pipeline prunes the now-unused fmt
import — EXCEPT in cgo files (import "C"), whose import block is never
edited: there the fixes are withheld and the report stays advisory. A
comment inside the rewritten scaffolding is never silently dropped: it
suppresses the fix and keeps the report.`,
		Before: `key := fmt.Sprintf("host=%s;port=%s", host, port)`,
		After:  `key := "host=" + host + ";port=" + port`,
		MeasuredWin: `BenchmarkPS2034 (a "host=" + host + ";port=" + port
splice of a 24-byte host and a 5-byte port, once per op, Apple M2 Pro):
fmt.Sprintf("host=%s;port=%s", ...) 79.7 ns/op, 80 B/op, 3 allocs/op
vs "host=" + host + ";port=" + port 27.6 ns/op, 48 B/op, 1 alloc/op
(~2.9x faster, one allocation instead of three — the two interface
boxings and the fmt pp buffer round-trip disappear, leaving only the
result string).`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2034",
		Doc:  "fmt.Sprintf splicing plain strings into literal text with bare %s verbs; direct + concatenation is identical and cheaper",
		Run:  runPS2034,
	},
})

func runPS2034(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		// Collect first: applying all fixes may rewrite the file's last
		// fmt reference and orphan the import. The fix pipeline prunes
		// such an orphan afterwards — except in a cgo file (import "C"),
		// whose import block is never edited, so there the fixes are
		// withheld and the reports stay advisory (same guard as
		// PS2107/PS2122).
		type site struct {
			call *ast.CallExpr
			fix  *analysis.SuggestedFix
		}
		var sites []site
		fixable := 0
		astutil.WithStack(f, func(n ast.Node, stack []ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) < 2 || call.Ellipsis.IsValid() {
				return true
			}
			if _, ok := astutil.PkgFuncCall(pass.TypesInfo, call.Fun, "fmt", map[string]bool{"Sprintf": true}); !ok {
				return true
			}
			// The format must be a string LITERAL made only of literal
			// text and bare %s verbs — anything else (%%, a flag, a
			// width, another verb, a variable format) disqualifies.
			lit, ok := call.Args[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			format, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			// len(segs)-1 is the verb count; it must equal the value
			// argument count (>= 1 here since len(call.Args) >= 2).
			segs, ok := ps2034Split(format)
			if !ok || len(segs) != len(call.Args) {
				return true
			}
			// At least one literal segment must be non-empty: the pure
			// ("%s")^k format is PS2122's, the lone bare "%s" PS2130's.
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
			// The in-loop shape is PS2103's (sprintf-concat-in-loop);
			// reporting it here too would double-report the call.
			if _, inLoop := astutil.InLoop(stack); inLoop {
				return true
			}
			// Every value argument must be EXACTLY the predeclared string
			// (untyped constant strings default to it). A named string
			// type could implement fmt.Stringer/fmt.Formatter, which %s
			// honors and + would not — bit-identity requires plain string.
			for _, a := range call.Args[1:] {
				t := pass.TypesInfo.TypeOf(a)
				if t == nil || !types.Identical(types.Default(t), types.Typ[types.String]) {
					return true
				}
			}
			var parent ast.Node
			if len(stack) > 0 {
				parent = stack[len(stack)-1]
			}
			fix := ps2034Fix(f, call, segs, ps2121NeedsParens(parent, call))
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
				Message: "fmt.Sprintf splicing plain strings into literal text boxes every argument and walks fmt's formatter state machine; + concatenation with the literal segments builds the identical string",
			}
			if emitFixes && st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps2034Split splits a DECODED format string on its bare %s verbs, returning
// the literal segments around them (len(segs) == verbs+1). Any other use of
// % — another verb, %%, a flag, a width like %5s, a trailing % — returns
// ok=false: those either need fmt or (for %%) are simply excluded from this
// check's scope. The scan is over the decoded text, so a "\x25d" smuggling a
// %d past a syntactic filter is rejected the same as a literal %d.
func ps2034Split(format string) (segs []string, ok bool) {
	segs = make([]string, 0, 4)
	var cur strings.Builder
	cur.Grow(len(format))
	for i := 0; i < len(format); i++ {
		c := format[i]
		if c != '%' {
			cur.WriteByte(c)
			continue
		}
		if i+1 >= len(format) || format[i+1] != 's' {
			return nil, false
		}
		segs = append(segs, cur.String())
		cur.Reset()
		i++
	}
	segs = append(segs, cur.String())
	return segs, true
}

// ps2034Fix builds the "lead" + a + "mid" + b + "trail" replacement. Every
// argument stays byte-verbatim in place; only the scaffolding is edited —
// the fmt.Sprintf( prefix together with the format literal becomes the
// (optional) opening parenthesis plus the quoted leading segment, each
// inter-argument comma becomes " + " around the quoted segment between the
// two verbs (a bare " + " when that segment is empty), and the closing
// parenthesis of the call becomes the quoted trailing segment plus the
// (optional) closing one. Empty segments contribute nothing — adjacent verbs
// join with a plain " + ". Each non-empty segment is re-emitted with
// strconv.Quote of its decoded value, which always denotes the identical
// runtime string. String-typed operands are primaries or + chains (the only
// string-producing binary operator is the associative + itself), so the
// operands themselves never need parentheses. A comment inside any
// scaffolding span would be destroyed by the edits — the fix is withheld
// then and the report stays advisory.
func ps2034Fix(f *ast.File, call *ast.CallExpr, segs []string, needParens bool) *analysis.SuggestedFix {
	args := call.Args[1:]
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
	lead := open
	if segs[0] != "" {
		lead += strconv.Quote(segs[0]) + " + "
	}
	trail := closing
	if last := segs[len(segs)-1]; last != "" {
		trail = " + " + strconv.Quote(last) + closing
	}
	edits := make([]analysis.TextEdit, 0, len(args)+1)
	edits = append(edits, analysis.TextEdit{Pos: call.Pos(), End: args[0].Pos(), NewText: []byte(lead)})
	for i := 0; i+1 < len(args); i++ {
		mid := " + "
		if seg := segs[i+1]; seg != "" {
			mid = " + " + strconv.Quote(seg) + " + "
		}
		edits = append(edits, analysis.TextEdit{Pos: args[i].End(), End: args[i+1].Pos(), NewText: []byte(mid)})
	}
	edits = append(edits, analysis.TextEdit{Pos: args[len(args)-1].End(), End: call.End(), NewText: []byte(trail)})
	return &analysis.SuggestedFix{
		Message:   "replace fmt.Sprintf with direct string concatenation of the literal segments and arguments",
		TextEdits: edits,
	}
}
