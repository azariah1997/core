import 'dart:async';
import 'dart:convert';
import 'package:web_socket_channel/web_socket_channel.dart';

import 'auth.dart';

/// Mirrors realtime-gateway's real wire format exactly
/// (backend/realtime-gateway/internal/ws/messages.go's serverMessage) -
/// type is one of "connected", "subscribed", "unsubscribed", "message",
/// "direct", "error"; the rest are populated per type.
class RealtimeMessage {
  final String type;
  final String? connectionId;
  final String? userId;
  final String? deviceId;
  final String? channel;
  final String? fromUserId;
  final dynamic data;
  final String? message;

  RealtimeMessage({
    required this.type,
    this.connectionId,
    this.userId,
    this.deviceId,
    this.channel,
    this.fromUserId,
    this.data,
    this.message,
  });

  factory RealtimeMessage.fromJson(Map<String, dynamic> json) => RealtimeMessage(
        type: json['type'] as String,
        connectionId: json['connectionId'] as String?,
        userId: json['userId'] as String?,
        deviceId: json['deviceId'] as String?,
        channel: json['channel'] as String?,
        fromUserId: json['fromUserId'] as String?,
        data: json['data'],
        message: json['message'] as String?,
      );
}

/// A live connection to realtime-gateway. Reconnection is deliberately
/// the caller's responsibility (via RealtimeClient.dial again) rather
/// than automatic inside RealtimeConn - transparent auto-reconnect would
/// silently replay subscribe() calls the caller made against the old
/// connection, which needs application-level judgment about what
/// "recovered" even means for a given channel.
class RealtimeConn {
  final WebSocketChannel _channel;
  late final Stream<RealtimeMessage> messages;

  RealtimeConn(this._channel) {
    messages = _channel.stream.map((raw) => RealtimeMessage.fromJson(jsonDecode(raw as String) as Map<String, dynamic>)).asBroadcastStream();
  }

  void subscribe(String channel) => _send({'type': 'subscribe', 'channel': channel});

  void unsubscribe(String channel) => _send({'type': 'unsubscribe', 'channel': channel});

  /// Sends data to every other subscriber of channel. The server
  /// requires the sender to also be subscribed first - see
  /// internal/ws/handler.go's real "must subscribe before publishing"
  /// check, confirmed live during this SDK's own validation.
  void publish(String channel, dynamic data) => _send({'type': 'publish', 'channel': channel, 'data': data});

  /// Sends data to one specific user across every device/replica they're
  /// connected from (server-side Redis-fanned pub/sub, Phase 10).
  void direct(String targetUserId, dynamic data) => _send({'type': 'direct', 'targetUserId': targetUserId, 'data': data});

  void _send(Map<String, dynamic> msg) => _channel.sink.add(jsonEncode(msg));

  Future<void> close() => _channel.sink.close();
}

/// Dials realtime-gateway. Kept separate from CoreClient (core-api's
/// HTTP client) since it's a genuinely different service with its own
/// base URL - the same split the platform itself has between core-api
/// and realtime-gateway.
class RealtimeClient {
  final String baseUrl;
  final TokenSource tokenSource;

  RealtimeClient(this.baseUrl, this.tokenSource);

  /// Opens a real WebSocket connection, exactly reproducing
  /// realtime-gateway's actual auth contract: access_token and deviceId
  /// as query parameters (see cmd/server/auth.go's wsAuthMiddleware) -
  /// both required, not just the token.
  Future<RealtimeConn> dial(String deviceId) async {
    if (deviceId.isEmpty) {
      throw ArgumentError('coresdk: deviceId is required to dial realtime-gateway');
    }
    final token = await tokenSource.token();
    var wsUrl = baseUrl.replaceFirst('https://', 'wss://').replaceFirst('http://', 'ws://').replaceFirst(RegExp(r'/$'), '');
    final uri = Uri.parse('$wsUrl/ws').replace(queryParameters: {
      'access_token': token,
      'deviceId': deviceId,
    });
    final channel = WebSocketChannel.connect(uri);
    await channel.ready;
    return RealtimeConn(channel);
  }
}
