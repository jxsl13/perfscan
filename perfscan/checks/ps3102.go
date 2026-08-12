package checks

import (
	"go/ast"
	"go/token"
	"go/types"
	"go/version"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS3102 reports the map-clearing idiom `for k := range m { delete(m, k) }`
// and rewrites the whole loop to `clear(m)`.
var PS3102 = register(&lint.Check{
	ID:       "PS3102",
	Category: "indirect",
	Slug:     "map-clear-loop",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "deleting every key in a range loop is O(n) with rehashing; clear(m) is the direct idiom",
		Text: `The loop for k := range m { delete(m, k) } empties a map
key-by-key: every iteration hashes the key a second time to find and
remove its entry. The Go 1.21 builtin clear(m) says the same thing
directly and lets the runtime wipe the map in one shot (runtime.mapclear).
Per the Go spec, clear on a map deletes all entries, leaving len(m) == 0 —
exactly the loop's result — and clear(nil-map) is a no-op just as the loop
is. The rewrite is import-neutral.

Only the exact shape is matched and fixed: the range binds ONLY the key
with :=, the ranged operand is a plain identifier of map type, and the
body is the single statement delete(m, k) calling the builtin delete on
the SAME map variable with the range key. Anything else in the body — or
a different map, a shadowed delete, a value variable — is left alone.

Two sub-cases keep an advisory report without a fix:

  - a key type that can hold NaN (floats, complex, or any struct/array/
    interface embedding them): delete(m, k) can never remove a NaN key
    (NaN != NaN), while clear does — the rewrite would not be
    bit-identical, it would clear MORE than the loop;
  - a file whose language version is below go1.21, or a scope where the
    identifier clear is shadowed by a user declaration.

Honesty note: for NaN-free key types gc has pattern-matched this exact
loop into runtime.mapclear since Go 1.11, so the fix is usually a clarity
win at performance parity — but any drift from the exact shape silently
loses that compiler optimization, while clear(m) always takes the fast
path.`,
		Before: `for k := range m {
	delete(m, k)
}`,
		After: `clear(m)`,
		MeasuredWin: `BenchmarkPS3102 (refill 1024 int keys then empty the
map, Apple M2 Pro): parity — 8.7 µs/op before vs 8.8 µs/op after
(refill-dominated), 0 allocs either way. Expected: gc already compiles
the exact range-delete loop to the same runtime.mapclear call clear(m)
makes. The win is directness and robustness: any deviation from the
exact loop shape silently drops the compiler pattern-match, clear(m)
cannot.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS3102",
		Doc:  "map emptied with a range-delete loop instead of clear(m)",
		Run:  runPS3102,
	},
})

func runPS3102(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		clearOK := ps3102ClearAvailable(pass, f)
		ast.Inspect(f, func(n ast.Node) bool {
			rs, ok := n.(*ast.RangeStmt)
			if !ok {
				return true
			}
			name, keyType, ok := ps3102Match(pass, rs)
			if !ok {
				return true
			}
			repl := "clear(" + name + ")"
			msg := name + " is emptied key-by-key by a range-delete loop, hashing every key a second time; " + repl + " empties the map in one call"
			diag := analysis.Diagnostic{
				Pos:     rs.Pos(),
				End:     rs.End(),
				Message: msg,
			}
			switch {
			case !ps3102Reflexive(keyType):
				// delete(m, k) cannot remove a NaN key (NaN != NaN), but
				// clear does — the rewrite would clear MORE than the loop.
				diag.Message = msg + " — advisory: the key type can hold NaN, which delete cannot remove but clear does, so the rewrite is not bit-identical"
			case !clearOK, !builtinInScope(pass, rs.Pos(), "clear"), ps3102CommentsOverlap(f, rs):
				// Pre-1.21 file, shadowed clear, or a comment inside the
				// replaced span: report without a fix.
			default:
				diag.SuggestedFixes = []analysis.SuggestedFix{{
					Message: "replace the range-delete loop with " + repl,
					TextEdits: []analysis.TextEdit{
						{Pos: rs.Pos(), End: rs.End(), NewText: []byte(repl)},
					},
				}}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}

// ps3102Match reports whether rs is EXACTLY `for k := range m { delete(m, k) }`
// with m a plain identifier of map type and delete the builtin, returning
// m's name and the map's key type. The key must be freshly declared by the
// loop (:=): a `for k = range m` form assigns some final key to an outer
// variable, which clear(m) would not, so it is not matched at all.
func ps3102Match(pass *analysis.Pass, rs *ast.RangeStmt) (mapName string, keyType types.Type, ok bool) {
	if rs.Tok != token.DEFINE || rs.Value != nil || rs.Body == nil || len(rs.Body.List) != 1 {
		return "", nil, false
	}
	key, ok := rs.Key.(*ast.Ident)
	if !ok || key.Name == "_" {
		return "", nil, false
	}
	keyObj := pass.TypesInfo.Defs[key]
	if keyObj == nil {
		return "", nil, false
	}
	mIdent, ok := rs.X.(*ast.Ident)
	if !ok {
		return "", nil, false
	}
	mObj := pass.TypesInfo.Uses[mIdent]
	if mObj == nil {
		return "", nil, false
	}
	mType := pass.TypesInfo.TypeOf(rs.X)
	if mType == nil {
		return "", nil, false
	}
	mt, ok := mType.Underlying().(*types.Map)
	if !ok {
		return "", nil, false
	}
	es, ok := rs.Body.List[0].(*ast.ExprStmt)
	if !ok {
		return "", nil, false
	}
	call, ok := es.X.(*ast.CallExpr)
	if !ok || len(call.Args) != 2 || call.Ellipsis.IsValid() {
		return "", nil, false
	}
	fn, ok := call.Fun.(*ast.Ident)
	if !ok {
		return "", nil, false
	}
	// Type information pins the callee to the predeclared delete builtin;
	// a shadowing function or variable named delete does not resolve to a
	// *types.Builtin and is rejected.
	if b, ok := pass.TypesInfo.Uses[fn].(*types.Builtin); !ok || b.Name() != "delete" {
		return "", nil, false
	}
	a0, ok := call.Args[0].(*ast.Ident)
	if !ok || pass.TypesInfo.Uses[a0] != mObj {
		return "", nil, false
	}
	a1, ok := call.Args[1].(*ast.Ident)
	if !ok || pass.TypesInfo.Uses[a1] != keyObj {
		return "", nil, false
	}
	return mIdent.Name, mt.Key(), true
}

// ps3102Reflexive reports whether == is reflexive for every value of t —
// i.e. t cannot hold a NaN anywhere. This mirrors the compiler's own guard
// on the range-delete -> mapclear rewrite: a NaN key is never equal to
// itself, so delete cannot remove it but clear would.
func ps3102Reflexive(t types.Type) bool {
	switch u := t.Underlying().(type) {
	case *types.Basic:
		if u.Kind() == types.Invalid {
			return false
		}
		return u.Info()&(types.IsFloat|types.IsComplex) == 0
	case *types.Pointer, *types.Chan:
		return true
	case *types.Struct:
		for i := 0; i < u.NumFields(); i++ {
			if !ps3102Reflexive(u.Field(i).Type()) {
				return false
			}
		}
		return true
	case *types.Array:
		return ps3102Reflexive(u.Elem())
	default:
		// Interfaces (and type parameters, whose underlying type is their
		// constraint interface) may hold a float dynamically; anything
		// else is not a valid map key — be conservative either way.
		return false
	}
}

// ps3102ClearAvailable reports whether f's language version has the clear
// builtin (go1.21). An unknown or unparseable version does not block the
// fix: perfscan itself requires a far newer toolchain, so an empty version
// means "module default", not "ancient".
func ps3102ClearAvailable(pass *analysis.Pass, f *ast.File) bool {
	v := ""
	if pass.TypesInfo.FileVersions != nil {
		v = pass.TypesInfo.FileVersions[f]
	}
	if v == "" && pass.Pkg != nil {
		v = pass.Pkg.GoVersion()
	}
	if v == "" {
		return true
	}
	lang := version.Lang(v)
	if lang == "" {
		return true
	}
	return version.Compare(lang, "go1.21") >= 0
}

// ps3102CommentsOverlap reports whether a comment overlaps the replaced
// loop range — the single-call replacement could not reproduce it.
func ps3102CommentsOverlap(f *ast.File, rs *ast.RangeStmt) bool {
	for _, cg := range f.Comments {
		if cg.End() > rs.Pos() && cg.Pos() < rs.End() {
			return true
		}
	}
	return false
}
