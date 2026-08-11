package ps3003

func probe(ids []int, seen map[int]bool) int {
	n := 0
	for _, id := range ids {
		if seen[id] { // want `integer-keyed map seen probed in a loop pays a hash per probe; dense keys in a known range fit a slice`
			n++
		}
	}
	return n
}

func writeOnly(ids []int) map[int]bool {
	m := map[int]bool{}
	for _, id := range ids {
		m[id] = true
	}
	return m
}

func stringKeys(names []string, seen map[string]bool) int {
	n := 0
	for _, name := range names {
		if seen[name] {
			n++
		}
	}
	return n
}

func outsideLoop(seen map[int]bool, id int) bool {
	return seen[id]
}
