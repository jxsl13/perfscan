// Command perfscan-coefficientcodegen records controlled Mach-O ARM64 evidence.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/jxsl13/perfscan/internal/closureenv"
	"github.com/jxsl13/perfscan/internal/coefficientcodegen"
)

func main() {
	dir := flag.String("dir", "", "complete module directory")
	pattern := flag.String("package", "", "one package pattern")
	function := flag.String("function", "", "package-level function name")
	goBinary := flag.String("go", "go", "selected Go command")
	flag.Parse()
	if *dir == "" || *pattern == "" || *function == "" {
		fail(errors.New("provide -dir, -package and -function"))
	}
	artifact, err := coefficientcodegen.Collect(context.Background(), &closureenv.PackageRequest{Dir: *dir, Pattern: *pattern, GoBinary: *goBinary}, *function)
	if err != nil {
		fail(err)
	}
	if err := json.NewEncoder(os.Stdout).Encode(artifact); err != nil {
		fail(err)
	}
}

func fail(err error) { fmt.Fprintln(os.Stderr, "perfscan-coefficientcodegen:", err); os.Exit(2) }
