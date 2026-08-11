package ps2005

import "regexp"

func hits(lines []string) int {
	hits := 0
	for _, s := range lines {
		if regexp.MustCompile("^a+$").MatchString(s) { // want `regexp\.MustCompile inside a loop recompiles an invariant pattern every iteration; hoist it out of the loop`
			hits++
		}
	}
	return hits
}

func compileErr(lines []string) int {
	n := 0
	for _, s := range lines {
		re, err := regexp.Compile("^b+$") // want `regexp\.Compile inside a loop recompiles an invariant pattern every iteration; hoist it out of the loop`
		if err == nil && re.MatchString(s) {
			n++
		}
	}
	return n
}

func hoisted(lines []string) int {
	re := regexp.MustCompile("^a+$")
	n := 0
	for _, s := range lines {
		if re.MatchString(s) {
			n++
		}
	}
	return n
}
