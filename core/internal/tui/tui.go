// Package tui is the Bubble Tea front-end for the standalone harness.
// It speaks to the same foundry.Client + agent.Loop used by the sidecar,
// so behavior (hard lock, tool policy, etc.) is identical.
package tui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/patmeh1/foundry-copilot/core/internal/agent"
	"github.com/patmeh1/foundry-copilot/core/internal/config"
	"github.com/patmeh1/foundry-copilot/core/internal/foundry"
	"github.com/patmeh1/foundry-copilot/core/internal/tools"
)

// Mode determines which screen drives the input.
type Mode int

const (
	ModeChat Mode = iota
	ModeAgent
)

func (m Mode) String() string {
	if m == ModeAgent {
		return "agent"
	}
	return "chat"
}

// Run launches the TUI.
func Run(ctx context.Context, initialMode Mode) error {
	cfg := config.Get()
	if cfg.Endpoint == "" {
		return fmt.Errorf("foundryCopilot.endpoint not configured; run `foundry-copilot init` first")
	}
	cred, err := foundry.NewCredential()
	if err != nil {
		return fmt.Errorf("azure credential: %w", err)
	}
	client, err := foundry.NewClient(cfg.Endpoint, cred)
	if err != nil {
		return fmt.Errorf("foundry client: %w", err)
	}
	m := newModel(cfg, client, initialMode)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithContext(ctx))
	program = p
	_, err = p.Run()
	return err
}

// ─── model ────────────────────────────────────────────────────────────────

type model struct {
	cfg     config.Config
	client  *foundry.Client
	mode    Mode
	width   int
	height  int

	viewport viewport.Model
	input    textarea.Model
	history  []historyEntry
	busy     bool
	status   string

	// Per-conversation state.
	chatMessages []foundry.ChatMessage
}

type historyEntry struct {
	speaker string // "you" | "foundry" | "tool" | "system"
	body    string
}

var (
	speakerStyle = map[string]lipgloss.Style{
		"you":     lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true),
		"foundry": lipgloss.NewStyle().Foreground(lipgloss.Color("219")).Bold(true),
		"tool":    lipgloss.NewStyle().Foreground(lipgloss.Color("214")),
		"system":  lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Italic(true),
	}
	frameStyle  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
)

func newModel(cfg config.Config, client *foundry.Client, mode Mode) *model {
	ta := textarea.New()
	ta.Placeholder = "Type your prompt — Enter to send, Ctrl+T to toggle mode, Ctrl+C to quit."
	ta.Focus()
	ta.Prompt = "▎"
	ta.SetHeight(3)
	ta.CharLimit = 8000
	vp := viewport.New(80, 20)
	return &model{
		cfg:    cfg,
		client: client,
		mode:   mode,
		input:  ta,
		viewport: vp,
		status: "ready",
		history: []historyEntry{{
			speaker: "system",
			body: fmt.Sprintf("foundry-copilot TUI · mode=%s · endpoint=%s",
				mode, cfg.Endpoint),
		}},
	}
}

func (m *model) Init() tea.Cmd { return textarea.Blink }

// ─── messages ─────────────────────────────────────────────────────────────

type assistantChunkMsg struct {
	delta    string
	finished bool
	err      error
}
type agentEventMsg struct {
	event   agent.Event
	final   bool
	final_  string // populated when final
	err     error
}
type tickMsg time.Time

// ─── update ───────────────────────────────────────────────────────────────

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		ih := 5
		vph := msg.Height - ih - 4
		if vph < 5 {
			vph = 5
		}
		m.viewport.Width = msg.Width - 4
		m.viewport.Height = vph
		m.input.SetWidth(msg.Width - 4)
		m.refreshViewport()
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "ctrl+t":
			if !m.busy {
				if m.mode == ModeChat {
					m.mode = ModeAgent
				} else {
					m.mode = ModeChat
				}
				m.appendHistory("system", "switched to "+m.mode.String()+" mode")
				m.refreshViewport()
			}
			return m, nil
		case "enter":
			if !m.busy {
				return m, m.submit()
			}
			return m, nil
		case "shift+enter":
			m.input, _ = m.input.Update(tea.KeyMsg{Type: tea.KeyEnter})
			return m, nil
		}

	case assistantChunkMsg:
		if msg.err != nil {
			m.appendHistory("system", "error: "+msg.err.Error())
			m.busy = false
			m.status = "ready"
			m.refreshViewport()
			return m, nil
		}
		// Append delta to the in-progress assistant message (last history entry).
		if n := len(m.history); n > 0 && m.history[n-1].speaker == "foundry" {
			m.history[n-1].body += msg.delta
		} else {
			m.appendHistory("foundry", msg.delta)
		}
		m.refreshViewport()
		if msg.finished {
			if n := len(m.history); n > 0 {
				m.chatMessages = append(m.chatMessages, foundry.ChatMessage{
					Role: "assistant", Content: m.history[n-1].body,
				})
			}
			m.busy = false
			m.status = "ready"
		}
		return m, nil

	case agentEventMsg:
		if msg.err != nil {
			m.appendHistory("system", "agent error: "+msg.err.Error())
			m.busy = false
			m.status = "ready"
			m.refreshViewport()
			return m, nil
		}
		if msg.final {
			m.appendHistory("foundry", msg.final_)
			m.busy = false
			m.status = "ready"
			m.refreshViewport()
			return m, nil
		}
		switch msg.event.Kind {
		case "think":
			m.status = fmt.Sprintf("step %d · thinking…", msg.event.Step)
		case "tool_call":
			m.appendHistory("tool", fmt.Sprintf("→ %s %s", msg.event.ToolName, msg.event.ToolArgs))
			m.status = fmt.Sprintf("step %d · %s", msg.event.Step, msg.event.ToolName)
		case "tool_result":
			truncated := msg.event.ToolResult
			if len(truncated) > 400 {
				truncated = truncated[:400] + "…"
			}
			m.appendHistory("tool", "← "+truncated)
		case "error":
			m.appendHistory("system", "tool error: "+msg.event.ToolError)
		}
		m.refreshViewport()
		return m, nil
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	cmds = append(cmds, cmd)
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)
	return m, tea.Batch(cmds...)
}

