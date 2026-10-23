// Browser test adapter only. The bundler replaces the authentication import
// with fixture headers; every operation uses the production host bridge.
import { createCanvasBridge } from "../../web/src/features/canvas/bridge";

const frame = document.createElement("iframe");
frame.id = "canvas";
frame.sandbox.add("allow-scripts", "allow-downloads");
frame.style.width = "100vw";
frame.style.height = "100vh";
frame.addEventListener("load", () => {
  const channel = new MessageChannel();
  const nonce = crypto.randomUUID();
  channel.port1.onmessage = (event) => {
    if (event.data?.type !== "ready" || event.data.nonce !== nonce) return;
    createCanvasBridge(channel.port1, () => {
      throw new Error("Canvas child failed");
    });
    (window as Window & { canvasSetTheme?: (theme: "light" | "dark") => void }).canvasSetTheme = (theme) => channel.port1.postMessage({ type: "theme", theme });
    (window as Window & { bridgeReady?: boolean }).bridgeReady = true;
  };
  channel.port1.start();
  frame.contentWindow?.postMessage({ type: "canvas:init", protocol: 1, nonce, theme: "light" }, "*", [
    channel.port2,
  ]);
});
document.body.append(frame);
frame.src = "/canvas-app/index.html#/canvas";
