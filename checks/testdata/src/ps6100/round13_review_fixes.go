package ps6100

func round13Cycle() bool { return round13Cycle() }

func round13ShortCircuitOr(values []float64, skip bool) {
	_ = skip || round13Cycle()
	for step := 0; step < 4; step++ { // want `1 bounded indexed mutation site\(s\)`
		for index := range values {
			if values[index] > 0 {
				_ = index
			}
		}
		for index := range values {
			if values[index] < 1 {
				_ = index
			}
		}
		values[0]++
	}
}

func round13ShortCircuitAnd(values []float64, skip bool) {
	_ = skip && round13Cycle()
	for step := 0; step < 4; step++ { // want `1 bounded indexed mutation site\(s\)`
		for index := range values {
			if values[index] > 0 {
				_ = index
			}
		}
		for index := range values {
			if values[index] < 1 {
				_ = index
			}
		}
		values[0]++
	}
}

func round13ConstantShortCircuit(values []float64) {
	_ = true || round13Cycle()
	_ = false && round13Cycle()
	for step := 0; step < 4; step++ { // want `1 bounded indexed mutation site\(s\)`
		for index := range values {
			if values[index] > 0 {
				_ = index
			}
		}
		for index := range values {
			if values[index] < 1 {
				_ = index
			}
		}
		values[0]++
	}
}

func round13SetBound(pointer *int) { *pointer = 4 }

func round13BoundCompound(other []float64) {
	low := 2
	low += 2
	alpha := other[low:]
	for step := 0; step < 4; step++ { // want `2 bounded indexed mutation site\(s\).*alpha\[0\].*alpha\[1\]`
		for index := range alpha {
			if alpha[index] > 0 {
				_ = index
			}
		}
		for index := range alpha {
			if alpha[index] < 1 {
				_ = index
			}
		}
		other[4]++
		alpha[1]++
	}
}

func round13BoundCompoundSequence(other []float64) {
	low := 4
	low++
	low--
	low += 2
	low -= 2
	alpha := other[low:]
	for step := 0; step < 4; step++ { // want `2 bounded indexed mutation site\(s\).*alpha\[0\].*alpha\[1\]`
		for index := range alpha {
			if alpha[index] > 0 {
				_ = index
			}
		}
		for index := range alpha {
			if alpha[index] < 1 {
				_ = index
			}
		}
		other[4]++
		alpha[1]++
	}
}

func round13BoundAddress(other []float64) {
	low := 2
	pointer := &low
	*pointer = 4
	alpha := other[low:]
	for step := 0; step < 4; step++ { // want `2 bounded indexed mutation site\(s\).*alpha\[0\].*alpha\[1\]`
		for index := range alpha {
			if alpha[index] > 0 {
				_ = index
			}
		}
		for index := range alpha {
			if alpha[index] < 1 {
				_ = index
			}
		}
		other[4]++
		alpha[1]++
	}
}

func round13BoundHelper(other []float64) {
	low := 2
	round13SetBound(&low)
	alpha := other[low:]
	for step := 0; step < 4; step++ { // want `2 bounded indexed mutation site\(s\).*alpha\[0\].*alpha\[1\]`
		for index := range alpha {
			if alpha[index] > 0 {
				_ = index
			}
		}
		for index := range alpha {
			if alpha[index] < 1 {
				_ = index
			}
		}
		other[4]++
		alpha[1]++
	}
}

func round13BoundRange(other []float64) {
	low := 2
	for _, low = range []int{4} {
	}
	alpha := other[low:]
	for step := 0; step < 4; step++ { // want `2 bounded indexed mutation site\(s\).*alpha\[0\].*alpha\[1\]`
		for index := range alpha {
			if alpha[index] > 0 {
				_ = index
			}
		}
		for index := range alpha {
			if alpha[index] < 1 {
				_ = index
			}
		}
		other[4]++
		alpha[1]++
	}
}

func round13UnknownBound(other []float64, low int) {
	alpha := other[low:]
	low = 4
	_ = low
	for step := 0; step < 4; step++ { // want `1 bounded indexed mutation site\(s\).*pre-loop slice bounds prevent exact alias offset proof`
		for index := range alpha {
			if alpha[index] > 0 {
				_ = index
			}
		}
		for index := range alpha {
			if alpha[index] < 1 {
				_ = index
			}
		}
		other[2]++
		alpha[1]++
	}
}

type round13Leaf struct{ values []float64 }
type round13Inner struct{ *round13Leaf }
type round13Outer struct{ *round13Inner }

func (value *round13Leaf) bump(index int) { value.values[index]++ }
func round13Replace(value *round13Outer, replacement []float64) {
	value.round13Inner = &round13Inner{&round13Leaf{replacement}}
}

