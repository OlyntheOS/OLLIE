package tools

import "encoding/json"

// Call is the JSON-RPC payload for tool.call.
type Call struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}
