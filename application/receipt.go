package application

import (
	"github.com/0xAtelerix/sdk/gosdk/apptypes"
	"github.com/holiman/uint256"
)

var _ apptypes.Receipt = &Receipt{}

//nolint:errname // Receipt is not an error type, it just implements Error() method for interface compliance
type Receipt struct {
	// Base receipt fields
	TxnHash      [32]byte                 `json:"tx_hash"`
	ErrorMessage string                   `json:"error,omitempty"`
	TxStatus     apptypes.TxReceiptStatus `json:"tx_status"`

	// Additional fields based on txn
	Sender          string       `json:"sender"`
	SenderBalance   *uint256.Int `json:"sender_balance"`
	Receiver        string       `json:"receiver"`
	ReceiverBalance *uint256.Int `json:"receiver_balance"`
	Token           string       `json:"token"`
	Value           uint64       `json:"value"`
}

func (r Receipt) TxHash() [32]byte {
	return r.TxnHash
}

func (r Receipt) Status() apptypes.TxReceiptStatus {
	return r.TxStatus
}

func (r Receipt) Error() string {
	return r.ErrorMessage
}
