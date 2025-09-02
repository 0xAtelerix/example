package application

import "github.com/ledgerwatch/erigon-lib/kv"

// step 4:
// How to verify that your block state transition is correct between validators
type RootCalculator struct{}

func NewRootCalculator() *RootCalculator {
	return &RootCalculator{}
}

func (*RootCalculator) StateRootCalculator(_ kv.RwTx) ([32]byte, error) {
	return [32]byte{}, nil
}
