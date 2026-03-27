package work

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"mu/internal/data"

	"github.com/google/uuid"
)

// Post kinds
const (
	KindTask = "task" // Looking for someone to build something
	KindShow = "show" // Sharing work you've done
)

// Task states (only relevant for kind=task)
const (
	StatusOpen      = "open"      // Accepting claims
	StatusClaimed   = "claimed"   // Someone is working on it
	StatusDelivered = "delivered" // Work submitted, awaiting acceptance
	StatusCompleted = "completed" // Accepted and paid
	StatusCancelled = "cancelled" // Cancelled by poster
)

// Post represents a work post — either a task (request) or show (share)
type Post struct {
	ID          string    `json:"id"`
	Kind        string    `json:"kind"`        // "task" or "show"
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Link        string    `json:"link"`        // URL, app slug, or any external link
	Cost        int       `json:"cost"`        // Max spend budget (credits)
	Spent       int       `json:"spent"`       // Credits consumed so far
	AuthorID    string    `json:"author_id"`
	Author      string    `json:"author"`      // Display name
	WorkerID    string    `json:"worker_id"`   // Who claimed a task
	Worker      string    `json:"worker"`      // Worker display name
	Status      string    `json:"status"`      // Task status (open/claimed/delivered/completed/cancelled)
	Delivery    string    `json:"delivery"`    // Deliverable for tasks
	Tags        string    `json:"tags"`        // Comma-separated
	Tips        int       `json:"tips"`        // Total tips received (show)
	Log         []LogEntry `json:"log"`        // Agent work log
	Feedback    []Comment `json:"feedback"`    // Comments/feedback
	CreatedAt   time.Time `json:"created_at"`
	ClaimedAt   time.Time `json:"claimed_at,omitempty"`
	DeliveredAt time.Time `json:"delivered_at,omitempty"`
	CompletedAt time.Time `json:"completed_at,omitempty"`
}

// LogEntry records a step in the agent's work
type LogEntry struct {
	Step      string    `json:"step"`       // "build", "verify", "fix", "complete", "error", "budget"
	Message   string    `json:"message"`    // What happened
	Credits   int       `json:"credits"`    // Credits consumed in this step
	CreatedAt time.Time `json:"created_at"`
}

// Comment is a piece of feedback on a work post
type Comment struct {
	ID        string    `json:"id"`
	AuthorID  string    `json:"author_id"`
	Author    string    `json:"author"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"created_at"`
}

// BuildApp is wired by main.go to call the apps builder.
// Takes (prompt, authorID, authorName) and returns (appSlug, appName, error).
var BuildApp func(prompt, authorID, authorName string) (string, string, error)

// VerifyApp is wired by main.go to check if an app works.
// Takes (appSlug) and returns (issues string, ok bool).
var VerifyApp func(appSlug string) (string, bool)

// FixApp is wired by main.go to fix issues with an app.
// Takes (appSlug, issues) and returns (error).
var FixApp func(appSlug, issues string) error

// ConsumeCredits is wired by main.go to charge credits.
// Takes (userID, amount) and returns (error).
var ConsumeCredits func(userID string, amount int) error

// Notify is wired by main.go to send notifications (e.g. internal mail).
// Takes (toUserID, subject, body).
var Notify func(toUserID, subject, body string)

var (
	mutex sync.RWMutex
	posts = map[string]*Post{}
)

func init() {
	b, _ := data.LoadFile("work.json")
	json.Unmarshal(b, &posts)
}

// Load initializes the work building block
func Load() {
	if len(posts) == 0 {
		seedPosts()
	}
	data.RegisterDeleter("work", DeletePost)

	// Resume any in-progress agent tasks after startup
	// (delayed to allow callbacks to be wired first)
	go func() {
		time.Sleep(2 * time.Second)
		ResumeAgentWork()
	}()
}

func save() {
	data.SaveJSON("work.json", posts)
}

