/* 门禁 fixture：只用令牌、用令牌派生的 color-mix、以及全透明，都应放行。 */
export const Good = () => (
  <div
    style={{
      color: 'var(--text-primary)',
      background: 'color-mix(in srgb, var(--accent) 20%, transparent)',
      boxShadow: '0 0 0 4px rgba(0, 0, 0, 0)',
    }}
  />
);
