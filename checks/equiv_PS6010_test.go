package checks

import (
	"math"
	"sync"
	"testing"
)

// ps6010ForOwnedRows models the production ownership boundary from issue #911:
// small matrices stay serial, while every parallel callback exclusively writes
// one complete output row.
func ps6010ForOwnedRows(n int, fn func(int)) {
	if n < 12 {
		for row := 0; row < n; row++ {
			fn(row)
		}
		return
	}
	var wg sync.WaitGroup
	for row := 0; row < n; row++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fn(row)
		}()
	}
	wg.Wait()
}

func ps6010RowProductBefore(inner, v []float64, n int) []float64 {
	tmp := make([]float64, n*n)
	ps6010ForOwnedRows(n, func(a int) {
		row := a * n
		for j := 0; j < n; j++ { //perfscan:ignore PS6010 deliberate Before shape under equivalence test
			sum := 0.0
			for b := 0; b < n; b++ {
				sum += inner[row+b] * v[j*n+b]
			}
			tmp[row+j] = sum
		}
	})
	return tmp
}

func ps6010RowProductTiled(inner, v []float64, n int) []float64 {
	tmp := make([]float64, n*n)
	ps6010ForOwnedRows(n, func(a int) {
		row := a * n
		j := 0
		for ; j+3 < n; j += 4 {
			var s0, s1, s2, s3 float64
			for b := 0; b < n; b++ {
				innerAB := inner[row+b]
				s0 += innerAB * v[j*n+b]
				s1 += innerAB * v[(j+1)*n+b]
				s2 += innerAB * v[(j+2)*n+b]
				s3 += innerAB * v[(j+3)*n+b]
			}
			tmp[row+j] = s0
			tmp[row+j+1] = s1
			tmp[row+j+2] = s2
			tmp[row+j+3] = s3
		}
		for ; j < n; j++ {
			sum := 0.0
			for b := 0; b < n; b++ {
				sum += inner[row+b] * v[j*n+b]
			}
			tmp[row+j] = sum
		}
	})
	return tmp
}

func ps6010RowProductReversed(inner, v []float64, n int) []float64 {
	tmp := make([]float64, n*n)
	ps6010ForOwnedRows(n, func(a int) {
		row := a * n
		j := 0
		for ; j+3 < n; j += 4 {
			var s0, s1, s2, s3 float64
			for b := n - 1; b >= 0; b-- {
				innerAB := inner[row+b]
				s0 += innerAB * v[j*n+b]
				s1 += innerAB * v[(j+1)*n+b]
				s2 += innerAB * v[(j+2)*n+b]
				s3 += innerAB * v[(j+3)*n+b]
			}
			tmp[row+j] = s0
			tmp[row+j+1] = s1
			tmp[row+j+2] = s2
			tmp[row+j+3] = s3
		}
		for ; j < n; j++ {
			sum := 0.0
			for b := n - 1; b >= 0; b-- {
				sum += inner[row+b] * v[j*n+b]
			}
			tmp[row+j] = sum
		}
	})
	return tmp
}

func ps6010Inputs(n int) ([]float64, []float64) {
	inner := make([]float64, n*n)
	v := make([]float64, n*n)
	for a := 0; a < n; a++ {
		for b := 0; b < n; b++ {
			inner[a*n+b] = float64(((a+3)*(b+5)*17)%29-14) / float64((b%5)+1)
		}
	}
	for j := 0; j < n; j++ {
		for b := 0; b < n; b++ {
			v[j*n+b] = float64(((j+7)*(b+2)*11)%31-15) / float64((j%7)+1)
		}
	}
	if n == 5 {
		copy(inner[:n], []float64{1e16, -1e16, 1, 1, 1})
		for j := 0; j < n; j++ {
			for b := 0; b < n; b++ {
				v[j*n+b] = 1
			}
		}
	}
	return inner, v
}

func ps6010EqualBits(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if math.Float64bits(a[i]) != math.Float64bits(b[i]) {
			return false
		}
	}
	return true
}

func TestEquivPS6010RowOwnedOutputTile(t *testing.T) {
	t.Parallel()
	for _, n := range []int{5, 6, 7, 12, 48, 64} {
		inner, v := ps6010Inputs(n)
		before := ps6010RowProductBefore(inner, v, n)
		after := ps6010RowProductTiled(inner, v, n)
		if !ps6010EqualBits(before, after) {
			for i := range before {
				if math.Float64bits(before[i]) != math.Float64bits(after[i]) {
					t.Fatalf("n=%d output=%d: ascending four-output tile changed bits: before=%016x after=%016x", n, i, math.Float64bits(before[i]), math.Float64bits(after[i]))
				}
			}
		}
	}

	inner, v := ps6010Inputs(5)
	ascending := ps6010RowProductBefore(inner, v, 5)
	reversed := ps6010RowProductReversed(inner, v, 5)
	if ps6010EqualBits(ascending, reversed) {
		t.Fatal("reversed-b control unexpectedly passed raw-bit oracle")
	}
}

