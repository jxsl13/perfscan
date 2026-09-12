package coefficientcodegen

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"os"
	"testing"
)

func words(instructions ...uint32) []byte {
	code := make([]byte, 4*len(instructions))
	for i, instruction := range instructions {
		binary.LittleEndian.PutUint32(code[4*i:], instruction)
	}
	return code
}

func TestFreshAddressTriples(t *testing.T) {
	t.Parallel()
	// Exact instructions from the pinned owner's first two coefficient loads.
	code := words(0xd0001a3b, 0x9128437b, 0x3dc00363, 0xd0001a3b, 0x9128037b, 0x3dc00364)
	loads, err := Decode(code, 0x1001bd9b8, map[uint64]string{0x100503a10: "cpu.c1", 0x100503a00: "cpu.c0"})
	if err != nil || len(loads) != 2 || loads[0].Symbol != "cpu.c1" || loads[1].Symbol != "cpu.c0" || loads[0].Page != loads[1].Page {
		t.Fatalf("loads=%+v err=%v", loads, err)
	}
	for _, reg := range []uint32{0, 5, 19, 30} {
		code := words(0x90000000|reg, 0x91040000|(reg<<5)|reg, 0x3dc00000|(reg<<5)|3)
		loads, err := Decode(code, 0x4000, map[uint64]string{0x4100: "coefficient"})
		if err != nil || len(loads) != 1 || loads[0].Address != 0x4100 {
			t.Fatalf("reg%d: %+v %v", reg, loads, err)
		}
	}
}

func TestEvidenceNegatives(t *testing.T) {
	t.Parallel()
	for name, code := range map[string][]byte{
		"retained base":       words(0x90000000, 0x3dc04003, 0x3dc04404),
		"add wrong register":  words(0x90000000, 0x91040001, 0x3dc00023),
		"load wrong register": words(0x90000000, 0x91040000, 0x3dc00023),
		"scalar load":         words(0x90000000, 0x91040000, 0xf9400003),
		"unresolved":          words(0x90000000, 0x91040400, 0x3dc00003),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			loads, err := Decode(code, 0x4000, map[uint64]string{0x4100: "coefficient"})
			if err != nil || len(loads) != 0 {
				t.Fatalf("%+v %v", loads, err)
			}
		})
	}
	for _, code := range [][]byte{{1}, words(0x94000000), words(0xd63f0000), words(0xf94003e0)} {
		if _, err := Decode(code, 0x4000, nil); err == nil {
			t.Fatal("accepted unsupported leaf")
		}
	}
	if _, err := Inspect(t.TempDir()+"/missing", "missing"); err == nil {
		t.Fatal("accepted missing binary")
	}
	for _, data := range [][]byte{[]byte("MZ"), []byte("\x7fELF"), []byte("\xcf\xfa\xed\xfe")} {
		if _, err := InspectBytes(data, "missing"); err == nil {
			t.Fatal("accepted PE/ELF/truncated Mach-O")
		}
	}
}

func TestPinnedOwnerBinary(t *testing.T) {
	t.Parallel()
	path := os.Getenv("PERFSCAN_PS6130_OWNER_BINARY")
	if path == "" {
		t.Skip("optional local owner artifact; not distributed")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	if hex.EncodeToString(digest[:]) != "b768ca10228a7487aad1cbef3a658c99cb21078b5059b815fa690b0d9bcc7614" {
		t.Fatal("original owner binary digest differs; a controlled rebuild is separate evidence")
	}
	loads, err := Inspect(path, "github.com/jxsl13/goai/backend/cpu.erfF64x2GELU")
	if err != nil || len(loads) < 20 {
		t.Fatalf("owner loads=%d err=%v", len(loads), err)
	}
	t.Logf("decoded %d resolved fresh coefficient/global loads", len(loads))
}
