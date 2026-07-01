package platform

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	maxSSESubscribers = 4
	eventBufferSize   = 50
	sessionTimeout    = 30 * time.Minute
)

type sessionState string

const (
	sessionActive    sessionState = "active"
	sessionReady     sessionState = "ready"
	sessionCompleted sessionState = "completed"
	sessionFailed    sessionState = "failed"
	sessionTimedOut  sessionState = "timed_out"
)

// bridgeEvent is one line of the agent's bridge-mode stdout.
type bridgeEvent struct {
	Type            string      `json:"type"`
	Role            string      `json:"role,omitempty"`
	Text            string      `json:"text,omitempty"`
	Checklist       []checkItem `json:"checklist,omitempty"`
	ManifestVersion int         `json:"manifestVersion,omitempty"`
	Reason          string      `json:"reason,omitempty"`
	AppID           string      `json:"appId,omitempty"`
}

type checkItem struct {
	Text string `json:"text"`
	Done bool   `json:"done"`
}

// planSession tracks one live planning session.
type planSession struct {
	id           string
	state        sessionState
	appID        string // set once the agent emits ready
	cmd          *exec.Cmd
	stdin        io.WriteCloser
	events       []bridgeEvent // rolling buffer, max eventBufferSize
	subs         []chan bridgeEvent
	lastActivity time.Time
	mu           sync.Mutex
}

// planServer manages all planning sessions.
type planServer struct {
	agentBin string
	baseURL  string   // this server's own address, passed to agent subprocesses as GO_BASE_URL
	sessions sync.Map // sessionID → *planSession
}

func newPlanServer(agentBin, addr string) *planServer {
	ps := &planServer{agentBin: agentBin, baseURL: baseURLFromAddr(addr)}
	go ps.reapLoop()
	return ps
}

// envWithout returns env with any entry for key removed, so the caller can
// append its own authoritative KEY=VALUE without risking an ambiguous
// duplicate entry in the child process's environment.
func envWithout(env []string, key string) []string {
	prefix := key + "="
	out := env[:0:0]
	for _, kv := range env {
		if !strings.HasPrefix(kv, prefix) {
			out = append(out, kv)
		}
	}
	return out
}

// baseURLFromAddr turns a listen address (e.g. ":8080", "0.0.0.0:8080") into
// a URL an agent subprocess running on the same machine can call back on.
// The host is always localhost: the agent is always a local child process,
// regardless of what host the server is publicly bound to.
func baseURLFromAddr(addr string) string {
	_, port, err := net.SplitHostPort(addr)
	if err != nil || port == "" {
		return "http://localhost:8080"
	}
	return "http://localhost:" + port
}

func (ps *planServer) route(mux *http.ServeMux) {
	mux.HandleFunc("/platform/plan", ps.handleCreate)
	mux.HandleFunc("/platform/plan/", ps.handleSession)
}

// handleCreate implements POST /platform/plan.
func (ps *planServer) handleCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Prompt string `json:"prompt"`
		AppID  string `json:"appId"`
	}
	if err := decodeJSON(r, &body); err != nil || strings.TrimSpace(body.Prompt) == "" {
		writeError(w, http.StatusBadRequest, "invalid_body", "prompt is required")
		return
	}

	sess, err := ps.startSession(body.Prompt, body.AppID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "spawn_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"sessionId": sess.id, "state": string(sess.state)})
}

// handleSession dispatches /platform/plan/{sessionId}/...
func (ps *planServer) handleSession(w http.ResponseWriter, r *http.Request) {
	// path: /platform/plan/{sessionId}[/events|/message|/approve]
	rest := strings.TrimPrefix(r.URL.Path, "/platform/plan/")
	parts := strings.SplitN(rest, "/", 2)
	sessionID := parts[0]
	sub := ""
	if len(parts) == 2 {
		sub = parts[1]
	}

	raw, ok := ps.sessions.Load(sessionID)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "session not found")
		return
	}
	sess := raw.(*planSession)

	switch sub {
	case "events":
		ps.handleEvents(w, r, sess)
	case "message":
		ps.handleMessage(w, r, sess)
	case "approve":
		ps.handleApprove(w, r, sess)
	default:
		http.NotFound(w, r)
	}
}

// startSession spawns the agent subprocess in bridge mode.
func (ps *planServer) startSession(prompt, appID string) (*planSession, error) {
	id := uuid.New().String()
	args := []string{"--bridge-mode", "--prompt", prompt}
	if appID != "" {
		args = append(args, "--app", appID)
	}

	agentBin := ps.agentBin
	if agentBin == "" {
		agentBin = "npx"
		args = append([]string{"tsx", "agent/src/cli.ts"}, args...)
	}

	cmd := exec.Command(agentBin, args...)
	cmd.Env = append(envWithout(os.Environ(), "GO_BASE_URL"), "GO_BASE_URL="+ps.baseURL)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start agent: %w", err)
	}

	sess := &planSession{
		id:           id,
		state:        sessionActive,
		cmd:          cmd,
		stdin:        stdin,
		lastActivity: time.Now(),
	}
	ps.sessions.Store(id, sess)

	go ps.readOutput(sess, stdout)
	return sess, nil
}

