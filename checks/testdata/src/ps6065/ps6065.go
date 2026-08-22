package ps6065

const smallestNormal32 = 1.17549435e-38

func fastApproxExp32(value float32) float32 {
	if value < -87 {
		return smallestNormal32
	}
	return value
}

func vectorExp64(value float64) float64 {
	value = max(value, -700)
	return value
}

func nativeApproxSigmoid(value float64) float64 {
	if value < -700 {
		value = -700
	}
	return value
}

func fastApproxExpZero(value float32) float32 {
	if value < -87 {
		return 0
	}
	return value
}

func exactExp(value float32) float32 { return value }

func fusedSigmoidBackward(upstream, value float32) float32 {
	exponential := fastApproxExp32(-value)
	sigmoid := exponential / (1 + exponential)
	return upstream * sigmoid // want `fastApproxExp32 has a nonzero deep-negative exp/sigmoid clamp and its residue is multiplied by a potentially unbounded runtime term in fusedSigmoidBackward`
}

func attentionVJP(upstream, value float64) float64 {
	return nativeApproxSigmoid(value) * upstream // want `nativeApproxSigmoid has a nonzero deep-negative exp/sigmoid clamp and its residue is multiplied by a potentially unbounded runtime term in attentionVJP`
}

func activationGradient(upstream, value float64) float64 {
	approximation := vectorExp64(value)
	wrapped := approximation / (1 + approximation)
	return upstream * wrapped // want `vectorExp64 has a nonzero deep-negative exp/sigmoid clamp and its residue is multiplied by a potentially unbounded runtime term in activationGradient`
}

func forwardOnly(upstream, value float32) float32 {
	return upstream * fastApproxExp32(value)
}

func constantGradient(value float32) float32 {
	return 2 * fastApproxExp32(value)
}

func zeroUnderflowBackward(upstream, value float32) float32 {
	return upstream * fastApproxExpZero(value)
}

func exactBackward(upstream, value float32) float32 {
	return upstream * exactExp(value)
}

func integerGradient(upstream int, value float32) int {
	return upstream * int(fastApproxExp32(value))
}

func mutableGradient(upstream, value float32) float32 {
	approximation := fastApproxExp32(value)
	approximation = exactExp(value)
	return upstream * approximation
}

//perfscan:exp-clamp-amplification-validated multiplier bound and extreme parity proved.
func boundedGradient(upstream, value float32) float32 {
	return upstream * fastApproxExp32(value)
}
