package dashboard

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"keel/pkg/model"
	"keel/pkg/store"
)

func TestDashboardEndpoints(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "keel-dashboard-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	s, err := store.Init(tmpDir, "Dashboard Test", "Testing dashboard HTTP server")
	if err != nil {
		t.Fatal(err)
	}

	_ = s.CreateTask(model.Task{ID: "t-1", Title: "Task One", Status: "pending"})

	srv := NewServer(s)
	handler := srv.Handler()

	// 1. Root HTML
	reqRoot := httptest.NewRequest("GET", "/", nil)
	wRoot := httptest.NewRecorder()
	handler.ServeHTTP(wRoot, reqRoot)

	if wRoot.Code != http.StatusOK {
		t.Errorf("expected 200 for /, got %d", wRoot.Code)
	}
	if !strings.Contains(wRoot.Body.String(), "Keel · Mission Control") {
		t.Errorf("HTML title not found in response body")
	}
	if wRoot.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("missing nosniff security header")
	}

	// 2. /api/project JSON
	reqAPI := httptest.NewRequest("GET", "/api/project", nil)
	wAPI := httptest.NewRecorder()
	handler.ServeHTTP(wAPI, reqAPI)

	if wAPI.Code != http.StatusOK {
		t.Errorf("expected 200 for /api/project, got %d", wAPI.Code)
	}
	var view ProjectView
	if err := json.Unmarshal(wAPI.Body.Bytes(), &view); err != nil {
		t.Fatalf("failed to unmarshal ProjectView: %v", err)
	}
	if len(view.Tasks) != 1 || view.Tasks[0].ID != "t-1" {
		t.Errorf("expected task t-1 in view, got: %+v", view.Tasks)
	}
	if len(view.Evaluation.Ready) != 1 {
		t.Errorf("expected evaluated ready task, got: %+v", view.Evaluation.Ready)
	}

	// 3. /api/events SSE CORS Check
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	reqSSE := httptest.NewRequest("GET", "/api/events", nil).WithContext(ctx)
	reqSSE.Header.Set("Origin", "http://127.0.0.1:7788")
	wSSE := httptest.NewRecorder()

	handler.ServeHTTP(wSSE, reqSSE)

	if wSSE.Header().Get("Content-Type") != "text/event-stream" {
		t.Errorf("expected text/event-stream, got %s", wSSE.Header().Get("Content-Type"))
	}
	if wSSE.Header().Get("Access-Control-Allow-Origin") != "http://127.0.0.1:7788" {
		t.Errorf("CORS allow origin mismatch, got %s", wSSE.Header().Get("Access-Control-Allow-Origin"))
	}

	// 4. Reject foreign CORS
	reqBadOrigin := httptest.NewRequest("GET", "/api/events", nil)
	reqBadOrigin.Header.Set("Origin", "https://malicious-site.com")
	wBadOrigin := httptest.NewRecorder()
	handler.ServeHTTP(wBadOrigin, reqBadOrigin)

	if wBadOrigin.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for bad origin, got %d", wBadOrigin.Code)
	}

	// 5. /api/brief endpoint
	reqBrief := httptest.NewRequest("GET", "/api/brief", nil)
	wBrief := httptest.NewRecorder()
	handler.ServeHTTP(wBrief, reqBrief)

	if wBrief.Code != http.StatusOK {
		t.Errorf("expected 200 for /api/brief, got %d", wBrief.Code)
	}
	var briefPayload map[string]string
	if err := json.Unmarshal(wBrief.Body.Bytes(), &briefPayload); err != nil {
		t.Fatalf("failed to unmarshal brief payload: %v", err)
	}
	if !strings.Contains(briefPayload["brief"], "Project Brief") {
		t.Errorf("expected brief to contain Project Brief, got: %s", briefPayload["brief"])
	}

	// 6. Test /api/task/update endpoint
	updatePayload := `{"id":"t-1","status":"in_progress","result":"WIP"}`
	reqUpdate := httptest.NewRequest("POST", "/api/task/update", strings.NewReader(updatePayload))
	wUpdate := httptest.NewRecorder()
	handler.ServeHTTP(wUpdate, reqUpdate)

	if wUpdate.Code != http.StatusOK {
		t.Errorf("expected 200 for /api/task/update, got %d: %s", wUpdate.Code, wUpdate.Body.String())
	}
	updatedTask, err := s.GetTask("t-1")
	if err != nil || updatedTask.Status != "in_progress" {
		t.Errorf("expected task status in_progress, got %v, err: %v", updatedTask, err)
	}

	// 7. Test /api/task/create endpoint
	createPayload := `{"title":"Create API Route","priority":"high","assignee":"human"}`
	reqCreate := httptest.NewRequest("POST", "/api/task/create", strings.NewReader(createPayload))
	wCreate := httptest.NewRecorder()
	handler.ServeHTTP(wCreate, reqCreate)

	if wCreate.Code != http.StatusCreated {
		t.Errorf("expected 201 for /api/task/create, got %d: %s", wCreate.Code, wCreate.Body.String())
	}
	createdTask, err := s.GetTask("create-api-route")
	if err != nil || createdTask.Title != "Create API Route" {
		t.Errorf("expected created task, got %v, err: %v", createdTask, err)
	}

	// 8. Test cross-origin security on mutation endpoints
	reqBadPost := httptest.NewRequest("POST", "/api/task/create", strings.NewReader(createPayload))
	reqBadPost.Header.Set("Origin", "https://evil-site.com")
	wBadPost := httptest.NewRecorder()
	handler.ServeHTTP(wBadPost, reqBadPost)
	if wBadPost.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for bad origin on POST, got %d", wBadPost.Code)
	}

	// 9. Test srv.Notify() and Close()
	srv.Notify()
	srv.Close()
	// Multiple close should be safe
	srv.Close()
}

func TestDashboardCycleDetectionNotice(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "keel-dashboard-cycle-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	s, err := store.Init(tmpDir, "Cycle Dashboard Test", "Testing cycle detection notice")
	if err != nil {
		t.Fatal(err)
	}

	_ = s.CreateTask(model.Task{ID: "t-a", Title: "Task A", Status: "pending", DependsOn: []string{"t-b"}})
	_ = s.CreateTask(model.Task{ID: "t-b", Title: "Task B", Status: "pending", DependsOn: []string{"t-a"}})

	srv := NewServer(s)
	defer srv.Close()

	req := httptest.NewRequest("GET", "/api/project", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var view ProjectView
	if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.ToLower(view.GraphError), "dependency cycle detected") {
		t.Errorf("expected GraphError to report dependency cycle, got %q", view.GraphError)
	}
}
