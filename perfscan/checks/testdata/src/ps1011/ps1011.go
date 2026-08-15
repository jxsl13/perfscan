package ps1011

// Positives — every zeroing spelling and every increment spelling is
// folded into a single len assignment. The counting loops sit on one
// line so the fixture's want comments land OUTSIDE the rewritten span
// (a comment inside the span withholds the fix by design).

func basic(m map[string]int) int {
	n := 0
	for range m { n++ } // want `counting the entries of m with a range loop walks every bucket to compute a number the runtime already stores; len\(m\) reads it in O\(1\)`
	return n
}

func plusEquals(m map[string]bool) int {
	n := 0
	for range m { n += 1 } // want `counting the entries of m with a range loop`
	return n
}

func selfAdd(m map[int]int) int {
	n := 0
	for range m { n = n + 1 } // want `counting the entries of m with a range loop`
	return n
}

func selfAddSwapped(m map[int]int) int {
	n := 0
	for range m { n = 1 + n } // want `counting the entries of m with a range loop`
	return n
}

// The pre-declared spelling keeps its plain assignment token.
func reassigned(m map[string]int, n int) int {
	println(n)
	n = 0
	for range m { n++ } // want `counting the entries of m with a range loop`
	return n
}

// var n int declares exactly int with a zero value: same rewrite.
func varSpelling(m map[string]int) int {
	var n int
	for range m { n++ } // want `counting the entries of m with a range loop`
	return n
}

// Old-style blank bindings are still a pure count.
func blankKey(m map[string]int) int {
	n := 0
	for _ = range m { n++ } // want `counting the entries of m with a range loop`
	return n
}

func blankBoth(m map[string]int) int {
	n := 0
	for _, _ = range m { n++ } // want `counting the entries of m with a range loop`
	return n
}

// The range expression carries over verbatim — a call is evaluated
// exactly once in both forms (the accumulator is declared here, so the
// call cannot touch it).
func viaCall(get func() map[string]int) int {
	n := 0
	for range get() { n++ } // want `counting the entries of get\(\) with a range loop`
	return n
}

// A named map type has the same len and the same range.
type registry map[string]int

func named(r registry) int {
	n := 0
	for range r { n++ } // want `counting the entries of r with a range loop`
	return n
}

// Field and index receivers carry over verbatim.
type holder struct{ m map[int]int }

func field(h holder, hs []holder) int {
	n := 0
	for range h.m { n++ } // want `counting the entries of h\.m with a range loop`
	k := 0
	for range hs[0].m { k++ } // want `counting the entries of hs\[0\]\.m with a range loop`
	return n + k
}

// Advisory — reported, but the fix is withheld.

// The accumulator is not zeroed by the immediately preceding statement.
func nonZeroSeed(m map[int]int) int {
	n := 1
	for range m { n++ } // want `the accumulator is not zeroed by the immediately preceding statement`
	return n
}

func separatedInit(m map[int]int) int {
	n := 0
	println()
	for range m { n++ } // want `the accumulator is not zeroed by the immediately preceding statement`
	return n
}

// The loop is the first statement of its block: no init to fold.
func firstStmt(m map[int]int, n int) int {
	for range m { n++ } // want `the accumulator is not zeroed by the immediately preceding statement`
	return n
}

// The init zeroes a DIFFERENT variable (object identity, not name).
func wrongVar(m map[int]int) int {
	n, k := 0, 0
	k = 0
	for range m { n++ } // want `the accumulator is not zeroed by the immediately preceding statement`
	return n + k
}

// A pre-declared accumulator whose type is not exactly int: the
// rewritten assignment of len's int would not compile.
type counter int

func namedIntAcc(m map[int]int) counter {
	var c counter
	c = 0
	for range m { c++ } // want `the accumulator is not zeroed by the immediately preceding statement`
	return c
}

func uintAcc(m map[int]int) uint {
	var u uint
	u = 0
	for range m { u++ } // want `the accumulator is not zeroed by the immediately preceding statement`
	return u
}

// The range expression mentions the accumulator: folding it into the
// accumulator's own (re)definition would change what it refers to.
func mentionsAcc(ms []map[int]int) int {
	n := 0
	for range ms[n] { n++ } // want `the range expression mentions the accumulator`
	return n
}

// A non-plain-variable range expression in the PRE-DECLARED spelling:
// a call could write the accumulator through a captured reference, and
// a panicking index or dereference would leave it un-zeroed for a
// deferred reader — the rewrite must not reorder either against the
// zeroing.
func callReorder(get func() map[int]int, n int) int {
	println(n)
	n = 0
	for range get() { n++ } // want `the range expression is not a plain variable`
	return n
}

func recvReorder(ch chan map[int]int, n int) int {
	println(n)
	n = 0
	for range <-ch { n++ } // want `the range expression is not a plain variable`
	return n
}

func panicReorder(ms []map[int]int, n int) int {
	println(n)
	n = 0
	for range ms[5] { n++ } // want `the range expression is not a plain variable`
	return n
}

// A labeled loop may be a goto target: removing it would orphan the
// label (and a jump to it must still find a loop).
func labeled(m map[int]int) int {
	n := 0
loop:
	for range m { n++ } // want `the loop is labeled`
	if n == 0 {
		n = 1
		goto loop
	}
	return n
}

// The count is never read after the loop: n++ is the original's only
// use, so n := len(m) alone would be "declared and not used".
func deadCounter(m map[int]int) {
	n := 0
	for range m { n++ } // want `the count is never read after the loop`
}

// The builtin len is shadowed at the loop's position.
func shadowedLen(m map[int]int) int {
	len := 5
	_ = len
	n := 0
	for range m { n++ } // want `the builtin len is shadowed at this position`
	return n
}

// A comment inside the rewritten span would be destroyed by the edit.
func commented(m map[int]int) int {
	n := 0 // seed for the count
	for range m { n++ } // want `a comment inside the rewritten syntax withholds the automatic fix`
	return n
}

// Silent — not the pattern at all.

// A bound, used key is not a pure count.
func keyUsed(m map[string]int) int {
	n := 0
	for k := range m {
		if k != "" {
			n++
		}
	}
	return n
}

// Extra statements in the body change the trip's work.
func extraStmt(m map[string]int) int {
	n := 0
	for range m {
		n++
		println(n)
	}
	return n
}

// Ranging over a slice, array, string, or channel is not a map count
// (draining a channel is not len!).
func notAMap(s []int, a [4]int, str string, ch chan int) int {
	n := 0
	for range s {
		n++
	}
	for range a {
		n++
	}
	for range str {
		n++
	}
	for range ch {
		n++
	}
	return n
}

// A float accumulator is out of scope (float increments and a float
// conversion of len are not the check's integer count).
func floatAcc(m map[int]int) float64 {
	f := 0.0
	for range m {
		f++
	}
	return f
}

// Incrementing by anything but one is not an entry count.
func byTwo(m map[int]int) int {
	n := 0
	for range m {
		n += 2
	}
	return n
}
