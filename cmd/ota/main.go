package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/xxnuo/ota/internal/client"
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

var psCmd = &cobra.Command{
	Use:   "ps",
	Short: "List connected clients",
	RunE:  runPs,
}

var (
	flagPort   int
	flagServer string
	flagDir    string
	flagArgs   string
	flagID     string
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

	disconnectCmd.Flags().StringVar(&flagID, "id", "", "Target client (or OTA_ID env)")
	disconnectCmd.Flags().StringVar(&flagID, "name", "", "Alias for --id")

	rootCmd.AddCommand(serverCmd, clientCmd, sendCmd, disconnectCmd, psCmd)
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
