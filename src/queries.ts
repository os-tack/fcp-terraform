import type { TerraformConfig, TfBlock } from "./types.js";
import type { EventLog } from "@aetherwing/fcp-core";
import type { TerraformEvent } from "./types.js";
import { serializeToHcl } from "./hcl.js";
import { findByLabel, findByKind } from "./model.js";

export function dispatchQuery(
  query: string,
  config: TerraformConfig,
  eventLog?: EventLog<TerraformEvent>,
): string {
  const parts = query.trim().split(/\s+/);
  const cmd = parts[0]?.toLowerCase() ?? "";
  const args = parts.slice(1);

  switch (cmd) {
    case "map":
      return queryMap(config);
    case "list":
      return queryList(config, args);
    case "describe":
      return queryDescribe(config, args[0]);
    case "plan":
      return queryPlan(config);
    case "graph":
      return queryGraph(config);
    case "validate":
      return queryValidate(config);
    case "stats":
      return queryStats(config);
    case "status":
      return queryStatus(config);
    case "find":
      return queryFind(config, args.join(" "));
    case "history":
      return queryHistory(eventLog, parseInt(args[0]) || 10);
    default:
      return `Unknown query: "${cmd}". Available: map, list, describe, plan, graph, validate, stats, status, find, history`;
  }
}

function queryMap(config: TerraformConfig): string {
  const lines: string[] = [];
  lines.push(`Terraform Config: ${config.title}`);
  lines.push("");

  const resources = findByKind(config, "resource");
  const variables = findByKind(config, "variable");
  const outputs = findByKind(config, "output");
  const providers = findByKind(config, "provider");
  const data = findByKind(config, "data");

  if (providers.length > 0) {
    lines.push(`Providers: ${providers.map((p) => p.label).join(", ")}`);
  }

  if (resources.length > 0) {
    const typeCounts = new Map<string, number>();
    for (const r of resources) {
      typeCounts.set(r.fullType, (typeCounts.get(r.fullType) ?? 0) + 1);
    }
    const types = [...typeCounts.entries()].map(([t, c]) => `${t} x${c}`).join(", ");
    lines.push(`Resources (${resources.length}): ${types}`);
  }

  if (data.length > 0) lines.push(`Data Sources: ${data.length}`);
  if (variables.length > 0) lines.push(`Variables: ${variables.length}`);
  if (outputs.length > 0) lines.push(`Outputs: ${outputs.length}`);
  if (config.connections.size > 0) lines.push(`Connections: ${config.connections.size}`);

  return lines.join("\n");
}

function queryList(config: TerraformConfig, args: string[]): string {
  let blocks = [...config.blocks.values()];

  // Filter by selector if provided
  if (args.length > 0 && args[0].startsWith("@")) {
    const sel = args[0];
    if (sel.startsWith("@type:")) {
      const type = sel.slice(6);
      blocks = blocks.filter((b) => b.fullType === type);
    } else if (sel.startsWith("@kind:")) {
      const kind = sel.slice(6);
      blocks = blocks.filter((b) => b.kind === kind);
    } else if (sel.startsWith("@provider:")) {
      const provider = sel.slice(10);
      blocks = blocks.filter((b) => b.provider === provider);
    }
  }

  if (blocks.length === 0) return "No blocks found.";

  const lines: string[] = [];
  for (const block of blocks) {
    const type = block.kind === "resource" || block.kind === "data"
      ? `${block.fullType}.${block.label}`
      : `${block.kind}.${block.label}`;
    const attrCount = block.attributes.size;
    lines.push(`  ${block.kind.padEnd(10)} ${type.padEnd(35)} (${attrCount} attrs)`);
  }
  return lines.join("\n");
}

function queryDescribe(config: TerraformConfig, label: string | undefined): string {
  if (!label) return "describe requires a LABEL";
  const block = findByLabel(config, label);
  if (!block) return `block "${label}" not found`;

  const lines: string[] = [];
  const ref = block.kind === "resource" || block.kind === "data"
    ? `${block.fullType}.${block.label}`
    : `${block.kind} "${block.label}"`;
  lines.push(`${block.kind}: ${ref}`);
  if (block.provider) lines.push(`  provider: ${block.provider}`);

  if (block.attributes.size > 0) {
    lines.push("  attributes:");
    for (const attr of block.attributes.values()) {
      lines.push(`    ${attr.key} = ${attr.valueType === "string" ? `"${attr.value}"` : attr.value} (${attr.valueType})`);
    }
  }

  if (block.tags.size > 0) {
    lines.push("  tags:");
    for (const [k, v] of block.tags) {
      lines.push(`    ${k} = "${v}"`);
    }
  }

  if (block.nestedBlocks.length > 0) {
    lines.push("  nested blocks:");
    for (const nested of block.nestedBlocks) {
      lines.push(`    ${nested.type} {}`);
      for (const attr of nested.attributes.values()) {
        lines.push(`      ${attr.key} = ${attr.value}`);
      }
    }
  }

  // Show connections
  const conns = [...config.connections.values()].filter(
    (c) => c.sourceId === block.id || c.targetId === block.id,
  );
  if (conns.length > 0) {
    lines.push("  connections:");
    for (const c of conns) {
      if (c.sourceId === block.id) {
        lines.push(`    → ${c.targetLabel}`);
      } else {
        lines.push(`    ← ${c.sourceLabel}`);
      }
    }
  }

  if (block.meta.count) lines.push(`  count: ${block.meta.count}`);
  if (block.meta.forEach) lines.push(`  for_each: ${block.meta.forEach}`);
  if (block.meta.dependsOn.length > 0) lines.push(`  depends_on: ${block.meta.dependsOn.join(", ")}`);

  return lines.join("\n");
}

