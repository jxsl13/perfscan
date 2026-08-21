package checks

// Runtime differential for PS2018: string(bytes.Repeat([]byte(s), n))
// vs strings.Repeat(s, n). The fix's safety argument is that both
// functions perform pure byte-level repetition — the seed's bytes laid
// down count times, no case folding, no normalization, no UTF-8
// validation — so the resulting string VALUE is byte-for-byte identical
// for every seed and every count, and that under the check's gate (a
// provably non-negative constant count) the panic paths coincide too.
// This suite pins:
//
//   - VALUE identity over adversarial seeds: empty, single byte, the
//     five seeds strings.Repeat special-cases via lookup tables
//     (' ', '-', '0', '=', '\t') at counts inside and beyond the table
//     lengths, multi-byte UTF-8, INVALID UTF-8 (bare continuation
//     bytes, truncated sequences, 0xFF), NUL bytes, and a randomized
//     long binary seed — crossed with counts 0..8 and counts large
//     enough to cross strings.Repeat's 8KB chunked-fill strategy;
//   - PANIC parity on the two panic paths: a negative count and an
//     int-overflowing len(s)*count panic in BOTH forms on exactly the
//     same inputs. The recovered messages differ only in the package
//     prefix ("bytes:" vs "strings:") — asserted here, because that
//     prefix is exactly why the fix requires a provably non-negative
//     constant count (removing the only panic path reachable with
//     realistic data) and why other counts stay advisory;
//   - the perf premise: the After performs exactly one allocation.

import (
	"bytes"
	"math/rand"
	"strings"
	"testing"
)

// ps2018Before is the exact Before-shape of the check.
func ps2018Before(s string, n int) string { return string(bytes.Repeat([]byte(s), n)) }

// ps2018After is the exact After-shape of the check.
func ps2018After(s string, n int) string { return strings.Repeat(s, n) }

func ps2018Check(t *testing.T, s string, n int) {
	t.Helper()
	before := ps2018Before(s, n)
	after := ps2018After(s, n)
	if before != after {
		t.Fatalf("Repeat divergence on seed %q count %d:\n before=%q\n after=%q", s, n, before, after)
	}
}

func TestEquiv_PS2018_AdversarialSeeds(t *testing.T) {
	seeds := []string{
		"", // empty seed: result is "" for every count in both forms
		"a", "ab", "abc",
		" ", "-", "0", "=", "\t", // strings.Repeat's lookup-table fast-path seeds
		"  ", "--=", "\t\t",
		"\x00", "a\x00b", // NUL bytes are plain bytes for both
		"héllo", "日本語", "🚀", // multi-byte UTF-8
		"\xff", "\xc3", "\x80", "a\xffb", // invalid UTF-8: pure byte repetition either way
		"\xc3(", // truncated 2-byte sequence
	}
	counts := []int{0, 1, 2, 3, 4, 7, 8, 63, 64, 100}
	for _, s := range seeds {
		for _, n := range counts {
			ps2018Check(t, s, n)
		}
	}
	// Beyond the fast-path tables (repeatedSpaces etc. are shorter than
	// 1024) and across the 8KB chunking limit of strings.Repeat.
	for _, s := range []string{" ", "-", "ab", "abc"} {
		for _, n := range []int{1024, 4096, 8192, 8193, 20000} {
			ps2018Check(t, s, n)
		}
	}
}

func TestEquiv_PS2018_RandomizedSeeds(t *testing.T) {
	rng := rand.New(rand.NewSource(0x2018))
	for range 500 {
		b := make([]byte, rng.Intn(33))
		for i := range b {
			b[i] = byte(rng.Intn(256)) // full byte range, valid or not
		}
		ps2018Check(t, string(b), rng.Intn(17))
	}
}

// ps2018Recover runs f and returns the recovered panic value as a
// string, or "" when f does not panic.
func ps2018Recover(f func()) (msg string) {
	defer func() {
		if r := recover(); r != nil {
			s, _ := r.(string)
			msg = s
		}
	}()
	f()
	return ""
}

// Both panic paths fire on exactly the same inputs in both forms; only
// the message's package prefix differs. The negative-count divergence
// is the reason the fix requires a provably NON-NEGATIVE CONSTANT count
// — that gate removes the only panic path reachable with realistic
// data. The overflow path survives the gate, but both forms panic on identical
// inputs (same condition, same lengths, same limit) and needs a
// petabyte-scale seed for any plausible constant count — the same
// both-panic residual PS3111 accepts for slices.MaxFunc -> slices.Max.
func TestEquiv_PS2018_PanicParity(t *testing.T) {
	// Negative count.
	beforeMsg := ps2018Recover(func() { _ = ps2018Before("x", -1) })
	afterMsg := ps2018Recover(func() { _ = ps2018After("x", -1) })
	if beforeMsg == "" || afterMsg == "" {
		t.Fatalf("negative count must panic in both forms (before=%q after=%q)", beforeMsg, afterMsg)
	}
	if beforeMsg != "bytes: negative Repeat count" || afterMsg != "strings: negative Repeat count" {
		t.Fatalf("negative-count messages drifted: before=%q after=%q", beforeMsg, afterMsg)
	}
	// Overflow: len(s)*count exceeds maxInt. The check happens BEFORE
	// any result allocation in both implementations, so a 1MB seed with
	// a huge count exercises the path cheaply.
	seed := strings.Repeat("a", 1<<20)
	huge := (int(^uint(0)>>1) / len(seed)) + 1
	beforeMsg = ps2018Recover(func() { _ = ps2018Before(seed, huge) })
	afterMsg = ps2018Recover(func() { _ = ps2018After(seed, huge) })
	if beforeMsg != "bytes: Repeat output length overflow" || afterMsg != "strings: Repeat output length overflow" {
		t.Fatalf("overflow messages drifted: before=%q after=%q", beforeMsg, afterMsg)
	}
}

// The perf premise: the After builds the result in a single allocation
// (one buffer, returned as the string zero-copy), where the Before pays
// the repeat buffer plus the string copy (plus the seed copy when it
// escapes).
func TestEquiv_PS2018_AfterSingleAllocation(t *testing.T) {
	s := "ab"
	var sink string
	allocs := testing.AllocsPerRun(100, func() {
		sink = strings.Repeat(s, 64)
	})
	_ = sink
	if allocs != 1 {
		t.Fatalf("strings.Repeat(s, 64) = %v allocs/op, want exactly 1", allocs)
	}
}
