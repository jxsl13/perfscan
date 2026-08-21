package ps5119

import (
	"bytes"
	"strings"
)

const slash = "/"

func assignPrefix(value, prefix string) string {
	if strings.HasPrefix(value, prefix) { // want `strings.HasPrefix proves the boundary and strings.TrimPrefix immediately repeats that proof; strings.CutPrefix returns the identical remainder and predicate in one direct call`
		value = strings.TrimPrefix(value, prefix)
	}
	return value
}

func declarePrefix(value, prefix string) string {
	if strings.HasPrefix(value, prefix) { // want `strings.HasPrefix proves the boundary and strings.TrimPrefix immediately repeats that proof; strings.CutPrefix returns the identical remainder and predicate in one direct call`
		rest := strings.TrimPrefix(value, prefix)
		return rest
	}
	return value
}

func returnSuffix(value, suffix string) string {
	if strings.HasSuffix(value, suffix) { // want `strings.HasSuffix proves the boundary and strings.TrimSuffix immediately repeats that proof; strings.CutSuffix returns the identical remainder and predicate in one direct call`
		return strings.TrimSuffix(value, suffix)
	}
	return value
}

func constantSpellings(value string) string {
	if strings.HasPrefix(value, slash) { // want `strings.HasPrefix proves the boundary and strings.TrimPrefix immediately repeats that proof; strings.CutPrefix returns the identical remainder and predicate in one direct call`
		return strings.TrimPrefix(value, "/")
	}
	return value
}

func bytePrefix(value, prefix []byte) []byte {
	if bytes.HasPrefix(value, prefix) { // want `bytes.HasPrefix proves the boundary and bytes.TrimPrefix immediately repeats that proof; bytes.CutPrefix returns the identical remainder and predicate in one direct call`
		value = bytes.TrimPrefix(value, prefix)
	}
	return value
}

func byteSuffix(value, suffix []byte) []byte {
	if bytes.HasSuffix(value, suffix) { // want `bytes.HasSuffix proves the boundary and bytes.TrimSuffix immediately repeats that proof; bytes.CutSuffix returns the identical remainder and predicate in one direct call`
		return bytes.TrimSuffix(value, suffix)
	} else {
		return value
	}
}

func parenthesized(value, prefix string) string {
	if strings.HasPrefix(value, prefix) { // want `strings.HasPrefix proves the boundary and strings.TrimPrefix immediately repeats that proof; strings.CutPrefix returns the identical remainder and predicate in one direct call`
		return (strings.TrimPrefix(value, prefix))
	}
	return value
}

func collision(value, prefix, after, found string) string {
	if strings.HasPrefix(value, prefix) { // want `strings.HasPrefix proves the boundary and strings.TrimPrefix immediately repeats that proof; strings.CutPrefix returns the identical remainder and predicate in one direct call`
		value = strings.TrimPrefix(value, prefix)
		return value + after + found
	}
	return value
}

// --- negatives ---

func empty(value string) string {
	if strings.HasPrefix(value, "") {
		return strings.TrimPrefix(value, "")
	}
	return value
}

func differentPrefix(value, left, right string) string {
	if strings.HasPrefix(value, left) {
		return strings.TrimPrefix(value, right)
	}
	return value
}

func differentValue(value, other, prefix string) string {
	if strings.HasPrefix(value, prefix) {
		return strings.TrimPrefix(other, prefix)
	}
	return value
}

func mismatchedDirection(value, boundary string) string {
	if strings.HasPrefix(value, boundary) {
		return strings.TrimSuffix(value, boundary)
	}
	return value
}

func delayed(value, prefix string) string {
	if strings.HasPrefix(value, prefix) {
		value += ""
		return strings.TrimPrefix(value, prefix)
	}
	return value
}

func existingInit(value, prefix string) string {
	if ok := true; ok && strings.HasPrefix(value, prefix) {
		return strings.TrimPrefix(value, prefix)
	}
	return value
}

func compound(value, prefix string, enabled bool) string {
	if enabled && strings.HasPrefix(value, prefix) {
		return strings.TrimPrefix(value, prefix)
	}
	return value
}

func dynamic(value string, prefix func() string) string {
	if strings.HasPrefix(value, prefix()) {
		return strings.TrimPrefix(value, prefix())
	}
	return value
}

func selected(value string, holder struct{ prefix string }) string {
	if strings.HasPrefix(value, holder.prefix) {
		return strings.TrimPrefix(value, holder.prefix)
	}
	return value
}

func constructedBytes(value []byte) []byte {
	if bytes.HasPrefix(value, []byte("/")) {
		return bytes.TrimPrefix(value, []byte("/"))
	}
	return value
}

func functionValues(value, prefix string) string {
	has, trim := strings.HasPrefix, strings.TrimPrefix
	if has(value, prefix) {
		return trim(value, prefix)
	}
	return value
}

type helper string

func (value helper) HasPrefix(prefix string) bool { return len(value) >= len(prefix) }
func (value helper) TrimPrefix(prefix string) helper {
	return value
}

func methods(value helper, prefix string) helper {
	if value.HasPrefix(prefix) {
		return value.TrimPrefix(prefix)
	}
	return value
}

var _ = []any{
	assignPrefix, declarePrefix, returnSuffix, constantSpellings, bytePrefix,
	byteSuffix, parenthesized, collision, empty, differentPrefix, differentValue,
	mismatchedDirection, delayed, existingInit, compound, dynamic, selected,
	constructedBytes, functionValues, methods,
}
