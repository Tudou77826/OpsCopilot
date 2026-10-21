import { useState } from "react";
import {
  ConnectionDialog,
  emptyProfile,
} from "../../../../frontend-shell/src/ui/database/ConnectionDialog";
import type { DatabaseProfile } from "../../../../frontend-shell/src/ui/database/types";
import { wailsDatabaseRuntime } from "../../shell-adapter/wailsDatabaseRuntime";
import type { InteractionAnswer, InteractionPrompt } from "./types";

export function DatabaseConnectionRequest({
  prompt,
  onRespond,
}: {
  prompt: InteractionPrompt;
  onRespond(answer: InteractionAnswer): Promise<void>;
}) {
  const [open, setOpen] = useState(false);
  const [error, setError] = useState("");
  const draft = {
    ...emptyProfile(),
    ...(prompt.data as Partial<DatabaseProfile>),
  };
  return (
    <section className="oc-assistant-interaction" aria-label={prompt.title}>
      <header>
        <span className="oc-interaction-eyebrow">需要补全连接信息</span>
      </header>
      <h3>{prompt.title}</h3>
      <p>{prompt.description}</p>
      <footer>
        <button
          onClick={() => void onRespond({ text: "", selected: ["reject"] }).catch(reason => setError(String(reason)))}
        >
          取消
        </button>
        <button
          className="oc-interaction-primary"
          onClick={() => setOpen(true)}
        >
          填写连接信息
        </button>
      </footer>
      {error && <p role="alert">{error}</p>}
      {open && (
        <ConnectionDialog
          profile={draft}
          runtime={wailsDatabaseRuntime}
          assistantRequest
          onClose={() => setOpen(false)}
          onSaved={(profile) => {
            setOpen(false);
            void onRespond({ text: profile.id, selected: ["allow"] }).catch(
              (reason) => setError(String(reason)),
            );
          }}
        />
      )}
    </section>
  );
}
