package checks

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/cfg"

	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/lint"
)

// PS6087 implements owner issue #828 through explicit project API contracts.
// Go does not encode storage ownership or legal operand aliasing in types, so
// an unconfigured producer/capability pair must remain silent.
var PS6087 = register(&lint.Check{
	ID:          "PS6087",
	Category:    "alloc",
	Slug:        "last-use-private-intermediate-inplace-fusion",
	Level:       lint.LevelStructured,
	AutoFix:     false,
	NeedsConfig: true,
	Vocab:       []string{"inPlaceFusionContracts"},
	Doc: lint.Documentation{
		Title: "a contract-proven private single-use tensor chain has a provider-bound in-place fusion capability",
		Text: `A hot eager tensor path can allocate one full-width activation
result and another result for a following elementwise operation even though the
activation input is a private producer result with no remaining consumer. A
backend may be able to overwrite that earliest-dead buffer with the composed
result, eliminating most of the temporary allocation traffic.

Local Go syntax cannot prove that an opaque producer returns fresh exclusive
storage, that an Into method permits dst to alias an input, or that an optional
interface belongs to the operation provider. PS6087 therefore consumes an
explicit inPlaceFusionContracts entry from perfscan.yaml. The contract names
exact typed method identities (import/path.Type.Method), exact operand
positions, the provider's exact optional interface/method, and the provider's
exact non-recording guard. It must affirm all of these semantic guarantees:

  - the producer returns fresh, exclusively owned storage that aliases none of
    its inputs or receiver state;
  - the activation and binary operations each return fresh exclusive storage,
    rather than an input alias or view;
  - the capability overwrites its first argument with the complete composed
    result, preserves its second argument, and matches the observable value,
    panic, and side-effect semantics of the two configured operations;
  - the capability returns false for unsupported runtime shapes, dtypes,
    storage layouts, and synchronization states without modifying either
    operand; and
  - the configured guard proves eager, recorder-free, autograd-invisible
    execution.

Against that opt-in contract, the source match is deliberately narrow. The
producer, activation, and binary calls must be direct assignments in one
lexical block on a live panic-aware CFG path. The producer may return exactly
one data-shaped result plus optional errors; the activation and binary each
return exactly one total result, with no error or metadata result. Vector
result containers and arbitrary result indexes are not guessed. The binary's
other operand must already exist
before the activation, and the activation and binary assignments must be
adjacent, so the fused call moves across no operation, error, or side-effect
boundary. The calls accept exactly the configured direct identifier operands,
with identical static types. Gate, activation, and binary results must each
have exactly one use, including closure captures. The activation and binary
methods must use the same plain receiver object, that receiver must be
statically assertable to (or implement) the configured provider-package
capability, and it must be a function-local variable or parameter whose only
three function-body uses are the guard and the two matched receivers. Both
calls must be direct statements in the body of the same exact positive
non-recording guard on that receiver.

There is NO operation-name heuristic, generic Into inference, package-global capability
search, or automatic fix. Cached/borrowed producers, nested Clone/view/slice
operands, multiple data results, different or effectful receivers, receiver
reassignment, dead CFG regions, recorder paths, incompatible types, missing
contracts, and unsupported capability signatures stay silent. Runtime shape,
dtype, layout, and synchronization acceptance remains the capability's job, so
the original composed path must remain as its fallback. The After example uses
an interface provider; a concrete provider that directly implements the
capability calls its capability method without a type assertion.`,
		Before: `if ops.IsEager() {
	up, _ := upProjection.Forward(x)
	gate, _ := gateProjection.Forward(x)
	act := ops.SiLU(gate)
	product := ops.Mul(act, up)
	return downProjection.Forward(product)
}`,
		After: `if ops.IsEager() {
	up, _ := upProjection.Forward(x)
	gate, _ := gateProjection.Forward(x)
	if fuser, ok := ops.(SwiGLUInPlaceFuser); ok && fuser.FuseSwiGLUInPlace(gate, up) {
		return downProjection.Forward(gate)
	}
	act := ops.SiLU(gate)
	product := ops.Mul(act, up)
	return downProjection.Forward(product) // established fallback
}`,
		MeasuredWin: `The permanent GoAI benchmark and retained evidence are in
github.com/jxsl13/goai at commit 9e58e031334f66aab5381cf837913845a29bbde1
(internal/benchcompare/leadership/evidence/m2-cpu-swiglu-inplace-20260822),
and the follow-up permanent benchmark is
format/gguf/quant_matmul_pair_apply_bench_test.go at commit
ee55a6fb64c6f34325bb9c08cea9ea13cfc73648. On Apple M2 Pro with Go
1.26.6 and GOMAXPROCS=8, ten retained 500 ms samples measured 44.01 us/op
composed versus 39.53 us/op in-place (-10.18%, p=0.000), 49,800 versus 64
B/op, and 12 versus 1 allocs/op. The 64-step TinyLlama Q4_K_M decode improved
from 2.211 s to 1.982 s (-10.35%, p=0.011), with 25.04% fewer allocated bytes,
5.22% fewer allocations, and an unchanged exact output digest.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6087",
		Doc:  "contract-proven private single-use tensor chain has a provider-bound in-place fusion capability",
		Run:  runPS6087,
	},
})

type ps6087Binding struct {
	call        *ast.CallExpr
	function    string
	object      types.Object
	resultType  types.Type
	resultCount int
	block       *ast.BlockStmt
	index       int
}

type ps6087Provider struct {
	object types.Object
	typeOf types.Type
}

func runPS6087(pass *analysis.Pass) (any, error) {
	return runPS6087WithContracts(pass, config.Current().InPlaceFusionContracts)
}

func runPS6087WithContracts(pass *analysis.Pass, contracts []config.InPlaceFusionContract) (any, error) {
	if len(contracts) == 0 {
		return nil, nil
	}
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			fn, ok := declaration.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			bindings, byObject := ps6087Bindings(pass, fn.Body)
			uses := ps6087UseCounts(pass, fn.Body)
			parents := ps6087Parents(fn.Body)
			for index := range contracts {
				contract := &contracts[index]
				if ps6087ValidContract(contract) {
					ps6087ReportContract(pass, fn, contract, bindings, byObject, uses, parents)
				}
			}
		}
	}
	return nil, nil
}

func ps6087ReportContract(
	pass *analysis.Pass,
	fn *ast.FuncDecl,
	contract *config.InPlaceFusionContract,
	bindings []ps6087Binding,
	byObject map[types.Object][]ps6087Binding,
	uses map[types.Object]int,
	parents map[ast.Node]ast.Node,
) {
	for _, activation := range bindings {
		if activation.function != contract.Activation {
			continue
		}
		provider, ok := ps6087CallProvider(pass, activation.call)
		if !ok || provider.object.Pkg() != pass.Pkg || provider.object.Parent() == pass.Pkg.Scope() || uses[provider.object] != 3 || activation.resultCount != 1 || len(activation.call.Args) != 1 || activation.call.Ellipsis.IsValid() {
			continue
		}
		activationGuard, ok := ps6087Guard(pass, activation.call, provider.object, contract.NonRecordingGuard, parents)
		if !ok || activationGuard.Body != activation.block {
			continue
		}
		gate, gateType, ok := ps6087DirectDataArgument(pass, activation.call, contract.ActivationInputArg)
		if !ok || uses[gate] != 1 || !types.Identical(activation.resultType, gateType) {
			continue
		}
		producer, ok := ps6087UniqueBinding(byObject[gate])
		if !ok || producer.function != contract.Producer || producer.block != activation.block || producer.index >= activation.index || !types.Identical(producer.resultType, gateType) || !ps6087PositionDominates(pass, fn.Body, producer.call.Pos(), activation.call.Pos()) {
			continue
		}
		for _, binary := range bindings {
			if binary.function != contract.Binary || binary.block != activation.block || binary.index != activation.index+1 || binary.resultCount != 1 || len(binary.call.Args) != 2 || binary.call.Ellipsis.IsValid() || uses[activation.object] != 1 || uses[binary.object] != 1 {
				continue
			}
			binaryProvider, ok := ps6087CallProvider(pass, binary.call)
			if !ok || binaryProvider.object != provider.object || !types.Identical(binaryProvider.typeOf, provider.typeOf) {
				continue
			}
			binaryGuard, ok := ps6087Guard(pass, binary.call, provider.object, contract.NonRecordingGuard, parents)
			if !ok || binaryGuard != activationGuard {
				continue
			}
			activationObject, activationType, activationOK := ps6087DirectDataArgument(pass, binary.call, contract.BinaryActivationArg)
			other, otherType, otherOK := ps6087DirectDataArgument(pass, binary.call, contract.BinaryOtherArg)
			if !activationOK || !otherOK || activationObject != activation.object || other == gate || other == activation.object || other.Pos() >= activation.call.Pos() || !types.Identical(activationType, gateType) || !types.Identical(otherType, gateType) || !types.Identical(binary.resultType, gateType) {
				continue
			}
			if !ps6087PositionDominates(pass, fn.Body, activation.call.Pos(), binary.call.Pos()) || !ps6087DirectTerminalUse(pass, fn.Body, binary.object, parents) {
				continue
			}
			capability, ok := ps6087Capability(pass, provider.typeOf, gateType, otherType, contract)
			if !ok {
				continue
			}
			pass.Reportf(activation.call.Pos(), "%s: configured fresh-owned %s result %s is consumed only by fresh-output %s and that result only by fresh-output %s; the local provider's only uses are this %s guard and the two adjacent calls, and it implements or is assertable to %s.%s, whose contract preserves operand 2, matches the composition when successful, and leaves both operands unchanged when unsupported — evaluate reusing %s inside this non-recording guard while retaining the composed fallback (advisory, no automatic fix)", contract.Name, contract.Producer, gate.Name(), contract.Activation, contract.Binary, contract.NonRecordingGuard, capability, contract.CapabilityMethod, gate.Name())
			break
		}
	}
}

func ps6087ValidContract(contract *config.InPlaceFusionContract) bool {
	return contract.Name != "" && contract.Producer != "" && contract.Activation != "" &&
		contract.Binary != "" && contract.CapabilityInterface != "" &&
		contract.CapabilityMethod != "" && contract.NonRecordingGuard != "" &&
		contract.ActivationInputArg >= 0 && contract.BinaryActivationArg >= 0 &&
		contract.BinaryOtherArg >= 0 && contract.BinaryActivationArg != contract.BinaryOtherArg &&
		contract.ProducerReturnsFreshOwned && contract.ActivationReturnsFreshOwned &&
		contract.BinaryReturnsFreshOwned && contract.CapabilityOverwritesFirstArg &&
		contract.CapabilityPreservesSecondArg && contract.CapabilityRejectsUnsupported &&
		contract.CapabilityFailureUnmodified && contract.CapabilityMatchesComposition &&
		contract.GuardProvesNonRecording
}

func ps6087Bindings(pass *analysis.Pass, body *ast.BlockStmt) ([]ps6087Binding, map[types.Object][]ps6087Binding) {
	var bindings []ps6087Binding
	byObject := make(map[types.Object][]ps6087Binding)
	ps6032Blocks(body, func(block *ast.BlockStmt) {
		for index, statement := range block.List {
			binding, ok := ps6087StatementBinding(pass, statement)
			if !ok {
				continue
			}
			binding.block = block
			binding.index = index
			bindings = append(bindings, binding)
			byObject[binding.object] = append(byObject[binding.object], binding)
		}
	})
	return bindings, byObject
}

func ps6087StatementBinding(pass *analysis.Pass, statement ast.Stmt) (ps6087Binding, bool) {
	call := ps6032StatementCall(statement)
	if call == nil {
		return ps6087Binding{}, false
	}
	_, signature, ok := typedCallee(pass, call.Fun)
	if !ok || signature.Results() == nil || signature.Results().Len() == 0 {
		return ps6087Binding{}, false
	}
	names, ok := ps6087ResultNames(statement)
	if !ok || len(names) != signature.Results().Len() {
		return ps6087Binding{}, false
	}
	dataIndex := -1
	for index := 0; index < signature.Results().Len(); index++ {
		resultType := signature.Results().At(index).Type()
		if ps6087DataType(resultType) {
			if dataIndex >= 0 {
				return ps6087Binding{}, false
			}
			dataIndex = index
			continue
		}
		if !ps6087ErrorType(resultType) {
			return ps6087Binding{}, false
		}
	}
	if dataIndex < 0 || names[dataIndex] == nil || names[dataIndex].Name == "_" {
		return ps6087Binding{}, false
	}
	object := identObject(pass, names[dataIndex])
	if _, ok := object.(*types.Var); !ok {
		return ps6087Binding{}, false
	}
	return ps6087Binding{
		call:        call,
		function:    ps6087FunctionID(pass, call),
		object:      object,
		resultType:  signature.Results().At(dataIndex).Type(),
		resultCount: signature.Results().Len(),
	}, true
}

func ps6087ResultNames(statement ast.Stmt) ([]*ast.Ident, bool) {
	switch value := statement.(type) {
	case *ast.AssignStmt:
		names := make([]*ast.Ident, len(value.Lhs))
		for index, expression := range value.Lhs {
			id, ok := ps2110Unparen(expression).(*ast.Ident)
			if !ok {
				return nil, false
			}
			names[index] = id
		}
		return names, true
	case *ast.DeclStmt:
		declaration, ok := value.Decl.(*ast.GenDecl)
		if !ok || len(declaration.Specs) != 1 {
			return nil, false
		}
		spec, ok := declaration.Specs[0].(*ast.ValueSpec)
		if !ok || len(spec.Values) != 1 {
			return nil, false
		}
		return spec.Names, true
	default:
		return nil, false
	}
}

func ps6087ErrorType(value types.Type) bool {
	errorType := types.Universe.Lookup("error").Type()
	return types.AssignableTo(value, errorType)
}

func ps6087DataType(value types.Type) bool {
	if value == nil {
		return false
	}
	switch types.Unalias(value).Underlying().(type) {
	case *types.Slice, *types.Array:
		return true
	}
	name := ps6007NormalizeName(types.TypeString(value, func(*types.Package) string { return "" }))
	if ps6007ContainsAny(name, "recorder", "context", "operation", "dtype", "shape") {
		return false
	}
	return ps6007ContainsAny(name, "buffer", "buf", "tensor", "storage", "vector", "matrix", "logit", "qkv")
}

func ps6087FunctionID(pass *analysis.Pass, call *ast.CallExpr) string {
	function, signature, ok := typedCallee(pass, call.Fun)
	if !ok || function.Pkg() == nil {
		return ""
	}
	prefix := function.Pkg().Path() + "."
	if signature.Recv() == nil {
		return prefix + function.Name()
	}
	named := ps6087Named(signature.Recv().Type())
	if named == nil {
		return ""
	}
	return prefix + named.Obj().Name() + "." + function.Name()
}

func ps6087Named(value types.Type) *types.Named {
	value = types.Unalias(value)
	if pointer, ok := value.(*types.Pointer); ok {
		value = types.Unalias(pointer.Elem())
	}
	named, _ := value.(*types.Named)
	return named
}

func ps6087UseCounts(pass *analysis.Pass, body *ast.BlockStmt) map[types.Object]int {
	uses := make(map[types.Object]int)
	ast.Inspect(body, func(node ast.Node) bool {
		id, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		if object := pass.TypesInfo.Uses[id]; object != nil {
			uses[object]++
		}
		return true
	})
	return uses
}

func ps6087Parents(root ast.Node) map[ast.Node]ast.Node {
	parents := make(map[ast.Node]ast.Node)
	var stack []ast.Node
	ast.Inspect(root, func(node ast.Node) bool {
		if node == nil {
			stack = stack[:len(stack)-1]
			return false
		}
		if len(stack) > 0 {
			parents[node] = stack[len(stack)-1]
		}
		stack = append(stack, node)
		return true
	})
	return parents
}

func ps6087UniqueBinding(bindings []ps6087Binding) (ps6087Binding, bool) {
	if len(bindings) != 1 {
		return ps6087Binding{}, false
	}
	return bindings[0], true
}

func ps6087CallProvider(pass *analysis.Pass, call *ast.CallExpr) (ps6087Provider, bool) {
	selector, ok := ps2110Unparen(call.Fun).(*ast.SelectorExpr)
	if !ok {
		return ps6087Provider{}, false
	}
	receiver, ok := ps2110Unparen(selector.X).(*ast.Ident)
	if !ok {
		return ps6087Provider{}, false
	}
	object := pass.TypesInfo.Uses[receiver]
	if object == nil {
		object = pass.TypesInfo.Defs[receiver]
	}
	if _, ok := object.(*types.Var); !ok {
		return ps6087Provider{}, false
	}
	return ps6087Provider{object: object, typeOf: pass.TypesInfo.TypeOf(receiver)}, true
}

func ps6087DirectDataArgument(pass *analysis.Pass, call *ast.CallExpr, index int) (types.Object, types.Type, bool) {
	if index < 0 || index >= len(call.Args) {
		return nil, nil, false
	}
	id, ok := ps2110Unparen(call.Args[index]).(*ast.Ident)
	if !ok {
		return nil, nil, false
	}
	object := pass.TypesInfo.Uses[id]
	if _, ok := object.(*types.Var); !ok || !ps6087DataType(pass.TypesInfo.TypeOf(id)) {
		return nil, nil, false
	}
	return object, pass.TypesInfo.TypeOf(id), true
}

func ps6087Guard(pass *analysis.Pass, node ast.Node, provider types.Object, guard string, parents map[ast.Node]ast.Node) (*ast.IfStmt, bool) {
	for parent := parents[node]; parent != nil; parent = parents[parent] {
		statement, ok := parent.(*ast.IfStmt)
		if !ok || statement.Body == nil || !(statement.Body.Pos() <= node.Pos() && node.End() <= statement.Body.End()) {
			continue
		}
		call, ok := ps2110Unparen(statement.Cond).(*ast.CallExpr)
		if !ok || len(call.Args) != 0 || ps6087FunctionID(pass, call) != guard {
			continue
		}
		guardProvider, ok := ps6087CallProvider(pass, call)
		if ok && guardProvider.object == provider {
			return statement, true
		}
	}
	return nil, false
}

func ps6087PositionDominates(pass *analysis.Pass, body *ast.BlockStmt, before, after token.Pos) bool {
	graph := cfg.New(body, func(call *ast.CallExpr) bool { return !ps6079PanicCall(pass, call) })
	return ps6079GraphPositionDominates(graph, before, after)
}

func ps6087DirectTerminalUse(pass *analysis.Pass, body *ast.BlockStmt, object types.Object, parents map[ast.Node]ast.Node) bool {
	direct := false
	valid := true
	ast.Inspect(body, func(node ast.Node) bool {
		if !valid {
			return false
		}
		id, ok := node.(*ast.Ident)
		if !ok || pass.TypesInfo.Uses[id] != object {
			return true
		}
		expression := ast.Expr(id)
		parent := parents[id]
		for {
			paren, ok := parent.(*ast.ParenExpr)
			if !ok || paren.X != expression {
				break
			}
			expression = paren
			parent = parents[parent]
		}
		switch value := parent.(type) {
		case *ast.CallExpr:
			direct = false
			for _, argument := range value.Args {
				if argument == expression {
					direct = true
					break
				}
			}
		case *ast.ReturnStmt:
			direct = false
			for _, result := range value.Results {
				if result == expression {
					direct = true
					break
				}
			}
		default:
			direct = false
		}
		valid = direct
		return false
	})
	return valid && direct
}

func ps6087Capability(pass *analysis.Pass, providerType, gateType, otherType types.Type, contract *config.InPlaceFusionContract) (string, bool) {
	separator := strings.LastIndexByte(contract.CapabilityInterface, '.')
	if separator < 0 {
		return "", false
	}
	path := contract.CapabilityInterface[:separator]
	name := contract.CapabilityInterface[separator+1:]
	providerNamed := ps6087Named(providerType)
	if providerNamed == nil || providerNamed.Obj().Pkg() == nil || providerNamed.Obj().Pkg().Path() != path {
		return "", false
	}
	capabilityName, _ := providerNamed.Obj().Pkg().Scope().Lookup(name).(*types.TypeName)
	if capabilityName == nil || (capabilityName.Pkg() != pass.Pkg && (!ast.IsExported(capabilityName.Name()) || !ast.IsExported(contract.CapabilityMethod))) {
		return "", false
	}
	capabilityType := capabilityName.Type()
	if ps6087UninstantiatedGeneric(capabilityType) {
		return "", false
	}
	capability, ok := types.Unalias(capabilityType).Underlying().(*types.Interface)
	if !ok {
		return "", false
	}
	capability.Complete()
	if !capability.IsMethodSet() {
		return "", false
	}
	providerInterface, providerIsInterface := types.Unalias(providerType).Underlying().(*types.Interface)
	if providerIsInterface {
		providerInterface.Complete()
		if !types.AssertableTo(providerInterface, capabilityType) {
			return "", false
		}
	} else if !types.Implements(providerType, capability) {
		return "", false
	}
	var method *types.Func
	for index := 0; index < capability.NumMethods(); index++ {
		if capability.Method(index).Name() == contract.CapabilityMethod {
			method = capability.Method(index)
			break
		}
	}
	if method == nil {
		return "", false
	}
	signature, ok := method.Type().(*types.Signature)
	if !ok || signature.Params() == nil || signature.Results() == nil || signature.Variadic() || signature.Params().Len() != 2 || !types.Identical(signature.Params().At(0).Type(), gateType) || !types.Identical(signature.Params().At(1).Type(), otherType) || signature.Results().Len() != 1 {
		return "", false
	}
	result, ok := types.Unalias(signature.Results().At(0).Type()).Underlying().(*types.Basic)
	if !ok || result.Kind() != types.Bool {
		return "", false
	}
	return contract.CapabilityInterface, true
}

func ps6087UninstantiatedGeneric(value types.Type) bool {
	switch value := value.(type) {
	case *types.Named:
		params, args := value.TypeParams(), value.TypeArgs()
		return params != nil && params.Len() > 0 && (args == nil || args.Len() == 0)
	case *types.Alias:
		params, args := value.TypeParams(), value.TypeArgs()
		return params != nil && params.Len() > 0 && (args == nil || args.Len() == 0)
	default:
		return false
	}
}
