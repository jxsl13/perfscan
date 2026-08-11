package checks

import (
	"fmt"
	"go/ast"
	"go/printer"
	"go/token"
	"go/types"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS2102 reports string concatenation with += (or s = s + x) inside a loop.
var PS2102 = register(&lint.Check{
	ID:       "PS2102",
	Category: "alloc",
	Slug:     "string-concat-in-loop",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "string concatenation with += inside a loop",
		Text: `Strings are immutable: s += x allocates a fresh string of
len(s)+len(x) and copies both halves, so a loop of appends does O(n²) copy
traffic. A strings.Builder amortizes the growth (and PS2002 will then ask
it to be pre-sized).

Advisory: the rewrite touches every later use of the variable (b.String()),
which is mechanical but not single-site.

The automatic fix rewrites the one provably safe shape: a declaration
s := "" as a direct block statement whose every write, function-wide, is
an 's += <string expression>' inside the single loop that directly follows
it (no intervening statement mentions s), with s otherwise only read after
that loop. The declaration becomes a strings.Builder, each += becomes a
WriteString call, and 's := b.String()' is re-bound right after the loop,
so the final string is bit-identical and later reads are untouched. Reads
of s inside the loop, writes outside it, &s, a goto in the function, a
shadowed strings package name, or comments inside a replaced statement all
keep the plain advisory report. The fix adds the "strings" import when the
file lacks it.`,
		Before: `s := ""
for _, part := range parts {
	s += part
}`,
		After: `var b strings.Builder
b.Grow(total) // if a bound is known
for _, part := range parts {
	b.WriteString(part)
}
s := b.String()`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2102",
		Doc:  "string += concatenation in a loop",
		Run:  runPS2102,
	},
})

