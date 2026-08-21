package ps5092

import (
	"bytes"
	"maps"
	"slices"
	"strings"
)

func stringsEqual(left, right string) bool {
	return strings.Clone(strings.Clone(left)) == strings.Clone(right) // want "== comparison consumes 3 throwaway standard-library Clone layer[(]s[)] across 2 operand"
}

func stringsOrdered(left, right string) bool {
	return strings.Clone(left) < right // want "< comparison consumes 1 throwaway standard-library Clone layer"
}

func stringsReverseOrdered(left, right string) bool {
	return left >= strings.Clone(strings.Clone(right)) // want ">= comparison consumes 2 throwaway standard-library Clone layer"
}

func bytesNil(data []byte) bool {
	return bytes.Clone(slices.Clone(bytes.Clone(data))) == nil // want "== comparison consumes 3 throwaway standard-library Clone layer"
}

func slicesNotNil(values []int) bool {
	return nil != slices.Clone(values) // want "!= comparison consumes 1 throwaway standard-library Clone layer"
}

func mapsNil(values map[string]int) bool {
	return maps.Clone(maps.Clone(values)) == nil // want "== comparison consumes 2 throwaway standard-library Clone layer"
}

func commentPreserved(left, right string) bool {
	return strings.Clone( /* comparison rationale */ left) == right // want "== comparison consumes 1 throwaway standard-library Clone layer"
}

// Result-producing and retention-sensitive consumers remain untouched.
func standaloneClone(value string) string {
	return strings.Clone(value)
}

func concatenationMayRetain(left, right string) string {
	return strings.Clone(left) + right
}

func substringMayRetain(value string) string {
	return strings.Clone(value)[1:]
}

type namedString string

func typeChangingClone(value namedString, other string) bool {
	return namedString(strings.Clone(string(value))) == namedString(other)
}

type cloner struct{}

func (cloner) Clone(value string) string { return value }

func userClone(c cloner, left, right string) bool {
	return c.Clone(left) == right
}

func functionValue(left, right string) bool {
	clone := strings.Clone
	return clone(left) == right
}
