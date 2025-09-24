package application

import (
	"context"

	"github.com/0xAtelerix/sdk/gosdk"
	"github.com/0xAtelerix/sdk/gosdk/apptypes"
	"github.com/ledgerwatch/erigon-lib/kv"
	"github.com/rs/zerolog/log"
)

var (
	_ gosdk.StateTransitionSimplified                               = &StateTransition{}
	_ gosdk.StateTransitionInterface[Transaction[Receipt], Receipt] = gosdk.BatchProcesser[Transaction[Receipt], Receipt]{}
)

type StateTransition struct {
	msa *gosdk.MultichainStateAccess
}

func NewStateTransition(msa *gosdk.MultichainStateAccess) *StateTransition {
	return &StateTransition{
		msa: msa,
	}
}

// how to external chains blocks
func (st *StateTransition) ProcessBlock(
	b apptypes.ExternalBlock,
	_ kv.RwTx,
) ([]apptypes.ExternalTransaction, error) {
	block, err := st.msa.EthBlock(context.Background(), b)
	if err != nil {
		return nil, err
	}

	receipts, err := st.msa.EthReceipts(context.Background(), b)
	if err != nil {
		return nil, err
	}

	log.Info().
		Uint64("chainID", b.ChainID).
		Uint64("n", block.Header.Number.Uint64()).
		Str("hash", block.Header.Hash().String()).
		Int("transactions", len(block.Body.Transactions)).
		Int("receipts", len(receipts)).
		Msg("External block")

	return nil, nil
}
