/**
 * 养成素材归一化配方（唯一实现）。
 *
 * 调优台（garden-art-tuner.html，浏览器内交互调参）与批量脚本
 * （tools/garden/normalize-art.mjs，出货时跑整套素材）都从这里取配方，
 * 避免两处实现漂移——之前用 Python 复刻检查时正是踩了这个坑。
 *
 * 配方顺序：源图预缩 → 去背景 → 源图预量化 → 采样 → Alpha 处理 → 调色板量化 → 描边。
 * 参数含义见 docs/garden-presentation-architecture.md 第 4.2 节。
 */

/* 目标画布上的摆放：stretch 铺满；contain 保持比例留透明边；默认 cover 居中裁切 */
function fitPlacement(sw, sh, outW, outH, contain) {
  if (contain) {
    const scale = Math.min(outW / sw, outH / sh)
    const w = Math.max(1, Math.round(sw * scale))
    const h = Math.max(1, Math.round(sh * scale))
    return { x: Math.floor((outW - w) / 2), y: Math.floor((outH - h) / 2), w, h, sx: 0, sy: 0, sw, sh }
  }
  const targetRatio = outW / outH
  const srcRatio = sw / sh
  let cw = sw
  let ch = sh
  let cx = 0
  let cy = 0
  if (srcRatio > targetRatio) {
    cw = Math.round(sh * targetRatio)
    cx = Math.floor((sw - cw) / 2)
  } else {
    ch = Math.round(sw / targetRatio)
    cy = Math.floor((sh - ch) / 2)
  }
  return { x: 0, y: 0, w: outW, h: outH, sx: cx, sy: cy, sw: cw, sh: ch }
}

function removeBackground(data, p) {
  const px = data.data
  let key = null
  if (p.bgMode === 'corners') {
    const corners = [
      [0, 0], [data.width - 1, 0], [0, data.height - 1], [data.width - 1, data.height - 1],
    ].map(([x, y]) => {
      const i = (y * data.width + x) * 4
      return [px[i], px[i + 1], px[i + 2]]
    })
    key = [0, 1, 2].map(ch => Math.round(corners.reduce((sum, c) => sum + c[ch], 0) / corners.length))
  } else if (p.bgMode === 'white') {
    key = [255, 255, 255]
  }
  const tol2 = p.bgTol * p.bgTol
  for (let i = 0; i < px.length; i += 4) {
    if (px[i + 3] === 0) continue
    if (key) {
      const dr = px[i] - key[0]
      const dg = px[i + 1] - key[1]
      const db = px[i + 2] - key[2]
      if (dr * dr + dg * dg + db * db <= tol2) px[i + 3] = 0
    } else {
      const luma = 0.299 * px[i] + 0.587 * px[i + 1] + 0.114 * px[i + 2]
      if (luma >= p.bgLuma) px[i + 3] = 0
    }
  }
}

