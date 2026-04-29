package client

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/xxnuo/ota/internal/protocol"
)

func TestRetryFileBusy(t *testing.T) {
	oldCount := fileBusyRetryCount
	oldDelay := fileBusyRetryDelay
	fileBusyRetryCount = 3
	fileBusyRetryDelay = 10 * time.Millisecond
	defer func() {
		fileBusyRetryCount = oldCount
		fileBusyRetryDelay = oldDelay
	}()

	c := New("ws://127.0.0.1:1", t.TempDir(), "")
	calls := 0

	err := c.retryFileBusy("write binary", func() error {
		calls++
		if calls < 3 {
			return fmt.Errorf("text file busy")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("retry failed: %v", err)
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls, got %d", calls)
	}
}

func TestWriteBinaryFileReplacesRunningFile(t *testing.T) {
	dir := t.TempDir()
	binPath := filepath.Join(dir, "app.sh")
	oldScript := []byte("#!/bin/sh\nwhile true; do sleep 1; done\n")
	newScript := []byte("#!/bin/sh\necho updated\n")

	if err := os.WriteFile(binPath, oldScript, 0755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	cmd := exec.Command(binPath)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start script: %v", err)
	}
	defer func() {
		cmd.Process.Kill()
		cmd.Wait()
	}()

	c := New("ws://127.0.0.1:1", dir, "")
	if err := c.writeBinaryFile(binPath, newScript); err != nil {
		t.Fatalf("replace binary: %v", err)
	}

	data, err := os.ReadFile(binPath)
	if err != nil {
		t.Fatalf("read binary: %v", err)
	}
	if string(data) != string(newScript) {
		t.Fatalf("expected replaced binary, got %q", string(data))
	}
}

func TestHandleBinaryWaitsForProcessExitBeforeReplace(t *testing.T) {
	oldStopTimeout := stopTimeout
	stopTimeout = 50 * time.Millisecond
	defer func() {
		stopTimeout = oldStopTimeout
	}()

	dir := t.TempDir()
	binPath := filepath.Join(dir, "app.sh")
	script := []byte("#!/bin/sh\ntrap 'sleep 0.2; exit 0' TERM\nwhile true; do sleep 1; done\n")
	next := []byte("#!/bin/sh\necho updated\n")

	if err := os.WriteFile(binPath, script, 0755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	c := New("ws://127.0.0.1:1", dir, "")
	c.mu.Lock()
	c.lastBin = &protocol.BinaryPayload{Filename: filepath.Base(binPath)}
	c.mu.Unlock()

	c.startProc()
	defer c.Stop()

	c.handleBinary(&protocol.BinaryPayload{
		Filename: filepath.Base(binPath),
		Content:  next,
	})

	data, err := os.ReadFile(binPath)
	if err != nil {
		t.Fatalf("read binary: %v", err)
	}
	if string(data) != string(next) {
		t.Fatalf("expected replaced binary, got %q", string(data))
	}
}

func TestRestartAfterUnexpectedExit(t *testing.T) {
	oldDelay := crashRestartDelay
	crashRestartDelay = 50 * time.Millisecond
	defer func() {
		crashRestartDelay = oldDelay
	}()

	dir := t.TempDir()
	countPath := filepath.Join(dir, "count")
	binPath := filepath.Join(dir, "app.sh")
	script := fmt.Sprintf(`#!/bin/sh
count=0
if [ -f %q ]; then
	count=$(cat %q)
fi
count=$((count + 1))
echo "$count" > %q
if [ "$count" -eq 1 ]; then
	exit 2
fi
while true; do sleep 1; done
`, countPath, countPath, countPath)

	if err := os.WriteFile(binPath, []byte(script), 0755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	c := New("ws://127.0.0.1:1", dir, "")
	c.mu.Lock()
	c.lastBin = &protocol.BinaryPayload{Filename: filepath.Base(binPath)}
	c.mu.Unlock()

	c.startProc()
	defer c.Stop()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(countPath)
		if err == nil && string(data) == "2\n" {
			c.mu.Lock()
			proc := c.proc
			c.mu.Unlock()
			if proc != nil && proc.IsRunning() {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}

	data, _ := os.ReadFile(countPath)
	t.Fatalf("expected restart, count=%q", string(data))
}
