package bridge

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/user"
	"strings"

	"github.com/os-tack/fcp-terraform/internal/fcpcore"
	"github.com/os-tack/fcp-terraform/internal/terraform"
)

// Config holds everything needed to route bridge requests.
type Config struct {
	Domain     string
	Extensions []string
	Session    *fcpcore.Session
	Adapter    *terraform.TerraformAdapter
	Registry   *fcpcore.VerbRegistry
}

type jsonrpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type jsonrpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *jsonrpcError   `json:"error,omitempty"`
}

type jsonrpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type textResult struct {
	Text string `json:"text"`
}

// Connect discovers the slipstream socket and runs the bridge loop.
// It blocks, so call it as `go bridge.Connect(cfg)`.
// Silently returns on any failure.
func Connect(cfg Config) {
	socketPath := discoverSocket()
	if socketPath == "" {
		return
	}

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return
	}
	defer conn.Close()

	// Send registration
	reg := map[string]any{
		"jsonrpc": "2.0",
		"method":  "fcp.register",
		"params": map[string]any{
			"handler_name": "fcp-terraform",
			"extensions":   cfg.Extensions,
			"capabilities": []string{"ops", "query", "session"},
		},
	}
	regBytes, err := json.Marshal(reg)
	if err != nil {
		return
	}
	if _, err := conn.Write(append(regBytes, '\n')); err != nil {
		return
	}

	// NDJSON request/response loop
	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		var req jsonrpcRequest
		if err := json.Unmarshal(line, &req); err != nil {
			continue
		}

		resp := handleRequest(cfg, req)
		respBytes, err := json.Marshal(resp)
		if err != nil {
			continue
		}
		if _, err := conn.Write(append(respBytes, '\n')); err != nil {
			return
		}
	}
}

func discoverSocket() string {
	// 1. Explicit env var
	if p := os.Getenv("SLIPSTREAM_SOCKET"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	// 2. XDG_RUNTIME_DIR
	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		p := xdg + "/slipstream/daemon.sock"
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	// 3. /tmp/slipstream-{uid}/daemon.sock
	u, err := user.Current()
	if err != nil {
		return ""
	}
	p := fmt.Sprintf("/tmp/slipstream-%s/daemon.sock", u.Uid)
	if _, err := os.Stat(p); err == nil {
		return p
	}

	return ""
}

func handleRequest(cfg Config, req jsonrpcRequest) jsonrpcResponse {
	switch req.Method {
	case "fcp.session":
		return handleSession(cfg, req)
	case "fcp.ops":
		return handleOps(cfg, req)
	case "fcp.query":
		return handleQuery(cfg, req)
	default:
		return jsonrpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &jsonrpcError{Code: -32601, Message: fmt.Sprintf("unknown method %q", req.Method)},
		}
	}
}

func handleSession(cfg Config, req jsonrpcRequest) jsonrpcResponse {
	var params struct {
		Action string `json:"action"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return jsonrpcResponse{JSONRPC: "2.0", ID: req.ID, Error: &jsonrpcError{Code: -32602, Message: "invalid params"}}
	}

	result := cfg.Session.Dispatch(params.Action)
	if cfg.Session.Model != nil {
		if digest := cfg.Adapter.GetDigest(cfg.Session.Model); digest != "" {
			result = result + "\n" + digest
		}
	}
	return jsonrpcResponse{JSONRPC: "2.0", ID: req.ID, Result: textResult{Text: result}}
}

func handleOps(cfg Config, req jsonrpcRequest) jsonrpcResponse {
	var params struct {
		Ops []string `json:"ops"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return jsonrpcResponse{JSONRPC: "2.0", ID: req.ID, Error: &jsonrpcError{Code: -32602, Message: "invalid params"}}
	}

	if cfg.Session.Model == nil {
		return jsonrpcResponse{JSONRPC: "2.0", ID: req.ID, Result: textResult{Text: "ERROR: No session. Use terraform_session to create one first."}}
	}

	model, ok := cfg.Session.Model.(*terraform.TerraformModel)
	if !ok {
		return jsonrpcResponse{JSONRPC: "2.0", ID: req.ID, Result: textResult{Text: "ERROR: Invalid model type"}}
	}

	snapshot := model.Snapshot()
	var results []string

	for i, opStr := range params.Ops {
		parsed := fcpcore.ParseOp(opStr)
		if parsed.Err != nil {
			model.Restore(snapshot)
			msg := fmt.Sprintf("! Batch failed at op %d: %s. Error: %s. State rolled back (%d ops reverted).", i+1, opStr, parsed.Err.Error, i)
			return jsonrpcResponse{JSONRPC: "2.0", ID: req.ID, Result: textResult{Text: msg}}
		}

		// Verb validation: try bare verb, then compound verb
		verbKey := parsed.Op.Verb
		if _, ok := cfg.Registry.Lookup(verbKey); !ok {
			if len(parsed.Op.Positionals) > 0 {
				verbKey = parsed.Op.Verb + " " + parsed.Op.Positionals[0]
			}
			if _, ok := cfg.Registry.Lookup(verbKey); !ok {
				msg := fmt.Sprintf("unknown verb %q", parsed.Op.Verb)
				verbNames := make([]string, 0)
				for _, v := range cfg.Registry.Verbs() {
					verbNames = append(verbNames, v.Name)
				}
				if suggestion := fcpcore.Suggest(parsed.Op.Verb, verbNames); suggestion != "" {
					msg += "\n  try: " + suggestion
				}
				model.Restore(snapshot)
				msg = fmt.Sprintf("! Batch failed at op %d: %s. Error: %s. State rolled back (%d ops reverted).", i+1, opStr, msg, i)
				return jsonrpcResponse{JSONRPC: "2.0", ID: req.ID, Result: textResult{Text: msg}}
			}
		}

		result, event := cfg.Adapter.DispatchOp(parsed.Op, model)
		if strings.HasPrefix(result, "ERROR:") {
			model.Restore(snapshot)
			msg := fmt.Sprintf("! Batch failed at op %d: %s. Error: %s. State rolled back (%d ops reverted).", i+1, opStr, result, i)
			return jsonrpcResponse{JSONRPC: "2.0", ID: req.ID, Result: textResult{Text: msg}}
		}
		cfg.Session.Log.Append(event)
		results = append(results, result)
	}

	body := strings.Join(results, "\n")
	if digest := cfg.Adapter.GetDigest(cfg.Session.Model); digest != "" {
		body = body + "\n" + digest
	}
	return jsonrpcResponse{JSONRPC: "2.0", ID: req.ID, Result: textResult{Text: body}}
}

func handleQuery(cfg Config, req jsonrpcRequest) jsonrpcResponse {
	var params struct {
		Q string `json:"q"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return jsonrpcResponse{JSONRPC: "2.0", ID: req.ID, Error: &jsonrpcError{Code: -32602, Message: "invalid params"}}
	}

	if cfg.Session.Model == nil {
		return jsonrpcResponse{JSONRPC: "2.0", ID: req.ID, Result: textResult{Text: "ERROR: No session. Use terraform_session to create one first."}}
	}

	model, ok := cfg.Session.Model.(*terraform.TerraformModel)
	if !ok {
		return jsonrpcResponse{JSONRPC: "2.0", ID: req.ID, Result: textResult{Text: "ERROR: Invalid model type"}}
	}

	result := terraform.DispatchQuery(params.Q, model, cfg.Session.Log)
	return jsonrpcResponse{JSONRPC: "2.0", ID: req.ID, Result: textResult{Text: result}}
}
