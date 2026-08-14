package ps2133

import t "time"

// The stdlib time imported under an alias still resolves to the same
// package: the check must fire through the alias exactly as it does
// through the canonical qualifier.
func aliasedInBerlin(when t.Time) t.Time {
	loc, _ := t.LoadLocation("Europe/Berlin") // want `time\.LoadLocation with a constant zone name re-reads and parses the tzdata on every call`
	return when.In(loc)
}

// NEGATIVE: the fast-path names touch no database, alias or not: silent.
func aliasedFastPath() {
	_, _ = t.LoadLocation("UTC")
}

// NEGATIVE: a runtime zone name through the alias is genuinely per-call: silent.
func aliasedDynamic(when t.Time, name string) t.Time {
	loc, _ := t.LoadLocation(name)
	return when.In(loc)
}
