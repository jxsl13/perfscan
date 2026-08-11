package ps2103

import "fmt"

type user struct {
	name string
	id   int
}

// %d would need strconv in the rewrite: reported, but left advisory.
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

// %v over a non-string argument: reported, no fix.
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

// Width/precision genuinely needs fmt: silent.
func padded(xs []float64) []string {
	out := make([]string, 0, len(xs))
	for _, x := range xs {
		out = append(out, fmt.Sprintf("%8.3f", x))
	}
	return out
}

// Outside a loop the one-off cost is irrelevant: silent.
func label(u user) string {
	return fmt.Sprintf("%s:%d", u.name, u.id)
}
