package application

import (
	"context"
	"encoding/json"
	"math/big"
	"strconv"
	"strings"

	"github.com/0xAtelerix/sdk/gosdk"
	"github.com/0xAtelerix/sdk/gosdk/apptypes"
	"github.com/0xAtelerix/sdk/gosdk/evmtypes"
	"github.com/0xAtelerix/sdk/gosdk/external"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ledgerwatch/erigon-lib/kv"
	"github.com/rs/zerolog/log"

	"github.com/0xAtelerix/example/application/metrics"
)

const (
	BridgeInitiatedSigHash = "0xa43a2e0bb4454dc2f20f4a34be7549f0e1b00e4f5e88805c729900e40471a0cb"
	AssetClaimedSigHash    = "0x260120404c049bd806f3d5d3444295a9eab7f94112fdec90a6072ad39acae708"
)

const bridgeInitiatedEventABI = `[{"anonymous":false,"inputs":[` +
	`{"indexed":true,"internalType":"bytes32","name":"bridgeId","type":"bytes32"},` +
	`{"indexed":true,"internalType":"uint256","name":"sourceChainId","type":"uint256"},` +
	`{"indexed":true,"internalType":"uint256","name":"destChainId","type":"uint256"},` +
	`{"indexed":false,"internalType":"address","name":"token","type":"address"},` +
	`{"indexed":false,"internalType":"uint256","name":"amount","type":"uint256"},` +
	`{"indexed":false,"internalType":"address","name":"sender","type":"address"},` +
	`{"indexed":false,"internalType":"address","name":"recipient","type":"address"}],` +
	`"name":"BridgeInitiated","type":"event"}]`

var _ gosdk.ExternalBlockProcessor = &ExtBlockProcessor{}

type ExtBlockProcessor struct {
	msa             gosdk.MultichainStateAccessor
	bridgeABI       abi.ABI
	bridgeContracts map[uint64]common.Address                    // chainID -> bridge contract
	tokenMappings   map[uint64]map[common.Address]common.Address // sourceChain -> token -> destToken
}

func NewExtBlockProcessor(
	msa gosdk.MultichainStateAccessor,
	cfg *AppConfig,
) *ExtBlockProcessor {
	bridgeABI, err := abi.JSON(strings.NewReader(bridgeInitiatedEventABI))
	if err != nil {
		panic("failed to parse BridgeInitiated ABI: " + err.Error())
	}

	return &ExtBlockProcessor{
		msa:             msa,
		bridgeABI:       bridgeABI,
		bridgeContracts: cfg.Bridge.GetBridgeContracts(),
		tokenMappings:   cfg.Bridge.GetTokenMappings(),
	}
}

// ProcessBlock processes external chain blocks (EVM chains only)
func (p *ExtBlockProcessor) ProcessBlock(
	b apptypes.ExternalBlock,
	tx kv.RwTx,
) ([]apptypes.ExternalTransaction, error) {
	if _, ok := p.bridgeContracts[b.ChainID]; !ok {
		log.Warn().Uint64("chainID", b.ChainID).Msg("Unsupported chain, skipping...")

		return nil, nil
	}

	return p.processEVMBlock(b, tx)
}

func (p *ExtBlockProcessor) processEVMBlock(
	b apptypes.ExternalBlock,
	dbtx kv.RwTx,
) ([]apptypes.ExternalTransaction, error) {
	var externalTxs []apptypes.ExternalTransaction

	receipts, err := p.msa.EVMReceipts(context.Background(), b)
	if err != nil {
		return nil, err
	}

	for _, r := range receipts {
		extTxs := p.processReceipt(dbtx, r, b.ChainID)
		if len(extTxs) > 0 {
			externalTxs = append(externalTxs, extTxs...)
		}
	}

	// Update last processed block metric
	metrics.LastProcessedBlock.WithLabelValues(strconv.FormatUint(b.ChainID, 10)).Set(float64(b.BlockNumber))

	log.Info().
		Uint64("chainID", b.ChainID).
		Uint64("blockNumber", b.BlockNumber).
		Int("receipts", len(receipts)).
		Int("extTxs", len(externalTxs)).
		Msg("Processed EVM external block")

	return externalTxs, nil
}

