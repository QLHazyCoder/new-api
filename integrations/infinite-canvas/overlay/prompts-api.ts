import { useAssetStore } from "../stores/use-asset-store";
import type { Prompt, PromptListResponse } from "../services/api/prompts";
export type {
  Prompt,
  PromptListResponse,
  PromptSourceStatus,
  PromptSourceRefreshResult,
  PromptSourceRefreshSummary,
} from "../services/api/prompts";
export { formatPromptDate } from "../services/api/prompts";

export const ALL_PROMPTS_OPTION = "all";

// The embedded prompt library is the current user's text assets. External
// feeds and WebDAV are excluded, so these are the same account-owned records
// used by the canvas asset picker and export.
export async function fetchPrompts({
  keyword = "",
  tag = [],
  category = ALL_PROMPTS_OPTION,
  page = 1,
  pageSize = 20,
}: {
  keyword?: string;
  tag?: string[];
  category?: string;
  page?: number;
  pageSize?: number;
} = {}): Promise<PromptListResponse> {
  const all = useAssetStore.getState().assets.filter((asset) => asset.kind === "text");
  const items: Prompt[] = all
    .map((asset) => ({
      id: asset.id,
      title: asset.title,
      prompt: asset.data.content,
      description: asset.note || "",
      coverUrl: "",
      referenceImageUrls: [],
      tags: asset.tags,
      preview: "",
      createdAt: asset.createdAt,
      updatedAt: asset.updatedAt,
      sourceId: "account",
      category: "我的提示词",
      githubUrl: "",
    }))
    .filter(
      (item) =>
        (!keyword || (item.title + "\n" + item.prompt).includes(keyword)) &&
        (!tag.length || tag.every((entry) => item.tags.includes(entry))) &&
        (category === ALL_PROMPTS_OPTION || category === item.category),
    );
  return {
    items: items.slice((page - 1) * pageSize, page * pageSize),
    total: items.length,
    categories: ["我的提示词"],
    tags: [...new Set(all.flatMap((asset) => asset.tags))],
  };
}

export const fetchSourcePrompts = async (): Promise<Prompt[]> => [];
export const fetchPromptSourceStatuses = async () => ({});
export const refreshSource = async () => {
  throw new Error("External prompt sources are disabled");
};
export const refreshAllSources = async () => ({
  results: [],
  total: 0,
  successCount: 0,
  failureCount: 0,
});
export const refreshDueSources = refreshAllSources;
