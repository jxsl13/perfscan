package ps3030

import (
	"bytes"
	stdbytes "bytes"
	"strings"
	"unicode"
)

// --- positives: the fix applies ---

func basic(b []byte) [][]byte {
	return bytes.FieldsFunc(b, unicode.IsSpace) // want `bytes\.FieldsFunc\(b, unicode\.IsSpace\) decodes every rune and calls the predicate indirectly per rune; bytes\.Fields\(b\) is the identical split — Fields is defined as FieldsFunc\(s, unicode\.IsSpace\) — with a byte-table ASCII fast path and no indirect calls`
}

func assigned(b []byte) int {
	parts := bytes.FieldsFunc(b, unicode.IsSpace) // want `bytes\.FieldsFunc\(b, unicode\.IsSpace\) decodes every rune`
	return len(parts)
}

// An aliased bytes import keeps its qualifier: only the selected name
// changes.
func aliasedBytes(b []byte) [][]byte {
	return stdbytes.FieldsFunc(b, unicode.IsSpace) // want `bytes\.FieldsFunc\(b, unicode\.IsSpace\) decodes every rune`
}

// A parenthesized predicate is still the bare selector; the parens sit
// inside the deleted span.
func parenPred(b []byte) [][]byte {
	return bytes.FieldsFunc(b, (unicode.IsSpace)) // want `bytes\.FieldsFunc\(b, unicode\.IsSpace\) decodes every rune`
}

// The b argument passes through byte-verbatim: side effects keep their
// text, count, and order.
func sideEffects(m map[string][]byte, k string) [][]byte {
	return bytes.FieldsFunc(append(m[k], lookup()...), unicode.IsSpace) // want `bytes\.FieldsFunc\(b, unicode\.IsSpace\) decodes every rune`
}

func lookup() []byte { return []byte(" x y ") }

// --- advisory: matched, but the fix is withheld ---

// A comment inside the deleted ", unicode.IsSpace)" scaffolding would be
// silently destroyed — report only.
func commented(b []byte) [][]byte {
	return bytes.FieldsFunc(b /* fields, not words */, unicode.IsSpace) // want `bytes\.FieldsFunc\(b, unicode\.IsSpace\) decodes every rune`
}

// --- negatives: not matched at all ---

// A wrapper literal is not the bare selector (equivalent, but out of
// scope by design — the guard demands the provable shape).
func wrapper(b []byte) [][]byte {
	return bytes.FieldsFunc(b, func(r rune) bool { return unicode.IsSpace(r) })
}

// Any other predicate splits differently.
func otherPredicate(b []byte) [][]byte {
	return bytes.FieldsFunc(b, unicode.IsPunct)
}

// A variable holding unicode.IsSpace is not the bare selector.
func predicateVar(b []byte) [][]byte {
	pred := unicode.IsSpace
	return bytes.FieldsFunc(b, pred)
}

// strings.FieldsFunc is PS5034's territory, out of scope here.
func stringsVariant(s string) []string {
	return strings.FieldsFunc(s, unicode.IsSpace)
}

// A shadowed bytes identifier does not resolve to the standard
// library's function.
func shadowedBytes(b []byte) [][]byte {
	bytes := fieldsFuncer{}
	return bytes.FieldsFunc(b, unicode.IsSpace)
}

type fieldsFuncer struct{}

func (fieldsFuncer) FieldsFunc([]byte, func(rune) bool) [][]byte { return nil }

// A shadowed unicode identifier makes IsSpace a func-typed field
// (*types.Var), not the package-level function.
func shadowedUnicode(b []byte) [][]byte {
	unicode := spacer{IsSpace: func(rune) bool { return false }}
	return bytes.FieldsFunc(b, unicode.IsSpace)
}

type spacer struct{ IsSpace func(rune) bool }
