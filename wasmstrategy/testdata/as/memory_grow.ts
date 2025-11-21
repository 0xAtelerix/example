@external("env", "gas")
declare function useGas(cost: i64): void;

export function on_block(): void {
  useGas(25);
  memory.grow(1);
}
