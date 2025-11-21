package wasmstrategy

import (
	"context"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
)

type gasListenerFactory struct {
	host *hostEnv
	cost uint64
}

func (f *gasListenerFactory) NewFunctionListener(definition api.FunctionDefinition) experimental.FunctionListener {
	// If no gas accounting is configured, skip installing listeners.
	if f.cost == 0 || f.host == nil {
		return nil
	}

	return &gasFunctionListener{
		host: f.host,
		cost: f.cost,
		name: definition.DebugName(),
	}
}

type gasFunctionListener struct {
	host *hostEnv
	cost uint64
	name string
}

func (l *gasFunctionListener) Before(ctx context.Context, mod api.Module, _ api.FunctionDefinition, _ []uint64, _ experimental.StackIterator) {
	l.host.charge(ctx, mod, l.cost, l.name)
}

func (l *gasFunctionListener) After(context.Context, api.Module, api.FunctionDefinition, []uint64) {}

func (l *gasFunctionListener) Abort(context.Context, api.Module, api.FunctionDefinition, error) {}
