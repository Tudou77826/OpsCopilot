package terminalcontrol

import (
	"context"
	"testing"
)

func TestHumanRiskFloorUsesOperationRatherThanSearchWords(t *testing.T) {
	for _, command := range []string{"grep -n restart /var/log/app.log", "grep -n rm /var/log/app.log", "ls /tmp/restart", "grep 'kill' /var/log/app.log", "sed -n '1,20p' /var/log/app.log", `{"path":"D:/reports/report.txt","content":"restart is mentioned"}`} {
		if RequiresHumanReview(command) {
			t.Errorf("harmless text treated as effect: %s", command)
		}
	}
	for _, command := range []string{"rm /tmp/report", "echo ready; systemctl restart api", "sudo cat /etc/app.conf", "cat /root/.ssh/id_rsa", "printf changed > /etc/app.conf", "sed -i 's/a/b/' /etc/app.conf", "cp /tmp/a /etc/app.conf", `{"command":"Remove-Item D:/data -Recurse"}`, `{"path":"C:/Users/me/.ssh/id_rsa"}`} {
		if !RequiresHumanReview(command) {
			t.Errorf("explicit risk escaped floor: %s", command)
		}
	}
}

func TestNullOutputRedirectIsNotAFileWrite(t *testing.T) {
	for _, command := range []string{
		"ls -lh /tmp/opscopilot-linux-e2e-20261026 /tmp/hsperfdata_root; du -sh /tmp 2>/dev/null",
		"du -sh /tmp 2> /dev/null", "ls /tmp >/dev/null 2>&1", "ls /tmp >>/dev/null",
	} {
		if RequiresHumanReview(command) {
			t.Errorf("discarding output triggered approval: %q", command)
		}
	}
	for _, command := range []string{
		"printf data > /tmp/result", "printf data >> /tmp/result", "ls /tmp 2>/dev/null.other",
		`ls /tmp 2>/dev/null"suffix"`, `ls /tmp 2>/dev/null\;suffix`, "ls /tmp 2>/dev/null$(printf x)",
		"ls /tmp 2>/dev/null; rm /tmp/result", "ls /tmp 2>&1; printf x > /tmp/result",
		`rm'' /tmp/result 2>/dev/null`,
	} {
		if !RequiresHumanReview(command) {
			t.Errorf("unsafe redirect or deletion escaped approval: %q", command)
		}
	}
}

func TestReadOnlyCompositeCommandCanPassHostReview(t *testing.T) {
	a, _, _, writer, target := protectedFixture(t, &memoryJournal{})
	a.scope.RequireActionReview = true
	run := retainedTake(t, a, target)
	command := "ls -lh /tmp/opscopilot-linux-e2e-20261026 /tmp/hsperfdata_root; du -sh /tmp 2>/dev/null"
	id := requireReview(t, run, context.Background(), command)
	if _, err := a.ApproveLowRiskReview(id, "iCode allowed an ordinary directory inspection"); err != nil {
		t.Fatalf("safe command rejected: %v", err)
	}
	select {
	case <-writer.writes:
		t.Fatal("approval alone must not execute the command")
	default:
	}
}

func TestReviewedScopeStillRequiredForLiteralCommand(t *testing.T) {
	a, _, _, w, target := protectedFixture(t, &memoryJournal{})
	a.scope.RequireActionReview = true
	run := retainedTake(t, a, target)
	_ = requireReview(t, run, context.Background(), "pwd")
	select {
	case <-w.writes:
		t.Fatal("literal command bypassed task review")
	default:
	}
}

func TestPolicyCannotApproveRecognizedHighRiskCommand(t *testing.T) {
	for _, command := range []string{"rm /tmp/report", "systemctl restart orders", "printf changed > /etc/app.conf", "cat /root/.ssh/id_rsa"} {
		t.Run(command, func(t *testing.T) {
			a, _, _, writer, target := protectedFixture(t, &memoryJournal{})
			a.scope.RequireActionReview = true
			run := retainedTake(t, a, target)
			id := requireReview(t, run, context.Background(), command)
			if _, err := a.ApproveLowRiskReview(id, "fixture iCode allowed"); err == nil {
				t.Fatal("high-risk command was automatically approved")
			}
			select {
			case <-writer.writes:
				t.Fatal("command reached terminal")
			default:
			}
		})
	}
}
