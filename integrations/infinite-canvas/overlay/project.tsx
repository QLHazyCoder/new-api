import { useEffect, useState } from "react";
import { Button, Spin } from "antd";
import { useParams } from "react-router-dom";
import { useTranslation } from "react-i18next";
import CanvasProjectPage from "../pages/canvas/project";
import { useCanvasStore, type CanvasProject } from "../stores/canvas/use-canvas-store";
import { loadCanvasProject } from "./storage";
import "./messages";

export default function ProjectPage() {
  const { id = "" } = useParams();
  const { t } = useTranslation();
  const [attempt, setAttempt] = useState(0);
  const [readyId, setReadyId] = useState("");
  const [status, setStatus] = useState<"loading" | "ready" | "failed">("loading");
  useEffect(() => {
    let current = true;
    const project = useCanvasStore.getState().projects.find((item) => item.id === id);
    if (project && !project.embeddedSummary) { setReadyId(id); setStatus("ready"); return; }
    setStatus("loading");
    void loadCanvasProject(id).then((document) => {
      if (!current) return;
      const store = useCanvasStore.getState();
      store.replaceProjects(store.projects.map((item) => item.id === id ? document as CanvasProject : item), store.deletedProjects);
      setReadyId(id);
      setStatus("ready");
    }).catch(() => { if (current) setStatus("failed"); });
    return () => { current = false; };
  }, [id, attempt]);
  if (status === "loading" || status === "ready" && readyId !== id) return <div role="status" className="flex h-full items-center justify-center gap-2"><Spin /><span>{t("embedded.loading")}</span></div>;
  if (status === "failed") return <div role="alert" className="flex h-full flex-col items-center justify-center gap-3"><p>{t("embedded.projectLoadFailed")}</p><Button onClick={() => setAttempt((value) => value + 1)}>{t("embedded.retry")}</Button></div>;
  return <CanvasProjectPage key={id} />;
}
