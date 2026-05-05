package agent

import (
	"encoding/json"
	"strings"
)

// --- Parsed tool call ---

// ParsedToolCall is the tool call pulled out of model output.
// The model emits JSON inside <tool_call>...</tool_call> tags.
type ParsedToolCall struct {
	// Tool identifier; must match the executor registry.
	Name string `json:"name"`

	// Args are stringified here; tools handle type parsing.
	Arguments map[string]string `json:"arguments"`
}

// --- Parse result ---

// ParseKind tells the loop what was detected.
type ParseKind int

const (
	// ParseKindText is normal text; forward to UI.
	ParseKindText ParseKind = iota

	// ParseKindToolCall is a complete tool call payload.
	ParseKindToolCall
)

// ParseResult is returned by Feed for each token.
type ParseResult struct {
	Kind     ParseKind
	Text     string          // for ParseKindText
	ToolCall *ParsedToolCall // for ParseKindToolCall
}

// --- Parser states ---

type parserState int

const (
	// stateText reads normal assistant text.
	stateText parserState = iota

	// stateMaybeOpen checks whether we just saw <tool_call>.
	stateMaybeOpen

	// stateInsideCall buffers JSON inside <tool_call>...</tool_call>.
	stateInsideCall

	// stateMaybeClose checks for the closing tag.
	stateMaybeClose
)

const (
	openTag  = "<tool_call>"
	closeTag = "</tool_call>"
)

// --- StreamParser ---

// StreamParser watches the token stream and extracts tool-call blocks.
// It forwards plain text immediately and buffers JSON inside the tags.
//
// Example stream:
// "I'll change your wallpaper.\n<tool_call>\n{\"name\":\"plasma_set_theme\",\"arguments\":{\"theme\":\"BreezeDark\"}}\n</tool_call>"
type StreamParser struct {
	state   parserState
	tagBuf  strings.Builder // tag matching buffer
	callBuf strings.Builder // JSON buffer
}

// NewStreamParser returns a fresh parser; use one per iteration.
func NewStreamParser() *StreamParser {
	return &StreamParser{}
}

// Reset clears state between iterations.
func (p *StreamParser) Reset() {
	p.state = stateText
	p.tagBuf.Reset()
	p.callBuf.Reset()
}

// Feed processes a token and returns a ParseResult.
// Hot path; keep it cheap.
func (p *StreamParser) Feed(token string) ParseResult {
	// Scan char-by-char to catch tag boundaries across tokens.
	for _, ch := range token {
		c := string(ch)
		result := p.feedChar(c)
		if result != nil {
			return *result
		}
	}
	// Fast path: return token as-is when in text mode.
	if p.state == stateText {
		return ParseResult{Kind: ParseKindText, Text: token}
	}
	// Mid-tag or mid-call; nothing to emit yet.
	return ParseResult{Kind: ParseKindText, Text: ""}
}

// feedChar returns a result when a boundary is hit, nil otherwise.
func (p *StreamParser) feedChar(c string) *ParseResult {
	switch p.state {

	// --- Normal text output ---
	case stateText:
		if c == "<" {
			// Possible tag start; switch to probe mode.
			p.state = stateMaybeOpen
			p.tagBuf.Reset()
			p.tagBuf.WriteString(c)
			return nil
		}
		// Plain text, emit immediately.
		return &ParseResult{Kind: ParseKindText, Text: c}

	// --- Probing for <tool_call> ---
	case stateMaybeOpen:
		p.tagBuf.WriteString(c)
		accumulated := p.tagBuf.String()

		if accumulated == openTag {
			// Confirmed; enter tool-call buffer.
			p.state = stateInsideCall
			p.callBuf.Reset()
			p.tagBuf.Reset()
			return nil
		}

		if !strings.HasPrefix(openTag, accumulated) {
			// Not a tool_call tag; flush and return to text mode.
			flushed := accumulated
			p.state = stateText
			p.tagBuf.Reset()
			return &ParseResult{Kind: ParseKindText, Text: flushed}
		}

		// Still a prefix; keep probing.
		return nil

	// --- Accumulating JSON inside <tool_call>...</tool_call> ---
	case stateInsideCall:
		if c == "<" {
			// Possible closing tag.
			p.state = stateMaybeClose
			p.tagBuf.Reset()
			p.tagBuf.WriteString(c)
			return nil
		}
		p.callBuf.WriteString(c)
		return nil

	// --- Probing for </tool_call> ---
	case stateMaybeClose:
		p.tagBuf.WriteString(c)
		accumulated := p.tagBuf.String()

		if accumulated == closeTag {
			// Closing tag found; parse buffered JSON.
			p.state = stateText
			raw := strings.TrimSpace(p.callBuf.String())
			p.callBuf.Reset()
			p.tagBuf.Reset()

			call, err := parseToolCallJSON(raw)
			if err != nil {
				// Malformed JSON; emit as text so the model can retry.
				return &ParseResult{
					Kind: ParseKindText,
					Text: openTag + raw + closeTag, // show raw in UI for debugging
				}
			}
			return &ParseResult{Kind: ParseKindToolCall, ToolCall: call}
		}

		if !strings.HasPrefix(closeTag, accumulated) {
			// Not a close tag; keep content in call buffer.
			p.callBuf.WriteString(accumulated)
			p.state = stateInsideCall
			p.tagBuf.Reset()
			return nil
		}

		// Still a prefix; keep probing.
		return nil
	}

	return nil
}

// --- JSON parsing ---

// rawToolCall matches the JSON emitted inside <tool_call> tags.
// Supports a couple of common model variants.
type rawToolCall struct {
	Name      string `json:"name"`

	// Format 1 (preferred): {"name":"fs_read","arguments":{"path":"/tmp"}}
	Arguments map[string]json.RawMessage `json:"arguments"`

	// Format 2 (some models): {"name":"fs_read","parameters":{"path":"/tmp"}}
	Parameters map[string]json.RawMessage `json:"parameters"`
}

// parseToolCallJSON parses JSON between <tool_call> tags.
// Args are stringified here; tools handle types.
func parseToolCallJSON(raw string) (*ParsedToolCall, error) {
	var rtc rawToolCall
	if err := json.Unmarshal([]byte(raw), &rtc); err != nil {
		return nil, err
	}

	// Prefer arguments, fall back to parameters.
	rawArgs := rtc.Arguments
	if rawArgs == nil {
		rawArgs = rtc.Parameters
	}

	args := make(map[string]string, len(rawArgs))
	for k, v := range rawArgs {
		// Unquote strings; leave other JSON as raw text.
		var s string
		if err := json.Unmarshal(v, &s); err == nil {
			args[k] = s
		} else {
			args[k] = string(v) // number, bool, or nested object
		}
	}

	return &ParsedToolCall{
		Name:      rtc.Name,
		Arguments: args,
	}, nil
}

// --- Qwen3 thinking block stripper ---

// Qwen3 models may emit <think>...</think> before the real reply.
// This is internal reasoning and should never surface to the user.
// StripThinkingBlock removes a leading <think>...</think> block on full text.
func StripThinkingBlock(response string) string {
	const thinkOpen  = "<think>"
	const thinkClose = "</think>"

	start := strings.Index(response, thinkOpen)
	if start == -1 {
		return response
	}
	end := strings.Index(response, thinkClose)
	if end == -1 {
		return response
	}
	return strings.TrimSpace(response[end+len(thinkClose):])
}