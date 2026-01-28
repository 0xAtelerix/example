package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/0xAtelerix/sdk/gosdk/rpc"
	"github.com/ledgerwatch/erigon-lib/kv"

	"github.com/0xAtelerix/example/application"
)

type CustomRPC struct {
	rpcServer *rpc.StandardRPCServer
	db        kv.RoDB
	cfg       *application.AppConfig
}

func NewCustomRPC(
	rpcServer *rpc.StandardRPCServer,
	db kv.RoDB,
	cfg *application.AppConfig,
) *CustomRPC {
	return &CustomRPC{
		rpcServer: rpcServer,
		db:        db,
		cfg:       cfg,
	}
}

func (c *CustomRPC) AddRPCMethods() {
	c.rpcServer.AddMethod("getBridgeStatus", c.GetBridgeStatus)
	c.rpcServer.AddMethod("getSupportedNetworks", c.GetSupportedNetworks)
}

// GetSupportedNetworks returns the list of supported networks and their bridge contracts
func (c *CustomRPC) GetSupportedNetworks(_ context.Context, _ []any) (any, error) {
	networks := make([]NetworkInfo, 0, len(c.cfg.Bridge.Contracts))

	for chainID, contract := range c.cfg.Bridge.Contracts {
		networks = append(networks, NetworkInfo{
			ChainID:  chainID,
			Contract: contract,
		})
	}

	return GetSupportedNetworksResponse{Networks: networks}, nil
}

// GetBridgeStatus retrieves the status of a bridge event
func (c *CustomRPC) GetBridgeStatus(ctx context.Context, params []any) (any, error) {
	if len(params) == 0 {
		return nil, application.ErrMissingParameters
	}

	paramBytes, err := json.Marshal(params[0])
	if err != nil {
		return nil, fmt.Errorf("failed to marshal parameter: %w", err)
	}

	var req GetBridgeStatusRequest

	if unmarshalErr := json.Unmarshal(paramBytes, &req); unmarshalErr != nil {
		return nil, fmt.Errorf("invalid parameters: %w", unmarshalErr)
	}

	if req.BridgeID == "" {
		return nil, application.ErrInvalidBridgeID
	}

	event, err := application.GetBridgeEvent(ctx, c.db, req.BridgeID)
	if err != nil {
		if errors.Is(err, application.ErrBridgeNotFound) {
			return nil, application.ErrBridgeNotFound
		}

		return nil, fmt.Errorf("failed to get bridge status: %w", err)
	}

	return GetBridgeStatusResponse{
		BridgeID:     req.BridgeID,
		Status:       event.Status,
		Claimed:      event.Status == application.BridgeStatusCompleted,
		SourceTxHash: event.SourceTxHash,
		ClaimTxHash:  event.ClaimTxHash,
	}, nil
}
