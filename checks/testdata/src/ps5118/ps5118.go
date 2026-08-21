package ps5118

import (
	"bytes"
	"strings"
)

const nul = "\x00"

func pair(payload string) string {
	return strings.ReplaceAll(strings.ReplaceAll(payload, nul, ""), nul, "?") // want `strings.ReplaceAll eliminates byte "\\x00", so 1 enclosing Replace/ReplaceAll pass\(es\) cannot change the proven result`
}

func deepMixed(payload string) string {
	return strings.Replace(strings.ReplaceAll(strings.Replace(payload, "x", "_", -2), "x", "outer"), "x", "again", 4) // want `strings.Replace eliminates byte "x", so 2 enclosing Replace/ReplaceAll pass\(es\) cannot change the proven result`
}

// The deepest replacement reintroduces x. The middle stage removes it, so it
// is retained and only the outer pass disappears.
func partialTerminal(payload string) string {
	return strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(payload, "x", "xx"), "x", ""), "x", "unused") // want `strings.ReplaceAll eliminates byte "x", so 1 enclosing Replace/ReplaceAll pass\(es\) cannot change the proven result`
}

func invalidByte(payload string) string {
	return strings.Replace(strings.ReplaceAll(payload, "\xff", "?"), "\xff", "unused", -1) // want `strings.ReplaceAll eliminates byte "\\xff", so 1 enclosing Replace/ReplaceAll pass\(es\) cannot change the proven result`
}

func parenthesized(payload string) string {
	return strings.ReplaceAll((strings.ReplaceAll((payload), "z", "")), "z", "outer") // want `strings.ReplaceAll eliminates byte "z", so 1 enclosing Replace/ReplaceAll pass\(es\) cannot change the proven result`
}

func retainedLocalUse(payload string) string {
	const outer = "unused"
	return strings.ReplaceAll(strings.ReplaceAll(payload, "q", ""), "q", outer) + outer // want `strings.ReplaceAll eliminates byte "q", so 1 enclosing Replace/ReplaceAll pass\(es\) cannot change the proven result`
}

// Removing the only use of outer would make the function fail to compile, so
// this finding is advisory and remains unchanged in the golden file.
func localLastUse(payload string) string {
	const outer = "unused"
	return strings.ReplaceAll(strings.ReplaceAll(payload, "q", ""), "q", outer) // want `strings.ReplaceAll eliminates byte "q", so 1 enclosing Replace/ReplaceAll pass\(es\) cannot change the proven result`
}

func dynamicNew(payload string, replacement func() string) string {
	return strings.ReplaceAll(strings.ReplaceAll(payload, "x", ""), "x", replacement()) // want `strings.ReplaceAll eliminates byte "x", so 1 enclosing Replace/ReplaceAll pass\(es\) cannot change the proven result`
}

func dynamicCount(payload string, count func() int) string {
	return strings.Replace(strings.ReplaceAll(payload, "x", ""), "x", "unused", count()) // want `strings.ReplaceAll eliminates byte "x", so 1 enclosing Replace/ReplaceAll pass\(es\) cannot change the proven result`
}

// --- negatives ---

func oneLayer(payload string) string {
	return strings.ReplaceAll(payload, "x", "")
}

// Deleting a multi-byte match can create a fresh match across the removed
// boundary: ReplaceAll("aabb", "ab", "") == "ab".
func multiByteOld(payload string) string {
	return strings.ReplaceAll(strings.ReplaceAll(payload, "ab", ""), "ab", "")
}

func terminalReintroduces(payload string) string {
	return strings.ReplaceAll(strings.ReplaceAll(payload, "x", "xx"), "x", "")
}

func limitedTerminal(payload string) string {
	return strings.ReplaceAll(strings.Replace(payload, "x", "", 1), "x", "")
}

func differentOld(payload string) string {
	return strings.ReplaceAll(strings.ReplaceAll(payload, "x", ""), "y", "")
}

func dynamicOld(payload, old string) string {
	return strings.ReplaceAll(strings.ReplaceAll(payload, old, ""), old, "")
}

// PS5118 owns a content-no-op stage when it occurs inside a fixable terminal
// pipeline, preventing PS5080 from leaving a second-pass outer rewrite.
func outerContentNoop(payload string) string {
	return strings.ReplaceAll(strings.ReplaceAll(payload, "x", ""), "x", "x") // want `strings.ReplaceAll eliminates byte "x", so 1 enclosing Replace/ReplaceAll pass\(es\) cannot change the proven result`
}

func byteSlices(payload []byte) []byte {
	return bytes.ReplaceAll(bytes.ReplaceAll(payload, []byte("x"), nil), []byte("x"), nil)
}

func functionValue(payload string) string {
	replace := strings.ReplaceAll
	return replace(replace(payload, "x", ""), "x", "")
}

func ReplaceAll(payload, old, replacement string) string {
	return payload + old + replacement
}

func userFunctions(payload string) string {
	return ReplaceAll(ReplaceAll(payload, "x", ""), "x", "")
}

type helper string

func (value helper) ReplaceAll(old, replacement string) helper {
	return value + helper(old) + helper(replacement)
}

func methods(value helper) helper {
	return value.ReplaceAll("x", "").ReplaceAll("x", "")
}

var _ = []any{
	pair, deepMixed, partialTerminal, invalidByte, parenthesized,
	retainedLocalUse, localLastUse, dynamicNew, dynamicCount, oneLayer,
	multiByteOld, terminalReintroduces, limitedTerminal, differentOld,
	dynamicOld, outerContentNoop, byteSlices, functionValue, userFunctions,
	methods,
}
