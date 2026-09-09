package checks

import (
	"go/ast"
	"go/constant"
	"go/types"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/lint"
)

// PS6120 implements owner issue #821. It finds a concrete repeated
// construction -> upload -> dispatch chain; names alone are never evidence.
var PS6120 = register(&lint.Check{
	ID: "PS6120", Category: "verify", Slug: "repeated-resident-codebook-construction",
	Level: lint.LevelAggressive, AutoFix: false, NeedsConfig: true,
	Vocab: []string{"residentCodebookContracts"},
	Doc: lint.Documentation{
		Title: "a small invariant accelerator codebook is rebuilt and uploaded in a repeated dispatch path",
		Text: `Small immutable quantization codebooks should not be reconstructed as wide
floating-point tables for every dispatch or output. PS6120 requires an exact
configured producer/upload/dispatch boundary and proves a direct per-iteration
data flow in Go source. Fixed loops prove at least two trips; dynamic ranges
are worded conditionally because a particular slice may contain zero or one
element. An inline literal must contain only finite constants, at
least two distinct values, and fit the configured compact byte ceiling. An
external producer (such as a public dequantization oracle) is accepted only
when its exactness, invariance, and fresh wide result are explicitly part of
the boundary contract; dimensions and packing bounds must still be configured
conservatively and reviewed against its source.

The matcher rejects aliases, rebinding, address-taking, mutation, extra uses,
dynamic calls, closures, go/defer, branches, and ambiguous sites. Construction
behind sync.Once or outside the repeated loop stays silent. Embedded native
shader internals are beyond Go AST visibility: configure the Go upload and
launch wrappers, then review the shader registry separately; configuration is
not evidence that an unobserved native reconstruction exists.

There is NO automatic fix and the diagnostic does not claim a win. Construct
the exact codebook once, prove a lossless compact encoding, give the device
buffer explicit immutable lifetime ownership, and decode cheaply in the
kernel. Preserve transfer-bound host fallbacks. Gate the resident path against
both its prior resident implementation and the host-transfer route with paired,
order-alternating end-to-end measurements plus numerical and lifecycle checks.`,
		Before: `for row := 0; row < 4; row++ {
	wide := exactCodebook()
	device := backend.Upload(wide)
	backend.Dispatch(device, input, output, row)
}`,
		After: `// Once per backend/session lifetime, outside the dispatch loop:
packed := buildExactPackedCodebook()
resident := backend.Upload(packed)
// Dispatches reuse resident; retain the host fallback and validate both routes.`,
		MeasuredWin: `GoAI PR #1160, merged as 04838955aa12ce709c46f5a3cea22446534b7cb4,
used an exact 1024 x 8 ternary IQ2_S codebook packed to 2 KiB on Apple M2 Pro.
Across three fresh-process count-seven campaigns the cooperative/scalar GPU
floor was 4.278x, recorder-wall floor 3.204x, and resident Metal/fused ARM64
CPU wall floor 2.302x; scalar-control drift stayed at or below 1.091x. The
direct host-input/output route did not reproduce the win and remained on CPU.
The shipped upload is protected by sync.Once and therefore is not a finding.`,
	},
	Analyzer: &analysis.Analyzer{Name: "PS6120", Doc: "repeated construction and upload of a tiny invariant accelerator codebook", Run: runPS6120},
})

func runPS6120(pass *analysis.Pass) (any, error) {
	return runPS6120WithContracts(pass, config.Current().ResidentCodebookContracts)
}