func runPS2102(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		fixes := ps2102Fixes(pass, f)
		astutil.WithStack(f, func(n ast.Node, stack []ast.Node) bool {
			as, ok := n.(*ast.AssignStmt)
			if !ok || len(as.Lhs) != 1 || len(as.Rhs) != 1 {
				return true
			}
			id, ok := as.Lhs[0].(*ast.Ident)
			if !ok {
				return true
			}
			isConcat := false
			switch as.Tok {
			case token.ADD_ASSIGN:
				isConcat = true
			case token.ASSIGN:
				// s = s + x
				if be, ok := as.Rhs[0].(*ast.BinaryExpr); ok && be.Op == token.ADD {
					if x, ok := be.X.(*ast.Ident); ok && x.Name == id.Name {
						isConcat = true
					}
				}
			}
			if !isConcat {
				return true
			}
			t := pass.TypesInfo.TypeOf(as.Lhs[0])
			if t == nil {
				return true
			}
			b, ok := t.Underlying().(*types.Basic)
			if !ok || b.Info()&types.IsString == 0 {
				return true
			}
			if _, inLoop := astutil.InLoop(stack); !inLoop {
				return true
			}
			diag := analysis.Diagnostic{
				Pos:     as.Pos(),
				End:     as.End(),
				Message: fmt.Sprintf("string %s grows by concatenation in a loop — O(n²) copy traffic; build it with a strings.Builder", id.Name),
			}
			if fix := fixes[as]; fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*fix}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}

// ps2102Group is one eligible accumulation: a `s := ""` declaration, the
// loop directly following it in the same block that holds every write to
// s in the whole function (all of them `s += <string expr>`), and those
// writes in source order.
type ps2102Group struct {
	decl      *ast.AssignStmt
	declIdent *ast.Ident
	loop      ast.Stmt
	writes    []*ast.AssignStmt
}

// ps2102Fixes computes the strings.Builder rewrite for every eligible
// accumulation group in f, keyed by the group's first += statement: that
// statement's diagnostic carries the whole multi-edit fix, later +=
// diagnostics of the same group stay advisory, so the edits are emitted
// exactly once. When the file must gain the "strings" import, only the
// first fix of the file carries the import edit — all fixes of a run are
// applied together (same convention as PS3077's helper injection).
func ps2102Fixes(pass *analysis.Pass, f *ast.File) map[*ast.AssignStmt]*analysis.SuggestedFix {
	fixes := map[*ast.AssignStmt]*analysis.SuggestedFix{}
	importAdded := false
	usedNames := map[string]bool{}
	astutil.WithStack(f, func(n ast.Node, stack []ast.Node) bool {
		block, ok := n.(*ast.BlockStmt)
		if !ok {
			return true
		}
		for j := range block.List {
			g := ps2102GroupAt(pass, block, j, stack)
			if g == nil {
				continue
			}
			needImport, ok := ps2102StringsUsable(pass, g.decl.Pos())
			if !ok {
				continue
			}
			if needImport && ps2102ImportsC(f) {
				// Never edit the import set of a cgo file: a misplaced
				// insertion could detach the preamble from import "C".
				continue
			}
			name := "psSb" + strconv.Itoa(pass.Fset.Position(g.decl.Pos()).Line)
			if usedNames[name] || ps2102NameTaken(pass, f, name) {
				continue
			}
			if ps2102CommentsOverlap(f, g) {
				continue
			}
			edits := []analysis.TextEdit{{
				Pos:     g.decl.Pos(),
				End:     g.decl.End(),
				NewText: []byte("var " + name + " strings.Builder"),
			}}
			renderOK := true
			for _, w := range g.writes {
				var b strings.Builder
				if err := printer.Fprint(&b, token.NewFileSet(), w.Rhs[0]); err != nil {
					renderOK = false
					break
				}
				edits = append(edits, analysis.TextEdit{
					Pos:     w.Pos(),
					End:     w.End(),
					NewText: []byte(name + ".WriteString(" + b.String() + ")"),
				})
			}
			if !renderOK {
				continue
			}
			// Assume gofmt indentation (tabs): the loop starts at column
			// Column, i.e. Column-1 tabs of indentation.
			//perfscan:ignore PS2003 indent depth varies per finding; fix-construction cold path
			indent := strings.Repeat("\t", pass.Fset.Position(g.loop.Pos()).Column-1)
			edits = append(edits, analysis.TextEdit{
				Pos:     g.loop.End(),
				End:     g.loop.End(),
				NewText: []byte("\n" + indent + g.declIdent.Name + " := " + name + ".String()"),
			})
			if needImport && !importAdded {
				edits = append(edits, ps2102ImportEdit(f))
				importAdded = true
			}
			usedNames[name] = true
			fixes[g.writes[0]] = &analysis.SuggestedFix{
				Message:   "build " + g.declIdent.Name + " with a strings.Builder",
				TextEdits: edits,
			}
		}
		return true
	})
	return fixes
}

// ps2102GroupAt matches the eligible shape rooted at block.List[j]:
//
//	s := ""
//	for ... {        // first later statement in the block mentioning s
//		s += E   // every write to s in the function, E string-typed
//	}
//	... only plain reads of s, all after the loop, at least one ...
//
// Any other use of s anywhere in the enclosing function — a read before or
// inside the loop, a write outside it, an assignment form other than +=,
// &s, use as a range/incdec target — returns nil, as does a goto in the
// function (the re-bound declaration could invalidate a jump).
func ps2102GroupAt(pass *analysis.Pass, block *ast.BlockStmt, j int, stack []ast.Node) *ps2102Group {
	decl, ok := block.List[j].(*ast.AssignStmt)
	if !ok || decl.Tok != token.DEFINE || len(decl.Lhs) != 1 || len(decl.Rhs) != 1 {
		return nil
	}
	id, ok := decl.Lhs[0].(*ast.Ident)
	if !ok || id.Name == "_" {
		return nil
	}
	lit, ok := decl.Rhs[0].(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING || lit.Value != `""` {
		return nil
	}
	obj := pass.TypesInfo.Defs[id]
	if obj == nil {
		return nil
	}
	// The first later statement in the block that mentions s must be a
	// loop directly consuming the declaration.
	var loop ast.Stmt
	for _, stmt := range block.List[j+1:] {
		if !stmtsMention([]ast.Stmt{stmt}, id.Name) {
			continue
		}
		if astutil.IsLoop(stmt) {
			loop = stmt
		}
		break
	}
	if loop == nil {
		return nil
	}
	// Collect the loop's `s += E` writes with E string-typed. Function
	// literals are not descended into: a write from a closure is not
	// ordered by the loop and surfaces as a disqualifying use below.
	var writes []*ast.AssignStmt
	allowed := map[*ast.Ident]bool{id: true}
	ast.Inspect(astutil.LoopBody(loop), func(n ast.Node) bool {
		if _, isLit := n.(*ast.FuncLit); isLit {
			return false
		}
		as, ok := n.(*ast.AssignStmt)
		if !ok || as.Tok != token.ADD_ASSIGN || len(as.Lhs) != 1 || len(as.Rhs) != 1 {
			return true
		}
		lhs, ok := as.Lhs[0].(*ast.Ident)
		if !ok || pass.TypesInfo.ObjectOf(lhs) != obj {
			return true
		}
		b, ok := pass.TypesInfo.TypeOf(as.Rhs[0]).(*types.Basic)
		if !ok || b.Info()&types.IsString == 0 {
			return true
		}
		writes = append(writes, as)
		allowed[lhs] = true
		return true
	})
	if len(writes) == 0 {
		return nil
	}
	fn := ps2102EnclosingFuncBody(stack)
	if fn == nil || ps2102HasGoto(fn) {
		return nil
	}
	// Every remaining use of s in the function must be a plain read after
	// the loop — and at least one must exist, or the re-bound declaration
	// would be unused.
	readsAfter := 0
	eligible := true
	astutil.WithStack(fn, func(n ast.Node, st []ast.Node) bool {
		if !eligible {
			return false
		}
		use, ok := n.(*ast.Ident)
		if !ok || pass.TypesInfo.ObjectOf(use) != obj || allowed[use] {
			return true
		}
		if use.Pos() >= loop.End() && ps2102PureRead(use, st) {
			readsAfter++
			return true
		}
		eligible = false
		return false
	})
	if !eligible || readsAfter == 0 {
		return nil
	}
	return &ps2102Group{decl: decl, declIdent: id, loop: loop, writes: writes}
}

// ps2102EnclosingFuncBody returns the body of the innermost enclosing
// function on the stack.
func ps2102EnclosingFuncBody(stack []ast.Node) *ast.BlockStmt {
	for i := len(stack) - 1; i >= 0; i-- {
		switch fn := stack[i].(type) {
		case *ast.FuncLit:
			return fn.Body
		case *ast.FuncDecl:
			return fn.Body
		}
	}
	return nil
}

// ps2102HasGoto reports whether the function body contains a goto: the fix
// moves the declaration of s past the loop, and a goto across the new
// declaration site would no longer compile.
func ps2102HasGoto(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if br, ok := n.(*ast.BranchStmt); ok && br.Tok == token.GOTO {
			found = true
			return false
		}
		return !found
	})
	return found
}

