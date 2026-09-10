package ps6122

import archsimd "ps6122archsimd"

func exponentBits(k archsimd.Int64x2) archsimd.Uint64x2 {
	return k.Add(archsimd.BroadcastInt64x2(1023)).ShiftAllLeft(52).ToBits() // want `constant 64-bit archsimd left shift by 52.*no automatic fix or speedup claim`
}
func hot(k archsimd.Int64x2) {
	for i := 0; i < 4; i++ {
		_ = exponentBits(k)
	}
}
func direct(k archsimd.Int64x2) {
	for i := 0; i < 4; i++ {
		_ = k.ShiftAllRight(7) // want `constant 64-bit archsimd arithmetic right shift by 7`
	}
}
func unsigned(k archsimd.Uint64x2, xs []int) {
	for range xs {
		_ = k.ShiftAllRight(9) // want `constant 64-bit archsimd logical right shift by 9`
	}
}
func variable(k archsimd.Int64x2, n uint64) {
	for i := 0; i < 4; i++ {
		_ = k.ShiftAllLeft(n)
	}
}
func zero(k archsimd.Int64x2) {
	for i := 0; i < 4; i++ {
		_ = k.ShiftAllLeft(0)
	}
}
func width(k archsimd.Int64x2) {
	for i := 0; i < 4; i++ {
		_ = k.ShiftAllLeft(64)
	}
}
func once(k archsimd.Int64x2) {
	for i := 0; i < 1; i++ {
		_ = k.ShiftAllLeft(52)
	}
}

func dynamicStartAndBound(k archsimd.Int64x2, start, bound int) {
	for i := start; i < 4; i++ {
		_ = k.ShiftAllLeft(50)
	}
	for i := 0; i < bound; i++ {
		_ = k.ShiftAllLeft(51)
	}
	for i := 2; i < 2; i++ {
		_ = k.ShiftAllLeft(52)
	}
}

type signed32 = archsimd.Int32x4

const namedDistance uint64 = 31
const twoRunes = "åβ"

func aliases(k signed32) {
	for i := 0; i < 2; i++ {
		_ = k.ShiftAllLeft(namedDistance) // want `constant 32-bit archsimd left shift by 31`
	}
}

func logical32(k archsimd.Uint32x4) {
	for range [2]struct{}{} {
		_ = k.ShiftAllRight(1) // want `constant 32-bit archsimd logical right shift by 1`
	}
}

func logical16(k archsimd.Uint16x8) {
	for range twoRunes {
		_ = k.ShiftAllLeft(15) // want `constant 16-bit archsimd left shift by 15`
	}
}

func integerRange(k archsimd.Int64x2) {
	for range 2 {
		_ = k.ShiftAllLeft(12) // want `constant 64-bit archsimd left shift by 12`
	}
}

func mapRange(k archsimd.Int64x2, values map[int]bool) {
	for range values {
		_ = k.ShiftAllLeft(3) // want `loop path that may repeat`
	}
}

func channelRange(k archsimd.Int64x2, values <-chan int) {
	for range values {
		_ = k.ShiftAllLeft(4) // want `loop path that may repeat`
	}
}

func coldAlsoCallsHelper(k archsimd.Int64x2) { _ = exponentBits(k) }

func dead(k archsimd.Int64x2) {
	return
	for i := 0; i < 4; i++ {
		_ = k.ShiftAllLeft(5)
	}
}

func constantDeadEdges(k archsimd.Int64x2) {
	if false {
		for i := 0; i < 4; i++ {
			_ = k.ShiftAllLeft(18)
		}
	}
	if true {
	} else {
		for i := 0; i < 4; i++ {
			_ = k.ShiftAllLeft(19)
		}
	}
	for false {
		for i := 0; i < 4; i++ {
			_ = k.ShiftAllLeft(20)
		}
	}
	for range 0 {
		for i := 0; i < 4; i++ {
			_ = k.ShiftAllLeft(21)
		}
	}
}

func deadHelper(k archsimd.Int64x2) archsimd.Int64x2 {
	return k
	_ = k.ShiftAllLeft(6)
	return k
}

func callsDeadHelper(k archsimd.Int64x2) {
	for i := 0; i < 4; i++ {
		_ = deadHelper(k)
	}
}

func mutatedIndex(k archsimd.Int64x2) {
	for i := 0; i < 4; i++ {
		i++
		_ = k.ShiftAllLeft(7)
	}
}

func escapedIndex(k archsimd.Int64x2) {
	for i := 0; i < 4; i++ {
		_ = &i
		_ = k.ShiftAllLeft(8)
	}
}

func earlyExit(k archsimd.Int64x2) {
	for i := 0; i < 4; i++ {
		_ = k.ShiftAllLeft(9)
		break
	}
}

func tooShortRanges(k archsimd.Int64x2) {
	for range [1]int{} {
		_ = k.ShiftAllLeft(10)
	}
	for range "x" {
		_ = k.ShiftAllLeft(11)
	}
	for range 0 {
		_ = k.ShiftAllLeft(12)
	}
	for range 1 {
		_ = k.ShiftAllLeft(13)
	}
}