// CreatePost creates a new work post (task or show)
func CreatePost(authorID, author, kind, title, description, link, tags string, cost int) (*Post, error) {
	if title == "" {
		return nil, errors.New("title is required")
	}
	if description == "" {
		return nil, errors.New("description is required")
	}
	if kind != KindTask && kind != KindShow {
		return nil, errors.New("kind must be task or show")
	}
	if kind == KindTask {
		if cost < 1 {
			return nil, errors.New("cost must be at least 1 credit")
		}
		if cost > 50000 {
			return nil, errors.New("maximum cost is 50,000 credits")
		}
	}

	post := &Post{
		ID:          uuid.New().String(),
		Kind:        kind,
		Title:       title,
		Description: description,
		Link:        link,
		Cost:        cost,
		AuthorID:    authorID,
		Author:      author,
		Status:      StatusOpen,
		Tags:        tags,
		Feedback:    []Comment{},
		CreatedAt:   time.Now(),
	}

	if kind == KindShow {
		post.Status = "" // shows don't have task status
	}

	mutex.Lock()
	posts[post.ID] = post
	save()
	mutex.Unlock()

	return post, nil
}

// AddFeedback adds a comment to a work post
func AddFeedback(postID, authorID, author, text string) error {
	if text == "" {
		return errors.New("feedback text is required")
	}

	mutex.Lock()
	defer mutex.Unlock()

	post, exists := posts[postID]
	if !exists {
		return errors.New("post not found")
	}

	comment := Comment{
		ID:        uuid.New().String(),
		AuthorID:  authorID,
		Author:    author,
		Text:      text,
		CreatedAt: time.Now(),
	}
	post.Feedback = append(post.Feedback, comment)

	save()
	return nil
}

// TipPost records a tip on a show post
func TipPost(postID string, amount int) {
	mutex.Lock()
	defer mutex.Unlock()

	post, exists := posts[postID]
	if !exists {
		return
	}
	post.Tips += amount
	save()
}

// ClaimTask marks a task as claimed by a worker
func ClaimTask(postID, workerID, worker string) error {
	mutex.Lock()
	defer mutex.Unlock()

	post, exists := posts[postID]
	if !exists {
		return errors.New("post not found")
	}
	if post.Kind != KindTask {
		return errors.New("only tasks can be claimed")
	}
	if post.Status != StatusOpen {
		return errors.New("task is not open")
	}
	if post.AuthorID == workerID {
		return errors.New("cannot claim your own task")
	}

	post.WorkerID = workerID
	post.Worker = worker
	post.Status = StatusClaimed
	post.ClaimedAt = time.Now()

	save()
	return nil
}

// DeliverTask submits work for review
func DeliverTask(postID, workerID, delivery string) error {
	if delivery == "" {
		return errors.New("delivery is required")
	}

	mutex.Lock()
	defer mutex.Unlock()

	post, exists := posts[postID]
	if !exists {
		return errors.New("post not found")
	}
	if post.Status != StatusClaimed {
		return errors.New("task is not claimed")
	}
	if post.WorkerID != workerID {
		return errors.New("only the assigned worker can deliver")
	}

	post.Delivery = delivery
	post.Status = StatusDelivered
	post.DeliveredAt = time.Now()

	save()
	return nil
}

// AcceptTask accepts delivery
func AcceptTask(postID, authorID string) error {
	mutex.Lock()
	defer mutex.Unlock()

	post, exists := posts[postID]
	if !exists {
		return errors.New("post not found")
	}
	if post.Status != StatusDelivered {
		return errors.New("task has not been delivered")
	}
	if post.AuthorID != authorID {
		return errors.New("only the poster can accept delivery")
	}

	post.Status = StatusCompleted
	post.CompletedAt = time.Now()

	save()
	return nil
}

// CancelTask cancels an open or claimed task
func CancelTask(postID, authorID string) error {
	mutex.Lock()
	defer mutex.Unlock()

	post, exists := posts[postID]
	if !exists {
		return errors.New("post not found")
	}
	if post.AuthorID != authorID {
		return errors.New("only the poster can cancel")
	}
	if post.Status == StatusCompleted {
		return errors.New("completed tasks cannot be cancelled")
	}

	post.Status = StatusCancelled
	save()
	return nil
}

// ReleaseTask releases a claimed task back to open
func ReleaseTask(postID, authorID string) error {
	mutex.Lock()
	defer mutex.Unlock()

	post, exists := posts[postID]
	if !exists {
		return errors.New("post not found")
	}
	if post.AuthorID != authorID && post.WorkerID != authorID {
		return errors.New("only the poster or worker can release")
	}
	if post.Status != StatusClaimed {
		return errors.New("task is not claimed")
	}

	post.WorkerID = ""
	post.Worker = ""
	post.Status = StatusOpen
	post.ClaimedAt = time.Time{}

	save()
	return nil
}

