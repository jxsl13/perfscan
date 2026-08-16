package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS2048 reports fmt.Fprint(w, a, b, ..., z) where EVERY operand is a
// plain string — the shape where Fprint's spacing rule inserts nothing
// and the bytes written are exactly the operands concatenated. The
// rewrite io.WriteString(w, a+b+...+z) writes the identical bytes in
// one call with the same (n, err), without the per-operand interface
// boxing or fmt's reflection walk. The single-operand fmt.Fprint(w, s)
// is PS2129's territory; PS2048 takes the multi-operand case.
var PS2048 = register(&lint.Check{
	ID:       "PS2048",
	Category: "alloc",
	Slug:     "fprint-strings-concat-writestring",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "fmt.Fprint over only plain strings is a reflection-priced concatenation; io.WriteString(w, a+b+...) writes the same bytes directly",
		Text: `fmt.Fprint boxes every operand into an interface (one heap
allocation per operand) and walks fmt's reflection-based default
formatter through a pooled pp buffer before its single w.Write. Its
spec adds a space between operands only "when neither is a string" —
so when EVERY operand is a string no separator is ever inserted, and
the buffer handed to w.Write is byte-for-byte the operands
concatenated. io.WriteString(w, a+b+...+z) is the same bytes without
fmt: a compiler-emitted concatenation (one length pass, one result
allocation, one copy per operand) and one write.

The result is BIT-IDENTICAL. fmt's default %v formatting emits a plain
string operand verbatim — empty strings and invalid UTF-8 included —
and with no separator the accumulated buffer IS the concatenation.
fmt.Fprint performs exactly ONE w.Write of that buffer and returns its
(n, err); io.WriteString performs one write of the identical bytes and
returns the same (n, err). (One boundary: io.WriteString prefers
w.WriteString when w is an io.StringWriter, whereas fmt always calls
w.Write; the io.StringWriter contract REQUIRES WriteString to write
the same bytes as Write([]byte(s)), so identity holds for any
contract-conforming writer — the exact reliance PS2129 already ships.)
Both forms evaluate w first and then each operand once, left to right.

The match is deliberately narrow — it is the whole safety story. The
callee must resolve via type information to the package-level
fmt.Fprint (a shadowed fmt, a same-named third-party package or a
method never matches), the call must not spread its arguments (no ...),
and there must be at least TWO operands — the single-operand form is
PS2129's. fmt.Fprintln (ALWAYS space-separated plus a trailing
newline) and fmt.Fprintf never match. Every operand must have type
EXACTLY the predeclared string (untyped constant strings default to
it): a NAMED string type could implement fmt.Stringer or
fmt.Formatter, which Fprint's %v formatting honors and + would not, so
named types, []byte, error, interfaces (a nil operand included),
numbers and bools all stay out; even one non-string operand would also
re-engage the spacing rule against its neighbors.

Two positions are excluded entirely: a matched call lexically inside
w's own WriteString or Write method. io.WriteString(w, s) dispatches
to w.WriteString when w implements io.StringWriter and to w.Write
otherwise, so inside either method the rewrite could dispatch back to
the enclosing method itself — unbounded recursion that still compiles.
The check reports nothing there; writing to a different object inside
those methods is still reported.

The writer and operand expressions are kept byte-verbatim in place —
evaluated once, in the original order; string-typed operands are
primaries or + chains (the only string-producing binary operator is
the associative + itself), and as a call argument the joined chain
never needs parentheses. Only the scaffolding is rewritten: the
fmt.Fprint( prefix becomes io.WriteString(, and each inter-operand
comma becomes " + ". The fix edits imports as needed: the io import
is added when missing, and the fmt import is dropped when the rewrites
remove the file's LAST fmt reference — when they replace the whole
import, the fmt spec is swapped for "io" in place. A dot-, blank- or
alias-imported io, a local declaration shadowing the io name at the
call site, a comment inside the rewritten scaffolding, or a cgo file
(whose import block must not be edited) keeps the report advisory.

Note the rewrite trades fmt's pooled buffer for one result-string
allocation: still a clear win, because Fprint pays an interface boxing
per operand plus the reflection walk before its write, all of which
disappear.`,
		Before: `n, err := fmt.Fprint(w, host, ":", port)`,
		After:  `n, err := io.WriteString(w, host+":"+port)`,
		MeasuredWin: `BenchmarkPS2048 (writing host + ":" + port, 24/1/5-byte
plain strings, to io.Discard, Apple M2 Pro, go1.26):
fmt.Fprint(w, a, b, c) 95.0 ns/op 48 B/op 3 allocs/op ->
io.WriteString(w, a+b+c) 30.9 ns/op 32 B/op 1 alloc/op (~3.1x, one
allocation — the concatenated result — instead of three interface
boxings plus the fmt pp buffer round-trip).`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2048",
		Doc:  "fmt.Fprint over only plain strings instead of io.WriteString(w, a+b+...)",
		Run:  runPS2048,
	},
})

