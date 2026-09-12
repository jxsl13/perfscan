package checks

import (
	"bytes"
	"errors"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
)

// ps6131ForcedBody regenerates a copy of the complete production body and
// replaces only the independently identified predicate. Parsing a fresh copy
// avoids mutating the analyzer's shared AST or claiming an arbitrary supplied
// clone's hash establishes production equivalence.
func ps6131ForcedBody(fset *token.FileSet, source *ps6131Source, serial bool) (string, error) {
	if source == nil || source.owner == nil || source.guard == nil {
		return "", errors.New("missing observed dispatch predicate")
	}
	var original bytes.Buffer
	if err := format.Node(&original, fset, source.owner.Body); err != nil {
		return "", err
	}
	// Identify the predicate by traversal position rather than textual matching:
	// repeated identical predicates elsewhere must remain untouched.
	index, wanted := -1, -1
	ast.Inspect(source.owner.Body, func(n ast.Node) bool {
		if statement, ok := n.(*ast.IfStmt); ok {
			index++
			if statement == source.guard {
				wanted = index
			}
		}
		return true
	})
	if wanted < 0 {
		return "", errors.New("observed predicate is outside production body")
	}
	parsed, err := parser.ParseFile(token.NewFileSet(), "copy.go", "package copy\nfunc copied() "+original.String(), parser.AllErrors)
	if err != nil {
		return "", err
	}
	body := parsed.Decls[0].(*ast.FuncDecl).Body
	index = -1
	changed := false
	ast.Inspect(body, func(n ast.Node) bool {
		if statement, ok := n.(*ast.IfStmt); ok {
			index++
			if index == wanted {
				value := "false"
				if serial {
					value = "true"
				}
				statement.Cond = ast.NewIdent(value)
				changed = true
			}
		}
		return true
	})
	if !changed {
		return "", errors.New("copied dispatch predicate disappeared")
	}
	var output bytes.Buffer
	if err := format.Node(&output, token.NewFileSet(), body); err != nil {
		return "", err
	}
	return output.String(), nil
}
