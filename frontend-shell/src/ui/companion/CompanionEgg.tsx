import React, { useId } from 'react';

// The finite visual sequence starts only after the local draw has committed.
export const HATCH_OPEN_DURATION_MS = 3400;

/** Shared vector artwork: a porcelain shell, inset jewel and gilded seam. */
export function CompanionEgg({ opening = false, shaking = opening, compact = false }: { opening?: boolean; shaking?: boolean; compact?: boolean }) {
  const id = `egg-${useId().replace(/:/g, '')}`;
  if (compact) return <svg className="oc-companion-egg" viewBox="0 0 120 144" role="img" aria-label="伙伴蛋"><path d="M60 10C43 10 22 51 20 82c-2 26 13 45 40 45s42-19 40-45C98 51 77 10 60 10Z" fill="none" stroke="currentColor" strokeWidth="4"/><path d="m60 43 4 10 10 4-10 4-4 10-4-10-10-4 10-4ZM24 84l14-9 13 10 12-11 13 11 12-9 10 8" fill="none" stroke="currentColor" strokeWidth="3" strokeLinejoin="round"/></svg>;
  return <svg className="oc-companion-egg" data-opening={opening} data-shaking={opening && shaking} style={{'--oc-hatch-duration': `${HATCH_OPEN_DURATION_MS}ms`} as React.CSSProperties} viewBox="0 0 120 144" role="img" aria-label={opening ? '正在打开的伙伴蛋' : '伙伴蛋'}>
    <defs>
      <linearGradient id={`${id}-porcelain`} x1=".15" y1="0" x2=".9" y2="1"><stop stopColor="#fff7df"/><stop offset=".46" stopColor="#eddbb1"/><stop offset="1" stopColor="#b89562"/></linearGradient>
      <linearGradient id={`${id}-gold`} x1="0" y1="0" x2="1" y2="1"><stop stopColor="#f5dca2"/><stop offset=".5" stopColor="#a77b42"/><stop offset="1" stopColor="#e4c58a"/></linearGradient>
      <radialGradient id={`${id}-light`}><stop stopColor="#fff8df"/><stop offset="1" stopColor="#fff8df" stopOpacity="0"/></radialGradient>
      <clipPath id={`${id}-upper`}><path d="M0 0h120v77l-22 3-13-7-12 11-13-12-13 11-14-10-13 7L0 77Z"/></clipPath>
      <clipPath id={`${id}-lower`}><path d="m0 77 20 3 13-7 14 10 13-11 13 12 12-11 13 7 22-3v67H0Z"/></clipPath>
      <g id={`${id}-shell`}>
        <path d="M60 10C43 10 22 51 20 82c-2 26 13 45 40 45s42-19 40-45C98 51 77 10 60 10Z" fill={`url(#${id}-porcelain)`} stroke="#805e3c" strokeWidth="1.6"/>
        <path d="M30 73c4-19 13-39 23-48" fill="none" stroke="#fffdf1" strokeWidth="6" strokeLinecap="round" opacity=".65"/>
        <path d="M23 83q37 25 74 0M25 91q35 23 70 0" fill="none" stroke={`url(#${id}-gold)`} strokeWidth="2"/>
        <path d="m60 38 9 14-9 14-9-14Z" fill="var(--accent)" stroke={`url(#${id}-gold)`} strokeWidth="2.5"/>
        <path d="m60 45 1.8 5.2L67 52l-5.2 1.8L60 59l-1.8-5.2L53 52l5.2-1.8Z" fill="#fff8df"/>
        <g fill="#a8804f" opacity=".6"><circle cx="38" cy="67" r="1.6"/><circle cx="82" cy="67" r="1.6"/><circle cx="45" cy="108" r="2"/><circle cx="77" cy="104" r="1.5"/></g>
        <path d="m60 96 3 5-3 5-3-5Z" fill="#a8804f" opacity=".65"/>
      </g>
    </defs>
    <ellipse className="oc-egg-shadow" cx="60" cy="136" rx="31" ry="4" fill="#1e2630" opacity=".25"/>
    <circle className="oc-egg-core" cx="60" cy="79" r="59" fill={`url(#${id}-light)`}/>
    <g className="oc-companion-eggshell">
      <use className="oc-egg-whole" href={`#${id}-shell`}/>
      <g className="oc-egg-top"><use href={`#${id}-shell`} clipPath={`url(#${id}-upper)`}/></g>
      <g className="oc-egg-bottom"><use href={`#${id}-shell`} clipPath={`url(#${id}-lower)`}/></g>
      <path className="oc-egg-crack" d="m21 80 12-7 14 10 13-11 13 12 12-11 13 7M60 72l-4-12 7-8M47 83l-4 12 5 7" fill="none" stroke="#fff9e7" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" pathLength="1"/>
    </g>
    <g className="oc-egg-fragments" fill={`url(#${id}-porcelain)`} stroke="#b89562" strokeWidth=".7"><path d="m26 72 7-5 3 10Z"/><path d="m89 62 6 4-4 6Z"/><path d="m85 96 8 3-5 6Z"/></g>
  </svg>;
}
