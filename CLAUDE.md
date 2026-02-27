# fcp-terraform

## Project Overview
MCP server that lets LLMs generate Terraform HCL through intent-level operation strings.
Depends on `@fcp/core` for the shared FCP framework (tokenizer, verb registry, session, events).

## Architecture
3-layer architecture:
1. **MCP Server (Intent Layer)** - `src/server/` - Parses op strings, resolves refs, dispatches
2. **Semantic Model (Domain)** - `src/model/` - In-memory Terraform graph (resources, data sources, variables, outputs)
3. **Serialization** - `src/serialization/` - Semantic model → HCL text output

## Key Directories
- `src/model/` - Semantic model for Terraform constructs
- `src/parser/` - Operation string parser
- `src/serialization/` - HCL serialization
- `src/server/` - MCP server, tools, intent layer
- `tests/` - Test files

## Commands
- `npm run build` - Compile TypeScript
- `npm test` - Run tests (vitest)

## Conventions
- TypeScript strict mode
- ESM modules (type: "module")
- vitest for testing
- Tests in `tests/` directory
