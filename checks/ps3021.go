package checks

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS3021 reports the map double-lookup idiom `if _, ok := m[k]; ok { ... m[k]
// ... }`: the comma-ok init already read the value, then the body hashes the
// SAME key again to re-read it. Binding the value once (`v, ok := m[k]`) and
// reusing v in the body halves the hashing.
var PS3021 = register(&lint.Check{
	ID:       "PS3021",
	Category: "indirect",
	Slug:     "map-double-lookup",
	Level:    lint.LevelStructured,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "a map looked up in a comma-ok guard and then re-read in the body for the same key hashes the key twice; bind the value once",
		Text: `if _, ok := m[k]; ok { use(m[k]) } evaluates m[k] TWICE for
the same key: once in the comma-ok init (which reads and DISCARDS the
value into _) and once per body re-read. Each map index is a hash of k
plus a bucket scan; the second lookup reproduces work the first already
did. Binding the value in the guard — if v, ok := m[k]; ok { use(v) } —
keeps the single existing lookup and reuses its result, so the body's
reads become plain local loads.

The rewrite is byte-for-byte identical: the value the body reads via
m[k] is exactly the value the comma-ok init already read, PROVIDED the
map is not mutated for that key between the guard and the reads. The
fix therefore fires only on the textbook-safe shape and stays silent
otherwise:

  - The if-init must be exactly ` + "`_, ok := m[k]`" + ` — a := with two
    names whose first is the blank identifier (the discarded value),
    one right-hand index expression m[k], and the if condition must be
    exactly that ok identifier. A used value name (v, ok := ...) already
    avoids the double lookup, and a negated/compound condition changes
    control flow, so both are skipped.
  - m's type must be a map. k must be side-effect-free (identifier,
    field selector, or literal/const only — never a call, channel
    receive, index or type assertion), so evaluating it once instead of
    twice is observationally identical.
  - The body must contain at least one read of m[k] whose m and k are
    syntactically identical to the guard's (same map object, same key
    text); with none the ` + "`_`" + ` form is already correct and nothing
    fires.
  - The body must NOT mutate the lookup: no m[...] = ... / m[...] op= /
    delete(m, ...) / reassignment of m or k anywhere in the body would
    change what a re-read returns, so any of them keeps the check
    silent. And no qualifying m[k] read may sit inside a nested function
    literal (a closure could be stored and called after m changes
    elsewhere, where the original re-reads but the bound value would
    not) — that stays silent too.

When all hold, the fix renames the blank value to a fresh name (v, or
v2/val/... if v is taken in scope) and replaces every qualifying body
m[k] read with it, keeping m, k and ok verbatim. A comment overlapping
any edited span keeps the report advisory (no auto-fix).`,
		Before: `if _, ok := cache[key]; ok {
	return cache[key], nil
}`,
		After: `if v, ok := cache[key]; ok {
	return v, nil
}`,
		MeasuredWin: `BenchmarkPS3021 (1024 string-keyed lookups, Apple M2 Pro,
go1.26): if _, ok := m[k]; ok { s += m[k] } ~18.1 us/op -> if v, ok :=
m[k]; ok { s += v } ~9.9 us/op (~1.83x, 0 allocs/op either way) — the
second hash+bucket-scan per key is removed.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS3021",
		Doc:  "map looked up in a comma-ok guard then re-read for the same key; bind the value once",
		Run:  runPS3021,
	},
})

const ps3021Msg = "map is looked up twice for the same key — the comma-ok guard already read the value; bind it (v, ok := m[k]) and reuse v instead of re-indexing m[k] in the body"

func runPS3021(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		astutil.WithStack(f, func(n ast.Node, stack []ast.Node) bool {
			ifStmt, ok := n.(*ast.IfStmt)
			if !ok || ifStmt.Init == nil {
				return true
			}
			as, ok := ifStmt.Init.(*ast.AssignStmt)
			if !ok || as.Tok != token.DEFINE || len(as.Lhs) != 2 || len(as.Rhs) != 1 {
				return true
			}
			// LHS: blank value, ok ident.
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
			// RHS must be m[k] over a map.
			idx, ok := as.Rhs[0].(*ast.IndexExpr)
			if !ok {
				return true
			}
			if mt, isMap := typeOfUnderlying(pass, idx.X).(*types.Map); !isMap || mt == nil {
				return true
			}
			if !ps3021PureKey(idx.Index) {
				return true
			}
			mText := exprTextRendered(idx.X)
			kText := exprTextRendered(idx.Index)
			mObj := ps3021BaseObj(pass, idx.X)

			// Collect qualifying body reads; bail if the body mutates the
			// lookup or a read hides in a closure.
			reads, safe := ps3021BodyReads(pass, ifStmt.Body, idx, mText, kText, mObj)
			if !safe || len(reads) == 0 {
				return true
			}

			// Comment overlapping any edited span -> advisory (report, no fix).
			spans := [][2]token.Pos{{blank.Pos(), blank.End()}}
			for _, r := range reads {
				spans = append(spans, [2]token.Pos{r.Pos(), r.End()})
			}
			commented := false
			for _, s := range spans {
				if ps2111CommentIn(f, s[0], s[1]) {
					commented = true
					break
				}
			}

			diag := analysis.Diagnostic{Pos: idx.Pos(), End: idx.End(), Message: ps3021Msg}
			if !commented {
				name := ps3021FreshName(pass, ifStmt, okIdent.Name)
				nameBytes := []byte(name)
				edits := []analysis.TextEdit{{Pos: blank.Pos(), End: blank.End(), NewText: nameBytes}}
				for _, r := range reads {
					edits = append(edits, analysis.TextEdit{Pos: r.Pos(), End: r.End(), NewText: nameBytes})
				}
				diag.SuggestedFixes = []analysis.SuggestedFix{{
					Message:   "bind the value (" + name + ", ok := " + mText + "[" + kText + "]) and reuse " + name,
					TextEdits: edits,
				}}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}

// typeOfUnderlying returns the underlying type of e, or nil.
func typeOfUnderlying(pass *analysis.Pass, e ast.Expr) types.Type {
	t := pass.TypesInfo.TypeOf(e)
	if t == nil {
		return nil
	}
	return t.Underlying()
}

// ps3021PureKey reports whether k is side-effect-free and cheap to evaluate
// twice: identifiers, field selectors, and literals only. Any call, channel
// receive, index, slice, or type assertion disqualifies it.
func ps3021PureKey(k ast.Expr) bool {
	pure := true
	ast.Inspect(k, func(n ast.Node) bool {
		if !pure {
			return false
		}
		switch n.(type) {
		case *ast.CallExpr, *ast.IndexExpr, *ast.IndexListExpr, *ast.SliceExpr,
			*ast.TypeAssertExpr, *ast.UnaryExpr, *ast.BinaryExpr, *ast.FuncLit:
			// UnaryExpr covers <-ch and &x; BinaryExpr could hide a call but
			// is conservatively excluded; a pure key is an ident/selector/lit.
			pure = false
			return false
		}
		return true
	})
	return pure
}

// ps3021BaseObj returns the object of the base identifier of a map expression
// (m for `m`, or s for `s.field`), used to confirm the body reads the SAME map.
func ps3021BaseObj(pass *analysis.Pass, e ast.Expr) types.Object {
	switch x := e.(type) {
	case *ast.Ident:
		if o := pass.TypesInfo.Uses[x]; o != nil {
			return o
		}
		return pass.TypesInfo.Defs[x]
	case *ast.SelectorExpr:
		return ps3021BaseObj(pass, x.X)
	case *ast.ParenExpr:
		return ps3021BaseObj(pass, x.X)
	}
	return nil
}

// ps3021BodyReads collects the body index expressions equal to the guard's
// m[k], and reports whether the body is safe to rewrite. It returns safe=false
// if the body mutates the lookup (assignment to m[...], m op= , delete(m,...),
// or reassignment of m or k) ANYWHERE, or if a qualifying m[k] read sits inside
// a nested function literal (a closure could run after m changes elsewhere, so
// the bound value would diverge from a re-read).
func ps3021BodyReads(pass *analysis.Pass, body *ast.BlockStmt, guard *ast.IndexExpr, mText, kText string, mObj types.Object) (reads []*ast.IndexExpr, safe bool) {
	kObj := ps3021BaseObj(pass, guard.Index) // nil for a literal key
	sameIndex := func(ix *ast.IndexExpr) bool {
		return exprTextRendered(ix.X) == mText && exprTextRendered(ix.Index) == kText &&
			ps3021BaseObj(pass, ix.X) == mObj
	}
	mutates := func(e ast.Expr) bool {
		for {
			p, ok := e.(*ast.ParenExpr)
			if !ok {
				break
			}
			e = p.X
		}
		switch x := e.(type) {
		case *ast.Ident:
			o := pass.TypesInfo.Uses[x]
			if o == nil {
				o = pass.TypesInfo.Defs[x]
			}
			return (mObj != nil && o == mObj) || (kObj != nil && o == kObj)
		case *ast.IndexExpr:
			return exprTextRendered(x.X) == mText && ps3021BaseObj(pass, x.X) == mObj
		}
		return false
	}

	// Pass 1 — mutation scan over the WHOLE body (closures included). Any
	// write to m/m[...]/k makes a re-read potentially differ from the guard.
	safe = true
	ast.Inspect(body, func(n ast.Node) bool {
		switch st := n.(type) {
		case *ast.AssignStmt:
			for _, lhs := range st.Lhs {
				if mutates(lhs) {
					safe = false
				}
			}
		case *ast.IncDecStmt:
			if mutates(st.X) {
				safe = false
			}
		case *ast.CallExpr:
			if id, ok := st.Fun.(*ast.Ident); ok && id.Name == "delete" && len(st.Args) >= 1 {
				// Only the predeclared builtin delete mutates the map; a
				// shadowing user function named delete does not.
				if b, isBuiltin := pass.TypesInfo.Uses[id].(*types.Builtin); isBuiltin && b.Name() == "delete" {
					if ps3021BaseObj(pass, st.Args[0]) == mObj {
						safe = false
					}
				}
			}
		}
		return true
	})
	if !safe {
		return nil, false
	}

	// Pass 2 — mark every IndexExpr that lives inside a nested function literal.
	inClosure := map[*ast.IndexExpr]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		fl, ok := n.(*ast.FuncLit)
		if !ok {
			return true
		}
		ast.Inspect(fl.Body, func(m ast.Node) bool {
			if ix, ok := m.(*ast.IndexExpr); ok {
				inClosure[ix] = true
			}
			return true
		})
		return true
	})

	// Pass 3 — collect qualifying reads; a closure read is unsafe to rewrite.
	ast.Inspect(body, func(n ast.Node) bool {
		ix, ok := n.(*ast.IndexExpr)
		if !ok || !sameIndex(ix) {
			return true
		}
		if inClosure[ix] {
			safe = false
			return false
		}
		reads = append(reads, ix)
		return true
	})
	if !safe {
		return nil, false
	}
	return reads, safe
}

// ps3021FreshName returns a value name not colliding with anything visible at
// the if-statement, the ok name, or names declared in the body.
func ps3021FreshName(pass *analysis.Pass, ifStmt *ast.IfStmt, okName string) string {
	taken := map[string]bool{okName: true, "_": true}
	// Names declared inside the body.
	ast.Inspect(ifStmt.Body, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok {
			if pass.TypesInfo.Defs[id] != nil {
				taken[id.Name] = true
			}
		}
		return true
	})
	scope := pass.Pkg.Scope().Innermost(ifStmt.Pos())
	for _, cand := range []string{"v", "val", "mv", "mval", "got", "cur", "value"} {
		if taken[cand] {
			continue
		}
		if scope != nil {
			if _, obj := scope.LookupParent(cand, ifStmt.Pos()); obj != nil {
				continue
			}
		}
		return cand
	}
	// Fallback: numbered.
	for i := 2; ; i++ {
		cand := "v" + string(rune('0'+i%10))
		if !taken[cand] {
			return cand
		}
	}
}
