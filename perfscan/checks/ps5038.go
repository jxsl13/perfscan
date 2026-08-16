package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS5038 reports fmt.Fprintln(w, s) with exactly ONE operand that is
// statically a plain string — interface boxing, a pooled printer and
// fmt's default-format walk paid just to hand w the bytes of s plus one
// newline, which io.WriteString(w, s+"\n") writes directly. The writer
// sibling of PS5029 (single-operand fmt.Sprintln(a) -> a + "\n") and the
// Fprintln member of the PS2129/PS5028 io.WriteString family.
var PS5038 = register(&lint.Check{
	ID:       "PS5038",
	Category: "alloc",
	Slug:     "fprintln-string-to-writestring",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: `fmt.Fprintln(w, s) on a single plain string pays fmt's whole machinery; io.WriteString(w, s+"\n") writes the identical bytes directly`,
		Text: `fmt.Fprintln(w, s) with a single string operand takes a pp printer
from fmt's sync.Pool, boxes s into an interface (a heap allocation for
the string header), walks doPrintln's default formatter into the pooled
buffer and performs one w.Write. All of that exists only to hand w the
very bytes s already holds plus one '\n'. io.WriteString(w, s+"\n")
forms the joined string in a single runtime.concatstrings allocation of
exactly the final length and writes it directly — w.WriteString when w
implements io.StringWriter, one w.Write otherwise — with no pool
traffic, no boxing and no format dispatch. When s is a constant the
+"\n" folds at compile time and the rewrite allocates nothing at all.

The result is BIT-IDENTICAL under this check's guards. With exactly one
operand, doPrintln applies no between-operand spaces and appends exactly
one trailing '\n', and the default %v formatting of a PLAIN string is
the verbatim bytes — empty strings and invalid UTF-8 included; a string
is never nil — so the assembled buffer is byte-for-byte s + "\n".
Fprintln performs exactly ONE w.Write of that buffer and returns that
Write's (n, err); io.WriteString performs the same single write of the
same bytes and forwards the same (n, err) — n == len(s)+1 on success,
and a short or failing write surfaces the identical truncated n and
error through both forms. (One boundary: io.WriteString prefers
w.WriteString when w is an io.StringWriter, whereas fmt always calls
w.Write; the io.StringWriter contract REQUIRES WriteString to write the
same bytes as Write([]byte(s)), so identity holds for any
contract-conforming writer — a writer that violates it is already
buggy. The same argument underwrites PS2129 and PS5028.) There is no
'%' hazard at all: Fprintln never interprets verbs, so a '%' in s is
data on both sides. w and s are kept byte-verbatim in place — each
evaluated exactly once, in the original order — and a nil writer panics
identically in both forms.

The match is deliberately strict; everything else is out of scope. The
callee is resolved with type information — only the standard library's
package-level fmt.Fprintln matches (an aliased fmt import is honored),
never a shadowed fmt, a same-named third-party package or a method. The
call passes EXACTLY two arguments (writer plus one operand) with no
variadic spread: a second operand would gain Fprintln's separating
space, and the zero-operand fmt.Fprintln(w) is left alone. The operand
must be statically the EXACT predeclared string type (untyped constant
strings default to it): a NAMED string type could implement
fmt.Stringer or fmt.Formatter — which Fprintln's %v formatting honors
and + would not — and would not compile as a + "\n" operand anyway;
[]byte, error, interfaces, numbers and bools all format through fmt's
own logic and never match.

Two positions are excluded entirely: a matched call lexically inside
w's own WriteString or Write method. io.WriteString(w, s+"\n")
dispatches to w.WriteString when w implements io.StringWriter and to
w.Write otherwise, so inside either method the rewrite could dispatch
back to the enclosing method itself — unbounded recursion that still
compiles. The check reports nothing there; writing to a different
object (a field, another variable) inside those methods is still
reported.

Only the wrapper around the two kept expressions is rewritten: the
fmt.Fprintln( prefix becomes io.WriteString( and the call's closing
parenthesis gains the +"\n" join. A string-typed operand is a primary
or a + chain (the only string-producing binary operator is + itself),
so s never needs parentheses inside the join. The fix edits imports as
needed (PS2129's machinery): the io import is added when missing, and
the fmt import is dropped when the rewrites remove the file's LAST fmt
reference — when they replace the whole import, the fmt spec is swapped
for "io" in place. A dot-, blank- or alias-imported io, a local
declaration shadowing the io name at the call site, a comment inside
the rewritten punctuation, a file importing fmt under more than one
name, or a cgo file (whose import block must not be edited) keeps the
report advisory.`,
		Before: `fmt.Fprintln(w, s)`,
		After:  `io.WriteString(w, s+"\n")`,
		MeasuredWin: `BenchmarkPS5038 (a 28-byte string variable to io.Discard,
Apple M2 Pro, go1.26): fmt.Fprintln(w, s) 33.5 ns/op, 16 B/op,
1 alloc/op vs io.WriteString(w, s+"\n") 24.7 ns/op, 32 B/op, 1 alloc/op
(~1.4x faster — the interface boxing, pool round-trip and format walk
disappear, leaving only the s+"\n" concatenation; for a constant s the
+"\n" folds at compile time and the rewrite is allocation-free).`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5038",
		Doc:  "fmt.Fprintln(w, s) on a single plain string instead of io.WriteString(w, s+\"\\n\")",
		Run:  runPS5038,
	},
})

