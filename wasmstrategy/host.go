package wasmstrategy

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ledgerwatch/erigon-lib/kv"
	"github.com/rs/zerolog"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/sys"

	"github.com/0xAtelerix/example/application"
)

const (
	gasExitCode       uint32 = 0xfffffff0
	hostAbortExitCode uint32 = 0xfffffff1
	defaultGasLimit          = 50_000
	costCheap         uint64 = 1
	costEvent         uint64 = 2
	costDbRead        uint64 = 15
	costDbWrite       uint64 = 40
	costLog           uint64 = 5
)

var (
	errGasLimitExceeded = errors.New("strategy gas limit exceeded")
	errNoEntryPoint     = errors.New("strategy does not export on_block")
)

var allowedEnvImports = map[string]struct{}{
	"get_block_number":     {},
	"get_chain_id":         {},
	"get_event_count":      {},
	"get_event_kind":       {},
	"get_event_address_id": {},
	"db_get_u64":           {},
	"db_put_u64":           {},
	"log":                  {},
	"abort":                {},
}

// StrategyLimits configures execution guard rails for a single strategy.
type StrategyLimits struct {
	GasLimit uint64
	Timeout  time.Duration
}

// hostEnv backs the env.* host functions consumed by AssemblyScript strategies.
type hostEnv struct {
	logger *zerolog.Logger
	db     kv.RwDB

	mu       sync.RWMutex
	blockCtx BlockContext
	gasLimit uint64
	gasUsed  uint64
}

func newHostEnv(logger *zerolog.Logger, db kv.RwDB) *hostEnv {
	return &hostEnv{logger: logger, db: db}
}

func (h *hostEnv) prepare(ctx BlockContext, gasLimit uint64) {
	h.mu.Lock()
	h.blockCtx = ctx
	h.gasLimit = gasLimit
	h.gasUsed = 0
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

func (h *hostEnv) charge(ctx context.Context, mod api.Module, cost uint64, reason string) {
	if cost == 0 {
		return
	}

	h.mu.Lock()
	h.gasUsed += cost
	limit := h.gasLimit
	used := h.gasUsed
	h.mu.Unlock()

	if limit == 0 || used <= limit {
		return
	}

	h.logger.Warn().
		Uint64("cost", cost).
		Uint64("used", used).
		Uint64("limit", limit).
		Str("reason", reason).
		Msg("strategy gas limit exceeded")

	h.trap(ctx, mod, gasExitCode)
}

func (h *hostEnv) getBlockNumber(ctx context.Context, mod api.Module, stack []uint64) {
	h.charge(ctx, mod, costCheap, "get_block_number")
	stack[0] = h.snapshot().BlockNumber
}

func (h *hostEnv) getChainID(ctx context.Context, mod api.Module, stack []uint64) {
	h.charge(ctx, mod, costCheap, "get_chain_id")
	stack[0] = uint64(h.snapshot().ChainID)
}

func (h *hostEnv) getEventCount(ctx context.Context, mod api.Module, stack []uint64) {
	h.charge(ctx, mod, costCheap, "get_event_count")
	stack[0] = uint64(uint32(len(h.snapshot().Events)))
}

func (h *hostEnv) getEventKind(ctx context.Context, mod api.Module, stack []uint64) {
	h.charge(ctx, mod, costEvent, "get_event_kind")
	idx := int32(int64(stack[0]))
	events := h.snapshot().Events
	if idx < 0 || int(idx) >= len(events) {
		stack[0] = 0

		return
	}

	stack[0] = uint64(uint32(events[idx].Kind))
}

func (h *hostEnv) getEventAddressID(ctx context.Context, mod api.Module, stack []uint64) {
	h.charge(ctx, mod, costEvent, "get_event_address_id")
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

func (h *hostEnv) dbGetU64(ctx context.Context, mod api.Module, stack []uint64) {
	h.charge(ctx, mod, costDbRead, "db_get_u64")
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

func (h *hostEnv) dbPutU64(ctx context.Context, mod api.Module, stack []uint64) {
	h.charge(ctx, mod, costDbWrite, "db_put_u64")
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

func (h *hostEnv) log(ctx context.Context, mod api.Module, stack []uint64) {
	h.charge(ctx, mod, costLog, "log")
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

func (h *hostEnv) abort(ctx context.Context, mod api.Module, stack []uint64) {
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

	h.trap(ctx, mod, hostAbortExitCode)
}

func (h *hostEnv) trap(ctx context.Context, mod api.Module, code uint32) {
	_ = mod.CloseWithExitCode(ctx, code)
	panic(sys.NewExitError(code))
}

// StrategyModule wraps a single WASM module instance plus host wiring.
type StrategyModule struct {
	id      string
	runtime wazero.Runtime
	module  api.Module
	host    *hostEnv
	limits  StrategyLimits
}

// ModuleConfig configures a StrategyModule instance.
type ModuleConfig struct {
	ID     string
	Wasm   []byte
	DB     kv.RwDB
	Logger *zerolog.Logger
	Limits StrategyLimits
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

	if err := validateModule(compiled); err != nil {
		return nil, err
	}

	moduleConfig := wazero.NewModuleConfig().WithName(fmt.Sprintf("strategy-%s", cfg.ID))
	module, err := runtime.InstantiateModule(ctx, compiled, moduleConfig)
	if err != nil {
		return nil, fmt.Errorf("instantiate strategy wasm: %w", err)
	}

	limits := cfg.Limits
	if limits.GasLimit == 0 {
		limits.GasLimit = defaultGasLimit
	}

	return &StrategyModule{
		id:      cfg.ID,
		runtime: runtime,
		module:  module,
		host:    host,
		limits:  limits,
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
	s.host.prepare(block, s.limits.GasLimit)

	execCtx := ctx
	cancel := func() {}
	if s.limits.Timeout > 0 {
		execCtx, cancel = context.WithTimeout(ctx, s.limits.Timeout)
	}
	defer cancel()

	exported := s.module.ExportedFunction("on_block")
	if exported == nil {
		return fmt.Errorf("%s: %w", s.id, errNoEntryPoint)
	}

	_, err := exported.Call(execCtx)
	if err == nil {
		return nil
	}

	var exitErr *sys.ExitError
	if errors.As(err, &exitErr) {
		switch exitErr.ExitCode() {
		case gasExitCode:
			return errGasLimitExceeded
		case hostAbortExitCode:
			return fmt.Errorf("strategy %s aborted execution", s.id)
		case sys.ExitCodeDeadlineExceeded:
			return fmt.Errorf("strategy %s timed out: %w", s.id, context.DeadlineExceeded)
		case sys.ExitCodeContextCanceled:
			return fmt.Errorf("strategy %s canceled: %w", s.id, context.Canceled)
		default:
			return exitErr
		}
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("strategy %s timed out: %w", s.id, err)
	}

	return err
}

func normalizeAddress(addr string) string {
	return strings.ToLower(addr)
}

func validateModule(compiled wazero.CompiledModule) error {
	for _, fn := range compiled.ImportedFunctions() {
		if _, _, imported := fn.Import(); !imported {
			continue
		}

		moduleName, funcName, _ := fn.Import()
		if moduleName != "env" {
			return fmt.Errorf("imports from module %q are not allowed", moduleName)
		}

		if _, ok := allowedEnvImports[funcName]; !ok {
			return fmt.Errorf("import env.%s is not allowed", funcName)
		}
	}

	if mems := compiled.ImportedMemories(); len(mems) > 0 {
		return fmt.Errorf("imported memories are not supported")
	}

	return nil
}
