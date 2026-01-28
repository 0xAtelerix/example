package application

import (
	"github.com/0xAtelerix/sdk/gosdk"
	"github.com/0xAtelerix/sdk/gosdk/apptypes"
	"github.com/0xAtelerix/sdk/gosdk/library"
	"github.com/rs/zerolog/log"
)

// Event signatures for subscription
const (
	BridgeInitiatedSignature = "BridgeInitiated(bytes32,uint256,uint256,address,uint256,address,address)"
	AssetClaimedSignature    = "AssetClaimed(bytes32,address,address,uint256)"
)

// SubscribeBridgeContracts registers the bridge contracts and events with the subscriber
// so that ProcessBlock receives blocks containing events from these contracts.
func SubscribeBridgeContracts(subscriber *gosdk.Subscriber, cfg *AppConfig) {
	contracts := cfg.Bridge.GetBridgeContracts()

	for chainID, contractAddr := range contracts {
		subscriber.SubscribeEthContract(
			apptypes.ChainType(chainID),
			library.EthereumAddress(contractAddr),
			nil,
			library.EventTopic(BridgeInitiatedSignature),
			library.EventTopic(AssetClaimedSignature),
		)

		log.Info().
			Uint64("chainID", chainID).
			Str("contract", contractAddr.Hex()).
			Msg("Subscribed to bridge contract")
	}
}
