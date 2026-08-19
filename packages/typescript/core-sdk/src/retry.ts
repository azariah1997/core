/**
 * "Retries where safe" - literally: only GET is retried by default.
 * Retrying a POST/PATCH/DELETE blindly risks a duplicate side effect
 * (a second entitlement grant, a second message send) the server has
 * no idempotency key to deduplicate against in most of this platform's
 * routes, so the safe default is "don't." Pass a custom RetryPolicy to
 * opt into different behavior for a route you know is idempotent.
 */
export interface RetryPolicy {
  maxAttemptsFor(method: string): number;
  delayMs(attempt: number): number;
  isRetryableStatus(status: number): boolean;
}

const RETRYABLE_STATUSES = new Set([429, 502, 503, 504]);

export class DefaultRetryPolicy implements RetryPolicy {
  constructor(
    private readonly maxAttempts = 3,
    private readonly baseDelayMs = 200,
  ) {}

  maxAttemptsFor(method: string): number {
    if (method.toUpperCase() !== "GET") return 1;
    return Math.max(1, this.maxAttempts);
  }

  delayMs(attempt: number): number {
    return this.baseDelayMs * 2 ** attempt;
  }

  isRetryableStatus(status: number): boolean {
    return RETRYABLE_STATUSES.has(status);
  }
}
