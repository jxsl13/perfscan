package ps2143

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
)

type tensor struct{ data []byte }

func parseAll([]byte) (map[string]tensor, error) { return nil, nil }
func parseReader(io.Reader) ([]tensor, error)    { return nil, nil }

func selected(path, name string, offset int64) (tensor, error) {
	f, err := os.Open(path)
	if err != nil {
		return tensor{}, err
	}
	payload := make([]byte, 4<<20)
	if _, err := f.ReadAt(payload, offset); err != nil {
		return tensor{}, err
	}
	header, err := json.Marshal(map[string]any{"name": name, "size": len(payload)})
	if err != nil {
		return tensor{}, err
	}
	var synthetic bytes.Buffer
	_, _ = synthetic.Write(header)
	_, _ = synthetic.Write(payload)
	items, err := parseAll(synthetic.Bytes()) // want `partial ReadAt payload is copied with a marshaled JSON header`
	if err != nil {
		return tensor{}, err
	}
	return items[name], nil
}

func selectedReader(path string) (tensor, error) {
	f, err := os.Open(path)
	if err != nil {
		return tensor{}, err
	}
	payload := make([]byte, 4096)
	if _, err := f.ReadAt(payload, 128); err != nil {
		return tensor{}, err
	}
	header, _ := json.Marshal(struct{ Count int }{1})
	var synthetic bytes.Buffer
	_, _ = synthetic.Write(header)
	_, _ = synthetic.Write(payload)
	items, err := parseReader(bytes.NewReader(synthetic.Bytes())) // want `partial ReadAt payload is copied with a marshaled JSON header`
	if err != nil {
		return tensor{}, err
	}
	return items[0], nil
}

// --- negatives ---

// A constant zero offset does not establish a partial-range path.
func fromStart(f *os.File) (tensor, error) {
	payload := make([]byte, 4096)
	_, _ = f.ReadAt(payload, 0)
	header, _ := json.Marshal(1)
	var synthetic bytes.Buffer
	_, _ = synthetic.Write(header)
	_, _ = synthetic.Write(payload)
	items, err := parseAll(synthetic.Bytes())
	return items["x"], err
}

// A negative constant offset cannot produce a successful partial read.
func invalidOffset(f *os.File) (tensor, error) {
	payload := make([]byte, 4096)
	_, _ = f.ReadAt(payload, -1)
	header, _ := json.Marshal(1)
	var synthetic bytes.Buffer
	_, _ = synthetic.Write(header)
	_, _ = synthetic.Write(payload)
	items, err := parseAll(synthetic.Bytes())
	return items["x"], err
}

// No synthetic header.
func payloadOnly(f *os.File) (tensor, error) {
	payload := make([]byte, 4096)
	_, _ = f.ReadAt(payload, 64)
	var synthetic bytes.Buffer
	_, _ = synthetic.Write(payload)
	items, err := parseAll(synthetic.Bytes())
	return items["x"], err
}

// The collection is retained/observed beyond one immediate item lookup.
func retainsCollection(f *os.File) (map[string]tensor, error) {
	payload := make([]byte, 4096)
	_, _ = f.ReadAt(payload, 64)
	header, _ := json.Marshal(1)
	var synthetic bytes.Buffer
	_, _ = synthetic.Write(header)
	_, _ = synthetic.Write(payload)
	items, err := parseAll(synthetic.Bytes())
	_ = items["x"]
	return items, err
}

// A caller-owned buffer is not proven fresh here.
func externalBuffer(f *os.File, synthetic *bytes.Buffer) (tensor, error) {
	payload := make([]byte, 4096)
	_, _ = f.ReadAt(payload, 64)
	header, _ := json.Marshal(1)
	_, _ = synthetic.Write(header)
	_, _ = synthetic.Write(payload)
	items, err := parseAll(synthetic.Bytes())
	return items["x"], err
}

// Reset destroys the header written before it, so the later parser input is
// not the reconstructed header+payload neighborhood.
func resetBetween(f *os.File) (tensor, error) {
	payload := make([]byte, 4096)
	_, _ = f.ReadAt(payload, 64)
	header, _ := json.Marshal(1)
	var synthetic bytes.Buffer
	_, _ = synthetic.Write(header)
	synthetic.Reset()
	_, _ = synthetic.Write(payload)
	items, err := parseAll(synthetic.Bytes())
	return items["x"], err
}

// Reassigning the marshaled local means its later bytes no longer have the
// source provenance required by the detector.
func reassignedHeader(f *os.File) (tensor, error) {
	payload := make([]byte, 4096)
	_, _ = f.ReadAt(payload, 64)
	header, _ := json.Marshal(1)
	header = payload
	var synthetic bytes.Buffer
	_, _ = synthetic.Write(header)
	_, _ = synthetic.Write(payload)
	items, err := parseAll(synthetic.Bytes())
	return items["x"], err
}
