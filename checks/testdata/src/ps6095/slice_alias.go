package ps6095

type sliceRow [1]float64
type sliceRowAlias = sliceRow

// Negative: a full slice of the addressable array element aliases output.
func freshFullSliceAlias(denominator float64) []sliceRowAlias {
	output := make([]sliceRowAlias, 16)
	alias := output[0][:]
	alias[0] = 1
	for index := range output {
		output[index] = sliceRowAlias{alias[0] / denominator}
	}
	return output
}

// Negative: a low-only slice of the element aliases output.
func freshLowSliceAlias(denominator float64) []sliceRowAlias {
	output := make([]sliceRowAlias, 16)
	alias := output[0][0:]
	alias[0] = 1
	for index := range output {
		output[index] = sliceRowAlias{alias[0] / denominator}
	}
	return output
}

// Negative: a high-only slice of the element aliases output.
func freshHighSliceAlias(denominator float64) []sliceRowAlias {
	output := make([]sliceRowAlias, 16)
	alias := output[0][:1]
	alias[0] = 1
	for index := range output {
		output[index] = sliceRowAlias{alias[0] / denominator}
	}
	return output
}

// Negative: an explicit low/high slice of the element aliases output.
func freshLowHighSliceAlias(denominator float64) []sliceRowAlias {
	output := make([]sliceRowAlias, 16)
	alias := output[0][0:1]
	alias[0] = 1
	for index := range output {
		output[index] = sliceRowAlias{alias[0] / denominator}
	}
	return output
}

// Negative: a three-index slice with Max still aliases output.
func freshThreeIndexSliceAlias(denominator float64) []sliceRowAlias {
	output := make([]sliceRowAlias, 16)
	alias := output[0][0:1:1]
	alias[0] = 1
	for index := range output {
		output[index] = sliceRowAlias{alias[0] / denominator}
	}
	return output
}

type sliceRowBox struct {
	row sliceRowAlias
}

// Negative: the fresh root remains discoverable through index, selector,
// parentheses, a type alias, and a three-index slice.
func freshNestedSelectorSliceAlias(denominator float64) []sliceRowBox {
	output := make([]sliceRowBox, 16)
	alias := (output[0].row)[0:1:1]
	alias[0] = 1
	for index := range output {
		output[index] = sliceRowBox{row: sliceRowAlias{alias[0] / denominator}}
	}
	return output
}

type sliceRowMatrix [1]sliceRowAlias

// Negative: nested array indexes and parentheses also expose the fresh root.
func freshNestedIndexSliceAlias(denominator float64) []sliceRowMatrix {
	output := make([]sliceRowMatrix, 16)
	alias := (output[0][0])[:1]
	alias[0] = 1
	for index := range output {
		output[index] = sliceRowMatrix{{alias[0] / denominator}}
	}
	return output
}

// A slice alias of the destination does not affect a value-only quotient.
func freshSliceAliasScalarControl(numerator, denominator float64) []sliceRowAlias {
	output := make([]sliceRowAlias, 16)
	alias := output[0][:]
	alias[0] = 1
	for index := range output {
		output[index] = sliceRowAlias{numerator / denominator} // want `this floating-point quotient is invariant in output index index; compute the original division once and reuse its rounded result \(never replace it with reciprocal multiplication, which is not bit-equivalent\)`
	}
	return output
}

var unrelatedGlobalRow sliceRowAlias

// Unrelated local and global array slices must not suppress a safe scalar-only
// candidate.
func unrelatedArraySliceControl(output []float64, numerator, denominator float64) {
	local := sliceRowAlias{1}
	localAlias := local[:]
	globalAlias := unrelatedGlobalRow[:]
	_ = localAlias[0] + globalAlias[0]
	for index := range output {
		output[index] = numerator / denominator // want `this floating-point quotient is invariant in output index index; compute the original division once and reuse its rounded result \(never replace it with reciprocal multiplication, which is not bit-equivalent\)`
	}
}

// A scalar used to index an unrelated sliced array remains a safe value-only
// quotient dependency; only the aggregate root receives slice-exposure facts.
func slicedArrayIndexScalarControl(output []float64, denominator float64) {
	rows := [1]sliceRowAlias{{1}}
	rowIndex := 0
	alias := rows[rowIndex][:]
	_ = alias[0]
	for index := range output {
		output[index] = float64(rowIndex) / denominator // want `this floating-point quotient is invariant in output index index; compute the original division once and reuse its rounded result \(never replace it with reciprocal multiplication, which is not bit-equivalent\)`
	}
}
