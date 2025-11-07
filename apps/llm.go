package apps

import (
	"fmt"
	
	"mu/chat"
)

// LLM prompt template for generating mini apps
const appGenerationPrompt = `You are an expert web developer. Generate a complete, self-contained single-page mini app based on the following request:

Request: %s

Generate valid HTML, CSS, and JavaScript code that creates a fully functional mini app. Follow these guidelines:

1. HTML: Create semantic HTML structure with proper tags
2. CSS: Include inline styles or a style tag with clean, modern styling
3. JavaScript: Include all necessary JavaScript functionality inline or in a script tag
4. Make it responsive and mobile-friendly
5. Use plain JavaScript (no external libraries unless absolutely necessary)
6. Ensure all code is production-ready and functional
7. Keep it simple but professional-looking

Return your response in the following JSON format:
{
  "html": "<!-- complete HTML code here -->",
  "css": "/* complete CSS code here */",
  "js": "// complete JavaScript code here",
  "name": "App Name",
  "description": "Brief description of the app"
}

Return ONLY the JSON, no additional text or explanation.`

// GenerateApp uses LLM to generate app code from a prompt
func GenerateApp(prompt, model string) (map[string]string, error) {
	// Use the chat LLM functionality to generate the app
	fullPrompt := fmt.Sprintf(appGenerationPrompt, prompt)
	
	// For now, return a simple template structure
	// In production, this would call the LLM
	result := map[string]string{
		"html": `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Mini App</title>
</head>
<body>
    <div id="app">
        <h1>Mini App</h1>
        <p>App generated from prompt: ` + prompt + `</p>
    </div>
</body>
</html>`,
		"css": `body {
    font-family: Arial, sans-serif;
    margin: 0;
    padding: 20px;
    background: #f5f5f5;
}

#app {
    max-width: 800px;
    margin: 0 auto;
    background: white;
    padding: 20px;
    border-radius: 8px;
    box-shadow: 0 2px 4px rgba(0,0,0,0.1);
}`,
		"js": `console.log("App initialized");`,
		"name": "Generated App",
		"description": "A mini app generated from your prompt",
	}
	
	// This is where we'd actually call the LLM
	_ = fullPrompt
	_ = chat.DefaultModel
	_ = model
	
	return result, nil
}
