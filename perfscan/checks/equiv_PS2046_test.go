package checks

// Runtime differential for PS2046: fmt.Appendf(buf, "%x", bs) vs
// hex.AppendEncode(buf, bs) — the advisory's suggested rewrite.
//
// Two claims are pinned here, and TOGETHER they are why the check is
// ADVISORY BY DESIGN (AutoFix:false):
//
//  1. IDENTITY whenever bs does not overlap buf's spare capacity: fmt's
//     bare lowercase %x over a []byte emits exactly two lowercase hex
//     digits per byte with no separators — the same bytes hex.Encode
//     produces (the identity PS2107's %x arm already relies on for
//     Sprintf -> hex.EncodeToString), carried to the append destination.
//     Pinned over all 256 single bytes, nil/empty sources (nil-ness of
//     the result asserted too — %x over an empty slice appends ZERO
//     bytes, so Appendf(nil, ...) stays nil on both sides, unlike
//     PS5015's always-emitting scalar verbs), randomized blobs, and the
//     adversarial destination shapes (nil, empty, prefixed, spare
//     capacity, tight capacity forcing growth).
//
//  2. DIVERGENCE when bs overlaps buf's SPARE CAPACITY (pure safe Go: a
//     reslice past len within cap). fmt.Appendf formats bs into a
//     separate pooled pp buffer and only then appends, so it always
//     reads bs's ORIGINAL bytes; hex.AppendEncode encodes forward
//     straight into buf[len(buf):] and overwrites source bytes before
//     reading them. No local type check can rule that shape out, so no
//     mechanical fix is offered. If this subtest ever FAILS (i.e. the
//     two sides agree), hex.AppendEncode has become overlap-safe and
//     PS2046 is a candidate for promotion to AutoFix.

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"math/rand"
	"testing"
)

// ps2046Buffers returns the adversarial destination shapes; each check
// clones them so both sides see an identical, unaliased buf.
func ps2046Buffers() [][]byte {
	return [][]byte{
		nil,
		{},
		[]byte("pre:"),
		make([]byte, 0, 64),
		append(make([]byte, 0, 4), 'x'), // tight capacity: forces growth
	}
}

// ps2046Compare asserts byte equality AND nil-ness equality of one
// before/after pair.
func ps2046Compare(t *testing.T, name string, before, after []byte) {
	t.Helper()
	if !bytes.Equal(before, after) {
		t.Fatalf("%s diverges: fmt.Appendf=%q hex.AppendEncode=%q", name, before, after)
	}
	if (before == nil) != (after == nil) {
		t.Fatalf("%s nil-ness diverges: fmt.Appendf nil=%v hex.AppendEncode nil=%v", name, before == nil, after == nil)
	}
}

// ps2046Check runs the exact before/after pair of the advisory's rewrite
// over every destination shape for one source.
func ps2046Check(t *testing.T, name string, bs []byte) {
	t.Helper()
	for _, b := range ps2046Buffers() {
		ps2046Compare(t, name,
			fmt.Appendf(bytes.Clone(b), "%x", bs),
			hex.AppendEncode(bytes.Clone(b), bs))
	}
}

func TestEquiv_PS2046_Identity(t *testing.T) {
	// Empty sources: %x over zero bytes appends NOTHING, so the nil buf
	// stays nil on both sides (asserted by ps2046Compare).
	ps2046Check(t, "nil src", nil)
	ps2046Check(t, "empty src", []byte{})

	// All 256 single bytes: every hex digit pair, invalid UTF-8 included
	// (irrelevant to %x, pinned anyway).
	for i := 0; i < 256; i++ {
		ps2046Check(t, fmt.Sprintf("byte 0x%02x", i), []byte{byte(i)})
	}

	// Structured multi-byte sources.
	ps2046Check(t, "ascii", []byte("the quick brown fox"))
	ps2046Check(t, "zeros", make([]byte, 33))
	ps2046Check(t, "high bytes", bytes.Repeat([]byte{0xff, 0x00, 0xab}, 11))

	// Randomized blobs of randomized lengths (deterministic seed).
	rng := rand.New(rand.NewSource(0x2046))
	for i := 0; i < 500; i++ {
		bs := make([]byte, rng.Intn(64))
		rng.Read(bs)
		ps2046Check(t, fmt.Sprintf("rand#%d len=%d", i, len(bs)), bs)
	}
}

// TestEquiv_PS2046_SpareCapacityOverlapDiverges pins claim 2: the exact
// divergence input from the Doc. buf holds "abcd" with spare capacity, bs
// is buf resliced past len — {0xAB, 0xCD} living in that spare capacity.
// fmt.Appendf reads the original {0xAB, 0xCD} and appends "abcd" (the hex
// digits of AB CD); hex.AppendEncode writes its first digit pair 'a','b'
// into buf[4] and buf[5] — overwriting the SECOND source byte
// (buf[5] == 0xCD) with 'b' (0x62) before reading it, so it appends
// "ab" + hex(0x62) = "ab62". This is the input that forbids a mechanical
// fix.
func TestEquiv_PS2046_SpareCapacityOverlapDiverges(t *testing.T) {
	mk := func() (buf, bs []byte) {
		backing := make([]byte, 16)
		copy(backing, "abcd")
		backing[4], backing[5] = 0xAB, 0xCD
		buf = backing[:4]
		bs = buf[4:6] // pure safe Go: a reslice past len within cap
		return buf, bs
	}

	buf, bs := mk()
	before := fmt.Appendf(buf, "%x", bs)
	if string(before) != "abcdabcd" {
		t.Fatalf("fmt.Appendf over the spare-capacity source = %q, want %q (reads the original bytes)", before, "abcdabcd")
	}

	buf, bs = mk()
	after := hex.AppendEncode(buf, bs)
	if bytes.Equal(before, after) {
		t.Fatalf("hex.AppendEncode no longer diverges on the spare-capacity overlap (got %q on both sides) — it has become overlap-safe; PS2046 is now a candidate for promotion to AutoFix", after)
	}
	// Pin the current forward-encode clobber shape too, so a CHANGE in the
	// divergence (not just its disappearance) is also surfaced.
	if string(after) != "abcdab62" {
		t.Fatalf("hex.AppendEncode over the spare-capacity source = %q, want the forward-encode clobber %q", after, "abcdab62")
	}
}

// TestEquiv_PS2046_LiveRegionAliasIsSafe documents the boundary of the
// hazard: bs aliasing buf's LIVE region (bs = buf itself) is fine — the
// encode destination buf[len(buf):] is disjoint from the source bytes
// buf[:len(buf)], so both sides agree. Only the spare-capacity overlap
// diverges.
func TestEquiv_PS2046_LiveRegionAliasIsSafe(t *testing.T) {
	mk := func() []byte {
		b := make([]byte, 4, 32)
		copy(b, []byte{0xAB, 0xCD, 0x01, 0x02})
		return b
	}
	b1 := mk()
	b2 := mk()
	ps2046Compare(t, "bs==buf", fmt.Appendf(b1, "%x", b1), hex.AppendEncode(b2, b2))
}
