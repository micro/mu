package ai

import (
	"context"
	gmai "go-micro.dev/v6/ai"
	"go-micro.dev/v6/ai/gemini"
)

// The pinned framework's built-in plan tool omits the schema of its steps.
// Repair that declaration at the Gemini boundary without mutating shared tools.
func init() {
	gmai.Register("gemini", func(opts ...gmai.Option) gmai.Model { return &geminiSchema{gemini.NewProvider(opts...)} })
}

type geminiSchema struct{ gmai.Model }

func (m *geminiSchema) Generate(ctx context.Context, r *gmai.Request, o ...gmai.GenerateOption) (*gmai.Response, error) {
	return m.Model.Generate(ctx, planSchema(r), o...)
}
func (m *geminiSchema) Stream(ctx context.Context, r *gmai.Request, o ...gmai.GenerateOption) (gmai.Stream, error) {
	return m.Model.Stream(ctx, planSchema(r), o...)
}
func planSchema(r *gmai.Request) *gmai.Request {
	if r == nil {
		return r
	}
	cp := *r
	cp.Tools = append([]gmai.Tool(nil), r.Tools...)
	for i, t := range cp.Tools {
		if t.Name != "plan" {
			continue
		}
		steps, ok := t.Properties["steps"].(map[string]any)
		if !ok || steps["type"] != "array" || steps["items"] != nil {
			continue
		}
		props := make(map[string]any, len(t.Properties))
		for k, v := range t.Properties {
			props[k] = v
		}
		array := make(map[string]any, len(steps)+1)
		for k, v := range steps {
			array[k] = v
		}
		array["items"] = map[string]any{"type": "object", "properties": map[string]any{"task": map[string]any{"type": "string"}, "status": map[string]any{"type": "string", "enum": []string{"pending", "in_progress", "done"}}}, "required": []string{"task", "status"}}
		props["steps"] = array
		cp.Tools[i].Properties = props
	}
	return &cp
}
