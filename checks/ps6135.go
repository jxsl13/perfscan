package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"

	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/lint"
	"golang.org/x/tools/go/analysis"
)

var PS6135 = register(&lint.Check{
	ID: "PS6135", Category: "verify", Slug: "explicit-active-bound-capacity-fallback", Level: lint.LevelAggressive,
	AutoFix: false, NeedsConfig: true, Vocab: []string{"activeBoundFallbackContracts"},
	Doc: lint.Documentation{
		Title: "an explicit active-bound capability falls back to capacity-wide work",
		Text: `PS6135 detects a narrow optional-interface dispatch wrapper: the same provider,
three buffer arguments and operation go to an explicit-bound capability with
positive rows times width, while the fallback omits that active extent.
It requires exact typed activeBoundFallbackContracts API roles. Names such as
Binary or BinaryN do not establish device capacity, prefix or tail semantics.

Source proves an exact two-statement wrapper, an optional named interface
capability assertion on the original named interface provider, positive rows
and width guards, identical direct buffer/operation formals, rows*width in the
bounded return, and the same provider's unbounded direct fallback return.
Extra statements, changed/rebound/aliased formals, alternate providers, indirect
calls, generics, unsupported guards and argument flow remain silent. Type
aliases resolving to the same canonical named interface are accepted.

The contract supplies bounded active-prefix and capacity-wide behavior,
buffer/operation roles, workloads whose active rows may be below retained
capacity, unobserved inactive tail, all provider/build paths, and error, shape,
completion and synchronization semantics. Those are reviewed project facts,
not inferred from the witnessed interface syntax. The source cannot prove a
runtime work ratio, capability absence on a measured provider, or speedup.
The cost candidate is conditional on valid positive nonoverflowing active
geometry, absent capability, and retained capacity exceeding that extent;
invalid/nonpositive or overflowing shape fallback is not an optimization claim.

There is NO automatic fix. Review whether each fallback provider can support
an explicit extent or a valid bounded/fused route. Preserve capability and
short-circuit selection, invalid-shape/overflow behavior, operation bits,
aliasing, errors and synchronization, and every provider fallback. Exact
high-water storage may still contain stale tails after smaller calls; do not
confuse backing capacity with the active invocation. Validate constructor and
all optional/recurrent paths, active-prefix parity and untouched tail, then
measure complete operations with pinned source/compiler/binary and alternating
same-binary controls. PS6106 covers a different concrete-provider producer/
consumer grammar; its existing coverage does not accept this interface wrapper.`,
		Before: `if bounded, ok := provider.(ActiveAPI); ok && rows > 0 && width > 0 {
	return bounded.ElementN(a,b,out,op,rows*width)
}
return provider.Element(a,b,out,op)`,
		After: "// Project-reviewed explicit-bound fallback support; no automatic rewrite.",
		MeasuredWin: `Owner issue #891 / GoAI PR1210 reports a 1.366x median eager/lazy
StepNLast16 same-binary result with unchanged 83 allocs/op, involving exact
scratch residency and backend work. This is complete-operation context, NOT
a measurement of binElem or this fallback in isolation. No PS6135-specific
replacement or measured gain is claimed. The pinned binElem remains unchanged
between parent74a7c5c923b25aa35773bb9e907b76aba04a553a and merge
ec20269a20e2028ec10aa02cd9d26095e8aa161b.`,
	},
	Analyzer: &analysis.Analyzer{Name: "PS6135", Doc: "optional active extent dispatch retains capacity-wide fallback", Run: runPS6135},
})

func runPS6135(pass *analysis.Pass) (any, error) {
	return runPS6135WithContracts(pass, config.Current().ActiveBoundFallbackContracts)
}

