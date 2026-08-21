package checks

import (
	"fmt"
	"go/ast"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS5098 flattens nested io.MultiReader/MultiWriter constructor trees when an
// immediate terminal method prevents any intermediate adapter from escaping.
var PS5098 = register(&lint.Check{
	ID:       "PS5098",
	Category: "alloc",
	Slug:     "terminal-nested-io-multi-constructor-tree",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "nested io.MultiReader/MultiWriter values are allocated only for one terminal operation",
		Text: `io.MultiReader and io.MultiWriter compose their inputs in source order.
Nesting those constructors before one immediate terminal operation allocates
intermediate adapter objects and argument slices without adding a boundary:

  io.MultiReader(io.MultiReader(a, b), c).Read(p)
  -> io.MultiReader(a, b, c).Read(p)

  io.MultiWriter(a, io.MultiWriter(b, c), d).Write(p)
  -> io.MultiWriter(a, b, c, d).Write(p)

The rule traverses the complete direct constructor-argument tree and splices
every non-empty nested constructor's arguments into the retained root in one
fix. The shared typed method-on-constructor abstraction pins io.MultiReader
with io.Reader.Read or io.MultiWriter with io.Writer.Write, and every nested
constructor is resolved through go/types against the same ordinary io package
binding. Aliases and parenthesized nested calls work; dot imports,
function-valued constructors, user lookalikes, ellipsis/empty nested calls,
wrong terminal methods, and cross-family nodes remain untouched.

The rewrite is BIT-IDENTICAL. Go evaluates the original reader/writer
expressions depth-first from left to right while building each adapter, then
evaluates the terminal byte slice. Removing only constructor punctuation keeps
that exact expression order and evaluates every operand once. MultiReader's
Read walks nested readers with the same EOF suppression, positive-n-with-EOF,
zero-progress, and non-EOF error rules as the flattened list. MultiWriter's
constructor already expands nested *multiWriter inputs; the source rewrite
feeds the identical ordered writer list to one constructor, preserving every
Write, short-write stop, error, count, and byte slice.

Standalone nested values and general reader/writer consumers are deliberately
excluded. Their adapter identity and internal slice shape can be observed via
reflection, and an API may retain the interface. At the exact Read/Write
boundary neither the root nor intermediate adapter can escape to user code.

The fix deletes only package-call scaffolding; all reader/writer and terminal
arguments remain byte-for-byte. A comment in removed punctuation keeps the
finding advisory. The root io qualifier stays, so import liveness is unchanged.
An arbitrarily branched tree reaches its flat one-constructor form in one
-fix pass.`,
		Before: `n, err := io.MultiWriter(
	io.MultiWriter(first, second),
	io.MultiWriter(third, fourth),
).Write(payload)`,
		After: `n, err := io.MultiWriter(
	first, second,
	third, fourth,
).Write(payload)`,
		MeasuredWin: `On an Apple M2 Pro, seven 300 ms samples of
benchmarks/ps5098_test.go reduced a terminal Write through a branched tree of
three MultiWriter constructors from a median 104.3 ns/op, 176 B/op, and 5
allocations to 13.05 ns/op with zero bytes and zero allocations: about 8.0x
faster, removing every intermediate adapter/slice allocation.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5098",
		Doc:  "nested io.MultiReader/MultiWriter constructor tree is used only for one immediate Read/Write",
		Run:  runPS5098,
	},
})

type ps5098Match struct {
	root        *ast.CallExpr
	constructor string
	method      string
	nested      []typedNestedPackageCall
}

func runPS5098(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		ast.Inspect(file, func(node ast.Node) bool {
			root, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			match, ok := ps5098TerminalTree(pass, root)
			if !ok {
				return true
			}
			diagnostic := analysis.Diagnostic{
				Pos:     match.root.Pos(),
				End:     match.root.End(),
				Message: fmt.Sprintf("io.%s tree allocates %d nested adapter(s) only to call %s once; flatten their ordered inputs into one constructor", match.constructor, len(match.nested), match.method),
			}
			edits := flattenNestedPackageCallEdits(match.nested)
			if fix, ok := fixReplacedCallScaffoldingPaths(pass, file, []string{"io"}, "flatten nested io multi-adapter constructors", edits...); ok {
				diagnostic.SuggestedFixes = []analysis.SuggestedFix{fix}
			}
			pass.Report(diagnostic)
			return false
		})
	}
	return nil, nil
}

func ps5098TerminalTree(pass *analysis.Pass, root *ast.CallExpr) (ps5098Match, bool) {
	chain, ok := matchTypedMethodOnPackageConstructor(pass, root)
	if !ok || len(root.Args) != 1 || root.Ellipsis.IsValid() || chain.constructorCall.Ellipsis.IsValid() ||
		chain.constructor.Pkg().Path() != "io" || chain.method.Pkg() == nil || chain.method.Pkg().Path() != "io" {
		return ps5098Match{}, false
	}
	constructor, method := chain.constructor.Name(), chain.method.Name()
	if constructor == "MultiReader" && method != "Read" || constructor == "MultiWriter" && method != "Write" ||
		constructor != "MultiReader" && constructor != "MultiWriter" {
		return ps5098Match{}, false
	}
	binding, ok := typedPackageBinding(pass, chain.constructorCall.Fun)
	if !ok {
		return ps5098Match{}, false
	}
	var nested []typedNestedPackageCall
	collectTypedNestedPackageCallTree(pass, chain.constructorCall.Args, "io", constructor, binding, &nested)
	if len(nested) == 0 {
		return ps5098Match{}, false
	}
	return ps5098Match{root: root, constructor: constructor, method: method, nested: nested}, true
}
