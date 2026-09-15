import React, { useMemo, useState } from 'react'
import FloraSpecimen, { stageNames } from './packs/floraRenderer'
import { DEFAULT_PACK_ID, getPack, stageOf, MAX_LEVEL, type GardenSnapshot, type GardenSpecimen } from './pack'
// 引入即登记内置呈现包（见 packs/index.ts）。
import './packs'
import { font } from '../settings/settingsStyles'
import { gardenPanel, gardenScene } from './palette'

// 面板用到的颜色统一在这里合成：场景色来自 gardenScene，卡片/文字来自 gardenPanel。
const panelPalette = { ...gardenScene, ...gardenPanel }

/**
 * 花园停靠面板：复用快捷命令的槽位形态（同一处停靠、可调高、关闭归还终端空间），
 * 但内部是完整场景——分组条 + 命令网格在这里没有复用价值（garden-design.md §5.2）。
 *
 * 视觉是花园自己的固定体系：不引用工作台主题令牌，因此三套皮肤下看到同一座花园。
 */
export interface GardenHost {
  snapshot(): Promise<GardenSnapshot>
  dismiss(at: string): Promise<void>
}

export interface GardenPanelProps {
  host?: GardenHost
  isOpen?: boolean
  /** 由 OpsApp 传入的当前高度（与快捷命令共用拖动把手时由外层持有）。 */
  height?: number
}


function levelProgress(specimen: GardenSpecimen): number {
  return Math.min(1, Math.max(0, specimen.xp / (80 + specimen.level * 20)))
}

/** 单株卡片：形象 + 等级 + 进度；点击打开详情。 */
function SpecimenCard({ specimen, packId, onOpen }: { specimen: GardenSpecimen; packId: string; onOpen(): void }) {
  const pack = getPack(packId)
  const meta = pack?.species[specimen.speciesId]
  const label = `${meta?.name ?? specimen.speciesId} ${specimen.level} 级 ${stageNames[stageOf(specimen.level)]}${specimen.shiny ? ' 闪光' : ''}`
  return (
    <button
      type="button"
      onClick={onOpen}
      title={label}
      aria-label={label}
      style={{
        display: 'flex', flexDirection: 'column', alignItems: 'center', gap: '4px',
        padding: '8px 6px', minWidth: '92px', cursor: 'pointer',
        background: panelPalette.card, border: `1px solid ${panelPalette.frame}`, borderRadius: '10px',
      }}
    >
      <FloraSpecimen speciesId={specimen.speciesId} level={specimen.level} shiny={specimen.shiny} size={64} label={label} />
      <span style={{ fontSize: font.sm, color: panelPalette.text }}>
        {meta?.name ?? specimen.speciesId}
        {specimen.shiny ? <span style={{ color: panelPalette.shinyMark, marginLeft: '4px' }}>✦闪光</span> : null}
      </span>
      <span style={{ fontSize: font.xs, color: panelPalette.textMuted }}>
        Lv.{specimen.level}{specimen.level >= MAX_LEVEL ? ' 常青' : ''}
      </span>
      <span aria-hidden="true" style={{ width: '60px', height: '3px', background: panelPalette.sceneShade, borderRadius: '2px', overflow: 'hidden' }}>
        <span style={{ display: 'block', width: `${Math.round(levelProgress(specimen) * 100)}%`, height: '100%', background: panelPalette.accent }} />
      </span>
    </button>
  )
}

