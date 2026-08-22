package checks

import (
	"bytes"
	"crypto/sha3"
	"encoding/hex"
	"slices"
	"testing"
	"unicode"
	"unicode/utf16"
)

func TestEquiv_PS5086ByteTransformers(t *testing.T) {
	inputs := [][]byte{nil, {}, []byte("already UPPER"), []byte("mixed Lower 世界"), {0xff, 'a', 0xfe, 'Z'}}
	for index, input := range inputs {
		assertBytes := func(name string, before, after []byte) {
			t.Helper()
			if !bytes.Equal(before, after) || (before == nil) != (after == nil) || cap(before) != cap(after) {
				t.Fatalf("input %d %s differs: %v/%v nil=%v/%v cap=%d/%d", index, name, before, after, before == nil, after == nil, cap(before), cap(after))
			}
		}
		assertBytes("upper", bytes.ToUpper(bytes.Clone(input)), bytes.ToUpper(input))
		assertBytes("lower", bytes.ToLower(bytes.Clone(input)), bytes.ToLower(input))
		assertBytes("title", bytes.ToTitle(bytes.Clone(input)), bytes.ToTitle(input))
		assertBytes("upper-special", bytes.ToUpperSpecial(unicode.TurkishCase, bytes.Clone(input)), bytes.ToUpperSpecial(unicode.TurkishCase, input))
		assertBytes("valid", bytes.ToValidUTF8(bytes.Clone(input), bytes.Clone([]byte("?"))), bytes.ToValidUTF8(input, []byte("?")))
		assertBytes("repeat", bytes.Repeat(bytes.Clone(input), 2), bytes.Repeat(input, 2))
		assertBytes("replace", bytes.Replace(bytes.Clone(input), bytes.Clone([]byte("a")), bytes.Clone([]byte("xyz")), 2), bytes.Replace(input, []byte("a"), []byte("xyz"), 2))
		assertBytes("replace-all", bytes.ReplaceAll(bytes.Clone(input), bytes.Clone([]byte("a")), bytes.Clone([]byte("xyz"))), bytes.ReplaceAll(input, []byte("a"), []byte("xyz")))

		beforeRunes := bytes.Runes(bytes.Clone(input))
		afterRunes := bytes.Runes(input)
		if !slices.Equal(beforeRunes, afterRunes) || (beforeRunes == nil) != (afterRunes == nil) || cap(beforeRunes) != cap(afterRunes) {
			t.Fatalf("input %d runes differ: %v/%v", index, beforeRunes, afterRunes)
		}
	}

	parts := [][]byte{nil, []byte("a"), []byte("世界")}
	beforeJoin := bytes.Join(slices.Clone(parts), bytes.Clone([]byte("/")))
	afterJoin := bytes.Join(parts, []byte("/"))
	if !bytes.Equal(beforeJoin, afterJoin) || cap(beforeJoin) != cap(afterJoin) {
		t.Fatalf("join differs: %q/%q cap=%d/%d", beforeJoin, afterJoin, cap(beforeJoin), cap(afterJoin))
	}

	beforeSHAKE := sha3.SumSHAKE256(bytes.Clone([]byte("payload")), 73)
	afterSHAKE := sha3.SumSHAKE256([]byte("payload"), 73)
	if !bytes.Equal(beforeSHAKE, afterSHAKE) || cap(beforeSHAKE) != cap(afterSHAKE) {
		t.Fatalf("SHAKE differs: %x/%x cap=%d/%d", beforeSHAKE, afterSHAKE, cap(beforeSHAKE), cap(afterSHAKE))
	}
	dumpInput := []byte{'p', 'a', 'y', 'l', 'o', 'a', 'd', 0, 0xff}
	if before, after := hex.Dump(bytes.Clone(dumpInput)), hex.Dump(dumpInput); before != after {
		t.Fatalf("hex.Dump differs: %q/%q", before, after)
	}
}

func TestEquiv_PS5086GenericSliceConstructors(t *testing.T) {
	inputs := [][]int{nil, {}, {1}, {1, 2, 3}}
	for index, input := range inputs {
		beforeRepeat := slices.Repeat(slices.Clone(input), 3)
		afterRepeat := slices.Repeat(input, 3)
		if !slices.Equal(beforeRepeat, afterRepeat) || (beforeRepeat == nil) != (afterRepeat == nil) || cap(beforeRepeat) != cap(afterRepeat) {
			t.Fatalf("repeat %d differs: %v/%v", index, beforeRepeat, afterRepeat)
		}

		beforeConcat := slices.Concat(slices.Clone(input), slices.Clone([]int{4, 5}))
		afterConcat := slices.Concat(input, []int{4, 5})
		if !slices.Equal(beforeConcat, afterConcat) || (beforeConcat == nil) != (afterConcat == nil) || cap(beforeConcat) != cap(afterConcat) {
			t.Fatalf("concat %d differs: %v/%v", index, beforeConcat, afterConcat)
		}
	}
}

func TestPS5086LaterArgumentMutationMakesSnapshotObservable(t *testing.T) {
	mutateAndNeedle := func(data []byte) []byte {
		data[0] = 'x'
		return []byte("a")
	}
	beforeInput := []byte("abc")
	before := bytes.Replace(bytes.Clone(beforeInput), mutateAndNeedle(beforeInput), []byte("z"), -1)
	hypotheticalInput := []byte("abc")
	hypothetical := bytes.Replace(hypotheticalInput, mutateAndNeedle(hypotheticalInput), []byte("z"), -1)
	if bytes.Equal(before, hypothetical) {
		t.Fatalf("test must expose the unsafe rewrite: snapshot=%q direct=%q", before, hypothetical)
	}
}

func TestEquiv_PS5086UTF16Transformers(t *testing.T) {
	runes := []rune{0, 'a', rune(0xFFFD), rune(0x1F642), -1, unicode.MaxRune + 1}
	beforeEncoded := utf16.Encode(slices.Clone(runes))
	afterEncoded := utf16.Encode(runes)
	if !slices.Equal(beforeEncoded, afterEncoded) || cap(beforeEncoded) != cap(afterEncoded) {
		t.Fatalf("utf16.Encode differs: %v/%v cap=%d/%d", beforeEncoded, afterEncoded, cap(beforeEncoded), cap(afterEncoded))
	}
	words := []uint16{0, 'a', 0xd83d, 0xde42, 0xd800, 0xffff}
	beforeDecoded := utf16.Decode(slices.Clone(words))
	afterDecoded := utf16.Decode(words)
	if !slices.Equal(beforeDecoded, afterDecoded) || cap(beforeDecoded) != cap(afterDecoded) {
		t.Fatalf("utf16.Decode differs: %v/%v cap=%d/%d", beforeDecoded, afterDecoded, cap(beforeDecoded), cap(afterDecoded))
	}
}
