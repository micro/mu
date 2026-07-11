// Package a2a implements the Agent-to-Agent (A2A) protocol, allowing
// external agents to discover and communicate with Mu's micro-agents.
//
// Spec: https://github.com/google/A2A
//
// Endpoints:
//
//	GET  /.well-known/agent.json  — Agent Card (discovery)
//	POST /a2a                     — JSON-RPC 2.0 (task execution)
package a2a

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"mu/agent/micro"
	"mu/internal/app"

	"github.com/google/uuid"
)

// ── Agent Card ──

type AgentCard struct {
	Name                string       `json:"name"`
	Description         string       `json:"description"`
	Version             string       `json:"version"`
	Provider            Provider     `json:"provider"`
	SupportedInterfaces []Interface  `json:"supportedInterfaces"`
	Capabilities        Capabilities `json:"capabilities"`
	DefaultInputModes   []string     `json:"defaultInputModes"`
	DefaultOutputModes  []string     `json:"defaultOutputModes"`
	Skills              []Skill      `json:"skills"`
}

type Provider struct {
	Organization string `json:"organization"`
	URL          string `json:"url"`
}

type Interface struct {
	URL             string `json:"url"`
	ProtocolBinding string `json:"protocolBinding"`
	ProtocolVersion string `json:"protocolVersion"`
}

type Capabilities struct {
	Streaming         bool `json:"streaming"`
	PushNotifications bool `json:"pushNotifications"`
}

type Skill struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags,omitempty"`
	Examples    []string `json:"examples,omitempty"`
}

func buildAgentCard(baseURL string) AgentCard {
	var skills []Skill
	for _, a := range micro.All() {
		skills = append(skills, Skill{
			ID:          a.ID,
			Name:        a.Name,
			Description: a.Description,
		})
	}

	return AgentCard{
		Name:        "Micro",
		Description: "Personal AI agent with news, markets, trading, mail, weather, search, and more",
		Version:     "1.0.0",
		Provider: Provider{
			Organization: "Micro",
			URL:          "https://github.com/micro/mu",
		},
		SupportedInterfaces: []Interface{
			{
				URL:             baseURL + "/a2a",
				ProtocolBinding: "jsonrpc-over-http",
				ProtocolVersion: "1.0.0",
			},
		},
		Capabilities: Capabilities{
			Streaming: false,
		},
		DefaultInputModes:  []string{"text"},
		DefaultOutputModes: []string{"text"},
		Skills:             skills,
	}
}

// ── Task Store ──

type Task struct {
	ID        string     `json:"id"`
	ContextID string     `json:"contextId,omitempty"`
	Status    TaskStatus `json:"status"`
	Artifacts []Artifact `json:"artifacts,omitempty"`
	History   []Message  `json:"history,omitempty"`
	CreatedAt string     `json:"createdAt"`
}

type TaskStatus struct {
	State     string   `json:"state"`
	Timestamp string   `json:"timestamp"`
	Message   *Message `json:"message,omitempty"`
}

type Message struct {
	Role      string `json:"role"`
	Parts     []Part `json:"parts"`
	MessageID string `json:"messageId,omitempty"`
}

type Part struct {
	Text string `json:"text,omitempty"`
}

type Artifact struct {
	ArtifactID string `json:"artifactId"`
	Name       string `json:"name,omitempty"`
	Parts      []Part `json:"parts"`
}

var (
	taskMu sync.RWMutex
	tasks  = map[string]*Task{}
)

// ── JSON-RPC ──

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	ID      any             `json:"id"`
}

