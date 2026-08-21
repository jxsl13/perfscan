package checks

import (
	"fmt"
	"go/ast"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS5087 removes strings.Clone chains before exact standard-library decoders
// whose returned values and errors cannot retain the input string.
var PS5087 = register(&lint.Check{
	ID:       "PS5087",
	Category: "alloc",
	Slug:     "string-clone-fed-independent-stdlib-decoder",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "an independent stdlib decoder is fed throwaway string clones",
		Text: `encoding/hex.DecodeString, encoding/base32.Encoding.DecodeString,
encoding/base64.Encoding.DecodeString, and net.ParseIP consume a string
synchronously and return storage independent of it. A strings.Clone directly
before one of these calls is therefore discarded immediately:

  hex.DecodeString(strings.Clone(text))
  base64.StdEncoding.DecodeString(strings.Clone(text))
  base32.StdEncoding.DecodeString(strings.Clone(strings.Clone(text)))
  net.ParseIP(strings.Clone(address))

This check resolves the exact package function or concrete standard-library
method plus every strings.Clone layer through go/types. Aliases and arbitrary
clone depth work. Shadowed functions, same-named user methods, method values,
dot-imported Clone calls, ellipsis, type-changing wrappers, and other parsers
stay untouched.

The allowlist is intentionally narrow. The three decoders allocate their
returned byte slice and report malformed input with value-only errors
(hex.InvalidByteError/ErrLength or base32/base64.CorruptInputError); net.ParseIP
returns a fresh 16-byte IP or nil. None can retain the cloned string. Parsers
whose result or error may keep the original text—strconv numeric parsers,
time.Parse, URL parsing, regexp compilation, readers, and string/subslice
transformers—are excluded because Clone may be an intentional retention
boundary.

For safe Go programs the rewrite preserves decoded bytes, partial output,
nilness, capacity, concrete error/value, IP bytes, panics, and evaluation
order. Strings are immutable and the base expression is still evaluated once;
only forced copies disappear. Comments keep the finding advisory. The shared
call-chain/import-liveness editor removes all Clone layers and a newly unused
strings import in one fix, and terminal ownership prevents an overlapping
nested-Clone diagnostic.`,
		Before: `decoded, err := base64.StdEncoding.DecodeString(
	strings.Clone(strings.Clone(encoded)),
)`,
		After: `decoded, err := base64.StdEncoding.DecodeString(encoded)`,
		MeasuredWin: `On Apple M2 Pro decoding a 91,036-byte base64 string (median
of five 100-iteration runs), DecodeString(strings.Clone(s)) measured
53,425 ns/op, 172,085 B/op, 2 allocs/op versus DecodeString(s) at 36,175 ns/op,
73,728 B/op, 1 alloc/op: 1.48x faster, 32.3% less time, and 57.2% fewer
allocated bytes while removing the forced string copy.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5087",
		Doc:  "independent stdlib decoder consumes throwaway strings.Clone chains",
		Run:  runPS5087,
	},
})

var ps5087Consumers = map[string]map[string]typedCallKind{
	"encoding/hex": {
		"DecodeString": typedPackageFunc,
	},
	"encoding/base32": {
		"DecodeString": typedMethod,
	},
	"encoding/base64": {
		"DecodeString": typedMethod,
	},
	"net": {
		"ParseIP": typedPackageFunc,
	},
}

func runPS5087(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			chain, packagePath, name, matched := ps5087Match(pass, call)
			if !matched {
				return true
			}
			diagnostic := analysis.Diagnostic{
				Pos:     call.Pos(),
				End:     call.End(),
				Message: fmt.Sprintf("%s.%s returns input-independent decoded data but receives %d throwaway strings.Clone layer(s); decode the original string directly", packagePath, name, len(chain.calls)),
			}
			if fix, ok := fixDeletedCallScaffoldingPaths(pass, file, chain.paths, "remove string clones before independent standard-library decoding", chain.spans...); ok {
				diagnostic.SuggestedFixes = []analysis.SuggestedFix{fix}
			}
			pass.Report(diagnostic)
			return true
		})
	}
	return nil, nil
}

func ps5087Match(pass *analysis.Pass, call *ast.CallExpr) (typedUnaryCallChain, string, string, bool) {
	if len(call.Args) != 1 || call.Ellipsis.IsValid() {
		return typedUnaryCallChain{}, "", "", false
	}
	function, signature, ok := typedCallee(pass, call.Fun)
	if !ok || function.Pkg() == nil {
		return typedUnaryCallChain{}, "", "", false
	}
	kind, ok := ps5087Consumers[function.Pkg().Path()][function.Name()]
	if !ok || (kind == typedPackageFunc) != (signature.Recv() == nil) {
		return typedUnaryCallChain{}, "", "", false
	}
	chain, ok := matchTypedUnaryPackageCallChain(pass, call.Args[0], isTypedStringStdlibClone)
	return chain, function.Pkg().Path(), function.Name(), ok
}
