package ps5120

import (
	"bytes"
	"strings"
)

const (
	colon = ":"
	two   = 2
	all   = -1
)

func define(value string) string {
	head := strings.SplitN(value, colon, two)[0] // want `strings.SplitN\(\.\.\.\)\[0\] allocates a piece slice only to assign its head; strings.Cut returns the identical head directly with no result-slice allocation`
	return head
}

func assign(value string) string {
	var head string
	head = strings.SplitN(value, ":", 3)[0] // want `strings.SplitN\(\.\.\.\)\[0\] allocates a piece slice only to assign its head; strings.Cut returns the identical head directly with no result-slice allocation`
	return head
}

func negativeCount(value string) string {
	head := strings.SplitN(value, ":", all)[0] // want `strings.SplitN\(\.\.\.\)\[0\] allocates a piece slice only to assign its head; strings.Cut returns the identical head directly with no result-slice allocation`
	return head
}

func parenthesized(value string) string {
	head := (strings.SplitN(value, ":", (2))[0]) // want `strings.SplitN\(\.\.\.\)\[0\] allocates a piece slice only to assign its head; strings.Cut returns the identical head directly with no result-slice allocation`
	return head
}

// --- negatives ---

func returned(value string) string {
	return strings.SplitN(value, ":", 2)[0]
}

func valueSpec(value string) string {
	var head = strings.SplitN(value, ":", 2)[0]
	return head
}

func emptySeparator(value string) string {
	head := strings.SplitN(value, "", 2)[0]
	return head
}

func dynamicSeparator(value, separator string) string {
	head := strings.SplitN(value, separator, 2)[0]
	return head
}

func countOne(value string) string {
	head := strings.SplitN(value, ":", 1)[0]
	return head
}

func countZero(value string) string {
	head := strings.SplitN(value, ":", 0)[0]
	return head
}

func dynamicCount(value string, count int) string {
	head := strings.SplitN(value, ":", count)[0]
	return head
}

func second(value string) string {
	tail := strings.SplitN(value, ":", 2)[1]
	return tail
}

func bytesHead(value []byte) []byte {
	head := bytes.SplitN(value, []byte(":"), 2)[0]
	return head
}

func complexLeft(value string, heads []string) {
	heads[0] = strings.SplitN(value, ":", 2)[0]
}

func functionValue(value string) string {
	split := strings.SplitN
	head := split(value, ":", 2)[0]
	return head
}

type helper string

func (value helper) SplitN(separator string, count int) []string {
	return []string{string(value), separator}
}

func method(value helper) string {
	head := value.SplitN(":", 2)[0]
	return head
}

var _ = []any{
	define, assign, negativeCount, parenthesized, returned, valueSpec,
	emptySeparator, dynamicSeparator, countOne, countZero, dynamicCount,
	second, bytesHead, complexLeft, functionValue, method,
}
