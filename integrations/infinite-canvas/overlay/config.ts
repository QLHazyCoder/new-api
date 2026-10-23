import { useMemo } from "react";
import {
  useConfigStore,
  modelOptionName,
  selectableModelsByCapability,
  type AiConfig,
  type ModelCapability,
} from "../stores/use-config-store";
import { canvasData } from "./bridge";
import storage from "./storage";
import type { ImageModelCapabilities } from "./playground/types";
export type { ImageModelCapabilities } from "./playground/types";

export * from "../stores/use-config-store";
export { useConfigStore };
type SessionConfig = AiConfig & { canvasGroup?: string };
const channelGroups = new Map<string, string>();
const imageCapabilities = new Map<string, ImageModelCapabilities>();
const videoAdapters = new Map<string, string>();
export type Bootstrap = {
  user_id: number;
  models: Array<{
    name: string;
    group: string;
    capability: ModelCapability;
    image_capabilities?: ImageModelCapabilities;
    video_adapter?: string;
  }>;
};

export async function initializeConfig(): Promise<Bootstrap> {
  const bootstrap = await canvasData<Bootstrap>("bootstrap");
  const config = useConfigStore.getState().config;
  channelGroups.clear();
  imageCapabilities.clear();
  videoAdapters.clear();
  const groups = [...new Set(bootstrap.models.map((item) => item.group))];
  config.channels = groups.flatMap((group) =>
    (["image", "video", "text", "audio"] as const).flatMap((capability) => {
      const models = bootstrap.models.filter(
        (item) => item.group === group && item.capability === capability,
      );
      if (!models.length) return [];
      const id = "g-" + encodeURIComponent(group) + "-" + capability;
      channelGroups.set(id, group);
      return [
        {
          id,
          // The group remains a routing/authorization identity. It is not a
          // model description and must not be appended to every model label.
          name: group,
          baseUrl: "",
          apiKey: "",
          apiFormat: "openai" as const,
          models: models.map((item) => {
            const value = id + "::" + item.name;
            if (item.image_capabilities) imageCapabilities.set(value, item.image_capabilities);
            if (item.video_adapter) videoAdapters.set(value, item.video_adapter);
            return { name: item.name, capability };
          }),
        },
      ];
    }),
  );
  config.models = config.channels.flatMap((channel) =>
    channel.models.map((model) => channel.id + "::" + model.name),
  );
  config.baseUrl = "";
  config.apiKey = "";
  config.proxyEnabled = false;
  config.proxyUrl = "";
  config.apiFormat = "openai";
  config.imageModel = selectableModelsByCapability(config, "image")[0] || "";
  config.videoModel = selectableModelsByCapability(config, "video")[0] || "";
  config.textModel = selectableModelsByCapability(config, "text")[0] || "";
  config.audioModel = selectableModelsByCapability(config, "audio")[0] || "";
  config.model = config.imageModel;
  const update = useConfigStore.getState().updateConfig;
  useConfigStore.setState({
    config: { ...config },
    isAiConfigReady: (value, model) => value.models.includes(model),
    importChannelCredentials: () => {
      throw new Error("Canvas credentials are managed by new-api");
    },
    updateConfig: (key, value) => {
      if (
        [
          "channels",
          "baseUrl",
          "apiKey",
          "proxyEnabled",
          "proxyUrl",
          "apiFormat",
          "channelMode",
          "models",
        ].includes(key)
      )
        throw new Error("Canvas credentials are managed by new-api");
      update(key, value);
    },
    openConfigDialog: () =>
      window.dispatchEvent(
        new CustomEvent("canvas:save-error", {
          detail: "当前账户没有此操作可用的模型，请检查分组与模型权限",
        }),
      ),
    updateWebdavConfig: () => {
      throw new Error("External synchronization is disabled");
    },
    setConfigDialogOpen: () => undefined,
  });
  const preferenceKeys = [
    "model",
    "imageModel",
    "videoModel",
    "textModel",
    "audioModel",
    "audioVoice",
    "audioFormat",
    "audioSpeed",
    "audioInstructions",
    "videoSeconds",
    "vquality",
    "videoGenerateAudio",
    "videoWatermark",
    "videoMode",
    "systemPrompt",
    "reasoningEffort",
    "quality",
    "size",
    "background",
    "count",
    "canvasImageCount",
    "imageAspectRatio",
    "imageResolution",
    "imageOutputFormat",
    "imageModeration",
    "imageOutputCompression",
  ] as const;
  const saved = await storage.getItem<{ preferences: Partial<AiConfig> }>("canvas:preferences");
  // Initial business-data failures must reach the host's retry state; an
  // empty UI after a failed hydration must never look like an empty account.
  await Promise.all([
    storage.getItem<string>("infinite-canvas:canvas_store"),
    storage.getItem<string>("infinite-canvas:asset_store"),
  ]);
  if (saved) {
    const clean = { ...useConfigStore.getState().config };
    for (const key of preferenceKeys) {
      const value = saved.preferences[key];
      if (typeof value !== "string") continue;
      if (key.endsWith("Model") || key === "model") {
        if (clean.models.includes(value))
          (clean as unknown as Record<string, unknown>)[key] = value;
      } else (clean as unknown as Record<string, unknown>)[key] = value;
    }
    useConfigStore.setState({ config: clean });
  }
  let timer: ReturnType<typeof setTimeout> | undefined;
  const persist = () => {
    clearTimeout(timer);
    timer = setTimeout(() => {
      const current = useConfigStore.getState().config;
      const preferences = Object.fromEntries(preferenceKeys.map((key) => [key, current[key]]));
      void storage
        .setItem("canvas:preferences", { preferences })
        .catch((error: Error) =>
          window.dispatchEvent(new CustomEvent("canvas:save-error", { detail: error.message })),
        );
    }, 500);
  };
  useConfigStore.subscribe(persist);
  return bootstrap;
}

