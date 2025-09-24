# Pelagos Appchain — Build an Appchain with the Go SDK

> This repo is a **skeleton** for building your own appchain on top of the Pelagos Go SDK.
> It comes with a runnable **docker-compose** that includes both your appchain node and a **consensus** (`pelacli`) that simulates consensus and feeds external chain data.

## What you get out of the box

* A minimal **block type** (`Block`) that satisfies `apptypes.AppchainBlock`.
* A **transaction** (`Transaction`) and **receipt** (`Receipt`) implementing a simple token transfer with balances in MDBX.
* A **stateless external-block adapter** (`StateTransition`) that shows how to fetch/inspect Ethereum/Solana data via `MultichainStateAccess`.
* **Genesis seeding** to fund demo users (`alice`, `bob`, …) with USDT/BTC/ETH balances.
* **Buckets** (tables) for app state (`appaccounts`), receipts, blocks, checkpoints, etc.
* A runnable `main.go` that wires the SDK, DBs, tx pool, validator set, the appchain loop, and **JSON-RPC**.
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

> Status lifecycle: **Pending** (in tx pool) → **Batched** (pulled by pelacli) → **Processed/Failed** (after your logic runs in a block).

* **Transactions don’t auto-finalize.**
  Without the **consensus** you’ll only ever see `Pending`. This compose includes `pelacli` to move them forward.

* **The appchain waits for real data sources.**
  It blocks until both exist:

  * the **event file**: `<stream-dir>/epoch_1.data`,
  * the **tx-batch MDBX**: `<tx-dir>` with the `txbatch` table.
    `pelacli` creates and updates both.

* **Multichain access uses local MDBX**
  The SDK reads external chain state from **MDBX databases** on disk. `pelacli` populates and updates them using your RPC **API key**.



## Project layout

```
.
├─ application/
│  ├─ appchain.go         # Interface assertions (compile-time contracts)
│  ├─ block.go            # Block type + constructor
│  ├─ buckets.go          # App buckets (tables)
│  ├─ errors.go           # App-level errors
│  ├─ genesis.go          # One-time state seeding (demo balances)
│  ├─ receipt.go          # Receipt type
│  ├─ state_transition.go # External-chain ingestion (stateless)
│  └─ transaction.go      # Business logic (transfers)
├─ application/api/
│  └─ api.go              # Custom RPC (getBalance)
├─ cmd/main.go            # Wiring & run loop (the app binary)
├─ Dockerfile
└─ docker-compose.yml
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
      - ./multichain:/multichain
    command:
      - consensus
      - --snapshot-dir=/consensus_data
      - --appchain=42=appchain:9090
      - --ask-period=1s
      - --multichain-dir=/consensus_data/multichain_db
      - --chains-json=/consensus_chains.json
```

**What the paths mean**

* Appchain:

    * `--stream-dir=/consensus_data/events` → pelacli writes `epoch_1.data` here.
    * `--tx-dir=/consensus_data/fetcher/snapshots/42` → pelacli writes the read-only MDBX with `txbatch` table here.
    * `--multichain-config=/data/chain_data.json` → maps chain IDs to MDBX DBs for external access.

* pelacli:

    * `--chains-json=/consensus_chains.json` → tells pelacli which L1/L2s to fetch (Sepolia in the example).
    * `--appchain=42=appchain:9090` → your **ChainID** is `42` and the appchain gRPC emitter is at `appchain:9090`.
    * `--snapshot-dir` and `--multichain-dir` live under `/consensus_data` (shared with appchain as `./pelacli_data`).

> Keep **ChainID=42** consistent across your code (`const ChainID = 42`), pelacli mapping, and the `--tx-dir` path that includes `/snapshots/42`.


## Configuration files

### `config/consensus_chains.json` (used by **pelacli**)

> **Put your API key** (Alchemy/Infura/etc.) so pelacli can fetch Sepolia and materialize MDBX.

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

### `config/chain_data.json` (used by **appchain**)

> Maps **chain IDs** to **MDBX paths** (the same ones pelacli maintains).

```json
{
  "11155111": "/multichain/sepolia"
}
```


## Build & Run
1. Fill configs:

* Put your API key into `config/consensus_chains.json`.
* Ensure `config/chain_data.json` points to the same MDBX path(s).

2. Start:

```bash
docker compose up -d
```

3. Check health:

```bash
curl -s http://localhost:8080/health | jq .
```

4. Tail logs:

```bash
docker compose logs -f pelacli
docker compose logs -f appchain
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
  Return `[]ExternalTransaction` if you want to emit cross-chain transaction from appchain to another blockchains.

* **`application/state_transition.go` → `ProcessBlock`**
  Turn **external blocks/receipts** (fetched via `MultichainStateAccess`) into **ExternalTransaction** items that your appchain will later execute (e.g., deposits). Keep this layer **stateless**; all state changes happen in `Transaction.Process`.

* **`application/block.go` → `BlockConstructor`**
  Builds per-block artifacts. Currently uses a **stub** state root; replace `StubRootCalculator` with your own when ready.

* **`application/buckets.go`**
  Add your own tables and merge them with `gosdk.DefaultTables()` in `main.go`.

* **`application/api/api.go`**
  Add read-only custom JSON-RPC methods for your UI.


## Flags (quick reference)

These are wired in `main.go` and already set in `docker-compose.yml`:

* `--emitter-port=:9090` — gRPC emitter (pelacli pulls txs here)
* `--db-path=/data/appchain-db` — appchain MDBX
* `--local-db-path=/data/local-db` — tx pool MDBX
* `--stream-dir=/consensus_data/events` — event file directory (pelacli writes)
* `--tx-dir=/consensus_data/fetcher/snapshots/42` — **read-only** tx-batch MDBX (pelacli writes)
* `--rpc-port=:8080` — JSON-RPC server
* `--multichain-config=/data/chain_data.json` — external chain MDBX mapping
