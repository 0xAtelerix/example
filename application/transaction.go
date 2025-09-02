package application

import (
	"encoding/json"
	"fmt"

	"github.com/0xAtelerix/sdk/gosdk/apptypes"
	"github.com/holiman/uint256"
	"github.com/ledgerwatch/erigon-lib/kv"
)

// appchain implementation
// step 1:
// your transaction
//

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
	var h [32]byte
	copy(h[:], e.TxHash)

	return h
}

func (e Transaction[R]) Process(
	dbTx kv.RwTx,
) (res R, txs []apptypes.ExternalTransaction, err error) {
	// get sender's balance
	var senderBalanceData []byte

	senderBalanceData, err = dbTx.GetOne(accontsBucket, AccountKey(e.Sender, e.Token))
	if err != nil {
		return res, txs, err
	}

	senderBalance := &uint256.Int{}
	senderBalance.SetBytes(senderBalanceData)

	if senderBalance.CmpUint64(e.Value) < 0 {
		return R{}, nil, ErrNotEnoughBalance
	}

	var receiverBalanceData []byte

	receiverBalanceData, err = dbTx.GetOne(accontsBucket, AccountKey(e.Receiver, e.Token))
	if err != nil {
		return res, txs, err
	}

	receiverBalance := &uint256.Int{}
	receiverBalance.SetBytes(receiverBalanceData)

	amount := uint256.NewInt(e.Value)

	// add receiver's balance
	// reduce sender's balance
	receiverBalance = receiverBalance.Add(receiverBalance, amount)
	senderBalance = senderBalance.Sub(senderBalance, amount)

	err = dbTx.Put(accontsBucket, AccountKey(e.Sender, e.Token), senderBalance.Bytes())
	if err != nil {
		return R{}, nil, fmt.Errorf("can't store sender's balance %w", err)
	}

	err = dbTx.Put(accontsBucket, AccountKey(e.Receiver, e.Token), receiverBalance.Bytes())
	if err != nil {
		return R{}, nil, fmt.Errorf("can't store receiver's balance %w", err)
	}

	res = R{
		Sender:          e.Sender,
		SenderBalance:   senderBalance,
		Receiver:        e.Receiver,
		ReceiverBalance: receiverBalance,
		Token:           e.Token,
	}

	return res, nil, nil
}

const (
	accontsBucket = "appaccounts" // account+token -> value
)

func Tables() kv.TableCfg {
	return kv.TableCfg{
		accontsBucket: {},
	}
}

func AccountKey(sender string, token string) []byte {
	return []byte(sender + token)
}
