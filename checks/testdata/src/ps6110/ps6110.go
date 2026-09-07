package ps6110

import "unsafe"

type nativeEvent struct {
	label  [32]byte
	length int
}

type Event struct {
	Label string
	Code  int
}

type Profile struct {
	Events []Event
}

type Recorder struct {
	native []nativeEvent
}

func snapshot(r *Recorder, events **nativeEvent, count *int) int {
	if len(r.native) == 0 {
		*events = nil
		*count = 0
		return 0
	}
	*events = &r.native[0]
	*count = len(r.native)
	return 0
}

func snapshotDirect(events **nativeEvent, count *int) int {
	*events = nil
	*count = 0
	return 0
}

func ownedString(p *byte) string { return string(unsafe.Slice(p, 1)) }

func ownedStringN(p *byte, n int) string { return string(unsafe.Slice(p, n)) }

func ownedBytes(p *byte) []byte { return append([]byte(nil), *p) }

func consume([]Event) {}

func (r *Recorder) Free() { r.native = nil }

func (r *Recorder) Profile() (Profile, error) {
	var native *nativeEvent
	var count int
	rc := snapshot(r, &native, &count)
	if rc != 0 {
		return Profile{}, nil
	}
	if count < 0 || count > 0 && native == nil {
		return Profile{}, nil
	}
	if count == 1 {
		return Profile{Events: []Event{{Label: ownedString(&native.label[0])}}}, nil
	}
	n := int(count)
	out := Profile{Events: make([]Event, n)}
	nativeRecords := unsafe.Slice(native, n)
	for i := range out.Events {
		event := &nativeRecords[i]
		out.Events[i] = Event{
			Label: ownedString(&event.label[0]), // want `labels: ps6110\.ownedString copies an owned Go string once per record from the configured bulk snapshot ps6110\.snapshot; consider extraction-scoped exact-content deduplication`
			Code:  i,
		}
	}
	return out, nil
}

func (r *Recorder) ProfileN() ([]Event, error) {
	var native *nativeEvent
	var count int
	rc := snapshot(r, &native, &count)
	if rc != 0 {
		return nil, nil
	}
	if count == 1 {
		return []Event{{Label: ownedStringN(&native.label[0], native.length)}}, nil
	}
	n := int(count)
	out := make([]Event, n)
	nativeRecords := unsafe.Slice(native, n)
	for i := 0; i < n; i++ {
		out[i].Label = ownedStringN(&nativeRecords[i].label[0], nativeRecords[i].length) // want `labels-n: ps6110\.ownedStringN copies an owned Go string once per record from the configured bulk snapshot ps6110\.snapshot; consider extraction-scoped exact-content deduplication`
	}
	return out, nil
}

func (r *Recorder) DirectCount() ([]Event, error) {
	var native *nativeEvent
	var count int
	rc := snapshot(r, &native, &count)
	if rc != 0 {
		return nil, nil
	}
	if count == 1 {
		return nil, nil
	}
	out := make([]Event, count)
	nativeRecords := unsafe.Slice(native, count)
	for i := range out {
		out[i].Label = ownedString(&nativeRecords[i].label[0]) // want `direct-count: ps6110\.ownedString copies an owned Go string once per record from the configured bulk snapshot ps6110\.snapshot; consider extraction-scoped exact-content deduplication`
	}
	return out, nil
}

// A parameter is not a locally acquired snapshot.
func (r *Recorder) ParameterSnapshot(native *nativeEvent, count int) ([]Event, error) {
	if count == 1 {
		return nil, nil
	}
	out := make([]Event, count)
	nativeRecords := unsafe.Slice(native, count)
	for i := range out {
		out[i].Label = ownedString(&nativeRecords[i].label[0])
	}
	return out, nil
}

