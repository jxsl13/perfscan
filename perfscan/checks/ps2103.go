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
format is an interpreted ("...") literal splicing only %s/%v verbs over
plain (unnamed) strings — those emit the argument verbatim, so the
concatenation is bit-identical — and/or %d verbs over plain (unnamed)
integers, rendered as strconv.Itoa(x) for int,
strconv.FormatInt(int64(x), 10) for the other signed widths, and
strconv.FormatUint(uint64(x), 10) for the unsigned kinds. %d does not
consult fmt.Stringer (a Stringer integer still prints plain decimal under
%d), but it DOES honor a named type's fmt.Formatter — a Format method
controls the output of every verb, %d included. The unnamed-predeclared-
integer restriction is therefore load-bearing for bit-identity, not
cosmetic: an unnamed type cannot carry a Format method, so its %d output is
guaranteed plain decimal, equal to the strconv form. A named integer stays
advisory. A %d rewrite adds the strconv import when missing (honoring an
existing strconv alias).
Other argument types (%d over a non-integer, %s over a named string),
raw-string formats, a shadowed strconv name, and cgo files that would
need an import edit stay advisory.`,
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
		// All fixes of a run are applied together, so only the first fix
		// needing the strconv import carries the import edit (same
		// convention as PS2107/PS2137).
		importAdded := false
		astutil.WithStack(f, func(n ast.Node, stack []ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) < 2 {
				return true
			}
			if _, ok := astutil.PkgFuncCall(pass.TypesInfo, call.Fun, "fmt", map[string]bool{"Sprintf": true}); !ok {
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
			fix := sprintfConcatFix(pass, f, stack, call, lit, &importAdded)
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
//   - the DECODED format consists only of literal text and bare %s/%v/%d
//     verbs (splitStringVerbs re-derives this on the decoded text, so a
//     "\x25x" smuggling a %x past the detection regex is rejected);
//   - the verb count matches the argument count exactly;
//   - every %s/%v argument is a plain (unnamed) basic string: %s and %v
//     both emit such a value verbatim, and concatenating a named string
//     type would change the type of the resulting expression;
//   - every %d argument is of an unnamed predeclared integer type,
//     rendered through the shared strconvDecimalRepl (strconv.Itoa /
//     FormatInt(int64(x), 10) / FormatUint(uint64(x), 10)). %d does not
//     consult fmt.Stringer, but it DOES honor a named type's fmt.Formatter
//     (a Format method controls every verb, %d included) — so the
//     unnamed-predeclared restriction is LOAD-BEARING for bit-identity, not
//     cosmetic: an unnamed integer cannot carry a Format method, so its %d
//     output is guaranteed plain decimal == the strconv form. (It also keeps
//     strconv.Itoa's exactly-int parameter compiling.) Mirrors PS2107/PS2137.
//
// A %d rewrite needs the strconv import: ps2107PkgUsable resolves how to
// reference it (reusing an existing import's name, e.g. an alias), and the
// first fix of the file needing the import carries the ps2107ImportEdit.
// Everything else — a %d argument that is not an unnamed integer, a
// non-string or named-string %s/%v argument, count mismatches, a shadowed
// strconv name, a cgo file (import "C") whose import block would need the
// strconv insertion — keeps the plain advisory report with no fix.
func sprintfConcatFix(pass *analysis.Pass, f *ast.File, stack []ast.Node, call *ast.CallExpr, lit *ast.BasicLit, importAdded *bool) *analysis.SuggestedFix {
	if !strings.HasPrefix(lit.Value, `"`) {
		return nil
	}
	format, err := strconv.Unquote(lit.Value)
	if err != nil {
		return nil
	}
	segs, verbs, ok := splitStringVerbs(format)
	if !ok || len(verbs) == 0 || len(verbs) != len(call.Args)-1 {
		return nil
	}
	// Per-argument type guards. basics[i] is non-nil exactly for the %d
	// arguments and carries the integer type strconvDecimalRepl formats.
	// types.Default materializes an untyped constant as its default type
	// (an untyped int becomes int — Itoa territory); a named type never
	// asserts to *types.Basic and bails.
	basics := make([]*types.Basic, len(verbs))
	needStrconv := false
	for i, arg := range call.Args[1:] {
		if verbs[i] == 'd' {
			b, isBasic := types.Default(pass.TypesInfo.TypeOf(arg)).(*types.Basic)
			if !isBasic || b.Info()&types.IsInteger == 0 {
				return nil
			}
			basics[i] = b
			needStrconv = true
			continue
		}
		b, isBasic := pass.TypesInfo.TypeOf(arg).(*types.Basic)
		if !isBasic || b.Info()&types.IsString == 0 {
			return nil
		}
	}
	strconvName := "strconv"
	needImport := false
	if needStrconv {
		useName, need, usable := ps2107PkgUsable(pass, f, call.Pos(), "strconv", "strconv")
		if !usable || (need && ps2107ImportsC(f)) {
			return nil
		}
		strconvName, needImport = useName, need
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
			argText := b.String()
			if basics[i] != nil {
				dec, _ := strconvDecimalRepl(basics[i], argText)
				// dec always begins with "strconv."; requalify that leading
				// qualifier when the import is used under another name. Slice
				// rather than strings.Replace so this per-argument loop stays
				// allocation-free (and off perfscan's own PS2003).
				if strconvName != "strconv" {
					dec = strconvName + "." + dec[len("strconv."):]
				}
				argText = dec
			}
			tokens = append(tokens, argText)
		}
	}
	repl := strings.Join(tokens, "+")
	if len(tokens) > 1 && concatNeedsParens(stack, call) {
		repl = "(" + repl + ")"
	}
	edits := []analysis.TextEdit{
		{Pos: call.Pos(), End: call.End(), NewText: []byte(repl)},
	}
	if needImport && !*importAdded {
		edits = append(edits, ps2107ImportEdit(f, "strconv"))
		*importAdded = true
	}
	return &analysis.SuggestedFix{
		Message:   "replace fmt.Sprintf with string concatenation",
		TextEdits: edits,
	}
}

// splitStringVerbs splits a DECODED format string on its %s/%v/%d verbs,
// returning the literal segments around them and the verb letters in
// order (len(segs) == len(verbs)+1). Any other use of % — %x, %%, a
// width like %3d, a trailing % — returns ok=false.
func splitStringVerbs(format string) (segs []string, verbs []byte, ok bool) {
	segs = make([]string, 0, len(format))
	var cur strings.Builder
	cur.Grow(len(format))
	for i := 0; i < len(format); i++ {
		c := format[i]
		if c != '%' {
			cur.WriteByte(c)
			continue
		}
		if i+1 >= len(format) || (format[i+1] != 's' && format[i+1] != 'v' && format[i+1] != 'd') {
			return nil, nil, false
		}
		segs = append(segs, cur.String())
		cur.Reset()
		verbs = append(verbs, format[i+1])
		i++
	}
	segs = append(segs, cur.String())
	return segs, verbs, true
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
