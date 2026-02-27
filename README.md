# fcp-terraform

MCP server for Terraform HCL generation through intent-level commands.

## What It Does

fcp-terraform lets LLMs build Terraform configurations by describing infrastructure intent -- resources, data sources, variables, outputs -- and renders them into valid HCL. Instead of writing raw HCL syntax, the LLM sends operations like `add resource aws_instance web ami:"ami-0c55b159" instance_type:t2.micro` and fcp-terraform manages the semantic model, dependency graph, and serialization. Built on the [FCP](https://github.com/aetherwing-io/fcp) framework.

## Quick Example

```
terraform_session('new "Main Infrastructure"')

terraform([
  'add resource aws_instance web ami:"ami-0c55b159" instance_type:t2.micro',
  'add resource aws_s3_bucket assets bucket:"my-assets"',
  'add variable region default:"us-east-1" type:string',
  'add output instance_ip value:"aws_instance.web.public_ip"',
])

terraform_query('plan')
```

The `plan` query produces:

```hcl
variable "region" {
  type    = string
  default = "us-east-1"
}

resource "aws_instance" "web" {
  ami           = "ami-0c55b159"
  instance_type = "t2.micro"
}

resource "aws_s3_bucket" "assets" {
  bucket = "my-assets"
}

output "instance_ip" {
  value = aws_instance.web.public_ip
}
```

### Available MCP Tools

| Tool | Purpose |
|------|---------|
| `terraform(ops)` | Batch mutations -- add, set, remove, connect, nest, label, style |
| `terraform_query(q)` | Inspect the config -- map, list, describe, plan, graph, validate, find |
| `terraform_session(action)` | Lifecycle -- new, open, save, checkpoint, undo, redo |
| `terraform_help()` | Full reference card |

### Supported Block Types

| Verb | Syntax |
|------|--------|
| `add resource` | `add resource TYPE LABEL [key:value...]` |
| `add provider` | `add provider PROVIDER [region:R] [key:value...]` |
| `add variable` | `add variable NAME [type:T] [default:V] [description:D]` |
| `add output` | `add output NAME value:EXPR [description:D]` |
| `add data` | `add data TYPE LABEL [key:value...]` |
| `add module` | `add module LABEL source:PATH [key:value...]` |
| `connect` | `connect SRC -> TGT [label:TEXT]` |
| `set` | `set LABEL key:value [key:value...]` |
| `nest` | `nest LABEL BLOCK_TYPE [key:value...]` |
| `remove` | `remove LABEL` or `remove @SELECTOR` |

### Selectors

```
@type:aws_instance      All resources of a given type
@provider:aws            All blocks from a given provider
@kind:resource           All blocks of kind (resource, variable, output, data)
@tag:KEY or @tag:KEY=VAL Blocks matching a tag
@all                     All blocks
```

## Installation

Requires Node >= 18.

```bash
npm install fcp-terraform
```

### MCP Client Configuration

```json
{
  "mcpServers": {
    "terraform": {
      "command": "node",
      "args": ["node_modules/fcp-terraform/dist/index.js"]
    }
  }
}
```

## Architecture

3-layer architecture:

```
MCP Server (Intent Layer)
  src/server/ -- Parses op strings, resolves refs, dispatches
        |
Semantic Model (Domain)
  src/model/ -- In-memory Terraform graph (resources, data, variables, outputs)
  src/types/ -- Core TypeScript interfaces
        |
Serialization (HCL)
  src/hcl.ts -- Semantic model -> HCL text output
```

Supporting modules:

- `src/ops.ts` -- Operation string parser
- `src/verbs.ts` -- Verb specs and reference card
- `src/queries.ts` -- Query dispatcher (map, plan, graph, validate, etc.)
- `src/adapter.ts` -- FCP core adapter

Provider is auto-detected from resource type prefixes (`aws_`, `google_`, `azurerm_`).

## Development

```bash
npm install
npm run build     # tsc
npm test          # vitest, 138 tests
```

## License

MIT

