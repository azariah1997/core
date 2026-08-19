import 'dart:async';
import 'dart:convert';
import 'dart:math';
import 'package:http/http.dart' as http;

import 'auth.dart';
import 'errors.dart';
import 'retry.dart';

const _correlationIdHeader = 'X-Correlation-ID';

/// The one entry point into core-api. Every call attaches a real Bearer
/// token (via TokenSource), a correlation ID, and applies the
/// configured retry policy - a caller never touches package:http
/// directly. See operations.dart for typed convenience methods; request
/// is the escape hatch for everything else.
class CoreClient {
  final String baseUrl;
  final http.Client httpClient;
  final TokenSource? tokenSource;
  final RetryPolicy retryPolicy;

  CoreClient(
    String baseUrl, {
    this.tokenSource,
    http.Client? httpClient,
    RetryPolicy? retryPolicy,
  })  : baseUrl = baseUrl.replaceFirst(RegExp(r'/$'), ''),
        httpClient = httpClient ?? http.Client(),
        retryPolicy = retryPolicy ?? const DefaultRetryPolicy();

  Future<T> request<T>(
    String method,
    String path, {
    Object? body,
    Map<String, String?>? query,
    String? correlationId,
    T Function(dynamic json)? decode,
  }) async {
    final id = correlationId ?? _newCorrelationId();
    var uri = Uri.parse(baseUrl + path);
    if (query != null) {
      final params = Map<String, String>.from(uri.queryParameters);
      for (final entry in query.entries) {
        if (entry.value != null) params[entry.key] = entry.value!;
      }
      uri = uri.replace(queryParameters: params);
    }

    final attempts = retryPolicy.maxAttemptsFor(method);
    Object? lastError;
    for (var attempt = 0; attempt < attempts; attempt++) {
      if (attempt > 0) {
        await Future.delayed(retryPolicy.delay(attempt));
      }

      final headers = <String, String>{_correlationIdHeader: id};
      if (body != null) headers['Content-Type'] = 'application/json';
      if (tokenSource != null) {
        final token = await tokenSource!.token();
        if (token.isNotEmpty) headers['Authorization'] = 'Bearer $token';
      }

      http.Response res;
      try {
        res = await _send(method, uri, headers, body);
      } catch (err) {
        lastError = err;
        continue; // network error - eligible for retry per policy
      }

      if (res.statusCode >= 200 && res.statusCode < 300) {
        if (decode == null) return null as T;
        if (res.body.isEmpty) return decode(null);
        return decode(jsonDecode(res.body));
      }

      final apiError = ApiError.fromResponseBody(res.statusCode, res.body);
      if (!retryPolicy.isRetryableStatus(res.statusCode)) {
        throw apiError;
      }
      lastError = apiError;
    }
    throw lastError!;
  }

  Future<http.Response> _send(String method, Uri uri, Map<String, String> headers, Object? body) {
    final encodedBody = body != null ? jsonEncode(body) : null;
    switch (method.toUpperCase()) {
      case 'GET':
        return httpClient.get(uri, headers: headers);
      case 'POST':
        return httpClient.post(uri, headers: headers, body: encodedBody);
      case 'PATCH':
        return httpClient.patch(uri, headers: headers, body: encodedBody);
      case 'PUT':
        return httpClient.put(uri, headers: headers, body: encodedBody);
      case 'DELETE':
        return httpClient.delete(uri, headers: headers, body: encodedBody);
      default:
        throw ArgumentError('coresdk: unsupported method $method');
    }
  }
}

final _rand = Random.secure();

String _newCorrelationId() {
  final bytes = List<int>.generate(16, (_) => _rand.nextInt(256));
  return bytes.map((b) => b.toRadixString(16).padLeft(2, '0')).join();
}
