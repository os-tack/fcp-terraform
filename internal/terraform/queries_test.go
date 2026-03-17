package terraform

import (
	"strings"
	"testing"

	"github.com/os-tack/fcp-terraform/internal/fcpcore"
)

func setupQueryModel() *TerraformModel {
	m := NewModel("test-infra")
	m.AddProvider("aws", map[string]string{"region": "us-east-1"})
	m.AddResource("aws_instance", "web", map[string]string{
		"ami":           "ami-abc123",
		"instance_type": "t3.micro",
	}, nil)
	m.AddResource("aws_vpc", "main", map[string]string{
		"cidr_block": "10.0.0.0/16",
	}, nil)
	m.AddVariable("region", map[string]string{"type": "string", "default": "us-east-1"})
	m.AddOutput("instance_ip", map[string]string{"value": "aws_instance.web.public_ip"})
	m.Connect("web", "main", "networking")
	return m
}

// ── plan ─────────────────────────────────────────────────

func TestQueryPlan_Empty(t *testing.T) {
	m := NewModel("empty")
	result := DispatchQuery("plan", m, nil)
	if !strings.Contains(result, "Empty configuration") {
		t.Errorf("plan on empty model = %q, want empty message", result)
	}
}

func TestQueryPlan_WithBlocks(t *testing.T) {
	m := setupQueryModel()
	result := DispatchQuery("plan", m, nil)
	if !strings.Contains(result, "aws_instance") {
		t.Errorf("plan should contain aws_instance, got:\n%s", result)
	}
	if !strings.Contains(result, "resource") {
		t.Errorf("plan should contain resource keyword, got:\n%s", result)
	}
}

// ── graph ────────────────────────────────────────────────

func TestQueryGraph_Empty(t *testing.T) {
	m := NewModel("no-conns")
	m.AddResource("aws_instance", "lonely", nil, nil)
	result := DispatchQuery("graph", m, nil)
	if !strings.Contains(result, "No connections") {
		t.Errorf("graph with no connections = %q", result)
	}
}

func TestQueryGraph_WithConnections(t *testing.T) {
	m := setupQueryModel()
	result := DispatchQuery("graph", m, nil)
	if !strings.Contains(result, "web") || !strings.Contains(result, "main") {
		t.Errorf("graph should show web -> main, got:\n%s", result)
	}
	if !strings.Contains(result, "networking") {
		t.Errorf("graph should show edge label, got:\n%s", result)
	}
}

// ── describe ─────────────────────────────────────────────

func TestQueryDescribe_NotFound(t *testing.T) {
	m := setupQueryModel()
	result := DispatchQuery("describe ghost", m, nil)
	if !strings.Contains(result, "not found") {
		t.Errorf("describe ghost = %q, want not found", result)
	}
}

func TestQueryDescribe_Resource(t *testing.T) {
	m := setupQueryModel()
	result := DispatchQuery("describe web", m, nil)
	if !strings.Contains(result, "aws_instance.web") {
		t.Errorf("describe web should show qualified name, got:\n%s", result)
	}
	if !strings.Contains(result, "ami") {
		t.Errorf("describe web should show attributes, got:\n%s", result)
	}
}

func TestQueryDescribe_Connections(t *testing.T) {
	m := setupQueryModel()
	result := DispatchQuery("describe web", m, nil)
	if !strings.Contains(result, "connections") {
		t.Errorf("describe web should show connections, got:\n%s", result)
	}
	if !strings.Contains(result, "main") {
		t.Errorf("describe web should show connection to main, got:\n%s", result)
	}
}

func TestQueryDescribe_NoLabel(t *testing.T) {
	m := setupQueryModel()
	result := DispatchQuery("describe", m, nil)
	if !strings.Contains(result, "requires a LABEL") {
		t.Errorf("describe without label = %q", result)
	}
}

// ── stats ────────────────────────────────────────────────

func TestQueryStats(t *testing.T) {
	m := setupQueryModel()
	result := DispatchQuery("stats", m, nil)
	if !strings.Contains(result, "Total blocks: 5") {
		t.Errorf("stats should show 5 blocks, got:\n%s", result)
	}
	if !strings.Contains(result, "resource: 2") {
		t.Errorf("stats should show 2 resources, got:\n%s", result)
	}
	if !strings.Contains(result, "Connections: 1") {
		t.Errorf("stats should show 1 connection, got:\n%s", result)
	}
}

// ── map ──────────────────────────────────────────────────

func TestQueryMap(t *testing.T) {
	m := setupQueryModel()
	result := DispatchQuery("map", m, nil)
	if !strings.Contains(result, "test-infra") {
		t.Errorf("map should contain title, got:\n%s", result)
	}
	if !strings.Contains(result, "Providers: aws") {
		t.Errorf("map should list providers, got:\n%s", result)
	}
	if !strings.Contains(result, "Resources (2)") {
		t.Errorf("map should show resource count, got:\n%s", result)
	}
	if !strings.Contains(result, "Variables: 1") {
		t.Errorf("map should show variable count, got:\n%s", result)
	}
}

