import 'package:test/test.dart';
import 'package:core_sdk/core_sdk.dart';

import 'test_server.dart';

void main() {
  test('request attaches Bearer token and a correlation ID', () async {
    String? gotAuth;
    String? gotCorrelation;
    final server = await TestServer.start((req) async {
      gotAuth = req.headers.value('authorization');
      gotCorrelation = req.headers.value('x-correlation-id');
      await respondJson(req, 200, {});
    });
    addTearDown(server.close);

    final client = CoreClient(server.baseUrl, tokenSource: StaticTokenSource('test-token'));
    await client.request('GET', '/whatever');

    expect(gotAuth, 'Bearer test-token');
    expect(gotCorrelation, isNotNull);
    expect(gotCorrelation, isNotEmpty);
  });

  test('request propagates an explicit correlation ID', () async {
    String? got;
    final server = await TestServer.start((req) async {
      got = req.headers.value('x-correlation-id');
      await respondJson(req, 200, {});
    });
    addTearDown(server.close);

    final client = CoreClient(server.baseUrl);
    await client.request('GET', '/x', correlationId: 'caller-supplied-id');

    expect(got, 'caller-supplied-id');
  });

  test('request decodes a successful JSON response', () async {
    final server = await TestServer.start((req) async {
      await respondJson(req, 200, {'name': 'core-platform'});
    });
    addTearDown(server.close);

    final client = CoreClient(server.baseUrl);
    final out = await client.request<String>('GET', '/x', decode: (j) => j['name'] as String);

    expect(out, 'core-platform');
  });

  test('request decodes the real error envelope into an ApiError', () async {
    final server = await TestServer.start((req) async {
      await respondJson(req, 403, {'code': 'ACCESS_DENIED', 'message': 'not allowed', 'correlationId': 'abc-123'});
    });
    addTearDown(server.close);

    final client = CoreClient(server.baseUrl);
    try {
      await client.request('GET', '/x');
      fail('expected an ApiError');
    } catch (err) {
      expect(ApiError.isCode(err, ErrorCodes.accessDenied), isTrue);
      final apiErr = err as ApiError;
      expect(apiErr.statusCode, 403);
      expect(apiErr.correlationId, 'abc-123');
    }
  });

  test('request sends a JSON body with Content-Type', () async {
    String? gotContentType;
    String? gotBody;
    final server = await TestServer.start((req) async {
      gotContentType = req.headers.value('content-type');
      gotBody = await readBody(req);
      await respondJson(req, 201, {});
    });
    addTearDown(server.close);

    final client = CoreClient(server.baseUrl);
    await client.request('POST', '/x', body: {'name': 'acme'});

    expect(gotContentType, contains('application/json'));
    expect(gotBody, '{"name":"acme"}');
  });

  test('GET is retried on a transient status until it succeeds', () async {
    var calls = 0;
    final server = await TestServer.start((req) async {
      calls++;
      if (calls < 3) {
        await respondJson(req, 503, {'code': 'DEPENDENCY_FAILURE', 'message': 'try again'});
        return;
      }
      await respondJson(req, 200, {});
    });
    addTearDown(server.close);

    final client = CoreClient(server.baseUrl, retryPolicy: const DefaultRetryPolicy(maxAttempts: 3, baseDelay: Duration.zero));
    await client.request('GET', '/x');

    expect(calls, 3);
  });

  test('POST is not retried by default (retries where safe means GET-only)', () async {
    var calls = 0;
    final server = await TestServer.start((req) async {
      calls++;
      await respondJson(req, 503, {'code': 'DEPENDENCY_FAILURE', 'message': 'down'});
    });
    addTearDown(server.close);

    final client = CoreClient(server.baseUrl, retryPolicy: const DefaultRetryPolicy(maxAttempts: 3, baseDelay: Duration.zero));
    await expectLater(client.request('POST', '/x'), throwsA(isA<ApiError>()));

    expect(calls, 1);
  });

  test('a non-transient status (404) is not retried', () async {
    var calls = 0;
    final server = await TestServer.start((req) async {
      calls++;
      await respondJson(req, 404, {'code': 'RESOURCE_NOT_FOUND', 'message': 'missing'});
    });
    addTearDown(server.close);

    final client = CoreClient(server.baseUrl, retryPolicy: const DefaultRetryPolicy(maxAttempts: 3, baseDelay: Duration.zero));
    try {
      await client.request('GET', '/x');
      fail('expected an ApiError');
    } catch (err) {
      expect(ApiError.isCode(err, ErrorCodes.notFound), isTrue);
    }
    expect(calls, 1);
  });
}
