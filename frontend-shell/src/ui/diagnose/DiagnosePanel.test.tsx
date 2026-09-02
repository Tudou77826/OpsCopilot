import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import React from 'react';
import DiagnosePanel, { DiagnoseRuntime, DiagnoseEvent } from './DiagnosePanel';

function makeRuntime(withCases = false) {
  let handler: ((e: DiagnoseEvent) => void) | null = null;
  const start = vi.fn(async () => ({ runId: 'diag-test1', caseId: withCases ? 'case-1' : '' }));
  const cancel = vi.fn(async () => {});
  const stop = vi.fn(async () => {});
  const conclusion = vi.fn(async () => {});
  const archive = vi.fn(async () => ({ filePath: 'archive/x.md' }));
  const runtime: DiagnoseRuntime = {
    start,
    cancel,
    onEvent: (h) => { handler = h; return () => { handler = null; }; },
    ...(withCases ? { cases: { stop, conclusion, archive } } : {}),
  };
  return { runtime, start, cancel, stop, conclusion, archive, emit: (e: DiagnoseEvent) => handler?.(e) };
}

describe('DiagnosePanel（中性事件契约）', () => {
  it('空问题时开始按钮禁用；填入后可启动并显示运行态', async () => {
    const { runtime, start, emit } = makeRuntime();
    render(<DiagnosePanel runtime={runtime} />);
    const btn = screen.getByRole('button', { name: '开始诊断' }) as HTMLButtonElement;
    expect(btn.disabled).toBe(true);
    fireEvent.change(screen.getByRole('textbox'), { target: { value: '磁盘打满' } });
    expect(btn.disabled).toBe(false);
    fireEvent.click(btn);
    await waitFor(() => expect(start).toHaveBeenCalledWith('磁盘打满', undefined));
    emit({ runId: 'diag-test1', kind: 'status', stage: 'retrieving', message: '检索知识库' });
    await waitFor(() => expect(screen.getByText(/检索知识库/)).toBeTruthy());
  });

  it('done 事件渲染 summary/steps/commands（中性结果契约）', async () => {
    const { runtime, emit } = makeRuntime();
    render(<DiagnosePanel runtime={runtime} />);
    fireEvent.change(screen.getByRole('textbox'), { target: { value: '磁盘打满' } });
    fireEvent.click(screen.getByRole('button', { name: '开始诊断' }));
    const result = JSON.stringify({
      summary: '磁盘空间被日志占满',
      steps: [{ step: '定位大文件' }],
      commands: [{ command: 'du -sh /var/log', description: '查看日志目录体积', risk: 'low' }],
    });
    emit({ runId: 'diag-test1', kind: 'done', result });
    await waitFor(() => expect(screen.getByText('磁盘空间被日志占满')).toBeTruthy());
    expect(screen.getByText('du -sh /var/log')).toBeTruthy();
    expect(screen.getByText(/排查步骤/)).toBeTruthy();
  });

  it('C2：结案流式 token→concl-done、结束案例与归档（案例端口存在时）', async () => {
    const { runtime, emit, conclusion, stop, archive } = makeRuntime(true);
    render(<DiagnosePanel runtime={runtime} />);
    fireEvent.change(screen.getByRole('textbox'), { target: { value: '磁盘打满' } });
    fireEvent.click(screen.getByRole('button', { name: '开始诊断' }));
    emit({ runId: 'diag-test1', kind: 'done', result: JSON.stringify({ summary: '日志占满', steps: [], commands: [] }), caseId: 'case-1' });
    // 结果摘要与预填的根因框都含该文本
    await waitFor(() => expect(screen.getAllByText('日志占满').length).toBeGreaterThan(0));
    const rootCauseBox = screen.getByPlaceholderText('根因分析（可编辑，默认取诊断摘要）') as HTMLTextAreaElement;
    expect(rootCauseBox.value).toBe('日志占满');
    // 生成结案报告 → token 流 → concl-done
    fireEvent.click(screen.getByRole('button', { name: '生成结案报告' }));
    await waitFor(() => expect(conclusion).toHaveBeenCalledWith('日志占满'));
    emit({ runId: 'concl-1', kind: 'token', text: '结案：' });
    emit({ runId: 'concl-1', kind: 'token', text: '清理日志' });
    emit({ runId: 'concl-1', kind: 'concl-done', text: '结案：清理日志' });
    await waitFor(() => expect(screen.getByText('结案：清理日志')).toBeTruthy());
    // 结束案例 → 归档区块出现
    fireEvent.click(screen.getByRole('button', { name: '结束案例' }));
    await waitFor(() => expect(stop).toHaveBeenCalled());
    fireEvent.change(screen.getByPlaceholderText('微服务名（必填）'), { target: { value: '磁盘治理' } });
    fireEvent.click(screen.getByRole('button', { name: '归档到知识库' }));
    await waitFor(() => expect(archive).toHaveBeenCalled());
    await waitFor(() => expect(screen.getByText(/archive\/x\.md/)).toBeTruthy());
  });

  it('C2：案例端口缺省时结案/归档区块不渲染', async () => {
    const { runtime, emit } = makeRuntime(false);
    render(<DiagnosePanel runtime={runtime} />);
    fireEvent.change(screen.getByRole('textbox'), { target: { value: '磁盘打满' } });
    fireEvent.click(screen.getByRole('button', { name: '开始诊断' }));
    emit({ runId: 'diag-test1', kind: 'done', result: JSON.stringify({ summary: '日志占满' }) });
    await waitFor(() => expect(screen.getByText('日志占满')).toBeTruthy());
    expect(screen.queryByText('生成结案报告')).toBeNull();
    expect(screen.queryByPlaceholderText('微服务名（必填）')).toBeNull();
  });

  it('canceled 事件结束运行态；error 事件显示可读错误', async () => {
    const { runtime, emit } = makeRuntime();
    render(<DiagnosePanel runtime={runtime} />);
    fireEvent.change(screen.getByRole('textbox'), { target: { value: '网络不通' } });
    fireEvent.click(screen.getByRole('button', { name: '开始诊断' }));
    emit({ runId: 'diag-test1', kind: 'canceled' });
    await waitFor(() => expect(screen.getByText(/本次诊断已取消/)).toBeTruthy());

    fireEvent.click(screen.getByRole('button', { name: '开始诊断' }));
    emit({ runId: 'diag-test1', kind: 'error', message: 'AI provider error: EOF' });
    await waitFor(() => expect(screen.getByText(/AI provider error/)).toBeTruthy());
    expect((screen.getByRole('button', { name: '开始诊断' }) as HTMLButtonElement).disabled).toBe(false);
  });
});
