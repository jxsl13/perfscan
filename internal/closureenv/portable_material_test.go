package closureenv

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestPortableMaterialObservedIdentitySurvivesCheckoutRelocation(t *testing.T) {
	t.Parallel()
	collect := func(root, body string) [32]byte {
		for name, text := range map[string]string{"go.mod": "module example.test/input\ngo 1.25\n", "leaf.go": "package input\n", "worker_test.go": body, "leaf.s": "// native body\n", "input.dat": "embedded bytes"} {
			if err := os.WriteFile(filepath.Join(root, name), []byte(text), 0600); err != nil {
				t.Fatal(err)
			}
		}
		record := map[string]any{"ImportPath": "example.test/input", "Dir": root, "GoFiles": []string{"leaf.go"}, "TestGoFiles": []string{"worker_test.go"}, "SFiles": []string{"leaf.s"}, "EmbedFiles": []string{"input.dat"}, "Module": map[string]string{"Path": "example.test/input", "Dir": root, "GoMod": filepath.Join(root, "go.mod")}}
		encoded, err := json.Marshal(record)
		if err != nil {
			t.Fatal(err)
		}
		digest, err := binaryPortableMaterialDigest(encoded)
		if err != nil {
			t.Fatal(err)
		}
		return digest
	}
	first := collect(t.TempDir(), "package input\n// same test instrumentation\n")
	second := collect(t.TempDir(), "package input\n// same test instrumentation\n")
	if first != second {
		t.Fatal("checkout path incorrectly changes portable observed identity")
	}
	changed := collect(t.TempDir(), "package input\n// changed helper instrumentation\n")
	if first == changed {
		t.Fatal("helper-only changes escaped portable material binding")
	}
}

func TestPortableMaterialBindsSelectionNativeEmbedAndModuleVersion(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for name, text := range map[string]string{"go.mod": "module example.test/input\ngo 1.25\n", "leaf.go": "package input\n", "alternate.go": "package input\n", "leaf.s": "// same native bytes\n", "other.s": "// same native bytes\n", "input.dat": "same embedded bytes", "other.dat": "same embedded bytes"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	encode := func(mutate func(map[string]any)) [32]byte {
		record := map[string]any{"ImportPath": "example.test/input", "Dir": dir, "GoFiles": []string{"leaf.go"}, "SFiles": []string{"leaf.s"}, "EmbedFiles": []string{"input.dat"}, "Standard": false, "Goroot": false, "Module": map[string]string{"Path": "example.test/input", "Version": "v1.0.0", "GoVersion": "1.25", "Dir": dir, "GoMod": filepath.Join(dir, "go.mod")}}
		if mutate != nil {
			mutate(record)
		}
		data, err := json.Marshal(record)
		if err != nil {
			t.Fatal(err)
		}
		digest, err := binaryPortableMaterialDigest(data)
		if err != nil {
			t.Fatal(err)
		}
		return digest
	}
	baseline := encode(nil)
	for _, tc := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"selected_build_partition", func(m map[string]any) { m["GoFiles"] = []string{"alternate.go"} }},
		{"native_selection", func(m map[string]any) { m["SFiles"] = []string{"other.s"} }},
		{"embed_selection", func(m map[string]any) { m["EmbedFiles"] = []string{"other.dat"} }},
		{"module_version", func(m map[string]any) { m["Module"].(map[string]string)["Version"] = "v1.0.1" }},
		{"sdk_selection", func(m map[string]any) { m["Standard"] = true; m["Goroot"] = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if encode(tc.mutate) == baseline {
				t.Fatal("changed selection with identical content escaped freshness binding")
			}
		})
	}
}
