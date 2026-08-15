package ps5034

import (
	"bytes"
	"strings"
	stdstrings "strings"
	"unicode"
)

// --- positives: the fix applies ---

func basic(s string) []string {
	return strings.FieldsFunc(s, unicode.IsSpace) // want `strings\.FieldsFunc\(s, unicode\.IsSpace\) decodes every rune and calls the predicate indirectly per rune; strings\.Fields\(s\) is the identical split — Fields is defined as FieldsFunc\(s, unicode\.IsSpace\) — with a byte-table ASCII fast path and no indirect calls`
}

func assigned(s string) int {
	parts := strings.FieldsFunc(s, unicode.IsSpace) // want `strings\.FieldsFunc\(s, unicode\.IsSpace\) decodes every rune`
	return len(parts)
}

// An aliased strings import keeps its qualifier: only the selected name
// changes.
func aliasedStrings(s string) []string {
	return stdstrings.FieldsFunc(s, unicode.IsSpace) // want `strings\.FieldsFunc\(s, unicode\.IsSpace\) decodes every rune`
}

// A parenthesized predicate is still the bare selector; the parens sit
// inside the deleted span.
func parenPred(s string) []string {
	return strings.FieldsFunc(s, (unicode.IsSpace)) // want `strings\.FieldsFunc\(s, unicode\.IsSpace\) decodes every rune`
}

// The s argument passes through byte-verbatim: side effects keep their
// text, count, and order.
func sideEffects(m map[string]string, k string) []string {
	return strings.FieldsFunc(m[k]+lookup(), unicode.IsSpace) // want `strings\.FieldsFunc\(s, unicode\.IsSpace\) decodes every rune`
}

func lookup() string { return " x y " }

// --- advisory: matched, but the fix is withheld ---

// A comment inside the deleted ", unicode.IsSpace)" scaffolding would be
// silently destroyed — report only.
func commented(s string) []string {
	return strings.FieldsFunc(s /* fields, not words */, unicode.IsSpace) // want `strings\.FieldsFunc\(s, unicode\.IsSpace\) decodes every rune`
}

// --- negatives: not matched at all ---

// A wrapper literal is not the bare selector (equivalent, but out of
// scope by design — the guard demands the provable shape).
func wrapper(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return unicode.IsSpace(r) })
}

// Any other predicate splits differently.
func otherPredicate(s string) []string {
	return strings.FieldsFunc(s, unicode.IsPunct)
}

// A variable holding unicode.IsSpace is not the bare selector.
func predicateVar(s string) []string {
	pred := unicode.IsSpace
	return strings.FieldsFunc(s, pred)
}

// bytes.FieldsFunc is a different implementation, out of scope here.
func bytesVariant(b []byte) [][]byte {
	return bytes.FieldsFunc(b, unicode.IsSpace)
}

// A shadowed strings identifier does not resolve to the standard
// library's function.
func shadowedStrings(s string) []string {
	strings := fieldsFuncer{}
	return strings.FieldsFunc(s, unicode.IsSpace)
}

type fieldsFuncer struct{}

func (fieldsFuncer) FieldsFunc(string, func(rune) bool) []string { return nil }

// A shadowed unicode identifier makes IsSpace a func-typed field
// (*types.Var), not the package-level function.
func shadowedUnicode(s string) []string {
	unicode := spacer{IsSpace: func(rune) bool { return false }}
	return strings.FieldsFunc(s, unicode.IsSpace)
}

type spacer struct{ IsSpace func(rune) bool }