/* 把源矩形采样到目标矩形。采样按"源像素落进哪个格"累加，等价于面积平均。 */
function resample(srcData, outW, outH, place, method) {
  const out = new ImageData(outW, outH)
  const src = srcData.data
  const sw = srcData.width
  const dst = out.data
  const { x: dx0, y: dy0, w: dw, h: dh, sx, sy, sw: cw, sh: ch } = place

  if (method === 'box') {
    const sum = new Float64Array(dw * dh * 4)
    const hits = new Uint32Array(dw * dh)
    for (let y = sy; y < sy + ch; y++) {
      const ty = Math.min(dh - 1, Math.floor(((y - sy) / ch) * dh))
      for (let x = sx; x < sx + cw; x++) {
        const tx = Math.min(dw - 1, Math.floor(((x - sx) / cw) * dw))
        const cell = ty * dw + tx
        const i = (y * sw + x) * 4
        const a = src[i + 3]
        sum[cell * 4] += src[i] * a
        sum[cell * 4 + 1] += src[i + 1] * a
        sum[cell * 4 + 2] += src[i + 2] * a
        sum[cell * 4 + 3] += a
        hits[cell]++
      }
    }
    for (let cell = 0; cell < dw * dh; cell++) {
      const tx = cell % dw
      const ty = Math.floor(cell / dw)
      const a = sum[cell * 4 + 3]
      const o = ((dy0 + ty) * outW + (dx0 + tx)) * 4
      if (!hits[cell] || a <= 0) {
        const near = nearestSource(srcData, place, tx, ty)
        dst[o] = near[0]; dst[o + 1] = near[1]; dst[o + 2] = near[2]; dst[o + 3] = near[3]
        continue
      }
      dst[o] = Math.round(sum[cell * 4] / a)
      dst[o + 1] = Math.round(sum[cell * 4 + 1] / a)
      dst[o + 2] = Math.round(sum[cell * 4 + 2] / a)
      dst[o + 3] = Math.round(a / hits[cell])
    }
    return out
  }

  for (let ty = 0; ty < dh; ty++) {
    for (let tx = 0; tx < dw; tx++) {
      const x0 = sx + Math.floor((tx / dw) * cw)
      const x1 = Math.max(x0 + 1, sx + Math.floor(((tx + 1) / dw) * cw))
      const y0 = sy + Math.floor((ty / dh) * ch)
      const y1 = Math.max(y0 + 1, sy + Math.floor(((ty + 1) / dh) * ch))
      const o = ((dy0 + ty) * outW + (dx0 + tx)) * 4
      if (method === 'nearest') {
        const near = nearestSource(srcData, place, tx, ty)
        dst[o] = near[0]; dst[o + 1] = near[1]; dst[o + 2] = near[2]; dst[o + 3] = near[3]
        continue
      }
      if (method === 'mode') {
        const counts = new Map()
        let best = null
        let bestN = 0
        for (let y = y0; y < Math.min(y1, sy + ch); y++) {
          for (let x = x0; x < Math.min(x1, sx + cw); x++) {
            const i = (y * sw + x) * 4
            if (src[i + 3] < 8) continue
            const key = ((src[i] >> 3) << 10) | ((src[i + 1] >> 3) << 5) | (src[i + 2] >> 3)
            const entry = counts.get(key) ?? { n: 0, r: 0, g: 0, b: 0 }
            entry.n++; entry.r += src[i]; entry.g += src[i + 1]; entry.b += src[i + 2]
            counts.set(key, entry)
            if (entry.n > bestN) { bestN = entry.n; best = entry }
          }
        }
        if (best) {
          dst[o] = Math.round(best.r / best.n)
          dst[o + 1] = Math.round(best.g / best.n)
          dst[o + 2] = Math.round(best.b / best.n)
          dst[o + 3] = 255
        }
        continue
      }
      // median
      const rs = [], gs = [], bs = []
      for (let y = y0; y < Math.min(y1, sy + ch); y++) {
        for (let x = x0; x < Math.min(x1, sx + cw); x++) {
          const i = (y * sw + x) * 4
          if (src[i + 3] < 8) continue
          rs.push(src[i]); gs.push(src[i + 1]); bs.push(src[i + 2])
        }
      }
      if (!rs.length) {
        const near = nearestSource(srcData, place, tx, ty)
        dst[o] = near[0]; dst[o + 1] = near[1]; dst[o + 2] = near[2]; dst[o + 3] = near[3]
        continue
      }
      const mid = arr => arr.sort((a, b) => a - b)[Math.floor(arr.length / 2)]
      dst[o] = mid(rs); dst[o + 1] = mid(gs); dst[o + 2] = mid(bs); dst[o + 3] = 255
    }
  }
  return out
}

function nearestSource(srcData, place, tx, ty) {
  const { sx, sy, w: dw, h: dh, sw: cw, sh: ch } = place
  const x = Math.min(srcData.width - 1, sx + Math.floor(((tx + 0.5) / dw) * cw))
  const y = Math.min(srcData.height - 1, sy + Math.floor(((ty + 0.5) / dh) * ch))
  const i = (y * srcData.width + x) * 4
  const d = srcData.data
  return [d[i], d[i + 1], d[i + 2], d[i + 3]]
}

function applyAlpha(image, p) {
  const px = image.data
  for (let i = 3; i < px.length; i += 4) {
    if (p.alphaBinarize) px[i] = px[i] >= p.alphaCut ? 255 : 0
    else if (px[i] < p.alphaCut) px[i] = 0
  }
}

/* ---------- 调色板 ---------- */

/* 就地量化到指定色数。源图预量化与结果量化共用同一实现，避免两处算法漂移。 */
function quantizeTo(image, colors, ditherKind, amount) {
  const px = image.data
  const hist = new Map()
  for (let i = 0; i < px.length; i += 4) {
    if (px[i + 3] < 128) continue
    const key = (px[i] << 16) | (px[i + 1] << 8) | px[i + 2]
    hist.set(key, (hist.get(key) ?? 0) + 1)
  }
  if (hist.size === 0) return 0
  const entries = [...hist].map(([key, count]) => ({ r: (key >> 16) & 255, g: (key >> 8) & 255, b: key & 255, count }))
  const palette = medianCut(entries, colors)
  const ordered = ditherMatrix(ditherKind)
  const diffuse = ditherKind === 'fs' || ditherKind === 'atkinson'
  if (diffuse) diffuseQuantize(image, palette, ditherKind, amount)
  else {
    for (let y = 0; y < image.height; y++) {
      for (let x = 0; x < image.width; x++) {
        const i = (y * image.width + x) * 4
        if (px[i + 3] < 128) continue
        let bias = 0
        if (ordered) {
          const n = ordered.length
          bias = ((ordered[y % n][x % n] / (n * n)) - 0.5) * 255 * amount * 0.5
        }
        const [r, g, b] = nearestColor(palette, px[i] + bias, px[i + 1] + bias, px[i + 2] + bias)
        px[i] = r; px[i + 1] = g; px[i + 2] = b
      }
    }
  }
  return palette.length
}

