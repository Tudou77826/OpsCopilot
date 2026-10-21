import { useEffect, useRef, useState } from "react";
import { TbArrowLeft, TbFolderUp, TbTrash, TbX } from 'react-icons/tb';
import { IconButton } from '@opscopilot/shell-terminal/ui';
import { CommandEditor } from "./CommandEditor";
import type {
  AssistantCommand,
  LibraryHost,
  LibraryState,
  SkillEntry,
} from "./types";
import "./assistantLibrary.css";

export function AssistantLibrary({
  host,
  busy,
  onClose,
  onChanged,
}: {
  host: LibraryHost;
  busy: boolean;
  onClose(): void;
  onChanged(state: LibraryState): void;
}) {
  const dialog = useRef<HTMLDialogElement>(null);
  const [state, setState] = useState<LibraryState>({
    skills: [],
    commands: [],
  });
  const [tab, setTab] = useState<"skills" | "commands">("skills");
  const [error, setError] = useState("");
  const [working, setWorking] = useState(false);
  const [detail, setDetail] = useState<SkillEntry>();
  const [edit, setEdit] = useState<AssistantCommand | null | undefined>();
  const [remove, setRemove] = useState<{
    kind: "skill" | "command";
    name: string;
  }>();
  const disabled = busy || working;
  async function refresh() {
    const next = await host.read();
    setState(next);
    onChanged(next);
  }
  useEffect(() => {
    dialog.current?.showModal();
    void refresh().catch((e) => setError(String(e)));
  }, []);
  async function run(action: () => Promise<void>) {
    if (disabled) return;
    setWorking(true);
    setError("");
    try {
      await action();
      await refresh();
    } catch (e) {
      setError(String(e));
    } finally {
      setWorking(false);
    }
  }
  async function importSkill(replace: boolean, name?: string) {
    await run(async () => {
      const path = await host.selectDirectory();
      if (!path) return;
      await host.importSkill(path, replace, name ?? "");
      if (name) setDetail(undefined);
    });
  }
  return (
    <dialog
      className="oc-assistant-library"
      ref={dialog}
      onCancel={(event) => {
        event.preventDefault();
        if (!working) onClose();
      }}
      aria-labelledby="assistant-library-title"
    >
      <header className="oc-library-heading">
        <div>
          <h2 id="assistant-library-title">助手能力</h2>
          <p>沉淀排查方法，用一句指令开始工作。</p>
        </div>
        <IconButton
          label="关闭助手能力"
          onClick={onClose}
          disabled={working}
        >
          <TbX size={18} />
        </IconButton>
      </header>
      <nav className="oc-library-tabs" aria-label="助手能力分类">
        <button
          aria-current={tab === "skills" ? "page" : undefined}
          onClick={() => {
            setTab("skills");
            setDetail(undefined);
            setEdit(undefined);
          }}
        >
          Skills <span>{state.skills.length}</span>
        </button>
        <button
          aria-current={tab === "commands" ? "page" : undefined}
          onClick={() => {
            setTab("commands");
            setDetail(undefined);
            setEdit(undefined);
          }}
        >
          快捷命令 <span>{state.commands.length}</span>
        </button>
      </nav>
      <div className="oc-library-body">
        {busy && (
          <p className="oc-library-note" role="status">
            助手正在工作。可以浏览，结束本轮后再更改。
          </p>
        )}
        {error && (
          <p role="alert" className="oc-library-error">
            {error}
          </p>
        )}
        {remove ? (
          <section className="oc-library-confirm">
            <h3>移除 {remove.name}？</h3>
            <p>
              {remove.kind === "skill"
                ? "移除本机导入的副本，源文件不会改变。"
                : "此命令将从 / 菜单移除。"}
            </p>
            <footer>
              <button onClick={() => setRemove(undefined)} disabled={disabled}>
                取消
              </button>
              <button
                className="oc-library-danger"
                disabled={disabled}
                onClick={() =>
                  void run(async () => {
                    if (remove.kind === "skill")
                      await host.removeSkill(remove.name);
                    else await host.removeCommand(remove.name);
                    setRemove(undefined);
                    setDetail(undefined);
                  })
                }
              >
                移除
              </button>
            </footer>
          </section>
        ) : tab === "skills" ? (
          detail ? (
            <section className="oc-library-detail">
              <IconButton
                label="返回全部 Skills"
                onClick={() => setDetail(undefined)}
              >
                <TbArrowLeft size={18} />
              </IconButton>
              <header>
                <h3>{detail.name}</h3>
                <span>{detail.enabled ? "已启用" : "已停用"}</span>
              </header>
              <p>{detail.description}</p>
              <pre>{detail.instructions}</pre>
              <details>
                <summary>包含 {detail.files.length} 个文件</summary>
                <ul>
                  {detail.files.map((file) => (
                    <li key={file}>{file}</li>
                  ))}
                </ul>
              </details>
              <footer>
                <IconButton label="从目录更新…"
                  disabled={disabled}
                  onClick={() => void importSkill(true, detail.name)}
                >
                  <TbFolderUp size={18} />
                </IconButton>
                <IconButton label={`移除 ${detail.name}`} tone="danger"
                  disabled={disabled}
                  onClick={() =>
                    setRemove({ kind: "skill", name: detail.name })
                  }
                >
                  <TbTrash size={18} />
                </IconButton>
              </footer>
            </section>
          ) : (
            <>
              <div className="oc-library-section-heading">
                <div>
                  <h3>排查方法与业务知识</h3>
                  <p>导入标准 Skill 目录，保留参考资料和附带脚本。</p>
                </div>
                <button
                  className="oc-library-primary"
                  disabled={disabled}
                  onClick={() => void importSkill(false)}
                >
                  导入 Skill…
                </button>
              </div>
              {state.skills.length ? (
                <ul className="oc-library-list">
                  {state.skills.map((skill) => (
                    <li key={skill.name}>
                      <button
                        className="oc-library-item"
                        onClick={() => setDetail(skill)}
                      >
                        <strong>{skill.name}</strong>
                        <span>{skill.description}</span>
                        <small>{skill.files.length} 个文件</small>
                      </button>
                      <label className="oc-library-toggle">
                        <input
                          type="checkbox"
                          checked={skill.enabled}
                          disabled={disabled}
                          onChange={(e) =>
                            void run(() =>
                              host.enableSkill(skill.name, e.target.checked),
                            )
                          }
                        />
                        <span>{skill.enabled ? "已启用" : "已停用"}</span>
                      </label>
                    </li>
                  ))}
                </ul>
              ) : (
                <div className="oc-library-empty">
                  <span aria-hidden="true">◇</span>
                  <h3>把熟悉的排查方法交给助手</h3>
                  <p>
                    选择包含 SKILL.md
                    的目录。导入后可直接用自然语言交代任务，也可以为它创建快捷命令。
                  </p>
                </div>
              )}
              <p className="oc-library-note">
                变更从下一轮开始生效，当前对话会保留。Skill 不会替代操作授权。
              </p>
            </>
          )
        ) : edit !== undefined ? (
          <CommandEditor
            key={edit?.name ?? "new"}
            initial={edit ?? undefined}
            skills={state.skills}
            disabled={disabled}
            onCancel={() => setEdit(undefined)}
            onSave={async (command) => {
              setWorking(true);
              try {
                await host.saveCommand(command);
                await refresh();
                setEdit(undefined);
              } finally {
                setWorking(false);
              }
            }}
          />
        ) : (
          <>
            <div className="oc-library-section-heading">
              <div>
                <h3>把常用任务变成一句话</h3>
                <p>在输入框键入 / 搜索命令，填写高亮空位后发送。</p>
              </div>
              <button
                className="oc-library-primary"
                disabled={disabled}
                onClick={() => setEdit(null)}
              >
                新建命令
              </button>
            </div>
            {state.commands.length ? (
              <ul className="oc-library-list">
                {state.commands.map((command) => (
                  <li key={command.name}>
                    <button
                      className="oc-library-item"
                      onClick={() => setEdit(command)}
                    >
                      <strong>/{command.name}</strong>
                      <span>{command.description}</span>
                      <small>
                        {command.skill || "不指定 Skill"} ·{" "}
                        {command.fields.length} 个空位
                      </small>
                    </button>
                    <IconButton label={`移除 ${command.name}`} tone="danger"
                      disabled={disabled}
                      onClick={() =>
                        setRemove({ kind: "command", name: command.name })
                      }
                    >
                      <TbTrash size={18} />
                    </IconButton>
                  </li>
                ))}
              </ul>
            ) : (
              <div className="oc-library-empty">
                <span aria-hidden="true">/</span>
                <h3>少打几遍重复的指令</h3>
                <p>
                  设置提示词与需要填写的空位，业务处理过程继续由 Skill
                  和助手完成。
                </p>
              </div>
            )}
          </>
        )}
      </div>
    </dialog>
  );
}
