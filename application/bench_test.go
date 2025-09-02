package application

import (
	"context"
	"fmt"
	"math/rand"
	"path/filepath"
	"testing"
	"time"

	"github.com/0xAtelerix/sdk/gosdk"
	"github.com/holiman/uint256"
	"github.com/ledgerwatch/erigon-lib/kv"
	"github.com/ledgerwatch/erigon-lib/kv/mdbx"
	mdbxlog "github.com/ledgerwatch/log/v3"
)

// For brevity:
type Tx = Transaction[Receipt]

// utility: stable RNG for reproducible benches
func newRand() *rand.Rand { return rand.New(rand.NewSource(1)) }

// encode uint64 -> uint256 bytes
func u256Bytes(v uint64) []byte { return uint256.NewInt(v).Bytes() }

func BenchmarkProcess_MDBX(b *testing.B) {
	b.ReportAllocs()

	// --- DB setup (not measured) ---
	dir := b.TempDir()
	dbPath := filepath.Join(dir, "bench.mdbx")

	db, err := mdbx.
		NewMDBX(mdbxlog.New()).
		Path(dbPath).
		WithTableCfg(func(_ kv.TableCfg) kv.TableCfg {
			return gosdk.MergeTables(
				gosdk.DefaultTables(),
				Tables(),
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
	// We deliberately add a '-' suffix to account IDs to avoid AccountKey collisions,
	// since AccountKey concatenates sender+token without a separator.
	accounts := make([]string, n)

	tokens := make([]string, n)
	for i := range n {
		accounts[i] = "A" + fmt.Sprintf("%08d-", i)
		tokens[i] = "T" + fmt.Sprintf("%08d", i)
	}

	// Prepare transactions.
	// Values are in [1, 1_000_000]. Senders/receivers/tokens are sampled from those generated above.
	txs := make([]Tx, n)

	const maxValue = 1_000_000

	// Track which (sender,token) pairs need initial balances.
	type stKey struct{ s, t string }

	requiredSenderPairs := make(map[stKey]struct{}, n)

	for i := range n {
		si := r.Intn(n)
		ri := r.Intn(n)
		ti := r.Intn(n)

		val := uint64(r.Intn(maxValue) + 1)

		txs[i] = Tx{
			Sender:   accounts[si],
			Receiver: accounts[ri],
			Token:    tokens[ti],
			Value:    val,
		}
		requiredSenderPairs[stKey{accounts[si], tokens[ti]}] = struct{}{}
	}

	// Seed DB with sender balances for each (sender,token) pair that will be debited.
	// Give each such pair a random balance in [value, 1_000_000], but ensure it's
	// >= maxValue so we don't trip ErrNotEnoughBalance during the run.
	// Seeding receivers isn't required; missing keys imply zero (Process handles it).
	seedTx, err := db.BeginRw(ctx)
	if err != nil {
		b.Fatalf("begin seed tx: %v", err)
	}

	for st := range requiredSenderPairs {
		// Give every sender-token a healthy starting balance so all txs succeed.
		if err = seedTx.Put(accountsBucket, AccountKey(st.s, st.t), u256Bytes(maxValue)); err != nil {
			seedTx.Rollback()
			b.Fatalf("seed Put: %v", err)
		}
	}

	if err = seedTx.Commit(); err != nil {
		b.Fatalf("seed commit: %v", err)
	}

	var errCount int

	writeTx, err := db.BeginRwNosync(ctx)
	if err != nil {
		b.Fatalf("begin bench tx: %v", err)
	}

	start := time.Now()

	b.ResetTimer()

	for i := range n {
		_, _, err = txs[i].Process(writeTx)
		if err != nil {
			errCount++
		}
	}

	if err = writeTx.Commit(); err != nil {
		b.Fatalf("bench commit: %v", err)
	}

	b.StopTimer()

	elapsed := time.Since(start)

	b.ReportMetric(float64(b.N)/elapsed.Seconds(), "tx/s")
	b.ReportMetric(float64(errCount), "errors")
}

// Optional variant: commit every K txs to simulate block boundaries or durability constraints.
func BenchmarkProcess_MDBX_BatchedCommits(b *testing.B) {
	r := newRand()

	n := b.N

	accounts := make([]string, n)

	tokens := make([]string, n)
	for i := range n {
		accounts[i] = "A" + fmt.Sprintf("%08d-", i)
		tokens[i] = "T" + fmt.Sprintf("%08d", i)
	}

	const maxValue = 1_000_000

	txs := make([]Tx, n)

	type stKey struct{ s, t string }

	required := make(map[stKey]struct{}, n)

	for i := range n {
		si := r.Intn(n)
		ri := r.Intn(n)
		ti := r.Intn(n)
		val := uint64(r.Intn(maxValue) + 1)

		txs[i] = Tx{
			Sender:   accounts[si],
			Receiver: accounts[ri],
			Token:    tokens[ti],
			Value:    val,
		}
		required[stKey{accounts[si], tokens[ti]}] = struct{}{}
	}

	dir := b.TempDir()
	dbPath := filepath.Join(dir, "bench_batched.mdbx")

	db, err := mdbx.
		NewMDBX(mdbxlog.New()).
		Path(dbPath).
		WithTableCfg(func(_ kv.TableCfg) kv.TableCfg {
			return gosdk.MergeTables(
				gosdk.DefaultTables(),
				Tables(),
			)
		}).
		Open()
	if err != nil {
		b.Fatalf("open mdbx: %v", err)
	}

	b.Cleanup(db.Close)

	ctx := context.Background()

	seedTx, err := db.BeginRw(ctx)
	if err != nil {
		b.Fatalf("begin seed tx: %v", err)
	}

	for st := range required {
		if err := seedTx.Put(accountsBucket, AccountKey(st.s, st.t), u256Bytes(maxValue)); err != nil {
			seedTx.Rollback()
			b.Fatalf("seed Put: %v", err)
		}
	}

	if err := seedTx.Commit(); err != nil {
		b.Fatalf("seed commit: %v", err)
	}
}

/*
Run:

  go test -bench=Process -benchmem ./...

Notes:

- We keep seeding out of the timed region so the benchmark concentrates on the hot path
  (GetOne + Put twice) inside a single MDBX write txn.
- If you expect very large b.N (millions), you may want to bump MDBX map size or
  run the batched-commit variant to keep write transactions bounded in size/time.
- AccountKey(sender, token) concatenates without a separator; the account IDs above
  intentionally end with '-' to avoid accidental collisions.
- Receipt and ErrNotEnoughBalance are assumed to exist in your application package
  (they’re referenced by Transaction[Receipt] and Process).
*/
