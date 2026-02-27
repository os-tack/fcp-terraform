import type { ParsedOp, OpResult } from "@aetherwing/fcp-core";
import type { EventLog } from "@aetherwing/fcp-core";
import type { TerraformConfig, TerraformEvent, TfBlock, Attribute } from "./types.js";
import {
  findByLabel, addBlock, removeBlock, addConnection, removeConnection,
  createBlock, generateId, makeAttribute, deriveProvider,
  findByType, findByKind, findByProvider,
} from "./model.js";

type Handler = (op: ParsedOp, config: TerraformConfig, log: EventLog<TerraformEvent>) => OpResult;

export function dispatchOp(
  op: ParsedOp,
  config: TerraformConfig,
  log: EventLog<TerraformEvent>,
): OpResult {
  const handler = HANDLERS[op.verb];
  if (!handler) {
    return { success: false, message: `unhandled verb "${op.verb}"` };
  }
  return handler(op, config, log);
}

// ── Verb handlers ──────────────────────────────────────────

function handleAdd(op: ParsedOp, config: TerraformConfig, log: EventLog<TerraformEvent>): OpResult {
  const subKind = op.positionals[0]?.toLowerCase();

  switch (subKind) {
    case "resource": {
      const fullType = op.positionals[1];
      const label = op.positionals[2];
      if (!fullType || !label) {
        return { success: false, message: "add resource requires TYPE and LABEL" };
      }
      const block = createBlock("resource", fullType, label, op.params);
      const err = addBlock(config, block);
      if (err) return { success: false, message: err };
      log.append({ type: "block_added", block: structuredClone(block) });
      return { success: true, message: `resource ${fullType}.${label}`, prefix: "+" };
    }

    case "provider": {
      const name = op.positionals[1];
      if (!name) return { success: false, message: "add provider requires PROVIDER name" };
      const block = createBlock("provider", name, name, op.params);
      block.provider = name;
      const err = addBlock(config, block);
      if (err) return { success: false, message: err };
      log.append({ type: "block_added", block: structuredClone(block) });
      return { success: true, message: `provider "${name}"`, prefix: "+" };
    }

    case "variable": {
      const label = op.positionals[1];
      if (!label) return { success: false, message: "add variable requires NAME" };
      const block = createBlock("variable", "variable", label, op.params);
      block.provider = "";
      const err = addBlock(config, block);
      if (err) return { success: false, message: err };
      log.append({ type: "block_added", block: structuredClone(block) });
      return { success: true, message: `variable "${label}"`, prefix: "+" };
    }

    case "output": {
      const label = op.positionals[1];
      if (!label) return { success: false, message: "add output requires NAME" };
      const attrs = { ...op.params };
      // Mark value attribute as reference if it looks like one
      const block = createBlock("output", "output", label, {});
      block.provider = "";
      for (const [k, v] of Object.entries(attrs)) {
        const attr = makeAttribute(k, v);
        if (k === "value" && !v.startsWith('"')) {
          attr.valueType = "reference";
        }
        block.attributes.set(k, attr);
      }
      const err = addBlock(config, block);
      if (err) return { success: false, message: err };
      log.append({ type: "block_added", block: structuredClone(block) });
      return { success: true, message: `output "${label}"`, prefix: "+" };
    }

    case "data": {
      const fullType = op.positionals[1];
      const label = op.positionals[2];
      if (!fullType || !label) {
        return { success: false, message: "add data requires TYPE and LABEL" };
      }
      const block = createBlock("data", fullType, label, op.params);
      block.kind = "data";
      const err = addBlock(config, block);
      if (err) return { success: false, message: err };
      log.append({ type: "block_added", block: structuredClone(block) });
      return { success: true, message: `data ${fullType}.${label}`, prefix: "+" };
    }

    case "module": {
      const label = op.positionals[1];
      if (!label) return { success: false, message: "add module requires LABEL" };
      const block = createBlock("module", "module", label, op.params);
      block.provider = "";
      block.kind = "module";
      const err = addBlock(config, block);
      if (err) return { success: false, message: err };
      log.append({ type: "block_added", block: structuredClone(block) });
      return { success: true, message: `module "${label}"`, prefix: "+" };
    }

    default:
      return { success: false, message: `unknown add type "${subKind}". Use: resource, provider, variable, output, data, module` };
  }
}

