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

  /// Pulse Back (product spec §17) - one call, faster than opening a
  /// messaging interface. interactionId is the *original* Pulse being
  /// responded to; the server creates, starts, and stops the reciprocal
  /// Pulse itself and returns it already completed.
  Future<PulseInteraction> pulseBack(String interactionId) => _client.request(
        'POST',
        '/v1/pulse/interactions/$interactionId/pulse-back',
        decode: (json) => PulseInteraction.fromJson(json as Map<String, dynamic>),
      );

  /// Knock (product spec §18) - a short predefined pattern, not a held
  /// gesture, so one call: the server creates, starts, and stops it
  /// itself and returns it already completed, same shape as pulseBack.
  /// pattern defaults server-side to double_tap when omitted.
  Future<PulseInteraction> knock(String receiverId, {String? pattern}) => _client.request(
        'POST',
        '/v1/pulse/knocks',
        body: {'receiverId': receiverId, 'pattern': ?pattern},
        decode: (json) => PulseInteraction.fromJson(json as Map<String, dynamic>),
      );

  /// Today's Mood (product spec §22-27) - a single visual symbol plus
  /// an audience; the server resolves audience into who can actually
  /// see it, so the client never computes visibility itself. circleId
  /// is only meaningful when audience is 'selected_circles' (Phase 9).
  Future<PulseMood> setMood(String emoji, String audience, {List<String>? customUserIds, String? circleId}) => _client.request(
        'PUT',
        '/v1/pulse/mood',
        body: {'emoji': emoji, 'audience': audience, 'customUserIds': ?customUserIds, 'circleId': ?circleId},
        decode: (json) => PulseMood.fromJson(json as Map<String, dynamic>),
      );

  Future<void> clearMood() => _client.request('DELETE', '/v1/pulse/mood');

  Future<PulseMood> myMood() => _client.request(
        'GET',
        '/v1/pulse/mood/me',
        decode: (json) => PulseMood.fromJson(json as Map<String, dynamic>),
      );

  /// Throws a real 404 ApiError both when userId has no Mood set and
  /// when one exists but the caller isn't in its audience - the server
  /// deliberately never distinguishes the two (see mood.Service.Get).
  Future<PulseMood> viewMood(String userId) => _client.request(
        'GET',
        '/v1/pulse/mood/$userId',
        decode: (json) => PulseMood.fromJson(json as Map<String, dynamic>),
      );

  /// Circles (product spec §10, Phase 9) - a thin wrapper Pulse's own
  /// pulse-connections module puts over Core's real groups capability;
  /// creating a Circle already makes the caller its first member (Core's
  /// own Create guarantee), so no separate "add myself" call is needed.
  Future<PulseCircle> createCircle(String name) => _client.request(
        'POST',
        '/v1/pulse/circles',
        body: {'name': name},
        decode: (json) => PulseCircle.fromJson(json as Map<String, dynamic>),
      );

  Future<List<PulseCircle>> listCircles() => _client.request(
        'GET',
        '/v1/pulse/circles',
        decode: (json) => ((json as Map<String, dynamic>)['items'] as List).map((e) => PulseCircle.fromJson(e as Map<String, dynamic>)).toList(),
      );

  Future<List<PulseCircleMember>> listCircleMembers(String circleId) => _client.request(
        'GET',
        '/v1/pulse/circles/$circleId/members',
        decode: (json) => ((json as Map<String, dynamic>)['items'] as List).map((e) => PulseCircleMember.fromJson(e as Map<String, dynamic>)).toList(),
      );

  /// Throws a real 403 ApiError if userId isn't already a real,
  /// active Pulse connection (see pulseconnections.Service.AddCircleMember) -
  /// a Circle can never reach a stranger.
  Future<PulseCircleMember> addCircleMember(String circleId, String userId) => _client.request(
        'POST',
        '/v1/pulse/circles/$circleId/members',
        body: {'userId': userId},
        decode: (json) => PulseCircleMember.fromJson(json as Map<String, dynamic>),
      );

  Future<void> removeCircleMember(String circleId, String userId) => _client.request('DELETE', '/v1/pulse/circles/$circleId/members/$userId');

  /// The caller's current active Bond partner, if any - throws a real
  /// 404 ApiError when unbonded. Live Touch (below) is gated on this.
  Future<PulseBond> myBond() => _client.request(
        'GET',
        '/v1/pulse/bond',
        decode: (json) => PulseBond.fromJson(json as Map<String, dynamic>),
      );

  /// Live Touch (product spec §21, Phase 10) - the flagship synchronous
  /// two-way touch feature, gated on an active Bond (never merely a
  /// Friend). The server resolves the caller's own current Bond partner
  /// itself, so no target is passed here. Touch-start/touch-stop events
  /// themselves never go through this HTTP client at all - once active,
  /// both participants exchange them directly over the session's own
  /// realtime channel (see LiveTouchScreen).
  Future<PulseLiveTouchSession> inviteLiveTouch() => _client.request(
        'POST',
        '/v1/pulse/live-touch/sessions',
        decode: (json) => PulseLiveTouchSession.fromJson(json as Map<String, dynamic>),
      );

  Future<PulseLiveTouchSession> acceptLiveTouch(String sessionId) => _client.request(
        'POST',
        '/v1/pulse/live-touch/sessions/$sessionId/accept',
        decode: (json) => PulseLiveTouchSession.fromJson(json as Map<String, dynamic>),
      );

  Future<PulseLiveTouchSession> declineLiveTouch(String sessionId) => _client.request(
        'POST',
        '/v1/pulse/live-touch/sessions/$sessionId/decline',
        decode: (json) => PulseLiveTouchSession.fromJson(json as Map<String, dynamic>),
      );

  Future<PulseLiveTouchSession> endLiveTouch(String sessionId) => _client.request(
        'POST',
        '/v1/pulse/live-touch/sessions/$sessionId/end',
        decode: (json) => PulseLiveTouchSession.fromJson(json as Map<String, dynamic>),
      );

  /// Custom Signals (product spec §19-20, Phase 11) - a private touch
  /// pattern bound to exactly one specific connection ("the same
  /// pattern means something different to different pairs"). segments
  /// is a list of {'type': 'tap'|'hold'|'pause', 'durationMs': int}
  /// maps - bounds (segment count, per-segment and total duration) are
  /// enforced server-side, never merely a client-side UI limit.
  Future<PulseSignal> createSignal(String targetUserId, String label, List<Map<String, dynamic>> segments) => _client.request(
        'POST',
        '/v1/pulse/signals',
        body: {'targetUserId': targetUserId, 'label': label, 'segments': segments},
        decode: (json) => PulseSignal.fromJson(json as Map<String, dynamic>),
      );

  Future<List<PulseSignal>> listSignals() => _client.request(
        'GET',
        '/v1/pulse/signals',
        decode: (json) => ((json as Map<String, dynamic>)['items'] as List).map((e) => PulseSignal.fromJson(e as Map<String, dynamic>)).toList(),
      );

  Future<void> deleteSignal(String id) => _client.request('DELETE', '/v1/pulse/signals/$id');

  /// Felt exactly like a Pulse or Knock (live if the receiver is
  /// connected, a durable push otherwise) - the receiver is always the
  /// signal's own bound target, resolved server-side, never a
  /// client-supplied value.
  Future<PulseInteraction> sendSignal(String id) => _client.request(
        'POST',
        '/v1/pulse/signals/$id/send',
        decode: (json) => PulseInteraction.fromJson(json as Map<String, dynamic>),
      );

  /// Moments (product spec §30-31, Phase 12) - "Save this moment ♥."
  /// A personal bookmark: saving is idempotent (saving the same
  /// interaction twice returns the original), and never automatically
  /// shared with the other participant - they'd need their own Save.
  Future<PulseMoment> saveMoment(String interactionId) => _client.request(
        'POST',
        '/v1/pulse/moments/$interactionId/save',
        decode: (json) => PulseMoment.fromJson(json as Map<String, dynamic>),
      );

  Future<List<PulseMoment>> listMoments() => _client.request(
        'GET',
        '/v1/pulse/moments',
        decode: (json) => ((json as Map<String, dynamic>)['items'] as List).map((e) => PulseMoment.fromJson(e as Map<String, dynamic>)).toList(),
      );

  Future<void> deleteMoment(String id) => _client.request('DELETE', '/v1/pulse/moments/$id');

  /// Experience Controls (Phase 13) - notification detail level and
  /// haptic intensity are Pulse's own preferences (Core has no concept
  /// of either).
  Future<PulsePreferences> getPreferences() => _client.request(
        'GET',
        '/v1/pulse/preferences',
        decode: (json) => PulsePreferences.fromJson(json as Map<String, dynamic>),
      );

  Future<PulsePreferences> setPreferences({required String notificationDetail, required double hapticIntensity}) => _client.request(
        'PUT',
        '/v1/pulse/preferences',
        body: {'notificationDetail': notificationDetail, 'hapticIntensity': hapticIntensity},
        decode: (json) => PulsePreferences.fromJson(json as Map<String, dynamic>),
      );

  /// Quiet Hours - a thin Pulse-side settings surface over Core's real,
  /// already-live notifications.QuietHours (scoped to Pulse's own
  /// AppID server-side); already enforced automatically before any push
  /// leaves the platform.
  Future<PulseQuietHours> getQuietHours() => _client.request(
        'GET',
        '/v1/pulse/preferences/quiet-hours',
        decode: (json) => PulseQuietHours.fromJson(json as Map<String, dynamic>),
      );

  Future<PulseQuietHours> setQuietHours({required String timezone, required int startMinute, required int endMinute, required bool enabled}) => _client.request(
        'PUT',
        '/v1/pulse/preferences/quiet-hours',
        body: {'timezone': timezone, 'startMinute': startMinute, 'endMinute': endMinute, 'enabled': enabled},
        decode: (json) => PulseQuietHours.fromJson(json as Map<String, dynamic>),
      );

  /// Mute - a thin settings surface over Core's real, platform-wide
  /// trustsafety.Mute (one-directional and silent, distinct from
  /// Block).
  Future<PulseMute> mute(String mutedUserId) => _client.request(
        'POST',
        '/v1/pulse/preferences/mutes',
        body: {'mutedUserId': mutedUserId},
        decode: (json) => PulseMute.fromJson(json as Map<String, dynamic>),
      );

  Future<List<PulseMute>> listMutes() => _client.request(
        'GET',
        '/v1/pulse/preferences/mutes',
        decode: (json) => ((json as Map<String, dynamic>)['items'] as List).map((e) => PulseMute.fromJson(e as Map<String, dynamic>)).toList(),
      );

  Future<void> unmute(String mutedUserId) => _client.request('DELETE', '/v1/pulse/preferences/mutes/$mutedUserId');
}

