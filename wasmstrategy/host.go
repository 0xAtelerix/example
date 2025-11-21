package wasmstrategy

import (
	"context"
	"encoding/binary"
	"fmt"
	"strings"
	"sync"

	"github.com/ledgerwatch/erigon-lib/kv"
	"github.com/rs/zerolog"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"

	"github.com/0xAtelerix/example/application"
)

// hostEnv backs the env.* host functions consumed by AssemblyScript strategies.
type hostEnv struct {
	logger *zerolog.Logger
	db     kv.RwDB

	mu       sync.RWMutex
	blockCtx BlockContext
}

func newHostEnv(logger *zerolog.Logger, db kv.RwDB) *hostEnv {
	return &hostEnv{logger: logger, db: db}
}

func (h *hostEnv) setContext(ctx BlockContext) {
	h.mu.Lock()
	h.blockCtx = ctx
	h.mu.Unlock()
}

func (h *hostEnv) register(ctx context.Context, runtime wazero.Runtime) error {
	builder := runtime.NewHostModuleBuilder("env")

	builder.NewFunctionBuilder().
		WithGoModuleFunction(api.GoModuleFunc(h.getBlockNumber), []api.ValueType{}, []api.ValueType{api.ValueTypeI64}).
		Export("get_block_number")

	builder.NewFunctionBuilder().
		WithGoModuleFunction(api.GoModuleFunc(h.getChainID), []api.ValueType{}, []api.ValueType{api.ValueTypeI64}).
		Export("get_chain_id")

	builder.NewFunctionBuilder().
		WithGoModuleFunction(api.GoModuleFunc(h.getEventCount), []api.ValueType{}, []api.ValueType{api.ValueTypeI32}).
		Export("get_event_count")

	builder.NewFunctionBuilder().
		WithGoModuleFunction(api.GoModuleFunc(h.getEventKind), []api.ValueType{api.ValueTypeI32}, []api.ValueType{api.ValueTypeI32}).
		Export("get_event_kind")

	builder.NewFunctionBuilder().
		WithGoModuleFunction(api.GoModuleFunc(h.getEventAddressID), []api.ValueType{api.ValueTypeI32}, []api.ValueType{api.ValueTypeI32}).
		Export("get_event_address_id")

	builder.NewFunctionBuilder().
		WithGoModuleFunction(api.GoModuleFunc(h.dbGetU64), []api.ValueType{api.ValueTypeI32}, []api.ValueType{api.ValueTypeI64}).
		Export("db_get_u64")

	builder.NewFunctionBuilder().
		WithGoModuleFunction(api.GoModuleFunc(h.dbPutU64), []api.ValueType{api.ValueTypeI32, api.ValueTypeI64}, []api.ValueType{}).
		Export("db_put_u64")

	builder.NewFunctionBuilder().
		WithGoModuleFunction(api.GoModuleFunc(h.log), []api.ValueType{api.ValueTypeI32, api.ValueTypeI32, api.ValueTypeI32}, []api.ValueType{}).
		Export("log")

	builder.NewFunctionBuilder().
		WithGoModuleFunction(api.GoModuleFunc(h.abort), []api.ValueType{
			api.ValueTypeI32,
			api.ValueTypeI32,
			api.ValueTypeI32,
			api.ValueTypeI32,
		}, []api.ValueType{}).
		Export("abort")

	_, err := builder.Instantiate(ctx)

	return err
}

func (h *hostEnv) snapshot() BlockContext {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return h.blockCtx
}

func (h *hostEnv) getBlockNumber(_ context.Context, _ api.Module, stack []uint64) {
	stack[0] = h.snapshot().BlockNumber
}

func (h *hostEnv) getChainID(_ context.Context, _ api.Module, stack []uint64) {
	stack[0] = uint64(h.snapshot().ChainID)
}

func (h *hostEnv) getEventCount(_ context.Context, _ api.Module, stack []uint64) {
	stack[0] = uint64(uint32(len(h.snapshot().Events)))
}

func (h *hostEnv) getEventKind(_ context.Context, _ api.Module, stack []uint64) {
	idx := int32(int64(stack[0]))
	events := h.snapshot().Events
	if idx < 0 || int(idx) >= len(events) {
		stack[0] = 0

		return
	}

	stack[0] = uint64(uint32(events[idx].Kind))
}

func (h *hostEnv) getEventAddressID(_ context.Context, _ api.Module, stack []uint64) {
	idx := int32(int64(stack[0]))
	events := h.snapshot().Events
	if idx < 0 || int(idx) >= len(events) {
		stack[0] = 0

		return
	}

	stack[0] = uint64(uint32(events[idx].Target))
}

func (h *hostEnv) slotKey(slot int32) []byte {
	var k [4]byte
	binary.BigEndian.PutUint32(k[:], uint32(slot))

	return k[:]
}

