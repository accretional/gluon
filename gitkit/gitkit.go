// Package gitkit provides utilities for cloning and managing git repositories
// based on Repo descriptors. It implements the logic behind the Git gRPC
// service defined in git.proto.
package gitkit

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	pb "github.com/accretional/gluon/pb"
)

// DefaultBasePath is the default directory for cloning repos (parent of CWD).
const DefaultBasePath = ".."

// RepoURL returns the clone URL for a Repo.
func RepoURL(repo *pb.Repo) (string, error) {
	switch s := repo.GetSource().(type) {
	case *pb.Repo_Gh:
		return fmt.Sprintf("https://github.com/%s/%s.git", s.Gh.Owner, s.Gh.Name), nil
	default:
		return "", fmt.Errorf("unsupported repo source type: %T", s)
	}
}

// RepoDir returns the local directory name for a Repo.
func RepoDir(repo *pb.Repo) (string, error) {
	switch s := repo.GetSource().(type) {
	case *pb.Repo_Gh:
		return s.Gh.Name, nil
	default:
		return "", fmt.Errorf("unsupported repo source type: %T", s)
	}
}

// Fetch clones or updates a repository. If dest is empty, DefaultBasePath
// is used. Returns the local path and HEAD commit hash.
func Fetch(repo *pb.Repo, dest string, shallow bool) (*pb.FetchResult, error) {
	url, err := RepoURL(repo)
	if err != nil {
		return nil, err
	}
	dirName, err := RepoDir(repo)
	if err != nil {
		return nil, err
	}

	if dest == "" {
		dest = DefaultBasePath
	}
	localPath := filepath.Join(dest, dirName)

	existed := false
	if info, err := os.Stat(filepath.Join(localPath, ".git")); err == nil && info.IsDir() {
		existed = true
		// Repo already exists — fetch updates
		cmd := exec.Command("git", "-C", localPath, "fetch", "--all")
		if out, err := cmd.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("git fetch in %s: %w\n%s", localPath, err, out)
		}
		cmd = exec.Command("git", "-C", localPath, "pull", "--ff-only")
		if out, err := cmd.CombinedOutput(); err != nil {
			// Pull may fail on detached HEAD or diverged branch; that's ok
			_ = out
		}
	} else {
		// Fresh clone
		args := []string{"clone"}
		if shallow {
			args = append(args, "--depth", "1")
		}
		args = append(args, url, localPath)
		cmd := exec.Command("git", args...)
		if out, err := cmd.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("git clone %s: %w\n%s", url, err, out)
		}
	}

	// Get HEAD commit
	head, err := gitHead(localPath)
	if err != nil {
		return nil, err
	}

	return &pb.FetchResult{
		Path:           localPath,
		HeadCommit:     head,
		AlreadyExisted: existed,
	}, nil
}

// ListProtoFiles returns all .proto files in a local repo directory.
func ListProtoFiles(repoPath string) ([]string, error) {
	var protos []string
	err := filepath.Walk(repoPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors (permission denied, etc.)
		}
		// Skip .git directory
		if info.IsDir() && info.Name() == ".git" {
			return filepath.SkipDir
		}
		// Skip common vendored/third_party dirs to avoid duplicates
		if info.IsDir() && (info.Name() == "vendor" || info.Name() == "third_party" || info.Name() == "node_modules") {
			return filepath.SkipDir
		}
		if !info.IsDir() && strings.HasSuffix(info.Name(), ".proto") {
			rel, err := filepath.Rel(repoPath, path)
			if err != nil {
				rel = path
			}
			protos = append(protos, rel)
		}
		return nil
	})
	return protos, err
}

func gitHead(repoPath string) (string, error) {
	cmd := exec.Command("git", "-C", repoPath, "rev-parse", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse HEAD in %s: %w", repoPath, err)
	}
	return strings.TrimSpace(string(out)), nil
}
