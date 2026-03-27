package process

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
)

type Manager struct {
	mu      sync.Mutex
	cmd     *exec.Cmd
	binPath string
	args    []string
	workDir string
	running bool
	onLog   func(source, line string)
}

func New(binPath string, args []string, workDir string, onLog func(source, line string)) *Manager {
	return &Manager{
		binPath: binPath,
		args:    args,
		workDir: workDir,
		onLog:   onLog,
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
	if m.cmd != nil && m.cmd.Process != nil {
		state, _ := m.cmd.Process.Wait()
		m.mu.Lock()
		m.running = false
		m.mu.Unlock()
		if m.onLog != nil {
			if state != nil {
				m.onLog("client", fmt.Sprintf("app exited: %s", state.String()))
			} else {
				m.onLog("client", "app exited")
			}
		}
	}
}

func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running || m.cmd == nil || m.cmd.Process == nil {
		return
	}

	stopProcess(m.cmd.Process)
	m.running = false
}

func (m *Manager) Kill() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running || m.cmd == nil || m.cmd.Process == nil {
		return
	}

	killProcess(m.cmd.Process)
	m.running = false
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
