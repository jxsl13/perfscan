package ps2102

func concat(parts []string) string {
	s := ""
	for _, p := range parts {
		s += p // want `string s grows by concatenation in a loop — O\(n²\) copy traffic; build it with a strings\.Builder`
	}
	return s
}

func plainAdd(parts []string) string {
	s := ""
	for _, p := range parts {
		s = s + p // want `string s grows by concatenation in a loop — O\(n²\) copy traffic; build it with a strings\.Builder`
	}
	return s
}

func intSum(xs []int) int {
	n := 0
	for _, x := range xs {
		n += x
	}
	return n
}

func outsideLoop(a, b string) string {
	a += b
	return a
}

func indexed(parts []string) string {
	s := ""
	for i := 0; i < len(parts); i++ {
		s += parts[i] // want `string s grows by concatenation in a loop — O\(n²\) copy traffic; build it with a strings\.Builder`
	}
	return s
}

func multiWrite(parts []string) string {
	s := ""
	for _, p := range parts {
		s += p // want `string s grows by concatenation in a loop — O\(n²\) copy traffic; build it with a strings\.Builder`

		s += "," // want `string s grows by concatenation in a loop — O\(n²\) copy traffic; build it with a strings\.Builder`
	}
	return s
}

// s is read inside the loop (the emptiness test): reported, never rewritten.
func readInLoop(parts []string) string {
	s := ""
	for _, p := range parts {
		if s != "" {
			s += "," // want `string s grows by concatenation in a loop — O\(n²\) copy traffic; build it with a strings\.Builder`
		}
		s += p // want `string s grows by concatenation in a loop — O\(n²\) copy traffic; build it with a strings\.Builder`
	}
	return s
}

// s is also written after the loop: reported, never rewritten.
func writeAfterLoop(parts []string, tail string) string {
	s := ""
	for _, p := range parts {
		s += p // want `string s grows by concatenation in a loop — O\(n²\) copy traffic; build it with a strings\.Builder`
	}
	s += tail
	return s
}

// A statement between the declaration and the loop reads s: reported,
// never rewritten.
func gapUse(parts []string) string {
	s := ""
	t := s
	for _, p := range parts {
		s += p // want `string s grows by concatenation in a loop — O\(n²\) copy traffic; build it with a strings\.Builder`
	}
	return s + t
}

// s is never read after the loop, so the rewritten binding would be an
// unused variable: reported, never rewritten.
func discard(parts []string) {
	s := ""
	for _, p := range parts {
		s += p // want `string s grows by concatenation in a loop — O\(n²\) copy traffic; build it with a strings\.Builder`
	}
}

// s escapes by address after the loop: reported, never rewritten.
func escapes(parts []string) string {
	s := ""
	for _, p := range parts {
		s += p // want `string s grows by concatenation in a loop — O\(n²\) copy traffic; build it with a strings\.Builder`
	}
	mutate(&s)
	return s
}

func mutate(p *string) {}

// The strings package name is shadowed by a parameter: reported, never
// rewritten.
func shadowedStrings(parts []string, strings int) string {
	s := ""
	for _, p := range parts {
		s += p // want `string s grows by concatenation in a loop — O\(n²\) copy traffic; build it with a strings\.Builder`
	}
	return s
}

// A goto could cross the re-bound declaration site: reported, never
// rewritten.
func withGoto(parts []string) string {
	s := ""
	for _, p := range parts {
		s += p // want `string s grows by concatenation in a loop — O\(n²\) copy traffic; build it with a strings\.Builder`
	}
	if len(s) > 3 {
		goto done
	}
done:
	return s
}

type myString string

// The accumulator has a named string type and is not declared as s := "":
// reported, never rewritten.
func namedType(parts []string) myString {
	var m myString
	for _, p := range parts {
		m += myString(p) // want `string m grows by concatenation in a loop — O\(n²\) copy traffic; build it with a strings\.Builder`
	}
	return m
}
