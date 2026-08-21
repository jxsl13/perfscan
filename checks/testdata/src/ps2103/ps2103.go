package ps2103

import "fmt"

type user struct {
	name string
	id   int
}

// Mixed %s/%d splice: fixed — the %s arg is spliced verbatim, the %d arg
// is rendered with strconv.Itoa, and the strconv import is added.
func keys(users []user) []string {
	out := make([]string, 0, len(users))
	for _, u := range users {
		out = append(out, fmt.Sprintf("%s:%d", u.name, u.id)) // want `fmt\.Sprintf in a loop parses its format and boxes every argument per iteration; this format only splices simple verbs — build the string with concatenation or strconv instead`
	}
	return out
}

// Pure %s splice over plain strings: fixed to concatenation.
func hostports(hosts []string, port string) []string {
	out := make([]string, 0, len(hosts))
	for _, h := range hosts {
		out = append(out, fmt.Sprintf("%s:%s", h, port)) // want `fmt\.Sprintf in a loop parses its format and boxes every argument per iteration; this format only splices simple verbs — build the string with concatenation or strconv instead`
	}
	return out
}

// %v over a string behaves exactly like %s: fixed; the empty trailing
// segment is dropped.
func labeled(names []string) []string {
	out := make([]string, 0, len(names))
	for _, n := range names {
		out = append(out, fmt.Sprintf("name=%v", n)) // want `fmt\.Sprintf in a loop parses its format and boxes every argument per iteration; this format only splices simple verbs — build the string with concatenation or strconv instead`
	}
	return out
}

// Escape sequence in a middle segment and a literal tail: fixed, the
// segment is re-quoted verbatim.
func joined(pairs [][2]string) []string {
	out := make([]string, 0, len(pairs))
	for _, p := range pairs {
		out = append(out, fmt.Sprintf("%s\t%s!", p[0], p[1])) // want `fmt\.Sprintf in a loop parses its format and boxes every argument per iteration; this format only splices simple verbs — build the string with concatenation or strconv instead`
	}
	return out
}

// %d over a plain int: fixed to strconv.Itoa.
func items(nums []int) []string {
	out := make([]string, 0, len(nums))
	for _, i := range nums {
		out = append(out, fmt.Sprintf("item-%d", i)) // want `fmt\.Sprintf in a loop parses its format and boxes every argument per iteration; this format only splices simple verbs — build the string with concatenation or strconv instead`
	}
	return out
}

// %d over a signed non-int width: fixed to strconv.FormatInt with the
// int64 widening.
func seqs(nums []int64) []string {
	out := make([]string, 0, len(nums))
	for _, n := range nums {
		out = append(out, fmt.Sprintf("n=%d", n)) // want `fmt\.Sprintf in a loop parses its format and boxes every argument per iteration; this format only splices simple verbs — build the string with concatenation or strconv instead`
	}
	return out
}

// %d over an unsigned width: fixed to strconv.FormatUint with the uint64
// widening.
func counts(nums []uint) []string {
	out := make([]string, 0, len(nums))
	for _, u := range nums {
		out = append(out, fmt.Sprintf("u=%d", u)) // want `fmt\.Sprintf in a loop parses its format and boxes every argument per iteration; this format only splices simple verbs — build the string with concatenation or strconv instead`
	}
	return out
}

// %v over a non-string argument: %v honors Stringer/Formatter, so unlike
// %d it is NOT extended to integers — reported, no fix.
func ids(nums []int) []string {
	out := make([]string, 0, len(nums))
	for _, id := range nums {
		out = append(out, fmt.Sprintf("id=%v", id)) // want `fmt\.Sprintf in a loop parses its format and boxes every argument per iteration; this format only splices simple verbs — build the string with concatenation or strconv instead`
	}
	return out
}

type tag string

// Named string type: concatenation would change the expression's type —
// reported, no fix.
func tags(ts []tag) []string {
	out := make([]string, 0, len(ts))
	for _, t := range ts {
		out = append(out, fmt.Sprintf("tag=%s", t)) // want `fmt\.Sprintf in a loop parses its format and boxes every argument per iteration; this format only splices simple verbs — build the string with concatenation or strconv instead`
	}
	return out
}

type fmtInt int

// Format makes fmtInt an fmt.Formatter: it controls EVERY verb, %d included,
// so `fmt.Sprintf("%d", fmtInt(42))` is "F<42>", not "42".
func (f fmtInt) Format(s fmt.State, verb rune) { fmt.Fprintf(s, "F<%d>", int(f)) }

// Named fmt.Formatter integer under %d: %d honors the Format method, so
// strconv.Itoa would diverge (see TestPS2103DFormatterDiverges). The
// unnamed-predeclared-integer guard excludes it — reported, no fix. This is
// the load-bearing regression: it must stay advisory (golden UNCHANGED).
func formatterInts(ns []fmtInt) []string {
	out := make([]string, 0, len(ns))
	for _, n := range ns {
		out = append(out, fmt.Sprintf("v=%d", n)) // want `fmt\.Sprintf in a loop parses its format and boxes every argument per iteration; this format only splices simple verbs — build the string with concatenation or strconv instead`
	}
	return out
}

// %d over a float: %d does not format a float, and there is no strconv
// decimal for it — reported, no fix.
func floatD(xs []float64) []string {
	out := make([]string, 0, len(xs))
	for _, x := range xs {
		out = append(out, fmt.Sprintf("f=%d", x)) // want `fmt\.Sprintf in a loop parses its format and boxes every argument per iteration; this format only splices simple verbs — build the string with concatenation or strconv instead`
	}
	return out
}

// %d over a bool: not an integer — reported, no fix.
func boolD(bs []bool) []string {
	out := make([]string, 0, len(bs))
	for _, b := range bs {
		out = append(out, fmt.Sprintf("b=%d", b)) // want `fmt\.Sprintf in a loop parses its format and boxes every argument per iteration; this format only splices simple verbs — build the string with concatenation or strconv instead`
	}
	return out
}

// %d over a string: not an integer — reported, no fix.
func stringD(ss []string) []string {
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		out = append(out, fmt.Sprintf("s=%d", s)) // want `fmt\.Sprintf in a loop parses its format and boxes every argument per iteration; this format only splices simple verbs — build the string with concatenation or strconv instead`
	}
	return out
}

// Width/precision genuinely needs fmt: silent.
func padded(xs []float64) []string {
	out := make([]string, 0, len(xs))
	for _, x := range xs {
		out = append(out, fmt.Sprintf("%8.3f", x))
	}
	return out
}

// A padded integer width like %3d genuinely needs fmt: silent.
func paddedInt(nums []int) []string {
	out := make([]string, 0, len(nums))
	for _, n := range nums {
		out = append(out, fmt.Sprintf("%3d", n))
	}
	return out
}

// Hex/octal/quote verbs genuinely need fmt (PS2107's bare-verb territory,
// not a simple splice): silent.
func based(nums []int, ss []string) []string {
	out := make([]string, 0, len(nums))
	for i, n := range nums {
		out = append(out, fmt.Sprintf("x=%x", n))
		out = append(out, fmt.Sprintf("o=%o", n))
		out = append(out, fmt.Sprintf("q=%q", ss[i]))
	}
	return out
}

// Outside a loop the one-off cost is irrelevant: silent.
func label(u user) string {
	return fmt.Sprintf("%s:%d", u.name, u.id)
}
