package checks

// Runtime differential for PS2047: append(dst, strconv.Format*(...)...) vs
// the strconv.Append*(dst, ...) form the fix emits, for every verb in the
// shared PS2136 table. The safety claim is that both spellings append
// byte-for-byte the same output — strconv documents each Format*/Quote*/
// Itoa result as the string of its Append* twin's bytes — over every
// value, base, float verb (including unrecognized verbs), precision,
// bitSize, NaN/±Inf/-0.0, denormals, bool, and invalid-UTF-8 quote input,
// and that dst is treated the same way. This suite pins:
//
//   - byte and length identity per verb over adversarial values (the
//     small-int cache boundary, extremes incl. the MinInt64 negation
//     wrap, every legal base class, denormals, -0.0, NaN, ±Inf, an
//     UNRECOGNIZED float verb, invalid UTF-8, surrogates and
//     out-of-range quote runes) crossed with adversarial destinations
//     (nil, non-nil empty, seeded, spare capacity, tight capacity that
//     forces growth);
//   - nil-ness: every matched formatter renders at least one byte, so a
//     nil dst becomes non-nil in BOTH spellings — there is no
//     nil-vs-empty corner;
//   - the in-place path: with spare capacity for the WHOLE output both
//     spellings write the SAME backing array and preserve its capacity
//     exactly;
//   - panic parity: an illegal integer base and an illegal float
//     bitSize panic with the IDENTICAL strconv panic value in both
//     spellings;
//   - the perf premise: the After side never allocates when dst has
//     room, while the Before side allocates its intermediate string.
//
// On the GROWTH path only bytes and length are pinned: the fresh
// slice's capacity is an unspecified implementation detail (the
// sign/float/quote formatters append into dst piecewise, so the growth
// steps can differ from the spread's single grow-and-copy) — the
// accepted PS2112 class that the dst-position rewrites PS2035/PS2036/
// PS5015/PS2044 already carry.

import (
	"bytes"
	"fmt"
	"math"
	"strconv"
	"strings"
	"testing"
)

// ps2047Case is one Before/After pair closed over the formatter
// arguments, so every verb runs through the same destination matrix.
type ps2047Case struct {
	name   string
	before func(dst []byte) []byte
	after  func(dst []byte) []byte
}

