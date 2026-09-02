package ps6010

// The generated accumulators must infer their type from untyped zero literals:
// a local declaration may legally shadow the predeclared float64 identifier.
func float64ShadowedFix(a [4]float64, w [16]float64, out, n int) [4]float64 {
	var dst [4]float64
	float64 := "shadow"
	_ = float64
	for o := 0; o < out; o++ {
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * w[i*out+o] // want `this operand does not vary with the output index o but is re-streamed once per output element; unroll the output loop by 4 with independent accumulators to amortize the load \(bit-identical per output\)`
		}
		dst[o] = acc
	}
	return dst
}

// A fresh local allocation is provably disjoint from both input parameters.
func freshDestinationFix(a, w []float64, out, n int) []float64 {
	dst := make([]float64, out)
	for o := 0; o < out; o++ {
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * w[i*out+o] // want `this operand does not vary with the output index o but is re-streamed once per output element; unroll the output loop by 4 with independent accumulators to amortize the load \(bit-identical per output\)`
		}
		dst[o] = acc
	}
	return dst
}

func sliceAliasMutation(a, w []float64, out, n int) [64]float64 {
	var dst [64]float64
	alias := (a[:])
	for o := 0; o < out; o++ {
		alias[0] = float64(o)
		sum := 0.0
		for i := 0; i < n; i++ {
			sum += a[i] * w[o]
		}
		dst[o] = sum
	}
	return dst
}

func slicedOpaqueMutation(a, w []float64, out, n int) [64]float64 {
	var dst [64]float64
	for o := 0; o < out; o++ {
		mutatePS6010Container((a[:]), o)
		sum := 0.0
		for i := 0; i < n; i++ {
			sum += a[i] * w[o]
		}
		dst[o] = sum
	}
	return dst
}

// len, cap, min, and max are read-only and must preserve a safe finding.
func readOnlyBuiltins(a, w []float64, out, n int) [64]float64 {
	var dst [64]float64
	_ = max(len(a), cap(a))
	n = min(n, len(a))
	for o := 0; o < out; o++ {
		sum := 0.0
		for i := 0; i < n; i++ {
			sum += a[i] * w[o] // want `this operand does not vary with the output index o but is re-streamed once per output element; unroll the output loop by 4 with independent accumulators to amortize the load \(bit-identical per output\)`
		}
		dst[o] = sum
	}
	return dst
}

func appendMutatesRoot(a, w []float64, out, n int) [64]float64 {
	var dst [64]float64
	for o := 0; o < out; o++ {
		_ = append(a, float64(o))
		sum := 0.0
		for i := 0; i < n; i++ {
			sum += a[i] * w[o]
		}
		dst[o] = sum
	}
	return dst
}

func appendAliasMutation(a, w []float64, out, n int) [64]float64 {
	var dst [64]float64
	alias := append(a, 0)
	for o := 0; o < out; o++ {
		clear(alias)
		sum := 0.0
		for i := 0; i < n; i++ {
			sum += a[i] * w[o]
		}
		dst[o] = sum
	}
	return dst
}

func clearMutatesRoot(a, w []float64, out, n int) [64]float64 {
	var dst [64]float64
	for o := 0; o < out; o++ {
		clear(a)
		sum := 0.0
		for i := 0; i < n; i++ {
			sum += a[i] * w[o]
		}
		dst[o] = sum
	}
	return dst
}

func deleteMutatesRoot(a map[int]float64, w []float64, out, n int) [64]float64 {
	var dst [64]float64
	for o := 0; o < out; o++ {
		delete(a, o)
		sum := 0.0
		for i := 0; i < n; i++ {
			sum += a[i] * w[o]
		}
		dst[o] = sum
	}
	return dst
}

// copy writes only its destination. A fresh destination cannot overlap a.
func copyReadOnlySource(a, w []float64, out, n int) [64]float64 {
	var dst [64]float64
	scratch := make([]float64, n)
	copy(scratch, a)
	for o := 0; o < out; o++ {
		sum := 0.0
		for i := 0; i < n; i++ {
			sum += a[i] * w[o] // want `this operand does not vary with the output index o but is re-streamed once per output element; unroll the output loop by 4 with independent accumulators to amortize the load \(bit-identical per output\)`
		}
		dst[o] = sum
	}
	return dst
}

// Compatible slice parameters may overlap, so a copy into scratch can mutate a.
func copyOverlappingParameters(scratch, a, w []float64, out, n int) [64]float64 {
	var dst [64]float64
	copy(scratch, a)
	for o := 0; o < out; o++ {
		sum := 0.0
		for i := 0; i < n; i++ {
			sum += a[i] * w[o]
		}
		dst[o] = sum
	}
	return dst
}

func indirectFreshDestinationRebind(a, w []float64, out, n int) []float64 {
	dst := make([]float64, out)
	pointer := &dst
	*pointer = a
	for o := 0; o < out; o++ {
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * w[i*out+o]
		}
		dst[o] = acc
	}
	return dst
}

