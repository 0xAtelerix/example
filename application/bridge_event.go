package application

// BridgeEvent represents a cross-chain bridge event from external chains.
type BridgeEvent struct {
	BridgeID     string `json:"bridgeId"`               // Unique bridge identifier
	SourceChain  uint64 `json:"sourceChain"`            // Source chain ID
	DestChain    uint64 `json:"destChain"`              // Destination chain ID
	Token        string `json:"token"`                  // Token address (address(0) for native)
	Amount       string `json:"amount"`                 // Amount as string (supports > uint64)
	Sender       string `json:"sender"`                 // Original sender address
	Recipient    string `json:"recipient"`              // Recipient on destination chain
	Status       string `json:"status"`                 // Confirmed or Completed
	SourceTxHash string `json:"sourceTxHash,omitempty"` // Source chain tx hash
	ClaimTxHash  string `json:"claimTxHash,omitempty"`  // Destination chain tx hash
	ConfirmedAt  int64  `json:"confirmedAt,omitempty"`  // Unix timestamp when event was confirmed
	RetryCount   int    `json:"retryCount,omitempty"`   // Number of retry attempts
}

// BridgeEvent status constants
const (
	BridgeStatusConfirmed = "Confirmed" // Event received and external tx generated
	BridgeStatusCompleted = "Completed" // Claimed on destination chain
	BridgeStatusFailed    = "Failed"    // Max retries exhausted
)
