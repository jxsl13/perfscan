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
		// The version regex captures \d+, but an absurd run of digits overflows
		// int64 so strconv.Atoi fails -- MajorVersion must degrade to (0,false)
		// rather than propagate the parse error (pins the Atoi guard, line 48).
		{"overflow digits", "LLVM version 99999999999999999999\n", nil, 0, false},
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

// TestMajorVersionEmptyBinaryDefaults pins that an empty binary name falls back
// to "clang-tidy" — the argv the Executor receives must start with it.
func TestMajorVersionEmptyBinaryDefaults(t *testing.T) {
	orig := Executor
	defer func() { Executor = orig }()

	var gotArgv []string
	Executor = func(_ context.Context, argv []string, stdout, _ *bytes.Buffer) (int, error) {
		gotArgv = argv
		stdout.WriteString("LLVM version 22.0.0\n")
		return 0, nil
	}
	major, ok := MajorVersion(context.Background(), "")
	if !ok || major != 22 {
		t.Fatalf("MajorVersion(\"\") = (%d, %v), want (22, true)", major, ok)
	}
	if len(gotArgv) == 0 || gotArgv[0] != "clang-tidy" {
		t.Errorf("empty binary must default to clang-tidy; argv = %v", gotArgv)
	}
}

// TestExperimentalUnsupported pins the stderr-signature detector that lets the
// caller degrade (drop custom checks, re-run built-ins) when a clang-tidy too old
// for --experimental-custom-checks — but whose version we could NOT parse, so the
// numeric gate missed it — rejects that flag and aborts. A false negative here
// would let the empty payload misreport a failed run as "clean".
func TestExperimentalUnsupported(t *testing.T) {
	yes := []string{
		"error: clang-tidy: Unknown command line argument '--experimental-custom-checks'.  Try: 'clang-tidy --help'\n",
		"clang-tidy: Unknown command line argument '--experimental-custom-checks'.",
	}
	for _, s := range yes {
		if !ExperimentalUnsupported(s) {
			t.Errorf("ExperimentalUnsupported(%q) = false, want true", s)
		}
	}
	no := []string{
		"",
		"1 warning generated.\n",
		// An unknown OTHER flag must not be mistaken for the experimental one.
		"error: clang-tidy: Unknown command line argument '--not-a-real-flag'.\n",
		// The flag named in a non-error line (e.g. our own help text) is not a rejection.
		"perfscanxx passes --experimental-custom-checks for query-based checks\n",
	}
	for _, s := range no {
		if ExperimentalUnsupported(s) {
			t.Errorf("ExperimentalUnsupported(%q) = true, want false", s)
		}
	}
}