export function resolveModelRequestConfig(config: AiConfig, value: string): SessionConfig {
  const separator = value.indexOf("::");
  const group = separator < 0 ? "" : value.slice(0, separator);
  const model = modelOptionName(value);
  if (
    !config.channels.some(
      (channel) => channel.id === group && channel.models.some((item) => item.name === model),
    )
  )
    throw new Error("Model not found");
  return {
    ...config,
    model,
    baseUrl: "",
    apiKey: "",
    apiFormat: "openai",
    canvasGroup: channelGroups.get(group),
  };
}

export function imageCapabilitiesForModel(value: string): ImageModelCapabilities | undefined {
  return imageCapabilities.get(value);
}

export function videoAdapterForModel(value: string): string {
  return videoAdapters.get(value) || "unsupported";
}

export function canvasHeaders(config: SessionConfig, contentType?: string): Record<string, string> {
  const group = config.canvasGroup || "";
  return {
    // HTTP headers are ASCII-only in both fetch and Axios. Keep the original
    // group in the in-app model value, and encode it only at this boundary.
    "X-Canvas-Group": group ? encodeURIComponent(group) : "",
    ...(contentType ? { "Content-Type": contentType } : {}),
  };
}

export function buildApiUrl(_base: string, path: string): string {
  return "/api/canvas/v1" + path;
}
export function withLocalProxy(url: string): string {
  return url;
}
export function resolveModelScript(): string {
  return "";
}
export function resolveModelForCapability(
  config: AiConfig,
  current: string | undefined,
  capability: ModelCapability,
): string {
  const options = selectableModelsByCapability(config, capability);
  return current && options.includes(current) ? current : options[0] || "";
}
export function useEffectiveConfig() {
  const config = useConfigStore((state) => state.config);
  return useMemo(() => ({ ...config, channelMode: "local" as const }), [config]);
}
