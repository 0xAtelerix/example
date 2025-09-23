package application

import (
	"github.com/0xAtelerix/sdk/gosdk/apptypes"
	"github.com/ledgerwatch/erigon-lib/kv"
	"github.com/rs/zerolog/log"
)

// step 2:
// How do you process external blocks and send external transactions
// We strongly recomment to keep it stateless
type StateTransition struct{}

func NewStateTransition() *StateTransition {
	return &StateTransition{}
}

// how to external chains blocks
func (*StateTransition) ProcessBlock(
	b apptypes.ExternalBlock,
	_ kv.RwTx,
) ([]apptypes.ExternalTransaction, error) {
	log.Info().Uint64("n", b.BlockNumber).Msg("block processing is disabled")
	return nil, nil
}
