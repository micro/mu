package agent

// Pending replies are delivery work, not user-visible tasks. They reuse Ask and
// the conversation record. A running reply is never replayed after a restart:
// an interrupted tool may already have changed something outside this process.
import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/data"
	"mu/internal/thread"
)

type pendingReply struct {
	Context ClientContext `json:"context,omitempty"`
	Digest  string        `json:"digest"`
	ID      string        `json:"id"`
	Account string        `json:"account"`
	Thread  string        `json:"thread"`
	Text    string        `json:"text,omitempty"`
	Agent   string        `json:"agent,omitempty"`
	State   string        `json:"state"`
	Created time.Time     `json:"created"`
}

const repliesFile = "agent_replies.json"

var replies = struct {
	sync.Mutex
	once   sync.Once
	jobs   map[string]pendingReply
	active map[string]string // job -> account; never persisted
	err    error
}{jobs: map[string]pendingReply{}, active: map[string]string{}}

func init() {
	auth.AccountDeleteHooks = append(auth.AccountDeleteHooks, func(accountID string) {
		loadReplies()
		replies.Lock()
		defer replies.Unlock()
		if replies.err != nil {
			return
		}
		for id, j := range replies.jobs {
			if j.Account == accountID {
				delete(replies.jobs, id)
			}
		}
		if err := data.SaveJSON(repliesFile, replies.jobs); err != nil {
			app.Log("agent", "removing account reply queue: %v", err)
		}
	})
}

func loadReplies() {
	replies.once.Do(func() {
		err := data.LoadJSON(repliesFile, &replies.jobs)
		if err != nil && !os.IsNotExist(err) {
			replies.err = err
			return
		}
		if replies.jobs == nil {
			replies.jobs = map[string]pendingReply{}
		}
		for id, j := range replies.jobs {
			if j.State == "running" {
				j.State = "interrupted"
				replies.jobs[id] = j
			}
		}
		// Keep one execution per account, with capacity for other people to work.
		for i := 0; i < 4; i++ {
			go func() {
				for {
					runReply()
					time.Sleep(time.Second)
				}
			}()
		}
	})
}

// SubmitReply acknowledges only a durable job and message. The optional client
// reference deduplicates SMTP retries within the retained seven-day receipt window.
func SubmitReply(accountID, threadID, text, ref string) error {
	return submitReply(accountID, threadID, text, ref, ClientContext{})
}

func submitReply(accountID, threadID, text, ref string, context ClientContext) error {
	loadReplies()
	text = strings.TrimSpace(text)
	if text == "" || len([]rune(text)) > 8000 {
		return fmt.Errorf("write a message of up to 8,000 characters")
	}
	acc, err := auth.GetAccount(accountID)
	if err != nil || acc == nil || acc.Banned || (!acc.Admin && !acc.Approved && !acc.EmailVerified) {
		return fmt.Errorf("account verification required")
	}
	t := thread.Get(accountID, threadID)
	if t == nil || thread.IsHeld(*t) {
		return errNoConversation
	}
	if t.Agent != "" && Platform(t.Agent) == nil {
		if _, err := AskAs(accountID, t.Agent); err != nil {
			return err
		}
	}
	id := newFlowID()
	if ref != "" {
		id = replyID(accountID, threadID, ref)
	}
	replies.Lock()
	defer replies.Unlock()
	if replies.err != nil {
		return fmt.Errorf("reply queue unavailable")
	}
	if j, ok := replies.jobs[id]; ok {
		digest := sha256.Sum256([]byte(text))
		if j.Digest != hex.EncodeToString(digest[:]) {
			return fmt.Errorf("message identifier already used")
		}
		return thread.Flush()
	}
	total, own := 0, 0
	for key, j := range replies.jobs {
		if j.State == "done" {
			if time.Since(j.Created) > 7*24*time.Hour {
				delete(replies.jobs, key)
			}
			continue
		}
		total++
		if j.Account == accountID {
			own++
			if j.Thread == threadID {
				return fmt.Errorf("a reply is already pending in this conversation")
			}
		}
	}
	if total >= 64 || own >= 4 || len(replies.jobs) >= 10000 {
		return fmt.Errorf("reply queue is full; try again later")
	}
	if reason, ok := affordable(accountID); !ok {
		return fmt.Errorf("%s", reason)
	}
	// Persist the conversation before a queued job can survive a restart.
	if err := thread.Flush(); err != nil {
		return err
	}
	digest := sha256.Sum256([]byte(text))
	j := pendingReply{Context: context, Digest: hex.EncodeToString(digest[:]), ID: id, Account: accountID, Thread: threadID, Text: text, Agent: t.Agent, State: "queued", Created: time.Now().UTC()}
	replies.jobs[id] = j
	if err := data.SaveJSON(repliesFile, replies.jobs); err != nil {
		delete(replies.jobs, id)
		return err
	}
	// The worker cannot start until the message is visible and flushed.
	if thread.Add(thread.Message{Account: accountID, Thread: threadID, Role: thread.RolePerson, Text: text, Ref: "reply:" + id}) == "" {
		return errNoConversation
	}
	return thread.Flush()
}

