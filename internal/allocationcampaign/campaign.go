// Package allocationcampaign runs and independently verifies fixed-count,
// process-isolated allocation diagnostics. It does not attribute allocation sites.
package allocationcampaign

import (
	"archive/tar"
	"bufio"
	"bytes"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/jxsl13/perfscan/benchmarkevidence"
	"golang.org/x/mod/modfile"
)

const N = 1024

const maxInvocations = 100000

type Options struct {
	OldRoot, CandidateRoot, Output, Diagnostic, Package string
	Pairs                                               int
	Procs                                               []int
	Instrumentation                                     []string
}

type Build struct {
	Root            string            `json:"root"`
	Commit          string            `json:"commit"`
	Instrumentation map[string]string `json:"instrumentationSHA256"`
	ArchiveSHA256   string            `json:"archiveSHA256"`
	TreeSHA256      string            `json:"treeSHA256"`
	Environment     string            `json:"environment"`
	Binary          string            `json:"binary"`
	SHA256          string            `json:"sha256"`
}

type Invocation struct {
	Phase     string `json:"phase"`
	Pair      int    `json:"pair"`
	Procs     int    `json:"gomaxprocs"`
	Arm       string `json:"arm"`
	Binary    string `json:"binary"`
	Selection string `json:"selection"`
}

type Plan struct {
	Schema               int               `json:"schema"`
	N                    int               `json:"n"`
	Pairs                int               `json:"pairs"`
	Procs                []int             `json:"procs"`
	Package              string            `json:"package"`
	Diagnostic           string            `json:"diagnostic"`
	Builds               []Build           `json:"builds"`
	Invocations          []Invocation      `json:"invocations"`
	RuntimeEnvironment   map[string]string `json:"runtimeEnvironment"`
	InstrumentationRoots []string          `json:"instrumentationRoots"`
}

type Record struct {
	Invocation   Invocation `json:"invocation"`
	Exit         int        `json:"exit"`
	StdoutSHA256 string     `json:"stdoutSHA256"`
	StderrSHA256 string     `json:"stderrSHA256"`
}

type Pair struct {
	Phase string                       `json:"phase"`
	Procs int                          `json:"gomaxprocs"`
	Index int                          `json:"pair"`
	A     benchmarkevidence.Allocation `json:"a"`
	B     benchmarkevidence.Allocation `json:"b"`
	Delta benchmarkevidence.Delta      `json:"pairedTotalDelta"`
}

// RoundedMedianComparison is deliberately separate from paired raw totals.
// Rational strings avoid floating-point loss for integer-counter medians.
type RoundedMedianComparison struct {
	Phase            string `json:"phase"`
	Procs            int    `json:"gomaxprocs"`
	ABytes           string `json:"aMedianRoundedBytesPerOp"`
	BBytes           string `json:"bMedianRoundedBytesPerOp"`
	BytesDifference  string `json:"differenceOfArmMediansBytesPerOp"`
	AAllocs          string `json:"aMedianRoundedAllocsPerOp"`
	BAllocs          string `json:"bMedianRoundedAllocsPerOp"`
	AllocsDifference string `json:"differenceOfArmMediansAllocsPerOp"`
}

type Report struct {
	Pairs             []Pair                    `json:"pairedRawTotals"`
	RoundedArmMedians []RoundedMedianComparison `json:"roundedArmMedians"`
}

func median(values []int64) *big.Rat {
	slices.Sort(values)
	middle := len(values) / 2
	if len(values)%2 != 0 {
		return new(big.Rat).SetInt64(values[middle])
	}
	sum := new(big.Int).Add(big.NewInt(values[middle-1]), big.NewInt(values[middle]))
	return new(big.Rat).SetFrac(sum, big.NewInt(2))
}

