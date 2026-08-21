package checks

import (
	"go/ast"
	"go/types"
	"unicode/utf8"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS5116 detects UTF-8 validation immediately after a sanitizer that already
// guarantees valid output. Its fix removes the whole dead pipeline only when
// every discarded evaluation is pure and non-panicking.
var PS5116 = register(&lint.Check{
	ID:       "PS5116",
	Category: "arith",
	Slug:     "validate-proven-valid-utf8-sanitizer-result",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "unicode/utf8 validates a ToValidUTF8 result that is guaranteed to be valid",
		Text: `strings.ToValidUTF8 and bytes.ToValidUTF8 guarantee valid UTF-8
when their replacement is valid. Immediately validating that result can only
return true:

  utf8.ValidString(strings.ToValidUTF8(s, "�")) -> true
  utf8.Valid(bytes.ToValidUTF8(b, []byte("?"))) -> true

The replacement is analyzed by compile-time byte value, not spelling. String
literals, named/folded string constants, nil or empty byte replacements, and
[]byte conversions of constant strings work. utf8.ValidString rejects an
invalid string replacement, and utf8.Valid rejects an invalid byte
replacement, because the sanitizer can emit those bytes and make the validator
meaningful.

The report applies even when the sanitizer input has side effects: the boolean
is still guaranteed true, but deleting the composition would delete those
effects. The automatic fix therefore requires the complete sanitizer
evaluation to be side-effect-free and non-panicking. Plain variables,
constants, literals, safe value-field selections, and pure arithmetic/string
expressions are accepted. Calls, indexing, pointer dereferences, receives, and
other potentially observable inputs keep the finding advisory. Function-local
declarations whose last use would disappear receive the same protection.

Adjacent ToValidUTF8 calls are handled as one composition when every nested
input and companion evaluation is independently discardable. PS5116 then owns
those sanitizer calls ahead of PS5115 and replaces the whole validator tree
with true in one pass. If PS5116 cannot safely delete the complete tree,
PS5115 remains free to remove any redundant inner sanitizer scans.

The new shared cross-package consumer/producer matcher resolves the
unicode/utf8 consumer and strings/bytes producer independently through
go/types and ordinary import bindings. Exact arity, package functions, and
type-checked producer/consumer compatibility are required. Aliases and parentheses work; dot imports,
function values, methods, shadowed lookalikes, mismatched Valid/ValidString
families, and other UTF-8-producing functions stay silent.

The fix is BIT-IDENTICAL whenever attached: the original bool is true for
every byte sequence and every accepted valid replacement, and all removed
evaluations are proven inert. It replaces the full expression with the
predeclared true constant, removes newly orphaned ordinary imports, and loses
no comments. Comments, cgo imports, or required local/import uses keep the
diagnostic advisory. A replacement that would introduce a constant switch case
or map key also stays advisory: the runtime expression may legally coexist with
an equal constant, while replacing it with true could create an illegal
duplicate constant.`,
		Before: `ok := utf8.ValidString(strings.ToValidUTF8(payload, "�"))`,
		After:  `ok := true`,
		MeasuredWin: `benchmarks/ps5116_test.go measures validation of a sanitized
96 KiB invalid-byte payload on an Apple M2 Pro (10 runs, one CPU). The dead
sanitize/validate pipeline measured a median 453,692 ns/op, 655,360 B/op, and
4 allocs/op; the proven true result measured 2.001 ns/op, 0 B/op, and 0
allocs/op. The fix removes about 453.7 microseconds, all 640 KiB of allocation,
and all four allocations per call on this identity-only workload.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5116",
		Doc:  "unicode/utf8 validates the guaranteed-valid result of strings/bytes.ToValidUTF8",
		Run:  runPS5116,
	},
})

type ps5116Match struct {
	composition typedPackageConsumerProducerComposition
	producerPkg string
}

func runPS5116(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		parents := ps6071Parents(file)
		ast.Inspect(file, func(node ast.Node) bool {
			consumer, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			match, ok := ps5116ValidationOfSanitizer(pass, consumer)
			if !ok {
				return true
			}
			diagnostic := analysis.Diagnostic{
				Pos: consumer.Pos(), End: consumer.End(),
				Message: "utf8." + match.composition.consumerFunction.Name() + " validates " + match.producerPkg + ".ToValidUTF8 output whose replacement already guarantees valid UTF-8; the result is always true",
			}
			if fix, ok := ps5116SuggestedFix(pass, file, match, parents); ok {
				diagnostic.SuggestedFixes = []analysis.SuggestedFix{fix}
			}
			pass.Report(diagnostic)
			return true
		})
	}
	return nil, nil
}

func ps5116SuggestedFix(pass *analysis.Pass, file *ast.File, match ps5116Match, parents map[ast.Node]ast.Node) (analysis.SuggestedFix, bool) {
	consumer := match.composition.consumer
	if !ps5116DiscardableSanitizer(pass, match.composition.producer, match.producerPkg, match.composition.producerBinding) ||
		replacementIntroducesConstantInUniqueContext(pass, consumer, parents) ||
		!deletionsKeepRequiredLocalVariables(pass, file, tokenSpan{start: consumer.Pos(), end: consumer.End()}) {
		return analysis.SuggestedFix{}, false
	}
	edit := analysis.TextEdit{Pos: consumer.Pos(), End: consumer.End(), NewText: []byte("true")}
	return fixReplacedCallScaffoldingPaths(pass, file, []string{"unicode/utf8", match.producerPkg}, "replace validation of proven-valid UTF-8 with true", edit)
}

func ps5116ValidationOfSanitizer(pass *analysis.Pass, consumer *ast.CallExpr) (ps5116Match, bool) {
	consumerFunction, _, ok := typedCallee(pass, consumer.Fun)
	if !ok || consumerFunction.Pkg() == nil || consumerFunction.Pkg().Path() != "unicode/utf8" {
		return ps5116Match{}, false
	}
	var producerPkg string
	switch consumerFunction.Name() {
	case "ValidString":
		producerPkg = "strings"
	case "Valid":
		producerPkg = "bytes"
	default:
		return ps5116Match{}, false
	}
	composition, ok := matchTypedCrossPackageConsumerProducerComposition(
		pass, consumer, "unicode/utf8", consumerFunction.Name(), 1, 0,
		func(function *types.Func, signature *types.Signature, call *ast.CallExpr) bool {
			return signature.Recv() == nil && function.Pkg() != nil && function.Pkg().Path() == producerPkg &&
				function.Name() == "ToValidUTF8" && len(call.Args) == 2 && !call.Ellipsis.IsValid() &&
				ps5116ValidReplacement(pass, call, producerPkg)
		},
	)
	return ps5116Match{composition: composition, producerPkg: producerPkg}, ok
}

func ps5116ValidReplacement(pass *analysis.Pass, call *ast.CallExpr, pkgPath string) bool {
	replacement, ok := ps5116ConstantReplacement(pass, call.Args[1], pkgPath)
	return ok && utf8.ValidString(replacement)
}

func ps5116ConstantReplacement(pass *analysis.Pass, expression ast.Expr, pkgPath string) (string, bool) {
	if pkgPath == "strings" {
		return ps5077Cutset(pass, expression)
	}
	return ps5080ConstBytes(pass, expression)
}

func ps5116DiscardableSanitizer(pass *analysis.Pass, call *ast.CallExpr, pkgPath string, binding *types.PkgName) bool {
	if !ps5116TypedSanitizerCall(pass, call, pkgPath, binding) {
		return false
	}
	if _, ok := ps5116ConstantReplacement(pass, call.Args[1], pkgPath); !ok {
		return false
	}
	if inner, ok := ps2110Unparen(call.Args[0]).(*ast.CallExpr); ok && ps5116TypedSanitizerCall(pass, inner, pkgPath, binding) {
		return ps5116DiscardableSanitizer(pass, inner, pkgPath, binding)
	}
	return ps2106PureArg(pass, call.Args[0], nil)
}

func ps5116TypedSanitizerCall(pass *analysis.Pass, call *ast.CallExpr, pkgPath string, binding *types.PkgName) bool {
	if call == nil || len(call.Args) != 2 || call.Ellipsis.IsValid() ||
		!typedPackageCallWithBinding(pass, call, pkgPath, "ToValidUTF8", binding) {
		return false
	}
	function, signature, ok := typedCallee(pass, call.Fun)
	return ok && signature.Recv() == nil && function.Pkg() != nil && function.Pkg().Path() == pkgPath
}

// ps5116OwnedStringSanitizers marks strings.ToValidUTF8 calls that disappear
// with a safe terminal PS5116 replacement. PS5115 yields these sites to avoid
// overlapping fixes; advisory PS5116 sites own nothing.
func ps5116OwnedStringSanitizers(pass *analysis.Pass, file *ast.File) map[*ast.CallExpr]bool {
	owned := make(map[*ast.CallExpr]bool)
	parents := ps6071Parents(file)
	ast.Inspect(file, func(node ast.Node) bool {
		consumer, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		match, ok := ps5116ValidationOfSanitizer(pass, consumer)
		if !ok || match.producerPkg != "strings" {
			return true
		}
		if _, fixable := ps5116SuggestedFix(pass, file, match, parents); !fixable {
			return true
		}
		for current := match.composition.producer; ps5116TypedSanitizerCall(pass, current, "strings", match.composition.producerBinding); {
			owned[current] = true
			next, nested := ps2110Unparen(current.Args[0]).(*ast.CallExpr)
			if !nested {
				break
			}
			current = next
		}
		return true
	})
	return owned
}
