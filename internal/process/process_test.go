package process

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStopKeepsRunningUntilExit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.sh")

	if err := os.WriteFile(path, []byte("#!/bin/sh\ntrap '' TERM\nwhile true; do sleep 1; done\n"), 0755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	proc := New(path, nil, dir, nil, nil)
	if err := proc.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	time.Sleep(100 * time.Millisecond)
	proc.Stop()

	if !proc.IsRunning() {
		t.Fatal("expected process to remain running before exit")
	}

	proc.Kill()

	if !proc.WaitTimeout(2 * time.Second) {
		t.Fatal("expected process to exit after kill")
	}
}
