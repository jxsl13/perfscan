package checks

import (
	"fmt"
	"go/ast"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS5109 flattens only the first-argument spine of path.Join. Cleaned prefixes
// compose with later path elements; nested Joins in later positions do not.
var PS5109 = register(&lint.Check{
	ID:       "PS5109",
	Category: "alloc",
	Slug:     "nested-path-join-prefix-spine",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "path.Join repeatedly cleans and allocates the same completed prefix",
		Text: `A path.Join nested as the FIRST element of another path.Join builds
and cleans a completed prefix only for the outer call to copy and clean it
again:

  path.Join(path.Join(base, directory), file)
  -> path.Join(base, directory, file)

  path.Join(path.Join(path.Join(a, b), c), d)
  -> path.Join(a, b, c, d)

PS5109 follows the complete first-argument spine and removes every nested Join
in one fix. Empty and ellipsis calls are included only when they are the sole
argument of their parent, where splicing leaves a valid empty or spread call;
with later siblings they remain untouched. Each removed layer eliminates an
intermediate byte buffer, string materialization, and Clean pass.

The FIRST-ARGUMENT restriction is the safety proof, not a convenience. For a
slash-joined prefix x and remaining elements y, path.Clean's iterative lexical
rules give:

  Clean(Clean(x) + "/" + y) == Clean(x + "/" + y)

If every prefix element is empty, both Join forms ignore that prefix and return
the same remaining join. Otherwise expanding the cleaned first prefix therefore
preserves the exact result. Applying the identity repeatedly proves the whole
left spine. String-producing element expressions retain their depth-first
left-to-right evaluation order and are each evaluated once; Join and Clean are
pure lexical operations.

A Join in ANY LATER argument is deliberately untouched because the analogous
right-side identity is false. One concrete counterexample is:

  path.Join(".", path.Join("", "/../a")) == "a"
  path.Join(".", "", "/../a")            == "../a"

The inner call cleans a rooted path before the outer prefix is attached, so
flattening it changes how the root and leading .. are interpreted. Branched
trees consequently flatten only their safe first spine. path/filepath.Join is
also excluded: its volume, UNC, and separator rules vary by target platform.

The package-agnostic typed spine abstraction resolves every call through
go/types and requires one ordinary path import binding. Aliases and parentheses
work; dot imports, function values, user lookalikes, non-final empty/spread
nested calls, later-position nesting, and filepath.Join do not match. The fix
deletes only nested call punctuation, preserving every path element byte-for-byte.
Comments in removed scaffolding keep the finding advisory, while the retained
root keeps the path import live.`,
		Before: `name := path.Join(
	path.Join(root, category),
	file,
)`,
		After: `name := path.Join(
	root, category,
	file,
)`,
		MeasuredWin: `On an Apple M2 Pro, benchmarks/ps5109_test.go (10 runs,
single CPU) measured a four-element path built through three left-nested Joins
at a median 304.75 ns/op, 352 B/op, and 10 allocs/op, versus 149.85 ns/op,
240 B/op, and 4 allocs/op with one flat Join: about 2.03x faster, 112 fewer
bytes, and 6 fewer allocations per operation.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5109",
		Doc:  "path.Join has a nested first-prefix spine that can be joined and cleaned once",
		Run:  runPS5109,
	},
})

type ps5109Match struct {
	root   *ast.CallExpr
	nested []typedNestedPackageCall
}

func runPS5109(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		ast.Inspect(file, func(node ast.Node) bool {
			root, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			match, ok := ps5109PrefixSpine(pass, root)
			if !ok {
				return true
			}
			diagnostic := analysis.Diagnostic{
				Pos:     match.root.Pos(),
				End:     match.root.End(),
				Message: fmt.Sprintf("path.Join cleans and materializes %d completed prefix layer(s) only to join them again; flatten the first-argument spine and clean once", len(match.nested)),
			}
			if fix, ok := fixReplacedCallScaffoldingPaths(pass, file, []string{"path"}, "flatten the nested path.Join prefix spine", flattenNestedPackageCallEdits(match.nested)...); ok {
				diagnostic.SuggestedFixes = []analysis.SuggestedFix{fix}
			}
			pass.Report(diagnostic)
			return false
		})
	}
	return nil, nil
}

func ps5109PrefixSpine(pass *analysis.Pass, root *ast.CallExpr) (ps5109Match, bool) {
	if root == nil || len(root.Args) == 0 {
		return ps5109Match{}, false
	}
	fn, signature, ok := typedCallee(pass, root.Fun)
	if !ok || signature.Recv() != nil || fn.Pkg() == nil || fn.Pkg().Path() != "path" || fn.Name() != "Join" {
		return ps5109Match{}, false
	}
	binding, ok := typedPackageBinding(pass, root.Fun)
	if !ok {
		return ps5109Match{}, false
	}
	nested := collectTypedNestedPackageCallSpine(pass, root, 0, "path", "Join", binding)
	if len(nested) == 0 {
		return ps5109Match{}, false
	}
	return ps5109Match{root: root, nested: nested}, true
}