func ps2047Cases() []ps2047Case {
	var cases []ps2047Case

	ints := []int64{0, 1, -1, 9, 10, 99, 100, -99, -100, 101, 12345, -12345,
		1<<40 + 7, -(1<<40 + 7), math.MaxInt64, math.MinInt64, math.MinInt64 + 1}
	bases := []int{2, 3, 8, 10, 16, 36} // the full legal range is 2..36
	for _, n := range ints {
		n := n
		if int64(int(n)) == n {
			i := int(n)
			cases = append(cases, ps2047Case{
				name:   fmt.Sprintf("Itoa(%d)", i),
				before: func(dst []byte) []byte { return append(dst, strconv.Itoa(i)...) },
				after:  func(dst []byte) []byte { return strconv.AppendInt(dst, int64(i), 10) },
			})
		}
		for _, b := range bases {
			b := b
			cases = append(cases, ps2047Case{
				name:   fmt.Sprintf("FormatInt(%d,%d)", n, b),
				before: func(dst []byte) []byte { return append(dst, strconv.FormatInt(n, b)...) },
				after:  func(dst []byte) []byte { return strconv.AppendInt(dst, n, b) },
			})
		}
	}

	uints := []uint64{0, 1, 99, 100, 101, 4096, 1<<40 + 7, math.MaxUint64}
	for _, u := range uints {
		u := u
		for _, b := range bases {
			b := b
			cases = append(cases, ps2047Case{
				name:   fmt.Sprintf("FormatUint(%d,%d)", u, b),
				before: func(dst []byte) []byte { return append(dst, strconv.FormatUint(u, b)...) },
				after:  func(dst []byte) []byte { return strconv.AppendUint(dst, u, b) },
			})
		}
	}

	floats := []float64{0, math.Copysign(0, -1), 1, -1.5, 0.1, 1.0 / 3.0,
		math.NaN(), math.Inf(1), math.Inf(-1), 5e-324, math.SmallestNonzeroFloat32,
		math.MaxFloat64, math.MaxFloat32, 1e300, -2.7182818284590455, 123456.789}
	fmts := []byte{'b', 'e', 'E', 'f', 'g', 'G', 'x', 'X', 'q'} // 'q' is UNRECOGNIZED: both emit "%q"
	precs := []int{-1, 0, 1, 6, 40}
	for _, f := range floats {
		f := f
		for _, verb := range fmts {
			verb := verb
			for _, prec := range precs {
				prec := prec
				if verb == 'b' && prec != -1 {
					continue // 'b' ignores precision; keep the matrix small
				}
				for _, bs := range []int{32, 64} {
					bs := bs
					cases = append(cases, ps2047Case{
						name: fmt.Sprintf("FormatFloat(%g,%q,%d,%d)", f, verb, prec, bs),
						before: func(dst []byte) []byte {
							return append(dst, strconv.FormatFloat(f, verb, prec, bs)...)
						},
						after: func(dst []byte) []byte {
							return strconv.AppendFloat(dst, f, verb, prec, bs)
						},
					})
				}
			}
		}
	}

	for _, v := range []bool{true, false} {
		v := v
		cases = append(cases, ps2047Case{
			name:   fmt.Sprintf("FormatBool(%v)", v),
			before: func(dst []byte) []byte { return append(dst, strconv.FormatBool(v)...) },
			after:  func(dst []byte) []byte { return strconv.AppendBool(dst, v) },
		})
	}

	quoteStrings := []string{"", "hello", `he said "hi"`, "\x00\x01\x02", "\xff\xfe invalid",
		"日本語", "mixed \xc3\x28 bad", "\u202e bidi", "tab\tnl\n", "\U0001F600",
		strings.Repeat("long \" line \\ with \x80 junk — ", 8)}
	for _, s := range quoteStrings {
		s := s
		cases = append(cases,
			ps2047Case{
				name:   fmt.Sprintf("Quote(%q)", s),
				before: func(dst []byte) []byte { return append(dst, strconv.Quote(s)...) },
				after:  func(dst []byte) []byte { return strconv.AppendQuote(dst, s) },
			},
			ps2047Case{
				name:   fmt.Sprintf("QuoteToASCII(%q)", s),
				before: func(dst []byte) []byte { return append(dst, strconv.QuoteToASCII(s)...) },
				after:  func(dst []byte) []byte { return strconv.AppendQuoteToASCII(dst, s) },
			},
			ps2047Case{
				name:   fmt.Sprintf("QuoteToGraphic(%q)", s),
				before: func(dst []byte) []byte { return append(dst, strconv.QuoteToGraphic(s)...) },
				after:  func(dst []byte) []byte { return strconv.AppendQuoteToGraphic(dst, s) },
			},
		)
	}

	quoteRunes := []rune{'a', '"', '\'', '\\', '\n', 0, 0x7F, 0xE9, 0xFFFD, 0xD800, 0xDFFF,
		0x10FFFF, 0x110000, -1, math.MinInt32, math.MaxInt32}
	for _, r := range quoteRunes {
		r := r
		cases = append(cases,
			ps2047Case{
				name:   fmt.Sprintf("QuoteRune(%#x)", r),
				before: func(dst []byte) []byte { return append(dst, strconv.QuoteRune(r)...) },
				after:  func(dst []byte) []byte { return strconv.AppendQuoteRune(dst, r) },
			},
			ps2047Case{
				name:   fmt.Sprintf("QuoteRuneToASCII(%#x)", r),
				before: func(dst []byte) []byte { return append(dst, strconv.QuoteRuneToASCII(r)...) },
				after:  func(dst []byte) []byte { return strconv.AppendQuoteRuneToASCII(dst, r) },
			},
			ps2047Case{
				name:   fmt.Sprintf("QuoteRuneToGraphic(%#x)", r),
				before: func(dst []byte) []byte { return append(dst, strconv.QuoteRuneToGraphic(r)...) },
				after:  func(dst []byte) []byte { return strconv.AppendQuoteRuneToGraphic(dst, r) },
			},
		)
	}

	return cases
}

