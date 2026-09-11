import React from 'react';

type Props = {
    /** 出错时展示的区域名，例如「会话管理」。 */
    label: string;
    children: React.ReactNode;
};

type State = { error: Error | null };

/**
 * 渲染错误边界。
 *
 * React 在渲染期抛出的异常会卸载整棵组件树，用户看到的是整个应用黑屏而不是出错的
 * 那一块。把它包在面板外层，故障就被限制在该面板内：显示原因并允许重试，其余功能
 * 仍然可用。
 *
 * 这类边界不替代修 bug——它只是把"一处出错全盘不可用"降级为"一处出错局部不可用"。
 */
class ErrorBoundary extends React.Component<Props, State> {
    state: State = { error: null };

    static getDerivedStateFromError(error: Error): State {
        return { error };
    }

    componentDidCatch(error: Error, info: React.ErrorInfo): void {
        console.error(`[${this.props.label}] 渲染失败`, error, info.componentStack);
    }

    render(): React.ReactNode {
        const { error } = this.state;
        if (!error) return this.props.children;

        return (
            <div style={styles.box}>
                <div style={styles.title}>{this.props.label}出现了一个界面错误</div>
                <div style={styles.message}>{error.message || String(error)}</div>
                <button style={styles.button} onClick={() => this.setState({ error: null })}>
                    重试
                </button>
            </div>
        );
    }
}

const styles: Record<string, React.CSSProperties> = {
    box: {
        display: 'flex',
        flexDirection: 'column',
        gap: 10,
        alignItems: 'flex-start',
        padding: 16,
        margin: 10,
        border: '1px solid var(--danger)',
        borderRadius: 6,
        backgroundColor: 'var(--bg-secondary)',
        color: 'var(--text-secondary)',
        fontSize: 13,
    },
    title: { color: 'var(--text-primary)', fontWeight: 600 },
    message: {
        fontFamily: 'monospace',
        fontSize: 12,
        wordBreak: 'break-word',
        whiteSpace: 'pre-wrap',
        color: 'var(--danger)',
    },
    button: {
        padding: '6px 14px',
        borderRadius: 4,
        border: '1px solid var(--border-strong)',
        backgroundColor: 'var(--bg-elevated)',
        color: 'var(--text-primary)',
        cursor: 'pointer',
    },
};

export default ErrorBoundary;
