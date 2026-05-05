// Package agent runs the main think/act loop for OLLIE.
// This is the orchestration layer that turns intent into OS actions.
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"lumin-engine/internal/permissions"
	"lumin-engine/internal/tools"
)

// MaxIterations is a hard cap to avoid runaway loops.
// Most tasks finish in a few passes; hitting this is a safety net.
const MaxIterations = 20

// --- Message roles ---

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Message is a single turn in the conversation.
type Message struct {
	Role       Role
	Content    string
	ToolName   string // set for RoleTool only
	TokenCount int    // filled by context manager
}

// --- Events streamed back to the caller ---

// EventType tags the downstream event type.
type EventType string

const (
	// EventToken is a text fragment for live UI output.
	EventToken EventType = "token"

	// EventToolCall marks a tool invocation starting.
	EventToolCall EventType = "tool_call"

	// EventToolResult carries the tool outcome.
	EventToolResult EventType = "tool_result"

	// EventDone marks a clean finish.
	EventDone EventType = "done"

	// EventError marks a fatal stop.
	EventError EventType = "error"
)

// Event is streamed to IPC and UI.
type Event struct {
	Type EventType

	// Token payload
	Text string

	// Tool call payload
	ToolCall *ParsedToolCall

	// Tool result payload
	ToolResult *tools.Result

	// Error payload
	Err error
}

// --- Inference interface (implemented by internal/inference) ---

// Inferencer is the agent's view of the model runtime.
type Inferencer interface {
	// Generate streams tokens to out; closes out on EOS/stop.
	Generate(ctx context.Context, tokens []int32, out chan<- string) error

	// Tokenize maps text to token IDs.
	Tokenize(text string) []int32

	// Detokenize maps a single token back to text.
	Detokenize(token int32) string
}

// --- Context builder interface (implemented by internal/context) ---

// ContextBuilder builds prompt tokens from the current conversation state.
type ContextBuilder interface {
	// Add appends a message to history.
	Add(msg Message)

	// BuildTokens returns the prompt token sequence, trimming if needed.
	BuildTokens() []int32

	// Reset clears history except the system prompt.
	Reset()
}

// --- Agent ---

// Agent wires inference, context, tools, and permissions into one loop.
type Agent struct {
	model    Inferencer
	ctx      ContextBuilder
	executor *tools.Executor
	perms    *permissions.Policy
	log      *slog.Logger
}

// New builds an Agent. Use Run per request.
func New(
	model Inferencer,
	ctx ContextBuilder,
	executor *tools.Executor,
	perms *permissions.Policy,
	log *slog.Logger,
) *Agent {
	return &Agent{
		model:    model,
		ctx:      ctx,
		executor: executor,
		perms:    perms,
		log:      log,
	}
}

// --- Run: main agent loop ---

