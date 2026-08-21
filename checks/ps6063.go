package checks

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS6063 implements owner issue #780. It recognizes exact flat float
// negation loops whose native integer-domain SIMD form toggles only the sign
// bit, including for signaling NaNs.
var PS6063 = register(&lint.Check{
	ID:       "PS6063",
	Category: "arith",
	Slug:     "exact-float-negation-loop-is-not-vectorized",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "an exact float negation loop remains scalar despite a sign-bit SIMD form",
		Text: `Unary negation of a runtime float32 or float64 toggles exactly the
IEEE-754 sign bit. A flat copy loop therefore has a particularly strong SIMD
oracle:

  float32: outputBits = inputBits XOR 0x80000000
  float64: outputBits = inputBits XOR 0x8000000000000000

This swaps +0/-0, negates finite values and infinities, and changes only the
sign of quiet or signaling NaNs while preserving the quiet/signaling state and
every payload bit. An integer-domain vector XOR implements that contract
without floating comparison, conversion, or arithmetic exceptions.

This check implements owner issue #780. It reports only a canonical range loop
over flat []float32 or []float64 slices when the body is exactly one assignment
of dst[i] = -src[i], source and destination use the same induction variable and
element dtype, and the range expression is one of those slices. Named float
and slice types are supported through go/types. In-place negation is a valid
candidate. Integer/complex slices, conversions, double negation, changed
indices, compound assignments, constants, and loops with extra work stay
silent.

A //perfscan:measured-neg-fallback annotation suppresses a deliberately kept
scalar tail/site. The finding is also suppressed when the package already has
a same-dtype Neg/Negate sibling or registration carrying SIMD, NEON, SVE, AVX,
assembly, native, or vectorized vocabulary.

Validate both zero encodings, subnormals, finite extremes, infinities, positive
and negative quiet/signaling NaNs, and random raw-bit payloads; include
unaligned starts and lengths around every vector tail. Preserve a portable
scalar fallback, define alias/overlap behavior, inspect emitted instructions,
and gate allocations.

Remeasure an operation-specific serial/parallel crossover after changing the
leaf implementation class; a threshold inherited from a more expensive unary
kernel is not evidence. Use allocation-free leaf and complete allocated
operation campaigns in the same binary with alternating order. If a
unified-memory accelerator wrapper synchronously returns host tensors,
remeasure CPU-versus-device routing at production shapes—the new CPU path may
move or eliminate the device crossover. Finally gate affected workloads and
report Amdahl-limited results rather than promoting a leaf win directly.

There is NO automatic fix because Go has no portable integer-domain float SIMD
primitive and architecture dispatch, assembly support, aliases, tails, and
routing thresholds are project-specific.`,
		Before: `for i := range dst {
	dst[i] = -src[i]
}`,
		After: `// Architecture-specific candidate, not portable Go pseudocode:
outputBits := inputBits XOR signBitMask // EOR on vector lanes
// Keep the scalar Go loop as the tail/fallback; retune parallel and device gates.`,
		MeasuredWin: `On Apple M2 Pro, the 16-lane arm64 EOR implementation behind
issue #780 improved complete allocated F32 Neg operations by roughly 1.1x-3x
across 2K-8M elements. The faster host path beat direct synchronous Metal by
about 4.1x at 4M, 2.8x at 8M, and 3.0x at 16M in default and GOEXPERIMENT=simd
builds. An affected SigmoidFocalLoss workload was neutral at 349K and only
about 1.08x at 2M, demonstrating the required Amdahl/workload gate.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6063",
		Doc:  "exact scalar float negation loop has an integer sign-bit XOR SIMD form",
		Run:  runPS6063,
	},
})

type ps6063Match struct {
	dtype    types.BasicKind
	srcName  string
	destName string
}

func runPS6063(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			ast.Inspect(function.Body, func(node ast.Node) bool {
				if _, nested := node.(*ast.FuncLit); nested {
					return false
				}
				loop, ok := node.(*ast.RangeStmt)
				if !ok {
					return true
				}
				match, ok := ps6063ExactNegation(pass, loop)
				if !ok || ps6063MeasuredFallback(file, function, loop) || ps6063NativeSibling(pass, function, match.dtype) {
					return true
				}
				dtype := "float64"
				mask := "0x8000000000000000"
				if match.dtype == types.Float32 {
					dtype = "float32"
					mask = "0x80000000"
				}
				pass.Reportf(loop.For, "exact %s negation loop %s[i] -> -%s[i] -> %s[i] remains scalar; native SIMD can XOR raw lanes with %s, preserving both zero signs, infinities, signaling/quiet NaNs, and every payload bit — retain alias/tail/fallback handling, gate random raw bits and allocations, inspect instructions, derive a Neg-specific serial/parallel crossover, remeasure synchronous unified-memory CPU/device routing, and require complete-operation plus affected-workload/Amdahl evidence (advisory, no automatic fix)", dtype, match.srcName, match.srcName, match.destName, mask)
				return true
			})
		}
	}
	return nil, nil
}

func ps6063ExactNegation(pass *analysis.Pass, loop *ast.RangeStmt) (ps6063Match, bool) {
	index, ok := loop.Key.(*ast.Ident)
	if !ok || index.Name == "_" || loop.Value != nil || len(loop.Body.List) != 1 {
		return ps6063Match{}, false
	}
	indexObject := identObject(pass, index)
	if indexObject == nil {
		return ps6063Match{}, false
	}
	assignment, ok := loop.Body.List[0].(*ast.AssignStmt)
	if !ok || assignment.Tok != token.ASSIGN || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
		return ps6063Match{}, false
	}
	destination, destinationKind, destinationName, ok := ps6060IndexedSlice(pass, assignment.Lhs[0], indexObject)
	if !ok {
		return ps6063Match{}, false
	}
	negation, ok := ps2110Unparen(assignment.Rhs[0]).(*ast.UnaryExpr)
	if !ok || negation.Op != token.SUB {
		return ps6063Match{}, false
	}
	source, sourceKind, sourceName, ok := ps6060IndexedSlice(pass, negation.X, indexObject)
	if !ok || sourceKind != destinationKind || sourceKind != types.Float32 && sourceKind != types.Float64 {
		return ps6063Match{}, false
	}
	rangeID, ok := ps2110Unparen(loop.X).(*ast.Ident)
	if !ok {
		return ps6063Match{}, false
	}
	rangeObject := identObject(pass, rangeID)
	if rangeObject != source && rangeObject != destination {
		return ps6063Match{}, false
	}
	return ps6063Match{dtype: sourceKind, srcName: sourceName, destName: destinationName}, true
}

func ps6063MeasuredFallback(file *ast.File, function *ast.FuncDecl, loop *ast.RangeStmt) bool {
	for _, group := range file.Comments {
		adjacentToFunction := group.End() <= function.Pos() && function.Pos()-group.End() <= 3
		adjacentToLoop := group.End() <= loop.Pos() && loop.Pos()-group.End() <= 3
		insideFunction := group == function.Doc || adjacentToFunction || adjacentToLoop || group.Pos() >= function.Body.Pos() && group.End() <= function.Body.End()
		if !insideFunction {
			continue
		}
		for _, comment := range group.List {
			text := ps6058Compact(comment.Text)
			if strings.Contains(text, "perfscanmeasuredfallback") || strings.Contains(text, "perfscanmeasurednegfallback") || strings.Contains(text, "perfscanmeasurednegationfallback") {
				return true
			}
		}
	}
	return false
}

func ps6063NativeSibling(pass *analysis.Pass, current *ast.FuncDecl, dtype types.BasicKind) bool {
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function == current || !ps6063NativeNegName(function.Name.Name) {
				continue
			}
			if ps6060NameDType(function.Name.Name, dtype) || ps6060SignatureDType(pass, function, dtype) {
				return true
			}
		}
		found := false
		ast.Inspect(file, func(node ast.Node) bool {
			if found {
				return false
			}
			call, ok := node.(*ast.CallExpr)
			if !ok || !ps6015RegistrationCall(pass, call) {
				return true
			}
			text := ps6007NormalizeName(ps6015NodeText(pass, call))
			found = ps6063NegName(text) && ps6060NativeMarker(text) && ps6060NameDType(text, dtype)
			return !found
		})
		if found {
			return true
		}
	}
	return false
}

func ps6063NativeNegName(name string) bool {
	name = ps6007NormalizeName(name)
	return ps6063NegName(name) && ps6060NativeMarker(name)
}

func ps6063NegName(name string) bool {
	return strings.Contains(name, "neg") || strings.Contains(name, "negative")
}
