package checks

import (
	"go/token"
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// ps6136ConstructorWrites is a fresh-owner source census, not permission to
// assume selected constructor geometry. Every initialization target is tied
// to an actual invocation's fresh allocation. Selected final field values and
// post-publication invariance must be checked independently.
type ps6136ConstructorWrites struct {
	positions  map[token.Pos]bool
	writers    map[*ssa.Function]bool
	contexts   map[*ssa.Function]bool
	calls      map[*ssa.Call]bool
	reads      map[token.Pos]bool
	readFields map[token.Pos]*types.Var
}

func ps6136FreshConstructorWrites(pkg *ssa.Package, owner *types.Named, fields map[*types.Var]bool) *ps6136ConstructorWrites {
	inventory := ps6136ConstructorInventory(pkg, owner)
	if inventory == nil {
		return nil
	}
	result := &ps6136ConstructorWrites{positions: make(map[token.Pos]bool), writers: make(map[*ssa.Function]bool), contexts: make(map[*ssa.Function]bool), calls: make(map[*ssa.Call]bool), reads: make(map[token.Pos]bool), readFields: make(map[token.Pos]*types.Var)}
	remaining := 16384
	for function := range inventory {
		pending := []*ps6125SSAContext{ps6125NewSSAContext(function, nil, nil, 1024)}
		seen := make(map[*ps6125SSAContext]bool)
		for len(pending) != 0 {
			context := pending[len(pending)-1]
			pending = pending[:len(pending)-1]
			if seen[context] {
				continue
			}
			seen[context] = true
			remaining--
			if remaining < 0 {
				return nil
			}
			result.contexts[context.flow.function] = true
			paths := ps6125AccessPaths{flow: context.flow}
			for _, block := range context.flow.function.Blocks {
				if !context.flow.blocks[block] {
					continue
				}
				for _, instruction := range block.Instrs {
					if load, ok := instruction.(*ssa.UnOp); ok && load.Op == token.MUL {
						path := paths.resolve(load.X)
						if path.known && len(path.access.fields) == 1 && fields[path.access.fields[0]] {
							if _, fresh := ps6136AccessOwnerRoot(context, path, owner).value.(*ssa.Alloc); !fresh {
								return nil
							}
							result.reads[load.Pos()] = true
							result.readFields[load.Pos()] = path.access.fields[0]
						}
					}
					if write, ok := instruction.(*ssa.Store); ok {
						if address, ok := write.Addr.(*ssa.FieldAddr); ok {
							pointer, ok := address.X.Type().Underlying().(*types.Pointer)
							if ok {
								if structure, ok := pointer.Elem().Underlying().(*types.Struct); ok && fields[structure.Field(address.Field)] {
									root := address.X
									for {
										parent, ok := root.(*ssa.FieldAddr)
										if !ok {
											break
										}
										root = parent.X
									}
									if types.Identical(root.Type(), types.NewPointer(owner)) {
										if _, fresh := ps6136OwnerRoot(context, root, owner).value.(*ssa.Alloc); !fresh || !write.Pos().IsValid() {
											return nil
										}
										result.positions[write.Pos()] = true
										result.writers[context.flow.function] = true
										continue
									}
									if _, localValue := root.(*ssa.Alloc); localValue && !types.Identical(root.Type(), types.NewPointer(owner)) {
										continue
									}
								}
							}
						}
						if address, ok := write.Addr.(*ssa.FieldAddr); ok && types.Identical(address.X.Type(), types.NewPointer(owner)) {
							structure := owner.Underlying().(*types.Struct)
							if fields[structure.Field(address.Field)] {
								if _, fresh := ps6136OwnerRoot(context, address.X, owner).value.(*ssa.Alloc); !fresh || !write.Pos().IsValid() {
									return nil
								}
								result.positions[write.Pos()] = true
								result.writers[context.flow.function] = true
								continue
							}
						}
						path := paths.resolve(write.Addr)
						if !path.known || len(path.access.fields) == 0 || !fields[path.access.fields[len(path.access.fields)-1]] {
							continue
						}
						identity := ps6136AccessOwnerRoot(context, path, owner)
						if _, fresh := identity.value.(*ssa.Alloc); !fresh || !write.Pos().IsValid() {
							return nil
						}
						result.positions[write.Pos()] = true
						result.writers[context.flow.function] = true
					}
					call, ok := instruction.(*ssa.Call)
					if !ok {
						continue
					}
					if child := ps6136Call(context, call); child != nil {
						for _, argument := range call.Call.Args {
							if types.Identical(argument.Type(), types.NewPointer(owner)) {
								if _, fresh := ps6136OwnerRoot(context, argument, owner).value.(*ssa.Alloc); !fresh {
									return nil
								}
								result.calls[call] = true
							}
						}
						pending = append(pending, child)
					} else {
						for _, argument := range call.Call.Args {
							if types.Identical(argument.Type(), types.NewPointer(owner)) || types.Identical(argument.Type(), owner) {
								return nil // Unknown source ownership edge.
							}
						}
					}
				}
			}
		}
	}
	return result
}
