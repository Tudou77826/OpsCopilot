package shellsidecar

import (
	"fmt"
	"opscopilot/pkg/garden"
	"time"
)

// GardenService 是养成系统在 sidecar 里的薄外壳：状态与规则都在 pkg/garden，
// 这里只做数据目录挂接、事件入口和 RPC 形状。未初始化（无 --data-dir）时方法报错，
// 与 QuickCmds/Settings 的能力边界纪律一致。
type GardenService struct {
	inner *garden.Store
}

// NewGardenService 打开数据目录下的 garden.json。
func NewGardenService(dataDir string) (*GardenService, error) {
	inner, err := garden.Open(dataDir + "/garden.json")
	if err != nil {
		return nil, err
	}
	return &GardenService{inner: inner}, nil
}

// Snapshot 返回只读快照（无呈现语义）。
func (s *GardenService) Snapshot() *garden.Snapshot { return s.inner.Snapshot() }

// Dismiss 把一条反馈标记为已读。at 与快照里 pending 的时间戳同格式（RFC3339）。
func (s *GardenService) Dismiss(at string) error {
	parsed, err := time.Parse(time.RFC3339Nano, at)
	if err != nil {
		return fmt.Errorf("时间戳格式无效: %w", err)
	}
	s.inner.DismissAt(parsed.UnixNano())
	return nil
}

// RecordWithResult 供其他服务在业务结果处调用；错误只记日志不上抛——
// 花园是附属能力，绝不影响业务操作本身。

// Record 把一次合格业务事件交给花园结算。eventKey 必须来自业务结果本身
// （传输任务 id、回放批次 id 等），同一 key 只结算一次。
func (s *GardenService) Record(kind garden.EventKind, eventKey string) error {
	_, err := s.inner.Record(kind, eventKey)
	return err
}
