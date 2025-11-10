package application

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/0xAtelerix/sdk/gosdk/apptypes"
)

var _ apptypes.AppchainBlock = Block{}

type Block struct {
	BlockNum     uint64                 `json:"number"`
	BlockHash    [32]byte               `json:"blockHash"`
	ParentHash   [32]byte               `json:"parentHash"`
	Root         [32]byte               `json:"root"`
	Transactions []Transaction[Receipt] `json:"transactions,omitempty"`
}

func (b Block) Hash() [32]byte {
	return b.BlockHash
}

func (b Block) StateRoot() [32]byte {
	return b.Root
}

func BlockConstructor(
	blockNumber uint64, // blockNumber
	stateRoot [32]byte, // stateRoot
	previousBlockHash [32]byte, // previousBlockHash
	batch apptypes.Batch[Transaction[Receipt], Receipt], // txsBatch
) *Block {
	hasher := sha256.New()

	var blockNumBytes [8]byte
	binary.BigEndian.PutUint64(blockNumBytes[:], blockNumber)
	hasher.Write(blockNumBytes[:])
	hasher.Write(stateRoot[:])
	hasher.Write(previousBlockHash[:])

	// Compute final hash
	var blockHash [32]byte
	copy(blockHash[:], hasher.Sum(nil))

	return &Block{
		BlockNum:     blockNumber,
		BlockHash:    blockHash,
		Root:         stateRoot,
		ParentHash:   previousBlockHash,
		Transactions: batch.Transactions,
	}
}
