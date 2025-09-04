package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0xAtelerix/sdk/gosdk"
	fiberclient "github.com/gofiber/fiber/v3/client"
	"github.com/holiman/uint256"
	"github.com/ledgerwatch/erigon-lib/kv"
	"github.com/ledgerwatch/erigon-lib/kv/mdbx"
	mdbxlog "github.com/ledgerwatch/log/v3"

	"github.com/0xAtelerix/example/application"
)

const accountsBucketName = "appaccounts"

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
	// --- dirs
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
		"-rpc-port", ":0",
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
		UseFiber:       true,
	}, ready)

	rpcPort := <-ready
	if rpcPort == 0 {
		cancel()
		b.Fatal("runner didn't report a port")
	}

	rpcURL := fmt.Sprintf("http://127.0.0.1:%d/rpc", rpcPort)

	// Fiber client (fasthttp-based), reuse across workers.
	cli := fiberclient.New()
	cli.SetJSONMarshal(json.Marshal)
	cli.SetJSONUnmarshal(json.Unmarshal)

	// --- readiness: just attempt a GET; any 2xx/4xx means listener is up.
	if err := waitUntil(ctx, func() bool {
		req := fiberclient.AcquireRequest().SetClient(cli)
		defer fiberclient.ReleaseRequest(req)

		resp, err := req.Get(rpcURL)
		if err != nil {
			return false
		}
		// don't care about status/body here—only that the listener accepts connections
		_ = resp // avoid unused

		return true
	}); err != nil {
		b.Fatalf("service not ready: %v", err)
	}

	// --- slam POST /rpc with b.N txs using Fiber client
	var (
		next uint64
		errs int64
	)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			i := int(atomic.AddUint64(&next, 1)) - 1
			if i >= len(txs) {
				return
			}

			func() {
				// Build & send request with Fiber client
				req := fiberclient.AcquireRequest().SetClient(cli)
				defer fiberclient.ReleaseRequest(req)

				req.SetJSON(txs[i])

				resp, err := req.Post(rpcURL)
				if err != nil {
					b.Log("err Post", err)
					atomic.AddInt64(&errs, 1)

					return
				}

				// If your handler returns a body you want to discard, you can read it:
				// _ = resp.Body() // []byte
				// We only check status here:
				if sc := resp.StatusCode(); sc < 200 || sc >= 300 {
					b.Log("err response:", resp)
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
