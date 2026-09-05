package admin

func init() {
	settingGroups = append(settingGroups, settingGroup{
		Name:  "x402 host",
		Does:  "An optional second public hostname for this instance's x402 surface. It serves the same MCP, API and x402 endpoints with its own machine interface.",
		Needs: nil,
		Vars:  []string{"X402_HOST"},
	})
}
