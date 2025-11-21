package wasmstrategy

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/binary"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ledgerwatch/erigon-lib/kv"
	"github.com/ledgerwatch/erigon-lib/kv/mdbx"
	mdbxlog "github.com/ledgerwatch/log/v3"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"github.com/tetratelabs/wazero"

	"github.com/0xAtelerix/example/application"
)

//go:embed testdata/uniswap_strategy.wasm
var testStrategyWasm []byte

//go:embed testdata/forbidden_import.wasm
var testForbiddenImport []byte

//go:embed testdata/memory_import.wasm
var testMemoryImport []byte

func newTestDB(t *testing.T) kv.RwDB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "strategy.mdbx")

	db, err := mdbx.NewMDBX(mdbxlog.New()).
		Path(dbPath).
		WithTableCfg(func(_ kv.TableCfg) kv.TableCfg {
			return kv.TableCfg{
				application.StrategyStateBucket: {},
			}
		}).Open()
	require.NoError(t, err)

	t.Cleanup(func() {
		db.Close()
	})

	return db
}

func readSlot(t *testing.T, ctx context.Context, db kv.RwDB, slot int32) uint64 {
	t.Helper()

	key := make([]byte, 4)
	binary.BigEndian.PutUint32(key, uint32(slot))

	var out uint64

	require.NoError(t, db.View(ctx, func(tx kv.Tx) error {
		data, err := tx.GetOne(application.StrategyStateBucket, key)
		if err != nil {
			return err
		}

		if len(data) == 8 {
			out = binary.BigEndian.Uint64(data)
		}

		return nil
	}))

	return out
}

func writeSlot(t *testing.T, ctx context.Context, db kv.RwDB, slot int32, value uint64) {
	t.Helper()

	key := make([]byte, 4)
	binary.BigEndian.PutUint32(key, uint32(slot))

	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], value)

	require.NoError(t, db.Update(ctx, func(tx kv.RwTx) error {
		return tx.Put(application.StrategyStateBucket, key, buf[:])
	}))
}

func newStrategyModule(t *testing.T, ctx context.Context, db kv.RwDB, sink *bytes.Buffer) *StrategyModule {
	t.Helper()

	logger := zerolog.New(sink).Level(zerolog.DebugLevel)

	module, err := NewStrategyModule(ctx, ModuleConfig{
		ID:     "test-strategy",
		Wasm:   testStrategyWasm,
		DB:     db,
		Logger: &logger,
		Limits: StrategyLimits{
			GasLimit: 5000,
			Timeout:  time.Second,
		},
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, module.Close(ctx))
	})

	return module
}

func TestStrategyModuleWritesSlotOnEvent(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	var logs bytes.Buffer

	module := newStrategyModule(t, ctx, db, &logs)

	blockNumber := uint64(42_000)
	err := module.OnBlock(ctx, BlockContext{
		BlockNumber: blockNumber,
		ChainID:     42,
		Events: []StrategyEvent{
			{Kind: EventKindERC20Transfer, Target: AddressIDUniswapV2Pair},
		},
	})
	require.NoError(t, err)

	value := readSlot(t, ctx, db, SlotLastUniTransferBlock)
	require.Equal(t, blockNumber, value, "strategy should persist last observed block number")
	require.Contains(t, logs.String(), "Uniswap transfer at block", "expected info log from WASM strategy")
}

func TestStrategyModuleLogsStaleTransferWarning(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	var logs bytes.Buffer

	module := newStrategyModule(t, ctx, db, &logs)

	lastSeen := uint64(1)
	writeSlot(t, ctx, db, SlotLastUniTransferBlock, lastSeen)

	blockNumber := lastSeen + 7000
	err := module.OnBlock(ctx, BlockContext{
		BlockNumber: blockNumber,
		ChainID:     42,
	})
	require.NoError(t, err)

	value := readSlot(t, ctx, db, SlotLastUniTransferBlock)
	require.Equal(t, lastSeen, value, "slot should remain unchanged when no transfer is seen")

	expected := fmt.Sprintf("No Uniswap transfers since block %d", lastSeen)
	require.Truef(t, strings.Contains(logs.String(), expected), "expected stale warning log containing %q", expected)
}

func TestStrategyModuleGasLimitExceeded(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	var logs bytes.Buffer

	module := newStrategyModule(t, ctx, db, &logs)
	module.limits = StrategyLimits{GasLimit: 1, Timeout: time.Second}

	err := module.OnBlock(ctx, BlockContext{
		BlockNumber: 1,
		ChainID:     42,
	})
	require.ErrorIs(t, err, errGasLimitExceeded)
}

func TestValidateModuleRejectsUnknownImports(t *testing.T) {
	ctx := context.Background()
	rt := wazero.NewRuntime(ctx)
	t.Cleanup(func() {
		_ = rt.Close(ctx)
	})

	module, err := rt.CompileModule(ctx, testForbiddenImport)
	require.NoError(t, err)

	err = validateModule(module)
	require.EqualError(t, err, "import env.evil is not allowed")
}

func TestValidateModuleRejectsMemoryImports(t *testing.T) {
	ctx := context.Background()
	rt := wazero.NewRuntime(ctx)
	t.Cleanup(func() {
		_ = rt.Close(ctx)
	})

	module, err := rt.CompileModule(ctx, testMemoryImport)
	require.NoError(t, err)

	err = validateModule(module)
	require.EqualError(t, err, "imported memories are not supported")
}
