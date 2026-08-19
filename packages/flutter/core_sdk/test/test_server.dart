import 'dart:convert';
import 'dart:io';

/// A tiny real HTTP server for tests - mirrors the Go SDK's
/// httptest.Server and the TypeScript SDK's node:http harness, so all
/// three SDKs' test suites prove the same thing the same way: real
/// request/response handling, not a mocked HTTP client.
class TestServer {
  final HttpServer _server;
  final void Function(HttpRequest req) handler;

  TestServer._(this._server, this.handler) {
    _server.listen((req) => handler(req));
  }

  static Future<TestServer> start(void Function(HttpRequest req) handler) async {
    final server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
    return TestServer._(server, handler);
  }

  String get baseUrl => 'http://127.0.0.1:${_server.port}';

  Future<void> close() => _server.close(force: true);
}

Future<void> respondJson(HttpRequest req, int status, Map<String, dynamic> body) async {
  req.response.statusCode = status;
  req.response.headers.contentType = ContentType.json;
  req.response.write(jsonEncode(body));
  await req.response.close();
}

Future<String> readBody(HttpRequest req) => utf8.decoder.bind(req).join();
