package main

import (
	"os"
	"strings"
	"testing"
)

// memReadFS is a minimal in-memory diffpkg.FS (ReadFile+WriteFile) for exercising
// verifyRestored without touching disk. err[path] forces a read error.
type memReadFS struct {
	data map[string][]byte
	err  map[string]error
}

func (m memReadFS) ReadFile(p string) ([]byte, error) {
	if e := m.err[p]; e != nil {
		return nil, e
	}
	b, ok := m.data[p]
	if !ok {
		return nil, os.ErrNotExist
	}
	return b, nil
}

func (m memReadFS) WriteFile(p string, d []byte) error { m.data[p] = d; return nil }

// TestVerifyRestored pins the last -diff safety net: after the snapshot->fix->
// restore cycle, verifyRestored re-reads every touched file and must ERROR if any
// is not byte-identical to its snapshot — so a botched restore makes -diff exit
// non-zero instead of silently leaving a user's file modified. Only the happy
// path was covered (via internal/diff's TestBuildDiffsAndRestores); this pins the
// failure branches.
func TestVerifyRestored(t *testing.T) {
	snaps := map[string][]byte{"/a.cpp": []byte("x"), "/b.cpp": []byte("y")}

	// All files restored correctly -> nil.
	ok := memReadFS{data: map[string][]byte{"/a.cpp": []byte("x"), "/b.cpp": []byte("y")}}
	if err := verifyRestored(ok, snaps); err != nil {
		t.Errorf("clean restore: want nil, got %v", err)
	}

	// A file left MODIFIED -> a "left modified" error (the corruption guard).
	modified := memReadFS{data: map[string][]byte{"/a.cpp": []byte("x"), "/b.cpp": []byte("MODIFIED")}}
	if err := verifyRestored(modified, snaps); err == nil || !strings.Contains(err.Error(), "left modified") {
		t.Errorf("modified file: want a 'left modified' error, got %v", err)
	}

	// A file unreadable during verify -> the read error is surfaced, not swallowed.
	unreadable := memReadFS{data: map[string][]byte{"/a.cpp": []byte("x")}, err: map[string]error{"/b.cpp": os.ErrPermission}}
	if err := verifyRestored(unreadable, snaps); err == nil || !strings.Contains(err.Error(), "verifying restore of /b.cpp") {
		t.Errorf("unreadable file: want a verify error naming the file, got %v", err)
	}
}
