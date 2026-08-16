package checks

import (
	"math"
	"strconv"
	"testing"
)

// Runtime differential for PS5071: strconv.Itoa(x) == c / != c versus
// x == v / x != v, where c is the canonical decimal spelling of v. Itoa is
// injective and emits exactly one canonical decimal per int, so for a
// canonical constant the string test is the integer test. This suite sweeps a
// wide range of x against several canonical constants and confirms the two
// forms agree for both operators; it also pins the NON-canonical cases the
// check excludes — where Itoa(x) == c is constant-false (and != constant-true)
// for every x, which the integer form would NOT reproduce.
func TestEquiv_PS5071ItoaConstCompare(t *testing.T) {
	// (constant string, its int value) — all canonical.
	canon := []struct {
		s string
		v int
	}{
		{"0", 0}, {"1", 1}, {"-1", -1}, {"200", 200}, {"404", 404},
		{"-5", -5}, {"2147483647", math.MaxInt32}, {"-2147483648", math.MinInt32},
		{"42", 42}, {"1000000", 1000000},
	}
	xs := []int{
		0, 1, -1, 5, -5, 200, 404, 42, 1000000,
		math.MaxInt32, math.MinInt32, 123456, -999999,
	}
	for _, c := range canon {
		for _, x := range xs {
			if (strconv.Itoa(x) == c.s) != (x == c.v) {
				t.Fatalf("== diverged: Itoa(%d)==%q is %v but %d==%d is %v", x, c.s, strconv.Itoa(x) == c.s, x, c.v, x == c.v)
			}
			if (strconv.Itoa(x) != c.s) != (x != c.v) {
				t.Fatalf("!= diverged: Itoa(%d)!=%q vs %d!=%d", x, c.s, x, c.v)
			}
		}
	}

	// Non-canonical constants: Itoa(x) never equals them, so == is always
	// false and != always true — the check must NOT rewrite these.
	nonCanon := []string{"007", "+5", "-0", " 5", "5 ", "", "0x10", "1_000", "abc", "3000000000"}
	for _, s := range nonCanon {
		for _, x := range xs {
			if strconv.Itoa(x) == s {
				t.Fatalf("non-canonical %q unexpectedly equals Itoa(%d)=%q", s, x, strconv.Itoa(x))
			}
		}
	}
}
