package api

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

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
	localDB, err := mdbx.NewMDBX(mdbxlog.New()).
		Path(t.TempDir()).
		WithTableCfg(func(_ kv.TableCfg) kv.TableCfg {
			return txpool.Tables()
		}).
		Open()
	require.NoError(t, err)

	defer localDB.Close()

	txPool := txpool.NewTxPool[application.Transaction[application.Receipt], application.Receipt](
		localDB,
	)

	// Create appchain DB for AddStandardMethods
	appchainDB, err := mdbx.NewMDBX(mdbxlog.New()).
		Path(t.TempDir()).
		Open()
	require.NoError(t, err)

	defer appchainDB.Close()

	rpcServer := rpc.NewStandardRPCServer(nil)
	rpc.AddStandardMethods[
		application.Transaction[application.Receipt],
		application.Receipt,
		application.Block,
	](rpcServer, appchainDB, txPool, 42)

	rpcAddress := "http://127.0.0.1:18545/rpc"

	errServer := make(chan error, 1)
	wg := sync.WaitGroup{}
	wg.Add(1)

	go func() {
		wg.Done()

		errServer <- rpcServer.StartHTTPServer(t.Context(), ":18545")
	}()

	select {
	case serverErr := <-errServer:
		if serverErr != nil {
			t.Fatalf("Failed to start HTTP server: %v", serverErr)
		}
	default:
		// continue
		wg.Wait()
		time.Sleep(100 * time.Millisecond)
	}

	txHash := "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"

	// Send transaction via JSON-RPC (include hash)
	jsonReq := `{"jsonrpc":"2.0","method":"sendTransaction","params":[{"sender":"alice","token":"USDT","amount":"1234","hash":"` + txHash + `"}],"id":1}`
	resp, err := sendJSONRPCRequest(rpcAddress, jsonReq)
	require.NoError(t, err)
	require.Contains(t, resp, "result")

	jsonReqGet := `{"jsonrpc":"2.0","method":"getTransaction","params":["` + txHash + `"],"id":2}`
	respGet, err := sendJSONRPCRequest(rpcAddress, jsonReqGet)
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
	localDB, err := mdbx.NewMDBX(mdbxlog.New()).
		Path(t.TempDir()).
		WithTableCfg(func(_ kv.TableCfg) kv.TableCfg {
			return txpool.Tables()
		}).
		Open()
	require.NoError(t, err)

	defer localDB.Close()

	// Create txpool
	txPool := txpool.NewTxPool[application.Transaction[application.Receipt], application.Receipt](
		localDB,
	)

	// Create RPC server and add standard methods
	rpcServer := rpc.NewStandardRPCServer(nil)

	// Test that AddStandardMethods doesn't panic (even with minimal setup)
	require.NotPanics(t, func() {
		rpc.AddStandardMethods[
			application.Transaction[application.Receipt],
			application.Receipt,
			application.Block,
		](rpcServer, nil, txPool, 42)
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
