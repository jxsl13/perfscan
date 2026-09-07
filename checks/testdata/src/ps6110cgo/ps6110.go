package ps6110cgo

/*
#include <stddef.h>

typedef struct {
	char label[16];
	int length;
} profile_event;

static profile_event events[2] = {{{'a', 0}, 1}, {{'a', 0}, 1}};

static int profile_snapshot(profile_event **out, int *count) {
	*out = events;
	*count = 2;
	return 0;
}
*/
import "C"

import "unsafe"

type Event struct{ Label string }
type recorder struct{}

func (r *recorder) Free() {}

func (r *recorder) Profile() ([]Event, error) {
	var native *C.profile_event
	var count C.int
	rc := C.profile_snapshot(&native, &count)
	if rc != 0 {
		return nil, nil
	}
	if count == 1 {
		return []Event{{Label: C.GoString(&native.label[0])}}, nil
	}
	n := int(count)
	out := make([]Event, n)
	nativeRecords := unsafe.Slice(native, n)
	for i := range out {
		out[i].Label = C.GoString(&nativeRecords[i].label[0]) // want `cgo-labels: C\.GoString copies an owned Go string once per record from the configured bulk snapshot C\.profile_snapshot; consider extraction-scoped exact-content deduplication`
	}
	return out, nil
}

func (r *recorder) ProfileN() ([]Event, error) {
	var native *C.profile_event
	var count C.int
	rc := C.profile_snapshot(&native, &count)
	if rc != 0 {
		return nil, nil
	}
	if count == 1 {
		return nil, nil
	}
	n := int(count)
	out := make([]Event, n)
	nativeRecords := unsafe.Slice(native, n)
	for i := range out {
		out[i].Label = C.GoStringN(&nativeRecords[i].label[0], nativeRecords[i].length) // want `cgo-labels-n: C\.GoStringN copies an owned Go string once per record from the configured bulk snapshot C\.profile_snapshot; consider extraction-scoped exact-content deduplication`
	}
	return out, nil
}
