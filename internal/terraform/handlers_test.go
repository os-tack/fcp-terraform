package terraform

import (
	"strings"
	"testing"

	"github.com/os-tack/fcp-terraform/internal/fcpcore"
)

// helper: parse an op string and dispatch it against a model
func dispatch(input string, model *TerraformModel) string {
	r := fcpcore.ParseOp(input)
	if r.IsError() {
		return "PARSE ERROR: " + r.Err.Error
	}
	return Dispatch(r.Op, model)
}

// ── add tests ────────────────────────────────────────────

func TestHandler_AddResource(t *testing.T) {
	m := NewModel("test")
	result := dispatch("add resource aws_instance web ami:ami-123 instance_type:t2.micro", m)
	if !strings.HasPrefix(result, "+") {
		t.Errorf("expected + prefix, got: %s", result)
	}
	if !strings.Contains(result, "aws_instance.web") {
		t.Errorf("expected aws_instance.web in result: %s", result)
	}

	hcl := string(m.Bytes())
	if !strings.Contains(hcl, `resource "aws_instance" "web"`) {
		t.Errorf("HCL missing resource:\n%s", hcl)
	}
	if !strings.Contains(hcl, `"ami-123"`) {
		t.Errorf("HCL missing ami:\n%s", hcl)
	}
}

func TestHandler_AddResource_MissingArgs(t *testing.T) {
	m := NewModel("test")
	result := dispatch("add resource aws_instance", m)
	if !strings.HasPrefix(result, "ERROR:") {
		t.Errorf("expected error, got: %s", result)
	}
}

func TestHandler_AddResource_Duplicate(t *testing.T) {
	m := NewModel("test")
	dispatch("add resource aws_instance web", m)
	result := dispatch("add resource aws_instance web", m)
	if !strings.HasPrefix(result, "ERROR:") {
		t.Errorf("expected error for duplicate, got: %s", result)
	}
}

func TestHandler_AddProvider(t *testing.T) {
	m := NewModel("test")
	result := dispatch("add provider aws region:us-east-1", m)
	if !strings.HasPrefix(result, "+") {
		t.Errorf("expected + prefix, got: %s", result)
	}
	hcl := string(m.Bytes())
	if !strings.Contains(hcl, `provider "aws"`) {
		t.Errorf("HCL missing provider:\n%s", hcl)
	}
}

func TestHandler_AddProvider_MissingName(t *testing.T) {
	m := NewModel("test")
	result := dispatch("add provider", m)
	if !strings.HasPrefix(result, "ERROR:") {
		t.Errorf("expected error, got: %s", result)
	}
}

func TestHandler_AddVariable(t *testing.T) {
	m := NewModel("test")
	result := dispatch("add variable region type:string default:us-east-1", m)
	if !strings.HasPrefix(result, "+") {
		t.Errorf("expected + prefix, got: %s", result)
	}
	hcl := string(m.Bytes())
	if !strings.Contains(hcl, `variable "region"`) {
		t.Errorf("HCL missing variable:\n%s", hcl)
	}
}

func TestHandler_AddOutput(t *testing.T) {
	m := NewModel("test")
	dispatch("add resource aws_vpc main cidr_block:10.0.0.0/16", m)
	result := dispatch("add output vpc_id value:aws_vpc.main.id", m)
	if !strings.HasPrefix(result, "+") {
		t.Errorf("expected + prefix, got: %s", result)
	}
	hcl := string(m.Bytes())
	if !strings.Contains(hcl, `output "vpc_id"`) {
		t.Errorf("HCL missing output:\n%s", hcl)
	}
	if !strings.Contains(hcl, "aws_vpc.main.id") {
		t.Errorf("HCL missing reference:\n%s", hcl)
	}
}

