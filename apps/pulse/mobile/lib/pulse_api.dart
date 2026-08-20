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

  Future<List<PulseConnection>> listConnections() => _client.request(
        'GET',
        '/v1/pulse/connections',
        decode: (json) => ((json as Map<String, dynamic>)['items'] as List)
            .map((e) => PulseConnection.fromJson(e as Map<String, dynamic>))
            .toList(),
      );

  /// Creates and immediately starts a Pulse in one call - the mobile
  /// app's "press and hold" begins the instant the finger goes down
  /// (product spec §13), so there's no meaningful gap between create
  /// and start from the user's perspective.
  Future<PulseInteraction> createAndStart(String receiverId) async {
    final created = await _client.request(
      'POST',
      '/v1/pulse/interactions',
      body: {'type': 'pulse', 'receiverId': receiverId},
      decode: (json) => PulseInteraction.fromJson(json as Map<String, dynamic>),
    );
    return _client.request(
      'POST',
      '/v1/pulse/interactions/${created.id}/start',
      decode: (json) => PulseInteraction.fromJson(json as Map<String, dynamic>),
    );
  }

  Future<PulseInteraction> stop(String interactionId) => _client.request(
        'POST',
        '/v1/pulse/interactions/$interactionId/stop',
        decode: (json) => PulseInteraction.fromJson(json as Map<String, dynamic>),
      );
}

class PulseConnection {
  final String relationshipId;
  final String otherUserId;
  final String status;
  final String classification;

  PulseConnection({required this.relationshipId, required this.otherUserId, required this.status, required this.classification});

  factory PulseConnection.fromJson(Map<String, dynamic> json) => PulseConnection(
        relationshipId: json['relationshipId'] as String,
        otherUserId: json['otherUserId'] as String,
        status: json['status'] as String,
        classification: json['classification'] as String,
      );
}

class PulseInteraction {
  final String id;
  final String status;
  final int? durationMs;

  PulseInteraction({required this.id, required this.status, this.durationMs});

  factory PulseInteraction.fromJson(Map<String, dynamic> json) => PulseInteraction(
        id: json['id'] as String,
        status: json['status'] as String,
        durationMs: json['durationMs'] as int?,
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
