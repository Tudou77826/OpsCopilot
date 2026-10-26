import type { CompanionStage, CompanionPalette } from './types';

export type CompanionBaseAction = 'idle' | 'happy' | 'greet' | 'sleep';
export type CompanionAmbientAction = 'look' | 'stretch' | 'walk';
export type CompanionTransitionAction = 'stand' | 'turnOut' | 'walkStart' | 'walkStop' | 'turnIn';
export type CompanionAction = CompanionBaseAction | CompanionAmbientAction | CompanionTransitionAction;
export interface PixelAsset {
  path: string;
  role: 'character' | 'icon' | 'background';
  width: number;
  height: number;
}
export interface CompanionClip {
  frames: string[];
  fps: number;
  /** Optional pose timing; duration includes the pause before the next pose. */
  sequence?: { frame: number; durationMs: number }[];
  /** Finite local responses finish on the last pose, then notify their host. */
  loop?: boolean;
  /** Deliberate still pose for reduced motion; it need not be frame zero. */
  staticFrame?: number;
  /** Shared foot anchor in the content pack's native frame. */
  anchor: { x: number; y: number };
}
/** Companion art contracts, checked by check-companion-assets.mjs.
 * Content specifies appearance only. It cannot declare XP, prices, friendship
 * rewards, scripts or arbitrary character movement.
 */
interface ContentPackBase {
  id: string;
  speciesId: string;
  style: 'pixel';
  assets: Record<string, PixelAsset>;
  stages: Record<CompanionStage, Record<CompanionBaseAction, CompanionClip>>;
}
export interface PixelContentPack extends ContentPackBase {
  contractVersion: 1;
  palette: string[];
  /** Target colors at the same indexes as the base palette; recoloring keeps
   * geometry and frame timing intact. Assets use the base palette only. */
  palettes: Record<CompanionPalette, string[]>;
}

/** Preserve the AI source's colors and soft alpha; no indexed-color reduction. */
export interface DetailedContentPack extends ContentPackBase {
  contractVersion: 2;
  frameSize: 256;
  colorMode: 'rgba';
  paletteHues: Record<CompanionPalette, number | null>;
  /** Three stages share the same native artwork instead of three bitmap copies. */
  stageScale?: Record<CompanionStage, number>;
  /** Build-checked union of visible pixels across every authored pose. */
  sceneBounds?: { left: number; top: number; right: number; bottom: number };
  /** Authored ambient poses share the base frames' foot anchor and scale. */
  ambient?: Record<CompanionAmbientAction, CompanionClip>;
  locomotion?: {
    /** Distance in native artwork units per complete gait cycle. */
    stride: number;
    /** Grounded poses where an interrupted walk may enter its stop clip. */
    contacts: number[];
    /** Alternate grounded contacts need phase-matched settling poses. Contact 0 uses walkStop. */
    stopByContact?: Record<string, CompanionClip>;
    phases: Record<CompanionTransitionAction, CompanionClip>;
  };
}
export type CompanionContentPack = PixelContentPack | DetailedContentPack;

/** Presentation and travel use the same scale; native artwork stays unchanged. */
export const COMPANION_DISPLAY_SCALE = 0.7;
export function companionDisplayScale(pack: CompanionContentPack, stage: CompanionStage): number {
  return COMPANION_DISPLAY_SCALE * (pack.contractVersion === 2 ? pack.stageScale?.[stage] ?? 1 : 1);
}

export function companionClip(pack: CompanionContentPack, stage: CompanionStage, action: CompanionAction, stopContact?: number): CompanionClip {
  if(action==='walkStop'&&stopContact!==undefined&&pack.contractVersion===2&&pack.locomotion?.stopByContact?.[stopContact])return pack.locomotion.stopByContact[stopContact];
  if(action==='stand'||action==='turnOut'||action==='walkStart'||action==='walkStop'||action==='turnIn') return (pack.contractVersion===2?pack.locomotion?.phases[action]:undefined)??pack.stages[stage].idle;
  if (action === 'look' || action === 'stretch' || action === 'walk') return (pack.contractVersion === 2 ? pack.ambient?.[action] : undefined) ?? pack.stages[stage].idle;
  return pack.stages[stage][action];
}

export function clipDuration(clip: CompanionClip): number {
  return (clip.sequence??clip.frames.map((_,frame)=>({frame,durationMs:1000/clip.fps}))).reduce((sum,pose)=>sum+pose.durationMs,0);
}

/** A finite number of complete cycles ends on the contact pose instead of an arbitrary frame. */
export function playbackStep(clip: CompanionClip, elapsedMs: number, repetitions?: number) {
  if(repetitions!==undefined&&elapsedMs>=clipDuration(clip)*repetitions) return {frame:0,waitMs:undefined};
  return animationStep(repetitions===undefined?clip:{...clip,loop:true},elapsedMs);
}

/** Visible artwork height, including the display scale and authored footer.
 * Scene fitting must not mistake the sprite's transparent canvas for its body. */
export function visibleFrameHeightRatio(pack: CompanionContentPack, stage: CompanionStage): number {
  const clip=pack.stages[stage].idle, height=pack.assets[clip.frames[0]].height;
  const bounds=pack.contractVersion===2 ? pack.sceneBounds : undefined;
  return (bounds ? bounds.bottom-bounds.top : height-12)*companionDisplayScale(pack,stage)/height;
}

/** Symmetric radius around the shared anchor keeps every pose inside its lane. */
export function visibleFrameWidthRatio(pack: CompanionContentPack, stage: CompanionStage): number {
  const clip=pack.stages[stage].idle, width=pack.assets[clip.frames[0]].width;
  const bounds=pack.contractVersion===2 ? pack.sceneBounds : undefined;
  return bounds ? 2*Math.max(clip.anchor.x-bounds.left,bounds.right-clip.anchor.x)/width : 1;
}

export function animationFrameIndex(clip: CompanionClip, elapsedMs: number): number {
  if (!Number.isFinite(elapsedMs) || elapsedMs < 0) return 0;
  return animationStep(clip, elapsedMs).frame;
}

export function animationStep(clip: CompanionClip, elapsedMs: number): { frame: number; waitMs?: number } {
  const poses = clip.sequence ?? clip.frames.map((_, frame) => ({ frame, durationMs: 1000 / clip.fps }));
  const duration = poses.reduce((sum, pose) => sum + pose.durationMs, 0);
  const elapsed = Number.isFinite(elapsedMs) ? Math.max(0, elapsedMs) : 0;
  if (clip.loop === false && elapsed >= duration) return { frame: poses[poses.length - 1].frame };
  let position = elapsed % duration;
  for (const pose of poses) {
    if (position < pose.durationMs) return { frame: pose.frame, waitMs: Math.max(1, Math.ceil(pose.durationMs - position)) };
    position -= pose.durationMs;
  }
  return { frame: poses[0].frame, waitMs: 1 };
}

export function animationFrame(clip: CompanionClip, elapsedMs: number, reducedMotion = false): string {
  if (reducedMotion || !Number.isFinite(elapsedMs) || elapsedMs < 0) return clip.frames[0];
  return clip.frames[animationFrameIndex(clip, elapsedMs)];
}
