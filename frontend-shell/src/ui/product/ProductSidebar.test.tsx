import React from 'react';
import { act, fireEvent, render } from '@testing-library/react';
import { describe, expect, it, vi, beforeAll } from 'vitest';
import { ProductSidebar } from './ProductSidebar';

// jsdom 没有 setPointerCapture，ProductSidebar 的 onPointerDown 会调用它
beforeAll(() => {
    Element.prototype.setPointerCapture = () => {};
});

const renderSidebar = (activeTab: 'sessions' | 'troubleshoot' | 'chat' | 'knowledge' | 'script' = 'sessions') => {
    const onToggle = vi.fn();
    const utils = render(
        <div style={{ width: 1280, display: 'flex' }} data-testid="row">
            <ProductSidebar isOpen activeTab={activeTab} onToggle={onToggle}>
                <div>panel-content</div>
            </ProductSidebar>
        </div>,
    );
    const handle = utils.container.querySelector('[role="separator"]') as HTMLElement;
    if (!handle) throw new Error('resize handle not rendered');
    // jsdom 不做布局：clientWidth 恒为 0，会让 max 公式算出负值。显式定义
    // 父行宽度，模拟真实浏览器行宽 1280（max = min(800, 1280-120) = 800）。
    const row = utils.container.querySelector('[data-testid="row"]') as HTMLElement;
    Object.defineProperty(row, 'clientWidth', { value: 1280, configurable: true });
    return { ...utils, handle, onToggle };
};

describe('ProductSidebar', () => {
    it('拖拽手柄在按下后随左移增大宽度，且受 800 上限钳制', () => {
        const { handle } = renderSidebar();
        act(() => {
            fireEvent.pointerDown(handle, { pointerId: 1, clientX: 1000 });
            fireEvent.pointerMove(handle, { pointerId: 1, clientX: 300 });
            fireEvent.pointerUp(handle, { pointerId: 1 });
        });
        const shell = handle.parentElement as HTMLElement;
        expect(shell.style.width).toBe('800px');
    });

    it('低于最小值时钳制到 250', () => {
        const { handle } = renderSidebar();
        act(() => {
            fireEvent.pointerDown(handle, { pointerId: 1, clientX: 300 });
            fireEvent.pointerMove(handle, { pointerId: 1, clientX: 2000 });
            fireEvent.pointerUp(handle, { pointerId: 1 });
        });
        const shell = handle.parentElement as HTMLElement;
        expect(shell.style.width).toBe('250px');
    });

    // 回归（v1.10.4 之后发现）：updater 在渲染期才执行，若其中读取 drag.current，
    // "最后一次 pointermove 排队的更新" 与 "pointerup 清空 ref" 竞态时会抛
    // undefined.width，顶层无错误边界 → 整棵 React 树卸载白屏（用户表现为卡死）。
    //
    // 该竞态依赖 React 18 对连续事件（pointermove）的延迟渲染调度，RTL 的 act()
    // 会逐事件立即冲刷、恰好消灭此时序，因此这里临时关闭 act 环境让更新走真实
    // 调度器：move 入队 → up 清空 ref → 调度器随后渲染 → updater 在 ref 已清空后执行。
    it('move 排队更新与 pointerup 清空 ref 竞态不得崩溃', async () => {
        const { handle } = renderSidebar();
        const prevFlag = (globalThis as any).IS_REACT_ACT_ENVIRONMENT;
        (globalThis as any).IS_REACT_ACT_ENVIRONMENT = false;
        try {
            handle.dispatchEvent(new MouseEvent('pointerdown', { bubbles: true, clientX: 1000 }));
            handle.dispatchEvent(new MouseEvent('pointermove', { bubbles: true, clientX: 10 }));
            handle.dispatchEvent(new MouseEvent('pointerup', { bubbles: true }));
            // 让 React 调度器冲刷排队的渲染（连续事件优先级是延迟的）
            await new Promise((r) => setTimeout(r, 100));
        } finally {
            (globalThis as any).IS_REACT_ACT_ENVIRONMENT = prevFlag;
        }
        // 白屏守护：崩溃时 React 会卸载整棵树
        expect(handle.isConnected).toBe(true);
        const shell = handle.parentElement as HTMLElement;
        expect(shell.style.width).toBe('800px');
    });

    it('松手后再来的 move 不改变宽度', () => {
        const { handle } = renderSidebar();
        act(() => {
            fireEvent.pointerDown(handle, { pointerId: 1, clientX: 1000 });
            fireEvent.pointerMove(handle, { pointerId: 1, clientX: 300 });
            fireEvent.pointerUp(handle, { pointerId: 1 });
        });
        const before = (handle.parentElement as HTMLElement).style.width;
        act(() => {
            fireEvent.pointerMove(handle, { pointerId: 1, clientX: 5 });
        });
        expect((handle.parentElement as HTMLElement).style.width).toBe(before);
    });
});

// ── 结构守卫 ─────────────────────────────────────────────────────────────
// 回归（v1.10.4 之后发现）：侧边栏拖拽把 setState updater 写成读 drag.current，
// updater 在渲染期才执行，与 onPointerUp 清空 ref 竞态 → undefined.width →
// 顶层无错误边界 → 整树卸载白屏（用户表现为"拖动超过最大值程序卡死"）。
//
// 该竞态依赖 React 18 连续事件的延迟渲染调度，jsdom/RTL 的 act 冲刷模型
// 复现不了（三种构造均无法让旧代码在单测中变红），故这里用确定性的源码
// 结构守卫兜底：禁止任何 setState updater 读取 .current。竞态本身由 E2E
// 浏览器黄金用例（越界拖动 + 释放）在真实调度环境下守护。
describe('源码守卫：setState updater 不得读取可变 ref', () => {
    it('前端源码中不存在 updater 内读取 .current 的反模式', () => {
        // 本仓库 tsconfig 的 types 仅含 vite/client（无 node 类型声明）；
        // vitest 运行时提供 CJS require/__dirname，此处按 untyped 引入。
        // @ts-expect-error vitest 运行时注入 require
        const { readFileSync, readdirSync, statSync } = require('node:fs');
        // @ts-expect-error vitest 运行时注入 require
        const { join, resolve } = require('node:path');
        // 测试文件位于 frontend-shell/src/ui/product/，向上四级到仓库根
        // @ts-expect-error vitest 运行时注入 __dirname
        const repoRoot = resolve(__dirname, '../../../..');
        const roots = [
            join(repoRoot, 'frontend-shell', 'src'),
            join(repoRoot, 'frontend', 'src'),
        ];
        // setState updater 形参后同一行内出现 .current 读取。出现时请改为在
        // 事件处理器内计算数值后传入纯值 updater（见 ProductSidebar 拖拽回归）。
        const offender = /set[A-Z][a-zA-Z]*\(\s*(?:v|prev|p|s)\s*=>\s*[^)]*\.current\b/;
        const violations: string[] = [];
        const walk = (dir: string) => {
            for (const name of readdirSync(dir)) {
                if (name === 'node_modules' || name === 'dist') continue;
                const full = join(dir, name);
                if (statSync(full).isDirectory()) { walk(full); continue; }
                if (!/\.(tsx|ts)$/.test(name) || name.endsWith('.test.tsx') || name.endsWith('.test.ts')) continue;
                const lines = readFileSync(full, 'utf-8').split('\n') as string[];
                lines.forEach((line: string, i: number) => {
                    if (offender.test(line)) violations.push(`${full}:${i + 1}: ${line.trim().slice(0, 120)}`);
                });
            }
        };
        for (const root of roots) walk(root);
        expect(violations).toEqual([]);
    });
});
