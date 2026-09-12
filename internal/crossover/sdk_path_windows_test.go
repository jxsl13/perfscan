package crossover

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestSDKPathWindowsJunction(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	selected, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(selected)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	sdk := filepath.Join(root, "physical-sdk")
	bin := filepath.Join(sdk, "bin")
	if err = os.MkdirAll(bin, 0700); err != nil {
		t.Fatal(err)
	}
	goBinary := filepath.Join(bin, "go.exe")
	if err = os.WriteFile(goBinary, data, 0700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "sdk-junction")
	if out, err := exec.CommandContext(ctx, "cmd.exe", "/d", "/c", "mklink", "/J", alias, sdk).CombinedOutput(); err != nil {
		t.Fatalf("task-owned ordinary junction: %v: %s", err, out)
	}
	physical, err := canonicalSDKPath(sdk)
	if err != nil {
		t.Fatal(err)
	}
	got, err := canonicalSDKPath(alias)
	if err != nil || got != physical {
		t.Fatalf("junction SDK changed physical identity: %q %v", got, err)
	}
	if !sameObservedSDK(alias, physical, "go1.27") {
		t.Fatal("junction observation rejected same SDK")
	}
	env, resolved, err := (&BuildSelection{Root: root, GoBinary: filepath.Join(alias, "bin", "go.exe")}).Environment(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want, err := canonicalSDKPath(goBinary)
	if err != nil {
		t.Fatal(err)
	}
	if resolved != want || len(env) == 0 {
		t.Fatal("junction executable lost pinned physical SDK identity")
	}
}