// The native and destination indexes must be the same object.
func (r *Recorder) OffsetIndex() ([]Event, error) {
	var native *nativeEvent
	var count int
	rc := snapshot(r, &native, &count)
	if rc != 0 {
		return nil, nil
	}
	if count == 1 {
		return nil, nil
	}
	out := make([]Event, count)
	nativeRecords := unsafe.Slice(native, count)
	for i := range out {
		out[i].Label = ownedString(&nativeRecords[i+1].label[0])
	}
	return out, nil
}

// No source proof distinguishes a one-record extraction.
func (r *Recorder) NoSingleFastPath() ([]Event, error) {
	var native *nativeEvent
	var count int
	rc := snapshot(r, &native, &count)
	if rc != 0 {
		return nil, nil
	}
	out := make([]Event, count)
	nativeRecords := unsafe.Slice(native, count)
	for i := range out {
		out[i].Label = ownedString(&nativeRecords[i].label[0])
	}
	return out, nil
}

// The destination escapes to an opaque call before return.
func (r *Recorder) EscapedDestination() ([]Event, error) {
	var native *nativeEvent
	var count int
	rc := snapshot(r, &native, &count)
	if rc != 0 {
		return nil, nil
	}
	if count == 1 {
		return nil, nil
	}
	out := make([]Event, count)
	consume(out)
	nativeRecords := unsafe.Slice(native, count)
	for i := range out {
		out[i].Label = ownedString(&nativeRecords[i].label[0])
	}
	return out, nil
}

// Freeing the source before materialization invalidates the native view.
func (r *Recorder) EarlyFree() ([]Event, error) {
	var native *nativeEvent
	var count int
	rc := snapshot(r, &native, &count)
	if rc != 0 {
		return nil, nil
	}
	if count == 1 {
		return nil, nil
	}
	r.Free()
	out := make([]Event, count)
	nativeRecords := unsafe.Slice(native, count)
	for i := range out {
		out[i].Label = ownedString(&nativeRecords[i].label[0])
	}
	return out, nil
}

func touchNative(*nativeEvent) {}

// Passing the snapshot root to an opaque call invalidates the local proof.
func (r *Recorder) ExposedSnapshot() ([]Event, error) {
	var native *nativeEvent
	var count int
	rc := snapshot(r, &native, &count)
	if rc != 0 {
		return nil, nil
	}
	if count == 1 {
		return nil, nil
	}
	touchNative(native)
	out := make([]Event, count)
	nativeRecords := unsafe.Slice(native, count)
	for i := range out {
		out[i].Label = ownedString(&nativeRecords[i].label[0])
	}
	return out, nil
}

// Capturing the destination changes its ownership boundary.
func (r *Recorder) CapturedDestination() ([]Event, error) {
	var native *nativeEvent
	var count int
	rc := snapshot(r, &native, &count)
	if rc != 0 {
		return nil, nil
	}
	if count == 1 {
		return nil, nil
	}
	out := make([]Event, count)
	_ = func() int { return len(out) }
	nativeRecords := unsafe.Slice(native, count)
	for i := range out {
		out[i].Label = ownedString(&nativeRecords[i].label[0])
	}
	return out, nil
}

// Only one direct native field is accepted; a pointer alias is not followed.
func (r *Recorder) LabelAlias() ([]Event, error) {
	var native *nativeEvent
	var count int
	rc := snapshot(r, &native, &count)
	if rc != 0 {
		return nil, nil
	}
	if count == 1 {
		return nil, nil
	}
	out := make([]Event, count)
	nativeRecords := unsafe.Slice(native, count)
	for i := range out {
		label := &nativeRecords[i].label[0]
		out[i].Label = ownedString(label)
	}
	return out, nil
}

// Mutating the canonical count after deriving the two slices invalidates the
// same-count proof even when the loop would remain in bounds.
func (r *Recorder) MutatedCanonicalCount() ([]Event, error) {
	var native *nativeEvent
	var count int
	rc := snapshot(r, &native, &count)
	if rc != 0 {
		return nil, nil
	}
	if count == 1 {
		return nil, nil
	}
	n := int(count)
	out := make([]Event, n)
	nativeRecords := unsafe.Slice(native, n)
	n--
	for i := 0; i < n; i++ {
		out[i].Label = ownedString(&nativeRecords[i].label[0])
	}
	return out, nil
}

