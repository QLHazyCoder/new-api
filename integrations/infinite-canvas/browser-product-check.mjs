import assert from "node:assert/strict";
import http from "node:http";
import { readFile } from "node:fs/promises";
import { join } from "node:path";
import { pathToFileURL } from "node:url";

// All APIs are isolated local fixtures. The host is bundled from the actual
// production bridge; no production identity, data or paid generation is used.

const staticRoot = process.env.CANVAS_STATIC_ROOT;
const staticOrigin = process.env.CANVAS_STATIC_ORIGIN ? new URL(process.env.CANVAS_STATIC_ORIGIN) : null;
const hostBundle = await readFile(process.env.CANVAS_HOST_BUNDLE);
const { chromium } = await import(pathToFileURL(process.env.CANVAS_PLAYWRIGHT_MODULE).href);
const expectedGroup = "Codex特惠";
const encodedGroup = encodeURIComponent(expectedGroup);
let generationRequests = 0;
let assetUploads = 0;
let lastGroupHeader = "";
const settings = new Map();
const assets = new Map();
let emptyGenerationFailure = false;
let projectReads = 0;
let videoRequests = 0;
let firstHistoryWrite = true;
let releaseHistory;
const historyGate = new Promise((resolve) => { releaseHistory = resolve; });
const timestamp = Date.now();
const project = { id: "existing", title: "保留旧画布", createdAt: new Date(timestamp).toISOString(), updatedAt: new Date(timestamp).toISOString(), nodes: [{ id: "text-one", type: "text", title: "中文旧内容", position: { x: 0, y: 0 }, width: 200, height: 100, metadata: { content: "不可丢失的节点" } }], connections: [], chatSessions: [], activeChatId: null, backgroundMode: "lines", showImageInfo: false, viewport: { x: 0, y: 0, k: 1 } };
let projectRevision = 1;
const png = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=";
const fixtureModels = [
  { name: "gpt-image-2", group: expectedGroup, capability: "image", image_capabilities: { provider: "openai", size_mode: "dimensions", sizes: ["1024x1024", "1024x1536"], aspect_ratios: [], resolutions: [], qualities: ["auto", "high"], output_formats: ["png", "jpeg"], default_size: "1024x1024", supports_editing: true, supports_moderation: false, supports_output_compression: true, max_images: 2 } },
  { name: "gpt-image-2", group: "GPT-Pro-正价", capability: "image", image_capabilities: { provider: "gemini", size_mode: "aspect_ratio_resolution", sizes: [], aspect_ratios: ["1:1", "16:9"], resolutions: ["1K", "2K"], qualities: [], output_formats: [], supports_editing: false, supports_moderation: false, supports_output_compression: false, max_images: 1 } },
  { name: "video-sora", group: expectedGroup, capability: "video", video_adapter: "sora" },
];

function envelope(data) {
  return JSON.stringify({ success: true, data });
}

function send(response, status, body, type = "application/json") {
  response.writeHead(status, { "Content-Type": type, "Cache-Control": "no-store", "Access-Control-Allow-Origin": "*" });
  response.end(Buffer.isBuffer(body) || typeof body === "string" ? body : JSON.stringify(body));
}

