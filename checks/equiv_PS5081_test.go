package checks

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha3"
	"crypto/subtle"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"hash/maphash"
	"maps"
	"net/http"
	"slices"
	"testing"
	"unicode/utf8"
)

func TestEquiv_PS5081CloneFedObservers(t *testing.T) {
	byteInputs := [][]byte{
		nil,
		{},
		[]byte("payload"),
		[]byte("alpha-beta-alpha"),
		{0xff, 'a', 0xfe},
	}
	seed := maphash.MakeSeed()
	for ai, a := range byteInputs {
		for bi, b := range byteInputs {
			beforeEqual := bytes.Equal(bytes.Clone(slices.Clone(bytes.Clone(a))), slices.Clone(b))
			afterEqual := bytes.Equal(a, b)
			if beforeEqual != afterEqual {
				t.Fatalf("bytes.Equal input %d/%d: clone=%v direct=%v", ai, bi, beforeEqual, afterEqual)
			}
			beforeCompare := bytes.Compare(bytes.Clone(a), slices.Clone(b))
			afterCompare := bytes.Compare(a, b)
			if beforeCompare != afterCompare {
				t.Fatalf("bytes.Compare input %d/%d: clone=%d direct=%d", ai, bi, beforeCompare, afterCompare)
			}
		}
		if before, after := utf8.Valid(bytes.Clone(a)), utf8.Valid(a); before != after {
			t.Fatalf("utf8.Valid input %d: clone=%v direct=%v", ai, before, after)
		}
		if before, after := utf8.RuneCount(slices.Clone(a)), utf8.RuneCount(a); before != after {
			t.Fatalf("utf8.RuneCount input %d: clone=%d direct=%d", ai, before, after)
		}
		if before, after := sha256.Sum256(bytes.Clone(a)), sha256.Sum256(a); before != after {
			t.Fatalf("sha256 input %d differs", ai)
		}
		if before, after := sha3.Sum256(bytes.Clone(a)), sha3.Sum256(a); before != after {
			t.Fatalf("sha3 input %d differs", ai)
		}
		if before, after := base64.StdEncoding.EncodeToString(bytes.Clone(a)), base64.StdEncoding.EncodeToString(a); before != after {
			t.Fatalf("base64 input %d: clone=%q direct=%q", ai, before, after)
		}
		if before, after := maphash.Bytes(seed, bytes.Clone(a)), maphash.Bytes(seed, a); before != after {
			t.Fatalf("maphash input %d: clone=%d direct=%d", ai, before, after)
		}
		beforeVarint, beforeVarintN := binary.Varint(bytes.Clone(a))
		afterVarint, afterVarintN := binary.Varint(a)
		if beforeVarint != afterVarint || beforeVarintN != afterVarintN {
			t.Fatalf("binary.Varint input %d differs: %d,%d / %d,%d", ai, beforeVarint, beforeVarintN, afterVarint, afterVarintN)
		}
		beforeUvarint, beforeUvarintN := binary.Uvarint(bytes.Clone(a))
		afterUvarint, afterUvarintN := binary.Uvarint(a)
		if beforeUvarint != afterUvarint || beforeUvarintN != afterUvarintN {
			t.Fatalf("binary.Uvarint input %d differs: %d,%d / %d,%d", ai, beforeUvarint, beforeUvarintN, afterUvarint, afterUvarintN)
		}
		if before, after := http.DetectContentType(bytes.Clone(a)), http.DetectContentType(a); before != after {
			t.Fatalf("http.DetectContentType input %d differs: %q/%q", ai, before, after)
		}
	}

	sliceInputs := [][]int{nil, {}, {1}, {1, 2, 3}, {1, 2, 2, 4}}
	for ai, a := range sliceInputs {
		for bi, b := range sliceInputs {
			if before, after := slices.Equal(slices.Clone(a), slices.Clone(b)), slices.Equal(a, b); before != after {
				t.Fatalf("slices.Equal input %d/%d differs", ai, bi)
			}
			if before, after := slices.Compare(slices.Clone(a), slices.Clone(b)), slices.Compare(a, b); before != after {
				t.Fatalf("slices.Compare input %d/%d: clone=%d direct=%d", ai, bi, before, after)
			}
		}
		if len(a) > 0 {
			beforeValue, beforeFound := slices.BinarySearch(slices.Clone(a), 2)
			afterValue, afterFound := slices.BinarySearch(a, 2)
			if beforeValue != afterValue || beforeFound != afterFound {
				t.Fatalf("slices.BinarySearch input %d differs", ai)
			}
		}
	}

	mapInputs := []map[string]int{
		nil,
		{},
		{"a": 1},
		{"a": 1, "b": 2},
	}
	for ai, a := range mapInputs {
		for bi, b := range mapInputs {
			if before, after := maps.Equal(maps.Clone(a), maps.Clone(b)), maps.Equal(a, b); before != after {
				t.Fatalf("maps.Equal input %d/%d differs", ai, bi)
			}
		}
	}

	seedBytes := make([]byte, ed25519.SeedSize)
	for index := range seedBytes {
		seedBytes[index] = byte(index)
	}
	privateKey := ed25519.NewKeyFromSeed(seedBytes)
	publicKey := privateKey.Public().(ed25519.PublicKey)
	message := []byte("clone-fed signature verification")
	signature := ed25519.Sign(privateKey, message)
	if before, after := ed25519.Verify(slices.Clone(publicKey), bytes.Clone(message), slices.Clone(signature)), ed25519.Verify(publicKey, message, signature); before != after || !after {
		t.Fatalf("ed25519 Verify differs: clone=%v direct=%v", before, after)
	}
	if before, after := hmac.Equal(bytes.Clone(message), slices.Clone(message)), hmac.Equal(message, message); before != after || !after {
		t.Fatalf("hmac.Equal differs: clone=%v direct=%v", before, after)
	}
	if before, after := subtle.ConstantTimeCompare(bytes.Clone(message), slices.Clone(message)), subtle.ConstantTimeCompare(message, message); before != after || after != 1 {
		t.Fatalf("subtle.ConstantTimeCompare differs: clone=%d direct=%d", before, after)
	}
	values := []uint32{0, 1, 0xffffffff}
	if before, after := binary.Size(slices.Clone(values)), binary.Size(values); before != after {
		t.Fatalf("binary.Size differs: clone=%d direct=%d", before, after)
	}
}

