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
	"github.com/0xAtelerix/sdk/gosdk/apptypes"
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

func newBlock() *application.Block { return &application.Block{} }

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

func newTxPool[T apptypes.AppTransaction[R], R apptypes.Receipt](
	t *testing.T,
) *txpool.TxPool[T, R] {
	t.Helper()

	return txpool.NewTxPool[T, R](openTxPoolDB(t))
}

func openBlocksDB(t *testing.T) kv.RwDB {
	t.Helper()

	db, err := mdbx.NewMDBX(mdbxlog.New()).
		Path(t.TempDir()).
		WithTableCfg(func(_ kv.TableCfg) kv.TableCfg {
			return gosdk.DefaultTables()
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

func newStandardRPCClient[T apptypes.AppTransaction[R], R apptypes.Receipt](
	t *testing.T,
	appchainDB kv.RwDB,
	txPool *txpool.TxPool[T, R],
) *rpcTestClient {
	t.Helper()

	server := rpc.NewStandardRPCServer(nil)
	rpc.AddStandardMethods(
		server,
		appchainDB,
		txPool,
		newBlock,
	)

	baseURL := startRPCServer(t, server)

	return newRPCTestClient(t, baseURL)
}

type rpcTestClient struct {
	t          *testing.T
	baseURL    string
	httpClient *http.Client
	nextID     int
}

func newRPCTestClient(t *testing.T, baseURL string) *rpcTestClient {
	t.Helper()

	return &rpcTestClient{
		t:       t,
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 2 * time.Second,
		},
	}
}

func (c *rpcTestClient) Call(method string, params []any) rpc.JSONRPCResponse {
	c.t.Helper()

	c.nextID++

	reqBody := rpc.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
		ID:      c.nextID,
	}

	payload, err := json.Marshal(reqBody)
	require.NoError(c.t, err)

	httpReq, err := http.NewRequestWithContext(
		c.t.Context(),
		http.MethodPost,
		c.baseURL+"/rpc",
		bytes.NewReader(payload),
	)
	require.NoError(c.t, err)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	require.NoError(c.t, err)

	defer func() {
		require.NoError(c.t, resp.Body.Close())
	}()

	body, err := io.ReadAll(resp.Body)
	require.NoError(c.t, err)

	require.Equalf(
		c.t,
		http.StatusOK,
		resp.StatusCode,
		"unexpected HTTP status %s: %s",
		resp.Status,
		string(body),
	)

	var rpcResp rpc.JSONRPCResponse
	require.NoError(c.t, json.Unmarshal(body, &rpcResp))

	return rpcResp
}

func (c *rpcTestClient) MustCall(method string, params []any) rpc.JSONRPCResponse {
	c.t.Helper()

	resp := c.Call(method, params)
	require.Nil(c.t, resp.Error, "json-rpc error: %+v", resp.Error)

	return resp
}

func decodeRPCResult[T any](t *testing.T, resp rpc.JSONRPCResponse, out *T) {
	t.Helper()

	payload := mustMarshalJSON(t, resp.Result)
	require.NoError(t, json.Unmarshal(payload, out))
}

func mustMarshalJSON(t *testing.T, v any) []byte {
	t.Helper()

	data, err := json.Marshal(v)
	require.NoError(t, err)

	return data
}

func makeTestBlock(
	blockNum uint64,
	txs []application.TestTransaction[application.TestReceipt],
) *application.Block {
	var root [32]byte
	copy(root[:], []byte("example-root-hash-0000000000000000"))

	return &application.Block{
		BlockNum: blockNum,
		Root:     root,
		Txs:      txs,
	}
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
	txPool := newTxPool[application.Transaction[application.Receipt], application.Receipt](t)

	client := newStandardRPCClient(t, nil, txPool)

	txHash := "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"

	tx := application.Transaction[application.Receipt]{
		Sender:   "alice",
		Receiver: "bob",
		Token:    "USDT",
		Value:    1234,
		TxHash:   txHash,
	}

	sendResp := client.MustCall("sendTransaction", []any{tx})

	hashResult, ok := sendResp.Result.(string)
	require.True(t, ok)
	require.True(t, strings.EqualFold(hashResult, txHash))

	getResp := client.MustCall("getTransactionByHash", []any{txHash})

	var fetched application.Transaction[application.Receipt]
	decodeRPCResult(t, getResp, &fetched)

	require.Equal(t, tx.Sender, fetched.Sender)
	require.Equal(t, tx.Receiver, fetched.Receiver)
	require.Equal(t, tx.Token, fetched.Token)
	require.Equal(t, tx.Value, fetched.Value)
	require.True(t, strings.EqualFold(tx.TxHash, fetched.TxHash))
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

func TestDefaultRPC_Integration_GetAppBlock(t *testing.T) {
	ctx := context.Background()
	appchainDB := openBlocksDB(t)
	txPool := newTxPool[application.Transaction[application.Receipt], application.Receipt](t)

	block := makeTestBlock(42, nil)

	storeBlock(ctx, t, appchainDB, block)

	client := newStandardRPCClient(t, appchainDB, txPool)

	resp := client.MustCall("getAppBlock", []any{block.BlockNum})

	var fv appblock.FieldsValues
	decodeRPCResult(t, resp, &fv)

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
	txPool := newTxPool[application.TestTransaction[application.TestReceipt], application.TestReceipt](
		t,
	)

	testTxs := []application.TestTransaction[application.TestReceipt]{
		{From: "0x1111", To: "0x2222", Value: 10},
		{From: "0x3333", To: "0x4444", Value: 20},
	}

	block := makeTestBlock(42, testTxs)

	storeBlock(ctx, t, appchainDB, block)

	client := newStandardRPCClient(t, appchainDB, txPool)

	resp := client.MustCall("getTransactionsByBlockNumber", []any{block.BlockNum})

	var got []application.TestTransaction[application.TestReceipt]
	decodeRPCResult(t, resp, &got)
	require.Equal(t, block.Txs, got)

	for i := range block.Txs {
		require.Equal(t, block.Txs[i].From, got[i].From)
		require.Equal(t, block.Txs[i].To, got[i].To)
		require.Equal(t, block.Txs[i].Value, got[i].Value)
	}
}
