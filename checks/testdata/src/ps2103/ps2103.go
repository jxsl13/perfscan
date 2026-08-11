package ps2103

import "fmt"

type user struct {
	name string
	id   int
}

func keys(users []user) []string {
	out := make([]string, 0, len(users))
	for _, u := range users {
		out = append(out, fmt.Sprintf("%s:%d", u.name, u.id)) // want `fmt\.Sprintf in a loop parses its format and boxes every argument per iteration; this format only splices simple verbs — build the string with concatenation or strconv instead`
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
