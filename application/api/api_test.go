package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/0xAtelerix/sdk/gosdk"
	"github.com/0xAtelerix/sdk/gosdk/appblock"
	"github.com/0xAtelerix/sdk/gosdk/rpc"
	"github.com/0xAtelerix/sdk/gosdk/txpool"
	"github.com/holiman/uint256"
	"github.com/ledgerwatch/erigon-lib/kv"
	"github.com/ledgerwatch/erigon-lib/kv/mdbx"
	"github.com/ledgerwatch/erigon-lib/kv/memdb"
	mdbxlog "github.com/ledgerwatch/log/v3"
	"github.com/stretchr/testify/require"

	"github.com/0xAtelerix/example/application"
)

// createTempDBWithBalance creates a temporary in-memory database with test balance data
func createTempDBWithBalance(t *testing.T, user, token string, balance uint64) kv.RoDB {
	t.Helper()

	db := memdb.New("")
	ctx := context.Background()

	// Create tables
	tx, err := db.BeginRw(ctx)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	// Create the accounts bucket/table
	if err := tx.CreateBucket(application.AccountsBucket); err != nil {
		t.Fatalf("Failed to create accounts bucket: %v", err)
	}

	accountKey := application.AccountKey(user, token)

	balanceValue := uint256.NewInt(balance)
	if err := tx.Put(application.AccountsBucket, accountKey, balanceValue.Bytes()); err != nil {
		t.Fatalf("Failed to set test balance: %v", err)
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("Failed to commit transaction: %v", err)
	}

	return db
}

func openTxPoolDB(t *testing.T) kv.RwDB {
	t.Helper()

	db, err := mdbx.NewMDBX(mdbxlog.New()).
		Path(t.TempDir()).
		WithTableCfg(func(_ kv.TableCfg) kv.TableCfg {
			return txpool.Tables()
		}).
		Open()
	require.NoError(t, err)
	t.Cleanup(func() {
		db.Close()
	})

	return db
}

func openBlocksDB(t *testing.T) kv.RwDB {
	t.Helper()

	db, err := mdbx.NewMDBX(mdbxlog.New()).
		Path(t.TempDir()).
		WithTableCfg(func(_ kv.TableCfg) kv.TableCfg {
			return kv.TableCfg{
				gosdk.BlocksBucket: {},
			}
		}).
		Open()
	require.NoError(t, err)
	t.Cleanup(func() {
		db.Close()
	})

	return db
}

func startRPCServer(t *testing.T, server *rpc.StandardRPCServer) string {
	t.Helper()

	resetDefaultServeMux()

	addr := randomLocalAddress(t)
	errServer := make(chan error, 1)

	go func() {
		errServer <- server.StartHTTPServer(t.Context(), addr)
	}()

	baseURL := "http://" + addr

	waitForServerHealthy(t, baseURL+"/health", errServer)

	return baseURL
}

func randomLocalAddress(t *testing.T) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	require.NoError(t, err)

	addr := listener.Addr().String()
	require.NoError(t, listener.Close())

	return addr
}

func waitForServerHealthy(t *testing.T, healthURL string, errServer <-chan error) {
	t.Helper()

	client := &http.Client{
		Timeout: 200 * time.Millisecond,
	}

	const attempts = 20
	for range attempts {
		select {
		case serverErr := <-errServer:
			require.NoError(t, serverErr, "Failed to start HTTP server")

			return
		default:
		}

		req, err := http.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			healthURL,
			http.NoBody,
		)
		require.NoError(t, err)

		resp, err := client.Do(req)
		if err == nil {
			if closeErr := resp.Body.Close(); closeErr != nil {
				t.Errorf("close health response body: %v", closeErr)
			}

			return
		}

		time.Sleep(50 * time.Millisecond)
	}

	t.Fatalf("RPC server at %s did not become healthy", healthURL)
}

func storeBlock(ctx context.Context, t *testing.T, db kv.RwDB, block *application.Block) {
	t.Helper()

	require.NoError(t, appblock.StoreAppBlock(ctx, db, block.BlockNum, block))
}

func resetDefaultServeMux() {
	http.DefaultServeMux = http.NewServeMux()
}

