// ============================================================
// Design Tokens — 所有设置页面组件的唯一样式真相源
// ============================================================

// ---- Colors ----
export const colors = {
  bgPrimary: '#1e1e1e',
  bgSecondary: '#252526',
  bgTertiary: '#2d2d2d',
  bgHover: '#3c3c3c',
  borderPrimary: '#3c3c3c',
  borderSubtle: '#2d2d2d',
  textPrimary: '#ffffff',
  textSecondary: '#cccccc',
  textTertiary: '#888888',
  textMuted: '#666666',
  accent: '#007acc',
  success: '#4caf50',
  danger: '#f44336',
  warning: '#ff9800',
  overlay: 'rgba(0, 0, 0, 0.7)',
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
  border: '1px solid #5a3a3a',
  backgroundColor: 'transparent',
  color: colors.danger,
  cursor: 'pointer',
  fontSize: font.sm,
};

export const btnGhost: React.CSSProperties = {
  padding: '6px 12px',
  borderRadius: radius.sm,
  border: '1px solid #444444',
  backgroundColor: 'transparent',
  color: '#aaaaaa',
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
  boxShadow: '0 4px 12px rgba(0, 0, 0, 0.5)',
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