// addLog appends a log entry to a post and persists it.
func addLog(post *Post, step, message string, credits int) {
	mutex.Lock()
	post.Log = append(post.Log, LogEntry{
		Step:      step,
		Message:   message,
		Credits:   credits,
		CreatedAt: time.Now(),
	})
	post.Spent += credits
	save()
	mutex.Unlock()
}

// spendCredits charges credits against a task's budget.
// Returns false if the budget would be exceeded.
func spendCredits(post *Post, authorID string, amount int) bool {
	if post.Cost > 0 && post.Spent+amount > post.Cost {
		return false
	}
	if ConsumeCredits != nil {
		if err := ConsumeCredits(authorID, amount); err != nil {
			return false
		}
	}
	return true
}

const (
	maxAgentIterations = 5
	creditPerStep      = 3 // credits per AI call
)

// AssignToAgent assigns an open task to the AI agent.
// It claims the task as "agent", runs the app builder in a goroutine,
// and posts the delivery when complete. The poster reviews the result.
func AssignToAgent(postID, authorID string) error {
	mutex.Lock()
	post, exists := posts[postID]
	if !exists {
		mutex.Unlock()
		return errors.New("post not found")
	}
	if post.Kind != KindTask {
		mutex.Unlock()
		return errors.New("only tasks can be assigned")
	}
	if post.Status != StatusOpen {
		mutex.Unlock()
		return errors.New("task is not open")
	}
	if post.AuthorID != authorID {
		mutex.Unlock()
		return errors.New("only the poster can assign to agent")
	}

	post.WorkerID = "agent"
	post.Worker = "agent"
	post.Status = StatusClaimed
	post.ClaimedAt = time.Now()
	save()
	mutex.Unlock()

	// Run the agent in background
	go runAgent(post)

	return nil
}

// runAgent executes the iterative build loop for a task.
// Build → verify → fix → verify → ... until done or budget exceeded.
// All progress is logged and persisted so it survives restarts.
func runAgent(post *Post) {
	if BuildApp == nil {
		failAgent(post, "No builder configured")
		return
	}

	authorID := post.AuthorID
	description := post.Description

	// Step 1: Build
	if !spendCredits(post, authorID, creditPerStep) {
		addLog(post, "budget", "Budget exceeded before build", 0)
		failAgent(post, "Budget exceeded")
		return
	}

	addLog(post, "build", "Building app from description...", creditPerStep)

	slug, name, err := BuildApp(description, "agent", "agent")
	if err != nil {
		addLog(post, "error", fmt.Sprintf("Build failed: %v", err), 0)
		failAgent(post, "Build failed: "+err.Error())
		return
	}

	addLog(post, "build", fmt.Sprintf("Built %s (/apps/%s/run)", name, slug), 0)

	// Steps 2-N: Verify and fix loop
	for i := 0; i < maxAgentIterations; i++ {
		// Verify
		if VerifyApp == nil {
			// No verifier — accept what we have
			break
		}

		if !spendCredits(post, authorID, creditPerStep) {
			addLog(post, "budget", "Budget exceeded during verification", 0)
			break
		}

		issues, ok := VerifyApp(slug)
		addLog(post, "verify", fmt.Sprintf("Verify attempt %d: %s", i+1, issues), creditPerStep)

		if ok {
			addLog(post, "verify", "App verified successfully", 0)
			break
		}

		// Fix
		if FixApp == nil {
			break
		}

		if !spendCredits(post, authorID, creditPerStep) {
			addLog(post, "budget", "Budget exceeded during fix", 0)
			break
		}

		if err := FixApp(slug, issues); err != nil {
			addLog(post, "error", fmt.Sprintf("Fix failed: %v", err), creditPerStep)
			break
		}

		addLog(post, "fix", fmt.Sprintf("Applied fix for: %s", issues), creditPerStep)
	}

	// Deliver the result
	delivery := fmt.Sprintf("%s — /apps/%s/run", name, slug)
	mutex.Lock()
	post.Delivery = delivery
	post.Status = StatusDelivered
	post.DeliveredAt = time.Now()
	save()
	title := post.Title
	postID := post.ID
	mutex.Unlock()

	addLog(post, "complete", fmt.Sprintf("Delivered: %s (spent %d credits)", delivery, post.Spent), 0)

	if Notify != nil {
		Notify(authorID, "Agent completed: "+title,
			fmt.Sprintf(`The agent built %s for your task. Spent %d of %d credits.

<a href="/work/%s">Review delivery →</a>`, name, post.Spent, post.Cost, postID))
	}
}

