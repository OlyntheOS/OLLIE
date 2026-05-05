package orchestrator

import (
	"fmt"
	"strings"

	"lumin-engine/internal/agent"
	enginecontext "lumin-engine/internal/context"
	"lumin-engine/internal/inference"
)

const DefaultSystemPrompt = "You are OLLIE, the local system operator."

// PromptBuilder builds prompt tokens from conversation history.
type PromptBuilder struct {
	model        *inference.Model
	systemPrompt string
	templateName string
	maxTokens    int
	history      *enginecontext.Manager
}

func NewPromptBuilder(model *inference.Model, systemPrompt, templateName string, maxTokens int) *PromptBuilder {
	if systemPrompt == "" {
		systemPrompt = DefaultSystemPrompt
	}
	var counter func(string) int
	if model != nil {
		counter = func(text string) int {
			tokens, err := model.Tokenize(text, false)
			if err != nil {
				return len(strings.Fields(text))
			}
			return len(tokens)
		}
	}
	return &PromptBuilder{
		model:        model,
		systemPrompt: systemPrompt,
		templateName: templateName,
		maxTokens:    maxTokens,
		history:      enginecontext.NewManager(maxTokens, counter),
	}
}

func (b *PromptBuilder) Add(msg agent.Message) {
	b.history.Add(toTemplateMessage(msg))
}

func (b *PromptBuilder) BuildTokens() []int32 {
	if b.model == nil {
		return nil
	}
	messages := b.history.Messages
	for {
		prompt := b.renderPrompt(messages)
		tokens, err := b.model.Tokenize(prompt, false)
		if err != nil {
			return nil
		}
		if b.maxTokens <= 0 || len(tokens) <= b.maxTokens {
			b.history.Messages = messages
			return tokens
		}
		if !b.history.DropOldestNonSystem() {
			return nil
		}
		messages = b.history.Messages
	}
}

func (b *PromptBuilder) Reset() {
	b.history.Reset()
}

func (b *PromptBuilder) renderPrompt(messages []enginecontext.Message) string {
	return enginecontext.Render(b.templateName, b.systemPrompt, messages)
}

func toTemplateMessage(msg agent.Message) enginecontext.Message {
	role := string(msg.Role)
	content := msg.Content
	if msg.Role == agent.RoleTool && msg.ToolName != "" {
		content = fmt.Sprintf("%s: %s", msg.ToolName, msg.Content)
	}
	return enginecontext.Message{Role: role, Content: content, ToolName: msg.ToolName}
}
