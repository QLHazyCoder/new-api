type ThemeName = "light" | "dark";
type Reply =
  | {
      type: "response" | "chunk" | "end" | "error";
      id: string;
      status?: number;
      headers?: Record<string, string>;
      data?: ArrayBuffer;
      message?: string;
    }
  | { type: "theme"; theme: ThemeName };
type Pending = {
  resolve: (response: Response) => void;
  reject: (error: Error) => void;
  controller?: ReadableStreamDefaultController<Uint8Array>;
  cleanup: () => void;
  awaitingPull?: boolean;
};

let port: MessagePort;
let canvasTheme: ThemeName = "light";
const themeListeners = new Set<(theme: ThemeName) => void>();
const pending = new Map<string, Pending>();
export const ready = new Promise<void>((resolve) => {
  window.addEventListener("message", function initialize(event: MessageEvent) {
    if (
      event.source !== window.parent ||
      event.data?.type !== "canvas:init" ||
      event.data.protocol !== 1 ||
      !event.ports[0]
    ) {
      return;
    }
    const parentOrigin = new URL(document.referrer).origin;
    if (event.origin !== parentOrigin || typeof event.data.nonce !== "string") return;
    if (event.data.theme === "light" || event.data.theme === "dark")
      canvasTheme = event.data.theme;
    window.removeEventListener("message", initialize);
    port = event.ports[0];
    port.onmessage = (message: MessageEvent<Reply>) => {
      const reply = message.data;
      if (reply?.type === "theme") {
        if (reply.theme !== "light" && reply.theme !== "dark") return;
        canvasTheme = reply.theme;
        themeListeners.forEach((listener) => listener(canvasTheme));
        return;
      }
      if (!reply?.id) return;
      const request = pending.get(reply.id);
      if (!request) return;
      if (reply.type === "response") {
        const body = new ReadableStream<Uint8Array>({
          start(controller) {
            request.controller = controller;
          },
          pull() {
            if (request.awaitingPull) {
              request.awaitingPull = false;
              port.postMessage({ type: "ack", id: reply.id });
            }
          },
          cancel() {
            port.postMessage({ type: "cancel", id: reply.id });
            request.cleanup();
            pending.delete(reply.id);
          },
        });
        request.resolve(new Response(body, { status: reply.status, headers: reply.headers }));
      } else if (reply.type === "chunk" && reply.data) {
        request.awaitingPull = true;
        request.controller?.enqueue(new Uint8Array(reply.data));
        if ((request.controller?.desiredSize ?? 0) > 0) {
          request.awaitingPull = false;
          port.postMessage({ type: "ack", id: reply.id });
        }
      } else if (reply.type === "end") {
        request.controller?.close();
        request.cleanup();
        pending.delete(reply.id);
      } else if (reply.type === "error") {
        const error = new Error(reply.message || "Request failed");
        if (request.controller) request.controller.error(error);
        else request.reject(error);
        request.cleanup();
        pending.delete(reply.id);
      }
    };
    port.start();
    port.postMessage({ type: "ready", nonce: event.data.nonce });
    resolve();
  });
});

export function reportCanvasFailure(): void {
  port?.postMessage({ type: "fatal" });
}

export function reportCanvasReady(): void {
  port.postMessage({ type: "loaded" });
}

export function getCanvasTheme(): ThemeName {
  return canvasTheme;
}

export function subscribeCanvasTheme(listener: (theme: ThemeName) => void): () => void {
  themeListeners.add(listener);
  return () => themeListeners.delete(listener);
}

export async function canvasRequest(
  operation: string,
  payload: Record<string, unknown> = {},
  signal?: AbortSignal,
): Promise<Response> {
  await ready;
  if (signal?.aborted) throw new DOMException("Aborted", "AbortError");
  const id = crypto.randomUUID();
  return new Promise<Response>((resolve, reject) => {
    const abort = () => {
      port.postMessage({ type: "cancel", id });
      const error = new DOMException("Aborted", "AbortError");
      const request = pending.get(id);
      request?.controller?.error(error);
      request?.cleanup();
      pending.delete(id);
      reject(error);
    };
    signal?.addEventListener("abort", abort, { once: true });
    pending.set(id, {
      resolve,
      reject,
      cleanup: () => signal?.removeEventListener("abort", abort),
    });
    port.postMessage({ type: "request", protocol: 1, id, operation, payload });
  });
}

export async function canvasData<T>(
  operation: string,
  payload: Record<string, unknown> = {},
): Promise<T> {
  const response = await canvasRequest(operation, payload);
  const mediaErrorKey = operation === "assets.upload" && typeof payload.run_id === "string" ? `media:${payload.run_id}` : "";
  const text = await response.text();
  type CanvasEnvelope = {
    success?: boolean;
    data?: T;
    message?: string;
    error?: { message?: string };
  };
  let result: CanvasEnvelope | null = null;
  if (text.trim()) {
    try {
      const parsed: unknown = JSON.parse(text);
      if (parsed && typeof parsed === "object" && !Array.isArray(parsed))
        result = parsed as CanvasEnvelope;
    } catch {
      throw new Error(`Canvas request failed (${response.status})`);
    }
  }
  if (!response.ok || !result?.success) {
    const error = new Error(
      result?.message || result?.error?.message || `Canvas request failed (${response.status})`,
    );
    window.dispatchEvent(new CustomEvent("canvas:save-error", { detail: mediaErrorKey ? { key: mediaErrorKey, message: error.message } : error.message }));
    throw error;
  }
  if (mediaErrorKey) window.dispatchEvent(new CustomEvent("canvas:saved", { detail: { key: mediaErrorKey } }));
  return result.data as T;
}