// failAgent marks a task as failed and releases it back to open.
func failAgent(post *Post, reason string) {
	mutex.Lock()
	post.WorkerID = ""
	post.Worker = ""
	post.Status = StatusOpen
	post.ClaimedAt = time.Time{}
	save()
	authorID := post.AuthorID
	title := post.Title
	mutex.Unlock()

	if Notify != nil {
		Notify(authorID, "Agent failed: "+title, reason)
	}
}

// RetryWithFeedback resets a delivered task and re-runs the agent with feedback.
func RetryWithFeedback(post *Post, feedback string) {
	mutex.Lock()
	post.Status = StatusClaimed
	post.Delivery = ""
	post.DeliveredAt = time.Time{}
	save()
	mutex.Unlock()

	addLog(post, "retry", "Retrying with feedback: "+feedback, 0)

	go func() {
		if BuildApp == nil {
			failAgent(post, "No builder configured")
			return
		}

		prompt := post.Description + "\n\nFeedback from previous attempt:\n" + feedback
		authorID := post.AuthorID

		if !spendCredits(post, authorID, creditPerStep) {
			addLog(post, "budget", "Budget exceeded", 0)
			failAgent(post, "Budget exceeded")
			return
		}

		addLog(post, "build", "Rebuilding with feedback...", creditPerStep)

		slug, name, err := BuildApp(prompt, "agent", "agent")
		if err != nil {
			addLog(post, "error", fmt.Sprintf("Build failed: %v", err), 0)
			failAgent(post, "Build failed: "+err.Error())
			return
		}

		delivery := fmt.Sprintf("%s — /apps/%s/run", name, slug)
		mutex.Lock()
		post.Delivery = delivery
		post.Status = StatusDelivered
		post.DeliveredAt = time.Now()
		save()
		title := post.Title
		postID := post.ID
		mutex.Unlock()

		addLog(post, "complete", fmt.Sprintf("Delivered: %s (spent %d credits)", delivery, post.Spent), 0)

		if Notify != nil {
			Notify(authorID, "Agent updated: "+title,
				fmt.Sprintf(`The agent rebuilt %s with your feedback. Spent %d/%d credits.

<a href="/work/%s">Review delivery →</a>`, name, post.Spent, post.Cost, postID))
		}
	}()
}

// ResumeAgentWork restarts any in-progress agent tasks (e.g. after server restart).
func ResumeAgentWork() {
	mutex.RLock()
	var inProgress []*Post
	for _, p := range posts {
		if p.WorkerID == "agent" && p.Status == StatusClaimed {
			inProgress = append(inProgress, p)
		}
	}
	mutex.RUnlock()

	for _, p := range inProgress {
		fmt.Printf("[work] Resuming agent task: %s\n", p.Title)
		go runAgent(p)
	}
}

// DeletePost removes a work post by ID
func DeletePost(id string) error {
	mutex.Lock()
	defer mutex.Unlock()
	if _, exists := posts[id]; !exists {
		return errors.New("post not found")
	}
	delete(posts, id)
	save()
	return nil
}

// GetPost returns a single post
func GetPost(id string) *Post {
	mutex.RLock()
	defer mutex.RUnlock()
	return posts[id]
}

// ListPosts returns posts filtered by kind and/or status, sorted newest first
func ListPosts(kind, status string, limit int) []*Post {
	mutex.RLock()
	defer mutex.RUnlock()

	var result []*Post
	for _, p := range posts {
		if kind != "" && p.Kind != kind {
			continue
		}
		if status != "" && p.Status != status {
			continue
		}
		result = append(result, p)
	}

	// Sort newest first
	for i := 0; i < len(result); i++ {
		for j := i + 1; j < len(result); j++ {
			if result[j].CreatedAt.After(result[i].CreatedAt) {
				result[i], result[j] = result[j], result[i]
			}
		}
	}

	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}

	return result
}
