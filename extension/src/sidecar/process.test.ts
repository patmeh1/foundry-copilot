// Pure unit tests for the supervisor's backoff/breaker math. Runnable
// without a VS Code host because we only exercise the side-effect-free
// ./supervisor module.
import test from 'node:test';
import assert from 'node:assert/strict';

import {
    BREAKER_FAILURE_THRESHOLD,
    BREAKER_OPEN_MS,
    BREAKER_WINDOW_MS,
    computeBackoffMs,
    shouldOpenBreaker,
} from './supervisor';

test('11.07 backoff sequence is 1s,2s,4s,8s,16s,30s capped', () => {
    assert.equal(computeBackoffMs(1), 1000);
    assert.equal(computeBackoffMs(2), 2000);
    assert.equal(computeBackoffMs(3), 4000);
    assert.equal(computeBackoffMs(4), 8000);
    assert.equal(computeBackoffMs(5), 16000);
    assert.equal(computeBackoffMs(6), 30000); // cap kicks in: 32000 -> 30000
    assert.equal(computeBackoffMs(7), 30000);
    assert.equal(computeBackoffMs(0), 1000); // floor guard
});

test('11.08 breaker constants are 5 failures / 60s window / 60s open', () => {
    assert.equal(BREAKER_FAILURE_THRESHOLD, 5);
    assert.equal(BREAKER_WINDOW_MS, 60_000);
    assert.equal(BREAKER_OPEN_MS, 60_000);
});

test('11.09 shouldOpenBreaker fires only at threshold', () => {
    assert.equal(shouldOpenBreaker(4), false);
    assert.equal(shouldOpenBreaker(5), true);
    assert.equal(shouldOpenBreaker(6), true);
});
