import React from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import ErrorBoundary from './ErrorBoundary';
// 注意：jsdom 匹配器由 src/setupTests.ts 统一注册（@testing-library/jest-dom/vitest），
// 本包未开启 vitest globals，这里不能再导入非 /vitest 入口。

// React 会把被边界捕获的异常打到 console.error，这里静音以免污染测试输出。
let consoleErrorSpy: ReturnType<typeof vi.spyOn>;

beforeEach(() => {
    consoleErrorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
});

afterEach(() => {
    consoleErrorSpy.mockRestore();
});

const Boom: React.FC = () => {
    throw new Error('渲染炸了');
};

describe('渲染错误边界', () => {
    it('子组件渲染异常时只隔离该区域，页面其余部分仍然可用', () => {
        render(
            <div>
                <div data-testid="outside">外部内容</div>
                <ErrorBoundary label="会话管理">
                    <Boom />
                </ErrorBoundary>
            </div>
        );

        // 未包裹的内容必须存活 —— 这正是"一处出错全盘黑屏"与"局部降级"的分界。
        expect(screen.getByTestId('outside')).toBeInTheDocument();
        expect(screen.getByText(/会话管理出现了一个界面错误/)).toBeInTheDocument();
        expect(screen.getByText(/渲染炸了/)).toBeInTheDocument();
    });

    it('点击重试后重新渲染子组件', () => {
        let failing = true;
        const Flaky: React.FC = () => {
            if (failing) throw new Error('第一次失败');
            return <div>已恢复</div>;
        };

        render(
            <ErrorBoundary label="面板">
                <Flaky />
            </ErrorBoundary>
        );
        expect(screen.getByText(/第一次失败/)).toBeInTheDocument();

        failing = false;
        fireEvent.click(screen.getByRole('button', { name: '重试' }));

        expect(screen.getByText('已恢复')).toBeInTheDocument();
        expect(screen.queryByText(/出现了一个界面错误/)).not.toBeInTheDocument();
    });

    it('没有异常时原样渲染子组件', () => {
        render(
            <ErrorBoundary label="面板">
                <div>正常内容</div>
            </ErrorBoundary>
        );
        expect(screen.getByText('正常内容')).toBeInTheDocument();
        expect(screen.queryByRole('button', { name: '重试' })).not.toBeInTheDocument();
    });
});
