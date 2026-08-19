import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:core_platform_mobile/main.dart';

/// Flutter's widget-test binding (TestWidgetsFlutterBinding) forces
/// every real HttpClient request to fail with a 400 by design, to catch
/// accidental real network calls in a widget test - discovered live
/// while first writing this suite against a real local HTTP server the
/// same way packages/flutter/core_sdk's own `dart test` suite does,
/// which only works there because plain `dart test` has no such
/// binding. The framework's own fix (see its warning text) is to
/// inject a fake http.Client, which is what MockClient below does -
/// this file tests LoginPage's real widget behavior (form, navigation,
/// error display), while the SDK's own HTTP logic is proven separately
/// against a real server in packages/flutter/core_sdk/test.
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
    if (req.url.path == '/v1/users/me') {
      return http.Response(
        jsonEncode({
          'id': 'user-1',
          'displayName': 'Demo User',
          'locale': 'en-US',
          'timezone': 'UTC',
          'status': 'active',
          'createdAt': '2026-01-01T00:00:00.000Z',
          'updatedAt': '2026-01-01T00:00:00.000Z',
        }),
        200,
        headers: {'content-type': 'application/json'},
      );
    }
    if (req.url.path == '/v1/users/me/devices') {
      return http.Response(
        jsonEncode({
          'id': 'device-1',
          'clientDeviceId': 'core-mobile-demo',
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

  testWidgets('successful login navigates to the real profile page', (tester) async {
    await tester.pumpWidget(MaterialApp(home: LoginPage(httpClient: _fakeBackend())));

    await tester.tap(find.text('Sign in'));
    await tester.pumpAndSettle();

    expect(find.text('Demo User'), findsOneWidget);
    expect(find.textContaining('Registered device:'), findsOneWidget);
  });

  testWidgets('an auth failure shows the actual error, not a crash', (tester) async {
    await tester.pumpWidget(MaterialApp(home: LoginPage(httpClient: _fakeBackend(failLogin: true))));

    await tester.tap(find.text('Sign in'));
    await tester.pumpAndSettle();

    expect(find.textContaining('Invalid user credentials'), findsOneWidget);
    expect(find.text('Demo User'), findsNothing);
  });
}
