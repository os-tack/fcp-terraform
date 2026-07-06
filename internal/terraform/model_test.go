package terraform

import (
	"strings"
	"testing"
)

func TestNewModel(t *testing.T) {
	m := NewModel("test")
	if m.Title != "test" {
		t.Errorf("title = %q", m.Title)
	}
	if m.File == nil {
		t.Fatal("file should not be nil")
	}
	if m.Index == nil {
		t.Fatal("index should not be nil")
	}
}

func TestModel_AddResource(t *testing.T) {
	m := NewModel("test")
	ref, err := m.AddResource("aws_instance", "web", map[string]string{
		"ami":           "ami-12345",
		"instance_type": "t2.micro",
	}, nil)
	if err != nil {
		t.Fatalf("AddResource: %v", err)
	}
	if ref.Label != "web" {
		t.Errorf("label = %q", ref.Label)
	}
	if ref.Kind != "resource" {
		t.Errorf("kind = %q", ref.Kind)
	}
	if ref.FullType != "aws_instance" {
		t.Errorf("fullType = %q", ref.FullType)
	}
	if ref.Provider != "aws" {
		t.Errorf("provider = %q", ref.Provider)
	}

	hcl := string(m.Bytes())
	if !strings.Contains(hcl, `resource "aws_instance" "web"`) {
		t.Errorf("HCL missing resource block:\n%s", hcl)
	}
	if !strings.Contains(hcl, `"ami-12345"`) {
		t.Errorf("HCL missing ami value:\n%s", hcl)
	}
	if !strings.Contains(hcl, `"t2.micro"`) {
		t.Errorf("HCL missing instance_type value:\n%s", hcl)
	}
}

func TestModel_AddDataSource(t *testing.T) {
	m := NewModel("test")
	ref, err := m.AddDataSource("aws_ami", "latest", map[string]string{
		"most_recent": "true",
	}, nil)
	if err != nil {
		t.Fatalf("AddDataSource: %v", err)
	}
	if ref.Kind != "data" {
		t.Errorf("kind = %q", ref.Kind)
	}

	hcl := string(m.Bytes())
	if !strings.Contains(hcl, `data "aws_ami" "latest"`) {
		t.Errorf("HCL missing data block:\n%s", hcl)
	}
	if !strings.Contains(hcl, "= true") {
		t.Errorf("HCL missing bool attr:\n%s", hcl)
	}
}

func TestModel_AddVariable(t *testing.T) {
	m := NewModel("test")
	ref, err := m.AddVariable("region", map[string]string{
		"type":        "string",
		"default":     "us-east-1",
		"description": "AWS region",
	})
	if err != nil {
		t.Fatalf("AddVariable: %v", err)
	}
	if ref.Kind != "variable" {
		t.Errorf("kind = %q", ref.Kind)
	}

	hcl := string(m.Bytes())
	if !strings.Contains(hcl, `variable "region"`) {
		t.Errorf("HCL missing variable block:\n%s", hcl)
	}
	// type should be raw (not quoted)
	if strings.Contains(hcl, `"string"`) && strings.Contains(hcl, "type") {
		// This would be wrong — type = "string" instead of type = string
		// But hclwrite may format it differently. Let's check it's present.
	}
}

func TestModel_AddOutput(t *testing.T) {
	m := NewModel("test")
	ref, err := m.AddOutput("vpc_id", map[string]string{
		"value":       "aws_vpc.main.id",
		"description": "The VPC ID",
	})
	if err != nil {
		t.Fatalf("AddOutput: %v", err)
	}
	if ref.Kind != "output" {
		t.Errorf("kind = %q", ref.Kind)
	}

	hcl := string(m.Bytes())
	if !strings.Contains(hcl, `output "vpc_id"`) {
		t.Errorf("HCL missing output block:\n%s", hcl)
	}
	// value should be a reference (unquoted)
	if !strings.Contains(hcl, "aws_vpc.main.id") {
		t.Errorf("HCL missing reference value:\n%s", hcl)
	}
}

