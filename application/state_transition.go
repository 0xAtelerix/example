package application

import (
	"github.com/0xAtelerix/sdk/gosdk/apptypes"
	"github.com/ledgerwatch/erigon-lib/kv"
)

// step 2:
// How do you process incoming transactions, external blocks and send external transactions
// We strongly recomment to keep it stateless
type StateTransition[AppTransaction apptypes.AppTransaction] struct{}

func NewStateTransition[AppTransaction apptypes.AppTransaction]() *StateTransition[AppTransaction] {
	return &StateTransition[AppTransaction]{}
}

// how to process appchain transactions
func (*StateTransition[AppTransaction]) ProcessTX(
	_ AppTransaction,
	_ kv.RwTx,
) ([]apptypes.ExternalTransaction, error) {
	return nil, nil
}

// how to external chains blocks
func (*StateTransition[AppTransaction]) ProcessBlock(
	_ apptypes.ExternalBlock,
	_ kv.RwTx,
) ([]apptypes.ExternalTransaction, error) {
	return nil, nil
}
