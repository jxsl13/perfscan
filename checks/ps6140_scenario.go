package checks

import (
	"go/token"
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// A conditional object-sensitive method analysis: the receiver denotes an
// object returned by this source-proved constructor/backend. It is not an
// actual caller/return-value association (ps6140RuntimeFlag handles that).
// Results are valid only for this constructor class and must remain attached
// to its allocation proof. They cannot classify an arbitrary Owner instance.
type ps6140ConstructorScenario struct {
	collection *ps6140SelectedCollectionProof
	flags      *ps6140ImmutableFlags
	ownerType  *types.Named
	root       *ps6125SSAContext
	receiver   ps6125SSAReference
	refined    map[*ps6125SSAContext]bool
	remaining  int
}

func ps6140NewConstructorScenario(collection *ps6140SelectedCollectionProof, flags *ps6140ImmutableFlags, owner *types.Named, method *ssa.Function, budget int) *ps6140ConstructorScenario {
	if owner == nil || method == nil || method.Signature.Recv() == nil || !types.Identical(method.Signature.Recv().Type(), types.NewPointer(owner)) {
		return nil
	}
	return ps6140NewConstructorScenarioEntry(collection, flags, owner, method, 0, budget)
}

// An exported function may accept the selected owner in a non-receiver role.
// Each exact formal gets its own scenario; sibling owner inputs remain unknown.
func ps6140NewConstructorScenarioEntry(collection *ps6140SelectedCollectionProof, flags *ps6140ImmutableFlags, owner *types.Named, method *ssa.Function, parameter, budget int) *ps6140ConstructorScenario {
	if collection == nil || flags == nil || flags.snapshot == nil || owner == nil || method == nil || len(method.Blocks) == 0 || parameter < 0 || parameter >= len(method.Params) || budget <= 0 {
		return nil
	}
	snapshot := flags.snapshot
	if collection.constructor == nil || collection.constructor.flow == nil || collection.constructor != snapshot.constructor || collection.owner != snapshot.owner || collection.owner.value == nil || !types.Identical(collection.owner.value.Type(), types.NewPointer(owner)) || method.Pkg != collection.constructor.flow.function.Pkg || !types.Identical(method.Params[parameter].Type(), types.NewPointer(owner)) {
		return nil
	}
	root := ps6125NewSSAContext(method, nil, nil, budget)
	if root == nil {
		return nil
	}
	return &ps6140ConstructorScenario{collection: collection, flags: flags, ownerType: owner, root: root, receiver: root.reference(method.Params[parameter]), refined: make(map[*ps6125SSAContext]bool), remaining: budget}
}

// Only the scenario's actual method formal and its source-forwarded aliases
// can select these facts. A second parameter, local construction or uncertain
// phi remains unknown, even if it has the same owner type.
func (scenario *ps6140ConstructorScenario) flag(context *ps6125SSAContext, load *ssa.UnOp) (bool, bool) {
	if scenario == nil || context == nil || load == nil || load.Op != token.MUL || load.Parent() != context.flow.function {
		return false, false
	}
	address, ok := load.X.(*ssa.FieldAddr)
	structure, structured := scenario.ownerType.Underlying().(*types.Struct)
	if !ok || !structured || !types.Identical(address.X.Type(), types.NewPointer(scenario.ownerType)) || address.Field < 0 || address.Field >= structure.NumFields() || ps6136OwnerRoot(context, address.X, scenario.ownerType) != scenario.receiver {
		return false, false
	}
	truth, known := scenario.flags.snapshot.values[structure.Field(address.Field)]
	return truth, known
}

func (scenario *ps6140ConstructorScenario) refine(context *ps6125SSAContext) bool {
	if scenario == nil || context == nil || context.flow == nil || scenario.remaining <= 0 {
		return false
	}
	if scenario.refined[context] {
		return true
	}
	if context != scenario.root && (context.parent == nil || !scenario.refined[context.parent] || context.site == nil || context.parent.calls[context.site] != context) {
		return false // Do not mutate unrelated/shared contexts or their caches.
	}
	immutable := make(map[ssa.Value]bool)
	for _, block := range context.flow.function.Blocks {
		for _, instruction := range block.Instrs {
			scenario.remaining--
			if scenario.remaining <= 0 {
				return false
			}
			if !context.flow.blocks[block] {
				continue
			}
			if load, ok := instruction.(*ssa.UnOp); ok {
				if truth, known := scenario.flag(context, load); known {
					immutable[load] = truth
				}
			}
			if comparison, ok := instruction.(*ssa.BinOp); ok {
				if truth, known := scenario.nilComparison(context, comparison); known {
					immutable[comparison] = truth
				}
			}
		}
	}
	inputs := make(map[*ssa.Parameter]ps6125Scalar, len(context.flow.function.Params))
	for _, parameter := range context.flow.function.Params {
		inputs[parameter] = context.scalar(parameter)
	}
	flow := ps6125AnalyzeSSAExtentsWithImmutableBools(context.flow.function, inputs, context.flow.lengths, immutable)
	if flow == nil {
		return false
	}
	context.flow = flow
	// A scenario owns these contexts exclusively. Clear analysis derived from
	// their older, less precise CFG before constructing any child invocations.
	context.calls, context.resolved, context.resolving = nil, nil, nil
	context.cells, context.structs = nil, nil
	scenario.refined[context] = true
	return true
}

func (scenario *ps6140ConstructorScenario) nilComparison(context *ps6125SSAContext, comparison *ssa.BinOp) (bool, bool) {
	if comparison.Op != token.EQL && comparison.Op != token.NEQ {
		return false, false
	}
	value, other := comparison.X, comparison.Y
	if literal, ok := value.(*ssa.Const); ok && literal.IsNil() {
		value, other = other, value
	}
	literal, nilOther := other.(*ssa.Const)
	if !nilOther || !literal.IsNil() {
		return false, false
	}
	block, ok := scenario.collection.appended.value.Type().(*types.Named)
	if !ok {
		return false, false
	}
	for field := range scenario.collection.nilFields {
		if ps6140RuntimeProjectionMember(context, value, scenario.receiver, scenario.ownerType, block, scenario.collection.blocks, field, 256) {
			return comparison.Op == token.EQL, true
		}
	}
	return false, false
}

// Unknown calls are still visited, and stopping at a call requires the caller's
// independent leaf proof. This traversal alone is not a complete observation,
// native effect, lifetime or unused-buffer proof.
func (scenario *ps6140ConstructorScenario) walk(context *ps6125SSAContext, visit func(*ps6125SSAContext, *ssa.Call) bool) bool {
	if visit == nil || !scenario.refine(context) {
		return false
	}
	for _, block := range context.flow.function.Blocks {
		if !context.flow.blocks[block] {
			continue
		}
		for _, instruction := range block.Instrs {
			scenario.remaining--
			if scenario.remaining <= 0 {
				return false
			}
			call, ok := instruction.(*ssa.Call)
			if !ok || visit(context, call) {
				continue
			}
			if child := ps6136Call(context, call); child != nil && !scenario.walk(child, visit) {
				return false
			}
		}
	}
	return true
}
