import { useEffect } from "react";
import { Select } from "antd";
import { useTranslation } from "react-i18next";
import type { CanvasTheme } from "../lib/canvas-theme";
import { videoAdapterForModel, type AiConfig } from "./config";
import { ImageSettingsTheme } from "../components/image-settings-panel";
import { VideoSettingsPanel as OriginalSettings } from "../components/video-settings-panel";
import "./messages";

type Props = { config: AiConfig; onConfigChange: (key: "vquality" | "size" | "videoSeconds" | "videoGenerateAudio" | "videoWatermark" | "videoMode", value: string) => void; theme: CanvasTheme; showTitle?: boolean; className?: string };
export function VideoSettingsPanel(props: Props) {
  const { t } = useTranslation();
  const adapter = videoAdapterForModel(props.config.model || props.config.videoModel);
  const onConfigChange = props.onConfigChange;
  const seconds = props.config.videoSeconds;
  const size = props.config.size;
  useEffect(() => {
    if (adapter !== "sora") return;
    if (!["4", "8", "12"].includes(seconds)) onConfigChange("videoSeconds", "4");
    if (!["1280x720", "720x1280", "1792x1024", "1024x1792"].includes(size)) onConfigChange("size", "1280x720");
  }, [adapter, seconds, size, onConfigChange]);
  if (adapter !== "sora") return <OriginalSettings {...props} />;
  return <ImageSettingsTheme theme={props.theme}><div className={props.className || "w-[320px] space-y-4"}>
    <label className="block space-y-1"><span>{t("settingsPanels.video.seconds")}</span><Select className="w-full" aria-label={t("settingsPanels.video.seconds")} value={props.config.videoSeconds} options={[4, 8, 12].map((value) => ({ value: String(value), label: value + "s" }))} onChange={(value) => props.onConfigChange("videoSeconds", value)} /></label>
    <label className="block space-y-1"><span>{t("embedded.size")}</span><Select className="w-full" aria-label={t("embedded.size")} value={props.config.size} options={["1280x720", "720x1280", "1792x1024", "1024x1792"].map((value) => ({ value, label: value }))} onChange={(value) => props.onConfigChange("size", value)} /></label>
  </div></ImageSettingsTheme>;
}
