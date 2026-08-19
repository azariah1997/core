import 'dart:async';
import 'dart:convert';
import 'package:http/http.dart' as http;

/// Returns a valid Bearer token for the current request, refreshing it
/// internally if needed. Every core-api call goes through one -
/// "authentication" and "token refresh" are the SDK's own job, not
/// something every calling product should reimplement.
abstract class TokenSource {
  Future<String> token();
}

/// Wraps an already-minted token with no refresh behavior - useful for
/// short-lived scripts, tests, and callers that already hold a session
/// token from somewhere else.
class StaticTokenSource implements TokenSource {
  final String _value;
  StaticTokenSource(this._value);
  @override
  Future<String> token() async => _value;
}

/// A real Resource Owner Password Credentials grant against Keycloak
/// (the same grant every phase's live validation and apps/admin's
/// server-side login in this repo have used) with transparent refresh -
/// a caller just awaits token() and either gets the cached one or a
/// freshly refreshed one, never an expired one.
class PasswordTokenSource implements TokenSource {
  final String keycloakUrl;
  final String realm;
  final String clientId;
  final String username;
  final String password;
  final http.Client httpClient;
  final Duration refreshSkew;

  String? _cachedToken;
  DateTime? _expiresAt;
  Future<String>? _inFlight;

  PasswordTokenSource({
    required this.keycloakUrl,
    required this.realm,
    required this.clientId,
    required this.username,
    required this.password,
    http.Client? httpClient,
    this.refreshSkew = const Duration(seconds: 15),
  }) : httpClient = httpClient ?? http.Client();

  @override
  Future<String> token() {
    final cached = _cachedToken;
    final expiresAt = _expiresAt;
    if (cached != null && expiresAt != null && DateTime.now().add(refreshSkew).isBefore(expiresAt)) {
      return Future.value(cached);
    }
    // Coalesce concurrent refreshes into a single in-flight request.
    return _inFlight ??= _mint().whenComplete(() => _inFlight = null);
  }

  Future<String> _mint() async {
    final base = keycloakUrl.replaceFirst(RegExp(r'/$'), '');
    final uri = Uri.parse('$base/realms/$realm/protocol/openid-connect/token');
    final res = await httpClient.post(
      uri,
      headers: {'Content-Type': 'application/x-www-form-urlencoded'},
      body: {
        'grant_type': 'password',
        'client_id': clientId,
        'username': username,
        'password': password,
      },
    );
    final body = jsonDecode(res.body) as Map<String, dynamic>;
    if (res.statusCode != 200 || body['access_token'] == null) {
      throw Exception('coresdk: token request failed with status ${res.statusCode}: ${body['error_description'] ?? body['error'] ?? 'unknown error'}');
    }
    _cachedToken = body['access_token'] as String;
    _expiresAt = DateTime.now().add(Duration(seconds: body['expires_in'] as int));
    return _cachedToken!;
  }
}