func runPS6120WithContracts(pass *analysis.Pass, configured []config.ResidentCodebookContract) (any, error) {
	declarations := ps6115Declarations(pass)
	for _, contract := range ps6120Contracts(configured) {
		declaration := declarations[contract.ConfiguredSite]
		if declaration == nil || ps6113GenericFunction(pass, declaration) {
			continue
		}
		file := ps6115FileContaining(pass, declaration)
		matches := ps6120Matches(pass, file, declaration, contract)
		if len(matches) != 1 {
			continue
		}
		match := matches[0]
		path := "inside a repeated dispatch path"
		if !match.tripCountProven {
			path = "inside a dispatch loop that may execute repeatedly (once per ranged element)"
		}
		pass.Report(analysis.Diagnostic{Pos: match.construct.Pos(), End: match.construct.End(),
			Message: contract.Name + ": invariant codebook is freshly materialized and uploaded " + path + " (compact ceiling " + strconv.FormatInt(contract.MaxCodebookBytes, 10) + " bytes); construct and losslessly pack it once, keep the immutable device buffer resident with explicit lifetime ownership, preserve the host-transfer fallback, and benchmark resident and host routes separately (PS6120 advisory, no automatic fix)",
			Related: []analysis.RelatedInformation{{Pos: match.upload.Pos(), End: match.upload.End(), Message: "fresh wide codebook crosses the configured synchronous upload boundary"}, {Pos: match.dispatch.Pos(), End: match.dispatch.End(), Message: "configured dispatch consumes only the freshly uploaded device buffer"}},
		})
	}
	return nil, nil
}

type ps6120Match struct {
	construct        ast.Expr
	upload, dispatch *ast.CallExpr
	tripCountProven  bool
}

func ps6120Contracts(in []config.ResidentCodebookContract) []*config.ResidentCodebookContract {
	names, claims := map[string]int{}, map[string]int{}
	for i := range in {
		if in[i].Valid() {
			names[in[i].Name]++
			claims[in[i].ConfiguredSite+"\x00"+in[i].UploadCallable+"\x00"+in[i].DispatchCallable]++
		}
	}
	var out []*config.ResidentCodebookContract
	for i := range in {
		c := &in[i]
		if c.Valid() && names[c.Name] == 1 && claims[c.ConfiguredSite+"\x00"+c.UploadCallable+"\x00"+c.DispatchCallable] == 1 {
			out = append(out, c)
		}
	}
	slices.SortFunc(out, func(a, b *config.ResidentCodebookContract) int { return strings.Compare(a.Name, b.Name) })
	return out
}

func ps6120Matches(pass *analysis.Pass, file *ast.File, declaration *ast.FuncDecl, c *config.ResidentCodebookContract) []ps6120Match {
	var result []ps6120Match
	unreachable := ps2144Unreachable(pass, declaration.Body)
	ast.Inspect(declaration.Body, func(node ast.Node) bool {
		if _, closure := node.(*ast.FuncLit); closure {
			return false
		}
		var body *ast.BlockStmt
		proven := true
		switch loop := node.(type) {
		case *ast.ForStmt:
			if ps6120RepeatedFor(pass, loop) {
				body = loop.Body
			}
		case *ast.RangeStmt:
			if eligible, fixed := ps6120RepeatedRange(pass, loop); eligible {
				body = loop.Body
				proven = fixed
			}
		}
		if body == nil || ps2144PositionIn(body.Pos(), unreachable) {
			return true
		}
		if match, ok := ps6120Loop(pass, file, body, c); ok {
			match.tripCountProven = proven
			result = append(result, match)
			return false
		}
		return true
	})
	return result
}

func ps6120RepeatedFor(pass *analysis.Pass, loop *ast.ForStmt) bool {
	initial, ok := loop.Init.(*ast.AssignStmt)
	if !ok || initial.Tok.String() != ":=" || len(initial.Lhs) != 1 || len(initial.Rhs) != 1 || !ps6118IntegerConstant(pass, initial.Rhs[0], 0) {
		return false
	}
	id, ok := initial.Lhs[0].(*ast.Ident)
	if !ok {
		return false
	}
	object := pass.TypesInfo.Defs[id]
	if object == nil {
		return false
	}
	condition, ok := ps2110Unparen(loop.Cond).(*ast.BinaryExpr)
	if !ok || condition.Op.String() != "<" || ps6114ExprObject(pass, condition.X) != object {
		return false
	}
	value := pass.TypesInfo.Types[ps2110Unparen(condition.Y)].Value
	if value == nil || value.Kind() != constant.Int {
		return false
	}
	count, exact := constant.Int64Val(value)
	if !exact || count < 2 {
		return false
	}
	post, ok := loop.Post.(*ast.IncDecStmt)
	return ok && post.Tok.String() == "++" && ps6114ExprObject(pass, post.X) == object && !ps6120ObjectMutated(pass, loop.Body, object)
}