function handleSet(op: ParsedOp, config: TerraformConfig, log: EventLog<TerraformEvent>): OpResult {
  const label = op.positionals[0];
  if (!label) return { success: false, message: "set requires a block LABEL" };
  const block = findByLabel(config, label);
  if (!block) return { success: false, message: `block "${label}" not found` };

  const keys = Object.keys(op.params);
  if (keys.length === 0) return { success: false, message: "set requires at least one key:value" };

  for (const [k, v] of Object.entries(op.params)) {
    const before = block.attributes.get(k) ?? null;
    const after = makeAttribute(k, v);
    block.attributes.set(k, after);
    log.append({ type: "attribute_set", blockId: block.id, key: k, before: before ? structuredClone(before) : null, after: structuredClone(after) });
  }

  return { success: true, message: `${label}: set ${keys.join(", ")}`, prefix: "*" };
}

function handleRemove(op: ParsedOp, config: TerraformConfig, log: EventLog<TerraformEvent>): OpResult {
  // Selector-based removal
  if (op.selectors.length > 0) {
    const resolved = resolveSelectors(op.selectors, config);
    if (resolved.length === 0) return { success: false, message: "no blocks match selector" };
    for (const block of resolved) {
      removeBlock(config, block.id);
      log.append({ type: "block_removed", block: structuredClone(block) });
    }
    return { success: true, message: `removed ${resolved.length} block(s)`, prefix: "@" };
  }

  // Label-based removal
  const label = op.positionals[0];
  if (!label) return { success: false, message: "remove requires LABEL or @selector" };
  const block = findByLabel(config, label);
  if (!block) return { success: false, message: `block "${label}" not found` };
  removeBlock(config, block.id);
  log.append({ type: "block_removed", block: structuredClone(block) });
  return { success: true, message: `${block.kind} "${label}"`, prefix: "-" };
}

function handleConnect(op: ParsedOp, config: TerraformConfig, log: EventLog<TerraformEvent>): OpResult {
  // Expect: positionals = ["source", "->", "target"]
  const arrowIdx = op.positionals.indexOf("->");
  if (arrowIdx < 0) return { success: false, message: "connect requires SRC -> TGT" };

  const srcLabel = op.positionals.slice(0, arrowIdx).join(" ");
  const tgtLabel = op.positionals.slice(arrowIdx + 1).join(" ");
  if (!srcLabel || !tgtLabel) return { success: false, message: "connect requires SRC -> TGT" };

  const src = findByLabel(config, srcLabel);
  const tgt = findByLabel(config, tgtLabel);
  if (!src) return { success: false, message: `source "${srcLabel}" not found` };
  if (!tgt) return { success: false, message: `target "${tgtLabel}" not found` };

  const conn = {
    id: generateId(),
    sourceId: src.id,
    targetId: tgt.id,
    sourceLabel: src.label,
    targetLabel: tgt.label,
    label: op.params["label"],
  };
  addConnection(config, conn);
  log.append({ type: "connection_added", connection: structuredClone(conn) });
  return { success: true, message: `${srcLabel} -> ${tgtLabel}`, prefix: "~" };
}

function handleDisconnect(op: ParsedOp, config: TerraformConfig, log: EventLog<TerraformEvent>): OpResult {
  const arrowIdx = op.positionals.indexOf("->");
  if (arrowIdx < 0) return { success: false, message: "disconnect requires SRC -> TGT" };

  const srcLabel = op.positionals.slice(0, arrowIdx).join(" ");
  const tgtLabel = op.positionals.slice(arrowIdx + 1).join(" ");

  const src = findByLabel(config, srcLabel);
  const tgt = findByLabel(config, tgtLabel);
  if (!src || !tgt) return { success: false, message: "source or target not found" };

  for (const [id, conn] of config.connections) {
    if (conn.sourceId === src.id && conn.targetId === tgt.id) {
      removeConnection(config, id);
      log.append({ type: "connection_removed", connection: structuredClone(conn) });
      return { success: true, message: `${srcLabel} -> ${tgtLabel}`, prefix: "-" };
    }
  }
  return { success: false, message: `no connection from "${srcLabel}" to "${tgtLabel}"` };
}

function handleLabel(op: ParsedOp, config: TerraformConfig, log: EventLog<TerraformEvent>): OpResult {
  const oldLabel = op.positionals[0];
  const newLabel = op.positionals[1];
  if (!oldLabel || !newLabel) return { success: false, message: "label requires OLD_LABEL NEW_LABEL" };

  const block = findByLabel(config, oldLabel);
  if (!block) return { success: false, message: `block "${oldLabel}" not found` };
  if (findByLabel(config, newLabel)) return { success: false, message: `label "${newLabel}" already exists` };

  const before = block.label;
  block.label = newLabel;
  log.append({ type: "block_renamed", blockId: block.id, before, after: newLabel });
  return { success: true, message: `"${before}" → "${newLabel}"`, prefix: "*" };
}

