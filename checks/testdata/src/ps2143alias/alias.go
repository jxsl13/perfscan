package ps2143alias

import (
	b "bytes"
	j "encoding/json"
	"io"
	"os"
)

func parse(io.Reader) ([]int, error) { return nil, nil }

func load(f *os.File) (int, error) {
	payload := make([]byte, 32)
	_, _ = f.ReadAt(payload, 8)
	header, _ := j.Marshal(struct{ N int }{1})
	var synthetic b.Buffer
	_, _ = synthetic.Write(header)
	_, _ = synthetic.Write(payload)
	items, err := parse(b.NewReader(synthetic.Bytes())) // want `partial ReadAt payload is copied with a marshaled JSON header`
	return items[0], err
}
