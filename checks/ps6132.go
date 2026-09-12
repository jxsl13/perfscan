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

var PS6132 = register(&lint.Check{
	ID: "PS6132", Category: "alloc", Slug: "constructor-maximum-row-transient-workspace",
	Level: lint.LevelAggressive, AutoFix: false, NeedsConfig: true,
	Vocab: []string{"contextTransientWorkspaceContracts"},
	Doc: lint.Documentation{
		Title: "constructor retains maximum-row transient storage despite explicit active-row execution",
		Text: `A constructor can retain context-times-width activation buffers even though
steady-state decode uses one row and prefill supplies its actual row count.
PS6132 reports an allocation candidate only with an exact typed
contextTransientWorkspaceContracts entry. It does not identify transformers,
allocators, or transient storage from names.

Source proves a direct pointer-method field assignment through its exact
allocator formal (float slice to a non-generic named slot pointer), a builtin
float32/float64 make length containing the stable
maximum-row field, a selected same-owner row helper reading that field, and
same-owner direct calls to that helper at one row and at a stable dynamic
batch-row binding. The helper passes its immutable rows formal to every
source-visible call reading the relevant buffer. Geometry-preserving method
exceptions are exact reviewed identities (also synchronous, non-retaining,
identity-preserving forwarding of the allocator formal); unknown/captured receiver methods
cannot silently change an allocation snapshot. It rejects alternate writes, aliases, address exposure,
unclassified field observations, unsupported callback signatures, generics,
indirect row calls, unstable row bindings, and contradictory site contracts.

The contract supplies constructor-only allocation, fresh exclusive device
ownership of the callback result, transient active-row behavior on ALL
execution/provider/build-tag paths, recurrent paths restricted to one row,
sequential receiver use, checked geometry, completion/synchronization and
lifecycle, and external/reflection/unsafe observations. Those reviewed facts
are NOT inferred from the witnessed source calls. Every field selected for
reporting must be audited; the witness is not a closed whole-program proof.

There is NO automatic fix. Evaluate one resident decode row plus one complete,
reusable exact high-water batch generation, with every optional quantized,
post-norm, sandwich, fused gate/up, MoE and MLA member included. Build growth
privately; release partial allocations on failure before publishing anything.
Do not expose mixed old/new capacities. Release replaced/final generations
exactly once, restore resident selection on EVERY batched exit, and read hidden
state from the generation actually selected by that call. Preserve recurrent
one-row paths and errors; failed growth may invalidate the old generation if
that is the documented ownership policy, but must never publish a partial one.

Smaller storage can expose incorrect backend guards and capacity-wide work.
Validate active dispatch extents separately; a high-water generation can still
have a stale tail on smaller calls. For row-local strided bands, validate each
offset+band-width within stride and rows-times-stride backing storage; do not
blindly remove an offset from an arbitrary flat/subview capacity requirement.
Preserve checked arithmetic, invalid-shape behavior and backend fallbacks.
Gate reference F32/Q8, sequential versus batched parity, hidden-state readback,
partial failures, growth/shrink/release lifecycle and every backend, then
measure constructor resident bytes/allocation bytes and public decode/prefill
latency and allocations in order-alternating same-binary campaigns.`,
		Before: `func (d *Decoder) allocScratch(mk func([]float32) *slot) {
	c := d.maxRows
	d.x = mk(make([]float32, c*d.width))
}
// Both one-row decode and rows-aware prefill use d.x.`,
		After: `// Project-reviewed grouped ownership, not an automatic rewrite:
// retain one decode row; privately build an exact-row batch generation;
// publish only after all allocations succeed; restore selection on exit;
// release partial/replaced/final generations and retain selected readback.`,
		MeasuredWin: `Owner issue #891 and GoAI PR #1210 (merge
ec20269a20e2028ec10aa02cd9d26095e8aa161b) report Apple M2 Pro same-binary F32
shared-Decoder results: resident scratch 201,326,592 to 98,304 bytes
(-201,228,288), constructor allocation bytes about 201,329,344 to 105,040 B/op,
and five-sample constructor median 4,805,583 to 23,917 ns (about 201x).
Order-alternated public Step eager/lazy median ratio was about 0.991x with
7 allocs/op unchanged; StepNLast at 16 rows ratio about 1.366x with 83 allocs/op unchanged.
These are attributed project measurements, not universal throughput claims.
All 16 owner CI checks passed, including Metal/CUDA/Vulkan, race and pure-Go.`,
	},
	Analyzer: &analysis.Analyzer{Name: "PS6132", Doc: "maximum-row constructor transient storage has reviewed active-row ownership", Run: runPS6132},
})