func deferredAndAsync(k archsimd.Int64x2) {
	for i := 0; i < 4; i++ {
		defer func() { _ = k.ShiftAllLeft(14) }()
	}
	for i := 0; i < 4; i++ {
		go func() { _ = k.ShiftAllLeft(15) }()
	}
	for i := 0; i < 4; i++ {
		_ = func() archsimd.Int64x2 { return k.ShiftAllLeft(16) }
	}
}

func iifeEscapesIndex(k archsimd.Int64x2) {
	for i := 0; i < 4; i++ {
		func() { _ = &i }()
		_ = k.ShiftAllLeft(17)
	}
}

func malformed(k8 archsimd.Int8x16, u8 archsimd.Uint8x16, i16 archsimd.Int16x8, odd archsimd.Int8x2) {
	for i := 0; i < 4; i++ {
		_ = k8.ShiftAllLeft(1)
		_ = u8.ShiftAllRight(1)
		_ = i16.ShiftAllLeft(1)
		_ = odd.ShiftAllLeft(1)
	}
}

func oneTripPrefixBlocks(k archsimd.Int64x2) {
	for range [1]int{} {
		select {}
	}
	for i := 0; i < 4; i++ {
		_ = k.ShiftAllLeft(22)
	}
}

func explicitNilChannelBlocks(k archsimd.Int64x2) {
	for range (chan int)(nil) {
	}
	for i := 0; i < 4; i++ {
		_ = k.ShiftAllLeft(23)
	}
}

func zeroPrefixReturns(k archsimd.Int64x2) {
	for range ([]int)(nil) {
		select {}
	}
	for i := 0; i < 4; i++ {
		_ = k.ShiftAllLeft(24) // want `constant 64-bit archsimd left shift by 24`
	}
}

func labelledContinueRepeats(k archsimd.Int64x2) {
outer:
	for i := 0; i < 4; i++ {
		for range [1]int{} {
			_ = k.ShiftAllLeft(25) // want `constant 64-bit archsimd left shift by 25`
			continue outer
		}
	}
}

func labelledBreakRunsOnce(k archsimd.Int64x2) {
outer:
	for i := 0; i < 4; i++ {
		_ = k.ShiftAllLeft(26)
		break outer
	}
}

func deadNestedBreakDoesNotExitOuter(k archsimd.Int64x2) {
	for i := 0; i < 4; i++ {
		for false {
			break
		}
		_ = k.ShiftAllLeft(27) // want `constant 64-bit archsimd left shift by 27`
	}
}

func channelFrom(ch chan int) chan int { return ch }

func nilSpellingIsNotNilProof(k archsimd.Int64x2) {
	for range channelFrom(nil) {
		_ = k.ShiftAllLeft(28) // want `constant 64-bit archsimd left shift by 28`
	}
	nil := make(chan int)
	for range nil {
		_ = k.ShiftAllLeft(29) // want `constant 64-bit archsimd left shift by 29`
	}
}

func unrelatedForCondition(k archsimd.Int64x2, dynamic int) {
	for j := 10; dynamic < 2; j++ {
		for i := 0; i < 4; i++ {
			_ = k.ShiftAllLeft(30) // want `constant 64-bit archsimd left shift by 30`
		}
	}
}

func blockingSelectOperand(k archsimd.Int64x2, output chan int) {
	select {
	case output <- <-(chan int)(nil):
	default:
	}
	for i := 0; i < 4; i++ {
		_ = k.ShiftAllLeft(31)
	}
}

func disabledNilSelectCase(k archsimd.Int64x2) {
	select {
	case <-(chan int)(nil):
		for i := 0; i < 4; i++ {
			_ = k.ShiftAllLeft(32)
		}
	default:
	}
}

func guaranteedTwoTripExit(k archsimd.Int64x2) {
	for range [2]int{} {
		return
	}
	for i := 0; i < 4; i++ {
		_ = k.ShiftAllLeft(33)
	}
}

func iifeExecutionRegions(k archsimd.Int64x2) {
	func() {
		if false {
			_ = k.ShiftAllLeft(34)
		}
	}()
	func() {
		for range [0]int{} {
			for i := 0; i < 4; i++ {
				_ = k.ShiftAllLeft(35)
			}
		}
	}()
}

func iifeDeadHelper(k archsimd.Int64x2) {
	func() {
		if false {
			_ = k.ShiftAllLeft(37)
		}
	}()
}

func hotIIFEDeadHelper(k archsimd.Int64x2) {
	for i := 0; i < 4; i++ {
		iifeDeadHelper(k)
	}
}

func iifeReturningIntoBlockedHelper(k archsimd.Int64x2) {
	func() { _ = k.ShiftAllLeft(38) }()
	panic("does not return to hot caller")
}

func hotBlockedIIFEHelper(k archsimd.Int64x2) {
	for i := 0; i < 4; i++ {
		iifeReturningIntoBlockedHelper(k)
	}
}

func shortCircuitNilReceive(k archsimd.Int64x2) {
	for i := 0; i < 4; i++ {
		_ = false && <-(chan bool)(nil)
		_ = k.ShiftAllLeft(36) // want `constant 64-bit archsimd left shift by 36`
		_ = true || <-(chan bool)(nil)
	}
}
