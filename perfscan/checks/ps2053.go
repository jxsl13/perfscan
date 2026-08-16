package checks

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS2053 reports for k := range m { ... m[k] ... }: ranging a map by key alone
// and then re-indexing m[k] in the body re-hashes the key the range already
// walked past. for k, v := range m { ... v ... } binds the value the range
// already has, so the body's reads become plain local loads. The range sibling
// of PS2052 (the if-condition double lookup) and PS3021 (the comma-ok form).
var PS2053 = register(&lint.Check{
	ID:       "PS2053",
	Category: "indirect",
	Slug:     "range-map-key-rehash",
	Level:    lint.LevelStructured,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "ranging a map by key and re-indexing m[k] in the body re-hashes the key the range already walked; bind the value with for k, v := range m",
		Text: `for k := range m { use(m[k]) } walks the map's buckets to produce each
key, then m[k] hashes that key again and re-scans its bucket to fetch the value
the walk already had in hand. for k, v := range m { use(v) } takes the value
straight from the range, so every body read becomes a plain local load and the
per-iteration re-hash disappears.

The rewrite is byte-for-byte identical when the map is not mutated for the
current key within the iteration: the value the range binds to v is exactly what
m[k] returns for that k, so replacing the reads with v changes nothing. The fix
fires only on the textbook-safe shape and stays silent otherwise:

  - the range is for k := range m — a := range with a key and NO value, whose key
    is a plain non-blank identifier and whose ranged expression is an identifier
    of map type (a call or selector is not re-indexable by the same text, so it
    never matches);
  - the map's value type is NOT a struct or array: for k, v := range m copies the
    value into v every iteration, so binding a large value that the body reads
    only conditionally could copy more than the original m[k] did — those maps
    are left alone. Basic, pointer, slice, map, channel and interface values copy
    a word or two and are always a win;
  - the body contains at least one single-value read of m[k] (same map object,
    same key) — a comma-ok read (v2, ok := m[k]) cannot become v and is left in
    place;
  - the body must NOT mutate the lookup: an assignment to m[...] / m[...] op= /
    delete(m, ...) / reassignment of m or k anywhere keeps the check silent, and
    a qualifying read inside a nested function literal (which could run after m
    changes) stays silent too.
When all hold, the fix inserts the value binding (v, or val/mv/... if v is taken)
into the range clause and replaces every qualifying body read with it, keeping k,
m and the map verbatim. A comment overlapping an edited span keeps the report
advisory.`,
		Before: `for k := range counts {
	total += counts[k]
}`,
		After: `for k, v := range counts {
	total += v
}`,
		MeasuredWin: `BenchmarkPS2053 (4096-entry map[string]int, Apple M2 Pro, go1.26): ` +
			`for k := range m { s += m[k] } ~64.7 us/op -> for k, v := range m { s += v } ` +
			`~28.1 us/op (~2.3x, 0 allocs/op either way) — the per-iteration hash+bucket-scan is removed.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2053",
		Doc:  "map ranged by key then re-indexed m[k] in the body; bind the value with for k, v := range m",
		Run:  runPS2053,
	},
})

const ps2053Msg = "map is ranged by key then re-indexed for the same key — the range already has the value; bind it (for k, v := range m) and reuse v instead of re-indexing m[k]"

func runPS2053(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			rng, ok := n.(*ast.RangeStmt)
			if !ok || rng.Tok != token.DEFINE || rng.Value != nil || rng.Key == nil {
				return true
			}
			keyIdent, ok := rng.Key.(*ast.Ident)
			if !ok || keyIdent.Name == "_" {
				return true
			}
			// The ranged expression must be a plain identifier of map type.
			mapIdent, ok := rng.X.(*ast.Ident)
			if !ok {
				return true
			}
			mt, ok := typeOfUnderlying(pass, rng.X).(*types.Map)
			if !ok || mt == nil {
				return true
			}
			// A struct/array value would be copied into v every iteration; a
			// body that reads it only sometimes could copy more than m[k] did.
			if elem := mt.Elem().Underlying(); elem != nil {
				if _, isStruct := elem.(*types.Struct); isStruct {
					return true
				}
				if _, isArray := elem.(*types.Array); isArray {
					return true
				}
			}
			mObj := pass.TypesInfo.Uses[mapIdent]
			kObj := pass.TypesInfo.Defs[keyIdent]
			if mObj == nil || kObj == nil {
				return true
			}

			reads, safe := ps2053BodyReads(pass, rng.Body, mObj, kObj)
			if !safe || len(reads) == 0 {
				return true
			}

			commented := false
			for _, r := range reads {
				if ps2111CommentIn(f, r.Pos(), r.End()) {
					commented = true
					break
				}
			}

			diag := analysis.Diagnostic{Pos: rng.Key.Pos(), End: rng.Key.End(), Message: ps2053Msg}
			if !commented {
				name := ps2053FreshName(pass, rng, keyIdent.Name)
				nameBytes := []byte(name)
				edits := []analysis.TextEdit{
					{Pos: rng.Key.End(), End: rng.Key.End(), NewText: []byte(", " + name)},
				}
				for _, r := range reads {
					edits = append(edits, analysis.TextEdit{Pos: r.Pos(), End: r.End(), NewText: nameBytes})
				}
				diag.SuggestedFixes = []analysis.SuggestedFix{{
					Message:   "bind the value (for k, " + name + " := range m) and reuse " + name,
					TextEdits: edits,
				}}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}

// ps2053BodyReads collects every single-value read of m[k] in the range body
// (m the ranged map object, k the range key) and reports whether the rewrite is
// safe. It is unsafe if the body mutates the lookup (write to m[...], m[...] op=,
// delete(m, ...), reassignment of m or k) or if a qualifying read sits inside a
// nested function literal. Comma-ok reads (v2, ok := m[k]) are left in place —
// they cannot become the single value v — and never count toward the reads.
func ps2053BodyReads(pass *analysis.Pass, body *ast.BlockStmt, mObj, kObj types.Object) (reads []*ast.IndexExpr, safe bool) {
	sameIndex := func(ix *ast.IndexExpr) bool {
		xi, ok := ix.X.(*ast.Ident)
		if !ok || pass.TypesInfo.Uses[xi] != mObj {
			return false
		}
		ki, ok := ix.Index.(*ast.Ident)
		return ok && pass.TypesInfo.Uses[ki] == kObj
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
			return o == mObj || o == kObj
		case *ast.IndexExpr:
			xi, ok := x.X.(*ast.Ident)
			return ok && pass.TypesInfo.Uses[xi] == mObj
		}
		return false
	}

	// Mutation scan over the whole body (closures included).
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
				if b, isBuiltin := pass.TypesInfo.Uses[id].(*types.Builtin); isBuiltin && b.Name() == "delete" {
					if bi, ok := st.Args[0].(*ast.Ident); ok && pass.TypesInfo.Uses[bi] == mObj {
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

	// Comma-ok reads (2-LHS assign whose sole RHS is m[k]) cannot become v.
	commaOk := map[*ast.IndexExpr]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		if as, ok := n.(*ast.AssignStmt); ok && len(as.Lhs) == 2 && len(as.Rhs) == 1 {
			if ix, ok := as.Rhs[0].(*ast.IndexExpr); ok && sameIndex(ix) {
				commaOk[ix] = true
			}
		}
		return true
	})

	// Reads inside nested function literals are unsafe to rewrite.
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

	ast.Inspect(body, func(n ast.Node) bool {
		ix, ok := n.(*ast.IndexExpr)
		if !ok || !sameIndex(ix) || commaOk[ix] {
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
	return reads, true
}

// ps2053FreshName returns a value name not colliding with the key name, any name
// declared in the range body, or anything visible at the range statement.
func ps2053FreshName(pass *analysis.Pass, rng *ast.RangeStmt, keyName string) string {
	taken := map[string]bool{keyName: true, "_": true}
	ast.Inspect(rng.Body, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && pass.TypesInfo.Defs[id] != nil {
			taken[id.Name] = true
		}
		return true
	})
	scope := pass.Pkg.Scope().Innermost(rng.Pos())
	for _, cand := range []string{"v", "val", "mv", "mval", "got", "cur", "value"} {
		if taken[cand] {
			continue
		}
		if scope != nil {
			if _, obj := scope.LookupParent(cand, rng.Pos()); obj != nil {
				continue
			}
		}
		return cand
	}
	for i := 2; ; i++ {
		cand := "v" + string(rune('0'+i%10))
		if !taken[cand] {
			return cand
		}
	}
}