class PulsePreferences {
  final String notificationDetail;
  final double hapticIntensity;

  PulsePreferences({required this.notificationDetail, required this.hapticIntensity});

  factory PulsePreferences.fromJson(Map<String, dynamic> json) => PulsePreferences(
        notificationDetail: json['notificationDetail'] as String,
        hapticIntensity: (json['hapticIntensity'] as num).toDouble(),
      );
}

class PulseQuietHours {
  final String timezone;
  final int startMinute;
  final int endMinute;
  final bool enabled;

  PulseQuietHours({required this.timezone, required this.startMinute, required this.endMinute, required this.enabled});

  factory PulseQuietHours.fromJson(Map<String, dynamic> json) => PulseQuietHours(
        timezone: json['timezone'] as String,
        startMinute: json['startMinute'] as int,
        endMinute: json['endMinute'] as int,
        enabled: json['enabled'] as bool,
      );
}

class PulseMute {
  final String id;
  final String mutedUserId;

  PulseMute({required this.id, required this.mutedUserId});

  factory PulseMute.fromJson(Map<String, dynamic> json) => PulseMute(
        id: json['id'] as String,
        mutedUserId: json['mutedUserId'] as String,
      );
}

class PulseMoment {
  final String id;
  final String otherUserId;
  final String interactionType;
  final int? durationMs;
  final String occurredAt;

