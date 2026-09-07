package benchmarks

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"hash/fnv"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// PS2145 compares the exact detector remedy boundary: a streaming parser that
// heap-stages one complete payload against a Close-owned read-only mapping of
// the same regular file. Both arms construct the same two retained views,
// consume every payload byte into the same digest, and close every lifetime.

type ps2145Owner struct {
	file    *os.File
	mapped  []byte
	meta    []byte
	weights []byte
	unmap   func() error
}

func (owner *ps2145Owner) Close() error {
	var unmapErr error
	if owner.unmap != nil {
		unmapErr = owner.unmap()
	}
	closeErr := owner.file.Close()
	owner.unmap = nil
	owner.mapped = nil
	owner.meta = nil
	owner.weights = nil
	if unmapErr != nil {
		return unmapErr
	}
	return closeErr
}

//go:noinline
func ps2145LoadBefore(path string) (*ps2145Owner, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	reader := bufio.NewReader(file)
	var header [8]byte
	if _, err := io.ReadFull(reader, header[:]); err != nil {
		_ = file.Close()
		return nil, err
	}
	size := binary.LittleEndian.Uint64(header[:])
	if size > uint64(^uint(0)>>1) {
		_ = file.Close()
		return nil, errors.New("payload too large")
	}
	payload := make([]byte, int(size))
	if _, err := io.ReadFull(reader, payload); err != nil {
		_ = file.Close()
		return nil, err
	}
	cut := len(payload) / 8
	return &ps2145Owner{file: file, meta: payload[:cut:cut], weights: payload[cut:]}, nil
}

//go:noinline
func ps2145LoadMapped(path string, mapper ps2145Mapper) (*ps2145Owner, bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	info, err := file.Stat()
	if err == nil && info.Mode().IsRegular() && info.Size() >= 8 {
		mapped, unmap, mapErr := mapper(file, int(info.Size()))
		if mapErr == nil && len(mapped) >= 8 && unmap != nil {
			size := binary.LittleEndian.Uint64(mapped[:8])
			if size == uint64(len(mapped)-8) {
				payload := mapped[8:]
				cut := len(payload) / 8
				return &ps2145Owner{
					file: file, mapped: mapped,
					meta: payload[:cut:cut], weights: payload[cut:], unmap: unmap,
				}, true, nil
			}
			_ = unmap()
		}
	}
	// Stat and the attempted mapping do not advance the file offset. Reuse the
	// same opened handle for the portable authoritative streaming fallback.
	reader := bufio.NewReader(file)
	var header [8]byte
	if _, err := io.ReadFull(reader, header[:]); err != nil {
		_ = file.Close()
		return nil, false, err
	}
	size := binary.LittleEndian.Uint64(header[:])
	if size > uint64(^uint(0)>>1) {
		_ = file.Close()
		return nil, false, errors.New("payload too large")
	}
	payload := make([]byte, int(size))
	if _, err := io.ReadFull(reader, payload); err != nil {
		_ = file.Close()
		return nil, false, err
	}
	cut := len(payload) / 8
	return &ps2145Owner{file: file, meta: payload[:cut:cut], weights: payload[cut:]}, false, nil
}

//go:noinline
func ps2145Digest(owner *ps2145Owner) uint64 {
	hash := fnv.New64a()
	_, _ = hash.Write(owner.meta)
	_, _ = hash.Write(owner.weights)
	return hash.Sum64()
}

func ps2145ConsumeAndClose(owner *ps2145Owner) (uint64, error) {
	digest := ps2145Digest(owner)
	runtime.KeepAlive(owner)
	return digest, owner.Close()
}

func ps2145Corpus(tb testing.TB, payloadBytes int) (string, []byte) {
	tb.Helper()
	directory := tb.TempDir()
	path := filepath.Join(directory, "model.bin")
	data := make([]byte, 8+payloadBytes)
	binary.LittleEndian.PutUint64(data[:8], uint64(payloadBytes))
	for index := 8; index < len(data); index++ {
		data[index] = byte(index*131 + index/251)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		tb.Fatal(err)
	}
	return path, data
}

func TestPS2145WorkPair(t *testing.T) {
	t.Parallel()
	path, original := ps2145Corpus(t, 2<<20)
	payload := original[8:]

	before, err := ps2145LoadBefore(path)
	if err != nil {
		t.Fatal(err)
	}
	ps2145AssertViews(t, before, payload)
	beforeDigest, err := ps2145ConsumeAndClose(before)
	if err != nil {
		t.Fatal(err)
	}
	ps2145AssertClosed(t, before)

	var mappedFile *os.File
	failedMapper := func(file *os.File, _ int) ([]byte, func() error, error) {
		mappedFile = file
		return nil, nil, errors.New("mapping unavailable")
	}
	fallback, mapped, err := ps2145LoadMapped(path, failedMapper)
	if err != nil || mapped {
		t.Fatalf("fallback: mapped=%v err=%v", mapped, err)
	}
	if fallback.file != mappedFile {
		t.Fatal("mapping fallback did not retain the exact *os.File passed to the mapper")
	}
	ps2145AssertViews(t, fallback, payload)
	fallbackDigest, err := ps2145ConsumeAndClose(fallback)
	if err != nil {
		t.Fatal(err)
	}
	ps2145AssertClosed(t, fallback)
	if fallbackDigest != beforeDigest {
		t.Fatalf("fallback digest = %x, want %x", fallbackDigest, beforeDigest)
	}

	if ps2145MappingSupported {
		after, mapped, err := ps2145LoadMapped(path, ps2145MapReadOnly)
		if err != nil || !mapped {
			t.Fatalf("mapping: mapped=%v err=%v", mapped, err)
		}
		ps2145AssertViews(t, after, payload)
		afterDigest, err := ps2145ConsumeAndClose(after)
		if err != nil {
			t.Fatal(err)
		}
		ps2145AssertClosed(t, after)
		if afterDigest != beforeDigest {
			t.Fatalf("mapped digest = %x, want %x", afterDigest, beforeDigest)
		}
	}

	unchanged, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(unchanged, original) {
		t.Fatal("read-only work pair modified the source file")
	}
}

