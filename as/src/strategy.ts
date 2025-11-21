import {
  AddressId,
  EventKind,
  SLOT_LAST_UNI_TRANSFER_BLOCK,
  dbGetU64,
  dbPutU64,
  getBlockNumber,
  getEventAddressId,
  getEventCount,
  getEventKind,
  logInfo,
} from "./sdk";

const BLOCK_STALE_THRESHOLD: i64 = 6000;

function hasUniswapTransfer(events: i32): bool {
  for (let index = 0; index < events; index++) {
    if (getEventKind(index) != EventKind.ERC20Transfer) {
      continue;
    }

    const addressId = getEventAddressId(index);
    if (
      addressId == AddressId.UniswapV2Pair ||
      addressId == AddressId.UniswapV3Pool
    ) {
      return true;
    }
  }

  return false;
}

// on_block is invoked by the Go runtime every Pelagos block.
export function on_block(): void {
  const blockNumber = getBlockNumber();
  const eventCount = getEventCount();

  if (hasUniswapTransfer(eventCount)) {
    dbPutU64(SLOT_LAST_UNI_TRANSFER_BLOCK, blockNumber);
    logInfo(
      "Uniswap ERC20 transfer detected at block " + blockNumber.toString()
    );

    return;
  }

  const lastSeen = dbGetU64(SLOT_LAST_UNI_TRANSFER_BLOCK);
  if (lastSeen <= 0) {
    return;
  }

  const delta = blockNumber - lastSeen;
  if (delta > BLOCK_STALE_THRESHOLD) {
    logInfo(
      "No Uniswap transfers observed since block " +
        lastSeen.toString() +
        ", delta=" +
        delta.toString()
    );
  }
}