// ps2102PureRead reports whether the identifier use is a plain read: not an
// assignment target, not &-escaped, not a range or ++/-- target.
func ps2102PureRead(id *ast.Ident, stack []ast.Node) bool {
	if len(stack) == 0 {
		return false
	}
	switch p := stack[len(stack)-1].(type) {
	case *ast.AssignStmt:
		for _, lhs := range p.Lhs {
			if lhs == ast.Expr(id) {
				return false
			}
		}
	case *ast.UnaryExpr:
		return p.Op != token.AND
	case *ast.IncDecStmt:
		return false
	case *ast.RangeStmt:
		return p.Key != ast.Expr(id) && p.Value != ast.Expr(id)
	}
	return true
}

// ps2102NameTaken reports whether name appears anywhere in f or is declared
// at package scope — the builder variable the fix introduces must not
// capture or collide with anything.
func ps2102NameTaken(pass *analysis.Pass, f *ast.File, name string) bool {
	if pass.Pkg.Scope().Lookup(name) != nil {
		return true
	}
	found := false
	ast.Inspect(f, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && id.Name == name {
			found = true
			return false
		}
		return !found
	})
	return found
}

// ps2102CommentsOverlap reports whether any comment overlaps a replaced
// range (the declaration or a += statement) — the rewrite would not
// reproduce it.
func ps2102CommentsOverlap(f *ast.File, g *ps2102Group) bool {
	ranges := [][2]token.Pos{{g.decl.Pos(), g.decl.End()}}
	for _, w := range g.writes {
		ranges = append(ranges, [2]token.Pos{w.Pos(), w.End()})
	}
	for _, cg := range f.Comments {
		for _, r := range ranges {
			if cg.End() > r[0] && cg.Pos() < r[1] {
				return true
			}
		}
	}
	return false
}

// ps2102StringsUsable reports whether the name "strings" can reference the
// strings package at pos: needImport is true when the file must add the
// import, ok is false when some other object owns the name there (a local
// or package-level declaration, or a same-named non-stdlib package).
func ps2102StringsUsable(pass *analysis.Pass, pos token.Pos) (needImport, ok bool) {
	scope := pass.Pkg.Scope().Innermost(pos)
	if scope == nil {
		return false, false
	}
	if _, obj := scope.LookupParent("strings", pos); obj != nil {
		pn, isPkg := obj.(*types.PkgName)
		return false, isPkg && pn.Imported().Path() == "strings"
	}
	return true, true
}

// ps2102ImportsC reports whether f is a cgo file.
func ps2102ImportsC(f *ast.File) bool {
	for _, imp := range f.Imports {
		if imp.Path.Value == `"C"` {
			return true
		}
	}
	return false
}

// ps2102ImportEdit returns the edit adding `import "strings"` to f: into
// the first import group at its sorted position, above a single import
// declaration, or after the package clause when the file has no imports.
func ps2102ImportEdit(f *ast.File) analysis.TextEdit {
	var group *ast.GenDecl
	for _, d := range f.Decls {
		if gd, ok := d.(*ast.GenDecl); ok && gd.Tok == token.IMPORT {
			group = gd
			break
		}
	}
	if group == nil {
		pos := f.Name.End()
		return analysis.TextEdit{Pos: pos, End: pos, NewText: []byte("\n\nimport \"strings\"")}
	}
	if !group.Lparen.IsValid() {
		pos := group.Pos()
		return analysis.TextEdit{Pos: pos, End: pos, NewText: []byte("import \"strings\"\n")}
	}
	for _, spec := range group.Specs {
		is, ok := spec.(*ast.ImportSpec)
		if !ok || is.Path == nil || is.Path.Value <= `"strings"` {
			continue
		}
		pos := spec.Pos()
		return analysis.TextEdit{Pos: pos, End: pos, NewText: []byte("\"strings\"\n\t")}
	}
	if n := len(group.Specs); n > 0 {
		pos := group.Specs[n-1].End()
		return analysis.TextEdit{Pos: pos, End: pos, NewText: []byte("\n\t\"strings\"")}
	}
	pos := group.Lparen + 1
	return analysis.TextEdit{Pos: pos, End: pos, NewText: []byte("\n\t\"strings\"")}
}
