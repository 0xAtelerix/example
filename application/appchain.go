package application

import (
	"github.com/0xAtelerix/sdk/gosdk"
	"github.com/0xAtelerix/sdk/gosdk/apptypes"

	"github.com/0xAtelerix/example/application/transactions"
)

var (
	_ gosdk.StateTransitionSimplified                                                                      = &StateTransition{}
	_ apptypes.AppchainBlock                                                                               = &Block{}
	_ gosdk.StateTransitionInterface[transactions.Transaction[transactions.Receipt], transactions.Receipt] = gosdk.BatchProcesser[
		transactions.Transaction[transactions.Receipt], transactions.Receipt]{}
)
