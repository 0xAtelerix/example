package api

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/ledgerwatch/erigon-lib/kv"
	"github.com/ledgerwatch/erigon-lib/kv/mdbx"
	mdbxlog "github.com/ledgerwatch/log/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/0xAtelerix/example/application"
)

func setupAPITestDB(t *testing.T) kv.RwDB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := mdbx.NewMDBX(mdbxlog.New()).
		Path(dbPath).
		WithTableCfg(func(_ kv.TableCfg) kv.TableCfg {
			return application.Tables()
		}).
		Open()
	require.NoError(t, err)

	t.Cleanup(func() {
		db.Close()
	})

	return db
}

func insertAPITestEvent(t *testing.T, db kv.RwDB, event application.BridgeEvent) {
	t.Helper()

	err := db.Update(context.Background(), func(tx kv.RwTx) error {
		data, err := json.Marshal(event)
		if err != nil {
			return err
		}

		return tx.Put(application.BridgeEventsBucket, []byte(event.BridgeID), data)
	})
	require.NoError(t, err)
}

func testAppConfig() *application.AppConfig {
	return &application.AppConfig{
		Bridge: application.BridgeConfig{
			Contracts:     map[uint64]string{},
			TokenMappings: map[uint64]map[string]string{},
		},
	}
}

func TestGetBridgeStatus_Found(t *testing.T) {
	db := setupAPITestDB(t)
	ctx := context.Background()

	rpc := NewCustomRPC(nil, db, testAppConfig())

	event := application.BridgeEvent{
		BridgeID:     "0x1234",
		SourceChain:  11155111,
		DestChain:    50591822,
		Status:       application.BridgeStatusConfirmed,
		SourceTxHash: "0xsourcetx",
	}
	insertAPITestEvent(t, db, event)

	params := []any{map[string]any{"bridgeId": "0x1234"}}
	result, err := rpc.GetBridgeStatus(ctx, params)
	require.NoError(t, err)

	resp, ok := result.(GetBridgeStatusResponse)
	require.True(t, ok)
	assert.Equal(t, "0x1234", resp.BridgeID)
	assert.Equal(t, application.BridgeStatusConfirmed, resp.Status)
	assert.False(t, resp.Claimed)
	assert.Equal(t, "0xsourcetx", resp.SourceTxHash)
}

func TestGetBridgeStatus_Completed(t *testing.T) {
	db := setupAPITestDB(t)
	ctx := context.Background()

	rpc := NewCustomRPC(nil, db, testAppConfig())

	event := application.BridgeEvent{
		BridgeID:     "0x1234",
		SourceChain:  11155111,
		DestChain:    50591822,
		Status:       application.BridgeStatusCompleted,
		SourceTxHash: "0xsourcetx",
		ClaimTxHash:  "0xclaimtx",
	}
	insertAPITestEvent(t, db, event)

	params := []any{map[string]any{"bridgeId": "0x1234"}}
	result, err := rpc.GetBridgeStatus(ctx, params)
	require.NoError(t, err)

	resp, ok := result.(GetBridgeStatusResponse)
	require.True(t, ok)
	assert.Equal(t, application.BridgeStatusCompleted, resp.Status)
	assert.True(t, resp.Claimed)
	assert.Equal(t, "0xclaimtx", resp.ClaimTxHash)
}

func TestGetBridgeStatus_NotFound(t *testing.T) {
	db := setupAPITestDB(t)
	ctx := context.Background()

	rpc := NewCustomRPC(nil, db, testAppConfig())

	params := []any{map[string]any{"bridgeId": "0xnonexistent"}}
	_, err := rpc.GetBridgeStatus(ctx, params)
	require.Error(t, err)
	assert.Equal(t, application.ErrBridgeNotFound, err)
}

func TestGetBridgeStatus_MissingParams(t *testing.T) {
	db := setupAPITestDB(t)
	ctx := context.Background()

	rpc := NewCustomRPC(nil, db, testAppConfig())

	// Empty params
	_, err := rpc.GetBridgeStatus(ctx, []any{})
	require.Error(t, err)
	assert.Equal(t, application.ErrMissingParameters, err)

	// Missing bridgeId
	params := []any{map[string]any{}}
	_, err = rpc.GetBridgeStatus(ctx, params)
	require.Error(t, err)
	assert.Equal(t, application.ErrInvalidBridgeID, err)
}

func TestGetSupportedNetworks(t *testing.T) {
	db := setupAPITestDB(t)
	ctx := context.Background()

	cfg := &application.AppConfig{
		Bridge: application.BridgeConfig{
			Contracts: map[uint64]string{
				11155111: "0x844E740Ea7F404c6208fd85Ee6114a14F8037df7",
				50591822: "0x3C1c8351a09DB0300786148B56EcB7be2FaA322e",
			},
			TokenMappings: map[uint64]map[string]string{},
		},
	}

	rpc := NewCustomRPC(nil, db, cfg)

	result, err := rpc.GetSupportedNetworks(ctx, nil)
	require.NoError(t, err)

	resp, ok := result.(GetSupportedNetworksResponse)
	require.True(t, ok)
	assert.Len(t, resp.Networks, 2)

	// Check that both networks are present
	chainIDs := make(map[uint64]string)
	for _, n := range resp.Networks {
		chainIDs[n.ChainID] = n.Contract
	}

	assert.Equal(t, "0x844E740Ea7F404c6208fd85Ee6114a14F8037df7", chainIDs[11155111])
	assert.Equal(t, "0x3C1c8351a09DB0300786148B56EcB7be2FaA322e", chainIDs[50591822])
}

func TestGetSupportedNetworks_Empty(t *testing.T) {
	db := setupAPITestDB(t)
	ctx := context.Background()

	rpc := NewCustomRPC(nil, db, testAppConfig())

	result, err := rpc.GetSupportedNetworks(ctx, nil)
	require.NoError(t, err)

	resp, ok := result.(GetSupportedNetworksResponse)
	require.True(t, ok)
	assert.Empty(t, resp.Networks)
}
