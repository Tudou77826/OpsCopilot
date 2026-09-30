import React from "react";
import {
  act,
  cleanup,
  fireEvent,
  render,
  renderHook,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  StartupConsent,
  ServiceCenterSettings,
  ServiceAnnouncement,
  ServiceLinks,
  useServiceAnnouncements,
} from "./ServiceCenter";
import { useCommandQuery } from "../../../../frontend-shell/src/ui/product/useCommandQuery";
const bridge = vi.hoisted(() => ({
  settings: vi.fn(),
  choose: vi.fn(),
  configure: vi.fn(),
  announcements: vi.fn(),
  open: vi.fn(),
  link: vi.fn(),
  confirm: vi.fn(),
  testConnection: vi.fn(),
}));
vi.mock("../ConfirmDialog/ConfirmDialog", () => ({
  confirmDialog: { show: bridge.confirm },
}));
vi.mock("../../../wailsjs/go/main/App", () => ({
  GetServiceCenterSettings: bridge.settings,
  SetReportingChoice: bridge.choose,
  ConfigureServiceCenter: bridge.configure,
  TestServiceCenterConnection: bridge.testConnection,
  GetServiceAnnouncements: bridge.announcements,
  OpenServicePage: bridge.open,
  OpenServiceAnnouncement: bridge.link,
}));
const initial = {
  baseUrl: "https://ops.internal",
  choice: "",
  needsConsent: true,
  ready: false,
  policy: {
    version: "3",
    notice:
      "仅收集每日活跃、连接计数、Ctrl+K 计数、快捷命令计数、基础环境及升级结果。保存90天，日汇总1年。",
  },
};
beforeEach(() => {
  vi.resetAllMocks();
  bridge.confirm.mockResolvedValue(true);
  bridge.settings.mockResolvedValue(initial);
  bridge.choose.mockImplementation(async (choice: string) => {
    const next = { ...initial, choice, needsConsent: false };
    bridge.settings.mockResolvedValue(next);
    return next;
  });
  bridge.announcements.mockResolvedValue([]);
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
});
describe("startup reporting consent", () => {
  it("opens service settings from either footer entry when no address is configured", async () => {
    bridge.settings.mockResolvedValue({ ...initial, baseUrl: "" });
    const configure = vi.fn();
    render(<ServiceLinks onConfigure={configure} />);
    fireEvent.click(screen.getByRole("button", { name: "产品介绍" }));
    fireEvent.click(screen.getByRole("button", { name: "问题与建议" }));
    await waitFor(() => expect(configure).toHaveBeenCalledTimes(2));
    expect(bridge.open).not.toHaveBeenCalled();
    expect(screen.queryByRole("status")).toBeNull();
  });

  it("discloses recipient and independent sharing, with two explicit choices", async () => {
    render(<StartupConsent />);
    const dialog = await screen.findByRole("dialog");
    expect(dialog).toHaveTextContent("数据发送至内网服务器，不涉及任何形式的外发");
    expect(dialog).toHaveTextContent("共享知识库");
    expect(dialog).toHaveTextContent("我们会收集");
 expect(dialog).toHaveTextContent("脚本创建次数");
 expect(dialog).toHaveTextContent("CLI 的 exec");
 expect(dialog).toHaveTextContent("每日汇总");
 expect(dialog).toHaveTextContent("文件名或路径");
    expect(dialog).toHaveTextContent("我们不会收集");
    expect(dialog).toHaveTextContent("拒绝不影响正常使用");
    expect(screen.getByRole("button", { name: "同意上报" })).toBeEnabled();
    expect(
      screen.getByRole("button", { name: "拒绝上报" }),
    ).toBeEnabled();
    expect(bridge.choose).not.toHaveBeenCalled();
  });
  it("accepting regular reporting never opens the fallback", async () => {
    render(<StartupConsent />);
    fireEvent.click(await screen.findByRole("button", { name: "同意上报" }));
    await waitFor(() => expect(bridge.choose).toHaveBeenCalledWith("standard"));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });
  it("refusal is saved before offering separately accepted minimal reporting", async () => {
    render(<StartupConsent />);
    fireEvent.click(await screen.findByRole("button", { name: "拒绝上报" }));
    await waitFor(() => expect(bridge.choose).toHaveBeenCalledWith("disabled"));
    const minimal = await screen.findByRole("button", { name: "同意最简上报" });
    expect(screen.getByRole("dialog")).toHaveTextContent("是否愿意接受一个更精简的方案");
    expect(screen.getByRole("dialog")).not.toHaveTextContent("常规上报已关闭");
    expect(screen.getByRole("dialog")).toHaveTextContent("功能使用次数、操作系统信息、升级结果");
    expect(bridge.choose).not.toHaveBeenCalledWith("minimal");
    fireEvent.click(minimal);
    await waitFor(() => expect(bridge.choose).toHaveBeenLastCalledWith("minimal"));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });
  it("fallback can be declined completely", async () => {
    render(<StartupConsent />);
    fireEvent.click(await screen.findByRole("button", { name: "拒绝上报" }));
    fireEvent.click(await screen.findByRole("button", { name: "不上报任何数据" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(bridge.choose.mock.calls).toEqual([["disabled"], ["disabled"]]);
  });
  it("closing the popup persists refusal", async () => {
    render(<StartupConsent />);
    await screen.findByRole("dialog");
    fireEvent.click(screen.getByRole("button", { name: "关闭并拒绝数据上报" }));
    await waitFor(() => expect(bridge.choose).toHaveBeenCalledWith("disabled"));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });
  it("Escape means refusal and save failure keeps popup visible", async () => {
    bridge.choose.mockRejectedValue(Error("disk full"));
    render(<StartupConsent />);
    await screen.findByRole("dialog");
    fireEvent.keyDown(window, { key: "Escape" });
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "数据采集保持关闭",
    );
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(bridge.choose).toHaveBeenCalledWith("disabled");
  });
  it("does not prompt a previously disabled installation", async () => {
    bridge.settings.mockResolvedValue({
      ...initial,
      choice: "disabled",
      needsConsent: false,
    });
    render(<StartupConsent />);
    await act(async () => {});
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(bridge.choose).not.toHaveBeenCalled();
  });
  it("settings allows later withdrawal", async () => {
    bridge.settings.mockResolvedValue({
      ...initial,
      choice: "standard",
      needsConsent: false,
    });
    render(<ServiceCenterSettings />);
    expect(await screen.findByRole("radio", { name: "常规上报" })).toBeChecked();
    expect(document.querySelector(".sc-reporting-details")).not.toHaveAttribute("open");
    expect(bridge.choose).not.toHaveBeenCalled();
    fireEvent.click(
      await screen.findByRole("radio", { name: "完全关闭" }),
    );
    await waitFor(() => expect(bridge.choose).toHaveBeenCalledWith("disabled"));
    expect(await screen.findByRole("radio", { name: "完全关闭" })).toBeChecked();
  });
  it("tests the entered address without saving it or changing consent", async () => {
    bridge.testConnection.mockResolvedValue(undefined);
    render(<ServiceCenterSettings />);
    const field = await screen.findByLabelText("服务地址");
    fireEvent.change(field, { target: { value: "https://other.internal" } });
    fireEvent.click(screen.getByRole("button", { name: "测试内网服务连接" }));
    expect(await screen.findByText("连接成功")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "测试内网服务连接" })).toHaveClass("is-success");
    expect(bridge.testConnection).toHaveBeenCalledWith("https://other.internal");
    expect(bridge.configure).not.toHaveBeenCalled();
    expect(bridge.choose).not.toHaveBeenCalled();
  });
  it("keeps address save errors on the save icon and permits retry", async () => {
    bridge.configure.mockRejectedValue(new Error("服务地址必须是 HTTP 或 HTTPS 根地址，例如 http://88.45.4.2:8090"));
    render(<ServiceCenterSettings />);
    const button = await screen.findByRole("button", { name: "保存服务地址" });
    fireEvent.click(button);
    await waitFor(() => expect(button).toHaveClass("is-failure"));
    expect(button).toBeEnabled();
    expect(button).toHaveAttribute("title", "请填写 http:// 或 https:// 开头的服务根地址，不要包含路径或参数。");
    expect(document.querySelector(".sc-error")).toBeNull();
    fireEvent.click(button);
    await waitFor(() => expect(bridge.configure).toHaveBeenCalledTimes(2));
  });
  it("does not change other controls while saving the service address", async () => {
    let finish!: (value: typeof initial) => void;
    bridge.configure.mockImplementation(() => new Promise(resolve => { finish = resolve; }));
    render(<ServiceCenterSettings />);
    const save = await screen.findByRole("button", { name: "保存服务地址" });
    fireEvent.click(save);
    expect(save).toBeDisabled();
    expect(screen.getByRole("radio", { name: "常规上报" })).toBeEnabled();
    expect(screen.getByRole("button", { name: "产品介绍（在浏览器中打开）" })).toBeEnabled();
    expect(screen.queryByText("正在保存…")).toBeNull();
    await act(async () => finish(initial));
    expect(save).toBeEnabled();
  });
  it("keeps failure feedback inside the connection icon", async () => {
    bridge.testConnection.mockRejectedValue(new Error("连接不可用"));
    render(<ServiceCenterSettings />);
    fireEvent.click(await screen.findByRole("button", { name: "测试内网服务连接" }));
    await waitFor(() => expect(screen.getByRole("button", { name: "测试内网服务连接" })).toHaveClass("is-failure"));
    expect(screen.getByRole("button", { name: "测试内网服务连接" })).toHaveAttribute("title", "连接不可用");
    expect(document.querySelector(".sc-connection-feedback")).toBeNull();
  });
  it("allows immediate retry at the same address after a network failure", async () => {
    bridge.testConnection.mockRejectedValueOnce(new Error("网络波动")).mockResolvedValueOnce(undefined);
    render(<ServiceCenterSettings />);
    const button = await screen.findByRole("button", { name: "测试内网服务连接" });
    fireEvent.click(button);
    await waitFor(() => expect(button).toHaveClass("is-failure"));
    expect(button).toBeEnabled();
    fireEvent.click(button);
    await waitFor(() => expect(button).toHaveClass("is-success"));
    expect(bridge.testConnection).toHaveBeenCalledTimes(2);
    expect(bridge.testConnection).toHaveBeenNthCalledWith(1, initial.baseUrl);
    expect(bridge.testConnection).toHaveBeenNthCalledWith(2, initial.baseUrl);
  });
  it("cancelling shutdown preserves the saved reporting choice", async () => {
    bridge.settings.mockResolvedValue({ ...initial, choice: "minimal", needsConsent: false });
    bridge.confirm.mockResolvedValue(false);
    render(<ServiceCenterSettings />);
    fireEvent.click(await screen.findByRole("radio", { name: "完全关闭" }));
    await waitFor(() => expect(bridge.confirm).toHaveBeenCalled());
    await waitFor(() => expect(screen.getByRole("radio", { name: "最简上报" })).toBeEnabled());
    expect(screen.getByRole("radio", { name: "最简上报" })).toBeChecked();
    expect(bridge.choose).not.toHaveBeenCalled();
  });
});
it("announcements hide expired entries and open only an explicit clicked link", async () => {
  bridge.announcements.mockResolvedValue([
    {
      id: "a",
      text: "有效公告",
      url: "https://ops.internal/help",
      startsAt: new Date(Date.now() - 60000).toISOString(),
      endsAt: new Date(Date.now() + 60000).toISOString(),
    },
    {
      id: "b",
      text: "过期公告",
      url: "",
      startsAt: new Date(Date.now() - 120000).toISOString(),
      endsAt: new Date(Date.now() - 60000).toISOString(),
    },
  ]);
  bridge.link.mockResolvedValue(undefined);
  const { result } = renderHook(() => useServiceAnnouncements());
  await waitFor(() => expect(result.current).toHaveLength(1));
  render(<ServiceAnnouncement announcement={result.current[0]} />);
  fireEvent.click(screen.getByRole("button", { name: "有效公告" }));
  expect(bridge.link).toHaveBeenCalledWith("https://ops.internal/help");
  expect(screen.queryByText("过期公告")).toBeNull();
});
it("Ctrl+K counts an opening once, ignoring repeats and an already open panel", () => {
  const opened = vi.fn();
  const host = {
    opened,
    generate: vi.fn(),
    type: vi.fn(),
    copy: vi.fn(),
    warn: vi.fn(),
  };
  const { result } = renderHook(() => useCommandQuery(host, "terminal"));
  fireEvent.keyDown(window, { key: "k", ctrlKey: true });
  expect(result.current.visible).toBe(true);
  expect(opened).toHaveBeenCalledTimes(1);
  fireEvent.keyDown(window, { key: "k", ctrlKey: true, repeat: true });
  fireEvent.keyDown(window, { key: "k", ctrlKey: true });
  expect(opened).toHaveBeenCalledTimes(1);
  act(() => result.current.setVisible(false));
  fireEvent.keyDown(window, { key: "k", ctrlKey: true });
  expect(opened).toHaveBeenCalledTimes(2);
});
it("Ctrl+K without a terminal or behind consent popup does not count", async () => {
  const opened = vi.fn(),
    host = {
      opened,
      generate: vi.fn(),
      type: vi.fn(),
      copy: vi.fn(),
      warn: vi.fn(),
    };
  const hook = renderHook(() => useCommandQuery(host, null));
  fireEvent.keyDown(window, { key: "k", ctrlKey: true });
  expect(opened).not.toHaveBeenCalled();
  hook.unmount();
  render(<StartupConsent />);
  await screen.findByRole("dialog");
  renderHook(() => useCommandQuery(host, "terminal"));
  fireEvent.keyDown(window, { key: "k", ctrlKey: true });
  expect(opened).not.toHaveBeenCalled();
});

it("generated command counts typing only after terminal dispatch, not copying or failure", async () => {
  const host = { generate: vi.fn().mockResolvedValue({command: 'pwd', explanation: ''}), type: vi.fn(), typed: vi.fn(), copy: vi.fn().mockResolvedValue(undefined), warn: vi.fn() };
  const {result} = renderHook(() => useCommandQuery(host, 'terminal'));
  await act(async () => { await result.current.generate('show directory'); });
  await act(async () => { await result.current.copy(); });
  expect(host.typed).not.toHaveBeenCalled();
  host.type.mockImplementationOnce(() => { throw Error('terminal unavailable'); });
  act(() => result.current.type());
  expect(host.typed).not.toHaveBeenCalled();
  act(() => result.current.type());
  expect(host.typed).toHaveBeenCalledTimes(1);
});
