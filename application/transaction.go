package application

import (
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/0xAtelerix/sdk/gosdk/apptypes"
	"github.com/holiman/uint256"
	"github.com/ledgerwatch/erigon-lib/kv"
)

var _ apptypes.AppTransaction[Receipt] = &Transaction[Receipt]{}

func AccountKey(sender string, token string) []byte {
	return []byte(token + sender)
}

type Transaction[R Receipt] struct {
	Sender   string `json:"sender"`
	Value    uint64 `json:"value"`
	Receiver string `json:"receiver"`
	Token    string `json:"token"`
	TxHash   string `json:"hash"`
}

func (e *Transaction[R]) Unmarshal(b []byte) error {
	return json.Unmarshal(b, e)
}

func (e Transaction[R]) Marshal() ([]byte, error) {
	return json.Marshal(e)
}

func (e Transaction[R]) Hash() [32]byte {
	txHash := strings.TrimPrefix(e.TxHash, "0x")

	hashBytes, err := hex.DecodeString(txHash)
	if err != nil {
		panic(err)
	}

	var h [32]byte
	copy(h[:], hashBytes)

	return h
}

func (e Transaction[R]) Process(
	dbTx kv.RwTx,
) (res R, txs []apptypes.ExternalTransaction, err error) {
	var senderBalanceData []byte

	senderTokenKey := AccountKey(e.Sender, e.Token)

	senderBalanceData, err = dbTx.GetOne(AccountsBucket, senderTokenKey)
	if err != nil {
		return e.failedReceipt(err), nil, nil
	}

	if len(senderBalanceData) == 0 {
		return e.failedReceipt(ErrNotEnoughBalance), nil, nil
	}

	senderBalance := &uint256.Int{}
	senderBalance.SetBytes(senderBalanceData)

	if senderBalance.CmpUint64(e.Value) < 0 {
		return e.failedReceipt(ErrNotEnoughBalance), nil, nil
	}

	var receiverBalanceData []byte

	receiverTokenKey := AccountKey(e.Receiver, e.Token)

	receiverBalanceData, err = dbTx.GetOne(AccountsBucket, receiverTokenKey)
	if err != nil {
		return e.failedReceipt(err), nil, nil
	}

	receiverBalance := &uint256.Int{}
	receiverBalance.SetBytes(receiverBalanceData)

	amount := uint256.NewInt(e.Value)

	receiverBalance.Add(receiverBalance, amount)
	senderBalance.Sub(senderBalance, amount)

	err = dbTx.Put(AccountsBucket, senderTokenKey, senderBalance.Bytes())
	if err != nil {
		return e.failedReceipt(err), nil, nil
	}

	err = dbTx.Put(AccountsBucket, receiverTokenKey, receiverBalance.Bytes())
	if err != nil {
		return e.failedReceipt(err), nil, nil
	}

	res = e.successReceipt(senderBalance, receiverBalance)

	return res, []apptypes.ExternalTransaction{}, nil
}

func (e *Transaction[R]) failedReceipt(err error) R {
	return R{
		TxnHash:      e.Hash(),
		Sender:       e.Sender,
		Receiver:     e.Receiver,
		Token:        e.Token,
		Value:        e.Value,
		ErrorMessage: err.Error(),
		TxStatus:     apptypes.ReceiptFailed,
	}
}

func (e *Transaction[R]) successReceipt(senderBalance, receiverBalance *uint256.Int) R {
	return R{
		TxnHash:         e.Hash(),
		Sender:          e.Sender,
		SenderBalance:   senderBalance,
		Receiver:        e.Receiver,
		ReceiverBalance: receiverBalance,
		Token:           e.Token,
		Value:           e.Value,
		TxStatus:        apptypes.ReceiptConfirmed,
	}
}
