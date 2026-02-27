import type { VerbSpec } from "@aetherwing/fcp-core";

export const VERB_SPECS: VerbSpec[] = [
  // Resources & Providers
  { verb: "add", syntax: 'add resource TYPE LABEL [key:value...]', category: "Resources", description: "Add a resource, provider, variable, output, data source, or module" },
  { verb: "add", syntax: 'add provider PROVIDER [region:R] [key:value...]', category: "Resources" },
  { verb: "add", syntax: 'add variable NAME [type:T] [default:V] [description:D]', category: "Resources" },
  { verb: "add", syntax: 'add output NAME value:EXPR [description:D]', category: "Resources" },
  { verb: "add", syntax: 'add data TYPE LABEL [key:value...]', category: "Resources" },
  { verb: "add", syntax: 'add module LABEL source:PATH [key:value...]', category: "Resources" },

  // Connections
  { verb: "connect", syntax: 'connect SRC -> TGT [label:TEXT]', category: "Connections", description: "Create a dependency between two blocks" },
  { verb: "disconnect", syntax: 'disconnect SRC -> TGT', category: "Connections", description: "Remove a dependency" },

  // Editing
  { verb: "set", syntax: 'set LABEL key:value [key:value...]', category: "Editing", description: "Set attributes on an existing block" },
  { verb: "unset", syntax: 'unset LABEL KEY [KEY...]', category: "Editing", description: "Remove attributes from a block" },
  { verb: "remove", syntax: 'remove LABEL | remove @SELECTOR', category: "Editing", description: "Remove a block by label or selector" },
  { verb: "label", syntax: 'label OLD_LABEL "new_label"', category: "Editing", description: "Rename a block" },
  { verb: "style", syntax: 'style LABEL tags:"Key=Val,Key2=Val2"', category: "Editing", description: "Set tags on a resource" },
  { verb: "nest", syntax: 'nest LABEL BLOCK_TYPE [key:value...]', category: "Editing", description: "Add a nested block (e.g., ingress, root_block_device)" },
];

export const REFERENCE_CARD_SECTIONS: Record<string, string> = {
  "Selectors": [
    "  @type:aws_instance      All resources of a given type",
    "  @provider:aws            All blocks from a given provider",
    "  @kind:resource            All blocks of kind (resource, variable, output, data)",
    "  @tag:KEY or @tag:KEY=VAL  Blocks matching a tag",
    "  @all                      All blocks",
  ].join("\n"),

  "Response Prefixes": [
    "  +  block created        ~  connection created/modified",
    "  *  block modified       -  block/connection removed",
    "  !  meta operation       @  bulk operation",
  ].join("\n"),

  "Conventions": [
    "  - Labels are unique short names — no full type paths needed",
    "  - Provider auto-detected from resource type prefix (aws_, google_, azurerm_)",
    "  - Use `plan` query to preview generated HCL without saving",
    "  - Use `graph` query to visualize dependency graph",
    "  - Call terraform_help for full reference",
  ].join("\n"),
};
