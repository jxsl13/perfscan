package checks

import (
	"go/types"
	"testing"

	"golang.org/x/tools/go/ssa"
)

func TestPS6140ConstructorReleasePhase(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, body, public string
		want               bool
	}{
		{"no-release", "return d,nil", "", true},
		{"failed-owner-cleanup", "if fail{d.Release();return nil,problem};return d,nil", "", true},
		{"failed-owner-helper-cleanup", "if fail{cleanup(d);return nil,problem};return d,nil", "", true},
		{"failed-owner-callback-cleanup", "if fail{f:=d.Release;f();return nil,problem};return d,nil", "", true},
		{"failed-owner-forwarded-twice", "if fail{d.Release();return nil,problem};return d,nil", "return wrap()", true},
		{"failed-owner-checked-public-return", "if fail{d.Release();return nil,problem};return d,nil", "d,e:=makeOwner();if e!=nil{return nil,e};return d,nil", true},
		{"two-success-returns", "if fail{return d,problem};return d,nil", "", true},
		{"early-release", "d.Release();return d,nil", "", false},
		{"conditional-early-release", "if fail{d.Release()};return d,nil", "", false},
		{"conditional-else-release", "if fail{}else{d.Release()};return d,nil", "", false},
		{"nonnil-error-is-publication", "if fail{d.Release();return d,problem};return d,nil", "", false},
		{"helper-early-release", "cleanup(d);return d,nil", "", false},
		{"conditional-helper-early-release", "maybeCleanup(d);return d,nil", "", false},
		{"bound-method-early-release", "f:=d.Release;f();return d,nil", "", false},
		{"closure-early-release", "f:=func(){d.Release()};f();return d,nil", "", false},
		{"sibling-helper-early-release", "f:=releaseClosure(d);invoke(f);return d,nil", "", false},
		{"deferred-release", "defer d.Release();return d,nil", "", false},
		{"goroutine-release", "go d.Release();return d,nil", "", false},
		{"unknown-owner-effect", "opaque(d);return d,nil", "", false},
		{"unknown-owner-receiver", "foreign.Release();return d,nil", "", false},
		{"recursive-owner-effect", "recurse(d);return d,nil", "", false},
		{"public-wrapper-release", "return d,nil", "d,e:=makeOwner();d.Release();return d,e", false},
		{"public-wrapper-helper-release", "return d,nil", "d,e:=makeOwner();cleanup(d);return d,e", false},
		{"public-wrapper-conditional-release", "return d,nil", "d,e:=makeOwner();if fail{d.Release()};return d,e", false},
		{"public-wrapper-failed-cleanup", "return d,nil", "d,e:=makeOwner();if fail{d.Release();return nil,problem};return d,e", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			public := test.public
			if public == "" {
				public = "return makeOwner()"
			}
			pkg := ps6125TestSSA(t, `package extent
type owner struct{closed bool}
func(d *owner)Release(){d.closed=true}
var fail bool;var problem error;var foreign *owner;var opaque func(*owner)
func cleanup(d *owner){d.Release()}
func maybeCleanup(d *owner){if fail{d.Release()}}
func releaseClosure(d *owner)func(){return func(){d.Release()}}
func invoke(f func()){f()}
func recurse(d *owner){if fail{recurse(d)};d.Release()}
func makeOwner()(*owner,error){d:=&owner{};`+test.body+`}
func wrap()(*owner,error){return makeOwner()}
func New()(*owner,error){`+public+`}
`)
			ownerType := pkg.Pkg.Scope().Lookup("owner").Type().(*types.Named)
			release, _, _ := types.LookupFieldOrMethod(types.NewPointer(ownerType), false, pkg.Pkg, "Release")
			publication := ps6125NewSSAContext(pkg.Func("New"), nil, nil, 16384)
			var owner ps6125SSAReference
			pending := []*ps6125SSAContext{publication}
			seen := make(map[*ps6125SSAContext]bool)
			for len(pending) != 0 {
				context := pending[len(pending)-1]
				pending = pending[:len(pending)-1]
				if seen[context] {
					continue
				}
				seen[context] = true
				if context.flow.function == pkg.Func("makeOwner") {
					for _, block := range context.flow.function.Blocks {
						for _, instruction := range block.Instrs {
							if allocation, ok := instruction.(*ssa.Alloc); ok && types.Identical(allocation.Type(), types.NewPointer(ownerType)) {
								if owner.value != nil {
									t.Fatal("ambiguous fresh-owner prerequisite")
								}
								owner = context.reference(allocation)
							}
						}
					}
				}
				for _, call := range ps6136Calls(context) {
					if child := ps6136Call(context, call); child != nil {
						pending = append(pending, child)
					}
				}
			}
			budget := 16384
			if owner.value == nil || len(ps6140SameOwnerPublications(publication, owner, ownerType, &budget)) == 0 {
				t.Fatal("actual fresh-owner and successful public-return prerequisites missing")
			}
			if got := ps6140ConstructorReleasePhase(publication, owner, ownerType, release.(*types.Func), 16384); got != test.want {
				t.Fatalf("release excluded before successful owner publication=%v want=%v", got, test.want)
			}
			if ps6140ConstructorReleasePhase(publication, owner, ownerType, release.(*types.Func), 0) {
				t.Fatal("exhausted release-phase budget admitted")
			}
		})
	}
}
