import React from 'react';
import { colors, radius, font } from './settingsStyles';

export interface KeyMapItem {
    keys: string;
    scope: string;
    action: string;
    notes?: string;
}

interface KeysMapProps {
    commandQueryShortcut: string;
}

const KeysMap: React.FC<KeysMapProps> = ({ commandQueryShortcut }) => {
    const disclaimer = '注意：部分 Ctrl 组合键由远端 Shell/应用解释，可能因 bash/zsh/readline 模式、tmux、vim、系统或终端配置不同而不生效或表现不同。';
    const items: KeyMapItem[] = [
        {
            keys: commandQueryShortcut || 'Ctrl+K',
            scope: '全局/终端',
            action: '呼出命令查询弹窗（快速模型生成命令）',
            notes: '当焦点在输入框/文本框时不触发',
        },
        {
            keys: 'Esc',
            scope: '命令查询/命令补全/重命名',
            action: '关闭弹窗/关闭补全/取消重命名',
        },
        {
            keys: '↑ / ↓',
            scope: '终端命令补全',
            action: '补全候选上下选择',
            notes: '仅在补全提示可见时生效',
        },
        {
            keys: 'Tab',
            scope: '终端命令补全',
            action: '接受当前补全候选',
            notes: '仅在补全提示可见时生效',
        },
        {
            keys: 'Ctrl+C',
            scope: '终端',
            action: '复制选中内容',
            notes: '终端内存在选区时才会拦截为复制；否则行为交给远端程序（例如中断）',
        },
        {
            keys: 'Ctrl+V',
            scope: '终端',
            action: '粘贴剪贴板内容',
            notes: '也可使用鼠标右键粘贴/中键粘贴；中键无选区时读取系统剪贴板',
        },
        {
            keys: 'Ctrl+P',
            scope: '远端 Shell/应用',
            action: '上一条历史命令/上一行（常见行为）',
            notes: '可能不生效或不同：由远端程序解释（如 bash/zsh 的 readline）；看起来像“粘贴”其实是调出历史输入',
        },
        {
            keys: 'Ctrl+L',
            scope: '远端 Shell/应用',
            action: '清屏（常见行为）',
            notes: '可能不生效或不同：由远端程序解释（如 bash/zsh）',
        },
        {
            keys: 'Ctrl+M',
            scope: '远端 Shell/应用',
            action: '回车（CR，等同于 Enter，常见行为）',
            notes: '可能不生效或不同：Ctrl+M 会发送回车字符 \\r，被远端当作执行/换行',
        },
        {
            keys: 'Ctrl+D',
            scope: '远端 Shell/应用',
            action: 'EOF（可能退出当前程序/退出 shell）',
            notes: '可能不生效或不同：发送 EOT(\\x04)，交给远端程序处理；在 shell 提示符下常表现为 exit',
        },
        {
            keys: 'Enter',
            scope: 'AI 问答/定位助手输入框',
            action: '发送消息/开始排查',
            notes: 'Shift+Enter 换行',
        },
    ];

    return (
        <div style={styles.container}>
            <div style={styles.disclaimer}>{disclaimer}</div>
            <div style={styles.table}>
                <div style={styles.rowHeader}>
                    <div style={styles.cellKey}>快捷键</div>
                    <div style={styles.cellScope}>范围</div>
                    <div style={styles.cellAction}>作用</div>
                </div>
                {items.map((item, idx) => (
                    <div key={idx} style={styles.row}>
                        <div style={styles.cellKey}>
                            <span style={styles.keyPill}>{item.keys}</span>
                        </div>
                        <div style={styles.cellScope}>{item.scope}</div>
                        <div style={styles.cellAction}>
                            <div>{item.action}</div>
                            {item.notes && <div style={styles.notes}>{item.notes}</div>}
                        </div>
                    </div>
                ))}
            </div>
        </div>
    );
};

const styles: Record<string, React.CSSProperties> = {
    container: {
        display: 'flex',
        flexDirection: 'column',
        gap: '10px',
        maxWidth: '820px',
    },
    title: {
        color: colors.textPrimary,
        fontSize: font.lg,
        fontWeight: 600,
    },
    disclaimer: {
        color: colors.textTertiary,
        fontSize: font.sm,
        lineHeight: 1.4,
    },
    table: {
        border: `1px solid ${colors.borderPrimary}`,
        borderRadius: radius.md,
        overflow: 'hidden',
        backgroundColor: colors.bgPrimary,
    },
    rowHeader: {
        display: 'grid',
        gridTemplateColumns: '140px 140px 1fr',
        gap: '0px',
        backgroundColor: colors.bgSecondary,
        borderBottom: `1px solid ${colors.borderPrimary}`,
        padding: '8px 10px',
        fontSize: font.sm,
        color: colors.textTertiary,
    },
    row: {
        display: 'grid',
        gridTemplateColumns: '140px 140px 1fr',
        padding: '10px',
        borderBottom: `1px solid ${colors.borderSubtle}`,
        fontSize: font.sm,
        color: colors.textSecondary,
    },
    cellKey: {
        display: 'flex',
        alignItems: 'flex-start',
    },
    cellScope: {
        color: colors.textSecondary,
    },
    cellAction: {
        display: 'flex',
        flexDirection: 'column',
        gap: '4px',
    },
    keyPill: {
        display: 'inline-block',
        fontFamily: 'var(--font-mono)',
        fontSize: font.sm,
        padding: '2px 8px',
        borderRadius: '999px',
        backgroundColor: colors.bgHover,
        border: `1px solid ${colors.borderPrimary}`,
        color: colors.textPrimary,
        userSelect: 'none',
    },
    notes: {
        color: colors.textTertiary,
        fontSize: font.xs,
        lineHeight: 1.4,
    },
};

export default KeysMap;
