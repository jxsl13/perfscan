package ps2145

import (
	"bufio"
	"encoding/binary"
	"errors"
	"io"
	"os"
)

var errTooLarge = errors.New("payload too large")

type model struct {
	meta    []byte
	weights []byte
}

func openDirect(path string) (*model, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parseDirect(f) // want `os\.Open file-path candidate is routed through a streaming parser`
}

func openBound(path string) (*model, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	result, err := parseDirect(f) // want `os\.Open file-path candidate is routed through a streaming parser`
	if err != nil {
		return nil, err
	}
	return result, nil
}

func parseDirect(r io.Reader) (*model, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, err
	}
	size := binary.LittleEndian.Uint32(header[:])
	payload := make([]byte, size)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return &model{meta: payload[: len(payload)/8 : len(payload)/8], weights: payload[len(payload)/8:]}, nil
}

func openBuffered(path string) (*model, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	reader := bufio.NewReader(f)
	return parseBuffered(reader) // want `os\.Open file-path candidate is routed through a streaming parser`
}

func parseBuffered(r *bufio.Reader) (*model, error) {
	header := make([]byte, 8)
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, err
	}
	size := binary.BigEndian.Uint64(header[:8])
	if size > uint64(^uint(0)>>1) {
		return nil, errTooLarge
	}
	count := size
	payload := make([]byte, int(count))
	if _, err := io.ReadFull(r, payload[:]); err != nil {
		return nil, err
	}
	weights := payload[1:]
	return &model{meta: payload[:1], weights: weights}, nil
}

func openLimited(path string) (model, error) {
	f, err := os.Open(path)
	if err != nil {
		return model{}, err
	}
	defer f.Close()
	return parseLimited(bufio.NewReaderSize(f, 32)) // want `os\.Open file-path candidate is routed through a streaming parser`
}

func openLateSmallGuard(path string) (*model, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	_, _ = f.Stat()
	return parseLateSmallGuard(f) // want `os\.Open file-path candidate is routed through a streaming parser`
}

// A bound installed after allocation cannot suppress the file-sized heap copy.
func parseLateSmallGuard(r io.Reader) (*model, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, err
	}
	size := binary.LittleEndian.Uint32(header[:])
	payload := make([]byte, size)
	if size > 1<<20 {
		return nil, errTooLarge
	}
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return &model{meta: payload[:1], weights: payload[1:]}, nil
}

func parseLimited(r *bufio.Reader) (model, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return model{}, err
	}
	size := binary.LittleEndian.Uint32(header[:4])
	payload, err := io.ReadAll(io.LimitReader(r, int64(size)))
	if err != nil {
		return model{}, err
	}
	return model{meta: payload[:1], weights: payload[1:]}, nil
}
