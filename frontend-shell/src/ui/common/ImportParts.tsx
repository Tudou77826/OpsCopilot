import React from 'react';

/**
 * 导入类对话框的共享外壳与原语。
 *
 * 会话导入与快捷命令导入的界面骨架是同一套：来源选择 → 预览（数字网格 + 提示列表）
 * → 结果报告。把外壳与这些原语收在一处，两边的差异就只剩各自的内容——这类样式在
 * 两个对话框里各写一遍，正是上一轮重构清掉的那类债。
 */

type ShellProps = {
    title: string;
    /** 有值时禁用关闭与遮罩点击，避免写到一半被关掉。 */
    busy?: string;
    onClose: () => void;
    footer: React.ReactNode;
    children: React.ReactNode;
    /** 弹窗宽度。逐条列出命令的导入面板需要更宽，默认 720。 */
    width?: number;
};

export const ImportDialogShell: React.FC<ShellProps> = ({ title, busy, onClose, footer, children, width }) => (
    <div style={importStyles.overlay} onClick={(e) => e.target === e.currentTarget && !busy && onClose()}>
        <div style={{ ...importStyles.modal, ...(width ? { width, maxWidth: '96vw' } : null) }} onClick={(e) => e.stopPropagation()}>
            <div style={importStyles.header}>
                <h2 style={importStyles.title}>{title}</h2>
                <button style={importStyles.closeButton} onClick={onClose} disabled={!!busy} title="关闭">
                    ✕
                </button>
            </div>
            <div style={importStyles.body}>{children}</div>
            <div style={importStyles.footer}>{footer}</div>
        </div>
    </div>
);

export const ImportSection: React.FC<{ title: string; children: React.ReactNode }> = ({ title, children }) => (
    <section style={importStyles.section}>
        <div style={importStyles.sectionTitle}>{title}</div>
        {children}
    </section>
);

export const ImportStatGrid: React.FC<{ children: React.ReactNode }> = ({ children }) => (
    <div style={importStyles.statGrid}>{children}</div>
);

export const ImportStat: React.FC<{ label: string; value: number; tone?: 'ok' | 'warn' }> = ({ label, value, tone }) => (
    <div style={importStyles.statCard}>
        <div
            style={{
                ...importStyles.statValue,
                color: tone === 'ok' ? 'var(--success)' : tone === 'warn' ? 'var(--warning)' : 'var(--text-primary)',
            }}
        >
            {value}
        </div>
        <div style={importStyles.statLabel}>{label}</div>
    </div>
);

/** warnings 允许为 null：后端契约应为空数组，但边界异常不应升级为整页白屏。 */
export const ImportWarningList: React.FC<{ warnings: string[] | null | undefined }> = ({ warnings }) => {
    const list = Array.isArray(warnings) ? warnings : [];
    if (list.length === 0) return null;
    return (
        <details style={importStyles.warningBox}>
            <summary style={importStyles.warningSummary}>{list.length} 条提示</summary>
            <ul style={importStyles.warningList}>
                {list.map((w, i) => (
                    <li key={i}>{w}</li>
                ))}
            </ul>
        </details>
    );
};

/**
 * 覆盖层内的公共样式。
 *
 * 只放两个导入对话框都要用的键；各自特有的（凭据状态条、导出引导、内联提示框）
 * 留在各自文件里，避免这里长成一个什么都装的杂物间。
 */
