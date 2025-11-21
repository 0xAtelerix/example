package wasmstrategy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ledgerwatch/erigon-lib/kv"
	"github.com/rs/zerolog"

	"github.com/0xAtelerix/example/application"
	"github.com/0xAtelerix/sdk/gosdk"
	"github.com/0xAtelerix/sdk/gosdk/apptypes"
)

// ManagerConfig wires the WASM runtime into the Pelagos example node.
type ManagerConfig struct {
	Logger         *zerolog.Logger
	DB             kv.RwDB
	Multichain     *gosdk.MultichainStateAccess
	StrategyDir    string
	ReloadInterval time.Duration
	AddressBook    map[string]AddressID
	ChainID        apptypes.ChainType
	Limits         StrategyLimits
	MaxParallel    int
}

type strategyHandle struct {
	module *StrategyModule
	hash   string
}

// Manager owns the wasm modules and implements gosdk.BlockObserver.
type Manager struct {
	logger      *zerolog.Logger
	multichain  *gosdk.MultichainStateAccess
	addressBook map[string]AddressID
	chainID     apptypes.ChainType

	strategyDir    string
	reloadInterval time.Duration
	limits         StrategyLimits
	maxParallel    int

	mu         sync.RWMutex
	modules    map[string]*strategyHandle
	lastReload time.Time
	db         kv.RwDB
}

// NewManager loads all WASM artifacts available in StrategyDir and keeps them in sync.
func NewManager(ctx context.Context, cfg ManagerConfig) (*Manager, error) {
	if cfg.Multichain == nil {
		return nil, fmt.Errorf("multichain state access is required")
	}

	if cfg.DB == nil {
		return nil, fmt.Errorf("database handle is required")
	}

	if cfg.StrategyDir == "" {
		return nil, fmt.Errorf("strategy directory must be set")
	}

	if _, err := os.Stat(cfg.StrategyDir); err != nil {
		return nil, fmt.Errorf("strategy dir %q: %w", cfg.StrategyDir, err)
	}

	logger := cfg.Logger
	if logger == nil {
		logger = zerolog.Ctx(ctx)
	}

	manager := &Manager{
		logger:         logger,
		multichain:     cfg.Multichain,
		addressBook:    normalizeAddressBook(cfg.AddressBook),
		chainID:        cfg.ChainID,
		strategyDir:    cfg.StrategyDir,
		reloadInterval: cfg.ReloadInterval,
		limits:         cfg.Limits,
		maxParallel:    cfg.MaxParallel,
		modules:        make(map[string]*strategyHandle),
		db:             cfg.DB,
	}

	if manager.maxParallel <= 0 {
		manager.maxParallel = 4
	}

	if manager.reloadInterval <= 0 {
		manager.reloadInterval = 5 * time.Second
	}

	if manager.limits.GasLimit == 0 {
		manager.limits.GasLimit = defaultGasLimit
	}

	if err := manager.reload(ctx); err != nil {
		return nil, err
	}

	return manager, nil
}

// Close frees all wasm runtimes.
func (m *Manager) Close(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, handle := range m.modules {
		if handle.module != nil {
			_ = handle.module.Close(ctx)
		}

		delete(m.modules, id)
	}
}

// OnBlock wires events and context into all configured WASM strategies.
func (m *Manager) OnBlock(ctx context.Context, meta gosdk.BlockObserverContext[application.Transaction[application.Receipt], application.Receipt]) error {
	if err := m.maybeReload(ctx); err != nil {
		m.logger.Error().Err(err).Msg("failed to reload strategies")
	}

	modules := m.snapshotModules()
	if len(modules) == 0 {
		return nil
	}

	blockCtx := BlockContext{
		BlockNumber: meta.BlockNumber,
		ChainID:     m.chainID,
		Events:      m.collectEvents(ctx, meta.Batch.ExternalBlocks),
	}

	sem := make(chan struct{}, m.maxParallel)
	var wg sync.WaitGroup

	for _, handle := range modules {
		wg.Add(1)
		go func(h *strategyHandle) {
			defer wg.Done()

			sem <- struct{}{}
			defer func() { <-sem }()

			if err := h.module.OnBlock(ctx, blockCtx); err != nil {
				m.logger.Error().Err(err).Str("strategy", h.module.id).Msg("strategy execution failed")
			}
		}(handle)
	}

	wg.Wait()

	return nil
}

func (m *Manager) maybeReload(ctx context.Context) error {
	m.mu.RLock()
	last := m.lastReload
	m.mu.RUnlock()

	if time.Since(last) < m.reloadInterval {
		return nil
	}

	return m.reload(ctx)
}

func (m *Manager) reload(ctx context.Context) error {
	entries, err := os.ReadDir(m.strategyDir)
	if err != nil {
		return fmt.Errorf("scan strategies dir: %w", err)
	}

	newHandles := make(map[string]*strategyHandle, len(entries))
	seen := make(map[string]struct{}, len(entries))

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".wasm" {
			continue
		}

		id := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		path := filepath.Join(m.strategyDir, entry.Name())

		bytes, readErr := os.ReadFile(path)
		if readErr != nil {
			m.logger.Error().Err(readErr).Str("strategy", id).Msg("failed to read strategy file")
			continue
		}

		hash := sha256.Sum256(bytes)
		hashHex := hex.EncodeToString(hash[:])
		seen[id] = struct{}{}

		if existing, ok := m.modules[id]; ok && existing.hash == hashHex {
			newHandles[id] = existing
			continue
		}

		module, instErr := NewStrategyModule(ctx, ModuleConfig{
			ID:     id,
			Wasm:   bytes,
			DB:     m.db,
			Logger: m.logger,
			Limits: m.limits,
		})
		if instErr != nil {
			m.logger.Error().Err(instErr).Str("strategy", id).Msg("failed to instantiate strategy")
			continue
		}

		newHandles[id] = &strategyHandle{
			module: module,
			hash:   hashHex,
		}

		m.logger.Info().Str("strategy", id).Msg("loaded strategy module")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for id, handle := range m.modules {
		if _, ok := seen[id]; ok && newHandles[id] == handle {
			continue
		}

		if handle.module != nil {
			_ = handle.module.Close(ctx)
		}
	}

	m.modules = newHandles
	m.lastReload = time.Now()

	return nil
}

func (m *Manager) snapshotModules() []*strategyHandle {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]*strategyHandle, 0, len(m.modules))
	for _, handle := range m.modules {
		out = append(out, handle)
	}

	return out
}

func (m *Manager) collectEvents(ctx context.Context, blocks []*apptypes.ExternalBlock) []StrategyEvent {
	if len(blocks) == 0 || len(m.addressBook) == 0 {
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

func normalizeAddressBook(in map[string]AddressID) map[string]AddressID {
	if len(in) == 0 {
		return in
	}

	out := make(map[string]AddressID, len(in))
	for k, v := range in {
		out[normalizeAddress(k)] = v
	}

	return out
}
