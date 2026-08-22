package ps5112

import (
	"bytes"
	"strings"
)

const comma = ","
const foldedComma = "" + ","
const negative = -2
const payload = "a,b,c"

func split(s string) string {
	return strings.Join(strings.Split(s, comma), foldedComma) // want `strings.Join exactly reverses strings.Split and reconstructs its original plain-string input`
}

func emptySeparator(s string) string {
	return strings.Join(strings.Split(s, ""), "") // want `strings.Join exactly reverses strings.Split and reconstructs its original plain-string input`
}

func splitAfter(s string) string {
	return strings.Join(strings.SplitAfter(s, ","), "") // want `strings.Join exactly reverses strings.SplitAfter and reconstructs its original plain-string input`
}

func splitN(s string) string {
	return strings.Join(strings.SplitN(s, ",", negative), ",") // want `strings.Join exactly reverses strings.SplitN and reconstructs its original plain-string input`
}

func splitNOne(s string) string {
	return strings.Join(strings.SplitN(s, ",", 1), ";") // want `strings.Join consumes the single piece returned by strings.SplitN and reconstructs its original plain-string input`
}

func splitAfterN(s string) string {
	return strings.Join(strings.SplitAfterN(s, ",", -1), "") // want `strings.Join exactly reverses strings.SplitAfterN and reconstructs its original plain-string input`
}

func splitAfterNOne(s string) string {
	return strings.Join(strings.SplitAfterN(s, ",", 1), "anything") // want `strings.Join consumes the single piece returned by strings.SplitAfterN and reconstructs its original plain-string input`
}

func parenthesized(s string) string {
	return strings.Join((strings.Split((s), ",")), ",") // want `strings.Join exactly reverses strings.Split and reconstructs its original plain-string input`
}

func source() string { return payload }

func evaluatedOnce() string {
	return strings.Join(strings.Split(source(), ","), ",") // want `strings.Join exactly reverses strings.Split and reconstructs its original plain-string input`
}

func untypedConstant() any {
	return strings.Join(strings.Split(payload, ","), ",") // want `strings.Join exactly reverses strings.Split and reconstructs its original plain-string input`
}

// Removing the composition would introduce a duplicate constant switch case.
func constantSwitch(selected string) int {
	switch selected {
	case "x":
		return 1
	case strings.Join(strings.Split("x", ":"), ":"): // want `strings.Join exactly reverses strings.Split and reconstructs its original plain-string input`
		return 2
	}
	return 0
}

type stringAlias = string

func aliasInput(s stringAlias) any {
	return strings.Join(strings.Split(s, ","), ",") // want `strings.Join exactly reverses strings.Split and reconstructs its original plain-string input`
}

// Unequal and dynamic separators do not prove an inverse.
func unequal(s string) string              { return strings.Join(strings.Split(s, ","), ";") }
func dynamic(s, sep string) string         { return strings.Join(strings.Split(s, sep), sep) }
func dynamicOuter(s, sep string) string    { return strings.Join(strings.Split(s, ","), sep) }
func splitAfterNonempty(s string) string   { return strings.Join(strings.SplitAfter(s, ","), ",") }
func splitNLimited(s string) string        { return strings.Join(strings.SplitN(s, ",", 2), ",") }
func splitAfterNLimited(s string) string   { return strings.Join(strings.SplitAfterN(s, ",", 2), "") }
func splitNDynamic(s string, n int) string { return strings.Join(strings.SplitN(s, ",", n), ",") }

// A stored slice may have other consumers and is not a direct composition.
func stored(s string) string {
	parts := strings.Split(s, ",")
	return strings.Join(parts, ",")
}

// A required conversion from a defined string is retained.
type definedString string

func definedInput(s definedString) any {
	return strings.Join(strings.Split(string(s), ","), ",") // want `strings.Join exactly reverses strings.Split and reconstructs its original plain-string input`
}

// bytes.Join returns a fresh slice, so substituting b would change aliasing.
func bytesTwin(b []byte) []byte {
	return bytes.Join(bytes.Split(b, []byte(",")), []byte(","))
}

func functionValues(s string) string {
	split := strings.Split
	join := strings.Join
	return join(split(s, ","), ",")
}

type fakeStrings struct{}

func (fakeStrings) Split(s, sep string) []string           { return []string{s} }
func (fakeStrings) Join(parts []string, sep string) string { return parts[0] }

func shadowed(s string) string {
	strings := fakeStrings{}
	return strings.Join(strings.Split(s, ","), ",")
}
