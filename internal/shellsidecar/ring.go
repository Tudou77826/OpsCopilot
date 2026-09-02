package shellsidecar

// ringBuffer 是重放环形缓冲：写满覆盖最旧，snapshot 返回按序快照。
// 只在 terminal.mu 内使用，自身不加锁。
type ringBuffer struct {
	buf  []byte
	head int // 下一个写入位置
	size int // 当前有效字节数
}

func newRingBuffer(capacity int) *ringBuffer {
	return &ringBuffer{buf: make([]byte, capacity)}
}

func (r *ringBuffer) write(data []byte) {
	if len(data) >= len(r.buf) {
		// 单次写入超过容量：只留尾部。
		copy(r.buf, data[len(data)-len(r.buf):])
		r.head, r.size = 0, len(r.buf)
		return
	}
	tail := (r.head + r.size) % len(r.buf)
	first := min(len(r.buf)-tail, len(data))
	copy(r.buf[tail:], data[:first])
	copy(r.buf, data[first:])
	r.size += len(data)
	if r.size > len(r.buf) {
		r.head = (r.head + r.size - len(r.buf)) % len(r.buf)
		r.size = len(r.buf)
	}
}

func (r *ringBuffer) snapshot() []byte {
	out := make([]byte, r.size)
	first := min(len(r.buf)-r.head, r.size)
	copy(out, r.buf[r.head:r.head+first])
	copy(out[first:], r.buf[:r.size-first])
	return out
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
