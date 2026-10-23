import { nanoid } from "nanoid";
import { imageToDataUrl, uploadImage } from "../services/image-storage";
import { dataUrlToFile } from "../lib/image-utils";
import { buildImageReferencePromptText } from "../lib/image-reference-prompt";
import { buildImageEditFormData, buildImageGenerationPayload } from "./playground/image-payload-builder";
import { normalizePlaygroundImageConfig } from "./playground/image-generation-capabilities";
import type { ImageGenerationConfig, ImageModelCapabilities } from "./playground/types";
import { canvasHeaders, imageCapabilitiesForModel, resolveModelRequestConfig, type AiConfig } from "./config";
import i18n from "./messages";
import type { ReferenceImage } from "../types/image";
import { canvasData, canvasRequest } from "./bridge";

export type CanvasImageFailure = { error: string; httpStatus?: number; code?: string; requestId?: string; runId?: string };
const pendingGeneratedImages = new Map<string, string>();
export class CanvasImageError extends Error {
  constructor(message: string, public details: Omit<CanvasImageFailure, "error"> = {}) { super(message); }
}
export class CanvasImageSaveError extends Error {
  constructor(public runId: string, cause: unknown) {
    super(i18n.t("embedded.generatedSaveFailed") + ": " + (cause instanceof Error ? cause.message : String(cause)));
  }
}
export async function persistCanvasGeneratedImage(image: { dataUrl: string; runId?: string }, signal?: AbortSignal) {
  try {
    const saved = await uploadImage(image.dataUrl, { signal, sourceRunId: image.runId });
    if (image.runId) pendingGeneratedImages.delete(image.runId);
    return saved;
  }
  catch (error) {
    if (signal?.aborted) throw error;
    if (image.runId) pendingGeneratedImages.set(image.runId, image.dataUrl);
    throw new CanvasImageSaveError(image.runId || "", error);
  }
}
export function imageFailure(error: unknown): CanvasImageFailure {
  return { error: error instanceof Error ? error.message : String(error), ...(error instanceof CanvasImageError ? error.details : {}) };
}

export async function recoverGeneratedImage(runId: string): Promise<string> {
  const pending = pendingGeneratedImages.get(runId);
  if (pending) return pending;
  const response = await canvasRequest("runs.result", { id: runId });
  if (!response.ok) throw new CanvasImageError(i18n.t("embedded.httpFailed", { status: response.status }), { runId, httpStatus: response.status });
  type ImagePayload = { b64_json?: string; url?: string };
  const result = await response.json() as { data?: ImagePayload[] | ImagePayload };
  const image = Array.isArray(result.data) ? result.data[0] : result.data;
  if (image?.b64_json) {
    const signature = atob(image.b64_json.slice(0, 24));
    let mime = "image/png";
    if (signature.startsWith("\xff\xd8\xff")) mime = "image/jpeg";
    else if (signature.startsWith("RIFF") && signature.slice(8, 12) === "WEBP") mime = "image/webp";
    return `data:${mime};base64,${image.b64_json}`;
  }
  if (!image?.url) throw new CanvasImageError(i18n.t("apiErrors.noImageReturned"), { runId });
  const id = "image:run-" + runId;
  const existing = await canvasRequest("assets.content", { id });
  if (existing.ok) return URL.createObjectURL(await existing.blob());
  await existing.body?.cancel();
  await canvasData("assets.import", { asset_id: id, run_id: runId, url: image.url });
  const content = await canvasRequest("assets.content", { id });
  if (!content.ok) throw new CanvasImageError(i18n.t("embedded.httpFailed", { status: content.status }), { runId });
  return URL.createObjectURL(await content.blob());
}

export function canvasImageConfig(config: AiConfig, selection: string, capabilities: ImageModelCapabilities): ImageGenerationConfig {
  const resolved = resolveModelRequestConfig(config, selection);
  const selected = {
    model: resolved.model, group: resolved.canvasGroup || "", n: 1, size: config.size,
    aspect_ratio: config.imageAspectRatio, resolution: config.imageResolution, quality: config.quality,
    response_format: "b64_json", output_format: config.imageOutputFormat,
    moderation: config.imageModeration || "auto", output_compression: Number(config.imageOutputCompression || 100),
  } as ImageGenerationConfig;
  return normalizePlaygroundImageConfig(selected, capabilities);
}

let imageRequests = 0;
const waitingImages: Array<() => void> = [];
async function requestImages(config: AiConfig, prompt: string, references: ReferenceImage[], options?: { signal?: AbortSignal }) {
  await new Promise<void>((resolve) => {
    if (imageRequests < 2) { imageRequests++; resolve(); }
    else waitingImages.push(resolve);
  });
  try {
    if (options?.signal?.aborted) throw new DOMException("Aborted", "AbortError");
    return await performImageRequest(config, prompt, references, options);
  } finally {
    const next = waitingImages.shift();
    if (next) next();
    else imageRequests--;
  }
}

