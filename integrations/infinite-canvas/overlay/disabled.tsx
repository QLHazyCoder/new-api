// These modules are replaced at build time; no executable plugin, credentials,
// local Agent or configuration-import surface is bundled into the product.
export const AppConfigModal = () => null;
export const AgentPanel = () => null;
export const ClientRootInit = (props: { children: import("react").ReactNode }) => props.children;
export const ensurePluginsLoaded = async () => undefined;
export const activatePlugin = (..._args: unknown[]) => {
  throw new Error("Extensions are disabled");
};
export const deactivatePlugin = () => undefined;
export const installPluginFromUrl = activatePlugin;
export const updatePlugin = activatePlugin;
export const setPluginEnabled = activatePlugin;
export const uninstallPlugin = () => undefined;
export const runModelPlugin = async <T,>(_args: unknown): Promise<T> => {
  throw new Error("Extensions are disabled");
};
export const normalizePluginImages = (): string[] => [];
export const getPluginVariables = () => [];
export const getPluginReturn = () => "";
export const getPluginAuthoringPrompt = () => "";
export const getPluginTemplates = () => ({ image: [], video: [], text: [], audio: [] });
