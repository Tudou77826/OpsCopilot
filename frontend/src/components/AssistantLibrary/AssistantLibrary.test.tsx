import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import AIChatPanel from "../Sidebar/AIChatPanel";
import { AssistantLibrary } from "./AssistantLibrary";
import { CommandEditor } from "./CommandEditor";
import type { AssistantCommand, LibraryHost } from "./types";
import type { AssistantRuntime } from "../../../../frontend-shell/src/assistant/runtime";

const command: AssistantCommand = {
  name: "sqlSessionInfoGetAndCreate",
  description: "获取数据库并打开 SQL 页签",
  skill: "database-discovery",
  template: "在 {{management}} 上查看 {{business}}，打开数据库页签。",
  fields: [
    {
      key: "management",
      label: "管理面连接",
      kind: "connection",
      required: true,
    },
    { key: "business", label: "业务名称", kind: "text", required: true },
  ],
};
function host(): LibraryHost {
  return {
    read: vi.fn(async () => ({
      skills: [
        {
          name: "database-discovery",
          description: "查询部署信息",
          instructions: "Use the application tools.",
          enabled: true,
          files: ["SKILL.md", "scripts/lookup.py"],
        },
      ],
      commands: [command],
    })),
    selectDirectory: vi.fn(async () => "/selected/skill"),
    importSkill: vi.fn(async () => {}),
    enableSkill: vi.fn(async () => {}),
    removeSkill: vi.fn(async () => {}),
    saveCommand: vi.fn(async () => {}),
    removeCommand: vi.fn(async () => {}),
    connections: vi.fn(async () => [
      { id: "saved-connection", name: "管理面 50 → 55" },
    ]),
    prepare: vi.fn(async () => "准备好的业务指令"),
  };
}
function runtime(): AssistantRuntime {
  return {
    start: vi.fn(async () => ({
      turnId: "turn",
      target: { tabId: "", connectionId: "" },
      phase: "completed",
      message: "完成",
      output: "已完成",
    })),
    state: vi.fn(async () => ({
      turnId: "",
      target: { tabId: "", connectionId: "" },
      phase: "idle",
      message: "",
    })),
    stop: vi.fn(async () => {}),
    respond: vi.fn(async () => {}),
    newConversation: vi.fn(async () => {}),
  };
}
beforeEach(() => {
  HTMLDialogElement.prototype.showModal = function () {
    this.setAttribute("open", "");
  };
});

