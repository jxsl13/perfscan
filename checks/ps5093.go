package checks

import (
	"fmt"
	"go/ast"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS5093 collapses Len/Size calls on newly constructed readers and buffers to
// the input's length, composing redundant Clone/conversion removal.
var PS5093 = register(&lint.Check{
	ID:       "PS5093",
	Category: "alloc",
	Slug:     "ephemeral-reader-buffer-size-chain",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "an ephemeral reader or buffer is constructed only to ask its size",
		Text: `Fresh bytes.Reader, strings.Reader, and bytes.Buffer values start at
offset zero, so constructing one solely to call Len or Size repeats information
already available from the constructor input:

  strings.NewReader(text).Len()             -> len(text)
  strings.NewReader(text).Size()            -> int64(len(text))
  bytes.NewReader(data).Len()                -> len(data)
  bytes.NewReader(data).Size()               -> int64(len(data))
  bytes.NewBuffer(data).Len()                -> len(data)
  bytes.NewBufferString(text).Len()          -> len(text)

The shared typed method-on-constructor matcher resolves both calls through
go/types and pins the exact named receiver, so aliases work while methods on
other types, user constructors, function values, dot imports, and readers
stored or consumed before Len/Size stay untouched.

The terminal reducer also removes arbitrarily deep strings.Clone and
bytes.Clone/slices.Clone chains from the constructor argument. For byte
constructors it composes the byte-length identity len([]byte(text)) ==
len(text), including nested strings.Clone layers, in the SAME fix. Thus
bytes.NewReader(bytes.Clone([]byte(strings.Clone(text)))).Size() becomes
int64(len(text)) in one pass with every orphaned import removed.

An untyped nil constructor argument is excluded because len(nil) does not
compile. Direct constant-string inputs are also excluded: replacing a runtime
method result with a constant len expression can make formerly legal duplicate
switch cases fail to compile, and these trivial calls are already optimized by
the compiler. Non-constant strings and every typed byte slice remain eligible.

The rewrite is BIT-IDENTICAL for race-free Go programs. Constructors evaluate
the input once and initialize offset zero; Len returns its byte length and Size
the same length converted to int64. Nil and empty slices, invalid UTF-8, named
values behind explicit conversions, panics in input evaluation, result types,
and evaluation order are unchanged. The fix is withheld if an injected len or
int64 identifier is shadowed. Comments keep the finding advisory. The
shared import-liveness editor and nested-Clone ownership deliver the clean
builtin fixed point in one -fix pass.`,
		Before: `size := strings.NewReader(strings.Clone(text)).Size()
n := bytes.NewBuffer(bytes.Clone(data)).Len()`,
		After: `size := int64(len(text))
n := len(data)`,
		MeasuredWin: `On an Apple M2 Pro, the benchmark in
benchmarks/ps5093_test.go reduced a fresh strings.Reader Size call fed a forced
Clone from a median 3,847 ns/op and 65,536 B/op (1 allocation) to 1.864 ns/op
and zero allocations: about 2,064x faster while removing 65,536 bytes per
operation.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5093",
		Doc:  "ephemeral standard-library reader or buffer is constructed only to ask its size",
		Run:  runPS5093,
	},
})

type ps5093ConstructorSpec struct {
	receiver  string
	stringArg bool
	allowLen  bool
	allowSize bool
}

var ps5093Constructors = map[string]map[string]ps5093ConstructorSpec{
	"bytes": {
		"NewBuffer":       {receiver: "Buffer", allowLen: true},
		"NewBufferString": {receiver: "Buffer", stringArg: true, allowLen: true},
		"NewReader":       {receiver: "Reader", allowLen: true, allowSize: true},
	},
	"strings": {
		"NewReader": {receiver: "Reader", stringArg: true, allowLen: true, allowSize: true},
	},
}

type ps5093Match struct {
	call        typedMethodConstructorCall
	kept        ast.Expr
	paths       []string
	clone       typedUnaryCallChain
	cloneOK     bool
	nested      typedUnaryCallChain
	nestedOK    bool
	methodName  string
	constructor string
}

func runPS5093(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		ast.Inspect(file, func(node ast.Node) bool {
			outer, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			match, ok := ps5093SizeMatch(pass, outer)
			if !ok {
				return true
			}
			cloneLayers := 0
			if match.cloneOK {
				cloneLayers += len(match.clone.calls)
			}
			if match.nestedOK {
				cloneLayers += len(match.nested.calls)
			}
			diagnostic := analysis.Diagnostic{
				Pos: outer.Pos(),
				End: outer.End(),
				Message: fmt.Sprintf("%s.%s(...).%s constructs an ephemeral container only to recover its input length and carries %d throwaway Clone layer(s); read the original length directly",
					match.call.constructor.Pkg().Path(), match.constructor, match.methodName, cloneLayers),
			}
			prefix, suffix := []byte("len("), []byte(")")
			if match.methodName == "Size" {
				prefix, suffix = []byte("int64(len("), []byte("))")
			}
			if ps5093ReplacementNamesAvailable(pass, outer, match.methodName) {
				if fix, ok := fixReplacedCallScaffoldingPaths(pass, file, match.paths, "replace ephemeral reader/buffer size chain with len",
					analysis.TextEdit{Pos: outer.Pos(), End: match.kept.Pos(), NewText: prefix},
					analysis.TextEdit{Pos: match.kept.End(), End: outer.End(), NewText: suffix},
				); ok {
					diagnostic.SuggestedFixes = []analysis.SuggestedFix{fix}
				}
			}
			pass.Report(diagnostic)
			return true
		})
	}
	return nil, nil
}

func ps5093ReplacementNamesAvailable(pass *analysis.Pass, call *ast.CallExpr, method string) bool {
	if !predeclaredInScope(pass, call.Pos(), "len") {
		return false
	}
	return method != "Size" || predeclaredInScope(pass, call.Pos(), "int64")
}

func ps5093SizeMatch(pass *analysis.Pass, outer *ast.CallExpr) (ps5093Match, bool) {
	chain, ok := matchTypedMethodOnPackageConstructor(pass, outer)
	if !ok || len(outer.Args) != 0 || len(chain.constructorCall.Args) != 1 {
		return ps5093Match{}, false
	}
	path := chain.constructor.Pkg().Path()
	spec, ok := ps5093Constructors[path][chain.constructor.Name()]
	if !ok || chain.method.Pkg().Path() != path || !typedReceiverNamed(chain.methodSignature, path, spec.receiver) {
		return ps5093Match{}, false
	}
	methodName := chain.method.Name()
	if methodName != "Len" && methodName != "Size" ||
		methodName == "Len" && !spec.allowLen ||
		methodName == "Size" && !spec.allowSize {
		return ps5093Match{}, false
	}

	argument := chain.constructorCall.Args[0]
	if ps5092Nil(pass, argument) {
		return ps5093Match{}, false
	}
	kept := argument
	paths := []string{path}
	allowedClone := isTypedSliceStdlibClone
	if spec.stringArg {
		allowedClone = isTypedStringStdlibClone
	}
	clone, cloneOK := matchTypedUnaryPackageCallChain(pass, kept, allowedClone)
	if cloneOK {
		kept = clone.base
		paths = append(paths, clone.paths...)
	}

	var nested typedUnaryCallChain
	nestedOK := false
	if !spec.stringArg {
		if conversion, conversionOK := ps5084StringByteConversion(pass, kept); conversionOK {
			candidate := conversion.Args[0]
			nested, nestedOK = matchTypedUnaryPackageCallChain(pass, candidate, isTypedStringStdlibClone)
			if nestedOK {
				candidate = nested.base
			}
			if value, typed := pass.TypesInfo.Types[candidate]; !typed || value.Value == nil {
				kept = candidate
				if qualifierPath := ps5084TypeQualifierPath(pass, conversion.Fun); qualifierPath != "" {
					paths = append(paths, qualifierPath)
				}
				if nestedOK {
					paths = append(paths, nested.paths...)
				}
			} else {
				nestedOK = false
			}
		}
	}
	if value, typed := pass.TypesInfo.Types[kept]; typed && value.Value != nil {
		return ps5093Match{}, false
	}
	return ps5093Match{
		call: chain, kept: kept, paths: paths, clone: clone, cloneOK: cloneOK,
		nested: nested, nestedOK: nestedOK, methodName: methodName,
		constructor: chain.constructor.Name(),
	}, true
}
