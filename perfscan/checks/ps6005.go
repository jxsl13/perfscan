package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"slices"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS6005 implements owner issue #756 for direct os/exec benchmark commands:
// workload size is explicit, but one or more recognized material semantic axes
// are left to an external accelerator benchmark's defaults.
var PS6005 = register(&lint.Check{
	ID:       "PS6005",
	Category: "verify",
	Slug:     "external-benchmark-implicit-semantics",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "an external accelerator benchmark pins workload size but leaves material semantic axes implicit",
		Text: `An external benchmark command can look reproducible while silently
using different precision, cache, attention, offload, batching, context, or
sampling defaults from the implementation it is compared against. Pinning a
model and token count is not a matched-semantics comparison if those axes
remain implicit or if the executable silently rejects/falls back from the
requested setting.

This check implements the direct os/exec form of owner issue #756. It examines
type-resolved os/exec.Command and CommandContext calls that:

  - are recognizably accelerator/ML-related from the executable, arguments, or
    enclosing function name (llama, GPU, CUDA, Metal, GGUF, safetensors, etc.);
  - explicitly set a workload-size flag such as tokens, prompt length, shape,
    or n-predict; and
  - pass arguments directly rather than through an opaque variadic slice.

It inventories eleven recognizable semantic axes: precision/dtype, K-cache type,
V-cache type, quantization, flash-attention mode, device/offload, batch size,
microbatch size, context length, warmup, and repetitions. The diagnostic names
the axes with no recognized explicit flag. Flag spellings cover common long
forms and llama-bench aliases such as -ctk, -ctv, -fa, -ngl, -b, -ub, -c, and
-r. Quantization encoded in a literal Q*/IQ* model filename is recognized.

The detector is intentionally fail-closed around an args... expansion because
the hidden slice may already contain every required flag. It raises the message
to high confidence when the surrounding function mentions a ratio, speedup,
incumbent, baseline, leadership claim, or tok/s result. Type information rejects
shadowed exec helpers and same-named user functions.

There is NO automatic fix: supported flags and correct values belong to the
specific executable and comparison contract. Pass every supported material
knob explicitly; parse machine-readable output and assert the effective values;
record the executable hash and upstream commit; and separate strict
matched-semantics rows from shipping-default rows when exact matching is
impossible. Use alternating fresh-process samples instead of comparing one
stale constant with unlike within-process statistics.`,
		Before: `cmd := exec.Command("llama-bench",
    "-m", model,
    "-n", "64",
    "-r", "5",
) // cache type, flash attention, offload, batching, context... default`,
		After: `cmd := exec.Command("llama-bench",
    "-m", model, "-n", "64", "-r", "5",
    "-ctk", "f32", "-ctv", "f32", "-fa", "0",
    "-ngl", requestedLayers, "-b", batch, "-ub", microbatch, "-c", context,
)
// Then parse the result manifest and assert every effective setting.`,
		MeasuredWin: `TinyLlama-1.1B Q4_K_M on Apple M2 Pro, llama.cpp b10450
(five alternating fresh-process samples): shipping f16-KV/FA-auto measured
197.24 tok/s; strict f32-KV/FA-off measured 179.45 tok/s. Against GoAI at
172.20 tok/s, the apparent incumbent lead changed from 1.145x to 1.042x solely
because of implicit semantic axes.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6005",
		Doc:  "external accelerator benchmark command leaves material semantic knobs implicit",
		Run:  runPS6005,
	},
})

type ps6005Axis struct {
	name  string
	flags map[string]bool
}

var ps6005Axes = []ps6005Axis{
	{name: "precision/dtype", flags: ps6005FlagSet("--dtype", "--precision", "--compute-type")},
	{name: "K-cache type", flags: ps6005FlagSet("-ctk", "--cache-type-k", "--type-k", "--kv-type")},
	{name: "V-cache type", flags: ps6005FlagSet("-ctv", "--cache-type-v", "--type-v", "--kv-type")},
	{name: "quantization", flags: ps6005FlagSet("--quant", "--quantization", "--quant-type")},
	{name: "flash-attention mode", flags: ps6005FlagSet("-fa", "--flash-attn", "--flash-attention")},
	{name: "device/offload", flags: ps6005FlagSet("-ngl", "--n-gpu-layers", "--gpu-layers", "--device", "--offload", "--split-mode")},
	{name: "batch size", flags: ps6005FlagSet("-b", "--batch-size", "--batch")},
	{name: "microbatch size", flags: ps6005FlagSet("-ub", "--ubatch-size", "--microbatch", "--micro-batch")},
	{name: "context length", flags: ps6005FlagSet("-c", "--ctx-size", "--context", "--context-length")},
	{name: "warmup", flags: ps6005FlagSet("--warmup", "--warmup-runs", "--no-warmup")},
	{name: "repetitions", flags: ps6005FlagSet("-r", "--repeat", "--repetitions", "--runs")},
}

var ps6005WorkloadFlags = ps6005FlagSet(
	"-n", "--n-predict", "--tokens", "--token-count", "-p", "--prompt-tokens", "--size", "--shape",
)

func runPS6005(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			highConfidence := ps6005ClaimSignal(fn)
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				if _, nested := node.(*ast.FuncLit); nested {
					return false
				}
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				flags, literals, ok := ps6005Command(pass, call, fn.Name.Name)
				if !ok || !ps6005HasAny(flags, ps6005WorkloadFlags) {
					return true
				}
				missing := ps6005MissingAxes(flags, literals)
				if len(missing) == 0 {
					return true
				}
				confidence := ""
				if highConfidence {
					confidence = "high-confidence comparison claim: "
				}
				pass.Report(analysis.Diagnostic{
					Pos:     call.Pos(),
					End:     call.End(),
					Message: confidence + "external accelerator benchmark pins workload size but leaves recognized semantic axes implicit (" + strings.Join(missing, ", ") + "); pass supported knobs explicitly and assert the executable's machine-readable effective settings, hash, and commit",
				})
				return true
			})
		}
	}
	return nil, nil
}

func ps6005Command(pass *analysis.Pass, call *ast.CallExpr, functionName string) (map[string]bool, []string, bool) {
	if call.Ellipsis.IsValid() {
		return nil, nil, false
	}
	fn, sig, ok := typedCallee(pass, call.Fun)
	if !ok || sig.Recv() != nil || fn.Pkg() == nil || fn.Pkg().Path() != "os/exec" ||
		(fn.Name() != "Command" && fn.Name() != "CommandContext") {
		return nil, nil, false
	}
	nameIndex := 0
	if fn.Name() == "CommandContext" {
		nameIndex = 1
	}
	if len(call.Args) <= nameIndex {
		return nil, nil, false
	}
	flags := make(map[string]bool)
	var literals []string
	for _, arg := range call.Args[nameIndex:] {
		value, ok := ps6005ConstString(pass, arg)
		if !ok {
			continue
		}
		value = strings.ToLower(value)
		literals = append(literals, value)
		if strings.HasPrefix(value, "-") {
			flag := value
			if before, _, found := strings.Cut(value, "="); found {
				flag = before
			}
			flags[flag] = true
		}
	}
	context := strings.ToLower(functionName) + " " + strings.Join(literals, " ")
	if !ps6005AcceleratorContext(context) {
		return nil, nil, false
	}
	return flags, literals, true
}

func ps6005ConstString(pass *analysis.Pass, expr ast.Expr) (string, bool) {
	tv, ok := pass.TypesInfo.Types[ps2110Unparen(expr)]
	if !ok || tv.Value == nil || tv.Value.Kind() != constant.String {
		return "", false
	}
	return constant.StringVal(tv.Value), true
}

func ps6005MissingAxes(flags map[string]bool, literals []string) []string {
	var missing []string
	for _, axis := range ps6005Axes {
		present := ps6005HasAny(flags, axis.flags)
		if axis.name == "quantization" && !present {
			present = ps6005LiteralQuantization(literals)
		}
		if !present {
			missing = append(missing, axis.name)
		}
	}
	slices.Sort(missing)
	return missing
}

func ps6005LiteralQuantization(literals []string) bool {
	for _, value := range literals {
		for _, marker := range []string{"q2_", "q3_", "q4_", "q5_", "q6_", "q8_", "iq1_", "iq2_", "iq3_", "iq4_"} {
			if strings.Contains(value, marker) {
				return true
			}
		}
	}
	return false
}

func ps6005AcceleratorContext(context string) bool {
	for _, clue := range []string{"llama", "ollama", "gpu", "cuda", "metal", "accelerator", "gguf", "safetensor", "tensor", "mlc"} {
		if strings.Contains(context, clue) {
			return true
		}
	}
	return false
}

func ps6005ClaimSignal(fn *ast.FuncDecl) bool {
	found := false
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		if found {
			return false
		}
		switch n := node.(type) {
		case *ast.Ident:
			name := strings.ToLower(n.Name)
			for _, clue := range []string{"ratio", "speedup", "incumbent", "baseline", "leader"} {
				if strings.Contains(name, clue) {
					found = true
					return false
				}
			}
		case *ast.BasicLit:
			if n.Kind != token.STRING {
				break
			}
			value := strings.ToLower(n.Value)
			for _, clue := range []string{"faster", "speedup", "lead", "tok/s"} {
				if strings.Contains(value, clue) {
					found = true
					return false
				}
			}
		}
		return true
	})
	return found
}

func ps6005FlagSet(values ...string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, value := range values {
		set[value] = true
	}
	return set
}

func ps6005HasAny(have, candidates map[string]bool) bool {
	for value := range candidates {
		if have[value] {
			return true
		}
	}
	return false
}