func runPS6132(pass *analysis.Pass) (any, error) {
	return runPS6132WithContracts(pass, config.Current().ContextTransientWorkspaceContracts)
}

func runPS6132WithContracts(pass *analysis.Pass, contracts []config.ContextTransientWorkspaceContract) (any, error) {
	counts := make(map[string]int, len(contracts))
	for i := range contracts {
		counts[contracts[i].AllocationMethod]++
	}
	methods := ps6107Methods(pass)
	for i := range contracts {
		c := &contracts[i]
		if c.Valid() && counts[c.AllocationMethod] == 1 {
			ps6132Contract(pass, c, methods)
		}
	}
	return nil, nil
}

func ps6132Contract(pass *analysis.Pass, c *config.ContextTransientWorkspaceContract, methods map[string]ps6107Method) {
	allocation, ok := methods[c.AllocationMethod]
	if !ok || !ps6132Method(allocation) {
		return
	}
	row, ok := methods[c.RowMethod]
	if !ok || !ps6132Method(row) || row.named != allocation.named || c.RowArgument >= row.signature.Params().Len() ||
		!types.Identical(row.signature.Params().At(c.RowArgument).Type(), types.Typ[types.Int]) {
		return
	}
	if !ps6132GeometryStable(pass, row.declaration.Body, row.signature.Params().At(c.RowArgument), nil, nil, nil) {
		return
	}
	if !ps6132BindingStable(pass, row.declaration.Body, row.signature.Recv(), ps6087Parents(row.declaration.Body)) {
		return
	}
	one, ok := methods[c.OneRowExecutionMethod]
	if !ok || !ps6132Method(one) || one.named != allocation.named {
		return
	}
	batch, ok := methods[c.BatchExecutionMethod]
	if !ok || !ps6132Method(batch) || batch.named != allocation.named {
		return
	}
	var callback types.Object
	for j := range allocation.signature.Params().Len() {
		parameter := allocation.signature.Params().At(j)
		if parameter.Name() == c.AllocatorParameter {
			callback = parameter
		}
	}
	if callback == nil {
		return
	}
	if !ps6132AllocatorStable(pass, allocation, callback, c.GeometryPreservingMethods) {
		return
	}
	callbackSig, ok := types.Unalias(callback.Type()).(*types.Signature)
	if !ok || callbackSig.Variadic() || callbackSig.Params().Len() != 1 || callbackSig.Results().Len() != 1 {
		return
	}
	input, ok := types.Unalias(callbackSig.Params().At(0).Type()).(*types.Slice)
	if !ok || (!types.Identical(input.Elem(), types.Typ[types.Float32]) && !types.Identical(input.Elem(), types.Typ[types.Float64])) {
		return
	}
	pointer, ok := types.Unalias(callbackSig.Results().At(0).Type()).(*types.Pointer)
	if !ok {
		return
	}
	slot, ok := types.Unalias(pointer.Elem()).(*types.Named)
	if !ok || slot.TypeParams().Len() != 0 {
		return
	}
	if _, ok := slot.Underlying().(*types.Struct); !ok {
		return
	}
	maximum := ps6132Field(allocation.named, c.MaximumRowsField)
	if maximum == nil || !types.Identical(maximum.Type(), types.Typ[types.Int]) {
		return
	}
	if !ps6132Witness(pass, one, row, c.RowArgument, "") || !ps6132Witness(pass, batch, row, c.RowArgument, c.BatchRowsBinding) {
		return
	}
	aliases := ps6107Aliases(pass, allocation.declaration.Body)
	parents := ps6087Parents(allocation.declaration.Body)
	reachable := ps6099ReachableNodesInBlock(pass, allocation.declaration.Body, parents)
	root := allocation.signature.Recv()
	for _, name := range c.ScratchFields {
		field := ps6132Field(allocation.named, name)
		if field == nil || !types.Identical(field.Type(), callbackSig.Results().At(0).Type()) {
			continue
		}
		var match *ast.CallExpr
		var initialized *ast.SelectorExpr
		for _, statement := range allocation.declaration.Body.List {
			if !reachable[statement] {
				continue
			}
			assignment, ok := statement.(*ast.AssignStmt)
			if !ok || assignment.Tok != token.ASSIGN || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
				continue
			}
			left, ok := assignment.Lhs[0].(*ast.SelectorExpr)
			if !ok || pass.TypesInfo.Selections[left] == nil || pass.TypesInfo.Selections[left].Obj() != field ||
				!ps6132Object(pass, left.X, root) {
				continue
			}
			call, ok := assignment.Rhs[0].(*ast.CallExpr)
			if !ok || len(call.Args) != 1 || call.Ellipsis.IsValid() || !ps6132Object(pass, call.Fun, callback) {
				continue
			}
			makeCall, ok := call.Args[0].(*ast.CallExpr)
			if !ok || len(makeCall.Args) != 2 || !ps6132Builtin(pass, makeCall.Fun, "make") ||
				!types.Identical(pass.TypesInfo.TypeOf(makeCall), callbackSig.Params().At(0).Type()) {
				continue
			}
			length := ps2110Unparen(makeCall.Args[1])
			product, ok := length.(*ast.BinaryExpr)
			if !ok || product.Op != token.MUL || !ps6132Maximum(pass, product.X, maximum, root, aliases) {
				continue
			}
			if !ps6132ScalarExtent(pass, product.Y, root) {
				continue
			}
			geometry := map[types.Object]bool{maximum: true}
			ast.Inspect(makeCall.Args[1], func(n ast.Node) bool {
				if selector, ok := n.(*ast.SelectorExpr); ok && pass.TypesInfo.Selections[selector] != nil {
					geometry[pass.TypesInfo.Selections[selector].Obj()] = true
				}
				return true
			})
			if !ps6132GeometryStable(pass, allocation.declaration.Body, root, aliases, geometry, c.GeometryPreservingMethods) {
				continue
			}
			if match != nil {
				match = nil
				break
			}
			match, initialized = makeCall, left
		}
		if match == nil || !ps6132FieldObservations(pass, field, initialized, allocation.named) || !ps6132RowReadsField(pass, row, field, c.RowArgument) {
			continue
		}
		pass.Report(analysis.Diagnostic{Pos: match.Pos(), End: match.End(), Message: "constructor retains maximum-row transient " + allocation.named.Obj().Name() + "." + name + " storage; source proves maximum-times-width allocation and exact same-owner helper calls at one row and a stable batch bound; all-path transient ownership, recurrent one-row behavior and callback semantics are reviewed contract facts, not inferred from these witnesses; evaluate one resident row plus atomic exact high-water grouped ownership, preserving partial-failure cleanup, selection restoration, selected hidden readback, backend bounds/active dispatch and lifecycle, then measure retained bytes and complete decode/prefill latency/allocations (advisory, no automatic fix)"})
	}
}