func TestHandler_AddData(t *testing.T) {
	m := NewModel("test")
	result := dispatch("add data aws_ami latest most_recent:true", m)
	if !strings.HasPrefix(result, "+") {
		t.Errorf("expected + prefix, got: %s", result)
	}
	hcl := string(m.Bytes())
	if !strings.Contains(hcl, `data "aws_ami" "latest"`) {
		t.Errorf("HCL missing data:\n%s", hcl)
	}
}

func TestHandler_AddModule(t *testing.T) {
	m := NewModel("test")
	result := dispatch("add module vpc source:terraform-aws-modules/vpc/aws", m)
	if !strings.HasPrefix(result, "+") {
		t.Errorf("expected + prefix, got: %s", result)
	}
	hcl := string(m.Bytes())
	if !strings.Contains(hcl, `module "vpc"`) {
		t.Errorf("HCL missing module:\n%s", hcl)
	}
}

func TestHandler_AddUnknownType(t *testing.T) {
	m := NewModel("test")
	result := dispatch("add widget foo", m)
	if !strings.Contains(result, "unknown add type") {
		t.Errorf("expected unknown add type error, got: %s", result)
	}
}

func TestHandler_AddNoSubtype(t *testing.T) {
	m := NewModel("test")
	result := dispatch("add", m)
	if !strings.HasPrefix(result, "ERROR:") {
		t.Errorf("expected error, got: %s", result)
	}
}

// ── set tests ────────────────────────────────────────────

func TestHandler_Set(t *testing.T) {
	m := NewModel("test")
	dispatch("add resource aws_instance web ami:ami-old", m)
	result := dispatch("set web ami:ami-new instance_type:t3.large", m)
	if !strings.HasPrefix(result, "*") {
		t.Errorf("expected * prefix, got: %s", result)
	}
	hcl := string(m.Bytes())
	if !strings.Contains(hcl, `"ami-new"`) {
		t.Errorf("HCL missing updated ami:\n%s", hcl)
	}
	if !strings.Contains(hcl, `"t3.large"`) {
		t.Errorf("HCL missing new instance_type:\n%s", hcl)
	}
}

func TestHandler_Set_Expression(t *testing.T) {
	m := NewModel("test")
	dispatch("add resource aws_instance web", m)
	r := fcpcore.ParseOp(`set web policy "jsonencode({Version = 1})"`)
	if r.IsError() {
		t.Fatalf("parse error: %s", r.Err.Error)
	}
	result := Dispatch(r.Op, m)
	if !strings.HasPrefix(result, "*") {
		t.Errorf("expected * prefix, got: %s", result)
	}
	if !strings.Contains(result, "(expression)") {
		t.Errorf("expected expression marker: %s", result)
	}
	hcl := string(m.Bytes())
	if !strings.Contains(hcl, "jsonencode") {
		t.Errorf("HCL missing expression:\n%s", hcl)
	}
}

func TestHandler_Set_NotFound(t *testing.T) {
	m := NewModel("test")
	result := dispatch("set nonexistent ami:test", m)
	if !strings.HasPrefix(result, "ERROR:") {
		t.Errorf("expected error, got: %s", result)
	}
}

func TestHandler_Set_NoParams(t *testing.T) {
	m := NewModel("test")
	dispatch("add resource aws_instance web", m)
	result := dispatch("set web", m)
	if !strings.HasPrefix(result, "ERROR:") {
		t.Errorf("expected error, got: %s", result)
	}
}

func TestHandler_Set_NoLabel(t *testing.T) {
	m := NewModel("test")
	result := dispatch("set", m)
	if !strings.HasPrefix(result, "ERROR:") {
		t.Errorf("expected error, got: %s", result)
	}
}

// ── unset tests ──────────────────────────────────────────

