package checks

import (
	"fmt"
	"go/ast"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS5082 removes strings.Clone layers immediately consumed by typed scalar
// standard-library observers that cannot retain or expose the cloned string.
var PS5082 = register(&lint.Check{
	ID:       "PS5082",
	Category: "alloc",
	Slug:     "string-clone-fed-scalar-stdlib-observer",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "a scalar standard-library observer is fed throwaway string clones",
		Text: `strings.Clone deliberately allocates a distinct copy. That copy is
wasted when it is passed directly to an operation that returns only a scalar
observation and cannot retain or expose the input:

  strings.Compare(strings.Clone(a), strings.Clone(b)) -> strings.Compare(a, b)
  strings.Contains(strings.Clone(s), strings.Clone(q)) -> strings.Contains(s, q)
  utf8.ValidString(strings.Clone(s))                    -> utf8.ValidString(s)
  filepath.IsAbs(strings.Clone(name))                   -> filepath.IsAbs(name)
  headers.Get(strings.Clone(key))                       -> headers.Get(key)

This check resolves every callee through go/types, matches an allowlisted
outer observer, and unwraps arbitrarily deep strings.Clone chains from all
safe arguments in one suggested fix. Package aliases and exact named-receiver
methods work. Shadowed helpers, unrelated same-named methods, methods named
Clone, dot imports, ellipsis calls, and type-changing wrappers stay untouched.

The allowlist contains strings.Compare, Contains, ContainsAny, ContainsRune,
Count, EqualFold, HasPrefix, HasSuffix, Index, IndexAny, IndexByte, IndexRune,
LastIndex, LastIndexAny, and LastIndexByte, plus unicode/utf8's scalar
DecodeRuneInString, DecodeLastRuneInString, FullRuneInString,
RuneCountInString, and ValidString observations, hash/maphash.String, and the
path and path/filepath Match/IsAbs functions. filepath.IsLocal,
strconv.CanBackquote, http.ParseHTTPVersion, mime.TypeByExtension,
os.Getenv/LookupEnv, and exact http.Header, textproto.MIMEHeader, and
url.Values Get/Has lookups are also covered. The path matchers use a package
sentinel for malformed patterns and retain neither string.

String-returning functions are intentionally excluded. For example,
strings.TrimSpace may return its input, so a caller could use strings.Clone to
prevent the result from retaining a much larger backing allocation. Readers
also retain their input. Callback-taking functions stay outside the rule
because callbacks can observe unrelated mutable state. strconv parsers are
excluded because their *NumError result may retain the input string.

The rewrite is BIT-IDENTICAL for race-free Go programs. Strings are immutable,
each base expression remains evaluated once in its original argument
position, and the observer returns the same scalar values. Only forced copies
disappear. Comments keep the finding advisory. The shared multi-package fix
engine removes a now-unused strings import only when doing so is structurally
and semantically safe.`,
		Before: `order := strings.Compare(
	strings.Clone(strings.Clone(left)),
	strings.Clone(right),
)`,
		After: `order := strings.Compare(left, right)`,
		MeasuredWin: `On Apple M2 Pro, strings.Compare over four Clone layers on
two equal 65,527-byte strings measured 17133 ns/op, 262144 B/op, 4 allocs/op ->
1782 ns/op, 0 B/op, 0 allocs/op (median of five runs): 9.61x faster and 89.6%
less time, eliminating all four forced string copies.
path.IsAbs(strings.Clone(name)) on a 65,527-byte relative path measured
3,920 ns/op, 65,536 B/op, 1 alloc/op versus 1.878 ns/op, 0 B/op, 0 allocs/op
(median of five 200ms runs): 2,087x faster and more than 99.9% less time.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5082",
		Doc:  "throwaway strings.Clone calls immediately feed scalar stdlib observers",
		Run:  runPS5082,
	},
})

type ps5082Observer struct {
	kind     typedCallKind
	receiver string
	indices  []int
}

func ps5082PackageObserver(indices ...int) ps5082Observer {
	return ps5082Observer{kind: typedPackageFunc, indices: indices}
}

func ps5082MethodObserver(receiver string, indices ...int) ps5082Observer {
	return ps5082Observer{kind: typedMethod, receiver: receiver, indices: indices}
}

var ps5082Observers = map[string]map[string]ps5082Observer{
	"strings": {
		"Compare":       ps5082PackageObserver(0, 1),
		"Contains":      ps5082PackageObserver(0, 1),
		"ContainsAny":   ps5082PackageObserver(0, 1),
		"ContainsRune":  ps5082PackageObserver(0),
		"Count":         ps5082PackageObserver(0, 1),
		"EqualFold":     ps5082PackageObserver(0, 1),
		"HasPrefix":     ps5082PackageObserver(0, 1),
		"HasSuffix":     ps5082PackageObserver(0, 1),
		"Index":         ps5082PackageObserver(0, 1),
		"IndexAny":      ps5082PackageObserver(0, 1),
		"IndexByte":     ps5082PackageObserver(0),
		"IndexRune":     ps5082PackageObserver(0),
		"LastIndex":     ps5082PackageObserver(0, 1),
		"LastIndexAny":  ps5082PackageObserver(0, 1),
		"LastIndexByte": ps5082PackageObserver(0),
	},
	"unicode/utf8": {
		"DecodeLastRuneInString": ps5082PackageObserver(0),
		"DecodeRuneInString":     ps5082PackageObserver(0),
		"FullRuneInString":       ps5082PackageObserver(0),
		"RuneCountInString":      ps5082PackageObserver(0),
		"ValidString":            ps5082PackageObserver(0),
	},
	"hash/maphash": {
		"String": ps5082PackageObserver(1),
	},
	"path": {
		"IsAbs": ps5082PackageObserver(0),
		"Match": ps5082PackageObserver(0, 1),
	},
	"path/filepath": {
		"IsAbs":   ps5082PackageObserver(0),
		"IsLocal": ps5082PackageObserver(0),
		"Match":   ps5082PackageObserver(0, 1),
	},
	"strconv": {
		"CanBackquote": ps5082PackageObserver(0),
	},
	"net/http": {
		"Get":              ps5082MethodObserver("Header", 0),
		"ParseHTTPVersion": ps5082PackageObserver(0),
	},
	"net/textproto": {
		"Get": ps5082MethodObserver("MIMEHeader", 0),
	},
	"net/url": {
		"Get": ps5082MethodObserver("Values", 0),
		"Has": ps5082MethodObserver("Values", 0),
	},
	"mime": {
		"TypeByExtension": ps5082PackageObserver(0),
	},
	"os": {
		"Getenv":    ps5082PackageObserver(0),
		"LookupEnv": ps5082PackageObserver(0),
	},
}

func runPS5082(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		compareToZero := ps5082CompareToZeroCalls(pass, file)
		ast.Inspect(file, func(node ast.Node) bool {
			observerCall, ok := node.(*ast.CallExpr)
			if !ok || observerCall.Ellipsis.IsValid() {
				return true
			}
			fn, sig, ok := typedCallee(pass, observerCall.Fun)
			if !ok || fn.Pkg() == nil {
				return true
			}
			observer, ok := ps5082Observers[fn.Pkg().Path()][fn.Name()]
			if !ok || (observer.kind == typedPackageFunc) != (sig.Recv() == nil) ||
				observer.receiver != "" && !typedReceiverNamed(sig, fn.Pkg().Path(), observer.receiver) {
				return true
			}
			// PS5106 owns Compare(...Clone...) compared against zero and emits
			// the final `a OP b` fixed point with the Clone layers removed. If
			// PS5082 also rewrote the inner call, the two fixes would overlap
			// and whichever won would leave work for a later -fix pass.
			if fn.Pkg().Path() == "strings" && fn.Name() == "Compare" && compareToZero[observerCall] {
				return true
			}

			var matches []typedUnaryCallChain
			totalLayers := 0
			for _, index := range observer.indices {
				if index < 0 || index >= len(observerCall.Args) {
					continue
				}
				if matched, ok := ps5082CloneChain(pass, observerCall.Args[index]); ok {
					matches = append(matches, matched)
					totalLayers += len(matched.calls)
				}
			}
			if totalLayers == 0 {
				return true
			}

			spans := make([]tokenSpan, 0, totalLayers*2)
			paths := make([]string, 0, totalLayers)
			for _, matched := range matches {
				spans = append(spans, matched.spans...)
				paths = append(paths, matched.paths...)
			}
			diagnostic := analysis.Diagnostic{
				Pos:     observerCall.Pos(),
				End:     observerCall.End(),
				Message: fmt.Sprintf("%s.%s scalar observation consumes %d throwaway strings.Clone layer(s) across %d argument(s); observe the original strings directly", fn.Pkg().Path(), fn.Name(), totalLayers, len(matches)),
			}
			if fix, ok := fixDeletedCallScaffoldingPaths(pass, file, paths, "remove string clones before scalar observation", spans...); ok {
				diagnostic.SuggestedFixes = []analysis.SuggestedFix{fix}
			}
			pass.Report(diagnostic)
			return true
		})
	}
	return nil, nil
}

func ps5082CompareToZeroCalls(pass *analysis.Pass, file *ast.File) map[*ast.CallExpr]bool {
	result := make(map[*ast.CallExpr]bool)
	ast.Inspect(file, func(node ast.Node) bool {
		bin, ok := node.(*ast.BinaryExpr)
		if !ok || ps5106OpString(bin.Op) == "" {
			return true
		}
		if call := ps5106CompareCall(pass, bin.X); call != nil && ps5105ZeroLit(pass, bin.Y) {
			result[call] = true
		}
		if call := ps5106CompareCall(pass, bin.Y); call != nil && ps5105ZeroLit(pass, bin.X) {
			result[call] = true
		}
		return true
	})
	return result
}

func ps5082CloneChain(pass *analysis.Pass, expr ast.Expr) (typedUnaryCallChain, bool) {
	return matchTypedUnaryPackageCallChain(pass, expr, isTypedStringStdlibClone)
}
