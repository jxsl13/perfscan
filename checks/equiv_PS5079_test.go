package checks

import (
	"bytes"
	"strings"
	"testing"
)

func TestEquiv_PS5079EmptyBoundaryOperations(t *testing.T) {
	t.Parallel()
	stringFns := []struct {
		name string
		fn   func(string, string) string
	}{
		{"Trim", strings.Trim},
		{"TrimLeft", strings.TrimLeft},
		{"TrimRight", strings.TrimRight},
		{"TrimPrefix", strings.TrimPrefix},
		{"TrimSuffix", strings.TrimSuffix},
	}
	inputs := []string{"", "payload", " x ", "\u00a0payload\u3000", string([]byte{0xff, 'p', 0xfe})}
	for _, fn := range stringFns {
		for _, input := range inputs {
			if got := fn.fn(input, ""); got != input {
				t.Fatalf("strings.%s(%q, empty) = %q", fn.name, input, got)
			}
		}
	}
	for _, input := range inputs {
		before := strings.Trim(strings.TrimPrefix(strings.TrimSuffix(strings.TrimLeft(strings.TrimRight(input, ""), ""), ""), ""), "")
		if before != input {
			t.Fatalf("deep strings empty-boundary chain changed %q to %q", input, before)
		}
	}

	byteTrimFns := []struct {
		name string
		fn   func([]byte, string) []byte
	}{
		{"TrimRight", bytes.TrimRight},
	}
	byteEdgeFns := []struct {
		name string
		fn   func([]byte, []byte) []byte
	}{
		{"TrimPrefix", bytes.TrimPrefix},
		{"TrimSuffix", bytes.TrimSuffix},
	}
	byteInputs := [][]byte{nil, {}, []byte("payload"), []byte(" x "), {0xff, 'p', 0xfe}}
	for _, input := range byteInputs {
		for _, fn := range byteTrimFns {
			backing := ps5079Backing(input)
			got := fn.fn(backing, "")
			ps5079CheckBytes(t, fn.name, backing, got)
		}
		for _, fn := range byteEdgeFns {
			for _, empty := range [][]byte{nil, {}} {
				backing := ps5079Backing(input)
				got := fn.fn(backing, empty)
				ps5079CheckBytes(t, fn.name, backing, got)
			}
		}
		backing := ps5079Backing(input)
		got := bytes.TrimPrefix(bytes.TrimRight(bytes.TrimSuffix(backing, nil), ""), []byte{})
		ps5079CheckBytes(t, "deep chain", backing, got)
	}

	// Guard the historical header behavior that excludes bytes.Trim/TrimLeft.
	empty := make([]byte, 0, 11)
	if got := bytes.Trim(empty, ""); got != nil || cap(got) != 0 {
		t.Fatalf("bytes.Trim empty-header boundary changed: %#v cap=%d", got, cap(got))
	}
	if got := bytes.TrimLeft(empty, ""); got != nil || cap(got) != 0 {
		t.Fatalf("bytes.TrimLeft empty-header boundary changed: %#v cap=%d", got, cap(got))
	}
}

func TestEquiv_PS5079DefinedByteInputChangesDynamicType(t *testing.T) {
	t.Parallel()
	type namedBytes []byte

	var bytesCall any = bytes.TrimPrefix(namedBytes("payload"), nil)
	var retainedBytes any = namedBytes("payload")
	if _, ok := bytesCall.([]byte); !ok {
		t.Fatalf("bytes.TrimPrefix result has dynamic type %T, want []byte", bytesCall)
	}
	if _, ok := retainedBytes.(namedBytes); !ok {
		t.Fatalf("retained input has dynamic type %T, want namedBytes", retainedBytes)
	}
}

func ps5079Backing(input []byte) []byte {
	if input == nil {
		return nil
	}
	backing := make([]byte, len(input), len(input)+11)
	copy(backing, input)
	return backing
}

func ps5079CheckBytes(t *testing.T, name string, backing, got []byte) {
	t.Helper()
	if (got == nil) != (backing == nil) || len(got) != len(backing) || cap(got) != cap(backing) || !bytes.Equal(got, backing) {
		t.Fatalf("bytes.%s changed header/content: got=%q len/cap=%d/%d backing=%q len/cap=%d/%d", name, got, len(got), cap(got), backing, len(backing), cap(backing))
	}
	if len(got) > 0 && &got[0] != &backing[0] {
		t.Fatalf("bytes.%s changed backing start", name)
	}
}
