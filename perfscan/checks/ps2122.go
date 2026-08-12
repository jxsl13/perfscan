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

// PS2122 reports fmt.Sprintf("%s%s...%s", a, b, ..., z) — a format that is
// nothing but two or more %s verbs over plain strings — where the direct
// concatenation a + b + ... + z produces the identical string without fmt's
// reflection machinery or its pp buffer.
var PS2122 = register(&lint.Check{
	ID:       "PS2122",
	Category: "alloc",
	Slug:     "sprintf-concat",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: `fmt.Sprintf("%s%s", a, b) over plain strings is a reflection-priced +; concatenate directly`,
		Text: `fmt.Sprintf parses its format string, boxes every argument into
an interface, and walks fmt's formatter state machine through a pooled pp
buffer — even when the format contains nothing but %s verbs splicing
plain strings back to back. For that shape the result is byte-for-byte
the concatenation of the arguments, and a + b + ... + z is a direct
compiler-emitted concatenation: one length computation, one allocation,
one copy per operand, no fmt at all.

The match is deliberately narrow — it is the whole safety story. The
callee must resolve via type information to the package-level
fmt.Sprintf (a shadowed fmt or a method named Sprintf does not), the
call must not spread its arguments (no ...), and the format must be a
string literal whose value is EXACTLY "%s" repeated two or more times:
any literal text, a space, a flag, a width, %% or any other verb
disqualifies it. The single-"%s" case belongs to PS2107
(sprintf-single-value) and is excluded here. Every value argument must
have type EXACTLY the predeclared string — a NAMED string type could
implement fmt.Stringer or fmt.Formatter, which %s would honor and +
would not, so named types, []byte, error, interfaces and numbers all
stay out. For plain strings %s emits the value verbatim, making the
rewrite bit-identical. Argument evaluation order is unchanged: both the
call and the + chain evaluate left to right.

The fix keeps every argument byte-verbatim in place and edits only the
scaffolding: the fmt.Sprintf( prefix and the format literal are dropped
and each inter-argument comma becomes " + ". String-typed operands are
primaries or + chains, so the operands never need parentheses; the
WHOLE replacement is parenthesized when the surrounding context binds
tighter than + (an index, a slice, a binary operand), exactly as
PS2121 decides it. Each rewrite removes the file's fmt.Sprintf
selector; the fix applies even when that removes the file's last fmt
reference — the fix pipeline prunes the now-unused fmt import — EXCEPT
in cgo files (import "C"), whose import block is never edited: there
the fixes are withheld and the report stays advisory. A comment inside
the rewritten scaffolding is never silently dropped: it suppresses the
fix and keeps the report.`,
		Before: `s := fmt.Sprintf("%s%s%s", host, ":", port)`,
		After:  `s := host + ":" + port`,
		MeasuredWin: `BenchmarkPS2122 (a host + ":" + port join of three
plain strings, 24/1/5 bytes, once per op, Apple M2 Pro):
fmt.Sprintf("%s%s%s", ...) 91.5 ns/op, 80 B/op, 4 allocs/op vs
a + b + c 24.8 ns/op, 32 B/op, 1 alloc/op (~3.7x faster, one
allocation instead of four — the three interface boxings and the fmt
pp buffer round-trip disappear, leaving only the result string).`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2122",
		Doc:  "fmt.Sprintf with a pure %s%s... format over plain strings; direct + concatenation is identical and cheaper",
		Run:  runPS2122,
	},
})

func runPS2122(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		// Collect first: applying all fixes may rewrite the file's last
		// fmt reference and orphan the import. The fix pipeline prunes
		// such an orphan afterwards — except in a cgo file (import "C"),
		// whose import block is never edited, so there the fixes are
		// withheld and the reports stay advisory (same guard as
		// PS2107/PS2118).
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
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			// Type info pins the callee to the package-level fmt.Sprintf:
			// a shadowed fmt resolves sel.Sel to some other object, and a
			// method named Sprintf carries a receiver.
			fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
			if !ok || fn.Name() != "Sprintf" || fn.Pkg() == nil || fn.Pkg().Path() != "fmt" {
				return true
			}
			if sig, isSig := fn.Type().(*types.Signature); !isSig || sig.Recv() != nil {
				return true
			}
			// The format must be a string LITERAL that is exactly "%s"
			// repeated k >= 2 times — anything else (literal text, %%, a
			// flag, another verb, a variable format) disqualifies.
			lit, ok := call.Args[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			format, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			k, ok := ps2122PureVerbCount(format)
			if !ok || len(call.Args)-1 != k {
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
			fix := ps2122Fix(f, call, ps2121NeedsParens(parent, call))
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
				Message: "fmt.Sprintf with a format of only %s verbs over plain strings boxes every argument and walks fmt's formatter state machine; direct + concatenation builds the identical string",
			}
			if emitFixes && st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps2122PureVerbCount reports whether format is EXACTLY "%s" repeated two
// or more times, returning the repetition count. The pairwise scan is
// strict: an odd length, literal text, %%, a flag, a width, or any other
// verb breaks a pair and rejects the format — any string whose every
// 2-byte pair is "%s" is precisely ("%s")^k, so nothing else can slip
// through.
func ps2122PureVerbCount(format string) (int, bool) {
	if len(format) < 4 || len(format)%2 != 0 {
		return 0, false
	}
	for i := 0; i < len(format); i += 2 {
		if format[i] != '%' || format[i+1] != 's' {
			return 0, false
		}
	}
	return len(format) / 2, true
}

// ps2122Fix builds the a + b + ... + z replacement. Every argument stays
// byte-verbatim in place; only the scaffolding is edited — the
// fmt.Sprintf( prefix together with the format literal becomes the
// (optional) opening parenthesis, each inter-argument comma becomes
// " + ", and the closing parenthesis of the call becomes the (optional)
// closing one. String-typed operands are primaries or + chains (the only
// string-producing binary operator is the associative + itself), so the
// operands themselves never need parentheses. A comment inside any
// scaffolding span would be destroyed by the edits — the fix is withheld
// then and the report stays advisory.
func ps2122Fix(f *ast.File, call *ast.CallExpr, needParens bool) *analysis.SuggestedFix {
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
	edits := make([]analysis.TextEdit, 0, len(args)+1)
	edits = append(edits, analysis.TextEdit{Pos: call.Pos(), End: args[0].Pos(), NewText: []byte(open)})
	for i := 0; i+1 < len(args); i++ {
		edits = append(edits, analysis.TextEdit{Pos: args[i].End(), End: args[i+1].Pos(), NewText: []byte(" + ")})
	}
	edits = append(edits, analysis.TextEdit{Pos: args[len(args)-1].End(), End: call.End(), NewText: []byte(closing)})
	return &analysis.SuggestedFix{
		Message:   "replace fmt.Sprintf with direct string concatenation",
		TextEdits: edits,
	}
}
