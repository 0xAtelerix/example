package application

import (
	"context"
	"math/big"
	"strings"

	"github.com/0xAtelerix/sdk/gosdk"
	"github.com/0xAtelerix/sdk/gosdk/apptypes"
	"github.com/0xAtelerix/sdk/gosdk/evmtypes"
	"github.com/0xAtelerix/sdk/gosdk/external"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/holiman/uint256"
	"github.com/ledgerwatch/erigon-lib/kv"
	"github.com/rs/zerolog/log"
)

const (
	// ExampleContractAddress is the deployed Example contract address
	//
	// DEPLOYMENT INSTRUCTIONS:
	// 1. Navigate to the SDK contracts directory:
	//    cd /path/to/0xAtelerix/sdk/contracts
	// 2. Deploy the Example contract:
	//    cd scripts && ./deploy_example.sh
	// 3. Update this address with your deployed contract address
	// 4. Update signature or ABI if your contract events differ
	//
	// This is a demo address on Polygon-Amoy testnet.
	ExampleContractAddress = "0x8D350d5351A936Ef3e2907C0a438Fc941DAE3bfd"

	// Event signatures for the Example contract events
	// These correspond to events in 0xAtelerix/sdk/contracts/example/Example.sol
	// Deposit(address,string,uint256) event signature
	DepositEventSignature = "0x2d4b597935f3cd67fb2eebf1db4debc934cee5c7baa7153f980fdbeb2e74084e"
	// Swap(address,string,string,uint256) event signature
	SwapEventSignature = "0x363ba239c72b81c4726aba8829ad4df22628bf7d09efc5f7a18063a53ec1c4ba"
	// WithdrawToSolana(uint256) event signature
	// This event triggers a cross-chain transfer from EVM to Solana
	WithdrawToSolanaSignature = "0x245ecbfbddf346446b302f2dc8237ed1144f6f9407cb9708e2d0734458c72950"

	// ABI definitions for event decoding
	depositEventABI = `[{"anonymous":false,"inputs":[{"indexed":true,"internalType":"address",` +
		`"name":"user","type":"address"},{"indexed":false,"internalType":"string","name":"token",` +
		`"type":"string"},{"indexed":false,"internalType":"uint256","name":"amount","type":"uint256"}],` +
		`"name":"Deposit","type":"event"}]`

	swapEventABI = `[{"anonymous":false,"inputs":[{"indexed":true,"internalType":"address",` +
		`"name":"user","type":"address"},{"indexed":false,"internalType":"string","name":"tokenIn",` +
		`"type":"string"},{"indexed":false,"internalType":"string","name":"tokenOut","type":"string"},` +
		`{"indexed":false,"internalType":"uint256","name":"amountIn","type":"uint256"}],"name":"Swap","type":"event"}]`

	withdrawToSolanaABI = `[{"anonymous":false,"inputs":[` +
		`{"indexed":false,"internalType":"uint256","name":"amount","type":"uint256"}],` +
		`"name":"WithdrawToSolana","type":"event"}]`
)

// Verify ExtBlockProcessor implements ExternalBlockProcessor interface.
var _ gosdk.ExternalBlockProcessor = &ExtBlockProcessor{}

type ExtBlockProcessor struct {
	msa gosdk.MultichainStateAccessor
}

func NewExtBlockProcessor(msa gosdk.MultichainStateAccessor) *ExtBlockProcessor {
	return &ExtBlockProcessor{
		msa: msa,
	}
}

// ProcessBlock handles external chain blocks (EVM, Solana).
func (p *ExtBlockProcessor) ProcessBlock(
	b apptypes.ExternalBlock,
	tx kv.RwTx,
) ([]apptypes.ExternalTransaction, error) {
	switch {
	case gosdk.IsEvmChain(apptypes.ChainType(b.ChainID)):
		return p.processEVMBlock(b, tx)
	case gosdk.IsSolanaChain(apptypes.ChainType(b.ChainID)):
		return p.processSolanaBlock(b, tx)
	default:
		log.Warn().Uint64("chainID", b.ChainID).Msg("Unsupported external chain, skipping...")
	}

	return nil, nil
}

func (p *ExtBlockProcessor) processSolanaBlock(
	b apptypes.ExternalBlock,
	_ kv.RwTx,
) ([]apptypes.ExternalTransaction, error) {
	var externalTxs []apptypes.ExternalTransaction

	solBlock, err := p.msa.SolanaBlock(context.Background(), b)
	if err != nil {
		return nil, err
	}

	log.Info().
		Uint64("chainID", b.ChainID).
		Uint64("slotNumber", b.BlockNumber).
		Int("transactions", len(solBlock.Transactions)).
		Msg("Solana External block")

	return externalTxs, nil
}

