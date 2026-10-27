package assistantcontrol

import "context"

// ReviewInput is exact host-observed context, never an agent's permission claim.
type ReviewInput struct {
	UserRequest     string              `json:"userRequest"`
	Location        string              `json:"location"`
	Tool            string              `json:"tool"`
	Action          string              `json:"action"`
	TaskContext     []string            `json:"taskContext,omitempty"`
	ResourceContext string              `json:"resourceContext,omitempty"`
	Observations    []ReviewObservation `json:"observations,omitempty"`
}

// ReviewObservation is bounded host-observed evidence about effects, never authority.
type ReviewObservation struct {
	Action   string `json:"action"`
	Result   string `json:"result"`
	Location string `json:"location"`
}
type ReviewVerdict struct {
	ReadOnly  bool   `json:"readOnly"`
	LowRisk   bool   `json:"lowRisk,omitempty"`
	InScope   bool   `json:"inScope"`
	Sensitive bool   `json:"sensitive"`
	Reason    string `json:"reason"`
	Notice    string `json:"notice,omitempty"`
}

func (v ReviewVerdict) Allowed() bool {
	return (v.LowRisk || v.ReadOnly) && v.InScope && !v.Sensitive && v.Reason != ""
}

type Reviewer func(context.Context, ReviewInput) ReviewVerdict
