// Package agent runs the tool-use loop: model → tool calls → results → model.
// All model calls go through foundry.Client (which enforces the hard lock).
package agent

import (
	"context"
	"fmt"
	"time"

	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/shared"

	"github.com/patmeh1/foundry-copilot/core/internal/foundry"
	"github.com/patmeh1/foundry-copilot/core/internal/tools"
)

// Loop wires a Foundry client and a tool registry into a multi-turn
// agent. It is constructed per agent/run RPC.
type Loop struct {
	Foundry    *foundry.Client
	Tools      *tools.Registry
	Deployment string
	System     string
	MaxSteps   int
}

// Event is one observable thing that happened during the loop. The RPC
// layer maps each into an "agent/event" notification.
type Event struct {
	Kind       string `json:"kind"` // "think" | "tool_call" | "tool_result" | "final" | "error"
	Step       int    `json:"step"`
	Text       string `json:"text,omitempty"`
	ToolName   string `json:"tool,omitempty"`
	ToolArgs   string `json:"args,omitempty"`
	ToolResult string `json:"result,omitempty"`
	ToolError  string `json:"tool_error,omitempty"`
}

// EmitFn is the callback the RPC layer hands in to forward events.
type EmitFn func(Event)

// Run executes the loop for one user prompt, emitting events along the
// way and returning the final assistant message.
func (l *Loop) Run(ctx context.Context, userPrompt string, emit EmitFn) (string, int, error) {
	if l.Foundry == nil {
		return "", 0, fmt.Errorf("agent: foundry client required")
	}
	if l.Tools == nil {
		return "", 0, fmt.Errorf("agent: tool registry required")
	}
	if l.Deployment == "" {
		return "", 0, fmt.Errorf("agent: deployment required")
	}
	if l.MaxSteps <= 0 {
		l.MaxSteps = 12
	}
	if emit == nil {
		emit = func(Event) {}
	}

	system := l.System
	if system == "" {
		system = defaultSystemPrompt
	}
	msgs := []foundry.ChatMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: userPrompt},
	}

	toolParams := buildToolParams(l.Tools)

	for step := 1; step <= l.MaxSteps; step++ {
		if err := ctx.Err(); err != nil {
			return "", step - 1, err
		}
		params := openai.ChatCompletionNewParams{
			Model:               shared.ChatModel(l.Deployment),
			Messages:            foundryMessagesToOpenAI(msgs),
			Temperature:         param.NewOpt(0.2),
			MaxCompletionTokens: param.NewOpt(int64(2048)),
			Tools:               toolParams,
		}
		resp, err := l.Foundry.Inner().Chat.Completions.New(ctx, params)
		if err != nil {
			emit(Event{Kind: "error", Step: step, Text: err.Error()})
			return "", step, err
		}
		if len(resp.Choices) == 0 {
			emit(Event{Kind: "final", Step: step})
			return "", step, nil
		}
		choice := resp.Choices[0]
		assistant := foundry.ChatMessage{
			Role:    "assistant",
			Content: choice.Message.Content,
		}
		if len(choice.Message.ToolCalls) > 0 {
			for _, tc := range choice.Message.ToolCalls {
				fn := tc.AsFunction()
				assistant.ToolCalls = append(assistant.ToolCalls, foundry.ToolCallSpec{
					ID:        fn.ID,
					Name:      fn.Function.Name,
					Arguments: fn.Function.Arguments,
				})
			}
		}
		msgs = append(msgs, assistant)
		if choice.Message.Content != "" {
			emit(Event{Kind: "think", Step: step, Text: choice.Message.Content})
		}
		if len(assistant.ToolCalls) == 0 {
			emit(Event{Kind: "final", Step: step, Text: choice.Message.Content})
			return choice.Message.Content, step, nil
		}
		// Invoke each tool, append a tool message per call.
		for _, call := range assistant.ToolCalls {
			emit(Event{Kind: "tool_call", Step: step, ToolName: call.Name, ToolArgs: call.Arguments})
			tool, ok := l.Tools.Get(call.Name)
			if !ok {
				msg := fmt.Sprintf("unknown tool: %s", call.Name)
				emit(Event{Kind: "tool_result", Step: step, ToolName: call.Name, ToolError: msg})
				msgs = append(msgs, foundry.ChatMessage{
					Role:       "tool",
					ToolCallID: call.ID,
					Content:    "ERROR: " + msg,
				})
				continue
			}
			tctx, cancel := context.WithTimeout(ctx, 60*time.Second)
			result, err := tool.Invoke(tctx, call.Arguments)
			cancel()
			if err != nil {
				emit(Event{Kind: "tool_result", Step: step, ToolName: call.Name, ToolError: err.Error(), ToolResult: result})
				msgs = append(msgs, foundry.ChatMessage{
					Role:       "tool",
					ToolCallID: call.ID,
					Content:    fmt.Sprintf("ERROR: %s\n%s", err.Error(), result),
				})
				continue
			}
			emit(Event{Kind: "tool_result", Step: step, ToolName: call.Name, ToolResult: result})
			msgs = append(msgs, foundry.ChatMessage{
				Role:       "tool",
				ToolCallID: call.ID,
				Content:    result,
			})
		}
	}
	final := "agent: reached max steps without final answer"
	emit(Event{Kind: "error", Step: l.MaxSteps, Text: final})
	return final, l.MaxSteps, fmt.Errorf("%s", final)
}

