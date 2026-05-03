package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// mockReader returns data in controlled chunks for testing readTerminalChunk.
type mockReader struct {
	chunks [][]byte
	idx    int
}

func (m *mockReader) Read(p []byte) (int, error) {
	if m.idx >= len(m.chunks) {
		return 0, errors.New("EOF")
	}
	chunk := m.chunks[m.idx]
	m.idx++
	n := copy(p, chunk)
	return n, nil
}

func TestReadTerminalChunk_PartialRead_NoAggregation(t *testing.T) {
	// Interactive scenario: small data, buffer not filled
	data := "hello world"
	r := strings.NewReader(data)
	buf := make([]byte, 32768)

	n, _ := r.Read(buf)
	got := readTerminalChunk(r, buf, n)

	if got != data {
		t.Errorf("partial read: got %q, want %q", got, data)
	}
}

func TestReadTerminalChunk_PartialRead_PreservesExactBytes(t *testing.T) {
	// Ensure no extra bytes from previous buffer contents
	buf := make([]byte, 32768)
	// Fill buffer with garbage
	for i := range buf {
		buf[i] = 0xFF
	}
	r := strings.NewReader("abc")
	n, _ := r.Read(buf)
	got := readTerminalChunk(r, buf, n)

	if got != "abc" {
		t.Errorf("got %q (len=%d), want %q", got, len(got), "abc")
	}
}

func TestReadTerminalChunk_FullReadThenShortRead_Aggregates(t *testing.T) {
	// High-throughput: first read fills buffer, second is short
	bufSize := 8
	chunk1 := bytes.Repeat([]byte("A"), bufSize) // fills buffer
	chunk2 := []byte("end")                       // short read

	r := &mockReader{chunks: [][]byte{chunk1, chunk2}}
	buf := make([]byte, bufSize)

	n, _ := r.Read(buf)
	got := readTerminalChunk(r, buf, n)

	want := string(chunk1) + string(chunk2)
	if got != want {
		t.Errorf("full+short: got %d bytes, want %d bytes", len(got), len(want))
	}
}

func TestReadTerminalChunk_MultipleFullReadsThenShort(t *testing.T) {
	// Multiple full reads before a short read
	bufSize := 4
	chunk1 := []byte("AAAA") // full
	chunk2 := []byte("BBBB") // full
	chunk3 := []byte("CCCC") // full
	chunk4 := []byte("end")  // short

	r := &mockReader{chunks: [][]byte{chunk1, chunk2, chunk3, chunk4}}
	buf := make([]byte, bufSize)

	n, _ := r.Read(buf)
	got := readTerminalChunk(r, buf, n)

	want := "AAAA" + "BBBB" + "CCCC" + "end"
	if got != want {
		t.Errorf("multi full+short: got %q, want %q", got, want)
	}
}

func TestReadTerminalChunk_SizeLimit(t *testing.T) {
	// Should stop aggregating at 256KB limit
	bufSize := 32768
	// Provide enough full chunks to exceed 256KB (8 full chunks = 256KB)
	chunks := make([][]byte, 10) // 10 full chunks = 320KB > 256KB
	for i := range chunks {
		chunks[i] = bytes.Repeat([]byte{byte('A' + i)}, bufSize)
	}

	r := &mockReader{chunks: chunks}
	buf := make([]byte, bufSize)

	n, _ := r.Read(buf)
	got := readTerminalChunk(r, buf, n)

	// Should be capped at 256KB (8 chunks of 32KB)
	if len(got) > 262144 {
		t.Errorf("size limit: got %d bytes, expected <= 262144", len(got))
	}
	if len(got) < bufSize {
		t.Errorf("size limit: got %d bytes, expected at least one chunk", len(got))
	}
}

func TestReadTerminalChunk_FullReadThenEOF(t *testing.T) {
	// Full read followed by error (connection close during aggregation)
	bufSize := 8
	chunk1 := bytes.Repeat([]byte("X"), bufSize) // full
	// No more chunks → mockReader returns error on next Read

	r := &mockReader{chunks: [][]byte{chunk1}}
	buf := make([]byte, bufSize)

	n, _ := r.Read(buf)
	got := readTerminalChunk(r, buf, n)

	// Should return what was accumulated before the error
	if got != string(chunk1) {
		t.Errorf("full+EOF: got %q, want %q", got, string(chunk1))
	}
}

func TestReadTerminalChunk_ExactBufferSize_NoMoreData(t *testing.T) {
	// Edge case: data is exactly bufSize bytes (fills buffer)
	// and no more data follows — aggregation enters inner loop,
	// gets error, breaks out with just the first chunk.
	bufSize := 16
	data := bytes.Repeat([]byte("Z"), bufSize)

	r := &mockReader{chunks: [][]byte{data}}
	buf := make([]byte, bufSize)

	n, _ := r.Read(buf)
	got := readTerminalChunk(r, buf, n)

	if got != string(data) {
		t.Errorf("exact buffer: got %d bytes, want %d bytes", len(got), len(data))
	}
}
