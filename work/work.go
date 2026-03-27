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
	Cost        int       `json:"cost"`        // Credits cost for the task
	AuthorID    string    `json:"author_id"`
	Author      string    `json:"author"`      // Display name
	WorkerID    string    `json:"worker_id"`   // Who claimed a task
	Worker      string    `json:"worker"`      // Worker display name
	Status      string    `json:"status"`      // Task status (open/claimed/delivered/completed/cancelled)
	Delivery    string    `json:"delivery"`    // Deliverable for tasks
	Tags        string    `json:"tags"`        // Comma-separated
	Tips        int       `json:"tips"`        // Total tips received (show)
	Feedback    []Comment `json:"feedback"`    // Comments/feedback
	CreatedAt   time.Time `json:"created_at"`
	ClaimedAt   time.Time `json:"claimed_at,omitempty"`
	DeliveredAt time.Time `json:"delivered_at,omitempty"`
	CompletedAt time.Time `json:"completed_at,omitempty"`
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
	if post.AuthorID != authorID {
		return errors.New("only the poster can release")
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

	// Run the builder in background
	go func() {
		if BuildApp == nil {
			// No builder wired — release back to open
			mutex.Lock()
			post.WorkerID = ""
			post.Worker = ""
			post.Status = StatusOpen
			post.ClaimedAt = time.Time{}
			save()
			mutex.Unlock()
			return
		}

		slug, name, err := BuildApp(post.Description, "agent", "agent")
		if err != nil {
			// Failed — release back to open
			mutex.Lock()
			post.WorkerID = ""
			post.Worker = ""
			post.Status = StatusOpen
			post.ClaimedAt = time.Time{}
			save()
			mutex.Unlock()
			return
		}

		// Deliver the result
		delivery := fmt.Sprintf("%s — /apps/%s/run", name, slug)
		mutex.Lock()
		post.Delivery = delivery
		post.Status = StatusDelivered
		post.DeliveredAt = time.Now()
		save()
		authorID := post.AuthorID
		title := post.Title
		postID := post.ID
		mutex.Unlock()

		if Notify != nil {
			Notify(authorID, "Agent completed: "+title,
				fmt.Sprintf("The agent built %s for your task. Review it at /work/%s", name, postID))
		}
	}()

	return nil
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
