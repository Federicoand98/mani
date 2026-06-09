package tool

type ToolRegistry struct {
	tools map[string]Tool
}

func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		tools: make(map[string]Tool),
	}
}

func (r *ToolRegistry) Register(tool Tool) {
	// TODO
}

func (r *ToolRegistry) Get(name string) (Tool, bool) {
	// TODO
	return nil, false
}

func (r *ToolRegistry) List() []Tool {
	// TODO
	return nil
}
