// cmd/healthcheck/main.go
//
// A minimal, zero-dependency HTTP health-check helper utility.
// Used inside distroless containers (which lack wget/curl) for docker-compose healthchecks.

package main

import (
	"fmt"
	"net/http"
	"os"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: healthcheck <url>")
		os.Exit(1)
	}

	url := os.Args[1]
	client := http.Client{
		Timeout: 3 * time.Second,
	}

	resp, err := client.Get(url)
	if err != nil {
		fmt.Printf("Health check failed: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		fmt.Printf("Health check failed: HTTP Status %d\n", resp.StatusCode)
		os.Exit(1)
	}

	fmt.Println("Health check passed.")
	os.Exit(0)
}
