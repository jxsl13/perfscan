package traceevidence

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

// InputPolicy is an audited USER declaration of all additional direct and
// indirect workload inputs, not a source-derived completeness proof. No files
// are staged and no OS privacy settings are inspected or modified.
type InputPolicy struct {
	InventoryComplete        bool     `json:"inventoryComplete"`
	Inputs                   []Input  `json:"inputs"`
	AdditionalProtectedRoots []string `json:"additionalProtectedRoots"`
	MaxInputBytes            int64    `json:"maxInputBytes"`
	Opened                   string   `json:"opened"`
}

type Input struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256,omitempty"`
}

type InputObservation struct {
	Declared   string           `json:"declared"`
	Absolute   string           `json:"absolute"`
	Canonical  string           `json:"canonical"`
	Executable bool             `json:"executable"`
	Bytes      int64            `json:"bytes"`
	SHA256     string           `json:"sha256"`
	Mount      MountObservation `json:"mount"`
}

type MountObservation struct {
	FileSystem string `json:"fileSystem"`
	MountPoint string `json:"mountPoint"`
	Flags      uint32 `json:"flags"`
	Local      bool   `json:"local"`
	Removable  bool   `json:"removable"`
}

type InputPreflight struct {
	Platform                string             `json:"platform"`
	Policy                  string             `json:"policy"`
	Passed                  bool               `json:"passed"`
	Reason                  string             `json:"reason"`
	Inputs                  []InputObservation `json:"inputs"`
	PermissionQualification string             `json:"permissionQualification"`
}

type inputEnvironment struct {
	platform   string
	home       string
	mount      func(string) (MountObservation, error)
	executable func(os.FileInfo) bool
	native     bool
}

func nativeInputEnvironment() (inputEnvironment, error) {
	if runtime.GOOS != "darwin" {
		return inputEnvironment{}, errors.New("privacy-input preflight supports Darwin only; OS/mount policy is unknown on this platform")
	}
	home, err := inputAccountHome()
	if err != nil || !filepath.IsAbs(home) {
		return inputEnvironment{}, errors.New("cannot resolve Darwin user home privacy roots")
	}
	return inputEnvironment{platform: "darwin", home: home, mount: observeInputMount, executable: func(info os.FileInfo) bool { return info.Mode().Perm()&0111 != 0 }, native: true}, nil
}

func (p *InputPolicy) valid() bool {
	if p == nil || !p.InventoryComplete || p.Inputs == nil || len(p.Inputs) > 4096 || len(p.AdditionalProtectedRoots) > 128 || p.MaxInputBytes < 1 || p.MaxInputBytes > 8<<30 || !singleLine(p.Opened) {
		return false
	}
	for _, in := range p.Inputs {
		if !singleLine(in.Path) {
			return false
		}
		if in.SHA256 != "" {
			if len(in.SHA256) != 64 || strings.ToLower(in.SHA256) != in.SHA256 {
				return false
			}
			if _, err := hex.DecodeString(in.SHA256); err != nil {
				return false
			}
		}
	}
	for _, root := range p.AdditionalProtectedRoots {
		if !singleLine(root) || !filepath.IsAbs(root) {
			return false
		}
	}
	return true
}

// PreflightInputs observes declared regular input availability and identity
// without launching a child. Passing never establishes profiler-child access.
// A nil policy, unknown platform/mount, protected input or unavailable file
// fails closed. The workload executable is included automatically.
func PreflightInputs(ctx context.Context, directory string, workload []string, p *InputPolicy) (*InputPreflight, error) {
	env, err := nativeInputEnvironment()
	if err != nil {
		return &InputPreflight{Platform: runtime.GOOS, Reason: err.Error()}, err
	}
	return preflightInputs(ctx, directory, workload, p, env)
}

