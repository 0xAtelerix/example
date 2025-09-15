package application

import (
	"crypto/sha256"

	"github.com/0xAtelerix/sdk/gosdk/apptypes"
	"github.com/holiman/uint256"
)

//nolint:errname // Receipt is not an error type, it just implements Error() method for interface compliance
type Receipt struct {
	Sender          string       `json:"sender"`
	SenderBalance   *uint256.Int `json:"sender_balance"`
	Receiver        string       `json:"receiver"`
	ReceiverBalance *uint256.Int `json:"receiver_balance"`
	Token           string       `json:"token"`
}

func (r Receipt) TxHash() [32]byte {
	return sha256.Sum256([]byte(r.Sender + r.Token + r.Receiver))
}

func (Receipt) Status() apptypes.TxReceiptStatus {
	return apptypes.ReceiptConfirmed
}

func (Receipt) Error() string {
	return ""
}
