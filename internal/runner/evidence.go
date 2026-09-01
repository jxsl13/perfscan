package runner

import (
	"encoding/json"
	"fmt"
	"go/version"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/mod/modfile"
	"golang.org/x/tools/go/packages"
	"gopkg.in/yaml.v3"
)

const (
	setupGoConflictWarning   = "toolchain-setup-go-conflict"
	baselineToolchainWarning = "baseline-toolchain-changed"
)

// evidenceMetadata describes the environment that produced one scan. It is
// deliberately attached to the scan/evidence container rather than individual
// findings: a clean scan is still evidence, and every finding in one run shares
// the same environment.
type evidenceMetadata struct {
	Toolchain toolchainFingerprint `json:"toolchain" yaml:"toolchain"`
	Warnings  []evidenceWarning    `json:"warnings,omitempty" yaml:"warnings,omitempty"`
}

type toolchainFingerprint struct {
	GoVersion string `json:"goVersion" yaml:"goVersion"`
	GOOS      string `json:"goos" yaml:"goos"`
	GOARCH    string `json:"goarch" yaml:"goarch"`
	ModuleGo  string `json:"moduleGo,omitempty" yaml:"moduleGo,omitempty"`
}

type evidenceWarning struct {
	Code    string `json:"code" yaml:"code"`
	Message string `json:"message" yaml:"message"`
	File    string `json:"file,omitempty" yaml:"file,omitempty"`
	Line    int    `json:"line,omitempty" yaml:"line,omitempty"`
}

func scanEvidenceMetadata(pkgs []*packages.Package) evidenceMetadata {
	goVersion, targetGOOS, targetGOARCH := packageLoadToolchain()
	metadata := evidenceMetadata{Toolchain: toolchainFingerprint{
		GoVersion: goVersion,
		GOOS:      targetGOOS,
		GOARCH:    targetGOARCH,
	}}

	moduleGo, moduleRoot := mainModuleGo(pkgs)
	metadata.Toolchain.ModuleGo = moduleGo
	if moduleGo != "" && moduleRoot != "" {
		metadata.Warnings = setupGoWarnings(repositoryWorkflowRoot(moduleRoot), moduleGo)
	}
	return metadata
}

// packageLoadToolchain asks the go command for the same compiler and effective
// target inherited by packages.Load. In particular, GOVERSION reflects
// GOTOOLCHAIN and a module's toolchain directive instead of the version that
// happened to build the perfscan binary. GOOS/GOARCH include direct overrides
// and defaults persisted through go env -w or an alternate GOENV file. The
// fallback keeps metadata available in direct unit calls where the go command
// is unavailable; a real scan has already required it for packages.Load.
func packageLoadToolchain() (goVersion, goos, goarch string) {
	output, err := exec.Command("go", "env", "-json", "GOVERSION", "GOOS", "GOARCH").Output()
	if err == nil {
		var toolchain struct {
			GOVERSION string
			GOOS      string
			GOARCH    string
		}
		if json.Unmarshal(output, &toolchain) == nil &&
			toolchain.GOVERSION != "" && toolchain.GOOS != "" && toolchain.GOARCH != "" {
			return toolchain.GOVERSION, toolchain.GOOS, toolchain.GOARCH
		}
	}

	goVersion = runtime.Version()
	goos = os.Getenv("GOOS")
	if goos == "" {
		goos = runtime.GOOS
	}
	goarch = os.Getenv("GOARCH")
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	return goVersion, goos, goarch
}