func TestHandler_Unset(t *testing.T) {
	m := NewModel("test")
	dispatch("add resource aws_instance web ami:ami-123 instance_type:t2.micro", m)
	result := dispatch("unset web ami", m)
	if !strings.HasPrefix(result, "*") {
		t.Errorf("expected * prefix, got: %s", result)
	}
	hcl := string(m.Bytes())
	if strings.Contains(hcl, "ami") {
		t.Errorf("HCL should not contain ami after unset:\n%s", hcl)
	}
	if !strings.Contains(hcl, "instance_type") {
		t.Errorf("HCL should still contain instance_type:\n%s", hcl)
	}
}

func TestHandler_Unset_NotFound(t *testing.T) {
	m := NewModel("test")
	result := dispatch("unset nonexistent key", m)
	if !strings.HasPrefix(result, "ERROR:") {
		t.Errorf("expected error, got: %s", result)
	}
}

func TestHandler_Unset_NoKeys(t *testing.T) {
	m := NewModel("test")
	dispatch("add resource aws_instance web", m)
	result := dispatch("unset web", m)
	if !strings.HasPrefix(result, "ERROR:") {
		t.Errorf("expected error, got: %s", result)
	}
}

// ── remove tests ─────────────────────────────────────────

func TestHandler_Remove_ByLabel(t *testing.T) {
	m := NewModel("test")
	dispatch("add resource aws_instance web", m)
	dispatch("add resource aws_vpc main", m)
	result := dispatch("remove web", m)
	if !strings.HasPrefix(result, "-") {
		t.Errorf("expected - prefix, got: %s", result)
	}
	hcl := string(m.Bytes())
	if strings.Contains(hcl, "aws_instance") {
		t.Errorf("HCL should not contain removed block:\n%s", hcl)
	}
	if !strings.Contains(hcl, "aws_vpc") {
		t.Errorf("HCL should still contain vpc:\n%s", hcl)
	}
}

func TestHandler_Remove_NotFound(t *testing.T) {
	m := NewModel("test")
	result := dispatch("remove nonexistent", m)
	if !strings.HasPrefix(result, "ERROR:") {
		t.Errorf("expected error, got: %s", result)
	}
}

func TestHandler_Remove_NoArgs(t *testing.T) {
	m := NewModel("test")
	result := dispatch("remove", m)
	if !strings.HasPrefix(result, "ERROR:") {
		t.Errorf("expected error, got: %s", result)
	}
}

func TestHandler_Remove_BySelector(t *testing.T) {
	m := NewModel("test")
	dispatch("add resource aws_instance web", m)
	dispatch("add resource aws_instance api", m)
	dispatch("add resource aws_vpc main", m)
	result := dispatch("remove @type:aws_instance", m)
	if !strings.HasPrefix(result, "@") {
		t.Errorf("expected @ prefix, got: %s", result)
	}
	if !strings.Contains(result, "2 block(s)") {
		t.Errorf("expected 2 blocks removed: %s", result)
	}
	hcl := string(m.Bytes())
	if strings.Contains(hcl, "aws_instance") {
		t.Errorf("HCL should not contain aws_instance:\n%s", hcl)
	}
	if !strings.Contains(hcl, "aws_vpc") {
		t.Errorf("HCL should still contain aws_vpc:\n%s", hcl)
	}
}

func TestHandler_Remove_SelectorNoMatch(t *testing.T) {
	m := NewModel("test")
	dispatch("add resource aws_instance web", m)
	result := dispatch("remove @type:aws_vpc", m)
	if !strings.HasPrefix(result, "ERROR:") {
		t.Errorf("expected error, got: %s", result)
	}
}

// ── connect/disconnect tests ─────────────────────────────

func TestHandler_Connect(t *testing.T) {
	m := NewModel("test")
	dispatch("add resource aws_instance web", m)
	dispatch("add resource aws_vpc vpc", m)
	result := dispatch("connect web -> vpc label:depends_on", m)
	if !strings.HasPrefix(result, "~") {
		t.Errorf("expected ~ prefix, got: %s", result)
	}
	if !strings.Contains(result, "web -> vpc") {
		t.Errorf("expected connection info: %s", result)
	}
}

