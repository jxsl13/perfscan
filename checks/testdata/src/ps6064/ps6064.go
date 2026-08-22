package ps6064

type F64 float64

func softplus64(value float64) float64        { return value }
func stableSoftplusF32(value float32) float32 { return value }
func softplusNamed(value F64) F64             { return value }
func softplusInt(value int) int               { return value }
func otherSoftplus(value float64) float64     { return value }
func load() float64                           { return 1 }

type graph struct{}

func (*graph) softplus(value float32) float32 { return value }

func direct(value float64) (float64, float64) {
	positive := softplus64(value)
	negative := softplus64(-value) // want `paired float64 softplus64 calls evaluate complementary x and -x inputs but recompute log1p`
	return positive, negative
}

func reverse(value float64) (float64, float64) {
	negative := softplus64(-value)
	positive := softplus64(+value) // want `paired float64 softplus64 calls evaluate complementary x and -x inputs`
	return positive, negative
}

func aliases(value float32) (float32, float32) {
	positiveInput := value
	negativeInput := -value
	positive := stableSoftplusF32(positiveInput)
	negative := stableSoftplusF32(negativeInput) // want `paired float32 stableSoftplusF32 calls evaluate complementary x and -x inputs`
	return positive, negative
}

func indexed(input, positive, negative []float32) {
	for index := range input {
		positive[index] = stableSoftplusF32(input[index])
		negative[index] = stableSoftplusF32(-input[index]) // want `paired float32 stableSoftplusF32 calls evaluate complementary x and -x inputs`
	}
}

func named(value F64) (F64, F64) {
	return softplusNamed(value), softplusNamed(-value) // want `paired float64 softplusNamed calls evaluate complementary x and -x inputs`
}

func converted(value float64) (float32, float32) {
	positive := stableSoftplusF32(float32(value))
	negative := stableSoftplusF32(-float32(value)) // want `paired float32 stableSoftplusF32 calls evaluate complementary x and -x inputs`
	return positive, negative
}

func method(receiver *graph, value float32) (float32, float32) {
	positive := receiver.softplus(value)
	negative := receiver.softplus(-value) // want `paired float32 softplus calls evaluate complementary x and -x inputs`
	return positive, negative
}

func differentInputs(left, right float64) {
	_, _ = softplus64(left), softplus64(-right)
}

func sameSigns(value float64) {
	_, _ = softplus64(value), softplus64(value)
}

func integer(value int) {
	_, _ = softplusInt(value), softplusInt(-value)
}

func differentCallees(value float64) {
	_, _ = softplus64(value), otherSoftplus(-value)
}

func differentReceivers(left, right *graph, value float32) {
	_, _ = left.softplus(value), right.softplus(-value)
}

func sideEffecting() {
	_, _ = softplus64(load()), softplus64(-load())
}

func acrossControlFlow(value float64, condition bool) {
	_ = softplus64(value)
	if condition {
		_ = value
	}
	_ = softplus64(-value)
}

func mutuallyExclusive(value float64, condition bool) {
	if condition {
		_ = softplus64(value)
	} else {
		_ = softplus64(-value)
	}
}

func mutableAlias(value float64) {
	alias := value
	_ = softplus64(alias)
	alias = -value
	_ = softplus64(alias)
}

func constants() {
	_, _ = softplus64(1), softplus64(-1)
}

//perfscan:shared-softplus-base-validated retained for an external raw-bit oracle.
func validated(value float64) {
	_, _ = softplus64(value), softplus64(-value)
}
