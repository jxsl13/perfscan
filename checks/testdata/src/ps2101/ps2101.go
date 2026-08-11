package ps2101

func filtered(src []string) []string {
	out := []string{} // want `out is appended to in the following bounded loop but declared without capacity; pre-size it with make\(\.\.\., 0, len\(src\)\) — an upper bound: all appends are conditional`
	for _, s := range src {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func varDecl(src []int) []int {
	var out []int // want `out is appended to in the following bounded loop but declared without capacity; pre-size it with make\(\.\.\., 0, len\(src\)\) — exact: one unconditional value per iteration`
	for _, v := range src {
		out = append(out, v*2)
	}
	return out
}

func fromMap(src map[string]int) []string {
	keys := []string{} // want `keys is appended to in the following bounded loop but declared without capacity; pre-size it with make\(\.\.\., 0, len\(src\)\) — exact: one unconditional value per iteration`
	for k := range src {
		keys = append(keys, k)
	}
	return keys
}

func countedLoop(n int) []int {
	out := []int{} // want `out is appended to in the following bounded loop but declared without capacity; pre-size it with make\(\.\.\., 0, n\) — exact: one unconditional value per iteration`
	for i := 0; i < n; i++ {
		out = append(out, i*i)
	}
	return out
}

// Two unconditional appends per iteration: the capacity doubles, exactly.
func pairs(src []int) []int {
	out := []int{} // want `out is appended to in the following bounded loop but declared without capacity; pre-size it with make\(\.\.\., 0, 2\*len\(src\)\) — exact: 2 unconditional value\(s\) per iteration`
	for _, v := range src {
		out = append(out, v)
		out = append(out, -v)
	}
	return out
}

// A multi-value append counts each value.
func multiValue(src []int) []int {
	out := []int{} // want `out is appended to in the following bounded loop but declared without capacity; pre-size it with make\(\.\.\., 0, 2\*len\(src\)\) — exact: 2 unconditional value\(s\) per iteration`
	for _, v := range src {
		out = append(out, v, v*v)
	}
	return out
}

// One unconditional plus one conditional: the unconditional count is a
// LOWER bound.
func mixed(src []int) []int {
	out := []int{} // want `out is appended to in the following bounded loop but declared without capacity; pre-size it with make\(\.\.\., 0, len\(src\)\) — a lower bound: 1 unconditional value\(s\) per iteration, conditional ones excluded`
	for _, v := range src {
		out = append(out, v)
		if v > 0 {
			out = append(out, -v)
		}
	}
	return out
}

// A spread append has an unknown per-call count: upper-bound form.
func spread(src [][]int) []int {
	out := []int{} // want `out is appended to in the following bounded loop but declared without capacity; pre-size it with make\(\.\.\., 0, len\(src\)\) — an upper bound: all appends are conditional`
	for _, vs := range src {
		out = append(out, vs...)
	}
	return out
}

type box struct{ items []string }

func selectorSource(b box) []string {
	out := []string{} // want `out is appended to in the following bounded loop but declared without capacity; pre-size it with make\(\.\.\., 0, len\(b\.items\)\) — exact: one unconditional value per iteration`
	for _, s := range b.items {
		out = append(out, s)
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

// Statements between declaration and loop that do not touch the variable
// no longer break the pairing (standalone declarations count).
func notAdjacent(src []string) []string {
	out := []string{} // want `out is appended to in the following bounded loop but declared without capacity; pre-size it with make\(\.\.\., 0, len\(src\)\) — exact: one unconditional value per iteration`
	n := 0
	_ = n
	for _, s := range src {
		out = append(out, s)
	}
	return out
}

// One loop filling two targets: one finding (and fix) per target.
func twoTargets(src map[string]int) ([]string, []int) {
	keys := []string{} // want `keys is appended to in the following bounded loop but declared without capacity; pre-size it with make\(\.\.\., 0, len\(src\)\) — exact: one unconditional value per iteration`
	vals := []int{}    // want `vals is appended to in the following bounded loop but declared without capacity; pre-size it with make\(\.\.\., 0, len\(src\)\) — exact: one unconditional value per iteration`
	for k, v := range src {
		keys = append(keys, k)
		vals = append(vals, v)
	}
	return keys, vals
}

// The bound's subject is defined AFTER the declaration: the advisory
// stays, but no fix may reference b at the declaration site (found by
// auto-fixing a real-world codebase).
func indentString(s string) []byte {
	var res []byte // want `res is appended to in the following bounded loop but declared without capacity; pre-size it with make\(\.\.\., 0, bound\) — an upper bound: all appends are conditional`
	b := []byte(s)
	for _, c := range b {
		if c != 0 {
			res = append(res, c)
		}
	}
	return res
}
