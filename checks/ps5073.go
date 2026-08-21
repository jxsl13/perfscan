package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS5073 reports log/slog calls that route already-constructed Attr values
// through a ...any API instead of the corresponding Attr-only fast path.
var PS5073 = register(&lint.Check{
	ID:       "PS5073",
	Category: "alloc",
	Slug:     "slog-attr-only-fast-path",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "slog.Log, Logger.Log, or slog.Group receives only Attr values; the Attrs variant avoids ...any conversion",
		Text: `log/slog exposes Attr-only variants for call sites that have already
constructed every attribute:

  slog.Log(ctx, level, msg, attrs...) -> slog.LogAttrs(ctx, level, msg, attrs...)
  logger.Log(ctx, level, msg, attrs...) -> logger.LogAttrs(ctx, level, msg, attrs...)
  slog.Group(key, attrs...) -> slog.GroupAttrs(key, attrs...)

Here attrs... denotes one or more individual expressions whose static type is
exactly slog.Attr. (A []slog.Attr cannot be expanded into the original ...any
APIs.) The standard library documents Logger.LogAttrs and GroupAttrs as their
more efficient Attr-only counterparts. They bypass interface boxing and the
runtime argument-to-Attr classification performed by the ...any forms.

The rewrite is BIT-IDENTICAL for this exact input domain. Logger.Log defines an
Attr argument to be used as-is, so a list containing only exact Attr values is
the same list passed by Logger.LogAttrs. Both logger paths perform the same nil
context normalization, Enabled check, caller-PC skip, Record construction,
empty-group filtering, and handler call. Group converts each exact Attr as-is
before GroupValue; GroupAttrs passes the identical values directly. Argument
expressions retain their source order and are still evaluated exactly once.

The check resolves callees through go/types. Package aliases work, while
shadowed functions and same-named user APIs do not match. It requires at least
one variadic argument and requires every such argument to have the exact named
type log/slog.Attr (including true type aliases). Defined wrapper types, mixed
key/value arguments, interface-typed values, and ellipsis calls are rejected.
For Logger.Log, the receiver's static type must be exactly slog.Logger or
*slog.Logger, preventing an embedded wrapper's own LogAttrs method from
intercepting the replacement.

The fix changes only the resolved selector name, preserving receivers,
arguments, comments, evaluation order, and imports byte-for-byte.`,
		Before: `logger.Log(ctx, slog.LevelInfo, "ready",
    slog.String("service", service),
    slog.Int("port", port),
)`,
		After: `logger.LogAttrs(ctx, slog.LevelInfo, "ready",
    slog.String("service", service),
    slog.Int("port", port),
)`,
		MeasuredWin: `BenchmarkPS5073 Logger.Log with four prebuilt Attr values
(Apple M2 Pro, go1.26; five runs): Logger.Log median 251.4 ns/op, 192 B/op,
4 allocs/op -> Logger.LogAttrs median 169.4 ns/op, 0 B/op, 0 allocs/op
(~1.48x, -32.6%). The Attr-only path removes four interface-box allocations
and the runtime argument-to-Attr classification loop.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5073",
		Doc:  "slog ...any API used with only exact Attr values instead of its Attrs fast path",
		Run:  runPS5073,
	},
})

type ps5073Variant struct {
	name        string
	replacement string
	kind        typedCallKind
	fixedArgs   int
	receiver    string
}

var ps5073Variants = []ps5073Variant{
	{name: "Log", replacement: "LogAttrs", kind: typedPackageFunc, fixedArgs: 3},
	{name: "Log", replacement: "LogAttrs", kind: typedMethod, fixedArgs: 3, receiver: "Logger"},
	{name: "Group", replacement: "GroupAttrs", kind: typedPackageFunc, fixedArgs: 1},
}

func runPS5073(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			variant, selector, ok := ps5073Match(pass, call)
			if !ok {
				return true
			}
			pass.Report(analysis.Diagnostic{
				Pos:     call.Pos(),
				End:     call.End(),
				Message: "log/slog " + variant.name + " receives only slog.Attr values; " + variant.replacement + " avoids ...any boxing and argument classification",
				SuggestedFixes: []analysis.SuggestedFix{{
					Message: "use the Attr-only slog fast path",
					TextEdits: []analysis.TextEdit{{
						Pos:     selector.Sel.Pos(),
						End:     selector.Sel.End(),
						NewText: []byte(variant.replacement),
					}},
				}},
			})
			return true
		})
	}
	return nil, nil
}

func ps5073Match(pass *analysis.Pass, call *ast.CallExpr) (ps5073Variant, *ast.SelectorExpr, bool) {
	if call.Ellipsis.IsValid() {
		return ps5073Variant{}, nil, false
	}
	selector, ok := ps2110Unparen(call.Fun).(*ast.SelectorExpr)
	if !ok {
		return ps5073Variant{}, nil, false
	}
	fn, sig, ok := typedCallee(pass, selector)
	if !ok || fn.Pkg() == nil || fn.Pkg().Path() != "log/slog" {
		return ps5073Variant{}, nil, false
	}
	for _, variant := range ps5073Variants {
		if fn.Name() != variant.name || (variant.kind == typedPackageFunc) != (sig.Recv() == nil) ||
			len(call.Args) <= variant.fixedArgs {
			continue
		}
		if variant.receiver != "" && !ps5073ExactSlogReceiver(pass.TypesInfo.TypeOf(selector.X), variant.receiver) {
			continue
		}
		allAttrs := true
		for _, arg := range call.Args[variant.fixedArgs:] {
			if !ps5073ExactSlogNamed(pass.TypesInfo.TypeOf(arg), "Attr") {
				allAttrs = false
				break
			}
		}
		if allAttrs {
			return variant, selector, true
		}
	}
	return ps5073Variant{}, nil, false
}

func ps5073ExactSlogReceiver(t types.Type, name string) bool {
	if t == nil {
		return false
	}
	t = types.Unalias(t)
	if ptr, ok := t.(*types.Pointer); ok {
		t = types.Unalias(ptr.Elem())
	}
	return ps5073ExactSlogNamed(t, name)
}

func ps5073ExactSlogNamed(t types.Type, name string) bool {
	if t == nil {
		return false
	}
	t = types.Unalias(t)
	named, ok := t.(*types.Named)
	return ok && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == "log/slog" && named.Obj().Name() == name
}
