package closureenv

import (
	"bytes"
	"testing"
)

func TestArtifactRoundTripRevalidatesEvidence(t *testing.T) {
	t.Parallel()
	before := Evidence{GoVersion: "go1.27", GOOS: "darwin", GOARCH: "arm64", Package: "p", Function: "work", Escapes: true, EnvironmentBytes: 224, SizeClassBytes: 224, Captures: []Capture{{Name: "a", Type: "[]byte", Size: 24, Align: 8, HasPointers: true}}, Scanned: true}
	after := before
	after.EnvironmentBytes, after.SizeClassBytes = 232, 240
	after.Captures = append(slicesClone(before.Captures), Capture{Name: "width", Type: "uint8", Size: 1, Align: 1})
	growth, err := Compare(&before, &after)
	if err != nil {
		t.Fatal(err)
	}
	var encoded bytes.Buffer
	if err := WriteArtifact(&encoded, growth); err != nil {
		t.Fatal(err)
	}
	decoded, err := ReadArtifact(&encoded)
	if err != nil || len(decoded.Added) != 1 {
		t.Fatalf("ReadArtifact() = %+v, %v", decoded, err)
	}
}

func TestArtifactRejectsTrailingValue(t *testing.T) {
	t.Parallel()
	if _, err := ReadArtifact(bytes.NewBufferString(`{"version":1}{}`)); err == nil {
		t.Fatal("accepted trailing artifact value")
	}
}
