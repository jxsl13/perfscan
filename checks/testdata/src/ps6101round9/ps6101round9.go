package ps6101round9

import (
	"math/rand"
	"testing"
)

var sink float64

func BenchmarkNestedLocalLiteralIdentity(b *testing.B) {
	identity := func(weight float64) float64 { return weight }
	weight := identity(identity(rand.NormFloat64()))
	total := weight
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func BenchmarkNestedLocalRunAliases(b *testing.B) {
	alias := func(run func(string, func(*testing.B)) bool) func(string, func(*testing.B)) bool {
		return run
	}
	run := alias(alias(b.Run))
	weight := rand.NormFloat64()
	run("sub", func(b *testing.B) {
		total := weight
		for b.Loop() {
			if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
				sink = total
			}
		}
	})
}

func BenchmarkTrueLiteralRecursionRemainsConservative(b *testing.B) {
	var recursive func(float64, int) float64
	recursive = func(weight float64, depth int) float64 {
		if depth <= 0 {
			return weight
		}
		return recursive(weight, depth-1)
	}
	weight := rand.NormFloat64()
	total := recursive(weight, b.N)
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkMutualLiteralRecursionRemainsConservative(b *testing.B) {
	var left, right func(float64, int) float64
	left = func(weight float64, depth int) float64 {
		if depth <= 0 {
			return weight
		}
		return right(weight, depth-1)
	}
	right = func(weight float64, depth int) float64 {
		if depth <= 0 {
			return weight
		}
		return left(weight, depth-1)
	}
	weight := rand.NormFloat64()
	total := left(weight, b.N)
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkDirectBoxTypeSwitchBinding(b *testing.B) {
	var boxed any = []float64{rand.NormFloat64()}
	switch weights := boxed.(type) {
	case []float64:
		total := weights[0]
		for b.Loop() {
			if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
				sink = total
			}
		}
	}
}

type weightSliceAlias = []float64

func BenchmarkAliasTypeSwitchBinding(b *testing.B) {
	var boxed any = weightSliceAlias{rand.NormFloat64()}
	switch weights := boxed.(type) {
	case weightSliceAlias:
		total := weights[0]
		for b.Loop() {
			if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
				sink = total
			}
		}
	}
}

func BenchmarkCommaOKBindingControl(b *testing.B) {
	var boxed any = []float64{rand.NormFloat64()}
	weights, ok := boxed.([]float64)
	_ = ok
	total := weights[0]
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func BenchmarkUnreachableTypeSwitchControlIsSilent(b *testing.B) {
	var boxed any = float64(1)
	switch weights := boxed.(type) {
	case []float64:
		total := weights[0]
		for b.Loop() {
			if total > 0 {
				sink = total
			}
		}
	}
}

func BenchmarkTypeSwitchAliasOverwriteIsSilent(b *testing.B) {
	var boxed any = []float64{rand.NormFloat64()}
	switch weights := boxed.(type) {
	case []float64:
		weights[0] = 1
		total := weights[0]
		for b.Loop() {
			if total > 0 {
				sink = total
			}
		}
	}
}

func BenchmarkPointerTypeSwitchBinding(b *testing.B) {
	value := rand.NormFloat64()
	var boxed any = &value
	switch weight := boxed.(type) {
	case *float64:
		total := *weight
		for b.Loop() {
			if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
				sink = total
			}
		}
	}
}

func BenchmarkDoublePointerTypeSwitchBinding(b *testing.B) {
	value := rand.NormFloat64()
	pointer := &value
	var boxed any = &pointer
	switch weight := boxed.(type) {
	case **float64:
		total := **weight
		for b.Loop() {
			if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
				sink = total
			}
		}
	}
}

type weightPointer *float64

type genericWeightPointer[T any] *T

func BenchmarkNamedPointerTypeSwitchBinding(b *testing.B) {
	value := rand.NormFloat64()
	var boxed any = weightPointer(&value)
	switch weight := boxed.(type) {
	case weightPointer:
		total := *weight
		for b.Loop() {
			if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
				sink = total
			}
		}
	}
}

func BenchmarkGenericNamedPointerTypeSwitchBinding(b *testing.B) {
	value := rand.NormFloat64()
	var boxed any = genericWeightPointer[float64](&value)
	switch weight := boxed.(type) {
	case genericWeightPointer[float64]:
		total := *weight
		for b.Loop() {
			if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
				sink = total
			}
		}
	}
}

