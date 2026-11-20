package tui

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// readTurnLog returns each JSONL line of path decoded as a map.
func readTurnLog(t *testing.T, path string) []map[string]any {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open turn log: %v", err)
	}
	defer f.Close()
	var out []map[string]any
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var row map[string]any
		if err := json.Unmarshal(sc.Bytes(), &row); err != nil {
			t.Fatalf("decode line %q: %v", sc.Text(), err)
		}
		out = append(out, row)
	}
	return out
}

func TestAppendTurnRecord_CreatesDirAndAppends(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "turn-timings.jsonl")
	start := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	first := turnRecord{
		Start: start, End: start.Add(83123 * time.Millisecond), DurationMs: 83123,
		Outcome: "done", SessionID: "s1", Cwd: "/tmp", Model: "m",
		APIRequests: 2, ToolCalls: 1, InputTokens: 150, OutputTokens: 30,
	}
	if err := appendTurnRecord(path, first); err != nil {
		t.Fatalf("append first: %v", err)
	}
	if err := appendTurnRecord(path, turnRecord{Outcome: "error"}); err != nil {
		t.Fatalf("append second: %v", err)
	}
	rows := readTurnLog(t, path)
	if len(rows) != 2 {
		t.Fatalf("want 2 lines, got %d", len(rows))
	}
	if rows[0]["outcome"] != "done" || rows[0]["duration_ms"] != float64(83123) {
		t.Errorf("first row = %v", rows[0])
	}
	for _, k := range []string{"start", "end", "session_id", "cwd", "model", "api_requests", "tool_calls", "input_tokens", "output_tokens"} {
		if _, ok := rows[0][k]; !ok {
			t.Errorf("first row missing key %q", k)
		}
	}
	if rows[1]["outcome"] != "error" {
		t.Errorf("second row outcome = %v", rows[1]["outcome"])
	}
}

func TestWriteTurnRecordCmd_EmptyPathIsNil(t *testing.T) {
	if writeTurnRecordCmd("", turnRecord{}) != nil {
		t.Fatal("empty path should yield a nil Cmd")
	}
}
