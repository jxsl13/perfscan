package checks

import (
	"fmt"
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS5079 removes strings/bytes boundary operations whose companion prefix,
// suffix, or cutset is statically empty, collapsing heterogeneous chains in a
// single fix.
var PS5079 = register(&lint.Check{
	ID:       "PS5079",
	Category: "arith",
	Slug:     "empty-boundary-operation-chain",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "strings/bytes boundary operations use an empty prefix, suffix, or cutset",
		Text: `Trim, TrimLeft, and TrimRight with an empty cutset return their
input unchanged. TrimPrefix and TrimSuffix with an empty prefix/suffix do the
same. A mixed chain only adds calls and guards:

  strings.Trim(strings.TrimPrefix(s, ""), "") -> s
  bytes.TrimPrefix(bytes.TrimRight(b, ""), nil) -> b

This check covers strings.Trim/TrimLeft/TrimRight/TrimPrefix/TrimSuffix plus
bytes.TrimRight/TrimPrefix/TrimSuffix. bytes.Trim and bytes.TrimLeft are
deliberately excluded: for a non-nil empty input they historically return nil,
so replacing them with the input would change its observable slice header. The
rule collapses a maximal adjacent heterogeneous chain in one fix. String
companions must be compile-time empty strings. For bytes TrimPrefix/TrimSuffix,
only nil, []byte{}, and []byte("") are accepted; dynamic zero-length slices and
make calls are excluded so the rewrite never removes an evaluation with
unknown provenance.

The rewrite is BIT-IDENTICAL. Empty boundary sets cannot consume a byte or
rune. The original string is returned unchanged; bytes operations return the
same slice header, preserving nil state, backing pointer, length, and capacity.
The base expression is still evaluated exactly once, while all removed
companions are side-effect-free constants/literals.

Every layer is resolved through go/types and must use the same concrete stdlib
package binding. Aliases and mixed allowed function names work; shadowed
helpers, methods, dot imports, nonempty/dynamic companions, ellipsis calls, and
cross-package chains do not match. Comments and last-use local constants keep
the finding advisory. If the chain carries the file's last strings/bytes
qualifier, the fix safely removes that import too; cgo files or commented
import declarations stay advisory.

This is both a standalone no-op check and a multi-call abstraction: the matcher
walks by typed function family rather than requiring every layer to have the
same callee.`,
		Before:      `clean := strings.Trim(strings.TrimSuffix(payload, ""), "")`,
		After:       `clean := payload`,
		MeasuredWin: `On Apple M2 Pro, collapsing a five-call strings chain measured 2.929 ns/op -> 0.5441 ns/op (median of 5 runs): 5.38x faster, 81.4% less time, with 0 allocations in both versions.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5079",
		Doc:  "empty strings/bytes boundary operation or chain",
		Run:  runPS5079,
	},
})

var ps5079Functions = map[string]bool{
	"Trim": true, "TrimLeft": true, "TrimRight": true,
	"TrimPrefix": true, "TrimSuffix": true,
}

type ps5079Match struct {
	outer   *ast.CallExpr
	base    ast.Expr
	calls   []*ast.CallExpr
	pkgPath string
	spans   []tokenSpan
}

func runPS5079(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		covered := make(map[*ast.CallExpr]bool)
		ast.Inspect(file, func(node ast.Node) bool {
			outer, ok := node.(*ast.CallExpr)
			if !ok || covered[outer] {
				return true
			}
			matched, ok := ps5079MatchChain(pass, outer)
			if !ok {
				return true
			}
			diagnostic := analysis.Diagnostic{
				Pos:     outer.Pos(),
				End:     outer.End(),
				Message: fmt.Sprintf("%d adjacent %s boundary operation(s) use an empty prefix, suffix, or cutset and return the input unchanged", len(matched.calls), matched.pkgPath),
			}
			if fix, ok := ps5079Fix(pass, file, matched); ok {
				diagnostic.SuggestedFixes = []analysis.SuggestedFix{fix}
			}
			pass.Report(diagnostic)
			for _, call := range matched.calls {
				covered[call] = true
			}
			return true
		})
	}
	return nil, nil
}

func ps5079MatchChain(pass *analysis.Pass, outer *ast.CallExpr) (ps5079Match, bool) {
	fn, sig, ok := typedCallee(pass, outer.Fun)
	if !ok || sig.Recv() != nil || fn.Pkg() == nil ||
		(fn.Pkg().Path() != "strings" && fn.Pkg().Path() != "bytes") {
		return ps5079Match{}, false
	}
	binding, ok := typedPackageBinding(pass, outer.Fun)
	if !ok {
		return ps5079Match{}, false
	}
	pkgPath := fn.Pkg().Path()
	var calls []*ast.CallExpr
	current := outer
	for ps5079NoopCall(pass, current, pkgPath, binding) {
		calls = append(calls, current)
		next, ok := ps2110Unparen(current.Args[0]).(*ast.CallExpr)
		if !ok || !ps5079NoopCall(pass, next, pkgPath, binding) {
			base := current.Args[0]
			return ps5079Match{
				outer: outer, base: base, calls: calls, pkgPath: pkgPath,
				spans: []tokenSpan{
					{start: outer.Pos(), end: base.Pos()},
					{start: base.End(), end: outer.End()},
				},
			}, true
		}
		current = next
	}
	return ps5079Match{}, false
}

func ps5079NoopCall(pass *analysis.Pass, call *ast.CallExpr, pkgPath string, binding *types.PkgName) bool {
	if len(call.Args) != 2 || call.Ellipsis.IsValid() {
		return false
	}
	fn, sig, ok := typedCallee(pass, call.Fun)
	if !ok || sig.Recv() != nil || fn.Pkg() == nil || fn.Pkg().Path() != pkgPath || !ps5079Functions[fn.Name()] {
		return false
	}
	callBinding, ok := typedPackageBinding(pass, call.Fun)
	if !ok || callBinding != binding {
		return false
	}
	if pkgPath == "bytes" {
		switch fn.Name() {
		case "TrimPrefix", "TrimSuffix":
			return ps5079EmptyBytes(pass, call.Args[1])
		case "TrimRight":
			value, ok := ps5077Cutset(pass, call.Args[1])
			return ok && value == ""
		default:
			return false
		}
	}
	value, ok := ps5077Cutset(pass, call.Args[1])
	return ok && value == ""
}

func ps5079EmptyBytes(pass *analysis.Pass, expr ast.Expr) bool {
	expr = ps2110Unparen(expr)
	if id, ok := expr.(*ast.Ident); ok {
		_, isNil := pass.TypesInfo.Uses[id].(*types.Nil)
		return isNil
	}
	if !types.Identical(pass.TypesInfo.TypeOf(expr), types.NewSlice(types.Typ[types.Uint8])) {
		return false
	}
	switch n := expr.(type) {
	case *ast.CompositeLit:
		_, plain := ps2110Unparen(n.Type).(*ast.ArrayType)
		return plain && len(n.Elts) == 0
	case *ast.CallExpr:
		_, plain := ps2110Unparen(n.Fun).(*ast.ArrayType)
		if !plain || len(n.Args) != 1 || n.Ellipsis.IsValid() {
			return false
		}
		value, ok := ps5077Cutset(pass, n.Args[0])
		return ok && value == ""
	default:
		return false
	}
}

func ps5079Fix(pass *analysis.Pass, file *ast.File, matched ps5079Match) (analysis.SuggestedFix, bool) {
	return fixDeletedCallScaffolding(pass, file, matched.pkgPath, "remove empty boundary operations", matched.spans...)
}
