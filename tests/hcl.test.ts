import { describe, it, expect, beforeEach } from "vitest";
import { serializeToHcl } from "../src/hcl.js";
import {
  createEmptyConfig, addBlock, createBlock, addConnection,
  rebuildLabelIndex, generateId,
} from "../src/model.js";
import type { TerraformConfig, Connection } from "../src/types.js";

describe("serializeToHcl", () => {
  let config: TerraformConfig;

  beforeEach(() => {
    config = createEmptyConfig("Test");
    rebuildLabelIndex(config);
  });

  it("serializes a provider", () => {
    const block = createBlock("provider", "aws", "aws", { region: "us-east-1" });
    block.provider = "aws";
    addBlock(config, block);

    const hcl = serializeToHcl(config);
    expect(hcl).toContain('provider "aws"');
    expect(hcl).toContain('region = "us-east-1"');
  });

  it("serializes a resource with string and number attrs", () => {
    const block = createBlock("resource", "aws_instance", "web", {
      ami: "ami-0c55b159",
      instance_type: "t2.micro",
    });
    addBlock(config, block);

    const hcl = serializeToHcl(config);
    expect(hcl).toContain('resource "aws_instance" "web"');
    expect(hcl).toContain('ami  = "ami-0c55b159"');
    expect(hcl).toContain('instance_type = "t2.micro"');
  });

  it("serializes a resource with bool attrs", () => {
    const block = createBlock("resource", "aws_s3_bucket", "assets", {
      versioning: "true",
    });
    addBlock(config, block);

    const hcl = serializeToHcl(config);
    expect(hcl).toContain("versioning = true");
  });

  it("serializes a resource with count", () => {
    const block = createBlock("resource", "aws_instance", "web", {});
    block.meta.count = "2";
    addBlock(config, block);

    const hcl = serializeToHcl(config);
    expect(hcl).toContain("count = 2");
  });

  it("serializes tags", () => {
    const block = createBlock("resource", "aws_instance", "web", {});
    block.tags.set("Name", "WebServer");
    block.tags.set("Env", "prod");
    addBlock(config, block);

    const hcl = serializeToHcl(config);
    expect(hcl).toContain("tags = {");
    expect(hcl).toContain('Name = "WebServer"');
    expect(hcl).toContain('Env = "prod"');
  });

  it("serializes a variable", () => {
    const block = createBlock("variable", "variable", "env", {
      type: "string",
      default: "production",
    });
    block.provider = "";
    addBlock(config, block);

    const hcl = serializeToHcl(config);
    expect(hcl).toContain('variable "env"');
    expect(hcl).toContain("type    = string");
    expect(hcl).toContain('default = "production"');
  });

  it("serializes an output with reference value", () => {
    const block = createBlock("output", "output", "web_ip", {});
    block.provider = "";
    block.attributes.set("value", {
      key: "value", value: "aws_instance.web.public_ip", valueType: "reference",
    });
    addBlock(config, block);

    const hcl = serializeToHcl(config);
    expect(hcl).toContain('output "web_ip"');
    expect(hcl).toContain("value = aws_instance.web.public_ip");
  });

  it("serializes depends_on from connections", () => {
    const b1 = createBlock("resource", "aws_instance", "web", { ami: "ami-123" });
    const b2 = createBlock("resource", "aws_s3_bucket", "assets", { acl: "private" });
    addBlock(config, b1);
    addBlock(config, b2);

    const conn: Connection = {
      id: generateId(), sourceId: b1.id, targetId: b2.id,
      sourceLabel: "web", targetLabel: "assets",
    };
    addConnection(config, conn);

    const hcl = serializeToHcl(config);
    expect(hcl).toContain("depends_on = [aws_s3_bucket.assets]");
  });

  it("orders blocks: provider → variable → resource → output", () => {
    addBlock(config, createBlock("output", "output", "ip", {}));
    addBlock(config, createBlock("resource", "aws_instance", "web", {}));
    const prov = createBlock("provider", "aws", "aws", {});
    prov.provider = "aws";
    addBlock(config, prov);
    const v = createBlock("variable", "variable", "env", {});
    v.provider = "";
    addBlock(config, v);

    const hcl = serializeToHcl(config);
    const provIdx = hcl.indexOf('provider "aws"');
    const varIdx = hcl.indexOf('variable "env"');
    const resIdx = hcl.indexOf('resource "aws_instance"');
    const outIdx = hcl.indexOf('output "ip"');
    expect(provIdx).toBeLessThan(varIdx);
    expect(varIdx).toBeLessThan(resIdx);
    expect(resIdx).toBeLessThan(outIdx);
  });
});
