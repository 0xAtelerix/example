package application

import (
	"context"

	"github.com/0xAtelerix/sdk/gosdk"
	"github.com/0xAtelerix/sdk/gosdk/apptypes"
	"github.com/0xAtelerix/sdk/gosdk/library/tokens"
	"github.com/blocto/solana-go-sdk/client"
	gethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ledgerwatch/erigon-lib/kv"
)

// step 2:
// How do you process external blocks and send external transactions
// We strongly recomment to keep it stateless
type StateTransition struct {
	MultichainAccess *gosdk.MultichainStateAccess
	Subscriber       *gosdk.Subscriber
}

func NewStateTransition(
	multichainAccess *gosdk.MultichainStateAccess,
	subscriber *gosdk.Subscriber,
) *StateTransition {
	return &StateTransition{
		MultichainAccess: multichainAccess,
		Subscriber:       subscriber,
	}
}

// how to external chains blocks
func (st *StateTransition) ProcessBlock(
	blk apptypes.ExternalBlock,
	dbtx kv.RwTx,
) ([]apptypes.ExternalTransaction, error) {
	var extTxs []apptypes.ExternalTransaction

	switch {
	case gosdk.IsEvmChain(apptypes.ChainType(blk.ChainID)):
		ethReceipts, err := st.MultichainAccess.EthReceipts(context.Background(), blk)
		if err != nil {
			return nil, err
		}

		var ok bool

		for _, rec := range ethReceipts {
			ok = st.Subscriber.IsEthSubscription(
				apptypes.ChainType(blk.ChainID),
				gosdk.EthereumAddress(rec.ContractAddress),
			)
			if ok {
				ext, err := st.ProcessEthereumReceipt(rec, dbtx)
				if err != nil {
					return nil, err
				}

				extTxs = append(extTxs, ext...)
			}
		}

	case gosdk.IsSolanaChain(apptypes.ChainType(blk.ChainID)):
		block, err := st.MultichainAccess.SolanaBlock(context.Background(), blk)
		if err != nil {
			return nil, err
		}

		for _, tx := range block.Transactions {
			for i := range tx.Transaction.Message.Header.NumRequireSignatures {
				pub := tx.Transaction.Message.Accounts[i]

				ok := st.Subscriber.IsSolanaSubscription(
					apptypes.ChainType(blk.ChainID),
					gosdk.SolanaAddress(pub),
				)
				if ok {
					ext, err := st.ProcessSolanaTransaction(tx, dbtx)
					if err != nil {
						return nil, err
					}

					extTxs = append(extTxs, ext...)

					break
				}
			}
		}

	default:
		// nothing to do
	}

	return extTxs, nil
}

//nolint:unparam // initial implementation
func (*StateTransition) ProcessEthereumReceipt(
	extTx gethtypes.Receipt,
	dbtx kv.RwTx,
) ([]apptypes.ExternalTransaction, error) {
	transfers := tokens.ExtractErcTransfers(extTx)

	for _, transfer := range transfers {
		_ = transfer.To
		_ = transfer.From
		_ = transfer.Amount
		_ = transfer.Token
	}

	_ = dbtx

	// handle other events
	/*
		for _, txEvent := range extTx.Logs {
			// ABI-encoded events
			// match with ABI
			// read data and use
			// txEvent.Data
		}
	*/

	return nil, nil
}

//nolint:unparam // initial implementation
func (*StateTransition) ProcessSolanaTransaction(
	extTx client.BlockTransaction,
	dbtx kv.RwTx,
) ([]apptypes.ExternalTransaction, error) {
	transfers := tokens.ExtractSplTransfers(extTx)

	for _, transfer := range transfers {
		_ = transfer.Balances.PreTokenBalances
		_ = transfer.Balances.PostTokenBalances
	}

	_ = dbtx

	return nil, nil
}