func runPS6135WithContracts(pass *analysis.Pass, contracts []config.ActiveBoundFallbackContract) (any, error) {
	counts := make(map[string]int, len(contracts))
	for i := range contracts {
		counts[contracts[i].WrapperMethod]++
	}
	methods := ps6107Methods(pass)
	for i := range contracts {
		c := &contracts[i]
		if !c.Valid() || counts[c.WrapperMethod] != 1 {
			continue
		}
		method, ok := methods[c.WrapperMethod]
		if !ok || !method.pointer || method.named == nil || method.named.TypeParams().Len() != 0 || method.signature.TypeParams().Len() != 0 || method.signature.Variadic() ||
			!ps6135ErrorResult(method.signature) || len(method.declaration.Body.List) != 2 {
			continue
		}
		params := method.signature.Params()
		roles := append([]int{c.ProviderArgument, c.OperationArgument, c.RowsArgument, c.WidthArgument}, c.BufferArguments...)
		valid := params.Len() == len(roles)
		for _, role := range roles {
			if role >= params.Len() {
				valid = false
			}
		}
		if !valid {
			continue
		}
		provider := params.At(c.ProviderArgument)
		providerNamed, providerAPI := ps6135Interface(provider.Type(), c.ProviderType)
		if providerNamed == nil {
			continue
		}
		rows, width := params.At(c.RowsArgument), params.At(c.WidthArgument)
		operation := params.At(c.OperationArgument)
		if !types.Identical(rows.Type(), types.Typ[types.Int]) || !types.Identical(width.Type(), types.Typ[types.Int]) ||
			!types.Identical(operation.Type(), types.Typ[types.Int]) {
			continue
		}
		branch, ok := method.declaration.Body.List[0].(*ast.IfStmt)
		if !ok || branch.Else != nil || len(branch.Body.List) != 1 {
			continue
		}
		init, ok := branch.Init.(*ast.AssignStmt)
		if !ok || init.Tok != token.DEFINE || len(init.Lhs) != 2 || len(init.Rhs) != 1 {
			continue
		}
		boundID, boundOK := init.Lhs[0].(*ast.Ident)
		okID, okOK := init.Lhs[1].(*ast.Ident)
		assertion, assertOK := init.Rhs[0].(*ast.TypeAssertExpr)
		if !boundOK || !okOK || !assertOK || assertion.Type == nil || !ps6135Object(pass, assertion.X, provider) ||
			pass.TypesInfo.Defs[boundID] == nil || pass.TypesInfo.Defs[okID] == nil {
			continue
		}
		_, capability := ps6135Interface(pass.TypesInfo.TypeOf(assertion.Type), c.CapabilityType)
		if capability == nil || capability.NumMethods() != 1 || types.Implements(providerNamed, capability) ||
			!ps6135Guards(pass, branch.Cond, pass.TypesInfo.Defs[okID], rows, width) {
			continue
		}
		bounded := ps6135Return(branch.Body.List[0])
		fallback := ps6135Return(method.declaration.Body.List[1])
		if bounded == nil || fallback == nil {
			continue
		}
		buffers := make([]types.Object, len(c.BufferArguments))
		for j, role := range c.BufferArguments {
			buffers[j] = params.At(role)
		}
		if !ps6135Call(pass, bounded, pass.TypesInfo.Defs[boundID], capability, c.BoundedMethod, buffers, operation, true) ||
			!ps6135Call(pass, fallback, provider, providerAPI, c.CapacityMethod, buffers, operation, false) {
			continue
		}
		product, ok := ps2110Unparen(bounded.Args[4]).(*ast.BinaryExpr)
		if !ok || product.Op != token.MUL || !((ps6135Object(pass, product.X, rows) && ps6135Object(pass, product.Y, width)) ||
			(ps6135Object(pass, product.X, width) && ps6135Object(pass, product.Y, rows))) {
			continue
		}
		pass.Report(analysis.Diagnostic{Pos: fallback.Pos(), End: fallback.End(), Message: "for positive active rows/width with a valid nonoverflowing product, absent capability and retained capacity greater than rows*width, this fallback omits the active extent shared by its bounded provider/buffer/operation roles; capacity behavior is a reviewed API fact, not runtime capacity proved from source; review extent-aware fallback support across providers, preserving shape/errors/aliasing/tail/synchronization, then measure complete operations (PS6135 advisory, no automatic fix or isolated measured gain)"})
	}
	return nil, nil
}