func ps6132Method(m ps6107Method) bool {
	return m.pointer && m.named != nil && m.named.TypeParams().Len() == 0 && m.signature.TypeParams().Len() == 0 && !m.signature.Variadic()
}

func ps6132Field(owner *types.Named, name string) *types.Var {
	structure, ok := owner.Underlying().(*types.Struct)
	if !ok {
		return nil
	}
	for i := range structure.NumFields() {
		if field := structure.Field(i); field.Name() == name && !field.Exported() {
			return field
		}
	}
	return nil
}

func ps6132Object(pass *analysis.Pass, expression ast.Expr, object types.Object) bool {
	id, ok := ps2110Unparen(expression).(*ast.Ident)
	return ok && identObject(pass, id) == object
}

func ps6132Builtin(pass *analysis.Pass, expression ast.Expr, name string) bool {
	id, ok := expression.(*ast.Ident)
	if !ok {
		return false
	}
	builtin, ok := pass.TypesInfo.Uses[id].(*types.Builtin)
	return ok && builtin.Name() == name
}

func ps6132Maximum(pass *analysis.Pass, expression ast.Expr, field *types.Var, root types.Object, aliases map[types.Object]ast.Expr) bool {
	expression = ps2110Unparen(expression)
	if id, ok := expression.(*ast.Ident); ok {
		expression = aliases[identObject(pass, id)]
	}
	selector, ok := expression.(*ast.SelectorExpr)
	return ok && pass.TypesInfo.Selections[selector] != nil && pass.TypesInfo.Selections[selector].Obj() == field && ps6132Object(pass, selector.X, root)
}

