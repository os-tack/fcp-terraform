package terraform

import (
	"fmt"
	"os"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"

	"github.com/os-tack/fcp-terraform/internal/fcpcore"
)

// SnapshotEvent captures before/after HCL bytes for undo/redo.
type SnapshotEvent struct {
	Before  []byte
	After   []byte
	Summary string
}

// TerraformAdapter implements fcpcore.SessionHooks, bridging Session to TerraformModel.
type TerraformAdapter struct{}

// NewAdapter creates a new TerraformAdapter.
func NewAdapter() *TerraformAdapter {
	return &TerraformAdapter{}
}

// OnNew creates a new TerraformModel with the given params.
func (a *TerraformAdapter) OnNew(params map[string]string) (any, error) {
	title := params["title"]
	if title == "" {
		title = "Untitled"
	}
	return NewModel(title), nil
}

// OnOpen reads a .tf file, parses it with hclwrite, and builds a TerraformModel.
func (a *TerraformAdapter) OnOpen(path string) (any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read %q: %w", path, err)
	}

	f, diags := hclwrite.ParseConfig(data, path, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return nil, fmt.Errorf("parse error: %s", diags.Error())
	}

	model := &TerraformModel{
		File:     f,
		Index:    NewIndex(),
		Title:    pathToTitle(path),
		FilePath: path,
	}
	model.Index.Rebuild(f)
	return model, nil
}

// OnSave writes model.Bytes() to the given file path.
func (a *TerraformAdapter) OnSave(modelAny any, path string) error {
	model, ok := modelAny.(*TerraformModel)
	if !ok {
		return fmt.Errorf("invalid model type")
	}
	model.FilePath = path
	return os.WriteFile(path, model.Bytes(), 0644)
}

// OnRebuildIndices rebuilds the index from the hclwrite file.
func (a *TerraformAdapter) OnRebuildIndices(modelAny any) {
	model, ok := modelAny.(*TerraformModel)
	if !ok {
		return
	}
	model.Index.Rebuild(model.File)
}

// GetDigest returns a compact summary of the model state.
func (a *TerraformAdapter) GetDigest(modelAny any) string {
	model, ok := modelAny.(*TerraformModel)
	if !ok {
		return "invalid model"
	}

	counts := make(map[string]int)
	for _, label := range model.Index.Order {
		ref := model.Index.Get(label)
		if ref != nil {
			counts[ref.Kind]++
		}
	}

	parts := []string{fmt.Sprintf("%d blocks", len(model.Index.Order))}
	for _, kind := range []string{"resource", "data", "variable", "output", "provider", "module"} {
		if c, ok := counts[kind]; ok {
			parts = append(parts, fmt.Sprintf("%d %s", c, kind))
		}
	}
	return strings.Join(parts, ", ")
}

// DispatchOp dispatches a parsed op against the model, capturing snapshots.
// Returns the result string and a SnapshotEvent for undo/redo.
func (a *TerraformAdapter) DispatchOp(op *fcpcore.ParsedOp, model *TerraformModel) (string, *SnapshotEvent) {
	before := model.Snapshot()
	result := Dispatch(op, model)
	after := model.Snapshot()

	event := &SnapshotEvent{
		Before:  before,
		After:   after,
		Summary: op.Raw,
	}
	return result, event
}

// ReverseSnapshot restores the model to the "before" state of a SnapshotEvent.
func ReverseSnapshot(event any, modelAny any) {
	se, ok := event.(*SnapshotEvent)
	if !ok {
		return
	}
	model, ok := modelAny.(*TerraformModel)
	if !ok {
		return
	}
	model.Restore(se.Before)
}

// ReplaySnapshot restores the model to the "after" state of a SnapshotEvent.
func ReplaySnapshot(event any, modelAny any) {
	se, ok := event.(*SnapshotEvent)
	if !ok {
		return
	}
	model, ok := modelAny.(*TerraformModel)
	if !ok {
		return
	}
	model.Restore(se.After)
}

// NewTerraformSession creates a fully wired fcpcore.Session for Terraform.
func NewTerraformSession() (*fcpcore.Session, *TerraformAdapter) {
	adapter := NewAdapter()
	session := fcpcore.NewSession(adapter, ReverseSnapshot, ReplaySnapshot)
	return session, adapter
}

// pathToTitle extracts a title from a file path.
func pathToTitle(path string) string {
	// Strip directory
	idx := strings.LastIndex(path, "/")
	if idx >= 0 {
		path = path[idx+1:]
	}
	// Strip .tf extension
	if strings.HasSuffix(path, ".tf") {
		path = path[:len(path)-3]
	}
	return path
}
