@external("env", "get_block_number")
declare function hostGetBlockNumber(): i64;

@external("env", "get_event_count")
declare function hostGetEventCount(): i32;

@external("env", "get_event_kind")
declare function hostGetEventKind(index: i32): i32;

@external("env", "get_event_address_id")
declare function hostGetEventAddressId(index: i32): i32;

@external("env", "db_get_u64")
declare function hostDbGet(slot: i32): i64;

@external("env", "db_put_u64")
declare function hostDbPut(slot: i32, value: i64): void;

@external("env", "log")
declare function hostLog(level: i32, ptr: usize, len: i32): void;

const SLOT_LAST_UNI_TRANSFER_BLOCK: i32 = 1;

enum EventKind {
  Transfer = 1,
}

enum AddressId {
  UniswapV2Pair = 1,
  UniswapV3Pool = 2,
}

function logInfo(message: string): void {
  const encoded = String.UTF8.encode(message);
  hostLog(20, changetype<usize>(encoded), encoded.byteLength);
}

function hasUniswapTransfer(events: i32): bool {
  for (let index = 0; index < events; index++) {
    if (hostGetEventKind(index) != EventKind.Transfer) {
      continue;
    }

    const address = hostGetEventAddressId(index);
    if (address == AddressId.UniswapV2Pair || address == AddressId.UniswapV3Pool) {
      return true;
    }
  }

  return false;
}

export function on_block(): void {
  const block = hostGetBlockNumber();
  const events = hostGetEventCount();

  if (hasUniswapTransfer(events)) {
    hostDbPut(SLOT_LAST_UNI_TRANSFER_BLOCK, block);
    logInfo("Uniswap transfer at block " + block.toString());
    return;
  }

  const lastSeen = hostDbGet(SLOT_LAST_UNI_TRANSFER_BLOCK);
  if (lastSeen > 0) {
    logInfo("No Uniswap transfers since block " + lastSeen.toString());
  }
}
