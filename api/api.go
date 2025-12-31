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
	Name:        "News Search",
	Path:        "/news",
	Method:      "POST",
	Description: "Search for news articles",
	Params: []*Param{
		{
			Name:        "query",
			Value:       "string",
			Description: "News search query",
		},
	},
	Response: []*Value{
		{
			Type: "JSON",
			Params: []*Param{
				{
					Name:        "query",
					Value:       "string",
					Description: "The search query",
				},
				{
					Name:        "results",
					Value:       "array",
					Description: "Array of news article objects with id, title, description, url, category, image, published",
				},
				{
					Name:        "count",
					Value:       "number",
					Description: "Number of results returned",
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
	Name:        "Blog",
	Path:        "/blog",
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
	Name:        "Video Search",
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
}}

// Additional endpoints for tokens and mail
func init() {
	// Token management endpoints
	Endpoints = append(Endpoints, &Endpoint{
		Name:        "List Tokens",
		Path:        "/token",
		Method:      "GET",
		Description: "List all Personal Access Tokens for authenticated user",
		Response: []*Value{
			{
				Type: "JSON",
				Params: []*Param{
					{
						Name:        "tokens",
						Value:       "array",
						Description: "Array of token objects with id, name, created, last_used, expires_at, permissions",
					},
				},
			},
		},
	})

	Endpoints = append(Endpoints, &Endpoint{
		Name:        "Create Token",
		Path:        "/token",
		Method:      "POST",
		Description: "Create a new Personal Access Token for API automation",
		Params: []*Param{
			{
				Name:        "name",
				Value:       "string",
				Description: "Token name/description",
			},
			{
				Name:        "permissions",
				Value:       "array",
				Description: "Permissions array (e.g., ['read', 'write']). Default: ['read', 'write']",
			},
			{
				Name:        "expires_in",
				Value:       "number",
				Description: "Expiration in days (0 = never expires). Default: 0",
			},
		},
		Response: []*Value{
			{
				Type: "JSON",
				Params: []*Param{
					{
						Name:        "success",
						Value:       "boolean",
						Description: "Whether token was created",
					},
					{
						Name:        "id",
						Value:       "string",
						Description: "Token ID",
					},
					{
						Name:        "token",
						Value:       "string",
						Description: "The actual token (SAVE IT - shown only once!)",
					},
					{
						Name:        "name",
						Value:       "string",
						Description: "Token name",
					},
					{
						Name:        "created",
						Value:       "string",
						Description: "Creation timestamp",
					},
					{
						Name:        "expires_at",
						Value:       "string",
						Description: "Expiration timestamp (if set)",
					},
					{
						Name:        "permissions",
						Value:       "array",
						Description: "Token permissions",
					},
				},
			},
		},
	})

	Endpoints = append(Endpoints, &Endpoint{
		Name:        "Delete Token",
		Path:        "/token?id={id}",
		Method:      "DELETE",
		Description: "Delete a Personal Access Token",
		Response: []*Value{
			{
				Type: "JSON",
				Params: []*Param{
					{
						Name:        "success",
						Value:       "boolean",
						Description: "Whether token was deleted",
					},
					{
						Name:        "message",
						Value:       "string",
						Description: "Success message",
					},
				},
			},
		},
	})

	Endpoints = append(Endpoints, &Endpoint{
		Name:        "Delete Post",
		Path:        "/post?id={id}",
		Method:      "DELETE",
		Description: "Delete a blog post (author only)",
		Response: []*Value{
			{
				Type: "JSON",
				Params: []*Param{
					{
						Name:        "success",
						Value:       "boolean",
						Description: "Whether the post was deleted",
					},
				},
			},
		},
	})

	Endpoints = append(Endpoints, &Endpoint{
		Name:        "User Profile",
		Path:        "/@{username}",
		Method:      "GET",
		Description: "Get user profile and their posts",
		Response: []*Value{
			{
				Type: "HTML",
				Params: []*Param{
					{
						Name:        "html",
						Value:       "string",
						Description: "Rendered user profile page",
					},
				},
			},
		},
	})

	Endpoints = append(Endpoints, &Endpoint{
		Name:        "Update User Status",
		Path:        "/@{username}",
		Method:      "POST",
		Description: "Update user status message (own profile only)",
		Params: []*Param{
			{
				Name:        "status",
				Value:       "string",
				Description: "Status message (max 100 characters)",
			},
		},
		Response: []*Value{
			{
				Type: "Redirect",
				Params: []*Param{
					{
						Name:        "location",
						Value:       "string",
						Description: "Redirects to user profile",
					},
				},
			},
		},
	})

	Endpoints = append(Endpoints, &Endpoint{
		Name:        "Add Comment",
		Path:        "/post/{id}/comment",
		Method:      "POST",
		Description: "Add a comment to a blog post",
		Params: []*Param{
			{
				Name:        "content",
				Value:       "string",
				Description: "Comment content (minimum 10 characters)",
			},
		},
		Response: []*Value{
			{
				Type: "Redirect",
				Params: []*Param{
					{
						Name:        "location",
						Value:       "string",
						Description: "Redirects back to the post",
					},
				},
			},
		},
	})

	Endpoints = append(Endpoints, &Endpoint{
		Name:        "Search Data",
		Path:        "/search",
		Method:      "GET",
		Description: "Search across all indexed content (posts, news, videos)",
		Params: []*Param{
			{
				Name:        "q",
				Value:       "string",
				Description: "Search query",
			},
		},
		Response: []*Value{
			{
				Type: "JSON",
				Params: []*Param{
					{
						Name:        "results",
						Value:       "array",
						Description: "Search results with type, id, title, content, score",
					},
				},
			},
		},
	})
}

// Register an endpoint
func Register(ep *Endpoint) {
	Endpoints = append(Endpoints, ep)
}

// Markdown API document
func Markdown() string {
	var data string

	// Add authentication section
	data += "# API Documentation\n\n"
	data += "## Authentication\n\n"
	data += "All authenticated endpoints require either:\n\n"
	data += "1. **Session Cookie** - Obtained via web login\n"
	data += "2. **Personal Access Token (PAT)** - For API automation\n\n"
	data += "### Using PAT Tokens\n\n"
	data += "Include your PAT in the request using one of these methods:\n\n"
	data += "- **Authorization header**: `Authorization: Bearer YOUR_TOKEN`\n"
	data += "- **X-Micro-Token header**: `X-Micro-Token: YOUR_TOKEN`\n\n"
	data += "Example:\n"
	data += "```bash\n"
	data += "curl -H \"Authorization: Bearer YOUR_TOKEN\" \\\n"
	data += "     -H \"Content-Type: application/json\" \\\n"
	data += "     https://example.com/api/endpoint\n"
	data += "```\n\n"
	data += "### Creating a PAT Token\n\n"
	data += "1. Log in to your account via the web interface\n"
	data += "2. Navigate to `/token` endpoint\n"
	data += "3. Create a new token with desired permissions\n"
	data += "4. **Save the token immediately** - it's only shown once!\n\n"
	data += "---\n\n"
	data += "## Endpoints\n\n"

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
