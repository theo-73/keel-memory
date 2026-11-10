package dashboard

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"strings"
	"sync"
	"time"

	"keel/pkg/brief"
	"keel/pkg/model"
	"keel/pkg/store"
	"keel/pkg/taskgraph"
)

//go:embed static/*
var staticFS embed.FS

type ProjectView struct {
	Manifest      model.ProjectManifest `json:"manifest"`
	State         model.ProjectState    `json:"state"`
	Goals         []model.Goal          `json:"goals"`
	Tasks         []model.Task          `json:"tasks"`
	Evaluation    taskgraph.GraphResult `json:"evaluation"`
	LatestWorklog *model.WorklogEntry   `json:"latestWorklog,omitempty"`
}

type Server struct {
	store   *store.Store
	clients map[chan []byte]bool
	mu      sync.Mutex
	stopCh  chan struct{}
}

func NewServer(s *store.Store) *Server {
	srv := &Server{
		store:   s,
		clients: make(map[chan []byte]bool),
		stopCh:  make(chan struct{}),
	}
	go srv.pollProjectChanges()
	return srv
}

func (s *Server) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	select {
	case <-s.stopCh:
	default:
		close(s.stopCh)
	}
}

func (s *Server) LoadView() (ProjectView, error) {
	manifest, err := s.store.Manifest()
	if err != nil {
		return ProjectView{}, err
	}
	state, err := s.store.State()
	if err != nil {
		state = model.DefaultState(manifest.Name)
	}
	goals, _ := s.store.ListGoals()
	tasks, _ := s.store.ListTasks()
	eval, _ := taskgraph.Evaluate(tasks)
	latestLog, _ := s.store.LatestWorklog()

	return ProjectView{
		Manifest:      manifest,
		State:         state,
		Goals:         goals,
		Tasks:         tasks,
		Evaluation:    eval,
		LatestWorklog: latestLog,
	}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	staticSub, _ := fs.Sub(staticFS, "static")
	fileServer := http.FileServer(http.FS(staticSub))

	// Static files
	mux.Handle("/static/", http.StripPrefix("/static/", fileServer))

	// Root HTML
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		data, err := staticFS.ReadFile("static/index.html")
		if err != nil {
			http.Error(w, "Error loading UI", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(data)
	})

	// JSON API
	mux.HandleFunc("/api/project", func(w http.ResponseWriter, r *http.Request) {
		view, err := s.LoadView()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(view)
	})

	// Brief endpoint for 1-click clipboard copy
	mux.HandleFunc("/api/brief", func(w http.ResponseWriter, r *http.Request) {
		b, err := brief.Generate(s.store)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"brief": b.FormatTerminal(),
		})
	})

	// Task Action endpoint for Human-in-the-Loop supervision
	mux.HandleFunc("/api/task/update", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var payload struct {
			ID            string `json:"id"`
			Status        string `json:"status"`
			BlockedReason string `json:"blockedReason"`
			Result        string `json:"result"`
			ClearBlocker  bool   `json:"clearBlocker"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		if payload.ID == "" {
			http.Error(w, "Task ID is required", http.StatusBadRequest)
			return
		}
		task, err := s.store.GetTask(payload.ID)
		if err != nil {
			http.Error(w, "Task not found: "+err.Error(), http.StatusNotFound)
			return
		}
		if payload.Status != "" {
			task.Status = strings.ReplaceAll(strings.ToLower(strings.TrimSpace(payload.Status)), "-", "_")
		}
		if payload.BlockedReason != "" {
			task.BlockedReason = payload.BlockedReason
		}
		if payload.Result != "" {
			task.Result = payload.Result
		}
		if payload.ClearBlocker || (payload.Status != "" && payload.Status != "blocked" && payload.BlockedReason == "") {
			task.BlockedReason = ""
		}
		if err := s.store.UpdateTask(task); err != nil {
			http.Error(w, "Failed to update task: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"task":    task,
		})
	})

	// Server-Sent Events (SSE) for Real-Time UI updates
	mux.HandleFunc("/api/events", func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
			return
		}

		origin := r.Header.Get("Origin")
		if origin != "" {
			if strings.HasPrefix(origin, "http://127.0.0.1:") || strings.HasPrefix(origin, "http://localhost:") {
				w.Header().Set("Access-Control-Allow-Origin", origin)
			} else {
				http.Error(w, "Cross-origin request rejected", http.StatusForbidden)
				return
			}
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		messageChan := make(chan []byte, 10)
		s.mu.Lock()
		s.clients[messageChan] = true
		s.mu.Unlock()

		defer func() {
			s.mu.Lock()
			delete(s.clients, messageChan)
			close(messageChan)
			s.mu.Unlock()
		}()

		// Send initial payload immediately
		if view, err := s.LoadView(); err == nil {
			data, _ := json.Marshal(view)
			_, _ = fmt.Fprintf(w, "event: update\ndata: %s\n\n", data)
			flusher.Flush()
		}

		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()

		notify := r.Context().Done()
		for {
			select {
			case <-notify:
				return
			case <-ticker.C:
				_, _ = fmt.Fprintf(w, ": heartbeat\n\n")
				flusher.Flush()
			case msg := <-messageChan:
				_, _ = fmt.Fprintf(w, "event: update\ndata: %s\n\n", msg)
				flusher.Flush()
			}
		}
	})

	// Wrap with security headers middleware
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self' 'unsafe-inline'; connect-src 'self';")
		mux.ServeHTTP(w, r)
	})
}

func (s *Server) pollProjectChanges() {
	var lastStateTime time.Time
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			st, err := s.store.State()
			if err != nil {
				continue
			}
			if st.UpdatedAt.After(lastStateTime) {
				lastStateTime = st.UpdatedAt
				view, err := s.LoadView()
				if err == nil {
					data, _ := json.Marshal(view)
					s.mu.Lock()
					for client := range s.clients {
						select {
						case client <- data:
						default:
						}
					}
					s.mu.Unlock()
				}
			}
		}
	}
}
