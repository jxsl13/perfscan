package ps2145neg

import (
	"bytes"
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

type boxed struct{ view any }

type owned struct{ view []byte }

func (*owned) Close() error { return nil }

func immediateClose(path string) (*model, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	_ = f.Close()
	return parse32(f)
}

func twoFiles(path, other string) (*model, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	g, err := os.Open(other)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	defer g.Close()
	return parse32(g)
}

func consumesResult(path string) (*model, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	result, err := parse32(f)
	return result, err
}

func returnsExistingOwner(path string) (*owned, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parseOwned(f)
}

func readAtFirst(path string) (*model, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var one [1]byte
	_, _ = f.ReadAt(one[:], 0)
	return parse32(f)
}

func parse32(r io.Reader) (*model, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, err
	}
	size := binary.LittleEndian.Uint32(header[:])
	payload := make([]byte, size)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return &model{meta: payload[:1], weights: payload[1:]}, nil
}

func parseOwned(r io.Reader) (*owned, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, err
	}
	size := binary.LittleEndian.Uint32(header[:])
	payload := make([]byte, size)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return &owned{view: payload[1:]}, nil
}

func small(path string) (*model, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parseSmall(f)
}

func parseSmall(r io.Reader) (*model, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, err
	}
	size := binary.LittleEndian.Uint32(header[:])
	if size > 1<<20 {
		return nil, errTooLarge
	}
	payload := make([]byte, size)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return &model{meta: payload[:1], weights: payload[1:]}, nil
}

func unchecked64(path string) (*model, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parseUnchecked64(f)
}

func parseUnchecked64(r io.Reader) (*model, error) {
	var header [8]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, err
	}
	size := binary.LittleEndian.Uint64(header[:])
	payload := make([]byte, int(size))
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return &model{meta: payload[:1], weights: payload[1:]}, nil
}

type order struct{}

func (order) Uint32([]byte) uint32 { return 2 << 20 }

func lookalike(path string) (*model, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parseLookalike(f)
}

func parseLookalike(r io.Reader) (*model, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, err
	}
	size := (order{}).Uint32(header[:])
	payload := make([]byte, size)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return &model{meta: payload[:1], weights: payload[1:]}, nil
}

func fullOnly(path string) (*model, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parseFullOnly(f)
}

func parseFullOnly(r io.Reader) (*model, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, err
	}
	size := binary.LittleEndian.Uint32(header[:])
	payload := make([]byte, size)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return &model{weights: payload}, nil
}

func copied(path string) (*model, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parseCopied(f)
}

func parseCopied(r io.Reader) (*model, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, err
	}
	size := binary.LittleEndian.Uint32(header[:])
	payload := make([]byte, size)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return &model{weights: bytes.Clone(payload[1:])}, nil
}

func mutated(path string) (*model, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parseMutated(f)
}

func parseMutated(r io.Reader) (*model, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, err
	}
	size := binary.LittleEndian.Uint32(header[:])
	payload := make([]byte, size)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	payload[0] = 0
	return &model{meta: payload[:1], weights: payload[1:]}, nil
}

func arithmetic(path string) (*model, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parseArithmetic(f)
}

func parseArithmetic(r io.Reader) (*model, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, err
	}
	size := binary.LittleEndian.Uint32(header[:])
	payload := make([]byte, size+1)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return &model{meta: payload[:1], weights: payload[1:]}, nil
}

func partialHeader(path string) (*model, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parsePartialHeader(f)
}

func parsePartialHeader(r io.Reader) (*model, error) {
	var header [8]byte
	if _, err := io.ReadFull(r, header[:4]); err != nil {
		return nil, err
	}
	size := binary.LittleEndian.Uint32(header[:4])
	payload := make([]byte, size)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return &model{meta: payload[:1], weights: payload[1:]}, nil
}

func wrongReader(path string) (*model, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parseWrongReader(f)
}

func parseWrongReader(r io.Reader) (*model, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, err
	}
	size := binary.LittleEndian.Uint32(header[:])
	payload := make([]byte, size)
	if _, err := io.ReadFull(bytes.NewReader(nil), payload); err != nil {
		return nil, err
	}
	return &model{meta: payload[:1], weights: payload[1:]}, nil
}

func uncheckedFill(path string) (*model, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parseUncheckedFill(f)
}

