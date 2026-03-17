# fcp-terraform

MCP server for Terraform HCL generation through intent-level commands.

## What It Does

fcp-terraform lets LLMs build Terraform configurations by describing infrastructure intent -- resources, data sources, variables, outputs -- and renders them into valid HCL. Instead of writing raw HCL syntax, the LLM sends operations like `add resource aws_instance web ami:"ami-0c55b159" instance_type:t2.micro` and fcp-terraform manages the semantic model, dependency graph, and serialization. Built on the [FCP](https://github.com/os-tack/fcp) framework.

Written in Go using HashiCorp's `hclwrite` library for native HCL AST generation -- no string concatenation or template rendering.

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

### Verb Reference

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
| `nest` | `nest LABEL BLOCK_TYPE[/CHILD_TYPE] [key:value...]` |
| `remove` | `remove LABEL` or `remove @SELECTOR` |
| `label` | `label OLD_LABEL "new_label"` |
| `style` | `style LABEL tags:"Key=Val,Key2=Val2"` |

### Selectors

```
@type:aws_instance      All resources of a given type
@provider:aws            All blocks from a given provider
@kind:resource           All blocks of kind (resource, variable, output, data)
@tag:KEY or @tag:KEY=VAL Blocks matching a tag
@all                     All blocks
```

## Installation

### Go install

```bash
go install github.com/os-tack/fcp-terraform/cmd/fcp-terraform@latest
```

### GitHub Releases

Download a prebuilt binary from [Releases](https://github.com/os-tack/fcp-terraform/releases) and put `fcp-terraform` on your PATH.

### MCP Client Configuration

```json
{
  "mcpServers": {
    "fcp-terraform": {
      "command": "fcp-terraform"
    }
  }
}
```

## Architecture

```
cmd/fcp-terraform/main.go     MCP server — 4 tools, stdio transport
        │
internal/terraform/            Domain layer
  ├── model.go                 Semantic model (blocks, attributes, connections)
  ├── adapter.go               FCP adapter (dispatch ops → handlers)
  ├── handlers.go              Verb handlers (add, set, remove, nest, etc.)
  ├── queries.go               Query dispatcher (plan, graph, describe, etc.)
  ├── selectors.go             @type, @provider, @kind, @tag, @all
  ├── values.go                Attribute type inference, hclwrite value generation
  ├── verb_specs.go            Verb specifications and reference card sections
  ├── block_ref.go             Terraform reference detection
  └── index.go                 Label index for O(1) lookups
        │
internal/fcpcore/              Shared FCP framework (Go port)
  ├── tokenizer.go             DSL tokenizer
  ├── parsed_op.go             Operation parser
  ├── verb_registry.go         Verb spec registry + reference card generator
  ├── event_log.go             Event sourcing (undo/redo)
  ├── session.go               Session lifecycle (new, open, save, checkpoint)
  └── formatter.go             Response formatter
```

HCL generation uses HashiCorp's `hclwrite` package for native AST manipulation. Provider is auto-detected from resource type prefixes (`aws_`, `google_`, `azurerm_`).

## Worked Example: AWS Production Web Stack

A realistic deployment showing how operations compose. This example creates a VPC with subnets, EC2, RDS, S3, IAM, and security groups -- 13 resources total.

```
terraform_session('new "Acme Corp Web Stack"')

# Provider + Variables
terraform([
  'add provider aws region:us-east-1',
  'add variable environment type:string default:"production" description:"Deployment environment"',
  'add variable project_name type:string default:"acme-web" description:"Project name"',
  'add variable vpc_cidr type:string default:"10.0.0.0/16" description:"VPC CIDR block"',
  'add variable instance_type type:string default:"t3.medium" description:"EC2 instance type"',
  'add variable db_instance_class type:string default:"db.t3.medium" description:"RDS instance class"',
])

# Networking
terraform([
  'add resource aws_vpc main cidr_block:var.vpc_cidr enable_dns_support:true enable_dns_hostnames:true',
  'add resource aws_subnet public_a vpc_id:aws_vpc.main.id cidr_block:"10.0.1.0/24" map_public_ip_on_launch:true',
  'add resource aws_subnet public_b vpc_id:aws_vpc.main.id cidr_block:"10.0.2.0/24" map_public_ip_on_launch:true',
  'add resource aws_internet_gateway igw vpc_id:aws_vpc.main.id',
  'add resource aws_route_table public vpc_id:aws_vpc.main.id',
])

# Nested blocks for route table and security groups
terraform([
  'nest public route cidr_block:"0.0.0.0/0" gateway_id:aws_internet_gateway.igw.id',
  'add resource aws_security_group web name:"${var.project_name}-web-sg" vpc_id:aws_vpc.main.id',
  'nest web ingress from_port:80 to_port:80 protocol:"tcp"',
  'nest web ingress from_port:443 to_port:443 protocol:"tcp"',
  'nest web egress from_port:0 to_port:0 protocol:"-1"',
])

# Compute + Database
terraform([
  'add resource aws_instance webserver ami:"ami-0c55b159" instance_type:var.instance_type subnet_id:aws_subnet.public_a.id',
  'nest webserver root_block_device volume_size:20 volume_type:"gp3"',
  'add resource aws_db_subnet_group dbsubnet name:"${var.project_name}-db-subnet"',
  'set dbsubnet subnet_ids:"[aws_subnet.public_a.id,aws_subnet.public_b.id]"',
  'add resource aws_db_instance rds engine:"postgresql" instance_class:var.db_instance_class allocated_storage:20',
  'add resource aws_s3_bucket assets bucket:"${var.project_name}-assets-${var.environment}"',
])

# Outputs
terraform([
  'add output vpc_id value:aws_vpc.main.id',
  'add output web_ip value:aws_instance.webserver.public_ip',
  'add output db_endpoint value:aws_db_instance.rds.endpoint',
])

# Tags (all resources at once)
terraform([
  'style main tags:"Name=${var.project_name}-vpc,Environment=${var.environment}"',
  'style public_a tags:"Name=${var.project_name}-public-a,Environment=${var.environment}"',
  'style public_b tags:"Name=${var.project_name}-public-b,Environment=${var.environment}"',
  'style igw tags:"Name=${var.project_name}-igw,Environment=${var.environment}"',
  'style webserver tags:"Name=${var.project_name}-web,Environment=${var.environment}"',
  'style rds tags:"Name=${var.project_name}-db,Environment=${var.environment}"',
  'style assets tags:"Name=${var.project_name}-assets,Environment=${var.environment}"',
])

# Day-2 modifications
terraform([
  'label webserver app_server',                         # Rename
  'set instance_type default:"t3.large"',               # Change default
  'remove assets',                                       # Replace S3 bucket
  'add resource aws_s3_bucket static_assets bucket:"${var.project_name}-static-${var.environment}"',
])

terraform_query('plan')  # Export final HCL
```

## Development

```bash
go test ./...                        # Run all tests
go build ./cmd/fcp-terraform         # Build binary
```

## License

MIT
