package checks

import (
	"testing"

	"golang.org/x/tools/go/ssa"
)

func TestPS6136ParameterObject(t *testing.T) {
	t.Parallel()
	for _, parameter := range []*ssa.Parameter{nil, {}} {
		if got := ps6136ParameterObject(parameter); got != nil {
			t.Fatalf("synthetic parameter returned source identity: %v", got)
		}
	}
	fixture, err := ps6136CompileOwners(t, "before", true, true)
	if err != nil {
		t.Fatal(err)
	}
	parameter := ps6136FixtureSSA(fixture).Func("newGPTDecoder").Params[0]
	got := ps6136ParameterObject(parameter)
	if got == nil || got != parameter.Object() {
		t.Fatalf("lost exact source parameter identity: %v", got)
	}
}

func TestPS6136NilConsumerParameterRejects(t *testing.T) {
	t.Parallel()
	context := &ps6125SSAContext{flow: &ps6125SSAExtents{function: &ssa.Function{Params: []*ssa.Parameter{nil}}}}
	if got := (&ps6136Selection{}).consumerCalls(context, nil, false, nil); got != nil {
		t.Fatal("nil receiver parameter did not reject")
	}
}
