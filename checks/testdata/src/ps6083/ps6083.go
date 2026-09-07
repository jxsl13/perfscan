package ps6083

type code8 uint8
type sample32 float32

func (value *code8) bump() { *value++ }

func direct32(output, input []float32, packed uint32) {
	for index := range output {
		output[index] = input[index] * float32(2*((packed>>(3*index))&7)+1) // want `maps exactly 8 masked unsigned values`
	}
}

func direct64(output, input []float64, packed uint16) {
	for index := range output {
		output[index] = input[index] * float64(3*((packed>>(2*index))&3)+2) // want `maps exactly 4 masked unsigned values`
	}
}

func namedUnsignedAlias(output []float32, input []code8) {
	for index := range output {
		field := input[index] & 7
		output[index] = float32(2*field + 1) // want `maps exactly 8 masked unsigned values`
	}
}

func reversedConstants(output []float32, input []uint8) {
	for index := range output {
		output[index] = float32(1 + 2*(7&input[index])) // want `maps exactly 8 masked unsigned values`
	}
}

func generatedNameCollision(output []float32, input []uint8) {
	psFloatLookup37_19 := 0
	_ = psFloatLookup37_19
	for index := range output {
		output[index] = float32(2*(input[index]&7) + 1) // want `maps exactly 8 masked unsigned values`
	}
}

func nonContiguousMask(output []float32, input []uint8) {
	for index := range output {
		output[index] = float32(2*(input[index]&5) + 1)
	}
}

func dynamicMask(output []float32, input []uint8, mask uint8) {
	for index := range output {
		output[index] = float32(2*(input[index]&mask) + 1)
	}
}

func domainTooLarge(output []float32, input []uint8) {
	for index := range output {
		output[index] = float32(2*(input[index]&31) + 1)
	}
}

func signedDomain(output []float32, input []int8) {
	for index := range output {
		output[index] = float32(2*(input[index]&7) + 1)
	}
}

func architectureUint(output []float32, input []uint) {
	for index := range output {
		output[index] = float32(2*(input[index]&7) + 1)
	}
}

func architectureUintptr(output []float64, input []uintptr) {
	for index := range output {
		output[index] = float64(2*(input[index]&7) + 1)
	}
}

func sourceCast(output []float32, input []uint16) {
	for index := range output {
		output[index] = float32(2*(uint8(input[index])&7) + 1)
	}
}

func constantCast(output []float32, input []uint8) {
	for index := range output {
		output[index] = float32(uint8(2)*(input[index]&7) + 1)
	}
}

func namedFloatCast(output []sample32, input []uint8) {
	for index := range output {
		output[index] = sample32(2*(input[index]&7) + 1)
	}
}

func integerOverflow(output []float32, input []uint8) {
	for index := range output {
		output[index] = float32(100*(input[index]&7) + 100)
	}
}

func nonExactFloat32(output []float32, input []uint32) {
	for index := range output {
		output[index] = float32(8388609 * (input[index] & 3))
	}
}

func nonExactFloat64(output []float64, input []uint64) {
	for index := range output {
		output[index] = float64(9007199254740992*(input[index]&1) + 1)
	}
}

func sourceCall(index int) uint8 { return uint8(index) }

func effectfulSource(output []float32) {
	for index := range output {
		output[index] = float32(2*(sourceCall(index)&7) + 1)
	}
}

func invariantSource(output []float32, source uint8) {
	for index := range output {
		output[index] = float32(2*(source&7) + 1)
	}
}

func mutatedAlias(output []float32, input []uint8) {
	for index := range output {
		field := input[index] & 7
		field++
		output[index] = float32(2*field + 1)
	}
}

func capturedAlias(output []float32, input []uint8) {
	for index := range output {
		field := input[index] & 7
		func() { field = 9 }()
		output[index] = float32(2*field + 1)
	}
}

func pointerReceiverAlias(output []float32, input []code8) {
	for index := range output {
		field := input[index] & 7
		field.bump()
		output[index] = float32(2*field + 1)
	}
}

func chainedAlias(output []float32, input []uint8) {
	for index := range output {
		field := input[index] & 7
		other := field
		output[index] = float32(2*other + 1)
	}
}

func subtraction(output []float32, input []uint8) {
	for index := range output {
		output[index] = float32(20 - 2*(input[index]&7))
	}
}

func conditional(output []float32, input []uint8) {
	for index := range output {
		if input[index] != 0 {
			output[index] = float32(2*(input[index]&7) + 1)
		}
	}
}

func shortLoop(output *[3]float32, input *[3]uint8) {
	for index := range output {
		output[index] = float32(2*(input[index]&7) + 1)
	}
}

func nestedLoop(output []float32, input [3]uint8) {
	for outer := range output {
		for inner := range input {
			output[outer] += float32(2*(input[inner]&7) + 1)
		}
	}
}

func commented(output []float32, input []uint8) {
	for index := range output {
		output[index] = float32(2*(input[index]&7) /* retain */ + 1)
	}
}

func genericDomain[T ~uint8](output []float32, input []T) {
	for index := range output {
		output[index] = float32(2*(input[index]&7) + 1)
	}
}

func countedLoop(output, input []float32, packed uint64, count int) {
	for index := 0; index < count; index++ {
		output[index] = input[index] * float32(2*((packed>>(3*index))&7)+1) // want `maps exactly 8 masked unsigned values`
	}
}

func addressedAlias(output []float32, input []uint8) {
	for index := range output {
		field := input[index] & 7
		address := &field
		_ = address
		output[index] = float32(2*field + 1)
	}
}

func readOnlyCapturedAlias(output []float32, input []uint8) {
	for index := range output {
		field := input[index] & 7
		read := func() uint8 { return field }
		_ = read
		output[index] = float32(2*field + 1)
	}
}

func labeledLoop(output []float32, input []uint8) {
loop:
	for index := range output {
		if index < 0 {
			break loop
		}
		output[index] = float32(2*(input[index]&7) + 1)
	}
}

func gotoLoop(output []float32, input []uint8) {
	goto loop
loop:
	for index := range output {
		output[index] = float32(2*(input[index]&7) + 1)
	}
}
