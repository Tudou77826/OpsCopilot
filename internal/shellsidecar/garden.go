package shellsidecar

import "opscopilot/pkg/garden"

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

// Signal 返回入口轻提示所需的轻量 revision。
func (s *GardenService) Signal() *garden.ChangeSignal { return s.inner.Signal() }

func (s *GardenService) Purchase(itemID garden.ItemID, price, initialLevel int) (*garden.PurchaseResult, error) {
	return s.inner.Purchase(itemID, price, initialLevel)
}

func (s *GardenService) Place(instanceID string, x, y, scale float64, flipX bool) (*garden.Snapshot, error) {
	return s.inner.Place(instanceID, x, y, scale, flipX)
}

func (s *GardenService) Stow(instanceID string) (*garden.Snapshot, error) {
	return s.inner.Stow(instanceID)
}

// RecordWithResult 供其他服务在业务结果处调用；错误只记日志不上抛——
// 花园是附属能力，绝不影响业务操作本身。

// Record 把一次合格业务事件交给花园结算。eventKey 必须来自业务结果本身
// （传输任务 id、回放批次 id 等），同一 key 只结算一次。
func (s *GardenService) Record(kind garden.EventKind, eventKey string) error {
	_, err := s.inner.Record(kind, eventKey)
	return err
}
