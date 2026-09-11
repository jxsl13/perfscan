package checks

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jxsl13/perfscan/config"
)

// Assignment must not reset an escaping constructed config from Unknown to
// Absent merely because the helper's returned identity is unsupported.
func TestPS6127ConstructedAssignmentPreservesUnknown(t *testing.T) {
	t.Parallel()
	candidate := ps6127PermanentFinalModelCases(t)[5].source
	for _, tail := range []string{"return c", "c=identity(c);return c", "alias:=identity(c);return alias"} {
		t.Run(tail, func(t *testing.T) {
			t.Parallel()
			if strings.Count(candidate, "return c\n}") != 1 {
				t.Fatal("owner return derivation is not unique")
			}
			source := strings.Replace(candidate, "return c\n}", tail+"\n}", 1) + "\nfunc identity(c *config)*config{return c}\n"
			contract := ps6127PermanentContract()
			contract.Matcher = "example.com/policy.config.ignoredBy"
			contract.DirectoryName = ".spectackle"
			if got := ps6127PermanentReports(t, source, []config.RecursiveMetadataIgnoreContract{contract}); got != 0 {
				t.Errorf("recursive owner has %d diagnostics, want 0", got)
			}
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "policy.go"), []byte(source), 0600); err != nil {
				t.Fatal(err)
			}
			behavior := `package main
import "testing"
func TestNestedPolicy(t *testing.T) {
 t.Parallel()
 c:=defaultConfig("/repo")
 if _,ignored:=c.ignoredBy("autograd/.spectackle/work/event.ndjson"); !ignored {t.Fatal("recursive owner policy was lost")}
}`
			if err := os.WriteFile(filepath.Join(dir, "policy_test.go"), []byte(behavior), 0600); err != nil {
				t.Fatal(err)
			}
			command := exec.CommandContext(t.Context(), "go", "test", "policy.go", "policy_test.go", "-count=1")
			command.Dir = dir
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatalf("actual owner policy: %v\n%s", err, output)
			}
		})
	}
}
