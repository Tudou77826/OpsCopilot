import React from 'react';
import { colors } from './settingsStyles';

interface SwitchProps {
  checked: boolean;
  onChange: (checked: boolean) => void;
  disabled?: boolean;
  size?: 'default' | 'small';
}

const SIZES = {
  default: {
    trackW: 40, trackH: 20,
    thumbW: 14, thumbH: 14,
    thumbTop: 3, thumbLeft: 3, slide: 20,
  },
  small: {
    trackW: 32, trackH: 16,
    thumbW: 12, thumbH: 12,
    thumbTop: 2, thumbLeft: 2, slide: 16,
  },
} as const;

const Switch: React.FC<SwitchProps> = ({ checked, onChange, disabled = false, size = 'default' }) => {
  const s = SIZES[size];

  const trackStyle: React.CSSProperties = {
    position: 'relative',
    display: 'inline-block',
    width: s.trackW,
    minWidth: s.trackW,
    height: s.trackH,
    backgroundColor: checked ? colors.accent : 'var(--border-strong)',
    borderRadius: s.trackH,
    cursor: disabled ? 'not-allowed' : 'pointer',
    opacity: disabled ? 0.55 : 1,
    transition: 'background-color 0.25s ease',
    flexShrink: 0,
    verticalAlign: 'middle',
  };

  const thumbStyle: React.CSSProperties = {
    position: 'absolute',
    top: s.thumbTop,
    left: s.thumbLeft,
    width: s.thumbW,
    height: s.thumbH,
    backgroundColor: 'var(--text-primary)',
    borderRadius: '50%',
    transition: 'transform 0.25s ease',
    transform: checked ? `translateX(${s.slide}px)` : 'translateX(0)',
    boxShadow: '0 1px 3px rgba(0,0,0,0.3)',
  };

  const inputStyle: React.CSSProperties = {
    position: 'absolute',
    opacity: 0,
    width: 0,
    height: 0,
    margin: 0,
    padding: 0,
    pointerEvents: 'none',
  };

  return (
    <label style={trackStyle}>
      <input
        type="checkbox"
        checked={checked}
        disabled={disabled}
        onChange={(e) => onChange(e.target.checked)}
        style={inputStyle}
      />
      <span style={thumbStyle} />
    </label>
  );
};

export default Switch;
