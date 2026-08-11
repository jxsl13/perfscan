package ps3007

func use(bool) {}

func probeOnly(seq []int64, window []int64) {
	breakers := make(map[int64]bool, len(seq))
	for _, b := range seq {
		breakers[b] = true
	}
	for _, tok := range window {
		use(breakers[tok]) // want `set breakers is built from slice seq and only probed; for a small source, scanning seq directly beats the map build plus a hash per probe \(crossover ≈8–16 elements\)`
	}
}

func structSet(seq []string, window []string) {
	seen := map[string]struct{}{}
	for _, s := range seq {
		seen[s] = struct{}{}
	}
	for _, w := range window {
		_, ok := seen[w] // want `set seen is built from slice seq and only probed; for a small source, scanning seq directly beats the map build plus a hash per probe \(crossover ≈8–16 elements\)`
		use(ok)
	}
}

// The set is written after its build loop: a mutable working set genuinely
// needs a map.
func mutable(seq []int64, window []int64) {
	avail := make(map[int64]bool, len(seq))
	for _, b := range seq {
		avail[b] = true
	}
	for _, tok := range window {
		if avail[tok] {
			avail[tok] = false
		}
	}
}

// Build guarded by a size threshold: this code has already taken the advice
// and keeps the map as its large-set fallback.
func thresholdGuarded(seq []int64, window []int64) {
	if len(seq) > 16 {
		breakers := make(map[int64]bool, len(seq))
		for _, b := range seq {
			breakers[b] = true
		}
		for _, tok := range window {
			use(breakers[tok])
		}
	}
}

// An emptiness guard is not a threshold: still reported.
func emptinessGuarded(seq []int64, window []int64) {
	if len(seq) > 0 {
		breakers := make(map[int64]bool, len(seq))
		for _, b := range seq {
			breakers[b] = true
		}
		for _, tok := range window {
			use(breakers[tok]) // want `set breakers is built from slice seq and only probed; for a small source, scanning seq directly beats the map build plus a hash per probe \(crossover ≈8–16 elements\)`
		}
	}
}

// Probe outside any loop: one probe does not repay a map, but it does not
// repeat either — silent.
func probeOnce(seq []int64, tok int64) bool {
	breakers := make(map[int64]bool, len(seq))
	for _, b := range seq {
		breakers[b] = true
	}
	return breakers[tok]
}