func ps6120ObjectMutated(pass *analysis.Pass, node ast.Node, object types.Object) bool {
	mutated := false
	ast.Inspect(node, func(n ast.Node) bool {
		if mutated {
			return false
		}
		switch value := n.(type) {
		case *ast.AssignStmt:
			for _, lhs := range value.Lhs {
				if ps6114ExprObject(pass, lhs) == object {
					mutated = true
				}
			}
		case *ast.IncDecStmt:
			mutated = ps6114ExprObject(pass, value.X) == object
		case *ast.UnaryExpr:
			if value.Op.String() == "&" && ps6114ExprObject(pass, value.X) == object {
				mutated = true
			}
		}
		return !mutated
	})
	return mutated
}

func ps6120RepeatedRange(pass *analysis.Pass, loop *ast.RangeStmt) (bool, bool) {
	t := pass.TypesInfo.TypeOf(loop.X)
	if t == nil {
		return false, false
	}
	switch value := t.Underlying().(type) {
	case *types.Array:
		return value.Len() >= 2, value.Len() >= 2
	case *types.Slice, *types.Map, *types.Chan:
		// Dynamic ranges are conditional candidates: source proves that the
		// expensive chain repeats per element, not that this invocation has >1.
		return true, false
	case *types.Basic:
		if value.Info()&types.IsString != 0 {
			return true, false
		}
		v := pass.TypesInfo.Types[ps2110Unparen(loop.X)].Value
		if value.Info()&types.IsInteger == 0 || v == nil {
			return false, false
		}
		n, exact := constant.Int64Val(v)
		return exact && n >= 2, exact && n >= 2
	}
	return false, false
}

func ps6120Loop(pass *analysis.Pass, file *ast.File, body *ast.BlockStmt, c *config.ResidentCodebookContract) (ps6120Match, bool) {
	if len(body.List) == 5 {
		return ps6120CheckedLoop(pass, file, body, c)
	}
	if len(body.List) != 3 {
		return ps6120Match{}, false
	}
	first, ok := body.List[0].(*ast.AssignStmt)
	if !ok || first.Tok.String() != ":=" || len(first.Lhs) != 1 || len(first.Rhs) != 1 {
		return ps6120Match{}, false
	}
	wideID, ok := first.Lhs[0].(*ast.Ident)
	if !ok {
		return ps6120Match{}, false
	}
	wide := pass.TypesInfo.Defs[wideID]
	if wide == nil {
		return ps6120Match{}, false
	}
	if !ps6120Construction(pass, file, first.Rhs[0], c) {
		return ps6120Match{}, false
	}
	second, ok := body.List[1].(*ast.AssignStmt)
	if !ok || second.Tok.String() != ":=" || len(second.Lhs) != 1 || len(second.Rhs) != 1 {
		return ps6120Match{}, false
	}
	deviceID, ok := second.Lhs[0].(*ast.Ident)
	if !ok {
		return ps6120Match{}, false
	}
	device := pass.TypesInfo.Defs[deviceID]
	if device == nil || ps6091ErrorType(device.Type()) || !ps6120DeviceHandle(device.Type()) {
		return ps6120Match{}, false
	}
	upload, ok := ps2110Unparen(second.Rhs[0]).(*ast.CallExpr)
	if !ok || !ps6115DirectCall(pass, file, upload, c.UploadCallable) || !ps6120ResultCount(pass, upload, 1) || !ps6120ArgObject(pass, upload, c.UploadDataArgument, wide) || !ps6120ExactParam(pass, upload, c.UploadDataArgument, wide.Type()) {
		return ps6120Match{}, false
	}
	expression, ok := body.List[2].(*ast.ExprStmt)
	if !ok {
		return ps6120Match{}, false
	}
	dispatch, ok := ps2110Unparen(expression.X).(*ast.CallExpr)
	if !ok || !ps6115DirectCall(pass, file, dispatch, c.DispatchCallable) || !ps6120ResultCount(pass, dispatch, 0) || !ps6120ArgObject(pass, dispatch, c.DispatchBufferArgument, device) || !ps6120ExactParam(pass, dispatch, c.DispatchBufferArgument, device.Type()) {
		return ps6120Match{}, false
	}
	if ps6120ObjectUses(pass, body, wide) != 2 || ps6120ObjectUses(pass, body, device) != 2 {
		return ps6120Match{}, false
	}
	return ps6120Match{construct: first.Rhs[0], upload: upload, dispatch: dispatch}, true
}