type rpcResponse struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      any       `json:"id"`
	Result  any       `json:"result,omitempty"`
	Error   *rpcError `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// BaseURL is set by main.go based on the configured domain.
var BaseURL string

// AgentCardHandler serves GET /.well-known/agent.json
func AgentCardHandler(w http.ResponseWriter, r *http.Request) {
	card := buildAgentCard(BaseURL)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(card)
}

// Handler serves POST /a2a — the JSON-RPC endpoint.
func Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", 405)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, nil, -32700, "Parse error")
		return
	}

	var req rpcRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, nil, -32700, "Parse error")
		return
	}

	app.Log("a2a", "Method: %s", req.Method)

	switch req.Method {
	case "SendMessage":
		handleSendMessage(w, req)
	case "GetTask":
		handleGetTask(w, req)
	case "CancelTask":
		handleCancelTask(w, req)
	default:
		writeError(w, req.ID, -32601, "Method not found: "+req.Method)
	}
}

type sendMessageParams struct {
	Message       Message `json:"message"`
	Configuration struct {
		AcceptedOutputModes []string `json:"acceptedOutputModes"`
	} `json:"configuration"`
}

func handleSendMessage(w http.ResponseWriter, req rpcRequest) {
	var params sendMessageParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		writeError(w, req.ID, -32602, "Invalid params")
		return
	}

	// Extract text from message parts
	var prompt string
	for _, p := range params.Message.Parts {
		if p.Text != "" {
			prompt += p.Text
		}
	}
	if prompt == "" {
		writeError(w, req.ID, -32602, "No text in message")
		return
	}

	// Determine which agent to route to
	agentIDs := micro.Route(prompt)
	if len(agentIDs) == 0 {
		agentIDs = []string{"micro"}
	}

	// Create task
	taskID := uuid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	task := &Task{
		ID:        taskID,
		ContextID: params.Message.MessageID,
		Status: TaskStatus{
			State:     "TASK_STATE_WORKING",
			Timestamp: now,
		},
		History:   []Message{params.Message},
		CreatedAt: now,
	}

	taskMu.Lock()
	tasks[taskID] = task
	taskMu.Unlock()

	// Execute agent
	// Use a generic account for A2A requests (no user session)
	answer, err := micro.Orchestrate("a2a", prompt, agentIDs, true)
	if err != nil {
		task.Status = TaskStatus{
			State:     "TASK_STATE_FAILED",
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Message: &Message{
				Role:  "agent",
				Parts: []Part{{Text: err.Error()}},
			},
		}
		taskMu.Lock()
		tasks[taskID] = task
		taskMu.Unlock()

		writeResult(w, req.ID, task)
		return
	}

	// Complete task
	task.Status = TaskStatus{
		State:     "TASK_STATE_COMPLETED",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Message: &Message{
			Role:  "agent",
			Parts: []Part{{Text: answer}},
		},
	}
	task.Artifacts = []Artifact{
		{
			ArtifactID: "response",
			Name:       "Response",
			Parts:      []Part{{Text: answer}},
		},
	}

	taskMu.Lock()
	tasks[taskID] = task
	// Cap stored tasks
	if len(tasks) > 1000 {
		for id, t := range tasks {
			if t.Status.State == "TASK_STATE_COMPLETED" || t.Status.State == "TASK_STATE_FAILED" {
				delete(tasks, id)
				if len(tasks) <= 500 {
					break
				}
			}
		}
	}
	taskMu.Unlock()

	writeResult(w, req.ID, task)
}

type getTaskParams struct {
	ID string `json:"id"`
}

func handleGetTask(w http.ResponseWriter, req rpcRequest) {
	var params getTaskParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		writeError(w, req.ID, -32602, "Invalid params")
		return
	}

	taskMu.RLock()
	task, ok := tasks[params.ID]
	taskMu.RUnlock()

	if !ok {
		writeError(w, req.ID, -32001, "Task not found")
		return
	}

	writeResult(w, req.ID, task)
}

func handleCancelTask(w http.ResponseWriter, req rpcRequest) {
	var params getTaskParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		writeError(w, req.ID, -32602, "Invalid params")
		return
	}

	taskMu.Lock()
	task, ok := tasks[params.ID]
	if ok {
		task.Status = TaskStatus{
			State:     "TASK_STATE_CANCELED",
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		}
	}
	taskMu.Unlock()

	if !ok {
		writeError(w, req.ID, -32001, "Task not found")
		return
	}

	writeResult(w, req.ID, task)
}

func writeResult(w http.ResponseWriter, id any, result any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	})
}

func writeError(w http.ResponseWriter, id any, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &rpcError{Code: code, Message: message},
	})
}

// AgentForSkill returns the right agent description string for use in
// external A2A directory listings.
func AgentForSkill(skillID string) string {
	a := micro.Get(skillID)
	if a == nil {
		return ""
	}
	return fmt.Sprintf("%s: %s", a.Name, a.Description)
}

var _ = strings.TrimSpace