func BenchmarkPointerAssertionBinding(b *testing.B) {
	value := rand.NormFloat64()
	var boxed any = &value
	weight, ok := boxed.(*float64)
	if !ok {
		b.Fatal("unexpected pointer type")
	}
	total := *weight
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func BenchmarkPointerTypeSwitchDoesNotRenamePointee(b *testing.B) {
	value := rand.NormFloat64()
	var boxed any = &value
	switch weight := boxed.(type) {
	case *float64:
		_ = weight
	}
	total := value
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkPointerTypeSwitchUnrelatedAliasIsSilent(b *testing.B) {
	value := rand.NormFloat64()
	unrelated := 1.0
	var boxed any = &value
	switch weight := boxed.(type) {
	case *float64:
		_ = weight
		pointer := &unrelated
		total := *pointer
		for b.Loop() {
			if total > 0 {
				sink = total
			}
		}
	}
}

func BenchmarkPointerTypeSwitchShadowIsSilent(b *testing.B) {
	value := rand.NormFloat64()
	other := 1.0
	var boxed any = &value
	switch weight := boxed.(type) {
	case *float64:
		_ = weight
		{
			weight := &other
			total := *weight
			for b.Loop() {
				if total > 0 {
					sink = total
				}
			}
		}
	}
}

func BenchmarkPointerTypeSwitchOverwriteThroughAliasIsSilent(b *testing.B) {
	value := rand.NormFloat64()
	var boxed any = &value
	switch weight := boxed.(type) {
	case *float64:
		alias := weight
		*alias = 1
		total := *weight
		for b.Loop() {
			if total > 0 {
				sink = total
			}
		}
	}
}

func BenchmarkDoublePointerTypeSwitchOverwriteIsSilent(b *testing.B) {
	value := rand.NormFloat64()
	pointer := &value
	var boxed any = &pointer
	switch weight := boxed.(type) {
	case **float64:
		**weight = 1
		total := **weight
		for b.Loop() {
			if total > 0 {
				sink = total
			}
		}
	}
}

func BenchmarkPointerTypeSwitchTestingMethodValue(b *testing.B) {
	weight := rand.NormFloat64()
	var boxed any = b
	switch benchmark := boxed.(type) {
	case *testing.B:
		run := benchmark.Run
		run("sub", func(b *testing.B) {
			total := weight
			for b.Loop() {
				if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
					sink = total
				}
			}
		})
	}
}

func BenchmarkDoublePointerTypeSwitchTestingMethodValue(b *testing.B) {
	weight := rand.NormFloat64()
	benchmarkPointer := b
	var boxed any = &benchmarkPointer
	switch benchmark := boxed.(type) {
	case **testing.B:
		run := (*benchmark).Run
		run("sub", func(b *testing.B) {
			total := weight
			for b.Loop() {
				if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
					sink = total
				}
			}
		})
	}
}

func BenchmarkPointerTypeSwitchCallable(b *testing.B) {
	identity := func(weight float64) float64 { return weight }
	var boxed any = &identity
	switch callback := boxed.(type) {
	case *func(float64) float64:
		weight := (*callback)(rand.NormFloat64())
		total := weight
		for b.Loop() {
			if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
				sink = total
			}
		}
	}
}

func BenchmarkDoublePointerTypeSwitchCallable(b *testing.B) {
	identity := func(weight float64) float64 { return weight }
	callbackPointer := &identity
	var boxed any = &callbackPointer
	switch callback := boxed.(type) {
	case **func(float64) float64:
		weight := (**callback)(rand.NormFloat64())
		total := weight
		for b.Loop() {
			if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
				sink = total
			}
		}
	}
}