func report(pairs []Pair) Report {
	result := Report{Pairs: pairs}
	seen := make(map[struct {
		phase string
		procs int
	}]bool, len(pairs))
	for firstIndex := range pairs {
		first := &pairs[firstIndex]
		phase, procs := first.Phase, first.Procs
		key := struct {
			phase string
			procs int
		}{phase, procs}
		if seen[key] {
			continue
		}
		seen[key] = true
		var ab, bb, aa, ba []int64
		for pairIndex := range pairs {
			pair := &pairs[pairIndex]
			if pair.Phase != phase || pair.Procs != procs {
				continue
			}
			ab = append(ab, pair.A.BytesPerOp)
			bb = append(bb, pair.B.BytesPerOp)
			aa = append(aa, pair.A.AllocsPerOp)
			ba = append(ba, pair.B.AllocsPerOp)
		}
		aBytes, bBytes, aAllocs, bAllocs := median(ab), median(bb), median(aa), median(ba)
		result.RoundedArmMedians = append(result.RoundedArmMedians, RoundedMedianComparison{phase, procs, aBytes.RatString(), bBytes.RatString(), new(big.Rat).Sub(bBytes, aBytes).RatString(), aAllocs.RatString(), bAllocs.RatString(), new(big.Rat).Sub(bAllocs, aAllocs).RatString()})
	}
	return result
}

func hash(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

func command(root string, env []string, name string, args ...string) ([]byte, []byte, int) {
	cmd := exec.Command(name, args...)
	cmd.Dir = root
	cmd.Env = env
	var out, errout bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errout
	err := cmd.Run()
	code := 0
	if err != nil {
		code = -1
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			code = exit.ExitCode()
		} else {
			_, _ = fmt.Fprintln(&errout, err)
		}
	}
	return out.Bytes(), errout.Bytes(), code
}

// Git archive applies checkout-style CRLF conversion under core.autocrlf=true.
// A build snapshot must contain committed blob bytes, not host checkout bytes.
// Override only this invocation; do not alter the caller's repository config.
// Attribute-driven substitutions remain subject to exactTree's strict check.
func sourceArchive(root string, env []string, commit string) ([]byte, []byte, int) {
	return command(root, env, "git", "-c", "core.autocrlf=false", "archive", "--format=tar", commit)
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0600)
}

func artifact(dir, name string, out, errout []byte, code int) error {
	for _, item := range []struct {
		name string
		data []byte
	}{{name + ".stdout", out}, {name + ".stderr", errout}, {name + ".exit", []byte(strconv.Itoa(code) + "\n")}} {
		if err := os.WriteFile(filepath.Join(dir, item.name), item.data, 0600); err != nil {
			return err
		}
	}
	return nil
}

func invocationName(index int) string { return fmt.Sprintf("sample-%04d", index) }

func unpack(data []byte, dir string) error {
	reader := tar.NewReader(bytes.NewReader(data))
	for {
		header, err := reader.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		name := filepath.Clean(header.Name)
		if strings.HasPrefix(header.Name, "/") || filepath.IsAbs(name) || name == ".." || strings.HasPrefix(name, ".."+string(filepath.Separator)) {
			return errors.New("archive path escapes snapshot")
		}
		path := filepath.Join(dir, name)
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(path, 0700); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				return err
			}
			file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, os.FileMode(header.Mode)&0777)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(file, reader)
			closeErr := file.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		case tar.TypeXGlobalHeader: // git archive's commit metadata
		default:
			return errors.New("snapshot rejects symlinks and other unpinned archive inputs")
		}
	}
}

