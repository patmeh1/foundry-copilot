package nes

import (
	"context"
	"errors"
	"testing"
)

func TestStubPredictor_ReturnsNotImplemented(t *testing.T) {
	p := StubPredictor{}
	_, err := p.Predict(context.Background(), PredictRequest{File: "foo.go", Line: 10, Column: 0})
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("want ErrNotImplemented, got %v", err)
	}
}

// Ensure the interface is satisfied at compile time.
var _ Predictor = StubPredictor{}
