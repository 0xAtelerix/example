package application

import (
	"github.com/0xAtelerix/sdk/gosdk"
	"github.com/0xAtelerix/sdk/gosdk/apptypes"
)

var (
	_ gosdk.StateTransitionSimplified                               = &StateTransition{}
	_ gosdk.StateTransitionInterface[Transaction[Receipt], Receipt] = gosdk.BatchProcesser[Transaction[Receipt], Receipt]{}
	_ apptypes.AppchainBlock                                        = &Block{}
)
