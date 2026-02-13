package application

import "github.com/ledgerwatch/erigon-lib/kv"

const (
	BridgeEventsBucket  = "bridge_events"  // bridgeId -> BridgeEvent (all events)
	PendingEventsBucket = "pending_events" // bridgeId -> BridgeEvent (only pending, for fast retry scan)
)

func Tables() kv.TableCfg {
	return kv.TableCfg{
		BridgeEventsBucket:  {},
		PendingEventsBucket: {},
	}
}
