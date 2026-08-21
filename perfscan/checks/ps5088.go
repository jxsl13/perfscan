package checks

import (
	"fmt"
	"go/ast"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS5088 removes Clone chains from the subject of exact standard-library
// regexp boolean matching calls, which never expose or retain the subject.
var PS5088 = register(&lint.Check{
	ID:       "PS5088",
	Category: "alloc",
	Slug:     "clone-fed-regexp-boolean-match",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "a regexp boolean match scans a throwaway Clone of its subject",
		Text: `regexp.Match and (*regexp.Regexp).Match only report whether a byte
subject matches; MatchString variants do the same for strings. They consume
the subject synchronously and cannot return or retain any view of it, so a
Clone immediately around that subject is wasted:

  compiled.Match(bytes.Clone(data))
  regexp.Match(pattern, slices.Clone(bytes.Clone(data)))
  compiled.MatchString(strings.Clone(text))
  regexp.MatchString(pattern, strings.Clone(strings.Clone(text)))

This check resolves both the exact regexp package function/method and every
bytes.Clone, slices.Clone, or strings.Clone layer through go/types. Package
aliases, explicit generic instantiations, promoted *regexp.Regexp methods, and
arbitrary clone depth work. Same-named user methods, interface dispatch,
method values, dot-imported Clone calls, ellipsis, and type-changing wrappers
stay untouched.

Only the match SUBJECT is rewritten. regexp.Match/MatchString compile their
pattern on each call and a syntax error can retain pattern text, so a
strings.Clone around the pattern may be an intentional retention boundary.
Find, FindAll, FindIndex, Split, Expand, and replacement APIs are excluded:
they may return input-backed slices/strings, append into caller storage, or
invoke callbacks.

For race-free safe Go programs the rewrite preserves the boolean, pattern
error type/text, input scan bytes, UTF-8 behavior, panics, and evaluation
order. Byte subjects remain mutable only outside the synchronous call; a
concurrent mutation would already be a data race. Each base expression is
still evaluated exactly once in its original argument position. Comments keep
the finding advisory. The shared typed call-chain/import editor removes all
Clone layers and newly unused imports in one fix, while terminal ownership
prevents an overlapping nested-Clone diagnostic.`,
		Before: `matched := compiled.Match(
	bytes.Clone(slices.Clone(subject)),
)`,
		After: `matched := compiled.Match(subject)`,
		MeasuredWin: `On Apple M2 Pro with a precompiled regexp scanning a
68,275-byte subject (median of five 100-iteration runs), Match(bytes.Clone(p))
measured 40,973 ns/op, 73,881 B/op, 1 alloc/op versus Match(p) at 27,251 ns/op,
76 B/op, 0 allocs/op: 1.50x faster, 33.5% less time, and effectively all
per-call allocated bytes removed.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5088",
		Doc:  "regexp boolean match consumes a throwaway Clone of its subject",
		Run:  runPS5088,
	},
})

func runPS5088(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			chain, name, matched := ps5088Match(pass, call)
			if !matched {
				return true
			}
			diagnostic := analysis.Diagnostic{
				Pos:     call.Pos(),
				End:     call.End(),
				Message: fmt.Sprintf("regexp.%s returns only match status but scans %d throwaway Clone layer(s) around its subject; match the original subject directly", name, len(chain.calls)),
			}
			if fix, ok := fixDeletedCallScaffoldingPaths(pass, file, chain.paths, "remove clones around regexp match subject", chain.spans...); ok {
				diagnostic.SuggestedFixes = []analysis.SuggestedFix{fix}
			}
			pass.Report(diagnostic)
			return true
		})
	}
	return nil, nil
}

func ps5088Match(pass *analysis.Pass, call *ast.CallExpr) (typedUnaryCallChain, string, bool) {
	if call.Ellipsis.IsValid() {
		return typedUnaryCallChain{}, "", false
	}
	function, signature, ok := typedCallee(pass, call.Fun)
	if !ok || function.Pkg() == nil || function.Pkg().Path() != "regexp" {
		return typedUnaryCallChain{}, "", false
	}
	method := signature.Recv() != nil
	var subjectIndex int
	var allowed func(string, string) bool
	switch function.Name() {
	case "Match":
		allowed = isTypedSliceStdlibClone
	case "MatchString":
		allowed = isTypedStringStdlibClone
	default:
		return typedUnaryCallChain{}, "", false
	}
	if method {
		if len(call.Args) != 1 {
			return typedUnaryCallChain{}, "", false
		}
		subjectIndex = 0
	} else {
		if len(call.Args) != 2 {
			return typedUnaryCallChain{}, "", false
		}
		subjectIndex = 1
	}
	chain, ok := matchTypedUnaryPackageCallChain(pass, call.Args[subjectIndex], allowed)
	return chain, function.Name(), ok
}
