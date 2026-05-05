package orchestrator

import (
	"context"

	"lumin-engine/internal/inference"
)

const defaultMaxGenerateTokens = 256

// ModelStreamer adapts inference.Model to the agent stream interface.
type ModelStreamer struct {
	model     *inference.Model
	maxTokens int
}

func NewModelStreamer(model *inference.Model, maxTokens int) *ModelStreamer {
	if maxTokens <= 0 {
		maxTokens = defaultMaxGenerateTokens
	}
	return &ModelStreamer{model: model, maxTokens: maxTokens}
}

func (m *ModelStreamer) Generate(ctx context.Context, tokens []int32, out chan<- string) error {
	defer close(out)
	return m.model.GenerateStream(ctx, tokens, m.maxTokens, out)
}

func (m *ModelStreamer) Tokenize(text string) []int32 {
	okens, err := m.model.Tokenize(text, false)
	if err != nil {
		return nil
	}
	return tokens
}

func (m *ModelStreamer) Detokenize(token int32) string {
	text, err := m.model.Detokenize([]int32{token})
	if err != nil {
		return ""
	}
	return text
}
