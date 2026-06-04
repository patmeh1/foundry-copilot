// Package telemetry is the local-first Foundry call-telemetry store.
//
// Every call the sidecar makes against the Foundry data-plane (chat,
// completion, agent, NES) emits a single Event row. v0.2.0 persists those
// rows as JSON Lines under ~/.foundry-copilot/telemetry.jsonl — pure stdlib,
// no SQLite dependency. v0.2.1 may swap to modernc.org/sqlite for indexed
// queries once the dependency tree review is complete; the Store interface
// shape will remain compatible.
//
// Telemetry is LOCAL by default. OTLP export is opt-in (Endpoint must be
// set) AND a per-host allow-list MUST contain the endpoint hostname or the
// export is refused. See otlp.go.
package telemetry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Event is one telemetry row.
type Event struct {
	Timestamp        time.Time `json:"timestamp"`
	Deployment       string    `json:"deployment"`
	Surface          string    `json:"surface"` // "chat" | "completion" | "agent" | "nes"
	PromptTokens     int       `json:"prompt_tokens"`
	CompletionTokens int       `json:"completion_tokens"`
	LatencyMS        int64     `json:"latency_ms"`
	Error            string    `json:"error,omitempty"`
}

// Store is the append-only JSONL store. Safe for concurrent use; appends
// serialize through a single mutex.
type Store struct {
	mu   sync.Mutex
	path string
}

// Open returns a Store rooted at the given file path. The parent directory
// is created if missing.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	return &Store{path: path}, nil
}

// Append writes one event. Timestamp is set to UTC now if zero-valued.
func (s *Store) Append(e Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now().UTC()
	}
	f, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(e)
}

// Summary is the aggregate returned by telemetry/summary RPC.
type Summary struct {
	Calls            int            `json:"calls"`
	PromptTokens     int            `json:"prompt_tokens"`
	CompletionTokens int            `json:"completion_tokens"`
	Errors           int            `json:"errors"`
	P50LatencyMS     int64          `json:"p50_latency_ms"`
	P95LatencyMS     int64          `json:"p95_latency_ms"`
	ByDeployment     map[string]int `json:"by_deployment"`
}

// Summary computes the rollup over the trailing window.
func (s *Store) Summary(window time.Duration) (Summary, error) {
	events, err := s.read()
	if err != nil {
		return Summary{}, err
	}
	cutoff := time.Now().UTC().Add(-window)
	out := Summary{ByDeployment: map[string]int{}}
	var lats []int64
	for _, e := range events {
		if e.Timestamp.Before(cutoff) {
			continue
		}
		out.Calls++
		out.PromptTokens += e.PromptTokens
		out.CompletionTokens += e.CompletionTokens
		if e.Error != "" {
			out.Errors++
		}
		out.ByDeployment[e.Deployment]++
		lats = append(lats, e.LatencyMS)
	}
	out.P50LatencyMS = percentile(lats, 50)
	out.P95LatencyMS = percentile(lats, 95)
	return out, nil
}

// Bucket is one hourly time-series cell.
type Bucket struct {
	Start  time.Time `json:"start"`
	Calls  int       `json:"calls"`
	Tokens int       `json:"tokens"`
}

// TimeSeries returns one Bucket per hour in the trailing window, in
// chronological order. Empty buckets are included.
func (s *Store) TimeSeries(window time.Duration) ([]Bucket, error) {
	events, err := s.read()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC().Truncate(time.Hour)
	hours := int(window / time.Hour)
	if hours <= 0 {
		hours = 1
	}
	buckets := make([]Bucket, hours)
	for i := 0; i < hours; i++ {
		buckets[i] = Bucket{Start: now.Add(-time.Duration(hours-1-i) * time.Hour)}
	}
	cutoff := buckets[0].Start
	for _, e := range events {
		if e.Timestamp.Before(cutoff) {
			continue
		}
		idx := int(e.Timestamp.Truncate(time.Hour).Sub(cutoff) / time.Hour)
		if idx < 0 || idx >= len(buckets) {
			continue
		}
		buckets[idx].Calls++
		buckets[idx].Tokens += e.PromptTokens + e.CompletionTokens
	}
	return buckets, nil
}

// ErrorRow is a single error row returned by telemetry/errors.
type ErrorRow struct {
	Timestamp  time.Time `json:"timestamp"`
	Deployment string    `json:"deployment"`
	Surface    string    `json:"surface"`
	Message    string    `json:"message"`
}

// RecentErrors returns up to `limit` most recent error events.
func (s *Store) RecentErrors(limit int) ([]ErrorRow, error) {
	events, err := s.read()
	if err != nil {
		return nil, err
	}
	var rows []ErrorRow
	for i := len(events) - 1; i >= 0 && len(rows) < limit; i-- {
		e := events[i]
		if e.Error == "" {
			continue
		}
		rows = append(rows, ErrorRow{Timestamp: e.Timestamp, Deployment: e.Deployment, Surface: e.Surface, Message: e.Error})
	}
	return rows, nil
}

// Clear deletes the store file. Used by telemetry/clear RPC.
func (s *Store) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := os.Remove(s.path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func (s *Store) read() ([]Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := os.Open(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	var out []Event
	for dec.More() {
		var e Event
		if err := dec.Decode(&e); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

func percentile(xs []int64, p int) int64 {
	if len(xs) == 0 {
		return 0
	}
	cp := make([]int64, len(xs))
	copy(cp, xs)
	sort.Slice(cp, func(i, j int) bool { return cp[i] < cp[j] })
	idx := (len(cp) * p) / 100
	if idx >= len(cp) {
		idx = len(cp) - 1
	}
	return cp[idx]
}