function queryPlan(config: TerraformConfig): string {
  if (config.blocks.size === 0) return "Empty configuration. Add some resources first.";
  return serializeToHcl(config);
}

function queryGraph(config: TerraformConfig): string {
  if (config.connections.size === 0) return "No connections. Use 'connect SRC -> TGT' to add dependencies.";

  const lines: string[] = ["Dependency Graph:"];
  for (const conn of config.connections.values()) {
    const src = config.blocks.get(conn.sourceId);
    const tgt = config.blocks.get(conn.targetId);
    if (src && tgt) {
      const label = conn.label ? ` (${conn.label})` : "";
      lines.push(`  ${src.label} -> ${tgt.label}${label}`);
    }
  }
  return lines.join("\n");
}

function queryValidate(config: TerraformConfig): string {
  const issues: string[] = [];

  for (const block of config.blocks.values()) {
    if (block.kind === "resource" && block.attributes.size === 0 && block.nestedBlocks.length === 0) {
      issues.push(`  WARNING: resource ${block.fullType}.${block.label} has no attributes`);
    }
    if (block.kind === "output" && !block.attributes.has("value")) {
      issues.push(`  ERROR: output "${block.label}" missing required "value" attribute`);
    }
    if (block.kind === "module" && !block.attributes.has("source")) {
      issues.push(`  ERROR: module "${block.label}" missing required "source" attribute`);
    }
  }

  // Check for dangling connections
  for (const conn of config.connections.values()) {
    if (!config.blocks.has(conn.sourceId)) {
      issues.push(`  ERROR: connection references missing source block`);
    }
    if (!config.blocks.has(conn.targetId)) {
      issues.push(`  ERROR: connection references missing target block`);
    }
  }

  if (issues.length === 0) return "Configuration is valid.";
  return `Found ${issues.length} issue(s):\n${issues.join("\n")}`;
}

function queryStats(config: TerraformConfig): string {
  const counts = new Map<string, number>();
  for (const block of config.blocks.values()) {
    counts.set(block.kind, (counts.get(block.kind) ?? 0) + 1);
  }

  const lines: string[] = [`Total blocks: ${config.blocks.size}`];
  for (const [kind, count] of counts) {
    lines.push(`  ${kind}: ${count}`);
  }
  lines.push(`Connections: ${config.connections.size}`);
  return lines.join("\n");
}

function queryStatus(config: TerraformConfig): string {
  const lines: string[] = [];
  lines.push(`Title: ${config.title}`);
  lines.push(`File: ${config.filePath ?? "(unsaved)"}`);
  lines.push(`Blocks: ${config.blocks.size}`);
  lines.push(`Connections: ${config.connections.size}`);
  return lines.join("\n");
}

function queryFind(config: TerraformConfig, text: string): string {
  if (!text) return "find requires search text";
  const lower = text.toLowerCase();
  const matches = [...config.blocks.values()].filter(
    (b) => b.label.toLowerCase().includes(lower) || b.fullType.toLowerCase().includes(lower),
  );
  if (matches.length === 0) return `No blocks matching "${text}"`;
  return matches.map((b) => `  ${b.kind} ${b.fullType}.${b.label}`).join("\n");
}

function queryHistory(eventLog: EventLog<TerraformEvent> | undefined, count: number): string {
  if (!eventLog) return "No event log available.";
  const events = eventLog.recent(count);
  if (events.length === 0) return "No events.";

  return events.map((e, i) => {
    switch (e.type) {
      case "block_added": return `  ${i + 1}. + ${e.block.kind} ${e.block.label}`;
      case "block_removed": return `  ${i + 1}. - ${e.block.kind} ${e.block.label}`;
      case "attribute_set": return `  ${i + 1}. * set ${e.key} on block`;
      case "attribute_removed": return `  ${i + 1}. * unset ${e.key}`;
      case "connection_added": return `  ${i + 1}. ~ ${e.connection.sourceLabel} -> ${e.connection.targetLabel}`;
      case "connection_removed": return `  ${i + 1}. - disconnect`;
      case "tag_set": return `  ${i + 1}. * tag ${e.key}`;
      case "tag_removed": return `  ${i + 1}. - tag ${e.key}`;
      case "block_renamed": return `  ${i + 1}. * rename ${e.before} → ${e.after}`;
      default: return `  ${i + 1}. ${e.type}`;
    }
  }).join("\n");
}
