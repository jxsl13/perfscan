package allocationcampaign

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Snapshot pins exact committed input bytes for another evidence campaign.
// It reuses the allocation campaign's strict archive/blob validation, not a
// mutable checkout's clean-status claim. Directory is an execution location,
// not a permission to execute commands supplied by a decoded artifact.
type Snapshot struct {
	Commit        string `json:"commit"`
	ArchiveSHA256 string `json:"archiveSHA256"`
	TreeSHA256    string `json:"treeSHA256"`
}

func exactCommit(commit string) bool {
	if len(commit) != 40 && len(commit) != 64 {
		return false
	}
	for _, c := range commit {
		if !strings.ContainsRune("0123456789abcdef", c) {
			return false
		}
	}
	return true
}

func RetainSnapshot(repository, commit, output string) (Snapshot, error) {
	var result Snapshot
	if !exactCommit(commit) {
		return result, errors.New("snapshot requires an exact full commit identity")
	}
	if err := os.Mkdir(output, 0700); err != nil {
		return result, err
	}
	env := make([]string, 0, len(os.Environ()))
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "GIT_") {
			env = append(env, value)
		}
	}
	observed, errout, code := command(repository, env, "git", "rev-parse", commit+"^{commit}")
	if err := artifact(output, "commit", observed, errout, code); err != nil {
		return result, err
	}
	if code != 0 || string(bytes.TrimSpace(observed)) != commit {
		return result, errors.New("exact commit identity cannot be observed")
	}
	tree, errout, code := command(repository, env, "git", "ls-tree", "-r", "-z", "--full-tree", commit)
	if err := artifact(output, "tree", tree, errout, code); err != nil {
		return result, err
	}
	if code != 0 {
		return result, errors.New("cannot retain exact committed tree")
	}
	archive, errout, code := sourceArchive(repository, env, commit)
	if err := artifact(output, "archive", archive, errout, code); err != nil {
		return result, err
	}
	if code != 0 {
		return result, errors.New("cannot retain canonical source archive")
	}
	root := filepath.Join(output, "source")
	if err := unpack(archive, root); err != nil {
		return result, err
	}
	if err := exactTree(root, tree); err != nil {
		return result, err
	}
	if err := pinnedModule(root); err != nil {
		return result, err
	}
	return Snapshot{Commit: commit, ArchiveSHA256: hash(archive), TreeSHA256: hash(tree)}, nil
}

// VerifySnapshot verifies retained source AGAINST exact retained blob identities.
// The independently pinned campaign manifest authenticates these stream hashes;
// callers must also reproduce the controlled build, never trust them alone.
func VerifySnapshot(repository, directory string, expected Snapshot) error {
	if !exactCommit(expected.Commit) {
		return errors.New("snapshot requires an exact full commit identity")
	}
	tree, err := os.ReadFile(filepath.Join(directory, "tree.stdout"))
	if err != nil {
		return err
	}
	archive, err := os.ReadFile(filepath.Join(directory, "archive.stdout"))
	if err != nil {
		return err
	}
	if hash(tree) != expected.TreeSHA256 || hash(archive) != expected.ArchiveSHA256 {
		return errors.New("retained snapshot stream changed")
	}
	env := make([]string, 0, len(os.Environ()))
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "GIT_") {
			env = append(env, value)
		}
	}
	observed, _, code := command(repository, env, "git", "ls-tree", "-r", "-z", "--full-tree", expected.Commit)
	if code != 0 || !bytes.Equal(observed, tree) {
		return errors.New("retained tree does not match observed pinned Git commit")
	}
	canonical, _, code := sourceArchive(repository, env, expected.Commit)
	if code != 0 || !bytes.Equal(canonical, archive) {
		return errors.New("retained archive does not match observed pinned Git commit")
	}
	for _, name := range []string{"commit", "tree", "archive"} {
		out, err := os.ReadFile(filepath.Join(directory, name+".exit"))
		if err != nil {
			return err
		}
		if string(out) != "0\n" {
			return errors.New("snapshot command did not succeed")
		}
		if _, err := os.Stat(filepath.Join(directory, name+".stderr")); err != nil {
			return err
		}
	}
	commit, err := os.ReadFile(filepath.Join(directory, "commit.stdout"))
	if err != nil {
		return err
	}
	if string(bytes.TrimSpace(commit)) != expected.Commit {
		return errors.New("retained commit differs from pin")
	}
	return exactTree(filepath.Join(directory, "source"), tree)
}
