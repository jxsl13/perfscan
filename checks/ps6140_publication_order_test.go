package checks

import (
	"go/types"
	"testing"

	"golang.org/x/tools/go/ssa"
)

func TestPS6140SuccessfulPublicationOrder(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, before, after string
		want                bool
	}{
		{"early-nil-error", "", "", true},
		{"late-nil-error", "", "if choose{return nil,nil}", true},
		{"early-uninitialized-owner", "if choose{return d,nil}", "", false},
		{"alternate-owner", "", "if choose{return &owner{},nil}", false},
		{"unknown-owner", "", "if choose{return foreign,nil}", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, pkg := ps6136CellTestPackage(t, `package cells
type owner struct{flag bool};var foreign *owner
func factory(stop,choose bool)(*owner,error){if stop{return nil,nil};d:=&owner{};`+test.before+`;d.flag=true;`+test.after+`;return d,nil}
func New(stop,choose bool)(*owner,error){return factory(stop,choose)}
`)
			ownerType := pkg.Pkg.Scope().Lookup("owner").Type().(*types.Named)
			publication := ps6125NewSSAContext(pkg.Func("New"), nil, nil, 16384)
			var factory *ps6125SSAContext
			var returned *ssa.Return
			for _, block := range publication.flow.function.Blocks {
				for _, instruction := range block.Instrs {
					switch instruction := instruction.(type) {
					case *ssa.Call:
						if instruction.Call.StaticCallee() == pkg.Func("factory") {
							factory = ps6136Call(publication, instruction)
						}
					case *ssa.Return:
						returned = instruction
					}
				}
			}
			if factory == nil || returned == nil {
				t.Fatal("actual factory/publication prerequisite missing")
			}
			var store *ssa.Store
			var owner ps6125SSAReference
			for _, block := range factory.flow.function.Blocks {
				for _, instruction := range block.Instrs {
					candidate, ok := instruction.(*ssa.Store)
					if !ok {
						continue
					}
					if address, ok := candidate.Addr.(*ssa.FieldAddr); ok && types.Identical(address.X.Type(), types.NewPointer(ownerType)) {
						if store != nil {
							t.Fatal("ambiguous selected flag store")
						}
						store, owner = candidate, ps6136OwnerRoot(factory, address.X, ownerType)
					}
				}
			}
			if store == nil || owner.value == nil {
				t.Fatal("selected fresh owner/store prerequisite missing")
			}
			if ps6136ContextInstructionDominates(factory, store, publication, returned, 16384) {
				t.Fatal("early nil path must not guarantee unconditional initialization")
			}
			if got := ps6140PublicationInstructionDominates(factory, store, publication, returned, owner, ownerType, 16384); got != test.want {
				t.Fatalf("successful owner publication ordered=%v want=%v", got, test.want)
			}
			if ps6140PublicationInstructionDominates(factory, store, publication, returned, owner, ownerType, 0) {
				t.Fatal("exhausted publication budget admitted")
			}
		})
	}
}
