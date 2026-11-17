package opssidecar

import "sync"

// ringBuffer 定长输出缓冲：保存最近 cap 字节的终端输出，供 Desktop 断线重连
// 时整段重放、恢复现场。写入远大于 cap 时从头部丢弃旧数据。
// 追加走「超限后整体压实」的朴素实现：输出洪泛下每 cap 字节摊销一次拷贝，
// 相比分片环形索引，换来 snapshot 一次 memcpy 的简单语义。
type ringBuffer struct {
	mu  sync.Mutex
	buf []byte
	cap int
}

func newRingBuffer(capacity int) *ringBuffer {
	return &ringBuffer{cap: capacity}
}

// append 追加一段输出，超过容量时丢弃最旧的头部。
func (r *ringBuffer) append(data []byte) {
	if len(data) == 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buf = append(r.buf, data...)
	if len(r.buf) > r.cap {
		r.buf = append(r.buf[:0:0], r.buf[len(r.buf)-r.cap:]...)
	}
}

// snapshot 按时间顺序返回当前缓冲的全部内容（拷贝）。
func (r *ringBuffer) snapshot() []byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]byte, len(r.buf))
	copy(out, r.buf)
	return out
}

// reset 清空缓冲。
func (r *ringBuffer) reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buf = r.buf[:0]
}
