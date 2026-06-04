// Pure, side-effect-free supervisor math. Keep separate from process.ts so
// the unit tests (which run under plain `node --test`, not a VS Code host)
// can import these without dragging in the 'vscode' module.

export const BREAKER_FAILURE_THRESHOLD = 5;
export const BREAKER_WINDOW_MS = 60_000;
export const BREAKER_OPEN_MS = 60_000;

/**
 * computeBackoffMs returns the backoff in ms for the given attempt
 * count (1-indexed). Sequence: 1s, 2s, 4s, 8s, 16s, 30s capped.
 */
export function computeBackoffMs(attempt: number): number {
    if (attempt < 1) return 1000;
    const base = 1000 * Math.pow(2, attempt - 1);
    return Math.min(base, 30_000);
}

/**
 * shouldOpenBreaker returns true if the given crash count within the
 * BREAKER_WINDOW_MS exceeds BREAKER_FAILURE_THRESHOLD.
 */
export function shouldOpenBreaker(crashCountInWindow: number): boolean {
    return crashCountInWindow >= BREAKER_FAILURE_THRESHOLD;
}
