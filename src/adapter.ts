import type { FcpDomainAdapter, OpResult, ParsedOp } from "@aetherwing/fcp-core";
import type { EventLog } from "@aetherwing/fcp-core";
import type { TerraformConfig, TerraformEvent, TfBlock, Attribute, NestedBlock } from "./types.js";
import {
  createEmptyConfig, rebuildLabelIndex, findByKind,
  addBlock, removeBlock, addConnection, removeConnection,
  createBlock, makeAttribute,
} from "./model.js";
import { dispatchOp } from "./ops.js";
import { dispatchQuery } from "./queries.js";

// Keep a reference to the event log for queries that need it
let currentEventLog: EventLog<TerraformEvent> | undefined;

export class TerraformAdapter implements FcpDomainAdapter<TerraformConfig, TerraformEvent> {
  createEmpty(title: string, params: Record<string, string>): TerraformConfig {
    const config = createEmptyConfig(title);

    // Auto-add provider if specified
    if (params["provider"]) {
      const providerName = params["provider"];
      const providerParams: Record<string, string> = {};
      if (params["region"]) providerParams["region"] = params["region"];
      const block = createBlock("provider", providerName, providerName, providerParams);
      block.provider = providerName;
      addBlock(config, block);
    }

    rebuildLabelIndex(config);
    return config;
  }

  serialize(model: TerraformConfig): string {
    return JSON.stringify(configToJson(model), null, 2);
  }

  deserialize(data: Buffer | string): TerraformConfig {
    const json = JSON.parse(typeof data === "string" ? data : data.toString());
    return jsonToConfig(json);
  }

  rebuildIndices(model: TerraformConfig): void {
    rebuildLabelIndex(model);
  }

  getDigest(model: TerraformConfig): string {
    const resources = findByKind(model, "resource").length;
    const variables = findByKind(model, "variable").length;
    const outputs = findByKind(model, "output").length;
    const providers = findByKind(model, "provider").map((p) => p.label);
    const provStr = providers.length > 0 ? ` | ${providers.join(",")}` : "";
    return `[${resources}r ${variables}v ${outputs}o ${model.connections.size}c${provStr}]`;
  }

  dispatchOp(op: ParsedOp, model: TerraformConfig, log: EventLog<TerraformEvent>): OpResult {
    currentEventLog = log;
    return dispatchOp(op, model, log);
  }

  dispatchQuery(query: string, model: TerraformConfig): string {
    return dispatchQuery(query, model, currentEventLog);
  }

  reverseEvent(event: TerraformEvent, model: TerraformConfig): void {
    switch (event.type) {
      case "block_added":
        removeBlock(model, event.block.id);
        break;
      case "block_removed": {
        const restored = restoreBlock(event.block);
        addBlock(model, restored);
        break;
      }
      case "attribute_set": {
        const block = model.blocks.get(event.blockId);
        if (!block) return;
        if (event.before === null) {
          block.attributes.delete(event.key);
        } else {
          block.attributes.set(event.key, structuredClone(event.before));
        }
        break;
      }
      case "attribute_removed": {
        const block = model.blocks.get(event.blockId);
        if (!block) return;
        block.attributes.set(event.key, structuredClone(event.before));
        break;
      }
      case "connection_added":
        removeConnection(model, event.connection.id);
        break;
      case "connection_removed":
        addConnection(model, structuredClone(event.connection));
        break;
      case "tag_set": {
        const block = model.blocks.get(event.blockId);
        if (!block) return;
        if (event.before === null) {
          block.tags.delete(event.key);
        } else {
          block.tags.set(event.key, event.before);
        }
        break;
      }
      case "tag_removed": {
        const block = model.blocks.get(event.blockId);
        if (!block) return;
        block.tags.set(event.key, event.before);
        break;
      }
      case "nested_block_added": {
        const block = model.blocks.get(event.blockId);
        if (!block) return;
        block.nestedBlocks = block.nestedBlocks.filter(
          (nb) => nb.type !== event.nestedBlock.type,
        );
        break;
      }
      case "nested_block_removed": {
        const block = model.blocks.get(event.blockId);
        if (!block) return;
        block.nestedBlocks.push(restoreNestedBlock(event.nestedBlock));
        break;
      }
      case "block_renamed": {
        const block = model.blocks.get(event.blockId);
        if (block) block.label = event.before;
        break;
      }
      case "title_changed":
        model.title = event.before;
        break;
    }
    rebuildLabelIndex(model);
  }

