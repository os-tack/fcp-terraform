package terraform

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/os-tack/fcp-terraform/internal/fcpcore"
)

// DispatchQuery routes a query string to the appropriate handler.
func DispatchQuery(query string, model *TerraformModel, eventLog *fcpcore.EventLog) string {
	parts := strings.Fields(query)
	if len(parts) == 0 {
		return unknownQuery("")
	}

	cmd := strings.ToLower(parts[0])
	args := parts[1:]

	switch cmd {
	case "plan":
		return queryPlan(model)
	case "graph":
		return queryGraph(model)
	case "describe":
		return queryDescribe(model, args)
	case "stats":
		return queryStats(model)
	case "map":
		return queryMap(model)
	case "status":
		return queryStatus(model)
	case "history":
		return queryHistory(eventLog, args)
	case "list":
		return queryList(model, args)
	case "find":
		return queryFind(model, strings.Join(args, " "))
	default:
		return unknownQuery(cmd)
	}
}

func unknownQuery(cmd string) string {
	available := "plan, graph, describe, stats, map, status, history, list, find"
	if cmd == "" {
		return "Query required. Available: " + available
	}
	return fmt.Sprintf("Unknown query %q. Available: %s", cmd, available)
}

// ── plan ─────────────────────────────────────────────────

func queryPlan(model *TerraformModel) string {
	b := model.Bytes()
	if len(strings.TrimSpace(string(b))) == 0 {
		return "Empty configuration. Add some resources first."
	}
	return string(b)
}

// ── graph ────────────────────────────────────────────────

func queryGraph(model *TerraformModel) string {
	if len(model.Index.Connections) == 0 {
		return "No connections. Use 'connect SRC -> TGT' to add dependencies."
	}

	lines := []string{"Dependency Graph:"}
	for src, targets := range model.Index.Connections {
		for dst, edgeLabel := range targets {
			suffix := ""
			if edgeLabel != "" {
				suffix = " (" + edgeLabel + ")"
			}
			lines = append(lines, fmt.Sprintf("  %s -> %s%s", src, dst, suffix))
		}
	}
	return strings.Join(lines, "\n")
}

// ── describe ─────────────────────────────────────────────

func queryDescribe(model *TerraformModel, args []string) string {
	if len(args) == 0 {
		return "describe requires a LABEL"
	}

	label := args[0]
	ref := model.resolveRef(label)
	if ref == nil {
		return fmt.Sprintf("block %q not found", label)
	}

	lines := []string{}

	// Header
	var header string
	if ref.Kind == "resource" || ref.Kind == "data" {
		header = fmt.Sprintf("%s %s.%s", ref.Kind, ref.FullType, ref.Label)
	} else {
		header = fmt.Sprintf("%s %q", ref.Kind, ref.Label)
	}
	lines = append(lines, header)

	if ref.Provider != "" && ref.Kind != "provider" {
		lines = append(lines, fmt.Sprintf("  provider: %s", ref.Provider))
	}

	// Attributes from hclwrite
	body := ref.Block.Body()
	attrs := body.Attributes()
	if len(attrs) > 0 {
		lines = append(lines, "  attributes:")
		for name, attr := range attrs {
			valBytes := strings.TrimSpace(string(attr.Expr().BuildTokens(nil).Bytes()))
			lines = append(lines, fmt.Sprintf("    %s = %s", name, valBytes))
		}
	}

	// Tags
	if len(ref.Tags) > 0 {
		lines = append(lines, "  tags:")
		for k, v := range ref.Tags {
			lines = append(lines, fmt.Sprintf("    %s = %q", k, v))
		}
	}

	// Nested blocks
	nested := body.Blocks()
	if len(nested) > 0 {
		lines = append(lines, "  nested blocks:")
		for _, nb := range nested {
			lines = append(lines, fmt.Sprintf("    %s {}", nb.Type()))
			for name, attr := range nb.Body().Attributes() {
				valBytes := strings.TrimSpace(string(attr.Expr().BuildTokens(nil).Bytes()))
				lines = append(lines, fmt.Sprintf("      %s = %s", name, valBytes))
			}
		}
	}

	// Connections
	srcKey := strings.ToLower(ref.Label)
	outgoing := model.Index.Connections[srcKey]
	var incoming []string
	for src, targets := range model.Index.Connections {
		if _, ok := targets[srcKey]; ok && src != srcKey {
			incoming = append(incoming, src)
		}
	}

	if len(outgoing) > 0 || len(incoming) > 0 {
		lines = append(lines, "  connections:")
		for dst := range outgoing {
			lines = append(lines, fmt.Sprintf("    -> %s", dst))
		}
		for _, src := range incoming {
			lines = append(lines, fmt.Sprintf("    <- %s", src))
		}
	}

	return strings.Join(lines, "\n")
}

// ── stats ────────────────────────────────────────────────

func queryStats(model *TerraformModel) string {
	counts := make(map[string]int)
	for _, label := range model.Index.Order {
		ref := model.Index.Get(label)
		if ref != nil {
			counts[ref.Kind]++
		}
	}

	total := len(model.Index.Order)
	lines := []string{fmt.Sprintf("Total blocks: %d", total)}

	// Show counts by kind in a consistent order
	for _, kind := range []string{"resource", "data", "variable", "output", "provider", "module"} {
		if c, ok := counts[kind]; ok {
			lines = append(lines, fmt.Sprintf("  %s: %d", kind, c))
		}
	}

	// Unique providers
	if len(model.Index.ByProvider) > 0 {
		providers := make([]string, 0, len(model.Index.ByProvider))
		for p := range model.Index.ByProvider {
			providers = append(providers, p)
		}
		lines = append(lines, fmt.Sprintf("Providers: %s", strings.Join(providers, ", ")))
	}

	connCount := 0
	for _, targets := range model.Index.Connections {
		connCount += len(targets)
	}
	lines = append(lines, fmt.Sprintf("Connections: %d", connCount))

	return strings.Join(lines, "\n")
}

