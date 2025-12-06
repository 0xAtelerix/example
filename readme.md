# Pelagos Appchain — Build an Appchain with the Go SDK

> This repo is a **skeleton** for building your own appchain on top of the Pelagos Go SDK.
> It comes with a runnable **docker-compose** stack that includes both your appchain node and a **consensus** (`pelacli`) that simulates consensus in your local environment and feeds external chain data.

## Table of Contents

- [What you get out of the box](#what-you-get-out-of-the-box)
- [Key concepts & execution model](#key-concepts--execution-model)
- [Project layout](#project-layout)
- [Docker — compose stack](#docker--compose-stack)
- [Zero-Config Setup](#zero-config-setup)
- [Custom Configuration (Optional)](#custom-configuration-optional)
- [Build & Run](#build--run)
- [JSON-RPC quickstart](#json-rpc-quickstart)
- [Code walkthrough (where to extend)](#code-walkthrough-where-to-extend)
- [Flags (quick reference)](#flags-quick-reference)
- [Additional Resources](#additional-resources)

## What you get out of the box

* A minimal **block type** (`Block`) that satisfies `apptypes.AppchainBlock` from the SDK.
* A **transaction** (`Transaction`) and **receipt** (`Receipt`) implementing a simple token transfer with balances in MDBX.
* A **stateless external-block adapter** (`StateTransition`) that shows how to fetch/inspect Ethereum/Solana data via `MultichainStateAccess`.
* **Cross-chain transaction support** via `pelacli` external transaction configuration for sending transactions to external networks.
* **Genesis seeding** to fund demo users (`alice`, `bob`, …) with USDT/BTC/ETH balances.
* **Buckets** (tables) for app state (`appaccounts`), receipts, blocks, checkpoints, etc.
* A runnable `main.go` that wires the SDK, DBs, tx-pool, validator set, the appchain loop, and default **JSON-RPC**.
* One **custom JSON-RPC** (`getBalance`) + **standard** ones (`sendTransaction`, `getTransactionStatus`, `getTransactionReceipt`, …).
* A **docker-compose** that runs the node together with `pelacli` so your txs actually progress.

## Key concepts & execution model

1. **Clients** submit transactions via JSON-RPC (`sendTransaction`) into the **tx pool**.
2. The **consensus** (`pelacli`) periodically:

    * pulls txs via the appchain's **gRPC emitter API** (`CreateInternalTransactionsBatch`),
    * writes **tx-batches** to an MDBX database (`txbatch`),
    * appends **events** (referencing those batches + external blocks) to the event stream file.
3. The appchain run loop consumes **events + tx-batches**, executes your `Transaction.Process` inside a DB write transaction, persists **receipts**, builds a **block**, and writes a **checkpoint**.
4. JSON-RPC exposes tx **status**, **receipts**, and your **custom methods**.

> Status lifecycle: **Pending** (in tx pool) → **Batched** (pulled by pelacli) → **Processed/Failed** (after your txn processing logic runs in a block).

* **Transactions don't auto-finalize.**
  Without the **consensus** you'll only ever see `Pending`. This compose includes `pelacli` to move them forward.

* **The appchain waits for real data sources.**
  It blocks until both exist:

  * the **event file**: `<stream-dir>/epoch_1.data`,
  * the **tx-batch MDBX**: `<tx-dir>` with the `txbatch` table.
    `pelacli` creates and updates both.

* **Multichain access uses local SQLite**
  The SDK reads external chain state from **SQLite databases** on disk. `pelacli` populates and updates them automatically. By default, Sepolia and Polygon Amoy are enabled. Add additional networks via `read_chains` in `pelacli.yaml` (see [`pelacli.example.yaml`](./pelacli.example.yaml)).

* **Cross-chain transaction flow**
  - **Read**: `pelacli` fetches data from external chains → stores in SQLite → appchain reads via `MultichainStateAccess`
  - **Write**: appchain generates `ExternalTransaction` → `pelacli` sends to Pelagos contract → Pelagos routes to specific AppChain contract based on appchainID
  - **Custom contracts**: Deploy your own AppChain contracts on external chains using the contracts in the [SDK contracts folder](https://github.com/0xAtelerix/sdk/tree/main/contracts) for more advanced cross-chain interactions


## Project layout

```
.
├─ application/
│  ├─ block.go                # Block type + constructor
│  ├─ buckets.go              # App buckets (tables)
│  ├─ errors.go               # App-level errors
│  ├─ genesis.go              # One-time state seeding (demo balances)
│  ├─ receipt.go              # Receipt type
│  ├─ state_transition.go     # External-chain ingestion (stateless)
│  ├─ transaction.go          # Business logic (transfers)
│  └─ api/
│     ├─ api.go               # Custom JSON-RPC methods (getBalance)
│     └─ middleware.go        # CORS and other middleware
├─ cmd/
│  ├─ config.go               # Config struct and YAML loading
│  ├─ main.go                 # Wiring & run loop (the app binary)
│  └─ main_test.go            # End-to-end integration test
├─ data/                      # Shared volume (created at runtime)
├─ config.example.yaml        # Example appchain config
├─ pelacli.example.yaml       # Example pelacli config
├─ Dockerfile                 # Dockerfile for the appchain node
├─ docker-compose.yml         # Compose for appchain + pelacli
└─ test_txns.sh               # Test script for sending transactions
```


## Docker — compose stack

This compose runs **both** your appchain and the **pelacli** streamer with **zero configuration required**.

### `docker-compose.yml`

```yaml
services:
  pelacli:
    container_name: pelacli
    image: pelagosnetwork/pelacli:latest
    working_dir: /app
    volumes:
      - ./data:/app/data
    command:
      - consensus
    # Zero-config: uses SDK defaults
    # - appchain: 42 @ :9090
    # - chains: Sepolia, Polygon Amoy
    # - paths: ./data/events, ./data/multichain

  appchain:
    build:
      context: .
      dockerfile: Dockerfile
    working_dir: /app
    pid: "service:pelacli"
    image: appchain:latest
    volumes:
      - ./data:/app/data
    ports:
      - "9090:9090"
      - "8080:8080"
    depends_on:
      - pelacli
    # Zero-config: uses SDK defaults
    # - chain_id: 42
    # - emitter: :9090
    # - rpc: :8080
    # - data: ./data
```

**Data directory structure**

Both containers share the same `./data` volume:

```
./data/
├── multichain/           # External chain data (written by pelacli, read by appchain)
│   ├── 11155111/         # Ethereum Sepolia
│   │   └── sqlite
│   └── 80002/            # Polygon Amoy
│       └── sqlite
├── events/               # Consensus events (written by pelacli, read by appchain)
│   └── epoch_1.data
├── fetcher/              # Transaction batches (written by pelacli, read by appchain)
│   └── 42/               # Per-appchain (chainID=42)
│       └── mdbx.dat
├── appchain/             # Appchain state (written by appchain)
│   └── 42/
│       └── mdbx.dat
└── local/                # Local node data like txpool (written by appchain)
    └── 42/
        └── mdbx.dat
```

* **pelacli** writes: `events/`, `fetcher/`, `multichain/`
* **appchain** writes: `appchain/`, `local/`
* **appchain** reads: `events/`, `fetcher/`, `multichain/`

> Keep **ChainID=42** consistent across your code and pelacli config (both default to 42).


## Zero-Config Setup

The default setup requires **no configuration files**. Pelacli comes with built-in testnet defaults:

| Network | Chain ID | Pelagos Contract |
|---------|----------|------------------|
| Sepolia | 11155111 | `0x922e02fFbDe8ABbF3058ccC3f9433018A2ff8C1d` |
| Polygon Amoy | 80002 | `0x98D34a83c45FEae289f6FA72ba009Ad49F3D26ED` |

Just clone and run:

```bash
git clone https://github.com/0xAtelerix/example
cd example
docker compose up -d
```


## Custom Configuration (Optional)

For production or custom setups, copy the example config files and modify as needed.

### Appchain Configuration

```bash
cp config.example.yaml config.yaml
# Edit config.yaml with your settings
./appchain -config=config.yaml
```

Available settings in `config.yaml`:

| Field | Default | Description |
|-------|---------|-------------|
| `chain_id` | 42 | Appchain identifier (must match pelacli) |
| `data_dir` | `./data` | Root data directory shared with pelacli |
| `emitter_port` | `:9090` | gRPC port for emitter API |
| `rpc_port` | `:8080` | HTTP port for JSON-RPC server |
| `log_level` | 1 | Log verbosity: 0=debug, 1=info, 2=warn, 3=error |

See [`config.example.yaml`](./config.example.yaml) for the complete example.

### Pelacli Configuration

```bash
cp pelacli.example.yaml pelacli.yaml
# Edit pelacli.yaml with your settings
pelacli consensus --config=pelacli.yaml
```

Key configuration sections:

| Section | Default | Description |
|---------|---------|-------------|
| `data_dir` | `./data` | Root data directory (must match appchain's `data_dir`) |
| `consensus` | - | Polling interval and fetcher settings |
| `api` | - | External transactions REST API settings |
| `avail_da` | - | Avail DA integration (optional) |
| `appchains` | `42@:9090` | Which appchains to connect to |
| `read_chains` | Sepolia, Amoy | WSS endpoints for reading external blocks |
| `write_chains` | Sepolia, Amoy | RPC endpoints for sending cross-chain transactions |

**Block offset values** (for `read_chains`):
- `-4` = safe (2 blocks behind latest)
- `-3` = finalized (most secure, ~15 min delay on Ethereum)
- `-2` = latest (no delay, default for testnets)
- `1`, `2`, etc. = fixed offset from latest (e.g., `2` means if latest is block X, you get block X-2)

See [`pelacli.example.yaml`](./pelacli.example.yaml) for all available options with detailed comments.


## Build & Run

1. **Start (zero-config):**

   ```bash
   docker compose up -d
   ```

2. **Check health:**

   ```bash
   curl -s http://localhost:8080/health | jq .
   ```

3. **Tail logs:**

   ```bash
   docker compose logs -f pelacli
   docker compose logs -f appchain
   ```

4. **Test:**

   ```bash
   ./test_txns.sh
   ```

> On the first run, pelacli will populate SQLite databases and start producing events/tx-batches. Your appchain waits until the event file and tx-batch DB exist, then begins processing.

---

## JSON-RPC quickstart

### Send a transfer

```bash
TX_HASH=0x$(date +%s%N | sha256sum | awk '{print $1}')

curl -s http://localhost:8080/rpc \
  -H 'Content-Type: application/json' \
  -d '{
    "jsonrpc":"2.0",
    "method":"sendTransaction",
    "params":[{"sender":"alice","receiver":"bob","value":1000,"token":"USDT","hash":"'"$TX_HASH"'"}],
    "id":1
  }' | jq
```

### Check status

```bash
curl -s http://localhost:8080/rpc \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","method":"getTransactionStatus","params":["'"$TX_HASH"'"],"id":2}' | jq
```

### Get receipt (after Processed/Failed)

```bash
curl -s http://localhost:8080/rpc \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","method":"getTransactionReceipt","params":["'"$TX_HASH"'"],"id":3}' | jq
```

### Custom method: balance

```bash
curl -s http://localhost:8080/rpc \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","method":"getBalance","params":[{"user":"alice","token":"USDT"}],"id":4}' | jq
```

> Demo balances are seeded on first start by `InitializeGenesis`.


## Code walkthrough (where to extend)

* **`application/transaction.go` → `Process`**
  Your business logic lives here (validation, state writes, receipts).
  Return `[]ExternalTransaction` if you want to emit cross-chain transactions from your appchain to external blockchains.

* **`application/state_transition.go` → `ProcessBlock`**
  Turn **external blocks/receipts** (fetched via `MultichainStateAccess`) into internal transactions that your appchain will execute (e.g., processing deposits from external chains). Keep this layer **stateless**; all state changes happen in `Transaction.Process`.

* **`application/block.go` → `BlockConstructor`**
  Builds per-block artifacts. Currently uses a **stub** state root; replace `StubRootCalculator` with your own when ready.

* **`application/buckets.go`**
  Add your own tables and merge them with `gosdk.DefaultTables()` in `main.go`.

* **`application/api/api.go`**
  Add read-only custom JSON-RPC methods for your UI.

* **`application/api/middleware.go`** (Optional)
  Configure Auth, Logging, and HTTP middleware for your JSON-RPC server.


## Flags (quick reference)

The appchain uses SDK defaults. Override with config file:

* `-config=config.yaml` — Path to YAML config file (optional)

All settings can be configured in the YAML file:
* `chain_id` — Appchain identifier (default: 42)
* `data_dir` — Root data directory (default: `./data`)
* `emitter_port` — gRPC emitter port (default: `:9090`)
* `rpc_port` — JSON-RPC server port (default: `:8080`)
* `log_level` — Log verbosity 0-3 (default: 1=info)

## Additional Resources

- [Pelagos SDK](https://github.com/0xAtelerix/sdk) — Core SDK documentation and source code
- [SDK Contracts](https://github.com/0xAtelerix/sdk/tree/main/contracts) — Deploy your own AppChain contracts on external chains for custom cross-chain functionality

---
**Happy building!** For questions or issues, check the [issues](https://github.com/0xAtelerix/example/issues) or reach out to the Pelagos community.
