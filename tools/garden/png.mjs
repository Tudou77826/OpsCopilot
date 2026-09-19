/**
 * 最小 PNG 编解码（只覆盖养成素材用得到的格式）。
 *
 * 解码支持：8 位 RGB / RGBA、非隔行。
 * 编码输出：8 位 RGBA、非隔行、逐行 filter 0。
 *
 * 不引入图像库：这批素材的格式固定，一个几百行的实现比多一个原生依赖更好维护，
 * 也避免 CI 与本地环境出现二进制差异。
 */
import { deflateSync, inflateSync } from 'node:zlib'

const PNG_SIGNATURE = 0x89504e47

/** 解码 PNG，返回与浏览器 ImageData 同形状的对象（width / height / data）。 */
export function decodePng(buffer) {
  if (buffer.readUInt32BE(0) !== PNG_SIGNATURE) throw new Error('不是 PNG 文件')
  let offset = 8
  let width = 0
  let height = 0
  let bitDepth = 0
  let colorType = 0
  let interlace = 0
  const idat = []
  while (offset + 8 <= buffer.length) {
    const length = buffer.readUInt32BE(offset)
    const type = buffer.subarray(offset + 4, offset + 8).toString('ascii')
    const data = buffer.subarray(offset + 8, offset + 8 + length)
    if (type === 'IHDR') {
      width = data.readUInt32BE(0)
      height = data.readUInt32BE(4)
      bitDepth = data[8]
      colorType = data[9]
      interlace = data[12]
    } else if (type === 'IDAT') {
      idat.push(data)
    } else if (type === 'IEND') {
      break
    }
    offset += 12 + length
  }
  if (bitDepth !== 8 || ![2, 6].includes(colorType) || interlace !== 0) {
    throw new Error(`仅支持 8 位 RGB/RGBA 非隔行 PNG（当前 位深${bitDepth} 颜色类型${colorType} 隔行${interlace}）`)
  }
  const raw = inflateSync(Buffer.concat(idat))
  const channels = colorType === 6 ? 4 : 3
  const stride = width * channels
  const data = new Uint8ClampedArray(width * height * 4)
  let previous = new Uint8ClampedArray(stride)
  for (let y = 0; y < height; y++) {
    const start = y * (stride + 1)
    const filter = raw[start]
    const line = raw.subarray(start + 1, start + 1 + stride)
    const current = new Uint8ClampedArray(stride)
    for (let i = 0; i < stride; i++) {
      const value = line[i]
      const left = i >= channels ? current[i - channels] : 0
      const up = previous[i]
      const upLeft = i >= channels ? previous[i - channels] : 0
      let result
      if (filter === 0) result = value
      else if (filter === 1) result = value + left
      else if (filter === 2) result = value + up
      else if (filter === 3) result = value + ((left + up) >> 1)
      else if (filter === 4) {
        const p = left + up - upLeft
        const pa = Math.abs(p - left)
        const pb = Math.abs(p - up)
        const pc = Math.abs(p - upLeft)
        result = value + (pa <= pb && pa <= pc ? left : pb <= pc ? up : upLeft)
      } else throw new Error(`未知的 PNG 行过滤类型 ${filter}`)
      current[i] = result & 0xff
    }
    for (let x = 0; x < width; x++) {
      const target = (y * width + x) * 4
      data[target] = current[x * channels]
      data[target + 1] = current[x * channels + 1]
      data[target + 2] = current[x * channels + 2]
      data[target + 3] = channels === 4 ? current[x * channels + 3] : 255
    }
    previous = current
  }
  return { width, height, data }
}

const CRC_TABLE = (() => {
  const table = new Int32Array(256)
  for (let n = 0; n < 256; n++) {
    let c = n
    for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1
    table[n] = c
  }
  return table
})()

function crc32(buffer) {
  let crc = -1
  for (let i = 0; i < buffer.length; i++) crc = CRC_TABLE[(crc ^ buffer[i]) & 0xff] ^ (crc >>> 8)
  return (crc ^ -1) >>> 0
}

function chunk(type, data) {
  const length = Buffer.alloc(4)
  length.writeUInt32BE(data.length)
  const typeBuffer = Buffer.from(type, 'ascii')
  const checksum = Buffer.alloc(4)
  checksum.writeUInt32BE(crc32(Buffer.concat([typeBuffer, data])))
  return Buffer.concat([length, typeBuffer, data, checksum])
}

/** 编码为 8 位 RGBA PNG。 */
export function encodePng(image) {
  const { width, height, data } = image
  const stride = width * 4
  const raw = Buffer.alloc((stride + 1) * height)
  for (let y = 0; y < height; y++) {
    const target = y * (stride + 1)
    raw[target] = 0
    Buffer.from(data.buffer, data.byteOffset + y * stride, stride).copy(raw, target + 1)
  }
  const header = Buffer.alloc(13)
  header.writeUInt32BE(width, 0)
  header.writeUInt32BE(height, 4)
  header[8] = 8
  header[9] = 6
  return Buffer.concat([
    Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]),
    chunk('IHDR', header),
    chunk('IDAT', deflateSync(raw, { level: 9 })),
    chunk('IEND', Buffer.alloc(0)),
  ])
}

/** 从大图里裁一块正方形/矩形，返回与 ImageData 同形状的对象。 */
export function cropImage(image, x, y, width, height) {
  const data = new Uint8ClampedArray(width * height * 4)
  for (let row = 0; row < height; row++) {
    const from = ((y + row) * image.width + x) * 4
    data.set(image.data.subarray(from, from + width * 4), row * width * 4)
  }
  return { width, height, data }
}
