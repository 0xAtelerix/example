package application

import (
	"context"
	"encoding/json"

	"github.com/ledgerwatch/erigon-lib/kv"
)

// GetBridgeEvent retrieves a bridge event by ID
func GetBridgeEvent(ctx context.Context, db kv.RoDB, bridgeID string) (*BridgeEvent, error) {
	var event BridgeEvent

	err := db.View(ctx, func(tx kv.Tx) error {
		data, err := tx.GetOne(BridgeEventsBucket, []byte(bridgeID))
		if err != nil {
			return err
		}

		if len(data) == 0 {
			return ErrBridgeNotFound
		}

		return json.Unmarshal(data, &event)
	})
	if err != nil {
		return nil, err
	}

	return &event, nil
}
