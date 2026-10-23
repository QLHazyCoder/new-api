import { useEffect, useState, type ReactNode } from "react";
import { Alert, Button, Modal } from "antd";
import { Link, Outlet, createHashRouter, useLocation } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { canvasData, canvasRequest } from "./bridge";
import { unsavedCanvasValues } from "./localforage-storage";
import { restoreCanvasProject } from "./storage";
import { useCanvasStore, type CanvasProject } from "../stores/canvas/use-canvas-store";

import CanvasPage from "../pages/canvas";
import CanvasProjectPage from "./project";
import ImagePage from "../pages/image";
import VideoPage from "../pages/video";
import AssetsPage from "../pages/assets";
import PromptsPage from "../pages/prompts";

export function UserStatusActions(props: {
  showConfig?: boolean;
  variant?: string;
  onOpenShortcuts?: () => void;
  onOpenPlugins?: () => void;
}) {
  return (
    <>
      {props.onOpenShortcuts ? (
        <Button type="text" onClick={props.onOpenShortcuts}>
          快捷键
        </Button>
      ) : null}
    </>
  );
}

type Run = {
  run_id: string;
  operation: string;
  status: string;
  task_id?: string;
  archive_status: string;
  result_mime: string;
  created_at: number;
};
const operationNames: Record<string, string> = {
  "/v1/responses": "文本助手",
  "/v1/images/generations": "生成图片",
  "/v1/images/edits": "编辑图片",
  "/v1/videos": "生成视频",
  "/v1/audio/speech": "生成语音",
};
const statusNames: Record<string, string> = {
  running: "处理中",
  succeeded: "已完成",
  failed: "失败",
  interrupted: "观察已中断",
};

