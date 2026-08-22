package checks

// Runtime differential for PS3025: fmt.Appendf(buf, lit) vs
// append(buf, lit...) for a string literal with no '%' byte. The fix's
// safety argument is fmt's own implementation: with no verbs doPrintf
// copies every byte of the format verbatim into the pooled pp buffer,
// and fmt.Appendf's final act is the single `b = append(b, p.buf...)` —
// so the whole call IS one append of exactly the literal's bytes, the
// same single append the rewrite performs directly. There is no operand
// at all, hence no named-type/String()/Formatter, nil-operand or
// evaluation-order concern. This suite pins:
//
//   - byte-identity of fmt.Appendf(buf, s) == append(buf, s...) over
//     adversarial verb-free strings (empty, NUL bytes, invalid UTF-8,
//     multi-byte UTF-8, control bytes, long strings) and adversarial
//     destinations (nil, empty non-nil, spare capacity, full);
//   - LENGTH and CAPACITY identity: both spellings are one append of
//     the same byte count onto identically-shaped buffers, so even the
//     growth step matches;
//   - nil-ness identity for the empty-append edge (a nil buf stays nil
//     through both spellings);
//   - a randomized fuzz over %-free payloads and buffer shapes with a
//     fixed seed;
//   - the divergence witnesses that motivate the no-'%' guard: %%
//     collapses, a lone verb prints %!d(MISSING), a trailing '%' prints
//     %!(NOVERB) — each shown to CHANGE the appended bytes.

import (
	"bytes"
	"fmt"
	"math/rand"
	"strings"
	"testing"
)

// ps3025Appendf is fmt.Appendf through a function value — the exact
// runtime behavior of the Before-shape, deliberately called below with
// NON-constant formats to pin fmt's divergent outputs. The indirection
// keeps go vet's printf checker from (correctly) rejecting what is
// intentionally adversarial test input.
var ps3025Appendf func(b []byte, format string, a ...any) []byte = fmt.Appendf

// ps3025Payloads are the adversarial verb-free strings.
var ps3025Payloads = []string{
	"",
	"x",
	"literal text",
	"HTTP/1.1 200 OK\r\n",
	"tab\there and newline\nthere",
	"NUL \x00 embedded and trailing \x00",
	"\xff",             // invalid UTF-8: lone continuation-range byte
	"\xc0\x80",         // invalid UTF-8: overlong encoding of NUL
	"\xe4\xb8",         // invalid UTF-8: truncated multi-byte sequence
	"世界 🚀 déjà vu",     // multi-byte UTF-8
	"\x01\x02\x1b[31m", // control bytes and an ANSI escape
	strings.Repeat("long-", 10_000),
	strings.Repeat("\x00\xff", 4096),
}

// ps3025Bufs builds a fresh pair of IDENTICALLY-shaped destination
// buffers for each shape: nil, empty non-nil, len>0 with spare cap,
// len==cap (forcing growth in both spellings).
func ps3025Bufs() [][2][]byte {
	prefix := []byte("prefix\x00\xfe")
	shapes := [][2][]byte{
		{nil, nil},
		{make([]byte, 0), make([]byte, 0)},
		{make([]byte, 0, 64), make([]byte, 0, 64)},
	}
	withPrefix := func(length, capacity int) [2][]byte {
		a := make([]byte, length, capacity)
		b := make([]byte, length, capacity)
		copy(a, prefix)
		copy(b, prefix)
		return [2][]byte{a, b}
	}
	shapes = append(shapes,
		withPrefix(len(prefix), 256),         // spare capacity
		withPrefix(len(prefix), len(prefix)), // full: both must grow
	)
	return shapes
}

func TestEquiv_PS3025_VerbFreeIdentity(t *testing.T) {
	for _, s := range ps3025Payloads {
		for _, pair := range ps3025Bufs() {
			before := ps3025Appendf(pair[0], s)
			after := append(pair[1], s...)
			if !bytes.Equal(before, after) {
				t.Fatalf("bytes diverge for payload %q: Appendf=%q append=%q", s, before, after)
			}
			if len(before) != len(after) || cap(before) != cap(after) {
				t.Fatalf("len/cap diverge for payload %q on buf(len=%d,cap=%d): Appendf len=%d cap=%d, append len=%d cap=%d",
					s, len(pair[0]), cap(pair[0]), len(before), cap(before), len(after), cap(after))
			}
			if (before == nil) != (after == nil) {
				t.Fatalf("nil-ness diverges for payload %q: Appendf nil=%v append nil=%v", s, before == nil, after == nil)
			}
		}
	}
}

// TestEquiv_PS3025_RandomizedVerbFree fuzzes %-free payloads and buffer
// shapes with a fixed seed: any byte value except '%', payloads up to
// 1 KiB, buffers up to 128 bytes long with up to 128 bytes of spare cap.
func TestEquiv_PS3025_RandomizedVerbFree(t *testing.T) {
	rng := rand.New(rand.NewSource(0x3025))
	for range 5000 {
		payload := make([]byte, rng.Intn(1024))
		for i := range payload {
			for {
				c := byte(rng.Intn(256))
				if c != '%' {
					payload[i] = c
					break
				}
			}
		}
		s := string(payload)
		length := rng.Intn(128)
		capacity := length + rng.Intn(128)
		bufA := make([]byte, length, capacity)
		bufB := make([]byte, length, capacity)
		for i := range bufA {
			c := byte(rng.Intn(256))
			bufA[i], bufB[i] = c, c
		}
		before := ps3025Appendf(bufA, s)
		after := append(bufB, s...)
		if !bytes.Equal(before, after) || len(before) != len(after) || cap(before) != cap(after) {
			t.Fatalf("diverge for payload %q on buf(len=%d,cap=%d): Appendf (len=%d,cap=%d) vs append (len=%d,cap=%d)",
				s, length, capacity, len(before), cap(before), len(after), cap(after))
		}
	}
}

// TestEquiv_PS3025_DivergenceWitnesses pins the concrete inputs the
// no-'%' guard excludes — each one makes fmt.Appendf append something
// OTHER than the format's own bytes, so each is why the match is narrow.
func TestEquiv_PS3025_DivergenceWitnesses(t *testing.T) {
	for _, w := range []struct{ format, want string }{
		{"100%% done", "100% done"},
		{"%d", "%!d(MISSING)"},
		{"%v", "%!v(MISSING)"},
		{"50%", "50%!(NOVERB)"},
	} {
		got := string(ps3025Appendf(nil, w.format))
		if got == w.format {
			t.Errorf("fmt.Appendf(nil, %q) appended its format verbatim — the no-'%%' guard would be unnecessary", w.format)
		}
		if got != w.want {
			t.Errorf("fmt.Appendf(nil, %q) = %q, want %q (divergence witness drifted)", w.format, got, w.want)
		}
	}
}

// TestEquiv_PS3025_Allocs pins the perf premise on a preallocated
// destination: the builtin append into existing capacity allocates
// nothing; fmt.Appendf must not allocate MORE than append does after
// the rewrite (its pool traffic is amortized, so the assertion is only
// that the After-shape is allocation-free — the win is the removed pool
// round-trip, format scan and pp-buffer copy, pinned by the benchmark).
func TestEquiv_PS3025_Allocs(t *testing.T) {
	const lit = "HTTP/1.1 200 OK\r\n"
	buf := make([]byte, 0, 256)
	var sink []byte
	if avg := testing.AllocsPerRun(100, func() { sink = append(buf[:0], lit...) }); avg != 0 {
		t.Errorf("append into existing capacity allocates %v times per run, want 0", avg)
	}
	_ = sink
}
