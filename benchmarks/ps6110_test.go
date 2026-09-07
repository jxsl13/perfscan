package benchmarks

import (
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"unsafe"
)

const ps6110LabelBytes = 96

type ps6110NativeRecord struct {
	token uintptr
	label [ps6110LabelBytes]byte
}

func ps6110Records(count, distinct int) []ps6110NativeRecord {
	records := make([]ps6110NativeRecord, count)
	for i := range records {
		value := "native-label-" + string(rune('a'+i%distinct))
		copy(records[i].label[:], value)
		records[i].token = uintptr(i%distinct + 1)
	}
	return records
}

func ps6110NativeView(record *ps6110NativeRecord) string {
	n := 0
	for n < len(record.label) && record.label[n] != 0 {
		n++
	}
	return unsafe.String(&record.label[0], n)
}

//go:noinline
func ps6110Before(records []ps6110NativeRecord) []string {
	result := make([]string, len(records))
	for i := range records {
		result[i] = strings.Clone(ps6110NativeView(&records[i]))
	}
	return result
}

const ps6110InlineLabels = 16

type ps6110OwnedLabels struct {
	inline   [ps6110InlineLabels]string
	count    int
	overflow map[string]string
}

func (labels *ps6110OwnedLabels) own(record *ps6110NativeRecord) string {
	return labels.ownView(ps6110NativeView(record))
}

func (labels *ps6110OwnedLabels) ownN(record *ps6110NativeRecord, length int) string {
	return labels.ownView(unsafe.String(&record.label[0], length))
}

func (labels *ps6110OwnedLabels) ownView(view string) string {
	for i := range labels.count {
		if view == labels.inline[i] {
			return labels.inline[i]
		}
	}
	if owned, ok := labels.overflow[view]; ok {
		return owned
	}
	owned := strings.Clone(view)
	if labels.count < len(labels.inline) {
		labels.inline[labels.count] = owned
		labels.count++
		return owned
	}
	if labels.overflow == nil {
		labels.overflow = make(map[string]string)
	}
	labels.overflow[owned] = owned
	return owned
}

// ps6110TokenOwnedLabels is a separate correctness experiment for an optional
// producer identity. PS6110's primary remedy and benchmarks do not depend on
// this unconfigured token.
type ps6110TokenOwnedLabels struct {
	byToken map[uintptr][]string
	byText  map[string]string
}

func (labels *ps6110TokenOwnedLabels) own(record *ps6110NativeRecord) string {
	view := ps6110NativeView(record)
	return labels.ownView(record.token, view)
}

func (labels *ps6110TokenOwnedLabels) ownView(token uintptr, view string) string {
	for _, owned := range labels.byToken[token] {
		if view == owned {
			return owned
		}
	}
	if owned, ok := labels.byText[view]; ok {
		labels.byToken[token] = append(labels.byToken[token], owned)
		return owned
	}
	owned := strings.Clone(view)
	if labels.byToken == nil {
		labels.byToken = make(map[uintptr][]string)
		labels.byToken[token] = []string{owned}
		labels.byText = map[string]string{owned: owned}
		return owned
	}
	labels.byToken[token] = append(labels.byToken[token], owned)
	labels.byText[owned] = owned
	return owned
}

//go:noinline
func ps6110After(records []ps6110NativeRecord) []string {
	if len(records) == 0 {
		return []string{}
	}
	if len(records) == 1 {
		return []string{strings.Clone(ps6110NativeView(&records[0]))}
	}
	result := make([]string, len(records))
	var labels ps6110OwnedLabels
	for i := range records {
		result[i] = labels.own(&records[i])
	}
	return result
}

func ps6110TokenAfter(records []ps6110NativeRecord) []string {
	result := make([]string, len(records))
	var labels ps6110TokenOwnedLabels
	for i := range records {
		result[i] = labels.own(&records[i])
	}
	return result
}

