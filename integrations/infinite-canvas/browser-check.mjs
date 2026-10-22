import { pathToFileURL } from "node:url";

// Install playwright-core in an isolated test directory, then pass its module
// path and the opt-in Go harness URL. No production service or key is used.
const { chromium } = await import(pathToFileURL(process.env.CANVAS_PLAYWRIGHT_MODULE).href);
const browser = await chromium.launch({
  executablePath: process.env.CANVAS_CHROMIUM_PATH,
  headless: true,
  args: ["--no-sandbox"],
});
const page = await browser.newPage({ viewport: { width: 1440, height: 1000 } });
const errors = [];
if (process.env.CANVAS_BROWSER_VERBOSE === "1") {
  page.on("response", async (response) => {
    if (response.url().includes("/api/canvas/projects"))
      console.log(
        response.request().method(),
        response.status(),
        response.url(),
        await response.text(),
      );
  });
}
page.on("pageerror", (error) => errors.push(error.message));
page.on("console", (message) => {
  if (message.type() === "error") errors.push(message.text());
});
const url = process.argv[2];
try {
  await page.goto(url);
  await page.waitForFunction(() => window.bridgeReady === true);
  const canvas = page.frameLocator("#canvas");
  await canvas.getByRole("button", { name: "新建画布", exact: true }).first().click();
  await page.waitForFunction(async () => {
    const result = await (await fetch("/api/canvas/projects")).json();
    return result.data.items.length === 1;
  });
  await page.reload();
  await canvas.getByText("无限画布 1", { exact: true }).first().waitFor();
  await canvas.getByRole("link", { name: "生图工作台", exact: true }).click();
  await canvas
    .getByPlaceholder("描述画面主体、风格、构图、光线和用途")
    .fill("中文🙂 浏览器图像生成验证");
  await canvas.getByRole("button", { name: "开始生成", exact: true }).click();
  const image = canvas.getByRole("img", { name: "生成结果 1", exact: true });
  await image.waitFor();
  await image.evaluate(async (element) => {
    await element.decode();
    if (element.naturalWidth !== 1) throw new Error("Generated image is invalid");
  });
  await page.waitForFunction(async () => {
    const result = await (await fetch("/api/canvas/state/image_generation_logs")).json();
    return result.data.length === 1;
  });
  await page.reload();
  await canvas.getByRole("link", { name: "生图工作台", exact: true }).click();
  await canvas
    .getByText(/中文🙂 浏览器/)
    .first()
    .click();
  await image.waitFor();
  await image.evaluate(async (element) => {
    await element.decode();
    if (element.naturalWidth !== 1) throw new Error("Saved image did not survive reload");
  });
  await canvas.getByRole("link", { name: "视频创作台", exact: true }).click();
  await canvas.getByRole("link", { name: "我的资产", exact: true }).click();
  await canvas.getByRole("link", { name: "提示词库", exact: true }).click();
  await canvas.getByRole("button", { name: "生成记录", exact: true }).click();
  await canvas.getByRole("button", { name: "读取结果", exact: true }).first().waitFor();
  // Close overlays using their standard accessible action.
  await canvas.getByRole("button", { name: "Close", exact: true }).click();
  const list = await (await page.request.get(url + "/api/canvas/projects")).json();
  const project = list.data.items[0];
  const deletion = await page.request.delete(
    url + "/api/canvas/projects/" + project.project_id + "?revision=" + project.revision,
  );
  if (!deletion.ok()) throw new Error("Project deletion failed");
  await page.reload();
  await canvas.getByRole("button", { name: "回收站", exact: true }).click();
  // Ant Design inserts spacing between two Chinese button characters.
  await canvas.getByRole("button", { name: /^恢\s*复$/ }).click();
  await page.waitForFunction(async () => {
    const result = await (await fetch("/api/canvas/projects")).json();
    return result.data.items.length === 1;
  });
  if (errors.length) throw new Error(errors.join("\n"));
  console.log(
    "Canvas browser checks passed: opaque sandbox, production bridge, project reload, image generation/archive/reload, navigation, run history, trash restore; no console errors",
  );
} catch (error) {
  console.error(errors.join("\n"));
  for (const frame of page.frames()) {
    console.error(
      frame.url(),
      await frame
        .locator("body")
        .innerText()
        .catch(() => ""),
    );
  }
  if (process.env.CANVAS_BROWSER_SCREENSHOT)
    await page.screenshot({ path: process.env.CANVAS_BROWSER_SCREENSHOT });
  throw error;
} finally {
  await browser.close();
}
