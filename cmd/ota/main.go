package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/xxnuo/ota/internal/client"
	"github.com/xxnuo/ota/internal/config"
	"github.com/xxnuo/ota/internal/logger"
	"github.com/xxnuo/ota/internal/server"
)

var rootCmd = &cobra.Command{
	Use:   "ota",
	Short: "OTA - Over The Air code sync & hot reload",
}

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start server daemon",
	RunE:  runServer,
}

var serverStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop server",
	RunE:  runServerStop,
}

var serverRestartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart server",
	RunE:  runServerRestart,
}

var serverKillCmd = &cobra.Command{
	Use:   "kill",
	Short: "Force kill server",
	RunE:  runServerKill,
}

var clientCmd = &cobra.Command{
	Use:   "client",
	Short: "Start client and connect to server",
	RunE:  runClient,
}

var cmdCmd = &cobra.Command{
	Use:   "cmd",
	Short: "Manage the hot-reload command",
}

var cmdStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the command",
	RunE:  runCmdAction("start"),
}

var cmdStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the command",
	RunE:  runCmdAction("stop"),
}

var cmdKillCmd = &cobra.Command{
	Use:   "kill",
	Short: "Kill the command",
	RunE:  runCmdAction("kill"),
}

var cmdRestartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart the command",
	RunE:  runCmdAction("restart"),
}

var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: "View logs",
	RunE:  runLogs,
}

var execCmd = &cobra.Command{
	Use:   "exec [command]",
	Short: "Execute command on remote client",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runExec,
}

var psCmd = &cobra.Command{
	Use:   "ps",
	Short: "Show connection info",
	RunE:  runPs,
}

var disconnectCmd = &cobra.Command{
	Use:   "disconnect [server|client]",
	Short: "Disconnect a connection",
	Args:  cobra.ExactArgs(1),
	RunE:  runDisconnect,
}

var (
	flagPort       int
	flagWorkDir    string
	flagServer     string
	flagCommand    string
	flagFollow     bool
	flagForeground bool
	flagTarget     string
	flagRestart    bool
)

func init() {
	serverCmd.Flags().IntVarP(&flagPort, "port", "p", 9867, "Server port")
	serverCmd.Flags().StringVarP(&flagWorkDir, "dir", "d", ".", "Working directory")
	serverCmd.Flags().BoolVar(&flagForeground, "foreground", false, "Run in foreground (default: daemon)")

	clientCmd.Flags().StringVarP(&flagServer, "server", "s", "", "Server address (or OTA_SERVER env)")
	clientCmd.Flags().StringVarP(&flagWorkDir, "dir", "d", ".", "Working directory")
	clientCmd.Flags().StringVarP(&flagCommand, "cmd", "c", "", "Hot-reload command (or OTA_CMD env)")
	clientCmd.Flags().BoolVarP(&flagRestart, "restart", "r", false, "Restart cmd on file sync (for non-hot-reload tools)")

	logsCmd.Flags().BoolVarP(&flagFollow, "follow", "f", false, "Follow log output")

	execCmd.Flags().StringVarP(&flagTarget, "target", "t", "", "Target client workDir")

	serverCmd.AddCommand(serverStopCmd, serverRestartCmd, serverKillCmd)
	cmdCmd.AddCommand(cmdStartCmd, cmdStopCmd, cmdKillCmd, cmdRestartCmd)
	rootCmd.AddCommand(serverCmd, clientCmd, cmdCmd, logsCmd, execCmd, psCmd, disconnectCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func runServer(cmd *cobra.Command, args []string) error {
	if !flagForeground {
		return startDaemon()
	}

	logger.InitSilent("server")

	srv := server.New(flagWorkDir, flagPort)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		srv.Stop()
	}()

	return srv.Start()
}

func startDaemon() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return err
	}

	absDir, err := filepath.Abs(flagWorkDir)
	if err != nil {
		return err
	}

	cmd := exec.Command(exe, "server", "--foreground", "--port", strconv.Itoa(flagPort), "--dir", absDir)
	cmd.Dir = absDir
	cmd.Env = os.Environ()
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start daemon: %w", err)
	}

	pid := cmd.Process.Pid
	cmd.Process.Release()
	fmt.Printf("Server started (PID: %d), logs: %s\n", pid, config.ServerLogFile())
	return nil
}

func runServerStop(cmd *cobra.Command, args []string) error {
	return signalServer(syscall.SIGTERM, "stopped")
}

func runServerRestart(cmd *cobra.Command, args []string) error {
	signalServer(syscall.SIGTERM, "stopped")
	time.Sleep(1 * time.Second)
	return startDaemon()
}

func runServerKill(cmd *cobra.Command, args []string) error {
	return signalServer(syscall.SIGKILL, "killed")
}

func signalServer(sig syscall.Signal, action string) error {
	pid, err := config.LoadPid(config.ServerPidFile())
	if err != nil {
		return fmt.Errorf("server not running (no pid file)")
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		config.RemovePid(config.ServerPidFile())
		return fmt.Errorf("server process not found")
	}

	if err := proc.Signal(sig); err != nil {
		config.RemovePid(config.ServerPidFile())
		return fmt.Errorf("failed to %s server: %w", action, err)
	}

	config.RemovePid(config.ServerPidFile())
	fmt.Printf("Server %s (PID: %d)\n", action, pid)
	return nil
}

