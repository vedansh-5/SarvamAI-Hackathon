import assert from "node:assert/strict";
import { test } from "node:test";
import { asMail, downloadFrom, judge, passOn } from "./handle.ts";

const good = (extra = {}) => JSON.stringify({ email: "asha@example.com", note: "Excel", company: "", waited: 5000, ...extra });

test("a real entry is accepted", () => {
  const v = judge("POST", "/api/windows", good());
  assert.equal(v.ok, true);
  assert.deepEqual(v.entry, { email: "asha@example.com", note: "Excel" });
});

test("the trap, a hurry, a bad address and a long note are each refused", () => {
  for (const [extra, why] of [
    [{ company: "Acme" }, "trap"],
    [{ waited: 300 }, "too quick"],
    [{ email: "not-an-address" }, "email"],
    [{ email: "a@b.co\r\nBcc: x@y.z" }, "email"],
    [{ note: "x".repeat(501) }, "note too long"],
  ]) {
    const v = judge("POST", "/api/windows", good(extra));
    assert.equal(v.ok, false);
    assert.equal(v.why, why);
  }
});

test("anything but a post to the one path is refused", () => {
  assert.equal(judge("GET", "/api/windows", "").status, 405);
  assert.equal(judge("POST", "/api/other", good()).status, 404);
  assert.equal(judge("POST", "/api/windows", "x".repeat(3000)).status, 413);
  assert.equal(judge("POST", "/api/windows", "[]").status, 400);
});

test("the mail carries the entry and nothing can add a header", () => {
  const raw = asMail("from@summonvans.in", "to@example.com", { email: "asha@example.com", note: "line one\nline two" }, "id1");
  const cut = raw.indexOf("\r\n\r\n");
  const head = raw.slice(0, cut);
  const body = raw.slice(cut + 4);
  assert.match(head, /^Subject: VANS on Windows$/m);
  assert.match(head, /^Reply-To: <asha@example\.com>$/m);
  assert.equal(head.split("\r\n").length, 9);
  assert.match(body, /line one\r\nline two/);
});

test("the download goes to the VANS server's download, whatever the slashes", () => {
  assert.equal(downloadFrom("https://server.example/"), "https://server.example/v1/download");
  assert.equal(downloadFrom("https://server.example"), "https://server.example/v1/download");
  assert.equal(downloadFrom("  "), null);
});

test("only the address to go to is passed on, never cached, kept out of search", () => {
  const h = passOn(new Headers({ location: "https://s3.example/x", "set-cookie": "a=b", "cache-control": "max-age=60" }));
  assert.equal(h.get("location"), "https://s3.example/x");
  assert.equal(h.get("set-cookie"), null);
  assert.equal(h.get("cache-control"), "no-store");
  assert.equal(h.get("x-robots-tag"), "noindex, nofollow");
});