func ps6132ScalarExtent(pass *analysis.Pass, e ast.Expr, root types.Object) bool {
	if value := pass.TypesInfo.Types[e].Value; value != nil {
		n, ok := constant.Int64Val(value)
		return ok && n > 0
	}
	switch e := ps2110Unparen(e).(type) {
	case *ast.SelectorExpr:
		return pass.TypesInfo.Selections[e] != nil && pass.TypesInfo.Selections[e].Kind() == types.FieldVal && types.Identical(pass.TypesInfo.TypeOf(e), types.Typ[types.Int]) && ps6132Object(pass, e.X, root)
	case *ast.BinaryExpr:
		return (e.Op == token.MUL || e.Op == token.ADD) && ps6132ScalarExtent(pass, e.X, root) && ps6132ScalarExtent(pass, e.Y, root)
	}
	return false
}

func ps6132GeometryStable(pass *analysis.Pass, body *ast.BlockStmt, root types.Object, aliases map[types.Object]ast.Expr, geometry map[types.Object]bool, preserving []string) bool {
	parents := ps6087Parents(body)
	if !ps6132BindingStable(pass, body, root, parents) {
		return false
	}
	valid := true
	ast.Inspect(body, func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if !ok {
			return true
		}
		object := identObject(pass, id)
		if object != root && aliases[object] == nil {
			return true
		}
		var owner ast.Node = id
		for {
			switch p := parents[owner].(type) {
			case *ast.ParenExpr:
				owner = p
			case *ast.SelectorExpr:
				if p.X == owner {
					owner = p
				} else {
					return true
				}
			default:
				goto classified
			}
		}
	classified:
		if selector, ok := owner.(*ast.SelectorExpr); ok && pass.TypesInfo.Selections[selector] != nil {
			selection := pass.TypesInfo.Selections[selector]
			if selection.Kind() == types.MethodVal {
				allowed := false
				for _, method := range preserving {
					if function, ok := selection.Obj().(*types.Func); ok && ps6090FunctionID(function) == method {
						allowed = true
					}
				}
				if !allowed {
					valid = false
					return false
				}
			} else if !geometry[selection.Obj()] {
				return true
			}
		}
		switch p := parents[owner].(type) {
		case *ast.AssignStmt:
			for _, lhs := range p.Lhs {
				if lhs == owner && (p.Tok != token.DEFINE || pass.TypesInfo.Defs[id] != object || owner != id) { // Scratch-field writes are separately classified; scalar/root writes are not.
					if selector, ok := owner.(*ast.SelectorExpr); !ok || types.Identical(pass.TypesInfo.TypeOf(selector), types.Typ[types.Int]) {
						valid = false
					}
				}
			}
		case *ast.IncDecStmt:
			valid = false
		case *ast.RangeStmt:
			if p.Tok == token.ASSIGN && (p.Key == owner || p.Value == owner) {
				valid = false
			}
		case *ast.UnaryExpr:
			if p.Op == token.AND {
				valid = false
			}
		case *ast.StarExpr:
			if _, pointer := types.Unalias(object.Type()).(*types.Pointer); pointer {
				valid = false
			}
		}
		return valid
	})
	return valid
}

func ps6132Witness(pass *analysis.Pass, execution, row ps6107Method, argument int, binding string) bool {
	var extent types.Object
	ambiguous := false
	if binding != "" {
		ast.Inspect(execution.declaration.Body, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok && id.Name == binding && pass.TypesInfo.Defs[id] != nil {
				if extent != nil && extent != pass.TypesInfo.Defs[id] {
					ambiguous = true
				}
				extent = pass.TypesInfo.Defs[id]
			}
			return true
		})
		if extent == nil || ambiguous {
			return false
		}
	}
	if extent != nil && ps6107AssignmentCount(pass, execution.declaration.Body, extent) != 1 {
		return false
	}
	parents := ps6087Parents(execution.declaration.Body)
	if extent != nil && !ps6132BatchExtent(pass, execution, extent, parents) {
		return false
	}
	if !ps6132BindingStable(pass, execution.declaration.Body, execution.signature.Recv(), parents) {
		return false
	}
	reachable := ps6099ReachableNodesInBlock(pass, execution.declaration.Body, parents)
	for _, call := range ps6022Calls(pass, execution.declaration.Body, parents) {
		if call.function != row.object || !reachable[call.call] || argument >= len(call.call.Args) {
			continue
		}
		selector, ok := call.call.Fun.(*ast.SelectorExpr)
		if !ok || !ps6132Object(pass, selector.X, execution.signature.Recv()) {
			continue
		}
		value := call.call.Args[argument]
		if extent != nil {
			if ps6132Object(pass, value, extent) && ps6132GeometryStable(pass, execution.declaration.Body, extent, nil, nil, nil) {
				return true
			}
			continue
		}
		if constantValue := pass.TypesInfo.Types[value].Value; constantValue != nil {
			n, ok := constant.Int64Val(constantValue)
			if ok && n == 1 {
				return true
			}
		}
	}
	return false
}

