package tools

import (
	"encoding/json"
	"fmt"
	"math"
)

// ValidateArgs enforces required fields and types.
func ValidateArgs(schema Schema, raw json.RawMessage) error {
	if len(schema.Required) == 0 && len(schema.Params) == 0 {
		if len(raw) == 0 || string(raw) == "null" {
			return nil
		}
	}

	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return fmt.Errorf("expected object: %w", err)
	}

	for _, key := range schema.Required {
		if _, ok := payload[key]; !ok {
			return fmt.Errorf("missing required field: %s", key)
		}
	}

	for key, paramType := range schema.Params {
		value, ok := payload[key]
		if !ok {
			continue
		}
		switch paramType {
		case ParamString:
			if _, ok := value.(string); !ok {
				return fmt.Errorf("field %s must be string", key)
			}
		case ParamBool:
			if _, ok := value.(bool); !ok {
				return fmt.Errorf("field %s must be bool", key)
			}
		case ParamInt:
			num, ok := value.(float64)
			if !ok || math.Trunc(num) != num {
				return fmt.Errorf("field %s must be int", key)
			}
		case ParamStringList:
			list, ok := value.([]any)
			if !ok {
				return fmt.Errorf("field %s must be string list", key)
			}
			for _, item := range list {
				if _, ok := item.(string); !ok {
					return fmt.Errorf("field %s must contain strings", key)
				}
			}
		case ParamObject:
			if _, ok := value.(map[string]any); !ok {
				return fmt.Errorf("field %s must be object", key)
			}
		}
	}

	return nil
}
