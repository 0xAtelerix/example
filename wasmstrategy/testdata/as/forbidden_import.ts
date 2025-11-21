@external("env", "evil")
declare function evil(): void;

export function on_block(): void {
  evil();
}