func TestCustomRPC_GetBalance(t *testing.T) {
	// Create temp DB with balance
	db := createTempDBWithBalance(t, "alice", "USDT", 1000)
	defer db.Close()

	ctx := context.Background()

	// Create RPC server and custom RPC
	rpcServer := rpc.NewStandardRPCServer(nil)
	customRPC := NewCustomRPC(rpcServer, db)
	customRPC.AddRPCMethods()

	tests := []struct {
		name            string
		params          []any
		expectedUser    string
		expectedToken   string
		expectedBalance string
		expectError     bool
	}{
		{
			name: "valid balance request",
			params: []any{
				map[string]any{
					"user":  "alice",
					"token": "USDT",
				},
			},
			expectedUser:    "alice",
			expectedToken:   "USDT",
			expectedBalance: "1000",
			expectError:     false,
		},
		{
			name: "zero balance for non-existent account",
			params: []any{
				map[string]any{
					"user":  "bob",
					"token": "USDT",
				},
			},
			expectedUser:    "bob",
			expectedToken:   "USDT",
			expectedBalance: "0",
			expectError:     false,
		},
		{
			name:        "missing parameters",
			params:      []any{},
			expectError: true,
		},
		{
			name: "invalid parameters format",
			params: []any{
				"invalid",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := customRPC.GetBalance(ctx, tt.params)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}

				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)

				return
			}

			// Check result type
			response, ok := result.(GetBalanceResponse)
			if !ok {
				t.Errorf("Expected GetBalanceResponse, got %T", result)

				return
			}

			if response.User != tt.expectedUser {
				t.Errorf("Expected user %s, got %s", tt.expectedUser, response.User)
			}

			if response.Token != tt.expectedToken {
				t.Errorf("Expected token %s, got %s", tt.expectedToken, response.Token)
			}

			if response.Balance != tt.expectedBalance {
				t.Errorf("Expected balance %s, got %s", tt.expectedBalance, response.Balance)
			}
		})
	}
}

// Integration test: start RPC server, send transaction, get transaction by hash
func TestDefaultRPC_Integration_SendAndGetTransaction(t *testing.T) {
	localDB := openTxPoolDB(t)

	txPool := txpool.NewTxPool[application.Transaction[application.Receipt], application.Receipt](
		localDB,
	)

	rpcServer := rpc.NewStandardRPCServer(nil)
	rpc.AddStandardMethods(rpcServer, nil, txPool, application.Block{})

	baseURL := startRPCServer(t, rpcServer)

	txHash := "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"

	// Send transaction via JSON-RPC (include hash)
	jsonReq := `{"jsonrpc":"2.0","method":"sendTransaction","params":[{"sender":"alice","token":"USDT","amount":"1234","hash":"` + txHash + `"}],"id":1}`
	resp, err := sendJSONRPCRequest(baseURL+"/rpc", jsonReq)
	require.NoError(t, err)
	require.Contains(t, resp, "result")

	jsonReqGet := `{"jsonrpc":"2.0","method":"getTransactionByHash","params":["` + txHash + `"],"id":2}`
	respGet, err := sendJSONRPCRequest(baseURL+"/rpc", jsonReqGet)
	require.NoError(t, err)
	require.Contains(t, respGet, "result")

	require.Contains(t, respGet, "alice")
	require.Contains(t, respGet, "USDT")
	require.Contains(t, respGet, "1234")
}

func TestCustomRPC_GetBalance_NilDatabase(t *testing.T) {
	// Test with nil database
	rpcServer := rpc.NewStandardRPCServer(nil)
	customRPC := NewCustomRPC(rpcServer, nil)

	params := []any{
		map[string]any{
			"user":  "alice",
			"token": "USDT",
		},
	}

	_, err := customRPC.GetBalance(context.Background(), params)
	if err == nil || !strings.Contains(err.Error(), application.ErrDatabaseNotAvailable.Error()) {
		t.Errorf(
			"Expected error containing %q, got %v",
			application.ErrDatabaseNotAvailable.Error(),
			err,
		)
	}
}

func TestDefaultRPC_MethodRegistration(t *testing.T) {
	// Create local DB for txpool
	localDB := openTxPoolDB(t)

	// Create txpool
	txPool := txpool.NewTxPool[application.Transaction[application.Receipt], application.Receipt](
		localDB,
	)

	// Create RPC server and add standard methods
	rpcServer := rpc.NewStandardRPCServer(nil)

	// Test that AddStandardMethods doesn't panic (even with minimal setup)
	require.NotPanics(t, func() {
		rpc.AddStandardMethods(rpcServer, nil, txPool, application.Block{})
	})
}

