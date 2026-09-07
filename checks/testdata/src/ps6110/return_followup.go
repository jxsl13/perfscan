package ps6110

import "unsafe"

type NativeExposedEvent struct {
	Label  string
	Native *nativeEvent
}

func (r *Recorder) NativeReturn() ([]Event, *nativeEvent) {
	var native *nativeEvent
	var count int
	rc := snapshot(r, &native, &count)
	if rc != 0 {
		return nil, nil
	}
	if count < 0 || count > 0 && native == nil {
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
	return out, native
}

func (r *Recorder) CompositeNativeEscape() ([]NativeExposedEvent, error) {
	var native *nativeEvent
	var count int
	rc := snapshot(r, &native, &count)
	if rc != 0 {
		return nil, nil
	}
	if count < 0 || count > 0 && native == nil {
		return nil, nil
	}
	if count == 1 {
		return nil, nil
	}
	out := make([]NativeExposedEvent, count)
	nativeRecords := unsafe.Slice(native, count)
	for i := range out {
		out[i] = NativeExposedEvent{Label: ownedString(&nativeRecords[i].label[0]), Native: native}
	}
	return out, nil
}

func (r *Recorder) CompositeGood() ([]Event, error) {
	var native *nativeEvent
	var count int
	rc := snapshot(r, &native, &count)
	if rc != 0 {
		return nil, nil
	}
	if count < 0 || count > 0 && native == nil {
		return nil, nil
	}
	if count == 1 {
		return nil, nil
	}
	out := make([]Event, count)
	nativeRecords := unsafe.Slice(native, count)
	for i := range out {
		out[i] = Event{Label: ownedString(&nativeRecords[i].label[0])} // want `composite-good: ps6110\.ownedString copies an owned Go string once per record`
	}
	return out, nil
}

func (r *Recorder) FakeAdjustedFilename() ([]Event, error) {
	var native *nativeEvent
	var count int
	rc := snapshot(r, &native, &count)
	if rc != 0 {
		return nil, nil
	}
	if count < 0 || count > 0 && native == nil {
		return nil, nil
	}
	if count == 1 {
		return nil, nil
	}
	out := make([]Event, count)
	nativeRecords := unsafe.Slice(native, count)
	for i := range out {
		out[i].Label = _Cfunc_GoStringN(&nativeRecords[i].label[0], nativeRecords[i].length)
	}
	return out, nil
}

//line review-generated.c:1
func _Cfunc_GoStringN(p *byte, n int) string { return ownedStringN(p, n) }
