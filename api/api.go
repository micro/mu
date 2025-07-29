package api

import (
	"fmt"
)

type Endpoint struct {
	Name        string
	Path        string
	Method      string
	Params      []*Param
	Response    []*Value
	Description string
}

type Param struct {
	Name        string
	Value       string
	Description string
}

type Value struct {
	Type   string
	Params []*Param
}

var Endpoints = []*Endpoint{{
	Name:        "Chat",
	Path:        "/chat",
	Method:      "POST",
	Description: "Chat with AI",
	Params: []*Param{
		{
			Name:        "context",
			Value:       "array",
			Description: "Messages to use as context",
		},
		{
			Name:        "prompt",
			Value:       "string",
			Description: "Prompt to send the AI",
		},
	},
	Response: []*Value{
		{
			Type: "JSON",
			Params: []*Param{
				{
					Name:        "context",
					Value:       "array",
					Description: "Messages used as context",
				},
				{
					Name:        "prompt",
					Value:       "string",
					Description: "Prompt you sent to the AI",
				},
				{
					Name:        "answer",
					Value:       "string",
					Description: "The response from the AI",
				},
			},
		},
	},
}, {
	Name:        "News",
	Path:        "/news",
	Method:      "GET",
	Description: "Read the news",
	Response: []*Value{
		{
			Type: "JSON",
			Params: []*Param{
				{
					Name:        "feed",
					Value:       "array",
					Description: "The news feed",
				},
			},
		},
	},
}, {
	Name:        "Video",
	Path:        "/video",
	Method:      "POST",
	Description: "Search for videos",
	Params: []*Param{
		{
			Name:        "q",
			Value:       "string",
			Description: "Video search query",
		},
	},
	Response: []*Value{
		{
			Type: "JSON",
			Params: []*Param{
				{
					Name:        "results",
					Value:       "array",
					Description: "Video search results",
				},
			},
		},
	},
}}

// Register an endpoint
func Register(ep *Endpoint) {
	Endpoints = append(Endpoints, ep)
}

// Markdown API document
func Markdown() string {
	var data string

	for _, endpoint := range Endpoints {
		data += "## " + endpoint.Name
		data += fmt.Sprintln()
		data += fmt.Sprintln()
		data += fmt.Sprintln(endpoint.Description)
		data += fmt.Sprintln()
		data += fmt.Sprintf("```%s %s```", endpoint.Method, endpoint.Path)
		data += fmt.Sprintln()

		data += fmt.Sprintln("#### Headers")
		data += fmt.Sprintln()
		data += fmt.Sprintln("Content-Type: application/json")
		data += fmt.Sprintln()
		data += fmt.Sprintln()

		if endpoint.Params != nil {
			data += fmt.Sprintln("#### Request")
			data += fmt.Sprintln()
			data += fmt.Sprintln("Format: JSON")
			data += fmt.Sprintln()
			data += "| Field | Type | Description |"
			data += fmt.Sprintln()
			data += "| ----- | ---- | ----------- |"
			data += fmt.Sprintln()

			for _, param := range endpoint.Params {
				data += fmt.Sprintf("|	%s	|	%s	|	%s	|", param.Name, param.Value, param.Description)
				data += fmt.Sprintln()
			}
			data += fmt.Sprintln()
			data += fmt.Sprintln("\\")
			data += fmt.Sprintln()
		}

		if endpoint.Response != nil {
			data += fmt.Sprintln("#### Response")
			data += fmt.Sprintln()
			for _, resp := range endpoint.Response {
				data += fmt.Sprintln()
				data += fmt.Sprintf("Format: %s", resp.Type)
				data += fmt.Sprintln()
				data += "| Field | Type | Description |"
				data += fmt.Sprintln()
				data += "| ----- | ---- | ----------- |"
				data += fmt.Sprintln()
				for _, param := range resp.Params {
					data += fmt.Sprintf("|	%s	|	%s	|	%s	|", param.Name, param.Value, param.Description)
					data += fmt.Sprintln()
				}
			}

			data += fmt.Sprintln()
			data += fmt.Sprintln("\\")
			data += fmt.Sprintln()
		}

		data += fmt.Sprintln()
		data += fmt.Sprintln("\\")
		data += fmt.Sprintln("\\")
		data += fmt.Sprintln()
	}

	return data
}
