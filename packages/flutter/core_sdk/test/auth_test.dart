import 'package:test/test.dart';
import 'package:core_sdk/core_sdk.dart';

import 'test_server.dart';

void main() {
  test('PasswordTokenSource mints and caches a token', () async {
    var calls = 0;
    final server = await TestServer.start((req) async {
      calls++;
      final body = await readBody(req);
      final form = Uri.splitQueryString(body);
      expect(form['grant_type'], 'password');
      expect(form['username'], 'demo');
      await respondJson(req, 200, {'access_token': 'minted-token', 'expires_in': 300});
    });
    addTearDown(server.close);

    final ts = PasswordTokenSource(
      keycloakUrl: server.baseUrl,
      realm: 'core',
      clientId: 'core-platform',
      username: 'demo',
      password: 'demo',
    );

    final first = await ts.token();
    expect(first, 'minted-token');
    final second = await ts.token();
    expect(second, 'minted-token');
    expect(calls, 1, reason: 'second call within the token lifetime should hit the cache, not mint again');
  });

  test('PasswordTokenSource refreshes once the cached token is near expiry', () async {
    var calls = 0;
    final server = await TestServer.start((req) async {
      calls++;
      await respondJson(req, 200, {'access_token': 'token-$calls', 'expires_in': 1});
    });
    addTearDown(server.close);

    final ts = PasswordTokenSource(
      keycloakUrl: server.baseUrl,
      realm: 'core',
      clientId: 'core-platform',
      username: 'demo',
      password: 'demo',
      refreshSkew: const Duration(seconds: 5),
    );

    await ts.token();
    await ts.token();
    expect(calls, 2, reason: 'the second call should trigger a real refresh (token expires inside the skew window)');
  });

  test('StaticTokenSource never calls out', () async {
    final ts = StaticTokenSource('fixed');
    expect(await ts.token(), 'fixed');
  });
}
