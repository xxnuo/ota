package main

import (
	"fmt"
	"log"
	"net/http"
	"time"
)

var startTime = time.Now()

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Hello OTA! Version: 2 - Hot Reload Works!\nUptime: %s\n", time.Since(startTime))
	})

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	})

	log.Println("Demo server starting on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
