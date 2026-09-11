// Command perfscan-closureenv produces compiler-backed closure growth evidence.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	"github.com/jxsl13/perfscan/internal/closureenv"
)

func main() {
	before := flag.String("before", "", "before-revision Go source file")
	after := flag.String("after", "", "after-revision Go source file")
	beforeDir := flag.String("before-dir", "", "before-revision module directory")
	afterDir := flag.String("after-dir", "", "after-revision module directory")
	pattern := flag.String("package", "", "single package pattern to compile in package mode")
	tags := flag.String("tags", "", "comma-separated build tags used for both revisions")
	function := flag.String("function", "", "enclosing function identity")
	file := flag.String("file", "", "optional exact target source basename in package mode")
	line := flag.Int("line", 0, "optional exact target closure line in package mode")
	column := flag.Int("column", 0, "optional exact target closure column in package mode")
	beforeFile := flag.String("before-file", "", "before-revision exact target source basename")
	beforeLine := flag.Int("before-line", 0, "before-revision exact target closure line")
	beforeColumn := flag.Int("before-column", 0, "before-revision exact target closure column")
	afterFile := flag.String("after-file", "", "after-revision exact target source basename")
	afterLine := flag.Int("after-line", 0, "after-revision exact target closure line")
	afterColumn := flag.Int("after-column", 0, "after-revision exact target closure column")
	goBinary := flag.String("go", "go", "Go command selecting the compiler and GOROOT")
	goos := flag.String("goos", runtime.GOOS, "compiler target GOOS")
	goarch := flag.String("goarch", runtime.GOARCH, "compiler target GOARCH")
	flag.Parse()
	fileMode := *before != "" || *after != ""
	packageMode := *beforeDir != "" || *afterDir != "" || *pattern != ""
	if *function == "" || fileMode == packageMode || fileMode && (*before == "" || *after == "") || packageMode && (*beforeDir == "" || *afterDir == "" || *pattern == "") {
		fail("select either -before/-after or -before-dir/-after-dir/-package, and provide -function")
	}
	var left, right []closureenv.Evidence
	var err error
	if fileMode {
		left, err = closureenv.CollectFile(context.Background(), *goBinary, *goos, *goarch, *before)
		if err == nil {
			right, err = closureenv.CollectFile(context.Background(), *goBinary, *goos, *goarch, *after)
		}
	} else {
		var buildTags []string
		for tag := range strings.SplitSeq(*tags, ",") {
			if tag = strings.TrimSpace(tag); tag != "" {
				buildTags = append(buildTags, tag)
			}
		}
		request := closureenv.PackageRequest{GoBinary: *goBinary, Pattern: *pattern, GOOS: *goos, GOARCH: *goarch, Tags: buildTags, Function: *function, File: *file, Line: *line, Column: *column}
		if *beforeFile != "" {
			request.File = *beforeFile
		}
		if *beforeLine != 0 {
			request.Line = *beforeLine
		}
		if *beforeColumn != 0 {
			request.Column = *beforeColumn
		}
		request.Dir = *beforeDir
		left, err = closureenv.CollectPackage(context.Background(), &request)
		if err == nil {
			request.File, request.Line, request.Column = *file, *line, *column
			if *afterFile != "" {
				request.File = *afterFile
			}
			if *afterLine != 0 {
				request.Line = *afterLine
			}
			if *afterColumn != 0 {
				request.Column = *afterColumn
			}
			request.Dir = *afterDir
			right, err = closureenv.CollectPackage(context.Background(), &request)
		}
	}
	if err != nil {
		fail(err.Error())
	}
	pick := func(all []closureenv.Evidence) (*closureenv.Evidence, error) {
		var found []*closureenv.Evidence
		for index := range all {
			evidence := &all[index]
			if evidence.Function == *function {
				found = append(found, evidence)
			}
		}
		if len(found) != 1 {
			return nil, fmt.Errorf("function %q has %d escaping closure sites, want exactly one", *function, len(found))
		}
		return found[0], nil
	}
	old, err := pick(left)
	if err != nil {
		fail(err.Error())
	}
	current, err := pick(right)
	if err != nil {
		fail(err.Error())
	}
	growth, err := closureenv.Compare(old, current)
	if err != nil {
		fail(err.Error())
	}
	if err := closureenv.WriteArtifact(os.Stdout, growth); err != nil {
		fail(err.Error())
	}
}

func fail(message string) {
	_, _ = io.WriteString(os.Stderr, "perfscan-closureenv: "+message+"\n")
	os.Exit(2)
}
