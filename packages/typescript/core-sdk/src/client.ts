import { ApiError } from "./errors.js";
import { DefaultRetryPolicy, type RetryPolicy } from "./retry.js";
import type { TokenSource } from "./auth.js";

const CORRELATION_ID_HEADER = "X-Correlation-ID";

export interface ClientOptions {
  tokenSource?: TokenSource;
  fetchImpl?: typeof fetch;
  retryPolicy?: RetryPolicy;
}

export interface RequestOptions {
  /** Propagates a caller-supplied correlation ID instead of letting the
   * client generate one - useful when a request is part of a larger
   * traced operation that already has an ID. */
  correlationId?: string;
  query?: Record<string, string | undefined>;
}

/**
 * The one entry point into core-api. Every call attaches a real Bearer
 * token (via TokenSource), a correlation ID, and applies the configured
 * retry policy - a caller never touches fetch() directly. See
 * operations.ts for typed convenience methods; request() is the escape
 * hatch for everything else.
 */
export class CoreClient {
  private readonly baseUrl: string;
  private readonly fetchImpl: typeof fetch;
  private readonly tokenSource?: TokenSource;
  private readonly retryPolicy: RetryPolicy;

  constructor(baseUrl: string, opts: ClientOptions = {}) {
    this.baseUrl = baseUrl.replace(/\/$/, "");
    this.fetchImpl = opts.fetchImpl ?? fetch;
    this.tokenSource = opts.tokenSource;
    this.retryPolicy = opts.retryPolicy ?? new DefaultRetryPolicy();
  }

  async request<T = unknown>(method: string, path: string, body?: unknown, opts: RequestOptions = {}): Promise<T> {
    const correlationId = opts.correlationId ?? crypto.randomUUID();
    let url = this.baseUrl + path;
    if (opts.query) {
      const params = new URLSearchParams();
      for (const [key, value] of Object.entries(opts.query)) {
        if (value !== undefined) params.set(key, value);
      }
      const qs = params.toString();
      if (qs) url += (path.includes("?") ? "&" : "?") + qs;
    }

    const attempts = this.retryPolicy.maxAttemptsFor(method);
    let lastError: unknown;
    for (let attempt = 0; attempt < attempts; attempt++) {
      if (attempt > 0) {
        await new Promise((resolve) => setTimeout(resolve, this.retryPolicy.delayMs(attempt)));
      }

      const headers: Record<string, string> = { [CORRELATION_ID_HEADER]: correlationId };
      if (body !== undefined) headers["Content-Type"] = "application/json";
      if (this.tokenSource) {
        const token = await this.tokenSource.token();
        if (token) headers["Authorization"] = `Bearer ${token}`;
      }

      let res: Response;
      try {
        res = await this.fetchImpl(url, {
          method,
          headers,
          body: body !== undefined ? JSON.stringify(body) : undefined,
        });
      } catch (err) {
        lastError = err;
        continue; // network error - eligible for retry per policy
      }

      if (res.ok) {
        if (res.status === 204) return undefined as T;
        const text = await res.text();
        return (text ? JSON.parse(text) : undefined) as T;
      }

      const apiError = await decodeApiError(res);
      if (!this.retryPolicy.isRetryableStatus(res.status)) {
        throw apiError;
      }
      lastError = apiError;
    }
    throw lastError;
  }
}

async function decodeApiError(res: Response): Promise<ApiError> {
  let code = "INTERNAL_ERROR";
  let message = res.statusText;
  let correlationId: string | undefined;
  try {
    const body = await res.json();
    if (body?.code) {
      code = body.code;
      message = body.message ?? message;
      correlationId = body.correlationId;
    }
  } catch {
    // non-JSON error body - fall back to the defaults above
  }
  return new ApiError(res.status, code, message, correlationId);
}
