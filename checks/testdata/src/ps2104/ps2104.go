package ps2104

func index(src []string) map[string]int {
	index := map[string]int{} // want `index is filled in the following bounded loop but declared without a size hint; pre-size it with make\(map\[\.\.\.\]\.\.\., len\(src\)\) — exact: one unconditional value per iteration \(a hint may over-reserve for repeated keys\)`
	for i, s := range src {
		index[s] = i
	}
	return index
}

func makeForm(src []int) map[int]bool {
	seen := make(map[int]bool) // want `seen is filled in the following bounded loop but declared without a size hint; pre-size it with make\(map\[\.\.\.\]\.\.\., len\(src\)\) — exact: one unconditional value per iteration \(a hint may over-reserve for repeated keys\)`
	for _, v := range src {
		seen[v] = true
	}
	return seen
}

func fromMap(src map[string]int) map[int]string {
	inv := map[int]string{} // want `inv is filled in the following bounded loop but declared without a size hint; pre-size it with make\(map\[\.\.\.\]\.\.\., len\(src\)\) — exact: one unconditional value per iteration \(a hint may over-reserve for repeated keys\)`
	for k, v := range src {
		inv[v] = k
	}
	return inv
}

func presized(src []string) map[string]int {
	index := make(map[string]int, len(src))
	for i, s := range src {
		index[s] = i
	}
	return index
}

// Intervening statements that do not touch the map no longer break the
// pairing.
func notAdjacent(src []string) map[string]int {
	index := map[string]int{} // want `index is filled in the following bounded loop but declared without a size hint; pre-size it with make\(map\[\.\.\.\]\.\.\., len\(src\)\) — exact: one unconditional value per iteration \(a hint may over-reserve for repeated keys\)`
	n := 0
	_ = n
	for i, s := range src {
		index[s] = i
	}
	return index
}
