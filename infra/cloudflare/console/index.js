export default {
  async fetch(request, env) {
    const url = new URL(request.url);
    if (url.protocol !== "https:") {
      url.protocol = "https:";
      return Response.redirect(url.toString(), 301);
    }
    const target = new URL(env.ORIGIN);
    target.pathname = "/console" + (url.pathname === "/" ? "" : url.pathname);
    target.search = url.search;

    const headers = new Headers(request.headers);
    headers.delete("host");
    headers.set("X-Vans-Console-Gate", env.CONSOLE_GATE);

    const answer = await fetch(target, {
      method: request.method,
      headers,
      body: request.method === "GET" || request.method === "HEAD" ? undefined : request.body,
      redirect: "manual",
    });
    const response = new Response(answer.body, answer);
    response.headers.set("X-Robots-Tag", "noindex, nofollow");
    response.headers.set("Strict-Transport-Security", "max-age=31536000");
    return response;
  },
};
