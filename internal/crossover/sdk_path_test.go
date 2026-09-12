package crossover

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSDKPathIdentityBoundaries(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	physical, err := canonicalSDKPath(root)
	if err != nil {
		t.Fatal(err)
	}
	if !sameObservedSDK(root, physical, "go1.27") {
		t.Fatal("same physical SDK rejected")
	}
	if sameObservedSDK(t.TempDir(), physical, "go1.27") || sameObservedSDK(root, physical, "") || sameObservedSDK(filepath.Join(root, "missing"), physical, "go1.27") {
		t.Fatal("wrong, versionless or missing SDK accepted")
	}
	if _, err := canonicalSDKPath(filepath.Join(root, "missing-go")); err == nil {
		t.Fatal("missing SDK path accepted")
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	for _, binary := range []string{executable, root, filepath.Join(root, "missing-go")} {
		if _, _, err := (&BuildSelection{GoBinary: binary}).Environment(ctx); err == nil {
			t.Fatal("non-SDK wrapper, directory or missing executable accepted")
		}
	}
}
