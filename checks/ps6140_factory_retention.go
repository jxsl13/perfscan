package checks

import (
	"go/token"
	"go/types"

	"golang.org/x/tools/go/ssa"
)

type ps6140FactoryRetentionProof struct {
	context *ps6125SSAContext
	append  *ssa.Call
	store   *ssa.Store
	load    *ssa.UnOp
	fields  map[*ssa.FieldAddr]bool
}

// Strengthen result provenance to an executed append and closed list uses.
// An append merely present in a conditional branch does not retain every
// successful result. This receipt does not grant native ownership or prove
// list isolation in callers; their observations need a separate census.
func ps6140FactoryRetention(context *ps6125SSAContext, backend *ssa.Call, owner ps6125SSAReference, ownerType *types.Named, list, slot *types.Var, budget int) *ps6140FactoryRetentionProof {
	if budget <= 0 || !ps6136FactoryRetention(context, backend, owner, ownerType, list, slot) {
		return nil
	}
	proof := &ps6140FactoryRetentionProof{context: context, fields: make(map[*ssa.FieldAddr]bool)}
	paths := ps6125AccessPaths{flow: context.flow}
	for _, block := range context.flow.function.Blocks {
		if !context.flow.blocks[block] {
			continue
		}
		for _, instruction := range block.Instrs {
			budget--
			if budget <= 0 {
				return nil
			}
			address, ok := instruction.(*ssa.FieldAddr)
			if !ok {
				continue
			}
			path := paths.resolve(address)
			if !path.known || len(path.access.fields) != 1 || path.access.fields[0] != list {
				continue
			}
			if ps6136AccessOwnerRoot(context, path, ownerType) != owner || address.Referrers() == nil {
				return nil
			}
			proof.fields[address] = true
			for _, user := range *address.Referrers() {
				budget--
				if budget <= 0 || user.Parent() != context.flow.function {
					return nil
				}
				if !context.flow.blocks[user.Block()] {
					continue
				}
				switch use := user.(type) {
				case *ssa.DebugRef:
				case *ssa.UnOp:
					if use.Op != token.MUL || use.X != address || proof.load != nil && proof.load != use {
						return nil
					}
					proof.load = use
				case *ssa.Store:
					call, ok := use.Val.(*ssa.Call)
					if !ok || use.Addr != address || proof.store != nil && proof.store != use {
						return nil
					}
					proof.store, proof.append = use, call
				default:
					return nil
				}
			}
		}
	}
	if proof.store == nil || proof.load == nil || len(proof.append.Call.Args) != 2 || proof.append.Call.Args[0] != proof.load || !ps6140OnlyConstructorUse(context, proof.load, proof.append, &budget) || !ps6140OnlyConstructorUse(context, proof.append, proof.store, &budget) {
		return nil
	}
	publications := 0
	for _, block := range context.flow.function.Blocks {
		if !context.flow.blocks[block] {
			continue
		}
		for _, instruction := range block.Instrs {
			budget--
			if budget <= 0 {
				return nil
			}
			returned, ok := instruction.(*ssa.Return)
			if !ok {
				continue
			}
			_, fields, known := context.returnedFields(returned, 0)
			if !known {
				return nil
			}
			if fields[slot].value == nil {
				continue
			}
			result, ok := fields[slot].value.(*ssa.Extract)
			if !ok || fields[slot].context != context || result.Index != 0 || result.Tuple != backend || !context.flow.instructionDominates(proof.store, returned) {
				return nil
			}
			publications++
		}
	}
	if publications != 1 {
		return nil
	}
	return proof
}
