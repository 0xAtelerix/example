package application

import "github.com/0xAtelerix/sdk/gosdk/apptypes"

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

func BlockConstructor(
	_ uint64, // blockNumber
	_ [32]byte, // stateRoot
	_ [32]byte, // previousBlockHash
	_ apptypes.Batch[Transaction], // txsBatch
) *Block {
	return &Block{}
}
