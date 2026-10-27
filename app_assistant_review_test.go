package main

import (
	"context"
	"testing"

	"opscopilot/pkg/assistantcontrol"
)

func TestAssistantRiskFloorDoesNotApproveStoppedTurn(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if assistantRiskFloor(ctx, assistantcontrol.ReviewInput{}).Allowed() {
		t.Fatal("stopped turn was approved")
	}
	if !assistantRiskFloor(context.Background(), assistantcontrol.ReviewInput{}).Allowed() {
		t.Fatal("ordinary Ops tool should proceed after iCode review")
	}
}
