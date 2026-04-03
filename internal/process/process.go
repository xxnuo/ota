package process

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

type Manager struct {
	mu       sync.Mutex
	cmd      *exec.Cmd
	binPath  string
	args     []string
	workDir  string
	running  bool
	onLog    func(source, line string)
	onExit   func(state *os.ProcessState)
	waitDone chan struct{}
}

func New(binPath string, args []string, workDir string, onLog func(source, line string), onExit func(state *os.ProcessState)) *Manager {
	return &Manager{
		binPath: binPath,
		args:    args,
		workDir: workDir,
		onLog:   onLog,
		onExit:  onExit,
	}
}

func (m *Manager) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return fmt.Errorf("already running")
	}

	m.cmd = exec.Command(m.binPath, m.args...)
	m.cmd.Dir = m.workDir
	m.cmd.Env = os.Environ()
	setSysProcAttr(m.cmd)

	stdout, _ := m.cmd.StdoutPipe()
	stderr, _ := m.cmd.StderrPipe()

	if err := m.cmd.Start(); err != nil {
		return err
	}
	m.running = true
	m.waitDone = make(chan struct{})

	go m.pipe("app:out", stdout)
	go m.pipe("app:err", stderr)
	go m.wait()

	return nil
}

func (m *Manager) pipe(source string, r io.ReadCloser) {
	scanner := bufio.NewScanner(r)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)
	for scanner.Scan() {
		if m.onLog != nil {
			m.onLog(source, scanner.Text())
		}
	}
}

func (m *Manager) wait() {
	var state *os.ProcessState
	if m.cmd != nil && m.cmd.Process != nil {
		state, _ = m.cmd.Process.Wait()
	}

	m.mu.Lock()
	m.running = false
	done := m.waitDone
	m.waitDone = nil
	m.mu.Unlock()

	if done != nil {
		close(done)
	}

	if m.onLog != nil {
		if state != nil {
			m.onLog("client", fmt.Sprintf("app exited: %s", state.String()))
		} else {
			m.onLog("client", "app exited")
		}
	}

	if m.onExit != nil {
		m.onExit(state)
	}
}

func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running || m.cmd == nil || m.cmd.Process == nil {
		return
	}

	stopProcess(m.cmd.Process)
}

func (m *Manager) Kill() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running || m.cmd == nil || m.cmd.Process == nil {
		return
	}

	killProcess(m.cmd.Process)
}

func (m *Manager) IsRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

func (m *Manager) UpdateBin(binPath string, args []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.binPath = binPath
	m.args = args
}

func (m *Manager) WaitTimeout(timeout time.Duration) bool {
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return true
	}
	done := m.waitDone
	m.mu.Unlock()

	if done == nil {
		return true
	}

	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}
