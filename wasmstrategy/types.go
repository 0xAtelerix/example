package wasmstrategy

import "github.com/0xAtelerix/sdk/gosdk/apptypes"

// EventKind mirrors AssemblyScript's enum for events surfaced to strategies.
type EventKind int32

const (
	EventKindUnknown       EventKind = 0
	EventKindERC20Transfer EventKind = 1
)

// AddressID classifies contracts and venues that emit events.
type AddressID int32

const (
	AddressIDUnknown       AddressID = 0
	AddressIDUniswapV2Pair AddressID = 1
	AddressIDUniswapV3Pool AddressID = 2
)

// StrategyEvent captures the minimal event metadata sent to WASM.
type StrategyEvent struct {
	Kind   EventKind
	Target AddressID
}

// BlockContext is stored inside the host functions before invoking WASM.
type BlockContext struct {
	BlockNumber uint64
	ChainID     apptypes.ChainType
	Events      []StrategyEvent
}

const (
	// SlotLastUniTransferBlock mirrors SLOT_LAST_UNI_TRANSFER_BLOCK in AssemblyScript.
	SlotLastUniTransferBlock int32 = 1
)

// erc20TransferTopic is the Keccak hash of Transfer(address,address,uint256).
const erc20TransferTopic = "0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"