  PulseMoment({required this.id, required this.otherUserId, required this.interactionType, this.durationMs, required this.occurredAt});

  factory PulseMoment.fromJson(Map<String, dynamic> json) => PulseMoment(
        id: json['id'] as String,
        otherUserId: json['otherUserId'] as String,
        interactionType: json['interactionType'] as String,
        durationMs: json['durationMs'] as int?,
        occurredAt: json['occurredAt'] as String,
      );
}

class PulseBond {
  final String otherUserId;
  final String status;

  PulseBond({required this.otherUserId, required this.status});

  factory PulseBond.fromJson(Map<String, dynamic> json) => PulseBond(
        otherUserId: json['otherUserId'] as String,
        status: json['status'] as String,
      );
}

class PulseSignal {
  final String id;
  final String targetUserId;
  final String label;
  final List<Map<String, dynamic>> segments;

  PulseSignal({required this.id, required this.targetUserId, required this.label, required this.segments});

  factory PulseSignal.fromJson(Map<String, dynamic> json) => PulseSignal(
        id: json['id'] as String,
        targetUserId: json['targetUserId'] as String,
        label: json['label'] as String? ?? '',
        segments: ((json['segments'] as List?) ?? []).map((e) => e as Map<String, dynamic>).toList(),
      );
}

class PulseLiveTouchSession {
  final String id;
  final String otherUserId;
  final String status;
  final String deliveryMode;
  final String? channel;

  PulseLiveTouchSession({required this.id, required this.otherUserId, required this.status, required this.deliveryMode, this.channel});

  factory PulseLiveTouchSession.fromJson(Map<String, dynamic> json) => PulseLiveTouchSession(
        id: json['id'] as String,
        otherUserId: json['otherUserId'] as String,
        status: json['status'] as String,
        deliveryMode: json['deliveryMode'] as String,
        channel: json['channel'] as String?,
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

class PulseCircle {
  final String id;
  final String name;

  PulseCircle({required this.id, required this.name});

  factory PulseCircle.fromJson(Map<String, dynamic> json) => PulseCircle(
        id: json['id'] as String,
        name: json['name'] as String,
      );
}

class PulseCircleMember {
  final String userId;
  final bool isManager;

  PulseCircleMember({required this.userId, required this.isManager});

  factory PulseCircleMember.fromJson(Map<String, dynamic> json) => PulseCircleMember(
        userId: json['userId'] as String,
        isManager: json['isManager'] as bool,
      );
}

class PulseMood {
  final String userId;
  final String emoji;
  final String expiresAt;

  PulseMood({required this.userId, required this.emoji, required this.expiresAt});

  factory PulseMood.fromJson(Map<String, dynamic> json) => PulseMood(
        userId: json['userId'] as String,
        emoji: json['emoji'] as String,
        expiresAt: json['expiresAt'] as String,
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
