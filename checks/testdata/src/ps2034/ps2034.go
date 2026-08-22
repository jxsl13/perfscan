package ps2034

import (
	"fmt"
	"strings"
)

func sink(string) {}

// Assignment context: no parentheses needed; the interleaved literal
// segments become quoted constants between the operands.
func hostPort(host, port string) string {
	key := fmt.Sprintf("host=%s;port=%s", host, port) // want `fmt\.Sprintf splicing plain strings into literal text boxes every argument and walks fmt's formatter state machine; \+ concatenation with the literal segments builds the identical string`
	return key
}

// A single verb with literal text around it is in scope (the lone bare
// "%s" with NO text belongs to PS2130).
func leadOnly(name string) string {
	return fmt.Sprintf("user=%s", name) // want `fmt\.Sprintf splicing plain strings into literal text boxes every argument and walks fmt's formatter state machine; \+ concatenation with the literal segments builds the identical string`
}

func trailOnly(name string) string {
	return fmt.Sprintf("%s!", name) // want `fmt\.Sprintf splicing plain strings into literal text boxes every argument and walks fmt's formatter state machine; \+ concatenation with the literal segments builds the identical string`
}

// Text only BETWEEN adjacent verbs; leading and trailing segments empty.
func midOnly(a, b string) string {
	return fmt.Sprintf("%s-%s", a, b) // want `fmt\.Sprintf splicing plain strings into literal text boxes every argument and walks fmt's formatter state machine; \+ concatenation with the literal segments builds the identical string`
}

// Three verbs; the empty segment between the second and third verbs
// contributes nothing (plain " + " join there).
func threeArgs(a, b, c string) string {
	return fmt.Sprintf("[%s:%s%s]", a, b, c) // want `fmt\.Sprintf splicing plain strings into literal text boxes every argument and walks fmt's formatter state machine; \+ concatenation with the literal segments builds the identical string`
}

// Escape sequences in the format re-emit as an identical interpreted
// literal (strconv.Quote of the decoded segment).
func escapes(a string) string {
	return fmt.Sprintf("a\tb%s\n", a) // want `fmt\.Sprintf splicing plain strings into literal text boxes every argument and walks fmt's formatter state machine; \+ concatenation with the literal segments builds the identical string`
}

// A raw-string format decodes the same way; the segments re-emit as
// interpreted literals with the identical runtime value.
func rawFormat(a string) string {
	return fmt.Sprintf(`x=%s`, a) // want `fmt\.Sprintf splicing plain strings into literal text boxes every argument and walks fmt's formatter state machine; \+ concatenation with the literal segments builds the identical string`
}

// Non-ASCII literal text survives the re-quote byte-for-byte.
func unicodeText(who string) string {
	return fmt.Sprintf("héllo %s 世界", who) // want `fmt\.Sprintf splicing plain strings into literal text boxes every argument and walks fmt's formatter state machine; \+ concatenation with the literal segments builds the identical string`
}

// An argument that is itself a call stays verbatim.
func callArg(a string) string {
	return fmt.Sprintf("v=%s", strings.ToUpper(a)) // want `fmt\.Sprintf splicing plain strings into literal text boxes every argument and walks fmt's formatter state machine; \+ concatenation with the literal segments builds the identical string`
}

// An argument that is itself a + concatenation needs no parentheses:
// string + is associative and the only string-producing binary operator.
func concatArg(a, b string) string {
	return fmt.Sprintf("v=%s.", a+b) // want `fmt\.Sprintf splicing plain strings into literal text boxes every argument and walks fmt's formatter state machine; \+ concatenation with the literal segments builds the identical string`
}

// An untyped constant string argument defaults to plain string.
func literalArg(b string) string {
	return fmt.Sprintf("<%s%s>", "pre-", b) // want `fmt\.Sprintf splicing plain strings into literal text boxes every argument and walks fmt's formatter state machine; \+ concatenation with the literal segments builds the identical string`
}

// Call-argument context is self-delimiting: no parentheses needed.
func asArg(a string) {
	sink(fmt.Sprintf("k:%s", a)) // want `fmt\.Sprintf splicing plain strings into literal text boxes every argument and walks fmt's formatter state machine; \+ concatenation with the literal segments builds the identical string`
}

