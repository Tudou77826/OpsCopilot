import React, { useMemo, useState } from 'react'
import FloraSpecimen from './packs/floraRenderer'
import ImageSpecimen from './packs/imageSpecimen'
import GardenScene, { Backdrop } from './GardenScene'
import { DEFAULT_PACK_ID, getPack, stageOf, stageLabel, resolveSpecimenImage, MAX_LEVEL, PITY_AT, type GardenSnapshot, type GardenSpecimen } from './pack'
// 引入即登记内置呈现包（见 packs/index.ts）。
import './packs'
import { font } from '../settings/settingsStyles'

/**
 * 花园停靠面板：与快捷命令共用槽位形态（同一处停靠、可关闭归还终端空间），
 * 内部是一整块有纵深的种植园场景——白天/夜晚跟随工作台明暗（gardenScene.css），
 * 闪光与新发现全部用视觉表达；点击植株打开详情抽屉。
 *
 * 状态只来自后端快照（宿主端口 GardenHost），前台不上报任何事件。
 */
export interface GardenHost {
  snapshot(): Promise<GardenSnapshot>
  dismiss(at: string): Promise<void>
}

export interface GardenPanelProps {
  host?: GardenHost
  isOpen?: boolean
  /** 由 OpsApp 传入的当前高度（与快捷命令共用停靠槽位时由外层持有）。 */
  height?: number
}

function levelProgress(specimen: GardenSpecimen): number {
  return Math.min(1, Math.max(0, specimen.xp / (80 + specimen.level * 20)))
}

/** 详情抽屉：只展示业务类别，不出现命令正文、凭据、服务器地址或文件内容。 */
function SpecimenDetail({ specimen, packId, onClose }: { specimen: GardenSpecimen; packId: string; onClose(): void }) {
  const pack = getPack(packId)
  const meta = pack?.species[specimen.speciesId]
  const stage = stageOf(specimen.level)
  const image = resolveSpecimenImage(pack, specimen.speciesId, specimen.level, specimen.shiny, specimen.instanceId)
  const rows: Array<[string, React.ReactNode]> = [
    ['物种', meta?.name ?? specimen.speciesId],
    ['含义', meta?.meaning ?? '—'],
    ['形态', `${stageLabel(pack, stage)}（第 ${stage + 1} / 6 档）`],
    ['品质', specimen.shiny
      ? <span key="q" className="garden-quality" data-tier="shiny">✦ 闪光</span>
      : <span key="q" className="garden-quality" data-tier="normal">普通</span>],
    ['获得', specimen.acquiredAt.slice(0, 10)],
  ]
  return (
    <aside
      role="dialog"
      aria-label={`${meta?.name ?? specimen.speciesId} 详情`}
      className="garden-drawer"
    >
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <strong style={{ fontSize: font.base }}>{meta?.name ?? specimen.speciesId}</strong>
        <button type="button" onClick={onClose} aria-label="关闭详情">×</button>
      </div>
      {/* 预览底座与场景同款（天空 + 草地）；image 画法直接放图，SVG 画法用无盆地面形态 */}
      <div className="garden-drawer-stage">
        <div style={{ position: 'absolute', left: '50%', bottom: -2, transform: 'translateX(-50%)' }}>
          {image
            ? <ImageSpecimen src={image.src} anchor={image.anchor} size={94} label={meta?.name} />
            : <FloraSpecimen speciesId={specimen.speciesId} level={specimen.level} shiny={specimen.shiny} size={94} label={meta?.name} variant="grounded" />}
        </div>
      </div>
      <div className="garden-progress-row">
        等级 {specimen.level} / {MAX_LEVEL}{specimen.level >= MAX_LEVEL ? '（常青）' : ''} · 本级成长 {Math.round(levelProgress(specimen) * 100)}%
        <span className="garden-pity-bar" aria-hidden="true">
          <span style={{ width: `${Math.round(levelProgress(specimen) * 100)}%` }} />
        </span>
      </div>
      <dl style={{ margin: '6px 0 0', fontSize: font.sm }}>
        {rows.map(([key, value]) => (
          <div key={key} style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: '8px', padding: '2.5px 0' }}>
            <dt>{key}</dt>
            <dd>{value}</dd>
          </div>
        ))}
      </dl>
    </aside>
  )
}

export default function GardenPanel({ host, isOpen = true, height = 260 }: GardenPanelProps) {
  const [snapshot, setSnapshot] = useState<GardenSnapshot | null>(null)
  const [error, setError] = useState('')
  const [openId, setOpenId] = useState<string>()

  React.useEffect(() => {
    if (!isOpen || !host) return
    let cancelled = false
    // 快照只在面板打开时拉一次（不做后台轮询：花园不在前台时零流量）。
    void host.snapshot().then(next => { if (!cancelled) { setSnapshot(next); setError('') } })
      .catch((reason: Error) => { if (!cancelled) setError(reason.message) })
    return () => { cancelled = true }
  }, [host, isOpen])

  const specimens = useMemo(() => snapshot?.specimens ?? [], [snapshot])
  const pending = useMemo(() => snapshot?.pending ?? [], [snapshot])
  const opened = specimens.find(spec => spec.instanceId === openId)

  const dismissPending = (at: string) => {
    setSnapshot(current => current ? {
      ...current,
      pending: (current.pending ?? []).filter(item => item.at !== at),
    } : current)
    void host?.dismiss(at).catch(() => {})
  }

  if (!host) {
    return (
      <div className="garden-root" style={{ height }}>
        <div className="garden-message">当前宿主未提供花园能力。</div>
      </div>
    )
  }
  if (error) {
    // 花园加载失败不影响终端与连接；错误必须显式说明原因。
    return (
      <div className="garden-root" style={{ height }}>
        <div className="garden-message" data-kind="error" role="status">花园加载失败：{error}</div>
      </div>
    )
  }

  return (
    <div className="garden-root" style={{ height, border: '1px solid var(--g-chip-border)' }}>
      {specimens.length === 0 ? (
        <>
          <Backdrop scene={getPack(DEFAULT_PACK_ID)?.images?.scene} />
          <div className="garden-empty" role="status">
            🌱 空花园 —— 建立一次连接、完成一次传输或跑完一次脚本，就会长出第一株
          </div>
        </>
      ) : (
        <GardenScene
          specimens={specimens}
          pending={pending}
          packId={DEFAULT_PACK_ID}
          selectedId={openId}
          onOpen={setOpenId}
          onDismissPending={dismissPending}
        />
      )}
      <div className="garden-chip-row">
        <span className="garden-chip" style={{ fontWeight: 600 }}>
          花园
          <span className="garden-chip-muted">
            {snapshot ? `等级 ${snapshot.gardenLevel} · ${specimens.length} 株` : '载入中…'}
          </span>
        </span>
        <span className="garden-chip garden-chip-muted" style={{ marginLeft: 'auto' }} title={`${snapshot?.pitySinceShiny ?? 0} / ${PITY_AT}，集满必出闪光`}>
          ✦ 保底 {snapshot?.pitySinceShiny ?? 0}/{PITY_AT}
          <span className="garden-pity-bar">
            <span style={{ width: `${Math.min(100, Math.round(((snapshot?.pitySinceShiny ?? 0) / PITY_AT) * 100))}%` }} />
          </span>
        </span>
      </div>
      {opened ? <SpecimenDetail specimen={opened} packId={DEFAULT_PACK_ID} onClose={() => setOpenId(undefined)} /> : null}
    </div>
  )
}
