package ps5067

import "unicode/utf8"

type namedStr string
type namedBytes []byte

// --- POSITIVES: reverse direction (the clear win) ---

func reverseB(b []byte) bool {
	return utf8.FullRuneInString(string(b)) // want `copies the whole \[\]byte through a throwaway conversion`
}

// Expression operand carries over verbatim.
func reverseExpr(b []byte) bool {
	return utf8.FullRuneInString(string(b[1:])) // want `copies the whole \[\]byte through a throwaway conversion`
}

// --- POSITIVES: forward direction (parity, never a regression) ---

func forwardS(s string) bool {
	return utf8.FullRune([]byte(s)) // want `copies the whole string through a throwaway conversion`
}

// --- ADVISORY: reported, no fix ---

func commentInside(b []byte) bool {
	return utf8.FullRuneInString(string( /* keep */ b)) // want `copies the whole \[\]byte through a throwaway conversion`
}

// --- NEGATIVES: silent ---

// Already the sibling shape, no conversion.
func alreadyBytes(b []byte) bool {
	return utf8.FullRune(b)
}

// A named byte-slice operand: the conversion also changes the static type.
func namedByteOperand(b namedBytes) bool {
	return utf8.FullRuneInString(string(b))
}

// A []rune operand: string([]rune) ENCODES, not a byte copy.
func runeOperand(r []rune) bool {
	return utf8.FullRuneInString(string(r))
}

// A named string operand in the forward direction.
func namedStrOperand(s namedStr) bool {
	return utf8.FullRune([]byte(s))
}
