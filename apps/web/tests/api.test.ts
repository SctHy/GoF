import { test, vi } from 'vitest'
import assert from "node:assert/strict";
import { apiRequest, apiData, uploadFile, errorMessage, errorStatus } from "../src/api/client.ts";
import { fetchFeedPage, loadFollowingMap } from "../src/api/resources.ts";

test("API client preserves abort, auth and JSON contracts", async () => {
  const controller = new AbortController();
  let captured: { url: Parameters<typeof fetch>[0]; options: RequestInit } | undefined;
  vi.spyOn(globalThis, "fetch").mockImplementation(async (url: Parameters<typeof fetch>[0], options?: RequestInit) => {
    captured = { url, options: options ?? {} };
    return new Response(JSON.stringify({ ok: true }), { status: 200 });
  });
  assert.deepEqual(await apiRequest("/api/example", { method: "POST", token: "test", body: false, signal: controller.signal }), { ok: true });
  assert.ok(captured);
  assert.equal(captured.options.signal, controller.signal);
  assert.equal(new Headers(captured.options.headers).get("Authorization"), "Bearer test");
  assert.equal(new Headers(captured.options.headers).get("Content-Type"), "application/json");
  assert.equal(captured.options.body, "false");
});

test("API error status survives parsing and 204 has no JSON body", async () => {
  const fetchMock = vi.spyOn(globalThis, "fetch").mockImplementation(async () => new Response(JSON.stringify({ error: "expired" }), { status: 401 }));
  await assert.rejects(apiRequest("/api/private"), (error: unknown) => errorStatus(error) === 401 && errorMessage(error) === "expired");
  fetchMock.mockImplementation(async () => new Response(null, { status: 204 }));
  assert.equal(await apiRequest("/api/resource", { method: "DELETE" }), null);
});

test("Feed pagination retains scene, cursor and request context", async () => {
  const calls: { url: Parameters<typeof fetch>[0]; options: RequestInit }[] = [];
  const signal = new AbortController().signal;
  vi.spyOn(globalThis, "fetch").mockImplementation(async (url: Parameters<typeof fetch>[0], options?: RequestInit) => {
    calls.push({ url, options: options ?? {} });
    return new Response(JSON.stringify({ items: [], has_more: false }));
  });
  await fetchFeedPage("recommend", "token", "cursor", "request", signal);
  assert.ok(calls[0]);
  assert.deepEqual(JSON.parse(String(calls[0].options.body)), {scene:"recommend", cursor:"cursor", limit:10, context:{request_id:"request"}});
  assert.equal(calls[0].options?.signal, signal);
  await fetchFeedPage("following", "token", "a+b", "request", signal);
  assert.ok(calls[1]);
  const query = new URL(String(calls[1].url), "http://example.test");
  assert.equal(query.searchParams.get("cursor"), "a+b");
  assert.equal(query.searchParams.get("scene"), "following");
});

test("cancellation stops following pagination before loading further pages", async () => {
  const controller = new AbortController();
  let calls = 0;
  vi.spyOn(globalThis, "fetch").mockImplementation(async (_url: Parameters<typeof fetch>[0], options?: RequestInit) => {
    calls++;
    if (options?.signal?.aborted) throw new DOMException("aborted", "AbortError");
    controller.abort();
    return new Response(JSON.stringify({items:[{user_id:7}], has_more:true, next_cursor:"next"}));
  });
  await assert.rejects(loadFollowingMap("token", controller.signal), { name: "AbortError" });
  assert.equal(calls, 2);
});

test("upload forwards abort signal and lets browser set multipart boundary", async () => {
  const signal = new AbortController().signal;
  vi.spyOn(globalThis, "fetch").mockImplementation(async (_url: Parameters<typeof fetch>[0], options?: RequestInit) => {
    assert.equal(options?.signal, signal);
    assert.equal(new Headers(options?.headers).get("Content-Type"), null);
    assert.ok(options?.body instanceof FormData);
    assert.equal(options.body.get("kind"), "video");
    return new Response(JSON.stringify({url:"/uploads/test.mp4"}));
  });
  assert.equal((await uploadFile(new File(["test"], "test.mp4"), "video", "token", signal)).url, "/uploads/test.mp4");
});

// Unlike commands, reads cannot hand a null payload to a typed component.
test("data requests reject empty responses while commands retain 204 semantics", async () => {
  vi.spyOn(globalThis, "fetch").mockImplementation(async () => new Response(null, { status: 204 }));
  await assert.rejects(apiData<{ items: unknown[] }>("/api/example"), /服务器未返回数据/);
  assert.equal(await apiRequest("/api/example", { method: "DELETE" }), null);
});

test("unknown errors preserve object messages and safely handle non-object rejections", () => {
  assert.equal(errorMessage(new Error("network failure")), "network failure");
  assert.equal(errorMessage({ message: "cursor expired", status: 400 }), "cursor expired");
  assert.equal(errorStatus({ status: 400 }), 400);
  for (const error of [null, undefined, "oops", { message: 42, status: "401" }]) {
    assert.equal(errorMessage(error), "");
    assert.equal(errorStatus(error), undefined);
  }
});
