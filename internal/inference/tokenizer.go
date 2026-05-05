package inference

// EncodePrompt is a thin wrapper over the model tokenizer.
func EncodePrompt(model *Model, text string, addSpecial bool) ([]int32, error) {
	return model.Tokenize(text, addSpecial)
}

// DecodeTokens stitches tokens back into text.
func DecodeTokens(model *Model, tokens []int32) (string, error) {
	return model.Detokenize(tokens)
}
