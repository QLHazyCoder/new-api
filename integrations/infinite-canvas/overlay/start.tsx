import React from "react";
import { createRoot } from "react-dom/client";
import { RouterProvider } from "react-router-dom";
import "antd/dist/reset.css";
import "streamdown/styles.css";
import "../styles/globals.css";
import "../i18n";
import "./transport";
import { AppProviders } from "../components/layout/app-providers";
import { useAgentStore } from "../stores/use-agent-store";
import { usePromptSourceStore } from "../stores/use-prompt-source-store";
import { initializeConfig } from "./config";
import { router } from "./layout";
import { reportCanvasFailure } from "./bridge";

const root = createRoot(document.getElementById("root")!);
// Agent is disabled at its execution/store boundary as well as at the UI.
useAgentStore.setState({
  enabled: false,
  token: "",
  connected: false,
  panelOpen: false,
  connectAgent: () => {
    throw new Error("Local Agent is disabled");
  },
  togglePanel: () => undefined,
  openPanel: () => undefined,
});
usePromptSourceStore.setState({ sources: [], schedule: { intervalMinutes: 0, lastFetchedAt: "" } });
initializeConfig()
  .then(() => {
    root.render(
      <React.StrictMode>
        <AppProviders>
          <RouterProvider router={router} />
        </AppProviders>
      </React.StrictMode>,
    );
  })
  .catch(() => {
    reportCanvasFailure();
    root.render(<p role="alert">画布连接失败，请使用控制台的重试按钮重新打开。</p>);
  });
