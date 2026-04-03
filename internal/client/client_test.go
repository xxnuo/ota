package client

import (
	"fmt"
	"os"
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