func ps6135Interface(t types.Type, id string) (*types.Named, *types.Interface) {
	named, ok := types.Unalias(t).(*types.Named)
	if !ok || named.Obj().Pkg() == nil || named.TypeParams().Len() != 0 || named.Obj().Pkg().Path()+"."+named.Obj().Name() != id {
		return nil, nil
	}
	api, ok := named.Underlying().(*types.Interface)
	if !ok {
		return nil, nil
	}
	return named, api.Complete()
}

func ps6135ErrorResult(sig *types.Signature) bool {
	return sig.Results().Len() == 1 && types.Identical(sig.Results().At(0).Type(), types.Universe.Lookup("error").Type())
}

func ps6135Object(pass *analysis.Pass, e ast.Expr, object types.Object) bool {
	id, ok := ps2110Unparen(e).(*ast.Ident)
	return ok && identObject(pass, id) == object
}

func ps6135Return(statement ast.Stmt) *ast.CallExpr {
	r, ok := statement.(*ast.ReturnStmt)
	if !ok || len(r.Results) != 1 {
		return nil
	}
	call, _ := ps2110Unparen(r.Results[0]).(*ast.CallExpr)
	return call
}

func ps6135Call(pass *analysis.Pass, call *ast.CallExpr, provider types.Object, api *types.Interface, name string, buffers []types.Object, operation types.Object, bounded bool) bool {
	selector, ok := ps2110Unparen(call.Fun).(*ast.SelectorExpr)
	if !ok || !ps6135Object(pass, selector.X, provider) || call.Ellipsis.IsValid() {
		return false
	}
	selection := pass.TypesInfo.Selections[selector]
	if selection == nil || selection.Kind() != types.MethodVal {
		return false
	}
	var method *types.Func
	for i := range api.NumMethods() {
		if api.Method(i).Name() == name {
			method = api.Method(i)
		}
	}
	if method == nil || selection.Obj() != method {
		return false
	}
	sig, _ := method.Type().(*types.Signature)
	want := 4
	if bounded {
		want++
	}
	if sig == nil || sig.Variadic() || sig.Params().Len() != want || !ps6135ErrorResult(sig) || len(call.Args) != want {
		return false
	}
	for i, buffer := range buffers {
		if !ps6135Object(pass, call.Args[i], buffer) || !types.Identical(sig.Params().At(i).Type(), buffer.Type()) ||
			!types.Identical(buffer.Type(), buffers[0].Type()) {
			return false
		}
	}
	if !ps6135Object(pass, call.Args[3], operation) || !types.Identical(sig.Params().At(3).Type(), types.Typ[types.Int]) {
		return false
	}
	return !bounded || types.Identical(sig.Params().At(4).Type(), types.Typ[types.Int])
}

func ps6135Guards(pass *analysis.Pass, e ast.Expr, okObject, rows, width types.Object) bool {
	var terms []ast.Expr
	var split func(ast.Expr)
	split = func(e ast.Expr) {
		if b, ok := ps2110Unparen(e).(*ast.BinaryExpr); ok && b.Op == token.LAND {
			split(b.X)
			split(b.Y)
		} else {
			terms = append(terms, e)
		}
	}
	split(e)
	if len(terms) != 3 {
		return false
	}
	seen := make(map[types.Object]bool, 3)
	for _, term := range terms {
		if ps6135Object(pass, term, okObject) {
			if seen[okObject] {
				return false
			}
			seen[okObject] = true
			continue
		}
		b, ok := ps2110Unparen(term).(*ast.BinaryExpr)
		if !ok || b.Op != token.GTR {
			return false
		}
		zero := pass.TypesInfo.Types[b.Y].Value
		if zero == nil || zero.Kind() != constant.Int || constant.Sign(zero) != 0 {
			return false
		}
		var object types.Object
		if ps6135Object(pass, b.X, rows) {
			object = rows
		} else if ps6135Object(pass, b.X, width) {
			object = width
		} else {
			return false
		}
		if seen[object] {
			return false
		}
		seen[object] = true
	}
	return seen[okObject] && seen[rows] && seen[width]
}