// mainModuleGo returns a directive only when package loading identifies exactly
// one main module. Guessing among multiple go.work modules could compare a
// workflow with the wrong go.mod, so the ambiguous case stays conservative.
func mainModuleGo(pkgs []*packages.Package) (directive, root string) {
	type moduleIdentity struct {
		goMod     string
		dir       string
		goVersion string
	}
	modules := make(map[string]moduleIdentity, len(pkgs))
	for _, pkg := range pkgs {
		if pkg == nil || pkg.Module == nil || !pkg.Module.Main {
			continue
		}
		module := pkg.Module
		key := module.GoMod
		if key == "" {
			key = module.Dir
		}
		modules[key] = moduleIdentity{goMod: module.GoMod, dir: module.Dir, goVersion: module.GoVersion}
	}
	if len(modules) != 1 {
		return "", ""
	}
	var module moduleIdentity
	for _, candidate := range modules {
		module = candidate
	}

	root = module.dir
	if root == "" && module.goMod != "" {
		root = filepath.Dir(module.goMod)
	}
	if module.goMod != "" {
		contents, err := os.ReadFile(module.goMod)
		if err == nil {
			parsed, parseErr := modfile.ParseLax(module.goMod, contents, nil)
			if parseErr == nil && parsed.Go != nil {
				return parsed.Go.Version, root
			}
		}
	}
	return module.goVersion, root
}

