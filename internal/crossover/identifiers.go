package crossover

import (
	"bytes"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
)

// replaceResultIdentifiers rewrites exact result objects by identifier token,
// never arbitrary _r-prefixed field/method names or neighboring identifiers.
func replaceResultIdentifiers(expression string, names map[string]string) (string, error) {
	fset := token.NewFileSet()
	parsed, err := parser.ParseExprFrom(fset, "expression.go", expression, parser.AllErrors)
	if err != nil {
		return "", err
	}
	selectors := map[*ast.Ident]bool{}
	ast.Inspect(parsed, func(n ast.Node) bool {
		if selector, ok := n.(*ast.SelectorExpr); ok {
			selectors[selector.Sel] = true
		}
		return true
	})
	ast.Inspect(parsed, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && !selectors[id] {
			if replacement, ok := names[id.Name]; ok {
				id.Name = replacement
			}
		}
		return true
	})
	var text bytes.Buffer
	if err := format.Node(&text, fset, parsed); err != nil {
		return "", err
	}
	return text.String(), nil
}
