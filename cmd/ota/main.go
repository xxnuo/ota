package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/cobra"
	"github.com/xxnuo/ota/internal/client"
	"github.com/xxnuo/ota/internal/clog"
	"github.com/xxnuo/ota/internal/server"
)

const portFile = ".ota"

var rootCmd = &cobra.Command{
	Use:   "ota",
	Short: "OTA - push binary to remote and run",
}

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start server (foreground)",
	RunE:  runServer,
}

var clientCmd = &cobra.Command{
	Use:   "client",
	Short: "Connect to server and wait for binaries",
	RunE:  runClient,
}

var sendCmd = &cobra.Command{
	Use:   "send <file>",
	Short: "Send binary to connected client",
	Args:  cobra.ExactArgs(1),
	RunE:  runSend,
}

var disconnectCmd = &cobra.Command{
	Use:   "disconnect",
	Short: "Disconnect a client",
	RunE:  runDisconnect,
}

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the running app (SIGTERM)",
	RunE:  runControlCmd("stop"),
}

var killCmd = &cobra.Command{
	Use:   "kill",
	Short: "Kill the running app (SIGKILL)",
	RunE:  runControlCmd("kill"),
}

var restartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart the running app",
	RunE:  runControlCmd("restart"),
}

var execCmd = &cobra.Command{
	Use:   "exec <command>",
	Short: "Execute a command on the client",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runExec,
}

var psCmd = &cobra.Command{
	Use:   "ps",
	Short: "List connected clients",
	RunE:  runPs,
}

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Watch directory for source changes and run command",
	RunE:  runWatch,
}

var (
	flagPort     int
	flagServer   string
	flagDir      string
	flagArgs     string
	flagID       string
	flagWatchDir  string
	flagWatchCmd  string
	flagDebounce  int
	flagExts      string
)

func init() {
	serverCmd.Flags().IntVarP(&flagPort, "port", "p", 0, "Server port (0 = auto)")

	clientCmd.Flags().StringVarP(&flagServer, "server", "s", "", "Server address (or OTA_SERVER env)")
	clientCmd.Flags().StringVarP(&flagDir, "dir", "d", ".", "Working directory")
	clientCmd.Flags().StringVar(&flagID, "id", "", "Client identifier (or OTA_ID env)")
	clientCmd.Flags().StringVar(&flagID, "name", "", "Alias for --id")

	sendCmd.Flags().StringVar(&flagArgs, "args", "", "Arguments for the binary")
	sendCmd.Flags().StringVar(&flagID, "id", "", "Target client (or OTA_ID env)")
	sendCmd.Flags().StringVar(&flagID, "name", "", "Alias for --id")

	for _, cmd := range []*cobra.Command{disconnectCmd, stopCmd, killCmd, restartCmd, execCmd} {
		cmd.Flags().StringVar(&flagID, "id", "", "Target client (or OTA_ID env)")
		cmd.Flags().StringVar(&flagID, "name", "", "Alias for --id")
	}

	watchCmd.Flags().StringVarP(&flagWatchDir, "dir", "d", ".", "Directory to watch")
	watchCmd.Flags().StringVarP(&flagWatchCmd, "cmd", "c", "", "Command to run on change (required)")
	watchCmd.Flags().IntVarP(&flagDebounce, "debounce", "i", 500, "Debounce interval in milliseconds")
	watchCmd.Flags().StringVar(&flagExts, "ext", "", "File extensions to watch, comma separated (e.g. go,js,py). Empty = all files")
	watchCmd.MarkFlagRequired("cmd")

	rootCmd.AddCommand(serverCmd, clientCmd, sendCmd, disconnectCmd, stopCmd, killCmd, restartCmd, execCmd, psCmd, watchCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func runServer(cmd *cobra.Command, args []string) error {
	srv := server.New(flagPort)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println()
		os.Remove(portFile)
		srv.Stop()
	}()

	port, err := srv.StartAndGetPort()
	if err != nil {
		return err
	}

	os.WriteFile(portFile, []byte(strconv.Itoa(port)), 0644)
	defer os.Remove(portFile)

	return srv.Serve()
}

func runClient(cmd *cobra.Command, args []string) error {
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

	id := flagID
	if id == "" {
		id = os.Getenv("OTA_ID")
	}

	c := client.New(serverURL, flagDir, id)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		c.Stop()
	}()

	return c.Start()
}

func loadPort() (int, error) {
	data, err := os.ReadFile(portFile)
	if err != nil {
		return 0, fmt.Errorf("no server running in this directory (missing %s file)", portFile)
	}
	port, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("invalid port in %s", portFile)
	}
	return port, nil
}

func resolveID() string {
	if flagID != "" {
		return flagID
	}
	if env := os.Getenv("OTA_ID"); env != "" {
		return env
	}
	return ""
}

