package opssidecar

import (
	"bufio"
	"encoding/json"
	"io"
	"sync"
)

// lineEmitter stdout 的唯一写入者：响应行（handle 串行路径）与事件行
// （终端 pump、传输进度等 goroutine）都经它落盘，整行加锁 + 逐行冲刷，
// 保证行边界完整、事件与响应不交错。
type lineEmitter struct {
	mu  sync.Mutex
	bw  *bufio.Writer
	enc *json.Encoder
}

func newLineEmitter(w io.Writer) *lineEmitter {
	bw := bufio.NewWriter(w)
	enc := json.NewEncoder(bw)
	return &lineEmitter{bw: bw, enc: enc}
}

// line 编码并冲刷一行。写失败仅记录（stdout 已断时进程即将退出，
// 没有可恢复的下游），不向调用方扩散错误。
func (e *lineEmitter) line(v any) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.enc.Encode(v); err != nil {
		return
	}
	if err := e.bw.Flush(); err != nil {
		return
	}
}
