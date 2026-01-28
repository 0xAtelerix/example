package application

import "github.com/ledgerwatch/erigon-lib/kv"

const (
	BridgeEventsBucket = "bridge_events" // bridgeId -> BridgeEvent
)

func Tables() kv.TableCfg {
	return kv.TableCfg{
		BridgeEventsBucket: {},
	}
}
