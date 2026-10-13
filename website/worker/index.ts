import { EmailMessage } from "cloudflare:email";
import { asMail, judge } from "./handle";

interface Env {
  ASSETS: { fetch(request: Request): Promise<Response> };
  LIST_MAIL: { send(message: EmailMessage): Promise<void> };
  LIST_TO?: string;
}

const FROM = "windows-list@summonvans.in";

function say(status: number, ok: boolean): Response {
  return new Response(JSON.stringify({ ok }), {
    status,
    headers: { "content-type": "application/json", "cache-control": "no-store" },
  });
}

export default {
  async fetch(request: Request, env: Env): Promise<Response> {
    const url = new URL(request.url);
    if (!url.pathname.startsWith("/api/")) return env.ASSETS.fetch(request);

    const verdict = judge(request.method, url.pathname, await request.text());
    if (!verdict.ok) return say(verdict.status, false);

    const to = (env.LIST_TO ?? "").trim();
    if (!to) {
      console.error("LIST_TO is not set, so the entry could not be delivered");
      return say(503, false);
    }

    try {
      const raw = asMail(FROM, to, verdict.entry, crypto.randomUUID());
      await env.LIST_MAIL.send(new EmailMessage(FROM, to, raw));
      return say(200, true);
    } catch (err) {
      console.error("could not send the entry", err);
      return say(502, false);
    }
  },
};
