/**
 * Mirrors the platform's real error envelope
 * (packages/go/platformkit/apperr.Error's wire format:
 * {code, message, correlationId}) - every non-2xx response from
 * core-api decodes into one of these, never a generic Error string.
 */
export class ApiError extends Error {
  readonly statusCode: number;
  readonly code: string;
  readonly correlationId?: string;

  constructor(statusCode: number, code: string, message: string, correlationId?: string) {
    super(`${code}: ${message} (status ${statusCode}${correlationId ? `, correlation ${correlationId}` : ""})`);
    this.name = "ApiError";
    this.statusCode = statusCode;
    this.code = code;
    this.correlationId = correlationId;
  }

  /** True when err is an ApiError carrying this specific code - the
   * idiomatic way to branch on a known failure instead of comparing
   * statusCode or parsing the message string. */
  static is(err: unknown, code: string): err is ApiError {
    return err instanceof ApiError && err.code === code;
  }
}

// Known apperr.Code values, duplicated here (not imported from a
// server-internal package) deliberately - a real product consuming
// this SDK should never need server source to check an error code.
export const ErrorCodes = {
  Validation: "VALIDATION_ERROR",
  Unauthenticated: "AUTHENTICATION_REQUIRED",
  AccessDenied: "ACCESS_DENIED",
  NotFound: "RESOURCE_NOT_FOUND",
  Conflict: "CONFLICT",
  RateLimited: "RATE_LIMITED",
  Internal: "INTERNAL_ERROR",
  Dependency: "DEPENDENCY_FAILURE",
  NotImplemented: "NOT_IMPLEMENTED",
} as const;
