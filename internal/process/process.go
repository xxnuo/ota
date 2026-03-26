package process

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"

	"github.com/xxnuo/ota/internal/logger"
)

type Manager struct {
	mu      sync.Mutex
	cmd     *exec.Cmd
	command string
	workDir string
	running bool
	stopCh  chan struct{}
}

func New(command, workDir string) *Manager {
	return &Manager{
		command: command,
		workDir: workDir,
	}
}

func (m *Manager) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return fmt.Errorf("process already running")
	}

	parts := strings.Fields(m.command)
	if len(parts) == 0 {
		return fmt.Errorf("empty command")
	}

	m.cmd = exec.Command(parts[0], parts[1:]...)
	m.cmd.Dir = m.workDir
	m.cmd.Env = os.Environ()
	m.cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdout, err := m.cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := m.cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := m.cmd.Start(); err != nil {
		return err
	}

	m.running = true
	m.stopCh = make(chan struct{})

	go m.streamOutput("stdout", stdout)
	go m.streamOutput("stderr", stderr)
	go m.wait()

	logger.Log.Info().Str("command", m.command).Int("pid", m.cmd.Process.Pid).Msg("process started")
	return nil
}

func (m *Manager) streamOutput(name string, r io.ReadCloser) {
	scanner := bufio.NewScanner(r)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		logger.Log.Info().Str("stream", name).Msg(line)
	}
}

func (m *Manager) wait() {
	if m.cmd != nil && m.cmd.Process != nil {
		m.cmd.Wait()
	}
	m.mu.Lock()
	m.running = false
	m.mu.Unlock()
	logger.Log.Info().Str("command", m.command).Msg("process exited")
}

func (m *Manager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running || m.cmd == nil || m.cmd.Process == nil {
		return fmt.Errorf("process not running")
	}

	pgid, err := syscall.Getpgid(m.cmd.Process.Pid)
	if err == nil {
		syscall.Kill(-pgid, syscall.SIGTERM)
	} else {
		m.cmd.Process.Signal(syscall.SIGTERM)
	}

	m.running = false
	logger.Log.Info().Str("command", m.command).Msg("process stopped")
	return nil
}

func (m *Manager) Kill() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running || m.cmd == nil || m.cmd.Process == nil {
		return fmt.Errorf("process not running")
	}

	pgid, err := syscall.Getpgid(m.cmd.Process.Pid)
	if err == nil {
		syscall.Kill(-pgid, syscall.SIGKILL)
	} else {
		m.cmd.Process.Kill()
	}

	m.running = false
	logger.Log.Info().Str("command", m.command).Msg("process killed")
	return nil
}

func (m *Manager) Restart() error {
	if m.IsRunning() {
		if err := m.Stop(); err != nil {
			m.Kill()
		}
	}
	return m.Start()
}

func (m *Manager) IsRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

func (m *Manager) Pid() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cmd != nil && m.cmd.Process != nil {
		return m.cmd.Process.Pid
	}
	return 0
}

func (m *Manager) Command() string {
	return m.command
}
