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
}
