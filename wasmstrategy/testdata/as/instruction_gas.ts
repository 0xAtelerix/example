@external("env", "db_get_u64")
declare function dbGet(slot: i32): i64;

export function on_block(): void {
  dbGet(0);
  dbGet(0);
}
