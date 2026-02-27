import { createFcpServer } from "@aetherwing/fcp-core";
import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";
import { TerraformAdapter } from "./adapter.js";
import { VERB_SPECS, REFERENCE_CARD_SECTIONS } from "./verbs.js";

// eslint-disable-next-line @typescript-eslint/no-explicit-any
export const server: any = createFcpServer({
  domain: "terraform",
  adapter: new TerraformAdapter(),
  verbs: VERB_SPECS,
  referenceCard: { sections: REFERENCE_CARD_SECTIONS },
});

const transport = new StdioServerTransport();
await server.connect(transport);

export { TerraformAdapter } from "./adapter.js";
export type { TerraformConfig, TerraformEvent, TfBlock, Connection, Attribute } from "./types.js";
