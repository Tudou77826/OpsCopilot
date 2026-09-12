// ============================================================
// Design Tokens — 所有设置页面组件的唯一样式真相源
// ============================================================

// ---- Colors ----
// 值全部映射到共享 shell-theme.css 的语义 token。
export const colors = {
  bgPrimary: 'var(--bg-primary)',
  bgSecondary: 'var(--bg-secondary)',
  bgTertiary: 'var(--bg-tertiary)',
  bgHover: 'var(--bg-hover)',
  borderPrimary: 'var(--border)',
  borderSubtle: 'var(--border-subtle)',
  textPrimary: 'var(--text-primary)',
  textSecondary: 'var(--text-secondary)',
  // 历史值 var(--text-muted) 对应 muted 档; var(--text-disabled) 对应 disabled 档(暗色下值完全一致)
  textTertiary: 'var(--text-muted)',
  textMuted: 'var(--text-disabled)',
  accent: 'var(--accent)',
  success: 'var(--success)',
  danger: 'var(--danger)',
  warning: 'var(--warning)',
  overlay: 'var(--overlay)',
} as const;

// ---- Border Radius ----
// 值是 shell-theme.css 的尺寸令牌，不是像素；所有使用者（含桌面旧设置页）因此自动跟随皮肤。
export const radius = {
  xs: 'var(--radius-xs)',
  sm: 'var(--radius-sm)',
  md: 'var(--radius-md)',
  lg: 'var(--radius-lg)',
  full: 'var(--radius-full)',
} as const;

// ---- Font Sizes ----
export const font = {
  xs: 'var(--font-size-xs)',
  sm: 'var(--font-size-sm)',
  base: 'var(--font-size-base)',
  lg: 'var(--font-size-lg)',
  xl: 'var(--font-size-xl)',
} as const;

// ---- Shared Component Styles ----
import React from 'react';

export const inputStyle: React.CSSProperties = {
  padding: 'var(--space-8) var(--space-12)',
  borderRadius: radius.sm,
  border: `1px solid ${colors.borderPrimary}`,
  backgroundColor: colors.bgPrimary,
  color: colors.textPrimary,
  outline: 'none',
  fontSize: font.base,
};

export const btnPrimary: React.CSSProperties = {
  padding: 'var(--space-8) var(--space-16)',
  borderRadius: radius.sm,
  border: 'none',
  backgroundColor: colors.accent,
  color: 'var(--text-on-accent)',
  cursor: 'pointer',
  fontSize: font.base,
  fontWeight: 500,
};

export const btnSecondary: React.CSSProperties = {
  padding: 'var(--space-8) var(--space-16)',
  borderRadius: radius.sm,
  border: `1px solid ${colors.borderPrimary}`,
  backgroundColor: colors.bgHover,
  color: colors.textPrimary,
  cursor: 'pointer',
  fontSize: font.base,
};

export const btnSmall: React.CSSProperties = {
  padding: 'var(--space-4) var(--space-10)',
  borderRadius: radius.sm,
  border: `1px solid ${colors.borderPrimary}`,
  backgroundColor: colors.bgHover,
  color: colors.textPrimary,
  cursor: 'pointer',
  fontSize: font.sm,
};

export const btnDanger: React.CSSProperties = {
  padding: 'var(--space-4) var(--space-10)',
  borderRadius: radius.sm,
  border: '1px solid var(--danger-border)',
  backgroundColor: 'transparent',
  color: colors.danger,
  cursor: 'pointer',
  fontSize: font.sm,
};

export const btnGhost: React.CSSProperties = {
  padding: 'var(--space-6) var(--space-12)',
  borderRadius: radius.sm,
  border: '1px solid var(--border-strong)',
  backgroundColor: 'transparent',
  color: 'var(--text-tertiary)',
  cursor: 'pointer',
  fontSize: font.sm,
};

export const sectionCard: React.CSSProperties = {
  padding: 'var(--space-16)',
  backgroundColor: colors.bgPrimary,
  borderRadius: radius.md,
  border: `1px solid ${colors.borderPrimary}`,
};

export const labelStyle: React.CSSProperties = {
  color: colors.textSecondary,
  fontSize: font.base,
  fontWeight: 500,
};

export const descStyle: React.CSSProperties = {
  color: colors.textTertiary,
  fontSize: font.sm,
  lineHeight: '1.5',
};

export const sectionTitle: React.CSSProperties = {
  color: colors.textPrimary,
  fontSize: font.lg,
  fontWeight: 600,
};

export const modalOverlay: React.CSSProperties = {
  position: 'fixed',
  top: 0,
  left: 0,
  right: 0,
  bottom: 0,
  backgroundColor: colors.overlay,
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'center',
  zIndex: 2100,
};

export const modalContainer: React.CSSProperties = {
  backgroundColor: colors.bgSecondary,
  borderRadius: radius.lg,
  display: 'flex',
  flexDirection: 'column',
  boxShadow: '0 4px 12px var(--shadow)',
  overflow: 'hidden',
};

