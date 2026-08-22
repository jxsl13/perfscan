package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS2052 reports the plain-condition map double-lookup `if m[k] <op> c { ...
// m[k] ... }`: the guard reads m[k] and the body (or the rest of the condition)
// re-reads the SAME key, hashing k twice or more. Binding the value once in the
// if-init — `if v := m[k]; v <op> c { ... v ... }` — keeps a single lookup and
// makes the reuses plain local loads. The non-comma-ok sibling of PS3021 (which
// covers `if _, ok := m[k]; ok { ... m[k] ... }`).
var PS2052 = register(&lint.Check{
	ID:       "PS2052",
	Category: "indirect",
	Slug:     "map-plain-double-lookup",
	Level:    lint.LevelStructured,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "a map read in a plain if-condition and again in the body for the same key hashes the key twice; bind the value in the if-init",
		Text: `if m[k] > 0 { total += m[k] } evaluates m[k] at least TWICE for the
same key: once in the condition and once per body (or further condition)
re-read. Each map index is a hash of k plus a bucket scan; every repeat
reproduces work the first lookup already did. Binding the value in the if-init —
if v := m[k]; v > 0 { total += v } — keeps the single existing lookup and reuses
its result, so the other reads become plain local loads. The map's zero value is
returned for an absent key in both forms, so no comma-ok is needed.

The rewrite is byte-for-byte identical: every reused m[k] returns exactly the
value the guard read, PROVIDED the map is not mutated for that key between the
reads. The fix therefore fires only on the textbook-safe shape and stays silent
otherwise:

  - The if has NO existing init (a comma-ok guard is PS3021's domain), and its
    condition contains a single-value read of m[k] over a map, with m and k
    syntactically stable.
  - k must be side-effect-free (identifier, field selector, or literal/const
    only — never a call, channel receive, index, or type assertion), so
    evaluating it once instead of many times is observationally identical.
  - The condition and body together must contain at least TWO reads of that same
    m[k] (same map object, same key text) — otherwise there is no repeat to
    remove.
  - The CONDITION must contain no call: a call sequenced between two condition
    reads could mutate m[k], which the single bound value would not observe.
  - Neither the condition nor the body may mutate the lookup: an assignment to
    m[...] / m[...] op= / delete(m, ...) / reassignment of m or k anywhere would
    change what a re-read returns, so any of them keeps the check silent. A
    qualifying read inside a nested function literal also stays silent (a closure
    could run after m changes elsewhere).

When all hold, the fix inserts the init (v, or val/mv/... if v is taken in
scope), rebinding m[k] once, and replaces every qualifying read in the condition
and body with it, keeping m, k and the operator verbatim. A comment overlapping
any edited span keeps the report advisory (no auto-fix).`,
		Before: `if counts[k] > 0 {
	total += counts[k]
}`,
		After: `if v := counts[k]; v > 0 {
	total += v
}`,
		MeasuredWin: `BenchmarkPS2052 (1024 string-keyed lookups, Apple M2 Pro, go1.26): ` +
			`if m[k] > 0 { s += m[k] } ~13.4 us/op -> if v := m[k]; v > 0 { s += v } ~8.4 us/op ` +
			`(~1.6x, 0 allocs/op either way) — the repeated hash+bucket-scan per key is removed. ` +
			`Mirrors PS3021's measured win for the comma-ok form.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2052",
		Doc:  "map read in a plain if-condition then re-read for the same key; bind the value in the if-init",
		Run:  runPS2052,
	},
})

const ps2052Msg = "map is looked up twice for the same key — bind the value in the if-init (if v := m[k]; ...) and reuse v instead of re-indexing m[k]"

func runPS2052(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		astutil.WithStack(f, func(n ast.Node, stack []ast.Node) bool {
			ifStmt, ok := n.(*ast.IfStmt)
			if !ok || ifStmt.Init != nil {
				return true
			}
			// Gather the distinct map-index groups appearing in the condition,
			// in source order, and use the first one that has >= 2 total reads
			// and passes the safety scan.
			var guard *ast.IndexExpr
			var reads []*ast.IndexExpr
			seen := map[string]bool{}
			ast.Inspect(ifStmt.Cond, func(cn ast.Node) bool {
				if guard != nil {
					return false
				}
				ix, ok := cn.(*ast.IndexExpr)
				if !ok {
					return true
				}
				if mt, isMap := typeOfUnderlying(pass, ix.X).(*types.Map); !isMap || mt == nil {
					return true
				}
				if !ps3021PureKey(ix.Index) {
					return true
				}
				key := exprTextRendered(ix.X) + "\x00" + exprTextRendered(ix.Index)
				if seen[key] {
					return true
				}
				seen[key] = true
				rs, safe := ps2052CollectReads(pass, ifStmt, ix)
				if safe && len(rs) >= 2 {
					guard, reads = ix, rs
					return false
				}
				return true
			})
			if guard == nil {
				return true
			}

			// A comment overlapping any edited span (a replaced read or the
			// insertion point before the condition) -> advisory.
			commented := ps2111CommentIn(f, ifStmt.Cond.Pos(), ifStmt.Cond.Pos())
			for _, r := range reads {
				if ps2111CommentIn(f, r.Pos(), r.End()) {
					commented = true
					break
				}
			}

			diag := analysis.Diagnostic{Pos: guard.Pos(), End: guard.End(), Message: ps2052Msg}
			if !commented {
				name := ps3021FreshName(pass, ifStmt, "")
				nameBytes := []byte(name)
				guardText := exprTextRendered(guard)
				edits := []analysis.TextEdit{
					{Pos: ifStmt.Cond.Pos(), End: ifStmt.Cond.Pos(), NewText: []byte(name + " := " + guardText + "; ")},
				}
				for _, r := range reads {
					edits = append(edits, analysis.TextEdit{Pos: r.Pos(), End: r.End(), NewText: nameBytes})
				}
				diag.SuggestedFixes = []analysis.SuggestedFix{{
					Message:   "bind the value (" + name + " := " + guardText + ") and reuse " + name,
					TextEdits: edits,
				}}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}

// ps2052CollectReads collects every read of guard's m[k] in the condition and
// body of ifStmt and reports whether the rewrite is safe. It is unsafe if the
// condition contains any call (which could mutate m[k] between two reads), if
// the condition or body mutates the lookup (write to m[...], m[...] op=,
// delete(m, ...), reassignment of m or k), or if a qualifying read sits inside a
// nested function literal.
func ps2052CollectReads(pass *analysis.Pass, ifStmt *ast.IfStmt, guard *ast.IndexExpr) (reads []*ast.IndexExpr, safe bool) {
	mText := exprTextRendered(guard.X)
	kText := exprTextRendered(guard.Index)
	mObj := ps3021BaseObj(pass, guard.X)
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

	// A call anywhere in the CONDITION could mutate m[k] between reads.
	condHasCall := false
	ast.Inspect(ifStmt.Cond, func(n ast.Node) bool {
		if _, ok := n.(*ast.CallExpr); ok {
			condHasCall = true
			return false
		}
		return true
	})
	if condHasCall {
		return nil, false
	}

	// Mutation scan over condition + body (closures included).
	safe = true
	scanMut := func(root ast.Node) {
		ast.Inspect(root, func(n ast.Node) bool {
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
					if b, isBuiltin := pass.TypesInfo.Uses[id].(*types.Builtin); isBuiltin && b.Name() == "delete" {
						if ps3021BaseObj(pass, st.Args[0]) == mObj {
							safe = false
						}
					}
				}
			}
			return true
		})
	}
	scanMut(ifStmt.Cond)
	scanMut(ifStmt.Body)
	if !safe {
		return nil, false
	}

	// Mark reads inside nested function literals (condition + body).
	inClosure := map[*ast.IndexExpr]bool{}
	markClosures := func(root ast.Node) {
		ast.Inspect(root, func(n ast.Node) bool {
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
	}
	markClosures(ifStmt.Cond)
	markClosures(ifStmt.Body)

	// Collect qualifying reads in source order: condition first, then body.
	collect := func(root ast.Node) {
		ast.Inspect(root, func(n ast.Node) bool {
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
	}
	collect(ifStmt.Cond)
	collect(ifStmt.Body)
	if !safe {
		return nil, false
	}
	return reads, true
}
