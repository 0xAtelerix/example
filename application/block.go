package application

import (
	"github.com/0xAtelerix/sdk/gosdk/apptypes"
	"github.com/goccy/go-json"

	"github.com/0xAtelerix/example/application/transactions"
)

// step 3:
// How do your block look like
type Block struct {
	BlockNum     uint64                         `json:"number"`
	Root         [32]byte                       `json:"root"`
	Transactions []apptypes.ExternalTransaction `json:"transactions"`
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

func (*Block) Bytes() []byte {
	return []byte{}
}

func (b *Block) Marshal() ([]byte, error) {
	return json.Marshal(b)
}

func (b *Block) Unmarshal(data []byte) error {
	return json.Unmarshal(data, b)
}

func BlockConstructor(
	blockNumber uint64, // blockNumber
	stateRoot [32]byte, // stateRoot
	_ [32]byte, // previousBlockHash
	_ apptypes.Batch[transactions.Transaction[transactions.Receipt], transactions.Receipt], // txsBatch
) *Block {
	return &Block{
		BlockNum: blockNumber,
		Root:     stateRoot,
	}
}
