package apps

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"mu/internal/ai"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/data"
	"mu/internal/result"
	"mu/internal/service"
	"mu/internal/thread"
)

// One durable record owns the request, candidate and final app. A process restart
// never changes the ID or resets the paid-attempt budget.
type BuildJob struct {
	Thread     string    `json:"thread,omitempty"`
	Delivered  bool      `json:"delivered,omitempty"`
	ID         string    `json:"id"`
	Account    string    `json:"account"`
	Prompt     string    `json:"prompt"`
	Key        string    `json:"key"`
	State      string    `json:"state"`
	Attempts   int       `json:"attempts"`
	Recoveries int       `json:"recoveries"`
	Created    time.Time `json:"created"`
	Updated    time.Time `json:"updated"`
	Error      string    `json:"error,omitempty"`
	Question   string    `json:"question"`
	Candidate  *written  `json:"candidate,omitempty"`
	App        *App      `json:"app,omitempty"`
}

var buildMu sync.Mutex
var buildJobs = map[string]*BuildJob{}
var buildLoadError error

func storeBuild(j *BuildJob) error {
	j.Updated = time.Now()
	return data.SaveJSON("app-builds/"+j.ID+".json", j)
}
func loadBuilds() {
	keys, err := data.ListKeys("app-builds")
	if err != nil {
		buildLoadError = err
		return
	}
	for _, key := range keys {
		if !strings.HasSuffix(key, ".json") {
			continue
		}
		b, err := data.LoadFile("app-builds/" + key)
		var j BuildJob
		if err == nil {
			err = json.Unmarshal(b, &j)
		}
		if err != nil || j.ID == "" || j.Account == "" || key != j.ID+".json" {
			buildLoadError = fmt.Errorf("could not recover build record %s", key)
			continue
		}
		if j.State == "generating" {
			j.Recoveries++
			j.State = "queued"
			j.Error = "Model call interrupted by restart; recovery uses the remaining attempt budget."
			if err = storeBuild(&j); err != nil {
				buildLoadError = err
				continue
			}
		}
		buildJobs[j.ID] = &j
	}
	if buildLoadError != nil {
		app.Log("apps", "Build recovery paused: %v", buildLoadError)
		return
	}

	go func() {
		for {
			runNextBuild()
			deliverBuilds()
			time.Sleep(time.Second)
		}
	}()
}
func submitBuild(prompt, account, key string, source ...string) (*BuildJob, error) {
	sourceThread := ""
	if len(source) > 0 {
		sourceThread = source[0]
	}
	if sourceThread != "" && thread.Get(account, sourceThread) == nil {
		return nil, fmt.Errorf("conversation not found")
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" || len(prompt) > 16000 || len(key) > 128 {
		return nil, fmt.Errorf("provide an app description up to 16000 bytes and a request key up to 128 bytes")
	}
	if err := auth.CheckCredentialAccess(account); err != nil {
		return nil, err
	}
	if key == "" {
		sum := sha256.Sum256([]byte(sourceThread + "\x00" + prompt))
		key = hex.EncodeToString(sum[:])
	}
	buildMu.Lock()
	defer buildMu.Unlock()
	if buildLoadError != nil {
		return nil, fmt.Errorf("build storage requires operator attention")
	}
	active, total := 0, 0
	for _, j := range buildJobs {
		if j.Account == account && j.Key == key {
			if j.Prompt != prompt || j.Thread != sourceThread {
				return nil, fmt.Errorf("request key already belongs to another build")
			}
			copy := *j
			return &copy, nil
		}
		if j.State != "complete" && j.State != "failed" {
			total++
			if j.Account == account {
				active++
			}
		}
	}
	if active >= 2 || total >= 32 {
		return nil, fmt.Errorf("build queue is full; wait for an existing build")
	}
	j := &BuildJob{Thread: sourceThread, ID: uuid.New().String(), Account: account, Prompt: prompt, Key: key, State: "queued", Created: time.Now(), Question: "Build this app: " + prompt}
	if err := storeBuild(j); err != nil {
		return nil, err
	}
	buildJobs[j.ID] = j
	copy := *j
	return &copy, nil
}
func runNextBuild() {
	buildMu.Lock()
	if buildLoadError != nil {
		buildMu.Unlock()
		return
	}
	var next *BuildJob
	for _, j := range buildJobs {
		if j.State != "complete" && j.State != "failed" && (next == nil || j.Created.Before(next.Created)) {
			next = j
		}
	}
	if next == nil {
		buildMu.Unlock()
		return
	}
	j := *next
	buildMu.Unlock()
	defer func() {
		if v := recover(); v != nil {
			j.State = "failed"
			j.Error = "Build interrupted by an internal error"
			checkpointBuild(&j)
		}
	}()
	if err := auth.CheckCredentialAccess(j.Account); err != nil {
		j.State = "failed"
		j.Error = "Account unavailable"
		checkpointBuild(&j)
		return
	}
	if j.App != nil {
		finishBuild(&j)
		return
	}
	if j.Candidate == nil {
		if j.Attempts >= buildAttempts {
			j.State = "failed"
			j.Error = "Build attempt limit reached. Review before submitting another request."
			checkpointBuild(&j)
			return
		}
		j.Attempts++
		j.State = "generating"
		if !checkpointBuild(&j) {
			return
		}
		raw, err := ai.Ask(&ai.Prompt{System: buildSystem, Question: j.Question, Caller: "app-build", MaxTokens: buildTokens})
		if err != nil {
			j.State = "failed"
			j.Error = ai.FailureMessage(err)
			checkpointBuild(&j)
			return
		}
		candidate := splitHeader(raw, j.Prompt)
		j.Candidate = &candidate
		j.State = "validating"
		if !checkpointBuild(&j) {
			return
		}
	}
	problems := buildProblems(j.Candidate.HTML, j.Account)
	if len(problems) > 0 {
		j.Error = strings.Join(problems, "; ")
		j.Question = "Build this app: " + j.Prompt + "\nCorrect these problems: " + j.Error + "\nPrevious candidate:\n" + j.Candidate.HTML
		j.Candidate = nil
		j.State = "queued"
		checkpointBuild(&j)
		return
	}
	out := j.Candidate
	j.App = &App{ID: j.ID, Slug: "build-" + j.ID, Name: out.Title, Description: j.Prompt, AuthorID: j.Account, Author: AuthorNameFor(j.Account), HTML: out.HTML, Tags: out.Tags, Icon: emojiSVG(out.Emoji), Public: false, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	j.State = "saving"
	if !checkpointBuild(&j) {
		return
	}
	finishBuild(&j)
}
func checkpointBuild(j *BuildJob) bool {
	buildMu.Lock()
	defer buildMu.Unlock()
	if err := storeBuild(j); err != nil {
		buildLoadError = err
		app.Log("apps", "Build checkpoint failed: %v", err)
		return false
	}
	copy := *j
	buildJobs[j.ID] = &copy
	return true
}
func finishBuild(j *BuildJob) {
	mutex.Lock()
	existing := apps[j.App.Slug]
	if existing != nil && (existing.ID != j.ID || existing.AuthorID != j.Account) {
		mutex.Unlock()
		j.State = "failed"
		j.Error = "App identity conflict"
		checkpointBuild(j)
		return
	}
	if existing == nil {
		apps[j.App.Slug] = j.App
	}
	list := make([]*App, 0, len(apps))
	for _, a := range apps {
		list = append(list, a)
	}
	err := data.SaveJSON("apps.json", list)
	mutex.Unlock()
	if err != nil {
		j.Error = "Could not persist app; will retry saving without another model call"
		checkpointBuild(j)
		return
	}
	j.State = "complete"
	j.Question = ""
	j.Candidate = nil
	j.Error = ""
	checkpointBuild(j)
}
func readBuild(id, account string) (*BuildJob, error) {
	buildMu.Lock()
	defer buildMu.Unlock()
	j := buildJobs[id]
	if j == nil || j.Account != account {
		return nil, fmt.Errorf("build not found")
	}
	copy := *j
	return &copy, nil
}

type BuildStatusRequest struct {
	ID string `json:"id" required:"true"`
}
type BuildStatusResponse struct {
	Item       *result.Item `json:"item,omitempty"`
	ID         string       `json:"id"`
	State      string       `json:"state"`
	Attempts   int          `json:"attempts"`
	Recoveries int          `json:"recoveries"`
	Error      string       `json:"error,omitempty"`
	URL        string       `json:"url,omitempty"`
}

func buildStatus(j *BuildJob) BuildStatusResponse {
	r := BuildStatusResponse{ID: j.ID, State: j.State, Attempts: j.Attempts, Recoveries: j.Recoveries, Error: j.Error}
	if j.State == "complete" && j.App != nil {
		r.Item = appResult(j.App)
		r.URL = "/apps/" + j.App.Slug
	}
	return r
}
func (Server) BuildStatus(ctx context.Context, req *BuildStatusRequest, rsp *BuildStatusResponse) error {
	j, err := readBuild(req.ID, service.AccountFrom(ctx))
	if err != nil {
		return err
	}
	*rsp = buildStatus(j)
	return nil
}
func buildPage(w http.ResponseWriter, r *http.Request) {
	_, acc := auth.TrySession(r)
	if acc == nil {
		app.Unauthorized(w, r)
		return
	}
	j, err := readBuild(strings.TrimPrefix(r.URL.Path, "/apps/builds/"), acc.ID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if app.WantsJSON(r) {
		app.RespondJSON(w, buildStatus(j))
		return
	}
	if j.State == "complete" {
		http.Redirect(w, r, "/apps/"+j.App.Slug, http.StatusSeeOther)
		return
	}
	status := html.EscapeString(j.State)
	body := `<h1>Building your app</h1><p>` + status + `</p><p>` + html.EscapeString(j.Error) + `</p><p>You can close this page. The build is saved.</p><a href="/apps/builds/` + j.ID + `">Refresh status</a>`
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(app.RenderHTML("App build", "", body, acc)))
}

// BuildSummary is the owner's progress entry, without prompts or candidates.
type BuildSummary struct {
	ID      string
	Title   string
	State   string
	Updated time.Time
}

// BuildsFor returns unfinished and failed builds for this account only.
func BuildsFor(owner string) []BuildSummary {
	buildMu.Lock()
	defer buildMu.Unlock()
	var out []BuildSummary
	for _, j := range buildJobs {
		if owner == "" || j.Account != owner || j.State == "complete" {
			continue
		}
		title := []rune(strings.Join(strings.Fields(j.Prompt), " "))
		if len(title) > 80 {
			title = append(title[:80], '…')
		}
		if len(title) == 0 {
			title = []rune("App build")
		}
		out = append(out, BuildSummary{ID: j.ID, Title: string(title), State: j.State, Updated: j.Updated})
	}
	return out
}

// Delivery is retried after restarts. A stable Ref deduplicates the message if
// the conversation was saved but the delivery checkpoint failed.
func deliverBuilds() {
	buildMu.Lock()
	var pending []BuildJob
	for _, j := range buildJobs {
		if j.Thread != "" && !j.Delivered && (j.State == "complete" || j.State == "failed") {
			pending = append(pending, *j)
		}
	}
	buildMu.Unlock()
	for _, j := range pending {
		if thread.Get(j.Account, j.Thread) == nil {
			continue
		}
		text := "Your app could not be built: " + j.Error
		var results []result.Item
		if j.State == "complete" && j.App != nil {
			text = "Your app is ready: " + j.App.Name
			results = []result.Item{*appResult(j.App)}
		}
		if thread.Add(thread.Message{Account: j.Account, Thread: j.Thread, Role: thread.RoleAgent, Text: text, Results: results, Ref: "app-build:" + j.ID}) == "" {
			continue
		}
		if err := thread.Flush(); err != nil {
			continue
		}
		j.Delivered = true
		checkpointBuild(&j)
	}
}