func ps6010AliasedDestinationBefore(dst, a, w []float64, out, n int) {
	for o := 0; o < out; o++ { //perfscan:ignore PS6010 deliberate alias regression baseline
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * w[i*out+o]
		}
		dst[o] = acc
	}
}

func ps6010AliasedDestinationUnsafeTile(dst, a, w []float64, out, n int) {
	for o := 0; o+3 < out; o += 4 {
		var a0, a1, a2, a3 float64
		for i := 0; i < n; i++ {
			ai := a[i]
			a0 += ai * w[i*out+o]
			a1 += ai * w[i*out+o+1]
			a2 += ai * w[i*out+o+2]
			a3 += ai * w[i*out+o+3]
		}
		dst[o], dst[o+1], dst[o+2], dst[o+3] = a0, a1, a2, a3
	}
}

func TestEquivPS6010WithholdsAliasingSliceFix(t *testing.T) {
	t.Parallel()
	weights := []float64{11, 13, 17, 19}
	before := []float64{2, 3, 5, 7}
	ps6010AliasedDestinationBefore(before, before, weights, 4, 1)
	want := []float64{22, 286, 374, 418}
	if !ps6010EqualBits(before, want) {
		t.Fatalf("n=1/out=4 dst==a baseline changed: got %v, want %v", before, want)
	}

	unsafe := []float64{2, 3, 5, 7}
	ps6010AliasedDestinationUnsafeTile(unsafe, unsafe, weights, 4, 1)
	if ps6010EqualBits(before, unsafe) {
		t.Fatalf("delayed-store control unexpectedly preserved aliased result: %v", unsafe)
	}
}

func ps6010ArrayDestinationBefore(dst *[4]float64, a, w []float64, out, n int) {
	for o := 0; o < out; o++ { //perfscan:ignore PS6010 deliberate array/slice alias baseline
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * w[i*out+o]
		}
		dst[o] = acc
	}
}

func ps6010ArrayDestinationUnsafeTile(dst *[4]float64, a, w []float64, out, n int) {
	for o := 0; o+3 < out; o += 4 {
		var a0, a1, a2, a3 float64
		for i := 0; i < n; i++ {
			ai := a[i]
			a0 += ai * w[i*out+o]
			a1 += ai * w[i*out+o+1]
			a2 += ai * w[i*out+o+2]
			a3 += ai * w[i*out+o+3]
		}
		dst[o] = a0
		dst[o+1] = a1
		dst[o+2] = a2
		dst[o+3] = a3
	}
}

func TestEquivPS6010WithholdsArraySliceOverlapFix(t *testing.T) {
	t.Parallel()
	weights := []float64{11, 13, 17, 19}
	before := [4]float64{2, 3, 5, 7}
	ps6010ArrayDestinationBefore(&before, before[:], weights, 4, 1)
	want := [4]float64{22, 286, 374, 418}
	if before != want {
		t.Fatalf("n=1/out=4 array/slice baseline changed: got %v, want %v", before, want)
	}

	unsafe := [4]float64{2, 3, 5, 7}
	ps6010ArrayDestinationUnsafeTile(&unsafe, unsafe[:], weights, 4, 1)
	if unsafe == before {
		t.Fatalf("delayed-store control unexpectedly preserved array/slice result: %v", unsafe)
	}
}

func ps6010PanicTimingBefore(a, w []float64, out, n int, observed *float64) {
	dst := make([]float64, out)
	defer func() { *observed = dst[0] }()
	for o := 0; o < out; o++ {
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * w[i*out+o]
		}
		dst[o] = acc
	}
}

func ps6010PanicTimingGuarded(a, w []float64, out, n int, observed *float64) {
	dst := make([]float64, out)
	defer func() { *observed = dst[0] }()
	output := 0
	if out <= 0 || (out <= len(dst) && (n <= 0 || (n <= len(a) && n <= len(w)/out))) {
		for ; output+3 < out; output += 4 {
			a0, a1, a2, a3 := 0.0, 0.0, 0.0, 0.0
			for i := 0; i < n; i++ {
				ai := a[i]
				a0 += ai * w[i*out+output]
				a1 += ai * w[i*out+output+1]
				a2 += ai * w[i*out+output+2]
				a3 += ai * w[i*out+output+3]
			}
			dst[output], dst[output+1], dst[output+2], dst[output+3] = a0, a1, a2, a3
		}
		for ; output < out; output++ {
			acc := 0.0
			for i := 0; i < n; i++ {
				acc += a[i] * w[i*out+output]
			}
			dst[output] = acc
		}
	} else {
		for ; output < out; output++ {
			acc := 0.0
			for i := 0; i < n; i++ {
				acc += a[i] * w[i*out+output]
			}
			dst[output] = acc
		}
	}
}

