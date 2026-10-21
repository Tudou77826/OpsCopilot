package terminalcontrol

import (
	"bytes"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestObservationWindowRetainsBoundedTailAndCursor(t *testing.T) {
	var window outputWindow
	var model []byte
	for _, count := range []int{11, OutputCapacity - 7, 29, OutputCapacity*3 + 13, 3} {
		chunk := bytes.Repeat([]byte{byte('a' + len(model)%23)}, count)
		window.append(chunk)
		model = append(model, chunk...)
		for _, limit := range []int{1, 33, DefaultOutputLimit, OutputCapacity} {
			for _, after := range []uint64{0, uint64(len(model) / 2), uint64(len(model))} {
				got := window.read(after, limit)
				start := len(model) - limit
				if start < int(after) {
					start = int(after)
				}
				if got.Text != string(model[start:]) || got.From != uint64(start) || got.To != uint64(len(model)) || got.Truncated != (start > int(after)) {
					t.Fatalf("wrong window after=%d limit=%d: %+v", after, limit, got)
				}
			}
		}
		if len(window.data) != OutputCapacity {
			t.Fatal("output storage grew beyond capacity")
		}
	}
}

func TestObserveIsReadOnlyAndFiltersFrames(t *testing.T) {
	var written bytes.Buffer
	c, _ := New(&written)
	view, output, err := c.Observe(0, 0)
	if err != nil || view.Ready || output.Text != "" || written.Len() != 0 {
		t.Fatal(view, output, err)
	}
	c.Feed(append([]byte("before\r\n"), frame(c, 1, "/current")...))
	c.Feed([]byte("after\r\n"))
	view, output, err = c.Observe(0, OutputCapacity)
	if err != nil || !view.Ready || view.Directory != "/current" || output.Text != "before\r\nafter\r\n" || written.Len() != 0 {
		t.Fatal(view, output, err)
	}
	if _, _, err := c.Observe(output.To+1, 1); err == nil {
		t.Fatal("future cursor accepted")
	}
	for _, limit := range []int{-1, OutputCapacity + 1} {
		if _, _, err := c.Observe(0, limit); err == nil {
			t.Fatal("invalid limit accepted")
		}
	}
}

func TestObservationClippedUTF8RemainsValid(t *testing.T) {
	var window outputWindow
	window.append([]byte("日志中文"))
	result := window.read(0, 4)
	if !utf8.ValidString(result.Text) || !strings.HasSuffix(result.Text, "文") || !result.Truncated || result.To != 12 {
		t.Fatal(result)
	}
}
