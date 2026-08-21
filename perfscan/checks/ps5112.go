package checks

import (
	"go/ast"
	"go/constant"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS5112 removes strings Join/Split compositions that are exact inverses and
// therefore allocate and rescan only to reconstruct their original input.
var PS5112 = register(&lint.Check{
	ID:       "PS5112",
	Category: "arith",
	Slug:     "inverse-strings-split-join-composition",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "strings.Join exactly reverses an adjacent Split operation and reconstructs its input",
		Text: `Several strings Split/Join compositions are exact inverses:

  strings.Join(strings.Split(s, sep), sep)          -> s
  strings.Join(strings.SplitAfter(s, sep), "")      -> s
  strings.Join(strings.SplitN(s, sep, -1), sep)     -> s
  strings.Join(strings.SplitAfterN(s, sep, -1), "") -> s

Split removes every non-overlapping separator and Join puts that same
separator back between the resulting pieces. SplitAfter retains the separator
at the end of each piece, so joining with the empty string reconstructs the
input. A negative SplitN count means all pieces and has the same identities.
For either N variant, a count of one always produces the one-element slice
[]string{s}, so Join returns s regardless of its separator.

The rule accepts compile-time string separators and compile-time SplitN counts.
This purity gate is load-bearing: deleting the composition also deletes the
inner and outer separator evaluations, so dynamic expressions—even two
textually identical ones—stay untouched. Empty separators are fully covered:
Split divides after each UTF-8 sequence and an empty Join concatenates those
original byte slices back exactly, including malformed UTF-8. Named and folded
constants work.

The replacement expression must have the exact built-in string type (or be an
untyped string constant). A defined string value reaches strings.Split through
an explicit string conversion; that conversion is retained, preserving the
composed expression's static and interface-dynamic type. Alias types remain
eligible. The explicit result-type guard protects the same invariant if future
language or library forms admit a distinct string-like argument directly.

The shared typed consumer/producer composition matcher resolves Join and its
direct Split producer through go/types and requires the same concrete strings
import binding. Aliases and parentheses work; dot imports, methods, function
values, shadowed lookalikes, stored intermediate slices, bytes operations,
limited counts other than one, and mismatched separators do not match. PS5112
owns exact inverse Split shapes ahead of PS2015, so one fix pass reaches the
allocation-free input rather than first spelling strings.ReplaceAll(s, sep,
sep).

The rewrite is BIT-IDENTICAL and retains the input expression byte-for-byte,
evaluated exactly once in its original position. It deletes only the pure
stdlib composition and constants. Comments and function-local declarations
whose last use would disappear keep the finding advisory; an otherwise orphaned
ordinary strings import is removed in the same fix. There is no Unicode
normalization, separator interpretation, or new panic surface.`,
		Before: `normalized := strings.Join(strings.Split(payload, separator), separator)`,
		After:  `normalized := payload`,
		MeasuredWin: `On an Apple M2 Pro with go1.26.6,
benchmarks/ps5112_test.go (10 runs, one CPU) measured inverse Split/Join over a
68 KiB string at a median 93,365 ns/op, 147,456 B/op, and 2 allocs/op. Directly
retaining the input measured 2.031 ns/op, 0 B/op, and 0 allocs/op: the rewrite
removes both allocations and essentially all work (about 46,000x in this
identity-only microbenchmark).`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5112",
		Doc:  "strings.Join directly reverses strings.Split, SplitAfter, SplitN, or SplitAfterN and returns the original plain string",
		Run:  runPS5112,
	},
})

type ps5112Match struct {
	composition typedPackageConsumerProducerComposition
	input       ast.Expr
	relation    string
}

func runPS5112(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		ast.Inspect(file, func(node ast.Node) bool {
			join, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			match, ok := ps5112SplitJoinIdentity(pass, join)
			if !ok {
				return true
			}
			producer := match.composition.producerFunction.Name()
			diagnostic := analysis.Diagnostic{
				Pos: join.Pos(), End: join.End(),
				Message: "strings.Join " + match.relation + " strings." + producer + " and reconstructs its original plain-string input; remove both calls instead of allocating and rescanning",
			}
			spans := []tokenSpan{
				{start: join.Pos(), end: match.input.Pos()},
				{start: match.input.End(), end: join.End()},
			}
			if fix, ok := fixDeletedCallScaffoldingPaths(pass, file, []string{"strings"}, "replace the inverse Split/Join composition with its input", spans...); ok {
				diagnostic.SuggestedFixes = []analysis.SuggestedFix{fix}
			}
			pass.Report(diagnostic)
			return true
		})
	}
	return nil, nil
}

func ps5112SplitJoinIdentity(pass *analysis.Pass, join *ast.CallExpr) (ps5112Match, bool) {
	composition, ok := matchTypedPackageConsumerProducerComposition(
		pass, join, "strings", "Join", 2, 0,
		func(function *types.Func, signature *types.Signature, call *ast.CallExpr) bool {
			if signature.Recv() != nil || function.Pkg() == nil || function.Pkg().Path() != "strings" {
				return false
			}
			switch function.Name() {
			case "Split", "SplitAfter":
				return len(call.Args) == 2
			case "SplitN", "SplitAfterN":
				return len(call.Args) == 3
			default:
				return false
			}
		},
	)
	if !ok {
		return ps5112Match{}, false
	}
	producer := composition.producer
	input := producer.Args[0]
	if !ps5112ReplacementType(pass, join, input) {
		return ps5112Match{}, false
	}
	innerSeparator, innerConstant := ps5112StringConstant(pass, producer.Args[1])
	outerSeparator, outerConstant := ps5112StringConstant(pass, join.Args[1])
	if !innerConstant || !outerConstant {
		return ps5112Match{}, false
	}

	relation := "exactly reverses"
	identity := false
	switch composition.producerFunction.Name() {
	case "Split":
		identity = outerSeparator == innerSeparator
	case "SplitAfter":
		identity = outerSeparator == ""
	case "SplitN", "SplitAfterN":
		count, ok := ps5112IntegerConstant(pass, producer.Args[2])
		if !ok {
			return ps5112Match{}, false
		}
		if count == 1 {
			identity = true
			relation = "consumes the single piece returned by"
		} else if count < 0 {
			if composition.producerFunction.Name() == "SplitN" {
				identity = outerSeparator == innerSeparator
			} else {
				identity = outerSeparator == ""
			}
		}
	}
	if !identity {
		return ps5112Match{}, false
	}
	return ps5112Match{composition: composition, input: input, relation: relation}, true
}

func ps5112StringConstant(pass *analysis.Pass, expression ast.Expr) (string, bool) {
	value, ok := pass.TypesInfo.Types[ps2110Unparen(expression)]
	if !ok || value.Value == nil || value.Value.Kind() != constant.String {
		return "", false
	}
	return constant.StringVal(value.Value), true
}

func ps5112IntegerConstant(pass *analysis.Pass, expression ast.Expr) (int64, bool) {
	value, ok := pass.TypesInfo.Types[ps2110Unparen(expression)]
	if !ok || value.Value == nil || value.Value.Kind() != constant.Int {
		return 0, false
	}
	return constant.Int64Val(value.Value)
}

func ps5112ReplacementType(pass *analysis.Pass, join *ast.CallExpr, input ast.Expr) bool {
	resultType := pass.TypesInfo.TypeOf(join)
	inputType := pass.TypesInfo.TypeOf(input)
	if resultType == nil || inputType == nil {
		return false
	}
	if types.Identical(resultType, inputType) {
		return true
	}
	basic, ok := types.Unalias(inputType).(*types.Basic)
	return ok && basic.Kind() == types.UntypedString
}
