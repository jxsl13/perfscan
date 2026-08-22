package ps5121

import (
	"bytes"
	"strings"
)

const (
	colon = ":"
	two   = 2
	all   = -1
)

func declareTail(value string) string {
	if strings.Contains(value, colon) { // want `strings.Contains proves.*index 1`
		tail := strings.SplitN(value, ":", two)[1]
		return tail
	}
	return value
}

func assignHead(value string) string {
	var head string
	if strings.Contains(value, ":") { // want `strings.Contains proves.*index 0`
		head = strings.SplitN(value, colon, 3)[0]
	}
	return head
}

func returnTail(value string) string {
	if strings.Contains(value, ":") { // want `strings.Contains proves.*index 1`
		return strings.SplitN(value, ":", 2)[1]
	} else {
		return value
	}
}

func returnHead(value string) string {
	if strings.Contains(value, colon) { // want `strings.Contains proves.*index 0`
		return strings.SplitN(value, ":", 4)[0]
	}
	return value
}

func parenthesized(value string) string {
	if strings.Contains(value, ":") { // want `strings.Contains proves.*index 1`
		return (strings.SplitN(value, ":", (2))[1])
	}
	return value
}

func collision(value, before, after, found string) string {
	if strings.Contains(value, ":") { // want `strings.Contains proves.*index 1`
		value = strings.SplitN(value, ":", 2)[1]
		return value + before + after + found
	}
	return value
}

func namedResult(value string) (found string) {
	if strings.Contains(value, ":") { // want `strings.Contains proves.*index 1`
		piece := strings.SplitN(value, ":", 2)[1]
		_ = piece
		return
	}
	return
}

func byteTail(value []byte) []byte {
	if bytes.Contains(value, []byte(":")) { // want `bytes.Contains proves.*index 1`
		return bytes.SplitN(value, []byte{':'}, 2)[1]
	}
	return value
}

func byteHead(value []byte) []byte {
	if bytes.Contains(value, []byte{':'}) { // want `bytes.Contains proves.*index 0`
		head := bytes.SplitN(value, []byte(":"), 5)[0]
		return head
	}
	return value
}

func sparseBytes(value []byte) []byte {
	if bytes.Contains(value, []byte{2: 'x'}) { // want `bytes.Contains proves.*index 1`
		return bytes.SplitN(value, []byte{0, 0, 'x'}, 2)[1]
	}
	return value
}

// --- negatives ---

func emptySeparator(value string) string {
	if strings.Contains(value, "") {
		return strings.SplitN(value, "", 2)[1]
	}
	return value
}

func dynamicSeparator(value, separator string) string {
	if strings.Contains(value, separator) {
		return strings.SplitN(value, separator, 2)[1]
	}
	return value
}

func differentSeparator(value string) string {
	if strings.Contains(value, ":") {
		return strings.SplitN(value, ";", 2)[1]
	}
	return value
}

func countOne(value string) string {
	if strings.Contains(value, ":") {
		return strings.SplitN(value, ":", 1)[0]
	}
	return value
}

func countZero(value string) string {
	if strings.Contains(value, ":") {
		return strings.SplitN(value, ":", 0)[0]
	}
	return value
}

func dynamicCount(value string, count int) string {
	if strings.Contains(value, ":") {
		return strings.SplitN(value, ":", count)[1]
	}
	return value
}

func tailCountThree(value string) string {
	if strings.Contains(value, ":") {
		return strings.SplitN(value, ":", 3)[1]
	}
	return value
}

func tailNegativeCount(value string) string {
	if strings.Contains(value, ":") {
		return strings.SplitN(value, ":", all)[1]
	}
	return value
}

func third(value string) string {
	if strings.Contains(value, ":") {
		return strings.SplitN(value, ":", 3)[2]
	}
	return value
}

func delayed(value string) string {
	if strings.Contains(value, ":") {
		value += ""
		return strings.SplitN(value, ":", 2)[1]
	}
	return value
}

func existingInit(value string) string {
	if ok := true; ok && strings.Contains(value, ":") {
		return strings.SplitN(value, ":", 2)[1]
	}
	return value
}

func compound(value string, enabled bool) string {
	if enabled && strings.Contains(value, ":") {
		return strings.SplitN(value, ":", 2)[1]
	}
	return value
}

func differentInput(value, other string) string {
	if strings.Contains(value, ":") {
		return strings.SplitN(other, ":", 2)[1]
	}
	return value
}

func selectedInput(holder struct{ value string }) string {
	if strings.Contains(holder.value, ":") {
		return strings.SplitN(holder.value, ":", 2)[1]
	}
	return holder.value
}

func plainSplit(value string) string {
	if strings.Contains(value, ":") {
		return strings.Split(value, ":")[1]
	}
	return value
}

func splitAfter(value string) string {
	if strings.Contains(value, ":") {
		return strings.SplitAfterN(value, ":", 2)[1]
	}
	return value
}

func stored(value string) string {
	if strings.Contains(value, ":") {
		pieces := strings.SplitN(value, ":", 2)
		return pieces[1]
	}
	return value
}

func complexLeft(value string, output []string) {
	if strings.Contains(value, ":") {
		output[0] = strings.SplitN(value, ":", 2)[1]
	}
}

func functionValues(value string) string {
	contains, split := strings.Contains, strings.SplitN
	if contains(value, ":") {
		return split(value, ":", 2)[1]
	}
	return value
}

func dynamicBytes(value, separator []byte) []byte {
	if bytes.Contains(value, separator) {
		return bytes.SplitN(value, separator, 2)[1]
	}
	return value
}

func emptyBytes(value []byte) []byte {
	if bytes.Contains(value, []byte{}) {
		return bytes.SplitN(value, []byte(""), 2)[1]
	}
	return value
}

func differentBytes(value []byte) []byte {
	if bytes.Contains(value, []byte(":")) {
		return bytes.SplitN(value, []byte(";"), 2)[1]
	}
	return value
}

type helper string

func (value helper) Contains(separator string) bool { return len(separator) != 0 }
func (value helper) SplitN(separator string, count int) []helper {
	return []helper{value, helper(separator)}
}

func methods(value helper) helper {
	if value.Contains(":") {
		return value.SplitN(":", 2)[1]
	}
	return value
}

var _ = []any{
	declareTail, assignHead, returnTail, returnHead, parenthesized, collision,
	namedResult,
	byteTail, byteHead, sparseBytes, emptySeparator, dynamicSeparator,
	differentSeparator, countOne, countZero, dynamicCount, tailCountThree,
	tailNegativeCount, third, delayed,
	existingInit, compound, differentInput, selectedInput, plainSplit,
	splitAfter, stored, complexLeft, functionValues, dynamicBytes, emptyBytes,
	differentBytes, methods,
}