// repositoryWorkflowRoot keeps module ownership separate from repository
// ownership. A main module may live below the checkout root, while GitHub only
// reads workflows from the checkout's top-level .github/workflows directory.
// Prefer an enclosing VCS root, then an enclosing workflow directory or
// go.work root for source archives and workspace fixtures without VCS metadata.
func repositoryWorkflowRoot(moduleRoot string) string {
	root, err := filepath.Abs(moduleRoot)
	if err != nil {
		root = filepath.Clean(moduleRoot)
	}
	workflowRoot := ""
	workspaceRoot := ""
	for directory := root; ; directory = filepath.Dir(directory) {
		if pathExists(filepath.Join(directory, ".git")) {
			return directory
		}
		if workflowRoot == "" && pathExists(filepath.Join(directory, ".github", "workflows")) {
			workflowRoot = directory
		}
		if workspaceRoot == "" && pathExists(filepath.Join(directory, "go.work")) {
			workspaceRoot = directory
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			break
		}
	}
	if workflowRoot != "" {
		return workflowRoot
	}
	if workspaceRoot != "" {
		return workspaceRoot
	}
	return root
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

var literalGoVersion = regexp.MustCompile(`^(?:go)?([1-9][0-9]*)\.([0-9]+)(?:\.(?:[0-9]+|x))?$`)

func goGeneration(value string) (string, bool) {
	match := literalGoVersion.FindStringSubmatch(strings.TrimSpace(value))
	if match == nil {
		return "", false
	}
	return match[1] + "." + match[2], true
}

func setupGoWarnings(repositoryRoot, moduleGo string) []evidenceWarning {
	moduleGeneration, ok := goGeneration(moduleGo)
	if !ok {
		return nil
	}
	workflowDir := filepath.Join(repositoryRoot, ".github", "workflows")
	entries, err := os.ReadDir(workflowDir)
	if err != nil {
		return nil
	}

	warnings := make([]evidenceWarning, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".yml" && ext != ".yaml" {
			continue
		}
		path := filepath.Join(workflowDir, entry.Name())
		contents, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var document yaml.Node
		if yaml.Unmarshal(contents, &document) != nil {
			continue
		}
		for _, input := range setupGoInputs(&document) {
			versionNode := input.version
			versionValue := ""
			inputDescription := "go-version"
			if versionNode != nil {
				// setup-go gives a non-empty go-version precedence when both
				// inputs are present. A dynamic or malformed winning input must
				// not fall through to a literal go-version-file guess.
				if versionNode.Kind != yaml.ScalarNode {
					continue
				}
				versionValue = versionNode.Value
			}
			if strings.TrimSpace(versionValue) == "" {
				versionNode = input.versionFile
				if versionNode == nil || versionNode.Kind != yaml.ScalarNode {
					continue
				}
				var literal bool
				versionValue, literal = setupGoVersionFromFile(repositoryRoot, versionNode.Value, input.toolchain)
				if !literal {
					continue
				}
				inputDescription = "go-version-file " + strconv.Quote(versionNode.Value) + " resolving to"
			}
			setupGeneration, literal := goGeneration(versionValue)
			if !literal || setupGeneration == moduleGeneration {
				continue
			}
			rel, err := filepath.Rel(repositoryRoot, path)
			if err != nil {
				rel = path
			}
			warnings = append(warnings, evidenceWarning{
				Code: setupGoConflictWarning,
				Message: "actions/setup-go " + inputDescription + " " + strconv.Quote(versionValue) +
					" selects Go " + setupGeneration + ", conflicting with go.mod generation " + moduleGeneration +
					"; regenerate performance baselines or explicitly carry them across the compiler-generation change",
				File: filepath.ToSlash(rel),
				Line: versionNode.Line,
			})
		}
	}
	slices.SortFunc(warnings, func(a, b evidenceWarning) int {
		if comparison := strings.Compare(a.File, b.File); comparison != 0 {
			return comparison
		}
		return a.Line - b.Line
	})
	return warnings
}

type setupGoInput struct {
	version     *yaml.Node
	versionFile *yaml.Node
	toolchain   setupGoToolchainMode
}

type setupGoToolchainMode uint8

const (
	setupGoToolchainUnknown setupGoToolchainMode = iota
	setupGoToolchainIgnored
	setupGoToolchainPreferred
)

var setupGoMajorVersion = regexp.MustCompile(`^v([1-9][0-9]*)(?:$|[.-])`)

func setupGoToolchainBehavior(ref string) setupGoToolchainMode {
	match := setupGoMajorVersion.FindStringSubmatch(strings.TrimSpace(ref))
	if match == nil {
		return setupGoToolchainUnknown
	}
	major, err := strconv.Atoi(match[1])
	if err != nil {
		return setupGoToolchainUnknown
	}
	if major >= 6 {
		return setupGoToolchainPreferred
	}
	return setupGoToolchainIgnored
}

// setupGoInputs walks only jobs.<job>.steps entries. Looking for matching
// scalar strings everywhere would misclassify comments, examples, unrelated
// actions, and reusable-workflow inputs as active setup-go configuration.
func setupGoInputs(document *yaml.Node) []setupGoInput {
	root := document
	if root == nil {
		return nil
	}
	if root.Kind == yaml.DocumentNode {
		if len(root.Content) != 1 {
			return nil
		}
		root = root.Content[0]
	}
	jobs := yamlMappingValue(root, "jobs")
	if jobs == nil || jobs.Kind != yaml.MappingNode {
		return nil
	}

	var inputs []setupGoInput
	for index := 1; index < len(jobs.Content); index += 2 {
		job := jobs.Content[index]
		steps := yamlMappingValue(job, "steps")
		if steps == nil || steps.Kind != yaml.SequenceNode {
			continue
		}
		for _, step := range steps.Content {
			uses := yamlMappingValue(step, "uses")
			if uses == nil || uses.Kind != yaml.ScalarNode {
				continue
			}
			action, ref, found := strings.Cut(strings.TrimSpace(uses.Value), "@")
			if !found || !strings.EqualFold(action, "actions/setup-go") {
				continue
			}
			with := yamlMappingValue(step, "with")
			if with == nil || with.Kind != yaml.MappingNode {
				continue
			}
			goVersion := yamlMappingValue(with, "go-version")
			goVersionFile := yamlMappingValue(with, "go-version-file")
			if goVersion != nil || goVersionFile != nil {
				inputs = append(inputs, setupGoInput{
					version:     goVersion,
					versionFile: goVersionFile,
					toolchain:   setupGoToolchainBehavior(ref),
				})
			}
		}
	}
	return inputs
}

func setupGoVersionFromFile(repositoryRoot, value string, toolchainMode setupGoToolchainMode) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" || strings.Contains(value, "${{") || filepath.IsAbs(value) {
		return "", false
	}
	root, err := filepath.Abs(repositoryRoot)
	if err != nil {
		return "", false
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", false
	}
	path, err := filepath.EvalSymlinks(filepath.Join(root, filepath.FromSlash(value)))
	if err != nil {
		return "", false
	}
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", false
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}

	switch filepath.Base(path) {
	case "go.mod":
		// setup-go v6+ gives the toolchain directive precedence, while older
		// releases use the go directive. Use the strict parser so both remain
		// available; ParseLax deliberately discards toolchain.
		parsed, err := modfile.Parse(path, contents, nil)
		if err != nil {
			return "", false
		}
		if parsed.Toolchain != nil && toolchainMode == setupGoToolchainUnknown {
			return "", false
		}
		if parsed.Toolchain != nil && toolchainMode == setupGoToolchainPreferred {
			return parsed.Toolchain.Name, true
		}
		if parsed.Go != nil {
			return parsed.Go.Version, true
		}
	case "go.work":
		parsed, err := modfile.ParseWork(path, contents, nil)
		if err != nil {
			return "", false
		}
		if parsed.Toolchain != nil && toolchainMode == setupGoToolchainUnknown {
			return "", false
		}
		if parsed.Toolchain != nil && toolchainMode == setupGoToolchainPreferred {
			return parsed.Toolchain.Name, true
		}
		if parsed.Go != nil {
			return parsed.Go.Version, true
		}
	case ".go-version":
		fields := strings.Fields(string(contents))
		if len(fields) == 1 {
			return fields[0], true
		}
	case ".tool-versions":
		version := ""
		for line := range strings.SplitSeq(string(contents), "\n") {
			fields := strings.Fields(line)
			if len(fields) == 0 || strings.HasPrefix(fields[0], "#") || fields[0] != "golang" {
				continue
			}
			if len(fields) < 2 || version != "" || len(fields) > 2 && !strings.HasPrefix(fields[2], "#") {
				return "", false
			}
			version = fields[1]
		}
		if version != "" {
			return version, true
		}
	}
	return "", false
}

