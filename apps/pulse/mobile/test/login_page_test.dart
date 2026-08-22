import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:pulse/main.dart';

/// Same MockClient-injection pattern apps/mobile's own login_page_test.dart
/// established (Flutter's TestWidgetsFlutterBinding forces every real
/// HttpClient request to fail with a 400 by design - see that file's
/// own comment for the full story). One fake backend answers both
/// core-api's routes (login, identity) and pulse-api's (profile), since
/// LoginPage forwards the same MockClient to both real API clients.
http.Client _fakeBackend({bool failLogin = false}) {
  // Mutable across requests within one test - lets a set/clear Mood
  // round trip through the same fake backend the way a real server's
  // stored state would, rather than every GET returning a fixed fixture.
  String? moodEmoji;
  final circles = <Map<String, dynamic>>[];
  final circleMembers = <String, List<String>>{};
  final signalsList = <Map<String, dynamic>>[];
  final signalsSent = <String>[];
  final momentsList = <Map<String, dynamic>>[];
  var prefs = {'notificationDetail': 'detailed', 'hapticIntensity': 1.0};
  var quietHours = {'timezone': 'UTC', 'startMinute': 0, 'endMinute': 0, 'enabled': false};
  final mutesList = <Map<String, dynamic>>[];
  return MockClient((req) async {
    if (req.url.path.contains('/protocol/openid-connect/token')) {
      if (failLogin) {
        return http.Response(
          jsonEncode({'error': 'invalid_grant', 'error_description': 'Invalid user credentials'}),
          401,
          headers: {'content-type': 'application/json'},
        );
      }
      return http.Response(
        jsonEncode({'access_token': 'fake-token', 'expires_in': 300}),
        200,
        headers: {'content-type': 'application/json'},
      );
    }
    if (req.url.path == '/v1/identity/me') {
      return http.Response(
        jsonEncode({'id': 'identity-1', 'provider': 'keycloak', 'providerSubject': 'sub-1', 'status': 'active', 'userId': 'user-1'}),
        200,
        headers: {'content-type': 'application/json'},
      );
    }
    if (req.url.path == '/v1/pulse/profile') {
      return http.Response(
        jsonEncode({
          'userId': 'user-1',
          'handle': 'demo_pulse',
          'createdAt': '2026-01-01T00:00:00.000Z',
          'updatedAt': '2026-01-01T00:00:00.000Z',
        }),
        201,
        headers: {'content-type': 'application/json'},
      );
    }
    if (req.url.path == '/v1/pulse/connections') {
      return http.Response(
        jsonEncode({
          'items': [
            {'relationshipId': 'rel-1', 'otherUserId': 'user-2friend', 'status': 'active', 'classification': 'friend'},
          ],
        }),
        200,
        headers: {'content-type': 'application/json'},
      );
    }
    if (req.url.path == '/v1/pulse/interactions') {
      return http.Response(
        jsonEncode({'id': 'interaction-1', 'type': 'pulse', 'otherUserId': 'user-2friend', 'role': 'sender', 'status': 'created', 'deliveryMode': 'live', 'createdAt': '2026-01-01T00:00:00.000Z'}),
        201,
        headers: {'content-type': 'application/json'},
      );
    }
    if (req.url.path == '/v1/pulse/interactions/interaction-1/start') {
      return http.Response(
        jsonEncode({'id': 'interaction-1', 'type': 'pulse', 'otherUserId': 'user-2friend', 'role': 'sender', 'status': 'started', 'deliveryMode': 'live', 'createdAt': '2026-01-01T00:00:00.000Z'}),
        200,
        headers: {'content-type': 'application/json'},
      );
    }
    if (req.url.path == '/v1/pulse/interactions/interaction-1/stop') {
      return http.Response(
        jsonEncode({'id': 'interaction-1', 'type': 'pulse', 'otherUserId': 'user-2friend', 'role': 'sender', 'status': 'completed', 'deliveryMode': 'live', 'durationMs': 842, 'createdAt': '2026-01-01T00:00:00.000Z'}),
        200,
        headers: {'content-type': 'application/json'},
      );
    }
    if (req.url.path == '/v1/pulse/knocks') {
      return http.Response(
        jsonEncode({'id': 'knock-1', 'type': 'knock', 'otherUserId': 'user-2friend', 'role': 'sender', 'status': 'completed', 'deliveryMode': 'live', 'pattern': 'double_tap', 'createdAt': '2026-01-01T00:00:00.000Z'}),
        201,
        headers: {'content-type': 'application/json'},
      );
    }
    if (req.url.path == '/v1/pulse/bond') {
      return http.Response(
        jsonEncode({'id': 'bond-1', 'otherUserId': 'user-2friend', 'status': 'active', 'requestedAt': '2026-01-01T00:00:00.000Z'}),
        200,
        headers: {'content-type': 'application/json'},
      );
    }
    if (req.url.path == '/v1/pulse/live-touch/sessions') {
      return http.Response(
        jsonEncode({'id': 'session-1', 'otherUserId': 'user-2friend', 'role': 'initiator', 'status': 'invited', 'deliveryMode': 'push', 'invitedAt': '2026-01-01T00:00:00.000Z'}),
        201,
        headers: {'content-type': 'application/json'},
      );
    }
    if (req.method == 'POST' && req.url.path == '/v1/pulse/signals') {
      final body = jsonDecode(req.body) as Map<String, dynamic>;
      final sig = {'id': 'signal-${signalsList.length + 1}', 'targetUserId': body['targetUserId'], 'label': body['label'], 'segments': body['segments'], 'createdAt': '2026-01-01T00:00:00.000Z'};
      signalsList.add(sig);
      return http.Response(jsonEncode(sig), 201, headers: {'content-type': 'application/json'});
    }
    if (req.method == 'GET' && req.url.path == '/v1/pulse/signals') {
      return http.Response(jsonEncode({'items': signalsList}), 200, headers: {'content-type': 'application/json'});
    }
    final sendSignalMatch = RegExp(r'^/v1/pulse/signals/([^/]+)/send$').firstMatch(req.url.path);
    if (sendSignalMatch != null && req.method == 'POST') {
      signalsSent.add(sendSignalMatch.group(1)!);
      return http.Response(
        jsonEncode({'id': 'interaction-signal-1', 'type': 'signal', 'otherUserId': 'user-2friend', 'role': 'sender', 'status': 'completed', 'deliveryMode': 'push', 'createdAt': '2026-01-01T00:00:00.000Z'}),
        201,
        headers: {'content-type': 'application/json'},
      );
    }
    final saveMomentMatch = RegExp(r'^/v1/pulse/moments/([^/]+)/save$').firstMatch(req.url.path);
    if (saveMomentMatch != null && req.method == 'POST') {
      final moment = {
        'id': 'moment-${momentsList.length + 1}',
        'interactionId': saveMomentMatch.group(1),
        'otherUserId': 'user-2friend',
        'interactionType': 'pulse',
        'durationMs': 842,
        'occurredAt': '2026-01-01T00:00:00.000Z',
        'savedAt': '2026-01-01T00:00:00.000Z',
      };
      momentsList.add(moment);
      return http.Response(jsonEncode(moment), 201, headers: {'content-type': 'application/json'});
    }
    if (req.method == 'GET' && req.url.path == '/v1/pulse/moments') {
      return http.Response(jsonEncode({'items': momentsList}), 200, headers: {'content-type': 'application/json'});
    }
    final deleteMomentMatch = RegExp(r'^/v1/pulse/moments/([^/]+)$').firstMatch(req.url.path);
    if (deleteMomentMatch != null && req.method == 'DELETE') {
      momentsList.removeWhere((m) => m['id'] == deleteMomentMatch.group(1));
      return http.Response('', 204);
    }
    if (req.method == 'GET' && req.url.path == '/v1/pulse/preferences') {
      return http.Response(jsonEncode(prefs), 200, headers: {'content-type': 'application/json'});
    }
    if (req.method == 'PUT' && req.url.path == '/v1/pulse/preferences') {
      final body = jsonDecode(req.body) as Map<String, dynamic>;
      prefs = {'notificationDetail': body['notificationDetail'], 'hapticIntensity': body['hapticIntensity']};
      return http.Response(jsonEncode(prefs), 200, headers: {'content-type': 'application/json'});
    }
    if (req.method == 'GET' && req.url.path == '/v1/pulse/preferences/quiet-hours') {
      return http.Response(jsonEncode(quietHours), 200, headers: {'content-type': 'application/json'});
    }
    if (req.method == 'PUT' && req.url.path == '/v1/pulse/preferences/quiet-hours') {
      final body = jsonDecode(req.body) as Map<String, dynamic>;
      quietHours = {'timezone': body['timezone'], 'startMinute': body['startMinute'], 'endMinute': body['endMinute'], 'enabled': body['enabled']};
      return http.Response(jsonEncode(quietHours), 200, headers: {'content-type': 'application/json'});
    }
    if (req.method == 'POST' && req.url.path == '/v1/pulse/preferences/mutes') {
      final body = jsonDecode(req.body) as Map<String, dynamic>;
      final mutedUserId = body['mutedUserId'] as String;
      final m = {'id': 'mute-${mutesList.length + 1}', 'mutedUserId': mutedUserId, 'createdAt': '2026-01-01T00:00:00.000Z'};
      mutesList.add(m);
      return http.Response(jsonEncode(m), 201, headers: {'content-type': 'application/json'});
    }
    if (req.method == 'GET' && req.url.path == '/v1/pulse/preferences/mutes') {
      return http.Response(jsonEncode({'items': mutesList}), 200, headers: {'content-type': 'application/json'});
    }
    final unmuteMatch = RegExp(r'^/v1/pulse/preferences/mutes/([^/]+)$').firstMatch(req.url.path);
    if (unmuteMatch != null && req.method == 'DELETE') {
      mutesList.removeWhere((m) => m['mutedUserId'] == unmuteMatch.group(1));
      return http.Response('', 204);
    }
    if (req.method == 'PUT' && req.url.path == '/v1/pulse/mood') {
      final body = jsonDecode(req.body) as Map<String, dynamic>;
      moodEmoji = body['emoji'] as String;
      return http.Response(
        jsonEncode({'userId': 'user-1', 'emoji': moodEmoji, 'audience': body['audience'], 'createdAt': '2026-01-01T00:00:00.000Z', 'expiresAt': '2026-01-02T00:00:00.000Z', 'updatedAt': '2026-01-01T00:00:00.000Z'}),
        200,
        headers: {'content-type': 'application/json'},
      );
    }
    if (req.method == 'DELETE' && req.url.path == '/v1/pulse/mood') {
      moodEmoji = null;
      return http.Response('', 204);
    }
    if (req.url.path == '/v1/pulse/mood/me') {
      if (moodEmoji == null) {
        return http.Response(jsonEncode({'code': 'NOT_FOUND', 'message': 'mood not found'}), 404, headers: {'content-type': 'application/json'});
      }
      return http.Response(
        jsonEncode({'userId': 'user-1', 'emoji': moodEmoji, 'createdAt': '2026-01-01T00:00:00.000Z', 'expiresAt': '2026-01-02T00:00:00.000Z'}),
        200,
        headers: {'content-type': 'application/json'},
      );
    }
    if (req.url.path == '/v1/pulse/mood/user-2friend') {
      return http.Response(
        jsonEncode({'userId': 'user-2friend', 'emoji': '🔥', 'createdAt': '2026-01-01T00:00:00.000Z', 'expiresAt': '2026-01-02T00:00:00.000Z'}),
        200,
        headers: {'content-type': 'application/json'},
      );
    }
    if (req.method == 'POST' && req.url.path == '/v1/pulse/circles') {
      final body = jsonDecode(req.body) as Map<String, dynamic>;
      final circle = {'id': 'circle-${circles.length + 1}', 'name': body['name'], 'status': 'active', 'createdAt': '2026-01-01T00:00:00.000Z', 'updatedAt': '2026-01-01T00:00:00.000Z'};
      circles.add(circle);
      return http.Response(jsonEncode(circle), 201, headers: {'content-type': 'application/json'});
    }
    if (req.method == 'GET' && req.url.path == '/v1/pulse/circles') {
      return http.Response(jsonEncode({'items': circles}), 200, headers: {'content-type': 'application/json'});
    }
    final membersMatch = RegExp(r'^/v1/pulse/circles/([^/]+)/members$').firstMatch(req.url.path);
    if (membersMatch != null && req.method == 'GET') {
      final ids = circleMembers[membersMatch.group(1)] ?? [];
      final items = ids.map((id) => {'userId': id, 'role': 'member', 'isManager': false, 'createdAt': '2026-01-01T00:00:00.000Z'}).toList();
      return http.Response(jsonEncode({'items': items}), 200, headers: {'content-type': 'application/json'});
    }
    if (membersMatch != null && req.method == 'POST') {
      final circleId = membersMatch.group(1)!;
      final body = jsonDecode(req.body) as Map<String, dynamic>;
      final userId = body['userId'] as String;
      circleMembers.putIfAbsent(circleId, () => []).add(userId);
      return http.Response(
        jsonEncode({'userId': userId, 'role': 'member', 'isManager': false, 'createdAt': '2026-01-01T00:00:00.000Z'}),
        201,
        headers: {'content-type': 'application/json'},
      );
    }
    if (req.url.path == '/v1/users/me/devices') {
      return http.Response(
        jsonEncode({
          'id': 'device-1',
          'clientDeviceId': 'pulse-mobile',
          'platform': 'flutter',
          'locale': 'en-US',
          'timezone': 'UTC',
          'hasPushToken': false,
          'sessionStatus': 'active',
          'lastActiveAt': '2026-01-01T00:00:00.000Z',
          'createdAt': '2026-01-01T00:00:00.000Z',
        }),
        200,
        headers: {'content-type': 'application/json'},
      );
    }
    return http.Response('not found', 404);
  });
}

