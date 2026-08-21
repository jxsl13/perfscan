package checks

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS5118 removes same-byte replacement passes outside a terminal replacement
// that proves the byte is absent. Replace and ReplaceAll are represented as
// interchangeable logical stages by the compound-normalizer abstraction.
var PS5118 = register(&lint.Check{
	ID:       "PS5118",
	Category: "arith",
	Slug:     "replace-after-byte-eliminating-replacement",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "Replace or ReplaceAll searches for a byte eliminated by an inner all-replacement",
		Text: `An all-replacement of one byte establishes a strong postcondition:
when the replacement text does not contain that byte, the result cannot
contain it. Any enclosing strings.Replace or strings.ReplaceAll search for the
same byte is therefore a no-match pass and returns the inner string unchanged:

  strings.ReplaceAll(
      strings.ReplaceAll(s, "\x00", ""),
      "\x00", "?",
  )
  -> strings.ReplaceAll(s, "\x00", "")

PS5118 models strings.ReplaceAll and strings.Replace with a negative constant
count as interchangeable terminal stages in the shared typed compound-
normalizer abstraction. Enclosing Replace calls may use any count because the
proven-absent byte has zero matches. Arbitrarily deep mixed pipelines collapse
to the deepest stage whose all-replacement removes the byte. If a deeper stage
can reintroduce it but a middle stage eliminates it, the middle stage is
retained and all redundant layers outside it disappear in one fix.

The one-BYTE restriction is load-bearing. For a multi-byte old pattern,
removing matches can concatenate surrounding fragments into a new match:
strings.ReplaceAll("aabb", "ab", "") is "ab", and a second call changes it to
"". Empty old also has boundary-insertion semantics. Both stay silent. The
terminal replacement must be a compile-time string constant that omits the
single old byte. Arbitrary bytes and invalid UTF-8 are supported because
Replace performs literal byte-substring matching, not rune matching.

Calls resolve through go/types to ordinary strings.Replace/ReplaceAll package
functions using one import binding and one concrete string result type.
Aliases, parentheses, folded constants, and mixed Replace/ReplaceAll layers
work. Dot imports, methods, function values, user lookalikes, different or
dynamic old patterns, a limited terminal Replace, and terminal replacements
that retain the byte stay silent. bytes.Replace is excluded: its no-match path
still returns a fresh slice, so removing an outer call can change observable
allocation identity and capacity.

Go evaluates old, new, and n before entering an outer call even when the
proven postcondition means they are unused. PS5118 reports a dynamic outer new
or count but leaves it advisory because deleting that evaluation could remove
effects or a panic. The automatic fix requires every deleted companion
expression to be compile-time constant. A content-no-op stage (old==new or
n==0) inside a fixable terminal pipeline yields to PS5118 so the complete
composition reaches its fixed point in one pass; standalone content no-ops
remain PS5080's responsibility. Comments and last-use local/import liveness are
guarded by the shared editor.

Within the fixable domain the rewrite is BIT-IDENTICAL. The retained terminal
call evaluates the original input exactly once, performs the only replacements
that can match, and returns the same string storage the removed no-match calls
would return directly. Length, bytes, invalid UTF-8, panics, and interface
dynamic type are unchanged.`,
		Before: `clean := strings.ReplaceAll(
	strings.ReplaceAll(payload, "\x00", ""),
	"\x00", "?",
)`,
		After: `clean := strings.ReplaceAll(payload, "\x00", "")`,
		MeasuredWin: `On Apple M2 Pro with Go 1.26.6 (10 runs, one CPU),
benchmarks/ps5118_test.go measured the common already-clean path over a 96 KiB
payload. Two single-byte sanitizers took a median 3,862 ns/op versus 1,812
ns/op for the retained terminal sanitizer: about 2.13x faster and 53.1% less
time. Both forms allocated 0 B/op in 0 allocations; the entire delta is the
removed no-match scan.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5118",
		Doc:  "strings Replace/ReplaceAll searches for a byte eliminated by an inner all-replacement",
		Run:  runPS5118,
	},
})

type ps5118StageMetadata struct {
	oldValue        string
	newValue        string
	newConstant     bool
	countConstant   bool
	replacesAll     bool
	independentNoop bool
	functionName    string
}

func runPS5118(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		covered := make(map[*ast.CallExpr]bool)
		ast.Inspect(file, func(node ast.Node) bool {
			root, ok := node.(*ast.CallExpr)
			if !ok || covered[root] {
				return true
			}
			matched, ok := ps5118ReplacementPipeline(pass, root)
			if !ok {
				return true
			}
			terminal := matched.stages[len(matched.stages)-1].metadata.(ps5118StageMetadata)
			diagnostic := analysis.Diagnostic{
				Pos: matched.outer.Pos(), End: matched.outer.End(),
				Message: fmt.Sprintf("strings.%s eliminates byte %q, so %d enclosing Replace/ReplaceAll pass(es) cannot change the proven result; remove the redundant passes", terminal.functionName, terminal.oldValue, len(matched.stages)-1),
			}
			if fix, ok := ps5118SuggestedFix(pass, file, matched); ok {
				diagnostic.SuggestedFixes = []analysis.SuggestedFix{fix}
			}
			pass.Report(diagnostic)
			markRepeatedTypedCompoundNormalizerPipeline(covered, matched)
			return true
		})
	}
	return nil, nil
}

func ps5118ReplacementPipeline(pass *analysis.Pass, root *ast.CallExpr) (repeatedTypedCompoundNormalizerPipeline, bool) {
	matchStage := func(expression ast.Expr) (typedCompoundNormalizerStage, bool) {
		return ps5118ReplacementStage(pass, expression)
	}
	return matchRepeatedTypedCompoundNormalizerPipeline(pass, root, matchStage, ps5118CompatibleStages)
}

func ps5118ReplacementStage(pass *analysis.Pass, expression ast.Expr) (typedCompoundNormalizerStage, bool) {
	call, ok := ps2110Unparen(expression).(*ast.CallExpr)
	if !ok || call.Ellipsis.IsValid() {
		return typedCompoundNormalizerStage{}, false
	}
	function, signature, ok := typedCallee(pass, call.Fun)
	if !ok || signature.Recv() != nil || function.Pkg() == nil || function.Pkg().Path() != "strings" {
		return typedCompoundNormalizerStage{}, false
	}
	metadata := ps5118StageMetadata{functionName: function.Name()}
	switch function.Name() {
	case "ReplaceAll":
		if len(call.Args) != 3 {
			return typedCompoundNormalizerStage{}, false
		}
		metadata.countConstant = true
		metadata.replacesAll = true
	case "Replace":
		if len(call.Args) != 4 {
			return typedCompoundNormalizerStage{}, false
		}
		if value, ok := ps5118IntegerConstant(pass, call.Args[3]); ok {
			metadata.countConstant = true
			metadata.replacesAll = constant.Sign(value) < 0
			metadata.independentNoop = constant.Sign(value) == 0
		}
	default:
		return typedCompoundNormalizerStage{}, false
	}
	binding, ok := typedPackageBinding(pass, call.Fun)
	if !ok {
		return typedCompoundNormalizerStage{}, false
	}
	metadata.oldValue, ok = ps5077Cutset(pass, call.Args[1])
	if !ok {
		return typedCompoundNormalizerStage{}, false
	}
	if metadata.newValue, metadata.newConstant = ps5077Cutset(pass, call.Args[2]); metadata.newConstant && metadata.newValue == metadata.oldValue {
		metadata.independentNoop = true
	}
	return typedCompoundNormalizerStage{
		root: call, input: call.Args[0], bindings: []*types.PkgName{binding}, metadata: metadata,
	}, true
}

func ps5118IntegerConstant(pass *analysis.Pass, expression ast.Expr) (constant.Value, bool) {
	typed, ok := pass.TypesInfo.Types[ps2110Unparen(expression)]
	return typed.Value, ok && typed.Value != nil && typed.Value.Kind() == constant.Int
}

func ps5118CompatibleStages(outer, terminal typedCompoundNormalizerStage) bool {
	outerMetadata, outerOK := outer.metadata.(ps5118StageMetadata)
	terminalMetadata, terminalOK := terminal.metadata.(ps5118StageMetadata)
	if !outerOK || !terminalOK ||
		len(terminalMetadata.oldValue) != 1 || !terminalMetadata.replacesAll ||
		!terminalMetadata.newConstant || strings.Contains(terminalMetadata.newValue, terminalMetadata.oldValue) {
		return false
	}
	if outerMetadata.independentNoop {
		return true
	}
	return outerMetadata.oldValue == terminalMetadata.oldValue
}

func ps5118DiscardableOuterCompanions(matched repeatedTypedCompoundNormalizerPipeline) bool {
	for _, stage := range matched.stages[:len(matched.stages)-1] {
		metadata := stage.metadata.(ps5118StageMetadata)
		if !metadata.newConstant || metadata.functionName == "Replace" && !metadata.countConstant {
			return false
		}
	}
	return true
}

func ps5118SuggestedFix(pass *analysis.Pass, file *ast.File, matched repeatedTypedCompoundNormalizerPipeline) (analysis.SuggestedFix, bool) {
	if !ps5118DiscardableOuterCompanions(matched) {
		return analysis.SuggestedFix{}, false
	}
	spans := []tokenSpan{
		{start: matched.outer.Pos(), end: matched.keep.Pos()},
		{start: matched.keep.End(), end: matched.outer.End()},
	}
	return fixDeletedCallScaffoldingPaths(pass, file, []string{"strings"}, "remove replacement passes for the proven-absent byte", spans...)
}

// ps5118OwnedIndependentNoops lets the terminal postcondition rule remove a
// complete fixable pipeline in one edit. PS5080 still reports every standalone
// content no-op and every pipeline for which PS5118 cannot safely delete all
// companion evaluations.
func ps5118OwnedIndependentNoops(pass *analysis.Pass, file *ast.File) map[*ast.CallExpr]bool {
	owned := make(map[*ast.CallExpr]bool)
	ast.Inspect(file, func(node ast.Node) bool {
		root, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		matched, ok := ps5118ReplacementPipeline(pass, root)
		if !ok {
			return true
		}
		if _, fixable := ps5118SuggestedFix(pass, file, matched); !fixable {
			return true
		}
		for _, stage := range matched.stages[:len(matched.stages)-1] {
			if stage.metadata.(ps5118StageMetadata).independentNoop {
				owned[stage.root] = true
			}
		}
		return true
	})
	return owned
}
