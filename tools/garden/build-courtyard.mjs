// Build the woodland theme from the checked-in imagegen masters. No runtime image processing.
import { readFileSync, writeFileSync, mkdirSync } from 'node:fs'
import { resolve, join } from 'node:path'
import { decodePng, encodePng, cropImage } from './png.mjs'
import { resample, quantizeTo, applyAlpha } from '../../frontend-shell/garden-recipe.mjs'

globalThis.ImageData ??= class { constructor(width, height) { this.width = width; this.height = height; this.data = new Uint8ClampedArray(width * height * 4) } }
const root = resolve(import.meta.dirname, '../..')
const sources = join(import.meta.dirname, 'courtyard-sources')
const out = join(root, 'frontend-shell/src/ui/garden/packs/woodlandCourtyard')
mkdirSync(join(out, 'assets'), { recursive: true })
const base = JSON.parse(readFileSync(join(root, 'frontend-shell/src/ui/garden/packs/pixelHabitat/manifest.json'), 'utf8'))
const assets = {}
const imports = []
function save(id, image) {
  const bytes = encodePng(image)
  const path = `assets/${id}.png`
  writeFileSync(join(out, path), bytes)
  assets[id] = { path, kind: 'image', bytes: bytes.length, width: image.width, height: image.height }
  imports.push(`import a${imports.length} from './${path}'`)
}
function resize(image, width, height) {
  return resample(image, width, height, { x: 0, y: 0, w: width, h: height, sx: 0, sy: 0, sw: image.width, sh: image.height }, 'box')
}
for (const time of ['day', 'night']) {
  const image = resize(decodePng(readFileSync(join(sources, `${time}.png`))), 768, 256)
  quantizeTo(image, 96, 'none', 0)
  save(time, image)
}
const ids = ['cmd-mint', 'transfer-fern', 'guard-orchid', 'knowledge-tree', 'session-tree', 'script-vine']
for (const quality of ['species', 'rare']) {
  const sheet = decodePng(readFileSync(join(sources, `${quality}.png`)))
  for (let row = 0; row < 6; row++) for (let stage = 0; stage < 6; stage++) {
    const x = Math.round(stage * sheet.width / 6), y = Math.round(row * sheet.height / 6)
    const cell = cropImage(sheet, x, y, Math.round((stage + 1) * sheet.width / 6) - x, Math.round((row + 1) * sheet.height / 6) - y)
    const scaled = resize(cell, 56, 56)
    applyAlpha(scaled, { alphaBinarize: true, alphaCut: 150 })
    quantizeTo(scaled, 40, 'none', 0)
    // Align feet, retaining the artwork's native progression instead of multiplying growth twice.
    let minX = 56, maxX = -1, maxY = -1
    for (let py = 0; py < 56; py++) for (let px = 0; px < 56; px++) if (scaled.data[(py * 56 + px) * 4 + 3]) { minX = Math.min(minX, px); maxX = Math.max(maxX, px); maxY = Math.max(maxY, py) }
    if (maxY < 0) throw new Error(`Empty source cell ${quality}/${row}/${stage}`)
    const image = new ImageData(64, 64)
    const dx = Math.round(32 - (minX + maxX) / 2), dy = 57 - maxY
    for (let py = 0; py < 56; py++) for (let px = 0; px < 56; px++) {
      const tx = px + dx, ty = py + dy
      if (tx >= 0 && tx < 64 && ty >= 0 && ty < 64) image.data.set(scaled.data.subarray((py * 56 + px) * 4, (py * 56 + px) * 4 + 4), (ty * 64 + tx) * 4)
    }
    save(`${ids[row]}-${stage}-${quality}`, image)
  }
}
const positions = [[0.32,0.69,0.36], [0.68,0.70,0.40], [0.79,0.85,0.39], [0.20,0.51,0.56], [0.20,0.84,0.33], [0.78,0.54,0.52]]
const prices = [35,45,70,90,55,80]
const categories = ['香草','蕨类','花卉','乔木','菌类','藤本']
const elements = {}, zones = { ground: { kind:'ground', polygon:[{x:0.04,y:0.42},{x:0.96,y:0.42},{x:0.96,y:0.94},{x:0.04,y:0.94}] } }, slots = []
ids.forEach((id, index) => {
  const [x,y,size] = positions[index]
  slots.push({x,y,size:1,zone:'ground',layer:'garden'})
  const { matureVariants, shinyMatureVariants, stageScales, ...meta } = base.elements[id]
  elements[id] = { ...meta, stages: Array.from({length:6},(_,s)=>`${id}-${s}-species`), shinyStages: Array.from({length:6},(_,s)=>`${id}-${s}-rare`), anchor:{x:0.5,y:0.9}, defaultSize:size,
    commerce:{price:prices[index],category:categories[index],initialLevel:50},
    placement:{type:'ambient',zone:'ground',layer:'garden'}, reducedMotion:{asset:`${id}-5-species`},
    actions:{idle:{format:'procedural',primitive:index===4?'still':'sway',durationMs:9000+index*1300,loop:true}},
    behavior:{durations:{idle:{minMs:12000,maxMs:18000}},weights:{idle:1}} }
})
const manifest = { ...base, id:'ops-woodland-courtyard', name:'林间小庭', version:'1.0.0', stateSchema:{min:3,max:3}, presentation:{...base.presentation,title:'林间小庭'}, assets, elements,
  scene:{dayAsset:'day',nightAsset:'night',aspectRatio:3,fit:'contain', framing:'fit', safeArea:{x:0.03,y:0.04,width:0.94,height:0.92},zones,layers:['garden'],slots,decorations:[],
    backdrop:{
      day:{brightness:0.94,saturation:0.66,contrast:0.86,opacity:0.84},
      night:{brightness:0.78,saturation:0.68,contrast:0.88,opacity:0.88}},
    lighting:{nightBrightness:0.98,nightSaturation:0.94},
    ambience:[{kind:'ripple',x:0.45,y:0.375,size:0.06,durationMs:7200,delayMs:0},{kind:'ripple',x:0.5,y:0.4,size:0.035,durationMs:9000,delayMs:4100},
      ...[[0.14,0.55],[0.35,0.7],[0.62,0.62],[0.83,0.76],[0.56,0.48]].map(([x,y],i)=>({kind:'mote',x,y,size:0.005,durationMs:11000+i*1700,delayMs:i*2700}))] }
}
writeFileSync(join(out,'manifest.json'), JSON.stringify(manifest,null,2)+'\n')
writeFileSync(join(out,'assets.generated.ts'), `${imports.join('\n')}\nexport const courtyardSources: Record<string,string> = {\n${Object.values(assets).map((a,i)=>`  '${a.path}': a${i},`).join('\n')}\n}\n`)
console.log(`woodland courtyard: ${Object.keys(assets).length} assets, ${Object.values(assets).reduce((sum,a)=>sum+a.bytes,0)} bytes`)