func round13StoredAlias(value *round13Outer, replacement []float64) {
	bump := value.bump
	alias := value
	alias.round13Inner = &round13Inner{&round13Leaf{replacement}}
	for step := 0; step < 4; step++ { // want `1 bounded indexed mutation site\(s\)`
		for index := range value.values {
			if value.values[index] > 0 {
				_ = index
			}
		}
		for index := range value.values {
			if value.values[index] < 1 {
				_ = index
			}
		}
		bump(0)
		value.values[1]++
	}
}

func round13StoredHelper(value *round13Outer, replacement []float64) {
	bump := value.bump
	round13Replace(value, replacement)
	for step := 0; step < 4; step++ { // want `1 bounded indexed mutation site\(s\)`
		for index := range value.values {
			if value.values[index] > 0 {
				_ = index
			}
		}
		for index := range value.values {
			if value.values[index] < 1 {
				_ = index
			}
		}
		bump(0)
		value.values[1]++
	}
}

func round13StoredRange(value *round13Outer, replacement []float64) {
	bump := value.bump
	for _, value.round13Inner = range []*round13Inner{{&round13Leaf{replacement}}} {
	}
	for step := 0; step < 4; step++ { // want `1 bounded indexed mutation site\(s\)`
		for index := range value.values {
			if value.values[index] > 0 {
				_ = index
			}
		}
		for index := range value.values {
			if value.values[index] < 1 {
				_ = index
			}
		}
		bump(0)
		value.values[1]++
	}
}

func round13StoredAddress(value *round13Outer, replacement []float64) {
	bump := value.bump
	pointer := &value.round13Inner
	*pointer = &round13Inner{&round13Leaf{replacement}}
	for step := 0; step < 4; step++ { // want `1 bounded indexed mutation site\(s\)`
		for index := range value.values {
			if value.values[index] > 0 {
				_ = index
			}
		}
		for index := range value.values {
			if value.values[index] < 1 {
				_ = index
			}
		}
		bump(0)
		value.values[1]++
	}
}

func round13SliceArrayBump(value [1][]float64)     { value[0][0]++ }
func round13PointerArrayBump(value [1]*[4]float64) { value[0][0]++ }

func round13ReferenceArray(value [1][]float64) {
	for step := 0; step < 4; step++ { // want `2 bounded indexed mutation site\(s\)`
		for index := range value[0] {
			if value[0][index] > 0 {
				_ = index
			}
		}
		for index := range value[0] {
			if value[0][index] < 1 {
				_ = index
			}
		}
		round13SliceArrayBump(value)
		value[0][1]++
	}
}

func round13PointerArray(value [1]*[4]float64) {
	for step := 0; step < 4; step++ { // want `2 bounded indexed mutation site\(s\)`
		for index := range value[0] {
			if value[0][index] > 0 {
				_ = index
			}
		}
		for index := range value[0] {
			if value[0][index] < 1 {
				_ = index
			}
		}
		round13PointerArrayBump(value)
		value[0][1]++
	}
}

func round13UnionBump[T ~[4]float64 | ~[]float64](value T) { value[0]++ }

func round13UnionArray(value [4]float64) {
	for step := 0; step < 4; step++ {
		for index := range value {
			if value[index] > 0 {
				_ = index
			}
		}
		for index := range value {
			if value[index] < 1 {
				_ = index
			}
		}
		round13UnionBump(value)
	}
}

type round13Container struct{ values [4]float64 }

func round13StructBump(value round13Container) { value.values[0]++ }

func round13StructArray(value round13Container) {
	for step := 0; step < 4; step++ {
		for index := range value.values {
			if value.values[index] > 0 {
				_ = index
			}
		}
		for index := range value.values {
			if value.values[index] < 1 {
				_ = index
			}
		}
		round13StructBump(value)
	}
}

type round13MethodArray [4]float64

func (value round13MethodArray) bump() { value[0]++ }

func round13ValueMethodArray(value round13MethodArray) {
	for step := 0; step < 4; step++ {
		for index := range value {
			if value[index] > 0 {
				_ = index
			}
		}
		for index := range value {
			if value[index] < 1 {
				_ = index
			}
		}
		value.bump()
	}
}

type round13ReferenceContainer struct{ values []float64 }

func round13StructReferenceBump(value round13ReferenceContainer) { value.values[0]++ }

func round13StructReference(value round13ReferenceContainer) {
	for step := 0; step < 4; step++ { // want `2 bounded indexed mutation site\(s\)`
		for index := range value.values {
			if value.values[index] > 0 {
				_ = index
			}
		}
		for index := range value.values {
			if value.values[index] < 1 {
				_ = index
			}
		}
		round13StructReferenceBump(value)
		value.values[1]++
	}
}