func (p *ExtBlockProcessor) processEVMBlock(
	b apptypes.ExternalBlock,
	dbtx kv.RwTx,
) ([]apptypes.ExternalTransaction, error) {
	var externalTxs []apptypes.ExternalTransaction

	block, err := p.msa.EVMBlock(context.Background(), b)
	if err != nil {
		return nil, err
	}

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

	log.Info().
		Uint64("chainID", b.ChainID).
		Uint64("blockNumber", b.BlockNumber).
		Int("transactions", len(block.Body.Transactions)).
		Int("receipts", len(receipts)).
		Msg("EVM External block")

	return externalTxs, nil
}

// processReceipt handles Deposit events from the external chain
// Just for example, In real use-case, handle according to your logic
func (*ExtBlockProcessor) processReceipt(
	tx kv.RwTx,
	r evmtypes.Receipt,
	chainID uint64,
) []apptypes.ExternalTransaction {
	var externalTxs []apptypes.ExternalTransaction

	for _, vlog := range r.Logs {
		// Check if this log is from our example contract
		if vlog.Address == common.HexToAddress(ExampleContractAddress) && len(vlog.Topics) > 0 {
			switch vlog.Topics[0].Hex() {
			case DepositEventSignature:
				// Decode deposit event using ABI
				token, amount, err := decodeDepositEvent(vlog)
				if err != nil {
					log.Error().Err(err).Msg("Failed to decode deposit event")

					continue
				}

				// Extract user address from topics[1] (indexed parameter)
				userAddr := common.HexToAddress(vlog.Topics[1].Hex())
				user := userAddr.Hex()

				// Convert to uint256 for storage
				amountUint256, overflow := uint256.FromBig(amount)
				if overflow {
					log.Error().Str("amount", amount.String()).Msg("Deposit amount too large")

					continue
				}

				// Update user balance in appchain
				accountKey := AccountKey(user, token)

				// Get current balance
				currentBalanceData, err := tx.GetOne(AccountsBucket, accountKey)
				if err != nil {
					log.Error().Err(err).Msg("Failed to get current balance")

					continue
				}

				currentBalance := uint256.NewInt(0)
				if len(currentBalanceData) > 0 {
					currentBalance.SetBytes(currentBalanceData)
				}

				// Add deposited amount
				newBalance := uint256.NewInt(0).Add(currentBalance, amountUint256)

				// Store new balance
				balanceBytes := newBalance.Bytes()
				if err := tx.Put(AccountsBucket, accountKey, balanceBytes); err != nil {
					log.Error().Err(err).Msg("Failed to update balance")

					continue
				}

				log.Info().
					Uint64("chainID", chainID).
					Str("user", userAddr.Hex()).
					Str("token", token).
					Str("amount", amount.String()).
					Str("new_balance", newBalance.String()).
					Msg("Processed deposit from external chain")

			case SwapEventSignature:
				// Decode swap event using ABI
				tokenIn, tokenOut, amountIn, err := decodeSwapEvent(vlog)
				if err != nil {
					log.Error().Err(err).Msg("Failed to decode swap event")

					continue
				}

				userAddr := common.HexToAddress(vlog.Topics[1].Hex())

				// Calculate output amount using fixed exchange rate
				amountOut := calculateSwapOutput(tokenIn, tokenOut, amountIn)

				// Create an external transaction record for the destination chain (EVM)
				extTx, err := external.NewExTxBuilder(
					createTokenMintPayload(userAddr, amountOut, tokenOut),
					gosdk.EthereumSepoliaChainID).
					Build()
				if err != nil {
					log.Error().Err(err).Msg("Failed to create external transaction for swap event")

					continue
				}

				externalTxs = append(externalTxs, extTx)

				log.Info().
					Uint64("source_chainID", chainID).
					Str("user", userAddr.Hex()).
					Str("tokenIn", tokenIn).
					Str("tokenOut", tokenOut).
					Str("amountIn", amountIn.String()).
					Str("amountOut", amountOut.String()).
					Uint64("target_chainID", uint64(gosdk.EthereumSepoliaChainID)).
					Msg("Processed swap event - EVM to EVM")

			case WithdrawToSolanaSignature:
				// Decode withdraw to Solana event using ABI
				amount, err := decodeWithdrawToSolanaEvent(vlog)
				if err != nil {
					log.Error().Err(err).Msg("Failed to decode WithdrawToSolana event")

					continue
				}

				// Create Solana mint payload for cross-chain transfer
				extTx, err := createSolanaMintPayload(amount.Uint64())
				if err != nil {
					log.Error().Err(err).Msg("Failed to create Solana mint payload")

					continue
				}

				externalTxs = append(externalTxs, extTx)

				log.Info().
					Uint64("source_chainID", chainID).
					Str("amount", amount.String()).
					Uint64("target_chainID", uint64(gosdk.SolanaDevnetChainID)).
					Msg("Processed withdraw to Solana event - EVM to Solana withdraw")

			default:
				log.Info().Msgf("Unhandled event signature: %s", vlog.Topics[0].Hex())
			}
		}
	}

	return externalTxs
}

