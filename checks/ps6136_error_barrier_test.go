package checks

import (
	"go/types"
	"strings"
	"testing"

	"golang.org/x/tools/go/ssa"
)

func TestPS6136ConstructorFailurePublicationEdges(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, barrier string
		want          bool
	}{
		{"canonical", "if err!=nil{d.Release();return nil,err};return d,nil", true},
		{"equivalent", "if nil!=err{d.Release();return nil,err};return d,nil", true},
		{"no-release", "if err!=nil{return nil,err};return d,nil", false},
		{"wrong-release", "if err!=nil{d.Other();return nil,err};return d,nil", false},
		{"wrong-owner", "if err!=nil{(&owner{}).Release();return nil,err};return d,nil", false},
		{"publish-failure", "if err!=nil{d.Release();return d,err};return d,nil", false},
		{"discard-error", "if err!=nil{d.Release();return nil,nil};return d,nil", false},
		{"unguarded", "return d,nil", false},
		{"late-allocation", "if err!=nil{d.Release();return nil,err};mk();return d,nil", false},
		{"late-error-write", "if err!=nil{d.Release();return nil,err};err=native();return d,nil", false},
		{"late-error-reset", "if err!=nil{d.Release();return nil,err};err=nil;return d,nil", false},
		{"stale-error-load", "old:=err;mk();if old!=nil{d.Release();return nil,err};return d,nil", false},
		{"stale-error-condition", "bad:=err!=nil;mk();if bad{d.Release();return nil,err};return d,nil", false},
		{"wrong-state", "var other error;if other!=nil{d.Release();return nil,other};return d,nil", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			pkg := ps6125TestSSA(t, strings.Replace(`package extent
type owner struct{}
func(*owner)Release(){}
func(*owner)Other(){}
func native()error{return nil}
func construct()(*owner,error){
 d:=&owner{};var err error
 mk:=func(){e:=native();if e!=nil{err=e}}
 mk()
 _ = err
 BARRIER
}`, "BARRIER", test.barrier, 1))
			function := pkg.Func("construct")
			context := ps6125NewSSAContext(function, nil, nil, 64)
			ownerType := pkg.Pkg.Scope().Lookup("owner").Type().(*types.Named)
			var owner, cell ps6125SSAReference
			for _, block := range function.Blocks {
				for _, instruction := range block.Instrs {
					allocation, ok := instruction.(*ssa.Alloc)
					if !ok {
						continue
					}
					if types.Identical(allocation.Type(), types.NewPointer(ownerType)) && owner.value == nil {
						owner = context.reference(allocation)
					}
					if types.Identical(allocation.Type(), types.NewPointer(types.Universe.Lookup("error").Type())) && allocation.Comment == "err" {
						cell = context.reference(allocation)
					}
				}
			}
			method, _, _ := types.LookupFieldOrMethod(ownerType, true, pkg.Pkg, "Release")
			if owner.value == nil || cell.value == nil {
				t.Fatal("fixture source identities unavailable")
			}
			if got := ps6136ConstructorErrorBarrier(context, owner, ownerType, cell, ps6090FunctionID(method.(*types.Func))); got != test.want {
				t.Fatalf("publication edge %v, want %v", got, test.want)
			}
		})
	}
}
