package checks

import (
	"go/constant"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// Scalar facts form a finite-height lattice: pending -> one exact geometry or
// boolean -> unknown. Pending means not evaluated on any executable edge; it
// must never be interpreted as zero, false, or an unreachable source value.
type ps6125Scalar struct {
	state  uint8
	truth  bool
	extent ps6125Extent
}

const (
	ps6125Pending uint8 = iota
	ps6125Integer
	ps6125Boolean
	ps6125Unknown
)

func ps6125IntegerFact(extent ps6125Extent) ps6125Scalar {
	if !extent.known {
		return ps6125Scalar{state: ps6125Unknown}
	}
	return ps6125Scalar{state: ps6125Integer, extent: extent}
}

func ps6125JoinScalars(left, right ps6125Scalar) ps6125Scalar {
	if left.state == ps6125Pending {
		return right
	}
	if right.state == ps6125Pending {
		return left
	}
	if left.state == right.state && (left.state == ps6125Boolean && left.truth == right.truth || left.state == ps6125Integer && ps6125SameExtent(left.extent, right.extent)) {
		return left
	}
	return ps6125Scalar{state: ps6125Unknown}
}

type ps6125SSAEdge struct{ from, to *ssa.BasicBlock }

type ps6125SSAExtents struct {
	function *ssa.Function
	facts    map[ssa.Value]ps6125Scalar
	lengths  map[*ssa.Parameter]ps6125Extent
	blocks   map[*ssa.BasicBlock]bool
	edges    map[ps6125SSAEdge]bool
}

// ps6125AnalyzeSSAExtents specializes scalar control/data flow for one invocation.
// Input facts must belong to the actual call's parameters. It does not infer
// heap/receiver field invariants, effects of opaque calls, allocation lifetimes,
// hotness or runtime no-overflow. Loads and unresolved calls stay unknown.
func ps6125AnalyzeSSAExtents(function *ssa.Function, inputs map[*ssa.Parameter]ps6125Scalar, lengths map[*ssa.Parameter]ps6125Extent) *ps6125SSAExtents {
	result := &ps6125SSAExtents{function: function, facts: make(map[ssa.Value]ps6125Scalar), lengths: lengths, blocks: make(map[*ssa.BasicBlock]bool), edges: make(map[ps6125SSAEdge]bool)}
	if function == nil || len(function.Blocks) == 0 {
		return result
	}
	for _, parameter := range function.Params {
		fact, ok := inputs[parameter]
		if !ok || fact.state == ps6125Pending {
			fact = ps6125Scalar{state: ps6125Unknown}
		}
		result.facts[parameter] = fact
	}
	var queue []ssa.Instruction
	queued := make(map[ssa.Instruction]bool)
	enqueue := func(instruction ssa.Instruction) {
		if result.blocks[instruction.Block()] && !queued[instruction] {
			queue = append(queue, instruction)
			queued[instruction] = true
		}
	}
	activate := func(from, to *ssa.BasicBlock) {
		edge := ps6125SSAEdge{from: from, to: to}
		if result.edges[edge] {
			return
		}
		result.edges[edge] = true
		result.blocks[to] = true
		for _, instruction := range to.Instrs {
			enqueue(instruction)
		}
	}
	activate(nil, function.Blocks[0])
	for len(queue) != 0 {
		instruction := queue[0]
		queue = queue[1:]
		queued[instruction] = false
		if value, ok := instruction.(ssa.Value); ok {
			old := result.facts[value]
			joined := ps6125JoinScalars(old, result.evaluate(value))
			// A known lattice element can only stay itself or become unknown.
			if joined.state != old.state {
				result.facts[value] = joined
				if users := value.Referrers(); users != nil {
					for _, user := range *users {
						enqueue(user)
					}
				}
			}
		}
		switch control := instruction.(type) {
		case *ssa.Jump:
			activate(control.Block(), control.Block().Succs[0])
		case *ssa.If:
			fact := result.scalar(control.Cond)
			if fact.state == ps6125Boolean {
				index := 1
				if fact.truth {
					index = 0
				}
				activate(control.Block(), control.Block().Succs[index])
			} else if fact.state != ps6125Pending {
				for _, next := range control.Block().Succs {
					activate(control.Block(), next)
				}
			}
		}
	}
	return result
}

func (flow *ps6125SSAExtents) scalar(value ssa.Value) ps6125Scalar {
	if literal, ok := value.(*ssa.Const); ok {
		if literal.Value != nil {
			switch literal.Value.Kind() {
			case constant.Bool:
				return ps6125Scalar{state: ps6125Boolean, truth: constant.BoolVal(literal.Value)}
			case constant.Int:
				integer, exact := constant.Int64Val(literal.Value)
				if exact {
					return ps6125IntegerFact(ps6125ConstantExtent(integer))
				}
			}
		}
		return ps6125Scalar{state: ps6125Unknown}
	}
	if fact, exists := flow.facts[value]; exists {
		return fact
	}
	if instruction, local := value.(ssa.Instruction); local && instruction.Parent() == flow.function {
		return ps6125Scalar{}
	}
	return ps6125Scalar{state: ps6125Unknown}
}

func (flow *ps6125SSAExtents) evaluate(value ssa.Value) ps6125Scalar {
	unknown := ps6125Scalar{state: ps6125Unknown}
	switch instruction := value.(type) {
	case *ssa.Phi:
		var joined ps6125Scalar
		for index, predecessor := range instruction.Block().Preds {
			if flow.edges[ps6125SSAEdge{from: predecessor, to: instruction.Block()}] {
				joined = ps6125JoinScalars(joined, flow.scalar(instruction.Edges[index]))
			}
		}
		return joined
	case *ssa.BinOp:
		left, right := flow.scalar(instruction.X), flow.scalar(instruction.Y)
		if left.state == ps6125Pending || right.state == ps6125Pending {
			return ps6125Scalar{}
		}
		if instruction.Op == token.MUL && left.state == ps6125Integer && right.state == ps6125Integer && types.Identical(instruction.Type().Underlying(), types.Typ[types.Int]) {
			return ps6125IntegerFact(ps6125MultiplyExtents(left.extent, right.extent))
		}
		if left.state == ps6125Boolean && right.state == ps6125Boolean && (instruction.Op == token.EQL || instruction.Op == token.NEQ) {
			return ps6125Scalar{state: ps6125Boolean, truth: (left.truth == right.truth) == (instruction.Op == token.EQL)}
		}
	case *ssa.UnOp:
		if instruction.Op == token.NOT {
			operand := flow.scalar(instruction.X)
			if operand.state == ps6125Pending {
				return operand
			}
			if operand.state == ps6125Boolean {
				return ps6125Scalar{state: ps6125Boolean, truth: !operand.truth}
			}
		}
	case *ssa.Call:
		if builtin, ok := instruction.Call.Value.(*ssa.Builtin); ok && builtin.Name() == "len" && len(instruction.Call.Args) == 1 {
			if parameter, ok := instruction.Call.Args[0].(*ssa.Parameter); ok && parameter.Parent() == flow.function {
				// A slice/string descriptor is an SSA value with stable length.
				// Map/channel length can change without rebinding the parameter;
				// an entry fact is not a point fact after effects on those objects.
				switch typ := parameter.Type().Underlying().(type) {
				case *types.Slice:
					return ps6125IntegerFact(flow.lengths[parameter])
				case *types.Basic:
					if typ.Info()&types.IsString != 0 {
						return ps6125IntegerFact(flow.lengths[parameter])
					}
				}
			}
			if allocation, ok := instruction.Call.Args[0].(*ssa.MakeSlice); ok {
				return flow.scalar(allocation.Len)
			}
		}
	}
	return unknown
}

// callInputs carries only source-derived scalar/length facts into a closed
// static callee. Receiver/heap identities and closures require separate proofs;
// they are not manufactured by matching names or concrete type spellings.
func (flow *ps6125SSAExtents) callInputs(call *ssa.Call) (*ssa.Function, map[*ssa.Parameter]ps6125Scalar, map[*ssa.Parameter]ps6125Extent) {
	if call == nil || call.Parent() != flow.function || !flow.blocks[call.Block()] || call.Call.IsInvoke() {
		return nil, nil, nil
	}
	callee := call.Call.StaticCallee()
	if callee == nil || len(callee.Blocks) == 0 || len(callee.FreeVars) != 0 || len(callee.Params) != len(call.Call.Args) {
		return nil, nil, nil
	}
	inputs := make(map[*ssa.Parameter]ps6125Scalar, len(callee.Params))
	lengths := make(map[*ssa.Parameter]ps6125Extent)
	for index, parameter := range callee.Params {
		argument := call.Call.Args[index]
		inputs[parameter] = flow.scalar(argument)
		if original, ok := argument.(*ssa.Parameter); ok && original.Parent() == flow.function {
			lengths[parameter] = flow.lengths[original]
		}
	}
	return callee, inputs, lengths
}