export const modalHeader: React.CSSProperties = {
  padding: 'var(--space-16) var(--space-24)',
  borderBottom: `1px solid ${colors.borderPrimary}`,
  display: 'flex',
  justifyContent: 'space-between',
  alignItems: 'center',
  backgroundColor: colors.bgPrimary,
};

export const modalTitle: React.CSSProperties = {
  margin: 0,
  fontSize: 'var(--font-size-modal-title)',
  color: colors.textPrimary,
  fontWeight: 600,
};

export const modalCloseBtn: React.CSSProperties = {
  background: 'none',
  border: 'none',
  color: colors.textSecondary,
  fontSize: 'var(--font-size-modal-close)',
  cursor: 'pointer',
  padding: '0',
  width: '32px',
  height: 'var(--space-32)',
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'center',
  borderRadius: radius.sm,
};

// ---- Orca 风格设置页原语 ----
// 全屏设置页：内容区限宽容器
export const pageContainer: React.CSSProperties = {
  width: '100%',
  maxWidth: '1100px',
  margin: '0 auto',
  padding: 'var(--space-20) var(--space-40) var(--space-32)',
  boxSizing: 'border-box' as const,
  display: 'flex',
  flexDirection: 'column',
  gap: 'var(--space-24)',
};

// 页面顶部大标题 + 描述 + 分隔线（orca SettingsSection 头部）
export const pageHeader: React.CSSProperties = {
  display: 'flex',
  flexDirection: 'column',
  gap: 'var(--space-6)',
  paddingBottom: 'var(--space-18)',
  borderBottom: `1px solid ${colors.borderPrimary}`,
};

export const pageTitle: React.CSSProperties = {
  margin: 0,
  fontSize: 'var(--font-size-page-title)',
  fontWeight: 600,
  color: colors.textPrimary,
  lineHeight: 1.3,
  display: 'flex',
  alignItems: 'center',
  gap: 'var(--space-10)',
};

export const pageDesc: React.CSSProperties = {
  ...descStyle,
  fontSize: font.base,
  maxWidth: '720px',
};

// 内容区圆角卡片（orca SettingsSection body）
export const settingsCard: React.CSSProperties = {
  backgroundColor: 'var(--bg-dialog)',
  borderRadius: radius.lg,
  border: `1px solid ${colors.borderPrimary}`,
  padding: 'var(--space-24) var(--space-28)',
  display: 'flex',
  flexDirection: 'column',
  gap: 'var(--space-4)',
};

// 卡片内小标题
export const cardTitle: React.CSSProperties = {
  ...sectionTitle,
  fontSize: font.lg,
  marginBottom: 'var(--space-6)',
};

// 两列设置行：左 label+desc（固定宽度），右 control（自适应填充）
// 固定左列宽度保证所有行控件起点一致；输入框 flex 填充保证右缘一致
export const settingRow: React.CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  gap: 'var(--space-24)',
  padding: 'var(--space-12) 0',
};

export const settingRowTop: React.CSSProperties = {
  ...settingRow,
  alignItems: 'flex-start',
};

export const settingRowLeft: React.CSSProperties = {
  width: '320px',
  flexShrink: 0,
  display: 'flex',
  flexDirection: 'column',
  gap: 'var(--space-4)',
};

export const settingRowRight: React.CSSProperties = {
  flex: '1 1 0',
  minWidth: 0,
  display: 'flex',
  alignItems: 'center',
  gap: 'var(--space-10)',
  flexWrap: 'wrap' as const,
};

export const settingRowLabel: React.CSSProperties = {
  ...labelStyle,
};

export const settingRowDesc: React.CSSProperties = {
  ...descStyle,
};

// ---- 侧边栏导航 ----
// 分组小标题（orca：11px uppercase 字母间距 muted）
export const navGroupTitle: React.CSSProperties = {
  margin: '0 var(--space-12) var(--space-6)',
  fontSize: font.xs,
  fontWeight: 600,
  color: colors.textTertiary,
  letterSpacing: '0.08em',
  textTransform: 'uppercase' as const,
};

export const navItem: React.CSSProperties = {
  position: 'relative' as const,
  display: 'flex',
  alignItems: 'center',
  gap: 'var(--space-10)',
  width: '100%',
  padding: 'var(--space-8) var(--space-12)',
  cursor: 'pointer',
  fontSize: font.base,
  color: colors.textSecondary,
  borderRadius: radius.md,
  margin: '0',
  boxSizing: 'border-box' as const,
  transition: 'background-color 140ms ease, color 140ms ease',
};

export const navItemActive: React.CSSProperties = {
  ...navItem,
  backgroundColor: 'var(--bg-active)',
  color: colors.textPrimary,
  fontWeight: 500,
};

// 卡片分隔线
export const cardDivider: React.CSSProperties = {
  height: 1,
  backgroundColor: colors.borderSubtle,
  margin: 'var(--space-8) 0',
};

// 两列行内的输入框：flex 填充右列（左缘/右缘与其它行完全对齐），大屏封顶 520px
export const inputWide: React.CSSProperties = {
  ...inputStyle,
  boxSizing: 'border-box' as const,
  flex: '1 1 0',
  minWidth: '240px',
  maxWidth: '520px',
};