func ps6010RecoveredObservation(run func([]float64, []float64, int, int, *float64), a, w []float64, out, n int) (observed float64, panicked bool) {
	defer func() {
		panicked = recover() != nil
	}()
	run(a, w, out, n, &observed)
	return observed, false
}

func TestEquivPS6010PreservesPanicTimingAndPartialStores(t *testing.T) {
	t.Parallel()

	a := []float64{2, 3}
	shortWeights := []float64{11, 13, 17, 19, 23}
	before, beforePanic := ps6010RecoveredObservation(ps6010PanicTimingBefore, a, shortWeights, 4, 2)
	after, afterPanic := ps6010RecoveredObservation(ps6010PanicTimingGuarded, a, shortWeights, 4, 2)
	if !beforePanic || !afterPanic || before != 91 || after != before {
		t.Fatalf("panic/defer observation changed: before=(%v,%t), guarded=(%v,%t)", before, beforePanic, after, afterPanic)
	}

	shortInput := []float64{2}
	weights := []float64{11, 13, 17, 19, 23, 29, 31, 37}
	before, beforePanic = ps6010RecoveredObservation(ps6010PanicTimingBefore, shortInput, weights, 4, 2)
	after, afterPanic = ps6010RecoveredObservation(ps6010PanicTimingGuarded, shortInput, weights, 4, 2)
	if !beforePanic || !afterPanic || after != before {
		t.Fatalf("input-bounds panic observation changed: before=(%v,%t), guarded=(%v,%t)", before, beforePanic, after, afterPanic)
	}

	fullWeights := []float64{11, 13, 17, 19, 23, 29, 31, 37}
	before, beforePanic = ps6010RecoveredObservation(ps6010PanicTimingBefore, a, fullWeights, 4, 2)
	after, afterPanic = ps6010RecoveredObservation(ps6010PanicTimingGuarded, a, fullWeights, 4, 2)
	if beforePanic || afterPanic || after != before {
		t.Fatalf("safe fast path changed final observation: before=(%v,%t), guarded=(%v,%t)", before, beforePanic, after, afterPanic)
	}
}

var psO133 = 42

func ps6010PackageNameBefore(a [4]float64, w [16]float64, out, n int) ([4]float64, int) {
	var dst [4]float64
	for o := 0; o < out; o++ { //perfscan:ignore PS6010 deliberate name-capture baseline
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * w[i*out+o]
		}
		dst[o] = acc
	}
	return dst, psO133
}

func ps6010PackageNameAfter(a [4]float64, w [16]float64, out, n int) ([4]float64, int) {
	var dst [4]float64
	psO133_2 := 0
	if out <= 0 || (out <= len(dst) && (n <= 0 || (n <= len(a) && n <= len(w)/out))) {
		for ; psO133_2 < out && out-psO133_2 >= 4; psO133_2 += 4 {
			a0, a1, a2, a3 := 0.0, 0.0, 0.0, 0.0
			for i := 0; i < n; i++ {
				ai := a[i]
				a0 += ai * w[i*out+psO133_2]
				a1 += ai * w[i*out+psO133_2+1]
				a2 += ai * w[i*out+psO133_2+2]
				a3 += ai * w[i*out+psO133_2+3]
			}
			dst[psO133_2], dst[psO133_2+1], dst[psO133_2+2], dst[psO133_2+3] = a0, a1, a2, a3
		}
		for ; psO133_2 < out; psO133_2++ {
			acc := 0.0
			for i := 0; i < n; i++ {
				acc += a[i] * w[i*out+psO133_2]
			}
			dst[psO133_2] = acc
		}
	} else {
		for ; psO133_2 < out; psO133_2++ {
			acc := 0.0
			for i := 0; i < n; i++ {
				acc += a[i] * w[i*out+psO133_2]
			}
			dst[psO133_2] = acc
		}
	}
	return dst, psO133
}

func TestEquivPS6010AvoidsPackageNameCapture(t *testing.T) {
	t.Parallel()
	a := [4]float64{2, 3, 5, 7}
	w := [16]float64{11, 13, 17, 19, 23, 29, 31, 37, 41, 43, 47, 53, 59, 61, 67, 71}
	before, beforePackage := ps6010PackageNameBefore(a, w, 4, 4)
	after, afterPackage := ps6010PackageNameAfter(a, w, 4, 4)
	if before != after {
		t.Fatalf("package-name-safe fix changed product: before=%v after=%v", before, after)
	}
	if beforePackage != 42 || afterPackage != beforePackage {
		t.Fatalf("package value captured: before=%d after=%d", beforePackage, afterPackage)
	}
}
