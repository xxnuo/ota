package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureDirs(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ota-config-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	origDir := configDir
	configDir = filepath.Join(tmpDir, "ota-config")
	defer func() { configDir = origDir }()

	if err := EnsureDirs(); err != nil {
		t.Fatal(err)
	}

	for _, dir := range []string{ServerDir(), ClientDir(), ServerLogDir(), ClientLogDir()} {
		info, err := os.Stat(dir)
		if err != nil {
			t.Errorf("dir %s should exist: %v", dir, err)
		}
		if !info.IsDir() {
			t.Errorf("%s should be a directory", dir)
		}
	}
}

func TestServerConfig(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ota-config-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	origDir := configDir
	configDir = filepath.Join(tmpDir, "ota-config")
	defer func() { configDir = origDir }()

	cfg := &ServerConfig{
		PID:     1234,
		Address: "0.0.0.0:9867",
		WorkDir: "/tmp/work",
		Port:    9867,
	}

	if err := SaveServerConfig(cfg); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadServerConfig()
	if err != nil {
		t.Fatal(err)
	}

	if loaded.PID != 1234 {
		t.Errorf("expected PID 1234, got %d", loaded.PID)
	}
	if loaded.Address != "0.0.0.0:9867" {
		t.Errorf("expected address 0.0.0.0:9867, got %s", loaded.Address)
	}
}

func TestClientConfig(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ota-config-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	origDir := configDir
	configDir = filepath.Join(tmpDir, "ota-config")
	defer func() { configDir = origDir }()

	cfg := &ClientConfig{
		PID:       5678,
		ServerURL: "ws://localhost:9867",
		WorkDir:   "/tmp/client",
		Command:   "air",
	}

	if err := SaveClientConfig(cfg); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadClientConfig()
	if err != nil {
		t.Fatal(err)
	}

	if loaded.PID != 5678 {
		t.Errorf("expected PID 5678, got %d", loaded.PID)
	}
	if loaded.Command != "air" {
		t.Errorf("expected command air, got %s", loaded.Command)
	}
}

func TestPidFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ota-pid-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	origDir := configDir
	configDir = filepath.Join(tmpDir, "ota-config")
	defer func() { configDir = origDir }()

	pidFile := filepath.Join(ServerDir(), "test.pid")
	if err := SavePid(pidFile, 42); err != nil {
		t.Fatal(err)
	}

	pid, err := LoadPid(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	if pid != 42 {
		t.Errorf("expected pid 42, got %d", pid)
	}

	RemovePid(pidFile)
	if _, err := os.Stat(pidFile); !os.IsNotExist(err) {
		t.Error("pid file should be removed")
	}
}

func TestServerState(t *testing.T) {
	state := NewServerState()

	state.AddClient(ClientInfo{ID: "c1", Hostname: "host1", WorkDir: "/work1"})
	state.AddClient(ClientInfo{ID: "c2", Hostname: "host2", WorkDir: "/work2"})

	clients := state.GetClients()
	if len(clients) != 2 {
		t.Errorf("expected 2 clients, got %d", len(clients))
	}

	state.AddClient(ClientInfo{ID: "c1", Hostname: "host1-updated", WorkDir: "/work1"})
	clients = state.GetClients()
	if len(clients) != 2 {
		t.Errorf("expected 2 clients after update, got %d", len(clients))
	}

	found := state.FindClientByWorkDir("/work1")
	if found == nil {
		t.Error("should find client by workDir")
	}
	if found.Hostname != "host1-updated" {
		t.Errorf("expected updated hostname, got %s", found.Hostname)
	}

	state.RemoveClient("c1")
	clients = state.GetClients()
	if len(clients) != 1 {
		t.Errorf("expected 1 client after remove, got %d", len(clients))
	}
}
