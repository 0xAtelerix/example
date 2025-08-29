package example

import (
	"encoding/json"

	"github.com/0xAtelerix/sdk/gosdk"
	"github.com/0xAtelerix/sdk/gosdk/apptypes"
	"github.com/ledgerwatch/erigon-lib/kv"
)

var (
	_ gosdk.StateTransitionSimplified[Transaction] = &StateTransition[Transaction]{}
	_ gosdk.StateTransitionInterface[Transaction]  = gosdk.BatchProcesser[Transaction]{}
	_ apptypes.AppchainBlock                       = &Block{}
)

// appchain implementation
// step 1:
// your transaction
//
//nolint:recvcheck // intentional use of pointer and non-pointer receivers because of generics
type Transaction struct {
	Sender string `json:"sender"`
	Value  int    `json:"value"`
	TxHash string `json:"hash"`
}

func (e *Transaction) Unmarshal(b []byte) error {
	return json.Unmarshal(b, e)
}

func (e Transaction) Marshal() ([]byte, error) {
	return json.Marshal(e)
}

func (e Transaction) Hash() [32]byte {
	var h [32]byte
	copy(h[:], e.TxHash)

	return h
}

// step 2:
// How do you process incoming transactions, external blocks and send external transactions
// We strongly recomment to keep it stateless
type StateTransition[AppTransaction apptypes.AppTransaction] struct{}

func NewStateTransitionExample[AppTransaction apptypes.AppTransaction]() *StateTransition[AppTransaction] {
	return &StateTransition[AppTransaction]{}
}

// how to process appchain transactions
func (*StateTransition[AppTransaction]) ProcessTX(
	_ AppTransaction,
	_ kv.RwTx,
) ([]apptypes.ExternalTransaction, error) {
	return nil, nil
}

// how to external chains blocks
func (*StateTransition[AppTransaction]) ProcessBlock(
	_ apptypes.ExternalBlock,
	_ kv.RwTx,
) ([]apptypes.ExternalTransaction, error) {
	return nil, nil
}

// step 3:
// How do your block look like
type Block struct {
	Transactions []apptypes.ExternalTransaction `json:"transactions"`
}

func (*Block) Number() uint64 {
	return 0
}

func (*Block) Hash() [32]byte {
	return [32]byte{}
}

func (*Block) StateRoot() [32]byte {
	return [32]byte{}
}

func (*Block) Bytes() []byte {
	return []byte{}
}

func (*Block) Marshal() ([]byte, error) {
	return nil, nil
}

func (*Block) Unmarshal([]byte) error {
	return nil
}

func AppchainExampleBlockConstructor(
	_ uint64, // blockNumber
	_ [32]byte, // stateRoot
	_ [32]byte, // previousBlockHash
	_ apptypes.Batch[Transaction], // txsBatch
) *Block {
	return &Block{}
}

// step 4:
// How to verify that your block state transition is correct between validators
type RootCalculator struct{}

func NewRootCalculatorExample() *RootCalculator {
	return &RootCalculator{}
}

func (*RootCalculator) StateRootCalculator(_ kv.RwTx) ([32]byte, error) {
	return [32]byte{}, nil
}