const defaultSystemPrompt = `You are Foundry Copilot, a coding assistant running on Microsoft Foundry.

You can call tools to inspect and modify a user's workspace. Prefer reading code before changing it. Be concise: think briefly, call tools when useful, and write a clear final answer. Do not call a tool if you already have the information you need. When you have finished, return a final assistant message with no tool calls.`

// buildToolParams converts our Tool registry into openai-go's tool union.
func buildToolParams(reg *tools.Registry) []openai.ChatCompletionToolUnionParam {
	all := reg.All()
	out := make([]openai.ChatCompletionToolUnionParam, 0, len(all))
	for _, t := range all {
		out = append(out, openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
			Name:        t.Name(),
			Description: param.NewOpt(t.Description()),
			Parameters:  shared.FunctionParameters(t.ParametersSchema()),
		}))
	}
	return out
}

// foundryMessagesToOpenAI is exported indirectly via foundry.Client.Inner();
// since toOpenAIMessages is unexported in foundry, we duplicate the small
// mapping here. (Keeping the foundry package's helper unexported preserves
// its package-private contract.)
//
// We re-build the param slice by going through the same conversions
// foundry.Client uses internally. The only realistic alternative would be
// to plumb a streaming call through the existing Client.Chat — but the
// agent needs the full response message (for tool_calls), so we hit
// Inner().Chat.Completions.New() directly and rebuild the param slice here.
func foundryMessagesToOpenAI(msgs []foundry.ChatMessage) []openai.ChatCompletionMessageParamUnion {
	out := make([]openai.ChatCompletionMessageParamUnion, 0, len(msgs))
	for _, m := range msgs {
		switch m.Role {
		case "system":
			out = append(out, openai.SystemMessage(m.Content))
		case "tool":
			id := m.ToolCallID
			if id == "" {
				id = m.ToolID
			}
			out = append(out, openai.ToolMessage(m.Content, id))
		case "assistant":
			if len(m.ToolCalls) == 0 {
				out = append(out, openai.AssistantMessage(m.Content))
				continue
			}
			calls := make([]openai.ChatCompletionMessageToolCallUnionParam, 0, len(m.ToolCalls))
			for _, tc := range m.ToolCalls {
				calls = append(calls, openai.ChatCompletionMessageToolCallUnionParam{
					OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
						ID: tc.ID,
						Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
							Name:      tc.Name,
							Arguments: tc.Arguments,
						},
					},
				})
			}
			ap := openai.ChatCompletionAssistantMessageParam{ToolCalls: calls}
			if m.Content != "" {
				ap.Content = openai.ChatCompletionAssistantMessageParamContentUnion{
					OfString: param.NewOpt(m.Content),
				}
			}
			out = append(out, openai.ChatCompletionMessageParamUnion{OfAssistant: &ap})
		case "user":
			fallthrough
		default:
			out = append(out, openai.UserMessage(m.Content))
		}
	}
	return out
}
