package checks

import (
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/config"
	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS6006 implements the conservative source-level part of owner issue #757:
// a repeated leaf benchmark exercises a configured production selector or
// default toggle, making it tempting to promote a leaf win without resident
// integration evidence.
var PS6006 = register(&lint.Check{
	ID:          "PS6006",
	Category:    "verify",
	Slug:        "selector-promotion-leaf-benchmark",
	Level:       lint.LevelStructured,
	AutoFix:     false,
	NeedsConfig: true,
	Vocab:       []string{"selectorPromotionSymbols"},
	Doc: lint.Documentation{
		Title: "a production selector or default toggle appears in a repeated leaf benchmark",
		Text: `A leaf dispatch benchmark can establish that one candidate wins its
isolated call shape. It cannot by itself establish that promoting the candidate
through a production selector or default improves the resident application.
Warm state, allocator pressure, command-buffer boundaries, synchronization,
shape mix, and neighboring kernels can reverse a leaf result.

This check implements the conservative source-level review boundary from owner
issue #757. Configure selectorPromotionSymbols with production fast-path
selectors, shipping default toggles, or selected kernel entry points. PS6006
reports a configured package-level or imported symbol when a real
func BenchmarkX(*testing.B) contains an explicit for/range repetition loop and
references that symbol. Type information rejects local shadows, fields, and
same-spelled unrelated declarations. Nested function literals are excluded so
an unrelated helper closure does not manufacture benchmark evidence.

A finding does not claim that integration evidence is absent; source cannot
prove what an external experiment recorded. It marks the exact promotion-
bearing leaf benchmark that requires review. Before changing the selector or
default, add an order-alternating resident integration benchmark, verify output
parity, and gate allocations and synchronization. Suppress the finding only
when the review can point to that external evidence.

There is NO automatic fix. Removing the benchmark, changing the production
selector, or inventing an integration harness would each change intent and
cannot be derived mechanically.`,
		Before: `var useSplitK = false

func BenchmarkSplitKDispatch(b *testing.B) {
    useSplitK = true
    for i := 0; i < b.N; i++ {
        dispatchAttention(input)
    }
}`,
		After: `// Keep the leaf benchmark, then validate the promotion separately:
// alternate incumbent/candidate order in a resident decoder benchmark,
// require output parity, and record allocations plus synchronization.
// Only then change useSplitK (or document the external evidence for suppression).`,
		MeasuredWin: `On the Metal split-K investigation behind issue #757, repeated
leaf dispatch favored the candidate by 1.139x at sk=1024 and 1.104x at sk=2048,
while the resident decoder measured only 0.997x and 0.986x respectively. The
leaf promotion signal therefore reversed into a resident regression.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6006",
		Doc:  "production selector promotion is backed by a repeated leaf benchmark",
		Run:  runPS6006,
	},
})

func runPS6006(pass *analysis.Pass) (any, error) {
	symbols := config.Current().SelectorPromotionSymbols
	if len(symbols) == 0 {
		return nil, nil
	}
	production := ps6006ProductionObjects(pass, symbols)
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !ps6006Benchmark(pass, fn) || !ps6006HasLoop(fn.Body) {
				continue
			}
			if id := ps6006SelectorUse(pass, fn.Body, symbols, production); id != nil {
				pass.Reportf(id.Pos(), "configured production selector/default %s appears in a repeated leaf benchmark; require order-alternating resident integration evidence with output parity and allocation/synchronization gates before promotion (suppress only with external evidence)", id.Name)
			}
		}
	}
	return nil, nil
}

func ps6006Benchmark(pass *analysis.Pass, fn *ast.FuncDecl) bool {
	if fn.Recv != nil || !strings.HasPrefix(fn.Name.Name, "Benchmark") {
		return false
	}
	obj, ok := pass.TypesInfo.Defs[fn.Name].(*types.Func)
	if !ok {
		return false
	}
	sig, ok := obj.Type().(*types.Signature)
	if !ok || sig.Params().Len() != 1 || sig.Results().Len() != 0 || sig.Variadic() {
		return false
	}
	ptr, ok := types.Unalias(sig.Params().At(0).Type()).(*types.Pointer)
	if !ok {
		return false
	}
	named, ok := types.Unalias(ptr.Elem()).(*types.Named)
	return ok && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == "testing" && named.Obj().Name() == "B"
}

func ps6006HasLoop(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(node ast.Node) bool {
		if found {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		switch node.(type) {
		case *ast.ForStmt, *ast.RangeStmt:
			found = true
			return false
		default:
			return true
		}
	})
	return found
}

func ps6006ProductionObjects(pass *analysis.Pass, symbols map[string]bool) map[types.Object]bool {
	objects := make(map[types.Object]bool)
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			switch n := decl.(type) {
			case *ast.FuncDecl:
				if symbols[n.Name.Name] {
					objects[pass.TypesInfo.Defs[n.Name]] = true
				}
			case *ast.GenDecl:
				for _, spec := range n.Specs {
					value, ok := spec.(*ast.ValueSpec)
					if !ok {
						continue
					}
					for _, name := range value.Names {
						if symbols[name.Name] {
							objects[pass.TypesInfo.Defs[name]] = true
						}
					}
				}
			}
		}
	}
	delete(objects, nil)
	return objects
}

func ps6006SelectorUse(pass *analysis.Pass, body *ast.BlockStmt, symbols map[string]bool, production map[types.Object]bool) *ast.Ident {
	var found *ast.Ident
	ast.Inspect(body, func(node ast.Node) bool {
		if found != nil {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		id, ok := node.(*ast.Ident)
		if !ok || !symbols[id.Name] {
			return true
		}
		obj := pass.TypesInfo.Uses[id]
		if production[obj] || (obj != nil && obj.Pkg() != nil && obj.Pkg() != pass.Pkg) {
			found = id
			return false
		}
		return true
	})
	return found
}
