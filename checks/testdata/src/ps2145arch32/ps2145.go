package ps2145arch32

import (
	"encoding/binary"
	"errors"
	"io"
	"os"
)

var errTooLarge = errors.New("payload too large")

type model struct{ view []byte }

func openChecked(path string) (*model, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parseChecked(f) // want `os\.Open file-path candidate is routed through a streaming parser`
}

func parseChecked(r io.Reader) (*model, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, err
	}
	size := binary.LittleEndian.Uint32(header[:])
	if size > 1<<30 {
		return nil, errTooLarge
	}
	payload := make([]byte, int(size))
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return &model{view: payload[1:]}, nil
}

func openUnchecked(path string) (*model, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parseUnchecked(f)
}

func parseUnchecked(r io.Reader) (*model, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, err
	}
	size := binary.LittleEndian.Uint32(header[:])
	payload := make([]byte, int(size))
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return &model{view: payload[1:]}, nil
}