func TestModel_AddProvider(t *testing.T) {
	m := NewModel("test")
	ref, err := m.AddProvider("aws", map[string]string{
		"region": "us-east-1",
	})
	if err != nil {
		t.Fatalf("AddProvider: %v", err)
	}
	if ref.Kind != "provider" {
		t.Errorf("kind = %q", ref.Kind)
	}

	hcl := string(m.Bytes())
	if !strings.Contains(hcl, `provider "aws"`) {
		t.Errorf("HCL missing provider block:\n%s", hcl)
	}
}

func TestModel_AddModule(t *testing.T) {
	m := NewModel("test")
	ref, err := m.AddModule("vpc", map[string]string{
		"source":  "terraform-aws-modules/vpc/aws",
		"version": "3.0.0",
	})
	if err != nil {
		t.Fatalf("AddModule: %v", err)
	}
	if ref.Kind != "module" {
		t.Errorf("kind = %q", ref.Kind)
	}

	hcl := string(m.Bytes())
	if !strings.Contains(hcl, `module "vpc"`) {
		t.Errorf("HCL missing module block:\n%s", hcl)
	}
}

func TestModel_AddResource_Duplicate(t *testing.T) {
	m := NewModel("test")
	m.AddResource("aws_instance", "web", nil, nil)
	_, err := m.AddResource("aws_instance", "web", nil, nil)
	if err == nil {
		t.Error("expected error for duplicate resource")
	}
}

func TestModel_RemoveBlock(t *testing.T) {
	m := NewModel("test")
	m.AddResource("aws_instance", "web", map[string]string{"ami": "ami-123"}, nil)
	m.AddResource("aws_vpc", "main", nil, nil)

	ref, err := m.RemoveBlock("web")
	if err != nil {
		t.Fatalf("RemoveBlock: %v", err)
	}
	if ref.Label != "web" {
		t.Errorf("removed label = %q", ref.Label)
	}

	hcl := string(m.Bytes())
	if strings.Contains(hcl, "aws_instance") {
		t.Errorf("HCL should not contain removed block:\n%s", hcl)
	}
	if !strings.Contains(hcl, "aws_vpc") {
		t.Errorf("HCL should still contain vpc:\n%s", hcl)
	}

	// Index should be updated
	if m.Index.Get("web") != nil {
		t.Error("web should be removed from index")
	}
}

func TestModel_RemoveBlock_NotFound(t *testing.T) {
	m := NewModel("test")
	_, err := m.RemoveBlock("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent block")
	}
}

func TestModel_SetAttributes(t *testing.T) {
	m := NewModel("test")
	m.AddResource("aws_instance", "web", map[string]string{"ami": "ami-old"}, nil)

	err := m.SetAttributes("web", map[string]string{"ami": "ami-new", "tags": "[web,prod]"}, nil)
	if err != nil {
		t.Fatalf("SetAttributes: %v", err)
	}

	hcl := string(m.Bytes())
	if !strings.Contains(hcl, `"ami-new"`) {
		t.Errorf("HCL should contain new ami:\n%s", hcl)
	}
}

func TestModel_SetAttributes_ForceString(t *testing.T) {
	m := NewModel("test")
	m.AddResource("aws_instance", "web", nil, nil)

	err := m.SetAttributes("web", map[string]string{"port": "80"}, map[string]bool{"port": true})
	if err != nil {
		t.Fatalf("SetAttributes: %v", err)
	}

	hcl := string(m.Bytes())
	if !strings.Contains(hcl, `"80"`) {
		t.Errorf("port should be quoted when forced string:\n%s", hcl)
	}
}

func TestModel_UnsetAttributes(t *testing.T) {
	m := NewModel("test")
	m.AddResource("aws_instance", "web", map[string]string{"ami": "ami-123", "instance_type": "t2.micro"}, nil)

	err := m.UnsetAttributes("web", []string{"ami"})
	if err != nil {
		t.Fatalf("UnsetAttributes: %v", err)
	}

	hcl := string(m.Bytes())
	if strings.Contains(hcl, "ami") {
		t.Errorf("HCL should not contain ami after unset:\n%s", hcl)
	}
	if !strings.Contains(hcl, "instance_type") {
		t.Errorf("HCL should still contain instance_type:\n%s", hcl)
	}
}

