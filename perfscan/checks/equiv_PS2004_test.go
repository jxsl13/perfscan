package checks

import (
	"bytes"
	"slices"
	"strings"
	"testing"
)

// ps2004FillForeignLabel models the contract required by PS2004's guarded
// cgo fix: every byte is defined, shorter labels are zero-padded, and byte 95
// is always NUL. It deliberately handles embedded NUL like strncpy.
func ps2004FillForeignLabel(dst []byte, label string) {
	clear(dst)
	if nul := strings.IndexByte(label, 0); nul >= 0 {
		label = label[:nul]
	}
	if len(label) >= len(dst) {
		label = label[:len(dst)-1]
	}
	copy(dst, label)
	dst[len(dst)-1] = 0
}

func ps2004DecodeForeignLabel(label []byte) string {
	end := bytes.IndexByte(label, 0)
	return string(label[:end])
}

func ps2004LabelsBefore(labels []string) []string {
	out := make([]string, 0, len(labels))
	for _, value := range labels {
		label := make([]byte, 96)
		ps2004FillForeignLabel(label, value)
		out = append(out, ps2004DecodeForeignLabel(label))
	}
	return out
}

func ps2004LabelsAfter(labels []string) []string {
	out := make([]string, 0, len(labels))
	label := make([]byte, 96)
	for _, value := range labels {
		ps2004FillForeignLabel(label, value)
		out = append(out, ps2004DecodeForeignLabel(label))
	}
	return out
}

func TestEquivPS2004CgoFullOverwriteReuse(t *testing.T) {
	long := strings.Repeat("L", 140)
	values := []string{
		"a label that is deliberately much longer than its successor",
		"x", // shrinking adjacency catches a stale suffix
		"",
		long,
		"grow again after empty",
		"prefix\x00ignored suffix",
		"z",
	}
	before, after := ps2004LabelsBefore(values), ps2004LabelsAfter(values)
	if !slices.Equal(before, after) {
		t.Fatalf("buffer reuse changed decoded labels:\nbefore: %#v\nafter:  %#v", before, after)
	}
	if after[1] != "x" || after[2] != "" || after[5] != "prefix" {
		t.Fatalf("zero-padding/NUL contract not preserved: %#v", after)
	}
	if len(after[3]) != 95 || after[3] != long[:95] {
		t.Fatalf("95-byte truncation changed: len=%d value=%q", len(after[3]), after[3])
	}
}
