package application

import (
	"github.com/0xAtelerix/sdk/gosdk/apptypes"
	"github.com/ledgerwatch/erigon-lib/kv"
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
	_ apptypes.ExternalBlock,
	_ kv.RwTx,
) ([]apptypes.ExternalTransaction, error) {
	return nil, nil
}
