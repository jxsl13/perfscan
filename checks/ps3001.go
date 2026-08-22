package checks

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS3001 reports reflection-based fmt scanning inside loops.
var PS3001 = register(&lint.Check{
	ID:       "PS3001",
	Category: "indirect",
	Slug:     "reflection-in-loop",
	Level:    lint.LevelIdiomatic,
	Doc: lint.Documentation{
		Title: "a reflection-based fmt scan (Sscanf/Sscan/Fscanf) in a loop",
		Text: `fmt's scanning functions parse their format and drive every
argument through reflection on each call. In a loop that cost repeats per
iteration. Replace with strconv conversions, manual splitting, or a
bufio.Scanner over the input.`,
		Before: `for _, line := range lines {
	fmt.Sscanf(line, "%d %d", &a, &b)
}`,
		After: `for _, line := range lines {
	i := strings.IndexByte(line, ' ')
	a, _ = strconv.Atoi(line[:i])
	b, _ = strconv.Atoi(line[i+1:])
}`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS3001",
		Doc:  "reflection-based fmt scan in a loop",
		Run:  runPS3001,
	},
})

var fmtScanFuncs = map[string]bool{
	"Sscan":   true,
	"Sscanf":  true,
	"Sscanln": true,
	"Fscan":   true,
	"Fscanf":  true,
	"Fscanln": true,
	"Scan":    true,
	"Scanf":   true,
	"Scanln":  true,
}

func runPS3001(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		astutil.WithStack(f, func(n ast.Node, stack []ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			name, ok := astutil.PkgFuncCall(pass.TypesInfo, call.Fun, "fmt", fmtScanFuncs)
			if !ok {
				return true
			}
			if _, inLoop := astutil.InLoop(stack); !inLoop {
				return true
			}
			pass.Report(analysis.Diagnostic{
				Pos:     call.Pos(),
				End:     call.End(),
				Message: "fmt." + name + " in a loop pays format parsing and reflection per iteration; use strconv or manual parsing",
			})
			return true
		})
	}
	return nil, nil
}
