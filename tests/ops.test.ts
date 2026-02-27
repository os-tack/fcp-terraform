import { describe, it, expect, beforeEach } from "vitest";
import { EventLog } from "@aetherwing/fcp-core";
import { dispatchOp } from "../src/ops.js";
import { createEmptyConfig, rebuildLabelIndex, createBlock, addBlock } from "../src/model.js";
import type { TerraformConfig, TerraformEvent } from "../src/types.js";

describe("dispatchOp", () => {
  let config: TerraformConfig;
  let log: EventLog<TerraformEvent>;

  beforeEach(() => {
    config = createEmptyConfig("Test");
    rebuildLabelIndex(config);
    log = new EventLog<TerraformEvent>();
  });

  // ── ADD ─────────────────────────────────────────────────

  describe("add resource", () => {
    it("adds a resource with attributes", () => {
      const result = dispatchOp(
        { verb: "add", positionals: ["resource", "aws_instance", "web"], params: { ami: "ami-123", instance_type: "t2.micro" }, selectors: [], raw: "" },
        config, log,
      );
      expect(result.success).toBe(true);
      expect(result.prefix).toBe("+");
      expect(result.message).toContain("aws_instance");
      expect(config.blocks.size).toBe(1);
      const block = [...config.blocks.values()][0];
      expect(block.kind).toBe("resource");
      expect(block.fullType).toBe("aws_instance");
      expect(block.label).toBe("web");
      expect(block.attributes.get("ami")?.value).toBe("ami-123");
    });

    it("emits block_added event", () => {
      dispatchOp(
        { verb: "add", positionals: ["resource", "aws_instance", "web"], params: {}, selectors: [], raw: "" },
        config, log,
      );
      const events = log.recent();
      expect(events).toHaveLength(1);
      expect(events[0].type).toBe("block_added");
    });

    it("returns error when missing TYPE or LABEL", () => {
      const r1 = dispatchOp(
        { verb: "add", positionals: ["resource"], params: {}, selectors: [], raw: "" },
        config, log,
      );
      expect(r1.success).toBe(false);
      expect(r1.message).toContain("requires");

      const r2 = dispatchOp(
        { verb: "add", positionals: ["resource", "aws_instance"], params: {}, selectors: [], raw: "" },
        config, log,
      );
      expect(r2.success).toBe(false);
    });

    it("returns error for duplicate label", () => {
      dispatchOp(
        { verb: "add", positionals: ["resource", "aws_instance", "web"], params: {}, selectors: [], raw: "" },
        config, log,
      );
      const result = dispatchOp(
        { verb: "add", positionals: ["resource", "aws_s3_bucket", "web"], params: {}, selectors: [], raw: "" },
        config, log,
      );
      expect(result.success).toBe(false);
      expect(result.message).toContain("already exists");
    });
  });

  describe("add provider", () => {
    it("adds a provider block", () => {
      const result = dispatchOp(
        { verb: "add", positionals: ["provider", "aws"], params: { region: "us-east-1" }, selectors: [], raw: "" },
        config, log,
      );
      expect(result.success).toBe(true);
      expect(result.prefix).toBe("+");
      expect(config.blocks.size).toBe(1);
      const block = [...config.blocks.values()][0];
      expect(block.kind).toBe("provider");
      expect(block.provider).toBe("aws");
      expect(block.attributes.get("region")?.value).toBe("us-east-1");
    });

    it("emits block_added event", () => {
      dispatchOp(
        { verb: "add", positionals: ["provider", "aws"], params: {}, selectors: [], raw: "" },
        config, log,
      );
      const events = log.recent();
      expect(events).toHaveLength(1);
      expect(events[0].type).toBe("block_added");
    });

    it("returns error when missing PROVIDER name", () => {
      const result = dispatchOp(
        { verb: "add", positionals: ["provider"], params: {}, selectors: [], raw: "" },
        config, log,
      );
      expect(result.success).toBe(false);
      expect(result.message).toContain("requires");
    });
  });

  describe("add variable", () => {
    it("adds a variable block", () => {
      const result = dispatchOp(
        { verb: "add", positionals: ["variable", "env"], params: { type: "string", default: "prod" }, selectors: [], raw: "" },
        config, log,
      );
      expect(result.success).toBe(true);
      expect(result.prefix).toBe("+");
      const block = [...config.blocks.values()][0];
      expect(block.kind).toBe("variable");
      expect(block.label).toBe("env");
    });

    it("emits block_added event", () => {
      dispatchOp(
        { verb: "add", positionals: ["variable", "env"], params: {}, selectors: [], raw: "" },
        config, log,
      );
      expect(log.recent()).toHaveLength(1);
      expect(log.recent()[0].type).toBe("block_added");
    });

    it("returns error when missing NAME", () => {
      const result = dispatchOp(
        { verb: "add", positionals: ["variable"], params: {}, selectors: [], raw: "" },
        config, log,
      );
      expect(result.success).toBe(false);
    });
  });

  describe("add output", () => {
    it("adds an output block", () => {
      const result = dispatchOp(
        { verb: "add", positionals: ["output", "web_ip"], params: { value: "aws_instance.web.public_ip" }, selectors: [], raw: "" },
        config, log,
      );
      expect(result.success).toBe(true);
      expect(result.prefix).toBe("+");
      const block = [...config.blocks.values()][0];
      expect(block.kind).toBe("output");
      expect(block.label).toBe("web_ip");
      expect(block.attributes.get("value")?.valueType).toBe("reference");
    });

    it("emits block_added event", () => {
      dispatchOp(
        { verb: "add", positionals: ["output", "web_ip"], params: { value: "aws_instance.web.public_ip" }, selectors: [], raw: "" },
        config, log,
      );
      expect(log.recent()[0].type).toBe("block_added");
    });

    it("returns error when missing NAME", () => {
      const result = dispatchOp(
        { verb: "add", positionals: ["output"], params: {}, selectors: [], raw: "" },
        config, log,
      );
      expect(result.success).toBe(false);
    });
  });

  describe("add data", () => {
    it("adds a data source block", () => {
      const result = dispatchOp(
        { verb: "add", positionals: ["data", "aws_ami", "latest"], params: { most_recent: "true" }, selectors: [], raw: "" },
        config, log,
      );
      expect(result.success).toBe(true);
      expect(result.prefix).toBe("+");
      const block = [...config.blocks.values()][0];
      expect(block.kind).toBe("data");
      expect(block.fullType).toBe("aws_ami");
      expect(block.label).toBe("latest");
    });

    it("emits block_added event", () => {
      dispatchOp(
        { verb: "add", positionals: ["data", "aws_ami", "latest"], params: {}, selectors: [], raw: "" },
        config, log,
      );
      expect(log.recent()[0].type).toBe("block_added");
    });

    it("returns error when missing TYPE or LABEL", () => {
      const r1 = dispatchOp(
        { verb: "add", positionals: ["data"], params: {}, selectors: [], raw: "" },
        config, log,
      );
      expect(r1.success).toBe(false);

      const r2 = dispatchOp(
        { verb: "add", positionals: ["data", "aws_ami"], params: {}, selectors: [], raw: "" },
        config, log,
      );
      expect(r2.success).toBe(false);
    });
  });

  describe("add module", () => {
    it("adds a module block", () => {
      const result = dispatchOp(
        { verb: "add", positionals: ["module", "vpc"], params: { source: "./modules/vpc" }, selectors: [], raw: "" },
        config, log,
      );
      expect(result.success).toBe(true);
      expect(result.prefix).toBe("+");
      const block = [...config.blocks.values()][0];
      expect(block.kind).toBe("module");
      expect(block.label).toBe("vpc");
      expect(block.attributes.get("source")?.value).toBe("./modules/vpc");
    });

    it("emits block_added event", () => {
      dispatchOp(
        { verb: "add", positionals: ["module", "vpc"], params: { source: "./modules/vpc" }, selectors: [], raw: "" },
        config, log,
      );
      expect(log.recent()[0].type).toBe("block_added");
    });

    it("returns error when missing LABEL", () => {
      const result = dispatchOp(
        { verb: "add", positionals: ["module"], params: {}, selectors: [], raw: "" },
        config, log,
      );
      expect(result.success).toBe(false);
    });
  });

  describe("add with missing/unknown sub-kind", () => {
    it("returns error for unknown add type", () => {
      const result = dispatchOp(
        { verb: "add", positionals: ["unknown_thing", "foo"], params: {}, selectors: [], raw: "" },
        config, log,
      );
      expect(result.success).toBe(false);
      expect(result.message).toContain("unknown add type");
    });

    it("returns error when no sub-kind provided", () => {
      const result = dispatchOp(
        { verb: "add", positionals: [], params: {}, selectors: [], raw: "" },
        config, log,
      );
      expect(result.success).toBe(false);
    });
  });

  // ── SET ─────────────────────────────────────────────────

  describe("set", () => {
    beforeEach(() => {
      dispatchOp(
        { verb: "add", positionals: ["resource", "aws_instance", "web"], params: { ami: "ami-old" }, selectors: [], raw: "" },
        config, log,
      );
    });

    it("sets attributes on an existing block", () => {
      const result = dispatchOp(
        { verb: "set", positionals: ["web"], params: { ami: "ami-new", instance_type: "t3.small" }, selectors: [], raw: "" },
        config, log,
      );
      expect(result.success).toBe(true);
      expect(result.prefix).toBe("*");
      const block = [...config.blocks.values()][0];
      expect(block.attributes.get("ami")?.value).toBe("ami-new");
      expect(block.attributes.get("instance_type")?.value).toBe("t3.small");
    });

    it("emits attribute_set events with before/after", () => {
      dispatchOp(
        { verb: "set", positionals: ["web"], params: { ami: "ami-new" }, selectors: [], raw: "" },
        config, log,
      );
      const events = log.recent();
      const setEvent = events.find((e) => e.type === "attribute_set");
      expect(setEvent).toBeDefined();
      if (setEvent?.type === "attribute_set") {
        expect(setEvent.key).toBe("ami");
        expect(setEvent.before?.value).toBe("ami-old");
        expect(setEvent.after.value).toBe("ami-new");
      }
    });

    it("records null as before for new attributes", () => {
      dispatchOp(
        { verb: "set", positionals: ["web"], params: { new_key: "new_val" }, selectors: [], raw: "" },
        config, log,
      );
      const events = log.recent();
      const setEvent = events.find((e) => e.type === "attribute_set" && e.key === "new_key");
      if (setEvent?.type === "attribute_set") {
        expect(setEvent.before).toBeNull();
      }
    });

    it("returns error for missing label", () => {
      const result = dispatchOp(
        { verb: "set", positionals: [], params: { x: "y" }, selectors: [], raw: "" },
        config, log,
      );
      expect(result.success).toBe(false);
    });

    it("returns error for non-existent block", () => {
      const result = dispatchOp(
        { verb: "set", positionals: ["nonexistent"], params: { x: "y" }, selectors: [], raw: "" },
        config, log,
      );
      expect(result.success).toBe(false);
      expect(result.message).toContain("not found");
    });

    it("returns error when no key:value params", () => {
      const result = dispatchOp(
        { verb: "set", positionals: ["web"], params: {}, selectors: [], raw: "" },
        config, log,
      );
      expect(result.success).toBe(false);
      expect(result.message).toContain("key:value");
    });
  });

  // ── REMOVE ──────────────────────────────────────────────

  describe("remove", () => {
    beforeEach(() => {
      dispatchOp(
        { verb: "add", positionals: ["resource", "aws_instance", "web"], params: {}, selectors: [], raw: "" },
        config, log,
      );
      dispatchOp(
        { verb: "add", positionals: ["resource", "aws_s3_bucket", "assets"], params: {}, selectors: [], raw: "" },
        config, log,
      );
    });

    it("removes a block by label", () => {
      const result = dispatchOp(
        { verb: "remove", positionals: ["web"], params: {}, selectors: [], raw: "" },
        config, log,
      );
      expect(result.success).toBe(true);
      expect(result.prefix).toBe("-");
      expect(config.blocks.size).toBe(1);
    });

    it("emits block_removed event", () => {
      dispatchOp(
        { verb: "remove", positionals: ["web"], params: {}, selectors: [], raw: "" },
        config, log,
      );
      const events = log.recent();
      const removeEvent = events.find((e) => e.type === "block_removed");
      expect(removeEvent).toBeDefined();
    });

    it("returns error for non-existent block", () => {
      const result = dispatchOp(
        { verb: "remove", positionals: ["nonexistent"], params: {}, selectors: [], raw: "" },
        config, log,
      );
      expect(result.success).toBe(false);
      expect(result.message).toContain("not found");
    });

    it("returns error when no label or selector", () => {
      const result = dispatchOp(
        { verb: "remove", positionals: [], params: {}, selectors: [], raw: "" },
        config, log,
      );
      expect(result.success).toBe(false);
    });
  });

  describe("remove @selector", () => {
    beforeEach(() => {
      dispatchOp(
        { verb: "add", positionals: ["resource", "aws_instance", "web1"], params: {}, selectors: [], raw: "" },
        config, log,
      );
      dispatchOp(
        { verb: "add", positionals: ["resource", "aws_instance", "web2"], params: {}, selectors: [], raw: "" },
        config, log,
      );
      dispatchOp(
        { verb: "add", positionals: ["resource", "aws_s3_bucket", "assets"], params: {}, selectors: [], raw: "" },
        config, log,
      );
    });

    it("removes blocks matching @type selector", () => {
      const result = dispatchOp(
        { verb: "remove", positionals: [], params: {}, selectors: ["@type:aws_instance"], raw: "" },
        config, log,
      );
      expect(result.success).toBe(true);
      expect(result.prefix).toBe("@");
      expect(config.blocks.size).toBe(1);
      expect([...config.blocks.values()][0].label).toBe("assets");
    });

    it("returns error when no blocks match selector", () => {
      const result = dispatchOp(
        { verb: "remove", positionals: [], params: {}, selectors: ["@type:nonexistent_type"], raw: "" },
        config, log,
      );
      expect(result.success).toBe(false);
      expect(result.message).toContain("no blocks match");
    });
  });

  // ── CONNECT ─────────────────────────────────────────────

  describe("connect", () => {
    beforeEach(() => {
      dispatchOp(
        { verb: "add", positionals: ["resource", "aws_instance", "web"], params: {}, selectors: [], raw: "" },
        config, log,
      );
      dispatchOp(
        { verb: "add", positionals: ["resource", "aws_s3_bucket", "assets"], params: {}, selectors: [], raw: "" },
        config, log,
      );
    });

    it("creates a connection between two blocks", () => {
      const result = dispatchOp(
        { verb: "connect", positionals: ["web", "->", "assets"], params: {}, selectors: [], raw: "" },
        config, log,
      );
      expect(result.success).toBe(true);
      expect(result.prefix).toBe("~");
      expect(config.connections.size).toBe(1);
      const conn = [...config.connections.values()][0];
      expect(conn.sourceLabel).toBe("web");
      expect(conn.targetLabel).toBe("assets");
    });

    it("emits connection_added event", () => {
      dispatchOp(
        { verb: "connect", positionals: ["web", "->", "assets"], params: {}, selectors: [], raw: "" },
        config, log,
      );
      const events = log.recent();
      const connEvent = events.find((e) => e.type === "connection_added");
      expect(connEvent).toBeDefined();
    });

    it("returns error when arrow is missing", () => {
      const result = dispatchOp(
        { verb: "connect", positionals: ["web", "assets"], params: {}, selectors: [], raw: "" },
        config, log,
      );
      expect(result.success).toBe(false);
      expect(result.message).toContain("->");
    });

    it("returns error for non-existent source", () => {
      const result = dispatchOp(
        { verb: "connect", positionals: ["nonexistent", "->", "assets"], params: {}, selectors: [], raw: "" },
        config, log,
      );
      expect(result.success).toBe(false);
      expect(result.message).toContain("not found");
    });

    it("returns error for non-existent target", () => {
      const result = dispatchOp(
        { verb: "connect", positionals: ["web", "->", "nonexistent"], params: {}, selectors: [], raw: "" },
        config, log,
      );
      expect(result.success).toBe(false);
      expect(result.message).toContain("not found");
    });
  });

  // ── DISCONNECT ──────────────────────────────────────────

  describe("disconnect", () => {
    beforeEach(() => {
      dispatchOp(
        { verb: "add", positionals: ["resource", "aws_instance", "web"], params: {}, selectors: [], raw: "" },
        config, log,
      );
      dispatchOp(
        { verb: "add", positionals: ["resource", "aws_s3_bucket", "assets"], params: {}, selectors: [], raw: "" },
        config, log,
      );
      dispatchOp(
        { verb: "connect", positionals: ["web", "->", "assets"], params: {}, selectors: [], raw: "" },
        config, log,
      );
    });

    it("removes a connection", () => {
      expect(config.connections.size).toBe(1);
      const result = dispatchOp(
        { verb: "disconnect", positionals: ["web", "->", "assets"], params: {}, selectors: [], raw: "" },
        config, log,
      );
      expect(result.success).toBe(true);
      expect(result.prefix).toBe("-");
      expect(config.connections.size).toBe(0);
    });

    it("emits connection_removed event", () => {
      dispatchOp(
        { verb: "disconnect", positionals: ["web", "->", "assets"], params: {}, selectors: [], raw: "" },
        config, log,
      );
      const events = log.recent();
      const disconnectEvent = events.find((e) => e.type === "connection_removed");
      expect(disconnectEvent).toBeDefined();
    });

    it("returns error when no connection exists", () => {
      dispatchOp(
        { verb: "disconnect", positionals: ["web", "->", "assets"], params: {}, selectors: [], raw: "" },
        config, log,
      );
      // Try disconnecting again
      const result = dispatchOp(
        { verb: "disconnect", positionals: ["web", "->", "assets"], params: {}, selectors: [], raw: "" },
        config, log,
      );
      expect(result.success).toBe(false);
      expect(result.message).toContain("no connection");
    });

    it("returns error when arrow is missing", () => {
      const result = dispatchOp(
        { verb: "disconnect", positionals: ["web", "assets"], params: {}, selectors: [], raw: "" },
        config, log,
      );
      expect(result.success).toBe(false);
    });
  });

  // ── LABEL ───────────────────────────────────────────────

  describe("label", () => {
    beforeEach(() => {
      dispatchOp(
        { verb: "add", positionals: ["resource", "aws_instance", "web"], params: {}, selectors: [], raw: "" },
        config, log,
      );
    });

    it("renames a block", () => {
      const result = dispatchOp(
        { verb: "label", positionals: ["web", "webserver"], params: {}, selectors: [], raw: "" },
        config, log,
      );
      expect(result.success).toBe(true);
      expect(result.prefix).toBe("*");
      const block = [...config.blocks.values()][0];
      expect(block.label).toBe("webserver");
    });

    it("emits block_renamed event", () => {
      dispatchOp(
        { verb: "label", positionals: ["web", "webserver"], params: {}, selectors: [], raw: "" },
        config, log,
      );
      const events = log.recent();
      const renameEvent = events.find((e) => e.type === "block_renamed");
      expect(renameEvent).toBeDefined();
      if (renameEvent?.type === "block_renamed") {
        expect(renameEvent.before).toBe("web");
        expect(renameEvent.after).toBe("webserver");
      }
    });

    it("returns error for non-existent block", () => {
      const result = dispatchOp(
        { verb: "label", positionals: ["nonexistent", "newname"], params: {}, selectors: [], raw: "" },
        config, log,
      );
      expect(result.success).toBe(false);
    });

    it("returns error when new label already exists", () => {
      dispatchOp(
        { verb: "add", positionals: ["resource", "aws_s3_bucket", "assets"], params: {}, selectors: [], raw: "" },
        config, log,
      );
      const result = dispatchOp(
        { verb: "label", positionals: ["web", "assets"], params: {}, selectors: [], raw: "" },
        config, log,
      );
      expect(result.success).toBe(false);
      expect(result.message).toContain("already exists");
    });

    it("returns error with missing args", () => {
      const result = dispatchOp(
        { verb: "label", positionals: ["web"], params: {}, selectors: [], raw: "" },
        config, log,
      );
      expect(result.success).toBe(false);
    });
  });

  // ── STYLE ───────────────────────────────────────────────

  describe("style", () => {
    beforeEach(() => {
      dispatchOp(
        { verb: "add", positionals: ["resource", "aws_instance", "web"], params: {}, selectors: [], raw: "" },
        config, log,
      );
    });

    it("sets tags on a block", () => {
      const result = dispatchOp(
        { verb: "style", positionals: ["web"], params: { tags: "Name=WebServer,Env=prod" }, selectors: [], raw: "" },
        config, log,
      );
      expect(result.success).toBe(true);
      expect(result.prefix).toBe("*");
      const block = [...config.blocks.values()][0];
      expect(block.tags.get("Name")).toBe("WebServer");
      expect(block.tags.get("Env")).toBe("prod");
    });

    it("emits tag_set events", () => {
      dispatchOp(
        { verb: "style", positionals: ["web"], params: { tags: "Name=WebServer,Env=prod" }, selectors: [], raw: "" },
        config, log,
      );
      const events = log.recent();
      const tagEvents = events.filter((e) => e.type === "tag_set");
      expect(tagEvents.length).toBe(2);
    });

    it("records before value for existing tags", () => {
      // Set initial tag
      dispatchOp(
        { verb: "style", positionals: ["web"], params: { tags: "Name=OldName" }, selectors: [], raw: "" },
        config, log,
      );
      // Update the tag
      dispatchOp(
        { verb: "style", positionals: ["web"], params: { tags: "Name=NewName" }, selectors: [], raw: "" },
        config, log,
      );
      const events = log.recent();
      const lastTagEvent = events.filter((e) => e.type === "tag_set").pop();
      if (lastTagEvent?.type === "tag_set") {
        expect(lastTagEvent.before).toBe("OldName");
        expect(lastTagEvent.after).toBe("NewName");
      }
    });

    it("returns error when label is missing", () => {
      const result = dispatchOp(
        { verb: "style", positionals: [], params: { tags: "X=Y" }, selectors: [], raw: "" },
        config, log,
      );
      expect(result.success).toBe(false);
    });

    it("returns error when block not found", () => {
      const result = dispatchOp(
        { verb: "style", positionals: ["nonexistent"], params: { tags: "X=Y" }, selectors: [], raw: "" },
        config, log,
      );
      expect(result.success).toBe(false);
    });

    it("returns error when tags param is missing", () => {
      const result = dispatchOp(
        { verb: "style", positionals: ["web"], params: {}, selectors: [], raw: "" },
        config, log,
      );
      expect(result.success).toBe(false);
      expect(result.message).toContain("tags");
    });
  });

  // ── NEST ────────────────────────────────────────────────

  describe("nest", () => {
    beforeEach(() => {
      dispatchOp(
        { verb: "add", positionals: ["resource", "aws_security_group", "sg"], params: {}, selectors: [], raw: "" },
        config, log,
      );
    });

    it("adds a nested block", () => {
      const result = dispatchOp(
        { verb: "nest", positionals: ["sg", "ingress"], params: { from_port: "80", to_port: "80", protocol: "tcp" }, selectors: [], raw: "" },
        config, log,
      );
      expect(result.success).toBe(true);
      expect(result.prefix).toBe("+");
      const block = [...config.blocks.values()][0];
      expect(block.nestedBlocks).toHaveLength(1);
      expect(block.nestedBlocks[0].type).toBe("ingress");
      expect(block.nestedBlocks[0].attributes.get("from_port")?.value).toBe("80");
    });

    it("emits nested_block_added event", () => {
      dispatchOp(
        { verb: "nest", positionals: ["sg", "ingress"], params: { from_port: "80" }, selectors: [], raw: "" },
        config, log,
      );
      const events = log.recent();
      const nestEvent = events.find((e) => e.type === "nested_block_added");
      expect(nestEvent).toBeDefined();
    });

    it("returns error with missing args", () => {
      const result = dispatchOp(
        { verb: "nest", positionals: ["sg"], params: {}, selectors: [], raw: "" },
        config, log,
      );
      expect(result.success).toBe(false);
    });

    it("returns error for non-existent block", () => {
      const result = dispatchOp(
        { verb: "nest", positionals: ["nonexistent", "ingress"], params: {}, selectors: [], raw: "" },
        config, log,
      );
      expect(result.success).toBe(false);
    });
  });

  // ── UNSET ───────────────────────────────────────────────

  describe("unset", () => {
    beforeEach(() => {
      dispatchOp(
        { verb: "add", positionals: ["resource", "aws_instance", "web"], params: { ami: "ami-123", instance_type: "t2.micro" }, selectors: [], raw: "" },
        config, log,
      );
    });

    it("removes an attribute from a block", () => {
      const result = dispatchOp(
        { verb: "unset", positionals: ["web", "ami"], params: {}, selectors: [], raw: "" },
        config, log,
      );
      expect(result.success).toBe(true);
      expect(result.prefix).toBe("*");
      const block = [...config.blocks.values()][0];
      expect(block.attributes.has("ami")).toBe(false);
      expect(block.attributes.has("instance_type")).toBe(true);
    });

    it("emits attribute_removed event", () => {
      dispatchOp(
        { verb: "unset", positionals: ["web", "ami"], params: {}, selectors: [], raw: "" },
        config, log,
      );
      const events = log.recent();
      const unsetEvent = events.find((e) => e.type === "attribute_removed");
      expect(unsetEvent).toBeDefined();
      if (unsetEvent?.type === "attribute_removed") {
        expect(unsetEvent.key).toBe("ami");
        expect(unsetEvent.before.value).toBe("ami-123");
      }
    });

    it("returns error with missing label", () => {
      const result = dispatchOp(
        { verb: "unset", positionals: [], params: {}, selectors: [], raw: "" },
        config, log,
      );
      expect(result.success).toBe(false);
    });

    it("returns error when block not found", () => {
      const result = dispatchOp(
        { verb: "unset", positionals: ["nonexistent", "ami"], params: {}, selectors: [], raw: "" },
        config, log,
      );
      expect(result.success).toBe(false);
    });

    it("returns error when no keys specified", () => {
      const result = dispatchOp(
        { verb: "unset", positionals: ["web"], params: {}, selectors: [], raw: "" },
        config, log,
      );
      expect(result.success).toBe(false);
      expect(result.message).toContain("KEY");
    });
  });

  // ── UNKNOWN VERB ────────────────────────────────────────

  describe("unknown verb", () => {
    it("returns error for unknown verb", () => {
      const result = dispatchOp(
        { verb: "frobnicate", positionals: [], params: {}, selectors: [], raw: "" },
        config, log,
      );
      expect(result.success).toBe(false);
      expect(result.message).toContain("unhandled verb");
    });
  });
});
