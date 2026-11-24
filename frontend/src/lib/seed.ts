export const SAFE_SEED_MIN = Number.MIN_SAFE_INTEGER
export const SAFE_SEED_MAX = Number.MAX_SAFE_INTEGER

export function isSafeSeed(seed: number | undefined): seed is number {
  return seed !== undefined && Number.isSafeInteger(seed)
}

/**
 * Builds only the initial assignment; all game metrics and transitions remain
 * server-calculated. BigInt avoids the former 32-bit collapse of safe seeds.
 */
export function seededAssignment(players: number, nodeIds: string[], seed: number): string[] {
  if (!isSafeSeed(seed) || nodeIds.length === 0) return []

  const mask = (1n << 64n) - 1n
  let state = BigInt(seed) & mask
  const next = (): number => {
    state = (state * 6364136223846793005n + 1442695040888963407n) & mask
    return Number(state >> 11n) / 9007199254740992
  }

  return Array.from({ length: players }, () => nodeIds[Math.floor(next() * nodeIds.length)] ?? '')
}
