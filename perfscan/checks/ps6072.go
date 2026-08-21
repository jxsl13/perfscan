package checks

import (
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// These are the default Go 1.26 compiler constants in
// cmd/compile/internal/inline/inl.go. PS6072 deliberately reports a lower
// bound rather than trying to duplicate the compiler's complete IR cost model.
const (
	ps6072InlineBudget     = 80
	ps6072NoinlineCallCost = 57
)

// PS6072 implements owner issue #789. It detects a specialization wrapper
// whose explicitly non-inlineable dispatch calls alone exceed Go's default
// inline budget, and requires a same-package loop call site as hotness evidence.
var PS6072 = register(&lint.Check{
	ID:       "PS6072",
	Category: "verify",
	Slug:     "hot-noinline-specialization-wrapper",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "a hot specialization wrapper exceeds Go's inline budget through non-inlineable calls",
		Text: `Moving rank-, dtype-, or shape-specific work into helpers can make a
public wrapper look smaller while making it impossible to inline. Go 1.26's
default inliner budget is 80 and its compiler charges 57 points for each
statically non-inlineable call. Two such call sites therefore consume at least
114 points before the switch, arguments, returns, and other body nodes are
counted. The wrapper call remains in every element or token iteration, and the
new helper boundary can be pure overhead.

This check implements owner issue #789. It reports only the high-signal static
composition where:

  - a switch, type switch, or if/else chain has calls in at least two mutually
    exclusive arms;
  - those calls resolve to at least two distinct package functions or methods
    carrying an exact //go:noinline compiler directive;
  - the wrapper or target names/documentation identify a fast, rank, dtype,
    scalar/vector, shape, stride, or numeric specialization; and
  - a direct, type-resolved call to the wrapper occurs in a repeatedly evaluated
    part of a for/range loop in the same package.

The diagnostic gives the compiler's 57-point per-call charge and the resulting
call-only lower bound. Calls in a range expression or for-loop initializer are
not hotness evidence because those expressions execute only once. Calls merely
captured in a function literal created by a loop also stay silent. Generic
dispatchers without specialization evidence, sequential calls without branch
dispatch, one non-inlineable target, indirect function values, and cold
wrappers stay silent. A //perfscan:inline-budget-validated annotation records a
same-binary result that deliberately retains the shape.

There is NO automatic fix. Removing a helper, duplicating a fast arm, or
changing //go:noinline can change code size, escape behavior, recursion, and
slow-rank performance. Run ` + "`go test -gcflags=all=-m=2`" + ` to obtain the exact
cost for the active compiler. Benchmark the original and candidate in the same
binary behind the identical route, include every common and fallback rank or
dtype, and retain the specialization only when the predeclared end-to-end gate
passes. Do not infer a win from a smaller-looking wrapper or helper-level
timings alone. The static detector intentionally limits loop reachability to
the analyzed package; cross-package callers require compiler/call-graph
correlation.`,
		Before: `//go:noinline
func (t *Tensor) atRank1(i int) float64 { /* specialized work */ }
//go:noinline
func (t *Tensor) atRank2(i, j int) float64 { /* specialized work */ }

func (t *Tensor) AtF64(ix ...int) float64 {
	switch len(ix) {
	case 1: return t.atRank1(ix[0])
	case 2: return t.atRank2(ix[0], ix[1])
	default: return t.atRankN(ix)
	}
}`,
		After: `// Keep a same-binary control and candidate. Confirm exact compiler
// costs with -gcflags=all=-m=2, then retain helper extraction only if
// end-to-end loop benchmarks across common and fallback ranks pass the gate.`,
		MeasuredWin: `On Go 1.26.6/arm64, the owner candidate's three
//go:noinline rank targets gave the small public wrapper a compiler cost of 198
against the default budget of 80 (about 57 points per call). In the Apple M2
Pro same-binary campaign, the direct rank-arm candidate raised public costs to
126/130 and measured rank-N at 0.778x–0.843x and half precision at
0.891x–0.915x. The three-helper candidate raised both public costs to 198;
reads stayed near parity while writes measured 0.727x–0.883x and rank-N
0.723x–0.863x. Both candidates were reverted to the product tree.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6072",
		Doc:  "hot specialization wrapper exceeds the default inline budget through non-inlineable dispatch calls",
		Run:  runPS6072,
	},
})

type ps6072DispatchFinding struct {
	callSites int
	targets   map[*types.Func]bool
}

func runPS6072(pass *analysis.Pass) (any, error) {
	declarations := make(map[*types.Func]*ast.FuncDecl)
	noinline := make(map[*types.Func]bool)
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			object, ok := pass.TypesInfo.Defs[function.Name].(*types.Func)
			if !ok {
				continue
			}
			declarations[object] = function
			if ps6072HasNoinlineDirective(function) {
				noinline[object] = true
			}
		}
	}

	loopCalls := ps6072LoopCalls(pass, declarations)
	for object, function := range declarations {
		if loopCalls[object] == 0 || ps6072Validated(function) {
			continue
		}
		finding, ok := ps6072BestDispatch(pass, function.Body, noinline)
		if !ok || !ps6072SpecializationEvidence(function, finding.targets, declarations) {
			continue
		}
		lowerBound := finding.callSites * ps6072NoinlineCallCost
		pass.Reportf(function.Name.Pos(), "%s is called from %d loop call site(s) and dispatches through %d call site(s) to %d distinct //go:noinline specialization targets; Go 1.26's inliner charges %d per non-inlineable call, so the call-only lower bound is %d > the default budget %d before switch/body cost; inspect go test -gcflags=all=-m=2 and benchmark same-binary control/candidate paths (advisory, no automatic fix)",
			function.Name.Name,
			loopCalls[object],
			finding.callSites,
			len(finding.targets),
			ps6072NoinlineCallCost,
			lowerBound,
			ps6072InlineBudget,
		)
	}
	return nil, nil
}

func ps6072HasNoinlineDirective(function *ast.FuncDecl) bool {
	if function.Doc == nil {
		return false
	}
	for _, comment := range function.Doc.List {
		if comment.Text == "//go:noinline" {
			return true
		}
	}
	return false
}

func ps6072Validated(function *ast.FuncDecl) bool {
	if function.Doc == nil {
		return false
	}
	for _, comment := range function.Doc.List {
		if strings.Contains(comment.Text, "perfscan:inline-budget-validated") {
			return true
		}
	}
	return false
}

// ps6072LoopCalls counts direct, type-resolved wrapper call sites in repeatedly
// evaluated loop regions. A syntactic call site is counted once even when it is
// nested in multiple loops.
func ps6072LoopCalls(pass *analysis.Pass, declarations map[*types.Func]*ast.FuncDecl) map[*types.Func]int {
	calls := make(map[*types.Func]int)
	for _, file := range pass.Files {
		parents := ps6071Parents(file)
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || !ps6072RepeatedByLoop(call, parents) {
				return true
			}
			callee, _, ok := typedCallee(pass, call.Fun)
			if !ok || declarations[callee] == nil {
				return true
			}
			calls[callee]++
			return true
		})
	}
	return calls
}

func ps6072RepeatedByLoop(node ast.Node, parents map[ast.Node]ast.Node) bool {
	for parent := parents[node]; parent != nil; parent = parents[parent] {
		switch value := parent.(type) {
		case *ast.FuncLit:
			// A closure body is not executed merely because the closure literal is
			// evaluated by an enclosing loop. A loop inside the closure would have
			// appeared before this boundary and returned true already.
			return false
		case *ast.RangeStmt:
			if ps6072ContainedBy(node, value.Body) {
				return true
			}
			// The range expression is evaluated once. Continue in case this whole
			// range statement is itself nested in a repeated outer-loop region.
		case *ast.ForStmt:
			if ps6072ContainedBy(node, value.Body) ||
				ps6072ContainedBy(node, value.Cond) ||
				ps6072ContainedBy(node, value.Post) {
				return true
			}
			// The initializer is evaluated once; an outer loop may still repeat it.
		}
	}
	return false
}

func ps6072ContainedBy(node ast.Node, container ast.Node) bool {
	return node != nil && container != nil &&
		node.Pos() >= container.Pos() && node.End() <= container.End()
}

func ps6072BestDispatch(pass *analysis.Pass, body *ast.BlockStmt, noinline map[*types.Func]bool) (ps6072DispatchFinding, bool) {
	var best ps6072DispatchFinding
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		var arms []ast.Node
		switch value := node.(type) {
		case *ast.SwitchStmt:
			arms = ps6072SwitchArms(value.Body)
		case *ast.TypeSwitchStmt:
			arms = ps6072SwitchArms(value.Body)
		case *ast.IfStmt:
			arms = ps6072IfArms(value)
		default:
			return true
		}
		finding, ok := ps6072DispatchFromArms(pass, arms, noinline)
		if ok && (finding.callSites > best.callSites ||
			(finding.callSites == best.callSites && len(finding.targets) > len(best.targets))) {
			best = finding
		}
		return true
	})
	return best, best.callSites*ps6072NoinlineCallCost > ps6072InlineBudget
}

func ps6072SwitchArms(body *ast.BlockStmt) []ast.Node {
	if body == nil {
		return nil
	}
	arms := make([]ast.Node, 0, len(body.List))
	for _, statement := range body.List {
		if clause, ok := statement.(*ast.CaseClause); ok {
			arms = append(arms, clause)
		}
	}
	return arms
}

func ps6072IfArms(root *ast.IfStmt) []ast.Node {
	var arms []ast.Node
	for current := root; current != nil; {
		arms = append(arms, current.Body)
		switch alternative := current.Else.(type) {
		case *ast.IfStmt:
			current = alternative
		case *ast.BlockStmt:
			arms = append(arms, alternative)
			current = nil
		default:
			current = nil
		}
	}
	return arms
}

func ps6072DispatchFromArms(pass *analysis.Pass, arms []ast.Node, noinline map[*types.Func]bool) (ps6072DispatchFinding, bool) {
	finding := ps6072DispatchFinding{targets: make(map[*types.Func]bool)}
	activeArms := 0
	for _, arm := range arms {
		armCalls := 0
		ast.Inspect(arm, func(node ast.Node) bool {
			if _, nested := node.(*ast.FuncLit); nested {
				return false
			}
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			callee, _, ok := typedCallee(pass, call.Fun)
			if ok && noinline[callee] {
				armCalls++
				finding.targets[callee] = true
			}
			return true
		})
		if armCalls != 0 {
			activeArms++
			finding.callSites += armCalls
		}
	}
	return finding, activeArms >= 2 && len(finding.targets) >= 2
}

func ps6072SpecializationEvidence(wrapper *ast.FuncDecl, targets map[*types.Func]bool, declarations map[*types.Func]*ast.FuncDecl) bool {
	var evidence strings.Builder
	evidence.Grow(len(wrapper.Name.Name) + len(targets)*16)
	evidence.WriteString(strings.ToLower(wrapper.Name.Name))
	if wrapper.Doc != nil {
		evidence.WriteByte(' ')
		evidence.WriteString(strings.ToLower(wrapper.Doc.Text()))
	}
	for target := range targets {
		evidence.WriteByte(' ')
		evidence.WriteString(strings.ToLower(target.Name()))
		if declaration := declarations[target]; declaration != nil && declaration.Doc != nil {
			evidence.WriteByte(' ')
			evidence.WriteString(strings.ToLower(declaration.Doc.Text()))
		}
	}
	text := evidence.String()
	for _, fragment := range []string{
		"fast", "special", "rank", "dtype", "scalar", "vector", "float",
		"f16", "f32", "f64", "bf16", "half", "shape", "dimension",
		"stride", "contiguous", "fixed", "small",
	} {
		if strings.Contains(text, fragment) {
			return true
		}
	}
	return false
}
