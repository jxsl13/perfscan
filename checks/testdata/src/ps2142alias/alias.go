package ps2142alias

import (
	i "io"
	o "os"
)

func decode([]byte) error { return nil }

func load(path string) error {
	f, err := o.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	b, err := i.ReadAll(f) // want `os\.Open file is fully heap-staged by io\.ReadAll before decode`
	if err != nil {
		return err
	}
	return decode(b)
}
