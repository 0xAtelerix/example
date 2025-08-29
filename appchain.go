package example

import (
	"encoding/json"

	"github.com/ledgerwatch/erigon-lib/kv"

	"github.com/0xAtelerix/sdk/gosdk"
	"github.com/0xAtelerix/sdk/gosdk/apptypes"
)

var (
	_ gosdk.StateTransitionSimplified[ExampleTransaction] = &ExampleStateTransition[ExampleTransaction]{}
	_ gosdk.StateTransitionInterface[ExampleTransaction]  = gosdk.BatchProcesser[ExampleTransaction]{}
	_ apptypes.AppchainBlock                              = &ExampleBlock{}
)

// appchain implimentation
// step 1:
// your transaction
type ExampleTransaction struct {
	Sender string `json:"sender"`
	Value  int    `json:"value"`
	TxHash string `json:"hash"`
}

func (e *ExampleTransaction) Unmarshal(b []byte) error {
	return json.Unmarshal(b, e)
}

func (e ExampleTransaction) Marshal() ([]byte, error) {
	return json.Marshal(e)
}

func (e ExampleTransaction) Hash() [32]byte {
	var h [32]byte
	copy(h[:], e.TxHash)

	return h
}

// step 2:
// How do you process incoming transactions, external blocks and send external transactions
// We strongly recomment to keep it stateless
type ExampleStateTransition[AppTransaction apptypes.AppTransaction] struct{}

func NewStateTransitionExample[AppTransaction apptypes.AppTransaction]() *ExampleStateTransition[AppTransaction] {
	return &ExampleStateTransition[AppTransaction]{}
}

// how to process appchain transactions
func (s *ExampleStateTransition[AppTransaction]) ProcessTX(tx AppTransaction, dbtx kv.RwTx) ([]apptypes.ExternalTransaction, error) {
	return nil, nil
}

// how to external chains blocks
func (s *ExampleStateTransition[AppTransaction]) ProcessBlock(block apptypes.ExternalBlock, dbtx kv.RwTx) ([]apptypes.ExternalTransaction, error) {
	return nil, nil
}

// step 3:
// How do your block look like
type ExampleBlock struct {
	Transactions []apptypes.ExternalTransaction `json:"transactions"`
}

func (e *ExampleBlock) Number() uint64 {
	return 0
}
func (e *ExampleBlock) Hash() [32]byte {
	return [32]byte{}
}
func (e *ExampleBlock) StateRoot() [32]byte {
	return [32]byte{}
}
func (e *ExampleBlock) Bytes() []byte {
	return []byte{}
}
func (e *ExampleBlock) Marshal() ([]byte, error) {
	return nil, nil
}
func (e *ExampleBlock) Unmarshal([]byte) error {
	return nil
}

func AppchainExampleBlockConstructor(blockNumber uint64, stateRoot [32]byte, previousBlockHash [32]byte, txsBatch apptypes.Batch[ExampleTransaction]) *ExampleBlock {
	return &ExampleBlock{}
}

// step 4:
// How to verify that your block state transition is correct between validators
type ExampleRootCalculator struct{}

func NewRootCalculatorExample() *ExampleRootCalculator {
	return &ExampleRootCalculator{}
}
func (r *ExampleRootCalculator) StateRootCalculator(tx kv.RwTx) ([32]byte, error) {
	return [32]byte{}, nil
}