function EmbeddedLayout() {
  const { pathname } = useLocation();
  const { t } = useTranslation();
  const [error, setError] = useState("");
  const [mediaErrors, setMediaErrors] = useState<Record<string, string>>({});
  const [runs, setRuns] = useState<Run[] | null>(null);
  const [runOffset, setRunOffset] = useState(-1);
  const [busy, setBusy] = useState(false);
  const [trash, setTrash] = useState<Array<{ project_id: string; title: string }> | null>(null);
  useEffect(() => {
    const failed = (event: Event) => {
      const detail = (event as CustomEvent).detail;
      if (detail && typeof detail.key === "string" && typeof detail.message === "string") {
        setMediaErrors((current) => ({ ...current, [detail.key]: detail.message }));
      } else setError(String(detail));
    };
    const saved = (event: Event) => {
      const key = (event as CustomEvent).detail?.key;
      if (typeof key === "string") {
        setMediaErrors((current) => {
          const next = { ...current };
          delete next[key];
          return next;
        });
      } else setError("");
    };
    window.addEventListener("canvas:save-error", failed);
    window.addEventListener("canvas:saved", saved);
    return () => {
      window.removeEventListener("canvas:save-error", failed);
      window.removeEventListener("canvas:saved", saved);
    };
  }, []);
  const loadRuns = async (offset = 0) => {
    setBusy(true);
    try {
      const page = await canvasData<{ items: Run[]; next_offset: number }>("runs.list", { offset });
      setRuns((current) => (offset === 0 ? page.items : [...(current ?? []), ...page.items]));
      setRunOffset(page.next_offset);
    } finally {
      setBusy(false);
    }
  };
  const recover = async (run: Run) => {
    let response =
      run.task_id && run.operation === "/v1/videos"
        ? await canvasRequest("videos.content", { id: run.task_id })
        : await canvasRequest("runs.result", { id: run.run_id });
    if (!response.ok && run.task_id && run.operation !== "/v1/videos") {
      await response.body?.cancel();
      const listing = await canvasRequest("artifacts.list", { id: run.task_id });
      const result = (await listing.json()) as { artifacts?: Array<{ key: string }> };
      if (!listing.ok || !result.artifacts?.length)
        throw new Error("此任务尚无可读取的素材，请稍后重新查看");
      response = await canvasRequest("artifacts.content", {
        id: run.task_id,
        key: result.artifacts[0].key,
      });
    }
    if (!response.ok) throw new Error("结果尚未就绪，请查看任务状态");
    const blob = await response.blob();
    const url = URL.createObjectURL(blob);
    const anchor = document.createElement("a");
    anchor.href = url;
    anchor.download = "canvas-" + run.run_id;
    anchor.click();
    window.setTimeout(() => URL.revokeObjectURL(url), 1000);
  };
  const visibleError = [error, ...Object.values(mediaErrors)].filter(Boolean).join("\n");
  return (
    <div className="flex h-dvh min-h-0 flex-col overflow-hidden bg-background text-foreground">
      <nav
        className="flex shrink-0 items-center gap-2 overflow-x-auto border-b px-3 py-1"
        aria-label="画布导航"
      >
        {["canvas", "image", "video", "assets", "prompts"].map((page) => (
          <Link
            key={page}
            to={"/" + page}
            className="shrink-0 px-2 py-1 text-sm"
            aria-current={pathname.startsWith("/" + page) ? "page" : undefined}
          >
            {t("navigation." + page)}
          </Link>
        ))}
        <div className="ml-auto flex shrink-0">
          <Button
            type="text"
            onClick={() =>
              void canvasData<{ items: Array<{ project_id: string; title: string }> }>(
                "projects.list",
                { trash: true },
              )
                .then((result) => setTrash(result.items))
                .catch((cause: Error) => setError(cause.message))
            }
          >
            回收站
          </Button>
          <Button
            type="text"
            loading={busy}
            onClick={() => void loadRuns().catch((cause: Error) => setError(cause.message))}
          >
            生成记录
          </Button>
          <UserStatusActions />
        </div>
      </nav>
      {visibleError ? (
        <Alert
          type="error"
          showIcon
          title="画布操作未完成"
          description={visibleError}
          action={
            <Button
              onClick={() => {
                const blob = new Blob([JSON.stringify(Object.fromEntries(unsavedCanvasValues))], {
                  type: "application/json",
                });
                const url = URL.createObjectURL(blob);
                const link = document.createElement("a");
                link.href = url;
                link.download = "canvas-unsaved.json";
                link.click();
                window.setTimeout(() => URL.revokeObjectURL(url), 1000);
              }}
            >
              导出未保存内容
            </Button>
          }
        />
      ) : null}
      <div className="min-h-0 flex-1 overflow-hidden">
        <Outlet />
      </div>
      <Modal title="生成记录" open={runs !== null} onCancel={() => setRuns(null)} footer={null}>
        {(runs ?? []).map((run) => (
          <div key={run.run_id} className="flex items-center gap-3 border-b py-3 text-sm">
            <div className="min-w-0 flex-1">
              <div className="truncate">{operationNames[run.operation] || "生成任务"}</div>
              <div>{statusNames[run.status] || "状态未知"}</div>
            </div>
            <Button
              disabled={run.archive_status !== "saved" && !run.task_id}
              onClick={() => void recover(run).catch((cause: Error) => setError(cause.message))}
            >
              读取结果
            </Button>
          </div>
        ))}
        {runs?.length === 0 ? <p>暂无生成记录</p> : null}
        {runOffset >= 0 ? (
          <Button
            loading={busy}
            onClick={() =>
              void loadRuns(runOffset).catch((cause: Error) => setError(cause.message))
            }
          >
            加载更多
          </Button>
        ) : null}
      </Modal>
      <Modal title="回收站" open={trash !== null} onCancel={() => setTrash(null)} footer={null}>
        {(trash ?? []).map((project) => (
          <div key={project.project_id} className="flex items-center justify-between gap-3 py-2">
            <span className="truncate">{project.title}</span>
            <Button
              onClick={() =>
                void restoreCanvasProject(project.project_id)
                  .then((document) => {
                    const current = useCanvasStore.getState();
                    current.replaceProjects(
                      [document as CanvasProject, ...current.projects],
                      current.deletedProjects.filter((item) => item.id !== project.project_id),
                    );
                    setTrash(
                      (items) =>
                        items?.filter((item) => item.project_id !== project.project_id) ?? [],
                    );
                  })
                  .catch((cause: Error) => setError(cause.message))
              }
            >
              恢复
            </Button>
          </div>
        ))}
        {trash?.length === 0 ? <p>回收站为空</p> : null}
      </Modal>
    </div>
  );
}

export const router = createHashRouter([
  {
    element: <EmbeddedLayout />,
    children: [
      { path: "/", element: <CanvasPage /> },
      { path: "/canvas", element: <CanvasPage /> },
      { path: "/canvas/:id", element: <CanvasProjectPage /> },
      { path: "/image", element: <ImagePage /> },
      { path: "/video", element: <VideoPage /> },
      { path: "/assets", element: <AssetsPage /> },
      { path: "/prompts", element: <PromptsPage /> },
      { path: "*", element: <CanvasPage /> },
    ],
  },
]);

export default function UserLayout(props: { children: ReactNode }) {
  return props.children;
}
