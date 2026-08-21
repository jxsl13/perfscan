package ps6076

import "slices"

func parallelFor(n int, body func(lo, hi int)) { body(0, n) }
func notParallel(n int, body func(lo, hi int)) { body(0, n) }

type bandPool struct{}

func (bandPool) Range(n int, body func(lo, hi int)) { body(0, n) }

func packInto(destination, source []float64, depth, columns int) {}
func packWeights(source []float64, depth, columns int) []float64 { return nil }
func transposeWeights(source []float64) [16]float64              { return [16]float64{} }
func consume(values any)                                         {}
func runRows(start, end int, packed, destination []float64)      {}

func gemm(right, destination []float64, rows, depth, columns int) {
	parallelFor(rows, func(rowStart, rowEnd int) {
		packed := make([]float64, depth*columns) // want `parallelFor callback repeats range-invariant packing loop from captured source 'right' into packed.*Packed footprint is 8\*\(depth \* columns\) bytes.*\(B-1\)`
		for p := 0; p < depth; p++ {
			for column := 0; column < columns; column++ {
				packed[p*columns+column] = right[p*columns+column]
			}
		}
		runRows(rowStart, rowEnd, packed, destination)
	})
}

func copiedTable(table []byte, rows int) {
	parallelFor(rows, func(lo, hi int) {
		packed := make([]byte, 4096) // want `parallelFor callback repeats range-invariant copy from captured source 'table' into packed.*Packed footprint is 4096 bytes.*B\*4096 bytes`
		copy(packed, table)
		consume(packed)
		_, _ = lo, hi
	})
}

func packedByHelper(weights []float64, rows, depth, columns int) {
	parallelFor(rows, func(lo, hi int) {
		packed := make([]float64, depth*columns) // want `parallelFor callback repeats range-invariant packInto from captured source 'weights' into packed.*8\*\(depth \* columns\) bytes`
		packInto(packed, weights, depth, columns)
		consume(packed)
		_, _ = lo, hi
	})
}

func opaquePack(weights []float64, rows, depth, columns int) {
	parallelFor(rows, func(lo, hi int) {
		packed := packWeights(weights, depth, columns) // want `parallelFor callback repeats range-invariant packWeights from captured source 'weights' into packed.*transform executes B times \(B-1 avoidable\).*capacity times element size`
		consume(packed)
		_, _ = lo, hi
	})
}

func clonedWeights(weights []float64, rows int) {
	parallelFor(rows, func(lo, hi int) {
		packed := slices.Clone(weights) // want `parallelFor callback repeats range-invariant Clone from captured source 'weights' into packed.*Packed footprint is 8\*len\(weights\) bytes`
		consume(packed)
		_, _ = lo, hi
	})
}

func fixedTranspose(weights []float64, rows int) {
	parallelFor(rows, func(lo, hi int) {
		packed := transposeWeights(weights) // want `parallelFor callback repeats range-invariant transposeWeights from captured source 'weights' into packed.*Packed footprint is 128 bytes`
		consume(packed)
		_, _ = lo, hi
	})
}

func methodFanout(pool bandPool, table []byte, rows int) {
	pool.Range(rows, func(lo, hi int) {
		packed := make([]byte, len(table)) // want `bandPool.Range callback repeats range-invariant copy from captured source 'table' into packed.*Packed footprint is 1\*\(len\(table\)\) bytes`
		copy(packed, table)
		consume(packed)
		_, _ = lo, hi
	})
}

func capacityFootprint(table []float32, rows int) {
	parallelFor(rows, func(lo, hi int) {
		packed := make([]float32, 0, 1024) // want `parallelFor callback repeats range-invariant copy from captured source 'table' into packed.*Packed footprint is 4096 bytes`
		copy(packed[:cap(packed)], table)
		consume(packed)
		_, _ = lo, hi
	})
}

// --- negatives ---

func rangeSized(source []float64, rows int) {
	parallelFor(rows, func(lo, hi int) {
		packed := make([]float64, hi-lo)
		copy(packed, source[lo:hi])
		consume(packed)
	})
}

func rangeLengthInvariantCapacity(source []float64, rows int) {
	parallelFor(rows, func(lo, hi int) {
		packed := make([]float64, hi-lo, rows)
		copy(packed, source)
		consume(packed)
	})
}

func derivedRange(source []float64, rows int) {
	parallelFor(rows, func(lo, hi int) {
		count := hi - lo
		packed := make([]float64, rows)
		for index := 0; index < count; index++ {
			packed[index] = source[lo+index]
		}
		consume(packed)
	})
}

func perBandCopy(source []float64, rows int) {
	parallelFor(rows, func(lo, hi int) {
		packed := make([]float64, rows)
		copy(packed, source[lo:hi])
		consume(packed)
	})
}

func firstBandOnly(source []float64, rows int) {
	parallelFor(rows, func(lo, hi int) {
		if lo == 0 {
			packed := make([]float64, rows)
			copy(packed, source)
			consume(packed)
		}
		_ = hi
	})
}

func derivedFirstBand(source []float64, rows int) {
	parallelFor(rows, func(lo, hi int) {
		first := lo == 0
		if first {
			packed := packWeights(source, 1, len(source))
			consume(packed)
		}
		_ = hi
	})
}

func scratchOnly(rows int) {
	parallelFor(rows, func(lo, hi int) {
		scratch := make([]float64, rows)
		consume(scratch)
		_, _ = lo, hi
	})
}

func callbackLocal(rows int) {
	parallelFor(rows, func(lo, hi int) {
		local := []float64{1, 2, 3}
		packed := packWeights(local, 1, len(local))
		consume(packed)
		_, _ = lo, hi
	})
}

func nonFanout(source []float64, rows int) {
	notParallel(rows, func(lo, hi int) {
		packed := make([]float64, rows)
		copy(packed, source)
		consume(packed)
		_, _ = lo, hi
	})
}

func indirectTransform(source []float64, rows int) {
	transform := packWeights
	parallelFor(rows, func(lo, hi int) {
		packed := transform(source, 1, len(source))
		consume(packed)
		_, _ = lo, hi
	})
}

func namedBody(source []float64, rows int) {
	body := func(lo, hi int) {
		packed := make([]float64, rows)
		copy(packed, source)
		consume(packed)
		_, _ = lo, hi
	}
	parallelFor(rows, body)
}

//perfscan:parallel-packing-validated scratch is intentionally private per worker.
func validated(source []float64, rows int) {
	parallelFor(rows, func(lo, hi int) {
		packed := make([]float64, rows)
		copy(packed, source)
		consume(packed)
		_, _ = lo, hi
	})
}

var _ = []any{
	gemm, copiedTable, packedByHelper, opaquePack, clonedWeights,
	fixedTranspose, methodFanout, capacityFootprint, rangeSized, derivedRange,
	rangeLengthInvariantCapacity, perBandCopy, firstBandOnly, derivedFirstBand,
	scratchOnly, callbackLocal, nonFanout, indirectTransform, namedBody, validated,
}