func TestPS6110OwnershipAndCollisions(t *testing.T) {
	t.Parallel()
	records := ps6110Records(6, 2)
	// Same token, different content must never be treated as equality.
	records[2].token = records[0].token
	clear(records[2].label[:])
	copy(records[2].label[:], "collision")
	// Different token, same content may share the owned immutable string.
	records[3].token = 99
	copy(records[3].label[:], records[1].label[:])
	want := ps6110Before(records)
	got := ps6110TokenAfter(records)
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("label %d = %q, want %q", i, got[i], want[i])
		}
	}
	for i := range records {
		clear(records[i].label[:])
	}
	for range 4 {
		churn := make([]byte, 1<<20)
		runtime.GC()
		runtime.KeepAlive(churn)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("owned label %d after native churn = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestPS6110PrimaryOwnershipInlineAndOverflow(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name     string
		count    int
		distinct int
	}{
		{name: "inline", count: 340, distinct: 10},
		{name: "overflow-mixed", count: 340, distinct: 170},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			records := ps6110Records(test.count, test.distinct)
			want := ps6110Before(records)
			got := ps6110After(records)
			for i := range want {
				if got[i] != want[i] {
					t.Fatalf("label %d = %q, want %q", i, got[i], want[i])
				}
			}
			for i := range records {
				clear(records[i].label[:])
				copy(records[i].label[:], "changed-native-storage")
			}
			for range 4 {
				churn := make([]byte, 1<<20)
				runtime.GC()
				runtime.KeepAlive(churn)
			}
			for i := range want {
				if got[i] != want[i] {
					t.Fatalf("owned label %d after native churn = %q, want %q", i, got[i], want[i])
				}
			}
		})
	}
}

func TestPS6110ByteCopiesMustStayIndependent(t *testing.T) {
	t.Parallel()
	record := ps6110Records(1, 1)[0]
	a := append([]byte(nil), ps6110NativeView(&record)...)
	b := append([]byte(nil), ps6110NativeView(&record)...)
	a[0] = 'X'
	if b[0] == 'X' {
		t.Fatal("fresh native byte copies unexpectedly alias")
	}
}

func TestPS6110StringSemanticsAndExtractionLifetime(t *testing.T) {
	t.Parallel()
	records := make([]ps6110NativeRecord, 4)
	records[0].token = 1 // Empty labels stay empty.
	records[1].token = 2
	copy(records[1].label[:], []byte{0xff, 0xfe}) // Invalid UTF-8 bytes stay exact.
	records[2].token = 3
	copy(records[2].label[:], []byte{'a', 0, 'b'}) // GoString-style NUL termination stays exact.
	records[3] = records[1]
	records[3].token = 4

	want := ps6110Before(records)
	first := ps6110After(records)
	second := ps6110After(records)
	for i := range want {
		if first[i] != want[i] || second[i] != want[i] {
			t.Fatalf("label %d: first %q, second %q, want %q", i, first[i], second[i], want[i])
		}
	}
	if want[0] != "" || want[1] != string([]byte{0xff, 0xfe}) || want[2] != "a" {
		t.Fatalf("native string semantics = %q, %q, %q", want[0], want[1], want[2])
	}
	var labels ps6110OwnedLabels
	if got := labels.ownN(&records[2], 3); got != "a\x00b" {
		t.Fatalf("explicit-length native string = %q, want embedded NUL", got)
	}
	lengthRecord := ps6110NativeRecord{}
	copy(lengthRecord.label[:], []byte{0xff, 0, 0xfe})
	lengthLabels := ps6110OwnedLabels{}
	retainedN := lengthLabels.ownN(&lengthRecord, 3)
	wantN := string([]byte{0xff, 0, 0xfe})
	clear(lengthRecord.label[:])
	copy(lengthRecord.label[:], "changed-native-storage")
	for range 4 {
		churn := make([]byte, 1<<20)
		runtime.GC()
		runtime.KeepAlive(churn)
	}
	if retainedN != wantN {
		t.Fatalf("retained explicit-length native string = %q, want %q", retainedN, wantN)
	}
	if unsafe.StringData(first[1]) == unsafe.StringData(second[1]) {
		t.Fatal("separate extractions unexpectedly share owned storage")
	}
}

