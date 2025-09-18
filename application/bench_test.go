package application

import (
	"context"
	"fmt"
	"math/rand"
	"path/filepath"
	"sort"
	"sync"
	"testing"

	"github.com/0xAtelerix/sdk/gosdk"
	"github.com/holiman/uint256"
	"github.com/ledgerwatch/erigon-lib/kv"
	"github.com/ledgerwatch/erigon-lib/kv/mdbx"
	mdbxlog "github.com/ledgerwatch/log/v3"

	db2 "github.com/0xAtelerix/example/application/db"
	"github.com/0xAtelerix/example/application/transactions"
)

// For brevity:
type Tx = transactions.Transaction[transactions.Receipt]

// utility: stable RNG for reproducible benches
func newRand() *rand.Rand { return rand.New(rand.NewSource(1)) }

// encode uint64 -> uint256 bytes
func u256Bytes(v uint64) []byte { return uint256.NewInt(v).Bytes() }

func BenchmarkProcess_MDBX(b *testing.B) {
	// --- DB setup (not measured) ---
	dir := b.TempDir()
	dbPath := filepath.Join(dir, "bench.mdbx")

	db, err := mdbx.
		NewMDBX(mdbxlog.New()).
		Path(dbPath).
		WithTableCfg(func(_ kv.TableCfg) kv.TableCfg {
			return gosdk.MergeTables(
				gosdk.DefaultTables(),
				db2.Tables(),
			)
		}).
		Open()
	if err != nil {
		b.Fatalf("open mdbx: %v", err)
	}

	b.Cleanup(db.Close)

	ctx := context.Background()

	// --- Data generation (not measured) ---
	r := newRand()

	n := b.N

	// Generate accounts and tokens.
	accountCount := n / 1000
	if accountCount < 10 {
		accountCount = 100
	}

	accounts := make([]string, accountCount)

	for i := range accountCount {
		accounts[i] = fmt.Sprintf("A%08d", i)
	}

	tokenCount := 50

	tokens := make([]string, tokenCount)

	for i := range tokenCount {
		tokens[i] = fmt.Sprintf("T%08d", i)
	}

	// Prepare transactions.
	// Values are in [1, 1_000_000]. Senders/receivers/tokens are sampled from those generated above.
	txs := make([]*Tx, n)

	maxValue := n * 100

	// Track which (sender,token) pairs need initial balances.
	type stKey struct{ s, t string }

	requiredSenderPairs := make(map[stKey]struct{}, n)

	for i := range n {
		si := r.Intn(accountCount)
		ri := r.Intn(accountCount)
		ti := r.Intn(tokenCount)

		val := uint64(r.Intn(maxValue/100) + 1)

		txs[i] = &Tx{
			Sender:   accounts[si],
			Receiver: accounts[ri],
			Token:    tokens[ti],
			Value:    val,
		}

		db2.GetAccountKey(txs[i].Sender, txs[i].Token) // warm-up cache

		requiredSenderPairs[stKey{accounts[si], tokens[ti]}] = struct{}{}
	}

	// Build a sorted slice of (token, account) pairs so we seed in a stable order:
	// first by token, then by account.
	pairs := make([]stKey, 0, len(requiredSenderPairs))
	for st := range requiredSenderPairs {
		pairs = append(pairs, st)
	}

	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].t == pairs[j].t {
			return pairs[i].s < pairs[j].s
		}

		return pairs[i].t < pairs[j].t
	})

	// Seed DB with sender balances for each (sender,token) pair that will be debited.
	// Give each such pair a random balance in [value, 1_000_000], but ensure it's
	// >= maxValue so we don't trip ErrNotEnoughBalance during the run.
	// Seeding receivers isn't required; missing keys imply zero (Process handles it).
	seedTx, err := db.BeginRw(ctx)
	if err != nil {
		b.Fatalf("begin seed tx: %v", err)
	}

	for _, st := range pairs {
		// Give every sender-token a healthy starting balance so all txs succeed.
		if err = seedTx.Append(db2.AccountsBucket, db2.AccountKey(st.s, st.t), u256Bytes(uint64(maxValue))); err != nil {
			seedTx.Rollback()
			b.Fatalf("seed Put: %v", err)
		}
	}

	if err = seedTx.Commit(); err != nil {
		b.Fatalf("seed commit: %v", err)
	}

	seedTx.Rollback()

	var errCount int

	writeTx, err := db.BeginRw(ctx)
	if err != nil {
		b.Fatalf("begin bench tx: %v", err)
	}

	shardedExecution := transactions.NewSharding(db, tokens...)

	b.ResetTimer()

	go func() {
		for i := range n {
			txs[i].ProcessOnSharding(db, shardedExecution)
		}

		shardedExecution.Close()
	}()

	for _, w := range shardedExecution.Pool {
		for resp := range w.ResCh {
			if resp.Err != nil {
				errCount++

				continue
			}

			err = writeTx.Put(db2.AccountsBucket, resp.SenderKey, resp.SenderValue.Bytes())
			if err != nil {
				errCount++
			}

			err = writeTx.Put(db2.AccountsBucket, resp.ReceiverKey, resp.ReceiverValue.Bytes())
			if err != nil {
				errCount++
			}
		}
	}

	if err = writeTx.Commit(); err != nil {
		b.Fatalf("bench commit: %v", err)
	}

	writeTx.Rollback()

	b.StopTimer()

	b.ReportMetric(float64(n)/b.Elapsed().Seconds(), "tx/s")
	b.ReportMetric(float64(errCount), "errors")
	b.ReportMetric(float64(accountCount), "accounts")
	b.ReportMetric(float64(tokenCount), "tokens")
	b.ReportMetric(float64(n), "transactions")
	b.ReportMetric(float64(b.N), "N")
}

// fanInRes merges many <-chan res into a single <-chan res.
// It stops when all inputs are closed. Cancel ctx to stop early.
func fanInRes(ctx context.Context, inputs ...<-chan transactions.Res) <-chan transactions.Res {
	out := make(chan transactions.Res, 1024) // a little buffering to smooth bursts

	var wg sync.WaitGroup
	wg.Add(len(inputs))

	for _, ch := range inputs {
		go func() {
			defer wg.Done()

			for {
				select {
				case v, ok := <-ch:
					if !ok {
						return
					}

					select {
					case out <- v:
					case <-ctx.Done():
						return
					}
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(out)
	}()

	return out
}
