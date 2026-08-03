// ============================================================
// Design Tokens — 所有设置页面组件的唯一样式真相源
// ============================================================

// ---- Colors ----
// 值全部映射到 App.css 的语义 token(暗色下与原值视觉一致,亮色下自动切换)
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
export const radius = {
  sm: '4px',
  md: '6px',
  lg: '8px',
  full: '20px',
} as const;

// ---- Font Sizes ----
export const font = {
  xs: '11px',
  sm: '12px',
  base: '13px',
  lg: '14px',
  xl: '18px',
} as const;

// ---- Shared Component Styles ----
import React from 'react';

export const inputStyle: React.CSSProperties = {
  padding: '8px 12px',
  borderRadius: radius.sm,
  border: `1px solid ${colors.borderPrimary}`,
  backgroundColor: colors.bgPrimary,
  color: colors.textPrimary,
  outline: 'none',
  fontSize: font.base,
};

export const btnPrimary: React.CSSProperties = {
  padding: '8px 16px',
  borderRadius: radius.sm,
  border: 'none',
  backgroundColor: colors.accent,
  color: colors.textPrimary,
  cursor: 'pointer',
  fontSize: font.base,
  fontWeight: 500,
};

export const btnSecondary: React.CSSProperties = {
  padding: '8px 16px',
  borderRadius: radius.sm,
  border: `1px solid ${colors.borderPrimary}`,
  backgroundColor: colors.bgHover,
  color: colors.textPrimary,
  cursor: 'pointer',
  fontSize: font.base,
};

export const btnSmall: React.CSSProperties = {
  padding: '4px 10px',
  borderRadius: radius.sm,
  border: `1px solid ${colors.borderPrimary}`,
  backgroundColor: colors.bgHover,
  color: colors.textPrimary,
  cursor: 'pointer',
  fontSize: font.sm,
};

export const btnDanger: React.CSSProperties = {
  padding: '4px 10px',
  borderRadius: radius.sm,
  border: '1px solid var(--danger-border)',
  backgroundColor: 'transparent',
  color: colors.danger,
  cursor: 'pointer',
  fontSize: font.sm,
};

export const btnGhost: React.CSSProperties = {
  padding: '6px 12px',
  borderRadius: radius.sm,
  border: '1px solid var(--border-strong)',
  backgroundColor: 'transparent',
  color: 'var(--text-tertiary)',
  cursor: 'pointer',
  fontSize: font.sm,
};

export const sectionCard: React.CSSProperties = {
  padding: '16px',
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
  padding: '16px 24px',
  borderBottom: `1px solid ${colors.borderPrimary}`,
  display: 'flex',
  justifyContent: 'space-between',
  alignItems: 'center',
  backgroundColor: colors.bgPrimary,
};

export const modalTitle: React.CSSProperties = {
  margin: 0,
  fontSize: '1.1rem',
  color: colors.textPrimary,
  fontWeight: 600,
};

export const modalCloseBtn: React.CSSProperties = {
  background: 'none',
  border: 'none',
  color: colors.textSecondary,
  fontSize: '1.5rem',
  cursor: 'pointer',
  padding: '0',
  width: '32px',
  height: '32px',
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'center',
  borderRadius: radius.sm,
};

