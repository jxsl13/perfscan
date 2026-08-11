package checks

import (
	"fmt"
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS2002 reports strings.Builder/bytes.Buffer values written in a loop with
// no Grow call anywhere in the enclosing function.
var PS2002 = register(&lint.Check{
	ID:       "PS2002",
	Category: "alloc",
	Slug:     "unsized-builder",
	Level:    lint.LevelIdiomatic,
	Doc: lint.Documentation{
		Title: "a strings.Builder/bytes.Buffer written in a loop with no Grow",
		Text: `A builder written in a loop without a capacity hint grows its
backing array geometrically: each growth is an allocation plus a copy of
everything written so far. One Grow call with an estimate (even a rough
lower bound) removes most of the copies.

Advisory: the right size is a fact about the data, not the syntax, so the
fix is left to the reader.`,
		Before: `var b strings.Builder
for _, s := range parts {
	b.WriteString(s)
}`,
		After: `var b strings.Builder
b.Grow(total) // estimated or computed total size
for _, s := range parts {
	b.WriteString(s)
}`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2002",
		Doc:  "builder written in a loop with no Grow",
		Run:  runPS2002,
	},
})

var builderWriteMethods = map[string]bool{
	"WriteString": true,
	"WriteByte":   true,
	"WriteRune":   true,
	"Write":       true,
}

func isBuilderType(t types.Type) bool {
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	named, ok := t.(*types.Named)
	if !ok {
		return false
	}
	obj := named.Obj()
	if obj.Pkg() == nil {
		return false
	}
	switch obj.Pkg().Path() {
	case "strings":
		return obj.Name() == "Builder"
	case "bytes":
		return obj.Name() == "Buffer"
	}
	return false
}

func runPS2002(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			checkBuilderFunc(pass, fn)
		}
	}
	return nil, nil
}

func checkBuilderFunc(pass *analysis.Pass, fn *ast.FuncDecl) {
	// Receivers with a Grow call anywhere in the function are considered
	// sized, wherever the call sits relative to the loop.
	grown := map[string]bool{}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Grow" {
			return true
		}
		if id, ok := sel.X.(*ast.Ident); ok {
			grown[id.Name] = true
		}
		return true
	})

	reported := map[string]bool{}
	astutil.WithStack(fn.Body, func(n ast.Node, stack []ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || !builderWriteMethods[sel.Sel.Name] {
			return true
		}
		id, ok := sel.X.(*ast.Ident)
		if !ok || grown[id.Name] || reported[id.Name] {
			return true
		}
		if _, inLoop := astutil.InLoop(stack); !inLoop {
			return true
		}
		t := pass.TypesInfo.TypeOf(sel.X)
		if t == nil || !isBuilderType(t) {
			return true
		}
		reported[id.Name] = true
		pass.Report(analysis.Diagnostic{
			Pos:     call.Pos(),
			End:     call.End(),
			Message: fmt.Sprintf("%s is written in a loop with no %s.Grow call in this function; a capacity hint removes the geometric growth copies", id.Name, id.Name),
		})
		return true
	})
}
