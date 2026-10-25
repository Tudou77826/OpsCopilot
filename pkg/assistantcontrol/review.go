package assistantcontrol

import "context"

// ReviewInput is exact host-observed context, never an agent's permission claim.
type ReviewInput struct {
	UserRequest string `json:"userRequest"`
	Location    string `json:"location"`
	Tool        string `json:"tool"`
	Action      string `json:"action"`
}
type ReviewVerdict struct {
	ReadOnly  bool   `json:"readOnly"`
	InScope   bool   `json:"inScope"`
	Sensitive bool   `json:"sensitive"`
	Reason    string `json:"reason"`
	Notice    string `json:"notice,omitempty"`
}

func (v ReviewVerdict) Allowed() bool {
	return v.ReadOnly && v.InScope && !v.Sensitive && v.Reason != ""
}

type Reviewer func(context.Context, ReviewInput) ReviewVerdict
