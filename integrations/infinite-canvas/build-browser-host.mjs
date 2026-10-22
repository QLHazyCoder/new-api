import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const root = dirname(fileURLToPath(import.meta.url));
const result = await Bun.build({
  entrypoints: [resolve(root, "browser-host.ts")],
  target: "browser",
  plugins: [
    {
      name: "canvas-fixture-auth",
      setup(build) {
        build.onResolve({ filter: /^@\/lib\/auth-session$/ }, () => ({
          path: "fixture-auth",
          namespace: "canvas-test",
        }));
        build.onLoad({ filter: /.*/, namespace: "canvas-test" }, () => ({
          contents:
            'export async function getFreshAuthHeaders(){return {"Content-Type":"application/json"}}',
          loader: "js",
        }));
      },
    },
  ],
});
if (!result.success) throw new Error(result.logs.join("\n"));
await Bun.write(process.argv[2], result.outputs[0]);
