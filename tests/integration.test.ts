import { describe, it, expect, beforeEach } from "vitest";
import { EventLog, parseOp, isParseError } from "@aetherwing/fcp-core";
import type { ParsedOp } from "@aetherwing/fcp-core";
import { TerraformAdapter } from "../src/adapter.js";
import { rebuildLabelIndex } from "../src/model.js";
import type { TerraformConfig, TerraformEvent } from "../src/types.js";

describe("Integration: TerraformAdapter end-to-end", () => {
  let adapter: TerraformAdapter;
  let model: TerraformConfig;
  let log: EventLog<TerraformEvent>;

  /** Parse an op string and assert it's not a parse error */
  function parse(input: string): ParsedOp {
    const result = parseOp(input);
    if (isParseError(result)) {
      throw new Error(`Parse error: ${result.error} for "${input}"`);
    }
    return result;
  }

  beforeEach(() => {
    adapter = new TerraformAdapter();
    model = adapter.createEmpty("Test", {});
    log = new EventLog<TerraformEvent>();
  });

  // ── Full workflow ─────────────────────────────────────

  describe("full infrastructure workflow", () => {
    it("builds, queries, and validates a complete config", () => {
      // Step 1: add provider
      let result = adapter.dispatchOp(parse("add provider aws region:us-east-1"), model, log);
      expect(result.success).toBe(true);

      // Step 2: add resources
      result = adapter.dispatchOp(parse("add resource aws_instance web ami:ami-123 instance_type:t2.micro"), model, log);
      expect(result.success).toBe(true);

      result = adapter.dispatchOp(parse("add resource aws_s3_bucket assets acl:private"), model, log);
      expect(result.success).toBe(true);

      // Step 3: connect resources
      result = adapter.dispatchOp(parse("connect web -> assets"), model, log);
      expect(result.success).toBe(true);

      // Step 4: style (tags)
      result = adapter.dispatchOp(parse('style web tags:"Name=WebServer,Env=prod"'), model, log);
      expect(result.success).toBe(true);

      // Step 5: verify plan generates valid HCL
      const plan = adapter.dispatchQuery("plan", model);
      expect(plan).toContain('provider "aws"');
      expect(plan).toContain('region = "us-east-1"');
      expect(plan).toContain('resource "aws_instance" "web"');
      expect(plan).toContain('ami  = "ami-123"');
      expect(plan).toContain('resource "aws_s3_bucket" "assets"');
      expect(plan).toContain("depends_on = [aws_s3_bucket.assets]");
      expect(plan).toContain('Name = "WebServer"');
      expect(plan).toContain('Env = "prod"');

      // Step 6: verify map summary
      const map = adapter.dispatchQuery("map", model);
      expect(map).toContain("Test");
      expect(map).toContain("Providers: aws");
      expect(map).toContain("Resources (2)");
      expect(map).toContain("Connections: 1");

      // Step 7: validate — should be valid
      const validation = adapter.dispatchQuery("validate", model);
      expect(validation).toContain("valid");
    });
  });

  // ── Serialization round-trip ──────────────────────────

  describe("serialization round-trip", () => {
    it("serialize -> deserialize preserves config", () => {
      // Build a config
      adapter.dispatchOp(parse("add provider aws region:us-east-1"), model, log);
      adapter.dispatchOp(parse("add resource aws_instance web ami:ami-123 instance_type:t2.micro"), model, log);
      adapter.dispatchOp(parse("add resource aws_s3_bucket assets acl:private"), model, log);
      adapter.dispatchOp(parse("connect web -> assets"), model, log);
      adapter.dispatchOp(parse('style web tags:"Name=WebServer"'), model, log);

      const digestBefore = adapter.getDigest(model);

      // Serialize
      const serialized = adapter.serialize(model);
      expect(typeof serialized).toBe("string");

      // Deserialize
      const restored = adapter.deserialize(serialized);
      adapter.rebuildIndices(restored);

      const digestAfter = adapter.getDigest(restored);
      expect(digestAfter).toBe(digestBefore);

      // Verify data integrity
      expect(restored.title).toBe("Test");
      expect(restored.blocks.size).toBe(model.blocks.size);
      expect(restored.connections.size).toBe(model.connections.size);

      // Verify block content survives round-trip
      for (const [id, block] of model.blocks) {
        const restoredBlock = restored.blocks.get(id);
        expect(restoredBlock).toBeDefined();
        expect(restoredBlock!.label).toBe(block.label);
        expect(restoredBlock!.kind).toBe(block.kind);
        expect(restoredBlock!.fullType).toBe(block.fullType);
        expect(restoredBlock!.attributes.size).toBe(block.attributes.size);
      }
    });

    it("preserves tags through serialization", () => {
      adapter.dispatchOp(parse("add resource aws_instance web ami:ami-123"), model, log);
      adapter.dispatchOp(parse('style web tags:"Name=WebServer,Env=prod"'), model, log);

      const serialized = adapter.serialize(model);
      const restored = adapter.deserialize(serialized);
      adapter.rebuildIndices(restored);

      const block = [...restored.blocks.values()].find((b) => b.label === "web");
      expect(block).toBeDefined();
      expect(block!.tags.get("Name")).toBe("WebServer");
      expect(block!.tags.get("Env")).toBe("prod");
    });

    it("preserves nested blocks through serialization", () => {
      adapter.dispatchOp(parse("add resource aws_security_group sg"), model, log);
      adapter.dispatchOp(parse("nest sg ingress from_port:80 to_port:80 protocol:tcp"), model, log);

      const serialized = adapter.serialize(model);
      const restored = adapter.deserialize(serialized);
      adapter.rebuildIndices(restored);

      const block = [...restored.blocks.values()].find((b) => b.label === "sg");
      expect(block).toBeDefined();
      expect(block!.nestedBlocks).toHaveLength(1);
      expect(block!.nestedBlocks[0].type).toBe("ingress");
      expect(block!.nestedBlocks[0].attributes.get("from_port")?.value).toBe("80");
    });
  });

  // ── Event reversal ────────────────────────────────────

  describe("reverseEvent", () => {
    it("reverses block_added (removes the block)", () => {
      adapter.dispatchOp(parse("add resource aws_instance web ami:ami-123"), model, log);
      expect(model.blocks.size).toBe(1);

      const events = log.recent();
      const event = events[0];
      adapter.reverseEvent(event, model);
      expect(model.blocks.size).toBe(0);
    });

    it("reverses block_removed (restores the block)", () => {
      adapter.dispatchOp(parse("add resource aws_instance web ami:ami-123"), model, log);
      adapter.dispatchOp(parse("remove web"), model, log);
      expect(model.blocks.size).toBe(0);

      const events = log.recent();
      const removeEvent = events.find((e) => e.type === "block_removed")!;
      adapter.reverseEvent(removeEvent, model);
      expect(model.blocks.size).toBe(1);

      // Verify restored block data
      const block = [...model.blocks.values()][0];
      expect(block.label).toBe("web");
      expect(block.attributes.get("ami")?.value).toBe("ami-123");
    });

    it("reverses attribute_set (restores previous value)", () => {
      adapter.dispatchOp(parse("add resource aws_instance web ami:ami-old"), model, log);
      adapter.dispatchOp(parse("set web ami:ami-new"), model, log);

      const block = [...model.blocks.values()][0];
      expect(block.attributes.get("ami")?.value).toBe("ami-new");

      const events = log.recent();
      const setEvent = events.find((e) => e.type === "attribute_set")!;
      adapter.reverseEvent(setEvent, model);
      expect(block.attributes.get("ami")?.value).toBe("ami-old");
    });

    it("reverses attribute_set for new attr (removes it)", () => {
      adapter.dispatchOp(parse("add resource aws_instance web"), model, log);
      adapter.dispatchOp(parse("set web ami:ami-123"), model, log);

      const block = [...model.blocks.values()][0];
      expect(block.attributes.has("ami")).toBe(true);

      const events = log.recent();
      const setEvent = events.find((e) => e.type === "attribute_set")!;
      adapter.reverseEvent(setEvent, model);
      expect(block.attributes.has("ami")).toBe(false);
    });

    it("reverses attribute_removed (restores the attr)", () => {
      adapter.dispatchOp(parse("add resource aws_instance web ami:ami-123"), model, log);
      adapter.dispatchOp(parse("unset web ami"), model, log);

      const block = [...model.blocks.values()][0];
      expect(block.attributes.has("ami")).toBe(false);

      const events = log.recent();
      const removeEvent = events.find((e) => e.type === "attribute_removed")!;
      adapter.reverseEvent(removeEvent, model);
      expect(block.attributes.get("ami")?.value).toBe("ami-123");
    });

    it("reverses connection_added (removes connection)", () => {
      adapter.dispatchOp(parse("add resource aws_instance web"), model, log);
      adapter.dispatchOp(parse("add resource aws_s3_bucket assets"), model, log);
      adapter.dispatchOp(parse("connect web -> assets"), model, log);
      expect(model.connections.size).toBe(1);

      const events = log.recent();
      const connEvent = events.find((e) => e.type === "connection_added")!;
      adapter.reverseEvent(connEvent, model);
      expect(model.connections.size).toBe(0);
    });

    it("reverses connection_removed (restores connection)", () => {
      adapter.dispatchOp(parse("add resource aws_instance web"), model, log);
      adapter.dispatchOp(parse("add resource aws_s3_bucket assets"), model, log);
      adapter.dispatchOp(parse("connect web -> assets"), model, log);
      adapter.dispatchOp(parse("disconnect web -> assets"), model, log);
      expect(model.connections.size).toBe(0);

      const events = log.recent();
      const disconnEvent = events.find((e) => e.type === "connection_removed")!;
      adapter.reverseEvent(disconnEvent, model);
      expect(model.connections.size).toBe(1);
    });

    it("reverses tag_set (removes new tag)", () => {
      adapter.dispatchOp(parse("add resource aws_instance web"), model, log);
      adapter.dispatchOp(parse('style web tags:"Name=WebServer"'), model, log);

      const block = [...model.blocks.values()][0];
      expect(block.tags.get("Name")).toBe("WebServer");

      const events = log.recent();
      const tagEvent = events.find((e) => e.type === "tag_set")!;
      adapter.reverseEvent(tagEvent, model);
      expect(block.tags.has("Name")).toBe(false);
    });

    it("reverses tag_set (restores previous tag value)", () => {
      adapter.dispatchOp(parse("add resource aws_instance web"), model, log);
      adapter.dispatchOp(parse('style web tags:"Name=OldName"'), model, log);
      adapter.dispatchOp(parse('style web tags:"Name=NewName"'), model, log);

      const block = [...model.blocks.values()][0];
      expect(block.tags.get("Name")).toBe("NewName");

      const events = log.recent();
      // The last tag_set event should be the one changing OldName -> NewName
      const tagEvents = events.filter((e) => e.type === "tag_set");
      const lastTagEvent = tagEvents[tagEvents.length - 1];
      adapter.reverseEvent(lastTagEvent, model);
      expect(block.tags.get("Name")).toBe("OldName");
    });

    it("reverses nested_block_added (removes nested block)", () => {
      adapter.dispatchOp(parse("add resource aws_security_group sg"), model, log);
      adapter.dispatchOp(parse("nest sg ingress from_port:80"), model, log);

      const block = [...model.blocks.values()][0];
      expect(block.nestedBlocks).toHaveLength(1);

      const events = log.recent();
      const nestEvent = events.find((e) => e.type === "nested_block_added")!;
      adapter.reverseEvent(nestEvent, model);
      expect(block.nestedBlocks).toHaveLength(0);
    });

    it("reverses block_renamed (restores old label)", () => {
      adapter.dispatchOp(parse("add resource aws_instance web"), model, log);
      adapter.dispatchOp(parse("label web webserver"), model, log);

      const block = [...model.blocks.values()][0];
      expect(block.label).toBe("webserver");

      const events = log.recent();
      const renameEvent = events.find((e) => e.type === "block_renamed")!;
      adapter.reverseEvent(renameEvent, model);
      expect(block.label).toBe("web");
    });

    it("reverses title_changed", () => {
      model.title = "New Title";
      const event: TerraformEvent = { type: "title_changed", before: "Test", after: "New Title" };
      adapter.reverseEvent(event, model);
      expect(model.title).toBe("Test");
    });
  });

  // ── Digest ────────────────────────────────────────────

  describe("getDigest", () => {
    it("reflects block counts", () => {
      const digest0 = adapter.getDigest(model);
      expect(digest0).toContain("0r");
      expect(digest0).toContain("0v");
      expect(digest0).toContain("0o");
      expect(digest0).toContain("0c");

      adapter.dispatchOp(parse("add provider aws region:us-east-1"), model, log);
      adapter.dispatchOp(parse("add resource aws_instance web"), model, log);
      adapter.dispatchOp(parse("add variable env"), model, log);
      adapter.dispatchOp(parse("add output ip value:aws_instance.web.public_ip"), model, log);
      adapter.dispatchOp(parse("add resource aws_s3_bucket assets"), model, log);
      adapter.dispatchOp(parse("connect web -> assets"), model, log);

      const digest = adapter.getDigest(model);
      expect(digest).toContain("2r");
      expect(digest).toContain("1v");
      expect(digest).toContain("1o");
      expect(digest).toContain("1c");
      expect(digest).toContain("aws");
    });
  });

  // ── createEmpty with params ───────────────────────────

  describe("createEmpty with params", () => {
    it("auto-adds provider when specified", () => {
      const m = adapter.createEmpty("My Project", { provider: "aws", region: "us-west-2" });
      expect(m.blocks.size).toBe(1);
      const block = [...m.blocks.values()][0];
      expect(block.kind).toBe("provider");
      expect(block.provider).toBe("aws");
      expect(block.attributes.get("region")?.value).toBe("us-west-2");
    });

    it("creates empty when no provider param", () => {
      const m = adapter.createEmpty("My Project", {});
      expect(m.blocks.size).toBe(0);
    });
  });

  // ── Complex multi-step workflow ───────────────────────

  describe("multi-step workflow with edits", () => {
    it("add -> set -> style -> nest -> connect -> plan", () => {
      // Build
      adapter.dispatchOp(parse("add provider aws region:us-east-1"), model, log);
      adapter.dispatchOp(parse("add resource aws_instance web ami:ami-123 instance_type:t2.micro"), model, log);
      adapter.dispatchOp(parse("add resource aws_security_group sg"), model, log);
      adapter.dispatchOp(parse("add variable env type:string default:prod"), model, log);
      adapter.dispatchOp(parse("add output ip value:aws_instance.web.public_ip"), model, log);

      // Edit
      adapter.dispatchOp(parse("set web instance_type:t3.small"), model, log);
      adapter.dispatchOp(parse('style web tags:"Name=WebServer,Env=prod"'), model, log);
      adapter.dispatchOp(parse("nest sg ingress from_port:443 to_port:443 protocol:tcp"), model, log);
      adapter.dispatchOp(parse("connect web -> sg"), model, log);

      // Query plan
      const plan = adapter.dispatchQuery("plan", model);
      expect(plan).toContain('provider "aws"');
      expect(plan).toContain('instance_type = "t3.small"');
      expect(plan).toContain("ingress {");
      expect(plan).toContain("depends_on");

      // Validate
      const validation = adapter.dispatchQuery("validate", model);
      expect(validation).toContain("valid");

      // Stats
      const stats = adapter.dispatchQuery("stats", model);
      expect(stats).toContain("Total blocks: 5");

      // Find
      const find = adapter.dispatchQuery("find web", model);
      expect(find).toContain("web");
    });
  });
});
