package ps2044

import (
	"fmt"
	"os"
	"strings"
)

// Keeps fmt referenced so the rewrites below do not orphan the import
// (orphan.go exercises the orphan path).
func keep() { fmt.Println("x") }

// --- POSITIVES: literal text spliced with bare %s verbs, plain strings,
// --- unnamed []byte dest ---

func midTrail(buf []byte, k, v string) []byte {
	return fmt.Appendf(buf, "%s=%s;", k, v) // want `fmt\.Appendf splicing plain strings into literal text`
}

func leading(buf []byte, h, p string) []byte {
	return fmt.Appendf(buf, "host=%s;port=%s", h, p) // want `fmt\.Appendf splicing plain strings into literal text`
}

func spaceMid(buf []byte, a, b string) []byte {
	return fmt.Appendf(buf, "%s %s", a, b) // want `fmt\.Appendf splicing plain strings into literal text`
}

func singleLead(buf []byte, v string) []byte {
	return fmt.Appendf(buf, "k=%s", v) // want `fmt\.Appendf splicing plain strings into literal text`
}

// With an EMPTY leading segment the chain's first write to buf happens after
// the first operand's evaluation — so the FIRST operand may be any string
// expression, exactly PS2033's rule.
func firstCallEmptyLead(buf []byte, a string) []byte {
	return fmt.Appendf(buf, "%s;", strings.ToUpper(a)) // want `fmt\.Appendf splicing plain strings into literal text`
}

// Literal segments are re-emitted from their DECODED value, so escape
// sequences keep an identical runtime value.
func escapes(buf []byte, a, b string) []byte {
	return fmt.Appendf(buf, "%s\n\t\"q\"%s", a, b) // want `fmt\.Appendf splicing plain strings into literal text`
}

// A raw-string format is decoded the same way.
func rawFormat(buf []byte, a string) []byte {
	return fmt.Appendf(buf, `x %s`, a) // want `fmt\.Appendf splicing plain strings into literal text`
}

// Untyped string constants default to string; a package-qualified identifier
// is inert.
func lits(buf []byte, a string) []byte {
	return fmt.Appendf(buf, "%s=%s.", a, os.DevNull) // want `fmt\.Appendf splicing plain strings into literal text`
}

// --- ADVISORY: reported but NOT fixed (not provably bit-identical) ---

type MyStr string

// A NAMED string type may implement fmt.Stringer/fmt.Formatter.
func named(buf []byte, a string, s MyStr) []byte {
	return fmt.Appendf(buf, "%s=%s", a, s) // want `fmt\.Appendf splicing plain strings into literal text`
}

type ByteBuf []byte

// A NAMED []byte destination changes the returned slice type.
func namedDest(buf ByteBuf, a, b string) ByteBuf {
	return fmt.Appendf(buf, "%s=%s", a, b) // want `fmt\.Appendf splicing plain strings into literal text`
}

// A []byte operand — fixable for single-operand PS2141 — is NOT fixable here:
// if it aliases buf's spare capacity, an earlier append in the chain clobbers
// it before a later append reads it, while fmt.Appendf reads every operand
// first.
func byteSliceOperand(buf []byte, a string, bs []byte) []byte {
	return fmt.Appendf(buf, "%s=%s", a, bs) // want `fmt\.Appendf splicing plain strings into literal text`
}

// A LATER operand that runs user code is evaluated after the chain's first
// write to buf but before fmt.Appendf's only write — an observable
// difference, so the call keeps the advisory report.
func laterCall(buf []byte, a, b string) []byte {
	return fmt.Appendf(buf, "%s=%s", a, strings.ToUpper(b)) // want `fmt\.Appendf splicing plain strings into literal text`
}

// With a NON-EMPTY leading segment the chain writes that literal to buf
// BEFORE evaluating any operand — so even the FIRST operand must be inert
// (unlike PS2033's empty-lead rule above).
func firstCallLeadingLit(buf []byte, a string) []byte {
	return fmt.Appendf(buf, "k=%s", strings.ToUpper(a)) // want `fmt\.Appendf splicing plain strings into literal text`
}

// An index expression may panic — panic timing would move across the first
// write, so it is not inert either.
func laterIndex(buf []byte, a string, parts []string) []byte {
	return fmt.Appendf(buf, "%s=%s", a, parts[0]) // want `fmt\.Appendf splicing plain strings into literal text`
}

// A comment inside the rewritten scaffolding would be destroyed — advisory.
func commented(buf []byte, a, b string) []byte {
	return fmt.Appendf(buf, "%s=%s" /* keep me */, a, b) // want `fmt\.Appendf splicing plain strings into literal text`
}

// --- NEGATIVES: not reported ---

// The pure repeated-%s format is PS2033's shape, the lone %s PS2141's and
// the verbless constant PS3025's.
func pureVerbs(buf []byte, a, b string) []byte {
	return fmt.Appendf(buf, "%s%s", a, b)
}

func single(buf []byte, a string) []byte {
	return fmt.Appendf(buf, "%s", a)
}

func verbless(buf []byte) []byte {
	return fmt.Appendf(buf, "constant")
}

// Any flag, width, %% or other verb disqualifies the format.
func otherVerb(buf []byte, a string, n int) []byte {
	return fmt.Appendf(buf, "%s=%d", a, n)
}

func percent(buf []byte, a, b string) []byte {
	return fmt.Appendf(buf, "%s%%;%s", a, b)
}

func flagged(buf []byte, a, b string) []byte {
	return fmt.Appendf(buf, "%s=%-8s", a, b)
}

// A verb/operand count mismatch is a broken call, not this pattern.
func mismatch(buf []byte, a, b string) []byte {
	return fmt.Appendf(buf, "%s=", a, b)
}

// A NIL destination is silent, not advisory: append(nil, ...) does not
// compile, so the advised chain is unspellable — that shape wants a
// different rewrite entirely.
func nilDest(a, b string) []byte {
	return fmt.Appendf(nil, "%s=%s", a, b)
}

// %s over an int is different formatting, not a string append.
func intVal(buf []byte, a string, n int) []byte {
	return fmt.Appendf(buf, "%s=%s", a, n)
}

// A spread call passes an unknown number of arguments.
func spread(buf []byte, args ...any) []byte {
	return fmt.Appendf(buf, "%s=%s", args...)
}

// A non-literal format proves nothing.
func varFormat(buf []byte, a, b string) []byte {
	format := "%s=%s"
	return fmt.Appendf(buf, format, a, b)
}

// A shadowed fmt is not the fmt package.
type fakeFmt struct{}

func (fakeFmt) Appendf(b []byte, format string, args ...any) []byte { return b }

func shadowed(buf []byte, a, b string) []byte {
	fmt := fakeFmt{}
	return fmt.Appendf(buf, "%s=%s", a, b)
}