func preflightInputs(ctx context.Context, directory string, workload []string, p *InputPolicy, env inputEnvironment) (*InputPreflight, error) {
	report := &InputPreflight{Platform: env.platform, Policy: "darwin-declared-local-inputs-v1", PermissionQualification: "parent read/hash observes availability only; profiler-child TCC/ACL/app-identity access remains unknown"}
	fail := func(err error) (*InputPreflight, error) { report.Reason = err.Error(); return report, err }
	if ctx == nil || !p.valid() || len(workload) == 0 || !singleLine(workload[0]) {
		return fail(errors.New("explicit audited-complete input policy, non-null inputs, bounded bytes and post-input-open marker are required"))
	}
	if env.platform != "darwin" || env.mount == nil || env.executable == nil || !filepath.IsAbs(env.home) {
		return fail(errors.New("unsupported or unknown input privacy/mount environment"))
	}
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	base := directory
	if base == "" {
		var err error
		base, err = os.Getwd()
		if err != nil {
			return fail(err)
		}
	}
	base, err := filepath.Abs(base)
	if err != nil {
		return fail(err)
	}
	roots := []string{filepath.Join(env.home, "Desktop"), filepath.Join(env.home, "Documents"), filepath.Join(env.home, "Downloads"), filepath.Join(env.home, "Library", "Mobile Documents"), filepath.Join(env.home, "Library", "CloudStorage"), "/Volumes"}
	roots = append(roots, p.AdditionalProtectedRoots...)
	// Include canonical root aliases where they exist. Missing default roots
	// still retain their lexical classification; other errors fail closed.
	for _, root := range slices.Clone(roots) {
		canonical, err := filepath.EvalSymlinks(root)
		if err == nil {
			roots = append(roots, canonical)
		} else if !os.IsNotExist(err) {
			return fail(fmt.Errorf("cannot classify protected root %s: %w", root, err))
		}
	}
	type protectedIdentity struct {
		path string
		info os.FileInfo
	}
	identities := make([]protectedIdentity, 0, len(roots))
	for _, root := range roots {
		info, err := os.Stat(root)
		if err == nil {
			identities = append(identities, protectedIdentity{root, info})
		} else if !os.IsNotExist(err) {
			return fail(fmt.Errorf("cannot observe protected root identity %s: %w", root, err))
		}
	}
	check := func(path string) error {
		if commonAccountPrivacyPath(path) {
			return fmt.Errorf("potential privacy-protected account input location %s; no input content was opened or staged", path)
		}
		for _, root := range roots {
			if withinPrivacyRoot(path, root) {
				return fmt.Errorf("potential privacy-protected input location %s intersects %s; do not launch: explicitly qualify an already approved non-protected input path outside this collector, or use a separately authorized workflow; no staging or privacy change was performed", path, root)
			}
		}
		// APFS firmlinks and alternate volume spellings need not appear as
		// symlinks. Compare existing ancestor identities as well as prefixes.
		for ancestor := filepath.Clean(path); ; ancestor = filepath.Dir(ancestor) {
			info, err := os.Stat(ancestor)
			if err == nil {
				for _, root := range identities {
					if os.SameFile(info, root.info) {
						return fmt.Errorf("potential privacy-protected input %s has ancestor identity of %s; no input content was opened or staged", path, root.path)
					}
				}
			} else if !os.IsNotExist(err) {
				return fmt.Errorf("cannot classify input ancestor %s: %w", ancestor, err)
			}
			if filepath.Dir(ancestor) == ancestor {
				break
			}
		}
		return nil
	}
	if err := check(base); err != nil {
		return fail(err)
	}
	canonicalBase, err := filepath.EvalSymlinks(base)
	if err != nil {
		return fail(err)
	}
	if err := check(canonicalBase); err != nil {
		return fail(err)
	}
	exe := workload[0]
	if !filepath.IsAbs(exe) {
		if strings.ContainsAny(exe, "/\\") {
			exe = filepath.Join(base, exe)
		} else {
			exe, err = exec.LookPath(exe)
			if err != nil {
				return fail(fmt.Errorf("resolve workload executable: %w", err))
			}
		}
	}
	inputs := make([]Input, 0, len(p.Inputs)+1)
	inputs = append(inputs, Input{Path: exe})
	inputs = append(inputs, p.Inputs...)
	seen := make(map[string]bool, len(inputs))
	// Classify EVERY path before reading ANY input content. This prevents a
	// later declared protected model from being opened during availability work.
	for i, in := range inputs {
		if err := ctx.Err(); err != nil {
			return fail(err)
		}
		absolute := in.Path
		if !filepath.IsAbs(absolute) {
			absolute = filepath.Join(base, absolute)
		}
		absolute, err = filepath.Abs(absolute)
		if err != nil {
			return fail(err)
		}
		observation := InputObservation{Declared: in.Path, Absolute: absolute, Executable: i == 0}
		if i == 0 {
			observation.Declared = workload[0]
		}
		report.Inputs = append(report.Inputs, observation)
		if err := check(absolute); err != nil {
			return fail(err)
		}
		canonical, err := filepath.EvalSymlinks(absolute)
		if err != nil {
			return fail(fmt.Errorf("resolve required input %s: %w", absolute, err))
		}
		report.Inputs[i].Canonical = canonical
		if err := check(canonical); err != nil {
			return fail(err)
		}
		key := strings.ToLower(canonical)
		if seen[key] {
			return fail(fmt.Errorf("duplicate canonical input %s", canonical))
		}
		seen[key] = true
		info, err := os.Lstat(canonical)
		if err != nil || !info.Mode().IsRegular() {
			return fail(fmt.Errorf("required input is missing or non-regular: %s", canonical))
		}
		if i == 0 && !env.executable(info) {
			return fail(fmt.Errorf("workload executable has no executable permission bits: %s", canonical))
		}
		mount, err := env.mount(canonical)
		if err != nil {
			return fail(fmt.Errorf("cannot qualify input mount %s: %w", canonical, err))
		}
		report.Inputs[i].Mount = mount
		if !mount.Local || mount.Removable || mount.FileSystem == "" || mount.MountPoint == "" {
			return fail(fmt.Errorf("input mount is non-local, removable or unknown: %s; profiler privacy access is not qualified", canonical))
		}
		if env.native && mount.FileSystem != "apfs" {
			return fail(fmt.Errorf("input filesystem %s is unsupported by selected Darwin/APFS preflight: %s", mount.FileSystem, canonical))
		}
	}
	var total int64
	for i, in := range inputs {
		observation := &report.Inputs[i]
		current, err := filepath.EvalSymlinks(observation.Absolute)
		if err != nil || current != observation.Canonical {
			return fail(fmt.Errorf("input path changed before availability check: %s", observation.Absolute))
		}
		digest, size, err := digestRegular(ctx, observation.Canonical, p.MaxInputBytes-total)
		if err != nil {
			return fail(fmt.Errorf("required input availability/hash %s: %w", observation.Canonical, err))
		}
		observation.Bytes, observation.SHA256 = size, hex.EncodeToString(digest)
		total += size
		if in.SHA256 != "" && in.SHA256 != observation.SHA256 {
			return fail(fmt.Errorf("required input SHA256 mismatch: %s", observation.Canonical))
		}
	}
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	report.Passed = true
	return report, nil
}

// Darwin classification is deliberately case-conservative. A prefix must end
// at a path boundary: DesktopBackup is not Desktop. This is NOT an exhaustive
// TCC rule engine, nor a proof concerning later symlink/inode changes.
func withinPrivacyRoot(path, root string) bool {
	path, root = strings.ToLower(filepath.Clean(path)), strings.ToLower(filepath.Clean(root))
	if root == string(filepath.Separator) {
		return filepath.IsAbs(path)
	}
	return path == root || strings.HasPrefix(path, root+string(filepath.Separator))
}

func commonAccountPrivacyPath(path string) bool {
	path = filepath.ToSlash(filepath.Clean(path))
	// The second spelling is the common APFS data-volume alias. These are
	// conservative exclusions, not an exhaustive alternate-home/TCC model.
	path = strings.TrimPrefix(strings.ToLower(path), "/system/volumes/data")
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) < 3 || parts[0] != "users" || parts[1] == "" {
		return false
	}
	switch parts[2] {
	case "desktop", "documents", "downloads":
		return true
	case "library":
		return len(parts) > 3 && (parts[3] == "mobile documents" || parts[3] == "cloudstorage")
	}
	return false
}
