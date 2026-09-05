package admin

func init() {
	settingGroups = append(settingGroups, settingGroup{
		Name:  "x402",
		Does:  "An optional public hostname for anonymous machine access to this instance's tools using x402. The normal /tools, MCP and API interfaces remain available on the primary host.",
		Needs: nil,
		Vars:  []string{"X402_HOST"},
	})
}
