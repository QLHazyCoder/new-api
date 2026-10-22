type Reply = {
  type: string;
  id: string;
  status?: number;
  headers?: Record<string, string>;
  data?: ArrayBuffer;
  message?: string;
};
type Pending = {
  resolve: (response: Response) => void;
  reject: (error: Error) => void;
  controller?: ReadableStreamDefaultController<Uint8Array>;
  cleanup: () => void;
  awaitingPull?: boolean;
};

let port: MessagePort;
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
    window.removeEventListener("message", initialize);
    port = event.ports[0];
    port.onmessage = (message: MessageEvent<Reply>) => {
      const reply = message.data;
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
  const result = await response.json();
  if (!response.ok || !result.success) {
    const error = new Error(result.message || result.error?.message || "Request failed");
    window.dispatchEvent(new CustomEvent("canvas:save-error", { detail: error.message }));
    throw error;
  }
  return result.data as T;
}
