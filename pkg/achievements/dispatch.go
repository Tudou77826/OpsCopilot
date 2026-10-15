package achievements

import (
	"fmt"
	social "opscopilot/pkg/companionsocial"
)

type DispatchResult struct {
	Sent    int    `json:"sent"`
	Failed  int    `json:"failed"`
	Warning string `json:"warning,omitempty"`
}

// Dispatch counts distinct saved commands after the transport accepts at least
// one write. A partial broadcast counts one command and reports target failures.
// Ledger failure must never prompt automatic re-execution of a dispatched command.
func Dispatch(store *Store, commandID string, targets []string, send func(string) error) (DispatchResult, error) {
	result := DispatchResult{}
	if !social.ValidID(commandID) || len(targets) == 0 || len(targets) > 100 {
		return result, fmt.Errorf("快捷指令派发参数无效")
	}
	unique := map[string]bool{}
	for _, target := range targets {
		if !social.ValidID(target) {
			return result, fmt.Errorf("终端标识无效")
		}
		unique[target] = true
	}
	for target := range unique {
		if err := send(target); err != nil {
			result.Failed++
		} else {
			result.Sent++
		}
	}
	if result.Sent == 0 {
		return result, fmt.Errorf("快捷指令未能发送，请检查目标终端")
	}
	if store != nil {
		if err := store.Record(social.MedalQuickCommandUsed, commandID, true); err != nil {
			result.Warning = "指令已发送，奖章记录未保存"
		}
	}
	return result, nil
}
