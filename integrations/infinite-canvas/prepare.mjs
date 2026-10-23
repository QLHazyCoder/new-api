import { cpSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

// All writes target a disposable, exact-SHA upstream checkout created by build.sh.
const integration = dirname(fileURLToPath(import.meta.url));
const target = resolve(process.argv[2], "web");
const overlay = resolve(integration, "overlay");
cpSync(overlay, resolve(target, "src/embedded"), { recursive: true });
cpSync(resolve(overlay, "index.html"), resolve(target, "index.html"));

// Share the actual Playground normalization and request builders. The isolated
// application has its own React tree, but it must not invent image parameters.
const playground = resolve(target, "src/embedded/playground");
mkdirSync(playground, { recursive: true });
const hostPlayground = resolve(integration, "../../web/src/features/playground");
cpSync(resolve(hostPlayground, "types.ts"), resolve(playground, "types.ts"));
const imageCount = readFileSync(resolve(hostPlayground, "constants.ts"), "utf8").match(/export const PLAYGROUND_IMAGE_MAX_COUNT = (\d+)/);
if (!imageCount) throw new Error("Playground image count contract changed");
for (const name of ["image-generation-capabilities", "image-payload-builder"]) {
  let shared = readFileSync(resolve(hostPlayground, "lib", name + ".ts"), "utf8");
  shared = shared.replace("import { PLAYGROUND_IMAGE_MAX_COUNT } from '../constants'", "const PLAYGROUND_IMAGE_MAX_COUNT = " + imageCount[1]);
  shared = shared.replace("from '../types'", "from './types'");
  writeFileSync(resolve(playground, name + ".ts"), shared);
}

for (const file of ["image", "video", "audio"]) {
  const path = resolve(target, "src/services/api", file + ".ts");
  let source = readFileSync(path, "utf8");
  if (!source.includes("function aiHeaders("))
    throw new Error("Upstream request boundary changed: " + file);
  source = 'import { canvasHeaders } from "../../embedded/config";\n' + source;
  source = source.replace(/function aiHeaders\([\s\S]*?\n}/, "const aiHeaders = canvasHeaders;");
  source = source.replace(
    /^\s*if \(!config\.(apiKey|baseUrl)\.trim\(\)\) throw new Error\(apiText\("[^"]+"\)\);$/gm,
    "",
  );
  if (file === "video") {
    source = source.replace(
      /\s*const url = videoResultUrl\(video\);\s*if \(url\) return \{ status: "completed", result: await videoResultFromUrl\(url, options\) \};/,
      "",
    );
    const start = source.indexOf('    const images = await Promise.all(references.map(async (image) => dataUrlToFile', source.indexOf('async function createOpenAIVideoTask('));
    const end = source.indexOf('    try {\n        const created = unwrapVideoResponse', start);
    if (start < 0 || end < start) throw new Error("Upstream video request boundary changed");
    source = 'import { canvasVideoBody } from "../../embedded/video-request";\n' + source.slice(0, start) + '    const body = await canvasVideoBody(config, model, prompt, references, options);\n' + source.slice(end);
  }
  if (file === "image") {
    const start = source.indexOf("export async function requestGeneration(");
    const end = source.indexOf("export async function requestImageQuestion(");
    if (start < 0 || end < start) throw new Error("Upstream image API boundary changed");
    source = source.slice(0, start) + 'export { requestGeneration, requestEdit } from "../../embedded/image-api";\n\n' + source.slice(end);
  }
  writeFileSync(path, source);
}

const toolbarPath = resolve(target, "src/components/canvas/canvas-top-bar.tsx");
let toolbar = readFileSync(toolbarPath, "utf8");
toolbar = toolbar.replace(
  "<CompactAgentStatus status={compactAgentStatus} onClick={onToggleAgent} />",
  "",
);
toolbar = toolbar.replace(
  /<Button\s+type="text"[\s\S]*?onClick=\{onToggleAgent\}[\s\S]*?<\/Button>/,
  "",
);
writeFileSync(toolbarPath, toolbar);

const dockPath = resolve(target, "src/components/canvas/canvas-toolbar.tsx");
let dock = readFileSync(dockPath, "utf8");
const themeStart = dock.indexOf('                    <div className="px-1 pb-1.5 text-[11px] font-medium opacity-50">{t("canvas.toolbar.themeMode")}');
const themeEnd = dock.indexOf('                    <div className="mt-3 px-1 pb-1.5', themeStart);
if (themeStart < 0 || themeEnd < themeStart) throw new Error("Upstream canvas theme controls changed");
dock = dock.slice(0, themeStart) + dock.slice(themeEnd);
writeFileSync(dockPath, dock);

const projectStorePath = resolve(target, "src/stores/canvas/use-canvas-store.ts");
let projectStore = readFileSync(projectStorePath, "utf8");
projectStore = projectStore.replace('export type CanvasProject = {', 'export type CanvasProject = {\n    embeddedSummary?: boolean;');
writeFileSync(projectStorePath, projectStore);
const exportPath = resolve(target, "src/lib/canvas/canvas-export.ts");
let exportSource = readFileSync(exportPath, "utf8");
exportSource = 'import { loadCanvasProject } from "../../embedded/storage";\n' + exportSource;
exportSource = exportSource.replace('        projects.map(async (project) => {', '        projects.map(async (summary) => {\n            const project = summary.embeddedSummary ? await loadCanvasProject(summary.id) as CanvasProject : summary;');
writeFileSync(exportPath, exportSource);
const projectCardPath = resolve(target, "src/components/canvas/canvas-project-card.tsx");
let projectCard = readFileSync(projectCardPath, "utf8");
projectCard = projectCard.replace('{t("canvas.project.stats", { nodes: project.nodes.length, connections: project.connections.length })}', '{project.embeddedSummary ? "" : t("canvas.project.stats", { nodes: project.nodes.length, connections: project.connections.length })}');
writeFileSync(projectCardPath, projectCard);

const promptListPath = resolve(target, "src/components/prompts/use-prompt-list.ts");
let promptList = readFileSync(promptListPath, "utf8");
promptList = 'import { useAssetStore } from "@/stores/use-asset-store";\n' + promptList;
promptList = promptList.replace(
  "const [debouncedKeyword, setDebouncedKeyword]",
  'const assets = useAssetStore(state => state.assets);\n    const promptRevision = assets.filter(asset => asset.kind === "text").map(asset => asset.id + ":" + asset.updatedAt).join(",");\n    const [debouncedKeyword, setDebouncedKeyword]',
);
promptList = promptList.replace(
  '["prompts", debouncedKeyword, tags, category]',
  '["prompts", debouncedKeyword, tags, category, promptRevision]',
);
writeFileSync(promptListPath, promptList);

// This upstream editor is excluded from the embedded product, but is still in
// upstream's tsc include list. Ant Design 6 renamed the semantic content slot.
const editorPath = resolve(target, "src/components/layout/model-script-editor.tsx");
writeFileSync(editorPath, readFileSync(editorPath, "utf8").replace("content: {", "container: {"));

const imageStoragePath = resolve(target, "src/services/image-storage.ts");
let imageStorage = readFileSync(imageStoragePath, "utf8");
imageStorage = imageStorage.replace('type ImageReadOptions = { signal?: AbortSignal };', 'type ImageReadOptions = { signal?: AbortSignal; sourceRunId?: string };');
imageStorage = imageStorage.replace('        await store.setItem(storageKey, blob);', '        if (options?.sourceRunId) Object.assign(blob, { canvasRunId: options.sourceRunId });\n        await store.setItem(storageKey, blob);');
writeFileSync(imageStoragePath, imageStorage);

// Canvas keeps the selected group in its internal model ID for routing, but
// the picker label must remain the model name. Group descriptions are managed
// by the host and should never be concatenated into every option.
const modelStorePath = resolve(target, "src/stores/use-config-store.ts");
let modelStore = readFileSync(modelStorePath, "utf8");
const modelLabel = "return channel ? `${decoded.model}（${channel.name}）` : decoded.model;";
if (!modelStore.includes(modelLabel))
  throw new Error("Upstream model label boundary changed");
modelStore = modelStore.replace(modelLabel, "return decoded.model;");
modelStore = modelStore.replace('    canvasImageCount: string;', '    canvasImageCount: string;\n    imageAspectRatio: string;\n    imageResolution: string;\n    imageOutputFormat: string;\n    imageModeration: string;\n    imageOutputCompression: string;');
modelStore = modelStore.replace('    canvasImageCount: "3",', '    canvasImageCount: "3",\n    imageAspectRatio: "",\n    imageResolution: "",\n    imageOutputFormat: "",\n    imageModeration: "auto",\n    imageOutputCompression: "100",');
writeFileSync(modelStorePath, modelStore);

const modelPickerPath = resolve(target, "src/components/model-picker.tsx");
let modelPicker = readFileSync(modelPickerPath, "utf8");
if (!modelPicker.includes('return "/icons/'))
  throw new Error("Upstream model icon boundary changed");
modelPicker = modelPicker.replaceAll('return "/icons/', 'return "/canvas-app/icons/');
writeFileSync(modelPickerPath, modelPicker);

// A successful upstream generation must survive a separate local-asset
// failure. Retrying the failed upload must never submit a second paid request.
const imagePagePath = resolve(target, "src/pages/image/index.tsx");
let imagePage = readFileSync(imagePagePath, "utf8");
if (!imagePage.includes("    mimeType?: string;\n};"))
  throw new Error("Upstream generated image type boundary changed");
imagePage = imagePage.replace(
  "    mimeType?: string;\n};",
  "    mimeType?: string;\n    saveError?: string;\n    runId?: string;\n};",
);
imagePage = 'import { imageFailure, persistCanvasGeneratedImage, recoverGeneratedImage, type CanvasImageFailure } from "../../embedded/image-api";\nimport "../../embedded/messages";\n' + imagePage;
imagePage = imagePage.replace('    images: GeneratedImage[];\n};', '    images: GeneratedImage[];\n    failures?: CanvasImageFailure[];\n};');
imagePage = imagePage.replace('Pick<AiConfig, "model" | "imageModel" | "quality" | "size" | "count">', 'Pick<AiConfig, "model" | "imageModel" | "quality" | "size" | "count" | "imageAspectRatio" | "imageResolution" | "imageOutputFormat" | "imageModeration" | "imageOutputCompression">');
const imagePreferenceKeys = ["imageAspectRatio", "imageResolution", "imageOutputFormat", "imageModeration", "imageOutputCompression"];
imagePage = imagePage.replace('        count: config.count,\n    };', '        count: config.count,\n' + imagePreferenceKeys.map((key) => '        ' + key + ': config.' + key + ',').join('\n') + '\n    };');
imagePage = imagePage.replace('        count: log.config?.count || String(log.imageCount || log.successCount || 1),', '        count: log.config?.count || String(log.imageCount || log.successCount || 1),\n' + imagePreferenceKeys.map((key) => '        ' + key + ': log.config?.' + key + ' || "",').join('\n'));
imagePage = imagePage.replace('        setReferences(log.references || []);', '        setReferences(log.references || []);\n' + imagePreferenceKeys.map((key) => '        if (log.config.' + key + ') updateConfig("' + key + '", log.config.' + key + ');').join('\n'));
imagePage = imagePage.replace('    const agentTaskIdRef = useRef<string | undefined>(undefined);', '    const agentTaskIdRef = useRef<string | undefined>(undefined);\n    const failureDetails = useRef<CanvasImageFailure[]>([]);');
imagePage = imagePage.replace('        setElapsedMs(0);\n        setRunning(true);', '        failureDetails.current = [];\n        setElapsedMs(0);\n        setRunning(true);');
const batch = `        const tasks = Array.from({ length: generationCount }, (_, index) => runGenerationSlot(index, snapshot));

        const result = await Promise.allSettled(tasks);`;
if (!imagePage.includes(batch)) throw new Error("Upstream image batch boundary changed");
imagePage = imagePage.replace(batch, `        let nextSlot = 0;
        const result: PromiseSettledResult<GeneratedImage>[] = [];
        await Promise.all(Array.from({ length: Math.min(2, generationCount) }, async () => {
            while (nextSlot < generationCount) {
                const index = nextSlot++;
                try { result[index] = { status: "fulfilled", value: await runGenerationSlot(index, snapshot) }; }
                catch (reason) { result[index] = { status: "rejected", reason }; }
            }
        }));`);
const uploadBlock = `            const stored = await uploadImage(image.dataUrl);
            const nextImage: GeneratedImage = { id: image.id, dataUrl: stored.url, ...(stored.storageKey ? { storageKey: stored.storageKey } : {}), durationMs: performance.now() - itemStartedAt, width: stored.width, height: stored.height, bytes: stored.bytes, mimeType: stored.mimeType };
            setResults((value) => updateResultAt(value, index, { status: "success", image: nextImage }));
            return nextImage;`;
if (!imagePage.includes(uploadBlock))
  throw new Error("Upstream image save boundary changed");
const uploadReplacement = `            const generated: GeneratedImage = { id: image.id, runId: image.runId, dataUrl: image.dataUrl, durationMs: performance.now() - itemStartedAt, width: 0, height: 0, bytes: 0, mimeType: "" };
            try {
                const stored = await persistCanvasGeneratedImage(image);
                const nextImage: GeneratedImage = { ...generated, dataUrl: stored.url, ...(stored.storageKey ? { storageKey: stored.storageKey } : {}), width: stored.width, height: stored.height, bytes: stored.bytes, mimeType: stored.mimeType };
                setResults((value) => updateResultAt(value, index, { status: "success", image: nextImage }));
                return nextImage;
            } catch (error) {
                const nextImage: GeneratedImage = { ...generated, saveError: error instanceof Error ? error.message : t("workbench.generationFailed") };
                setResults((value) => updateResultAt(value, index, { status: "success", image: nextImage }));
                return nextImage;
            }`;
imagePage = imagePage.replace(uploadBlock, uploadReplacement);
const retryAnchor = `    const retryResult = async (index: number) => {
        const snapshot = buildRequestSnapshot();`;
if (!imagePage.includes(retryAnchor))
  throw new Error("Upstream image retry boundary changed");
const retrySave = `    const retryImageSave = async (index: number, image: GeneratedImage) => {
        try {
            const source = image.dataUrl || (image.runId ? await recoverGeneratedImage(image.runId) : "");
            if (!source) throw new Error(t("imageWorkbench.missingResult"));
            const stored = await persistCanvasGeneratedImage({ dataUrl: source, runId: image.runId });
            const nextImage: GeneratedImage = { ...image, dataUrl: stored.url, ...(stored.storageKey ? { storageKey: stored.storageKey } : {}), width: stored.width, height: stored.height, bytes: stored.bytes, mimeType: stored.mimeType, saveError: undefined };
            setResults((value) => value.map((result) => result.image?.id === image.id ? { ...result, status: "success", image: nextImage } : result));
            for (const log of logs) {
                if (log.images.some((entry) => entry.id === image.id)) {
                    await logStore.setItem(log.id, serializeLog({ ...log, images: log.images.map((entry) => entry.id === image.id ? nextImage : entry) }));
                }
            }
            await refreshLogs();
            message.success(t("workbench.retrySuccess"));
        } catch (error) {
            const nextImage = { ...image, saveError: error instanceof Error ? error.message : t("workbench.generationFailed") };
            setResults((value) => value.map((result) => result.image?.id === image.id ? { ...result, status: "success", image: nextImage } : result));
            message.error(t("workbench.generationFailed"));
        }
    };

    const retryResult = async (index: number) => {
        const existing = results[index];
        if (existing?.status === "success" && existing.image?.saveError) {
            await retryImageSave(index, existing.image);
            return;
        }
        const snapshot = buildRequestSnapshot();`;
imagePage = imagePage.replace(retryAnchor, retrySave);
const resultCall = "<ResultImageCard key={result.id} image={result.image} index={index} onEdit={addResultToReferences} onDownload={downloadImage} onSaveAsset={saveResultToAssets} />";
if (!imagePage.includes(resultCall))
  throw new Error("Upstream result card boundary changed");
imagePage = imagePage.replace(resultCall, "<ResultImageCard key={result.id} image={result.image} index={index} onEdit={addResultToReferences} onDownload={downloadImage} onSaveAsset={saveResultToAssets} savingDisabled={running} onRetrySave={(image) => retryImageSave(index, image)} />");
const resultProps = `    onSaveAsset: (image: GeneratedImage, index: number) => void;
}) {`;
if (!imagePage.includes(resultProps))
  throw new Error("Upstream result card props boundary changed");
imagePage = imagePage.replace(resultProps, `    onSaveAsset: (image: GeneratedImage, index: number) => void;
    onRetrySave?: (image: GeneratedImage) => Promise<void>;
    savingDisabled?: boolean;
}) {
    const [saving, setSaving] = useState(false);`);
imagePage = imagePage.replace('    onSaveAsset,\n}: {', '    onSaveAsset,\n    onRetrySave,\n    savingDisabled,\n}: {');
const resultActions = `                <div className="grid min-w-0 grid-cols-3 gap-2">
                    <Tooltip title={t("common.addToAssets")}>`;
if (!imagePage.includes(resultActions))
  throw new Error("Upstream result card action boundary changed");
imagePage = imagePage.replace(resultActions, `                {image.saveError ? (
                    <div className="mb-2 flex items-center justify-between gap-2 rounded-md border border-red-200 bg-red-50 px-2 py-1.5 text-xs text-red-600 dark:border-red-950 dark:bg-red-950/20 dark:text-red-300">
                        <div><div>{t("embedded.generatedSaveFailed")}</div><Typography.Paragraph ellipsis={{ rows: 2 }} className="!mb-0 !text-xs !text-inherit">{image.saveError}</Typography.Paragraph></div>
                        <Button size="small" danger disabled={savingDisabled} loading={saving} onClick={async () => { setSaving(true); try { await onRetrySave?.(image); } finally { setSaving(false); } }}>{t("embedded.retrySave")}</Button>
                    </div>
                ) : null}
                <div className="grid min-w-0 grid-cols-3 gap-2">
                    <Tooltip title={t("common.addToAssets")}>`);
imagePage = imagePage.replace('<span>\n                        {image.width}x{image.height}\n                    </span>\n                    <span>{formatBytes(image.bytes)}</span>', '{image.width && image.height ? <span>{image.width}x{image.height}</span> : null}\n                    {image.bytes ? <span>{formatBytes(image.bytes)}</span> : null}');
const saveLog = `    const saveLog = (log: GenerationLog) => {
        void logStore.setItem(log.id, serializeLog(log)).then(refreshLogs);
    };`;
if (!imagePage.includes(saveLog)) throw new Error("Upstream generation history write boundary changed");
imagePage = imagePage.replace(saveLog, `    const saveLog = async (log: GenerationLog) => {
        try {
            await logStore.setItem(log.id, serializeLog({ ...log, failures: [...failureDetails.current] }));
            await refreshLogs();
        } catch (error) {
            window.dispatchEvent(new CustomEvent("canvas:save-error", { detail: error instanceof Error ? error.message : t("workbench.generationFailed") }));
        }
    };`);
imagePage = imagePage.replaceAll('            saveLog(\n', '            await saveLog(\n');
imagePage = imagePage.replace('            setResults((value) => updateResultAt(value, index, { status: "failed", error:', '            failureDetails.current.push(imageFailure(error));\n            setResults((value) => updateResultAt(value, index, { status: "failed", error:');
imagePage = imagePage.replace('        setResults(log.images.map((image) => ({ id: image.id, status: "success", image })));', '        setResults([...log.images.map((image): GenerationResult => ({ id: image.id, status: "success", image })), ...(log.failures || []).map((failure): GenerationResult => ({ id: nanoid(), status: "failed", error: [failure.error, failure.requestId, failure.runId].filter(Boolean).join(" · ") }))]);');
imagePage = imagePage.replace('        images,\n    };\n}\n\nfunction serializeLog', '        images,\n        failures: log.failures,\n    };\n}\n\nfunction serializeLog');
imagePage = imagePage.replace('dataUrl: image.storageKey ? "" : image.dataUrl', 'dataUrl: image.storageKey || image.saveError && image.runId ? "" : image.dataUrl');
imagePage = imagePage.replace('return { ...item, dataUrl: await resolveImageUrl(item.storageKey, item.dataUrl) };\n        }),\n    );\n    const config = normalizeLogConfig', 'return { ...item, dataUrl: await resolveImageUrl(item.storageKey, item.dataUrl || (!item.storageKey && item.runId ? await recoverGeneratedImage(item.runId).catch(() => "") : "")) };\n        }),\n    );\n    const config = normalizeLogConfig');
imagePage = imagePage.replace('config={config} onConfigChange={(key, value) => updateConfig(key, value)} theme={theme} showTitle={false}', 'config={{ ...config, model }} onConfigChange={(key, value) => updateConfig(key, value)} theme={theme} showTitle={false}');
writeFileSync(imagePagePath, imagePage);

const videoPagePath = resolve(target, "src/pages/video/index.tsx");
let videoPage = readFileSync(videoPagePath, "utf8");
videoPage = videoPage.replace('<VideoSettingsPanel config={config}', '<VideoSettingsPanel config={{ ...config, model }}');
writeFileSync(videoPagePath, videoPage);

// Every image-producing canvas node uses the same save boundary as the
// workbench. Keep the failed save's Run-ID on the specific node/image slot.
const canvasTypesPath = resolve(target, "src/types/canvas.ts");
let canvasTypes = readFileSync(canvasTypesPath, "utf8");
canvasTypes = canvasTypes.replaceAll('    errorDetails?: string;', '    errorDetails?: string;\n    pendingImageRunId?: string;');
writeFileSync(canvasTypesPath, canvasTypes);
const canvasProjectPath = resolve(target, "src/pages/canvas/project.tsx");
let canvasProject = readFileSync(canvasProjectPath, "utf8");
canvasProject = 'import { CanvasImageSaveError, persistCanvasGeneratedImage, recoverGeneratedImage } from "../../embedded/image-api";\n' + canvasProject;
const imageSaveCall = 'await uploadImage(image.dataUrl, { signal: controller.signal })';
if (!canvasProject.includes(imageSaveCall)) throw new Error("Upstream canvas image save boundary changed");
canvasProject = canvasProject.replaceAll(imageSaveCall, 'await persistCanvasGeneratedImage(image, controller.signal)');
canvasProject = canvasProject.replaceAll('status: NODE_STATUS_ERROR, errorDetails }', 'status: NODE_STATUS_ERROR, errorDetails, ...(error instanceof CanvasImageSaveError ? { pendingImageRunId: error.runId } : {}) }');
canvasProject = canvasProject.replace('errorDetails: item.metadata?.content ? undefined : errorDetails,', 'errorDetails: item.metadata?.content ? undefined : errorDetails,\n                                      ...(error instanceof CanvasImageSaveError ? { pendingImageRunId: error.runId } : {}),');
const nodeRetry = '        async (node: CanvasNodeData, imageId?: string) => {\n            if (hasResumableVideoTask(node))';
if (!canvasProject.includes(nodeRetry)) throw new Error("Upstream canvas image retry boundary changed");
canvasProject = canvasProject.replace(nodeRetry, `        async (node: CanvasNodeData, imageId?: string) => {
            const pendingRun = imageId ? node.metadata?.images?.find((image) => image.id === imageId)?.pendingImageRunId : node.metadata?.pendingImageRunId;
            if (pendingRun) {
                try {
                    const dataUrl = await recoverGeneratedImage(pendingRun);
                    const uploaded = await persistCanvasGeneratedImage({ dataUrl, runId: pendingRun });
                    setNodes((previous) => previous.map((item) => {
                        if (item.id !== node.id) return item;
                        const primary = !imageId || !item.metadata?.content;
                        return { ...item, metadata: {
                            ...item.metadata, ...(primary ? imageMetadata(uploaded) : {}),
                            status: NODE_STATUS_SUCCESS, errorDetails: undefined, pendingImageRunId: undefined,
                            ...(primary && imageId ? { primaryImageId: imageId } : {}),
                            images: item.metadata?.images?.map((image) => image.id === imageId ? { ...image, content: uploaded.url, storageKey: uploaded.storageKey, naturalWidth: uploaded.width, naturalHeight: uploaded.height, bytes: uploaded.bytes, mimeType: uploaded.mimeType, status: NODE_STATUS_SUCCESS, errorDetails: undefined, pendingImageRunId: undefined } : image),
                        } };
                    }));
                } catch (error) { message.error(error instanceof Error ? error.message : t("workbench.generationFailed")); }
                return;
            }
            if (hasResumableVideoTask(node))`);
writeFileSync(canvasProjectPath, canvasProject);

// Generated build config has no plugin manifest discovery, analytics, agent or
// credentials initializer. Alias order matters: exact boundaries precede "@/".
writeFileSync(
  resolve(target, "vite.config.ts"),
  `
import { resolve } from "node:path";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";
export default defineConfig({
  base: "/canvas-app/",
  plugins: [react()],
  resolve: { alias: [
    {find:"@/components/model-picker",replacement:resolve(__dirname,"src/embedded/model-picker.tsx")},
    {find:"@/components/image-settings-panel",replacement:resolve(__dirname,"src/embedded/image-settings.ts")},
    {find:"@/components/video-settings-panel",replacement:resolve(__dirname,"src/embedded/video-settings.ts")},
    {find:"@/stores/use-config-store",replacement:resolve(__dirname,"src/embedded/config.ts")},
    {find:"@/services/api/prompts",replacement:resolve(__dirname,"src/embedded/prompts-api.ts")},
    {find:"@/lib/localforage-storage",replacement:resolve(__dirname,"src/embedded/localforage-storage.ts")},
    {find:"localforage",replacement:resolve(__dirname,"src/embedded/storage.ts")},
    ...["components/layout/client-root-init","components/layout/app-config-modal","components/agent/agent-panel","lib/canvas/plugin-loader","services/api/model-plugin"].map(name=>({find:"@/"+name,replacement:resolve(__dirname,"src/embedded/disabled.tsx")})),
    {find:/^\\.\\/model-plugin$/,replacement:resolve(__dirname,"src/embedded/disabled.tsx")},
    {find:"@/components/layout/user-status-actions",replacement:resolve(__dirname,"src/embedded/layout.tsx")},
    {find:"@",replacement:resolve(__dirname,"src")}
  ]},
  define: { __APP_VERSION__:JSON.stringify("v0.19.0 (new-api)"), __APP_RELEASES__:"[]" },
  build: { outDir: "dist", emptyOutDir: true }
});
`,
);