// ── map ──────────────────────────────────────────────────

func queryMap(model *TerraformModel) string {
	lines := []string{fmt.Sprintf("Terraform Config: %s", model.Title), ""}

	providers := model.Index.FindByKind("provider")
	resources := model.Index.FindByKind("resource")
	dataSources := model.Index.FindByKind("data")
	variables := model.Index.FindByKind("variable")
	outputs := model.Index.FindByKind("output")
	modules := model.Index.FindByKind("module")

	if len(providers) > 0 {
		names := make([]string, len(providers))
		for i, p := range providers {
			names[i] = p.Label
		}
		lines = append(lines, fmt.Sprintf("Providers: %s", strings.Join(names, ", ")))
	}

	if len(resources) > 0 {
		typeCounts := make(map[string]int)
		for _, r := range resources {
			typeCounts[r.FullType]++
		}
		parts := make([]string, 0, len(typeCounts))
		for t, c := range typeCounts {
			parts = append(parts, fmt.Sprintf("%s x%d", t, c))
		}
		lines = append(lines, fmt.Sprintf("Resources (%d): %s", len(resources), strings.Join(parts, ", ")))
	}

	if len(dataSources) > 0 {
		lines = append(lines, fmt.Sprintf("Data Sources: %d", len(dataSources)))
	}
	if len(variables) > 0 {
		lines = append(lines, fmt.Sprintf("Variables: %d", len(variables)))
	}
	if len(outputs) > 0 {
		lines = append(lines, fmt.Sprintf("Outputs: %d", len(outputs)))
	}
	if len(modules) > 0 {
		lines = append(lines, fmt.Sprintf("Modules: %d", len(modules)))
	}

	connCount := 0
	for _, targets := range model.Index.Connections {
		connCount += len(targets)
	}
	if connCount > 0 {
		lines = append(lines, fmt.Sprintf("Connections: %d", connCount))
	}

	return strings.Join(lines, "\n")
}

// ── status ───────────────────────────────────────────────

func queryStatus(model *TerraformModel) string {
	filePath := model.FilePath
	if filePath == "" {
		filePath = "(unsaved)"
	}

	lines := []string{
		fmt.Sprintf("Title: %s", model.Title),
		fmt.Sprintf("File: %s", filePath),
		fmt.Sprintf("Blocks: %d", len(model.Index.Order)),
	}

	connCount := 0
	for _, targets := range model.Index.Connections {
		connCount += len(targets)
	}
	lines = append(lines, fmt.Sprintf("Connections: %d", connCount))

	return strings.Join(lines, "\n")
}

// ── history ──────────────────────────────────────────────

func queryHistory(eventLog *fcpcore.EventLog, args []string) string {
	if eventLog == nil {
		return "No event log available."
	}

	count := 10
	if len(args) > 0 {
		if n, err := strconv.Atoi(args[0]); err == nil && n > 0 {
			count = n
		}
	}

	events := eventLog.Recent(count)
	if len(events) == 0 {
		return "No events."
	}

	lines := make([]string, len(events))
	for i, ev := range events {
		// Events are SnapshotEvents with Summary strings
		if se, ok := ev.(*SnapshotEvent); ok {
			lines[i] = fmt.Sprintf("  %d. %s", i+1, se.Summary)
		} else if s, ok := ev.(string); ok {
			lines[i] = fmt.Sprintf("  %d. %s", i+1, s)
		} else {
			lines[i] = fmt.Sprintf("  %d. %v", i+1, ev)
		}
	}
	return strings.Join(lines, "\n")
}

// ── list ─────────────────────────────────────────────────

func queryList(model *TerraformModel, args []string) string {
	var refs []*BlockRef

	// Filter by selector if provided
	if len(args) > 0 && strings.HasPrefix(args[0], "@") {
		sel := ParseSelector(args[0])
		refs = ResolveSelector(sel, model)
	} else {
		// All blocks in insertion order
		for _, label := range model.Index.Order {
			if ref := model.Index.Get(label); ref != nil {
				refs = append(refs, ref)
			}
		}
	}

	if len(refs) == 0 {
		return "No blocks found."
	}

	lines := make([]string, len(refs))
	for i, ref := range refs {
		qualified := ref.QualifiedName()
		attrCount := len(ref.Block.Body().Attributes())
		lines[i] = fmt.Sprintf("  %-10s %-35s (%d attrs)", ref.Kind, qualified, attrCount)
	}
	return strings.Join(lines, "\n")
}

// ── find ─────────────────────────────────────────────────

func queryFind(model *TerraformModel, text string) string {
	if text == "" {
		return "find requires search text"
	}

	lower := strings.ToLower(text)
	var matches []*BlockRef
	for _, label := range model.Index.Order {
		ref := model.Index.Get(label)
		if ref == nil {
			continue
		}
		if strings.Contains(strings.ToLower(ref.Label), lower) ||
			strings.Contains(strings.ToLower(ref.FullType), lower) {
			matches = append(matches, ref)
		}
	}

	if len(matches) == 0 {
		return fmt.Sprintf("No blocks matching %q", text)
	}

	lines := make([]string, len(matches))
	for i, ref := range matches {
		lines[i] = fmt.Sprintf("  %s %s", ref.Kind, ref.QualifiedName())
	}
	return strings.Join(lines, "\n")
}
