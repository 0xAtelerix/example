package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0xAtelerix/sdk/gosdk"
	"github.com/holiman/uint256"
	"github.com/ledgerwatch/erigon-lib/kv"
	"github.com/ledgerwatch/erigon-lib/kv/mdbx"
	mdbxlog "github.com/ledgerwatch/log/v3"

	"github.com/0xAtelerix/example/application"
)

const accountsBucketName = "appaccounts"

// ---- helpers ---------------------------------------------------------------

func getFreePortTB(tb testing.TB) int {
	tb.Helper()

	l, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		tb.Fatalf("pick port: %v", err)
	}

	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()

	return port
}

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

func u256Bytes(v uint64) []byte { return uint256.NewInt(v).Bytes() }

// ---- benchmark -------------------------------------------------------------

func BenchmarkEndToEnd_RPC(b *testing.B) {
	// --- ports & dirs
	port := getFreePortTB(b)
	tmp := b.TempDir()
	dbPath := filepath.Join(tmp, "appchain.mdbx")
	localDB := filepath.Join(tmp, "local.mdbx")
	streamDir := filepath.Join(tmp, "stream")
	txDir := filepath.Join(tmp, "tx")

	// --- generate workload (deterministic)
	r := rand.New(rand.NewSource(1))

	accountCount := b.N / 1000
	if accountCount < 100 {
		accountCount = 100
	}

	tokenCount := 50
	maxValue := b.N * 100

	accounts := make([]string, accountCount)
	for i := range accountCount {
		accounts[i] = fmt.Sprintf("A%08d", i)
	}

	tokens := make([]string, tokenCount)
	for i := range tokenCount {
		tokens[i] = fmt.Sprintf("T%08d", i)
	}

	type stKey struct{ s, t string }

	required := make(map[stKey]struct{}, b.N)

	// Prebuild tx payloads so the hot path avoids RNG/alloc where possible.
	txs := make([]application.Transaction[application.Receipt], b.N)
	for i := range b.N {
		si := r.Intn(accountCount)
		ri := r.Intn(accountCount)
		ti := r.Intn(tokenCount)
		val := uint64(r.Intn(maxValue/100) + 1)

		txs[i] = application.Transaction[application.Receipt]{
			Sender:   accounts[si],
			Receiver: accounts[ri],
			Token:    tokens[ti],
			Value:    val,
			TxHash:   fmt.Sprintf("%016x", i), // unique-ish
		}
		required[stKey{accounts[si], tokens[ti]}] = struct{}{}
	}

	// --- seed DB with balances so the state transition accepts debits
	{
		db, err := mdbx.NewMDBX(mdbxlog.New()).
			Path(dbPath).
			WithTableCfg(func(_ kv.TableCfg) kv.TableCfg {
				return gosdk.MergeTables(gosdk.DefaultTables(), application.Tables())
			}).
			Open()
		if err != nil {
			b.Fatalf("open mdbx: %v", err)
		}

		seedTx, err := db.BeginRw(context.Background())
		if err != nil {
			db.Close()
			b.Fatalf("begin seed tx: %v", err)
		}

		// generous balance so all txs should pass at the state layer
		for st := range required {
			if err := seedTx.Put(accountsBucketName, application.AccountKey(st.s, st.t), u256Bytes(uint64(maxValue))); err != nil {
				seedTx.Rollback()
				db.Close()
				b.Fatalf("seed Put: %v", err)
			}
		}

		if err := seedTx.Commit(); err != nil {
			db.Close()
			b.Fatalf("seed commit: %v", err)
		}

		db.Close()
	}

	// --- boot the app (like main_test)
	oldArgs := os.Args

	defer func() { os.Args = oldArgs }()

	os.Args = []string{
		"bench-app",
		"-rpc-port", fmt.Sprintf(":%d", port),
		"-emitter-port", ":0", // 0 → let OS choose, we don’t care in the test
		"-db-path", dbPath,
		"-local-db-path", localDB,
		"-stream-dir", streamDir,
		"-tx-dir", txDir,
	}

	ready := make(chan int, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go Run(ctx, RuntimeArgs{
		RPCPort:        ":0",
		EmitterPort:    ":0",
		AppchainDBPath: dbPath,
		LocalDBPath:    localDB,
		EventStreamDir: streamDir,
		TxStreamDir:    txDir,
	}, ready)

	rpcPort := <-ready
	if rpcPort == 0 {
		cancel()
		b.Fatal("runner didn't report a port")
	}

	rpcURL := fmt.Sprintf("http://127.0.0.1:%d/rpc", rpcPort)

	readyCtx, readyCancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := waitUntil(readyCtx, func() bool {
		req, err := http.NewRequestWithContext(readyCtx, http.MethodGet, rpcURL, nil)
		if err != nil {
			return false
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return false
		}
		_ = resp.Body.Close()

		return true
	}); err != nil {
		readyCancel()
		b.Fatalf("JSON-RPC service never became ready: %v", err)
	}

	readyCancel()

	// --- slam POST /rpc with b.N txs
	var (
		next uint64
		errs int64
	)

	tr := &http.Transport{
		// keep-alive pools
		MaxIdleConns:        4096,
		MaxIdleConnsPerHost: 4096,
		MaxConnsPerHost:     512, // cap concurrent dials
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  true,
		ForceAttemptHTTP2:   false, // plain HTTP/1.1
	}
	client := &http.Client{Transport: tr, Timeout: 10 * time.Second}

	b.Cleanup(func() { tr.CloseIdleConnections() })

	const maxInFlight = 1024

	sem := make(chan struct{}, maxInFlight)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			i := int(atomic.AddUint64(&next, 1)) - 1
			if i >= len(txs) {
				return
			}

			sem <- struct{}{}

			func() {
				defer func() { <-sem }()

				var buf bytes.Buffer
				if err := json.NewEncoder(&buf).Encode(txs[i]); err != nil {
					atomic.AddInt64(&errs, 1)

					return
				}

				req, err := http.NewRequestWithContext(
					ctx,
					http.MethodPost,
					rpcURL,
					bytes.NewReader(buf.Bytes()),
				)
				if err != nil {
					atomic.AddInt64(&errs, 1)

					return
				}

				req.Header.Set("Content-Type", "application/json")

				resp, err := client.Do(req)
				if err != nil {
					atomic.AddInt64(&errs, 1)

					return
				}

				_, err = io.Copy(io.Discard, resp.Body)
				if err != nil {
					b.Log("discard response body errored", err)
				}

				err = resp.Body.Close()
				if err != nil {
					b.Log("close response body errored", err)
				}

				if resp.StatusCode < 200 || resp.StatusCode >= 300 {
					atomic.AddInt64(&errs, 1)
				}
			}()
		}
	})

	b.StopTimer()

	// --- teardown
	cancel()
	time.Sleep(200 * time.Millisecond)

	// --- metrics
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "req/s")
	b.ReportMetric(float64(errs), "errors")
	b.ReportMetric(float64(accountCount), "accounts")
	b.ReportMetric(float64(tokenCount), "tokens")
	b.ReportMetric(float64(b.N), "transactions")
}
