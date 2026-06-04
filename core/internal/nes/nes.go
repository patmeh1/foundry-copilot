// Package nes implements Next Edit Suggestions — predicting both the
// *location* and the *content* of the user's next edit, going beyond simple
// trailing-cursor inline completion.
//
// v0.2.0 ships the predictor *interface* and a stub implementation that
// always returns ErrNotImplemented. The real Foundry-backed predictor (using
// a streaming chat completion with a curated nes-system prompt) lands in
// v0.2.1. The interface is stable so the extension's NES provider can be
// registered behind foundryCopilot.nes.enabled today.
package nes

import (
	"context"
	"errors"
)

// ErrNotImplemented is returned by the StubPredictor.
var ErrNotImplemented = errors.New("nes.Predict: not yet implemented (v0.2 scaffold)")

// PredictRequest is sent by the extension via the nes/predict RPC.
type PredictRequest struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	Before   string `json:"before"`    // text before cursor (up to N bytes)
	After    string `json:"after"`     // text after cursor (up to N bytes)
	LastEdit string `json:"last_edit"` // short summary of the most recent edit
}

// PredictResponse is rendered as ghost text. Empty File signals no
// suggestion is available.
type PredictResponse struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
	Text   string `json:"text"`
}

// Predictor is the interface the RPC handler depends on.
type Predictor interface {
	Predict(ctx context.Context, req PredictRequest) (PredictResponse, error)
}

// StubPredictor is the v0.2.0 default. Returns ErrNotImplemented unconditionally.
type StubPredictor struct{}

// Predict implements Predictor.
func (StubPredictor) Predict(ctx context.Context, req PredictRequest) (PredictResponse, error) {
	return PredictResponse{}, ErrNotImplemented
}