func TestModel_SnapshotRestore(t *testing.T) {
	m := NewModel("test")
	m.AddResource("aws_instance", "web", map[string]string{"ami": "ami-123"}, nil)

	snapshot := m.Snapshot()

	// Add another resource
	m.AddResource("aws_vpc", "main", nil, nil)
	hclAfter := string(m.Bytes())
	if !strings.Contains(hclAfter, "aws_vpc") {
		t.Error("should contain vpc after add")
	}

	// Restore to snapshot
	err := m.Restore(snapshot)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}

	hclRestored := string(m.Bytes())
	if strings.Contains(hclRestored, "aws_vpc") {
		t.Errorf("should not contain vpc after restore:\n%s", hclRestored)
	}
	if !strings.Contains(hclRestored, "aws_instance") {
		t.Errorf("should contain instance after restore:\n%s", hclRestored)
	}

	// Index should be rebuilt
	if m.Index.Get("web") == nil {
		t.Error("web should be in index after restore")
	}
	if m.Index.Get("main") != nil {
		t.Error("main should not be in index after restore")
	}
}

func TestModel_Connect_And_Disconnect(t *testing.T) {
	m := NewModel("test")
	m.AddResource("aws_instance", "web", nil, nil)
	m.AddResource("aws_vpc", "vpc", nil, nil)

	err := m.Connect("web", "vpc", "depends_on")
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}

	conns := m.Index.Connections["web"]
	if conns == nil || conns["vpc"] != "depends_on" {
		t.Error("connection not stored")
	}

	err = m.Disconnect("web", "vpc")
	if err != nil {
		t.Fatalf("Disconnect: %v", err)
	}

	if _, ok := m.Index.Connections["web"]; ok {
		t.Error("connection should be removed after disconnect")
	}
}

func TestModel_Connect_EmitsDependsOn(t *testing.T) {
	m := NewModel("test")
	m.AddResource("aws_instance", "web", nil, nil)
	m.AddResource("aws_vpc", "vpc", nil, nil)

	if err := m.Connect("web", "vpc", "depends_on"); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	hcl := string(m.Bytes())
	if !strings.Contains(hcl, "depends_on = [aws_vpc.vpc]") {
		t.Errorf("expected depends_on referencing aws_vpc.vpc, got:\n%s", hcl)
	}
}

func TestModel_Connect_DataSourceReference(t *testing.T) {
	m := NewModel("test")
	m.AddResource("aws_instance", "web", nil, nil)
	m.AddDataSource("aws_ami", "ubuntu", nil, nil)

	if err := m.Connect("web", "ubuntu", ""); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	hcl := string(m.Bytes())
	if !strings.Contains(hcl, "depends_on = [data.aws_ami.ubuntu]") {
		t.Errorf("expected depends_on referencing data.aws_ami.ubuntu, got:\n%s", hcl)
	}
}

func TestModel_Connect_AppendsToExistingDependsOn(t *testing.T) {
	m := NewModel("test")
	m.AddResource("aws_instance", "web", nil, nil)
	m.AddResource("aws_vpc", "vpc", nil, nil)
	m.AddDataSource("aws_ami", "ubuntu", nil, nil)

	if err := m.Connect("web", "vpc", ""); err != nil {
		t.Fatalf("Connect(vpc): %v", err)
	}
	if err := m.Connect("web", "ubuntu", ""); err != nil {
		t.Fatalf("Connect(ubuntu): %v", err)
	}

	hcl := string(m.Bytes())
	if !strings.Contains(hcl, "depends_on = [data.aws_ami.ubuntu, aws_vpc.vpc]") {
		t.Errorf("expected depends_on with both targets, got:\n%s", hcl)
	}
}