// A conditional copy is not one owned-string allocation per snapshot record.
func (r *Recorder) ConditionalCopy() ([]Event, error) {
	var native *nativeEvent
	var count int
	rc := snapshot(r, &native, &count)
	if rc != 0 {
		return nil, nil
	}
	if count == 1 {
		return nil, nil
	}
	out := make([]Event, count)
	nativeRecords := unsafe.Slice(native, count)
	for i := range out {
		if i%2 == 0 {
			out[i].Label = ownedString(&nativeRecords[i].label[0])
		}
	}
	return out, nil
}

// Two configured copies per iteration do not match the single-copy claim.
func (r *Recorder) DuplicateCopy() ([]Event, error) {
	var native *nativeEvent
	var count int
	rc := snapshot(r, &native, &count)
	if rc != 0 {
		return nil, nil
	}
	if count == 1 {
		return nil, nil
	}
	out := make([]Event, count)
	nativeRecords := unsafe.Slice(native, count)
	for i := range out {
		out[i].Label = ownedString(&nativeRecords[i].label[0])
		out[i].Label = ownedString(&nativeRecords[i].label[0])
	}
	return out, nil
}

// The fresh []byte result is mutable and cannot be shared by this rule.
func (r *Recorder) Bytes() ([][]byte, error) {
	var native *nativeEvent
	var count int
	rc := snapshotDirect(&native, &count)
	if rc != 0 {
		return nil, nil
	}
	if count == 1 {
		return nil, nil
	}
	out := make([][]byte, count)
	nativeRecords := unsafe.Slice(native, count)
	for i := range out {
		out[i] = ownedBytes(&nativeRecords[i].label[0])
	}
	return out, nil
}

type fakeC struct{}

func (fakeC) GoString(*byte) string { return "" }

var C fakeC

// A C-shaped Go method is not cgo's C.GoString.
func (r *Recorder) FakeC() ([]Event, error) {
	var native *nativeEvent
	var count int
	rc := snapshot(r, &native, &count)
	if rc != 0 {
		return nil, nil
	}
	if count == 1 {
		return nil, nil
	}
	out := make([]Event, count)
	nativeRecords := unsafe.Slice(native, count)
	for i := range out {
		out[i].Label = C.GoString(&nativeRecords[i].label[0])
	}
	return out, nil
}

var escaped []Event
var globalNative *nativeEvent
var globalCount int

func _Cfunc_GoString(p *byte) string { return ownedString(p) }

func (r *Recorder) GlobalDestination() ([]Event, error) {
	var native *nativeEvent
	var count int
	rc := snapshot(r, &native, &count)
	if rc != 0 {
		return nil, nil
	}
	if count == 1 {
		return nil, nil
	}
	out := make([]Event, count)
	escaped = out
	nativeRecords := unsafe.Slice(native, count)
	for i := range out {
		out[i].Label = ownedString(&nativeRecords[i].label[0])
	}
	return out, nil
}

func (r *Recorder) AliasedNative() ([]Event, error) {
	var native *nativeEvent
	var count int
	rc := snapshot(r, &native, &count)
	if rc != 0 {
		return nil, nil
	}
	if count == 1 {
		return nil, nil
	}
	out := make([]Event, count)
	nativeRecords := unsafe.Slice(native, count)
	alias := nativeRecords
	alias[0].label[0] = 'x'
	for i := range out {
		out[i].Label = ownedString(&nativeRecords[i].label[0])
	}
	return out, nil
}

func (r *Recorder) MutatedIndex() ([]Event, error) {
	var native *nativeEvent
	var count int
	rc := snapshot(r, &native, &count)
	if rc != 0 {
		return nil, nil
	}
	if count == 1 {
		return nil, nil
	}
	out := make([]Event, count)
	nativeRecords := unsafe.Slice(native, count)
	for i := range out {
		if i == 0 {
			i++
		}
		out[i].Label = ownedString(&nativeRecords[i].label[0])
	}
	return out, nil
}

