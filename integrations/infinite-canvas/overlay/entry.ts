import { ready } from "./bridge";

// Opaque-origin frames cannot access Web Storage. This in-memory object only
// supports tiny upstream interface preferences, never projects or media.
class MemoryStorage {
  private data = new Map<string, string>();
  get length() {
    return this.data.size;
  }
  clear() {
    this.data.clear();
  }
  key(index: number) {
    return [...this.data.keys()][index] ?? null;
  }
  getItem(key: string) {
    return this.data.get(key) ?? null;
  }
  setItem(key: string, value: string) {
    this.data.set(key, String(value));
  }
  removeItem(key: string) {
    this.data.delete(key);
  }
}
Object.defineProperty(window, "localStorage", { value: new MemoryStorage() });
Object.defineProperty(window, "sessionStorage", { value: new MemoryStorage() });

// Dynamic import is intentional: no upstream module may touch storage before
// the opaque-origin storage shim has been installed.
void ready.then(() => import("./start"));
