package chat

import (
	"context"
	"os"
	"strings"
	"text/template"

	"github.com/sashabaranov/go-openai"
)

// There are many different ways to provide the context to the LLM.
// You can pass each context as user message, or the list as one user message,
// or pass it in the system prompt. The system prompt itself also has a big impact
// on how well the LLM handles the context, especially for LLMs with < 7B parameters.
// The prompt engineering is up to you, it's out of scope for the vector database.
var systemPrompt = template.Must(template.New("system_prompt").Parse(`
Answer the question in a concise manner. Use an unbiased and compassionate tone. Do not repeat text. Don't make anything up. If you are not sure about something, just say that you don't know.
{{- /* Stop here if no context is provided. The rest below is for handling contexts. */ -}}
{{- if . -}}
Answer the question keeping the context of the discussion in mind. If the results within the context are not relevant to the question, say I don't know.

Anything between the following 'context' XML blocks can be used as part of the conversation with the user. The bullet points are ordered by time, so the first one is the oldest.

<context>
    {{- if . -}}
    {{- range $context := .}}
    - {{.}}{{end}}
    {{- end}}
</context>
{{- end -}}
`))

func askLLM(ctx context.Context, prompt *Prompt) string {
	openAIClient := openai.NewClient(os.Getenv("OPENAI_API_KEY"))
	sb := &strings.Builder{}
	err := systemPrompt.Execute(sb, prompt.Context)
	if err != nil {
		panic(err)
	}
	messages := []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleSystem,
			Content: sb.String(),
		}, {
			Role:    openai.ChatMessageRoleUser,
			Content: "Question: " + prompt.Question,
		},
	}
	res, err := openAIClient.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model:    openai.GPT4oMini,
		Messages: messages,
	})
	if err != nil {
		panic(err)
	}
	reply := res.Choices[0].Message.Content
	reply = strings.TrimSpace(reply)

	return reply
}
