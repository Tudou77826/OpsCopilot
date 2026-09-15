import React, { useEffect, useRef, useState, type CSSProperties } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import day from './assets/day.png'
import night from './assets/night.png'
import mint from './assets/mint.png'
import fern from './assets/fern.png'
import orchid from './assets/orchid.png'
import shinyOrchid from './assets/orchid-shiny.png'
import '../../ui/garden/gardenScene.css'
import '../../ui/styles/shell-theme.css'
import './preview.css'

// 仅用于素材美术验收。示例数值不读取或写入用户花园；不冒充缺失的成长阶段素材。
const plants = [
  { id: 'mint', name: '命令薄荷', latin: 'Mentha', image: mint, level: 12, meaning: '让重复的工作变得轻巧。', description: '一簇清新的绿意，记录那些被认真整理、反复复用的快捷命令。', x: 19, bottom: 6, size: .76, root: '50% 90%' },
  { id: 'fern', name: '传输蕨', latin: 'Pteridophyta', image: fern, level: 8, meaning: '让信息安全地抵达。', description: '舒展的羽叶与卷曲的新芽，记录每一次可靠完成的文件传输。', x: 51, bottom: 14, size: .79, root: '50% 83%' },
  { id: 'orchid', name: '守护兰', latin: 'Orchidaceae', image: orchid, level: 30, meaning: '克制、边界与安全操作。', description: '淡紫花瓣沿着花茎舒展，为认真守护操作边界的你而绽放。', x: 85, bottom: 3, size: .9, root: '50% 93%' },
] as const
type Plant = typeof plants[number]

function Preview() {
  const [theme, setTheme] = useState<'light' | 'dark'>('light')
  const [height, setHeight] = useState(0)
  const [empty, setEmpty] = useState(false)
  const [shiny, setShiny] = useState(true)
  const [selected, setSelected] = useState<Plant | null>(null)
  const [imageError, setImageError] = useState(false)
  const closeRef = useRef<HTMLButtonElement>(null)
  const triggerRef = useRef<HTMLButtonElement | null>(null)
  useEffect(() => { document.documentElement.dataset.theme = theme }, [theme])
  useEffect(() => { if (selected) closeRef.current?.focus() }, [selected])
  const close = () => { setSelected(null); triggerRef.current?.focus() }
  const specimenImage = (plant: Plant) => plant.id === 'orchid' && shiny ? shinyOrchid : plant.image

  return <main className="art-page">
    <header className="art-heading">
      <div><p className="art-eyebrow">OPSCOPILOT · BOTANICAL GARDEN</p><h1>在忙碌之间，留一片绿意。</h1><p className="art-intro">你的每一次用心，都在这里慢慢生长。</p></div>
      <span className="art-preview-label">美术预览 · 示例收藏</span>
    </header>
    <section className="art-workbench" aria-label="花园美术预览">
      <div className="art-controls">
        <div className="art-control-group" aria-label="昼夜"><button aria-pressed={theme === 'light'} onClick={() => setTheme('light')}>☀ 白天</button><button aria-pressed={theme === 'dark'} onClick={() => setTheme('dark')}>☾ 夜晚</button></div>
        <div className="art-control-group" aria-label="面板高度">{[0, 320, 400, 520].map(h => <button key={h} aria-pressed={height === h} onClick={() => setHeight(h)}>{h === 0 ? '全景' : h === 520 ? '展开欣赏' : `${h}px`}</button>)}</div>
        <label className="art-check"><input type="checkbox" checked={empty} onChange={event => { setEmpty(event.target.checked); setSelected(null) }} />空花园</label>
        <label className="art-check"><input type="checkbox" checked={shiny} onChange={event => setShiny(event.target.checked)} />闪光兰花</label>
      </div>
      <div className="garden-root art-frame" data-night={theme === 'dark'} style={{ '--art-height': height ? `${height}px` : 'clamp(240px, calc((100vw - 74px) / 3), 508px)' } as CSSProperties}>
        <div className="art-scroll" tabIndex={0} aria-label="植物园全景，可左右滚动">
          <div className="art-scene">
            <img className="art-landscape art-day" src={day} alt="" onError={() => setImageError(true)} />
            <img className="art-landscape art-night" src={night} alt="" onError={() => setImageError(true)} />
            {!empty && plants.map(plant => <button key={plant.id} className={`art-specimen art-${plant.id}`} aria-label={`查看${plant.name}${plant.id === 'orchid' && shiny ? '（闪光）' : ''}`} aria-pressed={selected?.id === plant.id}
              style={{ left: `${plant.x}%`, bottom: `${plant.bottom}%`, width: `calc(var(--art-height) * ${plant.size})`, '--art-root': plant.root } as CSSProperties}
              onClick={event => { triggerRef.current = event.currentTarget; setSelected(plant) }}>
              <span className="art-contact" />
              <img className="art-plant-image" src={specimenImage(plant)} alt="" draggable={false} onError={() => setImageError(true)} />
              {plant.id === 'orchid' && shiny && <span className="art-sparkles" aria-hidden="true"><i>✧</i><i>·</i><i>✧</i></span>}
              <span className="art-plant-label">{plant.name}<small>{plant.id === 'orchid' && shiny ? '✧ 闪光收藏' : `Lv. ${plant.level}`}</small></span>
            </button>)}
          </div>
        </div>
        <div className="art-scene-title"><span>我的植物园</span><small>{empty ? '静待第一片新叶' : '三株收藏，一方天地'}</small></div>
        {empty && <div className="art-empty"><span>一座花园，从一次用心开始。</span><small>完成一次连接、文件传输或脚本任务，让这里长出第一株绿意。</small></div>}
        {imageError && <div className="art-image-error" role="alert">素材加载失败，请检查预览资源。</div>}
        {selected && <aside className="art-detail" role="dialog" aria-label={`${selected.name}详情`} onKeyDown={event => { if (event.key === 'Escape') { event.stopPropagation(); close() } }}>
          <button ref={closeRef} className="art-close" aria-label="关闭植物详情" onClick={close}>×</button>
          <span className="art-detail-kicker">BOTANICAL COLLECTION · 0{plants.indexOf(selected) + 1}</span>
          <img className="art-detail-image" src={specimenImage(selected)} alt={selected.name} />
          <div className="art-detail-copy"><span className="art-latin">{selected.latin}</span><h2>{selected.name}</h2><p className="art-meaning">{selected.meaning}</p><p>{selected.description}</p>
          <div className="art-detail-meta"><span>等级 <strong>{selected.level}</strong></span><span>{selected.id === 'orchid' && shiny ? '✧ 闪光品质' : '普通品质'}</span></div></div>
        </aside>}
      </div>
      <footer className="art-caption"><span>点击植物，翻开它的收藏标牌。</span><span>PNG 素材试装 · 昼夜场景</span></footer>
    </section>
    <section className="art-notes"><p><span>01 / 场景</span>原画昼夜底图，保留小径、苔藓与远处的湖光。</p><p><span>02 / 收藏</span>三种独立植物，普通与闪光兰花可以即时对比。</p><p><span>03 / 呈现</span>用真实面板高度验收，窄窗口可横向游览。</p></section>
    <p className="art-footnote">本页为独立视觉预览；植物等级为示例，尚未接入真实成长数据。</p>
  </main>
}

// 素材调参会频繁触发热更新，复用挂载点，避免重复创建 React root。
const root = (import.meta.hot?.data.root as Root | undefined) ?? createRoot(document.getElementById('root')!)
if (import.meta.hot) import.meta.hot.data.root = root
root.render(<Preview />)