export const importStyles: Record<string, React.CSSProperties> = {
    overlay: {
        position: 'fixed',
        inset: 0,
        backgroundColor: 'rgba(0, 0, 0, 0.7)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        zIndex: 1100,
    },
    modal: {
        backgroundColor: 'var(--bg-tertiary)',
        borderRadius: 8,
        width: 720,
        maxWidth: '92vw',
        maxHeight: '88vh',
        display: 'flex',
        flexDirection: 'column',
        color: 'var(--text-primary)',
        boxShadow: '0 8px 32px rgba(0,0,0,0.5)',
    },
    header: {
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        padding: '16px 20px 8px',
    },
    title: { margin: 0, fontSize: '1.15rem' },
    closeButton: {
        background: 'transparent',
        border: 'none',
        color: 'var(--text-muted)',
        fontSize: 16,
        cursor: 'pointer',
    },
    body: { overflowY: 'auto', padding: '0 20px', display: 'flex', flexDirection: 'column', gap: 14 },
    section: {
        border: '1px solid var(--border)',
        borderRadius: 6,
        padding: '12px 14px',
        backgroundColor: 'var(--bg-secondary)',
    },
    sectionTitle: { fontSize: 13, fontWeight: 600, marginBottom: 6 },
    subTitle: { fontSize: 12, fontWeight: 600, color: 'var(--text-primary)' },
    sectionHint: { fontSize: 12, color: 'var(--text-muted)', marginBottom: 8 },
    divider: { height: 1, backgroundColor: 'var(--border)', margin: '10px 0' },
    radioRow: { display: 'flex', alignItems: 'center', gap: 8, padding: '4px 0', fontSize: 13, cursor: 'pointer' },
    radioLabel: { flexShrink: 0 },
    pathText: {
        color: 'var(--text-muted)',
        fontSize: 11,
        overflow: 'hidden',
        textOverflow: 'ellipsis',
        whiteSpace: 'nowrap',
        direction: 'rtl',
    },
    muted: { color: 'var(--text-muted)' },
    buttonRow: { display: 'flex', gap: 8, flexWrap: 'wrap' },
    // 全块唯一的"选择结论"：把多种来源收敛成一行，避免同一个路径在多处出现、
    // 让人误以为别处也参与了选择。
    selectionLine: {
        marginTop: 10,
        paddingTop: 10,
        borderTop: '1px solid var(--border)',
        fontSize: 12,
        color: 'var(--text-secondary)',
    },
    selectionLineEmpty: {
        marginTop: 10,
        paddingTop: 10,
        borderTop: '1px solid var(--border)',
        fontSize: 12,
        color: 'var(--text-muted)',
    },
    selectionPath: {
        marginLeft: 6,
        color: 'var(--text-primary)',
        wordBreak: 'break-all',
    },
    statGrid: { display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(120px, 1fr))', gap: 8 },
    statCard: {
        backgroundColor: 'var(--bg-primary)',
        border: '1px solid var(--border)',
        borderRadius: 6,
        padding: '8px 10px',
    },
    statValue: { fontSize: 18, fontWeight: 600 },
    statLabel: { fontSize: 11, color: 'var(--text-muted)', marginTop: 2 },
    warningBox: { marginTop: 8, fontSize: 12 },
    warningSummary: { cursor: 'pointer', color: 'var(--text-muted)' },
    warningList: { margin: '6px 0 0', paddingLeft: 18, color: 'var(--text-secondary)', lineHeight: 1.7 },
    error: { color: 'var(--danger)', fontSize: 12 },
    footer: {
        display: 'flex',
        justifyContent: 'flex-end',
        gap: 10,
        padding: '12px 20px 16px',
    },
    cancelButton: {
        padding: '8px 16px',
        borderRadius: 6,
        border: '1px solid var(--border-strong)',
        backgroundColor: 'transparent',
        color: 'var(--text-primary)',
        cursor: 'pointer',
    },
    secondaryButton: {
        padding: '8px 16px',
        borderRadius: 6,
        border: '1px solid var(--border-strong)',
        backgroundColor: 'var(--bg-elevated)',
        color: 'var(--text-primary)',
        cursor: 'pointer',
    },
    primaryButton: {
        padding: '8px 18px',
        borderRadius: 6,
        border: 'none',
        backgroundColor: 'var(--accent)',
        color: 'var(--text-on-accent)',
        cursor: 'pointer',
        fontWeight: 600,
    },
};
