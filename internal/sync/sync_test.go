package sync

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xxnuo/ota/internal/protocol"
)

func TestBuildManifest(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ota-sync-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	os.WriteFile(filepath.Join(tmpDir, "a.txt"), []byte("hello"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "b.go"), []byte("package main"), 0644)
	os.MkdirAll(filepath.Join(tmpDir, "sub"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "sub", "c.txt"), []byte("world"), 0644)

	manifest, err := BuildManifest(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	if len(manifest.Files) < 3 {
		t.Errorf("expected at least 3 files, got %d", len(manifest.Files))
	}

	found := map[string]bool{}
	for _, f := range manifest.Files {
		found[f.Path] = true
	}
	for _, expected := range []string{"a.txt", "b.go", "sub"} {
		if !found[expected] {
			t.Errorf("expected file %s in manifest", expected)
		}
	}
}

func TestBuildManifestIgnoresGit(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ota-sync-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	os.MkdirAll(filepath.Join(tmpDir, ".git"), 0755)
	os.WriteFile(filepath.Join(tmpDir, ".git", "config"), []byte("git"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main"), 0644)

	manifest, err := BuildManifest(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	for _, f := range manifest.Files {
		if f.Path == ".git" || f.Path == ".git/config" {
			t.Errorf("should not include .git files: %s", f.Path)
		}
	}
}

func TestDiffManifest(t *testing.T) {
	local := &protocol.Manifest{
		Files: []protocol.FileInfo{
			{Path: "a.txt", Hash: "aaa"},
			{Path: "b.txt", Hash: "bbb"},
			{Path: "old.txt", Hash: "old"},
		},
	}
	remote := &protocol.Manifest{
		Files: []protocol.FileInfo{
			{Path: "a.txt", Hash: "aaa"},
			{Path: "b.txt", Hash: "bbb_new"},
			{Path: "c.txt", Hash: "ccc"},
		},
	}

	need, del := DiffManifest(local, remote)

	needMap := map[string]bool{}
	for _, p := range need {
		needMap[p] = true
	}
	if !needMap["b.txt"] {
		t.Error("should need b.txt (hash changed)")
	}
	if !needMap["c.txt"] {
		t.Error("should need c.txt (new file)")
	}
	if needMap["a.txt"] {
		t.Error("should not need a.txt (unchanged)")
	}

	delMap := map[string]bool{}
	for _, p := range del {
		delMap[p] = true
	}
	if !delMap["old.txt"] {
		t.Error("should delete old.txt")
	}
}

func TestApplyFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ota-apply-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	fd := &protocol.FileData{
		Path:    "sub/test.txt",
		Content: []byte("hello world"),
		Mode:    0644,
		IsDir:   false,
	}

	if err := ApplyFile(tmpDir, fd); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, "sub", "test.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello world" {
		t.Errorf("expected 'hello world', got '%s'", string(data))
	}
}

func TestApplyDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ota-apply-dir-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	fd := &protocol.FileData{
		Path:  "newdir",
		Mode:  0755,
		IsDir: true,
	}

	if err := ApplyFile(tmpDir, fd); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(filepath.Join(tmpDir, "newdir"))
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		t.Error("expected directory")
	}
}

func TestDeleteFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ota-delete-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	testFile := filepath.Join(tmpDir, "delete-me.txt")
	os.WriteFile(testFile, []byte("bye"), 0644)

	if err := DeleteFile(tmpDir, "delete-me.txt"); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(testFile); !os.IsNotExist(err) {
		t.Error("file should be deleted")
	}
}

func TestReadFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ota-read-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	os.WriteFile(filepath.Join(tmpDir, "read.txt"), []byte("read me"), 0644)

	fd, err := ReadFile(tmpDir, "read.txt")
	if err != nil {
		t.Fatal(err)
	}
	if string(fd.Content) != "read me" {
		t.Errorf("expected 'read me', got '%s'", string(fd.Content))
	}
	if fd.Path != "read.txt" {
		t.Errorf("expected path 'read.txt', got '%s'", fd.Path)
	}
}