// ─── view ─────────────────────────────────────────────────────────────────

func (m *model) View() string {
	header := fmt.Sprintf(" foundry-copilot · mode=%s · %s ", m.mode, m.status)
	header = lipgloss.NewStyle().Background(lipgloss.Color("63")).Foreground(lipgloss.Color("230")).Render(header)
	body := frameStyle.Width(m.width - 2).Render(m.viewport.View())
	prompt := frameStyle.Width(m.width - 2).Render(m.input.View())
	hint := statusStyle.Render("Enter to send · Shift+Enter newline · Ctrl+T toggle mode · Ctrl+C quit")
	return lipgloss.JoinVertical(lipgloss.Left, header, body, prompt, hint)
}

// ─── helpers ──────────────────────────────────────────────────────────────

func (m *model) submit() tea.Cmd {
	text := strings.TrimSpace(m.input.Value())
	if text == "" {
		return nil
	}
	m.input.Reset()
	m.appendHistory("you", text)
	m.refreshViewport()
	m.busy = true
	m.status = "sending…"
	if m.mode == ModeAgent {
		return m.runAgent(text)
	}
	return m.runChat(text)
}

func (m *model) appendHistory(speaker, body string) {
	m.history = append(m.history, historyEntry{speaker: speaker, body: body})
}

func (m *model) refreshViewport() {
	var b strings.Builder
	for _, e := range m.history {
		st, ok := speakerStyle[e.speaker]
		if !ok {
			st = lipgloss.NewStyle()
		}
		fmt.Fprintf(&b, "%s %s\n\n", st.Render(strings.ToUpper(e.speaker)+":"), e.body)
	}
	m.viewport.SetContent(b.String())
	m.viewport.GotoBottom()
}

// ─── chat round ──────────────────────────────────────────────────────────

func (m *model) runChat(prompt string) tea.Cmd {
	m.chatMessages = append(m.chatMessages, foundry.ChatMessage{Role: "user", Content: prompt})
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		dep := m.cfg.ChatDeployment
		if dep == "" {
			return assistantChunkMsg{err: fmt.Errorf("chat_deployment not configured")}
		}
		// We can't stream chunks asynchronously from within a single tea.Cmd
		// without channels; collect everything then emit once. The TUI is
		// still responsive (UI thread isn't blocked because Cmd runs in a
		// goroutine).
		var full strings.Builder
		err := m.client.Chat(ctx, foundry.ChatRequest{
			Deployment: dep, Messages: m.chatMessages,
		}, func(chunk foundry.ChatChunk) error {
			if chunk.Delta != "" {
				full.WriteString(chunk.Delta)
			}
			return nil
		})
		if err != nil {
			return assistantChunkMsg{err: err}
		}
		return assistantChunkMsg{delta: full.String(), finished: true}
	}
}

// ─── agent round ─────────────────────────────────────────────────────────

func (m *model) runAgent(task string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		root := m.cfg.WorkspaceRoot
		if root == "" {
			cwd, _ := os.Getwd()
			root = cwd
		}
		reg := tools.NewRegistry()
		reg.Register(tools.FSRead{Root: root})
		reg.Register(tools.CodeSearch{Root: root})
		reg.Register(tools.FSWrite{Root: root, Allow: m.cfg.AgentAllowWrite})
		reg.Register(tools.Shell{Root: root, Allow: m.cfg.AgentAllowShell})
		loop := &agent.Loop{
			Foundry:    m.client,
			Tools:      reg,
			Deployment: m.cfg.ChatDeployment,
			MaxSteps:   m.cfg.AgentMaxSteps,
		}
		final, _, err := loop.Run(ctx, task, func(e agent.Event) {
			// Events arrive synchronously inside the loop; surface them
			// by sending intermediate messages back into the program.
			// (tea.Program.Send is the recommended way to inject messages
			// from outside the Update loop.)
			if program != nil {
				program.Send(agentEventMsg{event: e})
			}
		})
		if err != nil {
			return agentEventMsg{err: err}
		}
		return agentEventMsg{final: true, final_: final}
	}
}

// program is set by Run so runAgent can push intermediate events.
var program *tea.Program
