package checks

import (
	"cmp"
	"go/ast"
	"go/token"
	"go/types"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/lint"
)

// PS6020 implements owner issue #766: price the layout-only dispatches created
// by a producer fusion before treating its leaf speedup as pipeline leverage.
var PS6020 = register(&lint.Check{
	ID:       "PS6020",
	Category: "verify",
	Slug:     "fusion-followed-by-layout-debt",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "a fused producer is followed by multiple layout-only operations over its output",
		Text: `A fused producer can win its leaf benchmark while losing most or all
of that leverage to the output layout it creates. A combined projection, GEMM,
or graph node often emits canonical packed bands; independently slicing,
transposing, materializing, or strided-copying those bands adds command
submissions and memory traffic that the unfused producer did not need.

This check implements owner issue #766. Within one lexical block it links:

  - a fused, grouped, mixed, combined, packed, or heterogeneous compute call;
  - the buffer/tensor objects passed to or returned by that producer; and
  - two or more later layout-only calls touching those same objects.

Typed names such as Copy2D, Blit, Transpose, Permute, Reshape, Slice,
Materialize, Contiguous, Reorder, Unpack, and Scatter identify ordinary layout
work. Project vocabulary extends the matcher: pureComputeFuncs can identify a
repository producer, layoutOpConstants identify layout operations passed to
Execute/Dispatch-style wrappers, and variadicDispatchWrappers identify those
wrappers. A fused scatter/split epilogue such as RoPEPairSplit is treated as the
solution boundary rather than another debt call.

Object identity prevents unrelated copies elsewhere in the function from
being charged to the fusion. One layout operation stays silent because a
single conversion may be an unavoidable contract boundary. Explicit comments
documenting a required public layout contract, a measured retained winner, or
the absence of a legal fused epilogue also stay silent.

There is NO automatic fix. Include all post-op materialization in the affected
pipeline benchmark. When semantics permit, fuse an existing epilogue with the
scatter into consumer layouts, but prove exact or explicitly bounded numerical
behavior and rerun fresh end-to-end promotion gates. Leaf speedup is not
pipeline leverage until the layout debt is priced.`,
		Before: `groupedProjection(input, packedQKV) // fast fused producer
ropePair(packedQKV)
copy2D(packedQKV, q)
copy2D(packedQKV, k)
copy2D(packedQKV, v)`,
		After: `groupedProjection(input, packedQKV)
ropePairSplit(packedQKV, q, k, v) // rotate q/k and scatter q/k/v once
// Benchmark producer + epilogue + consumer pipeline, not producer alone.`,
		MeasuredWin: `In the Apple-M2 mixed-QKV campaign behind issue #766, the
fused ten-GEMM stage improved 1.7308x at M64 and 1.2160x at M512, but three
standalone strided Copy2D dispatches left pp64 at only 1.0241x and regressed
pp512 to 0.9750x. Fusing RoPE with q/k/v scatter removed the copies; three
fresh-decoder campaigns then measured pp64 at 1.0408x-1.0572x and pp512 at
1.0093x-1.0164x while tg64 remained non-regressing.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6020",
		Doc:  "fused producer output incurs multiple post-op layout-only calls",
		Run:  runPS6020,
	},
})

type ps6020Call struct {
	call   *ast.CallExpr
	name   string
	block  *ast.BlockStmt
	assets map[types.Object]bool
}

func runPS6020(pass *analysis.Pass) (any, error) {
	ns := config.Current()
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			text := ps6015FunctionText(pass, file, fn)
			if !ps6020FusionContext(text) || ps6020Retention(text) {
				continue
			}
			parents := ps6019Parents(fn.Body)
			calls := ps6020Calls(pass, fn.Body, parents)
			for i, producer := range calls {
				if producer.block == nil || len(producer.assets) == 0 || !ps6020Producer(pass, producer.call, producer.name, text, &ns) {
					continue
				}
				var layouts []ps6020Call
				for _, candidate := range calls[i+1:] {
					if candidate.block != producer.block {
						continue
					}
					if ps6020Producer(pass, candidate.call, candidate.name, text, &ns) {
						break
					}
					if ps6020LayoutCall(pass, candidate.call, candidate.name, &ns) && ps6020Intersects(producer.assets, candidate.assets) {
						layouts = append(layouts, candidate)
					}
				}
				if len(layouts) < 2 {
					continue
				}
				pass.Reportf(producer.call.Pos(), "fused producer %s is followed by %d layout-only operations over the same buffer/tensor objects (%s); include those calls in the leaf-to-pipeline leverage benchmark and consider a semantics-proven fused epilogue/scatter before promotion", producer.name, len(layouts), ps6020LayoutSummary(layouts))
			}
		}
	}
	return nil, nil
}

func ps6020FusionContext(text string) bool {
	normalized := ps6007NormalizeName(text)
	fusion := ps6007ContainsAny(normalized, "fused", "fusion", "grouped", "combined", "heterogeneous", "mixed", "packed")
	compute := ps6007ContainsAny(normalized, "qkv", "projection", "matmul", "gemm", "compute", "producer", "graph")
	return fusion && compute
}

func ps6020Retention(text string) bool {
	normalized := strings.ToLower(text)
	return ps6007ContainsAny(normalized,
		"required public layout contract",
		"public output layout is required",
		"measured retained layout winner",
		"no legal fused epilogue",
		"layout debt measured and retained",
	)
}

func ps6020Calls(pass *analysis.Pass, body *ast.BlockStmt, parents map[ast.Node]ast.Node) []ps6020Call {
	var calls []ps6020Call
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		fn, _, ok := typedCallee(pass, call.Fun)
		if !ok {
			return true
		}
		calls = append(calls, ps6020Call{
			call:   call,
			name:   fn.Name(),
			block:  ps6020Block(parents, call),
			assets: ps6020Assets(pass, call, parents),
		})
		return true
	})
	slices.SortFunc(calls, func(left, right ps6020Call) int {
		return cmp.Compare(left.call.Pos(), right.call.Pos())
	})
	return calls
}

func ps6020Block(parents map[ast.Node]ast.Node, node ast.Node) *ast.BlockStmt {
	for parent := parents[node]; parent != nil; parent = parents[parent] {
		if block, ok := parent.(*ast.BlockStmt); ok {
			return block
		}
	}
	return nil
}

func ps6020Assets(pass *analysis.Pass, call *ast.CallExpr, parents map[ast.Node]ast.Node) map[types.Object]bool {
	assets := make(map[types.Object]bool)
	add := func(node ast.Node) {
		ast.Inspect(node, func(child ast.Node) bool {
			id, ok := child.(*ast.Ident)
			if !ok {
				return true
			}
			object := pass.TypesInfo.Uses[id]
			if object != nil && ps6020DataObject(object) {
				assets[object] = true
			}
			return true
		})
	}
	if selector, ok := ps2110Unparen(call.Fun).(*ast.SelectorExpr); ok {
		add(selector.X)
	}
	for _, arg := range call.Args {
		add(arg)
	}
	for parent := parents[call]; parent != nil; parent = parents[parent] {
		switch value := parent.(type) {
		case *ast.AssignStmt:
			for _, lhs := range value.Lhs {
				id, ok := ps2110Unparen(lhs).(*ast.Ident)
				if !ok {
					continue
				}
				object := identObject(pass, id)
				if object != nil && ps6020DataObject(object) {
					assets[object] = true
				}
			}
			return assets
		case *ast.ValueSpec:
			for _, name := range value.Names {
				object := identObject(pass, name)
				if object != nil && ps6020DataObject(object) {
					assets[object] = true
				}
			}
			return assets
		case ast.Stmt:
			return assets
		}
	}
	return assets
}

func ps6020DataObject(object types.Object) bool {
	if object == nil || object.Type() == nil {
		return false
	}
	switch types.Unalias(object.Type()).Underlying().(type) {
	case *types.Slice, *types.Array:
		return true
	}
	name := ps6007NormalizeName(types.TypeString(object.Type(), func(*types.Package) string { return "" }))
	if ps6007ContainsAny(name, "recorder", "context", "operation", "dtype", "shape") {
		return false
	}
	return ps6007ContainsAny(name, "buffer", "buf", "tensor", "storage", "vector", "matrix", "logit", "qkv")
}

func ps6020Producer(pass *analysis.Pass, call *ast.CallExpr, callName, context string, ns *config.Sets) bool {
	if ns.PureComputeFuncs[callName] {
		return true
	}
	name := ps6007NormalizeName(callName)
	fused := ps6007ContainsAny(name, "fused", "grouped", "combined", "heterogeneous", "mixed", "packed")
	compute := ps6007ContainsAny(name, "qkv", "projection", "project", "matmul", "gemm", "compute", "producer", "graph")
	if fused && compute {
		return true
	}
	if ps6020FusionContext(context) && ps6007ContainsAny(name, "record", "project", "matmul", "gemm", "qkv") {
		return !ps6020LayoutCall(pass, call, callName, ns)
	}
	return false
}

func ps6020LayoutCall(pass *analysis.Pass, call *ast.CallExpr, callName string, ns *config.Sets) bool {
	name := ps6007NormalizeName(callName)
	if ps6007ContainsAny(name, "ropepairsplit", "fusedscatter", "fusedlayout", "fusedepilogue") {
		return false
	}
	if ps6007ContainsAny(name,
		"copy2d", "stridedcopy", "blit", "transpose", "permute", "reshape", "materialize",
		"contiguous", "reorder", "unpack", "slice", "split", "scatterbands",
	) || name == "copy" || name == "scatter" {
		return true
	}
	if !ps6020ConfiguredLayout(pass, call, ns.LayoutOpConstants) {
		return false
	}
	return ns.VariadicDispatchWrappers[callName] || ps6007ContainsAny(name, "execute", "dispatch", "submit", "runop", "exec")
}

func ps6020ConfiguredLayout(pass *analysis.Pass, call *ast.CallExpr, configured map[string]bool) bool {
	if len(configured) == 0 {
		return false
	}
	found := false
	for _, arg := range call.Args {
		ast.Inspect(arg, func(node ast.Node) bool {
			if found {
				return false
			}
			switch value := node.(type) {
			case *ast.Ident:
				object := pass.TypesInfo.Uses[value]
				found = configured[value.Name] || object != nil && configured[object.Name()]
			case *ast.BasicLit:
				if value.Kind == token.STRING {
					text, err := strconv.Unquote(value.Value)
					found = err == nil && configured[text]
				}
			}
			return !found
		})
		if found {
			return true
		}
	}
	return false
}

func ps6020Intersects(left, right map[types.Object]bool) bool {
	for object := range left {
		if right[object] {
			return true
		}
	}
	return false
}

func ps6020LayoutSummary(layouts []ps6020Call) string {
	counts := make(map[string]int)
	var order []string
	for _, layout := range layouts {
		if counts[layout.name] == 0 {
			order = append(order, layout.name)
		}
		counts[layout.name]++
	}
	parts := make([]string, 0, len(order))
	for _, name := range order {
		if counts[name] == 1 {
			parts = append(parts, name)
		} else {
			parts = append(parts, name+" x"+strconv.Itoa(counts[name]))
		}
	}
	return strings.Join(parts, ", ")
}
