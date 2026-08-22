package checks

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS5076 reports io.NopCloser wrapped around a reader that is immediately
// consumed by an io operation which never calls Close.
var PS5076 = register(&lint.Check{
	ID:       "PS5076",
	Category: "indirect",
	Slug:     "nopcloser-inside-reader-only-io-call",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "io.NopCloser is immediately passed to io.ReadAll/Copy/CopyBuffer, where Close is never called",
		Text: `io.NopCloser adapts an io.Reader to io.ReadCloser by adding a
no-op Close method. That adapter is useful only when the resulting value reaches
an API that requires or stores a closer. io.ReadAll accepts a plain io.Reader,
does not expose the reader to another callback, and never calls Close, so an
immediately nested NopCloser adds only wrapper construction and another Read
delegation:

  io.ReadAll(io.NopCloser(r)) -> io.ReadAll(r)

Multiple nested NopCloser layers inside ReadAll are removed together.
Standalone NopCloser values, returns, assignments, and calls to other consumers
are deliberately left alone because wrapper identity or the Close method may
then be observed.

The ReadAll rewrite is BIT-IDENTICAL. NopCloser.Read forwards each call to the
underlying Reader unchanged, so ReadAll returns the same bytes and error, the
underlying reader sees the same operations, Close is never invoked in either
form, and the reader expression is still evaluated once in the same argument
position.

Copy and CopyBuffer are diagnostic-only. When the source lacks io.WriterTo and
the destination implements io.ReaderFrom, those functions pass the source to
dst.ReadFrom. The original call exposes the NopCloser wrapper while a direct
call exposes the underlying reader; ReaderFrom can observe io.Closer, another
dynamic type, or retain that object. Remove the wrapper manually only after
proving that fast-path boundary cannot observe or retain the source.

Both outer and inner functions are resolved through go/types, so aliases work
while shadowed functions, methods, and same-named user helpers do not match.
ReadAll must have one argument, Copy two, and CopyBuffer three; ellipsis calls
are rejected. The fix keeps the underlying reader expression byte-for-byte and
deletes only NopCloser scaffolding. A comment in deleted scaffolding withholds
the automatic fix.`,
		Before: `data, err := io.ReadAll(io.NopCloser(src))`,
		After:  `data, err := io.ReadAll(src)`,
		MeasuredWin: `Apple M2 Pro, go1.26, ten 300 ms samples over a 64 KiB
32-byte-chunk reader: the safe ReadAll rewrite was time-neutral at 32644 ->
32630 ns/op median while removing the escaping adapter (138136 -> 138120 B/op,
18 -> 17 allocs/op). The diagnostic-only Copy form measured 24911.5 -> 23836.5
ns/op median and likewise removed 16 B/one allocation, but is not fixed because
a destination ReaderFrom can observe or retain a different source object.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5076",
		Doc:  "io.NopCloser immediately consumed by a reader-only io operation",
		Run:  runPS5076,
	},
})

type ps5076Variant struct {
	name   string
	arity  int
	reader int
	fix    bool
}

var ps5076Variants = []ps5076Variant{
	{name: "ReadAll", arity: 1, reader: 0, fix: true},
	{name: "Copy", arity: 2, reader: 1},
	{name: "CopyBuffer", arity: 3, reader: 1},
}

func runPS5076(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		ast.Inspect(file, func(node ast.Node) bool {
			outer, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			variant, firstWrapper, reader, layers, ok := ps5076Match(pass, outer)
			if !ok {
				return true
			}
			wrappers := ps5076WrapperText(layers)
			message := "io." + variant.name + " never calls Close, but " + wrappers + " may still be observable"
			if variant.fix {
				message = "io.ReadAll observes only Read behavior; removing " + wrappers + " preserves behavior and avoids adapter/delegation work"
			} else {
				message += " through a destination ReaderFrom fast path; remove it only after proving that callback cannot inspect or retain the source"
			}
			diagnostic := analysis.Diagnostic{
				Pos:     firstWrapper.Pos(),
				End:     firstWrapper.End(),
				Message: message,
			}
			if variant.fix && !ps2111CommentIn(file, firstWrapper.Pos(), reader.Pos()) &&
				!ps2111CommentIn(file, reader.End(), firstWrapper.End()) {
				diagnostic.SuggestedFixes = []analysis.SuggestedFix{{
					Message: "pass the underlying reader directly",
					TextEdits: []analysis.TextEdit{
						{Pos: firstWrapper.Pos(), End: reader.Pos()},
						{Pos: reader.End(), End: firstWrapper.End()},
					},
				}}
			}
			pass.Report(diagnostic)
			return true
		})
	}
	return nil, nil
}

func ps5076Match(pass *analysis.Pass, outer *ast.CallExpr) (ps5076Variant, *ast.CallExpr, ast.Expr, int, bool) {
	for _, variant := range ps5076Variants {
		steps := []typedCallStep{
			{PkgPath: "io", Name: variant.name, Kind: typedPackageFunc, Arity: variant.arity, NextArg: variant.reader},
			{PkgPath: "io", Name: "NopCloser", Kind: typedPackageFunc, Arity: 1, NextArg: -1},
		}
		calls, ok := matchTypedCallChain(pass, outer, steps...)
		if !ok {
			continue
		}
		first := calls[1]
		last := first
		layers := 1
		for {
			inner, ok := ps2110Unparen(last.Args[0]).(*ast.CallExpr)
			if !ok || !ps5076NopCloser(pass, inner) {
				break
			}
			last = inner
			layers++
		}
		return variant, first, last.Args[0], layers, true
	}
	return ps5076Variant{}, nil, nil, 0, false
}

func ps5076NopCloser(pass *analysis.Pass, call *ast.CallExpr) bool {
	if len(call.Args) != 1 || call.Ellipsis.IsValid() {
		return false
	}
	fn, sig, ok := typedCallee(pass, call.Fun)
	return ok && sig.Recv() == nil && fn.Pkg() != nil && fn.Pkg().Path() == "io" && fn.Name() == "NopCloser"
}

func ps5076WrapperText(layers int) string {
	if layers == 1 {
		return "the immediately nested io.NopCloser wrapper"
	}
	return "all immediately nested io.NopCloser wrappers"
}
