import { useEffect, useRef, useState } from "react";
import { TbInfoCircle, TbMessageExclamation, TbExternalLink, TbDeviceFloppy, TbPlugConnected, TbLoader2, TbSpeakerphone } from "react-icons/tb";
import { confirmDialog } from "../ConfirmDialog/ConfirmDialog";
import * as serviceBridge from "../../../wailsjs/go/main/App";
import {
  GetServiceCenterSettings,
  ConfigureServiceCenter,
  SetReportingChoice,
  OpenServicePage,
  GetServiceAnnouncements,
  OpenServiceAnnouncement,
} from "../../../wailsjs/go/main/App";
import "./ServiceCenter.css";

type Settings = {
  baseUrl: string;
  choice: string;
  needsConsent: boolean;
  policy: { version: string; notice: string };
  ready: boolean;
};
export type Announcement = {
  id: string;
  text: string;
  url: string;
  startsAt: string;
  endsAt: string;
};
const changed = () => window.dispatchEvent(new Event("service-center-changed"));

function ConsentContent({
  settings,
  minimal = false,
}: {
  settings: Settings;
  minimal?: boolean;
}) {
  return (
    <div className="sc-consent-copy">
      <p className="sc-kicker">数据采集说明 · {settings.policy.version}</p>
      <h2 id="sc-consent-title">
        {minimal
          ? "🌱 是否愿意接受一个更精简的方案？"
          : "📊 是否愿意分享使用数据，帮助我们改进 OpsCopilot？"}
      </h2>
      <p className="sc-notice">
        {minimal
          ? "如果上述范围不合适，你也可以只分享以下数据，帮助我们了解有多少安装实例在使用，以及大家使用的版本。"
          : "我们希望了解哪些功能更常用、版本升级是否顺利，以便安排后续改进。"}
      </p>
      <div className="sc-consent-block">
        <h3>{minimal ? "✅ 只收集这几项" : "✅ 我们会收集"}</h3>
        {minimal ? (
          <ul>
            <li><strong>每日是否使用过 OpsCopilot</strong>，不记录具体使用时间。</li>
            <li><strong>客户端版本</strong>。</li>
            <li>必需的随机安装标识、统计日期及授权记录，用于去重和确认你的选择。</li>
          </ul>
        ) : (
          <ul>
            <li>每日活跃情况。</li>
            <li>连接发起及成功次数、Ctrl+K 使用次数、快捷命令使用次数。</li>
            <li>脚本创建次数，脚本执行、AI 定位、归档、文件上传与下载、命令生成的发起次数及完成、失败、取消次数。</li>
            <li>知识库手动搜索次数、有无匹配结果、文档打开次数，以及生成命令发送到终端的次数。</li>
            <li>CLI 的 exec、file 上传/下载、diagnose、knowledge 列表/搜索/读取调用次数及结果；策略拒绝的固定分类、连接恢复尝试及结果。</li>
            <li>以上均为每日汇总，不记录操作时间、时长或具体内容。AI 定位完成不代表故障解决，发送命令不代表执行成功。</li>
            <li>客户端版本、操作系统类型和架构。</li>
            <li>升级阶段、结果及固定错误分类。</li>
            <li>用于去重和记录授权的随机安装标识、统计日期及授权信息。</li>
          </ul>
        )}
      </div>
      <div className="sc-consent-block">
        <h3>{minimal ? "🔒 不包含这些数据" : "🔒 我们不会收集"}</h3>
        <p>
          {minimal
            ? "功能使用次数、操作系统信息、升级结果，以及任何连接信息、操作内容或业务数据。"
            : "你的账号、服务器地址、连接配置、脚本名称或内容、文件名或路径、搜索词、命令名称或参数、终端输入输出、AI 对话、知识文档、密码、令牌、日志或原始错误信息。"}
        </p>
      </div>
      <div className="sc-consent-block">
        <h3>{minimal ? "📍 你仍然可以随时关闭" : "📍 数据如何管理"}</h3>
        <p>数据发送至<strong>内网服务器，不涉及任何形式的外发</strong>。</p>
        <p>
          安装实例记录保存 <strong>90 天</strong>，去除安装标识的日汇总保存 <strong>1 年</strong>。
          你可以随时在设置中关闭上报。
        </p>
        {!settings.baseUrl && (
          <p className="sc-recipient">尚未配置内网服务，配置后需重新确认同意。</p>
        )}
      </div>
      <div className="sc-boundary">
        {minimal ? "是否接受由你决定，拒绝不会影响正常使用。" : "拒绝不影响正常使用。"}
        共享知识库和共享连接信息仍由各自功能独立管理。
      </div>
    </div>
  );
}

