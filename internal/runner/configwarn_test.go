package runner

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// loadConfig must surface a typo'd vocabulary key as a WARNING; yaml.Unmarshal
// silently drops it, which otherwise leaves the domain check keyed on it
// starved and silent.
func TestLoadConfigWarnsUnknownKeys(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "cfg.yaml")
	if err := os.WriteFile(p, []byte("elementAccesors: [AtF64]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var errBuf bytes.Buffer
	loadConfig(&Options{ConfigPath: p, Stderr: &errBuf})
	out := errBuf.String()
	if !strings.Contains(out, "unrecognized key") || !strings.Contains(out, "elementAccesors") {
		t.Errorf("loadConfig should warn on the typo'd key; stderr:\n%s", out)
	}

	// A correctly spelled key produces no unrecognized-key warning.
	if err := os.WriteFile(p, []byte("elementAccessors: [AtF64]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	errBuf.Reset()
	loadConfig(&Options{ConfigPath: p, Stderr: &errBuf})
	if strings.Contains(errBuf.String(), "unrecognized key") {
		t.Errorf("no warning expected for a correct config; stderr:\n%s", errBuf.String())
	}
}

func TestLoadConfigAcceptsCurrentGoAIVocabulary(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "perfscan.json")
	data := []byte(`{
  "_comment": "metadata",
  "inputViewFuncs": ["f64Data"],
  "kernelRegisterFuncs": ["add"],
  "layoutOpConstants": ["OpSlice"],
  "optimizedBackendPkgs": ["cpu"],
  "outputViewFuncs": ["outF64"],
  "pointerTypeNames": ["Tensor"],
  "pureComputeFuncs": ["forward"],
  "referenceBackendPkg": "ref",
  "topKSelectorFuncs": ["topKIndices"],
  "topKOneContracts": [{"name":"resident", "function":"example.com/goai.DeviceBuffer.TopKN", "kind":"method", "kArgPosition":2, "indicesResultPosition":1}],
  "variadicDispatchWrappers": ["exec"]
}`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	var errBuf bytes.Buffer
	cfg, _ := loadConfig(&Options{ConfigPath: path, Stderr: &errBuf})
	if strings.Contains(errBuf.String(), "unrecognized key") {
		t.Fatalf("current GoAI vocabulary must not produce compatibility warnings:\n%s", errBuf.String())
	}
	if cfg.ReferenceBackendPkg != "ref" || len(cfg.VariadicDispatchWrappers) != 1 || len(cfg.TopKOneContracts) != 1 {
		t.Fatalf("loadConfig lost current vocabulary: %+v", cfg)
	}
}