// PRECEDENCE: indexing binds tighter than + — the whole replacement must
// be parenthesized or the index would apply to the last operand only.
func indexed(a string) byte {
	return fmt.Sprintf("k:%s", a)[0] // want `fmt\.Sprintf splicing plain strings into literal text boxes every argument and walks fmt's formatter state machine; \+ concatenation with the literal segments builds the identical string`
}

// A FuncLit inside a loop is NOT a loop body for InLoop — PS2103 does not
// claim it, so this check does.
func closureInLoop(parts []string) []func() string {
	var fns []func() string
	for _, p := range parts {
		p := p
		fns = append(fns, func() string {
			return fmt.Sprintf("p=%s", p) // want `fmt\.Sprintf splicing plain strings into literal text boxes every argument and walks fmt's formatter state machine; \+ concatenation with the literal segments builds the identical string`
		})
	}
	return fns
}

// Reported but NOT fixed: a comment inside the rewritten scaffolding
// would be destroyed by the edits.
func commented(a, b string) string {
	return fmt.Sprintf( // keep me // want `fmt\.Sprintf splicing plain strings into literal text boxes every argument and walks fmt's formatter state machine; \+ concatenation with the literal segments builds the identical string`
		"a=%s b=%s", a, b)
}

// --- guards: none of the following may be reported or rewritten ---

// The pure ("%s")^k format is PS2122's turf.
func pureVerbs(a, b string) string {
	return fmt.Sprintf("%s%s", a, b)
}

// The lone bare "%s" is PS2130's turf.
func bareIdentity(a string) string {
	return fmt.Sprintf("%s", a)
}

// Inside a loop body this shape belongs to PS2103 (sprintf-concat-in-loop).
func inLoop(parts []string) []string {
	var out []string
	for _, p := range parts {
		out = append(out, fmt.Sprintf("p=%s", p))
	}
	return out
}

func inRangeInt(a string) []string {
	var out []string
	for i := range 3 {
		_ = i
		out = append(out, fmt.Sprintf("x-%s", a))
	}
	return out
}

// %% is excluded: fmt collapses it to one "%" — a different literal than
// the format text, out of this check's scope.
func percentEscape(a string) string {
	return fmt.Sprintf("100%%%s", a)
}

// Any other verb disqualifies the format.
func otherVerb(n int, b string) string {
	return fmt.Sprintf("%d:%s", n, b)
}

// A width, flag or precision genuinely needs fmt.
func width(a string) string {
	return fmt.Sprintf("v=%5s", a)
}

func precision(a string) string {
	return fmt.Sprintf("v=%.3s", a)
}

// A trailing lone % is malformed for this scope.
func trailingPercent(a string) string {
	return fmt.Sprintf("v=%s%", a)
}

// Argument count must equal the verb count.
func missingArg(a, b string) string {
	return fmt.Sprintf("%s=%s;%s", a, b)
}

// Non-string arguments: %s on them is not the identity.
func intArg(n int) string {
	return fmt.Sprintf("n=%s", n)
}

func byteSliceArg(b []byte) string {
	return fmt.Sprintf("b=%s", b)
}

// A NAMED string type may implement fmt.Stringer/fmt.Formatter, which %s
// would honor and + would not.
type name string

func (n name) String() string { return "Mx. " + string(n) }

func namedStringArg(n name) string {
	return fmt.Sprintf("who=%s", n)
}

func errorArg(err error) string {
	return fmt.Sprintf("err=%s", err)
}

func stringerArg(s fmt.Stringer) string {
	return fmt.Sprintf("s=%s", s)
}

// A shadowed fmt is not the fmt package.
type fakeFmt struct{}

func (fakeFmt) Sprintf(format string, args ...any) string { return format }

func shadowedFmt(a string) string {
	fmt := fakeFmt{}
	return fmt.Sprintf("v=%s", a)
}

// A spread call passes an unknown number of arguments.
func spread(args ...any) string {
	return fmt.Sprintf("a=%s b=%s", args...)
}

// A non-literal format proves nothing.
func varFormat(a string) string {
	f := "v=%s"
	return fmt.Sprintf(f, a)
}
