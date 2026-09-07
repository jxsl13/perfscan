package ps6110silent

import "unsafe"

type nativeEvent struct{ label [8]byte }
type Event struct{ Label string }
type Recorder struct{ native []nativeEvent }

func snapshot(r *Recorder, events **nativeEvent, count *int) int {
	if len(r.native) > 0 {
		*events = &r.native[0]
	}
	*count = len(r.native)
	return 0
}

func ownedString(p *byte) string { return string(unsafe.Slice(p, 1)) }
func (r *Recorder) Free()        {}

func (r *Recorder) Profile() ([]Event, error) {
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
