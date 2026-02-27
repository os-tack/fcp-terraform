/**
 * Kind of Terraform block.
 */
export type BlockKind =
  | "resource"
  | "data"
  | "provider"
  | "variable"
  | "output"
  | "locals"
  | "module";

/**
 * A single attribute on a block.
 */
export interface Attribute {
  key: string;
  value: string;
  valueType: "string" | "number" | "bool" | "list" | "map" | "reference" | "expression";
}

/**
 * A nested block within a resource (e.g., ingress {}, root_block_device {}).
 */
export interface NestedBlock {
  type: string;
  attributes: Map<string, Attribute>;
}

/**
 * A top-level Terraform block (resource, provider, variable, output, data, module).
 */
export interface TfBlock {
  id: string;
  kind: BlockKind;
  label: string;
  fullType: string;
  provider: string;
  attributes: Map<string, Attribute>;
  nestedBlocks: NestedBlock[];
  tags: Map<string, string>;
  meta: {
    count?: string;
    forEach?: string;
    dependsOn: string[];
  };
}

/**
 * A dependency connection between two blocks.
 */
export interface Connection {
  id: string;
  sourceId: string;
  targetId: string;
  sourceLabel: string;
  targetLabel: string;
  label?: string;
}

/**
 * The full in-memory Terraform configuration.
 */
export interface TerraformConfig {
  id: string;
  title: string;
  filePath: string | null;
  blocks: Map<string, TfBlock>;
  connections: Map<string, Connection>;
  blockOrder: string[];
}

/**
 * Events for undo/redo.
 */
export type TerraformEvent =
  | { type: "block_added"; block: TfBlock }
  | { type: "block_removed"; block: TfBlock }
  | { type: "attribute_set"; blockId: string; key: string; before: Attribute | null; after: Attribute }
  | { type: "attribute_removed"; blockId: string; key: string; before: Attribute }
  | { type: "nested_block_added"; blockId: string; nestedBlock: NestedBlock }
  | { type: "nested_block_removed"; blockId: string; nestedBlock: NestedBlock }
  | { type: "connection_added"; connection: Connection }
  | { type: "connection_removed"; connection: Connection }
  | { type: "tag_set"; blockId: string; key: string; before: string | null; after: string }
  | { type: "tag_removed"; blockId: string; key: string; before: string }
  | { type: "block_renamed"; blockId: string; before: string; after: string }
  | { type: "title_changed"; before: string; after: string };
