package checks

// Runtime differential for PS2043: len(EncodeToString(b)) versus
// EncodedLen(len(b)) for encoding/hex, encoding/base64 and encoding/base32.
// The fix's safety argument: EncodeToString allocates its output via EXACTLY
// EncodedLen(len(src)) — the stdlib body is
// "buf := make([]byte, enc.EncodedLen(len(src))); enc.Encode(buf, src);
// return string(buf)" — and Encode always fills the whole buffer, so the two
// integers agree for EVERY input and EVERY encoder variant by construction.
// This suite pins:
//
//   - integer identity for hex and for every stock base64/base32 encoder plus
//     custom-alphabet, WithPadding and Strict derivatives, over adversarial
//     inputs: nil, empty, every length 0..70 (crossing every padding phase of
//     the 3-byte base64 and 5-byte base32 quanta), random contents with a
//     fixed seed, NUL-laden and 64 KiB inputs — contents cannot matter (only
//     the length enters either side), and the length sweep proves it;
//   - evaluation order and count parity for the receiver and argument
//     expressions the fix keeps verbatim (both may be calls);
//   - the divergence the embedding guard excludes, reproduced positively: a
//     wrapper type embedding *base64.Encoding whose OWN EncodedLen shadows
//     the promoted one — the rewrite would change callees there;
//   - the perf premise: the After shape is allocation-free while the Before
//     shape allocates and encodes the whole string.

import (
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"math/rand"
	"strings"
	"testing"
)

func ps2043Inputs() [][]byte {
	inputs := [][]byte{
		nil,
		{},
		[]byte{0},
		[]byte("\x00\x00\x00"),
		[]byte("hello, world"),
		[]byte(strings.Repeat("\xff", 61)),
		make([]byte, 64<<10),
	}
	// Every length 0..70 crosses every padding phase of base64's 3-byte and
	// base32's 5-byte quanta several times over.
	rng := rand.New(rand.NewSource(0x2043))
	for n := 0; n <= 70; n++ {
		b := make([]byte, n)
		rng.Read(b)
		inputs = append(inputs, b)
	}
	// Random lengths and contents with a fixed seed.
	for range 200 {
		b := make([]byte, rng.Intn(4096))
		rng.Read(b)
		inputs = append(inputs, b)
	}
	return inputs
}

func TestEquiv_PS2043_IntegerIdentity(t *testing.T) {
	b64Custom := base64.NewEncoding("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/")
	b32Custom := base32.NewEncoding("ABCDEFGHIJKLMNOPQRSTUVWXYZ234567")
	b64s := map[string]*base64.Encoding{
		"StdEncoding":          base64.StdEncoding,
		"URLEncoding":          base64.URLEncoding,
		"RawStdEncoding":       base64.RawStdEncoding,
		"RawURLEncoding":       base64.RawURLEncoding,
		"custom":               b64Custom,
		"custom-no-padding":    b64Custom.WithPadding(base64.NoPadding),
		"custom-star-padding":  b64Custom.WithPadding('*'),
		"std-strict":           base64.StdEncoding.Strict(),
		"raw-derived-repadded": base64.RawStdEncoding.WithPadding('!'),
	}
	b32s := map[string]*base32.Encoding{
		"StdEncoding":         base32.StdEncoding,
		"HexEncoding":         base32.HexEncoding,
		"custom":              b32Custom,
		"custom-no-padding":   b32Custom.WithPadding(base32.NoPadding),
		"custom-star-padding": b32Custom.WithPadding('*'),
	}
	for _, b := range ps2043Inputs() {
		if before, after := len(hex.EncodeToString(b)), hex.EncodedLen(len(b)); before != after {
			t.Fatalf("hex diverges for len %d: len(EncodeToString)=%d EncodedLen=%d", len(b), before, after)
		}
		for name, enc := range b64s {
			if before, after := len(enc.EncodeToString(b)), enc.EncodedLen(len(b)); before != after {
				t.Fatalf("base64 %s diverges for len %d: len(EncodeToString)=%d EncodedLen=%d", name, len(b), before, after)
			}
		}
		for name, enc := range b32s {
			if before, after := len(enc.EncodeToString(b)), enc.EncodedLen(len(b)); before != after {
				t.Fatalf("base32 %s diverges for len %d: len(EncodeToString)=%d EncodedLen=%d", name, len(b), before, after)
			}
		}
	}
}

