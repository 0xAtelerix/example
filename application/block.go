package application

import (
	"crypto/rand"
	"crypto/sha256"
	"strconv"

	"github.com/0xAtelerix/sdk/gosdk/apptypes"
	"github.com/fxamacker/cbor/v2"
	"github.com/ledgerwatch/erigon-lib/kv"
)

var _ apptypes.AppchainBlock = &Block{}

// TestTransaction - test transaction implementation
type TestTransaction[R TestReceipt] struct {
	From  string `json:"from"  cbor:"1,keyasint"`
	To    string `json:"to"    cbor:"2,keyasint"`
	Value int    `json:"value" cbor:"3,keyasint"`
}

func (t TestTransaction[R]) Hash() [32]byte {
	s := t.From + t.To + strconv.Itoa(t.Value)

	return sha256.Sum256([]byte(s))
}

func (TestTransaction[R]) Process(
	_ kv.RwTx,
) (rec R, txs []apptypes.ExternalTransaction, err error) {
	return rec, txs, err
}

// TestReceipt - test receipt implementation
type TestReceipt struct {
	ReceiptStatus   apptypes.TxReceiptStatus `json:"status" cbor:"1,keyasint"`
	TransactionHash [32]byte                 `json:"txHash" cbor:"2,keyasint"`
}

func (r TestReceipt) TxHash() [32]byte {
	return r.TransactionHash
}

func (r TestReceipt) Status() apptypes.TxReceiptStatus {
	return r.ReceiptStatus
}

func (r TestReceipt) Error() string {
	if r.ReceiptStatus == apptypes.ReceiptFailed {
		return "transaction failed"
	}

	return ""
}

// Block represents an application block stored in the appchain state.
type Block struct {
	BlockNum uint64                         `json:"number" cbor:"1,keyasint"`
	Root     [32]byte                       `json:"root"   cbor:"2,keyasint"`
	Txs      []TestTransaction[TestReceipt] `json:"txs"    cbor:"3,keyasint"`
}

func (b *Block) Number() uint64 {
	return b.BlockNum
}

func (b *Block) Hash() [32]byte {
	return b.Root
}

func (b *Block) StateRoot() [32]byte {
	return b.Root
}

func (b *Block) Bytes() []byte {
	data, _ := cbor.Marshal(b)

	return data
}

func BlockConstructor(
	blockNumber uint64, // blockNumber
	stateRoot [32]byte, // stateRoot
	_ [32]byte, // previousBlockHash
	_ apptypes.Batch[Transaction[Receipt], Receipt], // txsBatch
) *Block {
	return &Block{
		BlockNum: blockNumber,
		Root:     stateRoot,
	}
}

// RandBytes returns a slice filled with cryptographically secure random bytes.
func RandBytes(length int) []byte {
	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}

	return buf
}