func TestEquiv_PS2047StrconvAppendSpread(t *testing.T) {
	seeds := [][]byte{nil, {}, []byte("prefix:"), []byte("héllo \xff\x80 日本語")}
	for _, c := range ps2047Cases() {
		// Byte and length identity over adversarial destinations,
		// including a tight capacity that forces growth mid-format.
		for _, seed := range seeds {
			for _, capExtra := range []int{0, 1, 2, 64} {
				dstB := append(make([]byte, 0, len(seed)+capExtra), seed...)
				dstA := append(make([]byte, 0, len(seed)+capExtra), seed...)
				b := c.before(dstB)
				a := c.after(dstA)
				if !bytes.Equal(b, a) || len(b) != len(a) {
					t.Fatalf("%s seed %q cap+%d: before %q (len %d) != after %q (len %d)",
						c.name, seed, capExtra, b, len(b), a, len(a))
				}
				// Nil-ness: every formatter renders >= 1 byte, so even from
				// a nil dst both sides are non-nil.
				if (b == nil) != (a == nil) || a == nil {
					t.Fatalf("%s seed %q: nil-ness diverged or empty output (before nil=%v, after nil=%v)",
						c.name, seed, b == nil, a == nil)
				}
			}
		}

		// In-place path: with spare capacity for the WHOLE output both
		// spellings write the SAME backing array and preserve its
		// capacity exactly.
		need := len(c.after(nil))
		back1 := make([]byte, 3, 3+need+8)
		back2 := make([]byte, 3, 3+need+8)
		copy(back1, "abc")
		copy(back2, "abc")
		b := c.before(back1)
		a := c.after(back2)
		if cap(b) != cap(back1) || cap(a) != cap(back2) || &b[0] != &back1[0] || &a[0] != &back2[0] {
			t.Fatalf("%s: in-place append diverged (capB=%d capA=%d, want %d/%d)",
				c.name, cap(b), cap(a), cap(back1), cap(back2))
		}
		if !bytes.Equal(b, a) {
			t.Fatalf("%s: in-place bytes diverged: %q != %q", c.name, b, a)
		}
	}
}

// TestEquiv_PS2047PanicParity pins that an illegal integer base and an
// illegal float bitSize panic with the IDENTICAL strconv panic value in
// both spellings (the shared formatter validates before formatting).
func TestEquiv_PS2047PanicParity(t *testing.T) {
	recovered := func(f func()) (v any) {
		defer func() { v = recover() }()
		f()
		return nil
	}
	dst := make([]byte, 0, 64)
	for _, base := range []int{-1, 0, 1, 37, 62, 100} {
		base := base
		for _, n := range []int64{12345, -12345} {
			n := n
			b := recovered(func() { _ = append(dst, strconv.FormatInt(n, base)...) })
			a := recovered(func() { _ = strconv.AppendInt(dst, n, base) })
			if b == nil || a == nil || b != a {
				t.Fatalf("FormatInt/AppendInt(%d, base %d): panic values diverged: %v vs %v", n, base, b, a)
			}
		}
		u := recovered(func() { _ = append(dst, strconv.FormatUint(7, base)...) })
		ua := recovered(func() { _ = strconv.AppendUint(dst, 7, base) })
		if u == nil || ua == nil || u != ua {
			t.Fatalf("FormatUint/AppendUint(base %d): panic values diverged: %v vs %v", base, u, ua)
		}
	}
	for _, bs := range []int{0, 16, 33, 128} {
		bs := bs
		b := recovered(func() { _ = append(dst, strconv.FormatFloat(1.5, 'g', -1, bs)...) })
		a := recovered(func() { _ = strconv.AppendFloat(dst, 1.5, 'g', -1, bs) })
		if b == nil || a == nil || b != a {
			t.Fatalf("FormatFloat/AppendFloat(bitSize %d): panic values diverged: %v vs %v", bs, b, a)
		}
	}
}

// TestEquiv_PS2047NoAllocs pins the perf premise: with room in dst the
// After side never allocates (the formatter writes straight into dst's
// backing array), while the Before side allocates its intermediate
// string for any value outside strconv's tiny small-int cache.
func TestEquiv_PS2047NoAllocs(t *testing.T) {
	dst := make([]byte, 0, 128)
	var sink []byte
	after := func() {
		sink = strconv.AppendInt(dst[:0], 1<<40+7, 10)
		sink = strconv.AppendUint(sink, 1<<40+9, 16)
		sink = strconv.AppendBool(sink, true)
		sink = strconv.AppendQuote(sink, "hé\"llo")
	}
	if n := testing.AllocsPerRun(200, after); n != 0 {
		t.Errorf("strconv.Append* quartet into spare capacity allocates: %v allocs/op", n)
	}
	before := func() { sink = append(dst[:0], strconv.FormatInt(1<<40+7, 10)...) }
	if n := testing.AllocsPerRun(200, before); n == 0 {
		t.Logf("note: the Before spelling no longer allocates its intermediate string — PS2047's win premise may need re-measuring")
	}
	_ = sink
}
