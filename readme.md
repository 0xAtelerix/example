# Pelagos Appchain — Build an Appchain with the Go SDK

> This repo is a **skeleton** for building your own appchain on top of the Pelagos Go SDK.
> It comes with a runnable **docker-compose** stack that includes both your appchain node and a **consensus** (`pelacli`) that simulates consensus in your local environment and feeds external chain data.

## Table of Contents

- [What you get out of the box](#what-you-get-out-of-the-box)
- [Key concepts & execution model](#key-concepts--execution-model)
- [Project layout](#project-layout)
- [Docker — compose stack](#docker--compose-stack)
- [Configuration files](#configuration-files)
  - [consensus_chains.json](#configconsensus_chainsjson-used-by-pelacli-for-reading-from-external-chains)
  - [chain_data.json](#configchain_datajson-used-by-appchain-for-reading-external-chain-data)
  - [ext_networks.json](#configext_networksjson-used-by-pelacli-for-writing-to-external-chains)
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

    * pulls txs via the appchain’s **gRPC emitter API** (`CreateInternalTransactionsBatch`),
    * writes **tx-batches** to an MDBX database (`txbatch`),
    * appends **events** (referencing those batches + external blocks) to the event stream file.
3. The appchain run loop consumes **events + tx-batches**, executes your `Transaction.Process` inside a DB write transaction, persists **receipts**, builds a **block**, and writes a **checkpoint**.
4. JSON-RPC exposes tx **status**, **receipts**, and your **custom methods**.

> Status lifecycle: **Pending** (in tx pool) → **Batched** (pulled by pelacli) → **Processed/Failed** (after your txn processing logic runs in a block).

* **Transactions don’t auto-finalize.**
  Without the **consensus** you’ll only ever see `Pending`. This compose includes `pelacli` to move them forward.

* **The appchain waits for real data sources.**
  It blocks until both exist:

  * the **event file**: `<stream-dir>/epoch_1.data`,
  * the **tx-batch MDBX**: `<tx-dir>` with the `txbatch` table.
    `pelacli` creates and updates both.

* **Multichain access uses local MDBX**
  The SDK reads external chain state from **MDBX databases** on disk. `pelacli` populates and updates them using your RPC **API key**.

* **Cross-chain transaction flow**
  - **Read**: `pelacli` fetches data from external chains → stores in MDBX → appchain reads via `MultichainStateAccess`
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
│  └─ main.go                 # Wiring & run loop (the app binary)
├─ config/
│  ├─ chain_data.json         # Chain ID → MDBX path mapping (appchain reads)
│  ├─ consensus_chains.json   # External chains to fetch data from (pelacli writes)
│  └─ ext_networks.json       # External chains to send txns to (pelacli sends txns)
├─ Dockerfile                 # Dockerfile for the appchain node
├─ docker-compose.yml         # Compose for appchain + pelacli
└─ test_txns.sh               # Test script for sending transactions and checking things are working
```


## Docker — compose stack

This compose runs **both** your appchain and the **pelacli** streamer.

### `docker-compose.yml`

```yaml
services:
  appchain:
    build:
      context: .
      dockerfile: Dockerfile
    pid: "service:pelacli"
    image: appchain:latest
    volumes:
      - ./pelacli_data:/consensus_data
      - ./app_data:/data
      - ./config/chain_data.json:/data/chain_data.json:ro
      - ./multichain:/multichain
    ports:
      - "9090:9090"
      - "8080:8080"
    depends_on:
      - pelacli
    command:
      - --emitter-port=:9090
      - --db-path=/data/appchain-db
      - --local-db-path=/data/local-db
      - --stream-dir=/consensus_data/events
      - --tx-dir=/consensus_data/fetcher/snapshots/42
      - --rpc-port=:8080
      - --multichain-config=/data/chain_data.json

  pelacli:
    container_name: pelacli
    image: pelagosnetwork/pelacli:latest
    volumes:
      - ./pelacli_data:/consensus_data
      - ./config/consensus_chains.json:/consensus_chains.json:ro
      - ./config/ext_networks.json:/ext_networks.json:ro
      - ./multichain:/multichain
    command:
      - consensus
      - --snapshot-dir=/consensus_data
      - --appchain=42=appchain:9090
      - --ask-period=1s
      - --multichain-dir=/consensus_data/multichain_db
      - --chains-json=/consensus_chains.json
      - --ext-txn-config-json=/ext_networks.json
```

**What the paths mean**

* Appchain:

    * `--stream-dir=/consensus_data/events` → pelacli writes `epoch_1.data` here.
    * `--tx-dir=/consensus_data/fetcher/snapshots/42` → pelacli writes the read-only MDBX with `txbatch` table here.
    * `--multichain-config=/data/chain_data.json` → maps chain IDs to MDBX DBs for external access.

* pelacli:

    * `--chains-json=/consensus_chains.json` → tells pelacli which L1/L2s to fetch (Sepolia in the example).
    * `--ext-txn-config-json=/ext_networks.json` → configures external chains that pelacli can send transactions to.
    * `--appchain=42=appchain:9090` → your **ChainID** is `42` and the appchain gRPC emitter is at `appchain:9090`.
    * `--snapshot-dir` and `--multichain-dir` live under `/consensus_data` (shared with appchain as `./pelacli_data`).

> Keep **ChainID=42** consistent across your code (`const ChainID = 42`), pelacli mapping, and the `--tx-dir` path that includes `/snapshots/42`.


## Configuration files

### `config/consensus_chains.json` (used by **pelacli** for **reading** from external chains)

> **Put your API key** (Alchemy/Infura/etc.) so pelacli can fetch external chain data and materialize it in MDBX.

```json
[
  {
    "ChainID": 11155111,
    "DBPath": "/multichain/sepolia",
    "APIKey": "YOUR_API_KEY_HERE",
    "StartBlock": 9214937
  }
]
```

* `DBPath` must live under the mounted `/multichain` volume (shared with appchain).
* `StartBlock` controls the initial sync point.

### `config/chain_data.json` (used by **appchain** for **reading** external chain data)

> Maps **chain IDs** to **MDBX paths** (the same ones pelacli maintains). Your appchain uses this to access external blockchain state.

```json
{
  "11155111": "/multichain/sepolia"
}
```

### `config/ext_networks.json` (used by **pelacli** for **writing** to external chains)

> Configures external chains that pelacli can send transactions to. Your appchain generates `ExternalTransaction` items that pelacli processes and submits using these credentials.

```json
[
  {
    "chainId": 137,
    "rpcUrl": "https://polygon-rpc.com",
    "contractAddress": "0x1234567890123456789012345678901234567890",
    "privateKey": "YOUR_PRIVATE_KEY_HERE"
  }
]
```

* `chainId` → The chain ID of the external chain (e.g., 137 for Polygon mainnet, 80002 for Polygon Amoy testnet).
* `rpcUrl` → RPC endpoint for the external chain.
* `contractAddress` → Address of the Pelagos contract on this chain that will route transactions to appchain contracts.
* `privateKey` → Private key that pelacli will use to sign and send transactions to this chain.

> ⚠️ **Security Note**: Keep your private keys secure. Never commit `ext_networks.json` with real private keys to version control. The private key account must have sufficient native tokens for gas fees and appropriate permissions to interact with the Pelagos contract.


## Build & Run

1. **Fill configs:**

   * Put your API key into `config/consensus_chains.json` (required for reading external chain data).
   * Ensure `config/chain_data.json` points to the same MDBX path(s).
   * Configure external transaction networks in `config/ext_networks.json` (optional, only needed for writing to external chains).

2. **Start:**
   * Make sure local docker daemon is working

   ```bash
   docker compose up -d
   ```

3. **Check health:**

   ```bash
   curl -s http://localhost:8080/health | jq .
   ```

4. **Tail logs:**

   ```bash
   docker compose logs -f pelacli
   docker compose logs -f appchain
   ```

5. **Test:**
   * If you are running the skeleton app without changes, you can use the provided script to send test transactions.
   
   ```bash
   ./test_txns.sh
   ```

> On the first run, pelacli will populate MDBX and start producing events/tx-batches. Your appchain waits until the event file and tx-batch DB exist, then begins processing.

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

These are wired in `main.go` and already set in `docker-compose.yml`:

* `--emitter-port=:9090` — gRPC emitter (pelacli pulls txs here)
* `--db-path=/data/appchain-db` — appchain MDBX
* `--local-db-path=/data/local-db` — tx pool MDBX
* `--stream-dir=/consensus_data/events` — event file directory (pelacli writes)
* `--tx-dir=/consensus_data/fetcher/snapshots/42` — **read-only** tx-batch MDBX (pelacli writes)
* `--rpc-port=:8080` — JSON-RPC server
* `--multichain-config=/data/chain_data.json` — external chain MDBX mapping

## Additional Resources

- [Pelagos SDK](https://github.com/0xAtelerix/sdk) — Core SDK documentation and source code
- [SDK Contracts](https://github.com/0xAtelerix/sdk/tree/main/contracts) — Deploy your own AppChain contracts on external chains for custom cross-chain functionality

---
**Happy building! 🚀** For questions or issues, check the [issues](https://github.com/0xAtelerix/example/issues) or reach out to the Pelagos community.