void main() {
  testWidgets('login form renders with demo/demo defaults', (tester) async {
    await tester.pumpWidget(MaterialApp(home: LoginPage(httpClient: _fakeBackend())));

    expect(find.text('Sign in'), findsOneWidget);
    expect(find.widgetWithText(TextField, 'Username'), findsOneWidget);
    expect(find.widgetWithText(TextField, 'Password'), findsOneWidget);
  });

  testWidgets('successful login navigates to the five-tab home shell', (tester) async {
    await tester.pumpWidget(MaterialApp(home: LoginPage(httpClient: _fakeBackend())));

    await tester.tap(find.text('Sign in'));
    await tester.pumpAndSettle();

    expect(find.text('Hold to Pulse'), findsOneWidget);
    expect(find.text('Home'), findsOneWidget);
    expect(find.text('People'), findsOneWidget);
    expect(find.text('Mood'), findsOneWidget);
    expect(find.text('Moments'), findsOneWidget);
    expect(find.text('Profile'), findsOneWidget);
  });

  testWidgets('an auth failure shows the actual error, not a crash', (tester) async {
    await tester.pumpWidget(MaterialApp(home: LoginPage(httpClient: _fakeBackend(failLogin: true))));

    await tester.tap(find.text('Sign in'));
    await tester.pumpAndSettle();

    expect(find.textContaining('Invalid user credentials'), findsOneWidget);
    expect(find.text('Hold to Pulse'), findsNothing);
  });

  testWidgets('pressing and releasing the Home button sends a real Pulse round trip', (tester) async {
    await tester.pumpWidget(MaterialApp(home: LoginPage(httpClient: _fakeBackend())));
    await tester.tap(find.text('Sign in'));
    await tester.pumpAndSettle();

    final gesture = await tester.startGesture(tester.getCenter(find.byKey(const Key('pulseButton'))));
    await tester.pump();
    expect(find.textContaining('Holding…'), findsOneWidget);

    await gesture.up();
    await tester.pumpAndSettle();

    expect(find.textContaining('Pulse sent — felt for 842ms'), findsOneWidget);
  });

  testWidgets('saving a completed Pulse as a Moment round-trips and shows up on the Moments tab', (tester) async {
    await tester.pumpWidget(MaterialApp(home: LoginPage(httpClient: _fakeBackend())));
    await tester.tap(find.text('Sign in'));
    await tester.pumpAndSettle();

    final gesture = await tester.startGesture(tester.getCenter(find.byKey(const Key('pulseButton'))));
    await tester.pump();
    await gesture.up();
    await tester.pumpAndSettle();

    expect(find.byKey(const Key('saveMomentButton')), findsOneWidget);
    await tester.tap(find.byKey(const Key('saveMomentButton')));
    await tester.pumpAndSettle();

    expect(find.text('Saved ♥'), findsOneWidget);

    await tester.tap(find.text('Moments'));
    await tester.pumpAndSettle();

    expect(find.textContaining('A Pulse — felt for 842ms'), findsOneWidget);
  });

  testWidgets('tapping Knock sends a real Knock round trip', (tester) async {
    await tester.pumpWidget(MaterialApp(home: LoginPage(httpClient: _fakeBackend())));
    await tester.tap(find.text('Sign in'));
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const Key('knockButton')));
    await tester.pumpAndSettle();

    expect(find.text('Knock sent'), findsOneWidget);
  });

  testWidgets('setting and clearing a Mood round-trips through the real pulse-api chain', (tester) async {
    await tester.pumpWidget(MaterialApp(home: LoginPage(httpClient: _fakeBackend())));
    await tester.tap(find.text('Sign in'));
    await tester.pumpAndSettle();

    await tester.tap(find.text('Mood'));
    await tester.pumpAndSettle();

    await tester.tap(find.text('🔥').first);
    await tester.pump();
    await tester.tap(find.byKey(const Key('setMoodButton')));
    await tester.pumpAndSettle();

    expect(find.text('Mood set'), findsOneWidget);
    expect(find.byKey(const Key('clearMoodButton')), findsOneWidget);

    await tester.tap(find.byKey(const Key('clearMoodButton')));
    await tester.pumpAndSettle();

    expect(find.text('Mood cleared'), findsOneWidget);
    expect(find.byKey(const Key('clearMoodButton')), findsNothing);
  });

  testWidgets('the Home tab shows the target connection\'s visible Mood', (tester) async {
    await tester.pumpWidget(MaterialApp(home: LoginPage(httpClient: _fakeBackend())));
    await tester.tap(find.text('Sign in'));
    await tester.pumpAndSettle();

    expect(find.textContaining('feeling 🔥'), findsOneWidget);
  });

  testWidgets('creating a Circle and adding a real connection as a member round-trips through the real pulse-api chain', (tester) async {
    await tester.pumpWidget(MaterialApp(home: LoginPage(httpClient: _fakeBackend())));
    await tester.tap(find.text('Sign in'));
    await tester.pumpAndSettle();

    await tester.tap(find.text('People'));
    await tester.pumpAndSettle();

    await tester.enterText(find.byKey(const Key('newCircleNameField')), 'Family');
    await tester.tap(find.byKey(const Key('createCircleButton')));
    await tester.pumpAndSettle();

    expect(find.text('Family'), findsOneWidget);

    await tester.tap(find.text('Family'));
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const Key('addCircleMemberDropdown')));
    await tester.pumpAndSettle();
    await tester.tap(find.text('user-2fr').last);
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const Key('addCircleMemberButton')));
    await tester.pumpAndSettle();

    expect(find.text('user-2fr'), findsOneWidget);
  });

  testWidgets('tapping Live Touch invites the real bond partner and shows a waiting state', (tester) async {
    await tester.pumpWidget(MaterialApp(home: LoginPage(httpClient: _fakeBackend())));
    await tester.tap(find.text('Sign in'));
    await tester.pumpAndSettle();

    expect(find.byKey(const Key('liveTouchButton')), findsOneWidget);
    expect(find.text('Live Touch'), findsOneWidget);

    await tester.tap(find.byKey(const Key('liveTouchButton')));
    await tester.pumpAndSettle();

    expect(find.text('Waiting for partner…'), findsOneWidget);
  });

  testWidgets('composing, saving, and sending a Custom Signal round-trips through the real pulse-api chain', (tester) async {
    await tester.pumpWidget(MaterialApp(home: LoginPage(httpClient: _fakeBackend())));
    await tester.tap(find.text('Sign in'));
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const Key('signalsButton')));
    await tester.pumpAndSettle();

    expect(find.text('No signals yet for this connection.'), findsOneWidget);

    await tester.tap(find.byKey(const Key('addTapButton')));
    await tester.tap(find.byKey(const Key('addPauseButton')));
    await tester.tap(find.byKey(const Key('addHoldButton')));
    await tester.pump();

    await tester.enterText(find.byType(TextField), 'I love you');
    await tester.tap(find.byKey(const Key('saveSignalButton')));
    await tester.pumpAndSettle();

    expect(find.text('Signal saved'), findsOneWidget);
    expect(find.text('I love you'), findsOneWidget);

    await tester.tap(find.byKey(const Key('sendSignalButton')));
    await tester.pumpAndSettle();

    expect(find.textContaining('Sent "I love you"'), findsOneWidget);
  });

  testWidgets('the Profile tab round-trips through the real pulse-api chain', (tester) async {
    await tester.pumpWidget(MaterialApp(home: LoginPage(httpClient: _fakeBackend())));
    await tester.tap(find.text('Sign in'));
    await tester.pumpAndSettle();

    await tester.tap(find.text('Profile'));
    await tester.pumpAndSettle();

    expect(find.text('@demo_pulse'), findsOneWidget);
    expect(find.textContaining('Core User ID: user-1'), findsOneWidget);
  });

  testWidgets('Experience Controls (notification detail, haptic intensity, quiet hours, mute) round-trip through the real pulse-api chain', (tester) async {
    await tester.pumpWidget(MaterialApp(home: LoginPage(httpClient: _fakeBackend())));
    await tester.tap(find.text('Sign in'));
    await tester.pumpAndSettle();

    await tester.tap(find.text('Profile'));
    await tester.pumpAndSettle();

    expect(find.byKey(const Key('notificationDetailDropdown')), findsOneWidget);
    await tester.tap(find.byKey(const Key('notificationDetailDropdown')));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Private — generic notification only').last);
    await tester.pumpAndSettle();

    expect(find.text('Private — generic notification only'), findsOneWidget);

    await tester.ensureVisible(find.byKey(const Key('quietHoursEnabledSwitch')));
    await tester.tap(find.byKey(const Key('quietHoursEnabledSwitch')));
    await tester.pumpAndSettle();

    final switchWidget = tester.widget<Switch>(find.byKey(const Key('quietHoursEnabledSwitch')));
    expect(switchWidget.value, isTrue);

    await tester.ensureVisible(find.byKey(const Key('muteUserIdField')));
    await tester.enterText(find.byKey(const Key('muteUserIdField')), 'user-2friend');
    await tester.ensureVisible(find.byKey(const Key('muteButton')));
    await tester.tap(find.byKey(const Key('muteButton')));
    await tester.pumpAndSettle();

    expect(find.text('user-2friend'), findsOneWidget);

    await tester.tap(find.text('Unmute'));
    await tester.pumpAndSettle();

    expect(find.text('user-2friend'), findsNothing);
  });
}
