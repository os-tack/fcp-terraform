package terraform

import "github.com/os-tack/fcp-terraform/internal/fcpcore"

// TerraformVerbSpecs returns all verb specifications for the Terraform FCP server.
func TerraformVerbSpecs() []fcpcore.VerbSpec {
	return []fcpcore.VerbSpec{
		// Resources
		{Name: "add resource", Syntax: "add resource TYPE LABEL [key:value...]", Category: "resources"},
		{Name: "add provider", Syntax: "add provider PROVIDER [region:R] [key:value...]", Category: "resources"},
		{Name: "add variable", Syntax: "add variable NAME [type:T] [default:V] [description:D]", Category: "resources"},
		{Name: "add output", Syntax: "add output NAME value:EXPR [description:D]", Category: "resources"},
		{Name: "add data", Syntax: "add data TYPE LABEL [key:value...]", Category: "resources"},
		{Name: "add module", Syntax: "add module LABEL source:PATH [key:value...]", Category: "resources"},
		// Connections
		{Name: "connect", Syntax: "connect SRC -> TGT [label:TEXT]", Category: "connections"},
		{Name: "disconnect", Syntax: "disconnect SRC -> TGT", Category: "connections"},
		// Editing
		{Name: "set", Syntax: "set LABEL key:value [key:value...]", Category: "editing"},
		{Name: "unset", Syntax: "unset LABEL KEY [KEY...]", Category: "editing"},
		{Name: "remove", Syntax: "remove LABEL | remove @SELECTOR", Category: "editing"},
		{Name: "label", Syntax: `label OLD_LABEL "new_label"`, Category: "editing"},
		{Name: "style", Syntax: `style LABEL tags:"Key=Val,Key2=Val2"`, Category: "editing"},
		{Name: "tag", Syntax: "tag LABEL key:value [key:value...] | tag @SELECTOR key:value...", Category: "editing"},
		{Name: "nest", Syntax: "nest LABEL BLOCK_TYPE[/CHILD_TYPE] [key:value...]", Category: "editing"},
		{Name: "unnest", Syntax: "unnest LABEL BLOCK_TYPE [INDEX]", Category: "editing"},
	}
}

// ExtraSections returns the extra reference card sections for selectors and response prefixes.
func ExtraSections() map[string]string {
	return map[string]string{
		"selectors": `  @type:aws_instance      All resources of a given type
  @provider:aws            All blocks from a given provider
  @kind:resource            All blocks of kind (resource, variable, output, data)
  @tag:KEY or @tag:KEY=VAL  Blocks matching a tag
  @all                      All blocks`,
		"response prefixes": `  +  block created        ~  connection created/modified
  *  block modified       -  block/connection removed
  !  meta operation       @  bulk operation`,
	}
}