// processReceipt handles Bridge events from the external chain
func (p *ExtBlockProcessor) processReceipt(
	dbtx kv.RwTx,
	r evmtypes.Receipt,
	chainID uint64,
) []apptypes.ExternalTransaction {
	var externalTxs []apptypes.ExternalTransaction

	for _, vlog := range r.Logs {
		// Check if log is from the expected bridge contract for this chain
		if p.bridgeContracts[chainID] != vlog.Address || len(vlog.Topics) == 0 {
			continue
		}

		switch vlog.Topics[0].Hex() {
		case BridgeInitiatedSigHash:
			bridgeEvent, err := p.decodeBridgeInitiatedEvent(vlog)
			if err != nil {
				log.Error().Err(err).Msg("Failed to decode BridgeInitiated event")

				continue
			}

			// Validate source chain matches
			if bridgeEvent.SourceChain != chainID {
				log.Warn().
					Str("bridgeId", bridgeEvent.BridgeID).
					Msg("Source chain mismatch, skipping")

				continue
			}

			// Validate destination chain is supported
			if _, ok := p.bridgeContracts[bridgeEvent.DestChain]; !ok {
				log.Warn().
					Str("bridgeId", bridgeEvent.BridgeID).
					Uint64("destChain", bridgeEvent.DestChain).
					Msg("Unsupported destination chain")

				continue
			}

			// Skip if already processed
			exists, err := dbtx.GetOne(BridgeEventsBucket, []byte(bridgeEvent.BridgeID))
			if err != nil {
				log.Error().Err(err).Msg("Failed to check existing bridge event")

				continue
			}

			if len(exists) > 0 {
				continue
			}

			// Create ExtTx for the destination chain
			extTx, err := p.createMintTransaction(
				bridgeEvent.DestChain,
				bridgeEvent.BridgeID,
				bridgeEvent.SourceChain,
				bridgeEvent.Token,
				bridgeEvent.Amount,
				bridgeEvent.Recipient,
			)
			if err != nil {
				log.Error().Err(err).Str("bridgeId", bridgeEvent.BridgeID).
					Msg("Failed to create mint transaction")

				continue
			}

			// Store event as Confirmed
			if storeErr := storeBridgeEvent(dbtx, bridgeEvent); storeErr != nil {
				log.Error().
					Err(storeErr).
					Str("bridgeId", bridgeEvent.BridgeID).
					Msg("Failed to store event")

				continue // Don't return ExtTx if store fails
			}

			// Update metrics
			srcChain := strconv.FormatUint(bridgeEvent.SourceChain, 10)
			dstChain := strconv.FormatUint(bridgeEvent.DestChain, 10)
			metrics.BridgeTransactionsTotal.WithLabelValues(srcChain, dstChain, "confirmed").Inc()
			metrics.BridgeTransactionsPending.Inc()
			metrics.TrackPending(bridgeEvent.BridgeID)

			log.Info().
				Str("bridgeId", bridgeEvent.BridgeID).
				Uint64("src", bridgeEvent.SourceChain).
				Uint64("dst", bridgeEvent.DestChain).
				Str("amount", bridgeEvent.Amount).
				Msg("Bridge event processed")

			externalTxs = append(externalTxs, extTx)

		case AssetClaimedSigHash:
			if len(vlog.Topics) < 2 {
				continue
			}

			bridgeID := vlog.Topics[1].Hex()
			bridgeKey := []byte(bridgeID)

			existing, getErr := dbtx.GetOne(BridgeEventsBucket, bridgeKey)
			if getErr != nil || len(existing) == 0 {
				continue
			}

			var event BridgeEvent
			if unmarshalErr := json.Unmarshal(existing, &event); unmarshalErr != nil {
				continue
			}

			if event.Status == BridgeStatusCompleted {
				continue
			}

			// Update status and save
			event.Status = BridgeStatusCompleted
			event.ClaimTxHash = vlog.TxHash.Hex()

			updatedData, err := json.Marshal(event)
			if err != nil {
				log.Error().Err(err).Str("bridgeId", bridgeID).Msg("Failed to marshal bridge event")

				continue
			}

			if err := dbtx.Put(BridgeEventsBucket, bridgeKey, updatedData); err != nil {
				log.Error().Err(err).Str("bridgeId", bridgeID).Msg("Failed to update bridge event")

				continue
			}

			// Update metrics
			metrics.BridgeTransactionsPending.Dec()
			metrics.ResolvePending(bridgeID)
			metrics.BridgeTransactionsCompleted.Inc()
			srcChain := strconv.FormatUint(event.SourceChain, 10)
			dstChain := strconv.FormatUint(event.DestChain, 10)
			metrics.BridgeTransactionsTotal.WithLabelValues(srcChain, dstChain, "completed").Inc()

			log.Info().Str("bridgeId", bridgeID).Msg("Bridge marked as completed")

		default:
			// Ignore other events
		}
	}

	return externalTxs
}

