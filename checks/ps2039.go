package checks

import (
	"go/ast"
	"go/token"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS2039 reports a map declared with no size hint that is populated by a
// maps.Copy from a known source later in the same block — the maps.Copy
// sibling of PS2104's loop-fill shape, which PS2104's for/range detector
// never sees. maps.Copy inserts len(src) entries one by one, so the
// unsized destination rehashes its way up exactly as a loop-filled map
// does; make(map[K]V, len(src)) reserves the buckets once.
var PS2039 = register(&lint.Check{
	ID:       "PS2039",
	Category: "alloc",
	Slug:     "maps-copy-into-unsized-map",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "an unsized map immediately populated by maps.Copy; make(map[K]V, len(src)) reserves the buckets once",
		Text: `A map made with no size hint starts at minimum bucket
capacity. maps.Copy(dst, src) is exactly "for k, v := range src {
dst[k] = v }": it inserts len(src) entries, and every time the growing
table crosses its load factor the runtime re-buckets each key inserted
so far — O(log n) growth passes over an increasingly large table.
Declaring the destination as make(map[K]V, len(src)) reserves the
buckets in one allocation and eliminates all incremental rehashing.
This is the maps.Copy sibling of PS2104's loop-fill shape, which that
check's for/range detector never matches.

A map's size hint is purely advisory and is never observable through
map semantics: iteration order is randomized independent of the hint,
and len, reads, writes and comma-ok lookups are unaffected. The rewrite
is bit-identical across all inputs — a nil source (len of a nil map is
0, and make(map[K]V, 0) is identical to make(map[K]V)), an empty
source, NaN or negative-zero float keys, any key type, invalid-UTF-8
string keys.

The automatic fix applies only when the proof is airtight:

  - the destination is freshly declared as make(map[K]V) or map[K]V{}
    and the maps.Copy statement is the IMMEDIATELY FOLLOWING statement
    in the same block — no code runs between the injected len(src) read
    and the copy that already reads src, so the hint cannot hoist the
    read across a lock acquisition or other synchronization (which
    could introduce a data race the original program did not have), and
    the source identifier is guaranteed to be in scope at the
    declaration;
  - the source is a plain identifier, so the injected len(src) is
    side-effect-free and cannot double-evaluate a call — src itself is
    still evaluated exactly once, inside maps.Copy;
  - maps.Copy is resolved with type information (an aliased import
    matches, a shadowed maps does not), the call is a standalone
    statement (a go/defer statement runs under different timing and is
    left alone), and copying a map into itself is skipped entirely;
  - for a map[K]V{} declaration, a comment inside the braces would be
    destroyed by rewriting them into the make call — the fix is
    withheld there and the report stays advisory.

When statements sit between the declaration and the copy (none of them
touching the destination) or the source is a field chain like s.data,
the report is advisory: the pre-size advice stands, but hoisting the
len read is a human decision.`,
		Before: `dst := make(map[string]int)
maps.Copy(dst, src)`,
		After: `dst := make(map[string]int, len(src))
maps.Copy(dst, src)`,
		MeasuredWin: `BenchmarkPS2039 (copying a 1024-entry map[int]int,
Apple M2 Pro, gc 1.26): unsized destination 55.7 µs/op, 74264 B/op,
20 allocs/op -> make(map[int]int, len(src)) 23.1 µs/op, 36944 B/op,
5 allocs/op — ~2.4x faster, half the bytes: the incremental
growth-step allocations and every growth-triggered rehash of
already-inserted keys disappear, leaving the up-front table.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2039",
		Doc:  "an unsized map populated by maps.Copy; make(map[K]V, len(src)) reserves the buckets once",
		Run:  runPS2039,
	},
})

func runPS2039(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			block, ok := n.(*ast.BlockStmt)
			if !ok {
				return true
			}
			for j, stmt := range block.List {
				dst, src := ps2039CopyStmt(pass, stmt)
				if dst == nil {
					continue
				}
				ps2039Report(pass, f, block, j, dst, src)
			}
			return true
		})
	}
	return nil, nil
}

// ps2039CopyStmt matches a standalone `maps.Copy(dst, src)` statement with
// dst a plain identifier, resolved with type information so an aliased
// import matches and a shadowed maps does not. A go/defer statement runs
// under different timing and never matches. Returns the destination
// identifier and the source expression, or nil.
func ps2039CopyStmt(pass *analysis.Pass, s ast.Stmt) (*ast.Ident, ast.Expr) {
	es, ok := s.(*ast.ExprStmt)
	if !ok {
		return nil, nil
	}
	call, ok := es.X.(*ast.CallExpr)
	if !ok || len(call.Args) != 2 || call.Ellipsis.IsValid() {
		return nil, nil
	}
	if _, ok := ps3107PkgFunc(pass, call.Fun, "maps", "Copy"); !ok {
		return nil, nil
	}
	dst, ok := ps2110Unparen(call.Args[0]).(*ast.Ident)
	if !ok {
		return nil, nil
	}
	return dst, call.Args[1]
}

// ps2039Report pairs the maps.Copy at index j with an earlier unsized
// declaration of its destination in the same block, as long as nothing
// between them touches the destination.
func ps2039Report(pass *analysis.Pass, f *ast.File, block *ast.BlockStmt, j int, dst *ast.Ident, src ast.Expr) {
	dstObj := pass.TypesInfo.Uses[dst]
	if dstObj == nil {
		return
	}
	srcText, srcOK := ps2039SrcText(src)
	if !srcOK {
		return // a call or other effectful source: len(src) advice would double-evaluate it
	}
	// Copying a map into itself is a no-op over an empty fresh map;
	// len(dst) is also not in scope on the declaration's RHS.
	if id, ok := ps2110Unparen(src).(*ast.Ident); ok && pass.TypesInfo.Uses[id] == dstObj {
		return
	}
	for i := j - 1; i >= 0; i-- {
		declIdent, makeCall, lit, ok := ps2039UnsizedDecl(block.List[i])
		if !ok || declIdent.Name != dst.Name {
			continue
		}
		if pass.TypesInfo.Defs[declIdent] != dstObj {
			return // same name, different variable: the fresh map is not this one
		}
		if stmtsMention(block.List[i+1:j], dst.Name) {
			return // something touches the map between declaration and copy
		}
		why := ""
		if j != i+1 {
			// Statements between the declaration and the copy: injecting
			// len(src) at the declaration would hoist the read across
			// them — across a lock acquisition or other synchronization
			// that could be a new data race, so no auto-fix.
			why = " (no auto-fix: statements between the declaration and the copy; hoisting the len read across them is a human decision)"
		} else if _, isIdent := ps2110Unparen(src).(*ast.Ident); !isIdent {
			why = " (no auto-fix: the size hint is only injected for a plain identifier source)"
		} else if lit != nil && ps2111CommentIn(f, lit.Type.End(), lit.End()) {
			why = " (no auto-fix: a comment inside the braces would be destroyed by the rewrite)"
		}
		diag := analysis.Diagnostic{
			Pos:     block.List[i].Pos(),
			End:     block.List[i].End(),
			Message: dst.Name + " is populated by maps.Copy from " + srcText + " but declared without a size hint; pre-size it with make(map[...]..., len(" + srcText + ")) to reserve the buckets once" + why,
		}
		if why == "" {
			hint := "len(" + srcText + ")"
			var edits []analysis.TextEdit
			if makeCall != nil {
				// make(map[K]V) -> make(map[K]V, len(src)): a pure
				// insertion after the type argument keeps every byte of
				// user code (including comments and a trailing comma).
				at := makeCall.Args[0].End()
				edits = []analysis.TextEdit{{Pos: at, End: at, NewText: []byte(", " + hint)}}
			} else {
				// map[K]V{} -> make(map[K]V, len(src)): the type spelling
				// stays byte-verbatim in place; the empty braces (proven
				// comment-free) become the hint and closing parenthesis.
				edits = []analysis.TextEdit{
					{Pos: lit.Pos(), End: lit.Pos(), NewText: []byte("make(")},
					{Pos: lit.Type.End(), End: lit.End(), NewText: []byte(", " + hint + ")")},
				}
			}
			diag.SuggestedFixes = []analysis.SuggestedFix{{
				Message:   "pre-size " + dst.Name + " to " + hint,
				TextEdits: edits,
			}}
		}
		pass.Report(diag)
		return
	}
}

// ps2039UnsizedDecl matches `m := make(map[K]V)` (no size hint) and
// `m := map[K]V{}` (empty literal), returning the declared identifier and
// exactly one of the make call or the composite literal. The map type must
// be spelled as a map type literal: `make(map[K]V)` with a type argument
// only compiles as the builtin make, so no shadowing check is needed, and
// a named map type is a different declaration shape that stays out.
func ps2039UnsizedDecl(s ast.Stmt) (*ast.Ident, *ast.CallExpr, *ast.CompositeLit, bool) {
	as, ok := s.(*ast.AssignStmt)
	if !ok || as.Tok != token.DEFINE || len(as.Lhs) != 1 || len(as.Rhs) != 1 {
		return nil, nil, nil, false
	}
	id, ok := as.Lhs[0].(*ast.Ident)
	if !ok {
		return nil, nil, nil, false
	}
	switch rhs := as.Rhs[0].(type) {
	case *ast.CompositeLit:
		if _, ok := rhs.Type.(*ast.MapType); ok && len(rhs.Elts) == 0 {
			return id, nil, rhs, true
		}
	case *ast.CallExpr:
		if fn, ok := rhs.Fun.(*ast.Ident); ok && fn.Name == "make" && len(rhs.Args) == 1 {
			if _, ok := rhs.Args[0].(*ast.MapType); ok {
				return id, rhs, nil, true
			}
		}
	}
	return nil, nil, nil, false
}

// ps2039SrcText renders the copy source for the diagnostic when it is a
// side-effect-free shape — a plain identifier or a selector chain of
// identifiers (s.data) — and reports false for anything that could carry
// side effects (a call, an index expression), where len(src) advice would
// be wrong.
func ps2039SrcText(e ast.Expr) (string, bool) {
	switch x := ps2110Unparen(e).(type) {
	case *ast.Ident:
		return x.Name, true
	case *ast.SelectorExpr:
		base, ok := ps2039SrcText(x.X)
		if !ok {
			return "", false
		}
		return base + "." + x.Sel.Name, true
	}
	return "", false
}
