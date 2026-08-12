package tidy

import (
	"bytes"
	"context"
	"errors"
	"testing"
)

func TestMajorVersion(t *testing.T) {
	orig := Executor
	defer func() { Executor = orig }()

	cases := []struct {
		name      string
		out       string
		runErr    error
		wantMajor int
		wantOK    bool
	}{
		{"homebrew", "Homebrew LLVM version 22.1.8\n", nil, 22, true},
		{"ubuntu", "Ubuntu LLVM version 18.1.3\nOptimized build.\n", nil, 18, true},
		{"plain", "LLVM version 20.0.0\n", nil, 20, true},
		{"unrecognized", "clang version 15\n", nil, 0, false},
		{"exec error", "", errors.New("not found"), 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			Executor = func(_ context.Context, _ []string, stdout, _ *bytes.Buffer) (int, error) {
				stdout.WriteString(tc.out)
				if tc.runErr != nil {
					return -1, tc.runErr
				}
				return 0, nil
			}
			major, ok := MajorVersion(context.Background(), "clang-tidy")
			if major != tc.wantMajor || ok != tc.wantOK {
				t.Errorf("MajorVersion = (%d, %v), want (%d, %v)", major, ok, tc.wantMajor, tc.wantOK)
			}
		})
	}
}
