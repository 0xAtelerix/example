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
	"testing"
	"time"

	"github.com/0xAtelerix/sdk/gosdk"
	"github.com/ledgerwatch/erigon-lib/kv"
	"github.com/ledgerwatch/erigon-lib/kv/mdbx"
	mdbxlog "github.com/ledgerwatch/log/v3"
	"github.com/stretchr/testify/require"

	"github.com/0xAtelerix/example/application"
)

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

// TestEndToEnd spins up the appchain, posts a transaction to the /rpc endpoint and
// verifies we get a 2xx response.
func TestEndToEnd(t *testing.T) {
	port := getFreePort(t)
	dataDir := t.TempDir()
	chainID := uint64(1001)

	// Create required directories and databases
	// SDK's Init() expects these paths to exist
	txBatchPath := gosdk.TxBatchPath(dataDir, chainID)
	eventsPath := gosdk.EventsPath(dataDir)

	require.NoError(t, os.MkdirAll(txBatchPath, 0o755))
	require.NoError(t, os.MkdirAll(eventsPath, 0o755))

	// Create TxBatchDB (normally created by pelacli's fetcher)
	err := createEmptyMDBXDatabase(txBatchPath, gosdk.TxBucketsTables())
	require.NoError(t, err, "create empty txBatch database")

	// Create events file (normally created by pelacli)
	eventsFile := filepath.Join(eventsPath, "epoch_1.data")
	f, err := os.Create(eventsFile)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	// Create config with test values
	cfg := &Config{
		ChainID:     chainID,
		DataDir:     dataDir,
		EmitterPort: ":0", // Let OS choose
		RPCPort:     fmt.Sprintf(":%d", port),
	}

	// Create cancellable context for the test
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	// Run appchain in background
	go func() {
		_ = Run(ctx, cfg)
	}()

	// Wait until HTTP service is up
	rpcURL := fmt.Sprintf("http://127.0.0.1:%d/rpc", port)

	waitCtx, waitCancel := context.WithTimeout(ctx, 5*time.Second)
	defer waitCancel()

	err = waitUntil(waitCtx, func() bool {
		req, reqErr := http.NewRequestWithContext(waitCtx, http.MethodGet, rpcURL, nil)
		if reqErr != nil {
			return false
		}

		resp, respErr := http.DefaultClient.Do(req)
		if respErr != nil {
			return false
		}

		_ = resp.Body.Close()

		return true
	})
	require.NoError(t, err, "JSON-RPC service never became ready")

	// Build & send a transaction
	tx := application.Transaction[application.Receipt]{
		Sender: "Vasya",
		Value:  42,
		TxHash: "deadbeef",
	}

	var buf bytes.Buffer

	err = json.NewEncoder(&buf).Encode(tx)
	require.NoError(t, err, "encode tx")

	req, err := http.NewRequestWithContext(
		waitCtx,
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

	require.True(t, resp.StatusCode >= 200 && resp.StatusCode < 300,
		"unexpected HTTP status: %s", resp.Status)

	// Cancel context to trigger graceful shutdown
	cancel()

	// Give Run() a moment to tear down
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

// createEmptyMDBXDatabase creates an empty MDBX database that can be opened in readonly mode.
func createEmptyMDBXDatabase(dbPath string, tableCfg kv.TableCfg) error {
	tempDB, err := mdbx.NewMDBX(mdbxlog.New()).
		Path(dbPath).
		WithTableCfg(func(_ kv.TableCfg) kv.TableCfg {
			return tableCfg
		}).
		Open()
	if err != nil {
		return err
	}

	// Close immediately - we just needed to create the database files
	tempDB.Close()

	return nil
}