func runSend(cmd *cobra.Command, args []string) error {
	port, err := loadPort()
	if err != nil {
		return err
	}

	filePath := args[0]
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return err
	}

	content, err := os.ReadFile(absPath)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	filename := filepath.Base(absPath)
	reqURL := fmt.Sprintf("http://localhost:%d/send?filename=%s", port, filename)
	if id := resolveID(); id != "" {
		reqURL += "&id=" + id
	}
	if flagArgs != "" {
		reqURL += "&args=" + flagArgs
	}

	resp, err := http.Post(reqURL, "application/octet-stream", bytes.NewReader(content))
	if err != nil {
		return fmt.Errorf("send failed (server on port %d): %w", port, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server: %s", strings.TrimSpace(string(body)))
	}

	fmt.Print(string(body))
	return nil
}

func runDisconnect(cmd *cobra.Command, args []string) error {
	port, err := loadPort()
	if err != nil {
		return err
	}

	reqURL := fmt.Sprintf("http://localhost:%d/disconnect", port)
	if id := resolveID(); id != "" {
		reqURL += "?id=" + id
	}
	resp, err := http.Post(reqURL, "", nil)
	if err != nil {
		return fmt.Errorf("disconnect failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server: %s", strings.TrimSpace(string(body)))
	}

	fmt.Print(string(body))
	return nil
}

func runPs(cmd *cobra.Command, args []string) error {
	port, err := loadPort()
	if err != nil {
		return err
	}

	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/ps", port))
	if err != nil {
		return fmt.Errorf("ps failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	fmt.Print(string(body))
	return nil
}

func runControlCmd(action string) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		port, err := loadPort()
		if err != nil {
			return err
		}

		reqURL := fmt.Sprintf("http://localhost:%d/%s", port, action)
		if id := resolveID(); id != "" {
			reqURL += "?id=" + id
		}
		resp, err := http.Post(reqURL, "", nil)
		if err != nil {
			return fmt.Errorf("%s failed: %w", action, err)
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("server: %s", strings.TrimSpace(string(body)))
		}

		fmt.Print(string(body))
		return nil
	}
}

func runExec(cmd *cobra.Command, args []string) error {
	port, err := loadPort()
	if err != nil {
		return err
	}

	cmdStr := strings.Join(args, " ")
	reqURL := fmt.Sprintf("http://localhost:%d/exec?cmd=%s", port, url.QueryEscape(cmdStr))
	if id := resolveID(); id != "" {
		reqURL += "&id=" + id
	}
	resp, err := http.Post(reqURL, "", nil)
	if err != nil {
		return fmt.Errorf("exec failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server: %s", strings.TrimSpace(string(body)))
	}

	fmt.Print(string(body))
	return nil
}

var skipDirs = map[string]bool{
	"node_modules": true, "vendor": true, "__pycache__": true,
	".git": true, ".hg": true, ".svn": true,
}

func runWatch(cmd *cobra.Command, args []string) error {
	dir, err := filepath.Abs(flagWatchDir)
	if err != nil {
		return err
	}

	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("invalid directory: %s", dir)
	}

	var extSet map[string]bool
	if flagExts != "" {
		extSet = make(map[string]bool)
		for _, e := range strings.Split(flagExts, ",") {
			e = strings.TrimSpace(e)
			if e != "" {
				if !strings.HasPrefix(e, ".") {
					e = "." + e
				}
				extSet[e] = true
			}
		}
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create watcher: %w", err)
	}
	defer watcher.Close()

	watchDirRecursive(watcher, dir)

	debounce := time.Duration(flagDebounce) * time.Millisecond
	if debounce < 100*time.Millisecond {
		debounce = 100 * time.Millisecond
	}

	clog.Info("watching %s (debounce %s, cmd: %s)", dir, debounce, flagWatchCmd)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	var timer *time.Timer

	for {
		select {
		case <-sigCh:
			clog.Info("stopped")
			return nil
		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			if event.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Remove|fsnotify.Rename) == 0 {
				continue
			}
			name := filepath.Base(event.Name)
			if strings.HasPrefix(name, ".") {
				continue
			}
			if extSet != nil && !extSet[filepath.Ext(name)] {
				continue
			}
			if event.Op&fsnotify.Create != 0 {
				if fi, err := os.Stat(event.Name); err == nil && fi.IsDir() {
					watchDirRecursive(watcher, event.Name)
				}
			}
			if timer != nil {
				timer.Stop()
			}
			timer = time.AfterFunc(debounce, func() {
				clog.Info("change detected, running: %s", flagWatchCmd)
				execWatch(flagWatchCmd)
			})
		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			clog.Error("watcher: %v", err)
		}
	}
}

func watchDirRecursive(watcher *fsnotify.Watcher, root string) {
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			return nil
		}
		name := info.Name()
		if skipDirs[name] || (strings.HasPrefix(name, ".") && path != root) {
			return filepath.SkipDir
		}
		watcher.Add(path)
		return nil
	})
}

func execWatch(cmdStr string) {
	c := exec.Command("sh", "-c", cmdStr)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		clog.Error("command failed: %v", err)
	}
}
