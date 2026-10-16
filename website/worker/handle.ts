export interface Entry {
  email: string;
  note: string;
}

export type Verdict = { ok: true; entry: Entry } | { ok: false; status: number; why: string };

const MOST_BYTES = 2048;
const MOST_NOTE = 500;
const QUICKEST_HUMAN_MS = 2000;
const EMAIL = /^[^\s@<>"',;]+@[^\s@<>"',;]+\.[^\s@<>"',;]{2,}$/;

export function judge(method: string, path: string, body: string): Verdict {
  if (path !== "/api/windows") return { ok: false, status: 404, why: "not found" };
  if (method !== "POST") return { ok: false, status: 405, why: "post only" };
  if (new TextEncoder().encode(body).length > MOST_BYTES) return { ok: false, status: 413, why: "too large" };

  let sent: Record<string, unknown>;
  try {
    const parsed = JSON.parse(body);
    if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) throw new Error("not an object");
    sent = parsed as Record<string, unknown>;
  } catch {
    return { ok: false, status: 400, why: "not json" };
  }

  if (typeof sent.company === "string" && sent.company.trim() !== "") return { ok: false, status: 400, why: "trap" };
  if (typeof sent.waited !== "number" || sent.waited < QUICKEST_HUMAN_MS) return { ok: false, status: 400, why: "too quick" };

  const email = typeof sent.email === "string" ? sent.email.trim() : "";
  if (email.length > 254 || !EMAIL.test(email)) return { ok: false, status: 400, why: "email" };

  const note = (typeof sent.note === "string" ? sent.note : "").replace(/[\u0000-\u0008\u000b\u000c\u000e-\u001f]/g, "").trim();
  if (note.length > MOST_NOTE) return { ok: false, status: 400, why: "note too long" };

  return { ok: true, entry: { email, note } };
}

export function asMail(from: string, to: string, entry: Entry, id: string): string {
  const body = [`Email: ${entry.email}`, "", entry.note ? `Note:\n${entry.note}` : "No note."].join("\n");
  return [
    `From: VANS site <${from}>`,
    `To: <${to}>`,
    `Reply-To: <${entry.email}>`,
    "Subject: VANS on Windows",
    `Message-ID: <${id}@summonvans.in>`,
    `Date: ${new Date().toUTCString()}`,
    "MIME-Version: 1.0",
    "Content-Type: text/plain; charset=utf-8",
    "Content-Transfer-Encoding: 8bit",
    "",
    body.replace(/\r?\n/g, "\r\n"),
    "",
  ].join("\r\n");
}

export function downloadFrom(server: string): string | null {
  const base = server.trim().replace(/\/+$/, "");
  return base ? `${base}/v1/download` : null;
}

const PASSED_ON = ["content-type", "location"];

export function passOn(from: Headers): Headers {
  const out = new Headers();
  for (const name of PASSED_ON) {
    const value = from.get(name);
    if (value !== null) out.set(name, value);
  }
  out.set("cache-control", "no-store");
  out.set("x-robots-tag", "noindex, nofollow");
  return out;
}
