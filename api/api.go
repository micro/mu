package api

import (
	"fmt"

	"net/http"
)

type Endpoint struct {
	Name        string
	Path        string
	Params      []*Param
	Response    []*Value
	Description string
	Handler     http.Handler
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

var Endpoints = []*Endpoint{}

// Register an endpoint
func Register(ep *Endpoint) {
	Endpoints = append(Endpoints, ep)
}

// Markdown API document
func Markdown() string {
	var data string

	data += "# Endpoints"
	data += fmt.Sprintln()
	data += fmt.Sprintln("A list of API endpoints")
	data += fmt.Sprintln()

	for _, endpoint := range Endpoints {
		data += fmt.Sprintln()
		data += "## " + endpoint.Name
		data += fmt.Sprintln()
		data += fmt.Sprintln("___")
		data += fmt.Sprintln("\\")
		data += fmt.Sprintln()
		data += fmt.Sprintln()
		data += fmt.Sprintln(endpoint.Description)
		data += fmt.Sprintln()
		data += fmt.Sprintf("URL: [`%s`](%s)", endpoint.Path, endpoint.Path)
		data += fmt.Sprintln()

		if endpoint.Params != nil {
			data += fmt.Sprintln("#### Request")
			data += fmt.Sprintln()
			data += fmt.Sprintln("Format `JSON`")
			data += fmt.Sprintln()
			for _, param := range endpoint.Params {
				data += fmt.Sprintf("- `%s` - **`%s`** - %s", param.Name, param.Value, param.Description)
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
				data += fmt.Sprintf("Format `%s`", resp.Type)
				data += fmt.Sprintln()
				for _, param := range resp.Params {
					data += fmt.Sprintf("- `%s` - **`%s`** - %s", param.Name, param.Value, param.Description)
					data += fmt.Sprintln()
				}
			}

			data += fmt.Sprintln()
			data += fmt.Sprintln("\\")
			data += fmt.Sprintln()
		}

		data += fmt.Sprintln()
		data += fmt.Sprintln()
	}

	return data
}

// Serve the /api/ handler
func Serve() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, ep := range Endpoints {
			if r.URL.Path != ep.Path {
				continue
			}

			ep.Handler.ServeHTTP(w, r)
			return
		}

		http.Error(w, "not found", 404)
	})
}
