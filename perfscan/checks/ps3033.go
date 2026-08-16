package checks

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS3033 reports the guarded map delete idiom `if _, ok := m[k]; ok {
// delete(m, k) }`: the comma-ok guard hashes the key and scans the bucket
// solely to decide whether to call delete, but the builtin delete on an
// absent key is a spec-guaranteed no-op, so the guard is pure overhead. On
// the present-key path the key is hashed twice; a plain `delete(m, k)`
// hashes it once. This is the delete sibling of PS3021 (map-double-lookup),
// which removes the second hash for the read idiom.
var PS3033 = register(&lint.Check{
	ID:       "PS3033",
	Category: "indirect",
	Slug:     "guarded-map-delete",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "a comma-ok presence guard around delete(m, k) hashes the key twice; the builtin delete of an absent key is already a no-op, so call it unconditionally",
		Text: `if _, ok := m[k]; ok { delete(m, k) } probes the map TWICE on the
present-key path: the comma-ok init hashes k and scans its bucket just
to produce the ok bit, then delete(m, k) hashes and scans AGAIN to do
the actual removal. But the guard buys nothing: the Go spec defines
delete on a key that is not in the map as a no-op, and delete on a nil
map is also a no-op (it does not panic, unlike an insert). Dropping the
guard — a plain delete(m, k) — hashes the key once, removes an if
branch and the throwaway ok local, and is the idiomatic spelling.

The rewrite is bit-identical on EVERY input, case by case:

  - Present key: original deletes, rewrite deletes — same final map.
  - Absent key: original's ok is false and does nothing; rewrite's
    delete of an absent key is a documented no-op — same final map.
  - Nil map: reading a nil map is legal (ok is false), and delete on a
    nil map is a no-op — both sides do nothing.
  - NaN float key: a NaN key can be stored but never matched, so the
    guard's ok is always false (original never deletes) and delete(m,
    NaN) also matches nothing — identical either way.

The ok variable is if-init-scoped and, in the matched shape, used only
as the condition, so dropping it is unobservable. The only remaining
difference is evaluation count: the original evaluates m and k twice
(guard + delete), the rewrite once. The check therefore fires only when
both m and k are side-effect-free expressions — a plain identifier,
field selector, or literal/constant; never a call, channel receive,
index, or type assertion — exactly the purity gate PS3021 applies,
making the single evaluation observationally identical.

The matched shape is deliberately exact and everything else stays
silent:

  - The if has NO else branch, its init is exactly ` + "`_, ok := m[k]`" + `
    (blank value, := , one map index on the right), and the condition
    is exactly that ok identifier — no negation, no compound
    expression (a negated guard means "delete only if ABSENT", which
    is dead code but not this pattern).
  - The body is EXACTLY the single statement delete(m, k) calling the
    predeclared builtin delete (a shadowing user function named delete
    is rejected via the type info), whose two arguments are
    syntactically identical to the guard's m and k and resolve to the
    same objects.
  - m's type must be a map (comma-ok over a channel receive or type
    assertion never matches, since the init must be an index
    expression).

A comment anywhere inside the if statement keeps the report advisory
(no auto-fix), so no comment text is ever deleted. When the if is the
else-branch of another if (else if _, ok := ...), the report is also
advisory: splicing a bare statement after else would not parse, and
perfscan does not restructure the outer if.`,
		Before: `if _, ok := m[k]; ok {
	delete(m, k)
}`,
		After: `delete(m, k)`,
		MeasuredWin: `BenchmarkPS3033 (1024 string-keyed present-key deletes per
iteration, map refilled identically on both sides so every delete hits,
Apple M2 Pro, go1.26): if _, ok := m[k]; ok { delete(m, k) } ~52.5
us/op -> delete(m, k) ~46.7 us/op (~1.13x end-to-end INCLUDING the
identical refill that dominates the loop; the guard's extra hash+
bucket-scan per present key is what the ~6 us delta removes, 0
allocs/op either way).`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS3033",
		Doc:  "comma-ok presence guard around delete(m, k); delete of an absent key is a no-op, so the guard only doubles the hashing",
		Run:  runPS3033,
	},
})

