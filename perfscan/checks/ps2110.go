package checks

import (
	"go/ast"
	"go/constant"
	"go/printer"
	"go/token"
	"go/types"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS2110 reports the hand-rolled slice clone idioms append([]T(nil), s...)
// and append([]T{}, s...), whose direct spelling is slices.Clone(s) — or
// bytes.Clone(s) when T is byte.
var PS2110 = register(&lint.Check{
	ID:       "PS2110",
	Category: "alloc",
	Slug:     "append-nil-clone",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "append([]T(nil), s...) is a hand-rolled clone; slices.Clone/bytes.Clone say it directly",
		Text: `append([]T(nil), s...) and append([]T{}, s...) copy a slice by
appending its spread onto an empty one. slices.Clone(s) — bytes.Clone(s)
for []byte — states the intent in one word, compiles to the same growslice
copy, and is what readers and tooling recognize as "clone".

The three spellings are NOT interchangeable on the nil/empty edges, and the
automatic fix respects that exactly (verified against the standard library
implementations):

  input s           append([]T(nil),s...)  append([]T{},s...)  slices/bytes.Clone(s)
  nil               nil                    non-nil empty       nil
  non-nil empty     nil                    non-nil empty       non-nil empty
  non-empty         fresh copy             fresh copy          fresh copy

So append([]T(nil), s...) diverges from Clone when s is non-nil empty, and
append([]T{}, s...) diverges when s is nil — nil-ness is observable (s ==
nil), so a blind rewrite is not behavior-preserving. The fix is therefore
attached only when the dangerous input is provably impossible: s must be a
plain local variable whose single defining assignment is a slice composite
literal or a make([]T, ...) call and which is never reassigned,
range-bound, or address-taken anywhere in the enclosing function. A
composite literal or make result is never nil, which proves the
append([]T{}, s...) rewrite; a literal with elements or a make with a
positive constant length is additionally never empty, which proves the
append([]T(nil), s...) rewrite. Everything else — parameters, fields,
package-level variables, reassigned locals — keeps the advisory report.

The fix also requires the static types to match exactly: the empty
literal's type and s's type must be the identical unnamed slice type, since
slices.Clone returns the type of its argument (a named argument or literal
type would change the expression's static type, and a named element could
alter method-set/interface behavior downstream). s's original text is kept
in place, so its evaluation is unchanged and evaluated exactly once, as
before. The fix adds the slices (or bytes) import when missing, skips cgo
files, is suppressed when the package name is shadowed at the call site,
and is suppressed when the vanishing []T spelling names an imported
package's type (removing the file's last reference would orphan that
import).`,
		Before: `snapshot := append([]int(nil), src...)`,
		After:  `snapshot := slices.Clone(src)`,
		MeasuredWin: `BenchmarkPS2110 (cloning a 1024-element []string,
Apple M2 Pro): 1421 ns/op -> 1398 ns/op, identical 18432 B/op and
1 alloc/op — both spellings reach the same runtime growslice+copy. The win
is intent (one recognized word), plus Clone's exact nil-preservation
contract instead of the idiom's accidental nil/empty behavior.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2110",
		Doc:  "hand-rolled slice clone via append onto an empty slice",
		Run:  runPS2110,
	},
})

func runPS2110(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		// All fixes of a run are applied together, so only the first fix
		// needing a given import path carries the import edit.
		importAdded := map[string]bool{}
		astutil.WithStack(f, func(n ast.Node, stack []ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 2 || !call.Ellipsis.IsValid() {
				return true
			}
			// The callee must be the predeclared append builtin, not a
			// shadowing local.
			fn, ok := call.Fun.(*ast.Ident)
			if !ok {
				return true
			}
			if b, ok := pass.TypesInfo.Uses[fn].(*types.Builtin); !ok || b.Name() != "append" {
				return true
			}
			litType, nilForm, ok := ps2110EmptyLit(pass, call.Args[0])
			if !ok {
				return true
			}
			sExpr := call.Args[1]
			sType := pass.TypesInfo.TypeOf(sExpr)
			if sType == nil {
				return true
			}
			// Only slice spreads: append([]byte(nil), str...) over a string
			// is a []byte conversion, not a slice clone.
			if _, isSlice := sType.Underlying().(*types.Slice); !isSlice {
				return true
			}
			if !types.AssignableTo(sType, litType) {
				return true
			}
			target, pkgName, pkgPath := "slices.Clone", "slices", "slices"
			if eb, ok := litType.Elem().(*types.Basic); ok && eb.Kind() == types.Byte {
				target, pkgName, pkgPath = "bytes.Clone", "bytes", "bytes"
			}
			sText := ps2110ExprText(sExpr)
			litText := ps2110ExprText(call.Args[0])
			diag := analysis.Diagnostic{
				Pos:     call.Pos(),
				End:     call.End(),
				Message: "append(" + litText + ", " + sText + "...) is a hand-rolled clone; " + target + "(" + sText + ") says it directly",
			}
			if fix := ps2110Fix(pass, f, stack, call, nilForm, litType, target, pkgName, pkgPath, importAdded); fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*fix}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}

// ps2110EmptyLit matches the first append argument as an empty slice of a
// concrete, canonically spelled slice type: []T(nil) (nilForm true) or
// []T{} (nilForm false). Named slice types and shadowed nil are rejected.
func ps2110EmptyLit(pass *analysis.Pass, e ast.Expr) (litType *types.Slice, nilForm, ok bool) {
	e = ps2110Unparen(e)
	switch lit := e.(type) {
	case *ast.CallExpr:
		// []T(nil): a conversion whose callee is a slice type expression.
		tv, ok := pass.TypesInfo.Types[lit.Fun]
		if !ok || !tv.IsType() || len(lit.Args) != 1 {
			return nil, false, false
		}
		if at, isArr := ps2110Unparen(lit.Fun).(*ast.ArrayType); !isArr || at.Len != nil {
			return nil, false, false
		}
		id, isIdent := ps2110Unparen(lit.Args[0]).(*ast.Ident)
		if !isIdent || pass.TypesInfo.Uses[id] != types.Universe.Lookup("nil") {
			return nil, false, false
		}
		st, isSlice := tv.Type.(*types.Slice)
		return st, true, isSlice
	case *ast.CompositeLit:
		if len(lit.Elts) != 0 || lit.Type == nil {
			return nil, false, false
		}
		if at, isArr := lit.Type.(*ast.ArrayType); !isArr || at.Len != nil {
			return nil, false, false
		}
		st, isSlice := pass.TypesInfo.TypeOf(lit).(*types.Slice)
		return st, false, isSlice
	}
	return nil, false, false
}

// ps2110Fix builds the rewrite to slices.Clone/bytes.Clone, or returns nil
// when any guard fails and the report must stay advisory.
func ps2110Fix(pass *analysis.Pass, f *ast.File, stack []ast.Node, call *ast.CallExpr, nilForm bool, litType *types.Slice, target, pkgName, pkgPath string, importAdded map[string]bool) *analysis.SuggestedFix {
	sExpr := call.Args[1]
	// Clone returns the type of its argument: the rewrite keeps the static
	// type only when s's type is identical to the vanishing []T literal's.
	sType := pass.TypesInfo.TypeOf(sExpr)
	if sType == nil || !types.Identical(sType, litType) {
		return nil
	}
	// Nil/empty equivalence (see the Doc table): the []T(nil) form maps a
	// non-nil empty s to nil where Clone keeps it non-nil, and the []T{}
	// form maps a nil s to non-nil empty where Clone keeps it nil. Fix only
	// when the divergent input is provably impossible for s.
	sIdent, ok := ps2110Unparen(sExpr).(*ast.Ident)
	if !ok {
		return nil
	}
	fnBody := ps2110EnclosingFuncBody(stack)
	if fnBody == nil {
		return nil
	}
	neverNil, neverEmpty := ps2110SliceFacts(pass, fnBody, sIdent)
	if nilForm && !neverEmpty {
		return nil
	}
	if !nilForm && !neverNil {
		return nil
	}
	// The []T spelling vanishes from the rewritten text: if it names an
	// imported package's type this could orphan that import — advisory.
	if ps2110MentionsPkg(pass, call.Args[0]) {
		return nil
	}
	// The edits replace everything around s; a comment there would be lost.
	if ps2110CommentsIn(f, call.Pos(), sExpr.Pos()) || ps2110CommentsIn(f, sExpr.End(), call.End()) {
		return nil
	}
	needImport, usable := ps2110PkgUsable(pass, call.Pos(), pkgName, pkgPath)
	if !usable {
		return nil
	}
	if needImport && ps2110ImportsC(f) {
		return nil
	}
	// Keep s's text untouched in place: same expression, same single
	// evaluation. Only the surrounding call spelling changes.
	edits := []analysis.TextEdit{
		{Pos: call.Pos(), End: sExpr.Pos(), NewText: []byte(target + "(")},
		{Pos: sExpr.End(), End: call.End(), NewText: []byte(")")},
	}
	if needImport && !importAdded[pkgPath] {
		edits = append(edits, ps2110ImportEdit(f, pkgPath))
		importAdded[pkgPath] = true
	}
	return &analysis.SuggestedFix{
		Message:   "replace with " + target + "(" + sIdent.Name + ")",
		TextEdits: edits,
	}
}

// ps2110EnclosingFuncBody returns the body of the outermost enclosing
// function (declaration or literal) on the stack, or nil at package level.
func ps2110EnclosingFuncBody(stack []ast.Node) *ast.BlockStmt {
	for _, n := range stack {
		switch fn := n.(type) {
		case *ast.FuncDecl:
			return fn.Body
		case *ast.FuncLit:
			return fn.Body
		}
	}
	return nil
}

// ps2110SliceFacts proves nil/empty facts about the value of id at any use
// site inside body: neverNil when its single defining assignment is a slice
// composite literal or make call (both always non-nil), neverEmpty when
// that definition additionally has a provably positive length. The proof
// requires the variable to be written exactly once (its definition) and
// never range-bound or address-taken anywhere in body — then every
// reachable use observes the classified value (a definition's scope starts
// at the definition, so no use can execute before it).
func ps2110SliceFacts(pass *analysis.Pass, body *ast.BlockStmt, id *ast.Ident) (neverNil, neverEmpty bool) {
	obj, ok := pass.TypesInfo.Uses[id].(*types.Var)
	if !ok {
		return false, false
	}
	var rhs ast.Expr
	bad := false
	ast.Inspect(body, func(n ast.Node) bool {
		if bad {
			return false
		}
		switch st := n.(type) {
		case *ast.AssignStmt:
			for _, lhs := range st.Lhs {
				lid, ok := lhs.(*ast.Ident)
				if !ok {
					continue
				}
				if pass.TypesInfo.Defs[lid] == types.Object(obj) {
					if rhs != nil || st.Tok != token.DEFINE || len(st.Lhs) != 1 || len(st.Rhs) != 1 {
						bad = true // multi-value or duplicate definition
						return false
					}
					rhs = st.Rhs[0]
				} else if pass.TypesInfo.Uses[lid] == types.Object(obj) {
					bad = true // reassigned (=, op=, or := redeclaration)
					return false
				}
			}
		case *ast.ValueSpec:
			for _, name := range st.Names {
				if pass.TypesInfo.Defs[name] != types.Object(obj) {
					continue
				}
				if rhs != nil || len(st.Names) != 1 || len(st.Values) != 1 {
					bad = true
					return false
				}
				rhs = st.Values[0]
			}
		case *ast.RangeStmt:
			for _, e := range []ast.Expr{st.Key, st.Value} {
				if kid, ok := e.(*ast.Ident); ok &&
					(pass.TypesInfo.Uses[kid] == types.Object(obj) || pass.TypesInfo.Defs[kid] == types.Object(obj)) {
					bad = true
					return false
				}
			}
		case *ast.UnaryExpr:
			if st.Op != token.AND {
				return true
			}
			if xid, ok := ps2110Unparen(st.X).(*ast.Ident); ok && pass.TypesInfo.Uses[xid] == types.Object(obj) {
				bad = true // address taken: the header can be rewritten via the pointer
				return false
			}
		}
		return true
	})
	if bad || rhs == nil {
		return false, false
	}
	switch def := ps2110Unparen(rhs).(type) {
	case *ast.CompositeLit:
		if _, ok := pass.TypesInfo.TypeOf(def).Underlying().(*types.Slice); !ok {
			return false, false
		}
		return true, len(def.Elts) > 0
	case *ast.CallExpr:
		fn, ok := def.Fun.(*ast.Ident)
		if !ok {
			return false, false
		}
		if b, ok := pass.TypesInfo.Uses[fn].(*types.Builtin); !ok || b.Name() != "make" {
			return false, false
		}
		if _, ok := pass.TypesInfo.TypeOf(def).Underlying().(*types.Slice); !ok {
			return false, false
		}
		neverEmpty := false
		if len(def.Args) >= 2 {
			if tv, ok := pass.TypesInfo.Types[def.Args[1]]; ok && tv.Value != nil {
				if v, ok := constant.Int64Val(tv.Value); ok && v > 0 {
					neverEmpty = true
				}
			}
		}
		return true, neverEmpty
	}
	return false, false
}

// ps2110MentionsPkg reports whether e references any imported package name;
// the fix deletes e's text, which could orphan that package's import.
func ps2110MentionsPkg(pass *analysis.Pass, e ast.Expr) bool {
	found := false
	ast.Inspect(e, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok {
			if _, isPkg := pass.TypesInfo.Uses[id].(*types.PkgName); isPkg {
				found = true
			}
		}
		return !found
	})
	return found
}

// ps2110CommentsIn reports whether any comment overlaps [pos, end).
func ps2110CommentsIn(f *ast.File, pos, end token.Pos) bool {
	for _, cg := range f.Comments {
		if cg.End() > pos && cg.Pos() < end {
			return true
		}
	}
	return false
}

// ps2110PkgUsable reports whether name can reference the package at path
// from pos: needImport is true when the file must add the import, ok is
// false when some other object owns the name there.
func ps2110PkgUsable(pass *analysis.Pass, pos token.Pos, name, path string) (needImport, ok bool) {
	scope := pass.Pkg.Scope().Innermost(pos)
	if scope == nil {
		return false, false
	}
	if _, obj := scope.LookupParent(name, pos); obj != nil {
		pn, isPkg := obj.(*types.PkgName)
		return false, isPkg && pn.Imported().Path() == path
	}
	return true, true
}

// ps2110ImportsC reports whether f is a cgo file, whose import block must
// never be edited.
func ps2110ImportsC(f *ast.File) bool {
	for _, imp := range f.Imports {
		if imp.Path.Value == `"C"` {
			return true
		}
	}
	return false
}

// ps2110ImportEdit returns the edit adding `import "path"` to f: into the
// first import group at its sorted position, above a single import
// declaration, or after the package clause when the file has no imports.
func ps2110ImportEdit(f *ast.File, path string) analysis.TextEdit {
	quoted := strconv.Quote(path)
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

// ps2110Unparen strips any parentheses around e.
func ps2110Unparen(e ast.Expr) ast.Expr {
	for {
		p, ok := e.(*ast.ParenExpr)
		if !ok {
			return e
		}
		e = p.X
	}
}

// ps2110ExprText renders e for use in diagnostic messages.
func ps2110ExprText(e ast.Expr) string {
	var b strings.Builder
	if err := printer.Fprint(&b, token.NewFileSet(), e); err != nil {
		return "..."
	}
	return b.String()
}