// ── status ───────────────────────────────────────────────

func TestQueryStatus(t *testing.T) {
	m := setupQueryModel()
	result := DispatchQuery("status", m, nil)
	if !strings.Contains(result, "Title: test-infra") {
		t.Errorf("status should show title, got:\n%s", result)
	}
	if !strings.Contains(result, "(unsaved)") {
		t.Errorf("status should show unsaved, got:\n%s", result)
	}
}

func TestQueryStatus_WithPath(t *testing.T) {
	m := setupQueryModel()
	m.FilePath = "/tmp/main.tf"
	result := DispatchQuery("status", m, nil)
	if !strings.Contains(result, "/tmp/main.tf") {
		t.Errorf("status should show file path, got:\n%s", result)
	}
}

// ── history ──────────────────────────────────────────────

func TestQueryHistory_NoLog(t *testing.T) {
	m := setupQueryModel()
	result := DispatchQuery("history", m, nil)
	if !strings.Contains(result, "No event log") {
		t.Errorf("history with nil log = %q", result)
	}
}

func TestQueryHistory_WithEvents(t *testing.T) {
	m := setupQueryModel()
	log := fcpcore.NewEventLog()
	log.Append(&SnapshotEvent{Summary: "add resource aws_instance web"})
	log.Append(&SnapshotEvent{Summary: "set web ami:ami-xyz"})

	result := DispatchQuery("history", m, log)
	if !strings.Contains(result, "1. add resource") {
		t.Errorf("history should show first event, got:\n%s", result)
	}
	if !strings.Contains(result, "2. set web") {
		t.Errorf("history should show second event, got:\n%s", result)
	}
}

func TestQueryHistory_WithCount(t *testing.T) {
	m := setupQueryModel()
	log := fcpcore.NewEventLog()
	log.Append(&SnapshotEvent{Summary: "event 1"})
	log.Append(&SnapshotEvent{Summary: "event 2"})
	log.Append(&SnapshotEvent{Summary: "event 3"})

	result := DispatchQuery("history 2", m, log)
	// Recent(2) returns last 2 events
	if !strings.Contains(result, "event 2") || !strings.Contains(result, "event 3") {
		t.Errorf("history 2 should show last 2 events, got:\n%s", result)
	}
}

// ── list ─────────────────────────────────────────────────

func TestQueryList_All(t *testing.T) {
	m := setupQueryModel()
	result := DispatchQuery("list", m, nil)
	if !strings.Contains(result, "aws_instance.web") {
		t.Errorf("list should show web, got:\n%s", result)
	}
	if !strings.Contains(result, "aws_vpc.main") {
		t.Errorf("list should show main, got:\n%s", result)
	}
}

func TestQueryList_WithSelector(t *testing.T) {
	m := setupQueryModel()
	result := DispatchQuery("list @type:aws_instance", m, nil)
	if !strings.Contains(result, "web") {
		t.Errorf("list @type should show web, got:\n%s", result)
	}
	if strings.Contains(result, "main") {
		t.Errorf("list @type:aws_instance should NOT show vpc main, got:\n%s", result)
	}
}

func TestQueryList_Empty(t *testing.T) {
	m := NewModel("empty")
	result := DispatchQuery("list", m, nil)
	if !strings.Contains(result, "No blocks") {
		t.Errorf("list on empty = %q", result)
	}
}

// ── find ─────────────────────────────────────────────────

func TestQueryFind(t *testing.T) {
	m := setupQueryModel()
	result := DispatchQuery("find web", m, nil)
	if !strings.Contains(result, "aws_instance.web") {
		t.Errorf("find web = %q", result)
	}
}

func TestQueryFind_NoMatch(t *testing.T) {
	m := setupQueryModel()
	result := DispatchQuery("find unicorn", m, nil)
	if !strings.Contains(result, "No blocks matching") {
		t.Errorf("find unicorn = %q", result)
	}
}

func TestQueryFind_ByType(t *testing.T) {
	m := setupQueryModel()
	result := DispatchQuery("find vpc", m, nil)
	if !strings.Contains(result, "aws_vpc.main") {
		t.Errorf("find vpc should match type, got:\n%s", result)
	}
}

// ── unknown ──────────────────────────────────────────────

func TestQueryUnknown(t *testing.T) {
	m := setupQueryModel()
	result := DispatchQuery("bogus", m, nil)
	if !strings.Contains(result, "Unknown query") {
		t.Errorf("unknown query = %q", result)
	}
}

func TestQueryEmpty(t *testing.T) {
	m := setupQueryModel()
	result := DispatchQuery("", m, nil)
	if !strings.Contains(result, "Query required") {
		t.Errorf("empty query = %q", result)
	}
}
