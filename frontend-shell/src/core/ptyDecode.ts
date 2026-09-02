/**
 * PtyDecoder：PTY 字节流 → 文本的流式解码。
 * PTY 是字节流，UTF-8 多字节字符可能被切在任意帧边界；逐帧独立 decode
 * 会产生替换符乱码。这里用 TextDecoder 的 stream 语义把半字符留到下一帧。
 */
export class PtyDecoder {
  private decoder = new TextDecoder('utf-8');

  push(bytes: Uint8Array): string {
    // stream: true 保留不完整尾字节，等后续 push 拼合
    return this.decoder.decode(bytes, { stream: true });
  }

  /** 主动刷出尾部（如通道关闭时）；残缺字节此时才替换为占位符。 */
  end(): string {
    return this.decoder.decode();
  }
}
