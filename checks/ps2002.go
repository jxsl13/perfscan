package checks

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"strings"

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
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "a strings.Builder/bytes.Buffer written in a loop with no Grow",
		Text: `A builder written in a loop without a capacity hint grows its
backing array geometrically: each growth is an allocation plus a copy of
everything written so far. One Grow call with an estimate (even a rough
lower bound) removes most of the copies.

The automatic fix covers only the simplest shape — a loop whose sole
statement is B.WriteString(s) over a []string — by inserting an exact
pre-count loop and a Grow call before it. Grow is a hint, so the rewrite is
behavior-preserving. Every other shape stays advisory: the right size is a
fact about the data, not the syntax, so the fix is left to the reader.`,
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
		diag := analysis.Diagnostic{
			Pos:     call.Pos(),
			End:     call.End(),
			Message: id.Name + " is written in a loop with no " + id.Name + ".Grow call in this function; a capacity hint removes the geometric growth copies",
		}
		if fix := growBuilderFix(pass, fn, stack, call, sel, id); fix != nil {
			diag.SuggestedFixes = []analysis.SuggestedFix{*fix}
		}
		pass.Report(diag)
		return true
	})
}

// growBuilderFix builds the pre-count-and-Grow fix for the one shape where
// the exact total is mechanically computable: the builder is written in
// exactly one loop of the form
//
//	for _, s := range src { b.WriteString(s) }
//
// with src a plain identifier of type []string and s the range value. The
// fix inserts, immediately before that loop at the same indentation,
//
//	psTotalN := 0
//	for _, s := range src {
//		psTotalN += len(s)
//	}
//	b.Grow(psTotalN)
//
// (N = loop line number). Grow is a capacity hint, so the rewrite is
// behavior-preserving. Anything else — multiple writes, other write methods,
// derived arguments, non-ident sources — stays advisory.
func growBuilderFix(pass *analysis.Pass, fn *ast.FuncDecl, stack []ast.Node, call *ast.CallExpr, sel *ast.SelectorExpr, builder *ast.Ident) *analysis.SuggestedFix {
	if sel.Sel.Name != "WriteString" || len(call.Args) != 1 {
		return nil
	}
	loopNode, ok := astutil.InLoop(stack)
	if !ok {
		return nil
	}
	rng, ok := loopNode.(*ast.RangeStmt)
	if !ok || rng.Tok != token.DEFINE {
		return nil
	}
	// The write must be the loop body's only statement.
	if len(rng.Body.List) != 1 {
		return nil
	}
	expr, ok := rng.Body.List[0].(*ast.ExprStmt)
	if !ok || expr.X != ast.Expr(call) {
		return nil
	}
	// for _, s := range src — blank key, named value.
	key, ok := rng.Key.(*ast.Ident)
	if !ok || key.Name != "_" {
		return nil
	}
	val, ok := rng.Value.(*ast.Ident)
	if !ok || val.Name == "_" {
		return nil
	}
	// The argument must be exactly the range value.
	arg, ok := call.Args[0].(*ast.Ident)
	if !ok {
		return nil
	}
	valObj := pass.TypesInfo.ObjectOf(val)
	if valObj == nil || pass.TypesInfo.ObjectOf(arg) != valObj {
		return nil
	}
	// src must be a plain identifier of type []string, so re-ranging it
	// before the loop is side-effect free and len(s) is the byte count.
	src, ok := rng.X.(*ast.Ident)
	if !ok || src.Name == "_" {
		return nil
	}
	slice, ok := pass.TypesInfo.TypeOf(src).(*types.Slice)
	if !ok {
		return nil
	}
	elem, ok := slice.Elem().(*types.Basic)
	if !ok || elem.Kind() != types.String {
		return nil
	}
	// The matched loop must hold the only in-loop write to this builder in
	// the function; otherwise the counted total is not the whole story.
	writes := 0
	astutil.WithStack(fn.Body, func(n ast.Node, st []ast.Node) bool {
		c, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		s, ok := c.Fun.(*ast.SelectorExpr)
		if !ok || !builderWriteMethods[s.Sel.Name] {
			return true
		}
		if id, ok := s.X.(*ast.Ident); !ok || id.Name != builder.Name {
			return true
		}
		if _, in := astutil.InLoop(st); in {
			writes++
		}
		return true
	})
	if writes != 1 {
		return nil
	}
	loopPos := pass.Fset.Position(rng.Pos())
	name := fmt.Sprintf("psTotal%d", loopPos.Line)
	// Assume gofmt indentation (tabs): the loop starts at column
	// loopPos.Column, i.e. loopPos.Column-1 tabs of indentation.
	indent := strings.Repeat("\t", loopPos.Column-1)
	text := fmt.Sprintf("%[1]s := 0\n%[2]sfor _, %[3]s := range %[4]s {\n%[2]s\t%[1]s += len(%[3]s)\n%[2]s}\n%[2]s%[5]s.Grow(%[1]s)\n%[2]s",
		name, indent, val.Name, src.Name, builder.Name)
	return &analysis.SuggestedFix{
		Message: "pre-count the total size and call " + builder.Name + ".Grow before the loop",
		TextEdits: []analysis.TextEdit{
			{Pos: rng.Pos(), End: rng.Pos(), NewText: []byte(text)},
		},
	}
}
