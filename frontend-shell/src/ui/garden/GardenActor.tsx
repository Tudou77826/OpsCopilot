import React, { useRef, type CSSProperties, type PointerEvent as ReactPointerEvent } from 'react'
import { pointInGardenPolygon, type GardenActionClip, type GardenElementManifest, type GardenPack } from './contentPack'
import { gardenClipFrame, type GardenActorState } from './gardenRuntime'
import type { GardenSpecimen, ResolvedGardenVisual } from './pack'
import ImageSpecimen from './packs/imageSpecimen'

function spriteFrame(clip: Extract<GardenActionClip, { format: 'sprite-sheet' }>, pack: GardenPack, frame: number, onAssetError: (id: string) => void) {
  const asset = pack.assets[clip.asset]
  const column = frame % clip.columns
  const row = Math.floor(frame / clip.columns)
  return <span className="garden-sprite-window" data-frame={frame}>
    <img
      className="garden-sprite-sheet"
      src={asset.src}
      alt=""
      aria-hidden="true"
      decoding="async"
      loading="lazy"
      style={{
        width: `${clip.columns * 100}%`,
        height: `${clip.rows * 100}%`,
        transform: `translate(${-column * (100 / clip.columns)}%, ${-row * (100 / clip.rows)}%)`,
      }}
      onError={() => onAssetError(asset.id)}
    />
  </span>
}

function reducedSpriteClip(element: GardenElementManifest): Extract<GardenActionClip, { format: 'sprite-sheet' }> | undefined {
  return Object.values(element.actions).find((clip): clip is Extract<GardenActionClip, { format: 'sprite-sheet' }> => clip?.format === 'sprite-sheet' && clip.asset === element.reducedMotion.asset)
}

function ActorImage({ pack, element, visual, actor, now, reducedMotion, label, onAssetError }: {
  pack: GardenPack
  element: GardenElementManifest
  visual: ResolvedGardenVisual
  actor: GardenActorState
  now: number
  reducedMotion: boolean
  label: string
  onAssetError(id: string): void
}) {
  if (reducedMotion || !actor.active || !actor.action) {
    const sprite = reducedSpriteClip(element)
    if (sprite && element.reducedMotion.frame !== undefined) return spriteFrame(sprite, pack, element.reducedMotion.frame, onAssetError)
    return <ImageSpecimen src={visual.src} anchor={visual.anchor} size="100%" label={label} onError={() => onAssetError(visual.assetId)} />
  }

  const clip = element.actions[actor.action]
  const frame = gardenClipFrame(clip, actor.actionStartedAt, now, false)
  if (clip?.format === 'sprite-sheet') return spriteFrame(clip, pack, frame, onAssetError)
  if (clip?.format === 'frame-sequence') {
    const asset = pack.assets[clip.assets[frame]]
    return <img className="garden-action-frame" src={asset.src} alt="" aria-hidden="true" decoding="async" loading="lazy" onError={() => onAssetError(asset.id)} />
  }
  return <ImageSpecimen src={visual.src} anchor={visual.anchor} size="100%" label={label} onError={() => onAssetError(visual.assetId)} />
}

