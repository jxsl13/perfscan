package checks

import (
	"go/types"
	"testing"

	"golang.org/x/tools/go/ssa"
)

func TestPS6140PublicFailureCleanup(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, public string
		want         bool
	}{
		{"direct-forward", "return makeOwner()", true},
		{"two-forwarding-levels", "return wrap()", true},
		{"failure-before-construction", "if fail{return nil,problem};return makeOwner()", true},
		{"discard-without-cleanup", "d,e:=makeOwner();if fail{return nil,problem};return d,e", false},
		{"direct-cleanup", "d,e:=makeOwner();if fail{d.Release();return nil,problem};return d,e", true},
		{"helper-cleanup", "d,e:=makeOwner();if fail{cleanup(d);return nil,problem};return d,e", true},
		{"bound-method-cleanup", "d,e:=makeOwner();if fail{f:=d.Release;f();return nil,problem};return d,e", true},
		{"returned-callback-cleanup", "d,e:=makeOwner();if fail{f:=releaseClosure(d);invoke(f);return nil,problem};return d,e", true},
		{"conditional-helper-cleanup", "d,e:=makeOwner();if fail{maybeCleanup(d);return nil,problem};return d,e", false},
		{"foreign-owner-cleanup", "d,e:=makeOwner();if fail{foreign.Release();return nil,problem};return d,e", false},
		{"deferred-cleanup", "d,e:=makeOwner();if fail{deferredCleanup(d);return nil,problem};return d,e", false},
		{"unproved-child-error-correlation", "d,e:=makeOwner();if e!=nil{return nil,e};return d,nil", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			pkg := ps6125TestSSA(t, `package extent
type owner struct{closed bool};var fail,other bool;var problem error;var foreign *owner
func(d *owner)Release(){d.closed=true}
func cleanup(d *owner){d.Release()}
func maybeCleanup(d *owner){if other{d.Release()}}
func deferredCleanup(d *owner){defer d.Release()}
func releaseClosure(d *owner)func(){return func(){d.Release()}}
func invoke(f func()){f()}
func makeOwner()(*owner,error){d:=&owner{};if fail{d.Release();return nil,problem};return d,nil}
func wrap()(*owner,error){return makeOwner()}
func New()(*owner,error){`+test.public+`}
`)
			ownerType := pkg.Pkg.Scope().Lookup("owner").Type().(*types.Named)
			release, _, _ := types.LookupFieldOrMethod(types.NewPointer(ownerType), false, pkg.Pkg, "Release")
			publication := ps6125NewSSAContext(pkg.Func("New"), nil, nil, 16384)
			var constructor *ps6125SSAContext
			var owner ps6125SSAReference
			pending := []*ps6125SSAContext{publication}
			for len(pending) != 0 {
				context := pending[len(pending)-1]
				pending = pending[:len(pending)-1]
				for _, call := range ps6136Calls(context) {
					callee := call.Call.StaticCallee()
					if callee != pkg.Func("makeOwner") && callee != pkg.Func("wrap") {
						continue
					}
					child := ps6136Call(context, call)
					if child == nil {
						t.Fatal("actual constructor forwarding missing")
					}
					if callee == pkg.Func("wrap") {
						pending = append(pending, child)
						continue
					}
					if constructor != nil {
						t.Fatal("ambiguous selected constructor")
					}
					constructor = child
					for _, block := range child.flow.function.Blocks {
						for _, instruction := range block.Instrs {
							if allocation, ok := instruction.(*ssa.Alloc); ok && types.Identical(allocation.Type(), types.NewPointer(ownerType)) {
								owner = child.reference(allocation)
							}
						}
					}
				}
			}
			budget := 16384
			if constructor == nil || owner.value == nil || len(ps6140SameOwnerPublications(publication, owner, ownerType, &budget)) == 0 {
				t.Fatal("genuine selected constructor/fresh owner/successful public-return prerequisites missing")
			}
			if got := ps6140PublicFailureCleanup(publication, constructor, owner, ownerType, release.(*types.Func), 16384); got != test.want {
				t.Fatalf("outer constructor failure cleanup=%v want=%v", got, test.want)
			}
			if ps6140PublicFailureCleanup(publication, constructor, owner, ownerType, release.(*types.Func), 0) ||
				ps6140PublicFailureCleanup(publication, ps6125NewSSAContext(pkg.Func("makeOwner"), nil, nil, 16384), owner, ownerType, release.(*types.Func), 16384) {
				t.Fatal("exhausted budget or unrelated constructor invocation admitted")
			}
		})
	}
}