func yamlMappingValue(mapping *yaml.Node, key string) *yaml.Node {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil
	}
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value == key {
			return mapping.Content[index+1]
		}
	}
	return nil
}

func emitEvidenceWarnings(warnings []evidenceWarning, stderr io.Writer) {
	for _, warning := range warnings {
		location := evidenceWarningLocation(warning)
		if location != "" {
			fmt.Fprintf(stderr, "perfscan: WARNING [%s] %s: %s\n", warning.Code, location, warning.Message)
			continue
		}
		fmt.Fprintf(stderr, "perfscan: WARNING [%s] %s\n", warning.Code, warning.Message)
	}
}

func evidenceWarningLocation(warning evidenceWarning) string {
	if warning.Line == 0 {
		return warning.File
	}
	return warning.File + ":" + strconv.Itoa(warning.Line)
}

func baselineMetadataWarnings(stored, current evidenceMetadata) []evidenceWarning {
	old := stored.Toolchain
	cur := current.Toolchain
	if old.GoVersion == "" {
		return []evidenceWarning{{
			Code:    baselineToolchainWarning,
			Message: "baseline has no toolchain fingerprint; regenerate it or explicitly carry it forward after verifying the compiler generation",
		}}
	}

	changes := make([]string, 0, 3)
	oldGo, oldGoOK := goCommandGeneration(old.GoVersion)
	curGo, curGoOK := goCommandGeneration(cur.GoVersion)
	if oldGoOK && curGoOK && oldGo != curGo {
		changes = append(changes, "compiler "+oldGo+" -> "+curGo)
	}
	oldModule, oldModuleOK := goGeneration(old.ModuleGo)
	curModule, curModuleOK := goGeneration(cur.ModuleGo)
	if oldModuleOK && curModuleOK && oldModule != curModule {
		changes = append(changes, "go.mod "+oldModule+" -> "+curModule)
	}
	if old.GOOS != "" && old.GOARCH != "" && cur.GOOS != "" && cur.GOARCH != "" &&
		(old.GOOS != cur.GOOS || old.GOARCH != cur.GOARCH) {
		changes = append(changes, "target "+old.GOOS+"/"+old.GOARCH+" -> "+cur.GOOS+"/"+cur.GOARCH)
	}
	if len(changes) == 0 {
		return nil
	}
	return []evidenceWarning{{
		Code: baselineToolchainWarning,
		Message: "baseline toolchain changed (" + strings.Join(changes, ", ") +
			"); regenerate performance baselines or explicitly carry them across after validating the new compiler generation",
	}}
}

func goCommandGeneration(goVersion string) (string, bool) {
	language := version.Lang(goVersion)
	if language == "" {
		return "", false
	}
	return strings.TrimPrefix(language, "go"), true
}
