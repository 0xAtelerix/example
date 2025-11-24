package application

import (
	"encoding/hex"
	"encoding/json"
	"math/big"
	"strings"

	"github.com/0xAtelerix/sdk/gosdk"
	"github.com/0xAtelerix/sdk/gosdk/apptypes"
	"github.com/0xAtelerix/sdk/gosdk/external"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/holiman/uint256"
	"github.com/ledgerwatch/erigon-lib/kv"
)

var _ apptypes.AppTransaction[Receipt] = &Transaction[Receipt]{}

func AccountKey(sender string, token string) []byte {
	return []byte(token + sender)
}

type Transaction[R Receipt] struct {
	Sender         string `json:"sender"`
	Value          uint64 `json:"value"`
	Receiver       string `json:"receiver"`
	Token          string `json:"token"`
	TxHash         string `json:"hash"`
	GenerateExtTxn bool   `json:"generate_ext_txn,omitempty"` // When true, generates an external transaction to Sepolia
}

func (e *Transaction[R]) Unmarshal(b []byte) error {
	return json.Unmarshal(b, e)
}

func (e Transaction[R]) Marshal() ([]byte, error) {
	return json.Marshal(e)
}

func (e Transaction[R]) Hash() [32]byte {
	// Remove all hex prefixes (handles cases like "0x0x..." or "0X0x...")
	txHash := e.TxHash
	for strings.HasPrefix(strings.ToLower(txHash), "0x") {
		txHash = txHash[2:]
	}

	hashBytes, err := hex.DecodeString(txHash)
	if err != nil {
		panic(err)
	}

	var h [32]byte
	copy(h[:], hashBytes)

	return h
}

func (e Transaction[R]) Process(
	dbTx kv.RwTx,
) (res R, txs []apptypes.ExternalTransaction, err error) {
	var senderBalanceData []byte

	senderTokenKey := AccountKey(e.Sender, e.Token)

	senderBalanceData, err = dbTx.GetOne(AccountsBucket, senderTokenKey)
	if err != nil {
		return e.failedReceipt(err), nil, nil
	}

	if len(senderBalanceData) == 0 {
		return e.failedReceipt(ErrNotEnoughBalance), nil, nil
	}

	senderBalance := &uint256.Int{}
	senderBalance.SetBytes(senderBalanceData)

	if senderBalance.CmpUint64(e.Value) < 0 {
		return e.failedReceipt(ErrNotEnoughBalance), nil, nil
	}

	var receiverBalanceData []byte

	receiverTokenKey := AccountKey(e.Receiver, e.Token)

	receiverBalanceData, err = dbTx.GetOne(AccountsBucket, receiverTokenKey)
	if err != nil {
		return e.failedReceipt(err), nil, nil
	}

	receiverBalance := &uint256.Int{}
	receiverBalance.SetBytes(receiverBalanceData)

	amount := uint256.NewInt(e.Value)

	receiverBalance.Add(receiverBalance, amount)
	senderBalance.Sub(senderBalance, amount)

	err = dbTx.Put(AccountsBucket, senderTokenKey, senderBalance.Bytes())
	if err != nil {
		return e.failedReceipt(err), nil, nil
	}

	err = dbTx.Put(AccountsBucket, receiverTokenKey, receiverBalance.Bytes())
	if err != nil {
		return e.failedReceipt(err), nil, nil
	}

	res = e.successReceipt(senderBalance, receiverBalance)

	// Generate external transaction if requested
	var externalTxs []apptypes.ExternalTransaction
	if e.GenerateExtTxn {
		extTx, err := e.createExternalTransaction()
		if err != nil {
			// Log error but don't fail the transaction
			return e.failedReceipt(err), nil, nil
		}
		externalTxs = append(externalTxs, extTx)
	}

	return res, externalTxs, nil
}

func (e *Transaction[R]) failedReceipt(err error) R {
	return R{
		TxnHash:      e.Hash(),
		Sender:       e.Sender,
		Receiver:     e.Receiver,
		Token:        e.Token,
		Value:        e.Value,
		ErrorMessage: err.Error(),
		TxStatus:     apptypes.ReceiptFailed,
	}
}

func (e *Transaction[R]) successReceipt(senderBalance, receiverBalance *uint256.Int) R {
	return R{
		TxnHash:         e.Hash(),
		Sender:          e.Sender,
		SenderBalance:   senderBalance,
		Receiver:        e.Receiver,
		ReceiverBalance: receiverBalance,
		Token:           e.Token,
		Value:           e.Value,
		TxStatus:        apptypes.ReceiptConfirmed,
	}
}

// createExternalTransaction creates an external transaction to Sepolia
// with a payload that matches the AppChain contract format
func (e *Transaction[R]) createExternalTransaction() (apptypes.ExternalTransaction, error) {
	// Parse receiver as Ethereum address
	// If it's not a valid address, create a deterministic address from the string
	var recipientAddr common.Address
	if common.IsHexAddress(e.Receiver) {
		recipientAddr = common.HexToAddress(e.Receiver)
	} else {
		// For non-address receivers (like "alice", "bob"), create a deterministic address
		// by hashing the name using Keccak256 and taking the last 20 bytes
		hash := crypto.Keccak256Hash([]byte(e.Receiver))
		recipientAddr = common.BytesToAddress(hash.Bytes())
	}

	// Convert value to big.Int
	amount := big.NewInt(int64(e.Value))

	// Create payload matching AppChain contract format using the existing function
	// Format: [recipient:20bytes][amount:32bytes][tokenName:variable]
	// This matches the contract in 0xAtelerix/sdk/contracts/pelacli/AppChain.sol
	payload := createTokenMintPayload(recipientAddr, amount, e.Token)

	// Create external transaction targeting Sepolia
	extTx, err := external.NewExTxBuilder(payload, gosdk.EthereumSepoliaChainID).Build()
	if err != nil {
		return apptypes.ExternalTransaction{}, err
	}

	return extTx, nil
}
