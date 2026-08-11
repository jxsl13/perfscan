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
