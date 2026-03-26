package sync

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/xxnuo/ota/internal/ignore"
	"github.com/xxnuo/ota/internal/protocol"
)

func BuildManifest(workDir string) (*protocol.Manifest, error) {
	matcher := ignore.New(workDir)
	manifest := &protocol.Manifest{
		WorkDir: workDir,
	}

	err := filepath.Walk(workDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		relPath, err := filepath.Rel(workDir, path)
		if err != nil {
			return nil
		}

		if relPath == "." {
			return nil
		}

		if matcher.Match(relPath, info.IsDir()) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		fi := protocol.FileInfo{
			Path:    relPath,
			Size:    info.Size(),
			Mode:    info.Mode(),
			ModTime: info.ModTime(),
			IsDir:   info.IsDir(),
		}

		if !info.IsDir() {
			hash, err := protocol.HashFile(path)
			if err != nil {
				return nil
			}
			fi.Hash = hash
		}

		manifest.Files = append(manifest.Files, fi)
		return nil
	})

	if err != nil {
		return nil, err
	}

	sort.Slice(manifest.Files, func(i, j int) bool {
		return manifest.Files[i].Path < manifest.Files[j].Path
	})

	return manifest, nil
}

func DiffManifest(local, remote *protocol.Manifest) (needFiles []string, deleteFiles []string) {
	remoteMap := make(map[string]protocol.FileInfo)
	for _, f := range remote.Files {
		remoteMap[f.Path] = f
	}

	localMap := make(map[string]protocol.FileInfo)
	for _, f := range local.Files {
		localMap[f.Path] = f
	}

	for _, rf := range remote.Files {
		lf, exists := localMap[rf.Path]
		if !exists || lf.Hash != rf.Hash {
			needFiles = append(needFiles, rf.Path)
		}
	}

	for _, lf := range local.Files {
		if _, exists := remoteMap[lf.Path]; !exists {
			deleteFiles = append(deleteFiles, lf.Path)
		}
	}

	return
}

func ApplyFile(workDir string, fd *protocol.FileData) error {
	fullPath := filepath.Join(workDir, fd.Path)

	if fd.IsDir {
		return os.MkdirAll(fullPath, fd.Mode)
	}

	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	return os.WriteFile(fullPath, fd.Content, fd.Mode)
}

func DeleteFile(workDir, relPath string) error {
	fullPath := filepath.Join(workDir, relPath)
	return os.RemoveAll(fullPath)
}

func ReadFile(workDir, relPath string) (*protocol.FileData, error) {
	fullPath := filepath.Join(workDir, relPath)

	info, err := os.Stat(fullPath)
	if err != nil {
		return nil, err
	}

	fd := &protocol.FileData{
		Path:  relPath,
		Mode:  info.Mode(),
		IsDir: info.IsDir(),
	}

	if !info.IsDir() {
		content, err := os.ReadFile(fullPath)
		if err != nil {
			return nil, err
		}
		fd.Content = content
	}

	return fd, nil
}
