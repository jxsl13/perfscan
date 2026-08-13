package ps2003

import "strings"

func emit(string) {}

func inLoop(names []string) {
	for _, name := range names {
		emit(strings.ReplaceAll(name, "-", "_")) // want `strings\.ReplaceAll in a loop allocates a fresh string per iteration; hoist the transform, build a strings\.Replacer once, or reuse a byte buffer`
	}
}

func repeatInLoop(n int) string {
	s := ""
	for i := 0; i < n; i++ {
		s += strings.Repeat("x", i) // want `strings\.Repeat in a loop allocates a fresh string per iteration; hoist the transform, build a strings\.Replacer once, or reuse a byte buffer`
	}
	return s
}

func outside(name string) string {
	return strings.ReplaceAll(name, "-", "_")
}

func replacer(names []string) {
	r := strings.NewReplacer("-", "_")
	for _, name := range names {
		emit(r.Replace(name))
	}
}

func hoistable(n int) {
	sep := "-"
	for i := 0; i < n; i++ {
		emit(strings.ReplaceAll("a-b-c", sep, "_")) // want `strings\.ReplaceAll in a loop allocates a fresh string per iteration; hoist the transform, build a strings\.Replacer once, or reuse a byte buffer`
	}
}

func nested(rows [][]string) {
	for _, row := range rows {
		for range row {
			emit(strings.Repeat("x", 3)) // want `strings\.Repeat in a loop allocates a fresh string per iteration; hoist the transform, build a strings\.Replacer once, or reuse a byte buffer`
		}
	}
}

func mutated(n int) {
	sep := "-"
	for i := 0; i < n; i++ {
		emit(strings.ReplaceAll("a-b", sep, "_")) // want `strings\.ReplaceAll in a loop allocates a fresh string per iteration; hoist the transform, build a strings\.Replacer once, or reuse a byte buffer`
		sep = "_"
	}
}

func mapHoist(lines []string) {
	for range lines {
		emit(strings.Map(swap, "abc")) // want `strings\.Map in a loop allocates a fresh string per iteration; hoist the transform, build a strings\.Replacer once, or reuse a byte buffer`
	}
}

// arg reassigned via += -> identWrittenIn catches the ADD_ASSIGN write, so the
// per-iteration result differs and the transform stays advisory.
func argAddAssign(names []string, s string) {
	for range names {
		emit(strings.ReplaceAll(s, "-", "_")) // want `strings\.ReplaceAll in a loop allocates a fresh string per iteration; hoist the transform, build a strings\.Replacer once, or reuse a byte buffer`
		s += "x"
	}
}

// arg reassigned inside a closure -> identWrittenIn descends into the FuncLit
// and declines the hoist.
func argClosureMut(names []string, s string) {
	for range names {
		emit(strings.ReplaceAll(s, "-", "_")) // want `strings\.ReplaceAll in a loop allocates a fresh string per iteration; hoist the transform, build a strings\.Replacer once, or reuse a byte buffer`
		func() { s = "y" }()
	}
}

// an index-expr arg is not a plain identifier, so it never reaches the fixable
// shape (only basic literals and plain, unwritten idents hoist).
func argIndexExpr(names []string, m map[int]string) {
	for range names {
		emit(strings.ReplaceAll(m[0], "-", "_")) // want `strings\.ReplaceAll in a loop allocates a fresh string per iteration; hoist the transform, build a strings\.Replacer once, or reuse a byte buffer`
		m[0] = "z"
	}
}

func swap(r rune) rune { return r }

// strings.Repeat panics when count < 0; a variable count cannot be proven
// non-negative, so hoisting would move (or, for a zero-iteration loop,
// introduce) that panic. Reported without a fix.
func repeatVarCount(lines []string, s string, n int) {
	for range lines {
		emit(strings.Repeat(s, n)) // want `strings\.Repeat in a loop allocates a fresh string per iteration; hoist the transform, build a strings\.Replacer once, or reuse a byte buffer`
	}
}

const repeatWidth = 4

// a non-negative constant count can never panic, so the hoist stays available
// for const-ident counts (literal counts are covered by nested above).
func repeatConstCount(lines []string) {
	for range lines {
		emit(strings.Repeat("y", repeatWidth)) // want `strings\.Repeat in a loop allocates a fresh string per iteration; hoist the transform, build a strings\.Replacer once, or reuse a byte buffer`
	}
}