export function StartupConsent() {
  const [settings, setSettings] = useState<Settings | null>(null);
  const [minimalOffer, setMinimalOffer] = useState(false);
  const showConsent = Boolean(settings?.needsConsent || minimalOffer);
  const [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  const panel = useRef<HTMLDivElement>(null);
  useEffect(() => {
    let alive = true;
    const load = () => {
      void Promise.resolve()
        .then(GetServiceCenterSettings)
        .then((s) => {
          if (alive) setSettings(s as Settings);
        })
        .catch(() => {});
    };
    load();
    window.addEventListener("service-center-changed", load);
    const timer = window.setInterval(load, 15000);
    return () => {
      alive = false;
      window.removeEventListener("service-center-changed", load);
      window.clearInterval(timer);
    };
  }, []);
  const choose = async (choice: string, offerMinimal = false) => {
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      setSettings((await SetReportingChoice(choice)) as Settings);
      setMinimalOffer(offerMinimal);
      changed();
    } catch {
      setError("未能保存选择，数据采集保持关闭。请检查配置目录权限后重试。");
    } finally {
      setBusy(false);
    }
  };
  useEffect(() => {
    if (!showConsent) return;
    const previous = document.activeElement as HTMLElement | null;
    panel.current?.focus();
    const key = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.preventDefault();
        void choose("disabled");
      }
      if (e.key === "Tab") {
        const buttons = panel.current?.querySelectorAll<HTMLButtonElement>(
          "button:not(:disabled)",
        );
        if (!buttons?.length) {
          e.preventDefault();
          return;
        }
        const first = buttons[0],
          last = buttons[buttons.length - 1];
        if (
          e.shiftKey &&
          (document.activeElement === first ||
            document.activeElement === panel.current)
        ) {
          e.preventDefault();
          last.focus();
        } else if (
          !e.shiftKey &&
          (document.activeElement === last ||
            document.activeElement === panel.current)
        ) {
          e.preventDefault();
          first.focus();
        }
      }
    };
    window.addEventListener("keydown", key, true);
    return () => {
      window.removeEventListener("keydown", key, true);
      previous?.focus();
    };
  }, [showConsent, busy]);
  if (!settings || !showConsent) return null;
  return (
    <div
      className="sc-overlay"
      onClick={(e) => {
        if (e.target === e.currentTarget) void choose("disabled");
      }}
    >
      <div
        className="sc-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby="sc-consent-title"
        tabIndex={-1}
        ref={panel}
      >
        <button
          className="sc-close"
          aria-label="关闭并拒绝数据上报"
          disabled={busy}
          onClick={() => void choose("disabled")}
        >
          ×
        </button>
        <ConsentContent settings={settings} minimal={minimalOffer} />
        <p className="sc-error" role="alert">
          {error}
        </p>
        <div className="sc-actions">
          <button disabled={busy} onClick={() => void choose(minimalOffer ? "minimal" : "standard")}>
            {minimalOffer ? "同意最简上报" : "同意上报"}
          </button>
          <button disabled={busy} onClick={() => void choose("disabled", !minimalOffer)}>
            {minimalOffer ? "不上报任何数据" : "拒绝上报"}
          </button>
        </div>
      </div>
    </div>
  );
}