function handleStyle(op: ParsedOp, config: TerraformConfig, log: EventLog<TerraformEvent>): OpResult {
  const label = op.positionals[0];
  if (!label) return { success: false, message: "style requires LABEL" };
  const block = findByLabel(config, label);
  if (!block) return { success: false, message: `block "${label}" not found` };

  const tagsStr = op.params["tags"];
  if (!tagsStr) return { success: false, message: "style requires tags:\"Key=Val,Key2=Val2\"" };

  const pairs = tagsStr.split(",").map((p) => p.trim());
  for (const pair of pairs) {
    const eqIdx = pair.indexOf("=");
    if (eqIdx < 0) continue;
    const key = pair.slice(0, eqIdx).trim();
    const val = pair.slice(eqIdx + 1).trim();
    const before = block.tags.get(key) ?? null;
    block.tags.set(key, val);
    log.append({ type: "tag_set", blockId: block.id, key, before, after: val });
  }
  return { success: true, message: `${label}: tags set`, prefix: "*" };
}

function handleNest(op: ParsedOp, config: TerraformConfig, log: EventLog<TerraformEvent>): OpResult {
  const label = op.positionals[0];
  const blockType = op.positionals[1];
  if (!label || !blockType) return { success: false, message: "nest requires LABEL BLOCK_TYPE" };

  const block = findByLabel(config, label);
  if (!block) return { success: false, message: `block "${label}" not found` };

  const attrs = new Map<string, Attribute>();
  for (const [k, v] of Object.entries(op.params)) {
    attrs.set(k, makeAttribute(k, v));
  }
  const nested = { type: blockType, attributes: attrs };
  block.nestedBlocks.push(nested);
  log.append({ type: "nested_block_added", blockId: block.id, nestedBlock: structuredClone(nested) });
  return { success: true, message: `${label}: ${blockType} block added`, prefix: "+" };
}

function handleUnset(op: ParsedOp, config: TerraformConfig, log: EventLog<TerraformEvent>): OpResult {
  const label = op.positionals[0];
  if (!label) return { success: false, message: "unset requires LABEL" };
  const block = findByLabel(config, label);
  if (!block) return { success: false, message: `block "${label}" not found` };

  const keys = op.positionals.slice(1);
  if (keys.length === 0) return { success: false, message: "unset requires at least one KEY" };

  for (const key of keys) {
    const before = block.attributes.get(key);
    if (before) {
      block.attributes.delete(key);
      log.append({ type: "attribute_removed", blockId: block.id, key, before: structuredClone(before) });
    }
  }
  return { success: true, message: `${label}: unset ${keys.join(", ")}`, prefix: "*" };
}

// ── Selector resolution ─────────────────────────────────

function resolveSelectors(selectors: string[], config: TerraformConfig): TfBlock[] {
  let results: TfBlock[] = [...config.blocks.values()];

  for (const sel of selectors) {
    if (sel === "@all") continue;
    if (sel.startsWith("@type:")) {
      const type = sel.slice(6);
      results = results.filter((b) => b.fullType === type);
    } else if (sel.startsWith("@kind:")) {
      const kind = sel.slice(6);
      results = results.filter((b) => b.kind === kind);
    } else if (sel.startsWith("@provider:")) {
      const provider = sel.slice(10);
      results = results.filter((b) => b.provider === provider);
    } else if (sel.startsWith("@tag:")) {
      const tagExpr = sel.slice(5);
      const eqIdx = tagExpr.indexOf("=");
      if (eqIdx >= 0) {
        const key = tagExpr.slice(0, eqIdx);
        const val = tagExpr.slice(eqIdx + 1);
        results = results.filter((b) => b.tags.get(key) === val);
      } else {
        results = results.filter((b) => b.tags.has(tagExpr));
      }
    }
  }
  return results;
}

// ── Handler registry ────────────────────────────────────

const HANDLERS: Record<string, Handler> = {
  add: handleAdd,
  set: handleSet,
  remove: handleRemove,
  connect: handleConnect,
  disconnect: handleDisconnect,
  label: handleLabel,
  style: handleStyle,
  nest: handleNest,
  unset: handleUnset,
};
