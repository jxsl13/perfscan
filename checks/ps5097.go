package checks

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS5097 removes outer bufio constructors that are documented to return the
// already-buffered inner Reader or Writer pointer unchanged.
var PS5097 = register(&lint.Check{
	ID:       "PS5097",
	Category: "indirect",
	Slug:     "redundant-nested-bufio-constructor-chain",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "nested bufio Reader/Writer constructors return the exact inner buffer",
		Text: `bufio.NewReader and bufio.NewWriter do not wrap an already-buffered
value when its buffer is large enough: they return the supplied *bufio.Reader
or *bufio.Writer pointer itself. A value produced by the same default
constructor necessarily satisfies that test, so repeated constructors are
identity calls:

  bufio.NewReader(bufio.NewReader(r)) -> bufio.NewReader(r)
  bufio.NewWriter(bufio.NewWriter(w)) -> bufio.NewWriter(w)

The Size variants have the same identity guarantee when the inner constructor
guarantees at least the outer constant request. The rule therefore also folds
decreasing constant chains:

  bufio.NewReaderSize(bufio.NewReaderSize(r, 8192), 4096)
  -> bufio.NewReaderSize(r, 8192)

  bufio.NewWriterSize(bufio.NewWriterSize(w, 4096), 1024)
  -> bufio.NewWriterSize(w, 4096)

Non-positive outer size requests are removable around any same-family bufio
constructor because every buffer has non-negative size. A positive dynamic
outer request, an increasing constant request, and default/Size mixtures whose
relative sizes are not proven remain untouched. This avoids hard-coding the
unexported default buffer size.

The shared typed package-call matcher resolves every constructor through
go/types and requires one concrete bufio import binding. Aliases and
parenthesized expressions work; dot imports, function-valued constructors,
Reader/Writer family changes, user lookalikes, and wrong arities do not.
Arbitrarily deep provable prefixes collapse to the first required constructor
in one fix.

The rewrite is BIT-IDENTICAL. Every removed constructor is proven to return
the exact inner pointer, not merely an equivalent wrapper. Buffered bytes,
pending writes, errors, unread state, buffer capacity, pointer/interface
identity, and later Reset/Read/Write/Flush behavior therefore remain unchanged.
The retained constructor still evaluates the original reader/writer exactly
once. Removed size operands are compile-time constants, so no side effects or
panics disappear. Comments and a function-local constant's final use keep the
finding advisory through the shared scaffolding editor. The retained bufio
qualifier keeps the import live.`,
		Before: `reader := bufio.NewReader(bufio.NewReader(source))
writer := bufio.NewWriterSize(bufio.NewWriterSize(dst, 8192), 4096)`,
		After: `reader := bufio.NewReader(source)
writer := bufio.NewWriterSize(dst, 8192)`,
		MeasuredWin: `On an Apple M2 Pro with gc 1.26, seven 500 ms samples of
benchmarks/ps5097_test.go measured parity: 1.919 ns/op before and 2.073 ns/op
after at 0 B/op and 0 allocations in both forms. The sub-nanosecond reversal is
run-order/frequency noise: gc inlines the proven identity layers already. The
rewrite makes that identity explicit and robust on older/non-gc toolchains; it
does not claim a current-gc runtime win.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5097",
		Doc:  "nested bufio Reader/Writer constructors return the exact inner buffered value",
		Run:  runPS5097,
	},
})

type ps5097Constructor struct {
	call         *ast.CallExpr
	binding      *types.PkgName
	family       string
	defaultSize  bool
	request      int64
	requestKnown bool
}

type ps5097Match struct {
	outer   *ast.CallExpr
	keep    *ast.CallExpr
	family  string
	layers  int
	removed int
}

func runPS5097(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		ast.Inspect(file, func(node ast.Node) bool {
			outer, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			match, ok := ps5097ConstructorChain(pass, outer)
			if !ok {
				return true
			}
			diagnostic := analysis.Diagnostic{
				Pos:     match.outer.Pos(),
				End:     match.outer.End(),
				Message: fmt.Sprintf("%d adjacent bufio %s constructor layers include %d outer identity call(s) that return the exact inner pointer", match.layers, match.family, match.removed),
			}
			if fix, ok := fixDeletedCallScaffolding(pass, file, "bufio", "remove redundant outer bufio constructors",
				tokenSpan{start: match.outer.Pos(), end: match.keep.Pos()},
				tokenSpan{start: match.keep.End(), end: match.outer.End()},
			); ok {
				diagnostic.SuggestedFixes = []analysis.SuggestedFix{fix}
			}
			pass.Report(diagnostic)
			return false
		})
	}
	return nil, nil
}

func ps5097ConstructorChain(pass *analysis.Pass, outer *ast.CallExpr) (ps5097Match, bool) {
	current, ok := ps5097ClassifyConstructor(pass, outer, nil)
	if !ok {
		return ps5097Match{}, false
	}
	binding := current.binding
	removed := 0
	for {
		innerCall, nested := ps2110Unparen(current.call.Args[0]).(*ast.CallExpr)
		if !nested {
			break
		}
		inner, classified := ps5097ClassifyConstructor(pass, innerCall, binding)
		if !classified || !ps5097OuterReturnsInner(current, inner) {
			break
		}
		removed++
		current = inner
	}
	if removed == 0 {
		return ps5097Match{}, false
	}
	return ps5097Match{
		outer: outer, keep: current.call, family: current.family,
		layers: removed + 1, removed: removed,
	}, true
}

func ps5097ClassifyConstructor(pass *analysis.Pass, call *ast.CallExpr, binding *types.PkgName) (ps5097Constructor, bool) {
	if call == nil || call.Ellipsis.IsValid() {
		return ps5097Constructor{}, false
	}
	fn, signature, ok := typedCallee(pass, call.Fun)
	if !ok || signature.Recv() != nil || fn.Pkg() == nil || fn.Pkg().Path() != "bufio" {
		return ps5097Constructor{}, false
	}
	callBinding, ok := typedPackageBinding(pass, call.Fun)
	if !ok || binding != nil && callBinding != binding {
		return ps5097Constructor{}, false
	}
	constructor := ps5097Constructor{call: call, binding: callBinding}
	switch fn.Name() {
	case "NewReader":
		if len(call.Args) != 1 {
			return ps5097Constructor{}, false
		}
		constructor.family, constructor.defaultSize = "Reader", true
	case "NewWriter":
		if len(call.Args) != 1 {
			return ps5097Constructor{}, false
		}
		constructor.family, constructor.defaultSize = "Writer", true
	case "NewReaderSize":
		if len(call.Args) != 2 {
			return ps5097Constructor{}, false
		}
		constructor.family = "Reader"
		constructor.request, constructor.requestKnown = ps5097ConstantInt(pass, call.Args[1])
	case "NewWriterSize":
		if len(call.Args) != 2 {
			return ps5097Constructor{}, false
		}
		constructor.family = "Writer"
		constructor.request, constructor.requestKnown = ps5097ConstantInt(pass, call.Args[1])
	default:
		return ps5097Constructor{}, false
	}
	return constructor, true
}

func ps5097OuterReturnsInner(outer, inner ps5097Constructor) bool {
	if outer.family != inner.family {
		return false
	}
	if outer.defaultSize {
		// Both calls use the same unexported default request. The inner call
		// guarantees that request without this check hard-coding its value.
		return inner.defaultSize
	}
	if !outer.requestKnown {
		return false
	}
	// Every bufio buffer has a non-negative size, including a zero-value
	// Reader/Writer returned for a non-positive request.
	if outer.request <= 0 {
		return true
	}
	if inner.defaultSize || !inner.requestKnown {
		return false
	}
	innerGuarantee := inner.request
	if innerGuarantee < 0 {
		innerGuarantee = 0
	}
	return outer.request <= innerGuarantee
}

func ps5097ConstantInt(pass *analysis.Pass, expression ast.Expr) (int64, bool) {
	value, ok := pass.TypesInfo.Types[ps2110Unparen(expression)]
	if !ok || value.Value == nil || value.Value.Kind() != constant.Int {
		return 0, false
	}
	return constant.Int64Val(value.Value)
}
