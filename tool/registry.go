package tool

type ToolRegistry struct {
	tools map[string]Tool
}

func (r *ToolRegistry) Register(tool Tool)
func (r *ToolRegistry) Get(name string) (Tool, bool)
func (r *ToolRegistry) List() []Tool
