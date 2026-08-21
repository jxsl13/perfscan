package checks

import (
	"go/ast"
	"go/constant"
	"go/types"
	"go/version"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS5120 replaces an assigned strings.SplitN head with strings.Cut's first
// result, removing the otherwise unavoidable []string allocation.
var PS5120 = register(&lint.Check{
	ID:       "PS5120",
	Category: "alloc",
	Slug:     "assigned-split-head-to-cut",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "an assigned strings.Split/SplitN head allocates a piece slice instead of using strings.Cut",
		Text: `Assigning the first piece of strings.SplitN still allocates a
[]string solely to transport one substring:

  head := strings.SplitN(value, ":", 2)[0]

For a proven non-empty separator, strings.Cut returns the identical head as its
first result without materializing a slice:

  head, _, _ := strings.Cut(value, ":")

PS5120 handles direct single-value = and := assignments of
strings.SplitN(...)[0] when the limit is a compile-time integer greater than
one or negative. PS2009 uses the same abstraction to take compatible
strings.Split(...)[0] assignments directly to Cut in one pass instead of first
introducing SplitN. The separator must be a non-empty compile-time string.
This guard is load-bearing: strings.Cut(s, "") returns an empty head, while
strings.SplitN(s, "", 2)[0] returns the first UTF-8 sequence (and both Split
forms return no piece for an empty input).

The shared typed assigned-indexed-producer matcher requires a direct index in
a one-result assignment and resolves the ordinary strings package binding and
function object. Aliases and parentheses work. Return expressions, value
specifications, stored piece slices, bytes.Split/SplitN, index one, limited
counts zero/one, dynamic or empty separators, methods, function values, dot
imports, and user lookalikes stay silent. bytes is intentionally excluded:
its Split head cap-clamps a matched subslice, while bytes.Cut's first result
retains the original capacity.

The rewrite preserves the original input and separator byte-for-byte and in
the same order. It adds two blank assignment targets, renames SplitN to Cut,
and deletes only the constant limit plus index scaffolding. Comments or a last
local/import use in deleted syntax keep the report advisory. The rule runs only
when the file's effective language version includes strings.Cut (Go 1.18).

Within the accepted domain the rewrite is BIT-IDENTICAL: both operations stop
at the first non-overlapping separator, return the same built-in string view,
and never panic because non-empty separators make Split/SplitN return at least
one piece. The discarded Cut remainder and found flag are pure values.`,
		Before: `head := strings.SplitN(line, ":", 2)[0]`,
		After:  `head, _, _ := strings.Cut(line, ":")`,
		MeasuredWin: `On Apple M2 Pro with Go 1.26.6 (10 runs, one CPU),
benchmarks/ps5120_test.go measured the assigned SplitN head at a median 29.32
ns/op, 32 B/op, and 1 allocation. strings.Cut measured 5.246 ns/op, 0 B/op,
and 0 allocations: about 5.59x faster and 82.1% less time while removing the
result-slice allocation.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5120",
		Doc:  "assigned strings.SplitN head allocates a result slice instead of using strings.Cut",
		Run:  runPS5120,
	},
})

type ps5120Match struct {
	composition typedAssignedIndexedPackageProducer
	producer    string
}

func runPS5120(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		if !ps5120CutAvailable(pass, file) {
			continue
		}
		parents := ps6071Parents(file)
		ast.Inspect(file, func(node ast.Node) bool {
			assignment, ok := node.(*ast.AssignStmt)
			if !ok || ps5121OwnsAssignment(pass, assignment, parents) {
				return true
			}
			match, ok := ps5120AssignedHead(pass, assignment, "SplitN")
			if !ok {
				return true
			}
			diagnostic := ps5120Diagnostic(match)
			if fix, ok := ps5120SuggestedFix(pass, file, match); ok {
				diagnostic.SuggestedFixes = []analysis.SuggestedFix{fix}
			}
			pass.Report(diagnostic)
			return true
		})
	}
	return nil, nil
}

func ps5120AssignedHead(pass *analysis.Pass, assignment *ast.AssignStmt, producerName string) (ps5120Match, bool) {
	composition, ok := matchTypedAssignedIndexedPackageProducer(
		pass, assignment, "strings",
		func(function *types.Func, _ *types.Signature, call *ast.CallExpr) bool {
			if function.Name() != producerName {
				return false
			}
			switch producerName {
			case "Split":
				return len(call.Args) == 2
			case "SplitN":
				if len(call.Args) != 3 {
					return false
				}
				count, ok := ps5120IntegerConstant(pass, call.Args[2])
				return ok && (count < 0 || count > 1)
			default:
				return false
			}
		},
	)
	if !ok || !ps2009IndexIsZero(pass, composition.index.Index) ||
		!ps5120NonemptySeparator(pass, composition.producer.Args[1]) {
		return ps5120Match{}, false
	}
	return ps5120Match{composition: composition, producer: producerName}, true
}

func ps5120AssignmentForIndex(index *ast.IndexExpr, parents map[ast.Node]ast.Node) (*ast.AssignStmt, bool) {
	for parent := parents[index]; parent != nil; parent = parents[parent] {
		switch current := parent.(type) {
		case *ast.ParenExpr:
			continue
		case *ast.AssignStmt:
			return current, true
		default:
			return nil, false
		}
	}
	return nil, false
}

func ps5120Diagnostic(match ps5120Match) analysis.Diagnostic {
	composition := match.composition
	return analysis.Diagnostic{
		Pos: composition.resultExpression.Pos(), End: composition.resultExpression.End(),
		Message: "strings." + match.producer + "(...)[0] allocates a piece slice only to assign its head; strings.Cut returns the identical head directly with no result-slice allocation",
	}
}

func ps5120SuggestedFix(pass *analysis.Pass, file *ast.File, match ps5120Match) (analysis.SuggestedFix, bool) {
	composition := match.composition
	spans := []tokenSpan{
		{start: composition.resultExpression.Pos(), end: composition.producer.Pos()},
		{start: composition.producer.End(), end: composition.resultExpression.End()},
	}
	if match.producer == "SplitN" {
		spans = append(spans, tokenSpan{
			start: composition.producer.Args[1].End(),
			end:   composition.producer.End(),
		})
	}
	for _, span := range spans {
		if ps2111CommentIn(file, span.start, span.end) {
			return analysis.SuggestedFix{}, false
		}
	}
	if !deletionsKeepRequiredUses(pass, file, spans...) {
		return analysis.SuggestedFix{}, false
	}
	edits := []analysis.TextEdit{
		{Pos: composition.assignment.Lhs[0].End(), End: composition.assignment.Lhs[0].End(), NewText: []byte(", _, _")},
		{Pos: composition.producerSelector.Sel.Pos(), End: composition.producerSelector.Sel.End(), NewText: []byte("Cut")},
		{Pos: spans[0].start, End: spans[0].end},
		{Pos: spans[1].start, End: spans[1].end},
	}
	if match.producer == "SplitN" {
		span := spans[2]
		edits = append(edits, analysis.TextEdit{Pos: span.start, End: span.end, NewText: []byte(")")})
	}
	return analysis.SuggestedFix{
		Message:   "replace the assigned Split head with strings.Cut",
		TextEdits: edits,
	}, true
}

func ps5120NonemptySeparator(pass *analysis.Pass, expression ast.Expr) bool {
	value, ok := pass.TypesInfo.Types[ps2110Unparen(expression)]
	return ok && value.Value != nil && value.Value.Kind() == constant.String && constant.StringVal(value.Value) != ""
}

func ps5120IntegerConstant(pass *analysis.Pass, expression ast.Expr) (int64, bool) {
	value, ok := pass.TypesInfo.Types[ps2110Unparen(expression)]
	if !ok || value.Value == nil || value.Value.Kind() != constant.Int {
		return 0, false
	}
	result, exact := constant.Int64Val(value.Value)
	return result, exact
}

func ps5120CutAvailable(pass *analysis.Pass, file *ast.File) bool {
	value := ""
	if pass.TypesInfo.FileVersions != nil {
		value = pass.TypesInfo.FileVersions[file]
	}
	if value == "" && pass.Pkg != nil {
		value = pass.Pkg.GoVersion()
	}
	if value == "" || version.Lang(value) == "" {
		return true
	}
	return version.Compare(version.Lang(value), "go1.18") >= 0
}
