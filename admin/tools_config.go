package admin

func init() {
	settingGroups = append(settingGroups, settingGroup{
		Name:  "Tools host",
		Does:  "An optional second public hostname for this instance's tools. It serves the same MCP, API and x402 endpoints with its own tools-facing home page.",
		Needs: nil,
		Vars:  []string{"TOOLS_HOST", "TOOLS_NAME"},
	})
}
