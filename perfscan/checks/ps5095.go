package checks

import (
	"fmt"
	"go/ast"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS5095 removes io.NopCloser adapters whose identity cannot escape because a
// terminal Read or Close method is invoked immediately.
var PS5095 = register(&lint.Check{
	ID:       "PS5095",
	Category: "indirect",
	Slug:     "nopcloser-terminal-method-chain",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "io.NopCloser wrappers are constructed only for an immediate Read or Close",
		Text: `io.NopCloser adds a no-op Close method to an io.Reader. It does not
change Read: its adapter forwards the call to the original reader. Constructing
one or several wrappers only to invoke Read immediately therefore adds escaping
interface adapters and delegated frames with no behavior:

  io.NopCloser(reader).Read(buf)                       -> reader.Read(buf)
  io.NopCloser(io.NopCloser(reader)).Read(buf)         -> reader.Read(buf)

For an immediate Close, one NopCloser must remain to supply the deliberate
no-op Close result, but every outer NopCloser is redundant:

  io.NopCloser(io.NopCloser(reader)).Close() -> io.NopCloser(reader).Close()

The shared typed method-on-constructor machinery resolves the terminal
io.ReadCloser method and every io.NopCloser package function through go/types.
Aliases and parenthesized expressions work; dot imports, function-valued
constructors, user lookalikes, wrong arities, and ellipsis calls stay untouched.

Read removal preserves the exact receiver-before-buffer evaluation order, calls
the same dynamic Reader.Read method once with the same []byte, and returns the
same int/error pair. Typed-nil readers and nil Reader interfaces fault at the
same terminal invocation after argument evaluation. NopCloser's conditional
WriterTo forwarding is irrelevant because the terminal method is Read. The
replacement parenthesizes the retained reader expression, so selectors remain
valid for calls, assertions, dereferences, and other non-identifier operands.

Close reduction retains the innermost adapter, hence the same typed nil error,
no-op behavior, and guarantee that an underlying io.Closer is NOT closed.
Wrapper identity cannot escape from either terminal call.

Standalone nested NopCloser values are deliberately excluded. Although their
Read/Close method sets agree, nested adapter structure can be observed through
interface equality, reflection, or diagnostic formatting. APIs that store a
ReadCloser are excluded for the same reason. PS5076 separately owns NopCloser
passed to io.ReadAll/Copy/CopyBuffer, where Close is never observed at all.

Comments in removed scaffolding keep the finding advisory. Import and local-use
liveness are handled by the shared editor, including files where the wrappers
were the final io reference. An arbitrarily deep terminal chain reaches its
minimal form in one -fix pass.`,
		Before: `n, err := io.NopCloser(io.NopCloser(reader)).Read(buf)
closeErr := io.NopCloser(io.NopCloser(reader)).Close()`,
		After: `n, err := reader.Read(buf)
closeErr := io.NopCloser(reader).Close()`,
		MeasuredWin: `On an Apple M2 Pro, benchmarks/ps5095_test.go reduced an
immediate Read through three NopCloser layers from a median 48.79 ns/op,
48 B/op, and 3 allocations to 1.921 ns/op with zero bytes and zero allocations:
about 25.4x faster, removing every adapter allocation and delegated Read frame.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5095",
		Doc:  "io.NopCloser adapters are constructed only to call Read or Close immediately",
		Run:  runPS5095,
	},
})

type ps5095Match struct {
	outer   *ast.CallExpr
	first   *ast.CallExpr
	keep    ast.Expr
	base    ast.Expr
	calls   []*ast.CallExpr
	paths   []string
	method  string
	removed int
}

func runPS5095(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		covered := map[*ast.CallExpr]bool{}
		ast.Inspect(file, func(node ast.Node) bool {
			outer, ok := node.(*ast.CallExpr)
			if !ok || covered[outer] {
				return true
			}
			match, ok := ps5095TerminalMatch(pass, outer)
			if !ok {
				return true
			}
			for _, call := range match.calls {
				covered[call] = true
			}
			diagnostic := analysis.Diagnostic{
				Pos:     match.first.Pos(),
				End:     match.first.End(),
				Message: fmt.Sprintf("io.NopCloser chain constructs %d adapter layer(s) only to call %s immediately; %d layer(s) add no observable behavior", len(match.calls), match.method, match.removed),
			}
			var fix analysis.SuggestedFix
			if match.method == "Read" {
				fix, ok = fixReplacedCallScaffoldingPaths(pass, file, match.paths, "call Read on the underlying reader directly",
					analysis.TextEdit{Pos: match.first.Pos(), End: match.base.Pos(), NewText: []byte("(")},
					analysis.TextEdit{Pos: match.base.End(), End: match.first.End(), NewText: []byte(")")},
				)
			} else {
				fix, ok = fixDeletedCallScaffoldingPaths(pass, file, match.paths, "remove redundant outer NopCloser adapters",
					tokenSpan{start: match.first.Pos(), end: match.keep.Pos()},
					tokenSpan{start: match.keep.End(), end: match.first.End()},
				)
			}
			if ok {
				diagnostic.SuggestedFixes = []analysis.SuggestedFix{fix}
			}
			pass.Report(diagnostic)
			return true
		})
	}
	return nil, nil
}

func ps5095TerminalMatch(pass *analysis.Pass, outer *ast.CallExpr) (ps5095Match, bool) {
	chain, ok := matchTypedMethodOnPackageConstructor(pass, outer)
	if !ok || chain.constructor.Pkg().Path() != "io" || chain.constructor.Name() != "NopCloser" ||
		chain.method.Pkg() == nil || chain.method.Pkg().Path() != "io" {
		return ps5095Match{}, false
	}
	method := chain.method.Name()
	if method == "Read" {
		if len(outer.Args) != 1 || outer.Ellipsis.IsValid() {
			return ps5095Match{}, false
		}
	} else if method == "Close" {
		if len(outer.Args) != 0 || outer.Ellipsis.IsValid() {
			return ps5095Match{}, false
		}
	} else {
		return ps5095Match{}, false
	}

	first := chain.constructorCall
	last := first
	calls := []*ast.CallExpr{first}
	paths := []string{"io"}
	for {
		inner, nested := ps2110Unparen(last.Args[0]).(*ast.CallExpr)
		if !nested || !ps5076NopCloser(pass, inner) {
			break
		}
		last = inner
		calls = append(calls, inner)
		paths = append(paths, "io")
	}
	if method == "Close" && len(calls) < 2 {
		return ps5095Match{}, false
	}
	keep, base, removed := ast.Expr(last), last.Args[0], len(calls)
	if method == "Close" {
		removed = len(calls) - 1
	}
	return ps5095Match{
		outer: outer, first: first, keep: keep, base: base, calls: calls,
		paths: paths, method: method, removed: removed,
	}, true
}
