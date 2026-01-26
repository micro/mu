package flow

// FlowTemplate represents a pre-built flow template
type FlowTemplate struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Source      string `json:"source"`
	Category    string `json:"category"`
}

// Templates are pre-built flows users can install
var Templates = []FlowTemplate{
	{
		Name:        "Morning Briefing",
		Description: "Daily Islamic reminder and tech news at 7am",
		Category:    "Daily",
		Source: `every day at 7am:
    get reminder
    then search news for "technology"
    then email to me with subject "Morning Briefing"`,
	},
	{
		Name:        "Crypto Price Alert",
		Description: "Get BTC price and email it to you",
		Category:    "Finance",
		Source: `get btc price
then email to me with subject "BTC Price Update"`,
	},
	{
		Name:        "Weekly News Digest",
		Description: "AI news summary every Friday",
		Category:    "Weekly",
		Source: `every friday at 5pm:
    search news for "artificial intelligence"
    then summarize
    then email to me with subject "Weekly AI Digest"`,
	},
	{
		Name:        "Daily Reminder",
		Description: "Islamic reminder every morning",
		Category:    "Daily",
		Source: `every day at 8am:
    get reminder
    then email to me with subject "Daily Reminder"`,
	},
	{
		Name:        "Hourly Headlines",
		Description: "News headlines every hour",
		Category:    "Hourly",
		Source: `every hour:
    get headlines
    then email to me with subject "Hourly Headlines"`,
	},
	{
		Name:        "Balance Check",
		Description: "Check your credit balance",
		Category:    "Utility",
		Source: `get balance`,
	},
}

// GetTemplates returns all available templates
func GetTemplates() []FlowTemplate {
	return Templates
}

// GetTemplateByName finds a template by name
func GetTemplateByName(name string) *FlowTemplate {
	for _, t := range Templates {
		if t.Name == name {
			return &t
		}
	}
	return nil
}
