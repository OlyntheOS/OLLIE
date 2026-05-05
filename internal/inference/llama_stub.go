//go:build !cgo

package inference

import (
	"context"
	"fmt"
)

type LlamaModel struct{}

type LlamaContext struct{}

func LoadModel(path string, nGPULayers int) (*LlamaModel, error) {
	_, _ = path, nGPULayers
	return nil, fmt.Errorf("cgo is required to load llama.cpp models")
}

func (m *LlamaModel) NewContext(nCtx int) (*LlamaContext, error) {
	_, _ = m, nCtx
	return nil, fmt.Errorf("cgo is required to create a llama.cpp context")
}

func (m *LlamaModel) Tokenize(text string, addSpecial bool) ([]int32, error) {
	_, _ = m, text
	return nil, fmt.Errorf("cgo is required for tokenizer access")
}

func (m *LlamaModel) TokenToText(token int32) (string, error) {
	_, _ = m, token
	return "", fmt.Errorf("cgo is required for tokenizer access")
}

func (m *LlamaModel) Detokenize(tokens []int32) (string, error) {
	_, _ = m, tokens
	return "", fmt.Errorf("cgo is required for tokenizer access")
}

func (ctx *LlamaContext) Eval(tokens []int32, pastTokens int) error {
	_, _ = ctx, tokens
	return fmt.Errorf("cgo is required to decode tokens")
}

func (m *LlamaModel) Generate(ctx *LlamaContext, prompt string, maxTokens int) (string, error) {
	_, _, _ = m, ctx, prompt
	return "", fmt.Errorf("cgo is required to generate text")
}

func (m *LlamaModel) GenerateStream(ctx context.Context, lctx *LlamaContext, promptTokens []int32, maxTokens int, onToken func(token int32) error) error {
	_, _, _, _, _ = ctx, lctx, promptTokens, maxTokens, onToken
	return fmt.Errorf("cgo is required to stream generate text")
}

func (ctx *LlamaContext) Free() {}

func (m *LlamaModel) Free() {}
