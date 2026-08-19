import { test } from "node:test";
import assert from "node:assert/strict";
import http from "node:http";
import { PasswordTokenSource, StaticTokenSource } from "./auth.js";

async function withServer(handler: http.RequestListener, run: (baseUrl: string) => Promise<void>) {
  const server = http.createServer(handler);
  await new Promise<void>((resolve) => server.listen(0, resolve));
  const address = server.address();
  const port = typeof address === "object" && address ? address.port : 0;
  try {
    await run(`http://127.0.0.1:${port}`);
  } finally {
    await new Promise<void>((resolve) => server.close(() => resolve()));
  }
}

test("PasswordTokenSource mints and caches a token", async () => {
  let calls = 0;
  await withServer(
    (req, res) => {
      calls++;
      let raw = "";
      req.on("data", (chunk) => (raw += chunk));
      req.on("end", () => {
        const form = new URLSearchParams(raw);
        assert.equal(form.get("grant_type"), "password");
        assert.equal(form.get("username"), "demo");
        res.writeHead(200, { "Content-Type": "application/json" }).end(
          JSON.stringify({ access_token: "minted-token", expires_in: 300 }),
        );
      });
    },
    async (baseUrl) => {
      const ts = new PasswordTokenSource({
        keycloakUrl: baseUrl,
        realm: "core",
        clientId: "core-platform",
        username: "demo",
        password: "demo",
      });
      const first = await ts.token();
      assert.equal(first, "minted-token");
      const second = await ts.token();
      assert.equal(second, "minted-token");
      assert.equal(calls, 1, "second call within the token lifetime should hit the cache, not mint again");
    },
  );
});

test("PasswordTokenSource refreshes once the cached token is near expiry", async () => {
  let calls = 0;
  await withServer(
    (_req, res) => {
      calls++;
      res.writeHead(200, { "Content-Type": "application/json" }).end(
        JSON.stringify({ access_token: `token-${calls}`, expires_in: 1 }),
      );
    },
    async (baseUrl) => {
      const ts = new PasswordTokenSource({
        keycloakUrl: baseUrl,
        realm: "core",
        clientId: "core-platform",
        username: "demo",
        password: "demo",
        refreshSkewMs: 5000,
      });
      await ts.token();
      await ts.token();
      assert.equal(calls, 2, "the second call should trigger a real refresh (token expires inside the skew window)");
    },
  );
});

test("StaticTokenSource never calls out", async () => {
  const ts = new StaticTokenSource("fixed");
  assert.equal(await ts.token(), "fixed");
});
