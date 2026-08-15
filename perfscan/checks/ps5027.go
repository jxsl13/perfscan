package checks

import (
	"go/ast"
	"go/token"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS5027 reports fmt.Sprintf calls whose SOLE argument is a string
// literal with no '%' in it — a constant string pushed through fmt's
// whole printf machinery (pooled printer, format walk, buffer copy,
// fresh heap result) when the literal itself IS the result. The
// plain-string sibling of PS2126 (fmt.Errorf of a verbless constant
// message).
var PS5027 = register(&lint.Check{
	ID:       "PS5027",
	Category: "alloc",
	Slug:     "sprintf-constant",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "fmt.Sprintf of a verbless constant string is the literal itself paid at fmt prices",
		Text: `fmt.Sprintf("constant") gets a printer from fmt's sync.Pool, walks
the format through doPrintf, copies the verb-free run into the pooled
pp buffer, materializes a FRESH heap string via string(p.buf) and puts
the printer back — the entire formatting pipeline spent reproducing
bytes the source code already spells out. With no verbs and no extra
operands fmt.Sprintf returns its format string verbatim, so the call
is byte-for-byte the literal itself: a compile-time constant with zero
pool traffic, zero format scan, zero copy and zero allocation.

The match is deliberately narrow — it is the whole safety story,
mirroring PS2126. The callee must resolve through type information to
the package-level fmt.Sprintf (an aliased fmt import is honored; a
shadowed fmt or a method named Sprintf is not), the call must pass
EXACTLY one argument and not spread it (no ...), and that argument
must be a string LITERAL whose unquoted value contains NO '%' byte
anywhere. A single '%' disqualifies it: fmt transforms it (%%->%, a
lone verb consumes an argument that is not there and prints
%!verb(MISSING)), while the bare literal keeps its bytes verbatim — so
any '%' breaks bit-identity. Extra arguments append %!(EXTRA ...), a
non-literal format proves nothing, and a spread passes an unknown
number of operands — all stay out. There is no operand at all, so no
named-type, String()/Error()/Formatter, nil or evaluation-order
concern exists; invalid UTF-8 and NUL bytes ride through both
spellings identically, and the untyped string constant defaults to
string exactly where the call's string result stood.

The fix deletes the fmt.Sprintf wrapper around the literal, keeping
the literal's text byte-verbatim in place (backticks and escapes
included). A string literal is self-delimiting, so it never needs
parentheses in the call's context. A call used as a bare statement (or
under go/defer, which require a call) cannot become a bare literal —
advisory there. The fix never ADDS an import; it drops the fmt import
spec when the rewrites remove the file's LAST fmt reference. A
renamed (aliased, dot or blank) fmt spec, a file importing fmt under
more than one name, a cgo file (whose import block must not be edited
when the drop is needed) or a comment inside the deleted wrapper keeps
the report advisory.`,
		Before: `s := fmt.Sprintf("database connection failed")`,
		After:  `s := "database connection failed"`,
		MeasuredWin: `BenchmarkPS5027 (the 26-byte constant "database connection
failed", Apple M2 Pro, go1.26): fmt.Sprintf("database connection failed")
~40 ns/op, 32 B/op, 1 alloc/op -> the literal itself ~0.75 ns/op, 0 B/op,
0 allocs/op (~50x; the pool round-trip, the format walk, the buffer copy
and the result-string allocation all vanish — the literal is a
compile-time constant).`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5027",
		Doc:  "fmt.Sprintf of a verbless constant string returns the literal verbatim; the literal itself is the bit-identical, allocation-free result",
		Run:  runPS5027,
	},
})

// ps5027Msg is the diagnostic text (shared by the fixed and advisory paths).
const ps5027Msg = "fmt.Sprintf on a verbless constant string runs fmt's whole format machinery to copy the literal into a fresh heap string; the string literal itself is the bit-identical, allocation-free result"

func runPS5027(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		// Collect first, decide import edits once per file: every fixable
		// call removes one fmt reference, and whether the fmt import is
		// orphaned depends on ALL of them together (same per-file site
		// collection as PS2130/PS2129).
		type site struct {
			call *ast.CallExpr
			fix  *analysis.SuggestedFix
		}
		var sites []site
		fixable := 0
		astutil.WithStack(f, func(n ast.Node, stack []ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 1 || call.Ellipsis.IsValid() {
				return true
			}
			if _, ok := astutil.PkgFuncCall(pass.TypesInfo, call.Fun, "fmt", map[string]bool{"Sprintf": true}); !ok {
				return true
			}
			lit, ok := call.Args[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			msg, err := strconv.Unquote(lit.Value)
			if err != nil || strings.IndexByte(msg, '%') >= 0 {
				return true
			}
			var parent ast.Node
			if len(stack) > 0 {
				parent = stack[len(stack)-1]
			}
			fix := ps5027Fix(f, call, lit, parent)
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
		// it (the runner never prunes imports itself). There is never an
		// import to ADD.
		orphansFmt := fixable > 0 && pkgRefCount(pass, f, "fmt") == fixable
		importEdits, importsOK := ps2130ImportEdits(f, orphansFmt)
		// A file importing "fmt" under more than one name breaks the
		// name-blind fmt ref count, and a renamed single spec is outside
		// the drop bookkeeping PS5027 reuses — advisory in both cases
		// (same guard family as PS2130).
		if fixable > 0 && (ps2129MultipleFmtSpecs(f) || ps2130RenamedFmt(f)) {
			importsOK = false
		}
		if !importsOK {
			// cgo file needing the drop, a fmt spec we could not locate, or
			// a renamed/duplicated fmt import: keep every report advisory.
			for i := range sites {
				sites[i].fix = nil
			}
		} else if len(importEdits) > 0 {
			// All fixes of a run are applied together, so only the first
			// fixable site carries the import edits (same convention as
			// PS2130/PS2129).
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
				Message: ps5027Msg,
			}
			if st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps5027Fix builds the rewrite for one call — the fmt.Sprintf wrapper
// around the literal is deleted and the literal's text stays untouched
// in place (same technique as PS2130) — or nil when a guard fails and
// the report must stay advisory. Import edits are appended later, once
// per file.
func ps5027Fix(f *ast.File, call *ast.CallExpr, lit *ast.BasicLit, parent ast.Node) *analysis.SuggestedFix {
	// A call is a valid statement; a bare string literal is not ("string
	// literal evaluated but not used"), and go/defer require a call
	// outright — advisory.
	switch parent.(type) {
	case *ast.ExprStmt, *ast.GoStmt, *ast.DeferStmt:
		return nil
	}
	// The deleted spans are the call text around the literal; a comment
	// there would be silently destroyed — advisory then. A string literal
	// is self-delimiting, so it never needs parentheses in the call's
	// context.
	if ps2111CommentIn(f, call.Pos(), lit.Pos()) ||
		ps2111CommentIn(f, lit.End(), call.End()) {
		return nil
	}
	return &analysis.SuggestedFix{
		Message: "replace fmt.Sprintf(" + lit.Value + ") with the string literal itself",
		TextEdits: []analysis.TextEdit{
			{Pos: call.Pos(), End: lit.Pos()},
			{Pos: lit.End(), End: call.End()},
		},
	}
}
