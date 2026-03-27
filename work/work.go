package work

import (
	"encoding/json"
	"errors"
	"sync"
	"time"

	"mu/internal/data"

	"github.com/google/uuid"
)

// Task states
const (
	StatusOpen       = "open"       // Accepting claims
	StatusClaimed    = "claimed"    // Someone is working on it
	StatusDelivered  = "delivered"  // Work submitted, awaiting acceptance
	StatusCompleted  = "completed"  // Accepted and paid
	StatusCancelled  = "cancelled"  // Cancelled by poster
)

// Task represents a work bounty
type Task struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Bounty      int       `json:"bounty"`      // Credits
	PosterID    string    `json:"poster_id"`
	Poster      string    `json:"poster"`       // Display name
	WorkerID    string    `json:"worker_id"`    // Who claimed it
	Worker      string    `json:"worker"`       // Worker display name
	Status      string    `json:"status"`
	Delivery    string    `json:"delivery"`     // Deliverable (URL, text, app slug)
	Tags        string    `json:"tags"`         // Comma-separated
	CreatedAt   time.Time `json:"created_at"`
	ClaimedAt   time.Time `json:"claimed_at,omitempty"`
	DeliveredAt time.Time `json:"delivered_at,omitempty"`
	CompletedAt time.Time `json:"completed_at,omitempty"`
}

var (
	mutex sync.RWMutex
	tasks = map[string]*Task{}
)

func init() {
	b, _ := data.LoadFile("work.json")
	json.Unmarshal(b, &tasks)
}

// Load initializes the work building block
func Load() {
	// Seed initial tasks if empty
	if len(tasks) == 0 {
		seedTasks()
	}
}

func save() {
	data.SaveJSON("work.json", tasks)
}

// CreateTask posts a new task with a bounty
func CreateTask(posterID, poster, title, description, tags string, bounty int) (*Task, error) {
	if title == "" {
		return nil, errors.New("title is required")
	}
	if description == "" {
		return nil, errors.New("description is required")
	}
	if bounty < 1 {
		return nil, errors.New("bounty must be at least 1 credit")
	}
	if bounty > 50000 {
		return nil, errors.New("maximum bounty is 50,000 credits")
	}

	task := &Task{
		ID:          uuid.New().String(),
		Title:       title,
		Description: description,
		Bounty:      bounty,
		PosterID:    posterID,
		Poster:      poster,
		Status:      StatusOpen,
		Tags:        tags,
		CreatedAt:   time.Now(),
	}

	mutex.Lock()
	tasks[task.ID] = task
	save()
	mutex.Unlock()

	return task, nil
}

// ClaimTask marks a task as claimed by a worker
func ClaimTask(taskID, workerID, worker string) error {
	mutex.Lock()
	defer mutex.Unlock()

	task, exists := tasks[taskID]
	if !exists {
		return errors.New("task not found")
	}
	if task.Status != StatusOpen {
		return errors.New("task is not open")
	}
	if task.PosterID == workerID {
		return errors.New("cannot claim your own task")
	}

	task.WorkerID = workerID
	task.Worker = worker
	task.Status = StatusClaimed
	task.ClaimedAt = time.Now()

	save()
	return nil
}

// DeliverTask submits work for review
func DeliverTask(taskID, workerID, delivery string) error {
	if delivery == "" {
		return errors.New("delivery is required")
	}

	mutex.Lock()
	defer mutex.Unlock()

	task, exists := tasks[taskID]
	if !exists {
		return errors.New("task not found")
	}
	if task.Status != StatusClaimed {
		return errors.New("task is not claimed")
	}
	if task.WorkerID != workerID {
		return errors.New("only the assigned worker can deliver")
	}

	task.Delivery = delivery
	task.Status = StatusDelivered
	task.DeliveredAt = time.Now()

	save()
	return nil
}

// AcceptTask accepts delivery and triggers payment
func AcceptTask(taskID, posterID string) error {
	mutex.Lock()
	defer mutex.Unlock()

	task, exists := tasks[taskID]
	if !exists {
		return errors.New("task not found")
	}
	if task.Status != StatusDelivered {
		return errors.New("task has not been delivered")
	}
	if task.PosterID != posterID {
		return errors.New("only the poster can accept delivery")
	}

	task.Status = StatusCompleted
	task.CompletedAt = time.Now()

	save()
	return nil
}

// CancelTask cancels an open or claimed task (poster only)
func CancelTask(taskID, posterID string) error {
	mutex.Lock()
	defer mutex.Unlock()

	task, exists := tasks[taskID]
	if !exists {
		return errors.New("task not found")
	}
	if task.PosterID != posterID {
		return errors.New("only the poster can cancel")
	}
	if task.Status == StatusCompleted {
		return errors.New("completed tasks cannot be cancelled")
	}

	task.Status = StatusCancelled

	save()
	return nil
}

// ReleaseTask releases a claimed task back to open (poster only, if worker hasn't delivered)
func ReleaseTask(taskID, posterID string) error {
	mutex.Lock()
	defer mutex.Unlock()

	task, exists := tasks[taskID]
	if !exists {
		return errors.New("task not found")
	}
	if task.PosterID != posterID {
		return errors.New("only the poster can release")
	}
	if task.Status != StatusClaimed {
		return errors.New("task is not claimed")
	}

	task.WorkerID = ""
	task.Worker = ""
	task.Status = StatusOpen
	task.ClaimedAt = time.Time{}

	save()
	return nil
}

// GetTask returns a single task
func GetTask(id string) *Task {
	mutex.RLock()
	defer mutex.RUnlock()
	return tasks[id]
}

// ListTasks returns tasks filtered by status, sorted by newest first
func ListTasks(status string, limit int) []*Task {
	mutex.RLock()
	defer mutex.RUnlock()

	var result []*Task
	for _, t := range tasks {
		if status != "" && t.Status != status {
			continue
		}
		result = append(result, t)
	}

	// Sort by created date, newest first
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

// ListTasksByPoster returns tasks posted by a user
func ListTasksByPoster(posterID string) []*Task {
	mutex.RLock()
	defer mutex.RUnlock()

	var result []*Task
	for _, t := range tasks {
		if t.PosterID == posterID {
			result = append(result, t)
		}
	}
	return result
}

// ListTasksByWorker returns tasks claimed/completed by a user
func ListTasksByWorker(workerID string) []*Task {
	mutex.RLock()
	defer mutex.RUnlock()

	var result []*Task
	for _, t := range tasks {
		if t.WorkerID == workerID {
			result = append(result, t)
		}
	}
	return result
}