const ps3033Msg = "the presence guard around delete is pure overhead — delete of an absent key is a no-op, and the guard hashes the key a second time on the present path; call delete(m, k) unconditionally"

func runPS3033(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		astutil.WithStack(f, func(n ast.Node, stack []ast.Node) bool {
			ifStmt, ok := n.(*ast.IfStmt)
			if !ok || ifStmt.Init == nil || ifStmt.Else != nil {
				return true
			}
			as, ok := ifStmt.Init.(*ast.AssignStmt)
			if !ok || as.Tok != token.DEFINE || len(as.Lhs) != 2 || len(as.Rhs) != 1 {
				return true
			}
			// LHS: blank value, named ok.
			blank, ok := as.Lhs[0].(*ast.Ident)
			if !ok || blank.Name != "_" {
				return true
			}
			okIdent, ok := as.Lhs[1].(*ast.Ident)
			if !ok || okIdent.Name == "_" {
				return true
			}
			// Condition must be exactly the ok identifier (same object).
			condIdent, ok := ifStmt.Cond.(*ast.Ident)
			if !ok {
				return true
			}
			okObj := pass.TypesInfo.Defs[okIdent]
			if okObj == nil || pass.TypesInfo.Uses[condIdent] != okObj {
				return true
			}
			// RHS must be m[k] over a map, with side-effect-free m and k
			// (the original evaluates both twice, the rewrite once).
			idx, ok := as.Rhs[0].(*ast.IndexExpr)
			if !ok {
				return true
			}
			if mt, isMap := typeOfUnderlying(pass, idx.X).(*types.Map); !isMap || mt == nil {
				return true
			}
			if !ps3021PureKey(idx.X) || !ps3021PureKey(idx.Index) {
				return true
			}
			// Body: EXACTLY one statement, the builtin delete(m, k) whose
			// args match the guard's m and k syntactically AND by object.
			if len(ifStmt.Body.List) != 1 {
				return true
			}
			es, ok := ifStmt.Body.List[0].(*ast.ExprStmt)
			if !ok {
				return true
			}
			call, ok := es.X.(*ast.CallExpr)
			if !ok || len(call.Args) != 2 {
				return true
			}
			fn, ok := call.Fun.(*ast.Ident)
			if !ok {
				return true
			}
			if b, isBuiltin := pass.TypesInfo.Uses[fn].(*types.Builtin); !isBuiltin || b.Name() != "delete" {
				// A shadowing user function named delete is NOT a no-op on
				// absent keys; only the predeclared builtin qualifies.
				return true
			}
			mText := exprTextRendered(idx.X)
			kText := exprTextRendered(idx.Index)
			if exprTextRendered(call.Args[0]) != mText || exprTextRendered(call.Args[1]) != kText {
				return true
			}
			if ps3021BaseObj(pass, call.Args[0]) != ps3021BaseObj(pass, idx.X) ||
				ps3021BaseObj(pass, call.Args[1]) != ps3021BaseObj(pass, idx.Index) {
				return true
			}

			diag := analysis.Diagnostic{Pos: ifStmt.Pos(), End: ifStmt.End(), Message: ps3033Msg}

			// Advisory when a comment lives anywhere inside the if (the fix
			// would delete it), or when the if is an else-branch (a bare
			// statement cannot follow `else`).
			elseBranch := false
			if len(stack) > 0 {
				if parent, isIf := stack[len(stack)-1].(*ast.IfStmt); isIf && parent.Else == ifStmt {
					elseBranch = true
				}
			}
			switch {
			case elseBranch:
				diag.Message = ps3033Msg + "; the guard is the else-branch of another if, where a bare statement cannot replace it — the automatic fix is withheld; rewrite by hand"
			case ps2111CommentIn(f, ifStmt.Pos(), ifStmt.End()):
				diag.Message = ps3033Msg + "; a comment inside the rewritten syntax withholds the automatic fix — rewrite by hand"
			default:
				diag.SuggestedFixes = []analysis.SuggestedFix{{
					Message:   "drop the presence guard and call delete(" + mText + ", " + kText + ") unconditionally",
					TextEdits: []analysis.TextEdit{{Pos: ifStmt.Pos(), End: ifStmt.End(), NewText: []byte("delete(" + mText + ", " + kText + ")")}},
				}}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}