func TestPS5081LaterArgumentMutationMakesSnapshotObservable(t *testing.T) {
	mutate := func(data []byte) []byte {
		data[0] ^= 0xff
		return data
	}
	beforeData := []byte{1}
	before := bytes.Equal(bytes.Clone(beforeData), mutate(beforeData))
	afterData := []byte{1}
	after := bytes.Equal(afterData, mutate(afterData))
	if before == after {
		t.Fatalf("expected early Clone snapshot to be observable across later mutation: before=%v after=%v", before, after)
	}
}

func TestEquiv_PS5081ValidationAndSignatureObservers(t *testing.T) {
	jsonInputs := [][]byte{
		nil,
		{},
		[]byte(`null`),
		[]byte(`{"message":"clone-fed validation","ok":true}`),
		[]byte(`{"unterminated":`),
		{0xff, 0xfe},
	}
	for index, input := range jsonInputs {
		before := json.Valid(bytes.Clone(input))
		after := json.Valid(input)
		if before != after {
			t.Fatalf("json.Valid input %d differs: clone=%v direct=%v", index, before, after)
		}
	}

	seed := make([]byte, ed25519.SeedSize)
	for index := range seed {
		seed[index] = byte(index)
	}
	edPrivate := ed25519.NewKeyFromSeed(seed)
	edPublic := edPrivate.Public().(ed25519.PublicKey)
	message := []byte("clone-fed signature verification")
	edSignature := ed25519.Sign(edPrivate, message)
	edOptions := &ed25519.Options{}
	beforeEd := ed25519.VerifyWithOptions(slices.Clone(edPublic), bytes.Clone(message), slices.Clone(edSignature), edOptions)
	afterEd := ed25519.VerifyWithOptions(edPublic, message, edSignature, edOptions)
	if !sameError(beforeEd, afterEd) || afterEd != nil {
		t.Fatalf("ed25519.VerifyWithOptions differs: clone=%v direct=%v", beforeEd, afterEd)
	}

	digest := sha256.Sum256(message)
	ecdsaPrivate, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ecdsaR, ecdsaS, err := ecdsa.Sign(rand.Reader, ecdsaPrivate, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	ecdsaSignature, err := ecdsa.SignASN1(rand.Reader, ecdsaPrivate, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	if before, after := ecdsa.Verify(&ecdsaPrivate.PublicKey, bytes.Clone(digest[:]), ecdsaR, ecdsaS), ecdsa.Verify(&ecdsaPrivate.PublicKey, digest[:], ecdsaR, ecdsaS); before != after || !after {
		t.Fatalf("ecdsa.Verify differs: clone=%v direct=%v", before, after)
	}
	if before, after := ecdsa.VerifyASN1(&ecdsaPrivate.PublicKey, bytes.Clone(digest[:]), slices.Clone(ecdsaSignature)), ecdsa.VerifyASN1(&ecdsaPrivate.PublicKey, digest[:], ecdsaSignature); before != after || !after {
		t.Fatalf("ecdsa.VerifyASN1 differs: clone=%v direct=%v", before, after)
	}

	rsaPrivate, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	pkcsSignature, err := rsa.SignPKCS1v15(rand.Reader, rsaPrivate, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	pssSignature, err := rsa.SignPSS(rand.Reader, rsaPrivate, crypto.SHA256, digest[:], nil)
	if err != nil {
		t.Fatal(err)
	}
	beforePKCS := rsa.VerifyPKCS1v15(&rsaPrivate.PublicKey, crypto.SHA256, bytes.Clone(digest[:]), slices.Clone(pkcsSignature))
	afterPKCS := rsa.VerifyPKCS1v15(&rsaPrivate.PublicKey, crypto.SHA256, digest[:], pkcsSignature)
	if !sameError(beforePKCS, afterPKCS) || afterPKCS != nil {
		t.Fatalf("rsa.VerifyPKCS1v15 differs: clone=%v direct=%v", beforePKCS, afterPKCS)
	}
	beforePSS := rsa.VerifyPSS(&rsaPrivate.PublicKey, crypto.SHA256, slices.Clone(digest[:]), bytes.Clone(pssSignature), nil)
	afterPSS := rsa.VerifyPSS(&rsaPrivate.PublicKey, crypto.SHA256, digest[:], pssSignature, nil)
	if !sameError(beforePSS, afterPSS) || afterPSS != nil {
		t.Fatalf("rsa.VerifyPSS differs: clone=%v direct=%v", beforePSS, afterPSS)
	}

	certificate := &x509.Certificate{PublicKey: &rsaPrivate.PublicKey}
	beforeX509 := certificate.CheckSignature(x509.SHA256WithRSA, bytes.Clone(message), slices.Clone(pkcsSignature))
	afterX509 := certificate.CheckSignature(x509.SHA256WithRSA, message, pkcsSignature)
	if !sameError(beforeX509, afterX509) || afterX509 != nil {
		t.Fatalf("x509.CheckSignature differs: clone=%v direct=%v", beforeX509, afterX509)
	}
}