// ps5038Msg is the diagnostic text (shared by the fixed and advisory paths).
const ps5038Msg = `fmt.Fprintln(w, s) on a single plain string pays fmt's interface boxing, pooled printer and format walk just to write s plus one newline; io.WriteString(w, s+"\n") writes the identical bytes with the same (n, err)`

func runPS5038(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		// Collect first, decide import edits once per file: every fixable
		// call removes one fmt reference, and whether the fmt import is
		// orphaned depends on ALL of them together (same per-file site
		// collection as PS2129/PS5028/PS2118/PS3104).
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
			w, s, ok := ps5038Match(pass, call)
			if !ok {
				return true
			}
			// io.WriteString(w, s+"\n") dispatches to w.WriteString when w
			// implements io.StringWriter and to w.Write otherwise: inside
			// the writer's own WriteString or Write method the rewrite
			// would dispatch back to the enclosing method itself — stay
			// silent, no valid rewrite exists there (PS2129's guard).
			if writeFixSelfDispatches(pass, call, w, "WriteString", "Write") {
				return true
			}
			fix := ps5038Fix(pass, f, call, w, s)
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
		// file's import edit. (Shared with PS2129/PS5028.)
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
			// PS2129/PS3104/PS2110).
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
				Message: ps5038Msg,
			}
			if st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps5038Match matches call as fmt.Fprintln(w, s) — exactly two arguments,
// no variadic spread, s statically the EXACT predeclared string type
// (untyped constant strings default to it). The callee is pinned by type
// information to the standard library fmt's package-level Fprintln, so a
// shadowed fmt, a same-named third-party package or a method never
// matches. Zero operands (fmt.Fprintln(w)) and two-plus operands (which
// would gain Fprintln's separating space) are out of scope, as is any
// operand whose type is not the plain string — a NAMED string type could
// implement fmt.Stringer/fmt.Formatter, which Fprintln's %v formatting
// honors and + would not.
func ps5038Match(pass *analysis.Pass, call *ast.CallExpr) (w, s ast.Expr, ok bool) {
	if len(call.Args) != 2 || call.Ellipsis.IsValid() {
		return nil, nil, false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil, nil, false
	}
	// The qualifier must BE the imported fmt package, not a value whose
	// evaluation the rewrite would drop.
	xid, ok := sel.X.(*ast.Ident)
	if !ok {
		return nil, nil, false
	}
	if pn, isPkg := pass.TypesInfo.Uses[xid].(*types.PkgName); !isPkg || pn.Imported().Path() != "fmt" {
		return nil, nil, false
	}
	fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !ok || fn.Pkg() == nil || fn.Pkg().Path() != "fmt" || fn.Name() != "Fprintln" {
		return nil, nil, false
	}
	if sig, isSig := fn.Type().(*types.Signature); !isSig || sig.Recv() != nil {
		return nil, nil, false
	}
	st := pass.TypesInfo.TypeOf(call.Args[1])
	if st == nil || !types.Identical(types.Default(st), types.Typ[types.String]) {
		return nil, nil, false
	}
	return call.Args[0], call.Args[1], true
}

// ps5038Fix builds the io.WriteString(w, s+"\n") rewrite for one call, or
// nil when a guard fails and the report must stay advisory. Only the
// wrapper around the two operands is replaced; w's and s's text stays
// untouched in place, preserving their single evaluation and order. A
// string-typed s is a primary or a + chain (the only string-producing
// binary operator is the associative + itself), so s never needs
// parentheses inside the +"\n" join. Import edits are appended later,
// once per file.
func ps5038Fix(pass *analysis.Pass, f *ast.File, call *ast.CallExpr, w, s ast.Expr) *analysis.SuggestedFix {
	if !ps2129IoUsable(pass, f, call.Pos()) {
		return nil
	}
	// The replaced spans are the call text around the operands; a comment
	// there would be silently destroyed — advisory then.
	if ps2111CommentIn(f, call.Pos(), w.Pos()) ||
		ps2111CommentIn(f, w.End(), s.Pos()) ||
		ps2111CommentIn(f, s.End(), call.End()) {
		return nil
	}
	return &analysis.SuggestedFix{
		Message: `replace with io.WriteString(w, s+"\n")`,
		TextEdits: []analysis.TextEdit{
			{Pos: call.Pos(), End: w.Pos(), NewText: []byte("io.WriteString(")},
			{Pos: w.End(), End: s.Pos(), NewText: []byte(", ")},
			{Pos: s.End(), End: call.End(), NewText: []byte(`+"\n")`)},
		},
	}
}