func parseUncheckedFill(r io.Reader) (*model, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, err
	}
	size := binary.LittleEndian.Uint32(header[:])
	payload := make([]byte, size)
	_, _ = io.ReadFull(r, payload)
	return &model{meta: payload[:1], weights: payload[1:]}, nil
}

func unbounded(path string) (*model, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parseUnbounded(f)
}

func boxedOutput(path string) (*boxed, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parseBoxed(f)
}

func parseBoxed(r io.Reader) (*boxed, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, err
	}
	size := binary.LittleEndian.Uint32(header[:])
	payload := make([]byte, size)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return &boxed{view: payload[1:]}, nil
}

func parseUnbounded(r io.Reader) (*model, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, err
	}
	_ = binary.LittleEndian.Uint32(header[:])
	payload, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return &model{meta: payload[:1], weights: payload[1:]}, nil
}

func fullEquivalent(path string) (*model, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parseFullEquivalent(f)
}

func parseFullEquivalent(r io.Reader) (*model, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, err
	}
	size := binary.LittleEndian.Uint32(header[:])
	payload := make([]byte, size)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return &model{weights: payload[0:]}, nil
}

func mutableSize(path string) (*model, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parseMutableSize(f)
}

func parseMutableSize(r io.Reader) (*model, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, err
	}
	size := binary.LittleEndian.Uint32(header[:])
	size = size / 2
	payload := make([]byte, size)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return &model{meta: payload[:1], weights: payload[1:]}, nil
}

func closureEscape(path string) (*model, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parseClosureEscape(f)
}

func parseClosureEscape(r io.Reader) (*model, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, err
	}
	size := binary.LittleEndian.Uint32(header[:])
	payload := make([]byte, size)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	deferred := func() int { return len(payload) }
	_ = deferred
	return &model{meta: payload[:1], weights: payload[1:]}, nil
}

func genericWrapper[T any](path string) (*model, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parse32(f)
}

func variadicParser(path string) (*model, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parseVariadic(f)
}

func parseVariadic(r io.Reader, _ ...int) (*model, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, err
	}
	size := binary.LittleEndian.Uint32(header[:])
	payload := make([]byte, size)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return &model{meta: payload[:1], weights: payload[1:]}, nil
}

func unreachableWrapper(path string) (*model, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return nil, nil
	return parse32(f)
}

func unreachableParserResult(path string) (*model, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parseUnreachableResult(f)
}

func parseUnreachableResult(r io.Reader) (*model, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, err
	}
	size := binary.LittleEndian.Uint32(header[:])
	payload := make([]byte, size)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return nil, nil
	return &model{meta: payload[:1], weights: payload[1:]}, nil
}

func nestedPartialHeader(path string) (*model, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parseNestedPartialHeader(f)
}

func parseNestedPartialHeader(r io.Reader) (*model, error) {
	var header [8]byte
	if _, err := io.ReadFull(r, header[:2][:]); err != nil {
		return nil, err
	}
	size := binary.LittleEndian.Uint32(header[:4])
	payload := make([]byte, size)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return &model{meta: payload[:1], weights: payload[1:]}, nil
}

func nestedPartialPayload(path string) (*model, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parseNestedPartialPayload(f)
}

func parseNestedPartialPayload(r io.Reader) (*model, error) {
	var header [8]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, err
	}
	size := binary.LittleEndian.Uint32(header[:4])
	payload := make([]byte, size)
	if _, err := io.ReadFull(r, payload[:1][:]); err != nil {
		return nil, err
	}
	return &model{meta: payload[:1], weights: payload[1:]}, nil
}

func nestedEndianPastExtent(path string) (*model, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parseNestedEndianPastExtent(f)
}

func parseNestedEndianPastExtent(r io.Reader) (*model, error) {
	var header [8]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, err
	}
	size := binary.LittleEndian.Uint32(header[6:][:4])
	payload := make([]byte, size)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return &model{meta: payload[:1], weights: payload[1:]}, nil
}

func constantTerminalWrapper(path string) (*model, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if true {
		return nil, io.EOF
	}
	return parse32(f)
}

func constantTerminalParser(path string) (*model, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parseConstantTerminal(f)
}

func parseConstantTerminal(r io.Reader) (*model, error) {
	if true {
		return nil, io.EOF
	}
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, err
	}
	size := binary.LittleEndian.Uint32(header[:])
	payload := make([]byte, size)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return &model{meta: payload[:1], weights: payload[1:]}, nil
}
