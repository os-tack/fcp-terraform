import { describe, it, expect, beforeEach } from "vitest";
import { EventLog } from "@aetherwing/fcp-core";
import { dispatchQuery } from "../src/queries.js";
import { dispatchOp } from "../src/ops.js";
import { createEmptyConfig, rebuildLabelIndex, createBlock, addBlock, addConnection, generateId } from "../src/model.js";
import type { TerraformConfig, TerraformEvent, Connection } from "../src/types.js";

describe("dispatchQuery", () => {
  let config: TerraformConfig;
  let log: EventLog<TerraformEvent>;

  beforeEach(() => {
    config = createEmptyConfig("Test Infrastructure");
    rebuildLabelIndex(config);
    log = new EventLog<TerraformEvent>();
  });

  // Helper: add blocks via dispatchOp to keep indexes in sync
  function addResource(type: string, label: string, params: Record<string, string> = {}) {
    dispatchOp(
      { verb: "add", positionals: ["resource", type, label], params, selectors: [], raw: "" },
      config, log,
    );
  }

  function addProvider(name: string, params: Record<string, string> = {}) {
    dispatchOp(
      { verb: "add", positionals: ["provider", name], params, selectors: [], raw: "" },
      config, log,
    );
  }

  function addVariable(name: string, params: Record<string, string> = {}) {
    dispatchOp(
      { verb: "add", positionals: ["variable", name], params, selectors: [], raw: "" },
      config, log,
    );
  }

  function addOutput(name: string, params: Record<string, string> = {}) {
    dispatchOp(
      { verb: "add", positionals: ["output", name], params, selectors: [], raw: "" },
      config, log,
    );
  }

  function addData(type: string, label: string, params: Record<string, string> = {}) {
    dispatchOp(
      { verb: "add", positionals: ["data", type, label], params, selectors: [], raw: "" },
      config, log,
    );
  }

  function connectBlocks(src: string, tgt: string) {
    dispatchOp(
      { verb: "connect", positionals: [src, "->", tgt], params: {}, selectors: [], raw: "" },
      config, log,
    );
  }

  // ── MAP ─────────────────────────────────────────────────

  describe("map", () => {
    it("returns config summary", () => {
      addProvider("aws", { region: "us-east-1" });
      addResource("aws_instance", "web", { ami: "ami-123" });
      addResource("aws_s3_bucket", "assets");
      addVariable("env");
      addOutput("web_ip", { value: "aws_instance.web.public_ip" });

      const result = dispatchQuery("map", config, log);
      expect(result).toContain("Test Infrastructure");
      expect(result).toContain("Providers: aws");
      expect(result).toContain("Resources (2)");
      expect(result).toContain("aws_instance x1");
      expect(result).toContain("aws_s3_bucket x1");
      expect(result).toContain("Variables: 1");
      expect(result).toContain("Outputs: 1");
    });

    it("shows connections count when present", () => {
      addResource("aws_instance", "web");
      addResource("aws_s3_bucket", "assets");
      connectBlocks("web", "assets");

      const result = dispatchQuery("map", config, log);
      expect(result).toContain("Connections: 1");
    });

    it("handles empty config", () => {
      const result = dispatchQuery("map", config, log);
      expect(result).toContain("Test Infrastructure");
    });
  });

  // ── LIST ────────────────────────────────────────────────

  describe("list", () => {
    beforeEach(() => {
      addResource("aws_instance", "web");
      addResource("aws_instance", "api");
      addResource("aws_s3_bucket", "assets");
      addVariable("env");
    });

    it("lists all blocks", () => {
      const result = dispatchQuery("list", config, log);
      expect(result).toContain("web");
      expect(result).toContain("api");
      expect(result).toContain("assets");
      expect(result).toContain("env");
    });

    it("filters by @type selector", () => {
      const result = dispatchQuery("list @type:aws_instance", config, log);
      expect(result).toContain("web");
      expect(result).toContain("api");
      expect(result).not.toContain("assets");
      expect(result).not.toContain("env");
    });

    it("filters by @kind selector", () => {
      const result = dispatchQuery("list @kind:variable", config, log);
      expect(result).toContain("env");
      expect(result).not.toContain("web");
    });

    it("filters by @provider selector", () => {
      const result = dispatchQuery("list @provider:aws", config, log);
      expect(result).toContain("web");
      expect(result).toContain("assets");
    });

    it("returns message when no blocks found", () => {
      const empty = createEmptyConfig("Empty");
      rebuildLabelIndex(empty);
      const result = dispatchQuery("list", empty, log);
      expect(result).toContain("No blocks found");
    });
  });

  // ── DESCRIBE ────────────────────────────────────────────

  describe("describe", () => {
    it("shows full block details", () => {
      addResource("aws_instance", "web", { ami: "ami-123", instance_type: "t2.micro" });
      const result = dispatchQuery("describe web", config, log);
      expect(result).toContain("resource");
      expect(result).toContain("aws_instance.web");
      expect(result).toContain("provider: aws");
      expect(result).toContain("attributes:");
      expect(result).toContain("ami");
      expect(result).toContain("ami-123");
      expect(result).toContain("instance_type");
    });

    it("shows tags", () => {
      addResource("aws_instance", "web");
      dispatchOp(
        { verb: "style", positionals: ["web"], params: { tags: "Name=WebServer" }, selectors: [], raw: "" },
        config, log,
      );
      const result = dispatchQuery("describe web", config, log);
      expect(result).toContain("tags:");
      expect(result).toContain("Name");
      expect(result).toContain("WebServer");
    });

    it("shows connections", () => {
      addResource("aws_instance", "web");
      addResource("aws_s3_bucket", "assets");
      connectBlocks("web", "assets");
      const result = dispatchQuery("describe web", config, log);
      expect(result).toContain("connections:");
      expect(result).toContain("assets");
    });

    it("returns error when no label provided", () => {
      const result = dispatchQuery("describe", config, log);
      expect(result).toContain("requires a LABEL");
    });

    it("returns error for non-existent block", () => {
      const result = dispatchQuery("describe nonexistent", config, log);
      expect(result).toContain("not found");
    });
  });

  // ── PLAN ────────────────────────────────────────────────

  describe("plan", () => {
    it("generates valid HCL", () => {
      addProvider("aws", { region: "us-east-1" });
      addResource("aws_instance", "web", { ami: "ami-123", instance_type: "t2.micro" });

      const result = dispatchQuery("plan", config, log);
      expect(result).toContain('provider "aws"');
      expect(result).toContain('region = "us-east-1"');
      expect(result).toContain('resource "aws_instance" "web"');
      expect(result).toContain("ami");
      expect(result).toContain("instance_type");
    });

    it("includes depends_on from connections", () => {
      addResource("aws_instance", "web", { ami: "ami-123" });
      addResource("aws_s3_bucket", "assets");
      connectBlocks("web", "assets");

      const result = dispatchQuery("plan", config, log);
      expect(result).toContain("depends_on = [aws_s3_bucket.assets]");
    });

    it("returns message for empty config", () => {
      const result = dispatchQuery("plan", config, log);
      expect(result).toContain("Empty configuration");
    });
  });

  // ── GRAPH ───────────────────────────────────────────────

  describe("graph", () => {
    it("shows dependency graph", () => {
      addResource("aws_instance", "web");
      addResource("aws_s3_bucket", "assets");
      connectBlocks("web", "assets");

      const result = dispatchQuery("graph", config, log);
      expect(result).toContain("Dependency Graph:");
      expect(result).toContain("web -> assets");
    });

    it("returns message when no connections", () => {
      addResource("aws_instance", "web");
      const result = dispatchQuery("graph", config, log);
      expect(result).toContain("No connections");
    });
  });

  // ── VALIDATE ────────────────────────────────────────────

  describe("validate", () => {
    it("returns valid for well-formed config", () => {
      addProvider("aws", { region: "us-east-1" });
      addResource("aws_instance", "web", { ami: "ami-123" });
      addOutput("web_ip", { value: "aws_instance.web.public_ip" });

      const result = dispatchQuery("validate", config, log);
      expect(result).toContain("valid");
    });

    it("finds missing value on output", () => {
      addOutput("web_ip");
      const result = dispatchQuery("validate", config, log);
      expect(result).toContain("ERROR");
      expect(result).toContain("output");
      expect(result).toContain("value");
    });

    it("finds missing source on module", () => {
      dispatchOp(
        { verb: "add", positionals: ["module", "vpc"], params: {}, selectors: [], raw: "" },
        config, log,
      );
      const result = dispatchQuery("validate", config, log);
      expect(result).toContain("ERROR");
      expect(result).toContain("module");
      expect(result).toContain("source");
    });

    it("warns about resource with no attributes", () => {
      addResource("aws_instance", "web");
      const result = dispatchQuery("validate", config, log);
      expect(result).toContain("WARNING");
      expect(result).toContain("no attributes");
    });
  });

  // ── STATS ───────────────────────────────────────────────

  describe("stats", () => {
    it("returns block counts by kind", () => {
      addProvider("aws");
      addResource("aws_instance", "web");
      addResource("aws_s3_bucket", "assets");
      addVariable("env");
      addOutput("ip", { value: "aws_instance.web.public_ip" });

      const result = dispatchQuery("stats", config, log);
      expect(result).toContain("Total blocks: 5");
      expect(result).toContain("provider: 1");
      expect(result).toContain("resource: 2");
      expect(result).toContain("variable: 1");
      expect(result).toContain("output: 1");
      expect(result).toContain("Connections: 0");
    });
  });

  // ── STATUS ──────────────────────────────────────────────

  describe("status", () => {
    it("returns title and file path", () => {
      const result = dispatchQuery("status", config, log);
      expect(result).toContain("Title: Test Infrastructure");
      expect(result).toContain("File: (unsaved)");
      expect(result).toContain("Blocks: 0");
      expect(result).toContain("Connections: 0");
    });

    it("shows block and connection counts", () => {
      addResource("aws_instance", "web");
      addResource("aws_s3_bucket", "assets");
      connectBlocks("web", "assets");

      const result = dispatchQuery("status", config, log);
      expect(result).toContain("Blocks: 2");
      expect(result).toContain("Connections: 1");
    });
  });

  // ── FIND ────────────────────────────────────────────────

  describe("find", () => {
    beforeEach(() => {
      addResource("aws_instance", "web_server");
      addResource("aws_s3_bucket", "my_assets");
    });

    it("finds blocks by label", () => {
      const result = dispatchQuery("find web", config, log);
      expect(result).toContain("web_server");
    });

    it("finds blocks by type", () => {
      const result = dispatchQuery("find s3", config, log);
      expect(result).toContain("my_assets");
    });

    it("returns message when nothing found", () => {
      const result = dispatchQuery("find nonexistent", config, log);
      expect(result).toContain("No blocks matching");
    });

    it("returns error when no search text", () => {
      const result = dispatchQuery("find", config, log);
      expect(result).toContain("requires search text");
    });
  });

  // ── UNKNOWN QUERY ───────────────────────────────────────

  describe("unknown query", () => {
    it("returns helpful error", () => {
      const result = dispatchQuery("frobnicate", config, log);
      expect(result).toContain('Unknown query');
      expect(result).toContain("frobnicate");
      expect(result).toContain("Available:");
      expect(result).toContain("map");
      expect(result).toContain("plan");
    });
  });
});