  replayEvent(event: TerraformEvent, model: TerraformConfig): void {
    switch (event.type) {
      case "block_added": {
        const restored = restoreBlock(event.block);
        addBlock(model, restored);
        break;
      }
      case "block_removed":
        removeBlock(model, event.block.id);
        break;
      case "attribute_set": {
        const block = model.blocks.get(event.blockId);
        if (!block) return;
        block.attributes.set(event.key, structuredClone(event.after));
        break;
      }
      case "attribute_removed": {
        const block = model.blocks.get(event.blockId);
        if (!block) return;
        block.attributes.delete(event.key);
        break;
      }
      case "connection_added":
        addConnection(model, structuredClone(event.connection));
        break;
      case "connection_removed":
        removeConnection(model, event.connection.id);
        break;
      case "tag_set": {
        const block = model.blocks.get(event.blockId);
        if (!block) return;
        block.tags.set(event.key, event.after);
        break;
      }
      case "tag_removed": {
        const block = model.blocks.get(event.blockId);
        if (!block) return;
        block.tags.delete(event.key);
        break;
      }
      case "nested_block_added": {
        const block = model.blocks.get(event.blockId);
        if (!block) return;
        block.nestedBlocks.push(restoreNestedBlock(event.nestedBlock));
        break;
      }
      case "nested_block_removed": {
        const block = model.blocks.get(event.blockId);
        if (!block) return;
        block.nestedBlocks = block.nestedBlocks.filter(
          (nb) => nb.type !== event.nestedBlock.type,
        );
        break;
      }
      case "block_renamed": {
        const block = model.blocks.get(event.blockId);
        if (block) block.label = event.after;
        break;
      }
      case "title_changed":
        model.title = event.after;
        break;
    }
    rebuildLabelIndex(model);
  }
}

// ── Serialization helpers ─────────────────────────────

function configToJson(config: TerraformConfig): Record<string, unknown> {
  return {
    id: config.id,
    title: config.title,
    filePath: config.filePath,
    blockOrder: config.blockOrder,
    blocks: Object.fromEntries(
      [...config.blocks.entries()].map(([id, block]) => [id, blockToJson(block)]),
    ),
    connections: Object.fromEntries(
      [...config.connections.entries()].map(([id, conn]) => [id, conn]),
    ),
  };
}

function blockToJson(block: TfBlock): Record<string, unknown> {
  return {
    ...block,
    attributes: Object.fromEntries(block.attributes),
    tags: Object.fromEntries(block.tags),
    nestedBlocks: block.nestedBlocks.map((nb) => ({
      type: nb.type,
      attributes: Object.fromEntries(nb.attributes),
    })),
  };
}

function jsonToConfig(json: Record<string, unknown>): TerraformConfig {
  const blocks = new Map<string, TfBlock>();
  const jsonBlocks = json["blocks"] as Record<string, Record<string, unknown>>;
  for (const [id, jb] of Object.entries(jsonBlocks)) {
    blocks.set(id, restoreBlock(jb as unknown as TfBlock));
  }

  const connections = new Map<string, any>();
  const jsonConns = json["connections"] as Record<string, unknown>;
  for (const [id, conn] of Object.entries(jsonConns)) {
    connections.set(id, conn);
  }

  return {
    id: json["id"] as string,
    title: json["title"] as string,
    filePath: json["filePath"] as string | null,
    blocks,
    connections,
    blockOrder: json["blockOrder"] as string[],
  };
}

function restoreBlock(data: TfBlock): TfBlock {
  const attrs = data.attributes instanceof Map
    ? data.attributes
    : new Map(Object.entries(data.attributes as unknown as Record<string, Attribute>));
  const tags = data.tags instanceof Map
    ? data.tags
    : new Map(Object.entries(data.tags as unknown as Record<string, string>));
  const nestedBlocks = (data.nestedBlocks ?? []).map(restoreNestedBlock);

  return {
    id: data.id,
    kind: data.kind,
    label: data.label,
    fullType: data.fullType,
    provider: data.provider,
    attributes: attrs,
    nestedBlocks,
    tags,
    meta: data.meta ?? { dependsOn: [] },
  };
}

function restoreNestedBlock(data: NestedBlock): NestedBlock {
  const attrs = data.attributes instanceof Map
    ? data.attributes
    : new Map(Object.entries(data.attributes as unknown as Record<string, Attribute>));
  return { type: data.type, attributes: attrs };
}
