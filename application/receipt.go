package application

import "github.com/0xAtelerix/sdk/gosdk/apptypes"

// Receipt is a minimal stub to satisfy the SDK's Receipt interface.
// This bridge app doesn't use receipts - bridge events are stored directly.
//
//nolint:errname // Name must be Receipt to implement apptypes.Receipt interface
type Receipt struct {
	TxnHash  [32]byte                 `json:"txHash"`
	TxStatus apptypes.TxReceiptStatus `json:"status"`
}

var _ apptypes.Receipt = &Receipt{}

func (r Receipt) TxHash() [32]byte {
	return r.TxnHash
}

func (r Receipt) Status() apptypes.TxReceiptStatus {
	return r.TxStatus
}

func (Receipt) Error() string {
	return ""
}
