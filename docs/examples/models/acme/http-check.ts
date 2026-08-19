import { defineModel } from "#lib/defineModel";

defineModel({
  type: "@acme/http-check",
  version: "1.0.0",
  arguments: {
    url: { type: "string", required: true },
    expect: { type: "number" },
    method: { type: "string", enum: ["GET", "HEAD", "POST"] },
  },
  outputs: ["status", "elapsed_ms"],
  execute: async (args) => {
    const start = Date.now();
    const res = await fetch(args.url, { method: args.method ?? "GET" });
    const elapsed_ms = Date.now() - start;

    if (args.expect !== undefined && res.status !== args.expect) {
      throw new Error(`expected ${args.expect}, got ${res.status}`);
    }

    return { status: res.status, elapsed_ms };
  },
});
