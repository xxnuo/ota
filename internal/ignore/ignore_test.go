package ignore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultIgnores(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ota-ignore-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	m := New(tmpDir)

	tests := []struct {
		path  string
		isDir bool
		want  bool
	}{
		{".git", true, true},
		{"node_modules", true, true},
		{".DS_Store", false, true},
		{"main.go", false, false},
		{"src/app.js", false, false},
	}

	for _, tt := range tests {
		got := m.Match(tt.path, tt.isDir)
		if got != tt.want {
			t.Errorf("Match(%q, %v) = %v, want %v", tt.path, tt.isDir, got, tt.want)
		}
	}
}

func TestGitignore(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ota-gitignore-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte("*.log\nbuild/\n"), 0644)

	m := New(tmpDir)

	if !m.Match("app.log", false) {
		t.Error("should match *.log")
	}
	if !m.Match("build", true) {
		t.Error("should match build/")
	}
	if m.Match("main.go", false) {
		t.Error("should not match main.go")
	}
}

func TestOtaignore(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ota-otaignore-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	os.WriteFile(filepath.Join(tmpDir, ".otaignore"), []byte("secret.key\n*.tmp\n"), 0644)

	m := New(tmpDir)

	if !m.Match("secret.key", false) {
		t.Error("should match secret.key")
	}
	if !m.Match("data.tmp", false) {
		t.Error("should match *.tmp")
	}
}
