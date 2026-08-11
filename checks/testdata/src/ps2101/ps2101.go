package ps2101

func filtered(src []string) []string {
	out := []string{} // want `out is appended to in the following bounded loop but declared without capacity; pre-size it with make\(\.\.\., 0, bound\) — exact for one unconditional append per iteration, an upper bound for filtered appends`
	for _, s := range src {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func varDecl(src []int) []int {
	var out []int // want `out is appended to in the following bounded loop but declared without capacity; pre-size it with make\(\.\.\., 0, bound\) — exact for one unconditional append per iteration, an upper bound for filtered appends`
	for _, v := range src {
		out = append(out, v*2)
	}
	return out
}

func fromMap(src map[string]int) []string {
	keys := []string{} // want `keys is appended to in the following bounded loop but declared without capacity; pre-size it with make\(\.\.\., 0, bound\) — exact for one unconditional append per iteration, an upper bound for filtered appends`
	for k := range src {
		keys = append(keys, k)
	}
	return keys
}

func countedLoop(n int) []int {
	out := []int{} // want `out is appended to in the following bounded loop but declared without capacity; pre-size it with make\(\.\.\., 0, bound\) — exact for one unconditional append per iteration, an upper bound for filtered appends`
	for i := 0; i < n; i++ {
		out = append(out, i*i)
	}
	return out
}

func presized(src []string) []string {
	out := make([]string, 0, len(src))
	for _, s := range src {
		out = append(out, s)
	}
	return out
}

func notAdjacent(src []string) []string {
	out := []string{}
	n := 0
	_ = n
	for _, s := range src {
		out = append(out, s)
	}
	return out
}
