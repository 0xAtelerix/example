package application

import "github.com/holiman/uint256"

type Receipt struct {
	Sender          string       `json:"sender"`
	SenderBalance   *uint256.Int `json:"sender_balance"`
	Receiver        string       `json:"receiver"`
	ReceiverBalance *uint256.Int `json:"receiver_balance"`
	Token           string       `json:"token"`
}
