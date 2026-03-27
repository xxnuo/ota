//go:build windows

package process

import (
	"os"
	"os/exec"
	"strconv"
)

func setSysProcAttr(cmd *exec.Cmd) {
}

func stopProcess(proc *os.Process) {
	exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(proc.Pid)).Run()
	proc.Kill()
}

func killProcess(proc *os.Process) {
	exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(proc.Pid)).Run()
	proc.Kill()
}