func TestPS6110ErrorAndPartialResultParity(t *testing.T) {
	t.Parallel()
	records := ps6110Records(4, 2)
	wantErr := errors.New("snapshot status")
	want, beforeErr := ps6110Partial(records, 2, wantErr, false)
	got, afterErr := ps6110Partial(records, 2, wantErr, true)
	if !errors.Is(beforeErr, wantErr) || !errors.Is(afterErr, wantErr) {
		t.Fatalf("errors = %v and %v, want %v", beforeErr, afterErr, wantErr)
	}
	if len(got) != len(want) {
		t.Fatalf("partial length = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("partial label %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func ps6110Partial(records []ps6110NativeRecord, count int, resultErr error, after bool) ([]string, error) {
	if after {
		return ps6110After(records[:count]), resultErr
	}
	return ps6110Before(records[:count]), resultErr
}

func TestPS6110EmptyAndSingleRecord(t *testing.T) {
	t.Parallel()
	if got := ps6110After(nil); len(got) != 0 {
		t.Fatalf("empty extraction length = %d, want 0", len(got))
	}
	records := ps6110Records(1, 1)
	if got, want := ps6110After(records), ps6110Before(records); len(got) != 1 || got[0] != want[0] {
		t.Fatalf("single extraction = %q, want %q", got, want)
	}
}

var ps6110BenchmarkSink []string

func BenchmarkPS6110_Before(b *testing.B) {
	records := ps6110Records(340, 10)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		ps6110BenchmarkSink = ps6110Before(records)
	}
}

func BenchmarkPS6110_After(b *testing.B) {
	records := ps6110Records(340, 10)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		ps6110BenchmarkSink = ps6110After(records)
	}
}

func BenchmarkPS6110_MixedBefore(b *testing.B) {
	records := ps6110Records(340, 170)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		ps6110BenchmarkSink = ps6110Before(records)
	}
}

func BenchmarkPS6110_MixedAfter(b *testing.B) {
	records := ps6110Records(340, 170)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		ps6110BenchmarkSink = ps6110After(records)
	}
}

func BenchmarkPS6110_ColdBefore(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps6110BenchmarkSink = ps6110Before(ps6110Records(340, 10))
	}
}

func BenchmarkPS6110_ColdAfter(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps6110BenchmarkSink = ps6110After(ps6110Records(340, 10))
	}
}

func BenchmarkPS6110_SingleBefore(b *testing.B) {
	records := ps6110Records(1, 1)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		ps6110BenchmarkSink = ps6110Before(records)
	}
}

func BenchmarkPS6110_SingleAfter(b *testing.B) {
	records := ps6110Records(1, 1)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		ps6110BenchmarkSink = ps6110After(records)
	}
}

func BenchmarkPS6110_EmptyBefore(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps6110BenchmarkSink = ps6110Before(nil)
	}
}

func BenchmarkPS6110_EmptyAfter(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps6110BenchmarkSink = ps6110After(nil)
	}
}

func TestPS6110AllocationShape(t *testing.T) {
	const child = "PERFSCAN_PS6110_ALLOCATION_CHILD"
	if os.Getenv(child) == "1" {
		ps6110AllocationChild(t)
		return
	}
	t.Parallel()
	command := exec.Command(os.Args[0], "-test.run=^TestPS6110AllocationShape$", "-test.parallel=1")
	command.Env = append(os.Environ(), child+"=1")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("isolated allocation child failed: %v\n%s", err, output)
	}
}

func ps6110AllocationChild(t *testing.T) {
	repeated := ps6110Records(340, 10)
	before := testing.AllocsPerRun(20, func() { ps6110BenchmarkSink = ps6110Before(repeated) })
	after := testing.AllocsPerRun(20, func() { ps6110BenchmarkSink = ps6110After(repeated) })
	if after >= before {
		t.Fatalf("repeated allocations: after %.0f, want below before %.0f", after, before)
	}
	if saved := before - after; saved < 300 {
		t.Fatalf("repeated allocations saved %.0f, want at least 300 (before %.0f, after %.0f)", saved, before, after)
	}
	mixed := ps6110Records(340, 170)
	mixedBefore := testing.AllocsPerRun(20, func() { ps6110BenchmarkSink = ps6110Before(mixed) })
	mixedAfter := testing.AllocsPerRun(20, func() { ps6110BenchmarkSink = ps6110After(mixed) })
	if mixedAfter >= mixedBefore {
		t.Fatalf("mixed allocations: after %.0f, want below before %.0f", mixedAfter, mixedBefore)
	}
	single := ps6110Records(1, 1)
	singleBefore := testing.AllocsPerRun(20, func() { ps6110BenchmarkSink = ps6110Before(single) })
	singleAfter := testing.AllocsPerRun(20, func() { ps6110BenchmarkSink = ps6110After(single) })
	if singleAfter != singleBefore {
		t.Fatalf("single allocations: after %.0f, want before %.0f", singleAfter, singleBefore)
	}
	emptyBefore := testing.AllocsPerRun(20, func() { ps6110BenchmarkSink = ps6110Before(nil) })
	emptyAfter := testing.AllocsPerRun(20, func() { ps6110BenchmarkSink = ps6110After(nil) })
	if emptyAfter != emptyBefore {
		t.Fatalf("empty allocations: after %.0f, want before %.0f", emptyAfter, emptyBefore)
	}
}