function quantizeImage(image, p) {
  const px = image.data
  const hist = new Map()
  for (let i = 0; i < px.length; i += 4) {
    if (px[i + 3] < 128) continue
    const key = (px[i] << 16) | (px[i + 1] << 8) | px[i + 2]
    hist.set(key, (hist.get(key) ?? 0) + 1)
  }
  if (hist.size === 0) return 0
  const entries = [...hist].map(([key, count]) => ({
    r: (key >> 16) & 255, g: (key >> 8) & 255, b: key & 255, count,
  }))
  const palette = medianCut(entries, p.colors)
  const ordered = ditherMatrix(p.dither)
  const amount = p.ditherAmt / 100
  const diffuse = p.dither === 'fs' || p.dither === 'atkinson'
  if (diffuse) diffuseQuantize(image, palette, p.dither, amount)
  else {
    for (let y = 0; y < image.height; y++) {
      for (let x = 0; x < image.width; x++) {
        const i = (y * image.width + x) * 4
        if (px[i + 3] < 128) continue
        let bias = 0
        if (ordered) {
          const n = ordered.length
          bias = ((ordered[y % n][x % n] / (n * n)) - 0.5) * 255 * amount * 0.5
        }
        const [r, g, b] = nearestColor(palette, px[i] + bias, px[i + 1] + bias, px[i + 2] + bias)
        px[i] = r; px[i + 1] = g; px[i + 2] = b
      }
    }
  }
  return palette.length
}

function medianCut(entries, maxColors) {
  const chan = (e, ch) => (ch === 0 ? e.r : ch === 1 ? e.g : e.b)
  let boxes = [entries]
  while (boxes.length < maxColors) {
    let pick = -1
    let bestScore = 0
    for (let i = 0; i < boxes.length; i++) {
      const box = boxes[i]
      if (box.length < 2) continue
      const ch = widestChannel(box)
      // 用循环求极值，不能写 Math.min(...box.map(...))：
      // 场景这类大图有几十万种颜色，展开成函数实参会直接爆栈。
      let lo = 255
      let hi = 0
      for (const e of box) {
        const v = chan(e, ch)
        if (v < lo) lo = v
        if (v > hi) hi = v
      }
      const score = (hi - lo) * Math.log2(box.length + 1)
      if (score > bestScore) { bestScore = score; pick = i }
    }
    if (pick < 0 || bestScore <= 0) break
    const box = boxes[pick]
    const ch = widestChannel(box)
    box.sort((a, b) => chan(a, ch) - chan(b, ch))
    const total = box.reduce((sum, e) => sum + e.count, 0)
    let acc = 0
    let cut = 1
    for (let i = 0; i < box.length; i++) {
      acc += box[i].count
      if (acc >= total / 2) { cut = i + 1; break }
    }
    cut = Math.min(box.length - 1, Math.max(1, cut))
    boxes.splice(pick, 1, box.slice(0, cut), box.slice(cut))
  }
  return boxes.map(box => {
    let r = 0, g = 0, b = 0, n = 0
    for (const e of box) { r += e.r * e.count; g += e.g * e.count; b += e.b * e.count; n += e.count }
    return [Math.round(r / n), Math.round(g / n), Math.round(b / n)]
  })
}

function widestChannel(box) {
  let best = 0
  let bestRange = -1
  for (let ch = 0; ch < 3; ch++) {
    let lo = 255
    let hi = 0
    for (const e of box) {
      const v = ch === 0 ? e.r : ch === 1 ? e.g : e.b
      if (v < lo) lo = v
      if (v > hi) hi = v
    }
    const range = hi - lo
    if (range > bestRange) { bestRange = range; best = ch }
  }
  return best
}

function nearestColor(palette, r, g, b) {
  let best = palette[0]
  let bestD = Infinity
  for (const c of palette) {
    const dr = r - c[0]
    const dg = g - c[1]
    const db = b - c[2]
    const d = dr * dr + dg * dg + db * db
    if (d < bestD) { bestD = d; best = c }
  }
  return best
}

function ditherMatrix(kind) {
  if (kind === 'b2') return bayer(1)
  if (kind === 'b4') return bayer(2)
  if (kind === 'b8') return bayer(3)
  return null
}