export default function GardenActor({ pack, element, specimen, visual, actor, now, reducedMotion, selected, editing, layer, size, label, onOpen, onMove, onStow, onReact, onAssetError }: {
  pack: GardenPack
  element: GardenElementManifest
  specimen: GardenSpecimen
  visual: ResolvedGardenVisual
  actor: GardenActorState
  now: number
  reducedMotion: boolean
  selected: boolean
  editing: boolean
  layer: number
  size: number
  label: string
  onOpen(id: string): void
  onMove?(id: string, x: number, y: number): void
  onStow?(id: string): void
  onReact(id: string): void
  onAssetError(id: string): void
}) {
  const dragged = useRef(false)
  const x = reducedMotion ? actor.homeX : actor.x
  const y = reducedMotion ? actor.homeY : actor.y
  const clip = actor.action ? element.actions[actor.action] : undefined
  const spriteAspect = clip?.format === 'sprite-sheet' ? clip.frameWidth / clip.frameHeight : undefined
  const motion = !reducedMotion && clip?.format === 'procedural' ? clip.primitive : undefined
  const placementPolygon = pack.scene.zones[element.placement.zone].polygon
  const bounds = placementPolygon.reduce((result, point) => ({
    minX: Math.min(result.minX, point.x), maxX: Math.max(result.maxX, point.x),
    minY: Math.min(result.minY, point.y), maxY: Math.max(result.maxY, point.y),
  }), { minX: 1, maxX: 0, minY: 1, maxY: 0 })
  const style = {
    left: `${x * 100}%`,
    top: `${y * 100}%`,
    width: `calc(var(--garden-world-height) * ${size * visual.defaultSize})`,
    zIndex: 20 + layer * 100 + Math.round(y * 90),
    aspectRatio: spriteAspect,
    '--garden-action-duration': clip?.format === 'procedural' ? `${clip.durationMs}ms` : undefined,
  } as CSSProperties

  const pointerPosition = (event: ReactPointerEvent<HTMLDivElement>) => {
    const world = event.currentTarget.parentElement?.getBoundingClientRect()
    if (!world) return
    const nextX = Math.max(bounds.minX, Math.min(bounds.maxX, (event.clientX - world.left) / world.width))
    const nextY = Math.max(bounds.minY, Math.min(bounds.maxY, (event.clientY - world.top) / world.height))
    if (!pointInGardenPolygon({ x: nextX, y: nextY }, placementPolygon)) return
    event.currentTarget.style.left = `${nextX * 100}%`
    event.currentTarget.style.top = `${nextY * 100}%`
    return { x: nextX, y: nextY }
  }
  const pointerDown = (event: ReactPointerEvent<HTMLDivElement>) => {
    if (!editing) return
    dragged.current = false
    event.currentTarget.setPointerCapture(event.pointerId)
  }
  const pointerMove = (event: ReactPointerEvent<HTMLDivElement>) => {
    if (!editing || !event.currentTarget.hasPointerCapture(event.pointerId)) return
    dragged.current = true
    pointerPosition(event)
  }
  const pointerUp = (event: ReactPointerEvent<HTMLDivElement>) => {
    if (!editing || !event.currentTarget.hasPointerCapture(event.pointerId)) return
    const next = pointerPosition(event)
    event.currentTarget.releasePointerCapture(event.pointerId)
    if (next && dragged.current) onMove?.(specimen.instanceId, next.x, next.y)
  }
  const runtimeFacing = clip && 'flipX' in clip && clip.flipX ? actor.facing : 'right'
  const facing = specimen.placement.flipX ? (runtimeFacing === 'left' ? 'right' : 'left') : runtimeFacing

  return <div className="garden-image-object" style={style}
    data-editing={editing ? 'true' : 'false'}
    onPointerDown={pointerDown} onPointerMove={pointerMove} onPointerUp={pointerUp}
    data-placement={element.placement.type} data-zone={element.placement.zone}
    data-quality={specimen.shiny ? 'shiny' : 'normal'}
    data-action={reducedMotion ? 'reduced-motion' : actor.action ?? 'static'}
    data-runtime-active={actor.active && !reducedMotion ? 'true' : 'false'}>
    {selected ? <span className="garden-selected-ring" aria-hidden="true" /> : null}
    <span className="garden-image-visual" aria-hidden="true">
      <span className="garden-image-facing" data-facing={facing}>
        <span className="garden-image-motion" data-motion={motion}>
          <ActorImage pack={pack} element={element} visual={visual} actor={actor} now={now} reducedMotion={reducedMotion} label={label} onAssetError={onAssetError} />
        </span>
      </span>
      {specimen.shiny && <span className="garden-image-quality" aria-hidden="true">✧</span>}
      <span className="garden-image-name">{element.name}</span>
    </span>
    {editing ? <button className="garden-stow-button" aria-label={`收回${element.name}`} title="收回背包" onPointerDown={event => event.stopPropagation()} onClick={() => onStow?.(specimen.instanceId)}>×</button> : null}
    <button className="garden-image-hit-target" aria-label={label} title={editing ? '拖动摆放' : label}
      onClick={() => { if (!editing) { onReact(specimen.instanceId); onOpen(specimen.instanceId) } }}
      style={{ left: `${visual.hitArea.x * 100}%`, top: `${visual.hitArea.y * 100}%`, width: `${visual.hitArea.width * 100}%`, height: `${visual.hitArea.height * 100}%` }} />
  </div>
}
