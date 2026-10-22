import { canvasData, canvasRequest } from "./bridge";

type Project = { project_id: string; revision: number };
type Setting = { key: string; value: string; revision: number };
type Asset = { asset_id: string };
type Page<T> = { items: T[]; next_offset: number };
const canvasKey = "infinite-canvas:canvas_store";
const projectRevisions = new Map<string, number>();
const documents = new Map<string, string>();
let writeQueue: Promise<unknown> = Promise.resolve();
let projectLoad: Promise<string> | undefined;

export async function restoreCanvasProject(id: string): Promise<Record<string, unknown>> {
  const result = await canvasData<{ project: Project; document: Record<string, unknown> }>(
    "projects.restore",
    { id },
  );
  projectRevisions.set(id, result.project.revision);
  documents.set(id, JSON.stringify(result.document));
  return result.document;
}

async function loadProjects(): Promise<string> {
  const projects: unknown[] = [];
  let offset = 0;
  do {
    const page = await canvasData<Page<Project>>("projects.list", { offset });
    for (const item of page.items) {
      const result = await canvasData<{ project: Project; document: Record<string, unknown> }>(
        "projects.get",
        { id: item.project_id },
      );
      projectRevisions.set(item.project_id, result.project.revision);
      documents.set(item.project_id, JSON.stringify(result.document));
      projects.push(result.document);
    }
    offset = page.next_offset;
  } while (offset >= 0);
  return JSON.stringify({ state: { projects, deletedProjects: [] }, version: 0 });
}

async function saveProjects(value: string): Promise<void> {
  const { state } = JSON.parse(value) as {
    state: {
      projects: Array<Record<string, unknown> & { id: string }>;
      deletedProjects: Array<{ id: string }>;
    };
  };
  for (const project of state.projects) {
    const encoded = JSON.stringify(project);
    if (documents.get(project.id) === encoded) continue;
    const saved = await canvasData<Project>("projects.save", {
      id: project.id,
      revision: projectRevisions.get(project.id) ?? 0,
      document: project,
    });
    projectRevisions.set(project.id, saved.revision);
    documents.set(project.id, encoded);
  }
  for (const deleted of state.deletedProjects ?? []) {
    const revision = projectRevisions.get(deleted.id);
    if (revision === undefined) continue;
    await canvasData("projects.delete", { id: deleted.id, revision });
    projectRevisions.delete(deleted.id);
    documents.delete(deleted.id);
  }
}

class CanvasStorage {
  private revisions = new Map<string, number>();
  private memory = new Map<string, unknown>();
  constructor(private namespace: string) {}
  config() {
    return true;
  }
  createInstance(options: { storeName?: string }) {
    return new CanvasStorage(options.storeName ?? "app_state");
  }
  private isMedia() {
    return this.namespace === "image_files" || this.namespace === "media_files";
  }
  private ephemeral() {
    return this.namespace === "image_previews" || this.namespace.includes("plugin");
  }
  async getItem<T>(key: string): Promise<T | null> {
    if (this.ephemeral()) return (this.memory.get(key) as T) ?? null;
    if (this.namespace === "app_state" && key === canvasKey) {
      projectLoad ??= loadProjects();
      return (await projectLoad) as T;
    }
    if (this.isMedia()) {
      const response = await canvasRequest("assets.content", { id: key });
      if (response.status === 404) {
        await response.body?.cancel();
        return null;
      }
      if (!response.ok) throw new Error("Unable to read canvas media");
      return (await response.blob()) as T;
    }
    const row = await canvasData<Setting | null>("state.get", { namespace: this.namespace, key });
    this.revisions.set(key, row?.revision ?? 0);
    return row ? (JSON.parse(row.value) as T) : null;
  }
  async setItem<T>(key: string, value: T): Promise<T> {
    if (this.ephemeral()) {
      this.memory.set(key, value);
      return value;
    }
    if (this.isMedia()) {
      if (!(value instanceof Blob)) throw new Error("Canvas media must be a file");
      await canvasData("assets.upload", { id: key, blob: value });
      return value;
    }
    const persist = async () => {
      if (this.namespace === "app_state" && key === canvasKey) {
        await projectLoad;
        await saveProjects(String(value));
        return;
      }
      if (!this.revisions.has(key)) await this.getItem(key);
      const record = await canvasData<Setting>("state.set", {
        namespace: this.namespace,
        key,
        revision: this.revisions.get(key) ?? 0,
        value,
      });
      this.revisions.set(key, record.revision);
    };
    // Preserve write order; a rejected write remains visible to the caller,
    // but must not permanently poison subsequent saves.
    const operation = writeQueue.then(persist);
    writeQueue = operation.catch(() => undefined);
    await operation;
    return value;
  }
  async removeItem(key: string): Promise<void> {
    if (this.ephemeral()) {
      this.memory.delete(key);
      return;
    }
    if (this.isMedia()) {
      const response = await canvasRequest("assets.delete", { id: key });
      // Automatic upstream garbage collection cannot delete media still
      // referenced by the server (including the recycle bin).
      if (response.status === 409) {
        await response.body?.cancel();
        return;
      }
      const result = await response.json();
      if (!response.ok || !result.success)
        throw new Error(result.message || "Unable to delete canvas media");
      return;
    }
    if (!this.revisions.has(key)) await this.getItem(key);
    await canvasData("state.delete", {
      namespace: this.namespace,
      key,
      revision: this.revisions.get(key),
    });
    this.revisions.delete(key);
  }
  async keys(): Promise<string[]> {
    if (this.ephemeral()) return [...this.memory.keys()];
    if (this.isMedia()) {
      const keys: string[] = [];
      let offset = 0;
      do {
        const page = await canvasData<Page<Asset>>("assets.list", { offset });
        for (const asset of page.items) {
          if (
            this.namespace === "image_files"
              ? asset.asset_id.startsWith("image:")
              : !asset.asset_id.startsWith("image:")
          )
            keys.push(asset.asset_id);
        }
        offset = page.next_offset;
      } while (offset >= 0);
      return keys;
    }
    return (await canvasData<Setting[]>("state.list", { namespace: this.namespace })).map(
      (row) => row.key,
    );
  }
  async iterate<T, R>(
    iterator: (value: T, key: string, iteration: number) => R | undefined,
  ): Promise<R | undefined> {
    let index = 0;
    for (const key of await this.keys()) {
      const value = await this.getItem<T>(key);
      const result = iterator(value as T, key, ++index);
      if (result !== undefined) return result;
    }
    return undefined;
  }
  async length(): Promise<number> {
    return (await this.keys()).length;
  }
  async clear(): Promise<void> {
    for (const key of await this.keys()) await this.removeItem(key);
  }
}

export default new CanvasStorage("app_state");