export function ServiceCenterSettings() {
  const [settings, setSettings] = useState<Settings | null>(null),
    [base, setBase] = useState(""),
    [message, setMessage] = useState(""),
    [connectionResult, setConnectionResult] = useState(""),
    [saveResult, setSaveResult] = useState(""),
    [savingAddress, setSavingAddress] = useState(false),
    [testing, setTesting] = useState(false),
    [busy, setBusy] = useState(false);
  useEffect(() => {
    void Promise.resolve()
      .then(GetServiceCenterSettings)
      .then((s) => {
        setSettings(s as Settings);
        setBase(s.baseUrl);
      })
      .catch(() => setMessage("无法读取服务设置"));
  }, []);
  const action = async (fn: () => Promise<unknown>) => {
    setBusy(true);
    setMessage("");
    try {
      await fn();
      changed();
    } catch (e) {
      setMessage(String(e));
    } finally {
      setBusy(false);
    }
  };
  if (!settings) return <p>{message || "正在读取服务设置…"}</p>;
  return (
    <div className="sc-settings">
      <h2>内网服务</h2>
      <p>请联系团队管理员获取内网服务地址，填写并保存后即可访问产品介绍、更新、公告和反馈。</p>
      <label htmlFor="sc-base">服务地址</label>
      <div className="sc-address-row">
      <input
        id="sc-base"
        value={base}
        placeholder="请填写管理员提供的内网服务地址"
        onChange={(e) => { setBase(e.target.value); setConnectionResult(""); setSaveResult(""); }}
      />
      <button
        className={`sc-test-connection ${!connectionResult ? "" : connectionResult === "连接成功" ? "is-success" : connectionResult === "正在测试…" ? "is-testing" : "is-failure"}`}
        aria-label="测试内网服务连接"
        disabled={testing}
        title={connectionResult || "测试内网服务连接"}
        onClick={() => void (async () => {
          setTesting(true);
          setConnectionResult("正在测试…");
          try {
            if (!base.trim()) throw new Error("请先填写服务地址");
            const test = serviceBridge.TestServiceCenterConnection;
            if (!test) throw new Error("请重启客户端后测试连接");
            await test(base.trim());
            setConnectionResult("连接成功");
          } catch (e) {
            setConnectionResult(e instanceof Error ? e.message : String(e));
          } finally {
            setTesting(false);
          }
        })()}
      >{connectionResult === "正在测试…" ? TbLoader2({ size: 18, "aria-hidden": true }) : TbPlugConnected({ size: 18, "aria-hidden": true })}</button>
      <span className="sc-test-status" role="status">{connectionResult}</span>
      <button
        className={`sc-save-address ${saveResult ? saveResult === "服务地址已保存" ? "is-success" : "is-failure" : ""}`}
        title={saveResult || "保存服务地址"}
        aria-label="保存服务地址"
        disabled={savingAddress}
        onClick={() =>
          void (async () => {
            setSavingAddress(true);
            setSaveResult("");
            try {
            const s = await ConfigureServiceCenter(base.trim());
            setSettings(s as Settings);
            setBase(s.baseUrl);
            setSaveResult("服务地址已保存");
            changed();
            } catch (e) {
              const error = e instanceof Error ? e.message : String(e);
              setSaveResult(error.includes("HTTPS") ? "请填写管理员提供的 HTTPS 服务地址，不要包含路径或参数。" : "保存失败，请重试或联系管理员。");
            } finally {
              setSavingAddress(false);
            }
          })()
        }
      >
        {TbDeviceFloppy({ size: 18, "aria-hidden": true })}
      </button>
      <span className="sc-test-status" role="status">{saveResult}</span>
      </div>
      <div className="sc-actions sc-web-links">
        {[
          ["", "产品介绍"],
          ["help", "使用帮助"],
          ["feedback", "快捷反馈"],
        ].map(([page, label]) => (
          <button
            key={label}
            title={`${label} · 在浏览器中打开`}
            aria-label={`${label}（在浏览器中打开）`}
            disabled={busy || !settings.baseUrl}
            onClick={() => void action(() => OpenServicePage(page))}
          >
            {label}
            {TbExternalLink({ size: 14, "aria-hidden": true })}
          </button>
        ))}
      </div>
      <section>
        <div className="sc-reporting-heading">
          <h3>使用数据上报</h3>
        <div className="sc-reporting-selector" role="radiogroup" aria-label="数据上报方式">
          {settings.choice && <span className={`sc-reporting-thumb sc-mode-${settings.choice}`} aria-hidden="true" style={{ transform: `translateX(${["standard", "minimal", "disabled"].indexOf(settings.choice) * 100}%)` }} />}
          {[{value: "standard", label: "常规上报"}, {value: "minimal", label: "最简上报"}, {value: "disabled", label: "完全关闭"}].map((option) => (
            <label key={option.value} className={`sc-mode-${option.value}`}>
              <input
                type="radio"
                name="reporting-choice"
                value={option.value}
                checked={settings.choice === option.value}
                disabled={busy}
                onChange={() => void action(async () => {
                  if (option.value === "disabled") {
                    const confirmed = await confirmDialog.show({
                      title: "确认关闭数据上报？",
                      message: "关闭后将停止采集、缓存和发送运营数据。正常功能、共享知识库和共享连接不受影响，你也可以随时在这里重新开启。",
                      confirmText: "确认关闭",
                      cancelText: "保留当前选择",
                      danger: true,
                    });
                    if (confirmed !== true) return;
                  }
                  setSettings((await SetReportingChoice(option.value)) as Settings);
                })}
              />
              <span>{option.label}</span>
            </label>
          ))}
        </div>
        </div>
        <div className="sc-reporting-summary" aria-live="polite">
          <p>{busy ? "正在保存…" : settings.choice === "standard" ? "分享每日活跃、版本与环境、功能计数和升级结果。" : settings.choice === "minimal" ? "仅分享活跃、版本及必要的去重和授权信息。" : settings.choice === "disabled" ? "不采集、不缓存、不发送运营数据，不影响正常使用。" : "尚未授权，数据采集保持关闭。"}</p>
        </div>
        <details className="sc-reporting-details">
          <summary>数据采集说明 · 查看完整范围</summary>
          <p>选择后自动保存，可随时调整。关闭不影响功能使用、共享知识库和共享连接。</p>
          <ConsentContent settings={settings} />
          <div className="sc-consent-block">
            <h3>最简上报范围</h3>
            <p>仅每日活跃、客户端版本、随机安装标识、统计日期和授权记录；不包含功能计数、系统环境或升级记录。</p>
          </div>
        </details>
        {message && <p role="status" className="sc-error">{message}</p>}
      </section>
    </div>
  );
}

