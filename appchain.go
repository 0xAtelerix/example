package example

import (
	"github.com/ledgerwatch/erigon-lib/kv"

	"github.com/0xAtelerix/sdk/gosdk"
	emitterproto "github.com/0xAtelerix/sdk/gosdk/proto"
	"github.com/0xAtelerix/sdk/gosdk/types"
)

var (
	_ gosdk.StateTransitionSimplified[*ExampleTransaction] = &ExampleStateTransition[*ExampleTransaction]{}
	_ gosdk.StateTransitionInterface[*ExampleTransaction]  = gosdk.BatchProcesser[*ExampleTransaction]{}
	_ types.AppchainBlock                                  = &ExampleBlock{}
)

// appchain implimentation
// step 1:
// your transaction
type ExampleTransaction struct {
	Sender string
	Value  int
}

func (e *ExampleTransaction) Marshal() ([]byte, error) {
	return nil, nil
}
func (e *ExampleTransaction) Unmarshal([]byte) error {
	return nil
}

// step 2:
// How do you process incoming transactions, external blocks and send external transactions
// We strongly recomment to keep it stateless
type ExampleStateTransition[AppTransaction types.AppTransaction] struct{}

func NewStateTransitionExample[AppTransaction types.AppTransaction]() *ExampleStateTransition[AppTransaction] {
	return &ExampleStateTransition[AppTransaction]{}
}

// how to process appchain transactions
func (s *ExampleStateTransition[AppTransaction]) ProcessTX(tx AppTransaction, dbtx kv.RwTx) ([]types.ExternalTransaction, error) {
	return nil, nil
}

// how to external chains blocks
func (s *ExampleStateTransition[AppTransaction]) ProcessBlock(block types.ExternalBlock, tx kv.RwTx) ([]types.ExternalTransaction, error) {
	return nil, nil
}

// step 3:
// How do your block look like
type ExampleBlock struct{}

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
func AppchainExampleBlockConstructor(blockNumber uint64, stateRoot [32]byte, previousBlockHash [32]byte, txsBatch types.Batch[*ExampleTransaction]) *ExampleBlock {
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

// step 5:
// How to communicate with validator
// not sure that it is an appchain part.
type EmitterServer struct {
	emitterproto.UnimplementedEmitterServer
}

type HealthServer struct {
	emitterproto.UnimplementedHealthServer
}