func ps6132BatchExtent(pass *analysis.Pass, execution ps6107Method, extent types.Object, parents map[ast.Node]ast.Node) bool {
	for _, statement := range execution.declaration.Body.List {
		assignment, ok := statement.(*ast.AssignStmt)
		if !ok || assignment.Tok != token.DEFINE || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 || !ps6132Object(pass, assignment.Lhs[0], extent) {
			continue
		}
		call, ok := assignment.Rhs[0].(*ast.CallExpr)
		if !ok || len(call.Args) != 1 || !ps6132Builtin(pass, call.Fun, "len") {
			return false
		}
		id, ok := call.Args[0].(*ast.Ident)
		if !ok {
			return false
		}
		for i := range execution.signature.Params().Len() {
			parameter := execution.signature.Params().At(i)
			if identObject(pass, id) != parameter {
				continue
			}
			if _, ok := types.Unalias(parameter.Type()).(*types.Slice); !ok {
				return false
			}
			return ps6132BindingStable(pass, execution.declaration.Body, parameter, parents)
		}
	}
	return false
}

func ps6132BindingStable(pass *analysis.Pass, body *ast.BlockStmt, object types.Object, parents map[ast.Node]ast.Node) bool {
	valid := true
	ast.Inspect(body, func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if !ok || identObject(pass, id) != object {
			return true
		}
		var owner ast.Node = id
		for {
			paren, ok := parents[owner].(*ast.ParenExpr)
			if !ok {
				break
			}
			owner = paren
		}
		switch p := parents[owner].(type) {
		case *ast.AssignStmt:
			if _, pointer := types.Unalias(object.Type()).(*types.Pointer); pointer {
				lhsUse := false
				for _, lhs := range p.Lhs {
					if lhs == owner {
						lhsUse = true
					}
				}
				if !lhsUse {
					valid = false
				}
			}
			for _, lhs := range p.Lhs {
				if lhs == owner && (p.Tok != token.DEFINE || pass.TypesInfo.Defs[id] != object) {
					valid = false
				}
			}
		case *ast.RangeStmt:
			if p.Tok == token.ASSIGN && (p.Key == owner || p.Value == owner) {
				valid = false
			}
		case *ast.UnaryExpr:
			if p.Op == token.AND {
				valid = false
			}
		case *ast.CallExpr:
			if _, pointer := types.Unalias(object.Type()).(*types.Pointer); pointer {
				valid = false
			}
		case *ast.ReturnStmt:
			if _, pointer := types.Unalias(object.Type()).(*types.Pointer); pointer {
				valid = false
			}
		case *ast.StarExpr:
			if _, pointer := types.Unalias(object.Type()).(*types.Pointer); pointer {
				valid = false
			}
		case *ast.SelectorExpr:
			if p.X != owner || pass.TypesInfo.Selections[p] == nil {
				valid = false
			}
		default:
			if _, pointer := types.Unalias(object.Type()).(*types.Pointer); pointer {
				valid = false
			}
		}
		return valid
	})
	return valid
}

// Geometry-preserving forwarding exceptions also attest that the exact
// allocator formal is called synchronously without rebinding/retaining it.
func ps6132AllocatorStable(pass *analysis.Pass, method ps6107Method, object types.Object, forwarding []string) bool {
	parents := ps6087Parents(method.declaration.Body)
	valid := true
	ast.Inspect(method.declaration.Body, func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if !ok || identObject(pass, id) != object {
			return true
		}
		call, ok := parents[id].(*ast.CallExpr)
		if !ok {
			valid = false
			return false
		}
		if call.Fun == id {
			return true
		}
		function, _, ok := typedCallee(pass, call.Fun)
		if !ok {
			valid = false
			return false
		}
		allowed := false
		for _, identity := range forwarding {
			if ps6090FunctionID(function) == identity {
				allowed = true
			}
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || !ps6132Object(pass, selector.X, method.signature.Recv()) {
			allowed = false
		}
		if !allowed {
			valid = false
		}
		return valid
	})
	return valid
}

