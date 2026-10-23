import { Select } from "antd";
import { useTranslation } from "react-i18next";
import { modelOptionName, type AiConfig, type ModelCapability } from "./config";
import "./messages";

type Props = {
  config: AiConfig;
  value?: string;
  onChange: (model: string) => void;
  capability?: ModelCapability;
  className?: string;
  fullWidth?: boolean;
  placeholder?: string;
  onMissingConfig?: () => void;
};

// Group and model are separate choices, matching the host's selection model.
// Model IDs keep their original group prefix so old projects retain routing.
export function ModelPicker(props: Props) {
  const { t } = useTranslation();
  const channels = props.config.channels.filter((channel) =>
    channel.models.some((model) => !props.capability || model.capability === props.capability),
  );
  const selected = channels.find((channel) => props.value?.startsWith(channel.id + "::"));
  const model = modelOptionName(props.value || "");
  const channel = selected || channels[0];
  const models = channel?.models.filter((item) => !props.capability || item.capability === props.capability) || [];
  return (
    <div className={"flex min-w-0 flex-wrap gap-2 " + (props.className || "")}
      onPointerDown={(event) => event.stopPropagation()} onMouseDown={(event) => event.stopPropagation()}>
      <Select
        aria-label={t("embedded.group")}
        className="min-w-[8rem] flex-1"
        value={channel?.id}
        options={channels.map((item) => ({ value: item.id, label: item.name }))}
        showSearch={{ optionFilterProp: "label" }}
        onChange={(id) => {
          const next = channels.find((item) => item.id === id);
          if (!next) return;
          const available = next.models.filter((item) => !props.capability || item.capability === props.capability);
          const nextModel = available.find((item) => item.name === model) || available[0];
          if (nextModel) props.onChange(next.id + "::" + nextModel.name);
        }}
        onOpenChange={(open) => { if (open && !channels.length) props.onMissingConfig?.(); }}
      />
      <Select
        aria-label={t("embedded.model")}
        className="canvas-model-picker-text min-w-[9rem] flex-1"
        value={models.some((item) => item.name === model) ? model : undefined}
        placeholder={props.placeholder || t("embedded.model")}
        options={models.map((item) => ({ value: item.name, label: item.name }))}
        showSearch={{ optionFilterProp: "label" }}
        onChange={(name) => { if (channel) props.onChange(channel.id + "::" + name); }}
      />
    </div>
  );
}
