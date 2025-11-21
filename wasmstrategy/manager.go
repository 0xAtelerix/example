package wasmstrategy

import (
	"context"
	"fmt"
	"os"

	"github.com/ledgerwatch/erigon-lib/kv"
	"github.com/rs/zerolog"

	"github.com/0xAtelerix/example/application"
	"github.com/0xAtelerix/sdk/gosdk"
	"github.com/0xAtelerix/sdk/gosdk/apptypes"
)

// ManagerConfig wires the WASM runtime into the Pelagos example node.
type ManagerConfig struct {
	Logger      *zerolog.Logger
	DB          kv.RwDB
	Multichain  *gosdk.MultichainStateAccess
	WasmPath    string
	AddressBook map[string]AddressID
	ChainID     apptypes.ChainType
}

// Manager owns the wasm modules and implements gosdk.BlockObserver.
type Manager struct {
	logger      *zerolog.Logger
	modules     []*StrategyModule
	multichain  *gosdk.MultichainStateAccess
	addressBook map[string]AddressID
	chainID     apptypes.ChainType
}

// NewManager loads the AssemblyScript-built WASM artifact from disk.
func NewManager(ctx context.Context, cfg ManagerConfig) (*Manager, error) {
	if cfg.Multichain == nil {
		return nil, fmt.Errorf("multichain state access is required")
	}

	if cfg.DB == nil {
		return nil, fmt.Errorf("database handle is required")
	}

	logger := cfg.Logger
	if logger == nil {
		logger = zerolog.Ctx(ctx)
	}

	wasmBytes, err := os.ReadFile(cfg.WasmPath)
	if err != nil {
		return nil, fmt.Errorf("read wasm %q: %w", cfg.WasmPath, err)
	}

	module, err := NewStrategyModule(ctx, ModuleConfig{
		ID:     "strategy-uniswap-monitor",
		Wasm:   wasmBytes,
		DB:     cfg.DB,
		Logger: logger,
	})
	if err != nil {
		return nil, err
	}

	book := cfg.AddressBook
	if book == nil {
		book = make(map[string]AddressID)
	}

	return &Manager{
		logger:      logger,
		modules:     []*StrategyModule{module},
		multichain:  cfg.Multichain,
		addressBook: book,
		chainID:     cfg.ChainID,
	}, nil
}

// Close frees all wasm runtimes.
func (m *Manager) Close(ctx context.Context) {
	for _, module := range m.modules {
		_ = module.Close(ctx)
	}
}

// OnBlock wires events and context into all configured WASM strategies.
func (m *Manager) OnBlock(ctx context.Context, meta gosdk.BlockObserverContext[application.Transaction[application.Receipt], application.Receipt]) error {
	if len(m.modules) == 0 {
		return nil
	}

	blockCtx := BlockContext{
		BlockNumber: meta.BlockNumber,
		ChainID:     m.chainID,
		Events:      m.collectEvents(ctx, meta.Batch.ExternalBlocks),
	}

	for _, module := range m.modules {
		if err := module.OnBlock(ctx, blockCtx); err != nil {
			m.logger.Error().Err(err).Msg("strategy execution failed")
		}
	}

	return nil
}

func (m *Manager) collectEvents(ctx context.Context, blocks []*apptypes.ExternalBlock) []StrategyEvent {
	if len(blocks) == 0 {
		return nil
	}

	if len(m.addressBook) == 0 {
		return nil
	}

	var events []StrategyEvent

	for _, blk := range blocks {
		if blk == nil {
			continue
		}

		chainID := apptypes.ChainType(blk.ChainID)
		if !gosdk.IsEvmChain(chainID) {
			continue
		}

		receipts, err := m.multichain.EthReceipts(ctx, *blk)
		if err != nil {
			m.logger.Debug().Err(err).Uint64("chain", blk.ChainID).Uint64("block", blk.BlockNumber).Msg("missing receipts")
			continue
		}

		for _, receipt := range receipts {
			for _, vlog := range receipt.Logs {
				if len(vlog.Topics) == 0 || vlog.Topics[0].Hex() != erc20TransferTopic {
					continue
				}

				addrID, ok := m.addressBook[normalizeAddress(vlog.Address.Hex())]
				if !ok {
					continue
				}

				events = append(events, StrategyEvent{Kind: EventKindERC20Transfer, Target: addrID})
			}
		}
	}

	return events
}