func runReply() {
	replies.Lock()
	if replies.err != nil {
		replies.Unlock()
		return
	}
	busy := map[string]bool{}
	for _, account := range replies.active {
		busy[account] = true
	}
	var job pendingReply
	for _, j := range replies.jobs {
		if !busy[j.Account] && j.State != "done" && (job.ID == "" || j.Created.Before(job.Created)) {
			job = j
		}
	}
	if job.ID == "" {
		replies.Unlock()
		return
	}
	original := job.State
	if original == "queued" {
		job.State = "running"
		replies.jobs[job.ID] = job
		if err := data.SaveJSON(repliesFile, replies.jobs); err != nil {
			job.State = "queued"
			replies.jobs[job.ID] = job
			replies.Unlock()
			return
		}
	}
	replies.active[job.ID] = job.Account
	replies.Unlock()
	defer func() {
		replies.Lock()
		delete(replies.active, job.ID)
		replies.Unlock()
	}()
	t := thread.Get(job.Account, job.Thread)
	acc, accErr := auth.GetAccount(job.Account)
	if t != nil && accErr == nil && acc != nil && !acc.Banned && !thread.IsHeld(*t) && (acc.Admin || acc.Approved || acc.EmailVerified) {
		switch original {
		case "queued":
			// Recheck the captured specialist; never silently replace it on execution.
			answer, err := Ask(AskRequest{Context: job.Context, Account: job.Account, On: job.Thread, Client: t.Client, Agent: job.Agent, Text: job.Text, MessageRef: "reply:" + job.ID})
			if err == nil && strings.TrimSpace(answer.Text) == "" {
				err = fmt.Errorf("no answer was recorded")
			}
			if err != nil {
				app.Log("agent", "queued reply %s failed: %v", job.ID, err)
				if err = AnsweredOnce(job.Account, job.Thread, "The reply could not be completed. Review the conversation before trying again.", "", "reply-error:"+job.ID); err != nil {
					markReply(job, "interrupted")
					return
				}
			}
		case "interrupted":
			if err := AnsweredOnce(job.Account, job.Thread, "The server stopped while processing this reply. It has not been retried because an action may already have happened. Please review before asking again.", "", "reply-interrupted:"+job.ID); err != nil {
				return
			}
		}
	}
	// On a flush failure, retry persistence only, never model/tool execution.
	if err := thread.Flush(); err != nil {
		markReply(job, "finishing")
		return
	}
	markReply(job, "done")
}

func markReply(job pendingReply, state string) {
	replies.Lock()
	defer replies.Unlock()
	if _, exists := replies.jobs[job.ID]; !exists {
		return
	}
	job.State = state
	if state == "done" {
		job.Text = ""
		job.Context = ClientContext{}
	}
	replies.jobs[job.ID] = job
	if err := data.SaveJSON(repliesFile, replies.jobs); err != nil {
		// Keep the in-memory terminal state so a storage error cannot replay work.
		if state == "done" {
			job.State = "finishing"
			replies.jobs[job.ID] = job
		}
		app.Log("agent", "persisting reply %s: %v", job.ID, err)
	}
}

func replyStatus(accountID, threadID string) string {
	replies.Lock()
	defer replies.Unlock()
	for _, j := range replies.jobs {
		if j.Account != accountID || j.Thread != threadID {
			continue
		}
		switch j.State {
		case "queued":
			return "Queued. You can leave this page; the reply will appear here."
		case "running":
			return "Working. You can leave this page; the reply will appear here."
		case "interrupted", "finishing":
			return "Saving the outcome."
		}
	}
	return ""
}

// Serialize opening and submitting so a retried first message keeps its agent
// and conversation, even when the acceptance response was lost.
var promptMu sync.Mutex

func queuePrompt(account, id, text, ref, agent, attachment string, context ClientContext) (string, error) {
	if len(ref) < 16 || len(ref) > 128 || strings.TrimSpace(text) == "" || len([]rune(text)) > 8000 {
		return "", fmt.Errorf("a message identifier and up to 8,000 characters are required")
	}
	promptMu.Lock()
	defer promptMu.Unlock()
	if id == "" {
		sum := sha256.Sum256([]byte(ref))
		key := "prompt:" + hex.EncodeToString(sum[:])
		if t := thread.Find(account, thread.WebClient, key); t != nil {
			if t.Agent != agent || thread.Attachment(account, t.ID) != attachment {
				return "", fmt.Errorf("message identifier already used")
			}
			id = t.ID
		} else {
			if reason, ok := affordable(account); !ok {
				return "", fmt.Errorf("%s", reason)
			}
			id = Opened(account, thread.WebClient, key, "", agent)
			thread.SetAttachment(account, id, attachment)
		}
	}
	if err := submitReply(account, id, text, ref, context); err != nil {
		return "", err
	}
	return id, nil
}

func replyID(accountID, threadID, ref string) string {
	sum := sha256.Sum256([]byte(accountID + "\x00" + threadID + "\x00" + ref))
	return hex.EncodeToString(sum[:])
}
