import { cpSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

// All writes target a disposable, exact-SHA upstream checkout created by build.sh.
const integration = dirname(fileURLToPath(import.meta.url));
const target = resolve(process.argv[2], "web");
const overlay = resolve(integration, "overlay");
cpSync(overlay, resolve(target, "src/embedded"), { recursive: true });
cpSync(resolve(overlay, "index.html"), resolve(target, "index.html"));

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
