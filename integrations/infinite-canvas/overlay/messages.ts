import i18n from "../i18n";

const messages = {
  "zh-CN": {
    group: "分组", model: "模型", unavailable: "当前模型没有可用的图片能力配置",
    size: "尺寸", ratio: "宽高比", resolution: "分辨率", quality: "质量", format: "输出格式",
    moderation: "内容审核", compression: "输出压缩", count: "生成总张数", auto: "自动",
    generatedSaveFailed: "图片已生成，保存失败", retrySave: "重试保存", saving: "正在保存",
    editingUnavailable: "当前模型不支持参考图编辑", httpFailed: "请求失败（HTTP {{status}}）",
    timeout: "请求超时，请查看生成记录，勿重复提交", requestId: "请求 ID", runId: "运行 ID",
    projectLoadFailed: "无法读取画布，请重试", retry: "重试", loading: "正在读取画布",
    videoUnsupported: "该分组的视频协议不明确，请使用协议一致的分组", videoMediaUnsupported: "当前视频接口尚不支持私有参考视频或音频，请移除这些输入",
    soraDuration: "Sora 时长必须为 4、8 或 12 秒", videoFramesLimit: "首尾帧模式最多支持两张图片", videoSingleReference: "当前视频协议只支持一张 input_reference 参考图",
  },
  "en-US": {
    group: "Group", model: "Model", unavailable: "Image capabilities are unavailable for this model",
    size: "Size", ratio: "Aspect ratio", resolution: "Resolution", quality: "Quality", format: "Output format",
    moderation: "Moderation", compression: "Output compression", count: "Total images", auto: "Auto",
    generatedSaveFailed: "Image generated, but saving failed", retrySave: "Retry saving", saving: "Saving",
    editingUnavailable: "This model does not support reference image editing", httpFailed: "Request failed (HTTP {{status}})",
    timeout: "Request timed out. Check generation history before submitting again", requestId: "Request ID", runId: "Run ID",
    projectLoadFailed: "Unable to load canvas. Please retry", retry: "Retry", loading: "Loading canvas",
    videoUnsupported: "The video protocol is ambiguous in this group. Select a group with one protocol", videoMediaUnsupported: "Private reference video or audio is not supported by this video interface. Remove those inputs",
    soraDuration: "Sora duration must be 4, 8 or 12 seconds", videoFramesLimit: "First/last frame mode supports at most two images", videoSingleReference: "This video protocol supports a single input_reference image",
  },
};
for (const [locale, values] of Object.entries(messages))
  i18n.addResourceBundle(locale, "translation", { embedded: values }, true, true);

export default i18n;