func ps6132RowReadsField(pass *analysis.Pass, row ps6107Method, field *types.Var, argument int) bool {
	found := false
	valid := true
	parents := ps6087Parents(row.declaration.Body)
	ast.Inspect(row.declaration.Body, func(n ast.Node) bool {
		selector, ok := n.(*ast.SelectorExpr)
		if ok && pass.TypesInfo.Selections[selector] != nil && pass.TypesInfo.Selections[selector].Obj() == field && ps6132Object(pass, selector.X, row.signature.Recv()) {
			found = true
			var owner ast.Node = selector
			for {
				p, ok := parents[owner].(*ast.SelectorExpr)
				if !ok || p.X != owner {
					break
				}
				owner = p
			}
			call, ok := parents[owner].(*ast.CallExpr)
			if !ok {
				valid = false
				return false
			}
			bounded := false
			for _, arg := range call.Args {
				if ps6132ActiveExtent(pass, arg, row.signature.Params().At(argument), row.signature.Recv()) {
					bounded = true
				}
			}
			if !bounded {
				valid = false
			}
		}
		return true
	})
	return found && valid
}

func ps6132ActiveExtent(pass *analysis.Pass, expression ast.Expr, rows, receiver types.Object) bool {
	if ps6132Object(pass, expression, rows) {
		return true
	}
	product, ok := ps2110Unparen(expression).(*ast.BinaryExpr)
	if !ok || product.Op != token.MUL {
		return false
	}
	return (ps6132ActiveExtent(pass, product.X, rows, receiver) && ps6132ScalarExtent(pass, product.Y, receiver)) ||
		(ps6132ScalarExtent(pass, product.X, receiver) && ps6132ActiveExtent(pass, product.Y, rows, receiver))
}

func ps6132FieldObservations(pass *analysis.Pass, field *types.Var, initializer *ast.SelectorExpr, typeOwner *types.Named) bool {
	for _, file := range pass.Files {
		parents := ps6087Parents(file)
		valid := true
		ast.Inspect(file, func(n ast.Node) bool {
			switch value := n.(type) {
			case *ast.StarExpr:
				if types.Identical(pass.TypesInfo.TypeOf(value), typeOwner) {
					valid = false
					return false
				}
			case *ast.AssignStmt:
				for _, lhs := range value.Lhs {
					if types.Identical(pass.TypesInfo.TypeOf(lhs), typeOwner) {
						valid = false
						return false
					}
				}
			case *ast.CompositeLit:
				if types.Identical(pass.TypesInfo.TypeOf(value), typeOwner) && len(value.Elts) > 0 {
					for _, element := range value.Elts {
						entry, keyed := element.(*ast.KeyValueExpr)
						if !keyed {
							valid = false
							return false
						}
						key, ok := entry.Key.(*ast.Ident)
						if !ok || key.Name == field.Name() {
							valid = false
							return false
						}
					}
				}
			}
			selector, ok := n.(*ast.SelectorExpr)
			if !ok || pass.TypesInfo.Selections[selector] == nil || pass.TypesInfo.Selections[selector].Obj() != field || selector == initializer {
				return true
			}
			var owner ast.Node = selector
			methodReceiver := false
			for {
				switch p := parents[owner].(type) {
				case *ast.ParenExpr:
					owner = p
				case *ast.SelectorExpr:
					if p.X == owner && pass.TypesInfo.Selections[p] != nil && pass.TypesInfo.Selections[p].Kind() == types.FieldVal {
						owner = p
					} else if p.X == owner && pass.TypesInfo.Selections[p] != nil && pass.TypesInfo.Selections[p].Kind() == types.MethodVal {
						owner = p
						methodReceiver = true
						goto observed
					} else {
						goto observed
					}
				default:
					goto observed
				}
			}
		observed:
			call, ok := parents[owner].(*ast.CallExpr)
			if !ok {
				valid = false
				return false
			}
			function, _, ok := typedCallee(pass, call.Fun)
			if !ok || function == nil || call.Ellipsis.IsValid() {
				valid = false
				return false
			}
			argument := methodReceiver && call.Fun == owner
			for _, arg := range call.Args {
				if arg == owner {
					argument = true
				}
			}
			if !argument {
				valid = false
			}
			return valid
		})
		if !valid {
			return false
		}
	}
	return true
}
