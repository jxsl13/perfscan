package checks

import (
	"bytes"
	"errors"
	"io"
	"slices"
	"testing"
)

type ps5076Reader struct {
	data      []byte
	chunk     int
	reads     int
	requested []int
	err       error
}

func (r *ps5076Reader) Read(p []byte) (int, error) {
	r.reads++
	r.requested = append(r.requested, len(p))
	if len(r.data) == 0 {
		if r.err != nil {
			err := r.err
			r.err = nil
			return 0, err
		}
		return 0, io.EOF
	}
	n := min(len(p), len(r.data))
	if r.chunk > 0 {
		n = min(n, r.chunk)
	}
	copy(p, r.data[:n])
	r.data = r.data[n:]
	return n, nil
}

type ps5076WriterTo struct {
	data  []byte
	calls int
}

func (r *ps5076WriterTo) Read([]byte) (int, error) { return 0, errors.New("unexpected Read") }
func (r *ps5076WriterTo) WriteTo(w io.Writer) (int64, error) {
	r.calls++
	n, err := w.Write(r.data)
	return int64(n), err
}

func TestEquiv_PS5076NopCloserConsumers(t *testing.T) {
	payload := bytes.Repeat([]byte("reader-data"), 37)
	wantErr := errors.New("terminal")
	beforeReader := &ps5076Reader{data: bytes.Clone(payload), chunk: 7, err: wantErr}
	afterReader := &ps5076Reader{data: bytes.Clone(payload), chunk: 7, err: wantErr}
	beforeBytes, beforeErr := io.ReadAll(io.NopCloser(beforeReader))
	afterBytes, afterErr := io.ReadAll(afterReader)
	if !bytes.Equal(beforeBytes, afterBytes) || !errors.Is(beforeErr, wantErr) || !errors.Is(afterErr, wantErr) ||
		beforeReader.reads != afterReader.reads || !slices.Equal(beforeReader.requested, afterReader.requested) {
		t.Fatalf("ReadAll differs: bytes=%v/%v err=%v/%v reads=%d/%d requests=%v/%v", bytes.Equal(beforeBytes, payload), bytes.Equal(afterBytes, payload), beforeErr, afterErr, beforeReader.reads, afterReader.reads, beforeReader.requested, afterReader.requested)
	}

	beforeWT := &ps5076WriterTo{data: payload}
	afterWT := &ps5076WriterTo{data: payload}
	var beforeDst, afterDst bytes.Buffer
	beforeN, beforeErr := io.Copy(&beforeDst, io.NopCloser(beforeWT))
	afterN, afterErr := io.Copy(&afterDst, afterWT)
	if beforeN != afterN || beforeErr != afterErr || !bytes.Equal(beforeDst.Bytes(), afterDst.Bytes()) || beforeWT.calls != 1 || afterWT.calls != 1 {
		t.Fatalf("Copy WriterTo differs: n=%d/%d err=%v/%v calls=%d/%d", beforeN, afterN, beforeErr, afterErr, beforeWT.calls, afterWT.calls)
	}

	beforeReader = &ps5076Reader{data: bytes.Clone(payload), chunk: 11}
	afterReader = &ps5076Reader{data: bytes.Clone(payload), chunk: 11}
	beforeDst.Reset()
	afterDst.Reset()
	beforeN, beforeErr = io.CopyBuffer(&beforeDst, io.NopCloser(beforeReader), make([]byte, 19))
	afterN, afterErr = io.CopyBuffer(&afterDst, afterReader, make([]byte, 19))
	if beforeN != afterN || beforeErr != afterErr || !bytes.Equal(beforeDst.Bytes(), afterDst.Bytes()) ||
		beforeReader.reads != afterReader.reads || !slices.Equal(beforeReader.requested, afterReader.requested) {
		t.Fatalf("CopyBuffer differs: n=%d/%d err=%v/%v reads=%d/%d", beforeN, afterN, beforeErr, afterErr, beforeReader.reads, afterReader.reads)
	}
}