func instrumentation(root string, paths []string) (map[string]string, error) {
	result := make(map[string]string)
	for _, path := range paths {
		if filepath.IsAbs(path) || strings.HasPrefix(filepath.Clean(path), "..") {
			return nil, errors.New("instrumentation paths must stay inside snapshot")
		}
		err := filepath.WalkDir(filepath.Join(root, path), func(file string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				return nil
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return errors.New("instrumentation symlinks are unsupported")
			}
			data, err := os.ReadFile(file)
			if err != nil {
				return err
			}
			name, err := filepath.Rel(root, file)
			if err != nil {
				return err
			}
			result[filepath.ToSlash(name)] = hash(data)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func pinnedModule(root string) error {
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return err
	}
	module, err := modfile.Parse("go.mod", data, nil)
	if err != nil {
		return err
	}
	for _, replacement := range module.Replace {
		if replacement.New.Version == "" {
			return errors.New("local module replacements are not pinned diagnostic inputs")
		}
	}
	return nil
}

func exactTree(root string, tree []byte) error {
	expected := make(map[string]string, bytes.Count(tree, []byte{0}))
	for row := range bytes.SplitSeq(tree, []byte{0}) {
		if len(row) == 0 {
			continue
		}
		metadata, name, ok := bytes.Cut(row, []byte{'\t'})
		fields := strings.Fields(string(metadata))
		if !ok || len(fields) != 3 || fields[1] != "blob" || (fields[0] != "100644" && fields[0] != "100755") {
			return errors.New("source tree contains unsupported symlink/submodule or malformed entry")
		}
		path := string(name)
		if _, ok := expected[path]; ok {
			return errors.New("duplicate source tree path")
		}
		expected[path] = fields[2]
	}
	count := 0
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		name, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		pin, ok := expected[filepath.ToSlash(name)]
		if !ok {
			return errors.New("archive contains untracked input")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		blob := append([]byte("blob "+strconv.Itoa(len(data))+"\x00"), data...)
		actual := ""
		switch len(pin) {
		case 40:
			sum := sha1.Sum(blob)
			actual = hex.EncodeToString(sum[:])
		case 64:
			actual = hash(blob)
		default:
			return errors.New("invalid git blob hash")
		}
		if actual != pin {
			return errors.New("archive substituted committed source content")
		}
		count++
		return nil
	})
	if err != nil {
		return err
	}
	if count != len(expected) {
		return errors.New("archive omitted committed source input")
	}
	return nil
}

func invocations(pairs int, procs []int) []Invocation {
	var result []Invocation
	for _, phase := range []string{"control", "candidate"} {
		for pair := 1; pair <= pairs; pair++ {
			processOrder := slices.Clone(procs)
			if pair%2 == 0 {
				slices.Reverse(processOrder)
			}
			for _, p := range processOrder {
				arms := []string{"A", "B"}
				if pair%2 == 0 {
					arms = []string{"B", "A"}
				}
				for _, arm := range arms {
					binary, selection := "old.test", "before"
					if phase == "candidate" && arm == "B" {
						binary, selection = "candidate.test", "after"
					}
					result = append(result, Invocation{phase, pair, p, arm, binary, selection})
				}
			}
		}
	}
	return result
}

// Run builds two pinned clean trees and executes the predeclared serialized
// matrix. Output must not exist. Every build/sample error remains an artifact;
// failed samples do not stop collection of subsequent planned invocations.
func Run(o *Options) error {
	if o == nil {
		return errors.New("missing campaign options")
	}
	if os.Getenv("GOFLAGS") != "" {
		return errors.New("ambient GOFLAGS are unsupported; build options must come from the pinned runner")
	}
	if o.OldRoot == "" || o.CandidateRoot == "" || o.Output == "" || o.Pairs < 2 || o.Pairs%2 != 0 || len(o.Procs) == 0 || !strings.HasPrefix(o.Package, "./") || filepath.IsAbs(o.Diagnostic) || strings.HasPrefix(filepath.Clean(o.Diagnostic), "..") || filepath.Clean(strings.TrimPrefix(o.Package, "./")) != filepath.Dir(filepath.Clean(o.Diagnostic)) || !strings.HasSuffix(o.Diagnostic, "_test.go") {
		return errors.New("require even pairs >=2, positive procs, package and relative diagnostic file")
	}
	if len(o.Procs) > maxInvocations/4 || o.Pairs > maxInvocations/(4*len(o.Procs)) {
		return errors.New("diagnostic matrix exceeds 100000 invocation capacity")
	}
	for index, p := range o.Procs {
		if p <= 0 || slices.Contains(o.Procs[:index], p) {
			return errors.New("invalid or duplicate GOMAXPROCS")
		}
	}
	if err := os.Mkdir(o.Output, 0700); err != nil {
		return err
	}
	dir, err := filepath.Abs(o.Output)
	if err != nil {
		return err
	}
	env := make([]string, 0, len(os.Environ())+1)
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "GOWORK=") && !strings.HasPrefix(value, "GIT_") {
			env = append(env, value)
		}
	}
	env = append(env, "GOWORK=off")
	plan := Plan{Schema: 1, N: N, Pairs: o.Pairs, Procs: o.Procs, Package: o.Package, Diagnostic: o.Diagnostic, Invocations: invocations(o.Pairs, o.Procs)}
	plan.RuntimeEnvironment = make(map[string]string)
	plan.InstrumentationRoots = append([]string{o.Diagnostic, "benchmarkevidence"}, o.Instrumentation...)
	for _, key := range []string{"GODEBUG", "GOGC", "GOMEMLIMIT"} {
		plan.RuntimeEnvironment[key] = os.Getenv(key)
	}
	for i, root := range []string{o.OldRoot, o.CandidateRoot} {
		root, err = filepath.Abs(root)
		if err != nil {
			return err
		}
		prefix := []string{"old", "candidate"}[i]
		out, errout, code := command(root, env, "git", "status", "--porcelain", "--untracked-files=all")
		if err := artifact(dir, prefix+"-status", out, errout, code); err != nil {
			return err
		}
		if code != 0 || len(bytes.TrimSpace(out)) != 0 {
			return errors.New("source trees must be clean pinned git checkouts; status artifacts retained")
		}
		commit, errout, code := command(root, env, "git", "rev-parse", "HEAD")
		if err := artifact(dir, prefix+"-commit", commit, errout, code); err != nil {
			return err
		}
		if code != 0 {
			return errors.New("cannot pin source commit")
		}
		tree, errout, code := command(root, env, "git", "ls-tree", "-r", "-z", "--full-tree", strings.TrimSpace(string(commit)))
		if err := artifact(dir, prefix+"-tree", tree, errout, code); err != nil {
			return err
		}
		if code != 0 {
			return errors.New("cannot retain pinned source tree")
		}
		archive, errout, code := sourceArchive(root, env, strings.TrimSpace(string(commit)))
		if err := artifact(dir, prefix+"-archive", archive, errout, code); err != nil {
			return err
		}
		if code != 0 {
			return errors.New("cannot archive pinned source")
		}
		snapshot := filepath.Join(dir, prefix+"-source")
		if err := unpack(archive, snapshot); err != nil {
			return err
		}
		if err := exactTree(snapshot, tree); err != nil {
			return err
		}
		if err := pinnedModule(snapshot); err != nil {
			return err
		}
		instrument, err := instrumentation(snapshot, plan.InstrumentationRoots)
		if err != nil {
			return err
		}
		out, errout, code = command(snapshot, env, "go", "env", "-json", "GOVERSION", "GOOS", "GOARCH", "CGO_ENABLED", "GOFLAGS", "GOEXPERIMENT", "GOAMD64", "GOARM64", "CC", "CXX", "CGO_CFLAGS", "CGO_CPPFLAGS", "CGO_CXXFLAGS", "CGO_LDFLAGS")
		if err := artifact(dir, prefix+"-environment", out, errout, code); err != nil {
			return err
		}
		if code != 0 {
			return errors.New("cannot fingerprint effective build environment")
		}
		var buildEnvironment map[string]string
		if err := json.Unmarshal(out, &buildEnvironment); err != nil {
			return err
		}
		if buildEnvironment["GOFLAGS"] != "" {
			return errors.New("effective GOFLAGS are unsupported; external overlays/modfiles/tool wrappers are unpinned inputs")
		}
		build := Build{Root: root, Commit: strings.TrimSpace(string(commit)), Instrumentation: instrument, ArchiveSHA256: hash(archive), TreeSHA256: hash(tree), Environment: string(out), Binary: prefix + ".test"}
		if i == 1 && (!reflect.DeepEqual(build.Instrumentation, plan.Builds[0].Instrumentation) || build.Environment != plan.Builds[0].Environment) {
			return errors.New("diagnostic source or effective build environment differs")
		}
		out, errout, code = command(snapshot, env, "go", "test", "-c", "-trimpath", "-mod=readonly", "-tags=allocationdiagnostic", "-o", filepath.Join(dir, build.Binary), o.Package)
		if err := artifact(dir, prefix+"-build", out, errout, code); err != nil {
			return err
		}
		if code != 0 {
			return errors.New("diagnostic build failed; raw build artifacts retained")
		}
		out, errout, code = command(root, env, "git", "status", "--porcelain", "--untracked-files=all")
		if err := artifact(dir, prefix+"-postbuild-status", out, errout, code); err != nil {
			return err
		}
		post, err := instrumentation(snapshot, plan.InstrumentationRoots)
		if err != nil {
			return err
		}
		if code != 0 || len(bytes.TrimSpace(out)) != 0 || !reflect.DeepEqual(post, build.Instrumentation) {
			return errors.New("source changed during build")
		}
		postCommit, errout, code := command(root, env, "git", "rev-parse", "HEAD")
		if err := artifact(dir, prefix+"-postbuild-commit", postCommit, errout, code); err != nil {
			return err
		}
		if code != 0 || !bytes.Equal(commit, postCommit) {
			return errors.New("source commit changed during build")
		}
		binary, err := os.ReadFile(filepath.Join(dir, build.Binary))
		if err != nil {
			return err
		}
		build.SHA256 = hash(binary)
		plan.Builds = append(plan.Builds, build)
	}
	// Save the complete plan before starting either measurement phase.
	if err := writeJSON(filepath.Join(dir, "plan.json"), plan); err != nil {
		return err
	}
	planBytes, err := os.ReadFile(filepath.Join(dir, "plan.json"))
	if err != nil {
		return err
	}
	planHash := hash(planBytes)
	if _, err := io.WriteString(os.Stdout, "planSHA256="+planHash+"\n"); err != nil {
		return err
	}
	var records []Record
	for index, inv := range plan.Invocations {
		runenv := make([]string, 0, len(env)+2)
		for _, value := range env {
			if !strings.HasPrefix(value, "GOMAXPROCS=") && !strings.HasPrefix(value, "PERFSCAN_ALLOCATION_ARM=") {
				runenv = append(runenv, value)
			}
		}
		runenv = append(runenv, "GOMAXPROCS="+strconv.Itoa(inv.Procs), "PERFSCAN_ALLOCATION_ARM="+inv.Selection)
		source := strings.TrimSuffix(inv.Binary, ".test") + "-source"
		cwd := filepath.Join(dir, source, strings.TrimPrefix(o.Package, "./"))
		out, errout, code := command(cwd, runenv, filepath.Join(dir, inv.Binary), "-test.run=^TestAllocationDiagnostic$", "-test.benchtime=1024x")
		if err := artifact(dir, invocationName(index), out, errout, code); err != nil {
			return err
		}
		records = append(records, Record{inv, code, hash(out), hash(errout)})
		if err := writeJSON(filepath.Join(dir, "records.json"), records); err != nil {
			return err
		}
	}
	recordBytes, err := os.ReadFile(filepath.Join(dir, "records.json"))
	if err != nil {
		return err
	}
	recordHash := hash(recordBytes)
	if _, err := io.WriteString(os.Stdout, "recordsSHA256="+recordHash+"\n"); err != nil {
		return err
	}
	pairs, err := Verify(dir, planHash, recordHash)
	if err != nil {
		return err
	}
	return writeJSON(filepath.Join(dir, "analysis.json"), pairs)
}

