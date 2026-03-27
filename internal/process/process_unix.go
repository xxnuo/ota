//go:build !windows

package process

import (
	"os"
	"os/exec"
	"syscall"
)

func setSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func stopProcess(proc *os.Process) {
	pgid, err := syscall.Getpgid(proc.Pid)
	if err == nil {
		syscall.Kill(-pgid, syscall.SIGTERM)
	} else {
		proc.Signal(syscall.SIGTERM)
	}
}

func killProcess(proc *os.Process) {
	pgid, err := syscall.Getpgid(proc.Pid)
	if err == nil {
		syscall.Kill(-pgid, syscall.SIGKILL)
	} else {
		proc.Kill()
	}
}