describe("Skill 与快捷命令", () => {
  it("导入完整目录，浏览说明和文件，更新时指定原 Skill 名称", async () => {
    const h = host();
    render(
      <AssistantLibrary
        host={h}
        busy={false}
        onClose={vi.fn()}
        onChanged={vi.fn()}
      />,
    );
    await screen.findByText("database-discovery");
    fireEvent.click(screen.getByText("导入 Skill…"));
    await waitFor(() =>
      expect(h.importSkill).toHaveBeenCalledWith("/selected/skill", false, ""),
    );
    fireEvent.click(screen.getByText("database-discovery"));
    expect(screen.getByText("Use the application tools.")).toBeVisible();
    fireEvent.click(screen.getByRole("button", { name: "从目录更新…" }));
    await waitFor(() =>
      expect(h.importSkill).toHaveBeenCalledWith(
        "/selected/skill",
        true,
        "database-discovery",
      ),
    );
  });
  it("运行中允许查看但禁用导入和开关", async () => {
    render(
      <AssistantLibrary
        host={host()}
        busy
        onClose={vi.fn()}
        onChanged={vi.fn()}
      />,
    );
    await screen.findByText("database-discovery");
    expect(screen.getByRole("button", { name: "导入 Skill…" })).toBeDisabled();
    expect(screen.getByRole("checkbox")).toBeDisabled();
    fireEvent.click(screen.getByText("database-discovery"));
    expect(screen.getByText("Use the application tools.")).toBeVisible();
  });
  it("模板自动提取唯一空位，支持文字和连接类型，不要求编辑 JSON", async () => {
    const save = vi.fn(async () => {});
    render(
      <CommandEditor
        skills={[]}
        disabled={false}
        onCancel={vi.fn()}
        onSave={save}
      />,
    );
    fireEvent.change(screen.getByLabelText("命令名称"), {
      target: { value: "sqlSessionInfoGetAndCreate" },
    });
    fireEvent.change(screen.getByLabelText("用途"), {
      target: { value: "打开数据库" },
    });
    fireEvent.change(screen.getByLabelText("提示词模板"), {
      target: {
        value: "查询 {{management}} 和 {{business}}，再确认 {{business}}",
      },
    });
    expect(screen.getAllByLabelText(/显示名称/)).toHaveLength(2);
    fireEvent.change(screen.getByLabelText("management 填写方式"), {
      target: { value: "connection" },
    });
    fireEvent.click(screen.getByRole("button", { name: "保存命令" }));
    await waitFor(() =>
      expect(save).toHaveBeenCalledWith(
        expect.objectContaining({
          fields: [
            expect.objectContaining({
              key: "management",
              kind: "connection",
              required: true,
            }),
            expect.objectContaining({ key: "business" }),
          ],
        }),
      ),
    );
  });
  it("/ 搜索后直接在句子内填空，必填完成才发送，并传递真实连接 ID", async () => {
    const h = host(),
      r = runtime();
    render(<AIChatPanel libraryHost={h} runtime={r} />);
    await waitFor(() => expect(h.read).toHaveBeenCalled());
    fireEvent.change(screen.getByPlaceholderText("输入问题…"), {
      target: { value: "/sqlSession" },
    });
    fireEvent.click(
      await screen.findByRole("option", { name: /sqlSessionInfoGetAndCreate/ }),
    );
    expect(screen.getByRole("button", { name: "发送" })).toBeDisabled();
    fireEvent.change(await screen.findByLabelText("管理面连接"), {
      target: { value: "saved-connection" },
    });
    fireEvent.change(screen.getByLabelText("业务名称"), {
      target: { value: "订单服务" },
    });
    fireEvent.click(screen.getByRole("button", { name: "发送" }));
    await waitFor(() =>
      expect(h.prepare).toHaveBeenCalledWith(command.name, command.template, {
        management: "saved-connection",
        business: "订单服务",
      }),
    );
    await waitFor(() =>
      expect(r.start).toHaveBeenCalledWith("准备好的业务指令", undefined),
    );
    expect(await screen.findByText("已完成")).toBeVisible();
    expect(screen.getByPlaceholderText("输入问题…")).toBeEnabled();
  });
  it("键盘可选命令，Escape 关闭菜单，原始 / 文本不会误发送", async () => {
    const r = runtime();
    render(<AIChatPanel libraryHost={host()} runtime={r} />);
    const input = screen.getByPlaceholderText("输入问题…");
    fireEvent.change(input, { target: { value: "/" } });
    await screen.findByRole("option");
    fireEvent.keyDown(input, { key: "Enter" });
    await screen.findByLabelText("业务名称");
    expect(r.start).not.toHaveBeenCalled();
    fireEvent.click(screen.getByLabelText("退出快捷命令"));
    fireEvent.change(screen.getByPlaceholderText("输入问题…"), {
      target: { value: "/" },
    });
    await screen.findByRole("listbox");
    fireEvent.keyDown(screen.getByPlaceholderText("输入问题…"), {
      key: "Escape",
    });
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
  });
  it("准备失败保留填好的指令，不丢用户输入", async () => {
    const h = host();
    h.prepare = vi.fn(async () => {
      throw Error("连接已移除");
    });
    const r = runtime();
    render(<AIChatPanel libraryHost={h} runtime={r} />);
    fireEvent.change(screen.getByPlaceholderText("输入问题…"), {
      target: { value: "/" },
    });
    fireEvent.click(await screen.findByRole("option"));
    await screen.findByRole("option", { name: "管理面 50 → 55" });
    fireEvent.change(screen.getByLabelText("管理面连接"), {
      target: { value: "saved-connection" },
    });
    fireEvent.change(screen.getByLabelText("业务名称"), {
      target: { value: "订单服务" },
    });
    fireEvent.click(screen.getByRole("button", { name: "发送" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("连接已移除");
    expect(screen.getByLabelText("业务名称")).toHaveValue("订单服务");
    expect(r.start).not.toHaveBeenCalled();
  });
  it("命令菜单不处理输入区以外的终端按键", async () => {
    const r=runtime();
    render(<><textarea aria-label="用户终端"/><AIChatPanel libraryHost={host()} runtime={r}/></>);
    fireEvent.change(screen.getByPlaceholderText("输入问题…"),{target:{value:"/"}});
    await screen.findByRole("option");
    expect(fireEvent.keyDown(screen.getByLabelText("用户终端"),{key:"Enter"})).toBe(true);
    expect(fireEvent.keyDown(screen.getByLabelText("用户终端"),{key:"ArrowDown"})).toBe(true);
    expect(screen.queryByLabelText("业务名称")).not.toBeInTheDocument();
    expect(r.start).not.toHaveBeenCalled();
  });
});
