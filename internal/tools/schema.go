package tools

// ParamType is the expected JSON type for a tool parameter.
type ParamType string

const (
	ParamString     ParamType = "string"
	ParamBool       ParamType = "bool"
	ParamInt        ParamType = "int"
	ParamStringList ParamType = "string_list"
	ParamObject     ParamType = "object"
)

// Schema describes required params and their types.
type Schema struct {
	Name     string
	Required []string
	Params   map[string]ParamType
	ReadOnly bool
}

var schemaRegistry = map[string]Schema{}

// RegisterSchema adds a tool schema.
func RegisterSchema(schema Schema) {
	if schema.Name == "" {
		return
	}
	schemaRegistry[schema.Name] = schema
}

// SchemaFor returns the schema for a tool.
func SchemaFor(name string) (Schema, bool) {
	schema, ok := schemaRegistry[name]
	return schema, ok
}

func init() {
	RegisterSchema(Schema{
		Name:     "fs_read",
		Required: []string{"path"},
		Params: map[string]ParamType{
			"path": ParamString,
		},
		ReadOnly: true,
	})
	RegisterSchema(Schema{
		Name:     "fs_write",
		Required: []string{"path", "content"},
		Params: map[string]ParamType{
			"path":    ParamString,
			"content": ParamString,
		},
	})
	RegisterSchema(Schema{
		Name:     "exec_safe",
		Required: []string{"command"},
		Params: map[string]ParamType{
			"command": ParamString,
			"args":    ParamStringList,
		},
	})
	RegisterSchema(Schema{
		Name:     "notify",
		Required: []string{"title", "body"},
		Params: map[string]ParamType{
			"title": ParamString,
			"body":  ParamString,
		},
	})
	RegisterSchema(Schema{
		Name:     "web_fetch",
		Required: []string{"url"},
		Params: map[string]ParamType{
			"url": ParamString,
		},
		ReadOnly: true,
	})
	RegisterSchema(Schema{
		Name:     "plasma_status",
		Required: nil,
		Params:   map[string]ParamType{},
		ReadOnly: true,
	})
}
