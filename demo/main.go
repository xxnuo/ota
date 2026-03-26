package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

var version = "1"

func main() {
	hostname, _ := os.Hostname()
	startTime := time.Now()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Hello OTA! version=%s host=%s uptime=%s\n", version, hostname, time.Since(startTime).Round(time.Second))
	})

	log.Printf("demo v%s starting on :8080 (host=%s)", version, hostname)
	log.Fatal(http.ListenAndServe(":8080", nil))
}
