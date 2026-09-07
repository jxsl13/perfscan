package ps2145alias

import (
	b "bufio"
	bin "encoding/binary"
	i "io"
	o "os"
)

type result struct{ view []byte }

func open(path string) (*result, error) {
	f, err := o.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parse(b.NewReader(f)) // want `os\.Open file-path candidate is routed through a streaming parser`
}

func parse(r *b.Reader) (*result, error) {
	var header [4]byte
	if _, err := i.ReadFull(r, header[:]); err != nil {
		return nil, err
	}
	size := bin.BigEndian.Uint32(header[:])
	payload := make([]byte, size)
	if _, err := i.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return &result{view: payload[1:]}, nil
}
