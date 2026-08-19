/// "Retries where safe" - literally: only GET is retried by default.
/// Retrying a POST/PATCH/DELETE blindly risks a duplicate side effect
/// (a second entitlement grant, a second message send) the server has
/// no idempotency key to deduplicate against in most of this platform's
/// routes, so the safe default is "don't." Pass a custom RetryPolicy to
/// opt into different behavior for a route you know is idempotent.
abstract class RetryPolicy {
  int maxAttemptsFor(String method);
  Duration delay(int attempt);
  bool isRetryableStatus(int status);
}

class DefaultRetryPolicy implements RetryPolicy {
  final int maxAttempts;
  final Duration baseDelay;
  static const _retryableStatuses = {429, 502, 503, 504};

  const DefaultRetryPolicy({this.maxAttempts = 3, this.baseDelay = const Duration(milliseconds: 200)});

  @override
  int maxAttemptsFor(String method) {
    if (method.toUpperCase() != 'GET') return 1;
    return maxAttempts < 1 ? 1 : maxAttempts;
  }

  @override
  Duration delay(int attempt) => baseDelay * (1 << attempt);

  @override
  bool isRetryableStatus(int status) => _retryableStatuses.contains(status);
}
