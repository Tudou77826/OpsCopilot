import { useState } from "react";
import { TbArrowLeft } from 'react-icons/tb';
import { IconButton } from '@opscopilot/shell-terminal/ui';
import type { AssistantCommand, CommandField, SkillEntry } from "./types";

export function templateKeys(template: string): string[] {
  return [
    ...new Set(
      Array.from(
        template.matchAll(/\{\{([a-zA-Z][a-zA-Z0-9_]*)\}\}/g),
        (match) => match[1],
      ),
    ),
  ];
}
const empty: AssistantCommand = {
  name: "",
  description: "",
  skill: "",
  template: "",
  fields: [],
};
export function CommandEditor({
  initial,
  skills,
  disabled,
  onSave,
  onCancel,
}: {
  initial?: AssistantCommand;
  skills: SkillEntry[];
  disabled: boolean;
  onSave(command: AssistantCommand): Promise<void>;
  onCancel(): void;
}) {
  const [command, setCommand] = useState<AssistantCommand>(
    () => initial ?? empty,
  );
  const [error, setError] = useState("");
  const update = (key: keyof AssistantCommand, value: string) =>
    setCommand((old) => ({ ...old, [key]: value }));
  function template(value: string) {
    setCommand((old) => ({
      ...old,
      template: value,
      fields: templateKeys(value).map(
        (key) =>
          old.fields.find((f) => f.key === key) ?? {
            key,
            label: key,
            kind: "text",
            required: true,
          },
      ),
    }));
  }
  function field(key: string, change: Partial<CommandField>) {
    setCommand((old) => ({
      ...old,
      fields: old.fields.map((f) => (f.key === key ? { ...f, ...change } : f)),
    }));
  }
  return (
    <form
      className="oc-command-form"
      onSubmit={(event) => {
        event.preventDefault();
        setError("");
        void onSave(command).catch((reason) => setError(String(reason)));
      }}
    >
      <header>
        <h3>{initial ? "编辑快捷命令" : "新建快捷命令"}</h3>
        <p>定义一句常用指令，使用时在输入框内填空。</p>
      </header>
      <fieldset disabled={disabled}>
        <div className="oc-library-form-row">
          <label>
            命令名称
            <div className="oc-command-name">
              <span>/</span>
              <input
                aria-label="命令名称"
                value={command.name}
                onChange={(e) => update("name", e.target.value)}
                readOnly={!!initial}
                pattern="[a-zA-Z][a-zA-Z0-9_-]{0,63}"
                required
                maxLength={64}
              />
            </div>
          </label>
          <label>
            关联 Skill
            <select
              aria-label="关联 Skill"
              value={command.skill}
              onChange={(e) => update("skill", e.target.value)}
            >
              <option value="">不指定</option>
              {skills.map((s) => (
                <option key={s.name} value={s.name}>
                  {s.name}
                  {!s.enabled ? "（已停用）" : ""}
                </option>
              ))}
            </select>
          </label>
        </div>
        <label>
          用途
          <input
            value={command.description}
            onChange={(e) => update("description", e.target.value)}
            required
            maxLength={1024}
            placeholder="例如：获取数据库信息并打开查询页签"
          />
        </label>
        <label>
          提示词模板
          <textarea
            value={command.template}
            onChange={(e) => template(e.target.value)}
            rows={5}
            required
            maxLength={8192}
            placeholder="请在 {{management}} 上获取 {{business}} 的数据库信息，保存连接并打开 SQL 页签。"
          />
        </label>
        <p className="oc-library-note">
          用 {"{{名称}}"} 标记空位。密码请通过连接配置填写。
        </p>
        {!!command.fields.length && (
          <div className="oc-command-fields">
            <div className="oc-command-field-head">
              <span>空位</span>
              <span>显示名称</span>
              <span>填写方式</span>
              <span>必填</span>
            </div>
            {command.fields.map((f) => (
              <div className="oc-command-field-row" key={f.key}>
                <code>{f.key}</code>
                <input
                  aria-label={`${f.key} 显示名称`}
                  value={f.label}
                  onChange={(e) => field(f.key, { label: e.target.value })}
                  maxLength={80}
                  required
                />
                <select
                  aria-label={`${f.key} 填写方式`}
                  value={f.kind}
                  onChange={(e) =>
                    field(f.key, {
                      kind: e.target.value as CommandField["kind"],
                    })
                  }
                >
                  <option value="text">文字</option>
                  <option value="connection">已保存的 SSH 连接</option>
                </select>
                <input
                  type="checkbox"
                  aria-label={`${f.key} 必填`}
                  checked={f.required}
                  onChange={(e) => field(f.key, { required: e.target.checked })}
                />
              </div>
            ))}
          </div>
        )}
      </fieldset>
      {error && (
        <p role="alert" className="oc-library-error">
          {error}
        </p>
      )}
      <footer>
        <IconButton label="返回命令列表" onClick={onCancel} disabled={disabled}><TbArrowLeft size={18} /></IconButton>
        <button
          className="oc-library-primary"
          type="submit"
          disabled={disabled}
        >
          保存命令
        </button>
      </footer>
    </form>
  );
}
