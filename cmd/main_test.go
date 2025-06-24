package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/0xAtelerix/example"
)

// TestEndToEnd spins up main(), posts a transaction to the /rpc endpoint and
// verifies we get a 2xx response.
func TestEndToEnd(t *testing.T) {
	port := getFreePort(t)

	// temp dirs for clean DB state
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "appchain.mdbx")
	localDB := filepath.Join(tmp, "local.mdbx")
	streamDir := filepath.Join(tmp, "stream")
	txDir := filepath.Join(tmp, "tx")

	// craft os.Args for main()
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{
		"appchain-test-binary",
		"-rpc-port", fmt.Sprintf(":%d", port),
		"-emitter-port", ":0", // 0 → let OS choose, we don’t care in the test
		"-db-path", dbPath,
		"-local-db-path", localDB,
		"-stream-dir", streamDir,
		"-tx-dir", txDir,
	}

	go main() // NB: main() never returns except on fatal, so run in goroutine

	// wait until HTTP service is up
	rpcURL := fmt.Sprintf("http://127.0.0.1:%d/rpc", port)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := waitUntil(ctx, func() bool {
		resp, err := http.Get(rpcURL) // GET is fine; we only care the port is bound.
		if err != nil {
			return false
		}

		resp.Body.Close()
		return true
	}); err != nil {
		t.Fatalf("JSON-RPC service never became ready: %v", err)
	}

	// build & send a transaction
	tx := example.ExampleTransaction{
		Sender: "Vasya",
		Value:  42,
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(tx); err != nil {
		t.Fatalf("encode tx: %v", err)
	}

	resp, err := http.Post(rpcURL, "application/json", &buf)
	if err != nil {
		t.Fatalf("POST /rpc: %v", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		t.Fatalf("unexpected HTTP status: %s", resp.Status)
	}

	// graceful shutdown
	// The real program listens for SIGINT/SIGTERM,
	// so use the same mechanism to drain goroutines.
	proc, _ := os.FindProcess(os.Getpid())
	_ = proc.Signal(syscall.SIGINT)

	// Give main() a moment to tear down so the test runner’s
	// goroutine leak detector stays quiet.
	time.Sleep(500 * time.Millisecond)

	t.Log("Success!")
}

func getFreePort(t *testing.T) int {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("pick port: %v", err)
	}

	port := l.Addr().(*net.TCPAddr).Port

	err = l.Close()
	if err != nil {
		t.Fatalf("close port: %v", err)
	}

	return port
}

// waitUntil serves as a tiny helper that polls f() until it returns true or ctx
// expires.
func waitUntil(ctx context.Context, f func() bool) error {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if f() {
				return nil
			}
		}
	}
}
