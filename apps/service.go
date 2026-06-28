package apps

import "context"

// Server is the go-micro service handler for apps.
type Server struct{}

// BuildRequest describes an app to generate.
type BuildRequest struct {
	Prompt     string `json:"prompt" description:"Description of the app to build"`
	AuthorID   string `json:"author_id" description:"Account that will own the app"`
	AuthorName string `json:"author_name" description:"Display name of the author"`
}

// BuildResponse is the saved app's identity and URLs.
type BuildResponse struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
	URL  string `json:"url"`
	Run  string `json:"run"`
}

// Build generates a small app (tracker, checklist or counter) from a natural
// language description, saves it, and returns its details with URLs.
// @example {"prompt": "an expense tracker"}
func (Server) Build(_ context.Context, req *BuildRequest, rsp *BuildResponse) error {
	a, err := BuildMicroApp(req.Prompt, req.AuthorID, req.AuthorName)
	if err != nil {
		return err
	}
	rsp.Name = a.Name
	rsp.Slug = a.Slug
	rsp.URL = "/apps/" + a.Slug
	rsp.Run = "/apps/" + a.Slug + "/run"
	return nil
}
