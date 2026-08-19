import 'dart:convert';

/// Mirrors the platform's real error envelope
/// (packages/go/platformkit/apperr.Error's wire format:
/// {code, message, correlationId}) - every non-2xx response from
/// core-api decodes into one of these, never a generic Exception string.
class ApiError implements Exception {
  final int statusCode;
  final String code;
  final String message;
  final String? correlationId;

  ApiError(this.statusCode, this.code, this.message, [this.correlationId]);

  factory ApiError.fromResponseBody(int statusCode, String body) {
    try {
      final decoded = body.isNotEmpty ? jsonDecode(body) : <String, dynamic>{};
      if (decoded is Map<String, dynamic> && decoded['code'] != null) {
        return ApiError(
          statusCode,
          decoded['code'] as String,
          (decoded['message'] as String?) ?? 'unknown error',
          decoded['correlationId'] as String?,
        );
      }
    } catch (_) {
      // fall through to the generic envelope below - a non-JSON error
      // body shouldn't crash error handling, just lose detail.
    }
    return ApiError(statusCode, ErrorCodes.internal, body.isNotEmpty ? body : 'HTTP $statusCode');
  }

  @override
  String toString() =>
      'ApiError($code: $message, status: $statusCode${correlationId != null ? ", correlation: $correlationId" : ""})';

  /// True when err is an ApiError carrying this specific code - the
  /// idiomatic way to branch on a known failure instead of comparing
  /// statusCode or parsing the message string.
  static bool isCode(Object err, String code) => err is ApiError && err.code == code;
}

// Known apperr.Code values, duplicated here (not imported from a
// server-internal package) deliberately - a real product consuming
// this SDK should never need server source to check an error code.
abstract final class ErrorCodes {
  static const validation = 'VALIDATION_ERROR';
  static const unauthenticated = 'AUTHENTICATION_REQUIRED';
  static const accessDenied = 'ACCESS_DENIED';
  static const notFound = 'RESOURCE_NOT_FOUND';
  static const conflict = 'CONFLICT';
  static const rateLimited = 'RATE_LIMITED';
  static const internal = 'INTERNAL_ERROR';
  static const dependency = 'DEPENDENCY_FAILURE';
  static const notImplemented = 'NOT_IMPLEMENTED';
}
