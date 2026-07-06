package terraform

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
)

// TerraformModel is the Tier 1 model: hclwrite.File IS the source of truth.
// file.Bytes() IS the serializer. The Index is a thin read-only cache.
type TerraformModel struct {
	File     *hclwrite.File // THE source of truth
	Index    *Index         // thin index for O(1) label lookup
	Title    string
	FilePath string
}

// NewModel creates a new empty TerraformModel.
func NewModel(title string) *TerraformModel {
	return &TerraformModel{
		File:  hclwrite.NewEmptyFile(),
		Index: NewIndex(),
		Title: title,
	}
}

// Bytes returns the HCL bytes from the underlying file. This IS the serializer.
func (m *TerraformModel) Bytes() []byte {
	return m.File.Bytes()
}

// Snapshot returns a copy of the current HCL bytes for undo.
func (m *TerraformModel) Snapshot() []byte {
	return m.File.Bytes()
}

// Restore replaces the model's file from HCL bytes and rebuilds the index.
func (m *TerraformModel) Restore(data []byte) error {
	f, diags := hclwrite.ParseConfig(data, "", hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return fmt.Errorf("parse error: %s", diags.Error())
	}
	m.File = f
	m.Index.Rebuild(f)
	return nil
}

// AddResource adds a resource block: resource "type" "label" { attrs... }
func (m *TerraformModel) AddResource(fullType, label string, attrs map[string]string, quotedParams map[string]bool) (*BlockRef, error) {
	return m.addBlock("resource", fullType, label, attrs, quotedParams)
}

// AddDataSource adds: data "type" "label" { attrs... }
func (m *TerraformModel) AddDataSource(fullType, label string, attrs map[string]string, quotedParams map[string]bool) (*BlockRef, error) {
	return m.addBlock("data", fullType, label, attrs, quotedParams)
}

// AddVariable adds: variable "name" { type, default, description }
func (m *TerraformModel) AddVariable(name string, attrs map[string]string) (*BlockRef, error) {
	return m.addBlock("variable", "variable", name, attrs, nil)
}

// AddOutput adds: output "name" { value, description }
func (m *TerraformModel) AddOutput(name string, attrs map[string]string) (*BlockRef, error) {
	return m.addBlock("output", "output", name, attrs, nil)
}

// AddProvider adds: provider "name" { region, ... }
func (m *TerraformModel) AddProvider(name string, attrs map[string]string) (*BlockRef, error) {
	return m.addBlock("provider", name, name, attrs, nil)
}

// AddModule adds: module "label" { source, ... }
func (m *TerraformModel) AddModule(label string, attrs map[string]string) (*BlockRef, error) {
	return m.addBlock("module", "module", label, attrs, nil)
}

// addBlock is the internal method that creates any block type.
func (m *TerraformModel) addBlock(kind, fullType, label string, attrs map[string]string, quotedParams map[string]bool) (*BlockRef, error) {
	// Build HCL block labels based on kind
	var hclLabels []string
	switch kind {
	case "resource", "data":
		hclLabels = []string{fullType, label}
	case "variable", "output", "module":
		hclLabels = []string{label}
	case "provider":
		hclLabels = []string{fullType}
	}

	// Check for duplicate in index
	ref := &BlockRef{
		Label:    label,
		Kind:     kind,
		FullType: fullType,
		Provider: DeriveProvider(fullType),
		Tags:     make(map[string]string),
	}

	if err := m.Index.Add(ref); err != nil {
		return nil, err
	}

	// Add newline before block if file is not empty
	if len(m.File.Body().Blocks()) > 0 || len(m.File.Body().Attributes()) > 0 {
		m.File.Body().AppendNewline()
	}

	// Create the HCL block
	block := m.File.Body().AppendNewBlock(kind, hclLabels)
	ref.Block = block

	// Set attributes
	if attrs != nil {
		body := block.Body()
		for key, value := range attrs {
			forceStr := quotedParams != nil && quotedParams[key]
			// Variable "type" attribute is always a raw expression
			if kind == "variable" && key == "type" {
				body.SetAttributeRaw(key, rawTokens(value))
				continue
			}
			SetAttribute(body, key, value, forceStr)
		}
	}

	return ref, nil
}

// RemoveBlock removes a block by label from both the model and the HCL file.
func (m *TerraformModel) RemoveBlock(label string) (*BlockRef, error) {
	ref := m.Index.Get(label)
	if ref == nil {
		// Try qualified lookup
		ref = m.Index.GetByQualified(label)
	}
	if ref == nil {
		return nil, fmt.Errorf("block %q not found", label)
	}

	// Remove from hclwrite.File
	m.File.Body().RemoveBlock(ref.Block)

	// Remove from index
	m.Index.Remove(ref.Label)

	return ref, nil
}

// SetAttributes sets key:value pairs on an existing block.
func (m *TerraformModel) SetAttributes(label string, attrs map[string]string, quotedParams map[string]bool) error {
	ref := m.resolveRef(label)
	if ref == nil {
		return fmt.Errorf("block %q not found", label)
	}

	body := ref.Block.Body()
	for key, value := range attrs {
		forceStr := quotedParams != nil && quotedParams[key]
		if ref.Kind == "variable" && key == "type" {
			body.SetAttributeRaw(key, rawTokens(value))
			continue
		}
		SetAttribute(body, key, value, forceStr)
	}
	return nil
}

