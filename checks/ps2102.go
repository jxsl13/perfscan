package checks

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"

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
	Doc: lint.Documentation{
		Title: "string concatenation with += inside a loop",
		Text: `Strings are immutable: s += x allocates a fresh string of
len(s)+len(x) and copies both halves, so a loop of appends does O(n²) copy
traffic. A strings.Builder amortizes the growth (and PS2002 will then ask
it to be pre-sized).

Advisory: the rewrite touches every later use of the variable (b.String()),
which is mechanical but not single-site.`,
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
			pass.Report(analysis.Diagnostic{
				Pos:     as.Pos(),
				End:     as.End(),
				Message: fmt.Sprintf("string %s grows by concatenation in a loop — O(n²) copy traffic; build it with a strings.Builder", id.Name),
			})
			return true
		})
	}
	return nil, nil
}
