package checks

// PS2019's runtime differential: strings.<pred>(string(b), string(sub)) ->
// bytes.<pred>(b, sub) for the read-only members whose bytes twin has the
// identical shape — the exact mirror of PS3004, run backwards. The fix is
// bit-identical only if every listed predicate returns the same bool/int
// over the same bytes — across nil and empty slices, empty separators,
// invalid UTF-8, NUL bytes, Unicode case-folding pairs (EqualFold's Kelvin
// sign, dotless i, final sigma and ligatures) and needles at every
// position. string(b) round-trips b's bytes byte-exactly (a nil slice
// converts to ""), so the operands seen by both sides are identical; this
// pins that the RESULTS are too, against the REAL stdlib so a future
// stdlib divergence fails CI.
//
// None of the matched members invokes user code, so within one call
// nothing can mutate the slice between its evaluation and the scan in a
// race-free program — reading the live slice instead of a snapshot is
// unobservable. The one shape that COULD observe it (a callback member
// like Map or IndexFunc) is deliberately absent from ps2019Members.

import (
	"bytes"
	"strings"
	"testing"
)

func TestEquiv_PS2019StringsPredToBytes(t *testing.T) {
	inputs := [][]byte{
		nil, {}, []byte("a"), []byte("abc"), []byte("aaa"),
		[]byte("héllo\xffworld"), []byte("\xff"), []byte("\xff\xfe"),
		[]byte("ABC"), []byte("ﬀ"), []byte("ﬀ IS ff folded"),
		[]byte("κόσμε"), []byte("K"), []byte("k"), []byte("K"),
		[]byte("İstanbul"), []byte("ςσΣ"), []byte("a\x00b"), []byte("\x00"),
		[]byte("prefix-mid-suffix"),
		[]byte(strings.Repeat("xy", 100) + "z"),
	}
	subs := [][]byte{
		nil, {}, []byte("a"), []byte("abc"), []byte("aa"), []byte("\xff"),
		[]byte("héllo"), []byte("world"), []byte("b"), []byte("K"),
		[]byte("k"), []byte("K"), []byte("ﬀ"), []byte("FF"), []byte("ff"),
		[]byte("ΚΌΣΜΕ"), []byte("İSTANBUL"), []byte("istanbul"),
		[]byte("σςς"), []byte("xyx"), []byte("z"), []byte("\x00"),
		[]byte("prefix"), []byte("suffix"), []byte("missing"),
		[]byte(strings.Repeat("xy", 50)),
	}
	for _, b := range inputs {
		for _, sub := range subs {
			s, ssub := string(b), string(sub)
			if got, want := bytes.Contains(b, sub), strings.Contains(s, ssub); got != want {
				t.Errorf("Contains(%q,%q): bytes=%v strings=%v", b, sub, got, want)
			}
			if got, want := bytes.ContainsAny(b, ssub), strings.ContainsAny(s, ssub); got != want {
				t.Errorf("ContainsAny(%q,%q): bytes=%v strings=%v", b, ssub, got, want)
			}
			if got, want := bytes.HasPrefix(b, sub), strings.HasPrefix(s, ssub); got != want {
				t.Errorf("HasPrefix(%q,%q): bytes=%v strings=%v", b, sub, got, want)
			}
			if got, want := bytes.HasSuffix(b, sub), strings.HasSuffix(s, ssub); got != want {
				t.Errorf("HasSuffix(%q,%q): bytes=%v strings=%v", b, sub, got, want)
			}
			if got, want := bytes.Index(b, sub), strings.Index(s, ssub); got != want {
				t.Errorf("Index(%q,%q): bytes=%d strings=%d", b, sub, got, want)
			}
			if got, want := bytes.IndexAny(b, ssub), strings.IndexAny(s, ssub); got != want {
				t.Errorf("IndexAny(%q,%q): bytes=%d strings=%d", b, ssub, got, want)
			}
			if got, want := bytes.LastIndex(b, sub), strings.LastIndex(s, ssub); got != want {
				t.Errorf("LastIndex(%q,%q): bytes=%d strings=%d", b, sub, got, want)
			}
			if got, want := bytes.Count(b, sub), strings.Count(s, ssub); got != want {
				t.Errorf("Count(%q,%q): bytes=%d strings=%d", b, sub, got, want)
			}
			if got, want := bytes.EqualFold(b, sub), strings.EqualFold(s, ssub); got != want {
				t.Errorf("EqualFold(%q,%q): bytes=%v strings=%v", b, sub, got, want)
			}
		}
	}
	// The empty-operand edges the doc leans on, pinned explicitly: both
	// sides agree that Index of an empty needle is 0 and that Count of an
	// empty separator is RuneCount+1 — including on a nil haystack.
	if bytes.Index(nil, nil) != 0 || strings.Index("", "") != 0 {
		t.Error("Index(empty, empty) != 0 on one side")
	}
	if bytes.Count([]byte("héllo"), nil) != strings.Count("héllo", "") {
		t.Error("Count(s, empty) diverges from RuneCount+1 parity")
	}
	// Single evaluation, original order: each operand conversion is
	// rewritten to its bare operand expression, still evaluated exactly
	// once, left to right, in both forms.
	var before, after []string
	fb := func(b []byte) []byte { before = append(before, "f"); return b }
	gb := func(b []byte) []byte { before = append(before, "g"); return b }
	_ = strings.Contains(string(fb([]byte("x"))), string(gb([]byte("x"))))
	fa := func(b []byte) []byte { after = append(after, "f"); return b }
	ga := func(b []byte) []byte { after = append(after, "g"); return b }
	_ = bytes.Contains(fa([]byte("x")), ga([]byte("x")))
	if len(before) != 2 || len(after) != 2 || before[0] != after[0] || before[1] != after[1] {
		t.Errorf("operand evaluation order/count differs: before=%v after=%v", before, after)
	}
}
