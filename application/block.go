package application

import (
	"encoding/json"

	"github.com/0xAtelerix/sdk/gosdk/apptypes"
)

var _ apptypes.AppchainBlock = Block{}

type Block struct {
	BlockNum   uint64   `json:"number"`
	BlockHash  [32]byte `json:"blockHash"`
	ParentHash [32]byte `json:"parentHash"`
	Root       [32]byte `json:"root"`
	Txns       []Transaction[Receipt]
}

func (b Block) Number() uint64 {
	return b.BlockNum
}

func (b Block) Hash() [32]byte {
	return b.BlockHash
}

func (b Block) StateRoot() [32]byte {
	return b.Root
}

func (b Block) Bytes() []byte {
	data, _ := json.Marshal(b)
	return data
}

func BlockConstructor(
	blockNumber uint64, // blockNumber
	stateRoot [32]byte, // stateRoot
	previousBlockHash [32]byte, // previousBlockHash
	batch apptypes.Batch[Transaction[Receipt], Receipt], // txsBatch
) *Block {
	return &Block{
		BlockNum:   blockNumber,
		Root:       stateRoot,
		ParentHash: previousBlockHash,
		Txns:       batch.Transactions,
	}
}
