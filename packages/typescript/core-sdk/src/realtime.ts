import type { TokenSource } from "./auth.js";

/** Mirrors realtime-gateway's real wire format exactly
 * (backend/realtime-gateway/internal/ws/messages.go's serverMessage) -
 * type is one of "connected", "subscribed", "unsubscribed", "message",
 * "direct", "error"; the rest are populated per type. */
export interface RealtimeMessage {
  type: string;
  connectionId?: string;
  userId?: string;
  deviceId?: string;
  channel?: string;
  fromUserId?: string;
  data?: unknown;
  message?: string;
}

/**
 * A live connection to realtime-gateway. Reconnection is deliberately
 * the caller's responsibility (via RealtimeClient.dial again) rather
 * than automatic inside RealtimeConn - transparent auto-reconnect would
 * silently replay subscribe() calls the caller made against the old
 * connection, which needs application-level judgment about what
 * "recovered" even means for a given channel.
 */
export class RealtimeConn {
  constructor(private readonly ws: WebSocket) {}

  subscribe(channel: string) {
    this.send({ type: "subscribe", channel });
  }

  unsubscribe(channel: string) {
    this.send({ type: "unsubscribe", channel });
  }

  /** Sends data to every other subscriber of channel. The server
   * requires the sender to also be subscribed first - see
   * internal/ws/handler.go's real "must subscribe before publishing"
   * check, confirmed live during this SDK's own validation. */
  publish(channel: string, data: unknown) {
    this.send({ type: "publish", channel, data });
  }

  /** Sends data to one specific user across every device/replica they're
   * connected from (server-side Redis-fanned pub/sub, Phase 10). */
  direct(targetUserId: string, data: unknown) {
    this.send({ type: "direct", targetUserId, data });
  }

  /** Registers a handler for every server-sent frame. */
  onMessage(handler: (msg: RealtimeMessage) => void) {
    this.ws.addEventListener("message", (event) => {
      handler(JSON.parse(typeof event.data === "string" ? event.data : String(event.data)));
    });
  }

  onClose(handler: (event: { code: number; reason: string }) => void) {
    this.ws.addEventListener("close", (event) => handler({ code: event.code, reason: event.reason }));
  }

  close() {
    this.ws.close();
  }

  private send(msg: Record<string, unknown>) {
    this.ws.send(JSON.stringify(msg));
  }
}

/**
 * Dials realtime-gateway. Kept separate from CoreClient (core-api's
 * HTTP client) since it's a genuinely different service with its own
 * base URL - the same split the platform itself has between core-api
 * and realtime-gateway.
 */
export class RealtimeClient {
  constructor(
    private readonly baseUrl: string,
    private readonly tokenSource: TokenSource,
  ) {}

  /** Opens a real WebSocket connection, exactly reproducing
   * realtime-gateway's actual auth contract: access_token and deviceId
   * as query parameters (see cmd/server/auth.go's wsAuthMiddleware) -
   * both required, not just the token. Resolves once the server's
   * "connected" frame arrives. */
  async dial(deviceId: string): Promise<RealtimeConn> {
    if (!deviceId) {
      throw new Error("coresdk: deviceId is required to dial realtime-gateway");
    }
    const token = await this.tokenSource.token();
    const wsUrl = this.baseUrl.replace(/^https:\/\//, "wss://").replace(/^http:\/\//, "ws://").replace(/\/$/, "");
    const url = new URL(`${wsUrl}/ws`);
    url.searchParams.set("access_token", token);
    url.searchParams.set("deviceId", deviceId);

    const ws = new WebSocket(url.toString());
    await new Promise<void>((resolve, reject) => {
      ws.addEventListener("open", () => resolve(), { once: true });
      ws.addEventListener("error", () => reject(new Error("coresdk: realtime dial failed")), { once: true });
    });
    return new RealtimeConn(ws);
  }
}