// TestEquiv_PS2043_EvaluationOrder pins that both forms evaluate the receiver
// expression, then the argument expression, exactly once each and in that
// order — the fix keeps both byte-verbatim in place.
func TestEquiv_PS2043_EvaluationOrder(t *testing.T) {
	var trace []string
	encFn := func() *base64.Encoding { trace = append(trace, "enc"); return base64.StdEncoding }
	argFn := func() []byte { trace = append(trace, "arg"); return []byte("abcd") }

	trace = nil
	before := len(encFn().EncodeToString(argFn()))
	beforeTrace := strings.Join(trace, ",")

	trace = nil
	after := encFn().EncodedLen(len(argFn()))
	afterTrace := strings.Join(trace, ",")

	if beforeTrace != afterTrace {
		t.Fatalf("evaluation order diverges: before=%s after=%s", beforeTrace, afterTrace)
	}
	if beforeTrace != "enc,arg" {
		t.Fatalf("unexpected evaluation trace %q, want enc,arg", beforeTrace)
	}
	if before != after {
		t.Fatalf("results diverge: before=%d after=%d", before, after)
	}
}

// ps2043Wrapper is the divergence witness for the embedding guard: it embeds
// *base64.Encoding (so EncodeToString is promoted) but declares its OWN
// EncodedLen, which the rewrite would silently call instead of the promoted
// stdlib one.
type ps2043Wrapper struct{ *base64.Encoding }

func (ps2043Wrapper) EncodedLen(n int) int { return -1 }

func TestEquiv_PS2043_EmbeddingGuardWitness(t *testing.T) {
	w := ps2043Wrapper{base64.StdEncoding}
	b := []byte("abcd")
	before := len(w.EncodeToString(b)) // the promoted stdlib method: 8
	after := w.EncodedLen(len(b))      // the wrapper's own method: -1
	if before != 8 {
		t.Fatalf("promoted EncodeToString length = %d, want 8", before)
	}
	if after != -1 {
		t.Fatalf("wrapper EncodedLen = %d, want the shadowing -1", after)
	}
	if before == after {
		t.Fatal("expected the embedding shapes to diverge — the receiver-type guard would be unnecessary")
	}
}

// TestEquiv_PS2043_NilReceiverParity pins that a nil *Encoding receiver
// panics in BOTH forms (EncodeToString's first act is calling EncodedLen on
// the receiver), so the rewrite moves no panic across observable work.
func TestEquiv_PS2043_NilReceiverParity(t *testing.T) {
	panics := func(f func()) (p bool) {
		defer func() { p = recover() != nil }()
		f()
		return false
	}
	var enc *base64.Encoding
	if !panics(func() { _ = len(enc.EncodeToString([]byte("x"))) }) {
		t.Fatal("len(nil-enc.EncodeToString(b)) did not panic — re-audit the nil-receiver story")
	}
	if !panics(func() { _ = enc.EncodedLen(len([]byte("x"))) }) {
		t.Fatal("nil-enc.EncodedLen(len(b)) did not panic — re-audit the nil-receiver story")
	}
}

// TestEquiv_PS2043_AfterAllocProfile pins the perf premise: the After shape is
// allocation-free while the Before shape allocates (the encode buffer and its
// string conversion). The operands live at package scope so the compiler
// cannot prove anything away.
var (
	ps2043AllocIn  = make([]byte, 1024)
	ps2043AllocOut int
)

func TestEquiv_PS2043_AfterAllocProfile(t *testing.T) {
	if avg := testing.AllocsPerRun(200, func() {
		ps2043AllocOut = hex.EncodedLen(len(ps2043AllocIn))
	}); avg != 0 {
		t.Errorf("hex.EncodedLen(len(b)) allocates %v times per run, want 0", avg)
	}
	if avg := testing.AllocsPerRun(200, func() {
		ps2043AllocOut = len(hex.EncodeToString(ps2043AllocIn))
	}); avg < 1 {
		t.Logf("len(hex.EncodeToString(b)) allocates %v times per run — the compiler learned to elide the encode; the win claim may need re-framing", avg)
	}
}