func transitiveFreshDestinationRebind(a, w []float64, out, n int) []float64 {
	dst := make([]float64, out)
	pointer := &dst
	pointerPointer := &pointer
	**pointerPointer = a
	for o := 0; o < out; o++ {
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * w[i*out+o]
		}
		dst[o] = acc
	}
	return dst
}

func copiedPointerFreshDestinationRebind(a, w []float64, out, n int) []float64 {
	dst := make([]float64, out)
	pointer := &dst
	copiedPointer := pointer
	*copiedPointer = a
	for o := 0; o < out; o++ {
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * w[i*out+o]
		}
		dst[o] = acc
	}
	return dst
}

func rebindPS6010Slice(pointer *[]float64, values []float64) { *pointer = values }

func helperReboundFreshDestination(a, w []float64, out, n int) []float64 {
	dst := make([]float64, out)
	rebindPS6010Slice(&dst, a)
	for o := 0; o < out; o++ {
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * w[i*out+o]
		}
		dst[o] = acc
	}
	return dst
}

func capturePS6010SliceHeader(*[]float64) {}

func capturedFreshDestination(a, w []float64, out, n int) []float64 {
	dst := make([]float64, out)
	capturePS6010SliceHeader(&dst)
	for o := 0; o < out; o++ {
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * w[i*out+o]
		}
		dst[o] = acc
	}
	return dst
}

func closureReboundFreshDestination(a, w []float64, out, n int) []float64 {
	dst := make([]float64, out)
	rebind := func() { dst = a }
	rebind()
	for o := 0; o < out; o++ {
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * w[i*out+o]
		}
		dst[o] = acc
	}
	return dst
}

type ps6010NamedFloatSlice []float64

func convertedSliceAliasMutation(a, w []float64, out, n int) [64]float64 {
	var dst [64]float64
	alias := ps6010NamedFloatSlice(a)
	for o := 0; o < out; o++ {
		alias[0] = float64(o)
		sum := 0.0
		for i := 0; i < n; i++ {
			sum += a[i] * w[o]
		}
		dst[o] = sum
	}
	return dst
}

func convertedSliceAliasClear(a, w []float64, out, n int) [64]float64 {
	var dst [64]float64
	alias := ps6010NamedFloatSlice((a))
	aliasAgain := []float64(alias)
	for o := 0; o < out; o++ {
		clear(aliasAgain)
		sum := 0.0
		for i := 0; i < n; i++ {
			sum += a[i] * w[o]
		}
		dst[o] = sum
	}
	return dst
}

func convertedSliceAliasOpaque(a, w []float64, out, n int) [64]float64 {
	var dst [64]float64
	alias := ps6010NamedFloatSlice(a)
	for o := 0; o < out; o++ {
		mutatePS6010Container([]float64(alias), o)
		sum := 0.0
		for i := 0; i < n; i++ {
			sum += a[i] * w[o]
		}
		dst[o] = sum
	}
	return dst
}

func numericConversionRemainsValueOnly(a, w []float64, out, n int) [64]float64 {
	var dst [64]float64
	converted := int64(out)
	_ = converted
	for o := 0; o < out; o++ {
		sum := 0.0
		for i := 0; i < n; i++ {
			sum += a[i] * w[o] // want `this operand does not vary with the output index o but is re-streamed once per output element; unroll the output loop by 4 with independent accumulators to amortize the load \(bit-identical per output\)`
		}
		dst[o] = sum
	}
	return dst
}

// The fast path needs a bounds proof before delaying a block's stores. When w
// is short, the generated fallback must retain dst[0] before the later panic so
// recover/defer observes the same partial result as this scalar loop.
var ps6010PanicObserved float64

func panicTimingFix(a, w []float64, out, n int) []float64 {
	dst := make([]float64, out)
	defer func() { ps6010PanicObserved = dst[0] }()
	for o := 0; o < out; o++ {
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * w[i*out+o] // want `this operand does not vary with the output index o but is re-streamed once per output element; unroll the output loop by 4 with independent accumulators to amortize the load \(bit-identical per output\)`
		}
		dst[o] = acc
	}
	return dst
}

type ps6010Round11FixMutator interface {
	touchRound11Fix()
}

type ps6010Round11FixValue struct {
	values [2]int
}

func (value ps6010Round11FixValue) touchRound11Fix() {
	value.values[0]++
}

// Following inline conversions must not make value-only aggregates look like
// aliases. The canonical array loop remains eligible for its safe autofix.
func valueOnlyInlineConversionFix(a [4]float64, w [16]float64, out, n int) [4]float64 {
	ps6010Round11FixMutator(ps6010Round11FixValue{}).touchRound11Fix()
	var dst [4]float64
	for o := 0; o < out; o++ {
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * w[i*out+o] // want `this operand does not vary with the output index o but is re-streamed once per output element; unroll the output loop by 4 with independent accumulators to amortize the load \(bit-identical per output\)`
		}
		dst[o] = acc
	}
	return dst
}
