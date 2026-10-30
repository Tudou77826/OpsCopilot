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

func TestAssistantReviewContextUsesOnlyPriorUserRequests(t *testing.T) {
	var history assistantcontrol.History
	history.Begin("first", "排查支付服务")
	history.Begin("second", "再看数据库")
	history.Begin("current", "继续")
	got := assistantReviewContext(history.Snapshot(), "current")
	if len(got) != 2 || got[0] != "排查支付服务" || got[1] != "再看数据库" {
		t.Fatalf("context = %#v", got)
	}
}
