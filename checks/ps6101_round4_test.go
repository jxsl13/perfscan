package checks

import "testing"

func TestPS6101CloneValuePreservesChildren(t *testing.T) {
	t.Parallel()
	value := ps6101Value{
		elements: map[int64]ps6101Value{0: {kind: ps6101Symmetric}},
		analysis: &ps6101ValueAnalysis{
			fields: map[string]ps6101Value{"Values": {kind: ps6101Positive}},
		},
	}
	clone := ps6101CloneValue(value)
	if clone.elements[0].kind != ps6101Symmetric {
		t.Fatal("element metadata was dropped while cloning")
	}
	if clone.fieldValues()["Values"].kind != ps6101Positive {
		t.Fatal("aggregate descendant metadata was dropped while cloning")
	}
	clone.elements[0] = ps6101Value{}
	clone.fieldValues()["Values"] = ps6101Value{}
	if value.elements[0].kind != ps6101Symmetric || value.fieldValues()["Values"].kind != ps6101Positive {
		t.Fatal("cloned child maps alias the original value")
	}
}
