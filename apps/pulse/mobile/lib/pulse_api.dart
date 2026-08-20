import 'package:core_sdk/core_sdk.dart';
import 'package:http/http.dart' as http;

/// The thin, hand-written Pulse client the architecture audit called
/// for (apps/pulse/docs/ARCHITECTURE_AUDIT.md's "Mobile Architecture"
/// section) - built directly on core_sdk's CoreClient (same
/// auth/retry/correlation-ID machinery, pointed at pulse-api's base URL
/// instead of core-api's) rather than a second hand-rolled HTTP client.
/// Grows a typed method per Pulse endpoint as each module is built;
/// Phase 1 only needs pulse-profile's.
class PulseApi {
  final CoreClient _client;

  PulseApi(String baseUrl, {required TokenSource tokenSource, http.Client? httpClient})
      : _client = CoreClient(baseUrl, tokenSource: tokenSource, httpClient: httpClient);

  Future<PulseProfile> ensureProfile(String handle) => _client.request(
        'POST',
        '/v1/pulse/profile',
        body: {'handle': handle},
        decode: (json) => PulseProfile.fromJson(json as Map<String, dynamic>),
      );

  Future<PulseProfile> myProfile() => _client.request(
        'GET',
        '/v1/pulse/profile/me',
        decode: (json) => PulseProfile.fromJson(json as Map<String, dynamic>),
      );
}

class PulseProfile {
  final String userId;
  final String handle;
  final String createdAt;

  PulseProfile({required this.userId, required this.handle, required this.createdAt});

  factory PulseProfile.fromJson(Map<String, dynamic> json) => PulseProfile(
        userId: json['userId'] as String,
        handle: json['handle'] as String,
        createdAt: json['createdAt'] as String,
      );
}
