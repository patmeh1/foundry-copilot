package finetune

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestValidateExample_AcceptsValid(t *testing.T) {
	e := Example{Messages: []Message{
		{Role: "system", Content: "You are a helpful assistant."},
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "hello"},
	}}
	if err := ValidateExample(e); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestValidateExample_RequiresAssistant(t *testing.T) {
	e := Example{Messages: []Message{{Role: "user", Content: "x"}}}
	err := ValidateExample(e)
	if err == nil || !errors.Is(err, ErrInvalidExample) {
		t.Fatalf("want ErrInvalidExample, got %v", err)
	}
}

func TestValidateExample_RequiresInput(t *testing.T) {
	e := Example{Messages: []Message{{Role: "assistant", Content: "x"}}}
	err := ValidateExample(e)
	if err == nil || !errors.Is(err, ErrInvalidExample) {
		t.Fatalf("want ErrInvalidExample, got %v", err)
	}
}

func TestValidateExample_RejectsBadRole(t *testing.T) {
	e := Example{Messages: []Message{{Role: "robot", Content: "x"}, {Role: "assistant", Content: "y"}}}
	err := ValidateExample(e)
	if err == nil || !errors.Is(err, ErrInvalidExample) {
		t.Fatalf("want ErrInvalidExample, got %v", err)
	}
}

func TestValidateExample_RejectsEmptyContent(t *testing.T) {
	e := Example{Messages: []Message{{Role: "user", Content: ""}, {Role: "assistant", Content: "y"}}}
	err := ValidateExample(e)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateExample_RejectsEmptyMessages(t *testing.T) {
	if err := ValidateExample(Example{}); err == nil {
		t.Fatal("expected error")
	}
}

func TestWriteJSONL_RoundTrip(t *testing.T) {
	examples := []Example{
		{Messages: []Message{{Role: "user", Content: "hi"}, {Role: "assistant", Content: "hello"}}},
		{Messages: []Message{{Role: "user", Content: "what?"}, {Role: "assistant", Content: "this."}}},
	}
	var buf bytes.Buffer
	if err := WriteJSONL(&buf, examples); err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(buf.String(), "\n"); got != 2 {
		t.Fatalf("expected 2 newlines, got %d", got)
	}
	if err := ValidateJSONL(&buf); err != nil {
		t.Fatalf("round-trip validate: %v", err)
	}
}

func TestValidateJSONL_FirstErrorReported(t *testing.T) {
	// First example invalid (no assistant), second valid.
	in := `{"messages":[{"role":"user","content":"x"}]}
{"messages":[{"role":"user","content":"x"},{"role":"assistant","content":"y"}]}
`
	err := ValidateJSONL(strings.NewReader(in))
	if err == nil || !strings.Contains(err.Error(), "line 1") {
		t.Fatalf("want line 1 error, got %v", err)
	}
}

func TestValidateJSONL_SkipsBlankLines(t *testing.T) {
	in := "\n\n" + `{"messages":[{"role":"user","content":"x"},{"role":"assistant","content":"y"}]}` + "\n\n"
	if err := ValidateJSONL(strings.NewReader(in)); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}
