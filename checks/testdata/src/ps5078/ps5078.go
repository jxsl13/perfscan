package ps5078

import (
	"bytes"
	"strings"
)

const whitespace = " \t\r\n\u00a0"

func stringOuter(s string) string {
	return strings.TrimSpace(strings.Trim(s, whitespace)) // want `strings\.TrimSpace subsumes 1 adjacent constant whitespace-only Trim layer`
}

func stringDeep(s string) string {
	return strings.TrimSpace(strings.Trim(strings.TrimLeft(strings.TrimRight(s, "\n"), "\t"), " ")) // want `strings\.TrimSpace subsumes 3 adjacent constant whitespace-only Trim layer`
}

func stringInner(s string) string {
	return strings.Trim(strings.TrimSpace(s), whitespace) // want `strings\.TrimSpace precedes 1 adjacent constant whitespace-only Trim layer`
}

func stringInnerDeep(s string) string {
	return strings.TrimLeft(strings.TrimRight(strings.TrimSpace(s), "\n"), "\t") // want `strings\.TrimSpace precedes 2 adjacent constant whitespace-only Trim layer`
}

func byteOuter(b []byte) []byte {
	return bytes.TrimSpace(bytes.TrimRight(b, whitespace)) // want `bytes\.TrimSpace subsumes 1 adjacent constant whitespace-only Trim layer`
}

func byteInner(b []byte) []byte {
	return bytes.TrimLeft(bytes.TrimSpace(b), whitespace) // want `bytes\.TrimSpace precedes 1 adjacent constant whitespace-only Trim layer`
}

// Comments in deleted scaffolding keep the report advisory.
func commented(s string) string {
	return strings.TrimSpace(strings.Trim( /* retain */ s, whitespace)) // want `strings\.TrimSpace subsumes 1 adjacent constant whitespace-only Trim layer`
}

// Removing the Trim would leave this local constant unused.
func orphanLocal(s string) string {
	const local = " \t"
	return strings.TrimSpace(strings.Trim(s, local)) // want `strings\.TrimSpace subsumes 1 adjacent constant whitespace-only Trim layer`
}

// --- negatives ---

func nonspace(s string) string { return strings.TrimSpace(strings.Trim(s, " x")) }

func zeroWidthIsNotSpace(s string) string { return strings.TrimSpace(strings.Trim(s, "\u200b")) }

func invalidCutset(s string) string { return strings.TrimSpace(strings.Trim(s, "\xff")) }

func dynamic(s, cutset string) string { return strings.TrimSpace(strings.Trim(s, cutset)) }

func crossPackage(b []byte) []byte { return bytes.TrimSpace(bytes.Trim(b, "x")) }

func TrimSpace(s string) string { return s }

func shadowed(s string) string { return TrimSpace(strings.Trim(s, " ")) }
