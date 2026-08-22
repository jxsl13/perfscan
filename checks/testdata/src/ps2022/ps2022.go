package ps2022

import "bytes"

func use(bool) {}

// Basic condition: []byte + one string conversion, conversion on the right.
func basic(b []byte, s string) {
	if bytes.Equal(b, []byte(s)) { // want `bytes\.Equal`
		use(true)
	}
}

// Mirror: conversion on the left. The rewrite preserves the original
// left-to-right evaluation order (s first, then b).
func mirror(b []byte, s string) bool {
	return bytes.Equal([]byte(s), b) // want `bytes\.Equal`
}

// Negation: == binds looser than !, so the fix parenthesizes.
func negated(b []byte, s string) bool {
	return !bytes.Equal(b, []byte(s)) // want `bytes\.Equal`
}

// && and || bind looser than ==: no parentheses needed.
func logical(b []byte, s string, ok bool) bool {
	return bytes.Equal(b, []byte(s)) && ok // want `bytes\.Equal`
}

// Comparison parent: the LEFT operand needs no parentheses (== is
// left-associative); the RIGHT operand does.
func cmpParent(b []byte, s string, ok bool) bool {
	l := bytes.Equal(b, []byte(s)) == ok // want `bytes\.Equal`
	r := ok == bytes.Equal(b, []byte(s)) // want `bytes\.Equal`
	return l != r
}

// Delimited contexts: call argument and map index need no parentheses.
func delimited(b []byte, s string, m map[bool]int) int {
	use(bytes.Equal(b, []byte(s)))      // want `bytes\.Equal`
	return m[bytes.Equal(b, []byte(s))] // want `bytes\.Equal`
}

// Argument shapes spliced verbatim: selector, index, and call results.
type pair struct {
	buf []byte
	key string
}

func shapes(p pair, m map[string][]byte, f func() string, g func() []byte) bool {
	if bytes.Equal(p.buf, []byte(p.key)) { // want `bytes\.Equal`
		return true
	}
	if bytes.Equal(m["k"], []byte(f())) { // want `bytes\.Equal`
		return true
	}
	return bytes.Equal(g(), []byte(f())) // want `bytes\.Equal`
}

// Side effects are preserved: each operand is still evaluated exactly
// once, in the original left-to-right order, in the rewritten form.
func sideEffects(f func() string, g func() []byte) bool {
	l := bytes.Equal(g(), []byte(f())) // want `bytes\.Equal`
	r := bytes.Equal([]byte(f()), g()) // want `bytes\.Equal`
	return l != r
}

// A CONSTANT string operand is fine — string(b) is never constant, so
// the comparison stays non-constant (no PS2010-style both-constant
// hazard: a []byte operand is never a compile-time constant).
func constKey(b []byte) bool {
	return bytes.Equal(b, []byte("key")) // want `bytes\.Equal`
}

// Aliases are the identical types and match: an alias of []byte as the
// conversion target and as the byte side's declared type.
type bs = []byte

func alias(b bs, s string) bool {
	return bytes.Equal(b, bs(s)) // want `bytes\.Equal`
}

// Parenthesized operands still match; the conversion's parens are
// absorbed by the replacement.
func parens(b []byte, s string) bool {
	return bytes.Equal((b), ([]byte(s))) // want `bytes\.Equal`
}

// NEGATIVE (advisory): a comment inside the replaced call would be
// dropped by the rewrite — the fix is withheld.
func commented(b []byte, s string) bool {
	return bytes.Equal(b /* keep me */, []byte(s)) // want `bytes\.Equal`
}

// NEGATIVE (advisory): a bare expression statement, go, and defer
// syntactically require a call — string(b) == s would not compile there,
// so the fix is withheld (including behind parentheses).
func stmtOnly(b []byte, s string) {
	bytes.Equal(b, []byte(s))       // want `bytes\.Equal`
	(bytes.Equal(b, []byte(s)))     // want `bytes\.Equal`
	go bytes.Equal(b, []byte(s))    // want `bytes\.Equal`
	defer bytes.Equal(b, []byte(s)) // want `bytes\.Equal`
}

// NEGATIVE (advisory): the predeclared identifier string is shadowed at
// the call site — string(b) in the replacement would resolve to the
// local, so the fix is withheld.
func shadowedString(b []byte, s string) bool {
	string := len(s)
	_ = string
	return bytes.Equal(b, []byte(s)) // want `bytes\.Equal`
}

// NEGATIVE: BOTH operands are string conversions — that call is PS2010's
// (which deletes both conversions); PS2022 must not double-report it.
func bothConv(a, b string) bool {
	return bytes.Equal([]byte(a), []byte(b))
}

// NEGATIVE: NEITHER operand is a string conversion.
func noConv(a, b []byte) bool {
	return bytes.Equal(a, b)
}

// NEGATIVE: a named string type is not reported at all — the comparison
// belongs to the named type's own semantics (same stance as PS2010) —
// whether it appears as the conversion operand or beside a plain one.
type name string

func named(b []byte, n name) bool {
	return bytes.Equal(b, []byte(n))
}

func namedBeside(n name, s string) bool {
	return bytes.Equal([]byte(n), []byte(s))
}

// NEGATIVE: a defined byte-slice type is not the predeclared []byte —
// neither as the byte side's type nor as the conversion target.
type buf []byte

func namedSlice(b buf, s string) bool {
	return bytes.Equal(b, []byte(s))
}

func namedTarget(b []byte, s string) bool {
	return bytes.Equal(b, buf(s))
}

// NEGATIVE: an untyped nil byte side — string(nil) does not compile.
func nilSide(s string) bool {
	return bytes.Equal(nil, []byte(s))
}

// NEGATIVE: []byte of a []byte is a conversion, but not from a string —
// with no string conversion present there is nothing to delete.
func byteOfByte(a, b []byte) bool {
	return bytes.Equal(a, []byte(b))
}

// NEGATIVE: a shadowed bytes does not resolve to the stdlib function.
type fakeBytes struct{}

func (fakeBytes) Equal(a, b []byte) bool { return false }

func shadowed(b []byte, s string) bool {
	bytes := fakeBytes{}
	return bytes.Equal(b, []byte(s))
}