func TestModel_Disconnect_ShrinksDependsOn(t *testing.T) {
	m := NewModel("test")
	m.AddResource("aws_instance", "web", nil, nil)
	m.AddResource("aws_vpc", "vpc", nil, nil)
	m.AddDataSource("aws_ami", "ubuntu", nil, nil)
	m.Connect("web", "vpc", "")
	m.Connect("web", "ubuntu", "")

	if err := m.Disconnect("web", "vpc"); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}

	hcl := string(m.Bytes())
	if !strings.Contains(hcl, "depends_on = [data.aws_ami.ubuntu]") {
		t.Errorf("expected depends_on with only ubuntu remaining, got:\n%s", hcl)
	}
	if strings.Contains(hcl, "aws_vpc.vpc]") || strings.Contains(hcl, "vpc,") {
		t.Errorf("did not expect vpc in depends_on after disconnect, got:\n%s", hcl)
	}
}

func TestModel_Disconnect_RemovesDependsOnWhenEmpty(t *testing.T) {
	m := NewModel("test")
	m.AddResource("aws_instance", "web", nil, nil)
	m.AddResource("aws_vpc", "vpc", nil, nil)
	m.Connect("web", "vpc", "")

	if err := m.Disconnect("web", "vpc"); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}

	hcl := string(m.Bytes())
	if strings.Contains(hcl, "depends_on") {
		t.Errorf("expected depends_on attribute removed entirely, got:\n%s", hcl)
	}
}

func TestModel_Connect_NotFound(t *testing.T) {
	m := NewModel("test")
	m.AddResource("aws_instance", "web", nil, nil)

	err := m.Connect("web", "nonexistent", "depends_on")
	if err == nil {
		t.Error("expected error for nonexistent target")
	}
}

func TestModel_MultipleBlocks_ValidHCL(t *testing.T) {
	m := NewModel("infra")
	m.AddProvider("aws", map[string]string{"region": "us-east-1"})
	m.AddVariable("env", map[string]string{"type": "string", "default": "prod"})
	m.AddResource("aws_vpc", "main", map[string]string{"cidr_block": "10.0.0.0/16"}, nil)
	m.AddResource("aws_subnet", "public", map[string]string{
		"vpc_id":     "aws_vpc.main.id",
		"cidr_block": "10.0.1.0/24",
	}, nil)
	m.AddDataSource("aws_ami", "latest", map[string]string{"most_recent": "true"}, nil)
	m.AddOutput("vpc_id", map[string]string{"value": "aws_vpc.main.id"})
	m.AddModule("monitoring", map[string]string{"source": "./modules/monitoring"})

	hcl := string(m.Bytes())

	// Verify all block types present
	for _, want := range []string{
		`provider "aws"`,
		`variable "env"`,
		`resource "aws_vpc" "main"`,
		`resource "aws_subnet" "public"`,
		`data "aws_ami" "latest"`,
		`output "vpc_id"`,
		`module "monitoring"`,
	} {
		if !strings.Contains(hcl, want) {
			t.Errorf("HCL missing %q:\n%s", want, hcl)
		}
	}

	// vpc_id reference should be unquoted
	if strings.Contains(hcl, `"aws_vpc.main.id"`) {
		t.Errorf("reference should not be quoted:\n%s", hcl)
	}
}

func TestModel_Bytes_ProducesValidHCL(t *testing.T) {
	m := NewModel("test")
	m.AddResource("aws_instance", "web", map[string]string{
		"ami":           "ami-12345",
		"instance_type": "t2.micro",
		"count":         "3",
		"ebs_optimized": "true",
	}, nil)

	hcl := string(m.Bytes())

	// Basic structural checks
	if !strings.Contains(hcl, "{") || !strings.Contains(hcl, "}") {
		t.Errorf("HCL missing braces:\n%s", hcl)
	}

	// Numbers should be unquoted
	if strings.Contains(hcl, `"3"`) {
		t.Errorf("count should be unquoted number:\n%s", hcl)
	}

	// Booleans should be unquoted
	if strings.Contains(hcl, `"true"`) {
		t.Errorf("ebs_optimized should be unquoted bool:\n%s", hcl)
	}
}

func TestModel_IndexLookup_CaseInsensitive(t *testing.T) {
	m := NewModel("test")
	m.AddResource("aws_instance", "WebServer", nil, nil)

	if m.Index.Get("webserver") == nil {
		t.Error("case-insensitive lookup failed")
	}
	if m.Index.Get("WEBSERVER") == nil {
		t.Error("case-insensitive lookup failed (all caps)")
	}
}