// UnsetAttributes removes keys from a block.
func (m *TerraformModel) UnsetAttributes(label string, keys []string) error {
	ref := m.resolveRef(label)
	if ref == nil {
		return fmt.Errorf("block %q not found", label)
	}

	body := ref.Block.Body()
	for _, key := range keys {
		body.RemoveAttribute(key)
	}
	return nil
}

// Connect adds a logical connection between two blocks and emits a real
// depends_on = [...] entry on the source block's HCL body, so the edge is
// not just bookkeeping for the `graph` query — it actually affects the
// generated configuration.
func (m *TerraformModel) Connect(src, dst, edgeLabel string) error {
	srcRef := m.resolveRef(src)
	if srcRef == nil {
		return fmt.Errorf("source block %q not found", src)
	}
	dstRef := m.resolveRef(dst)
	if dstRef == nil {
		return fmt.Errorf("target block %q not found", dst)
	}

	srcKey := strings.ToLower(srcRef.Label)
	dstKey := strings.ToLower(dstRef.Label)

	if m.Index.Connections[srcKey] == nil {
		m.Index.Connections[srcKey] = make(map[string]string)
	}
	m.Index.Connections[srcKey][dstKey] = edgeLabel

	m.syncDependsOn(srcRef)
	return nil
}

// Disconnect removes a logical connection between two blocks and re-renders
// the source block's depends_on to match the remaining edges.
func (m *TerraformModel) Disconnect(src, dst string) error {
	srcRef := m.resolveRef(src)
	if srcRef == nil {
		return fmt.Errorf("source block %q not found", src)
	}
	dstRef := m.resolveRef(dst)
	if dstRef == nil {
		return fmt.Errorf("target block %q not found", dst)
	}

	srcKey := strings.ToLower(srcRef.Label)
	dstKey := strings.ToLower(dstRef.Label)

	if targets, ok := m.Index.Connections[srcKey]; ok {
		delete(targets, dstKey)
		if len(targets) == 0 {
			delete(m.Index.Connections, srcKey)
		}
	}

	m.syncDependsOn(srcRef)
	return nil
}

// syncDependsOn rewrites the depends_on attribute on ref's HCL block from
// the full current set of ref's outgoing connections. Rebuilding the whole
// list (rather than trying to patch individual tokens in place) means
// Connect naturally appends to whatever was already there and Disconnect
// naturally drops just its own entry, while staying correct regardless of
// how the attribute was last written.
func (m *TerraformModel) syncDependsOn(ref *BlockRef) {
	key := strings.ToLower(ref.Label)
	targets := m.Index.Connections[key]
	body := ref.Block.Body()

	dstKeys := make([]string, 0, len(targets))
	for dst := range targets {
		dstKeys = append(dstKeys, dst)
	}
	sort.Strings(dstKeys)

	var tokenGroups []hclwrite.Tokens
	for _, dstKey := range dstKeys {
		dstRef := m.resolveConnectionTarget(dstKey)
		if dstRef == nil {
			continue // stale edge (e.g. target label now ambiguous); skip rather than corrupt HCL
		}
		tokenGroups = append(tokenGroups, hclwrite.TokensForTraversal(dependencyTraversal(dstRef)))
	}

	if len(tokenGroups) == 0 {
		body.RemoveAttribute("depends_on")
		return
	}

	body.SetAttributeRaw("depends_on", hclwrite.TokensForTuple(tokenGroups))
}

// resolveConnectionTarget looks up a block by the lowercased label used as
// the key in Index.Connections, falling back to a scan for the case where
// the label has since become ambiguous across multiple block types.
func (m *TerraformModel) resolveConnectionTarget(lowerLabel string) *BlockRef {
	if ref := m.Index.Get(lowerLabel); ref != nil {
		return ref
	}
	for _, ref := range m.Index.ByQualified {
		if strings.ToLower(ref.Label) == lowerLabel {
			return ref
		}
	}
	return nil
}

// dependencyTraversal builds the hcl.Traversal used to reference ref inside
// a depends_on = [...] expression, following Terraform's addressing rules
// per block kind: resource -> TYPE.LABEL, data -> data.TYPE.LABEL,
// module -> module.LABEL, variable -> var.LABEL.
func dependencyTraversal(ref *BlockRef) hcl.Traversal {
	var parts []string
	switch ref.Kind {
	case "data":
		parts = []string{"data", ref.FullType, ref.Label}
	case "module":
		parts = []string{"module", ref.Label}
	case "variable":
		parts = []string{"var", ref.Label}
	default: // resource, provider, or anything else addressed as TYPE.LABEL
		parts = []string{ref.FullType, ref.Label}
	}

	traversal := hcl.Traversal{hcl.TraverseRoot{Name: parts[0]}}
	for _, p := range parts[1:] {
		traversal = append(traversal, hcl.TraverseAttr{Name: p})
	}
	return traversal
}

// resolveRef finds a BlockRef by label, trying plain label then qualified format.
func (m *TerraformModel) resolveRef(label string) *BlockRef {
	ref := m.Index.Get(label)
	if ref != nil {
		return ref
	}
	return m.Index.GetByQualified(label)
}
