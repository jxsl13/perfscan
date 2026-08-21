package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS5028 reports fmt.Fprintf(w, "constant") — a two-argument Fprintf whose
// format is a string literal with no '%' byte — a constant run of bytes
// pushed through fmt's pooled printer and format scanner when
// io.WriteString(w, s) hands the same bytes to w directly. The verbless
// sibling of PS2129 (which rewrites the "%s"/"%v" one-operand shapes).
var PS5028 = register(&lint.Check{
	ID:       "PS5028",
	Category: "indirect",
	Slug:     "fprintf-verbless-constant-to-writestring",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "fmt.Fprintf of a verbless constant format pays fmt's format scan and pooled-buffer copy; io.WriteString writes the bytes directly",
		Text: `fmt.Fprintf(w, "constant") with no operands still runs the whole
printf machinery: it takes a pp printer from fmt's sync.Pool, scans the
format byte-by-byte in doPrintf looking for verbs, copies the entire run
into the intermediate pp.buf, calls w.Write(pp.buf) and returns the
printer to the pool. With nothing to format, all of that exists only to
hand w the very bytes the literal already holds. io.WriteString(w, s)
writes them directly — w.WriteString(s) when w implements
io.StringWriter, one w.Write([]byte(s)) otherwise — with no pool
traffic, no format scan and no intermediate copy. Unlike PS2129's
"%s"-operand shape there is no interface-boxing allocation to save
(fmt pools its printer, so the constant shape is already 0 allocs/op in
steady state); the win is pure instruction count and it grows with the
length of the literal fmt would scan and copy.

The result is BIT-IDENTICAL under this check's guards. With EXACTLY two
arguments and a format containing NO '%' byte, doPrintf finds no verb:
pp.buf becomes the format bytes verbatim, w.Write receives exactly
those bytes ONCE, and Fprintf returns that Write's (n, err) —
n == len(format) on success. io.WriteString(w, s) performs the same
single write of the same bytes and forwards the same (n, err). (One
boundary: io.WriteString prefers w.WriteString when w is an
io.StringWriter, whereas fmt always calls w.Write; the io.StringWriter
contract REQUIRES WriteString to write the same bytes as
Write([]byte(s)), so identity holds for any contract-conforming writer
— a writer that violates it is already buggy. The same argument
underwrites PS2129 and PS2118.) The '%' exclusion is absolute because
ANY '%' formats: "%%" collapses to "%", a verb with no argument prints
%!v(MISSING), a trailing "%" prints %!(NOVERB) — while io.WriteString
would keep the bytes verbatim. A third argument is out for the same
reason ("%!(EXTRA ...)" is appended), and with no operands there is no
Stringer, error, nil or float to worry about.

The match is deliberately strict; everything else is out of scope. The
callee is resolved with type information — only the standard library's
package-level fmt.Fprintf matches (an aliased fmt import is honored),
never a shadowed fmt, a same-named third-party package or a method. The
call passes EXACTLY two arguments with no variadic spread, and the
format is a string LITERAL — a named constant or a constant expression
never matches, because the rewrite keeps the format expression
byte-verbatim and a symbolic name stays symbolic either way, while the
no-'%' proof needs the literal's constant value.

Two positions are excluded entirely: a matched call lexically inside
w's own WriteString or Write method. io.WriteString(w, s) dispatches to
w.WriteString when w implements io.StringWriter and to w.Write
otherwise, so inside either method the rewrite could dispatch back to
the enclosing method itself — unbounded recursion that still compiles.
The check reports nothing there; writing to a different object (a
field, another variable) inside those methods is still reported.

The writer and format expressions are kept byte-verbatim in place —
evaluated once, in the original order; only the wrapper around them is
rewritten to io.WriteString(w, s). The fix edits imports as needed
(PS2129's machinery): the io import is added when missing, and the fmt
import is dropped when the rewrites remove the file's LAST fmt
reference — when they replace the whole import, the fmt spec is swapped
for "io" in place. A dot-, blank- or alias-imported io, a local
declaration shadowing the io name at the call site, a comment inside
the rewritten punctuation, a file importing fmt under more than one
name, or a cgo file (whose import block must not be edited) keeps the
report advisory.`,
		Before: `fmt.Fprintf(w, "done\n")`,
		After:  `io.WriteString(w, "done\n")`,
		MeasuredWin: `BenchmarkPS5028 (a 29-byte verbless literal to io.Discard,
Apple M2 Pro, go1.26): fmt.Fprintf(w, "shutdown: all shards flushed\n")
26.2 ns/op -> io.WriteString(w, ...) 3.0 ns/op (~8.7x). 0 B/op and
0 allocs/op on BOTH sides — fmt pools its printer, so the saving is the
pool get/put, the byte-by-byte format scan and the copy into the pooled
buffer, and it widens as the literal grows.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5028",
		Doc:  "fmt.Fprintf(w, \"constant\") with a verbless literal and no operands instead of io.WriteString(w, \"constant\")",
		Run:  runPS5028,
	},
})

// ps5028Msg is the diagnostic text (shared by the fixed and advisory paths).
const ps5028Msg = "fmt.Fprintf with a verbless constant format and no operands pays fmt's pooled printer, format scan and intermediate buffer copy just to write the literal's bytes; io.WriteString(w, s) hands them to w directly with the same (n, err)"

func runPS5028(pass *analysis.Pass) (any, error) {
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
			w, s, ok := ps5028Match(pass, call)
			if !ok {
				return true
			}
			// io.WriteString(w, s) dispatches to w.WriteString when w
			// implements io.StringWriter and to w.Write otherwise: inside
			// the writer's own WriteString or Write method the rewrite
			// would dispatch back to the enclosing method itself — stay
			// silent, no valid rewrite exists there (PS2129's guard).
			if writeFixSelfDispatches(pass, call, w, "WriteString", "Write") {
				return true
			}
			// The rewrite wrapper, comment guard and io-import gate are
			// exactly PS2129's: only the punctuation around the two kept
			// operand expressions is replaced.
			fix := ps2129Fix(pass, f, call, w, s)
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
		// file's import edit. (Shared with PS2129.)
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
				Message: ps5028Msg,
			}
			if st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps5028Match matches call as fmt.Fprintf(w, "constant") — exactly two
// arguments, no variadic spread, format a string LITERAL whose constant
// value contains NO '%' byte. The callee is pinned by type information to
// the standard library fmt's package-level Fprintf, so a shadowed fmt, a
// same-named third-party package or a method never matches. The
// three-argument "%s"/"%v" shapes are PS2129's territory, and any '%' in
// the format means fmt would interpret it ("%%" collapses, a verb prints
// %!verb(MISSING), a trailing "%" prints %!(NOVERB)) — never matched.
func ps5028Match(pass *analysis.Pass, call *ast.CallExpr) (w, s ast.Expr, ok bool) {
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
	if !ok || fn.Pkg() == nil || fn.Pkg().Path() != "fmt" || fn.Name() != "Fprintf" {
		return nil, nil, false
	}
	if sig, isSig := fn.Type().(*types.Signature); !isSig || sig.Recv() != nil {
		return nil, nil, false
	}
	lit, isLit := ps2110Unparen(call.Args[1]).(*ast.BasicLit)
	if !isLit || lit.Kind != token.STRING {
		return nil, nil, false
	}
	tv, has := pass.TypesInfo.Types[call.Args[1]]
	if !has || tv.Value == nil || tv.Value.Kind() != constant.String {
		return nil, nil, false
	}
	if strings.IndexByte(constant.StringVal(tv.Value), '%') >= 0 {
		return nil, nil, false
	}
	return call.Args[0], call.Args[1], true
}
