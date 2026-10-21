import { useEffect, useRef, useState } from "react";
import type {
  AssistantDatabaseWorkspace,
  DatabaseWorkspaceUpdate,
} from "../../../../frontend-shell/src/workspace/database";
import type { TerminalWorkspaceController } from "../../../../frontend-shell/src/workspace/controller";
import * as api from "../../../wailsjs/go/main/App";

export function useAssistantDatabaseWorkspaces(
  client: string,
  controller?: TerminalWorkspaceController,
) {
  const [items, setItems] = useState<AssistantDatabaseWorkspace[]>([]);
  const ready = useRef(new Map<string, { nonce: string; sql: string }>());
  const latest = useRef(controller);
  latest.current = controller;
  useEffect(() => {
    if (!client) return;
    let live = true;
    let revision = 0;
    let current: AssistantDatabaseWorkspace[] = [];
    const seen = new Set<string>();
    void (async () => {
      while (live) {
        try {
          const update = (await api.DatabaseWorkspacePoll(
            client,
            revision,
          )) as DatabaseWorkspaceUpdate;
          if (!live) return;
          if (update.revision !== revision) {
            revision = update.revision;
            current = update.items.filter((w) => !w.dismissed);
            setItems(current);
          }
          for (const w of current) {
            if (!w.pending || seen.has(w.pending)) continue;
            latest.current?.selectWorkspace?.(w.id);
            const rendered = ready.current.get(w.id);
            if (
              latest.current?.presentedWorkspace?.() !== w.id ||
              rendered?.nonce !== w.pending ||
              rendered.sql !== w.sql
            )
              continue;
            await api.DatabaseWorkspacePresented(
              client,
              w.id,
              w.pending,
              w.session.id,
              rendered.sql,
            );
            seen.add(w.pending);
          }
        } catch (error) {
          if (String(error).includes("工作台请求已失效")) break;
        }
        await new Promise((resolve) => setTimeout(resolve, 200));
      }
    })();
    return () => {
      live = false;
      ready.current.clear();
    };
  }, [client]);
  return { items, ready };
}
