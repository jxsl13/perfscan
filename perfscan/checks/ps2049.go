package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS2049 reports fmt.Fprintln(w, a, b, ...) with TWO OR MORE operands
// that are ALL statically plain strings — doPrintln's UNCONDITIONAL rule
// (exactly one ' ' between adjacent operands, exactly one trailing '\n',
// no format interpretation) makes the written bytes byte-for-byte
// a + " " + b + ... + "\n", which io.WriteString(w, a+" "+b+...+"\n")
// hands the writer directly. The multi-operand extension of PS5038
// (single-operand fmt.Fprintln -> io.WriteString(w, s+"\n")) and the
// writer sibling of PS5029 (fmt.Sprintln over strings -> + join).
var PS2049 = register(&lint.Check{
	ID:       "PS2049",
	Category: "alloc",
	Slug:     "fprintln-strings-to-writestring",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: `fmt.Fprintln over only plain strings boxes every operand through fmt's machinery; io.WriteString(w, a+" "+b+"\n") writes the identical bytes directly`,
		Text: `fmt.Fprintln(w, a, b) with only string operands takes a pp printer
from fmt's sync.Pool, boxes EVERY operand into an interface (one heap
allocation per string header), walks doPrintln's per-operand default
formatter through the pooled buffer and performs one w.Write. All of
that exists only to hand w the operands' own bytes joined by single
spaces plus one '\n'. io.WriteString(w, a+" "+b+"\n") builds the joined
string in a single runtime.concatstrings allocation of exactly the
final length — the constant " " separators and the trailing "\n" fold
into adjacent literal operands at compile time — and writes it directly:
w.WriteString when w implements io.StringWriter, one w.Write otherwise,
with no pool traffic, no boxing and no format dispatch.

The result is BIT-IDENTICAL under this check's guards. doPrintln is
UNCONDITIONAL: it writes exactly one ' ' between every adjacent pair of
operands and exactly one trailing '\n', with no format-string
interpretation (a '%' in an operand is data on both sides), and the
default %v formatting of a PLAIN string is the verbatim bytes — empty
strings still get their separators (fmt.Fprintln(w, "", "") writes
" \n" == ""+" "+""+"\n") and invalid UTF-8 passes through untouched —
so fmt assembles byte-for-byte a + " " + b + ... + "\n" in its pooled
buffer. Fprintln then performs exactly ONE w.Write of that buffer and
returns that Write's (n, err); io.WriteString performs the same single
write of the same bytes and forwards the same (n, err) — a short or
failing write surfaces the identical truncated n and error through both
forms. (One boundary: io.WriteString prefers w.WriteString when w is an
io.StringWriter, whereas fmt always calls w.Write; the io.StringWriter
contract REQUIRES WriteString to write the same bytes as
Write([]byte(s)), so identity holds for any contract-conforming
writer — the same reliance as PS5038/PS2129/PS5028.) Both spellings
evaluate w first and then each operand exactly once, left to right, so
side-effect count and order carry over, and a nil writer panics
identically in both forms.

The match is deliberately strict; everything else is out of scope. The
callee is resolved with type information — only the standard library's
package-level fmt.Fprintln matches (an aliased fmt import is honored),
never a shadowed fmt, a same-named third-party package or a method. The
call passes a writer plus AT LEAST TWO operands with no variadic
spread; the single-operand fmt.Fprintln(w, s) is PS5038's territory and
the zero-operand fmt.Fprintln(w) is left alone. Every operand must be
statically the EXACT predeclared string type (untyped constant strings
default to it): a NAMED string type could implement fmt.Stringer or
fmt.Formatter — which Fprintln's %v formatting honors and + would
not — and would not compile as a + operand anyway; []byte, error,
interfaces, numbers and bools all format through fmt's own logic and
never match.

Two positions are excluded entirely: a matched call lexically inside
w's own WriteString or Write method. io.WriteString dispatches to
w.WriteString when w implements io.StringWriter and to w.Write
otherwise, so inside either method the rewrite could dispatch back to
the enclosing method itself — unbounded recursion that still compiles.
The check reports nothing there; writing to a different object (a
field, another variable) inside those methods is still reported.

Only the scaffolding around the kept expressions is rewritten: the
fmt.Fprintln( prefix becomes io.WriteString(, each inter-operand comma
becomes a +" "+ join and the call's closing parenthesis gains the +"\n"
join. String-typed operands are primaries or + chains (the only
string-producing binary operator is the associative + itself), so no
operand ever needs parentheses inside the join. The fix edits imports
as needed (PS2129's machinery): the io import is added when missing,
and the fmt import is dropped when the rewrites remove the file's LAST
fmt reference — when they replace the whole import, the fmt spec is
swapped for "io" in place. A dot-, blank- or alias-imported io, a local
declaration shadowing the io name at the call site, a comment inside
the rewritten scaffolding, a file importing fmt under more than one
name, or a cgo file (whose import block must not be edited) keeps the
report advisory.`,
		Before: `fmt.Fprintln(w, host, port)`,
		After:  `io.WriteString(w, host+" "+port+"\n")`,
		MeasuredWin: `BenchmarkPS2049 (a host/port pair of plain string variables,
22/4 bytes, to io.Discard, Apple M2 Pro, go1.26):
fmt.Fprintln(w, host, port) 62.4 ns/op, 32 B/op, 2 allocs/op vs
io.WriteString(w, host+" "+port+"\n") 33.3 ns/op, 32 B/op, 1 alloc/op
(~1.9x faster, one allocation instead of two — the per-operand
interface boxings, the pool round-trip and the format walk disappear,
leaving only the joined string; with literal operands the constant
separators fold at compile time).`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2049",
		Doc:  "fmt.Fprintln over only plain strings instead of io.WriteString(w, a+\" \"+b+\"\\n\")",
		Run:  runPS2049,
	},
})

