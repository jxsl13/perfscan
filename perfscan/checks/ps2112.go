package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS2112 reports a clipped append chain — two or more spread appends onto
// a provably fresh, empty base slice — which is a hand-rolled slice
// concatenation with one growth step per append; slices.Concat sizes and
// allocates the result once.
var PS2112 = register(&lint.Check{
	ID:       "PS2112",
	Category: "alloc",
	Slug:     "clipped-append-concat",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "append(append([]T(nil), a...), b...) is a hand-rolled concat; slices.Concat says it",
		Text: `Chaining spread appends onto an empty base to join slices —
append(append([]T(nil), a...), b...) — allocates for the first spread and
then grows (often reallocating and re-copying) once per further spread,
because no append knows the final length. slices.Concat (Go 1.21+) sums
the operand lengths first and allocates the result exactly once, and it
names the intent.

Only a chain whose base is PROVABLY fresh and empty is reported —
[]T(nil), []T{}, or make([]T, 0) with no capacity argument. With such a
base the chain cannot reuse an existing backing array, so slices.Concat
(which always allocates fresh) yields the same contents with the same
aliasing: a fresh array, sharing nothing with the operands. A chain onto
a non-empty slice variable is deliberately NOT reported: there append may
extend the variable's backing array in place — the normal, efficient way
to extend a slice — and Concat would change that. make([]T, 0, cap) is
likewise left alone: the explicit capacity is there to be reused by
later appends.

The automatic fix rewrites only the []T(nil) base: when every operand is
empty at runtime that chain returns nil, exactly as slices.Concat does.
A []T{} or make([]T, 0) base returns a NON-nil empty slice in that case,
where Concat returns nil — nil-ness is observable, so those bases keep
an advisory report without a fix. Every operand must be a spread whose
type is identical to the base's slice type (a named slice or a string
spread into []byte would change Concat's type inference or not compile),
each mixing single-element append breaks the chain, and the operand
expressions are left untouched in place (same evaluation order; the
dropped base is a side-effect-free literal). The fix adds the slices
import when the file lacks it and is suppressed when the name slices is
shadowed at the call site, when the dropped base text references an
imported package (removing its last use would orphan the import), when a
comment sits inside the rewritten punctuation, or in cgo files.`,
		Before: `merged := append(append([]string(nil), defaults...), overrides...)`,
		After:  `merged := slices.Concat(defaults, overrides)`,
		MeasuredWin: `BenchmarkPS2112 (two 512-element []string halves, Apple
M2 Pro): 2.5 µs/op, 2 allocs -> 1.5 µs/op, 1 alloc (~1.7x, half the
allocations): the chain's second spread reallocates and re-copies the
first half, Concat sizes the result once.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2112",
		Doc:  "chained spread appends onto an empty base instead of slices.Concat",
		Run:  runPS2112,
	},
})

func runPS2112(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		// consumed marks the inner append calls of a reported chain so the
		// preorder walk does not report the sub-chain again.
		consumed := map[*ast.CallExpr]bool{}
		// All fixes of a run are applied together, so only the first fix
		// needing the slices import carries the import edit (same
		// convention as PS2107's injection).
		importAdded := false
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || consumed[call] {
				return true
			}
			operands, base, inner := ps2112Chain(pass, call)
			if len(operands) < 2 {
				return true
			}
			kind := ps2112EmptyBase(pass, base)
			if kind == ps2112NotEmpty {
				return true
			}
			bt := pass.TypesInfo.TypeOf(base)
			if bt == nil {
				return true
			}
			if _, isSlice := bt.(*types.Slice); !isSlice {
				return true
			}
			// slices.Concat requires one common slice type S: every spread
			// operand must have exactly the base's (unnamed) slice type. A
			// named slice would change Concat's inferred result type; a
			// string spread into []byte would not compile.
			for _, op := range operands {
				ot := pass.TypesInfo.TypeOf(op)
				if ot == nil || !types.Identical(ot, bt) {
					return true
				}
			}
			for _, c := range inner {
				consumed[c] = true
			}
			diag := analysis.Diagnostic{
				Pos:     call.Pos(),
				End:     call.End(),
				Message: "chained spread appends onto an empty base hand-roll a concatenation, growing the result per append; slices.Concat sizes and allocates it once",
			}
			if fix := ps2112Fix(pass, f, call, operands, base, kind, &importAdded); fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*fix}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}

// ps2112Chain peels call as a chain of spread appends:
// append(append(BASE, a...), b...) yields operands [a b] (evaluation and
// source order), the base expression, and the peeled inner append calls.
// A single spread append yields one operand and is not a chain.
func ps2112Chain(pass *analysis.Pass, call *ast.CallExpr) (operands []ast.Expr, base ast.Expr, inner []*ast.CallExpr) {
	if !ps2112SpreadAppend(pass, call) {
		return nil, nil, nil
	}
	cur := call
	for {
		operands = append(operands, cur.Args[1])
		next, ok := cur.Args[0].(*ast.CallExpr)
		if !ok || !ps2112SpreadAppend(pass, next) {
			break
		}
		inner = append(inner, next)
		cur = next
	}
	// operands were collected outermost-first; reverse into source order.
	for i, j := 0, len(operands)-1; i < j; i, j = i+1, j-1 {
		operands[i], operands[j] = operands[j], operands[i]
	}
	return operands, cur.Args[0], inner
}

// ps2112SpreadAppend reports whether call is the builtin append applied to
// exactly one spread operand: append(x, y...). A shadowed append does not
// resolve to *types.Builtin and is rejected.
func ps2112SpreadAppend(pass *analysis.Pass, call *ast.CallExpr) bool {
	if len(call.Args) != 2 || !call.Ellipsis.IsValid() {
		return false
	}
	id, ok := call.Fun.(*ast.Ident)
	if !ok {
		return false
	}
	b, ok := pass.TypesInfo.Uses[id].(*types.Builtin)
	return ok && b.Name() == "append"
}

// ps2112BaseKind classifies the chain's base expression.
type ps2112BaseKind int

const (
	// ps2112NotEmpty: not provably a fresh empty slice — the chain may
	// legitimately extend an existing backing array in place; silent.
	ps2112NotEmpty ps2112BaseKind = iota
	// ps2112NilConv: []T(nil) — empty AND nil, so the chain's result is
	// nil exactly when slices.Concat's is; fixable.
	ps2112NilConv
	// ps2112EmptyNonNil: []T{} or make([]T, 0) — empty but non-nil: with
	// all-empty operands the chain returns a non-nil empty slice where
	// Concat returns nil; advisory only.
	ps2112EmptyNonNil
)

// ps2112EmptyBase classifies e as one of the recognized fresh empty base
// forms. make([]T, 0, cap) is deliberately NotEmpty: its capacity exists
// to be reused by later appends.
func ps2112EmptyBase(pass *analysis.Pass, e ast.Expr) ps2112BaseKind {
	switch x := e.(type) {
	case *ast.CompositeLit:
		if at, ok := x.Type.(*ast.ArrayType); ok && at.Len == nil && len(x.Elts) == 0 {
			return ps2112EmptyNonNil
		}
	case *ast.CallExpr:
		fun := x.Fun
		for {
			p, ok := fun.(*ast.ParenExpr)
			if !ok {
				break
			}
			fun = p.X
		}
		if at, ok := fun.(*ast.ArrayType); ok && at.Len == nil {
			// []T(nil) / ([]T)(nil) conversion.
			if len(x.Args) == 1 && !x.Ellipsis.IsValid() {
				if tv, ok := pass.TypesInfo.Types[x.Args[0]]; ok && tv.IsNil() {
					return ps2112NilConv
				}
			}
			return ps2112NotEmpty
		}
		if id, ok := fun.(*ast.Ident); ok && len(x.Args) == 2 {
			if b, ok := pass.TypesInfo.Uses[id].(*types.Builtin); ok && b.Name() == "make" {
				if at, ok := x.Args[0].(*ast.ArrayType); ok && at.Len == nil && ps2112IsZero(pass, x.Args[1]) {
					return ps2112EmptyNonNil
				}
			}
		}
	}
	return ps2112NotEmpty
}

// ps2112IsZero reports whether e is the integer constant 0.
func ps2112IsZero(pass *analysis.Pass, e ast.Expr) bool {
	tv, ok := pass.TypesInfo.Types[e]
	return ok && tv.Value != nil && tv.Value.Kind() == constant.Int && constant.Sign(tv.Value) == 0
}

// ps2112Fix builds the slices.Concat rewrite, or nil when any safety
// precondition fails and the report must stay advisory. Only the
// punctuation around the operands is replaced; the operand expressions
// stay untouched in place, preserving text and evaluation order (the
// dropped base is a side-effect-free literal evaluated before them).
func ps2112Fix(pass *analysis.Pass, f *ast.File, call *ast.CallExpr, operands []ast.Expr, base ast.Expr, kind ps2112BaseKind, importAdded *bool) *analysis.SuggestedFix {
	if kind != ps2112NilConv {
		// []T{} / make([]T, 0) yield a non-nil empty result when every
		// operand is empty; slices.Concat returns nil then. Nil-ness is
		// observable — advisory only.
		return nil
	}
	useName, needImport, usable := ps2112PkgUsable(pass, f, call.Pos(), "slices", "slices")
	if !usable {
		return nil
	}
	if needImport && ps2112ImportsC(f) {
		return nil
	}
	if ps2112MentionsPkg(pass, base) {
		// Dropping the base could remove the file's last reference to an
		// imported package and orphan the import.
		return nil
	}
	type gap struct {
		pos, end token.Pos
		text     string
	}
	gaps := []gap{{call.Pos(), operands[0].Pos(), useName + ".Concat("}}
	for i := 0; i+1 < len(operands); i++ {
		gaps = append(gaps, gap{operands[i].End(), operands[i+1].Pos(), ", "})
	}
	gaps = append(gaps, gap{operands[len(operands)-1].End(), call.End(), ")"})
	for _, g := range gaps {
		if ps2112CommentsOverlap(f, g.pos, g.end) {
			return nil
		}
	}
	edits := make([]analysis.TextEdit, 0, len(gaps)+1)
	for _, g := range gaps {
		edits = append(edits, analysis.TextEdit{Pos: g.pos, End: g.end, NewText: []byte(g.text)})
	}
	if needImport && !*importAdded {
		edits = append(edits, ps2112ImportEdit(f))
		*importAdded = true
	}
	return &analysis.SuggestedFix{
		Message:   "replace the clipped append chain with slices.Concat",
		TextEdits: edits,
	}
}

// ps2112MentionsPkg reports whether e references any imported package
// name.
func ps2112MentionsPkg(pass *analysis.Pass, e ast.Expr) bool {
	found := false
	ast.Inspect(e, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok {
			if _, isPkg := pass.TypesInfo.Uses[id].(*types.PkgName); isPkg {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

// ps2112PkgUsable reports how the package at path should be referenced from
// pos, returning the identifier to qualify the rewrite with: an existing
// import's name (its alias when aliased) so the fix reuses it instead of adding
// a SECOND import of the same path (the aliased-import duplicate PS2107 also
// guards against). needImport is true only when the file must add the import; ok
// is false when the name is unusable (a local/other-package shadow, or a
// blank/dot import that gives no usable qualifier).
func ps2112PkgUsable(pass *analysis.Pass, f *ast.File, pos token.Pos, name, path string) (useName string, needImport, ok bool) {
	quoted := `"` + path + `"`
	for _, imp := range f.Imports {
		if imp.Path.Value != quoted {
			continue
		}
		local := name
		if imp.Name != nil {
			local = imp.Name.Name
		}
		if local == "_" || local == "." {
			return name, false, false
		}
		return local, false, ps2112NameIsPkg(pass, pos, local, path)
	}
	scope := pass.Pkg.Scope().Innermost(pos)
	if scope == nil {
		return name, false, false
	}
	if _, obj := scope.LookupParent(name, pos); obj != nil {
		pn, isPkg := obj.(*types.PkgName)
		return name, false, isPkg && pn.Imported().Path() == path
	}
	return name, true, true
}

// ps2112NameIsPkg reports whether name resolves to the package at path at pos
// (not shadowed there by a local or other declaration).
func ps2112NameIsPkg(pass *analysis.Pass, pos token.Pos, name, path string) bool {
	scope := pass.Pkg.Scope().Innermost(pos)
	if scope == nil {
		return false
	}
	_, obj := scope.LookupParent(name, pos)
	pn, isPkg := obj.(*types.PkgName)
	return isPkg && pn.Imported() != nil && pn.Imported().Path() == path
}

// ps2112ImportsC reports whether f is a cgo file: its import block must
// never be edited, a misplaced insertion could detach the preamble from
// import "C".
func ps2112ImportsC(f *ast.File) bool {
	for _, imp := range f.Imports {
		if imp.Path.Value == `"C"` {
			return true
		}
	}
	return false
}

// ps2112CommentsOverlap reports whether a comment overlaps the half-open
// source range [pos, end) that a fix would replace.
func ps2112CommentsOverlap(f *ast.File, pos, end token.Pos) bool {
	for _, cg := range f.Comments {
		if cg.End() > pos && cg.Pos() < end {
			return true
		}
	}
	return false
}

// ps2112ImportEdit returns the edit adding `import "slices"` to f: into
// the first import group at its sorted position, above a single import
// declaration, or after the package clause when the file has no imports.
func ps2112ImportEdit(f *ast.File) analysis.TextEdit {
	const quoted = `"slices"`
	var group *ast.GenDecl
	for _, d := range f.Decls {
		if gd, ok := d.(*ast.GenDecl); ok && gd.Tok == token.IMPORT {
			group = gd
			break
		}
	}
	if group == nil {
		pos := f.Name.End()
		return analysis.TextEdit{Pos: pos, End: pos, NewText: []byte("\n\nimport " + quoted)}
	}
	if !group.Lparen.IsValid() {
		pos := group.Pos()
		return analysis.TextEdit{Pos: pos, End: pos, NewText: []byte("import " + quoted + "\n")}
	}
	for _, spec := range group.Specs {
		is, ok := spec.(*ast.ImportSpec)
		if !ok || is.Path == nil || is.Path.Value <= quoted {
			continue
		}
		pos := spec.Pos()
		return analysis.TextEdit{Pos: pos, End: pos, NewText: []byte(quoted + "\n\t")}
	}
	if n := len(group.Specs); n > 0 {
		pos := group.Specs[n-1].End()
		return analysis.TextEdit{Pos: pos, End: pos, NewText: []byte("\n\t" + quoted)}
	}
	pos := group.Lparen + 1
	return analysis.TextEdit{Pos: pos, End: pos, NewText: []byte("\n\t" + quoted)}
}