const server = http.createServer(async (request, response) => {
  const url = new URL(request.url, "http://127.0.0.1");
  if (url.pathname === "/canvas-test-host.js") {
    response.writeHead(200, { "Content-Type": "text/javascript", "Cache-Control": "no-store" });
    response.end(hostBundle);
    return;
  }
  if (url.pathname.startsWith("/api/")) {
    if (url.pathname === "/api/canvas/bootstrap") {
      send(response, 200, envelope({
        user_id: 1,
        groups: { [expectedGroup]: "不应出现在模型标签中" },
        models: fixtureModels,
      }));
      return;
    }
    if (url.pathname === "/api/canvas/projects" && request.method === "GET") {
      send(response, 200, envelope({ items: [{ project_id: project.id, title: project.title, revision: projectRevision, created_at: timestamp, updated_at: timestamp }], next_offset: -1 }));
      return;
    }
    if (url.pathname === "/api/canvas/projects/existing" && request.method === "GET") {
      projectReads++;
      send(response, 200, envelope({ project: { project_id: project.id, revision: projectRevision }, document: project }));
      return;
    }
    if (url.pathname === "/api/canvas/projects/existing" && request.method === "PATCH") {
      let raw = "";
      for await (const chunk of request) raw += chunk;
      const payload = JSON.parse(raw);
      assert.equal(payload.document.nodes[0].metadata.content, "不可丢失的节点");
      Object.assign(project, payload.document);
      projectRevision++;
      send(response, 200, envelope({ project_id: project.id, revision: projectRevision }));
      return;
    }
    if (url.pathname.startsWith("/api/canvas/state/") && request.method === "GET") {
      const parts = url.pathname.split("/").filter(Boolean);
      if (parts.length === 4) send(response, 200, envelope([...settings.entries()].filter(([key]) => key.startsWith(url.pathname + "/")).map(([, value]) => value)));
      else send(response, 200, envelope(settings.get(url.pathname) || null));
      return;
    }
    if (url.pathname.startsWith("/api/canvas/state/") && request.method === "PUT") {
      let raw = "";
      for await (const chunk of request) raw += chunk;
      const payload = JSON.parse(raw);
      if (url.pathname.includes("/image_generation_logs/") && firstHistoryWrite) {
        firstHistoryWrite = false;
        await historyGate;
      }
      const record = { key: decodeURIComponent(url.pathname.split("/").at(-1)), revision: payload.revision + 1, value: JSON.stringify(payload.value) };
      settings.set(url.pathname, record);
      send(response, 200, envelope(record));
      return;
    }
    if (url.pathname === "/api/canvas/assets" && request.method === "GET") {
      send(response, 200, envelope({ items: [], next_offset: -1 }));
      return;
    }
    if (url.pathname === "/api/canvas/v1/images/generations" && request.method === "POST") {
      generationRequests += 1;
      lastGroupHeader = request.headers["x-canvas-group"] || "";
      let raw = "";
      for await (const chunk of request) raw += chunk;
      const body = JSON.parse(raw);
      assert.equal(body.n, 1);
      assert.equal(body.size, "1024x1024");
      assert.equal(body.output_format, "png");
      if (emptyGenerationFailure) { send(response, 403, "", "text/plain"); return; }
      response.setHeader("X-Canvas-Run-ID", "fixture-run-" + generationRequests);
      send(response, 200, { created: 1, data: [{ b64_json: png }] });
      return;
    }
    if (url.pathname === "/api/canvas/v1/videos" && request.method === "POST") {
      let raw = "";
      for await (const chunk of request) raw += chunk;
      assert.match(raw, /name="input_reference"/);
      assert.doesNotMatch(raw, /name="first_frame"|name="image\[\]"/);
      assert.match(raw, /name="seconds"\r\n\r\n4/);
      videoRequests++;
      send(response, 200, { id: "fixturevideo", status: "queued" });
      return;
    }
    if (url.pathname === "/api/canvas/v1/videos/fixturevideo") {
      send(response, 200, { id: "fixturevideo", status: "failed", error: { message: "Fixture video stops after checking request fields" } });
      return;
    }
    if (/^\/api\/canvas\/runs\/fixture-run-\d+\/result$/.test(url.pathname)) {
      send(response, 200, { created: 1, data: [{ b64_json: png }] });
      return;
    }
    if (url.pathname.startsWith("/api/canvas/assets/") && url.pathname.endsWith("/content") && request.method === "PUT") {
      assetUploads += 1;
      if (assetUploads === 1) {
        send(response, 503, "", "text/plain");
        return;
      }
      const chunks = [];
      for await (const chunk of request) chunks.push(chunk);
      assets.set(url.pathname, Buffer.concat(chunks));
      send(response, 200, envelope({ asset_id: url.pathname.split("/").at(-2), mime: "image/png" }));
      return;
    }
    if (url.pathname.startsWith("/api/canvas/assets/") && request.method === "GET") {
      const data = assets.get(url.pathname);
      if (data) send(response, 200, data, "image/png");
      else send(response, 404, envelope(null));
      return;
    }
    send(response, 200, envelope(null));
    return;
  }
  if (url.pathname === "/") {
    send(response, 200, '<!doctype html><html><body><script type="module" src="/canvas-test-host.js"></script></body></html>', "text/html");
    return;
  }
  if (url.pathname.startsWith("/canvas-app/")) {
    response.setHeader("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' blob: data:; media-src blob: data:; font-src 'self' data:; connect-src blob: data:; frame-ancestors 'self'; base-uri 'none'; form-action 'none'");
    const relative = url.pathname.slice("/canvas-app/".length) || "index.html";
    try {
      if (staticOrigin) {
        const upstream = await fetch(new URL(request.url, staticOrigin), { signal: AbortSignal.timeout(15_000) });
        send(response, upstream.status, Buffer.from(await upstream.arrayBuffer()), upstream.headers.get("content-type") || "application/octet-stream");
        return;
      }
      const body = await readFile(join(staticRoot, relative));
      const type = relative.endsWith(".html") ? "text/html" : relative.endsWith(".css") ? "text/css" : "text/javascript";
      send(response, 200, body, type);
    } catch {
      send(response, 404, "");
    }
    return;
  }
  send(response, 404, "");
});
await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
const origin = `http://127.0.0.1:${server.address().port}`;
const browser = await chromium.launch({ executablePath: process.env.CANVAS_CHROMIUM_PATH, headless: true, args: ["--no-sandbox"] });
const page = await browser.newPage({ viewport: { width: 1440, height: 1000 } });
const errors = [];
page.on("pageerror", (error) => errors.push(error.message));
page.on("console", (message) => { if (message.type() === "error" && !/status of (403|404|503)/.test(message.text())) errors.push(message.text()); });
try {
  await page.goto(origin, { waitUntil: "domcontentloaded" });
  const frame = page.frameLocator("iframe#canvas");
  await frame.getByRole("heading", { name: "无限画布", exact: true }).waitFor({ timeout: 30_000 });
  assert.equal(projectReads, 0, "opening the library must not fetch full documents");
  assert.equal(await frame.locator("html").evaluate((element) => element.classList.contains("dark")), false);
  await page.evaluate(() => window.canvasSetTheme("dark"));
  await frame.locator("html.dark").waitFor();
  await page.evaluate(() => window.canvasSetTheme("light"));
  await frame.locator("html:not(.dark)").waitFor();
  await frame.getByRole("button", { name: "浅色", exact: true }).count().then((count) => assert.equal(count, 0));
  await frame.getByRole("button", { name: "重命名", exact: true }).first().click();
  await frame.locator('article input:not([type="checkbox"])').fill("重命名后保留内容");
  const renamed = page.waitForResponse((response) => response.request().method() === "PATCH" && response.url().endsWith("/api/canvas/projects/existing"));
  await frame.getByRole("button", { name: "保存名称", exact: true }).click();
  assert.equal((await renamed).status(), 200);
  assert.equal(project.nodes[0].metadata.content, "不可丢失的节点");
  assert.equal(projectReads, 1);
  await frame.getByRole("link", { name: "生图工作台", exact: true }).click();
  const picker = frame.locator(".canvas-model-picker-text");
  await picker.getByText("gpt-image-2", { exact: true }).waitFor();
  assert.equal(await picker.innerText(), "gpt-image-2");
  assert.equal(await frame.getByText("不应出现在模型标签中", { exact: true }).count(), 0);
  await frame.getByRole("combobox", { name: "分组", exact: true }).click();
  await frame.getByText("GPT-Pro-正价", { exact: true }).last().click();
  await frame.getByRole("combobox", { name: "宽高比", exact: true }).waitFor();
  await frame.getByRole("combobox", { name: "分组", exact: true }).click();
  await frame.getByText(expectedGroup, { exact: true }).last().click();
  await frame.getByRole("combobox", { name: "尺寸", exact: true }).waitFor();
  await frame.getByPlaceholder("描述画面主体、风格、构图、光线和用途").fill("中文分组请求验证");
  const historyWritten = page.waitForResponse((response) => response.request().method() === "PUT" && response.url().includes("/api/canvas/state/image_generation_logs/"));
  void historyWritten.catch(() => undefined);
  await frame.getByRole("button", { name: "开始生成", exact: true }).click();
  await frame.getByRole("img", { name: "生成结果 1", exact: true }).waitFor({ timeout: 30_000 });
  assert.equal(lastGroupHeader, encodedGroup);
  assert.equal(generationRequests, 1);
  await frame.getByText("图片已生成，保存失败", { exact: true }).waitFor();
  assert.equal(await frame.getByRole("button", { name: "重试保存", exact: true }).isDisabled(), true, "saving must wait for the original history write");
  releaseHistory();
  assert.equal((await historyWritten).status(), 200);
  await page.reload({ waitUntil: "domcontentloaded" });
  await frame.getByRole("link", { name: "生图工作台", exact: true }).click();
  await frame.getByText("中文分组请求验证", { exact: true }).first().click();
  await frame.getByRole("img", { name: "生成结果 1", exact: true }).waitFor();
  const retry = frame.getByRole("button", { name: "重试保存", exact: true }).last();
  await retry.click();
  await frame.getByText("图片已生成，保存失败", { exact: true }).waitFor({ state: "hidden" });
  await frame.getByText("Canvas request failed (503)", { exact: true }).waitFor({ state: "hidden" });
  assert.equal(generationRequests, 1, "save retry must not submit another generation");
  assert.equal(assetUploads, 2);
  await page.reload({ waitUntil: "domcontentloaded" });
  await frame.getByRole("link", { name: "生图工作台", exact: true }).click();
  await frame.getByText("中文分组请求验证", { exact: true }).first().click();
  await frame.getByRole("img", { name: "生成结果 1", exact: true }).waitFor();
  assert.equal(generationRequests, 1, "reloading a saved result must not regenerate");
  await frame.getByRole("img", { name: "生成结果 1", exact: true }).evaluate(async (element) => {
    await element.decode();
    if (element.naturalWidth !== 1) throw new Error("Saved image bytes did not survive reload");
  });
  emptyGenerationFailure = true;
  await frame.getByRole("button", { name: "开始生成", exact: true }).click();
  await frame.getByText("请求失败（HTTP 403）", { exact: true }).first().waitFor();
  assert.equal(await frame.getByText(/Unexpected end of JSON/).count(), 0);
  await frame.getByRole("link", { name: "视频创作台", exact: true }).click();
  await frame.getByRole("heading", { name: "视频创作台", exact: true }).waitFor();
  const videoPrompt = frame.getByPlaceholder("描述镜头运动、主体动作、场景氛围和画面风格", { exact: true });
  await videoPrompt.fill("视频请求契约验证");
  assert.equal(await videoPrompt.inputValue(), "视频请求契约验证");
  await frame.locator('input[type="file"]').first().setInputFiles({ name: "ref.png", mimeType: "image/png", buffer: Buffer.from(png, "base64") });
  await frame.getByRole("img", { name: "ref.png", exact: true }).waitFor();
  assert.equal(await videoPrompt.inputValue(), "视频请求契约验证", "the video prompt must survive reference upload");
  await frame.getByRole("button", { name: "开始生成", exact: true }).click();
  await frame.getByText("Fixture video stops after checking request fields", { exact: true }).first().waitFor();
  assert.equal(videoRequests, 1);
  assert.deepEqual(errors, []);
  console.log(JSON.stringify({ result: "PASS", generationRequests, assetUploads, lastGroupHeader, projectReads, videoRequests, themeSync: true }));
} catch (error) {
  console.error(error.message);
  const frameDetails = [];
  for (const child of page.frames()) {
    frameDetails.push({ url: child.url(), body: await child.locator("body").innerText().catch(() => "") });
  }
  console.error(JSON.stringify({ errors, pageUrl: page.url(), frames: frameDetails }));
  throw error;
} finally {
  releaseHistory();
  await browser.close();
  server.closeAllConnections();
  await new Promise((resolve) => server.close(resolve));
}