// calculateSwapOutput calculates the output amount for a token swap using fixed exchange rates
func calculateSwapOutput(tokenIn, tokenOut string, amountIn *big.Int) *big.Int {
	// Fixed exchange rates for token pairs (tokenIn:tokenOut -> rate)
	// Rate represents how many tokenOut you get for 1 tokenIn
	exchangeRates := map[string]float64{
		"ETH:USDT": 4200.0,
		"USDT:ETH": 1.0 / 4200.0,
		"BTC:USDT": 60000.0,
		"USDT:BTC": 1.0 / 60000.0,
	}

	pair := tokenIn + ":" + tokenOut

	rate, exists := exchangeRates[pair]
	if !exists {
		log.Warn().Str("pair", pair).Msg("Exchange rate not found, using 1:1 rate")

		return amountIn // Default to 1:1 if rate not found
	}

	// Convert amountIn to float64 for calculation
	amountInFloat := new(big.Float).SetInt(amountIn)
	rateFloat := new(big.Float).SetFloat64(rate)

	// Calculate output amount
	outputFloat := new(big.Float).Mul(amountInFloat, rateFloat)

	// Convert back to big.Int (round down)
	outputInt := new(big.Int)
	outputFloat.Int(outputInt)

	return outputInt
}

// createTokenMintPayload creates a payload for the AppChain contract
// This matches the demo contracts in 0xAtelerix/sdk/contracts/pelacli/AppChain.sol
// Payload format: [recipient:20bytes][amount:32bytes][tokenName:variable]
// The AppChain contract will mint these tokens to the recipient address
func createTokenMintPayload(recipient common.Address, amount *big.Int, token string) []byte {
	payload := make([]byte, 20+32+len(token))
	copy(payload[0:20], recipient.Bytes())
	amountBytes := amount.Bytes()
	copy(payload[52-len(amountBytes):52], amountBytes)
	copy(payload[52:], []byte(token))

	return payload
}

// decodeDepositEvent decodes a Deposit event using ABI
func decodeDepositEvent(vlog *types.Log) (string, *big.Int, error) {
	// Parse the ABI
	parsedABI, err := abi.JSON(strings.NewReader(depositEventABI))
	if err != nil {
		return "", nil, err
	}

	// Unpack the event data (non-indexed parameters)
	var depositEvent struct {
		Token  string
		Amount *big.Int
	}

	err = parsedABI.UnpackIntoInterface(&depositEvent, "Deposit", vlog.Data)
	if err != nil {
		return "", nil, err
	}

	return depositEvent.Token, depositEvent.Amount, nil
}

// decodeSwapEvent decodes a Swap event using ABI
func decodeSwapEvent(vlog *types.Log) (tokenIn, tokenOut string, amountIn *big.Int, err error) {
	// Parse the ABI
	parsedABI, err := abi.JSON(strings.NewReader(swapEventABI))
	if err != nil {
		return "", "", nil, err
	}

	// Unpack the event data (non-indexed parameters)
	var swapEvent struct {
		TokenIn  string
		TokenOut string
		AmountIn *big.Int
	}

	err = parsedABI.UnpackIntoInterface(&swapEvent, "Swap", vlog.Data)
	if err != nil {
		return "", "", nil, err
	}

	tokenIn = swapEvent.TokenIn
	tokenOut = swapEvent.TokenOut
	amountIn = swapEvent.AmountIn

	return
}

// decodeWithdrawToSolanaEvent decodes a WithdrawToSolana event using ABI
func decodeWithdrawToSolanaEvent(
	vlog *types.Log,
) (amount *big.Int, err error) {
	// Parse the ABI
	parsedABI, err := abi.JSON(strings.NewReader(withdrawToSolanaABI))
	if err != nil {
		return nil, err
	}

	// Unpack the event data (non-indexed parameters)
	var withdrawEvent struct {
		Amount *big.Int
	}

	err = parsedABI.UnpackIntoInterface(&withdrawEvent, "WithdrawToSolana", vlog.Data)
	if err != nil {
		return nil, err
	}

	amount = withdrawEvent.Amount

	return
}
