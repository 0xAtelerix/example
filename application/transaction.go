package application

import (
	"github.com/0xAtelerix/sdk/gosdk/apptypes"
	"github.com/ledgerwatch/erigon-lib/kv"
)

// Transaction is a minimal stub to satisfy the SDK's AppTransaction interface.
// This bridge app doesn't use appchain transactions - all processing happens
// via external chain events in ExtBlockProcessor.
//
//nolint:recvcheck // Mixed receivers required: Unmarshal needs pointer, others need value for interface
type Transaction struct {
	TxHash [32]byte `json:"hash"`
}

var _ apptypes.AppTransaction[Receipt] = &Transaction{}

func (*Transaction) Unmarshal(_ []byte) error {
	return nil
}

func (Transaction) Marshal() ([]byte, error) {
	return nil, nil
}

func (t Transaction) Hash() [32]byte {
	return t.TxHash
}

// Process is a no-op - bridge processing happens via ExtBlockProcessor.ProcessBlock()
func (t Transaction) Process(_ kv.RwTx) (Receipt, []apptypes.ExternalTransaction, error) {
	return Receipt{TxnHash: t.TxHash, TxStatus: apptypes.ReceiptConfirmed}, nil, nil
}
