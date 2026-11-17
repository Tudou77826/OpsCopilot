package opssidecar

import (
	"slices"
	"testing"
)

func TestOutputGateBuffersUntilReady(t *testing.T) {
	g := newOutputGate()
	var direct [][]byte

	g.deliver([]byte("a"), func(d []byte) { direct = append(direct, d) })
	g.deliver([]byte("b"), func(d []byte) { direct = append(direct, d) })
	if len(direct) != 0 {
		t.Fatalf("ready 前不应直通，实际直通 %d 段", len(direct))
	}

	flushed := g.markReady()
	want := []string{"a", "b"}
	if len(flushed) != len(want) {
		t.Fatalf("markReady 冲刷段数 = %d, want %d", len(flushed), len(want))
	}
	for i, chunk := range flushed {
		if string(chunk) != want[i] {
			t.Fatalf("冲刷段[%d] = %q, want %q", i, chunk, want[i])
		}
	}

	g.deliver([]byte("c"), func(d []byte) { direct = append(direct, d) })
	if len(direct) != 1 || string(direct[0]) != "c" {
		t.Fatalf("ready 后应直通，实际 %v", direct)
	}

	if again := g.markReady(); again != nil {
		t.Fatalf("markReady 应幂等，返回 %v", again)
	}
}

func TestOutputGateFlushAndDirectNeverInterleave(t *testing.T) {
	// Issue #74 场景：冲刷与直通共用一把锁，闸门内多段输出按到达顺序整段冲刷
	g := newOutputGate()
	var out []string
	emit := func(d []byte) { out = append(out, string(d)) }

	g.deliver([]byte("m1"), func(d []byte) { emit(d) })
	g.deliver([]byte("m2"), func(d []byte) { emit(d) })
	for _, chunk := range g.markReady() {
		emit(chunk)
	}
	g.deliver([]byte("live"), func(d []byte) { emit(d) })

	want := []string{"m1", "m2", "live"}
	if !slices.Equal(out, want) {
		t.Fatalf("输出顺序 = %v, want %v", out, want)
	}
}

func TestOutputGateOverflowPassesThrough(t *testing.T) {
	g := newOutputGate()
	big := make([]byte, outputGateMax) // 恰好达到上限：仍暂存
	g.deliver(big, func([]byte) {})
	if flushed := g.markReady(); len(flushed) != 1 {
		t.Fatalf("上限内的输出应暂存，实际冲刷 %d 段", len(flushed))
	}

	g2 := newOutputGate()
	var direct [][]byte
	g2.deliver(make([]byte, outputGateMax), func([]byte) {})
	overflow := make([]byte, 1)
	g2.deliver(overflow, func(d []byte) { direct = append(direct, d) })
	if len(direct) != 1 {
		t.Fatalf("超限输出应直通防积压，实际暂存")
	}
	if flushed := g2.markReady(); len(flushed) != 1 {
		t.Fatalf("首段应仍在闸门内，实际冲刷 %d 段", len(flushed))
	}
}

func TestOutputGateCloseDiscards(t *testing.T) {
	g := newOutputGate()
	g.deliver([]byte("x"), func([]byte) {})
	g.close()
	if flushed := g.markReady(); flushed != nil {
		t.Fatalf("close 后不应冲刷，返回 %v", flushed)
	}
	var direct [][]byte
	g.deliver([]byte("y"), func(d []byte) { direct = append(direct, d) })
	if len(direct) != 0 {
		t.Fatalf("close 后 deliver 应丢弃，实际直通")
	}
}