func TestHandler_Connect_NoArrow(t *testing.T) {
	m := NewModel("test")
	dispatch("add resource aws_instance web", m)
	result := dispatch("connect web vpc", m)
	if !strings.HasPrefix(result, "ERROR:") {
		t.Errorf("expected error, got: %s", result)
	}
}

func TestHandler_Connect_NotFound(t *testing.T) {
	m := NewModel("test")
	dispatch("add resource aws_instance web", m)
	result := dispatch("connect web -> nonexistent", m)
	if !strings.HasPrefix(result, "ERROR:") {
		t.Errorf("expected error, got: %s", result)
	}
}

func TestHandler_Disconnect(t *testing.T) {
	m := NewModel("test")
	dispatch("add resource aws_instance web", m)
	dispatch("add resource aws_vpc vpc", m)
	dispatch("connect web -> vpc", m)
	result := dispatch("disconnect web -> vpc", m)
	if !strings.HasPrefix(result, "-") {
		t.Errorf("expected - prefix, got: %s", result)
	}
}

func TestHandler_Disconnect_NoArrow(t *testing.T) {
	m := NewModel("test")
	result := dispatch("disconnect web vpc", m)
	if !strings.HasPrefix(result, "ERROR:") {
		t.Errorf("expected error, got: %s", result)
	}
}

// ── label tests ──────────────────────────────────────────

func TestHandler_Label(t *testing.T) {
	m := NewModel("test")
	dispatch("add resource aws_instance web", m)
	result := dispatch(`label web "web_server"`, m)
	if !strings.HasPrefix(result, "*") {
		t.Errorf("expected * prefix, got: %s", result)
	}

	// Old label should not resolve
	if m.Index.Get("web") != nil {
		t.Error("old label should not resolve")
	}
	// New label should resolve
	if m.Index.Get("web_server") == nil {
		t.Error("new label should resolve")
	}

	hcl := string(m.Bytes())
	if !strings.Contains(hcl, `"web_server"`) {
		t.Errorf("HCL should contain new label:\n%s", hcl)
	}
}

func TestHandler_Label_NotFound(t *testing.T) {
	m := NewModel("test")
	result := dispatch("label nonexistent new_name", m)
	if !strings.HasPrefix(result, "ERROR:") {
		t.Errorf("expected error, got: %s", result)
	}
}

func TestHandler_Label_MissingArgs(t *testing.T) {
	m := NewModel("test")
	result := dispatch("label web", m)
	if !strings.HasPrefix(result, "ERROR:") {
		t.Errorf("expected error, got: %s", result)
	}
}

func TestHandler_Label_Conflict(t *testing.T) {
	m := NewModel("test")
	dispatch("add resource aws_instance web", m)
	dispatch("add resource aws_instance api", m)
	result := dispatch("label web api", m)
	if !strings.HasPrefix(result, "ERROR:") {
		t.Errorf("expected error for duplicate, got: %s", result)
	}
}

// ── style tests ──────────────────────────────────────────

func TestHandler_Style(t *testing.T) {
	m := NewModel("test")
	dispatch("add resource aws_instance web", m)
	result := dispatch(`style web tags:"env=prod,team=backend"`, m)
	if !strings.HasPrefix(result, "*") {
		t.Errorf("expected * prefix, got: %s", result)
	}

	ref := m.Index.Get("web")
	if ref.Tags["env"] != "prod" {
		t.Errorf("tag env = %q, want prod", ref.Tags["env"])
	}
	if ref.Tags["team"] != "backend" {
		t.Errorf("tag team = %q, want backend", ref.Tags["team"])
	}
}

