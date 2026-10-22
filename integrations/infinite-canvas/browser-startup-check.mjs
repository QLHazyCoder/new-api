import assert from "node:assert/strict";
import http from "node:http";
import { pathToFileURL } from "node:url";

// Serve production builds with isolated API fixtures. No live account,
// credentials, database, or generation is used. Only public static GETs reach
// the candidate container; every API request is answered by this fixture.
const staticOrigin = new URL(process.env.CANVAS_STATIC_ORIGIN);
const { chromium } = await import(pathToFileURL(process.env.CANVAS_PLAYWRIGHT_MODULE).href);
const user = {
  id: 1,
  username: "canvas-startup-fixture",
  role: 100,
  group: "default",
  setting: "{}",
};
const bundle = {
  access_token: "isolated-fixture-not-a-real-token",
  token_type: "Bearer",
  access_expires_at: 2000000000,
  user,
  session: {
    sid: "fixture-session",
    current: true,
    login_method: "password",
    ip: "127.0.0.1",
    user_agent: "fixture",
    created_at: 1,
    last_active_at: 1,
    expires_at: 2000000000,
  },
};
const server = http.createServer(async (request, response) => {
  const pathname = new URL(request.url, "http://localhost").pathname;
  if (pathname.startsWith("/api/")) {
    let data = [];
    if (pathname === "/api/status") {
      data = { system_name: "Canvas Startup Test", logo: "/logo.png", version: "startup-fixture" };
    }
    if (pathname === "/api/setup") data = { status: true };
    if (pathname === "/api/user/auth/refresh") data = bundle;
    if (pathname === "/api/user/self") data = user;
    if (pathname === "/api/notice") data = "";
    if (pathname === "/api/canvas/bootstrap") data = { user_id: 1, models: [], groups: {} };
    if (pathname === "/api/canvas/projects") data = { items: [], next_offset: -1 };
    if (pathname.startsWith("/api/canvas/state/")) {
      data = request.method === "GET" ? null : { revision: 1 };
    }
    response.writeHead(200, { "Content-Type": "application/json" });
    response.end(JSON.stringify({ success: true, data }));
    return;
  }
  if (
    request.method !== "GET" ||
    !(
      ["/", "/canvas", "/logo.png", "/favicon.ico"].includes(pathname) ||
      pathname.startsWith("/static/") ||
      pathname.startsWith("/canvas-app/")
    )
  ) {
    response.writeHead(404).end();
    return;
  }
  try {
    const upstream = await fetch(new URL(request.url, staticOrigin), {
      headers: { "Accept-Encoding": "identity" },
      redirect: "manual",
      signal: AbortSignal.timeout(15_000),
    });
    const headers = Object.fromEntries(upstream.headers);
    for (const header of [
      "connection",
      "transfer-encoding",
      "content-encoding",
      "content-length",
    ]) {
      delete headers[header];
    }
    headers["cache-control"] = "no-store";
    response.writeHead(upstream.status, headers).end(Buffer.from(await upstream.arrayBuffer()));
  } catch {
    response.writeHead(502).end();
  }
});
await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
const origin = `http://127.0.0.1:${server.address().port}`;
let browser;
try {
  browser = await chromium.launch({
    executablePath: process.env.CANVAS_CHROMIUM_PATH,
    headless: true,
    args: ["--no-sandbox"],
  });
  for (const scenario of ["success", "module-failure", "bootstrap-stall", "bootstrap-failure"]) {
    const page = await browser.newPage({
      viewport: { width: 1440, height: 1000 },
      locale: "zh-CN",
    });
    const errors = [];
    page.on("pageerror", (error) => errors.push(error.message));
    try {
      if (scenario === "bootstrap-stall") await page.clock.install();
      const reachedBootstrap =
        scenario === "bootstrap-stall"
          ? page.waitForRequest("**/api/canvas/bootstrap", { timeout: 30_000 })
          : null;
      if (scenario === "module-failure") {
        await page.route("**/canvas-app/assets/start-*.js", (route) => route.abort("failed"));
      }
      if (scenario === "bootstrap-stall" || scenario === "bootstrap-failure") {
        await page.route("**/api/canvas/bootstrap", (route) => {
          if (scenario === "bootstrap-stall") return;
          return route.fulfill({
            status: 503,
            contentType: "application/json",
            body: JSON.stringify({ success: false, message: "Fixture initialization failure" }),
          });
        });
      }
      await page.goto(`${origin}/canvas`, { waitUntil: "domcontentloaded" });
      const frame = page.locator("iframe[title='无限画布']");
      const library = page
        .frameLocator("iframe[title='无限画布']")
        .getByRole("heading", { name: "无限画布", exact: true });
      if (scenario === "success") {
        await library.waitFor();
        assert.equal(await frame.getAttribute("aria-busy"), "false");
      } else {
        if (scenario === "bootstrap-stall") {
          await reachedBootstrap;
          assert.equal(await frame.getAttribute("aria-busy"), "true");
          await page.clock.fastForward(30_001);
        }
        await page.getByText("无法打开画布", { exact: true }).waitFor();
        assert.equal(await frame.isVisible(), false);
        await page.unrouteAll({ behavior: "ignoreErrors" });
        await page.getByRole("button", { name: "重试", exact: true }).click();
        await library.waitFor();
        assert.equal(await frame.getAttribute("aria-busy"), "false");
      }
      assert.deepEqual(errors, [], `${scenario}: unexpected page errors`);
      console.log(
        `PASS ${scenario}${scenario === "success" ? "" : ": failure shown, retry rendered canvas"}`,
      );
    } finally {
      await page.close();
    }
  }
} finally {
  await browser?.close();
  server.closeAllConnections();
  await new Promise((resolve) => server.close(resolve));
}
