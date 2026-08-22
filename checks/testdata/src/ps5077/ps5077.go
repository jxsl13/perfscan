package ps5077

import (
	"bytes"
	"net/http"
	"strings"
)

const trimSet = "xy"

func stringTrim(s string) string {
	return strings.Trim(strings.Trim(s, trimSet), "xy") // want `strings\.Trim is applied 2 times with the same constant cutset`
}

func stringLeft(s string) string {
	return strings.TrimLeft(strings.TrimLeft(strings.TrimLeft(s, "xy"), trimSet), "xy") // want `strings\.TrimLeft is applied 3 times with the same constant cutset`
}

func stringRight(s string) string {
	return strings.TrimRight(strings.TrimRight(s, ""), "") // want `strings\.TrimRight is applied 2 times with the same constant cutset`
}

func byteTrim(b []byte) []byte {
	return bytes.Trim(bytes.Trim(b, trimSet), "xy") // want `bytes\.Trim is applied 2 times with the same constant cutset`
}

func byteLeft(b []byte) []byte {
	return bytes.TrimLeft(bytes.TrimLeft(b, "xy"), trimSet) // want `bytes\.TrimLeft is applied 2 times with the same constant cutset`
}

func byteRight(b []byte) []byte {
	return bytes.TrimRight(bytes.TrimRight(b, trimSet), "xy") // want `bytes\.TrimRight is applied 2 times with the same constant cutset`
}

// A comment in deleted scaffolding keeps the diagnostic advisory.
func commented(s string) string {
	return strings.Trim( /* keep */ strings.Trim(s, trimSet), "xy") // want `strings\.Trim is applied 2 times with the same constant cutset`
}

// Removing the outer call would leave this function-local constant unused.
func orphanLocal(s string) string {
	const outerCutset = "xy"
	return strings.Trim(strings.Trim(s, "xy"), outerCutset) // want `strings\.Trim is applied 2 times with the same constant cutset`
}

// Removing the outer call would orphan the file's net/http import.
func orphanImport(s string) string {
	return strings.Trim(strings.Trim(s, "GET"), http.MethodGet) // want `strings\.Trim is applied 2 times with the same constant cutset`
}

// A local constant with a surviving use does not block the fix.
func survivingLocal(s string) (string, string) {
	const cutset = "xy"
	return strings.Trim(strings.Trim(s, "xy"), cutset), cutset // want `strings\.Trim is applied 2 times with the same constant cutset`
}

// --- negatives ---

func one(s string) string { return strings.Trim(s, trimSet) }

func different(s string) string { return strings.Trim(strings.Trim(s, "x"), "y") }

func dynamic(s, cutset string) string { return strings.Trim(strings.Trim(s, cutset), cutset) }

func crossFunction(s string) string { return strings.TrimLeft(strings.Trim(s, trimSet), trimSet) }

func Trim(s, cutset string) string { return s }

func shadowed(s string) string { return Trim(Trim(s, trimSet), trimSet) }

type trimmer string

func (t trimmer) Trim(string) trimmer { return t }

func method(t trimmer) trimmer { return t.Trim(trimSet).Trim(trimSet) }
