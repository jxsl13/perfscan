package ps6010

func multiResultOpaqueAliasRound17(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		alias, _ := ps6010Round17OpaquePair()
		alias[0] = float64(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func secondResultOpaqueAliasRound17(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		_, alias := ps6010Round17OpaqueSecond()
		alias[0] = float64(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func mapCommaOKAliasRound17(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		alias, _ := ps6010Round17GlobalSlices[0]
		alias[0] = float64(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func assertionCommaOKAliasRound17(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		alias, _ := ps6010Round17GlobalAny.([]float64)
		alias[0] = float64(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func receiveCommaOKAliasRound17(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		alias, _ := <-ps6010Round17GlobalChannel
		alias[0] = float64(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func valueSpecCommaOKAliasRound17(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		var alias, _ = ps6010Round17GlobalSlices[0]
		alias[0] = float64(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func assignmentCommaOKAliasRound17(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	var alias []float64
	var ok bool
	for o := 0; o < out; o++ {
		alias, ok = ps6010Round17GlobalAny.([]float64)
		_ = ok
		alias[0] = float64(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func transitiveGlobalMutationRound17(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		ps6010Round17MutateGlobalMiddle(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func recursiveGlobalMutationRound17(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		ps6010Round17RecursiveA(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func storedNamedGlobalMutationRound17(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	mutate := ps6010Round17MutateGlobalMiddle
	mutateAgain := mutate
	for o := 0; o < out; o++ {
		mutateAgain(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func transitiveUnsafeMutationRound17(a, weights []float64, raw uintptr, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		ps6010Round17MutateRaw(raw, o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func genericNamedMutationRound17(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		ps6010Round17MutateGeneric(float64(o))
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func bodylessNamedMutationRound17(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		ps6010Round17AssemblyLike(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func incompatibleTupleControlRound17(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		alias, _ := ps6010Round17OpaqueIntPair()
		alias[0] = o
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o] // want `this operand does not vary with the output index o but is re-streamed once per output element; unroll the output loop by 4 with independent accumulators to amortize the load \(bit-identical per output\)`
		}
		dst[o] = acc
	}
	return dst
}

func incompatibleMapControlRound17(a, weights []float64, values map[int][]int, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		alias, _ := values[0]
		alias[0] = o
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o] // want `this operand does not vary with the output index o but is re-streamed once per output element; unroll the output loop by 4 with independent accumulators to amortize the load \(bit-identical per output\)`
		}
		dst[o] = acc
	}
	return dst
}

func incompatibleAssertionControlRound17(a, weights []float64, value any, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		alias, _ := value.([]int)
		alias[0] = o
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func pureNamedCallControlRound17(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		_ = ps6010Round17PureScalar(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o] // want `this operand does not vary with the output index o but is re-streamed once per output element; unroll the output loop by 4 with independent accumulators to amortize the load \(bit-identical per output\)`
		}
		dst[o] = acc
	}
	return dst
}

func incompatibleNamedEffectControlRound17(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		ps6010Round17MutateIncompatibleGlobal(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o] // want `this operand does not vary with the output index o but is re-streamed once per output element; unroll the output loop by 4 with independent accumulators to amortize the load \(bit-identical per output\)`
		}
		dst[o] = acc
	}
	return dst
}