func TestHandler_Style_BySelector(t *testing.T) {
	m := NewModel("test")
	dispatch("add resource aws_instance web", m)
	dispatch("add resource aws_instance api", m)
	dispatch("add resource aws_vpc main", m)
	result := dispatch(`style @type:aws_instance tags:"tier=app"`, m)
	if !strings.HasPrefix(result, "@") {
		t.Errorf("expected @ prefix, got: %s", result)
	}
	if m.Index.Get("web").Tags["tier"] != "app" {
		t.Error("web should have tier=app tag")
	}
	if m.Index.Get("api").Tags["tier"] != "app" {
		t.Error("api should have tier=app tag")
	}
	// vpc should not be tagged
	if _, ok := m.Index.Get("main").Tags["tier"]; ok {
		t.Error("main should not have tier tag")
	}
}

func TestHandler_Style_NoTags(t *testing.T) {
	m := NewModel("test")
	dispatch("add resource aws_instance web", m)
	result := dispatch("style web", m)
	if !strings.HasPrefix(result, "ERROR:") {
		t.Errorf("expected error, got: %s", result)
	}
}

func TestHandler_Style_NotFound(t *testing.T) {
	m := NewModel("test")
	result := dispatch(`style nonexistent tags:"a=b"`, m)
	if !strings.HasPrefix(result, "ERROR:") {
		t.Errorf("expected error, got: %s", result)
	}
}

// ── nest/unnest tests ────────────────────────────────────

func TestHandler_Nest(t *testing.T) {
	m := NewModel("test")
	dispatch("add resource aws_security_group sg", m)
	result := dispatch("nest sg ingress protocol:tcp from_port:443 to_port:443", m)
	if !strings.HasPrefix(result, "+") {
		t.Errorf("expected + prefix, got: %s", result)
	}
	if !strings.Contains(result, "ingress block added") {
		t.Errorf("expected nest message: %s", result)
	}

	hcl := string(m.Bytes())
	if !strings.Contains(hcl, "ingress {") && !strings.Contains(hcl, "ingress{") {
		t.Errorf("HCL missing nested block:\n%s", hcl)
	}
	if !strings.Contains(hcl, `"tcp"`) {
		t.Errorf("HCL missing protocol:\n%s", hcl)
	}
}

func TestHandler_Nest_MissingArgs(t *testing.T) {
	m := NewModel("test")
	dispatch("add resource aws_security_group sg", m)
	result := dispatch("nest sg", m)
	if !strings.HasPrefix(result, "ERROR:") {
		t.Errorf("expected error, got: %s", result)
	}
}

func TestHandler_Nest_NotFound(t *testing.T) {
	m := NewModel("test")
	result := dispatch("nest nonexistent ingress", m)
	if !strings.HasPrefix(result, "ERROR:") {
		t.Errorf("expected error, got: %s", result)
	}
}

func TestHandler_Unnest(t *testing.T) {
	m := NewModel("test")
	dispatch("add resource aws_security_group sg", m)
	dispatch("nest sg ingress protocol:tcp from_port:80 to_port:80", m)
	dispatch("nest sg ingress protocol:tcp from_port:443 to_port:443", m)

	hclBefore := string(m.Bytes())
	if !strings.Contains(hclBefore, "ingress") {
		t.Fatalf("expected ingress blocks:\n%s", hclBefore)
	}

	// Remove last ingress (443)
	result := dispatch("unnest sg ingress", m)
	if !strings.HasPrefix(result, "-") {
		t.Errorf("expected - prefix, got: %s", result)
	}

	hclAfter := string(m.Bytes())
	// Should still have one ingress (80)
	if !strings.Contains(hclAfter, "ingress") {
		t.Errorf("HCL should still have one ingress:\n%s", hclAfter)
	}
}

func TestHandler_Unnest_ByIndex(t *testing.T) {
	m := NewModel("test")
	dispatch("add resource aws_security_group sg", m)
	dispatch("nest sg ingress protocol:tcp from_port:80 to_port:80", m)
	dispatch("nest sg ingress protocol:tcp from_port:443 to_port:443", m)

	// Remove first ingress (index 0 = port 80)
	result := dispatch("unnest sg ingress 0", m)
	if !strings.HasPrefix(result, "-") {
		t.Errorf("expected - prefix, got: %s", result)
	}

	hcl := string(m.Bytes())
	// Should still have port 443 but not port 80
	if !strings.Contains(hcl, "443") {
		t.Errorf("HCL should still have 443:\n%s", hcl)
	}
}