// ps2049Msg is the diagnostic text (shared by the fixed and advisory paths).
const ps2049Msg = `fmt.Fprintln over only plain strings writes exactly one space between operands and one trailing newline, boxing every operand through fmt's machinery; io.WriteString(w, a+" "+b+"\n") writes the identical bytes with the same (n, err)`

func runPS2049(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		// Collect first, decide import edits once per file: every fixable
		// call removes one fmt reference, and whether the fmt import is
		// orphaned depends on ALL of them together (same per-file site
		// collection as PS5038/PS2129/PS5028).
		type site struct {
			call *ast.CallExpr
			fix  *analysis.SuggestedFix
		}
		var sites []site
		fixable := 0
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			w, ok := ps2049Match(pass, call)
			if !ok {
				return true
			}
			// io.WriteString(w, ...) dispatches to w.WriteString when w
			// implements io.StringWriter and to w.Write otherwise: inside
			// the writer's own WriteString or Write method the rewrite
			// would dispatch back to the enclosing method itself — stay
			// silent, no valid rewrite exists there (PS5038's guard).
			if writeFixSelfDispatches(pass, call, w, "WriteString", "Write") {
				return true
			}
			fix := ps2049Fix(pass, f, call)
			if fix != nil {
				fixable++
			}
			sites = append(sites, site{call, fix})
			return true
		})
		if len(sites) == 0 {
			continue
		}
		// Each fixable call holds exactly one fmt reference (its selector's
		// package identifier); when those are ALL of the file's fmt
		// references, the rewrites orphan the import and the fix must drop
		// it. The io import is needed when the FILE lacks a plain one — a
		// site-local shadow only suppresses that site's fix, never the
		// file's import edit. (Shared with PS5038/PS2129/PS5028.)
		ioImported := false
		for _, imp := range f.Imports {
			if imp.Path.Value == `"io"` && imp.Name == nil {
				ioImported = true
				break
			}
		}
		needImport := fixable > 0 && !ioImported
		orphansFmt := fixable > 0 && pkgRefCount(pass, f, "fmt") == fixable
		importEdits, importsOK := ps2129ImportEdits(f, needImport, orphansFmt)
		// A file importing "fmt" under more than one name breaks the
		// name-blind fmt ref count and the first-match spec selection —
		// advisory only (see ps2129MultipleFmtSpecs).
		if fixable > 0 && ps2129MultipleFmtSpecs(f) {
			importsOK = false
		}
		if !importsOK {
			// cgo file needing import surgery, or a fmt spec we could not
			// locate: keep every report advisory.
			for i := range sites {
				sites[i].fix = nil
			}
		} else if len(importEdits) > 0 {
			// All fixes of a run are applied together, so only the first
			// fixable site carries the import edits (same convention as
			// PS5038/PS2129/PS3104).
			for i := range sites {
				if sites[i].fix != nil {
					sites[i].fix.TextEdits = append(sites[i].fix.TextEdits, importEdits...)
					break
				}
			}
		}
		for _, st := range sites {
			diag := analysis.Diagnostic{
				Pos:     st.call.Pos(),
				End:     st.call.End(),
				Message: ps2049Msg,
			}
			if st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps2049Match matches call as fmt.Fprintln(w, a, b, ...) — a writer plus
// AT LEAST TWO operands, no variadic spread, every operand statically the
// EXACT predeclared string type (untyped constant strings default to it).
// The callee is pinned by type information to the standard library fmt's
// package-level Fprintln, so a shadowed fmt, a same-named third-party
// package or a method never matches. Zero operands and the one-operand
// form (PS5038's territory) are out of scope, as is any operand whose
// type is not the plain string — a NAMED string type could implement
// fmt.Stringer/fmt.Formatter, which Fprintln's %v formatting honors and
// + would not.
func ps2049Match(pass *analysis.Pass, call *ast.CallExpr) (w ast.Expr, ok bool) {
	if len(call.Args) < 3 || call.Ellipsis.IsValid() {
		return nil, false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil, false
	}
	// The qualifier must BE the imported fmt package, not a value whose
	// evaluation the rewrite would drop.
	xid, ok := sel.X.(*ast.Ident)
	if !ok {
		return nil, false
	}
	if pn, isPkg := pass.TypesInfo.Uses[xid].(*types.PkgName); !isPkg || pn.Imported().Path() != "fmt" {
		return nil, false
	}
	fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !ok || fn.Pkg() == nil || fn.Pkg().Path() != "fmt" || fn.Name() != "Fprintln" {
		return nil, false
	}
	if sig, isSig := fn.Type().(*types.Signature); !isSig || sig.Recv() != nil {
		return nil, false
	}
	for _, a := range call.Args[1:] {
		t := pass.TypesInfo.TypeOf(a)
		if t == nil || !types.Identical(types.Default(t), types.Typ[types.String]) {
			return nil, false
		}
	}
	return call.Args[0], true
}

// ps2049Fix builds the io.WriteString(w, a+" "+b+...+"\n") rewrite for
// one call, or nil when a guard fails and the report must stay advisory.
// Only the scaffolding around the operands is replaced; w's and every
// operand's text stays untouched in place, preserving their single
// evaluation and order. String-typed operands are primaries or + chains
// (the only string-producing binary operator is the associative +
// itself), so no operand ever needs parentheses inside the join. Import
// edits are appended later, once per file.
func ps2049Fix(pass *analysis.Pass, f *ast.File, call *ast.CallExpr) *analysis.SuggestedFix {
	if !ps2129IoUsable(pass, f, call.Pos()) {
		return nil
	}
	// The replaced spans are the call text around the kept expressions; a
	// comment there would be silently destroyed — advisory then.
	args := call.Args
	if ps2111CommentIn(f, call.Pos(), args[0].Pos()) ||
		ps2111CommentIn(f, args[len(args)-1].End(), call.End()) {
		return nil
	}
	for i := 0; i+1 < len(args); i++ {
		if ps2111CommentIn(f, args[i].End(), args[i+1].Pos()) {
			return nil
		}
	}
	// The first comma (writer to first operand) stays a comma; every
	// inter-operand comma becomes a +" "+ join.
	edits := make([]analysis.TextEdit, 0, len(args)+1)
	edits = append(edits,
		analysis.TextEdit{Pos: call.Pos(), End: args[0].Pos(), NewText: []byte("io.WriteString(")},
		analysis.TextEdit{Pos: args[0].End(), End: args[1].Pos(), NewText: []byte(", ")})
	for i := 1; i+1 < len(args); i++ {
		edits = append(edits, analysis.TextEdit{Pos: args[i].End(), End: args[i+1].Pos(), NewText: []byte(`+" "+`)})
	}
	edits = append(edits, analysis.TextEdit{Pos: args[len(args)-1].End(), End: call.End(), NewText: []byte(`+"\n")`)})
	return &analysis.SuggestedFix{
		Message:   `replace with io.WriteString(w, a+" "+b+"\n")`,
		TextEdits: edits,
	}
}
