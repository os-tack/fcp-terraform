import type { TerraformConfig, TfBlock, Connection, Attribute, NestedBlock } from "./types.js";

let idCounter = 0;

export function generateId(): string {
  return `tf_${(++idCounter).toString(36).padStart(4, "0")}_${Math.random().toString(36).slice(2, 6)}`;
}

/**
 * Derive provider name from a Terraform resource type.
 * "aws_s3_bucket" → "aws", "google_compute_instance" → "google", "azurerm_resource_group" → "azurerm"
 */
export function deriveProvider(fullType: string): string {
  const idx = fullType.indexOf("_");
  return idx > 0 ? fullType.slice(0, idx) : fullType;
}

/**
 * Create a new empty TerraformConfig.
 */
export function createEmptyConfig(title: string): TerraformConfig {
  return {
    id: generateId(),
    title,
    filePath: null,
    blocks: new Map(),
    connections: new Map(),
    blockOrder: [],
  };
}

// ── Label index for O(1) lookup ─────────────────────────

const labelIndex = new Map<string, string>(); // label → block ID

export function rebuildLabelIndex(config: TerraformConfig): void {
  labelIndex.clear();
  for (const [id, block] of config.blocks) {
    labelIndex.set(block.label.toLowerCase(), id);
  }
}

export function findByLabel(config: TerraformConfig, label: string): TfBlock | undefined {
  const id = labelIndex.get(label.toLowerCase());
  return id ? config.blocks.get(id) : undefined;
}

export function findByType(config: TerraformConfig, fullType: string): TfBlock[] {
  const results: TfBlock[] = [];
  for (const block of config.blocks.values()) {
    if (block.fullType === fullType) results.push(block);
  }
  return results;
}

export function findByKind(config: TerraformConfig, kind: string): TfBlock[] {
  const results: TfBlock[] = [];
  for (const block of config.blocks.values()) {
    if (block.kind === kind) results.push(block);
  }
  return results;
}

export function findByProvider(config: TerraformConfig, provider: string): TfBlock[] {
  const results: TfBlock[] = [];
  for (const block of config.blocks.values()) {
    if (block.provider === provider) results.push(block);
  }
  return results;
}

export function findConnections(config: TerraformConfig, blockId: string): Connection[] {
  const results: Connection[] = [];
  for (const conn of config.connections.values()) {
    if (conn.sourceId === blockId || conn.targetId === blockId) results.push(conn);
  }
  return results;
}

export function addBlock(config: TerraformConfig, block: TfBlock): string | null {
  // Check label uniqueness
  if (findByLabel(config, block.label)) {
    return `label "${block.label}" already exists`;
  }
  config.blocks.set(block.id, block);
  config.blockOrder.push(block.id);
  labelIndex.set(block.label.toLowerCase(), block.id);
  return null;
}

export function removeBlock(config: TerraformConfig, id: string): TfBlock | null {
  const block = config.blocks.get(id);
  if (!block) return null;
  config.blocks.delete(id);
  config.blockOrder = config.blockOrder.filter((bid) => bid !== id);
  labelIndex.delete(block.label.toLowerCase());
  // Remove connections involving this block
  for (const [connId, conn] of config.connections) {
    if (conn.sourceId === id || conn.targetId === id) {
      config.connections.delete(connId);
    }
  }
  return block;
}

export function addConnection(config: TerraformConfig, conn: Connection): void {
  config.connections.set(conn.id, conn);
}

export function removeConnection(config: TerraformConfig, id: string): Connection | null {
  const conn = config.connections.get(id);
  if (!conn) return null;
  config.connections.delete(id);
  return conn;
}

/**
 * Create an Attribute from a string value, inferring the type.
 */
export function makeAttribute(key: string, value: string): Attribute {
  if (value === "true" || value === "false") {
    return { key, value, valueType: "bool" };
  }
  if (/^\d+(\.\d+)?$/.test(value)) {
    return { key, value, valueType: "number" };
  }
  if (value.startsWith("[") || value.startsWith("{")) {
    return { key, value, valueType: "expression" };
  }
  // Check for Terraform references: aws_xxx.name.attr or var.xxx
  if (/^(aws_|google_|azurerm_|var\.|local\.|data\.|module\.)/.test(value)) {
    return { key, value, valueType: "reference" };
  }
  return { key, value, valueType: "string" };
}

/**
 * Create a TfBlock from components.
 */
export function createBlock(
  kind: TfBlock["kind"],
  fullType: string,
  label: string,
  attrs: Record<string, string>,
): TfBlock {
  const attributes = new Map<string, Attribute>();
  for (const [k, v] of Object.entries(attrs)) {
    attributes.set(k, makeAttribute(k, v));
  }
  return {
    id: generateId(),
    kind,
    label,
    fullType,
    provider: deriveProvider(fullType),
    attributes,
    nestedBlocks: [],
    tags: new Map(),
    meta: { dependsOn: [] },
  };
}
