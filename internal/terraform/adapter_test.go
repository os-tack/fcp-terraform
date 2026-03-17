package terraform

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/os-tack/fcp-terraform/internal/fcpcore"
)

// ── adapter hooks ────────────────────────────────────────

func TestAdapterOnNew(t *testing.T) {
	adapter := NewAdapter()
	modelAny, err := adapter.OnNew(map[string]string{"title": "MyProject"})
	if err != nil {
		t.Fatalf("OnNew error: %v", err)
	}
	model, ok := modelAny.(*TerraformModel)
	if !ok {
		t.Fatal("OnNew did not return *TerraformModel")
	}
	if model.Title != "MyProject" {
		t.Errorf("Title = %q, want MyProject", model.Title)
	}
}

func TestAdapterOnNew_DefaultTitle(t *testing.T) {
	adapter := NewAdapter()
	modelAny, err := adapter.OnNew(map[string]string{})
	if err != nil {
		t.Fatalf("OnNew error: %v", err)
	}
	model := modelAny.(*TerraformModel)
	if model.Title != "Untitled" {
		t.Errorf("Title = %q, want Untitled", model.Title)
	}
}

func TestAdapterSaveAndOpen(t *testing.T) {
	adapter := NewAdapter()

	// Create model with a resource
	modelAny, _ := adapter.OnNew(map[string]string{"title": "SaveTest"})
	model := modelAny.(*TerraformModel)
	model.AddResource("aws_instance", "web", map[string]string{"ami": "ami-abc"}, nil)

	// Save to temp file
	dir := t.TempDir()
	path := filepath.Join(dir, "test.tf")
	err := adapter.OnSave(model, path)
	if err != nil {
		t.Fatalf("OnSave error: %v", err)
	}

	// Verify file was written
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if !strings.Contains(string(data), "aws_instance") {
		t.Errorf("saved file should contain aws_instance, got:\n%s", data)
	}

	// Open the saved file
	openedAny, err := adapter.OnOpen(path)
	if err != nil {
		t.Fatalf("OnOpen error: %v", err)
	}
	opened := openedAny.(*TerraformModel)
	if opened.Index.Get("web") == nil {
		t.Error("opened model should contain 'web' block")
	}
	if opened.FilePath != path {
		t.Errorf("FilePath = %q, want %q", opened.FilePath, path)
	}
}

func TestAdapterOnOpen_NotFound(t *testing.T) {
	adapter := NewAdapter()
	_, err := adapter.OnOpen("/nonexistent/path.tf")
	if err == nil {
		t.Error("OnOpen should error for nonexistent file")
	}
}

func TestAdapterRebuildIndices(t *testing.T) {
	adapter := NewAdapter()
	modelAny, _ := adapter.OnNew(map[string]string{"title": "rebuild"})
	model := modelAny.(*TerraformModel)
	model.AddResource("aws_instance", "a", nil, nil)
	model.AddResource("aws_vpc", "b", nil, nil)

	// Clear index manually
	model.Index = NewIndex()
	if model.Index.Get("a") != nil {
		t.Fatal("index should be empty after clear")
	}

	// Rebuild
	adapter.OnRebuildIndices(model)
	if model.Index.Get("a") == nil {
		t.Error("after rebuild, 'a' should be in index")
	}
	if model.Index.Get("b") == nil {
		t.Error("after rebuild, 'b' should be in index")
	}
}

func TestAdapterGetDigest(t *testing.T) {
	adapter := NewAdapter()
	modelAny, _ := adapter.OnNew(map[string]string{"title": "digest"})
	model := modelAny.(*TerraformModel)
	model.AddResource("aws_instance", "web", nil, nil)
	model.AddVariable("region", map[string]string{"type": "string"})

	digest := adapter.GetDigest(model)
	if !strings.Contains(digest, "2 blocks") {
		t.Errorf("digest = %q, want '2 blocks'", digest)
	}
	if !strings.Contains(digest, "1 resource") {
		t.Errorf("digest = %q, want '1 resource'", digest)
	}
}

// ── dispatch with snapshots ──────────────────────────────

func TestAdapterDispatchOp(t *testing.T) {
	adapter := NewAdapter()
	model := NewModel("dispatch-test")

	op := &fcpcore.ParsedOp{
		Verb:        "add",
		Positionals: []string{"resource", "aws_instance", "web"},
		Params:      map[string]string{"ami": "ami-xyz"},
		Raw:         "add resource aws_instance web ami:ami-xyz",
	}

	result, event := adapter.DispatchOp(op, model)
	if !strings.HasPrefix(result, "+") {
		t.Errorf("result = %q, want + prefix", result)
	}
	if event == nil {
		t.Fatal("event should not be nil")
	}
	if event.Summary != op.Raw {
		t.Errorf("event.Summary = %q, want %q", event.Summary, op.Raw)
	}
	if len(event.Before) >= len(event.After) {
		t.Error("after snapshot should be larger than before (resource was added)")
	}
}

// ── snapshot undo/redo ───────────────────────────────────

func TestSnapshotUndoRedo(t *testing.T) {
	model := NewModel("undo-redo")

	// Capture before state
	before := model.Snapshot()

	// Add a resource
	model.AddResource("aws_instance", "test", map[string]string{"ami": "x"}, nil)
	after := model.Snapshot()

	// Verify resource exists
	if model.Index.Get("test") == nil {
		t.Fatal("resource should exist after add")
	}

	// Undo via ReverseSnapshot
	event := &SnapshotEvent{Before: before, After: after, Summary: "add resource"}
	ReverseSnapshot(event, model)
	if model.Index.Get("test") != nil {
		t.Error("resource should be gone after undo")
	}

	// Redo via ReplaySnapshot
	ReplaySnapshot(event, model)
	if model.Index.Get("test") == nil {
		t.Error("resource should be back after redo")
	}
}

