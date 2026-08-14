package checks

// Runtime differential for PS2011: []byte(strings.Repeat(s, n)) vs
// bytes.Repeat([]byte(s), n). It pins BOTH halves of the check's stance:
//
//  1. the advisory's perf claim is sound — the two forms produce the same
//     byte content, length, and nil-ness on every input (empty seeds,
//     invalid UTF-8, NUL bytes, chunked long outputs);
//  2. the reason PS2011 is ADVISORY rather than an auto-fix — when the
//     result escapes to the heap, gc rounds the []byte(string)
//     conversion's CAPACITY up to an allocator size class while
//     bytes.Repeat clamps cap == len via a three-index slice. cap is
//     observable (directly, and through append aliasing), so no rewrite
//     is bit-identical. The divergence is asserted to still EXIST: if a
//     future toolchain reaches cap parity, the arm fails and PS2011's
//     advisory stance (and a possible auto-fix upgrade) must be
//     revisited. The panic-message divergence ("strings: ..." vs
//     "bytes: ...") is pinned the same way.

import (
	"bytes"
	"strings"
	"testing"
)

// Package-level sinks force the compared slices to escape to the heap —
// the escaping path is where the conversion's capacity rounding shows up.
var (
	ps2011SinkBefore []byte
	ps2011SinkAfter  []byte
)

func TestEquiv_PS2011_BytesRepeat(t *testing.T) {
	seeds := []string{
		"",                      // empty seed: output empty for every count
		"a",                     // single byte
		"ab",                    // the documented example
		"abc",                   // length not a power of two
		"\x00",                  // NUL byte
		"héllo",                 // multi-byte UTF-8
		"日本語",                   // 3-byte runes
		"\xff\xfe\x80",          // invalid UTF-8 — both forms are byte-level
		"x\x00y\xffz",           // mixed binary
		strings.Repeat("q", 63), // long seed
		strings.Repeat("r", 1000),
	}
	counts := []int{0, 1, 2, 3, 7, 16, 64, 129}
	for _, s := range seeds {
		for _, n := range counts {
			ps2011SinkBefore = []byte(strings.Repeat(s, n))
			ps2011SinkAfter = bytes.Repeat([]byte(s), n)
			before, after := ps2011SinkBefore, ps2011SinkAfter
			if !bytes.Equal(before, after) {
				t.Fatalf("byte content diverges for len(s)=%d n=%d:\nbefore=%q\nafter=%q", len(s), n, before, after)
			}
			if len(before) != len(after) {
				t.Errorf("len diverges for len(s)=%d n=%d: %d vs %d", len(s), n, len(before), len(after))
			}
			if (before == nil) != (after == nil) {
				t.Errorf("nil-ness diverges for len(s)=%d n=%d: before==nil %v, after==nil %v", len(s), n, before == nil, after == nil)
			}
			// bytes.Repeat documents nothing about cap, but its clamped
			// cap == len result is what makes the rewrite observable —
			// keep the premise pinned.
			if n > 0 && len(s) > 0 && cap(after) != len(after) {
				t.Errorf("bytes.Repeat no longer clamps cap==len (len=%d cap=%d) — re-examine PS2011's capacity reasoning", len(after), cap(after))
			}
		}
	}
}

// TestEquiv_PS2011_CapDivergenceStillHolds pins the divergence that keeps
// PS2011 advisory: a heap-escaping []byte(string) conversion of a
// non-size-class length gets a rounded-up capacity, bytes.Repeat an exact
// one. If cap parity is ever reached, this fails — signalling that the
// advisory stance should be revisited (a bit-identical auto-fix may have
// become possible).
func TestEquiv_PS2011_CapDivergenceStillHolds(t *testing.T) {
	// 9 bytes is not a gc allocator size class (8, 16, 24, ...): the
	// escaping conversion historically reports cap 16.
	ps2011SinkBefore = []byte(strings.Repeat("abc", 3))
	ps2011SinkAfter = bytes.Repeat([]byte("abc"), 3)
	if cap(ps2011SinkBefore) == cap(ps2011SinkAfter) {
		t.Errorf("cap parity reached (before=%d after=%d): the capacity divergence that keeps PS2011 advisory no longer holds — re-evaluate whether an auto-fix is now bit-identical", cap(ps2011SinkBefore), cap(ps2011SinkAfter))
	}
}

// TestEquiv_PS2011_PanicMessagesDiffer pins the second divergence: on a
// negative count the two forms panic with different messages, so a
// recovered and inspected panic string distinguishes them. If the stdlib
// ever unified the messages this test fails, removing one of the two
// obstacles to an auto-fix.
func TestEquiv_PS2011_PanicMessagesDiffer(t *testing.T) {
	recovered := func(f func()) (msg string) {
		defer func() {
			r := recover()
			if r == nil {
				t.Fatal("expected a panic on negative Repeat count")
			}
			s, ok := r.(string)
			if !ok {
				t.Fatalf("expected a string panic value, got %T", r)
			}
			msg = s
		}()
		f()
		return ""
	}
	sMsg := recovered(func() { ps2011SinkBefore = []byte(strings.Repeat("a", -1)) })
	bMsg := recovered(func() { ps2011SinkAfter = bytes.Repeat([]byte("a"), -1) })
	if !strings.HasPrefix(sMsg, "strings:") || !strings.HasPrefix(bMsg, "bytes:") {
		t.Fatalf("unexpected panic messages: %q vs %q", sMsg, bMsg)
	}
	if sMsg == bMsg {
		t.Fatalf("panic messages no longer differ (%q) — one PS2011 auto-fix obstacle is gone", sMsg)
	}
}
