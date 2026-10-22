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
import { useThemeStore } from "../stores/use-theme-store";

export * from "../stores/use-config-store";
export { useConfigStore };
type SessionConfig = AiConfig & { canvasGroup?: string };
const channelGroups = new Map<string, string>();
export type Bootstrap = {
  user_id: number;
  models: Array<{ name: string; group: string; capability: ModelCapability }>;
  groups: Record<string, string>;
};

export async function initializeConfig(): Promise<Bootstrap> {
  const bootstrap = await canvasData<Bootstrap>("bootstrap");
  const config = useConfigStore.getState().config;
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
          name: bootstrap.groups[group] || group,
          baseUrl: "",
          apiKey: "",
          apiFormat: "openai" as const,
          models: models.map((item) => ({ name: item.name, capability })),
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
  ] as const;
  const saved = await storage.getItem<{ preferences: Partial<AiConfig>; theme: "light" | "dark" }>(
    "canvas:preferences",
  );
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
    if (saved.theme === "light" || saved.theme === "dark")
      useThemeStore.getState().setTheme(saved.theme);
  }
  let timer: ReturnType<typeof setTimeout> | undefined;
  const persist = () => {
    clearTimeout(timer);
    timer = setTimeout(() => {
      const current = useConfigStore.getState().config;
      const preferences = Object.fromEntries(preferenceKeys.map((key) => [key, current[key]]));
      void storage
        .setItem("canvas:preferences", { preferences, theme: useThemeStore.getState().theme })
        .catch((error: Error) =>
          window.dispatchEvent(new CustomEvent("canvas:save-error", { detail: error.message })),
        );
    }, 500);
  };
  useConfigStore.subscribe(persist);
  useThemeStore.subscribe(persist);
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

export function canvasHeaders(config: SessionConfig, contentType?: string): Record<string, string> {
  return {
    "X-Canvas-Group": config.canvasGroup || "",
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
