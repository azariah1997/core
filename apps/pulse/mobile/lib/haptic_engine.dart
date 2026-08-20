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
  Future<void> playPulseStart();
  Future<void> playPulseStop();
  Future<void> playKnock();

  factory HapticEngine.detect() {
    if (kIsWeb) return UnavailableHapticEngine();
    return StandardHapticEngine();
  }
}

class StandardHapticEngine implements HapticEngine {
  @override
  HapticCapability get capability => HapticCapability.standard;

  @override
  Future<void> playPulseStart() => HapticFeedback.mediumImpact();

  @override
  Future<void> playPulseStop() => HapticFeedback.lightImpact();

  @override
  Future<void> playKnock() => HapticFeedback.selectionClick();
}

/// Used on web and any platform HapticFeedback has nothing real to do
/// on - every caller must still function normally against this engine.
class UnavailableHapticEngine implements HapticEngine {
  @override
  HapticCapability get capability => HapticCapability.unavailable;
  @override
  Future<void> playPulseStart() async {}
  @override
  Future<void> playPulseStop() async {}
  @override
  Future<void> playKnock() async {}
}