// readOutput reads bridge-mode JSON lines from the agent's stdout and fans out
// to SSE subscribers.
func (ps *planServer) readOutput(sess *planSession, r io.Reader) {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := sc.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		var ev bridgeEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			log.Printf("platform/plan: bad event line from session %s: %v", sess.id, err)
			continue
		}

		sess.mu.Lock()
		// Capture appId from ready event.
		if ev.Type == "ready" && ev.AppID != "" {
			sess.appID = ev.AppID
			sess.state = sessionReady
		}
		if ev.Type == "done" {
			sess.state = sessionCompleted
		}
		if ev.Type == "error" {
			sess.state = sessionFailed
		}
		sess.lastActivity = time.Now()
		// Append to rolling buffer.
		sess.events = append(sess.events, ev)
		if len(sess.events) > eventBufferSize {
			sess.events = sess.events[len(sess.events)-eventBufferSize:]
		}
		// Fan out to subscribers.
		for _, ch := range sess.subs {
			select {
			case ch <- ev:
			default:
			}
		}
		sess.mu.Unlock()

		if ev.Type == "done" || ev.Type == "error" {
			break
		}
	}
	if err := sc.Err(); err != nil {
		log.Printf("platform/plan: stdout read error for session %s: %v", sess.id, err)
	}
}

// handleEvents implements GET /platform/plan/{sessionId}/events as SSE.
func (ps *planServer) handleEvents(w http.ResponseWriter, r *http.Request, sess *planSession) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sess.mu.Lock()
	if len(sess.subs) >= maxSSESubscribers {
		sess.mu.Unlock()
		writeError(w, http.StatusTooManyRequests, "too_many_subscribers", "max subscribers reached")
		return
	}
	ch := make(chan bridgeEvent, 64)
	sess.subs = append(sess.subs, ch)

	// Replay buffered events from Last-Event-ID if provided.
	var replay []bridgeEvent
	lastID := r.Header.Get("Last-Event-ID")
	if lastID == "" {
		replay = append([]bridgeEvent{}, sess.events...)
	} else {
		// Find the event after the last seen index.
		for i := range sess.events {
			if fmt.Sprintf("%d", i) == lastID {
				if i+1 < len(sess.events) {
					replay = append([]bridgeEvent{}, sess.events[i+1:]...)
				}
				break
			}
		}
	}
	sess.mu.Unlock()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, _ := w.(http.Flusher)

	sendEvent := func(i int, ev bridgeEvent) {
		b, _ := json.Marshal(ev)
		fmt.Fprintf(w, "id: %d\ndata: %s\n\n", i, b)
		if flusher != nil {
			flusher.Flush()
		}
	}

	// Send replayed events.
	sess.mu.Lock()
	baseIdx := len(sess.events) - len(replay)
	sess.mu.Unlock()
	for i, ev := range replay {
		sendEvent(baseIdx+i, ev)
	}

	// Stream new events until client disconnects or session ends.
	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			sess.mu.Lock()
			for i, sub := range sess.subs {
				if sub == ch {
					sess.subs = append(sess.subs[:i], sess.subs[i+1:]...)
					break
				}
			}
			sess.mu.Unlock()
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			sess.mu.Lock()
			idx := len(sess.events) - 1
			sess.mu.Unlock()
			sendEvent(idx, ev)
			if ev.Type == "done" || ev.Type == "error" {
				return
			}
		}
	}
}

// handleMessage implements POST /platform/plan/{sessionId}/message.
func (ps *planServer) handleMessage(w http.ResponseWriter, r *http.Request, sess *planSession) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sess.mu.Lock()
	state := sess.state
	sess.mu.Unlock()

	if state != sessionActive && state != sessionReady {
		writeError(w, http.StatusConflict, "session_not_active", "session not active")
		return
	}

	var body struct {
		Text string `json:"text"`
	}
	if err := decodeJSON(r, &body); err != nil || strings.TrimSpace(body.Text) == "" {
		writeError(w, http.StatusBadRequest, "invalid_body", "text is required")
		return
	}

	msg, _ := json.Marshal(map[string]string{"type": "message", "text": body.Text})
	if _, err := fmt.Fprintf(sess.stdin, "%s\n", msg); err != nil {
		writeError(w, http.StatusInternalServerError, "write_failed", err.Error())
		return
	}
	sess.mu.Lock()
	sess.lastActivity = time.Now()
	sess.mu.Unlock()

	writeJSON(w, http.StatusAccepted, map[string]bool{"ok": true})
}

// handleApprove implements POST /platform/plan/{sessionId}/approve.
func (ps *planServer) handleApprove(w http.ResponseWriter, r *http.Request, sess *planSession) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sess.mu.Lock()
	state := sess.state
	appID := sess.appID
	sess.mu.Unlock()

	if state != sessionReady {
		writeError(w, http.StatusConflict, "not_ready", "session is not ready to build")
		return
	}

	msg, _ := json.Marshal(map[string]string{"type": "approve"})
	if _, err := fmt.Fprintf(sess.stdin, "%s\n", msg); err != nil {
		writeError(w, http.StatusInternalServerError, "write_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "appId": appID})
}

// reapLoop kills and removes sessions that have been inactive for sessionTimeout.
func (ps *planServer) reapLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		ps.sessions.Range(func(k, v any) bool {
			sess := v.(*planSession)
			sess.mu.Lock()
			inactive := time.Since(sess.lastActivity) > sessionTimeout
			state := sess.state
			sess.mu.Unlock()

			if inactive && (state == sessionActive || state == sessionReady) {
				log.Printf("platform/plan: session %s timed out", sess.id)
				_ = sess.cmd.Process.Kill()
				timeoutEv := bridgeEvent{Type: "error", Reason: "session_timeout"}
				sess.mu.Lock()
				for _, ch := range sess.subs {
					select {
					case ch <- timeoutEv:
					default:
					}
				}
				sess.state = sessionTimedOut
				sess.mu.Unlock()
			}
			return true
		})
	}
}
