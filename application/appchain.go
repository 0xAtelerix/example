package application

import (
	"github.com/0xAtelerix/sdk/gosdk"
	"github.com/0xAtelerix/sdk/gosdk/apptypes"
)

var (
	_ gosdk.StateTransitionSimplified[Transaction] = &StateTransition[Transaction]{}
	_ gosdk.StateTransitionInterface[Transaction]  = gosdk.BatchProcesser[Transaction]{}
	_ apptypes.AppchainBlock                       = &Block{}
)
