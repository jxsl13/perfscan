package checks

import (
	"fmt"
	"go/ast"
	"go/types"
	"unicode"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS5117 removes repeated Fields+Join canonicalization through the shared
// typed compound-normalizer pipeline abstraction.
var PS5117 = register(&lint.Check{
	ID:       "PS5117",
	Category: "alloc",
	Slug:     "repeated-fields-join-canonicalizer",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "Join+Fields canonicalizes an already canonicalized string or byte slice",
		Text: `A common whitespace canonicalizer splits a value into Unicode
fields and joins them with one chosen separator. Once that result has been
produced, applying the compatible Fields+Join pipeline again cannot change it:

  strings.Join(strings.Fields(
      strings.Join(strings.Fields(s), " "),
  ), " ")
  -> strings.Join(strings.Fields(s), " ")

  bytes.Join(bytes.Fields(
      bytes.Join(bytes.Fields(b), []byte(" ")),
  ), []byte(" "))
  -> bytes.Join(bytes.Fields(b), []byte(" "))

PS5117 uses the shared typed compound-normalizer matcher: one logical stage
can contain several standard-library calls, and the retained terminal stage's
postcondition is checked against every removed outer stage. Arbitrarily deep
compatible runs collapse to one stage in a single fix, even when an
intermediate separator would not be stable for arbitrary input. Calls must resolve through go/types to
ordinary strings.Join+strings.Fields or bytes.Join+bytes.Fields calls using the
same import binding at every layer. Aliases and parentheses work; dot imports,
function values, methods, user lookalikes, dynamic separators, changed package
bindings, FieldsFunc callbacks, and mixed string/byte pipelines stay silent.

For strings, two exact fixed-point families are accepted. A separator made
entirely of Unicode whitespace is canonical when every removed layer uses the
same separator. A separator containing no Unicode whitespace produces an
output with zero whitespace, so a compatible outer constant separator is
never consulted by Join's one-field path. A separator mixing whitespace and
non-whitespace cannot serve as the retained terminal, though such an outer
constant stage may be removed around a no-whitespace terminal because its
separator is unused. For bytes, only a non-empty,
all-Unicode-whitespace separator repeated by exact byte value is accepted.
The no-whitespace byte form is deliberately excluded: its second Fields call
has one field, so bytes.Join can take a different allocation-growth path and
change observable capacity even when element bytes agree.

Within that domain the rewrite is BIT-IDENTICAL. Fields discards leading and
trailing Unicode whitespace and returns nonempty fields; joining with the
retained separator creates the unique canonical representation. Re-splitting
recovers those exact fields, including invalid UTF-8 bytes as non-space
RuneError input, and rejoining produces the same value. Empty/all-space input,
nil byte slices, output nilness, length, capacity, independent byte storage,
overflow behavior, and the original input evaluation remain unchanged.

The fix keeps the deepest complete Fields+Join stage byte-for-byte and removes
only outer scaffolding and compile-time separator expressions. The shared
editor withholds an automatic fix if deleted syntax contains a comment or the
last required use of a local/import, while retaining the diagnostic.`,
		Before: `canonical := strings.Join(strings.Fields(
	strings.Join(strings.Fields(payload), " "),
), " ")`,
		After: `canonical := strings.Join(strings.Fields(payload), " ")`,
		MeasuredWin: `On Apple M2 Pro with Go 1.26.6 (10 runs, one CPU),
benchmarks/ps5117_test.go canonicalized a 95 KiB mixed-space payload. Two
Fields+Join stages measured a median 542,547 ns/op, 671,744 B/op, and 4
allocations versus 277,254 ns/op, 335,872 B/op, and 2 allocations for the one
required stage: about 1.96x faster, 48.9% less time, and exactly half the
allocation count and bytes.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5117",
		Doc:  "repeated strings/bytes Fields+Join canonicalization",
		Run:  runPS5117,
	},
})

type ps5117StageMetadata struct {
	pkgPath   string
	separator string
}

func runPS5117(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		covered := make(map[*ast.CallExpr]bool)
		ast.Inspect(file, func(node ast.Node) bool {
			root, ok := node.(*ast.CallExpr)
			if !ok || covered[root] {
				return true
			}
			matchStage := func(expression ast.Expr) (typedCompoundNormalizerStage, bool) {
				return ps5117FieldsJoinStage(pass, expression)
			}
			matched, ok := matchRepeatedTypedCompoundNormalizerPipeline(pass, root, matchStage, ps5117CompatibleStages)
			if !ok {
				return true
			}
			metadata := matched.stages[0].metadata.(ps5117StageMetadata)
			diagnostic := analysis.Diagnostic{
				Pos: matched.outer.Pos(), End: matched.outer.End(),
				Message: fmt.Sprintf("%s.Join+Fields canonicalization is applied %d times to an already canonical result; remove %d redundant scan/allocation layer(s)", metadata.pkgPath, len(matched.stages), len(matched.stages)-1),
			}
			spans := []tokenSpan{
				{start: matched.outer.Pos(), end: matched.keep.Pos()},
				{start: matched.keep.End(), end: matched.outer.End()},
			}
			if fix, ok := fixDeletedCallScaffoldingPaths(pass, file, []string{metadata.pkgPath}, "remove repeated Fields+Join canonicalization", spans...); ok {
				diagnostic.SuggestedFixes = []analysis.SuggestedFix{fix}
			}
			pass.Report(diagnostic)
			markRepeatedTypedCompoundNormalizerPipeline(covered, matched)
			return true
		})
	}
	return nil, nil
}

func ps5117FieldsJoinStage(pass *analysis.Pass, expression ast.Expr) (typedCompoundNormalizerStage, bool) {
	join, ok := ps2110Unparen(expression).(*ast.CallExpr)
	if !ok || len(join.Args) != 2 || join.Ellipsis.IsValid() {
		return typedCompoundNormalizerStage{}, false
	}
	joinFunction, joinSignature, ok := typedCallee(pass, join.Fun)
	if !ok || joinSignature.Recv() != nil || joinFunction.Pkg() == nil ||
		(joinFunction.Pkg().Path() != "strings" && joinFunction.Pkg().Path() != "bytes") ||
		joinFunction.Name() != "Join" {
		return typedCompoundNormalizerStage{}, false
	}
	binding, ok := typedPackageBinding(pass, join.Fun)
	if !ok {
		return typedCompoundNormalizerStage{}, false
	}
	fields, ok := ps2110Unparen(join.Args[0]).(*ast.CallExpr)
	if !ok || len(fields.Args) != 1 || fields.Ellipsis.IsValid() ||
		!typedPackageCallWithBinding(pass, fields, joinFunction.Pkg().Path(), "Fields", binding) {
		return typedCompoundNormalizerStage{}, false
	}
	separator, ok := ps5117ConstantSeparator(pass, join.Args[1], joinFunction.Pkg().Path())
	if !ok {
		return typedCompoundNormalizerStage{}, false
	}
	return typedCompoundNormalizerStage{
		root: join, input: fields.Args[0], bindings: []*types.PkgName{binding},
		metadata: ps5117StageMetadata{pkgPath: joinFunction.Pkg().Path(), separator: separator},
	}, true
}

func ps5117ConstantSeparator(pass *analysis.Pass, expression ast.Expr, pkgPath string) (string, bool) {
	if pkgPath == "strings" {
		return ps5077Cutset(pass, expression)
	}
	return ps5080ConstBytes(pass, expression)
}

func ps5117CompatibleStages(outer, terminal typedCompoundNormalizerStage) bool {
	outerMetadata, outerOK := outer.metadata.(ps5117StageMetadata)
	terminalMetadata, terminalOK := terminal.metadata.(ps5117StageMetadata)
	if !outerOK || !terminalOK || outerMetadata.pkgPath != terminalMetadata.pkgPath {
		return false
	}
	allSpace, noSpace := ps5117SeparatorClass(terminalMetadata.separator)
	if terminalMetadata.pkgPath == "strings" && noSpace {
		return true
	}
	return allSpace && terminalMetadata.separator != "" && outerMetadata.separator == terminalMetadata.separator
}

func ps5117SeparatorClass(separator string) (allSpace, noSpace bool) {
	if separator == "" {
		return false, true
	}
	allSpace = true
	noSpace = true
	for _, character := range separator {
		if unicode.IsSpace(character) {
			noSpace = false
		} else {
			allSpace = false
		}
	}
	return allSpace, noSpace
}
