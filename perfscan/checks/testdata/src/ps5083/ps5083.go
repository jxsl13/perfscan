package ps5083

import (
	"bytes"
	"maps"
	"slices"
	"strings"
)

func byteLengthDeep(data []byte) int {
	return len(bytes.Clone(slices.Clone(bytes.Clone(data)))) // want "len consumes 3 throwaway standard-library Clone layer"
}

func sliceLength(values []int) int {
	return len(slices.Clone(values)) // want "len consumes 1 throwaway standard-library Clone layer"
}

func mapLength(index map[string]int) int {
	return len(maps.Clone(index)) // want "len consumes 1 throwaway standard-library Clone layer"
}

func stringLength(value string) int {
	return len(strings.Clone(value)) // want "len consumes 1 throwaway standard-library Clone layer"
}

func explicitGeneric(values []int) int {
	return len(slices.Clone[[]int](values)) // want "len consumes 1 throwaway standard-library Clone layer"
}

func stringByteFixedPoint(value string) int {
	return len(bytes.Clone([]byte(strings.Clone(strings.Clone(value))))) // want "len consumes 1 throwaway standard-library Clone layer"
}

func stringByteExpression(left, right string) int {
	return len(bytes.Clone([]byte(left + right))) // want "len consumes 1 throwaway standard-library Clone layer"
}

func stringByteConstant() int {
	return len(bytes.Clone([]byte("constant"))) // want "len consumes 1 throwaway standard-library Clone layer"
}

func commentPreserved(data []byte) int {
	return len(bytes.Clone( /* snapshot rationale */ data)) // want "len consumes 1 throwaway standard-library Clone layer"
}

// Clone changes capacity, so cap is not a length-only observation.
func cloneCapacity(values []int) int {
	return cap(slices.Clone(values))
}

func standaloneClone(data []byte) []byte {
	return bytes.Clone(data)
}

type cloner struct{}

func (cloner) Clone(data []byte) []byte { return data }

func userMethod(c cloner, data []byte) int {
	return len(c.Clone(data))
}

func shadowedLen(data []byte) int {
	len := func([]byte) int { return 7 }
	return len(bytes.Clone(data))
}
