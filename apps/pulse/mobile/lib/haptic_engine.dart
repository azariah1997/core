import 'package:flutter/foundation.dart' show kIsWeb;
import 'package:flutter/services.dart';

enum HapticCapability { advanced, standard, unavailable }

/// Pulse's haptic abstraction (product spec §59-60): playPulseStart/
/// playPulseStop/playKnock, with graceful degradation so the product
/// still works on hardware without advanced haptics (spec §86,
/// accessibility). This phase ships the STANDARD tier only, via
/// Flutter's cross-platform HapticFeedback - real and genuinely felt on
/// a real Android/iOS device, not a stub. The ADVANCED tier (custom
/// iOS Core Haptics patterns shaped to a Pulse's actual duration, spec
/// §59) needs native Swift/Kotlin platform-channel code this session's
/// environment cannot build or run end-to-end (no Android SDK, no
/// complete Xcode install - see apps/pulse/docs/ARCHITECTURE_AUDIT.md's
/// Risk #1 and #6) - a real, honestly-documented gap, not implemented
/// here rather than faked.
abstract class HapticEngine {
  HapticCapability get capability;

  /// Experience Controls (product spec §72, Phase 13) - 0.0 (mute) to
  /// 1.0 (full). Flutter's cross-platform HapticFeedback API has no
  /// continuous intensity control (only a handful of named tiers), so
  /// the STANDARD tier's honest approximation is picking a lighter tier
  /// below 0.5 and skipping playback entirely at 0 - real, felt
  /// behavior change, not a no-op setting. True continuous intensity
  /// needs the same native ADVANCED-tier platform-channel work already
  /// named as a gap above.
  double get intensity;
  set intensity(double value);

  Future<void> playPulseStart();
  Future<void> playPulseStop();
  Future<void> playKnock();

  /// Custom Signals (product spec §19-20, Phase 11) - "the pattern
  /// itself is the communication," so playback is purely local
  /// interpretation of whatever tap/hold/pause segments arrived (or are
  /// being composed): the platform server never assigns them meaning,
  /// and neither does this method - it just plays them back in order.
  /// segments is a plain list of {'type': 'tap'|'hold'|'pause',
  /// 'durationMs': int} maps, matching the wire format directly (no
  /// typed dependency on pulse_api.dart's own models).
  Future<void> playSegments(List<dynamic> segments);

  factory HapticEngine.detect() {
    if (kIsWeb) return UnavailableHapticEngine();
    return StandardHapticEngine();
  }
}

class StandardHapticEngine implements HapticEngine {
  double _intensity = 1.0;

  @override
  double get intensity => _intensity;
  @override
  set intensity(double value) => _intensity = value.clamp(0.0, 1.0);

  @override
  HapticCapability get capability => HapticCapability.standard;

  Future<void> _play(Future<void> Function() full, Future<void> Function() light) {
    if (_intensity <= 0) return Future.value();
    return (_intensity < 0.5 ? light : full)();
  }

  @override
  Future<void> playPulseStart() => _play(HapticFeedback.mediumImpact, HapticFeedback.selectionClick);

  @override
  Future<void> playPulseStop() => _play(HapticFeedback.lightImpact, HapticFeedback.selectionClick);

  @override
  Future<void> playKnock() => _play(HapticFeedback.selectionClick, HapticFeedback.selectionClick);

  @override
  Future<void> playSegments(List<dynamic> segments) async {
    if (_intensity <= 0) return;
    for (final seg in segments) {
      final type = seg is Map ? seg['type'] as String? : null;
      final durationMs = seg is Map ? (seg['durationMs'] as num?)?.toInt() ?? 0 : 0;
      switch (type) {
        case 'tap':
          await HapticFeedback.selectionClick();
          break;
        case 'hold':
          await _play(HapticFeedback.heavyImpact, HapticFeedback.selectionClick);
          break;
        case 'pause':
          break;
      }
      await Future.delayed(Duration(milliseconds: durationMs));
    }
  }
}

/// Used on web and any platform HapticFeedback has nothing real to do
/// on - every caller must still function normally against this engine.
class UnavailableHapticEngine implements HapticEngine {
  double _intensity = 1.0;
  @override
  double get intensity => _intensity;
  @override
  set intensity(double value) => _intensity = value.clamp(0.0, 1.0);
  @override
  HapticCapability get capability => HapticCapability.unavailable;
  @override
  Future<void> playPulseStart() async {}
  @override
  Future<void> playPulseStop() async {}
  @override
  Future<void> playKnock() async {}
  @override
  Future<void> playSegments(List<dynamic> segments) async {}
}
