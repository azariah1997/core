import { test } from "node:test";
import assert from "node:assert/strict";
import http from "node:http";
import { CoreClient } from "./client.js";
import { ApiError } from "./errors.js";
import { StaticTokenSource } from "./auth.js";
import { DefaultRetryPolicy } from "./retry.js";

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

test("request attaches Bearer token and a correlation ID", async () => {
  let gotAuth = "";
  let gotCorrelation = "";
  await withServer(
    (req, res) => {
      gotAuth = req.headers["authorization"] ?? "";
      gotCorrelation = (req.headers["x-correlation-id"] as string) ?? "";
      res.writeHead(200).end();
    },
    async (baseUrl) => {
      const client = new CoreClient(baseUrl, { tokenSource: new StaticTokenSource("test-token") });
      await client.request("GET", "/whatever");
      assert.equal(gotAuth, "Bearer test-token");
      assert.notEqual(gotCorrelation, "");
    },
  );
});

test("request propagates an explicit correlation ID", async () => {
  let got = "";
  await withServer(
    (req, res) => {
      got = (req.headers["x-correlation-id"] as string) ?? "";
      res.writeHead(200).end();
    },
    async (baseUrl) => {
      const client = new CoreClient(baseUrl);
      await client.request("GET", "/x", undefined, { correlationId: "caller-supplied-id" });
      assert.equal(got, "caller-supplied-id");
    },
  );
});

test("request decodes a successful JSON response", async () => {
  await withServer(
    (_req, res) => {
      res.writeHead(200, { "Content-Type": "application/json" }).end(JSON.stringify({ name: "core-platform" }));
    },
    async (baseUrl) => {
      const client = new CoreClient(baseUrl);
      const out = await client.request<{ name: string }>("GET", "/x");
      assert.equal(out.name, "core-platform");
    },
  );
});

test("request decodes the real error envelope into an ApiError", async () => {
  await withServer(
    (_req, res) => {
      res.writeHead(403, { "Content-Type": "application/json" }).end(
        JSON.stringify({ code: "ACCESS_DENIED", message: "not allowed", correlationId: "abc-123" }),
      );
    },
    async (baseUrl) => {
      const client = new CoreClient(baseUrl);
      await assert.rejects(
        () => client.request("GET", "/x"),
        (err: unknown) => {
          assert.ok(ApiError.is(err, "ACCESS_DENIED"));
          assert.equal((err as ApiError).statusCode, 403);
          assert.equal((err as ApiError).correlationId, "abc-123");
          return true;
        },
      );
    },
  );
});

test("request sends a JSON body with Content-Type", async () => {
  let gotContentType = "";
  let gotBody = "";
  await withServer(
    (req, res) => {
      gotContentType = req.headers["content-type"] ?? "";
      let raw = "";
      req.on("data", (chunk) => (raw += chunk));
      req.on("end", () => {
        gotBody = raw;
        res.writeHead(201).end();
      });
    },
    async (baseUrl) => {
      const client = new CoreClient(baseUrl);
      await client.request("POST", "/x", { name: "acme" });
      assert.equal(gotContentType, "application/json");
      assert.deepEqual(JSON.parse(gotBody), { name: "acme" });
    },
  );
});

test("GET is retried on a transient status until it succeeds", async () => {
  let calls = 0;
  await withServer(
    (_req, res) => {
      calls++;
      if (calls < 3) {
        res.writeHead(503, { "Content-Type": "application/json" }).end(JSON.stringify({ code: "DEPENDENCY_FAILURE", message: "try again" }));
        return;
      }
      res.writeHead(200).end();
    },
    async (baseUrl) => {
      const client = new CoreClient(baseUrl, { retryPolicy: new DefaultRetryPolicy(3, 0) });
      await client.request("GET", "/x");
      assert.equal(calls, 3);
    },
  );
});

test("POST is not retried by default (retries where safe means GET-only)", async () => {
  let calls = 0;
  await withServer(
    (_req, res) => {
      calls++;
      res.writeHead(503, { "Content-Type": "application/json" }).end(JSON.stringify({ code: "DEPENDENCY_FAILURE", message: "down" }));
    },
    async (baseUrl) => {
      const client = new CoreClient(baseUrl, { retryPolicy: new DefaultRetryPolicy(3, 0) });
      await assert.rejects(() => client.request("POST", "/x"));
      assert.equal(calls, 1);
    },
  );
});

test("a non-transient status (404) is not retried", async () => {
  let calls = 0;
  await withServer(
    (_req, res) => {
      calls++;
      res.writeHead(404, { "Content-Type": "application/json" }).end(JSON.stringify({ code: "RESOURCE_NOT_FOUND", message: "missing" }));
    },
    async (baseUrl) => {
      const client = new CoreClient(baseUrl, { retryPolicy: new DefaultRetryPolicy(3, 0) });
      await assert.rejects(
        () => client.request("GET", "/x"),
        (err: unknown) => ApiError.is(err, "RESOURCE_NOT_FOUND"),
      );
      assert.equal(calls, 1);
    },
  );
});
