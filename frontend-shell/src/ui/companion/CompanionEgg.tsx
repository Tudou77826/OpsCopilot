import React from 'react';

// One duration drives both the finite CSS sequence and the committed-result reveal.
export const HATCH_OPEN_DURATION_MS = 2200;

/** Code-native shell artwork; no sprite downloads or animation loop while idle. */
export function CompanionEgg({ opening = false, shaking = opening }: { opening?: boolean; shaking?: boolean }) {
  return <svg className="oc-companion-egg" data-opening={opening} data-shaking={opening && shaking} style={{'--oc-hatch-duration': `${HATCH_OPEN_DURATION_MS}ms`} as React.CSSProperties} viewBox="0 0 120 144" role="img" aria-label={opening ? '正在打开的伙伴蛋' : '伙伴蛋'}>
    <ellipse className="oc-egg-shadow" cx="60" cy="133" rx="30" ry="5" fill="currentColor" opacity=".12"/>
    <g className="oc-companion-eggshell">
      <g className="oc-egg-top">
        <path d="M22 74C27 46 44 12 60 12s33 34 38 62L83 65 70 78 57 66 44 79 32 68Z" fill="var(--bg-elevated)" stroke="currentColor" strokeWidth="2.5" strokeLinejoin="round"/>
        <path d="M37 56c4-12 10-23 17-29" fill="none" stroke="currentColor" strokeWidth="4" strokeLinecap="round" opacity=".12"/>
        <path d="m60 36 3 8 8 3-8 3-3 8-3-8-8-3 8-3Z" fill="currentColor"/>
      </g>
      <g className="oc-egg-bottom">
        <path d="m22 74 10-6 12 11 13-13 13 12 13-13 15 9c1 4 2 8 2 11a40 40 0 0 1-80 0c0-4 1-8 2-11Z" fill="var(--bg-elevated)" stroke="currentColor" strokeWidth="2.5" strokeLinejoin="round"/>
        <circle cx="44" cy="98" r="4" fill="currentColor" opacity=".25"/>
        <circle cx="76" cy="104" r="6" fill="currentColor" opacity=".2"/>
      </g>
    </g>
  </svg>;
}
