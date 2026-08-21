package checks

import (
	"math/rand"
	"reflect"
	"slices"
	"testing"
)

type ps5110Named []int

func TestEquiv_PS5110ValuesNilCapacityAndIndependence(t *testing.T) {
	random := rand.New(rand.NewSource(5110))
	for iteration := range 20_000 {
		parts := make([][]int, 4)
		for index := range parts {
			if random.Intn(5) == 0 {
				continue
			}
			parts[index] = make([]int, random.Intn(12))
			for element := range parts[index] {
				parts[index][element] = random.Int()
			}
		}
		before := slices.Concat(slices.Concat(parts[0], parts[1]), slices.Concat(parts[2], parts[3]))
		after := slices.Concat(parts[0], parts[1], parts[2], parts[3])
		if !reflect.DeepEqual(before, after) || (before == nil) != (after == nil) || cap(before) != cap(after) {
			t.Fatalf("iteration %d differs: before=(%v,nil=%t,cap=%d) after=(%v,nil=%t,cap=%d)",
				iteration, before, before == nil, cap(before), after, after == nil, cap(after))
		}
		if len(before) != 0 {
			original := parts[0]
			for _, part := range parts {
				if len(part) != 0 {
					original = part
					break
				}
			}
			old := before[0]
			original[0]++
			if before[0] != old || after[0] != old {
				t.Fatalf("iteration %d result aliases an input", iteration)
			}
		}
	}
}

func TestEquiv_PS5110NamedConcreteType(t *testing.T) {
	a, b, c := ps5110Named{1, 2}, ps5110Named{}, ps5110Named{3, 4}
	var before any = slices.Concat[ps5110Named](slices.Concat[ps5110Named](a, b), c)
	var after any = slices.Concat[ps5110Named](a, b, c)
	if reflect.TypeOf(before) != reflect.TypeOf(after) || !reflect.DeepEqual(before, after) {
		t.Fatalf("named result differs: before=(%T,%v) after=(%T,%v)", before, before, after, after)
	}
}

func TestEquiv_PS5110OverlappingInputs(t *testing.T) {
	backing := []int{1, 2, 3, 4}
	a, b, c := backing[:3], backing[1:], backing[2:3]
	before := slices.Concat(slices.Concat(a, b), c)
	after := slices.Concat(a, b, c)
	if !reflect.DeepEqual(before, after) || cap(before) != cap(after) {
		t.Fatalf("overlapping input slices differ: before=%v/cap%d after=%v/cap%d", before, cap(before), after, cap(after))
	}
}

func TestEquiv_PS5110RightmostEvaluationOrder(t *testing.T) {
	var order []string
	source := func(name string) []int {
		order = append(order, name)
		return []int{len(order)}
	}
	before := slices.Concat(source("first"), slices.Concat(source("second"), source("third")))
	beforeOrder := slices.Clone(order)
	order = nil
	after := slices.Concat(source("first"), source("second"), source("third"))
	wantOrder := []string{"first", "second", "third"}
	if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(beforeOrder, order) || !reflect.DeepEqual(order, wantOrder) {
		t.Fatalf("rightmost flatten differs: before=%v/%v after=%v/%v", before, beforeOrder, after, order)
	}
}

func TestEquiv_PS5110RightmostMutationRemainsBeforeFinalCopy(t *testing.T) {
	beforeSeed := []int{1}
	before := slices.Concat(beforeSeed, slices.Concat(func() []int {
		beforeSeed[0] = 7
		return []int{2}
	}(), []int{3}))
	afterSeed := []int{1}
	after := slices.Concat(afterSeed, func() []int {
		afterSeed[0] = 7
		return []int{2}
	}(), []int{3})
	if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(after, []int{7, 2, 3}) {
		t.Fatalf("rightmost mutation moved across the final copy: before=%v after=%v", before, after)
	}
}

func TestEquiv_PS5110PartialTreeRetainsRequiredSnapshot(t *testing.T) {
	beforeSeed := []int{1}
	before := slices.Concat(slices.Concat(beforeSeed), slices.Concat(func() []int {
		beforeSeed[0] = 7
		return beforeSeed
	}(), []int{2}))
	afterSeed := []int{1}
	after := slices.Concat(slices.Concat(afterSeed), func() []int {
		afterSeed[0] = 7
		return afterSeed
	}(), []int{2})
	if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(after, []int{1, 7, 2}) {
		t.Fatalf("partial flatten changed the retained snapshot: before=%v after=%v", before, after)
	}
}

func TestEquiv_PS5110LaterMutationRequiresSnapshotGuard(t *testing.T) {
	beforeSeed := []int{1}
	before := slices.Concat(slices.Concat(beforeSeed), func() []int {
		beforeSeed[0] = 9
		return []int{2}
	}())
	afterSeed := []int{1}
	after := slices.Concat(afterSeed, func() []int {
		afterSeed[0] = 9
		return []int{2}
	}())
	if !reflect.DeepEqual(before, []int{1, 2}) || !reflect.DeepEqual(after, []int{9, 2}) {
		t.Fatalf("planted snapshot counterexample changed: before=%v after=%v", before, after)
	}
}