func runPS2048(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		// Collect first, decide import edits once per file: every fixable
		// call removes one fmt reference, and whether the fmt import is
		// orphaned depends on ALL of them together (same per-file site
		// collection as PS2129/PS2118/PS3104).
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
			w, ok := ps2048Match(pass, call)
			if !ok {
				return true
			}
			// io.WriteString(w, s) dispatches to w.WriteString when w
			// implements io.StringWriter and to w.Write otherwise: inside
			// the writer's own WriteString or Write method the rewrite
			// would dispatch back to the enclosing method itself — stay
			// silent, no valid rewrite exists there (same guard as PS2129).
			if writeFixSelfDispatches(pass, call, w, "WriteString", "Write") {
				return true
			}
			fix := ps2048Fix(pass, f, call)
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
		// file's import edit. The import bookkeeping is PS2129's,
		// byte-identical requirements, so its helpers are reused.
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
		// A file importing "fmt" under more than one spec breaks the
		// name-blind ref count and the first-match spec selection —
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
			// PS2129/PS3104).
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
				Message: "fmt.Fprint over only plain strings inserts no separators, boxes every operand and walks fmt's reflection printer just to write their concatenation; io.WriteString(w, a+b+...) writes the identical bytes with the same (n, err)",
			}
			if st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps2048Match matches call as fmt.Fprint(w, a, b, ..., z) with at least
// two operands, every one statically the predeclared string type. The
// callee is pinned by type information to the standard library fmt's
// package function, so a shadowed fmt, a same-named third-party package
// or a method never matches. The single-operand form is PS2129's and is
// rejected here.
func ps2048Match(pass *analysis.Pass, call *ast.CallExpr) (w ast.Expr, ok bool) {
	// At least (w, a, b); no variadic spread.
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
	if !ok || fn.Name() != "Fprint" || fn.Pkg() == nil || fn.Pkg().Path() != "fmt" {
		return nil, false
	}
	if sig, isSig := fn.Type().(*types.Signature); !isSig || sig.Recv() != nil {
		return nil, false
	}
	// Every operand must be EXACTLY the predeclared string (untyped
	// constant strings default to it). A named string type could
	// implement fmt.Stringer/fmt.Formatter, which Fprint's %v formatting
	// honors and + would not, and any non-string operand re-engages the
	// spacing rule — bit-identity requires plain string throughout.
	for _, a := range call.Args[1:] {
		t := pass.TypesInfo.TypeOf(a)
		if t == nil || !types.Identical(types.Default(t), types.Typ[types.String]) {
			return nil, false
		}
	}
	return call.Args[0], true
}

// ps2048Fix builds the io.WriteString(w, a+b+...+z) rewrite for one
// call, or nil when a guard fails and the report must stay advisory.
// Every operand stays byte-verbatim in place; only the scaffolding is
// edited — the fmt.Fprint( prefix becomes io.WriteString(, and each
// inter-operand comma becomes " + ". String-typed operands are
// primaries or + chains, so they never need parentheses, and as a call
// argument neither does the joined chain. Import edits are appended
// later, once per file.
func ps2048Fix(pass *analysis.Pass, f *ast.File, call *ast.CallExpr) *analysis.SuggestedFix {
	if !ps2129IoUsable(pass, f, call.Pos()) {
		return nil
	}
	// The replaced spans are the call text around the operands; a comment
	// there would be silently destroyed — advisory then.
	args := call.Args
	for i := 0; i+1 < len(args); i++ {
		if ps2111CommentIn(f, args[i].End(), args[i+1].Pos()) {
			return nil
		}
	}
	if ps2111CommentIn(f, call.Pos(), args[0].Pos()) ||
		ps2111CommentIn(f, args[len(args)-1].End(), call.End()) {
		return nil
	}
	edits := make([]analysis.TextEdit, 0, len(args)+1)
	edits = append(edits,
		analysis.TextEdit{Pos: call.Pos(), End: args[0].Pos(), NewText: []byte("io.WriteString(")},
		analysis.TextEdit{Pos: args[0].End(), End: args[1].Pos(), NewText: []byte(", ")})
	for i := 1; i+1 < len(args); i++ {
		edits = append(edits, analysis.TextEdit{Pos: args[i].End(), End: args[i+1].Pos(), NewText: []byte("+")})
	}
	edits = append(edits, analysis.TextEdit{Pos: args[len(args)-1].End(), End: call.End(), NewText: []byte(")")})
	return &analysis.SuggestedFix{
		Message:   "replace with io.WriteString(w, a+b+...)",
		TextEdits: edits,
	}
}