func (r *Recorder) DeadLoop() ([]Event, error) {
	return nil, nil
	var native *nativeEvent
	var count int
	rc := snapshot(r, &native, &count)
	if rc != 0 {
		return nil, nil
	}
	if count == 1 {
		return nil, nil
	}
	out := make([]Event, count)
	nativeRecords := unsafe.Slice(native, count)
	for i := range out {
		out[i].Label = ownedString(&nativeRecords[i].label[0])
	}
	return out, nil
}

func (r *Recorder) SkippedCopy() ([]Event, error) {
	var native *nativeEvent
	var count int
	rc := snapshot(r, &native, &count)
	if rc != 0 {
		return nil, nil
	}
	if count == 1 {
		return nil, nil
	}
	out := make([]Event, count)
	nativeRecords := unsafe.Slice(native, count)
	for i := range out {
		if i%2 == 0 {
			continue
		}
		out[i].Label = ownedString(&nativeRecords[i].label[0])
	}
	return out, nil
}

func (r *Recorder) FakeGenerated() ([]Event, error) {
	var native *nativeEvent
	var count int
	rc := snapshot(r, &native, &count)
	if rc != 0 {
		return nil, nil
	}
	if count == 1 {
		return nil, nil
	}
	out := make([]Event, count)
	nativeRecords := unsafe.Slice(native, count)
	for i := range out {
		out[i].Label = _Cfunc_GoString(&nativeRecords[i].label[0])
	}
	return out, nil
}

func (r *Recorder) ParameterOutArguments(native *nativeEvent, count int) ([]Event, error) {
	rc := snapshot(r, &native, &count)
	if rc != 0 {
		return nil, nil
	}
	if count == 1 {
		return nil, nil
	}
	out := make([]Event, count)
	nativeRecords := unsafe.Slice(native, count)
	for i := range out {
		out[i].Label = ownedString(&nativeRecords[i].label[0])
	}
	return out, nil
}

func (r *Recorder) GlobalAcquisition() ([]Event, error) {
	rc := snapshot(r, &globalNative, &globalCount)
	if rc != 0 {
		return nil, nil
	}
	if globalCount == 1 {
		return nil, nil
	}
	out := make([]Event, globalCount)
	nativeRecords := unsafe.Slice(globalNative, globalCount)
	for i := range out {
		out[i].Label = ownedString(&nativeRecords[i].label[0])
	}
	return out, nil
}

func (r *Recorder) ClobberedStatus() ([]Event, error) {
	var native *nativeEvent
	var count int
	rc := snapshot(r, &native, &count)
	rc = 0
	if rc != 0 {
		return nil, nil
	}
	if count == 1 {
		return nil, nil
	}
	out := make([]Event, count)
	nativeRecords := unsafe.Slice(native, count)
	for i := range out {
		out[i].Label = ownedString(&nativeRecords[i].label[0])
	}
	return out, nil
}

func (r *Recorder) OverwrittenOutput() ([]Event, error) {
	var native *nativeEvent
	var count int
	rc := snapshot(r, &native, &count)
	if rc != 0 {
		return nil, nil
	}
	if count == 1 {
		return nil, nil
	}
	out := make([]Event, count)
	nativeRecords := unsafe.Slice(native, count)
	for i := range out {
		out[i].Label = ownedString(&nativeRecords[i].label[0])
		out[i].Label = "discarded"
	}
	return out, nil
}

func (r *Recorder) UnresolvedLifecycle() ([]Event, error) {
	var native *nativeEvent
	var count int
	rc := snapshot(r, &native, &count)
	if rc != 0 {
		return nil, nil
	}
	if count == 1 {
		return nil, nil
	}
	out := make([]Event, count)
	nativeRecords := unsafe.Slice(native, count)
	for i := range out {
		out[i].Label = ownedString(&nativeRecords[i].label[0])
	}
	return out, nil
}
