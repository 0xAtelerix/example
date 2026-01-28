# Pelagos Bridge

> A production-ready cross-chain bridge enabling seamless token transfers between L1 and L2 networks using the Pelagos SDK.

[![Status](https://img.shields.io/badge/status-production%20ready-brightgreen)]()
[![License](https://img.shields.io/badge/license-MIT-blue)]()

## Overview

Pelagos Bridge is a cross-chain bridge that enables fast, secure token transfers between Ethereum L1 (Sepolia) and custom L2 networks (Stavanger). Built on the Pelagos SDK, it provides a simple, efficient bridging solution with validator-based consensus.

### Key Features

- ✅ **Bidirectional bridging** - L1 ↔ L2 token transfers
- ✅ **One-transaction bridging** - EIP-2612 permit support (gasless approval)
- ✅ **Automatic token mapping** - Seamless ERC20 ↔ native token conversion
- ✅ **Fast finality** - No ZK proof generation, validator-based consensus
- ✅ **Production frontend** - Modern UI with MetaMask integration
- ✅ **Event-driven architecture** - Real-time bridge transaction processing

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Bridge Appchain                          │
│  • Monitors external chain events (pelacli)                 │
│  • Processes BridgeInitiated events                         │
│  • Generates ExternalTransactions for destination           │
│  • Coordinates validator consensus                          │
└─────────────────┬───────────────────────────────────────────┘
                  │
    ┌─────────────┼─────────────┬
    │             │             │
┌───▼────┐  ┌────▼───┐  ┌──────▼──┐
│Sepolia │  │Stavanger│  │ Future  │
│ L1     │  │  L2    │  │ Chains  │
│Bridge  │  │ Bridge │  │ ...     │
└────────┘  └────────┘  └─────────┘
```

### How It Works

1. **User initiates bridge** - Calls `bridgeAsset()` on source chain
2. **Token locked** - Tokens locked in source Bridge contract
3. **Event emitted** - `BridgeInitiated` event with bridge details
4. **Appchain processes** - Detects event, maps token addresses
5. **ExternalTransaction created** - Generates mint transaction for destination
6. **Destination mints** - Bridge contract releases/mints tokens to recipient

## Deployed Contracts

| Network | Chain ID | Bridge Contract | Explorer |
|---------|----------|----------------|----------|
| Sepolia (L1) | 11155111 | `0x844E740Ea7F404c6208fd85Ee6114a14F8037df7` | [Etherscan](https://sepolia.etherscan.io/address/0x844E740Ea7F404c6208fd85Ee6114a14F8037df7) |
| Stavanger (L2) | 50591822 | `0x3C1c8351a09DB0300786148B56EcB7be2FaA322e` | [Blockscout](https://explorer.stavanger.gateway.fm/address/0x3C1c8351a09DB0300786148B56EcB7be2FaA322e) |

## Quick Start

### Prerequisites

- Go 1.25+
- Docker & Docker Compose

### Running the Bridge

1. **Clone the repository**
   ```bash
   git clone <repository-url>
   cd bridge
   ```

2. **Configure environment**
   ```bash
   # Edit config files with your RPC keys and private keys
   vim config.yaml       # Appchain configuration
   vim pelacli.yaml      # Pelacli configuration (RPC URLs, private keys)
   ```

3. **Start the bridge stack**
   ```bash
   docker compose up -d
   ```
   OR
    ```bash
    make up
    ```

4. **Access the UI**
   - Frontend: http://localhost:3000
   - Explorer: http://localhost:3001

### Testing the Bridge

#### Using the Frontend

1. Open http://localhost:3000
2. Connect MetaMask
3. Select source and destination networks
4. Enter amount and recipient
5. Click "Bridge with Permit" (one transaction)



## Project Structure

```
bridge/
├── application/              # Core bridge logic
│   ├── external_block_processor.go  # Event processing & ExternalTransaction generation
│   ├── subscription.go       # Bridge contract subscriptions
│   ├── bridge_event.go       # Bridge event data types
│   ├── state.go              # Database query functions
│   └── api/                  # JSON-RPC API endpoints
├── cmd/                      # Application entry point
│   └── main.go
├── config.yaml               # Appchain configuration
├── pelacli.yaml              # Pelacli configuration
├── contracts/                # Solidity contracts
│   └── contracts/
│       └── Bridge.sol
└── frontend/                 # Web UI
    ├── index.html
    ├── app.js
    └── styles.css
```

## Token Mapping

The bridge automatically maps token addresses between chains:

| Source Chain | Source Token | Destination Chain | Destination Token |
|--------------|--------------|-------------------|-------------------|
| Sepolia (L1) | POL ERC20 (`0x6a7c...`) | Stavanger (L2) | Native POL (`address(0)`) |
| Stavanger (L2) | Native POL (`address(0)`) | Sepolia (L1) | POL ERC20 (`0x6a7c...`) |

Token mapping logic is configurable in `application/external_block_processor.go` via the `tokenMappings` map.

## Configuration

### Appchain Config (`config.yaml`)

Configures the bridge appchain.

```yaml
chain_id: 42

data_dir: "/data"

emitter_port: ":9090"

rpc_port: ":8080"

log_level: 1

required_chains:
  - 11155111  # Ethereum Sepolia
  - 50591822  # Stavanger testnet
```

### Pelacli Config (`pelacli.yaml`)

Configures pelacli for consensus, chain monitoring, and external transaction processing.

```yaml
data_dir: "/data"

consensus:
  ask_period: 1s

api:
  port: 8081

appchains:
  - chain_id: 42
    address: "appchain:9090"

# Chains to monitor for bridge events
read_chains:
  - chain_id: 50591822            # Stavanger testnet
    api_key: "wss://stavanger-rpc.eu-north-2.gateway.fm/ws"
    block_offset: 1

# Chains to send external transactions to
write_chains:
  - chain_id: 11155111            # Ethereum Sepolia
    rpc_url: "https://eth-sepolia.g.alchemy.com/v2/<api_key>"
    private_key: "<private_key>"
    pelagos_contract: "0x049FBea1295B569378Fe0D5AB965131743f332b9"

  - chain_id: 50591822            # Stavanger testnet
    rpc_url: "https://stavanger-rpc.eu-north-2.gateway.fm"
    private_key: "<private_key>"
    pelagos_contract: "0x416b560B03e6d9EF473bf57ccbb2A569AF5d0736"
```

⚠️ **Security:** Never commit real private keys. Use environment variables or secret management.

## API Endpoints

The bridge exposes a JSON-RPC endpoint for querying bridge state:

### Bridge Methods
- `getBridgeStatus(bridgeId)` - Get bridge status and tx hashes

Example:
```bash
curl -X POST http://localhost:8080/rpc \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "getBridgeStatus",
    "params": [{"bridgeId": "0x123..."}],
    "id": 1
  }'
```

## Development

### Building

```bash
# Build the appchain
make build

# Run tests
make tests

# Run with race detection
go test -race ./...

# Lint code
make lints
```

### Adding New Chains

1. Deploy Bridge.sol to the new chain
2. Add chain to `read_chains` in `pelacli.yaml`
3. Add chain to `write_chains` in `pelacli.yaml`
4. Add chain ID to `required_chains` in `config.yaml`
5. Update bridge contracts and token mappings in `external_block_processor.go`
6. Add subscription in `subscription.go`
7. Restart the bridge stack

### Adding New Tokens

1. Add token mapping in `application/external_block_processor.go` `tokenMappings` map
2. Update frontend token selector (if using frontend)
3. Restart the bridge appchain

## Security Considerations

### Current Implementation

- ✅ **ReentrancyGuard** - Protects against reentrancy attacks
- ✅ **Pausable** - Emergency pause functionality
- ✅ **Access control** - Only Pelagos contract can call `executeTransaction()`
- ✅ **Balance checks** - Validates sufficient liquidity before claims
- ✅ **Event validation** - Verifies event signatures and contract addresses

### Production Recommendations

- 🔒 **Multi-sig validator** - Replace single-key validator with multi-sig
- 🔒 **Rate limiting** - Add per-address bridge limits
- 🔒 **Amount caps** - Set maximum bridge amounts
- 🔒 **Circuit breakers** - Automatic pause on anomalies
- 🔒 **Monitoring** - Set up alerts for large bridges
- 🔒 **Audit** - Get smart contracts audited before mainnet

## Monitoring & Logs

### Bridge Appchain Logs

```bash
docker compose logs -f appchain
```

Key log messages:
- `Bridge event processed` - Event detected and processed
- `Processed EVM External block` - Block processing complete

### Pelacli Logs

```bash
docker compose logs -f pelacli
```

Key log messages:
- `Processing external block` - Monitoring external chains
- `Submitting external transaction` - Sending mint transaction

## Troubleshooting

### Bridge transaction not completing

1. Check pelacli is running: `docker compose ps`
2. Check RPC connectivity: Verify RPC URLs in `pelacli.yaml`
3. Check logs: `docker compose logs pelacli | grep -i error`
4. Verify liquidity: Destination bridge must have sufficient tokens

### Event not detected

1. Verify event signature in `external_block_processor.go` matches contract
2. Check bridge contract address in `external_block_processor.go` matches deployed contract
3. Verify pelacli is monitoring the correct chain in `pelacli.yaml`
4. Check `start_block` in `pelacli.yaml` is before the bridge transaction

### Frontend connection issues

1. Verify MetaMask is installed
2. Check correct network is selected
3. Verify RPC URLs in frontend `app.js`
4. Check browser console for errors

## License

MIT License - see LICENSE file for details
**Built with [Pelagos SDK](https://github.com/0xAtelerix/sdk) 🌊**
