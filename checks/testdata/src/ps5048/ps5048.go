package ps5048

import "strconv"

// strconv stays referenced, so the rewrites below do not orphan the import.
var _ = strconv.Atoi

// --- POSITIVES ---

func eq(a, b int) bool {
	return strconv.Itoa(a) == strconv.Itoa(b) // want `strconv\.Itoa\(a\) == strconv\.Itoa\(b\) formats two ints to throwaway strings just to compare them; a == b compares the ints directly`
}

func neq(a, b int) bool {
	return strconv.Itoa(a) != strconv.Itoa(b) // want `strconv\.Itoa\(a\) != strconv\.Itoa\(b\) formats two ints to throwaway strings just to compare them; a != b compares the ints directly`
}

// Expression arguments unwrap without parentheses (arithmetic binds tighter).
func exprArgs(a, b int) bool {
	return strconv.Itoa(a+1) == strconv.Itoa(b*2) // want `strconv\.Itoa\(a\) == strconv\.Itoa\(b\) formats two ints to throwaway strings just to compare them; a == b compares the ints directly`
}

// --- ADVISORY: reported, no fix ---

func commentInside(a, b int) bool {
	return strconv.Itoa(a) == strconv.Itoa( /* keep */ b) // want `strconv\.Itoa\(a\) == strconv\.Itoa\(b\) formats two ints to throwaway strings just to compare them; a == b compares the ints directly`
}

// --- NEGATIVES: silent ---

// Ordering does NOT carry over (lexicographic vs numeric).
func ordering(a, b int) bool {
	return strconv.Itoa(a) < strconv.Itoa(b)
}

// Only one operand is Itoa.
func oneItoa(a int) bool {
	return strconv.Itoa(a) == "5"
}

// FormatInt, not Itoa.
func formatInt(a, b int64) bool {
	return strconv.FormatInt(a, 10) == strconv.FormatInt(b, 10)
}
