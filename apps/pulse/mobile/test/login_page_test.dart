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

  testWidgets('tapping Knock sends a real Knock round trip', (tester) async {
    await tester.pumpWidget(MaterialApp(home: LoginPage(httpClient: _fakeBackend())));
    await tester.tap(find.text('Sign in'));
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const Key('knockButton')));
    await tester.pumpAndSettle();

    expect(find.text('Knock sent'), findsOneWidget);
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
}
