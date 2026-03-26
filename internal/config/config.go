package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

var configDir string

func init() {
	home, err := os.UserHomeDir()
	if err != nil {
		configDir = "/tmp/ota"
	} else {
		configDir = filepath.Join(home, ".config", "ota")
	}
}

func Dir() string {
	return configDir
}

func ServerDir() string {
	return filepath.Join(configDir, "server")
}

func ClientDir() string {
	return filepath.Join(configDir, "client")
}

func ServerLogDir() string {
	return filepath.Join(ServerDir(), "logs")
}

func ClientLogDir() string {
	return filepath.Join(ClientDir(), "logs")
}

func EnsureDirs() error {
	dirs := []string{ServerDir(), ClientDir(), ServerLogDir(), ClientLogDir()}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return err
		}
	}
	return nil
}

type ServerConfig struct {
	PID     int    `json:"pid"`
	Address string `json:"address"`
	WorkDir string `json:"work_dir"`
	Port    int    `json:"port"`
}

type ClientConfig struct {
	PID       int    `json:"pid"`
	ServerURL string `json:"server_url"`
	WorkDir   string `json:"work_dir"`
	Command   string `json:"command"`
}

type ServerState struct {
	mu      sync.RWMutex
	Config  ServerConfig `json:"config"`
	Clients []ClientInfo `json:"clients"`
}

type ClientInfo struct {
	ID       string `json:"id"`
	Hostname string `json:"hostname"`
	WorkDir  string `json:"work_dir"`
	Addr     string `json:"addr"`
}

func LoadServerConfig() (*ServerConfig, error) {
	path := filepath.Join(ServerDir(), "config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg ServerConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func SaveServerConfig(cfg *ServerConfig) error {
	if err := EnsureDirs(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(ServerDir(), "config.json"), data, 0644)
}

func LoadClientConfig() (*ClientConfig, error) {
	path := filepath.Join(ClientDir(), "config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg ClientConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func SaveClientConfig(cfg *ClientConfig) error {
	if err := EnsureDirs(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(ClientDir(), "config.json"), data, 0644)
}

func ServerPidFile() string {
	return filepath.Join(ServerDir(), "server.pid")
}

func ClientPidFile() string {
	return filepath.Join(ClientDir(), "client.pid")
}

func SavePid(pidFile string, pid int) error {
	if err := EnsureDirs(); err != nil {
		return err
	}
	data, _ := json.Marshal(map[string]int{"pid": pid})
	return os.WriteFile(pidFile, data, 0644)
}

func LoadPid(pidFile string) (int, error) {
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return 0, err
	}
	var m map[string]int
	if err := json.Unmarshal(data, &m); err != nil {
		return 0, err
	}
	return m["pid"], nil
}

func RemovePid(pidFile string) {
	os.Remove(pidFile)
}

func ServerLogFile() string {
	return filepath.Join(ServerLogDir(), "server.log")
}

func ClientLogFile() string {
	return filepath.Join(ClientLogDir(), "client.log")
}

func NewServerState() *ServerState {
	return &ServerState{}
}

func (s *ServerState) AddClient(info ClientInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, c := range s.Clients {
		if c.ID == info.ID {
			s.Clients[i] = info
			return
		}
	}
	s.Clients = append(s.Clients, info)
}

func (s *ServerState) RemoveClient(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, c := range s.Clients {
		if c.ID == id {
			s.Clients = append(s.Clients[:i], s.Clients[i+1:]...)
			return
		}
	}
}

func (s *ServerState) GetClients() []ClientInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]ClientInfo, len(s.Clients))
	copy(result, s.Clients)
	return result
}

func (s *ServerState) FindClientByWorkDir(workDir string) *ClientInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, c := range s.Clients {
		if c.WorkDir == workDir {
			return &c
		}
	}
	return nil
}

func (s *ServerState) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(ServerDir(), "state.json"), data, 0644)
}

func LoadServerState() (*ServerState, error) {
	path := filepath.Join(ServerDir(), "state.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return NewServerState(), nil
	}
	var state ServerState
	if err := json.Unmarshal(data, &state); err != nil {
		return NewServerState(), nil
	}
	return &state, nil
}
