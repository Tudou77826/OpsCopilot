import { lazy, Suspense, useEffect, useMemo, useRef, useState } from "react";
import type { AssistantDatabaseWorkspace } from "../../../../frontend-shell/src/workspace/database";
import { wailsDatabaseRuntime } from "../../shell-adapter/wailsDatabaseRuntime";
import * as api from "../../../wailsjs/go/main/App";
import "../TerminalHandoff/terminalHandoff.css";
import "./assistantDatabase.css";

const DatabaseWorkspace = lazy(() =>
  import("../../../../frontend-shell/src/ui/database/DatabaseWorkspace").then(
    (module) => ({ default: module.DatabaseWorkspace }),
  ),
);

export function AssistantDatabaseWorkspaceView({
  workspace,
  ready,
}: {
  workspace: AssistantDatabaseWorkspace;
  ready: React.MutableRefObject<Map<string, { nonce: string; sql: string }>>;
}) {
  const content = useRef<HTMLDivElement>(null);
  const current = useRef(workspace);
  current.current = workspace;
  const writes = useRef(Promise.resolve());
  const [editing, setEditing] = useState(false);
  const [error, setError] = useState("");
  const pendingWrites = useRef(0);
  const owned = !!workspace.ownerTurn;
  useEffect(() => {
    if (content.current)
      (content.current as HTMLElement & { inert: boolean }).inert = owned;
  }, [owned]);
  useEffect(
    () => () => {
      ready.current.delete(workspace.id);
    },
    [workspace.id, ready],
  );
  const runtime = useMemo(
    () => ({
      ...wailsDatabaseRuntime,
      disconnect: async (id: string) => {
        if (id === current.current.session.id) {
          if (
            !window.confirm(
              "断开这个数据库会话？未提交事务可能回滚，SQL 和结果仍保留。",
            )
          )
            throw Error("已取消断开");
          await api.DatabaseWorkspaceClose(current.current.id);
        } else await wailsDatabaseRuntime.disconnect(id);
      },
    }),
    [workspace.id],
  );
  function edit(sql: string) {
    pendingWrites.current++;
    setEditing(true);
    writes.current = writes.current
      .then(() => api.DatabaseWorkspaceEdit(workspace.id, sql))
      .catch((reason) => setError(String(reason)))
      .finally(() => {
        pendingWrites.current--;
        if (!pendingWrites.current) setEditing(false);
      });
  }
  return (
    <section
      className="oc-ai-db"
      aria-label={`数据库工作区 · ${workspace.profile.name}`}
    >
      {error && (
        <div className="oc-db-banner" role="alert">
          {error}
        </div>
      )}
      <div className="oc-handoff-viewport">
        <div ref={content} className="oc-handoff-content" aria-disabled={owned}>
          <Suspense
            fallback={<div className="oc-ai-db-loading">正在打开数据库…</div>}
          >
            <DatabaseWorkspace
              runtime={runtime}
              managed={{
                workspace,
                editing,
                onEdit: edit,
                onPresented: (sql) =>
                  ready.current.set(workspace.id, {
                    nonce: workspace.pending,
                    sql,
                  }),
              }}
            />
          </Suspense>
        </div>
        {owned && (
          <div
            className="oc-handoff-shield"
            aria-label="AI 正在使用此数据库页签"
          >
            <div className="oc-handoff-control">
              <span className="oc-handoff-orbit" aria-hidden="true" />
              <div>
                <strong>AI 正在使用此数据库页签</strong>
                <span>操作结束后可在这里继续工作</span>
              </div>
              <button
                onClick={() =>
                  void api
                    .DatabaseWorkspaceTakeover(workspace.id)
                    .catch((reason) => setError(String(reason)))
                }
              >
                手动接管
              </button>
            </div>
          </div>
        )}
      </div>
    </section>
  );
}
