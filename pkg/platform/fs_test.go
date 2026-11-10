package platform

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAtomicWriteAndReadJSON(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "keel-fs-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	type Sample struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}

	target := filepath.Join(tmpDir, "sub", "data.json")
	sample := Sample{Name: "Keel", Count: 42}

	if err := AtomicWriteJSON(target, sample); err != nil {
		t.Fatalf("AtomicWriteJSON failed: %v", err)
	}

	read, err := ReadJSON[Sample](target)
	if err != nil {
		t.Fatalf("ReadJSON failed: %v", err)
	}
	if read.Name != sample.Name || read.Count != sample.Count {
		t.Errorf("read mismatch: got %+v, want %+v", read, sample)
	}
}

func TestNormalizePathAndProjectRelative(t *testing.T) {
	norm := NormalizePath("a\\b/c\\d")
	if norm != "a/b/c/d" {
		t.Errorf("unexpected normalized path: %s", norm)
	}

	if !IsProjectRelative("tasks/task-1.json") {
		t.Error("expected tasks/task-1.json to be project relative")
	}
	if IsProjectRelative("../escaping.json") {
		t.Error("expected escaping path to be non-relative")
	}
	if IsProjectRelative("") {
		t.Error("expected empty path to be non-relative")
	}
}
