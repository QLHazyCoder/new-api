import { dataUrlToFile } from "../lib/image-utils";
import { computeVideoSize, inferVideoRatio } from "../lib/media-size";
import { imageToDataUrl } from "../services/image-storage";
import { videoAdapterForModel, type AiConfig } from "./config";
import type { ReferenceImage } from "../types/image";
import i18n from "./messages";

export async function canvasVideoBody(config: AiConfig, selection: string, prompt: string, references: ReferenceImage[], options?: { signal?: AbortSignal; videos?: unknown[]; audios?: unknown[] }): Promise<FormData | string> {
  const adapter = videoAdapterForModel(selection);
  if (adapter === "unsupported") throw new Error(i18n.t("embedded.videoUnsupported"));
  if (options?.videos?.length || options?.audios?.length) throw new Error(i18n.t("embedded.videoMediaUnsupported"));
  const seconds = Number(config.videoSeconds);
  if (!Number.isInteger(seconds) || seconds < 1 || seconds > 3600) throw new Error(i18n.t("apiErrors.videoTaskCreateFailed"));
  if (adapter === "sora" && ![4, 8, 12].includes(seconds)) throw new Error(i18n.t("embedded.soraDuration"));
  const ratio = inferVideoRatio(config.size);
  const size = /^\d+x\d+$/.test(config.size) ? config.size : computeVideoSize(config.vquality || "720", ratio === "auto" ? "16:9" : ratio);
  if (adapter === "doubao") {
    if (config.videoMode !== "reference" && references.length > 2) throw new Error(i18n.t("embedded.videoFramesLimit"));
    const content = await Promise.all(references.map(async (image, index) => {
      let role = "reference_image";
      if (config.videoMode !== "reference") role = index === 0 ? "first_frame" : "last_frame";
      return { type: "image_url", image_url: { url: await imageToDataUrl(image, options) }, role };
    }));
    let outputRatio = ratio === "auto" ? "16:9" : ratio;
    if (references.length && config.videoMode !== "reference") outputRatio = "adaptive";
    return JSON.stringify({ model: config.model, prompt, seconds, metadata: {
      content, ratio: outputRatio,
      resolution: (config.vquality || "720").replace(/p$/, "") + "p",
      generate_audio: config.videoGenerateAudio !== "false", watermark: config.videoWatermark === "true",
    } });
  }
  if (references.length > 1) throw new Error(i18n.t("embedded.videoSingleReference"));
  const form = new FormData();
  form.set("model", config.model);
  form.set("prompt", prompt);
  form.set("seconds", String(seconds));
  form.set("size", size);
  if (references[0]) {
    const image = references[0];
    form.append("input_reference", dataUrlToFile({ ...image, dataUrl: await imageToDataUrl(image, options) }));
  }
  return form;
}