// ── session integration ──────────────────────────────────

func TestNewTerraformSession(t *testing.T) {
	session, _ := NewTerraformSession()

	// Create new session
	result := session.Dispatch(`new "TestProject"`)
	if !strings.Contains(result, "TestProject") {
		t.Errorf("new session = %q", result)
	}
	if session.Model == nil {
		t.Fatal("model should not be nil after new")
	}
}

func TestSessionFullWorkflow(t *testing.T) {
	session, adapter := NewTerraformSession()

	// 1. New session
	session.Dispatch(`new "Integration"`)
	model := session.Model.(*TerraformModel)

	// 2. Add resources via adapter
	op1 := &fcpcore.ParsedOp{
		Verb:        "add",
		Positionals: []string{"resource", "aws_vpc", "main"},
		Params:      map[string]string{"cidr_block": "10.0.0.0/16"},
		Raw:         "add resource aws_vpc main cidr_block:10.0.0.0/16",
	}
	result, event := adapter.DispatchOp(op1, model)
	if !strings.HasPrefix(result, "+") {
		t.Errorf("add vpc result = %q", result)
	}
	session.Log.Append(event)

	op2 := &fcpcore.ParsedOp{
		Verb:        "add",
		Positionals: []string{"resource", "aws_instance", "web"},
		Params:      map[string]string{"ami": "ami-abc"},
		Raw:         "add resource aws_instance web ami:ami-abc",
	}
	result2, event2 := adapter.DispatchOp(op2, model)
	if !strings.HasPrefix(result2, "+") {
		t.Errorf("add instance result = %q", result2)
	}
	session.Log.Append(event2)

	// 3. Verify both blocks exist
	if model.Index.Get("main") == nil || model.Index.Get("web") == nil {
		t.Error("both blocks should exist")
	}

	// 4. Undo last add
	undoResult := session.Dispatch("undo")
	if !strings.Contains(undoResult, "undone") {
		t.Errorf("undo = %q", undoResult)
	}

	// After undo, web should be gone, main should remain
	if model.Index.Get("web") != nil {
		t.Error("web should be gone after undo")
	}
	if model.Index.Get("main") == nil {
		t.Error("main should still exist after undo")
	}

	// 5. Redo
	redoResult := session.Dispatch("redo")
	if !strings.Contains(redoResult, "redone") {
		t.Errorf("redo = %q", redoResult)
	}
	if model.Index.Get("web") == nil {
		t.Error("web should be back after redo")
	}

	// 6. Save to temp file
	dir := t.TempDir()
	path := filepath.Join(dir, "output.tf")
	saveResult := session.Dispatch("save as:" + path)
	if !strings.Contains(saveResult, "saved") {
		t.Errorf("save = %q", saveResult)
	}

	// 7. Verify saved file
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	hcl := string(data)
	if !strings.Contains(hcl, "aws_vpc") || !strings.Contains(hcl, "aws_instance") {
		t.Errorf("saved HCL should contain both blocks, got:\n%s", hcl)
	}

	// 8. Query plan
	queryResult := DispatchQuery("plan", model, session.Log)
	if !strings.Contains(queryResult, "aws_instance") {
		t.Errorf("plan query should show instance, got:\n%s", queryResult)
	}

	// 9. History
	historyResult := DispatchQuery("history", model, session.Log)
	if !strings.Contains(historyResult, "add resource aws_vpc") {
		t.Errorf("history should show add vpc, got:\n%s", historyResult)
	}
}

// ── pathToTitle ──────────────────────────────────────────

func TestPathToTitle(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/tmp/main.tf", "main"},
		{"output.tf", "output"},
		{"/home/user/infra/network.tf", "network"},
		{"noext", "noext"},
	}
	for _, tt := range tests {
		got := pathToTitle(tt.path)
		if got != tt.want {
			t.Errorf("pathToTitle(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

// ── verb specs ───────────────────────────────────────────

func TestVerbSpecs(t *testing.T) {
	specs := TerraformVerbSpecs()
	if len(specs) != 16 {
		t.Errorf("TerraformVerbSpecs() = %d specs, want 16", len(specs))
	}

	// Check categories exist
	cats := make(map[string]bool)
	for _, s := range specs {
		cats[s.Category] = true
	}
	for _, want := range []string{"resources", "connections", "editing"} {
		if !cats[want] {
			t.Errorf("missing category %q", want)
		}
	}
}

func TestReferenceCard(t *testing.T) {
	registry := fcpcore.NewVerbRegistry()
	registry.RegisterMany(TerraformVerbSpecs())
	card := registry.GenerateReferenceCard(ExtraSections())

	if !strings.Contains(card, "RESOURCES:") {
		t.Error("reference card should contain RESOURCES section")
	}
	if !strings.Contains(card, "CONNECTIONS:") {
		t.Error("reference card should contain CONNECTIONS section")
	}
	if !strings.Contains(card, "EDITING:") {
		t.Error("reference card should contain EDITING section")
	}
	if !strings.Contains(card, "@type:aws_instance") {
		t.Error("reference card should contain selector examples")
	}
}
