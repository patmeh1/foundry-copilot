// Package finetune builds fine-tuning datasets from telemetry sessions and
// orchestrates fine-tuning jobs against the Foundry data-plane.
//
// v0.2.0 ships the dataset builder + validator (fully implemented and tested)
// and a stub JobsClient (real REST round-trippers in v0.2.1). The dataset
// format matches OpenAI chat-completions JSONL so any downstream tooling that
// validates against the OpenAI spec also validates ours.
package finetune

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Example is one OpenAI chat-completions fine-tune row.
type Example struct {
	Messages []Message `json:"messages"`
}

// Message is one entry in an Example.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ErrInvalidExample is returned by ValidateExample.
var ErrInvalidExample = errors.New("finetune: invalid example")

// ValidateExample enforces the OpenAI fine-tune dataset rules:
//   - at least one user OR system message
//   - at least one assistant message
//   - role is one of system|user|assistant
//   - content is non-empty
func ValidateExample(e Example) error {
	if len(e.Messages) == 0 {
		return fmt.Errorf("%w: messages is empty", ErrInvalidExample)
	}
	hasInput := false
	hasAssistant := false
	for i, m := range e.Messages {
		switch m.Role {
		case "system", "user":
			hasInput = true
		case "assistant":
			hasAssistant = true
		default:
			return fmt.Errorf("%w: message[%d] role %q not in system|user|assistant", ErrInvalidExample, i, m.Role)
		}
		if strings.TrimSpace(m.Content) == "" {
			return fmt.Errorf("%w: message[%d] content is empty", ErrInvalidExample, i)
		}
	}
	if !hasInput {
		return fmt.Errorf("%w: needs at least one user or system message", ErrInvalidExample)
	}
	if !hasAssistant {
		return fmt.Errorf("%w: needs at least one assistant message", ErrInvalidExample)
	}
	return nil
}

// WriteJSONL emits examples as JSONL (one object per line). The caller owns w.
func WriteJSONL(w io.Writer, examples []Example) error {
	enc := json.NewEncoder(w)
	for i, e := range examples {
		if err := enc.Encode(e); err != nil {
			return fmt.Errorf("finetune: write example[%d]: %w", i, err)
		}
	}
	return nil
}

// ValidateJSONL reads examples from r and returns the first validation error
// (or nil on success). Mirrors OpenAI's validator workflow so users get the
// same outcome locally as they would after upload.
func ValidateJSONL(r io.Reader) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	line := 0
	for scanner.Scan() {
		line++
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" {
			continue
		}
		var e Example
		if err := json.Unmarshal([]byte(raw), &e); err != nil {
			return fmt.Errorf("finetune: line %d: parse: %w", line, err)
		}
		if err := ValidateExample(e); err != nil {
			return fmt.Errorf("finetune: line %d: %w", line, err)
		}
	}
	return scanner.Err()
}
