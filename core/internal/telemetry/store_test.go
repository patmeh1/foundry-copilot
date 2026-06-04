package telemetry

import (
	"path/filepath"
	"testing"
	"time"
)

func TestOpen_CreatesParentDir(t *testing.T) {
	root := t.TempDir()
	deep := filepath.Join(root, "a", "b", "c", "telemetry.jsonl")
	if _, err := Open(deep); err != nil {
		t.Fatalf("Open: %v", err)
	}
}

func TestAppendAndSummary(t *testing.T) {
	root := t.TempDir()
	s, err := Open(filepath.Join(root, "t.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	events := []Event{
		{Timestamp: now.Add(-1 * time.Hour), Deployment: "gpt-4o", Surface: "chat", PromptTokens: 100, CompletionTokens: 50, LatencyMS: 800},
		{Timestamp: now.Add(-30 * time.Minute), Deployment: "gpt-4o", Surface: "chat", PromptTokens: 200, CompletionTokens: 75, LatencyMS: 1100, Error: "rate limit"},
		{Timestamp: now.Add(-48 * time.Hour), Deployment: "gpt-4o-mini", Surface: "completion", PromptTokens: 10, CompletionTokens: 5, LatencyMS: 60},
	}
	for _, e := range events {
		if err := s.Append(e); err != nil {
			t.Fatal(err)
		}
	}
	sum, err := s.Summary(24 * time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Calls != 2 {
		t.Fatalf("calls=%d want 2", sum.Calls)
	}
	if sum.PromptTokens != 300 {
		t.Fatalf("prompt=%d want 300", sum.PromptTokens)
	}
	if sum.CompletionTokens != 125 {
		t.Fatalf("completion=%d want 125", sum.CompletionTokens)
	}
	if sum.Errors != 1 {
		t.Fatalf("errors=%d want 1", sum.Errors)
	}
	if sum.ByDeployment["gpt-4o"] != 2 {
		t.Fatalf("by_deployment[gpt-4o]=%d", sum.ByDeployment["gpt-4o"])
	}
	if sum.ByDeployment["gpt-4o-mini"] != 0 {
		t.Fatalf("48h-old event should be outside 24h window")
	}
}

func TestTimeSeries_HourlyBuckets(t *testing.T) {
	root := t.TempDir()
	s, _ := Open(filepath.Join(root, "t.jsonl"))
	now := time.Now().UTC()
	s.Append(Event{Timestamp: now.Add(-1 * time.Hour), Deployment: "d", PromptTokens: 10, CompletionTokens: 5})
	s.Append(Event{Timestamp: now.Add(-2 * time.Hour), Deployment: "d", PromptTokens: 20, CompletionTokens: 0})
	s.Append(Event{Timestamp: now.Add(-3 * time.Hour), Deployment: "d", PromptTokens: 1, CompletionTokens: 1})
	buckets, err := s.TimeSeries(24 * time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if len(buckets) != 24 {
		t.Fatalf("len=%d want 24", len(buckets))
	}
	nonzero := 0
	for _, b := range buckets {
		if b.Calls > 0 {
			nonzero++
		}
	}
	if nonzero != 3 {
		t.Fatalf("nonzero buckets=%d want 3", nonzero)
	}
}

func TestRecentErrors(t *testing.T) {
	root := t.TempDir()
	s, _ := Open(filepath.Join(root, "t.jsonl"))
	s.Append(Event{Surface: "chat"})
	s.Append(Event{Surface: "chat", Error: "boom"})
	s.Append(Event{Surface: "completion", Error: "auth failed"})
	rows, err := s.RecentErrors(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows", len(rows))
	}
	if rows[0].Message != "auth failed" {
		t.Fatalf("most recent first: %q", rows[0].Message)
	}
}

func TestClear_RemovesFile(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "t.jsonl")
	s, _ := Open(p)
	s.Append(Event{Surface: "chat"})
	if err := s.Clear(); err != nil {
		t.Fatal(err)
	}
	// Second clear is also fine (idempotent).
	if err := s.Clear(); err != nil {
		t.Fatal(err)
	}
}

func TestPercentile(t *testing.T) {
	xs := []int64{10, 20, 30, 40, 50, 60, 70, 80, 90, 100}
	if p50 := percentile(xs, 50); p50 != 60 {
		t.Fatalf("p50=%d", p50)
	}
	if p95 := percentile(xs, 95); p95 != 100 {
		t.Fatalf("p95=%d", p95)
	}
	if got := percentile(nil, 50); got != 0 {
		t.Fatalf("empty: %d", got)
	}
}
