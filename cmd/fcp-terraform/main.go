package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/os-tack/fcp-terraform/internal/bridge"
	"github.com/os-tack/fcp-terraform/internal/fcpcore"
	"github.com/os-tack/fcp-terraform/internal/terraform"
)

func main() {
	// Create the Terraform session and adapter
	session, adapter := terraform.NewTerraformSession()

	// Build the reference card
	registry := fcpcore.NewVerbRegistry()
	registry.RegisterMany(terraform.TerraformVerbSpecs())
	refCard := registry.GenerateReferenceCard(terraform.ExtraSections())

	// Create MCP server
	s := server.NewMCPServer(
		"fcp-terraform",
		"0.1.0",
		server.WithToolCapabilities(false),
		server.WithResourceCapabilities(false, false),
		server.WithInstructions("FCP Terraform server for generating HashiCorp Terraform HCL configurations. Use terraform_session to start a new configuration or open an existing .tf file, terraform to add resources/variables/outputs/providers and modify infrastructure blocks, terraform_query to inspect the current configuration, and terraform_help for the full verb reference. Start every interaction with terraform_session."),
	)

	// ── terraform tool (batch mutations) ──────────────────
	terraformTool := mcp.NewTool("terraform",
		mcp.WithDescription("Execute terraform operations. Each op string follows the FCP verb DSL.\n\n"+refCard),
		mcp.WithArray("ops",
			mcp.Required(),
			mcp.Description("Array of operation strings"),
		),
	)
	s.AddTool(terraformTool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if session.Model == nil {
			return mcp.NewToolResultText("ERROR: No session. Use terraform_session to create one first."), nil
		}

		model, ok := session.Model.(*terraform.TerraformModel)
		if !ok {
			return mcp.NewToolResultText("ERROR: Invalid model type"), nil
		}

		args := req.GetArguments()
		opsRaw, ok2 := args["ops"]
		if !ok2 {
			return mcp.NewToolResultText("ERROR: ops parameter required"), nil
		}

		opsSlice, ok2 := opsRaw.([]interface{})
		if !ok2 {
			return mcp.NewToolResultText("ERROR: ops must be an array of strings"), nil
		}

		// Expansion phase: split ops on \n, filter empty lines
		var expandedOps []string
		for _, opRaw := range opsSlice {
			opStr, ok := opRaw.(string)
			if !ok {
				expandedOps = append(expandedOps, "")
				continue
			}
			for _, line := range strings.Split(opStr, "\n") {
				trimmed := strings.TrimSpace(line)
				if trimmed != "" {
					expandedOps = append(expandedOps, trimmed)
				}
			}
		}

		// Batch snapshot for atomicity
		snapshot := model.Snapshot()

		var results []string
		for i, opStr := range expandedOps {
			if opStr == "" {
				results = append(results, "ERROR: each op must be a string")
				continue
			}

			parsed := fcpcore.ParseOp(opStr)
			if parsed.Err != nil {
				log.Printf("[fcp-terraform] WARN parse error: %s (op: %s)", parsed.Err.Error, opStr)
				model.Restore(snapshot)
				return mcp.NewToolResultText(fmt.Sprintf("! Batch failed at op %d: %s. Error: %s. State rolled back (%d ops reverted).", i+1, opStr, parsed.Err.Error, i)), nil
			}

			// Verb validation: try bare verb, then compound verb (e.g. "add resource")
			verbKey := parsed.Op.Verb
			if _, ok := registry.Lookup(verbKey); !ok {
				if len(parsed.Op.Positionals) > 0 {
					verbKey = parsed.Op.Verb + " " + parsed.Op.Positionals[0]
				}
				if _, ok := registry.Lookup(verbKey); !ok {
					msg := fmt.Sprintf("unknown verb %q", parsed.Op.Verb)
					verbNames := make([]string, 0)
					for _, v := range registry.Verbs() {
						verbNames = append(verbNames, v.Name)
					}
					if suggestion := fcpcore.Suggest(parsed.Op.Verb, verbNames); suggestion != "" {
						msg += "\n  try: " + suggestion
					}
					model.Restore(snapshot)
					return mcp.NewToolResultText(fmt.Sprintf("! Batch failed at op %d: %s. Error: %s. State rolled back (%d ops reverted).", i+1, opStr, msg, i)), nil
				}
			}

			result, event := adapter.DispatchOp(parsed.Op, model)
			if strings.HasPrefix(result, "ERROR:") {
				model.Restore(snapshot)
				return mcp.NewToolResultText(fmt.Sprintf("! Batch failed at op %d: %s. Error: %s. State rolled back (%d ops reverted).", i+1, opStr, result, i)), nil
			}
			session.Log.Append(event)
			results = append(results, result)
		}

		body := strings.Join(results, "\n")
		if digest := adapter.GetDigest(session.Model); digest != "" {
			body = body + "\n" + digest
		}
		return mcp.NewToolResultText(body), nil
	})

	// ── terraform_query tool (read-only) ──────────────────
	queryTool := mcp.NewTool("terraform_query",
		mcp.WithDescription("Query terraform state. Read-only.\n\nQueries: plan, graph, describe LABEL, stats, map, status, history [N], list [@selector], find TEXT"),
		mcp.WithString("q",
			mcp.Required(),
			mcp.Description("Query string"),
		),
	)
	s.AddTool(queryTool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if session.Model == nil {
			return mcp.NewToolResultText("ERROR: No session. Use terraform_session to create one first."), nil
		}

		model, ok := session.Model.(*terraform.TerraformModel)
		if !ok {
			return mcp.NewToolResultText("ERROR: Invalid model type"), nil
		}

		q := req.GetString("q", "")
		result := terraform.DispatchQuery(q, model, session.Log)
		return mcp.NewToolResultText(result), nil
	})

	// ── terraform_session tool (lifecycle) ────────────────
	sessionTool := mcp.NewTool("terraform_session",
		mcp.WithDescription("terraform lifecycle: new, open, save, checkpoint, undo, redo.\n\nExamples:\n  new \"My Infrastructure\"\n  open ./main.tf\n  save\n  save as:./output.tf\n  checkpoint v1\n  undo\n  undo to:v1\n  redo"),
		mcp.WithString("action",
			mcp.Required(),
			mcp.Description("Action: 'new \"Title\"', 'open ./file', 'save', 'save as:./out', 'checkpoint v1', 'undo', 'undo to:v1', 'redo'"),
		),
	)
	s.AddTool(sessionTool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		action := req.GetString("action", "")
		log.Printf("[fcp-terraform] INFO session: %s", action)
		result := session.Dispatch(action)
		if session.Model != nil {
			if digest := adapter.GetDigest(session.Model); digest != "" {
				result = result + "\n" + digest
			}
		}
		return mcp.NewToolResultText(result), nil
	})

	// ── terraform_help tool (reference card) ──────────────
	helpTool := mcp.NewTool("terraform_help",
		mcp.WithDescription("Returns the terraform FCP reference card."),
	)
	s.AddTool(helpTool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultText(refCard), nil
	})

	// ── Resources ────────────────────────────────────────
	sessionResource := mcp.NewResource(
		"fcp://terraform/session",
		"session-status",
		mcp.WithResourceDescription("Current terraform session state"),
		mcp.WithMIMEType("text/plain"),
	)
	s.AddResource(sessionResource, func(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		if session.Model == nil {
			return []mcp.ResourceContents{
				mcp.TextResourceContents{URI: "fcp://terraform/session", MIMEType: "text/plain", Text: "No terraform session active."},
			}, nil
		}
		var lines []string
		if session.FilePath != "" {
			lines = append(lines, "File: "+session.FilePath)
		}
		if digest := adapter.GetDigest(session.Model); digest != "" {
			lines = append(lines, "State: "+digest)
		}
		return []mcp.ResourceContents{
			mcp.TextResourceContents{URI: "fcp://terraform/session", MIMEType: "text/plain", Text: strings.Join(lines, "\n")},
		}, nil
	})

	modelResource := mcp.NewResource(
		"fcp://terraform/model",
		"model-overview",
		mcp.WithResourceDescription("Current terraform model contents"),
		mcp.WithMIMEType("text/plain"),
	)
	s.AddResource(modelResource, func(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		if session.Model == nil {
			return []mcp.ResourceContents{
				mcp.TextResourceContents{URI: "fcp://terraform/model", MIMEType: "text/plain", Text: "No model loaded."},
			}, nil
		}
		model, _ := session.Model.(*terraform.TerraformModel)
		text := terraform.DispatchQuery("map", model, session.Log)
		return []mcp.ResourceContents{
			mcp.TextResourceContents{URI: "fcp://terraform/model", MIMEType: "text/plain", Text: text},
		}, nil
	})

	// Start slipstream bridge in background
	go bridge.Connect(bridge.Config{
		Domain:     "terraform",
		Extensions: []string{"tf", "tfvars"},
		Session:    session,
		Adapter:    adapter,
		Registry:   registry,
	})

	// Start stdio server
	if err := server.ServeStdio(s); err != nil {
		fmt.Printf("Server error: %v\n", err)
	}
}