function bayer(level) {
  let size = 1
  let matrix = [[0]]
  for (let i = 0; i < level; i++) {
    size *= 2
    const next = Array.from({ length: size }, () => new Array(size).fill(0))
    for (let y = 0; y < size / 2; y++) {
      for (let x = 0; x < size / 2; x++) {
        const v = matrix[y][x] * 4
        next[y][x] = v
        next[y][x + size / 2] = v + 2
        next[y + size / 2][x] = v + 3
        next[y + size / 2][x + size / 2] = v + 1
      }
    }
    matrix = next
  }
  return matrix
}

function diffuseQuantize(image, palette, kind, amount) {
  const w = image.width
  const h = image.height
  const px = image.data
  const buffer = new Float32Array(w * h * 3)
  for (let i = 0, j = 0; i < px.length; i += 4, j += 3) {
    buffer[j] = px[i]; buffer[j + 1] = px[i + 1]; buffer[j + 2] = px[i + 2]
  }
  const kernel = kind === 'fs'
    ? [[1, 0, 7 / 16], [-1, 1, 3 / 16], [0, 1, 5 / 16], [1, 1, 1 / 16]]
    : [[1, 0, 1 / 8], [2, 0, 1 / 8], [-1, 1, 1 / 8], [0, 1, 1 / 8], [1, 1, 1 / 8], [0, 2, 1 / 8]]
  for (let y = 0; y < h; y++) {
    for (let x = 0; x < w; x++) {
      const o = (y * w + x) * 4
      if (px[o + 3] < 128) continue
      const j = (y * w + x) * 3
      const r = Math.max(0, Math.min(255, buffer[j]))
      const g = Math.max(0, Math.min(255, buffer[j + 1]))
      const b = Math.max(0, Math.min(255, buffer[j + 2]))
      const [nr, ng, nb] = nearestColor(palette, r, g, b)
      px[o] = nr; px[o + 1] = ng; px[o + 2] = nb
      const er = (r - nr) * amount
      const eg = (g - ng) * amount
      const eb = (b - nb) * amount
      for (const [dx, dy, weight] of kernel) {
        const nx = x + dx
        const ny = y + dy
        if (nx < 0 || nx >= w || ny < 0 || ny >= h) continue
        const nj = (ny * w + nx) * 3
        if (px[(ny * w + nx) * 4 + 3] < 128) continue
        buffer[nj] += er * weight
        buffer[nj + 1] += eg * weight
        buffer[nj + 2] += eb * weight
      }
    }
  }
}

/* ---------- 描边 ---------- */

function addOutline(image, p) {
  const w = image.width
  const h = image.height
  const src = image.data
  const out = new ImageData(w, h)
  out.data.set(src)
  const dst = out.data
  const fixed = hexToRgb(p.outlineColor)
  const dark = p.outlineDark / 100
  const solid = new Uint8Array(w * h)
  for (let i = 0; i < w * h; i++) solid[i] = src[i * 4 + 3] >= 128 ? 1 : 0
  let frontier = solid.slice()
  for (let pass = 0; pass < p.outlineW; pass++) {
    const next = frontier.slice()
    for (let y = 0; y < h; y++) {
      for (let x = 0; x < w; x++) {
        const cell = y * w + x
        if (frontier[cell]) continue
        const neighbours = [
          [x - 1, y], [x + 1, y], [x, y - 1], [x, y + 1],
        ]
        let touching = -1
        for (const [nx, ny] of neighbours) {
          if (nx < 0 || nx >= w || ny < 0 || ny >= h) continue
          const n = ny * w + nx
          if (solid[n]) { touching = n; break }
        }
        if (touching < 0) continue
        const o = cell * 4
        let r
        let g
        let b
        if (p.outline === 'fixed') {
          r = fixed[0]; g = fixed[1]; b = fixed[2]
        } else {
          r = Math.round(src[touching * 4] * dark)
          g = Math.round(src[touching * 4 + 1] * dark)
          b = Math.round(src[touching * 4 + 2] * dark)
        }
        dst[o] = r; dst[o + 1] = g; dst[o + 2] = b; dst[o + 3] = 255
        next[cell] = 1
      }
    }
    frontier = next
  }
  return out
}

function hexToRgb(hex) {
  const v = Number.parseInt(hex.slice(1), 16)
  return [(v >> 16) & 255, (v >> 8) & 255, v & 255]
}

export {
  fitPlacement,
  removeBackground,
  resample,
  nearestSource,
  applyAlpha,
  quantizeTo,
  quantizeImage,
  medianCut,
  widestChannel,
  nearestColor,
  ditherMatrix,
  bayer,
  diffuseQuantize,
  addOutline,
  hexToRgb,
}