func ps6120CheckedLoop(pass *analysis.Pass, file *ast.File, body *ast.BlockStmt, c *config.ResidentCodebookContract) (ps6120Match, bool) {
	producerAssign, ok := body.List[0].(*ast.AssignStmt)
	if !ok || producerAssign.Tok.String() != ":=" || len(producerAssign.Lhs) != 2 || len(producerAssign.Rhs) != 1 {
		return ps6120Match{}, false
	}
	wideID, wok := producerAssign.Lhs[0].(*ast.Ident)
	errID, eok := producerAssign.Lhs[1].(*ast.Ident)
	producer, cok := ps2110Unparen(producerAssign.Rhs[0]).(*ast.CallExpr)
	if !wok || !eok || !cok || c.ProducerCallable == "" || !ps6115DirectCall(pass, file, producer, c.ProducerCallable) || len(producer.Args) != 0 || !ps6118ErrorResult(pass, producer, 2) {
		return ps6120Match{}, false
	}
	wide, producerErr := pass.TypesInfo.Defs[wideID], pass.TypesInfo.Defs[errID]
	if wide == nil || producerErr == nil || !ps6120WideResult(pass, producer, 1, c) || !ps6118ReturnedErrorGuard(pass, body.List[1], producerErr, nil) {
		return ps6120Match{}, false
	}
	uploadAssign, ok := body.List[2].(*ast.AssignStmt)
	if !ok || uploadAssign.Tok.String() != ":=" || len(uploadAssign.Lhs) != 2 || len(uploadAssign.Rhs) != 1 {
		return ps6120Match{}, false
	}
	deviceID, dok := uploadAssign.Lhs[0].(*ast.Ident)
	uploadErrID, ueok := uploadAssign.Lhs[1].(*ast.Ident)
	upload, uok := ps2110Unparen(uploadAssign.Rhs[0]).(*ast.CallExpr)
	if !dok || !ueok || !uok || !ps6115DirectCall(pass, file, upload, c.UploadCallable) || !ps6120ArgObject(pass, upload, c.UploadDataArgument, wide) || !ps6120ExactParam(pass, upload, c.UploadDataArgument, wide.Type()) || !ps6118ErrorResult(pass, upload, 2) {
		return ps6120Match{}, false
	}
	device, uploadErr := pass.TypesInfo.Defs[deviceID], pass.TypesInfo.ObjectOf(uploadErrID)
	if device == nil || uploadErr == nil || ps6091ErrorType(device.Type()) || !ps6120DeviceHandle(device.Type()) || !ps6118ReturnedErrorGuard(pass, body.List[3], uploadErr, nil) {
		return ps6120Match{}, false
	}
	guard, ok := body.List[4].(*ast.IfStmt)
	if !ok {
		return ps6120Match{}, false
	}
	init, ok := guard.Init.(*ast.AssignStmt)
	if !ok || init.Tok.String() != ":=" || len(init.Lhs) != 1 || len(init.Rhs) != 1 {
		return ps6120Match{}, false
	}
	dispatchErrID, ok := init.Lhs[0].(*ast.Ident)
	dispatch, okCall := ps2110Unparen(init.Rhs[0]).(*ast.CallExpr)
	dispatchErr := pass.TypesInfo.Defs[dispatchErrID]
	if !ok || !okCall || dispatchErr == nil || !ps6115DirectCall(pass, file, dispatch, c.DispatchCallable) || !ps6120ArgObject(pass, dispatch, c.DispatchBufferArgument, device) || !ps6120ExactParam(pass, dispatch, c.DispatchBufferArgument, device.Type()) || !ps6118ErrorResult(pass, dispatch, 1) || !ps6118ReturnedErrorGuard(pass, body.List[4], dispatchErr, init) {
		return ps6120Match{}, false
	}
	if ps6120ObjectUses(pass, body, wide) != 2 || ps6120ObjectUses(pass, body, device) != 2 {
		return ps6120Match{}, false
	}
	return ps6120Match{construct: producer, upload: upload, dispatch: dispatch}, true
}

