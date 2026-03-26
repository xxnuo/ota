package clog

import (
	"fmt"
	"os"
	"time"
)

const (
	reset   = "\033[0m"
	gray    = "\033[90m"
	red     = "\033[31m"
	green   = "\033[32m"
	yellow  = "\033[33m"
	cyan    = "\033[36m"
	magenta = "\033[35m"
	blue    = "\033[34m"
	bold    = "\033[1m"
)

func timestamp() string {
	return gray + time.Now().Format("15:04:05") + reset
}

func Server(format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	fmt.Fprintf(os.Stderr, "%s %s%s[server]%s %s\n", timestamp(), bold, magenta, reset, msg)
}

func Client(format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	fmt.Fprintf(os.Stderr, "%s %s%s[client]%s %s\n", timestamp(), bold, cyan, reset, msg)
}

func App(source, line string) {
	color := green
	if source == "app:err" {
		color = yellow
	}
	fmt.Fprintf(os.Stderr, "%s %s[%s]%s %s\n", timestamp(), color, source, reset, line)
}

func Remote(clientLabel, source, line string) {
	fmt.Printf("%s %s[%s %s]%s %s\n", timestamp(), blue, clientLabel, source, reset, line)
}

func RemoteSimple(source, line string) {
	fmt.Printf("%s %s[%s]%s %s\n", timestamp(), blue, source, reset, line)
}

func Error(format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	fmt.Fprintf(os.Stderr, "%s %s%s[error]%s %s\n", timestamp(), bold, red, reset, msg)
}

func Info(format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	fmt.Fprintf(os.Stderr, "%s %s[info]%s %s\n", timestamp(), green, reset, msg)
}
