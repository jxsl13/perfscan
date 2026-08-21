package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/config"
	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS6054 implements owner issue #724: a shape-keyed production selector with
// only leaf-benchmark coverage needs a caller-level promotion contract.
var PS6054 = register(&lint.Check{
	ID:          "PS6054",
	Category:    "verify",
	Slug:        "shape-selector-has-only-leaf-benchmark",
	Level:       lint.LevelStructured,
	AutoFix:     false,
	NeedsConfig: true,
	Vocab:       []string{"selectorPromotionSymbols"},
	Doc: lint.Documentation{
		Title: "a shape-keyed backend selector has leaf coverage but no end-to-end promotion contract",
		Text: `An isolated kernel crossover does not prove application leverage.
A shape/device/dtype selector can correctly pick a faster leaf and still lose
after call frequency, command scheduling, synchronization, host/GPU overlap,
and neighboring work are included.

This check implements owner issue #724. It is opt-in through
selectorPromotionSymbols. For configured selector functions, it recognizes a
narrow source shape:

  - an if/switch threshold is keyed by context, sequence, token, batch, shape,
    head, or vector dimensions;
  - different kernel/attention/decode/dispatch/implementation symbols are
    selected on opposite sides of the threshold; and
  - a repeated func BenchmarkX(*testing.B) references the selector or one of
    the selected leaf implementations.

Capability guards, dtype/correctness fallbacks, selectors with no repeated leaf
benchmark, and conditions without a shape threshold are suppressed. Type
information rejects local shadows and same-spelled unrelated symbols.

A repository can attach a trusted caller-level promotion contract in the same
selectorPromotionSymbols list using this form:

    selectAttention=>BenchmarkTinyLlamaDecode|samples=10|independent|
      alternating|equivalent-outputs|retained-shapes<=0.5%

The contract may name a benchmark in another package or an external harness.
Only an entry naming the selector and caller-level benchmark, at least ten
independent samples, alternating order, equivalent outputs, and a numeric
retained-shape regression bound silences the finding. An incomplete contract
is reported with its missing axes.

There is NO automatic fix. Perfscan cannot invent the correct caller workload,
promotion threshold, or numerical tolerance, and removing the measured leaf
selector would discard valid evidence.`,
		Before: `func selectAttention(sk int) kernel {
    if sk <= 2 { return scalarKernel }
    return stripedKernel
}
func BenchmarkAttentionLeaf(b *testing.B) { /* selector only */ }`,
		After: `# perfscan.yaml
selectorPromotionSymbols:
  - selectAttention
  - "selectAttention=>BenchmarkTinyLlamaDecode|samples=10|independent|alternating|equivalent-outputs|retained-shapes<=0.5%"`,
		MeasuredWin: `The issue #724 Metal leaf experiment won 1.333x at context
one and 1.161x at context two across ten alternating 500x process pairs. The
same <=2 selector lost ten complete TinyLlama Q4_K_M process pairs: 80.396 ms
control versus 82.907 ms candidate (0.970x), despite bit-identical logits and
neutral retained longer contexts. The selected leaf ran only twice.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6054",
		Doc:  "shape-keyed backend selector has only leaf benchmark evidence",
		Run:  runPS6054,
	},
})

var ps6054Number = regexp.MustCompile(`[0-9]+(?:\.[0-9]+)?`)

type ps6054PromotionGate struct {
	benchmark   string
	samples     int
	independent bool
	alternating bool
	outputs     bool
	retained    bool
}

type ps6054Dispatch struct {
	implementations map[types.Object]string
}

func runPS6054(pass *analysis.Pass) (any, error) {
	configured, gates := ps6054Config(config.Current().SelectorPromotionSymbols)
	if len(configured) == 0 {
		return nil, nil
	}
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || fn.Recv != nil || !configured[fn.Name.Name] {
				continue
			}
			dispatch, ok := ps6054ShapeDispatch(pass, fn)
			if !ok {
				continue
			}
			selector := pass.TypesInfo.Defs[fn.Name]
			benchmark := ps6054LeafBenchmark(pass, selector, dispatch.implementations)
			if benchmark == "" {
				continue
			}
			gate, found := gates[fn.Name.Name]
			missing := gate.missing()
			if found && len(missing) == 0 {
				continue
			}
			pass.Reportf(fn.Name.Pos(), "leaf-only shape selector %s is covered by %s but has no complete caller-level promotion contract; missing %s — require at least 10 independent order-alternating end-to-end samples, equivalent outputs, and retained-shape regression bounds before promotion", fn.Name.Name, benchmark, strings.Join(missing, ", "))
		}
	}
	return nil, nil
}

func ps6054Config(entries map[string]bool) (map[string]bool, map[string]ps6054PromotionGate) {
	selectors := make(map[string]bool, len(entries))
	gates := make(map[string]ps6054PromotionGate, len(entries))
	for entry := range entries {
		selector, contract, found := strings.Cut(entry, "=>")
		selector = strings.TrimSpace(selector)
		if selector == "" {
			continue
		}
		selectors[selector] = true
		if !found {
			continue
		}
		parts := strings.Split(contract, "|")
		gate := ps6054PromotionGate{benchmark: strings.TrimSpace(parts[0])}
		for _, raw := range parts[1:] {
			normalized := ps6054ContractToken(raw)
			switch {
			case strings.HasPrefix(normalized, "samples"):
				if text := ps6054Number.FindString(normalized); text != "" {
					gate.samples, _ = strconv.Atoi(strings.SplitN(text, ".", 2)[0])
				}
			case strings.Contains(normalized, "independent"):
				gate.independent = true
			case strings.Contains(normalized, "alternating"):
				gate.alternating = true
			case ps6007ContainsAny(normalized, "equivalentoutputs", "exactparity", "bitidenticaloutputs"):
				gate.outputs = true
			case strings.Contains(normalized, "retainedshape") && ps6054Number.MatchString(normalized):
				gate.retained = true
			}
		}
		gates[selector] = gate
	}
	return selectors, gates
}

func ps6054ContractToken(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, value)
}

func (gate ps6054PromotionGate) missing() []string {
	missing := make([]string, 0, 5)
	if gate.benchmark == "" {
		missing = append(missing, "caller-level benchmark")
	}
	if gate.samples < 10 || !gate.independent {
		missing = append(missing, "at least 10 independent samples")
	}
	if !gate.alternating {
		missing = append(missing, "order-alternating execution")
	}
	if !gate.outputs {
		missing = append(missing, "equivalent-output gate")
	}
	if !gate.retained {
		missing = append(missing, "retained-shape regression bound")
	}
	return missing
}

func ps6054ShapeDispatch(pass *analysis.Pass, fn *ast.FuncDecl) (ps6054Dispatch, bool) {
	for index, statement := range fn.Body.List {
		switch node := statement.(type) {
		case *ast.IfStmt:
			if !ps6054ShapeCondition(pass, node.Cond) {
				continue
			}
			left := ps6054Implementations(pass, node.Body)
			var right map[types.Object]string
			if node.Else != nil {
				right = ps6054Implementations(pass, node.Else)
			} else {
				right = ps6054Implementations(pass, &ast.BlockStmt{List: fn.Body.List[index+1:]})
			}
			if implementations, ok := ps6054DistinctImplementations(left, right); ok {
				return ps6054Dispatch{implementations: implementations}, true
			}
		case *ast.SwitchStmt:
			if node.Tag == nil || !ps6054ExprHasShape(pass, node.Tag) {
				continue
			}
			var branches []map[types.Object]string
			for _, statement := range node.Body.List {
				clause, ok := statement.(*ast.CaseClause)
				if ok {
					branches = append(branches, ps6054Implementations(pass, &ast.BlockStmt{List: clause.Body}))
				}
			}
			for left := 0; left < len(branches); left++ {
				for right := left + 1; right < len(branches); right++ {
					if implementations, ok := ps6054DistinctImplementations(branches[left], branches[right]); ok {
						return ps6054Dispatch{implementations: implementations}, true
					}
				}
			}
		}
	}
	return ps6054Dispatch{}, false
}

func ps6054ShapeCondition(pass *analysis.Pass, condition ast.Expr) bool {
	found := false
	ast.Inspect(condition, func(node ast.Node) bool {
		if found {
			return false
		}
		binary, ok := node.(*ast.BinaryExpr)
		if !ok || !ps6054ThresholdOperator(binary.Op) {
			return true
		}
		if ps6054ExprHasShape(pass, binary.X) && ps6054ConstantNumber(pass, binary.Y) || ps6054ExprHasShape(pass, binary.Y) && ps6054ConstantNumber(pass, binary.X) {
			found = true
			return false
		}
		return true
	})
	return found
}

func ps6054ThresholdOperator(operator token.Token) bool {
	switch operator {
	case token.LSS, token.LEQ, token.GTR, token.GEQ, token.EQL:
		return true
	default:
		return false
	}
}

func ps6054ConstantNumber(pass *analysis.Pass, expression ast.Expr) bool {
	value := pass.TypesInfo.Types[ps2110Unparen(expression)].Value
	return value != nil && (value.Kind() == constant.Int || value.Kind() == constant.Float)
}

func ps6054ExprHasShape(pass *analysis.Pass, expression ast.Expr) bool {
	found := false
	ast.Inspect(expression, func(node ast.Node) bool {
		if found {
			return false
		}
		switch value := node.(type) {
		case *ast.Ident:
			found = ps6054ShapeName(value.Name)
		case *ast.SelectorExpr:
			found = ps6054ShapeName(value.Sel.Name)
		case *ast.CallExpr:
			if id, ok := ps2110Unparen(value.Fun).(*ast.Ident); ok && id.Name == "len" && pass.TypesInfo.Uses[id] == types.Universe.Lookup("len") {
				found = true
			}
		}
		return !found
	})
	return found
}

func ps6054ShapeName(name string) bool {
	name = ps6007NormalizeName(name)
	if name == "sk" || name == "sq" || name == "dk" {
		return true
	}
	return ps6007ContainsAny(name, "context", "sequence", "seqlen", "token", "batch", "shape", "headdim", "headsize", "vectordim", "keylength", "querylength")
}

func ps6054Implementations(pass *analysis.Pass, node ast.Node) map[types.Object]string {
	implementations := make(map[types.Object]string)
	ast.Inspect(node, func(current ast.Node) bool {
		if _, nested := current.(*ast.FuncLit); nested {
			return false
		}
		call, ok := current.(*ast.CallExpr)
		if !ok {
			return true
		}
		object, name := ps6054Callee(pass, call.Fun)
		if object != nil && ps6054ImplementationName(name) {
			implementations[object] = name
		}
		return true
	})
	return implementations
}

func ps6054Callee(pass *analysis.Pass, expression ast.Expr) (types.Object, string) {
	switch value := ps2110Unparen(expression).(type) {
	case *ast.Ident:
		return pass.TypesInfo.Uses[value], value.Name
	case *ast.SelectorExpr:
		return pass.TypesInfo.Uses[value.Sel], value.Sel.Name
	default:
		return nil, ""
	}
}

func ps6054ImplementationName(name string) bool {
	name = ps6007NormalizeName(name)
	return ps6007ContainsAny(name, "kernel", "attention", "decode", "dispatch", "implementation", "impl", "scalar", "striped", "vector", "fused", "fastpath")
}

func ps6054DistinctImplementations(left, right map[types.Object]string) (map[types.Object]string, bool) {
	for object := range left {
		delete(right, object)
	}
	if len(left) == 0 || len(right) == 0 {
		return nil, false
	}
	all := make(map[types.Object]string, len(left)+len(right))
	for object, name := range left {
		all[object] = name
	}
	for object, name := range right {
		all[object] = name
	}
	return all, true
}

func ps6054LeafBenchmark(pass *analysis.Pass, selector types.Object, implementations map[types.Object]string) string {
	targets := make(map[types.Object]bool, len(implementations)+1)
	if selector != nil {
		targets[selector] = true
	}
	for object := range implementations {
		targets[object] = true
	}
	var names []string
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !ps6006Benchmark(pass, fn) || !ps6006HasLoop(fn.Body) || !ps6054UsesTarget(pass, fn.Body, targets) {
				continue
			}
			names = append(names, fn.Name.Name)
		}
	}
	if len(names) == 0 {
		return ""
	}
	slices.Sort(names)
	return names[0]
}

func ps6054UsesTarget(pass *analysis.Pass, body *ast.BlockStmt, targets map[types.Object]bool) bool {
	found := false
	ast.Inspect(body, func(node ast.Node) bool {
		if found {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		id, ok := node.(*ast.Ident)
		if ok && targets[pass.TypesInfo.Uses[id]] {
			found = true
			return false
		}
		return true
	})
	return found
}