func ps6120WideResult(pass *analysis.Pass, call *ast.CallExpr, position int, c *config.ResidentCodebookContract) bool {
	value := ps6118ResultType(pass, call, position)
	if value == nil {
		return false
	}
	slice, ok := types.Unalias(value).Underlying().(*types.Slice)
	if !ok {
		return false
	}
	basic, ok := types.Unalias(slice.Elem()).Underlying().(*types.Basic)
	return ok && basic.Info()&types.IsFloat != 0 && pass.TypesSizes.Sizeof(slice.Elem()) == c.WideElementBytes
}

func ps6120ResultCount(pass *analysis.Pass, call *ast.CallExpr, expected int) bool {
	_, signature, ok := typedCallee(pass, call.Fun)
	return ok && signature != nil && signature.Results().Len() == expected
}

func ps6120ExactParam(pass *analysis.Pass, call *ast.CallExpr, position int, expected types.Type) bool {
	_, signature, ok := typedCallee(pass, call.Fun)
	index := position - 1
	return ok && signature != nil && index >= 0 && index < signature.Params().Len() && types.Identical(signature.Params().At(index).Type(), expected)
}

func ps6120DeviceHandle(value types.Type) bool {
	_, pointer := types.Unalias(value).(*types.Pointer)
	return pointer
}

func ps6120Construction(pass *analysis.Pass, file *ast.File, expression ast.Expr, c *config.ResidentCodebookContract) bool {
	if call, ok := ps2110Unparen(expression).(*ast.CallExpr); ok {
		if c.ProducerCallable == "" || !ps6115DirectCall(pass, file, call, c.ProducerCallable) || len(call.Args) != 0 || !ps6120ResultCount(pass, call, 1) {
			return false
		}
		slice, ok := pass.TypesInfo.TypeOf(call).Underlying().(*types.Slice)
		if !ok {
			return false
		}
		basic, ok := slice.Elem().Underlying().(*types.Basic)
		return ok && basic.Info()&types.IsFloat != 0 && pass.TypesSizes.Sizeof(slice.Elem()) == c.WideElementBytes
	}
	literal, ok := ps2110Unparen(expression).(*ast.CompositeLit)
	if !ok || c.ProducerCallable != "" {
		return false
	}
	slice, ok := pass.TypesInfo.TypeOf(literal).Underlying().(*types.Slice)
	if !ok {
		return false
	}
	basic, ok := slice.Elem().Underlying().(*types.Basic)
	if !ok || basic.Info()&types.IsFloat == 0 || pass.TypesSizes.Sizeof(slice.Elem()) != c.WideElementBytes || int64(len(literal.Elts)) != c.SymbolCount {
		return false
	}
	distinct := make(map[string]bool, c.DistinctValueCount)
	for _, element := range literal.Elts {
		if _, keyed := element.(*ast.KeyValueExpr); keyed {
			return false
		}
		if element == nil {
			return false
		}
		value := pass.TypesInfo.Types[ps2110Unparen(element)].Value
		if value == nil || value.Kind() != constant.Float {
			return false
		}
		f, exact := constant.Float64Val(value)
		if !exact || f != f || f > 1.7976931348623157e308 || f < -1.7976931348623157e308 {
			return false
		}
		distinct[value.ExactString()] = true
	}
	return int64(len(distinct)) == c.DistinctValueCount
}

func ps6120ArgObject(pass *analysis.Pass, call *ast.CallExpr, position int, object types.Object) bool {
	index := position - 1
	return index >= 0 && index < len(call.Args) && ps6114ExprObject(pass, call.Args[index]) == object
}
func ps6120ObjectUses(pass *analysis.Pass, node ast.Node, object types.Object) int {
	count := 0
	ast.Inspect(node, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && pass.TypesInfo.ObjectOf(id) == object {
			count++
		}
		return true
	})
	return count
}