func BenchmarkDoublePointerCommaOKAssertion(b *testing.B) {
	value := rand.NormFloat64()
	pointer := &value
	var boxed any = &pointer
	weight, ok := boxed.(**float64)
	if !ok {
		b.Fatal("unexpected pointer type")
	}
	total := **weight
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func BenchmarkTriplePointerSingleAssertion(b *testing.B) {
	value := rand.NormFloat64()
	first := &value
	second := &first
	var boxed any = &second
	weight := boxed.(***float64)
	total := ***weight
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

type doubleWeightPointer **float64

func BenchmarkNamedDoublePointerAssertion(b *testing.B) {
	value := rand.NormFloat64()
	pointer := &value
	var boxed any = doubleWeightPointer(&pointer)
	weight := boxed.(doubleWeightPointer)
	total := **weight
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func BenchmarkGenericNamedDoublePointerAssertion(b *testing.B) {
	value := rand.NormFloat64()
	pointer := &value
	var boxed any = genericWeightPointer[*float64](&pointer)
	weight := boxed.(genericWeightPointer[*float64])
	total := **weight
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func consumeDoublePointer(weight **float64) float64 { return **weight }

func BenchmarkDoublePointerAssertionArgument(b *testing.B) {
	value := rand.NormFloat64()
	pointer := &value
	var boxed any = &pointer
	total := consumeDoublePointer(boxed.(**float64))
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func BenchmarkTriplePointerTypeSwitchBinding(b *testing.B) {
	value := rand.NormFloat64()
	first := &value
	second := &first
	var boxed any = &second
	switch weight := boxed.(type) {
	case ***float64:
		total := ***weight
		for b.Loop() {
			if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
				sink = total
			}
		}
	}
}

func BenchmarkDoublePointerAssertionAddressChain(b *testing.B) {
	value := rand.NormFloat64()
	pointer := &value
	var boxed any = &pointer
	weight := boxed.(**float64)
	alias := &*weight
	total := **alias
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func BenchmarkDoublePointerAssertionTestingMethodValue(b *testing.B) {
	weight := rand.NormFloat64()
	benchmarkPointer := b
	var boxed any = &benchmarkPointer
	benchmark := boxed.(**testing.B)
	run := (*benchmark).Run
	run("sub", func(b *testing.B) {
		total := weight
		for b.Loop() {
			if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
				sink = total
			}
		}
	})
}

func BenchmarkDoublePointerAssertionCallable(b *testing.B) {
	identity := func(weight float64) float64 { return weight }
	callbackPointer := &identity
	var boxed any = &callbackPointer
	callback := boxed.(**func(float64) float64)
	weight := (**callback)(rand.NormFloat64())
	total := weight
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func BenchmarkDoublePointerAssertionOverwriteIsSilent(b *testing.B) {
	value := rand.NormFloat64()
	pointer := &value
	var boxed any = &pointer
	weight := boxed.(**float64)
	**weight = 1
	total := **weight
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkDoublePointerAssertionRebindIsSilent(b *testing.B) {
	value := rand.NormFloat64()
	pointer := &value
	other := 1.0
	var boxed any = &pointer
	weight := boxed.(**float64)
	*weight = &other
	total := **weight
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkDoublePointerAssertionShadowIsSilent(b *testing.B) {
	value := rand.NormFloat64()
	pointer := &value
	other := 1.0
	otherPointer := &other
	var boxed any = &pointer
	weight := boxed.(**float64)
	_ = weight
	{
		weight := &otherPointer
		total := **weight
		for b.Loop() {
			if total > 0 {
				sink = total
			}
		}
	}
}

func BenchmarkFailedDoublePointerAssertionIsSilent(b *testing.B) {
	var boxed any = float64(1)
	if weight, ok := boxed.(**float64); ok {
		total := **weight
		for b.Loop() {
			if total > 0 {
				sink = total
			}
		}
	}
}

func BenchmarkDoublePointerAssertionDoesNotRenamePointee(b *testing.B) {
	value := rand.NormFloat64()
	pointer := &value
	var boxed any = &pointer
	if weight, ok := boxed.(**float64); ok {
		_ = weight
	}
	total := value
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkDoublePointerAssertionBranchMergeIsSilent(b *testing.B) {
	value := rand.NormFloat64()
	pointer := &value
	other := 1.0
	otherPointer := &other
	var boxed any = &pointer
	var weight **float64
	if sink > 0 {
		weight = boxed.(**float64)
	} else {
		weight = &otherPointer
	}
	total := **weight
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}

func overwriteDoublePointer(weight **float64) { **weight = 1 }

func BenchmarkDeferredDoublePointerAssertion(b *testing.B) {
	value := rand.NormFloat64()
	pointer := &value
	var boxed any = &pointer
	weight := boxed.(**float64)
	defer overwriteDoublePointer(weight)
	total := **weight
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func BenchmarkEscapedDoublePointerAssertionIsSilent(b *testing.B) {
	value := rand.NormFloat64()
	pointer := &value
	var boxed any = &pointer
	weight := boxed.(**float64)
	ch := make(chan **float64, 1)
	ch <- weight
	total := **weight
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkGoDoublePointerAssertionIsSilent(b *testing.B) {
	value := rand.NormFloat64()
	pointer := &value
	var boxed any = &pointer
	weight := boxed.(**float64)
	go overwriteDoublePointer(weight)
	total := **weight
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkSelectedDoublePointerAssertionIsSilent(b *testing.B) {
	value := rand.NormFloat64()
	pointer := &value
	other := 1.0
	var boxed any = &pointer
	weight := boxed.(**float64)
	replacements := make(chan *float64)
	select {
	case replacement := <-replacements:
		*weight = replacement
	default:
		_ = other
	}
	total := **weight
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}
