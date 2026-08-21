package ps5094shadow

import "bytes"

func converted(data []uint8) any {
	string := func([]uint8) any { return nil }
	_ = string
	return bytes.NewBuffer(data).String() // want "bytes.NewBuffer[(]...[)].String constructs an ephemeral Buffer"
}

func copied() []uint8 {
	type byte uint8
	return bytes.NewBufferString("payload").Bytes() // want "bytes.NewBufferString[(]...[)].Bytes constructs an ephemeral Buffer"
}
