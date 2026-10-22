import type { StateStorage } from "zustand/middleware";
import storage from "./storage";

export const unsavedCanvasValues = new Map<string, string>();
export const localForageStorage: StateStorage = {
  getItem: (key) => storage.getItem<string>(key),
  setItem: async (key, value) => {
    try {
      await storage.setItem(key, value);
      unsavedCanvasValues.delete(key);
      if (!unsavedCanvasValues.size) window.dispatchEvent(new CustomEvent("canvas:saved"));
    } catch (error) {
      unsavedCanvasValues.set(key, value);
      window.dispatchEvent(
        new CustomEvent("canvas:save-error", {
          detail: error instanceof Error ? error.message : "画布保存失败",
        }),
      );
    }
  },
  removeItem: (key) => storage.removeItem(key),
};