func TestPS2145ErrorParity(t *testing.T) {
	t.Parallel()
	tooLarge := make([]byte, 8)
	binary.LittleEndian.PutUint64(tooLarge, uint64(^uint(0)>>1)+1)
	for _, test := range []struct {
		name string
		data []byte
	}{
		{"short header", []byte{1, 2, 3, 4}},
		{"short payload", append([]byte{8, 0, 0, 0, 0, 0, 0, 0}, []byte{1, 2, 3}...)},
		{"oversized declared length", tooLarge},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "malformed.bin")
			if err := os.WriteFile(path, test.data, 0o600); err != nil {
				t.Fatal(err)
			}
			before, beforeErr := ps2145LoadBefore(path)
			if before != nil || beforeErr == nil {
				t.Fatalf("streaming result: owner=%v err=%v", before, beforeErr)
			}

			for _, mapper := range []struct {
				name string
				mapf ps2145Mapper
			}{
				{"mapped validation", ps2145TestMapper},
				{"mapping fallback", func(*os.File, int) ([]byte, func() error, error) {
					return nil, nil, errors.New("mapping unavailable")
				}},
			} {
				owner, mapped, err := ps2145LoadMapped(path, mapper.mapf)
				if owner != nil || mapped || err == nil {
					t.Fatalf("%s result: owner=%v mapped=%v err=%v", mapper.name, owner, mapped, err)
				}
				if err.Error() != beforeErr.Error() {
					t.Errorf("%s error = %q, want streaming %q", mapper.name, err, beforeErr)
				}
			}
		})
	}
}

func ps2145TestMapper(file *os.File, _ int) ([]byte, func() error, error) {
	data, err := os.ReadFile(file.Name())
	if err != nil {
		return nil, nil, err
	}
	return data, func() error { return nil }, nil
}

func ps2145AssertViews(t *testing.T, owner *ps2145Owner, payload []byte) {
	t.Helper()
	cut := len(payload) / 8
	if len(owner.meta) != cut || !bytes.Equal(owner.meta, payload[:cut]) {
		t.Fatalf("meta bytes differ: got length %d, want %d", len(owner.meta), cut)
	}
	if len(owner.weights) != len(payload)-cut || !bytes.Equal(owner.weights, payload[cut:]) {
		t.Fatalf("weights bytes differ: got length %d, want %d", len(owner.weights), len(payload)-cut)
	}
}

func ps2145AssertClosed(t *testing.T, owner *ps2145Owner) {
	t.Helper()
	if owner.unmap != nil || owner.mapped != nil || owner.meta != nil || owner.weights != nil {
		t.Fatalf("Close retained mapping state: unmap=%v mapped=%v meta=%v weights=%v", owner.unmap != nil, owner.mapped, owner.meta, owner.weights)
	}
	if _, err := owner.file.Stat(); !ps2145ClosedHandleError(err) {
		t.Fatalf("file.Stat after Close = %v, want a platform closed-handle error", err)
	}
}

var ps2145DigestSink uint64

func BenchmarkPS2145_Before(b *testing.B) {
	path, _ := ps2145Corpus(b, 16<<20)
	b.ReportAllocs()
	b.SetBytes(16 << 20)
	b.ResetTimer()
	for range b.N {
		owner, err := ps2145LoadBefore(path)
		if err != nil {
			b.Fatal(err)
		}
		ps2145DigestSink, err = ps2145ConsumeAndClose(owner)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPS2145_After(b *testing.B) {
	if !ps2145MappingSupported {
		b.Skip("read-only mapping benchmark is supported on Darwin and Linux")
	}
	path, _ := ps2145Corpus(b, 16<<20)
	b.ReportAllocs()
	b.SetBytes(16 << 20)
	b.ResetTimer()
	for range b.N {
		owner, mapped, err := ps2145LoadMapped(path, ps2145MapReadOnly)
		if err != nil || !mapped {
			b.Fatalf("mapping: mapped=%v err=%v", mapped, err)
		}
		ps2145DigestSink, err = ps2145ConsumeAndClose(owner)
		if err != nil {
			b.Fatal(err)
		}
	}
}
