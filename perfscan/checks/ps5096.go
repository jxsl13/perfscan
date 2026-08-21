package checks

import (
	"fmt"
	"go/ast"
	"go/types"
	"go/version"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS5096 collapses nested io.LimitReader adapters when their only observable
// operation is one immediate Read. One adapter remains with the minimum limit.
var PS5096 = register(&lint.Check{
	ID:       "PS5096",
	Category: "indirect",
	Slug:     "nested-limitreader-terminal-read-chain",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "nested io.LimitReader adapters are constructed only for one immediate Read",
		Text: `Every nested io.LimitReader layer independently truncates the buffer
passed to the next reader. For one immediate Read, the resulting bound is just
the minimum of all layer limits:

  io.LimitReader(io.LimitReader(r, inner), outer).Read(p)
  -> io.LimitReader(r, min(inner, outer)).Read(p)

The rule collapses an arbitrarily deep chain to one adapter in a single fix.
The shared typed method-on-constructor abstraction pins the terminal io.Reader
Read method and every io.LimitReader package function through go/types. All
layers must use the same ordinary package binding. Aliases and parenthesized
expressions work; dot imports, function-valued constructors, user lookalikes,
wrong methods, ellipsis calls, and a single required LimitReader stay untouched.

The rewrite is BIT-IDENTICAL. Go evaluates the original nested constructors in
base-reader, innermost-limit through outermost-limit order, followed by the Read
buffer. The generated min call keeps exactly that order and evaluates every
expression once. If any limit is non-positive, both forms return io.EOF without
calling the underlying reader. Otherwise every layer successively truncates p,
so the underlying Read receives the same slice as one layer bounded by the
minimum. Its int/error result is forwarded unchanged. The remaining counter and
discarded adapter identities cannot escape from the terminal call.

Standalone nested LimitReader values are deliberately excluded: callers can
observe and mutate their *io.LimitedReader structure, and reader-consuming APIs
may pass the outer reader to user-defined io.ReaderFrom implementations. This
rule therefore requires the exact immediate Read boundary instead of assuming
that any io.Reader consumer hides adapter identity.

The fix requires Go 1.21's predeclared min builtin and withholds itself when min
is shadowed or the effective file language version is older. Comments in
rewritten scaffolding also keep the finding advisory. Limit expressions survive
byte-for-byte, and the retained outer io qualifier keeps its import live.
Multiple layers reach the minimal one-adapter form in one -fix pass.`,
		Before: `n, err := io.LimitReader(
	io.LimitReader(reader, innerLimit),
	outerLimit,
).Read(buf)`,
		After: `n, err := io.LimitReader(
	reader,
	min(innerLimit, outerLimit),
).Read(buf)`,
		MeasuredWin: `On an Apple M2 Pro, benchmarks/ps5096_test.go reduced an
immediate Read through three LimitReader layers from a median 32.68 ns/op,
48 B/op, and 2 allocations to 3.203 ns/op with zero bytes and zero allocations:
about 10.2x faster, removing both escaping adapters and delegated Read frames.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5096",
		Doc:  "nested io.LimitReader adapters are constructed only to call Read immediately",
		Run:  runPS5096,
	},
})

type ps5096Match struct {
	root   *ast.CallExpr
	first  *ast.CallExpr
	base   ast.Expr
	calls  []*ast.CallExpr
	limits []ast.Expr
}

func runPS5096(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		ast.Inspect(file, func(node ast.Node) bool {
			root, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			match, ok := ps5096TerminalMatch(pass, root)
			if !ok {
				return true
			}
			diagnostic := analysis.Diagnostic{
				Pos:     match.first.Pos(),
				End:     match.first.End(),
				Message: fmt.Sprintf("%d nested io.LimitReader layers only bound one immediate Read; collapse them to one layer with the minimum limit", len(match.calls)),
			}
			if ps5096MinAvailable(pass, file, match.first) {
				if fix, ok := ps5096Fix(pass, file, match); ok {
					diagnostic.SuggestedFixes = []analysis.SuggestedFix{fix}
				}
			}
			pass.Report(diagnostic)
			// The complete constructor chain belongs to this one diagnostic.
			return false
		})
	}
	return nil, nil
}

func ps5096TerminalMatch(pass *analysis.Pass, root *ast.CallExpr) (ps5096Match, bool) {
	chain, ok := matchTypedMethodOnPackageConstructor(pass, root)
	if !ok || chain.constructor.Pkg().Path() != "io" || chain.constructor.Name() != "LimitReader" ||
		chain.method.Pkg() == nil || chain.method.Pkg().Path() != "io" || chain.method.Name() != "Read" ||
		len(root.Args) != 1 || root.Ellipsis.IsValid() {
		return ps5096Match{}, false
	}
	binding, ok := typedPackageBinding(pass, chain.constructorCall.Fun)
	if !ok {
		return ps5096Match{}, false
	}

	current := chain.constructorCall
	var calls []*ast.CallExpr
	for ps5096LimitReader(pass, current, binding) {
		calls = append(calls, current)
		inner, nested := ps2110Unparen(current.Args[0]).(*ast.CallExpr)
		if !nested || !ps5096LimitReader(pass, inner, binding) {
			break
		}
		current = inner
	}
	if len(calls) < 2 {
		return ps5096Match{}, false
	}

	limits := make([]ast.Expr, 0, len(calls))
	for index := len(calls) - 1; index >= 0; index-- {
		limits = append(limits, calls[index].Args[1])
	}
	return ps5096Match{
		root: root, first: calls[0], base: calls[len(calls)-1].Args[0],
		calls: calls, limits: limits,
	}, true
}

func ps5096LimitReader(pass *analysis.Pass, call *ast.CallExpr, binding *types.PkgName) bool {
	if call == nil || len(call.Args) != 2 || call.Ellipsis.IsValid() {
		return false
	}
	fn, signature, ok := typedCallee(pass, call.Fun)
	if !ok || signature.Recv() != nil || fn.Pkg() == nil || fn.Pkg().Path() != "io" || fn.Name() != "LimitReader" {
		return false
	}
	callBinding, ok := typedPackageBinding(pass, call.Fun)
	return ok && callBinding == binding
}

func ps5096Fix(pass *analysis.Pass, file *ast.File, match ps5096Match) (analysis.SuggestedFix, bool) {
	edits := []analysis.TextEdit{
		{Pos: match.first.Args[0].Pos(), End: match.base.Pos()},
		{Pos: match.base.End(), End: match.limits[0].Pos(), NewText: []byte(", min(")},
	}
	for index := 0; index+1 < len(match.limits); index++ {
		edits = append(edits, analysis.TextEdit{
			Pos: match.limits[index].End(), End: match.limits[index+1].Pos(), NewText: []byte(", "),
		})
	}
	edits = append(edits, analysis.TextEdit{
		Pos: match.limits[len(match.limits)-1].End(), End: match.first.End(), NewText: []byte("))"),
	})
	return fixReplacedCallScaffoldingPaths(pass, file, []string{"io"}, "collapse nested LimitReader layers to one minimum bound", edits...)
}

// ps5096MinAvailable verifies both the language-version and lexical-binding
// requirements of the min call injected by the fix.
func ps5096MinAvailable(pass *analysis.Pass, file *ast.File, call *ast.CallExpr) bool {
	if !builtinInScope(pass, call.Pos(), "min") {
		return false
	}
	v := ""
	if pass.TypesInfo.FileVersions != nil {
		v = pass.TypesInfo.FileVersions[file]
	}
	if v == "" && pass.Pkg != nil {
		v = pass.Pkg.GoVersion()
	}
	if v == "" || version.Lang(v) == "" {
		return true
	}
	return version.Compare(version.Lang(v), "go1.21") >= 0
}
