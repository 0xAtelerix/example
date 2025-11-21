// SPDX-License-Identifier: MIT
// Pelagos AssemblyScript SDK shim exposing host functions provided by the Go runtime.

// EventKind enumerates the event payloads that the Go host exposes to strategies.
export enum EventKind {
  Unknown = 0,
  ERC20Transfer = 1,
}

// AddressId groups frequently used contracts so strategies can branch on a small enum.
export enum AddressId {
  Unknown = 0,
  UniswapV2Pair = 1,
  UniswapV3Pool = 2,
}

// DbSlot identifiers backed by the runtime key/value store.
export const SLOT_LAST_UNI_TRANSFER_BLOCK: i32 = 1;

// LogLevel mirrors the numeric contract expected by env.log.
export enum LogLevel {
  Debug = 10,
  Info = 20,
  Warn = 30,
  Error = 40,
}

@external("env", "get_block_number")
declare function hostGetBlockNumber(): i64;

@external("env", "get_chain_id")
declare function hostGetChainId(): i64;

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

// getBlockNumber returns the current Pelagos block number.
export function getBlockNumber(): i64 {
  return hostGetBlockNumber();
}

// getChainId returns the appchain identifier.
export function getChainId(): i64 {
  return hostGetChainId();
}

// getEventCount returns how many events are available for this block.
export function getEventCount(): i32 {
  return hostGetEventCount();
}

// getEventKind exposes the strongly typed EventKind for the event at index.
export function getEventKind(index: i32): EventKind {
  return changetype<EventKind>(hostGetEventKind(index));
}

// getEventAddressId returns the AddressId classification for the event at index.
export function getEventAddressId(index: i32): AddressId {
  return changetype<AddressId>(hostGetEventAddressId(index));
}

// dbGetU64 reads a 64-bit slot from the strategy KV store.
export function dbGetU64(slot: i32): i64 {
  return hostDbGet(slot);
}

// dbPutU64 writes a 64-bit slot to the strategy KV store.
export function dbPutU64(slot: i32, value: i64): void {
  hostDbPut(slot, value);
}

function writeLog(level: LogLevel, message: string): void {
  const encoded = String.UTF8.encode(message);
  hostLog(level, changetype<usize>(encoded), encoded.byteLength);
}

// logDebug emits a debugging log line.
export function logDebug(message: string): void {
  writeLog(LogLevel.Debug, message);
}

// logInfo emits an informational log line.
export function logInfo(message: string): void {
  writeLog(LogLevel.Info, message);
}