/** 详情抽屉：只展示业务类别，不出现命令正文、凭据、服务器地址或文件内容。 */
function SpecimenDetail({ specimen, packId, onClose }: { specimen: GardenSpecimen; packId: string; onClose(): void }) {
  const pack = getPack(packId)
  const meta = pack?.species[specimen.speciesId]
  const rows: Array<[string, string]> = [
    ['物种', meta?.name ?? specimen.speciesId],
    ['含义', meta?.meaning ?? '—'],
    ['等级', `${specimen.level} / ${MAX_LEVEL}${specimen.level >= MAX_LEVEL ? '（常青）' : ''}`],
    ['形态', `${stageNames[stageOf(specimen.level)]}（第 ${stageOf(specimen.level) + 1} / 6 档）`],
    ['品质', specimen.shiny ? '闪光' : '普通'],
    ['获得', specimen.acquiredAt.slice(0, 10)],
    ['最近成长', specimen.lastGrewAt.slice(0, 10)],
  ]
  return (
    <aside
      role="dialog"
      aria-label={`${meta?.name ?? specimen.speciesId} 详情`}
      style={{
        position: 'absolute', top: 0, right: 0, bottom: 0, width: '260px', padding: '12px',
        background: panelPalette.card, borderLeft: `1px solid ${panelPalette.frame}`, overflow: 'auto',
      }}
    >
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '8px' }}>
        <strong style={{ color: panelPalette.text, fontSize: font.base }}>{meta?.name ?? specimen.speciesId}</strong>
        <button type="button" onClick={onClose} aria-label="关闭详情" style={{ background: 'transparent', border: 'none', cursor: 'pointer', color: panelPalette.textMuted }}>×</button>
      </div>
      <FloraSpecimen speciesId={specimen.speciesId} level={specimen.level} shiny={specimen.shiny} size={88} label={meta?.name} />
      <dl style={{ margin: '10px 0 0', fontSize: font.sm }}>
        {rows.map(([key, value]) => (
          <div key={key} style={{ display: 'flex', justifyContent: 'space-between', gap: '8px', padding: '3px 0' }}>
            <dt style={{ color: panelPalette.textMuted }}>{key}</dt>
            <dd style={{ margin: 0, color: panelPalette.text, textAlign: 'right' }}>{value}</dd>
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
  const opened = specimens.find(spec => spec.instanceId === openId)

  if (!host) {
    return <div style={{ padding: '12px', color: panelPalette.textMuted, fontSize: font.sm }}>当前宿主未提供花园能力。</div>
  }
  if (error) {
    // 花园加载失败不影响终端与连接；错误必须显式说明原因。
    return <div role="status" style={{ padding: '12px', color: panelPalette.danger, fontSize: font.sm }}>花园加载失败：{error}</div>
  }

  return (
    // 花园不随工作台主题变化（garden-design.md §4），因此外框要明确处理，
    // 让它在暗色工作区里看起来是有意嵌入的一块场景，而不是渲染缺陷。
    <div style={{
      position: 'relative', height, display: 'flex', flexDirection: 'column',
      background: panelPalette.scene, color: panelPalette.text,
      border: `1px solid ${panelPalette.frame}`, borderRadius: '8px', overflow: 'hidden',
    }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: '10px', padding: '8px 12px 4px' }}>
        <strong style={{ fontSize: font.base }}>花园</strong>
        <span style={{ fontSize: font.xs, color: panelPalette.textMuted }}>
          {snapshot ? `等级 ${snapshot.gardenLevel} · 收藏 ${specimens.length} 株` : '载入中…'}
        </span>
        <span style={{ marginLeft: 'auto', fontSize: font.xs, color: panelPalette.textMuted }}>
          距上次闪光 {snapshot?.pitySinceShiny ?? 0} 次发现
        </span>
      </div>
      <div style={{ flex: 1, overflow: 'auto', padding: '8px 12px 12px', display: 'flex', flexWrap: 'wrap', gap: '10px', alignContent: 'flex-start' }}>
        {specimens.length === 0 ? (
          <p style={{ fontSize: font.sm, color: panelPalette.textMuted, margin: '12px 0' }}>
            还没有植物。建立一次连接、完成一次传输或跑完一次脚本，就会长出第一株。
          </p>
        ) : specimens.map(spec => (
          <SpecimenCard key={spec.instanceId} specimen={spec} packId={DEFAULT_PACK_ID} onOpen={() => setOpenId(spec.instanceId)} />
        ))}
      </div>
      {/* 场景底部：土地与围栏，让空花园也有"一块可种植的土地" */}
      <div aria-hidden="true" style={{ height: '10px', background: panelPalette.soil, borderTop: `1px solid ${panelPalette.frame}` }} />
      {opened ? <SpecimenDetail specimen={opened} packId={DEFAULT_PACK_ID} onClose={() => setOpenId(undefined)} /> : null}
    </div>
  )
}
