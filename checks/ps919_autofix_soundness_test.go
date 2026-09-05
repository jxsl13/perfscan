package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS1006ZeroColumnInterchangeStaysAdvisory(t *testing.T) {
	t.Parallel()
	// The identical golden file proves the finding remains reportable while
	// carrying no rewrite that would execute the otherwise unreachable row loop.
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS1006.Analyzer, "ps1006_zero_column")
}

func TestPS1006SliceInterchangeCounterexamples(t *testing.T) {
	t.Parallel()
	serial := func(a, out []float64, rows, cols int) {
		for c := 0; c < cols; c++ {
			sum := 0.0
			for r := 0; r < rows; r++ {
				sum += a[r*cols+c]
			}
			out[c] = sum
		}
	}
	unsafeInterchange := func(a, out []float64, rows, cols int) {
		sums := make([]float64, cols)
		for r := 0; r < rows; r++ {
			base := r * cols
			for c := 0; c < cols; c++ {
				sums[c] += a[base+c]
			}
		}
		for c := 0; c < cols; c++ {
			out[c] = sums[c]
		}
	}
	panics := func(f func()) (panicked bool) {
		defer func() { panicked = recover() != nil }()
		f()
		return false
	}

	t.Run("negative columns", func(t *testing.T) {
		t.Parallel()
		if panics(func() { serial(nil, nil, 0, -1) }) {
			t.Fatal("serial loop must execute zero iterations for a negative bound")
		}
		if !panics(func() { unsafeInterchange(nil, nil, 0, -1) }) {
			t.Fatal("old interchange unexpectedly preserved negative-bound behavior")
		}
	})

	t.Run("source panic store timing", func(t *testing.T) {
		t.Parallel()
		serialOut := []float64{99, 88}
		interchangedOut := []float64{99, 88}
		if !panics(func() { serial([]float64{1, 2, 3}, serialOut, 2, 2) }) ||
			!panics(func() { unsafeInterchange([]float64{1, 2, 3}, interchangedOut, 2, 2) }) {
			t.Fatal("both forms must retain the source bounds panic")
		}
		if serialOut[0] != 4 || serialOut[1] != 88 {
			t.Fatalf("serial intermediate store changed: %v", serialOut)
		}
		if interchangedOut[0] != 99 || interchangedOut[1] != 88 {
			t.Fatalf("old interchange counterexample changed: %v", interchangedOut)
		}
	})

	t.Run("shared backing", func(t *testing.T) {
		t.Parallel()
		serialBacking := []float64{1, 2, 3, 4}
		interchangedBacking := []float64{1, 2, 3, 4}
		serial(serialBacking, serialBacking[1:3], 2, 2)
		unsafeInterchange(interchangedBacking, interchangedBacking[1:3], 2, 2)
		if serialBacking[1] != 4 || serialBacking[2] != 8 {
			t.Fatalf("serial alias semantics changed: %v", serialBacking)
		}
		if interchangedBacking[1] != 4 || interchangedBacking[2] != 6 {
			t.Fatalf("old interchange counterexample changed: %v", interchangedBacking)
		}
	})
}

func TestPS4008SliceAxpyCounterexamples(t *testing.T) {
	t.Parallel()
	serial := func(a, b, c [][]float64) {
		for i := range a {
			for j := range b[0] {
				sum := 0.0
				for k := range b {
					sum += a[i][k] * b[k][j]
				}
				c[i][j] = sum
			}
		}
	}
	unsafeAxpy := func(a, b, c [][]float64) {
		for i := range a {
			for j := range b[0] {
				c[i][j] = 0
			}
			for k := range b {
				for j := range b[0] {
					c[i][j] += a[i][k] * b[k][j]
				}
			}
		}
	}
	panics := func(f func()) (panicked bool) {
		defer func() { panicked = recover() != nil }()
		f()
		return false
	}

	t.Run("source panic store timing", func(t *testing.T) {
		t.Parallel()
		serialOutput := [][]float64{{77, 88}}
		axpyOutput := [][]float64{{77, 88}}
		a := [][]float64{{1, 2}}
		b := [][]float64{{3, 4}, {2}}
		if !panics(func() { serial(a, b, serialOutput) }) || !panics(func() { unsafeAxpy(a, b, axpyOutput) }) {
			t.Fatal("both forms must retain the ragged-source panic")
		}
		if serialOutput[0][0] != 7 || serialOutput[0][1] != 88 {
			t.Fatalf("serial intermediate stores changed: %v", serialOutput)
		}
		if axpyOutput[0][0] != 7 || axpyOutput[0][1] != 4 {
			t.Fatalf("old axpy counterexample changed: %v", axpyOutput)
		}
	})

	t.Run("output panic store timing", func(t *testing.T) {
		t.Parallel()
		serialOutput := [][]float64{{99}}
		axpyOutput := [][]float64{{99}}
		a := [][]float64{{1, 2}}
		b := [][]float64{{3, 4}, {2, 5}}
		if !panics(func() { serial(a, b, serialOutput) }) || !panics(func() { unsafeAxpy(a, b, axpyOutput) }) {
			t.Fatal("both forms must retain the short-output panic")
		}
		if serialOutput[0][0] != 7 {
			t.Fatalf("serial completed store changed: %v", serialOutput)
		}
		if axpyOutput[0][0] != 0 {
			t.Fatalf("old axpy counterexample changed: %v", axpyOutput)
		}
	})

	t.Run("shared rows", func(t *testing.T) {
		t.Parallel()
		serialInput := [][]float64{{1, 2}}
		axpyInput := [][]float64{{1, 2}}
		b := [][]float64{{1, 1}, {1, 1}}
		serial(serialInput, b, serialInput)
		unsafeAxpy(axpyInput, b, axpyInput)
		if serialInput[0][0] != 3 || serialInput[0][1] != 5 {
			t.Fatalf("serial shared-row semantics changed: %v", serialInput)
		}
		if axpyInput[0][0] != 0 || axpyInput[0][1] != 0 {
			t.Fatalf("old axpy counterexample changed: %v", axpyInput)
		}
	})
}

func TestPS4008FixedArrayOuterIndexCounterexample(t *testing.T) {
	t.Parallel()
	type matrixA [1][2]float64
	type matrixB [2][2]float64
	type matrixC [2][2]float64

	serial := func(index int, a matrixA, b matrixB) (output matrixC) {
		output = matrixC{{11, 12}, {21, 22}}
		defer func() { _ = recover() }()
		for outer := 0; outer < 1; outer++ {
			for column := range b[0] {
				sum := 0.0
				for inner := range b {
					sum += a[index][inner] * b[inner][column]
				}
				output[index][column] = sum
			}
		}
		return output
	}
	unsafeAxpy := func(index int, a matrixA, b matrixB) (output matrixC) {
		output = matrixC{{11, 12}, {21, 22}}
		defer func() { _ = recover() }()
		for outer := 0; outer < 1; outer++ {
			for column := range b[0] {
				output[index][column] = 0
			}
			for inner := range b {
				for column := range b[0] {
					output[index][column] += a[index][inner] * b[inner][column]
				}
			}
		}
		return output
	}

	if got, want := serial(1, matrixA{}, matrixB{}), (matrixC{{11, 12}, {21, 22}}); got != want {
		t.Fatalf("serial panic/store state = %v, want %v", got, want)
	}
	if got, want := unsafeAxpy(1, matrixA{}, matrixB{}), (matrixC{{11, 12}, {0, 0}}); got != want {
		t.Fatalf("old axpy panic/store state = %v, want %v", got, want)
	}
}
