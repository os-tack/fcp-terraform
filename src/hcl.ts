import type { TerraformConfig, TfBlock, Attribute, NestedBlock, Connection } from "./types.js";

/**
 * Serialize a TerraformConfig to valid HCL.
 */
export function serializeToHcl(config: TerraformConfig): string {
  const sections: string[] = [];

  // Group blocks by kind in Terraform-idiomatic order
  const providers = getBlocksByKind(config, "provider");
  const variables = getBlocksByKind(config, "variable");
  const data = getBlocksByKind(config, "data");
  const resources = getBlocksByKind(config, "resource");
  const modules = getBlocksByKind(config, "module");
  const outputs = getBlocksByKind(config, "output");

  for (const block of providers) sections.push(serializeProvider(block));
  for (const block of variables) sections.push(serializeVariable(block));
  for (const block of data) sections.push(serializeData(block));
  for (const block of modules) sections.push(serializeModule(block));
  for (const block of resources) sections.push(serializeResource(block, config));
  for (const block of outputs) sections.push(serializeOutput(block));

  return sections.join("\n\n") + "\n";
}

function getBlocksByKind(config: TerraformConfig, kind: string): TfBlock[] {
  return config.blockOrder
    .map((id) => config.blocks.get(id)!)
    .filter((b) => b && b.kind === kind);
}

function serializeProvider(block: TfBlock): string {
  const lines: string[] = [];
  lines.push(`provider "${block.fullType}" {`);
  for (const attr of block.attributes.values()) {
    lines.push(`  ${formatAttribute(attr)}`);
  }
  lines.push("}");
  return lines.join("\n");
}

function serializeVariable(block: TfBlock): string {
  const lines: string[] = [];
  lines.push(`variable "${block.label}" {`);
  for (const attr of block.attributes.values()) {
    if (attr.key === "type") {
      // type is unquoted in variables
      lines.push(`  type    = ${attr.value}`);
    } else {
      lines.push(`  ${formatAttribute(attr)}`);
    }
  }
  lines.push("}");
  return lines.join("\n");
}

function serializeData(block: TfBlock): string {
  const lines: string[] = [];
  lines.push(`data "${block.fullType}" "${block.label}" {`);
  for (const attr of block.attributes.values()) {
    lines.push(`  ${formatAttribute(attr)}`);
  }
  for (const nested of block.nestedBlocks) {
    lines.push(...serializeNestedBlock(nested, 2));
  }
  lines.push("}");
  return lines.join("\n");
}

function serializeModule(block: TfBlock): string {
  const lines: string[] = [];
  lines.push(`module "${block.label}" {`);
  for (const attr of block.attributes.values()) {
    lines.push(`  ${formatAttribute(attr)}`);
  }
  lines.push("}");
  return lines.join("\n");
}

function serializeResource(block: TfBlock, config: TerraformConfig): string {
  const lines: string[] = [];
  lines.push(`resource "${block.fullType}" "${block.label}" {`);

  // Attributes
  for (const attr of block.attributes.values()) {
    lines.push(`  ${formatAttribute(attr)}`);
  }

  // Meta: count
  if (block.meta.count) {
    lines.push(`  count = ${block.meta.count}`);
  }
  if (block.meta.forEach) {
    lines.push(`  for_each = ${block.meta.forEach}`);
  }

  // Tags
  if (block.tags.size > 0) {
    lines.push("");
    lines.push("  tags = {");
    for (const [k, v] of block.tags) {
      lines.push(`    ${k} = "${v}"`);
    }
    lines.push("  }");
  }

  // Nested blocks
  for (const nested of block.nestedBlocks) {
    lines.push("");
    lines.push(...serializeNestedBlock(nested, 2));
  }

  // depends_on from connections
  const deps = getDependsOn(block, config);
  if (deps.length > 0) {
    lines.push("");
    lines.push(`  depends_on = [${deps.join(", ")}]`);
  }

  // Explicit depends_on from meta
  if (block.meta.dependsOn.length > 0) {
    const existing = deps.length > 0;
    if (!existing) {
      lines.push("");
      lines.push(`  depends_on = [${block.meta.dependsOn.join(", ")}]`);
    }
  }

  lines.push("}");
  return lines.join("\n");
}

function serializeOutput(block: TfBlock): string {
  const lines: string[] = [];
  lines.push(`output "${block.label}" {`);
  for (const attr of block.attributes.values()) {
    if (attr.key === "value" && (attr.valueType === "reference" || attr.valueType === "expression")) {
      lines.push(`  value = ${attr.value}`);
    } else {
      lines.push(`  ${formatAttribute(attr)}`);
    }
  }
  lines.push("}");
  return lines.join("\n");
}

function serializeNestedBlock(nested: NestedBlock, indent: number): string[] {
  const pad = " ".repeat(indent);
  const lines: string[] = [];
  lines.push(`${pad}${nested.type} {`);
  for (const attr of nested.attributes.values()) {
    lines.push(`${pad}  ${formatAttribute(attr)}`);
  }
  lines.push(`${pad}}`);
  return lines;
}

function formatAttribute(attr: Attribute): string {
  const padded = attr.key.padEnd(Math.max(attr.key.length, 4));
  switch (attr.valueType) {
    case "string":
      return `${padded} = "${attr.value}"`;
    case "number":
    case "bool":
    case "reference":
      return `${padded} = ${attr.value}`;
    case "expression":
      return `${padded} = ${attr.value}`;
    case "list":
      return `${padded} = ${attr.value}`;
    case "map":
      return `${padded} = ${attr.value}`;
    default:
      return `${padded} = "${attr.value}"`;
  }
}

function getDependsOn(block: TfBlock, config: TerraformConfig): string[] {
  const deps: string[] = [];
  for (const conn of config.connections.values()) {
    if (conn.sourceId === block.id) {
      const target = config.blocks.get(conn.targetId);
      if (target && (target.kind === "resource" || target.kind === "data")) {
        deps.push(`${target.fullType}.${target.label}`);
      }
    }
  }
  return deps;
}