// Run handles a single request and streams events to out.
// Caller does not close out; Run does. cancelCtx aborts in-flight work.
func (a *Agent) Run(cancelCtx context.Context, userMessage string, out chan<- Event) {
	defer close(out)

	a.log.Info("agent loop started", "message_len", len(userMessage))

	// Stash user input in shared context.
	a.ctx.Add(Message{Role: RoleUser, Content: userMessage})

	parser := NewStreamParser()

	for iteration := range MaxIterations {
		a.log.Debug("agent iteration", "n", iteration+1)

		// Step A: build prompt tokens.
		tokens := a.ctx.BuildTokens()
		if len(tokens) == 0 {
			out <- Event{Type: EventError, Err: errors.New("context produced empty token sequence")}
			return
		}

		// Step B: stream tokens through the parser.
		tokenCh := make(chan string, 128)
		inferErr := make(chan error, 1)

		go func() {
			inferErr <- a.model.Generate(cancelCtx, tokens, tokenCh)
		}()

		var assistantBuf strings.Builder
		parser.Reset()
		toolCallFound := false

		feedLoop:
		for {
			select {
			case <-cancelCtx.Done():
				out <- Event{Type: EventError, Err: fmt.Errorf("request cancelled")}
				return

			case token, ok := <-tokenCh:
				if !ok {
					// Inference pass finished.
					break feedLoop
				}

				result := parser.Feed(token)

				switch result.Kind {
				case ParseKindText:
					// Plain text, forward to UI.
					assistantBuf.WriteString(result.Text)
					out <- Event{Type: EventToken, Text: result.Text}

				case ParseKindToolCall:
					// Tool call detected; pause text and handle it.
					toolCallFound = true

					// Persist assistant text before the call.
					if t := strings.TrimSpace(assistantBuf.String()); t != "" {
						a.ctx.Add(Message{Role: RoleAssistant, Content: t})
					}
					assistantBuf.Reset()

					// Drain remaining tokens for this pass.
					for range tokenCh {}

					// Step C: execute the tool call.
					a.log.Info("tool call", "tool", result.ToolCall.Name, "args", result.ToolCall.Arguments)
					out <- Event{Type: EventToolCall, ToolCall: result.ToolCall}

					toolResult := a.executeChecked(result.ToolCall)
					out <- Event{Type: EventToolResult, ToolResult: toolResult}

					// Step D: inject result, then loop.
					content := toolResult.Output
					if toolResult.Error != "" {
						content = fmt.Sprintf("error: %s", toolResult.Error)
					}
					a.ctx.Add(Message{
						Role:     RoleTool,
						ToolName: result.ToolCall.Name,
						Content:  content,
					})

					break feedLoop
				}

			case err := <-inferErr:
				if err != nil && !errors.Is(err, context.Canceled) {
					out <- Event{Type: EventError, Err: fmt.Errorf("inference error: %w", err)}
					return
				}
			}
		}

		// Wait for inference to finish before looping.
		if err := <-inferErr; err != nil && !errors.Is(err, context.Canceled) {
			out <- Event{Type: EventError, Err: fmt.Errorf("inference error: %w", err)}
			return
		}

		if toolCallFound {
			// Tool ran; loop so the model sees the result.
			continue
		}

		// Step E: no tool call, finalize response.
		if final := strings.TrimSpace(assistantBuf.String()); final != "" {
			a.ctx.Add(Message{Role: RoleAssistant, Content: final})
		}
		a.log.Info("agent loop complete", "iterations", iteration+1)
		out <- Event{Type: EventDone}
		return
	}

	// Safety cap reached.
	a.log.Warn("agent loop hit max iterations", "max", MaxIterations)
	out <- Event{
		Type: EventToken,
		Text: "\n\n[I've reached my step limit — the task may be incomplete. You can ask me to continue.]",
	}
	out <- Event{Type: EventDone}
}

// --- executeChecked: permission-gated tool execution ---

// executeChecked enforces permissions before touching the OS.
// Returns an error result if the capability is not granted.
func (a *Agent) executeChecked(call *ParsedToolCall) *tools.Result {
	// 1) Capability gate.
	if !a.perms.IsEnabled(call.Name) {
		return &tools.Result{
			Error: fmt.Sprintf(
				"capability '%s' is not enabled. The user can enable it in the LuminAI Control Panel.",
				call.Name,
			),
		}
	}

	// 2) Path allowlist for filesystem tools.
	if path, ok := call.Arguments["path"]; ok {
		if !a.perms.PathAllowed(path) {
			return &tools.Result{
				Error: fmt.Sprintf(
					"path '%s' is not in the allowlist. The user can add it in the LuminAI Control Panel.",
					path,
				),
			}
		}
	}

	// 3) Execute via the tool executor.
	rawArgs, err := json.Marshal(call.Arguments)
	if err != nil {
		return &tools.Result{Error: fmt.Sprintf("failed to encode tool arguments: %v", err)}
	}

	execResult, execErr := a.executor.Execute(call.Name, rawArgs)
	result := &tools.Result{}
	if execErr != nil {
		result.Error = execErr.Error()
	} else {
		switch v := execResult.(type) {
		case nil:
			result.Output = "ok"
		case string:
			result.Output = v
		default:
			b, mErr := json.Marshal(v)
			if mErr != nil {
				result.Output = fmt.Sprintf("%v", v)
			} else {
				result.Output = string(b)
			}
		}
	}

	// 4) Always log tool outcome.
	a.log.Info("tool executed",
		"tool", call.Name,
		"ok", result.Error == "",
		"error", result.Error,
	)

	return result
}