async function performImageRequest(config: AiConfig, prompt: string, references: ReferenceImage[], options?: { signal?: AbortSignal }) {
  const selection = config.model || config.imageModel;
  const capabilities = imageCapabilitiesForModel(selection);
  if (!capabilities) throw new Error(i18n.t("embedded.unavailable"));
  if (references.length && !capabilities.supports_editing) throw new Error(i18n.t("embedded.editingUnavailable"));
  const resolved = resolveModelRequestConfig(config, selection);
  const normalized = canvasImageConfig(config, selection, capabilities);
  const text = [config.systemPrompt.trim(), references.length ? buildImageReferencePromptText(prompt, references) : prompt].filter(Boolean).join("\n\n");
  let body: FormData | string;
  if (references.length) {
    const files = await Promise.all(references.map(async (image) => {
      const file = dataUrlToFile({ ...image, dataUrl: await imageToDataUrl(image, options) });
      return { ...image, size: file.size, file };
    }));
    const form = buildImageEditFormData(text, normalized, files, capabilities);
    form.delete("group");
    body = form;
  } else {
    const payload = buildImageGenerationPayload(text, normalized, capabilities);
    delete payload.group;
    body = JSON.stringify(payload);
  }
  const controller = new AbortController();
  const abort = () => controller.abort();
  let timedOut = false;
  if (options?.signal?.aborted) abort();
  else options?.signal?.addEventListener("abort", abort, { once: true });
  const timer = window.setTimeout(() => { timedOut = true; controller.abort(); }, 10 * 60_000);
  let details: Omit<CanvasImageFailure, "error"> = {};
  try {
    const response = await fetch("/api/canvas/v1/images/" + (references.length ? "edits" : "generations"), {
      method: "POST", body, headers: canvasHeaders(resolved, typeof body === "string" ? "application/json" : undefined), signal: controller.signal,
    });
    details = {
      httpStatus: response.status, requestId: response.headers.get("x-oneapi-request-id") || response.headers.get("x-request-id") || undefined,
      runId: response.headers.get("x-canvas-run-id") || undefined,
    };
    const raw = await response.text();
    let payload: { error?: { message?: string; code?: string }; message?: string; data?: Array<{ url?: string; b64_json?: string }> | { url?: string; b64_json?: string } };
    try { payload = JSON.parse(raw); } catch { throw new CanvasImageError(i18n.t("embedded.httpFailed", { status: response.status }), details); }
    if (!response.ok || payload?.error) {
      throw new CanvasImageError(payload?.error?.message || payload?.message || i18n.t("embedded.httpFailed", { status: response.status }), { ...details, code: payload?.error?.code });
    }
    const entries = Array.isArray(payload?.data) ? payload.data : [payload?.data];
    const images = entries.flatMap((entry) => {
      const format = normalized.output_format === "jpeg" ? "jpeg" : normalized.output_format || "png";
      const dataUrl = entry?.b64_json ? `data:image/${format};base64,${entry.b64_json}` : entry?.url;
      return dataUrl ? [{ id: nanoid(), dataUrl, runId: details.runId }] : [];
    });
    if (!images.length) throw new CanvasImageError(i18n.t("apiErrors.noImageReturned"), details);
    return images;
  } catch (error) {
    if (timedOut) throw new CanvasImageError(i18n.t("embedded.timeout"), details);
    throw error;
  } finally {
    window.clearTimeout(timer);
    options?.signal?.removeEventListener("abort", abort);
  }
}

export async function requestGeneration(config: AiConfig, prompt: string, options?: { signal?: AbortSignal }) {
  const count = Math.max(1, Math.min(10, Math.floor(Number(config.count) || 1)));
  const images: Awaited<ReturnType<typeof requestImages>> = [];
  // The workbench already queues individual n=1 slots. Canvas nodes can ask
  // for several images too; keep those requests bounded and never auto-retry.
  for (let index = 0; index < count; index++) images.push(...await requestImages(config, prompt, [], options));
  return images;
}
export async function requestEdit(config: AiConfig, prompt: string, references: ReferenceImage[], options?: { signal?: AbortSignal }) {
  const count = Math.max(1, Math.min(10, Math.floor(Number(config.count) || 1)));
  const images: Awaited<ReturnType<typeof requestImages>> = [];
  for (let index = 0; index < count; index++) images.push(...await requestImages(config, prompt, references, options));
  return images;
}
