package application

import "encoding/json"

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
