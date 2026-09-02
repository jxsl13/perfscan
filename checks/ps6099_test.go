package checks

import (
	"go/token"
	"go/types"
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6099(t *testing.T) {
	t.Parallel()
	analysistest.Run(t, analysistest.TestData(), PS6099.Analyzer,
		"ps6099", "ps6099noleaf", "ps6099importleaf", "ps6099mixed",
		"ps6099precision", "ps6099badsequence", "ps6099aliasleaf",
		"ps6099calleeprecision", "ps6099ignoredalias", "ps6099genericleaf",
		"ps6099calledsignature", "ps6099repeatedlane", "ps6099genericcalled",
		"ps6099ignoredunknown", "ps6099tuplealias", "ps6099lanebasebad",
		"ps6099lanebasealias", "ps6099deadleaf", "ps6099ignoredshadow",
		"ps6099operationreach", "ps6099deadscalar")
	analysistest.Run(t, analysistest.TestData(), PS6099.Analyzer, "ps6099adversarial")
}

func TestPS6099BooleanSwitchReachability(t *testing.T) {
	t.Parallel()
	analysistest.Run(t, analysistest.TestData(), PS6099.Analyzer, "ps6099boolswitch")
}

func TestPS6099InterfaceSwitchReachability(t *testing.T) {
	t.Parallel()
	analysistest.Run(t, analysistest.TestData(), PS6099.Analyzer, "ps6099interfaceswitch")
}

func TestPS6099FlowExactIterationsScaling(t *testing.T) {
	t.Parallel()
	object := types.NewVar(token.NoPos, nil, "dependent", types.Typ[types.Bool])
	state := map[types.Object]bool{object: true}
	calls := 0
	ps6099FlowExactIterations(state, 1_000_000, func(map[types.Object]bool) {
		calls++
	})
	if !state[object] || calls != 1 {
		t.Fatalf("stable million-iteration transfer = (%v, %d calls), want (dependent, 1 call)", state[object], calls)
	}
}

func TestPS6099FlowExactIterationsCap(t *testing.T) {
	t.Parallel()
	variables := make([]types.Object, ps6099FlowStateCap+1)
	for index := range variables {
		variables[index] = types.NewVar(token.NoPos, nil, "dependency", types.Typ[types.Bool])
	}
	state := map[types.Object]bool{variables[0]: true}
	ps6099FlowExactIterations(state, uint64(len(variables)), func(current map[types.Object]bool) {
		active := -1
		for index, variable := range variables {
			if current[variable] {
				active = index
			}
			current[variable] = false
		}
		if active >= 0 {
			current[variables[(active+1)%len(variables)]] = true
		}
	})
	for _, variable := range variables {
		if state[variable] {
			t.Fatal("state-cap fallback retained an unproved dependency")
		}
	}
}

func TestPS6099NestedBillionIterationScaling(t *testing.T) {
	t.Parallel()
	const (
		variablesCount = 7
		depth          = 5
		exact          = uint64(1_000_000_007)
	)
	variables := make([]types.Object, variablesCount)
	state := make(map[types.Object]bool, variablesCount)
	for index := range variables {
		variables[index] = types.NewVar(token.NoPos, nil, "dependency", types.Typ[types.Bool])
		state[variables[index]] = index == 0
	}
	calls := 0
	var transfer func(int, map[types.Object]bool)
	transfer = func(remaining int, current map[types.Object]bool) {
		if remaining > 0 {
			ps6099FlowExactIterations(current, exact, func(nested map[types.Object]bool) {
				transfer(remaining-1, nested)
			})
			return
		}
		calls++
		values := make([]bool, len(variables))
		for index, variable := range variables {
			values[index] = current[variable]
		}
		for index, variable := range variables {
			current[variable] = values[(index+1)%len(values)]
		}
	}
	ps6099FlowExactIterations(state, exact, func(current map[types.Object]bool) {
		transfer(depth-1, current)
	})
	for index, variable := range variables {
		if state[variable] != (index == 1) {
			t.Fatalf("nested billion-iteration state[%d] = %v, want dependency only at index 1", index, state[variable])
		}
	}
	if calls != 16_807 {
		t.Fatalf("nested billion-iteration transfer calls = %d, want bounded 16807", calls)
	}
}
