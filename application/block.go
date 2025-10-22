package application

import (
	"crypto/rand"

	"github.com/0xAtelerix/sdk/gosdk/apptypes"
	"github.com/fxamacker/cbor/v2"
)

var _ apptypes.AppchainBlock = &Block{}

// step 3:
// How do your block look like
type Block struct {
	BlockNum     uint64                        `json:"number"    cbor:"1,keyasint"`
	Root         [32]byte                       `json:"root"     cbor:"2,keyasint"`
	//Transactions []apptypes.ExternalTransaction `json:"transactions"`
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

func RandBytes(length int) []byte {
	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}

	return buf
}

