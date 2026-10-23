import { useEffect, useMemo } from "react";
import { InputNumber, Select } from "antd";
import { useTranslation } from "react-i18next";
import type { CanvasTheme } from "../lib/canvas-theme";
import { imageCapabilitiesForModel, type AiConfig } from "./config";
import { canvasImageConfig } from "./image-api";
import { ImageSettingsTheme } from "../components/image-settings-panel";
import "./messages";

type ConfigKey = "quality" | "size" | "count" | "background" | "imageAspectRatio" | "imageResolution" | "imageOutputFormat" | "imageModeration" | "imageOutputCompression";
type Props = {
  config: AiConfig;
  onConfigChange: (key: ConfigKey, value: string) => void;
  theme: CanvasTheme;
  showTitle?: boolean;
  className?: string;
  maxCount?: number;
  quickCount?: number;
};

export function ImageSettingsPanel(props: Props) {
  const { t } = useTranslation();
  const selection = props.config.model || props.config.imageModel;
  const capabilities = imageCapabilitiesForModel(selection);
  const config = props.config;
  const onConfigChange = props.onConfigChange;
  const normalized = useMemo(() => capabilities ? canvasImageConfig(config, selection, capabilities) : null, [config, selection, capabilities]);
  // Keep summaries, node snapshots and the actual request in agreement after
  // changing model. The normalizer is copied verbatim from Playground.
  useEffect(() => {
    if (!normalized) return;
    const values: Partial<Record<ConfigKey, string>> = {
      size: normalized.size, quality: normalized.quality, imageAspectRatio: normalized.aspect_ratio,
      imageResolution: normalized.resolution, imageOutputFormat: normalized.output_format || "",
    };
    for (const [key, value] of Object.entries(values)) {
      const configKey = key as ConfigKey;
      if (config[configKey] !== value) onConfigChange(configKey, value);
    }
  }, [config, normalized, onConfigChange]);
  if (!capabilities || !normalized) return <p role="status">{t("embedded.unavailable")}</p>;
  const fields: Array<{ key: ConfigKey; label: string; options: string[]; value: string }> = [];
  if (capabilities.size_mode === "dimensions") {
    fields.push({ key: "size", label: "size", options: capabilities.sizes, value: normalized.size });
  }
  if (capabilities.size_mode === "aspect_ratio_resolution") {
    fields.push({ key: "imageAspectRatio", label: "ratio", options: capabilities.aspect_ratios, value: normalized.aspect_ratio });
    fields.push({ key: "imageResolution", label: "resolution", options: capabilities.resolutions, value: normalized.resolution });
  }
  fields.push({ key: "quality", label: "quality", options: capabilities.qualities, value: normalized.quality });
  fields.push({ key: "imageOutputFormat", label: "format", options: capabilities.output_formats, value: normalized.output_format || "" });
  if (capabilities.supports_moderation) {
    fields.push({ key: "imageModeration", label: "moderation", options: ["auto", "low"], value: props.config.imageModeration || "auto" });
  }
  return (
    <ImageSettingsTheme theme={props.theme}>
      <div className={props.className || "w-[320px] space-y-4"} onPointerDown={(event) => event.stopPropagation()} onMouseDown={(event) => event.stopPropagation()}>
        {fields.filter((field) => field.options.length > 0).map((field) => (
          <label key={field.key} className="block space-y-1 text-sm">
            <span>{t("embedded." + field.label)}</span>
            <Select aria-label={t("embedded." + field.label)} className="w-full" value={field.value}
              options={field.options.map((value) => ({ value, label: value === "auto" ? t("embedded.auto") : value }))}
              onChange={(value) => props.onConfigChange(field.key, value)} />
          </label>
        ))}
        {capabilities.supports_output_compression && normalized.output_format !== "png" ? (
          <label className="block space-y-1 text-sm"><span>{t("embedded.compression")}</span>
            <InputNumber aria-label={t("embedded.compression")} min={0} max={100} className="w-full"
              value={Number(props.config.imageOutputCompression || 100)}
              onChange={(value) => props.onConfigChange("imageOutputCompression", String(value ?? 100))} />
          </label>
        ) : null}
        <label className="block space-y-1 text-sm"><span>{t("embedded.count")}</span>
          <InputNumber aria-label={t("embedded.count")} min={1} max={Math.min(props.maxCount || 10, 10)} precision={0}
            value={Math.max(1, Math.min(Number(props.config.count) || 1, 10))} className="w-full"
            onChange={(value) => props.onConfigChange("count", String(value || 1))} />
        </label>
      </div>
    </ImageSettingsTheme>
  );
}