func TestHandler_Unnest_NoMatch(t *testing.T) {
	m := NewModel("test")
	dispatch("add resource aws_security_group sg", m)
	result := dispatch("unnest sg egress", m)
	if !strings.HasPrefix(result, "ERROR:") {
		t.Errorf("expected error, got: %s", result)
	}
}

func TestHandler_Unnest_IndexOutOfRange(t *testing.T) {
	m := NewModel("test")
	dispatch("add resource aws_security_group sg", m)
	dispatch("nest sg ingress protocol:tcp", m)
	result := dispatch("unnest sg ingress 5", m)
	if !strings.HasPrefix(result, "ERROR:") {
		t.Errorf("expected error, got: %s", result)
	}
}

// ── unknown verb ─────────────────────────────────────────

func TestHandler_UnknownVerb(t *testing.T) {
	m := NewModel("test")
	result := dispatch("explode everything", m)
	if !strings.HasPrefix(result, "ERROR:") {
		t.Errorf("expected error, got: %s", result)
	}
}

func TestHandler_UnknownVerb_Suggestion(t *testing.T) {
	m := NewModel("test")
	result := dispatch("ad resource aws_instance web", m)
	if !strings.Contains(result, "Did you mean") {
		t.Errorf("expected suggestion, got: %s", result)
	}
}

// ── integration: full workflow ───────────────────────────

func TestHandler_FullWorkflow(t *testing.T) {
	m := NewModel("infra")

	// Add provider
	dispatch("add provider aws region:us-east-1", m)

	// Add variable
	dispatch("add variable env type:string default:prod", m)

	// Add resources
	dispatch("add resource aws_vpc main cidr_block:10.0.0.0/16", m)
	dispatch("add resource aws_subnet public vpc_id:aws_vpc.main.id cidr_block:10.0.1.0/24", m)
	dispatch("add resource aws_instance web ami:ami-123 instance_type:t2.micro subnet_id:aws_subnet.public.id", m)

	// Add data source
	dispatch("add data aws_ami latest most_recent:true", m)

	// Add output
	dispatch("add output vpc_id value:aws_vpc.main.id", m)

	// Connect
	dispatch("connect web -> main label:depends_on", m)

	// Set attribute
	dispatch("set web instance_type:t3.large", m)

	// Add nested block
	dispatch("nest web root_block_device volume_size:100 volume_type:gp3", m)

	// Style with tags
	dispatch(`style web tags:"env=prod,team=platform"`, m)

	// Verify HCL output
	hcl := string(m.Bytes())

	for _, want := range []string{
		`provider "aws"`,
		`variable "env"`,
		`resource "aws_vpc" "main"`,
		`resource "aws_subnet" "public"`,
		`resource "aws_instance" "web"`,
		`data "aws_ami" "latest"`,
		`output "vpc_id"`,
		"t3.large",     // updated instance_type
		"root_block_device", // nested block
	} {
		if !strings.Contains(hcl, want) {
			t.Errorf("HCL missing %q:\n%s", want, hcl)
		}
	}

	// Verify references are unquoted
	if strings.Contains(hcl, `"aws_vpc.main.id"`) {
		t.Errorf("reference should not be quoted:\n%s", hcl)
	}

	// Verify tags in index
	ref := m.Index.Get("web")
	if ref.Tags["env"] != "prod" {
		t.Error("tag env should be prod")
	}

	// Remove resource
	dispatch("remove latest", m)
	hclAfter := string(m.Bytes())
	if strings.Contains(hclAfter, "aws_ami") {
		t.Errorf("removed block should not appear in HCL:\n%s", hclAfter)
	}
}
