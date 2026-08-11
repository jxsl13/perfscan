package checks

import (
	"fmt"
	"go/ast"
	"go/printer"
	"go/token"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS2101 reports a slice declared with no capacity immediately before a
// range loop that appends to it. When the loop ranges over a plain
// identifier, the fix pre-sizes the slice with make(T, 0, len(src)).
//
// PS2101 is a perfscan-original check (the x1xx block per category is
// reserved for checks that did not originate in the goai reference
// registry).
var PS2101 = register(&lint.Check{
	ID:       "PS2101",
	Category: "alloc",
	Slug:     "append-without-prealloc",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "a slice built by append in a range loop directly after an unsized declaration",
		Text: `Appending into a slice declared without capacity grows the
backing array geometrically — each growth allocates and copies everything
appended so far. When the append target is declared immediately before a
range loop, the range source bounds the element count, so
make([]T, 0, len(src)) removes every growth copy.

The automatic fix rewrites the declaration when the loop ranges over a plain
identifier that is not reassigned inside the loop. Filtered appends
over-allocate at most len(src) elements, which is the same worst case append
itself reserves. Capacity is not observable through append semantics, so the
rewrite is bit-identical for the built slice's contents.`,
		Before: `out := []string{}
for _, s := range src {
	out = append(out, s)
}`,
		After: `out := make([]string, 0, len(src))
for _, s := range src {
	out = append(out, s)
}`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2101",
		Doc:  "append into unsized slice declared directly before the loop",
		Run:  runPS2101,
	},
})

func runPS2101(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			block, ok := n.(*ast.BlockStmt)
			if !ok {
				return true
			}
			for i := 0; i+1 < len(block.List); i++ {
				name, typ, ok := unsizedSliceDecl(block.List[i])
				if !ok {
					continue
				}
				loop, ok := block.List[i+1].(*ast.RangeStmt)
				if !ok {
					continue
				}
				if !loopAppendsTo(loop.Body, name) {
					continue
				}
				reportPrealloc(pass, block.List[i], name, typ, loop)
			}
			return true
		})
	}
	return nil, nil
}

// unsizedSliceDecl matches `s := []T{}`, `s := make([]T, 0)` and
// `var s []T`, returning the variable name and the slice type expression.
func unsizedSliceDecl(s ast.Stmt) (string, ast.Expr, bool) {
	switch st := s.(type) {
	case *ast.AssignStmt:
		if st.Tok != token.DEFINE || len(st.Lhs) != 1 || len(st.Rhs) != 1 {
			return "", nil, false
		}
		id, ok := st.Lhs[0].(*ast.Ident)
		if !ok {
			return "", nil, false
		}
		switch rhs := st.Rhs[0].(type) {
		case *ast.CompositeLit:
			if at, ok := rhs.Type.(*ast.ArrayType); ok && at.Len == nil && len(rhs.Elts) == 0 {
				return id.Name, rhs.Type, true
			}
		case *ast.CallExpr:
			if fn, ok := rhs.Fun.(*ast.Ident); ok && fn.Name == "make" && len(rhs.Args) == 2 {
				if at, ok := rhs.Args[0].(*ast.ArrayType); ok && at.Len == nil {
					if lit, ok := rhs.Args[1].(*ast.BasicLit); ok && lit.Value == "0" {
						return id.Name, rhs.Args[0], true
					}
				}
			}
		}
	case *ast.DeclStmt:
		gd, ok := st.Decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.VAR || len(gd.Specs) != 1 {
			return "", nil, false
		}
		vs, ok := gd.Specs[0].(*ast.ValueSpec)
		if !ok || len(vs.Names) != 1 || len(vs.Values) != 0 {
			return "", nil, false
		}
		if at, ok := vs.Type.(*ast.ArrayType); ok && at.Len == nil {
			return vs.Names[0].Name, vs.Type, true
		}
	}
	return "", nil, false
}

// loopAppendsTo reports whether body contains `name = append(name, ...)`.
func loopAppendsTo(body *ast.BlockStmt, name string) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		as, ok := n.(*ast.AssignStmt)
		if !ok || len(as.Lhs) != 1 || len(as.Rhs) != 1 {
			return true
		}
		lhs, ok := as.Lhs[0].(*ast.Ident)
		if !ok || lhs.Name != name {
			return true
		}
		call, ok := as.Rhs[0].(*ast.CallExpr)
		if !ok {
			return true
		}
		if fn, ok := call.Fun.(*ast.Ident); ok && fn.Name == "append" && len(call.Args) > 0 {
			if arg, ok := call.Args[0].(*ast.Ident); ok && arg.Name == name {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

func reportPrealloc(pass *analysis.Pass, decl ast.Stmt, name string, typ ast.Expr, loop *ast.RangeStmt) {
	diag := analysis.Diagnostic{
		Pos:     decl.Pos(),
		End:     decl.End(),
		Message: fmt.Sprintf("%s is appended to in the following range loop but declared without capacity; pre-size it with make(..., 0, len(...))", name),
	}
	// Deterministic fix only when the range source is a plain identifier
	// that the loop body does not reassign.
	if src, ok := loop.X.(*ast.Ident); ok && !reassigns(loop.Body, src.Name) {
		var b strings.Builder
		_ = printer.Fprint(&b, token.NewFileSet(), typ)
		newDecl := fmt.Sprintf("%s := make(%s, 0, len(%s))", name, b.String(), src.Name)
		diag.SuggestedFixes = []analysis.SuggestedFix{{
			Message: fmt.Sprintf("pre-size %s to len(%s)", name, src.Name),
			TextEdits: []analysis.TextEdit{
				{Pos: decl.Pos(), End: decl.End(), NewText: []byte(newDecl)},
			},
		}}
	}
	pass.Report(diag)
}

// reassigns reports whether body assigns to name.
func reassigns(body *ast.BlockStmt, name string) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		as, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for _, lhs := range as.Lhs {
			if id, ok := lhs.(*ast.Ident); ok && id.Name == name {
				found = true
				return false
			}
		}
		return true
	})
	return found
}
