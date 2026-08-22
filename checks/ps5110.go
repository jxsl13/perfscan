package checks

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS5110 flattens semantically transparent nested slices.Concat calls. Its
// snapshot-stability guard rejects later calls that could mutate an inner
// input after the original inner Concat copied it.
var PS5110 = register(&lint.Check{
	ID:       "PS5110",
	Category: "alloc",
	Slug:     "nested-slices-concat-tree",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "nested slices.Concat calls allocate and recopy intermediate slices",
		Text: `slices.Concat already accepts any number of slices. Nesting it
materializes an intermediate result that the parent immediately copies again:

  slices.Concat(slices.Concat(a, b), c)
  -> slices.Concat(a, b, c)

  slices.Concat(slices.Concat(a, b), slices.Concat(c, d))
  -> slices.Concat(a, b, c, d)

PS5110 uses the shared typed variadic-call tree abstraction to flatten every
safe branch in one fix. Each removed call eliminates one result allocation,
one length pass, and one copy of its complete subtree. Callees must resolve
through go/types to slices.Concat through the same ordinary package binding.
Every nested call and every argument spliced out of it must have exactly the
same concrete slice type as the root result; this keeps generic inference,
named result types, and interface dynamic types unchanged. Aliases,
parentheses, explicit matching instantiations, named slices, and nesting in any
argument work. Dot imports, function values, user lookalikes, empty/spread
nested calls, untyped nil operands, and mixed generic result types stay silent.

There is a second safety condition beyond associativity. In:

  slices.Concat(slices.Concat(a), mutateAndReturnTail())

the inner call snapshots a before the later call can mutate it. Flattening
would postpone that copy until after the mutation and can change the result.
Accordingly, an earlier nested call is flattened only when every later sibling
expression before its parent call contains no arbitrary call, append/copy-like
builtin, or channel receive. Pure conversions, len/cap, and recursively checked
slices.Concat/Clone expressions are allowed. A nested call in the final
argument needs no restriction because no evaluation crosses its snapshot.
This guard is applied independently at every tree level.

Within that accepted domain the rewrite is BIT-IDENTICAL. Leaf expressions
retain depth-first left-to-right evaluation and are evaluated once. Concat is a
shallow ordered copy, so replacing intermediate copies with the identical leaf
sequence preserves every element and alias held by an element. Both forms
return nil for an empty concatenation and one independent final slice of the
same named type, length, capacity class, and element order otherwise. Nonempty
length overflow reaches the same slices.Concat panic; later expressions that
could observe a moved snapshot boundary have already been excluded.

The fix deletes only nested callee/parenthesis scaffolding and retains every
slice expression byte-for-byte. Comments, import liveness, and local constants
inside removed generic syntax are checked by the shared editor; an unsafe
deletion remains diagnostic-only. A safe branched tree reaches one Concat call
in one -fix pass.`,
		Before: `combined := slices.Concat(
	slices.Concat(first, second),
	slices.Concat(third, fourth),
)`,
		After: `combined := slices.Concat(
	first, second,
	third, fourth,
)`,
		MeasuredWin: `On an Apple M2 Pro, benchmarks/ps5110_test.go (10 runs,
single CPU) built the same 64 KiB result from four 16 KiB byte slices. Three
nested Concat calls measured a median 10,174 ns/op, 131,072 B/op, and 3
allocations versus 4,630 ns/op, 65,536 B/op, and 1 allocation for one flat
call: about 2.20x faster while removing both 32 KiB intermediate results and
their recopies.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5110",
		Doc:  "nested slices.Concat calls can be flattened without intermediate allocations",
		Run:  runPS5110,
	},
})

type ps5110Match struct {
	root   *ast.CallExpr
	nested []typedNestedPackageCall
}

func runPS5110(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		covered := make(map[*ast.CallExpr]bool)
		ast.Inspect(file, func(node ast.Node) bool {
			root, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			if covered[root] {
				return true
			}
			match, ok := ps5110ConcatTree(pass, root)
			if !ok {
				return true
			}
			for _, nested := range match.nested {
				covered[nested.call] = true
			}
			diagnostic := analysis.Diagnostic{
				Pos:     match.root.Pos(),
				End:     match.root.End(),
				Message: fmt.Sprintf("slices.Concat materializes and recopies %d intermediate concatenation result(s); flatten the snapshot-safe tree into one variadic call", len(match.nested)),
			}
			if fix, ok := fixReplacedCallScaffoldingPaths(pass, file, []string{"slices"}, "flatten the nested slices.Concat tree", flattenNestedPackageCallEdits(match.nested)...); ok {
				diagnostic.SuggestedFixes = []analysis.SuggestedFix{fix}
			}
			pass.Report(diagnostic)
			return true
		})
	}
	return nil, nil
}

func ps5110ConcatTree(pass *analysis.Pass, root *ast.CallExpr) (ps5110Match, bool) {
	if root == nil || len(root.Args) == 0 || root.Ellipsis.IsValid() {
		return ps5110Match{}, false
	}
	fn, signature, ok := typedCallee(pass, root.Fun)
	if !ok || signature.Recv() != nil || fn.Pkg() == nil || fn.Pkg().Path() != "slices" || fn.Name() != "Concat" {
		return ps5110Match{}, false
	}
	binding, ok := typedPackageBinding(pass, root.Fun)
	if !ok {
		return ps5110Match{}, false
	}
	resultType := pass.TypesInfo.TypeOf(root)
	if resultType == nil {
		return ps5110Match{}, false
	}
	accept := func(parent *ast.CallExpr, argument int, nested *ast.CallExpr) bool {
		if nestedType := pass.TypesInfo.TypeOf(nested); nestedType == nil || !types.Identical(nestedType, resultType) {
			return false
		}
		for _, operand := range nested.Args {
			operandType := pass.TypesInfo.TypeOf(operand)
			if operandType == nil || !types.Identical(operandType, resultType) {
				return false
			}
		}
		return ps5110LaterArgumentsStable(pass, parent, argument)
	}
	var nested []typedNestedPackageCall
	collectTypedNestedPackageCallTreeMatching(pass, root, "slices", "Concat", binding, accept, &nested)
	if len(nested) == 0 {
		return ps5110Match{}, false
	}
	return ps5110Match{root: root, nested: nested}, true
}

func ps5110LaterArgumentsStable(pass *analysis.Pass, parent *ast.CallExpr, nestedArgument int) bool {
	for index := nestedArgument + 1; index < len(parent.Args); index++ {
		if !ps5110SnapshotStable(pass, parent.Args[index]) {
			return false
		}
	}
	return true
}

func ps5110SnapshotStable(pass *analysis.Pass, expression ast.Expr) bool {
	stable := true
	ast.Inspect(expression, func(node ast.Node) bool {
		if !stable || node == nil {
			return stable
		}
		switch value := node.(type) {
		case *ast.UnaryExpr:
			if value.Op == token.ARROW {
				stable = false
				return false
			}
		case *ast.CallExpr:
			if typed, ok := pass.TypesInfo.Types[ps2110Unparen(value.Fun)]; ok && typed.IsType() {
				return true
			}
			if typedBuiltinName(pass, value.Fun, "len") || typedBuiltinName(pass, value.Fun, "cap") {
				return true
			}
			fn, signature, ok := typedCallee(pass, value.Fun)
			if ok && signature.Recv() == nil && fn.Pkg() != nil && fn.Pkg().Path() == "slices" &&
				(fn.Name() == "Concat" || fn.Name() == "Clone") {
				return true
			}
			stable = false
			return false
		}
		return true
	})
	return stable
}