// Helper: send JSON-RPC request to local server
func sendJSONRPCRequest(rpcAddress string, jsonReq string) (string, error) {
	req, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		rpcAddress,
		bytes.NewBufferString(jsonReq),
	)
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

func TestDefaultRPC_Integration_GetAppBlock(t *testing.T) {
	ctx := context.Background()
	appchainDB := openBlocksDB(t)
	localDB := openTxPoolDB(t)

	// Create txpool
	txPool := txpool.NewTxPool[application.Transaction[application.Receipt], application.Receipt](
		localDB,
	)

	var root [32]byte
	copy(root[:], []byte("example-root-hash-0000000000000000"))

	block := &application.Block{
		BlockNum: 42,
		Root:     root,
		Txs:      nil,
	}

	storeBlock(ctx, t, appchainDB, block)

	rpcServer := rpc.NewStandardRPCServer(nil)
	rpc.AddStandardMethods(rpcServer, appchainDB, txPool, &application.Block{})

	baseURL := startRPCServer(t, rpcServer)

	jsonReq := fmt.Sprintf(
		`{"jsonrpc":"2.0","method":"getAppBlock","params":[%d],"id":1}`,
		block.BlockNum,
	)

	respBody, err := sendJSONRPCRequest(baseURL+"/rpc", jsonReq)
	require.NoError(t, err)

	var rpcResp rpc.JSONRPCResponse
	require.NoError(t, json.Unmarshal([]byte(respBody), &rpcResp))
	require.Nil(t, rpcResp.Error)

	payload, err := json.Marshal(rpcResp.Result)
	require.NoError(t, err)

	var fv appblock.FieldsValues
	require.NoError(t, json.Unmarshal(payload, &fv))

	require.Len(t, fv.Fields, 3)
	require.Len(t, fv.Values, 3)

	fieldValues := make(map[string]string, len(fv.Fields))
	for i := range fv.Fields {
		fieldValues[fv.Fields[i]] = fv.Values[i]
	}

	require.Equal(t, "42", fieldValues["number"])
	require.Equal(t, fmt.Sprintf("%v", block.Root), fieldValues["root"])
	require.Equal(t, "[]", fieldValues["txs"])
}

func TestDefaultRPC_Integration_GetTransactionsByBlock(t *testing.T) {
	ctx := context.Background()
	appchainDB := openBlocksDB(t)
	localDB := openTxPoolDB(t)

	// Create txpool
	txPool := txpool.NewTxPool[application.TestTransaction[application.TestReceipt]](
		localDB,
	)

	var root [32]byte
	copy(root[:], []byte("example-root-hash-0000000000000000"))

	testTxs := []application.TestTransaction[application.TestReceipt]{
		{From: "0x1111", To: "0x2222", Value: 10},
		{From: "0x3333", To: "0x4444", Value: 20},
	}

	block := &application.Block{
		BlockNum: 42,
		Root:     root,
		Txs:      testTxs,
	}

	storeBlock(ctx, t, appchainDB, block)

	rpcServer := rpc.NewStandardRPCServer(nil)
	rpc.AddStandardMethods(rpcServer, appchainDB, txPool, &application.Block{})

	baseURL := startRPCServer(t, rpcServer)

	jsonReq := fmt.Sprintf(
		`{"jsonrpc":"2.0","method":"getTransactionsByBlockNumber","params":[%d],"id":1}`,
		block.BlockNum,
	)

	respBody, err := sendJSONRPCRequest(baseURL+"/rpc", jsonReq)
	require.NoError(t, err)

	var rpcResp rpc.JSONRPCResponse
	require.NoError(t, json.Unmarshal([]byte(respBody), &rpcResp))
	require.Nil(t, rpcResp.Error)

	resultPayload, err := json.Marshal(rpcResp.Result)
	require.NoError(t, err)

	var got []application.TestTransaction[application.TestReceipt]
	require.NoError(t, json.Unmarshal(resultPayload, &got))
	require.Equal(t, block.Txs, got)

	for i := range block.Txs {
		require.Equal(t, block.Txs[i].From, got[i].From)
		require.Equal(t, block.Txs[i].To, got[i].To)
		require.Equal(t, block.Txs[i].Value, got[i].Value)
	}
}