// BridgeInitiatedEvent represents the decoded BridgeInitiated event
type BridgeInitiatedEvent struct {
	BridgeID    string
	SourceChain uint64
	DestChain   uint64
	Token       string
	Amount      string // String to support amounts > uint64
	Sender      string
	Recipient   string
	TxHash      string
}

// decodeBridgeInitiatedEvent decodes a BridgeInitiated event
func (p *ExtBlockProcessor) decodeBridgeInitiatedEvent(
	vlog *types.Log,
) (*BridgeInitiatedEvent, error) {
	if len(vlog.Topics) < 4 {
		return nil, Error("insufficient topics in BridgeInitiated event")
	}

	var eventData struct {
		Token     common.Address
		Amount    *big.Int
		Sender    common.Address
		Recipient common.Address
	}

	if err := p.bridgeABI.UnpackIntoInterface(&eventData, "BridgeInitiated", vlog.Data); err != nil {
		return nil, err
	}

	return &BridgeInitiatedEvent{
		BridgeID:    vlog.Topics[1].Hex(),
		SourceChain: vlog.Topics[2].Big().Uint64(),
		DestChain:   vlog.Topics[3].Big().Uint64(),
		Token:       eventData.Token.Hex(),
		Amount:      eventData.Amount.String(),
		Sender:      eventData.Sender.Hex(),
		Recipient:   eventData.Recipient.Hex(),
		TxHash:      vlog.TxHash.Hex(),
	}, nil
}

// createMintTransaction generates an ExternalTransaction for minting tokens on the destination chain
func (p *ExtBlockProcessor) createMintTransaction(
	destChainID uint64,
	bridgeID string,
	sourceChainID uint64,
	tokenAddress string,
	amount string,
	recipientAddress string,
) (apptypes.ExternalTransaction, error) {
	payload, err := p.createBridgePayload(
		bridgeID,
		sourceChainID,
		tokenAddress,
		amount,
		recipientAddress,
	)
	if err != nil {
		return apptypes.ExternalTransaction{}, err
	}

	extTx, err := external.NewExTxBuilder(payload, apptypes.ChainType(destChainID)).Build()
	if err != nil {
		log.Error().
			Err(err).
			Uint64("destChain", destChainID).
			Msg("Failed to build external transaction")

		return apptypes.ExternalTransaction{}, err
	}

	return extTx, nil
}

// createBridgePayload creates the 160-byte payload for Bridge.executeTransaction()
// Layout: bridgeId(32) | sourceChainId(32) | token(32, left-aligned) | amount(32) | recipient(32, left-aligned)
func (p *ExtBlockProcessor) createBridgePayload(
	bridgeID string,
	sourceChainID uint64,
	tokenAddress string,
	amount string,
	recipientAddress string,
) ([]byte, error) {
	payload := make([]byte, 160)

	// bridgeId (bytes32)
	bridgeHash := common.HexToHash(bridgeID)
	copy(payload[0:32], bridgeHash[:])

	// sourceChainId (uint256)
	copy(payload[32:64], new(big.Int).SetUint64(sourceChainID).FillBytes(make([]byte, 32)))

	// token (address, LEFT-aligned) - map if needed
	token := common.HexToAddress(tokenAddress)
	if mapped, ok := p.tokenMappings[sourceChainID][token]; ok {
		token = mapped
	}

	copy(payload[64:84], token[:])

	// amount (uint256) - parse from string to support large values
	amountBig, ok := new(big.Int).SetString(amount, 10)
	if !ok {
		return nil, Error("invalid amount: " + amount)
	}

	copy(payload[96:128], amountBig.FillBytes(make([]byte, 32)))

	// recipient (address, LEFT-aligned)
	recipient := common.HexToAddress(recipientAddress)
	copy(payload[128:148], recipient[:])

	return payload, nil
}

// storeBridgeEvent stores a bridge event in the database
func storeBridgeEvent(dbtx kv.RwTx, bridgeEvent *BridgeInitiatedEvent) error {
	event := BridgeEvent{
		BridgeID:     bridgeEvent.BridgeID,
		SourceChain:  bridgeEvent.SourceChain,
		DestChain:    bridgeEvent.DestChain,
		Token:        bridgeEvent.Token,
		Amount:       bridgeEvent.Amount,
		Sender:       bridgeEvent.Sender,
		Recipient:    bridgeEvent.Recipient,
		Status:       BridgeStatusConfirmed,
		SourceTxHash: bridgeEvent.TxHash,
	}

	eventData, err := json.Marshal(event)
	if err != nil {
		return err
	}

	return dbtx.Put(BridgeEventsBucket, []byte(bridgeEvent.BridgeID), eventData)
}