// uniqueJSON rejects duplicate keys recursively before typed decoding (which
// would otherwise silently accept the last duplicate), and trailing documents.
func uniqueJSON(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value func() error
	value = func() error {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		if token == nil {
			return errors.New("null JSON schema values are forbidden")
		}
		delim, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := make(map[string]bool)
			for decoder.More() {
				key, err := decoder.Token()
				if err != nil {
					return err
				}
				name, ok := key.(string)
				if !ok || seen[name] {
					return errors.New("invalid or duplicate JSON key")
				}
				seen[name] = true
				if err := value(); err != nil {
					return err
				}
			}
		case '[':
			for decoder.More() {
				if err := value(); err != nil {
					return err
				}
			}
		default:
			return errors.New("unexpected JSON delimiter")
		}
		_, err = decoder.Token()
		return err
	}
	if err := value(); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return errors.New("trailing JSON content")
	}
	return nil
}

func decode(data []byte, destination any) error {
	if err := uniqueJSON(data); err != nil {
		return err
	}
	if err := requiredFields(data, reflect.TypeOf(destination).Elem()); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode(destination)
}

func requiredFields(data []byte, kind reflect.Type) error {
	switch kind.Kind() {
	case reflect.Struct:
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(data, &fields); err != nil {
			return err
		}
		for index := 0; index < kind.NumField(); index++ {
			field := kind.Field(index)
			name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
			if name == "" {
				name = field.Name
			}
			value, ok := fields[name]
			if !ok {
				return fmt.Errorf("missing JSON schema field %s", name)
			}
			if err := requiredFields(value, field.Type); err != nil {
				return err
			}
		}
	case reflect.Slice:
		var values []json.RawMessage
		if err := json.Unmarshal(data, &values); err != nil {
			return err
		}
		for _, value := range values {
			if err := requiredFields(value, kind.Elem()); err != nil {
				return err
			}
		}
	}
	return nil
}

