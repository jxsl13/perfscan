package checks

import (
	"go/ast"
	"go/printer"
	"go/token"
	"go/types"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS2103 reports fmt.Sprintf calls in loops whose format only concatenates
// simple verbs — cheaper as strconv/concat/Builder writes.
var PS2103 = register(&lint.Check{
	ID:       "PS2103",
	Category: "alloc",
	Slug:     "sprintf-concat-in-loop",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "fmt.Sprintf in a loop for simple concatenation or conversion",
		Text: `fmt.Sprintf parses its format string and boxes every argument
into an interface on each call. When the format only splices %s/%d/%v
values into literal text, the same string is built cheaper with plain
concatenation, strconv.Itoa, or WriteString/strconv.AppendInt into an
existing builder — no format parsing, no boxing.

Only formats made of literal text and bare %s/%d/%v verbs are reported;
width, precision, flags, and float/hex verbs genuinely need fmt.

The automatic fix rewrites the call into a plain concatenation when the
format is an interpreted ("...") literal splicing only %s/%v verbs and
every spliced argument is a plain (unnamed) string — for those, %s and %v
emit the argument verbatim, so the concatenation is bit-identical and no
strconv import is needed. Formats with %d, other argument types, named
string types, and raw-string formats stay advisory.`,
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
		// Collect first: applying all fixes may rewrite the file's last
		// fmt reference and orphan the import. The fix pipeline prunes
		// such an orphan afterwards — except in a cgo file (import "C"),
		// whose import block is never edited, so there the fixes are
		// withheld and the reports stay advisory.
		type site struct {
			call *ast.CallExpr
			fix  *analysis.SuggestedFix
		}
		var sites []site
		fixable := 0
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
			fix := sprintfConcatFix(pass, stack, call, lit)
			if fix != nil {
				fixable++
			}
			sites = append(sites, site{call, fix})
			return true
		})
		emitFixes := fixable > 0 && (pkgRefCount(pass, f, "fmt") > fixable || !ps2110ImportsC(f))
		for _, st := range sites {
			diag := analysis.Diagnostic{
				Pos:     st.call.Pos(),
				End:     st.call.End(),
				Message: "fmt.Sprintf in a loop parses its format and boxes every argument per iteration; this format only splices simple verbs — build the string with concatenation or strconv instead",
			}
			if emitFixes && st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// sprintfConcatFix rewrites fmt.Sprintf(format, args...) into a plain
// concatenation expression when that is provably bit-identical:
//
//   - the format is an interpreted ("...") string literal — raw (backtick)
//     literals would force re-escaping decisions;
//   - the DECODED format consists only of literal text and bare %s/%v
//     verbs (splitStringVerbs re-derives this on the decoded text, so a
//     "\x25d" smuggling a %d past the detection regex is rejected);
//   - the verb count matches the argument count exactly;
//   - every spliced argument is a plain (unnamed) basic string: %s and %v
//     both emit such a value verbatim, and concatenating a named string
//     type would change the type of the resulting expression.
//
// Everything else — %d (which would need a strconv import), non-string or
// named-string arguments, count mismatches — keeps the plain advisory
// report with no fix.
func sprintfConcatFix(pass *analysis.Pass, stack []ast.Node, call *ast.CallExpr, lit *ast.BasicLit) *analysis.SuggestedFix {
	if !strings.HasPrefix(lit.Value, `"`) {
		return nil
	}
	format, err := strconv.Unquote(lit.Value)
	if err != nil {
		return nil
	}
	segs, ok := splitStringVerbs(format)
	if !ok || len(segs) < 2 || len(segs)-1 != len(call.Args)-1 {
		return nil
	}
	for _, arg := range call.Args[1:] {
		b, isBasic := pass.TypesInfo.TypeOf(arg).(*types.Basic)
		if !isBasic || b.Info()&types.IsString == 0 {
			return nil
		}
	}
	// Literal segments become quoted strings, args slot between them in
	// order; empty segments (leading, trailing, or between adjacent verbs)
	// contribute nothing.
	tokens := make([]string, 0, 2*len(segs)-1)
	// One throwaway FileSet for every render: the printed args carry no
	// positions relative to it, so a single empty set gives identical
	// output without allocating a fresh FileSet per verb.
	fset := token.NewFileSet()
	for i, seg := range segs {
		if seg != "" {
			tokens = append(tokens, strconv.Quote(seg))
		}
		if i < len(segs)-1 {
			var b strings.Builder
			if err := printer.Fprint(&b, fset, call.Args[i+1]); err != nil {
				return nil
			}
			tokens = append(tokens, b.String())
		}
	}
	repl := strings.Join(tokens, "+")
	if len(tokens) > 1 && concatNeedsParens(stack, call) {
		repl = "(" + repl + ")"
	}
	return &analysis.SuggestedFix{
		Message: "replace fmt.Sprintf with string concatenation",
		TextEdits: []analysis.TextEdit{
			{Pos: call.Pos(), End: call.End(), NewText: []byte(repl)},
		},
	}
}

// splitStringVerbs splits a DECODED format string on its %s/%v verbs,
// returning the literal segments around them (verb count = len(segs)-1).
// Any other use of % — %d, %%, a trailing % — returns ok=false.
func splitStringVerbs(format string) ([]string, bool) {
	segs := make([]string, 0, len(format))
	var cur strings.Builder
	cur.Grow(len(format))
	for i := 0; i < len(format); i++ {
		c := format[i]
		if c != '%' {
			cur.WriteByte(c)
			continue
		}
		if i+1 >= len(format) || (format[i+1] != 's' && format[i+1] != 'v') {
			return nil, false
		}
		segs = append(segs, cur.String())
		cur.Reset()
		i++
	}
	segs = append(segs, cur.String())
	return segs, true
}

// concatNeedsParens reports whether the call sits in a syntactic position
// where a multi-token concatenation must be parenthesized to keep its
// grouping: operand of an index, slice, selector, type assertion, deref,
// or unary expression. Binary contexts are safe — string + is associative,
// and every lower-precedence operator leaves the concatenation grouped.
func concatNeedsParens(stack []ast.Node, call *ast.CallExpr) bool {
	if len(stack) == 0 {
		return false
	}
	switch p := stack[len(stack)-1].(type) {
	case *ast.IndexExpr:
		return p.X == ast.Expr(call)
	case *ast.SliceExpr:
		return p.X == ast.Expr(call)
	case *ast.SelectorExpr:
		return p.X == ast.Expr(call)
	case *ast.TypeAssertExpr:
		return p.X == ast.Expr(call)
	case *ast.StarExpr, *ast.UnaryExpr:
		return true
	}
	return false
}
