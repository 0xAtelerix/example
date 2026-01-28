package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
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

// TestEndToEnd spins up the appchain, posts a request to the /rpc endpoint and
// verifies we get a 2xx response.
func TestEndToEnd(t *testing.T) {
	port := getFreePort(t)
	dataDir := t.TempDir()
	chainID := uint64(42)

	// Create required directories
	txBatchPath := gosdk.TxBatchPathForChain(dataDir, chainID)
	eventsPath := gosdk.EventsPath(dataDir)

	require.NoError(t, os.MkdirAll(txBatchPath, 0o755))
	require.NoError(t, os.MkdirAll(eventsPath, 0o755))

	// Create empty TxBatchDB
	err := createEmptyMDBXDatabase(txBatchPath, gosdk.TxBucketsTables())
	require.NoError(t, err, "create empty txBatch database")

	// Create test config with embedded SDK config and bridge config
	cfg := &application.AppConfig{
		InitConfig: gosdk.InitConfig{
			ChainID:        &chainID,
			DataDir:        dataDir,
			EmitterPort:    ":0",
			RPCPort:        fmt.Sprintf(":%d", port),
			RequiredChains: []uint64{},
			CustomTables:   application.Tables(),
		},
		Bridge: application.BridgeConfig{
			Contracts:     map[uint64]string{},
			TokenMappings: map[uint64]map[string]string{},
		},
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	// Setup logger
	ctx = gosdk.SetupLogger(ctx, 1) // Info level

	// Run appchain in background
	go func() {
		if runErr := Run(ctx, cfg); runErr != nil {
			t.Logf("Run error: %v", runErr)
		}
	}()

	// Wait for HTTP service
	rpcURL := fmt.Sprintf("http://127.0.0.1:%d/rpc", port)

	waitCtx, waitCancel := context.WithTimeout(ctx, 5*time.Second)
	defer waitCancel()

	err = waitUntil(waitCtx, func() bool {
		req, _ := http.NewRequestWithContext(waitCtx, http.MethodGet, rpcURL, nil)

		resp, respErr := http.DefaultClient.Do(req)
		if respErr != nil {
			return false
		}

		resp.Body.Close()

		return true
	})
	require.NoError(t, err, "JSON-RPC service never became ready")

	// Test getBridgeStatus RPC method
	rpcRequest := map[string]any{
		"jsonrpc": "2.0",
		"method":  "getBridgeStatus",
		"params":  []any{map[string]string{"bridgeId": "0x1234"}},
		"id":      1,
	}

	var buf bytes.Buffer
	require.NoError(t, json.NewEncoder(&buf).Encode(rpcRequest))

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

	require.True(
		t,
		resp.StatusCode >= 200 && resp.StatusCode < 300,
		"unexpected HTTP status: %s",
		resp.Status,
	)

	// Verify we get a valid JSON-RPC response (error expected since bridge doesn't exist)
	var rpcResp map[string]any

	err = json.NewDecoder(resp.Body).Decode(&rpcResp)
	require.NoError(t, err, "decode rpc response")
	// We expect an error since the bridge doesn't exist, but the RPC endpoint works
	require.NotNil(t, rpcResp["error"], "expected error in response for non-existent bridge")

	cancel()
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

// createEmptyMDBXDatabase creates an empty MDBX database that can be opened in readonly mode
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
