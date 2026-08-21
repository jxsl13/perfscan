package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/config"
	"github.com/jxsl13/perfscan/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS6019 implements owner issue #767: do not materialize an O(n)
// device-resident result when its only semantic consumer selects O(k) values.
var PS6019 = register(&lint.Check{
	ID:          "PS6019",
	Category:    "verify",
	Slug:        "device-result-materialized-before-topk",
	Level:       lint.LevelStructured,
	AutoFix:     false,
	NeedsConfig: true,
	Vocab:       []string{"topKSelectorFuncs"},
	Doc: lint.Documentation{
		Title: "a device-resident vector is fully materialized before a bounded Top-K consumer",
		Text: `A device-resident result can be O(n) to copy, widen, and scan even
when the operation's semantic output is only O(k). This commonly appears in a
generation loop that downloads every logit, widens f32 to f64, applies a
bounded Top-K selector, samples one token, and discards the remaining values.

This check implements owner issue #767 with local object-flow analysis. It
requires all of these facts in one function:

  - Download, DownloadF32, ToHost, CopyToHost, Readback, or an equivalent
    device-to-host call materializes a host slice/tensor;
  - optional view extraction, aliasing, and a pure elementwise widening or
    temperature-scaling loop lead from that materialization to a configured
    topKSelectorFuncs call;
  - the configured call establishes the repository-specific bounded selector
    boundary (argmax is the k=1 case); and
  - every use of the full host value is part of that flow or a len/cap query.

The last condition is the false-positive boundary. The check stays silent when
the full vector is returned or stored, passed through a penalty/history update,
used for unrelated statistics, or consumed by a pure Top-P/full-distribution
path without a configured bounded selector. It also stays silent for resident
Top-K calls that never materialize the whole vector. A fresh materialization
whose values die in the same loop iteration is identified as higher priority.

There is NO automatic fix. Keep selection at the resident boundary and return
only O(k) indices and values, but choose the implementation by hardware: a
coherent-UMA bounded native scan may beat another GPU dispatch, while a device
reduction is usually preferable on a discrete accelerator. Preserve the full
host fallback for penalties and distribution semantics that genuinely inspect
arbitrary values, and validate exact selected-token parity before promotion.`,
		Before: `host := make([]float32, vocab)
deviceLogits.DownloadF32(host)
wide := make([]float64, len(host))
for i, value := range host {
	wide[i] = float64(value) / temperature
}
candidates := topKIndices(wide, 40)
return sample(candidates)`,
		After: `indices, values := deviceLogits.TopKN(vocab, 56)
// Apply temperature/Top-K/Top-P over only the bounded candidate superset.
return sampleTopKCandidates(indices, values)
// Keep the full-host path for penalties or unresolved distribution semantics.`,
		MeasuredWin: `In the Apple-M2 TinyLlama campaign behind issue #767, the
32,000-logit shared-memory copy alone cost 3.993 us, while host widening plus
temperature 0.8, Top-K 40, and Top-P 0.9 cost 459.297 us/token. A bounded
coherent-UMA TopKN scan returning 56 candidates measured 62.669-66.691 us and
improved ten-pair sampled generation from 153.97 to 163.67 tok/s (1.06301x)
with exact generated-token parity.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6019",
		Doc:  "full device result is materialized on the host only to select bounded Top-K values",
		Run:  runPS6019,
	},
})

type ps6019Origin struct {
	object       types.Object
	materializer *ast.CallExpr
	fresh        bool
}

func runPS6019(pass *analysis.Pass) (any, error) {
	ns := config.Current()
	if len(ns.TopKSelectorFuncs) == 0 {
		return nil, nil
	}
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			parents := ps6019Parents(fn.Body)
			fresh := ps6019FreshObjects(pass, fn.Body)
			for _, origin := range ps6019Origins(pass, fn, fresh) {
				tracked := map[types.Object]bool{origin.object: true}
				allowed := map[ast.Node]bool{origin.materializer: true}
				ps6019Derivations(pass, fn.Body, tracked, allowed)
				selectors := ps6019Selectors(pass, fn.Body, origin.materializer.Pos(), tracked, ns.TopKSelectorFuncs)
				if len(selectors) == 0 {
					continue
				}
				for _, selector := range selectors {
					allowed[selector] = true
				}
				ps6019AllowAdministrativeCalls(pass, fn.Body, tracked, allowed)
				if !ps6019OnlyFlowUses(pass, fn.Body, parents, tracked, allowed) {
					continue
				}
				selector := selectors[0]
				name := ps6019CallName(pass, selector)
				priority := ""
				if origin.fresh && ps6019SameLoop(parents, origin.materializer, selector) {
					priority = " high-priority: the fresh full host materialization dies in the same loop iteration;"
				} else if origin.fresh {
					priority = " the full host materialization is fresh and has no other consumer;"
				}
				pass.Reportf(origin.materializer.Pos(), "device-resident result %s is fully materialized on the host and only feeds configured bounded selector %s%s keep selection at the resident boundary and return O(k) indices/values; preserve a full-host fallback for penalties or full-distribution semantics and benchmark the hardware-specific implementation", origin.object.Name(), ps6019SelectorLabel(pass, selector, name), priority)
			}
		}
	}
	return nil, nil
}

func ps6019Parents(root ast.Node) map[ast.Node]ast.Node {
	parents := make(map[ast.Node]ast.Node)
	astutil.WithStack(root, func(node ast.Node, stack []ast.Node) bool {
		if len(stack) > 0 {
			parents[node] = stack[len(stack)-1]
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		return true
	})
	return parents
}

func ps6019FreshObjects(pass *analysis.Pass, body *ast.BlockStmt) map[types.Object]bool {
	fresh := make(map[types.Object]bool)
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		switch value := node.(type) {
		case *ast.AssignStmt:
			if len(value.Lhs) != len(value.Rhs) {
				return true
			}
			for i, rhs := range value.Rhs {
				id, ok := ps2110Unparen(value.Lhs[i]).(*ast.Ident)
				if ok && ps6019FreshExpr(rhs) {
					fresh[identObject(pass, id)] = true
				}
			}
		case *ast.ValueSpec:
			if len(value.Names) != len(value.Values) {
				return true
			}
			for i, rhs := range value.Values {
				if ps6019FreshExpr(rhs) {
					fresh[identObject(pass, value.Names[i])] = true
				}
			}
		}
		return true
	})
	return fresh
}

func ps6019FreshExpr(expr ast.Expr) bool {
	switch value := ps2110Unparen(expr).(type) {
	case *ast.CompositeLit:
		return true
	case *ast.CallExpr:
		id, ok := ps2110Unparen(value.Fun).(*ast.Ident)
		return ok && (id.Name == "make" || id.Name == "new")
	}
	return false
}

func ps6019Origins(pass *analysis.Pass, fn *ast.FuncDecl, fresh map[types.Object]bool) []ps6019Origin {
	byObject := make(map[types.Object]ps6019Origin)
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		switch value := node.(type) {
		case *ast.AssignStmt:
			if len(value.Rhs) != 1 {
				return true
			}
			call, ok := ps2110Unparen(value.Rhs[0]).(*ast.CallExpr)
			if !ok || !ps6019MaterializerCall(pass, fn, call) {
				return true
			}
			for _, lhs := range value.Lhs {
				id, ok := ps2110Unparen(lhs).(*ast.Ident)
				if !ok || id.Name == "_" {
					continue
				}
				object := identObject(pass, id)
				if object != nil && ps6019HostVectorType(object.Type()) {
					byObject[object] = ps6019Origin{object: object, materializer: call, fresh: true}
				}
			}
		case *ast.CallExpr:
			if !ps6019MaterializerCall(pass, fn, value) {
				return true
			}
			for _, arg := range value.Args {
				ast.Inspect(arg, func(child ast.Node) bool {
					id, ok := child.(*ast.Ident)
					if !ok {
						return true
					}
					object := pass.TypesInfo.Uses[id]
					if object != nil && ps6019HostVectorType(object.Type()) {
						byObject[object] = ps6019Origin{object: object, materializer: value, fresh: fresh[object]}
					}
					return true
				})
			}
		}
		return true
	})
	origins := make([]ps6019Origin, 0, len(byObject))
	for _, origin := range byObject {
		origins = append(origins, origin)
	}
	return origins
}

func ps6019MaterializerCall(pass *analysis.Pass, enclosing *ast.FuncDecl, call *ast.CallExpr) bool {
	fn, _, ok := typedCallee(pass, call.Fun)
	if !ok {
		return false
	}
	name := ps6007NormalizeName(fn.Name())
	strong := ps6007ContainsAny(name,
		"tohost", "copytohost", "copyfromdevice", "readback", "downloadf32", "downloadf64", "downloadu16", "downloadi32",
	)
	if !strong && !ps6007ContainsAny(name, "download", "readbuffer", "getbytes") {
		return false
	}
	deviceContext := false
	if selector, ok := ps2110Unparen(call.Fun).(*ast.SelectorExpr); ok {
		deviceContext = ps6019DeviceType(pass.TypesInfo.TypeOf(selector.X))
	}
	for _, arg := range call.Args {
		deviceContext = deviceContext || ps6019DeviceType(pass.TypesInfo.TypeOf(arg))
	}
	return strong || deviceContext || ps6013AcceleratorText(enclosing.Name.Name)
}

func ps6019DeviceType(t types.Type) bool {
	if t == nil {
		return false
	}
	text := ps6007NormalizeName(types.TypeString(t, func(*types.Package) string { return "" }))
	return ps6007ContainsAny(text, "device", "gpu", "cuda", "metal", "vulkan", "mtl", "accelerator")
}

func ps6019HostVectorType(t types.Type) bool {
	if t == nil || ps6019DeviceType(t) {
		return false
	}
	underlying := types.Unalias(t).Underlying()
	switch underlying.(type) {
	case *types.Slice, *types.Array:
		return true
	}
	text := ps6007NormalizeName(types.TypeString(t, func(*types.Package) string { return "" }))
	return ps6007ContainsAny(text, "tensor", "logits", "hostvector", "hostvalues")
}

func ps6019Derivations(pass *analysis.Pass, body *ast.BlockStmt, tracked map[types.Object]bool, allowed map[ast.Node]bool) {
	for changed := true; changed; {
		changed = false
		ast.Inspect(body, func(node ast.Node) bool {
			if _, nested := node.(*ast.FuncLit); nested {
				return false
			}
			switch value := node.(type) {
			case *ast.AssignStmt:
				if len(value.Lhs) != len(value.Rhs) {
					return true
				}
				for i, rhs := range value.Rhs {
					id, ok := ps2110Unparen(value.Lhs[i]).(*ast.Ident)
					if !ok {
						continue
					}
					object := identObject(pass, id)
					if object != nil && !tracked[object] && ps6019DerivedExpr(pass, rhs, tracked) {
						tracked[object], allowed[value], changed = true, true, true
					}
				}
			case *ast.ValueSpec:
				if len(value.Names) != len(value.Values) {
					return true
				}
				for i, rhs := range value.Values {
					object := identObject(pass, value.Names[i])
					if object != nil && !tracked[object] && ps6019DerivedExpr(pass, rhs, tracked) {
						tracked[object], allowed[value], changed = true, true, true
					}
				}
			case *ast.RangeStmt:
				if object, ok := ps6019PureTransform(pass, value.Body, value, tracked); ok && !tracked[object] {
					tracked[object], allowed[value], changed = true, true, true
				}
			case *ast.ForStmt:
				if object, ok := ps6019PureTransform(pass, value.Body, value, tracked); ok && !tracked[object] {
					tracked[object], allowed[value], changed = true, true, true
				}
			}
			return true
		})
	}
}

func ps6019DerivedExpr(pass *analysis.Pass, expr ast.Expr, tracked map[types.Object]bool) bool {
	if !ps6019HostVectorType(pass.TypesInfo.TypeOf(expr)) || !ps6019MentionsAny(pass, expr, tracked) {
		return false
	}
	switch value := ps2110Unparen(expr).(type) {
	case *ast.Ident, *ast.SliceExpr:
		return true
	case *ast.CallExpr:
		if id, ok := ps2110Unparen(value.Fun).(*ast.Ident); ok && ps6007ContainsAny(ps6007NormalizeName(id.Name), "float32", "float64") {
			return true
		}
		fn, _, ok := typedCallee(pass, value.Fun)
		if !ok {
			// A type conversion has no *types.Func. Its result type and tracked
			// argument already establish that it preserves a host-vector flow.
			return len(value.Args) == 1
		}
		name := ps6007NormalizeName(fn.Name())
		return ps6007ContainsAny(name, "storage", "data", "values", "slice", "f32", "f64", "float32", "float64")
	}
	return false
}

func ps6019PureTransform(pass *analysis.Pass, body *ast.BlockStmt, loop ast.Node, tracked map[types.Object]bool) (types.Object, bool) {
	if body == nil || len(body.List) != 1 {
		return nil, false
	}
	assign, ok := body.List[0].(*ast.AssignStmt)
	if !ok || assign.Tok != token.ASSIGN || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
		return nil, false
	}
	index, ok := ps2110Unparen(assign.Lhs[0]).(*ast.IndexExpr)
	if !ok {
		return nil, false
	}
	destID, ok := ps2110Unparen(index.X).(*ast.Ident)
	if !ok {
		return nil, false
	}
	dest := identObject(pass, destID)
	if dest == nil || tracked[dest] || !ps6019HostVectorType(dest.Type()) {
		return nil, false
	}
	locals := make(map[types.Object]bool)
	derived := ps6019MentionsAny(pass, assign.Rhs[0], tracked)
	switch value := loop.(type) {
	case *ast.RangeStmt:
		if id, ok := ps2110Unparen(value.Key).(*ast.Ident); ok {
			locals[identObject(pass, id)] = true
		}
		if id, ok := ps2110Unparen(value.Value).(*ast.Ident); ok {
			locals[identObject(pass, id)] = true
		}
		derived = derived || ps6019MentionsAny(pass, value.X, tracked) && ps6019MentionsAny(pass, assign.Rhs[0], locals)
	case *ast.ForStmt:
		if init, ok := value.Init.(*ast.AssignStmt); ok {
			for _, lhs := range init.Lhs {
				if id, ok := ps2110Unparen(lhs).(*ast.Ident); ok {
					locals[identObject(pass, id)] = true
				}
			}
		}
	}
	if !derived || !ps6019ScalarTransform(pass, assign.Rhs[0], tracked, locals) {
		return nil, false
	}
	return dest, true
}

func ps6019ScalarTransform(pass *analysis.Pass, expr ast.Expr, tracked, locals map[types.Object]bool) bool {
	valid := true
	ast.Inspect(expr, func(node ast.Node) bool {
		if !valid {
			return false
		}
		switch value := node.(type) {
		case *ast.CallExpr:
			id, ok := ps2110Unparen(value.Fun).(*ast.Ident)
			if !ok || !ps6007ContainsAny(ps6007NormalizeName(id.Name), "float32", "float64") {
				valid = false
				return false
			}
		case *ast.Ident:
			object := pass.TypesInfo.Uses[value]
			if object == nil || tracked[object] || locals[object] {
				return true
			}
			if _, basic := types.Unalias(object.Type()).Underlying().(*types.Basic); !basic {
				valid = false
				return false
			}
		}
		return true
	})
	return valid
}

func ps6019Selectors(pass *analysis.Pass, body *ast.BlockStmt, after token.Pos, tracked map[types.Object]bool, configured map[string]bool) []*ast.CallExpr {
	var selectors []*ast.CallExpr
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok || call.Pos() <= after || !ps6019MentionsAny(pass, call, tracked) {
			return true
		}
		fn, _, ok := typedCallee(pass, call.Fun)
		if ok && configured[fn.Name()] {
			selectors = append(selectors, call)
			return false
		}
		return true
	})
	return selectors
}

func ps6019CallName(pass *analysis.Pass, call *ast.CallExpr) string {
	if fn, _, ok := typedCallee(pass, call.Fun); ok {
		return fn.Name()
	}
	return "Top-K"
}

func ps6019SelectorLabel(pass *analysis.Pass, call *ast.CallExpr, name string) string {
	normalized := ps6007NormalizeName(name)
	if strings.Contains(normalized, "argmax") {
		return name + " (k=1)"
	}
	for i := len(call.Args) - 1; i >= 0; i-- {
		tv, ok := pass.TypesInfo.Types[ps2110Unparen(call.Args[i])]
		if !ok || tv.Value == nil || tv.Value.Kind() != constant.Int || constant.Sign(tv.Value) <= 0 {
			continue
		}
		if value, ok := constant.Int64Val(tv.Value); ok {
			return name + " (k=" + strconv.FormatInt(value, 10) + ")"
		}
	}
	return name
}

func ps6019AllowAdministrativeCalls(pass *analysis.Pass, body *ast.BlockStmt, tracked map[types.Object]bool, allowed map[ast.Node]bool) {
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok || !ps6019MentionsAny(pass, call, tracked) {
			return true
		}
		if id, ok := ps2110Unparen(call.Fun).(*ast.Ident); ok && (id.Name == "len" || id.Name == "cap") {
			allowed[call] = true
			return false
		}
		return true
	})
}

func ps6019OnlyFlowUses(pass *analysis.Pass, body *ast.BlockStmt, parents map[ast.Node]ast.Node, tracked map[types.Object]bool, allowed map[ast.Node]bool) bool {
	safe := true
	ast.Inspect(body, func(node ast.Node) bool {
		if !safe {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		id, ok := node.(*ast.Ident)
		if !ok || !tracked[pass.TypesInfo.Uses[id]] {
			return true
		}
		for ancestor := ast.Node(id); ancestor != nil; ancestor = parents[ancestor] {
			if allowed[ancestor] {
				return true
			}
		}
		safe = false
		return false
	})
	return safe
}

func ps6019MentionsAny(pass *analysis.Pass, node ast.Node, objects map[types.Object]bool) bool {
	found := false
	ast.Inspect(node, func(child ast.Node) bool {
		if found {
			return false
		}
		id, ok := child.(*ast.Ident)
		if ok && objects[pass.TypesInfo.Uses[id]] {
			found = true
			return false
		}
		return true
	})
	return found
}

func ps6019SameLoop(parents map[ast.Node]ast.Node, left, right ast.Node) bool {
	nearest := func(node ast.Node) ast.Node {
		for parent := parents[node]; parent != nil; parent = parents[parent] {
			if astutil.IsLoop(parent) {
				return parent
			}
		}
		return nil
	}
	loop := nearest(left)
	return loop != nil && loop == nearest(right)
}
