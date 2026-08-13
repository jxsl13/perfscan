package ps2133

import "time"

const nyZone = "America/New_York"

// Constant real zone name (literal) -> flagged.
func inNY(t time.Time) time.Time {
	loc, _ := time.LoadLocation("Europe/Berlin") // want `time\.LoadLocation with a constant zone name re-reads and parses the tzdata on every call`
	return t.In(loc)
}

// Constant zone name via const ident -> flagged.
func inZone(t time.Time) time.Time {
	loc, _ := time.LoadLocation(nyZone) // want `time\.LoadLocation with a constant zone name re-reads and parses the tzdata on every call`
	return t.In(loc)
}

// NEGATIVE: the fast-path names "", "UTC", "Local" touch no database: silent.
func fastPaths(t time.Time) {
	_, _ = time.LoadLocation("UTC")
	_, _ = time.LoadLocation("")
	_, _ = time.LoadLocation("Local")
}

// NEGATIVE: a runtime (variable) zone name is genuinely per-call: silent.
func dynamic(t time.Time, name string) time.Time {
	loc, _ := time.LoadLocation(name)
	return t.In(loc)
}

// NEGATIVE: a package-level cached location is the good form, out of scope.
var berlin, _ = time.LoadLocation("Europe/Berlin")

func good(t time.Time) time.Time { return t.In(berlin) }

// NEGATIVE: a shadowed time does not resolve to the stdlib function.
type fakeTime struct{}

func (fakeTime) LoadLocation(_ string) (*time.Location, error) { return nil, nil }

func shadowed() {
	time := fakeTime{}
	_, _ = time.LoadLocation("America/New_York")
}
