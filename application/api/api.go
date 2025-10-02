package api

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/0xAtelerix/sdk/gosdk/rpc"
	"github.com/holiman/uint256"
	"github.com/ledgerwatch/erigon-lib/kv"

	"github.com/0xAtelerix/example/application"
)

type CustomRPC struct {
	rpcServer *rpc.StandardRPCServer
	db        kv.RoDB
}

func NewCustomRPC(rpcServer *rpc.StandardRPCServer, db kv.RoDB) *CustomRPC {
	return &CustomRPC{
		rpcServer: rpcServer,
		db:        db,
	}
}

func (c *CustomRPC) AddRPCMethods() {
	c.rpcServer.AddMethod("getBalance", c.GetBalance)
}

func (c *CustomRPC) GetBalance(ctx context.Context, params []any) (any, error) {
	if len(params) == 0 {
		return nil, application.ErrMissingParameters
	}

	// Convert params[0] directly to balanceReq using json marshal/unmarshal
	paramBytes, err := json.Marshal(params[0])
	if err != nil {
		return nil, fmt.Errorf("failed to marshal parameter: %w", err)
	}

	var balanceReq GetBalanceRequest
	if unmarshalErr := json.Unmarshal(paramBytes, &balanceReq); unmarshalErr != nil {
		return nil, fmt.Errorf("invalid parameters: %w", unmarshalErr)
	}

	// Get balance from database
	balance, err := c.getBalance(ctx, balanceReq.User, balanceReq.Token)
	if err != nil {
		return nil, fmt.Errorf("failed to get balance: %w", err)
	}

	response := GetBalanceResponse{
		User:    balanceReq.User,
		Token:   balanceReq.Token,
		Balance: balance.String(),
	}

	return response, nil
}

func (c *CustomRPC) getBalance(
	ctx context.Context,
	user, token string,
) (*uint256.Int, error) {
	if c.db == nil {
		return uint256.NewInt(0), application.ErrDatabaseNotAvailable
	}

	tx, err := c.db.BeginRo(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Get balance from accounts bucket
	accountKey := application.AccountKey(user, token)

	balanceData, err := tx.GetOne(application.AccountsBucket, accountKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get balance: %w", err)
	}

	balance := uint256.NewInt(0)
	if len(balanceData) > 0 {
		balance.SetBytes(balanceData)
	}

	return balance, nil
}

// GetBalanceRequest represents a balance query request
type GetBalanceRequest struct {
	User  string `json:"user"`
	Token string `json:"token"`
}

// GetBalanceResponse represents a balance query response
type GetBalanceResponse struct {
	User    string `json:"user"`
	Token   string `json:"token"`
	Balance string `json:"balance"`
}
