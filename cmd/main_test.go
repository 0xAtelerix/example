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

	"github.com/stretchr/testify/require"

	"github.com/0xAtelerix/example/application"
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

	go Run(t.Context())

	// wait until HTTP service is up
	rpcURL := fmt.Sprintf("http://127.0.0.1:%d/rpc", port)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := waitUntil(ctx, func() bool {
		// GET is fine; we only care the port is bound.
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rpcURL, nil)
		require.NoError(t, err, "GET req /rpc")

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err, "GET res /rpc")

		err = resp.Body.Close()
		require.NoError(t, err)

		return true
	}); err != nil {
		t.Fatalf("JSON-RPC service never became ready: %v", err)
	}

	// build & send a transaction
	tx := application.Transaction{
		Sender: "Vasya",
		Value:  42,
		TxHash: "deadbeef",
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(tx); err != nil {
		t.Fatalf("encode tx: %v", err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		rpcURL,
		bytes.NewReader(buf.Bytes()),
	)
	require.NoError(t, err, "POST req /rpc")

	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err, "POST res /rpc")

	defer func() {
		require.NoError(t, resp.Body.Close())
	}()

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

// not safe to use in concurrent env
func getFreePort(t *testing.T) int {
	t.Helper()

	l, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
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
