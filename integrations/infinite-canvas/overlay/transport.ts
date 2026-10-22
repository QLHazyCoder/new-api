import axios, { AxiosError, AxiosHeaders } from "axios";
import { canvasData, canvasRequest } from "./bridge";

const nativeFetch = window.fetch.bind(window);
const imageSources = new Map<string, string>();

function relayOperation(
  path: string,
  method: string,
): { operation: string; id?: string; key?: string } {
  if (method === "POST") {
    const operations: Record<string, string> = {
      "/api/canvas/v1/responses": "responses.create",
      "/api/canvas/v1/images/generations": "images.generate",
      "/api/canvas/v1/images/edits": "images.edit",
      "/api/canvas/v1/videos": "videos.create",
      "/api/canvas/v1/audio/speech": "audio.speech",
    };
    if (operations[path]) return { operation: operations[path] };
  }
  if (method === "GET") {
    const video = path.match(/^\/api\/canvas\/v1\/videos\/([\w:-]+)(\/content)?$/);
    if (video) return { operation: video[2] ? "videos.content" : "videos.retrieve", id: video[1] };
    const asset = path.match(/^\/api\/canvas\/assets\/([\w:-]+)\/content$/);
    if (asset) return { operation: "assets.content", id: asset[1] };
    const artifact = path.match(
      /^\/api\/canvas\/v1\/tasks\/([\w:-]+)\/artifacts\/([\w:-]+)\/content$/,
    );
    if (artifact) return { operation: "artifacts.content", id: artifact[1], key: artifact[2] };
  }
  throw new Error("This request is not available in Infinite Canvas");
}

async function rememberImageSources(response: Response): Promise<void> {
  const run = response.headers.get("x-canvas-run-id");
  if (!run || !response.ok || !response.headers.get("content-type")?.includes("json")) return;
  const payload = await response.clone().json();
  if (Array.isArray(payload.data)) {
    for (const image of payload.data) {
      if (typeof image.url === "string") imageSources.set(image.url, run);
    }
  }
}

window.fetch = async (input: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
  const source = input instanceof Request ? input.url : String(input);
  if (source.startsWith("blob:") || source.startsWith("data:")) return nativeFetch(input, init);
  const registered = imageSources.get(source);
  if (registered) {
    const id = "image:" + crypto.randomUUID().replaceAll("-", "");
    await canvasData("assets.import", { asset_id: id, url: source, run_id: registered });
    return canvasRequest("assets.content", { id }, init?.signal ?? undefined);
  }
  const url = new URL(source, window.location.href);
  // Signed artifact URLs are converted to an owner-checked, session endpoint.
  const artifact = url.pathname.match(/\/v1\/tasks\/([\w:-]+)\/artifacts\/([\w:-]+)\/content$/);
  if (artifact)
    return canvasRequest(
      "artifacts.content",
      { id: artifact[1], key: artifact[2] },
      init?.signal ?? undefined,
    );
  if (url.origin !== new URL(document.referrer).origin)
    throw new Error("External URL import is disabled");
  const method = String(init?.method ?? "GET").toUpperCase();
  const target = relayOperation(url.pathname, method);
  const headers = new Headers(init?.headers);
  const payload: Record<string, unknown> = {
    ...target,
    group: headers.get("X-Canvas-Group") || "",
  };
  if (init?.body instanceof FormData) {
    payload.form = [...init.body.entries()].map(([key, value]) => [
      key,
      value,
      value instanceof File ? value.name : undefined,
    ]);
  } else if (typeof init?.body === "string") payload.body = JSON.parse(init.body);
  const response = await canvasRequest(target.operation, payload, init?.signal ?? undefined);
  if (target.operation.startsWith("images.")) await rememberImageSources(response);
  return response;
};

// Axios uses the exact same transport as fetch; its XHR adapter is never used.
axios.defaults.adapter = async (config) => {
  const headers = new Headers();
  const group = config.headers.get("X-Canvas-Group");
  if (group) headers.set("X-Canvas-Group", String(group));
  const response = await window.fetch(config.url!, {
    method: config.method?.toUpperCase(),
    body: config.data as BodyInit,
    headers,
    signal: config.signal as AbortSignal | undefined,
  });
  let data: unknown;
  if (config.responseType === "blob") data = await response.blob();
  else if (config.responseType === "arraybuffer") data = await response.arrayBuffer();
  else if (config.responseType === "text") data = await response.text();
  else data = await response.json();
  const result = {
    data,
    status: response.status,
    statusText: response.statusText,
    headers: new AxiosHeaders(Object.fromEntries(response.headers.entries())),
    config,
  };
  if (!response.ok) throw new AxiosError("Request failed", undefined, config, undefined, result);
  return result;
};
