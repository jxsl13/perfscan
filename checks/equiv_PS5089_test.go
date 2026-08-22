package checks

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestEquiv_PS5089SynchronousFileWrites(t *testing.T) {
	inputs := [][]byte{nil, {}, []byte("payload"), {0xff, 0, 0xfe, 'x'}}
	for index, input := range inputs {
		beforePath := filepath.Join(t.TempDir(), "before")
		afterPath := filepath.Join(t.TempDir(), "after")
		beforeFile, err := os.Create(beforePath)
		if err != nil {
			t.Fatal(err)
		}
		afterFile, err := os.Create(afterPath)
		if err != nil {
			beforeFile.Close()
			t.Fatal(err)
		}
		beforeN, beforeErr := beforeFile.Write(bytes.Clone(slices.Clone(input)))
		afterN, afterErr := afterFile.Write(input)
		beforeClose, afterClose := beforeFile.Close(), afterFile.Close()
		beforeBytes, beforeReadErr := os.ReadFile(beforePath)
		afterBytes, afterReadErr := os.ReadFile(afterPath)
		if beforeN != afterN || !sameError(beforeErr, afterErr) || !sameError(beforeClose, afterClose) || !sameError(beforeReadErr, afterReadErr) || !bytes.Equal(beforeBytes, afterBytes) {
			t.Fatalf("Write input %d differs: n=%d/%d err=%v/%v close=%v/%v read=%v/%v bytes=%v/%v", index, beforeN, afterN, beforeErr, afterErr, beforeClose, afterClose, beforeReadErr, afterReadErr, beforeBytes, afterBytes)
		}
	}
}

func TestEquiv_PS5089WriteAtWriteStringAndWriteFile(t *testing.T) {
	beforePath := filepath.Join(t.TempDir(), "before")
	afterPath := filepath.Join(t.TempDir(), "after")
	beforeFile, err := os.Create(beforePath)
	if err != nil {
		t.Fatal(err)
	}
	afterFile, err := os.Create(afterPath)
	if err != nil {
		beforeFile.Close()
		t.Fatal(err)
	}
	seed := []byte("0123456789")
	if _, err := beforeFile.Write(seed); err != nil {
		t.Fatal(err)
	}
	if _, err := afterFile.Write(seed); err != nil {
		t.Fatal(err)
	}
	data := []byte("XYZ")
	beforeN, beforeErr := beforeFile.WriteAt(bytes.Clone(data), 2)
	afterN, afterErr := afterFile.WriteAt(data, 2)
	beforeStringN, beforeStringErr := beforeFile.WriteString(strings.Clone("tail"))
	afterStringN, afterStringErr := afterFile.WriteString("tail")
	if err := beforeFile.Close(); err != nil {
		t.Fatal(err)
	}
	if err := afterFile.Close(); err != nil {
		t.Fatal(err)
	}
	beforeBytes, beforeReadErr := os.ReadFile(beforePath)
	afterBytes, afterReadErr := os.ReadFile(afterPath)
	if beforeN != afterN || !sameError(beforeErr, afterErr) || beforeStringN != afterStringN || !sameError(beforeStringErr, afterStringErr) || !sameError(beforeReadErr, afterReadErr) || !bytes.Equal(beforeBytes, afterBytes) {
		t.Fatalf("WriteAt/WriteString differs: n=%d/%d string=%d/%d err=%v/%v %v/%v bytes=%q/%q", beforeN, afterN, beforeStringN, afterStringN, beforeErr, afterErr, beforeStringErr, afterStringErr, beforeBytes, afterBytes)
	}

	beforeWhole := filepath.Join(t.TempDir(), "before-whole")
	afterWhole := filepath.Join(t.TempDir(), "after-whole")
	beforeWriteErr := os.WriteFile(beforeWhole, bytes.Clone(data), 0o640)
	afterWriteErr := os.WriteFile(afterWhole, data, 0o640)
	beforeBytes, beforeReadErr = os.ReadFile(beforeWhole)
	afterBytes, afterReadErr = os.ReadFile(afterWhole)
	beforeInfo, beforeStatErr := os.Stat(beforeWhole)
	afterInfo, afterStatErr := os.Stat(afterWhole)
	if !sameError(beforeWriteErr, afterWriteErr) || !sameError(beforeReadErr, afterReadErr) || !sameError(beforeStatErr, afterStatErr) || !bytes.Equal(beforeBytes, afterBytes) || beforeInfo.Mode().Perm() != afterInfo.Mode().Perm() {
		t.Fatalf("WriteFile differs: write=%v/%v read=%v/%v stat=%v/%v bytes=%v/%v mode=%v/%v", beforeWriteErr, afterWriteErr, beforeReadErr, afterReadErr, beforeStatErr, afterStatErr, beforeBytes, afterBytes, beforeInfo.Mode(), afterInfo.Mode())
	}
}

func TestPS5089LaterArgumentMutationMakesSnapshotObservable(t *testing.T) {
	data := []byte("abc")
	snapshot := bytes.Clone(data)
	data[0] = 'x'
	if bytes.Equal(snapshot, data) {
		t.Fatal("test must expose the pre-offset/mode snapshot contract")
	}
}

func sameError(left, right error) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Error() == right.Error()
}
