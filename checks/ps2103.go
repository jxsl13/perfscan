package checks

import (
	"go/ast"
	"go/token"
	"regexp"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS2103 reports fmt.Sprintf calls in loops whose format only concatenates
// simple verbs — cheaper as strconv/concat/Builder writes.
var PS2103 = register(&lint.Check{
	ID:       "PS2103",
	Category: "alloc",
	Slug:     "sprintf-concat-in-loop",
	Level:    lint.LevelIdiomatic,
	Doc: lint.Documentation{
		Title: "fmt.Sprintf in a loop for simple concatenation or conversion",
		Text: `fmt.Sprintf parses its format string and boxes every argument
into an interface on each call. When the format only splices %s/%d/%v
values into literal text, the same string is built cheaper with plain
concatenation, strconv.Itoa, or WriteString/strconv.AppendInt into an
existing builder — no format parsing, no boxing.

Only formats made of literal text and bare %s/%d/%v verbs are reported;
width, precision, flags, and float/hex verbs genuinely need fmt.`,
		Before: `for _, u := range users {
	keys = append(keys, fmt.Sprintf("%s:%d", u.Name, u.ID))
}`,
		After: `for _, u := range users {
	keys = append(keys, u.Name+":"+strconv.Itoa(u.ID))
}`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2103",
		Doc:  "fmt.Sprintf for simple concatenation in a loop",
		Run:  runPS2103,
	},
})

// simpleVerbFormat matches formats consisting only of literal text (no %)
// and bare %s/%d/%v verbs.
var simpleVerbFormat = regexp.MustCompile(`^(?:[^%]|%[sdv])*$`)

func runPS2103(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		astutil.WithStack(f, func(n ast.Node, stack []ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) < 2 {
				return true
			}
			if _, ok := astutil.PkgFuncCall(call.Fun, "fmt", map[string]bool{"Sprintf": true}); !ok {
				return true
			}
			lit, ok := call.Args[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			format := strings.Trim(lit.Value, "`\"")
			if !simpleVerbFormat.MatchString(format) {
				return true
			}
			if _, inLoop := astutil.InLoop(stack); !inLoop {
				return true
			}
			pass.Report(analysis.Diagnostic{
				Pos:     call.Pos(),
				End:     call.End(),
				Message: "fmt.Sprintf in a loop parses its format and boxes every argument per iteration; this format only splices simple verbs — build the string with concatenation or strconv instead",
			})
			return true
		})
	}
	return nil, nil
}