func (h *hostEnv) dbGetU64(ctx context.Context, _ api.Module, stack []uint64) {
	slot := int32(int64(stack[0]))
	key := h.slotKey(slot)

	var val uint64
	err := h.db.View(ctx, func(tx kv.Tx) error {
		v, err := tx.GetOne(application.StrategyStateBucket, key)
		if err != nil {
			return err
		}

		if len(v) == 8 {
			val = binary.BigEndian.Uint64(v)
		}

		return nil
	})
	if err != nil {
		h.logger.Warn().Err(err).Msg("db_get_u64 failed")
		stack[0] = 0

		return
	}

	stack[0] = val
}

func (h *hostEnv) dbPutU64(ctx context.Context, _ api.Module, stack []uint64) {
	slot := int32(int64(stack[0]))
	value := stack[1]
	key := h.slotKey(slot)
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], value)

	err := h.db.Update(ctx, func(tx kv.RwTx) error {
		return tx.Put(application.StrategyStateBucket, key, buf[:])
	})
	if err != nil {
		h.logger.Warn().Err(err).Msg("db_put_u64 failed")
	}
}

func (h *hostEnv) log(_ context.Context, mod api.Module, stack []uint64) {
	level := int32(int64(stack[0]))
	ptr := uint32(stack[1])
	length := uint32(stack[2])

	memory := mod.Memory()
	if memory == nil {
		return
	}

	msg := ""
	if length > 0 {
		if data, ok := memory.Read(ptr, length); ok {
			msg = string(data)
		} else {
			h.logger.Warn().Uint32("ptr", ptr).Uint32("len", length).Msg("log read failed")
		}
	}

	switch level {
	case 10:
		h.logger.Debug().Msg(msg)
	case 20:
		h.logger.Info().Msg(msg)
	case 30:
		h.logger.Warn().Msg(msg)
	case 40:
		h.logger.Error().Msg(msg)
	default:
		h.logger.Info().Int32("level", level).Msg(msg)
	}
}

func (h *hostEnv) abort(_ context.Context, mod api.Module, stack []uint64) {
	messagePtr := uint32(stack[0])
	filePtr := uint32(stack[1])
	line := uint32(stack[2])
	column := uint32(stack[3])

	h.logger.Error().
		Uint32("message_ptr", messagePtr).
		Uint32("file_ptr", filePtr).
		Uint32("line", line).
		Uint32("column", column).
		Msg("wasm abort invoked")

	panic(fmt.Sprintf("wasm abort (%d:%d)", line, column))
}

// StrategyModule wraps a single WASM module instance plus host wiring.
type StrategyModule struct {
	id      string
	runtime wazero.Runtime
	module  api.Module
	host    *hostEnv
}

// ModuleConfig configures a StrategyModule instance.
type ModuleConfig struct {
	ID     string
	Wasm   []byte
	DB     kv.RwDB
	Logger *zerolog.Logger
}

// NewStrategyModule compiles and instantiates the WASM strategy and host functions.
func NewStrategyModule(ctx context.Context, cfg ModuleConfig) (*StrategyModule, error) {
	if len(cfg.Wasm) == 0 {
		return nil, fmt.Errorf("strategy wasm is empty")
	}

	logger := cfg.Logger
	if logger == nil {
		logger = zerolog.Ctx(ctx)
	}

	runtime := wazero.NewRuntime(ctx)
	host := newHostEnv(logger, cfg.DB)

	if err := host.register(ctx, runtime); err != nil {
		return nil, fmt.Errorf("register host env: %w", err)
	}

	compiled, err := runtime.CompileModule(ctx, cfg.Wasm)
	if err != nil {
		return nil, fmt.Errorf("compile strategy wasm: %w", err)
	}

	module, err := runtime.InstantiateModule(ctx, compiled, wazero.NewModuleConfig())
	if err != nil {
		return nil, fmt.Errorf("instantiate strategy wasm: %w", err)
	}

	return &StrategyModule{
		id:      cfg.ID,
		runtime: runtime,
		module:  module,
		host:    host,
	}, nil
}

// Close releases the underlying runtime resources.
func (s *StrategyModule) Close(ctx context.Context) error {
	if s.module != nil {
		_ = s.module.Close(ctx)
	}

	if s.runtime != nil {
		return s.runtime.Close(ctx)
	}

	return nil
}

// OnBlock invokes the exported on_block handler with the provided context.
func (s *StrategyModule) OnBlock(ctx context.Context, block BlockContext) error {
	s.host.setContext(block)

	exported := s.module.ExportedFunction("on_block")
	if exported == nil {
		return fmt.Errorf("strategy %s does not export on_block", s.id)
	}

	_, err := exported.Call(ctx)

	return err
}

func normalizeAddress(addr string) string {
	return strings.ToLower(addr)
}
