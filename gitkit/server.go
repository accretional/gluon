package gitkit

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	pb "github.com/accretional/gluon/pb"
)

// Server implements the gluon.Git gRPC service.
type Server struct {
	pb.UnimplementedGitServer

	// BasePath is the default directory for cloning repos.
	BasePath string
}

// NewServer creates a Git gRPC server with the given base path for clones.
func NewServer(basePath string) *Server {
	if basePath == "" {
		basePath = DefaultBasePath
	}
	return &Server{BasePath: basePath}
}

func (s *Server) Fetch(_ context.Context, req *pb.FetchRequest) (*pb.FetchResult, error) {
	dest := req.Dest
	if dest == "" {
		dest = s.BasePath
	}
	return Fetch(req.Repo, dest, req.Shallow)
}

func (s *Server) ListFiles(_ context.Context, req *pb.ListFilesRequest) (*pb.ListFilesResult, error) {
	dirName, err := RepoDir(req.Repo)
	if err != nil {
		return nil, err
	}
	repoPath := filepath.Join(s.BasePath, dirName)

	pattern := req.Pattern
	if pattern == "" {
		pattern = "**/*"
	}

	var files []*pb.FileEntry
	err = filepath.Walk(repoPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() && info.Name() == ".git" {
			return filepath.SkipDir
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(repoPath, path)
		if err != nil {
			return nil
		}
		matched, err := filepath.Match(pattern, filepath.Base(rel))
		if err != nil {
			return nil
		}
		// Also try matching against full relative path for ** patterns
		if !matched && strings.Contains(pattern, "*") {
			matched, _ = filepath.Match(pattern, rel)
		}
		if matched {
			files = append(files, &pb.FileEntry{
				Path: rel,
				Size: info.Size(),
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &pb.ListFilesResult{Files: files}, nil
}
