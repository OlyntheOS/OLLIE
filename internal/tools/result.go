package tools

// Result is the normalized tool result for streaming.
type Result struct {
	Output string `json:"output,omitempty"`
	Error  string `json:"error,omitempty"`
}