export function useServiceAnnouncements() {
  const [all, setAll] = useState<Announcement[]>([]);
  useEffect(() => {
    let alive = true;
    const load = () => {
      void Promise.resolve()
        .then(GetServiceAnnouncements)
        .then((a) => {
          if (alive) setAll(Array.isArray(a) ? a as Announcement[] : []);
        })
        .catch(() => {});
    };
    load();
    window.addEventListener("service-center-changed", load);
    const tick = window.setInterval(load, 60000);
    return () => {
      alive = false;
      window.clearInterval(tick);
      window.removeEventListener("service-center-changed", load);
    };
  }, []);
  const now = Date.now();
  return all.filter((a) => Date.parse(a.startsAt) <= now && Date.parse(a.endsAt) > now);
}

export function ServiceAnnouncement({ announcement: a }: { announcement: Announcement }) {
  return (
    <div className="sc-announcement">
      <span className="sc-announcement-label">{TbSpeakerphone({ size: 13, "aria-hidden": true })}公告</span>
      {a.url ? (
        <button
          title={a.text}
          onClick={() => void OpenServiceAnnouncement(a.url).catch(() => {})}
        >
          {a.text}
        </button>
      ) : (
        <span title={a.text}>{a.text}</span>
      )}
    </div>
  );
}

export function ServiceLinks({ onConfigure }: { onConfigure: () => void }) {
  return (
    <div className="sc-links">
      {[
        { page: "", label: "产品介绍", icon: TbInfoCircle },
        { page: "feedback", label: "问题与建议", icon: TbMessageExclamation },
      ].map(({ page, label, icon }) => (
        <button
          type="button"
          title={label}
          aria-label={label}
          key={label}
          onClick={() =>
            void GetServiceCenterSettings()
              .then((settings) => settings.baseUrl ? OpenServicePage(page) : onConfigure())
              .catch(onConfigure)
          }
        >
          {icon({ size: 15, "aria-hidden": true })}
        </button>
      ))}
    </div>
  );
}
