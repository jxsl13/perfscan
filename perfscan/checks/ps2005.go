package checks

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS2005 reports regexp.Compile/MustCompile calls inside loops. For a
// MustCompile with a constant pattern it attaches a deterministic hoist fix.
var PS2005 = register(&lint.Check{
	ID:       "PS2005",
	Category: "alloc",
	Slug:     "regexp-compile-in-loop",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "a regexp.Compile/MustCompile inside a loop",
		Text: `Compiling a regular expression parses the pattern and builds the
matcher program; doing that on every loop iteration repeats work whose result
never changes for a constant pattern. Hoist the compile out of the loop (or
to package level).

The automatic fix rewrites a MustCompile with a literal pattern by binding it
to a variable immediately before the outermost enclosing loop. The rewrite is
mechanical and bit-identical: the same pattern compiles to the same matcher.
Calls to Compile (two return values, error handling) are reported but left to
the reader.`,
		Before: `for _, s := range lines {
	if regexp.MustCompile("^a+$").MatchString(s) { hits++ }
}`,
		After: `psRe := regexp.MustCompile("^a+$")
for _, s := range lines {
	if psRe.MatchString(s) { hits++ }
}`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2005",
		Doc:  "regexp compile inside a loop",
		Run:  runPS2005,
	},
})

var regexpCompileFuncs = map[string]bool{
	"Compile":      true,
	"MustCompile":  true,
	"CompilePOSIX": true,
	// MustCompilePOSIX included for completeness.
	"MustCompilePOSIX": true,
}

func runPS2005(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		astutil.WithStack(f, func(n ast.Node, stack []ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			name, ok := astutil.PkgFuncCall(call.Fun, "regexp", regexpCompileFuncs)
			if !ok {
				return true
			}
			if _, inLoop := astutil.InLoop(stack); !inLoop {
				return true
			}
			diag := analysis.Diagnostic{
				Pos:     call.Pos(),
				End:     call.End(),
				Message: fmt.Sprintf("regexp.%s inside a loop recompiles an invariant pattern every iteration; hoist it out of the loop", name),
			}
			if strings.HasPrefix(name, "Must") {
				if fix := hoistRegexpFix(pass.Fset, stack, call); fix != nil {
					diag.SuggestedFixes = []analysis.SuggestedFix{*fix}
				}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}

// hoistRegexpFix builds the two-edit hoist: insert a binding before the
// outermost enclosing loop and replace the in-loop call with the variable.
// Only literal patterns are hoisted — a pattern built from loop state is not
// invariant.
func hoistRegexpFix(fset *token.FileSet, stack []ast.Node, call *ast.CallExpr) *analysis.SuggestedFix {
	if len(call.Args) != 1 {
		return nil
	}
	lit, ok := call.Args[0].(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return nil
	}
	loop, ok := astutil.OutermostLoop(stack)
	if !ok {
		return nil
	}
	sel := call.Fun.(*ast.SelectorExpr)
	pos := fset.Position(call.Pos())
	name := fmt.Sprintf("psRe%d", pos.Line)
	loopPos := fset.Position(loop.Pos())
	// Assume gofmt indentation (tabs): the loop starts at column loopPos.Column,
	// i.e. loopPos.Column-1 tabs of indentation.
	indent := strings.Repeat("\t", loopPos.Column-1)
	binding := fmt.Sprintf("%s := regexp.%s(%s)\n%s", name, sel.Sel.Name, lit.Value, indent)
	return &analysis.SuggestedFix{
		Message: fmt.Sprintf("hoist regexp.%s out of the loop", sel.Sel.Name),
		TextEdits: []analysis.TextEdit{
			{Pos: loop.Pos(), End: loop.Pos(), NewText: []byte(binding)},
			{Pos: call.Pos(), End: call.End(), NewText: []byte(name)},
		},
	}
}