func runClient(cmd *cobra.Command, args []string) error {
	logger.Init("client")

	serverURL := flagServer
	if serverURL == "" {
		serverURL = os.Getenv("OTA_SERVER")
	}
	if serverURL == "" {
		return fmt.Errorf("server address required (--server or OTA_SERVER env)")
	}

	if !strings.HasPrefix(serverURL, "ws://") && !strings.HasPrefix(serverURL, "wss://") && !strings.HasPrefix(serverURL, "http") {
		serverURL = "ws://" + serverURL
	}

	command := flagCommand
	if command == "" {
		command = os.Getenv("OTA_CMD")
	}

	c := client.New(serverURL, flagWorkDir, command, flagRestart)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		c.Stop()
	}()

	return c.Start()
}

func runCmdAction(action string) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		pid, err := config.LoadPid(config.ClientPidFile())
		if err != nil {
			return fmt.Errorf("client not running")
		}

		var sig syscall.Signal
		switch action {
		case "start":
			sig = syscall.SIGUSR1
		case "stop":
			sig = syscall.SIGUSR2
		case "kill":
			sig = syscall.SIGURG
		case "restart":
			sig = syscall.SIGHUP
		}

		proc, err := os.FindProcess(pid)
		if err != nil {
			return fmt.Errorf("client process not found")
		}

		if err := proc.Signal(sig); err != nil {
			return fmt.Errorf("failed to send signal: %w", err)
		}

		fmt.Printf("Command %s signal sent to client (PID: %d)\n", action, pid)
		return nil
	}
}

func runLogs(cmd *cobra.Command, args []string) error {
	logFiles := []string{config.ServerLogFile(), config.ClientLogFile()}
	var activeFile string
	for _, f := range logFiles {
		if _, err := os.Stat(f); err == nil {
			activeFile = f
			break
		}
	}
	if activeFile == "" {
		return fmt.Errorf("no log files found")
	}

	if flagFollow {
		return tailFollow(activeFile)
	}

	data, err := os.ReadFile(activeFile)
	if err != nil {
		return err
	}
	fmt.Print(string(data))
	return nil
}

func tailFollow(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	f.Seek(0, 2)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	scanner := bufio.NewScanner(f)
	for {
		select {
		case <-sigCh:
			return nil
		default:
			if scanner.Scan() {
				fmt.Println(scanner.Text())
			} else {
				time.Sleep(100 * time.Millisecond)
				scanner = bufio.NewScanner(f)
			}
		}
	}
}

func runExec(cmd *cobra.Command, args []string) error {
	command := strings.Join(args, " ")

	cfg, err := config.LoadServerConfig()
	if err != nil {
		return fmt.Errorf("server config not found, ensure server is running")
	}

	fmt.Printf("Sending exec to server at %s: %s\n", cfg.Address, command)
	fmt.Printf("Target workDir: %s\n", flagTarget)
	fmt.Println("(exec forwarding requires active server process - use ota server to start)")

	return nil
}

func runPs(cmd *cobra.Command, args []string) error {
	fmt.Println("=== Server Info ===")
	cfg, err := config.LoadServerConfig()
	if err != nil {
		fmt.Println("  Server: not running")
	} else {
		pid, _ := config.LoadPid(config.ServerPidFile())
		fmt.Printf("  PID:     %d\n", pid)
		fmt.Printf("  Address: %s\n", cfg.Address)
		fmt.Printf("  WorkDir: %s\n", cfg.WorkDir)

		state, err := config.LoadServerState()
		if err == nil {
			clients := state.GetClients()
			if len(clients) > 0 {
				fmt.Printf("  Clients: %d\n", len(clients))
				for _, c := range clients {
					fmt.Printf("    - ID: %s, Host: %s, WorkDir: %s, Addr: %s\n", c.ID, c.Hostname, c.WorkDir, c.Addr)
				}
			} else {
				fmt.Println("  Clients: none")
			}
		}
	}

	fmt.Println()
	fmt.Println("=== Client Info ===")
	ccfg, err := config.LoadClientConfig()
	if err != nil {
		fmt.Println("  Client: not running")
	} else {
		pid, _ := config.LoadPid(config.ClientPidFile())
		fmt.Printf("  PID:     %d\n", pid)
		fmt.Printf("  Server:  %s\n", ccfg.ServerURL)
		fmt.Printf("  WorkDir: %s\n", ccfg.WorkDir)
		fmt.Printf("  Command: %s\n", ccfg.Command)
	}

	return nil
}

func runDisconnect(cmd *cobra.Command, args []string) error {
	target := args[0]
	switch target {
	case "server":
		return signalServer(syscall.SIGTERM, "disconnected")
	case "client":
		pid, err := config.LoadPid(config.ClientPidFile())
		if err != nil {
			return fmt.Errorf("client not running")
		}
		proc, err := os.FindProcess(pid)
		if err != nil {
			return fmt.Errorf("client process not found")
		}
		proc.Signal(syscall.SIGTERM)
		config.RemovePid(config.ClientPidFile())
		fmt.Printf("Client disconnected (PID: %d)\n", pid)
		return nil
	default:
		return fmt.Errorf("unknown target: %s (use 'server' or 'client')", target)
	}
}
