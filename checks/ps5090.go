package checks

import (
	"fmt"
	"go/ast"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS5090 removes strings.Clone chains before strconv quoting functions that
// always materialize output independent of the source string.
var PS5090 = register(&lint.Check{
	ID:       "PS5090",
	Category: "alloc",
	Slug:     "string-clone-fed-stdlib-quote",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "a strconv quoting function formats a throwaway string Clone",
		Text: `strconv.Quote, QuoteToASCII, and QuoteToGraphic always construct a
quoted result with surrounding quotes and required escapes. Their AppendQuote
siblings append the same independent representation to caller-owned bytes.
Cloning the source string first cannot affect either result:

  strconv.Quote(strings.Clone(text))
  strconv.QuoteToASCII(strings.Clone(strings.Clone(text)))
  strconv.AppendQuote(dst, strings.Clone(text))
  strconv.AppendQuoteToGraphic(dst, strings.Clone(text))

This check resolves the exact strconv package function and every strings.Clone
layer through go/types. Package aliases and arbitrary clone depth work;
shadowed functions, function values, dot-imported Clone calls, ellipsis,
type-changing conversions, and same-named user helpers stay untouched.

The rule is deliberately not generalized to strconv.Unquote or numeric
parsers. Unquote can return text derived directly from its input and parser
errors retain the Num string, so Clone may be an intentional backing-retention
boundary in those calls. Generic fmt and encoding APIs also remain outside the
allowlist.

For safe Go programs the rewrite preserves every byte, invalid-UTF-8 escaping,
Unicode classification, destination length/capacity/aliasing for Append forms,
panics, and evaluation order. Strings are immutable and the source expression
is evaluated once in the same argument position; only its forced backing copy
disappears. Comments keep the finding advisory. The shared call-chain and
import-liveness editor removes all Clone layers and a newly unused strings
import in one fix, while terminal ownership prevents an overlapping nested
Clone diagnostic.`,
		Before: `quoted := strconv.QuoteToASCII(
	strings.Clone(strings.Clone(text)),
)`,
		After: `quoted := strconv.QuoteToASCII(text)`,
		MeasuredWin: `On Apple M2 Pro quoting a 68,275-byte string (median of five
100-iteration runs), strconv.Quote(strings.Clone(s)) measured 292,857 ns/op,
253,952 B/op, 3 allocs/op versus strconv.Quote(s) at 284,169 ns/op,
180,224 B/op, 2 allocs/op: 3.0% less time and 29.0% fewer allocated bytes,
removing one 73,728-byte copy.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5090",
		Doc:  "strconv quoting function consumes a throwaway strings.Clone",
		Run:  runPS5090,
	},
})

var ps5090Functions = map[string]struct {
	index int
	arity int
}{
	"Quote":                {index: 0, arity: 1},
	"QuoteToASCII":         {index: 0, arity: 1},
	"QuoteToGraphic":       {index: 0, arity: 1},
	"AppendQuote":          {index: 1, arity: 2},
	"AppendQuoteToASCII":   {index: 1, arity: 2},
	"AppendQuoteToGraphic": {index: 1, arity: 2},
}

func runPS5090(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			chain, name, matched := ps5090Match(pass, call)
			if !matched {
				return true
			}
			diagnostic := analysis.Diagnostic{
				Pos:     call.Pos(),
				End:     call.End(),
				Message: fmt.Sprintf("strconv.%s materializes an independent quoted representation but receives %d throwaway strings.Clone layer(s); quote the original string directly", name, len(chain.calls)),
			}
			if fix, ok := fixDeletedCallScaffoldingPaths(pass, file, chain.paths, "remove string clones before strconv quoting", chain.spans...); ok {
				diagnostic.SuggestedFixes = []analysis.SuggestedFix{fix}
			}
			pass.Report(diagnostic)
			return true
		})
	}
	return nil, nil
}

func ps5090Match(pass *analysis.Pass, call *ast.CallExpr) (typedUnaryCallChain, string, bool) {
	if call.Ellipsis.IsValid() {
		return typedUnaryCallChain{}, "", false
	}
	function, signature, ok := typedCallee(pass, call.Fun)
	if !ok || signature.Recv() != nil || function.Pkg() == nil || function.Pkg().Path() != "strconv" {
		return typedUnaryCallChain{}, "", false
	}
	spec, ok := ps5090Functions[function.Name()]
	if !ok || len(call.Args) != spec.arity {
		return typedUnaryCallChain{}, "", false
	}
	chain, ok := matchTypedUnaryPackageCallChain(pass, call.Args[spec.index], isTypedStringStdlibClone)
	return chain, function.Name(), ok
}
