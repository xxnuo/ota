package logger

import (
	"io"
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/xxnuo/ota/internal/config"
)

var Log zerolog.Logger

func Init(component string) {
	config.EnsureDirs()
	var logFile string
	switch component {
	case "server":
		logFile = config.ServerLogFile()
	case "client":
		logFile = config.ClientLogFile()
	default:
		logFile = config.ServerLogFile()
	}

	f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		Log = zerolog.New(os.Stderr).With().Timestamp().Str("component", component).Logger()
		return
	}

	consoleWriter := zerolog.ConsoleWriter{
		Out:        os.Stderr,
		TimeFormat: time.RFC3339,
	}

	multi := io.MultiWriter(consoleWriter, f)
	Log = zerolog.New(multi).With().Timestamp().Str("component", component).Logger()
}

func InitSilent(component string) {
	config.EnsureDirs()
	var logFile string
	switch component {
	case "server":
		logFile = config.ServerLogFile()
	case "client":
		logFile = config.ClientLogFile()
	default:
		logFile = config.ServerLogFile()
	}

	f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		Log = zerolog.New(io.Discard)
		return
	}

	Log = zerolog.New(f).With().Timestamp().Str("component", component).Logger()
}
