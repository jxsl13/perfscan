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

func variant(pats, lines []string) int {
	n := 0
	for _, p := range pats {
		re := regexp.MustCompile(p) // loop-variant pattern: p changes each iteration, cannot hoist
		for _, s := range lines {
			if re.MatchString(s) {
				n++
			}
		}
	}
	return n
}

func variantIndex(pats, lines []string) int {
	n := 0
	for i := range pats {
		re := regexp.MustCompile(pats[i]) // loop-variant pattern: depends on loop index i
		for _, s := range lines {
			if re.MatchString(s) {
				n++
			}
		}
	}
	return n
}

func invalidPattern(lines []string) int {
	n := 0
	for _, s := range lines {
		// Invalid pattern: MustCompile panics whenever it runs. Hoisting would
		// move the panic before the loop (and introduce it for empty lines).
		// Advisory only — no fix.
		if regexp.MustCompile("(").MatchString(s) { // want `regexp\.MustCompile inside a loop recompiles an invariant pattern every iteration; hoist it out of the loop`
			n++
		}
	}
	return n
}

func invalidPatternPOSIX(lines []string) int {
	n := 0
	for _, s := range lines {
		// Invalid POSIX pattern: MustCompilePOSIX panics whenever it runs.
		// Advisory only — no fix.
		if regexp.MustCompilePOSIX("(").MatchString(s) { // want `regexp\.MustCompilePOSIX inside a loop recompiles an invariant pattern every iteration; hoist it out of the loop`
			n++
		}
	}
	return n
}

func build() string { return "x" }

func invariantVar(lines []string) int {
	pat := build()
	n := 0
	for _, s := range lines {
		if regexp.MustCompile(pat).MatchString(s) { // want `regexp\.MustCompile inside a loop recompiles an invariant pattern every iteration; hoist it out of the loop`
			n++
		}
	}
	return n
}
