package api

import (
	"fmt"
)

const (
	TokenHeader = "X-Micro-Token"
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
			Description: "Past messages to use as context; [{'prompt': xxx, 'answer': xxx}]",
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
	Method:      "GET",
	Description: "Latest videos",
	Response: []*Value{
		{
			Type: "JSON",
			Params: []*Param{
				{
					Name:        "channels",
					Value:       "array",
					Description: "Latest videos",
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
			Name:        "query",
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
				{
					Name:        "html",
					Value:       "string",
					Description: "Pre-rendered html string of results",
				},
			},
		},
	},
}, {
	Name:        "Posts",
	Path:        "/posts",
	Method:      "GET",
	Description: "Get all blog posts",
	Response: []*Value{
		{
			Type: "JSON",
			Params: []*Param{
				{
					Name:        "posts",
					Value:       "array",
					Description: "Array of post objects",
				},
			},
		},
	},
}, {
	Name:        "Create Post",
	Path:        "/post",
	Method:      "POST",
	Description: "Create a new blog post",
	Params: []*Param{
		{
			Name:        "title",
			Value:       "string",
			Description: "Post title (optional)",
		},
		{
			Name:        "content",
			Value:       "string",
			Description: "Post content (minimum 50 characters)",
		},
	},
	Response: []*Value{
		{
			Type: "JSON",
			Params: []*Param{
				{
					Name:        "success",
					Value:       "boolean",
					Description: "Whether the post was created successfully",
				},
				{
					Name:        "id",
					Value:       "string",
					Description: "The ID of the created post",
				},
			},
		},
	},
}, {
	Name:        "Get Post",
	Path:        "/post?id={id}",
	Method:      "GET",
	Description: "Get a single blog post by ID",
	Response: []*Value{
		{
			Type: "JSON",
			Params: []*Param{
				{
					Name:        "id",
					Value:       "string",
					Description: "Post ID",
				},
				{
					Name:        "title",
					Value:       "string",
					Description: "Post title",
				},
				{
					Name:        "content",
					Value:       "string",
					Description: "Post content (raw markdown)",
				},
				{
					Name:        "author",
					Value:       "string",
					Description: "Author name",
				},
				{
					Name:        "author_id",
					Value:       "string",
					Description: "Author ID",
				},
				{
					Name:        "created_at",
					Value:       "string",
					Description: "Post creation timestamp",
				},
			},
		},
	},
}, {
	Name:        "Update Post",
	Path:        "/post?id={id}",
	Method:      "PATCH",
	Description: "Update an existing blog post (author only)",
	Params: []*Param{
		{
			Name:        "title",
			Value:       "string",
			Description: "Post title (optional)",
		},
		{
			Name:        "content",
			Value:       "string",
			Description: "Post content (minimum 50 characters)",
		},
	},
	Response: []*Value{
		{
			Type: "JSON",
			Params: []*Param{
				{
					Name:        "success",
					Value:       "boolean",
					Description: "Whether the post was updated successfully",
				},
				{
					Name:        "id",
					Value:       "string",
					Description: "The ID of the updated post",
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