func readJSON(path string, destination any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return decode(data, destination)
}

func parseSample(out []byte) (benchmarkevidence.Allocation, error) {
	scanner := bufio.NewScanner(bytes.NewReader(out))
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	var sample benchmarkevidence.Allocation
	count, passes := 0, 0
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if string(line) == "PASS" {
			passes++
		}
		if bytes.HasPrefix(line, []byte("FAIL")) {
			return sample, errors.New("failed invocation output")
		}
		if !bytes.HasPrefix(line, []byte("{")) {
			continue
		}
		var fields map[string]json.RawMessage
		if err := decode(line, &fields); err != nil {
			return sample, err
		}
		if len(fields) != 7 {
			return sample, errors.New("missing exact allocation fields")
		}
		if err := decode(line, &sample); err != nil {
			return sample, err
		}
		count++
	}
	if err := scanner.Err(); err != nil {
		return sample, err
	}
	if count != 1 || passes != 1 || sample.N != N {
		return sample, errors.New("expected exactly one exact fixed-count sample and PASS")
	}
	if _, err := benchmarkevidence.PairedDelta(sample, sample); err != nil {
		return sample, err
	}
	return sample, nil
}

// Verify reparses retained raw artifacts, validates binary/build/sample hashes,
// the exact predeclared matrix/order and integer identities. It never subtracts
// control noise or equates nonsignificance with equality.
func Verify(dir, planHash, recordHash string) (Report, error) {
	for path, pin := range map[string]string{"plan.json": planHash, "records.json": recordHash} {
		data, err := os.ReadFile(filepath.Join(dir, path))
		if err != nil {
			return Report{}, err
		}
		if len(pin) != 64 || hash(data) != pin {
			return Report{}, fmt.Errorf("%s does not match independently retained SHA256", path)
		}
	}
	var plan Plan
	if err := readJSON(filepath.Join(dir, "plan.json"), &plan); err != nil {
		return Report{}, err
	}
	if plan.Schema != 1 || plan.N != N || plan.Pairs < 2 || plan.Pairs%2 != 0 || len(plan.Procs) == 0 || len(plan.Procs) > maxInvocations/4 || plan.Pairs > maxInvocations/(4*len(plan.Procs)) || len(plan.Builds) != 2 || !reflect.DeepEqual(plan.Invocations, invocations(plan.Pairs, plan.Procs)) {
		return Report{}, errors.New("invalid predeclared matrix")
	}
	if len(plan.RuntimeEnvironment) != 3 {
		return Report{}, errors.New("missing runtime environment pins")
	}
	if len(plan.InstrumentationRoots) < 2 || plan.InstrumentationRoots[0] != plan.Diagnostic || plan.InstrumentationRoots[1] != "benchmarkevidence" {
		return Report{}, errors.New("missing complete wrapper/helper instrumentation roots")
	}
	for _, key := range []string{"GODEBUG", "GOGC", "GOMEMLIMIT"} {
		if _, ok := plan.RuntimeEnvironment[key]; !ok {
			return Report{}, errors.New("missing runtime environment pin")
		}
	}
	for index, p := range plan.Procs {
		if p <= 0 || slices.Contains(plan.Procs[:index], p) {
			return Report{}, errors.New("invalid process counts")
		}
	}
	for i, build := range plan.Builds {
		name := []string{"old", "candidate"}[i]
		if build.Binary != name+".test" || len(build.Commit) != 40 || len(build.Instrumentation) == 0 || len(build.ArchiveSHA256) != 64 || build.Environment == "" || !reflect.DeepEqual(build.Instrumentation, plan.Builds[0].Instrumentation) || build.Environment != plan.Builds[0].Environment {
			return Report{}, errors.New("invalid matched build pins")
		}
		data, err := os.ReadFile(filepath.Join(dir, build.Binary))
		if err != nil {
			return Report{}, err
		}
		if hash(data) != build.SHA256 {
			return Report{}, errors.New("binary hash mismatch")
		}
		archive, err := os.ReadFile(filepath.Join(dir, name+"-archive.stdout"))
		if err != nil {
			return Report{}, err
		}
		if hash(archive) != build.ArchiveSHA256 {
			return Report{}, errors.New("pinned source archive hash mismatch")
		}
		snapshot, err := os.MkdirTemp("", "allocation-verification-")
		if err != nil {
			return Report{}, err
		}
		defer os.RemoveAll(snapshot)
		if err := unpack(archive, snapshot); err != nil {
			return Report{}, err
		}
		tree, err := os.ReadFile(filepath.Join(dir, name+"-tree.stdout"))
		if err != nil {
			return Report{}, err
		}
		if hash(tree) != build.TreeSHA256 {
			return Report{}, errors.New("pinned git tree hash mismatch")
		}
		if err := exactTree(snapshot, tree); err != nil {
			return Report{}, err
		}
		if err := pinnedModule(snapshot); err != nil {
			return Report{}, err
		}
		instrument, err := instrumentation(snapshot, plan.InstrumentationRoots)
		if err != nil {
			return Report{}, err
		}
		if !reflect.DeepEqual(instrument, build.Instrumentation) {
			return Report{}, errors.New("retained instrumentation hash mismatch")
		}
		for _, kind := range []string{"status", "commit", "environment", "build", "postbuild-status", "postbuild-commit", "archive", "tree"} {
			code, err := os.ReadFile(filepath.Join(dir, name+"-"+kind+".exit"))
			if err != nil {
				return Report{}, err
			}
			if string(code) != "0\n" {
				return Report{}, errors.New("unsuccessful build provenance")
			}
			for _, suffix := range []string{".stdout", ".stderr"} {
				if _, err := os.Stat(filepath.Join(dir, name+"-"+kind+suffix)); err != nil {
					return Report{}, err
				}
			}
		}
		commit, err := os.ReadFile(filepath.Join(dir, name+"-commit.stdout"))
		if err != nil {
			return Report{}, err
		}
		if strings.TrimSpace(string(commit)) != build.Commit {
			return Report{}, errors.New("source pin mismatch")
		}
		postCommit, err := os.ReadFile(filepath.Join(dir, name+"-postbuild-commit.stdout"))
		if err != nil {
			return Report{}, err
		}
		if !bytes.Equal(commit, postCommit) {
			return Report{}, errors.New("source commit changed during build")
		}
		environment, err := os.ReadFile(filepath.Join(dir, name+"-environment.stdout"))
		if err != nil {
			return Report{}, err
		}
		if string(environment) != build.Environment {
			return Report{}, errors.New("environment pin mismatch")
		}
		status, err := os.ReadFile(filepath.Join(dir, name+"-status.stdout"))
		if err != nil {
			return Report{}, err
		}
		if len(bytes.TrimSpace(status)) != 0 {
			return Report{}, errors.New("dirty source pin")
		}
		poststatus, err := os.ReadFile(filepath.Join(dir, name+"-postbuild-status.stdout"))
		if err != nil {
			return Report{}, err
		}
		if len(bytes.TrimSpace(poststatus)) != 0 {
			return Report{}, errors.New("source changed during build")
		}
	}
	var records []Record
	if err := readJSON(filepath.Join(dir, "records.json"), &records); err != nil {
		return Report{}, err
	}
	if len(records) != len(plan.Invocations) {
		return Report{}, errors.New("missing or extra invocations")
	}
	files, err := os.ReadDir(dir)
	if err != nil {
		return Report{}, err
	}
	expected := make(map[string]bool, 3*len(records))
	for index := range records {
		for _, suffix := range []string{".stdout", ".stderr", ".exit"} {
			expected[invocationName(index)+suffix] = true
		}
	}
	for _, file := range files {
		if strings.HasPrefix(file.Name(), "sample-") && !expected[file.Name()] {
			return Report{}, errors.New("unplanned raw sample artifact")
		}
	}
	var result []Pair
	var current Pair
	for index, record := range records {
		if record.Invocation != plan.Invocations[index] || record.Exit != 0 {
			return Report{}, errors.New("failed or reordered invocation")
		}
		name := invocationName(index)
		out, err := os.ReadFile(filepath.Join(dir, name+".stdout"))
		if err != nil {
			return Report{}, err
		}
		errout, err := os.ReadFile(filepath.Join(dir, name+".stderr"))
		if err != nil {
			return Report{}, err
		}
		code, err := os.ReadFile(filepath.Join(dir, name+".exit"))
		if err != nil {
			return Report{}, err
		}
		if string(code) != "0\n" || hash(out) != record.StdoutSHA256 || hash(errout) != record.StderrSHA256 {
			return Report{}, errors.New("raw sample hash or exit mismatch")
		}
		sample, err := parseSample(out)
		if err != nil {
			return Report{}, fmt.Errorf("%s: %w", name, err)
		}
		inv := record.Invocation
		if index%2 == 0 {
			current = Pair{Phase: inv.Phase, Procs: inv.Procs, Index: inv.Pair}
		}
		if inv.Arm == "A" {
			current.A = sample
		} else {
			current.B = sample
		}
		if index%2 == 1 {
			current.Delta, err = benchmarkevidence.PairedDelta(current.A, current.B)
			if err != nil {
				return Report{}, err
			}
			result = append(result, current)
		}
	}
	return report(result), nil
}
