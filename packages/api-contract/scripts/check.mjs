#!/usr/bin/env node
// Fails when the committed generated types do not match openapi.yaml.
// Generates into a temp directory and diffs, so a dirty working tree is
// not rewritten just to find out that it was already in sync.

import { spawnSync } from "node:child_process";
import { mkdtempSync, readFileSync, rmSync, writeFileSync, mkdirSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));
const pkg = path.resolve(here, "..");
const repo = path.resolve(pkg, "../..");
const spec = path.join(pkg, "openapi.yaml");
const tsCommitted = path.join(pkg, "src/schema.ts");
const goCommitted = path.join(repo, "services/api/internal/contract/types.gen.go");
const config = path.join(pkg, "oapi-codegen.yaml");

function run(command, args, cwd) {
  const result = spawnSync(command, args, {
    cwd,
    encoding: "utf8",
    shell: process.platform === "win32",
  });
  if (result.status !== 0) {
    process.stderr.write(result.stderr || result.stdout || "");
    process.exit(result.status ?? 1);
  }
  return result;
}

function fail(message) {
  console.error(message);
  process.exit(1);
}

const tmp = mkdtempSync(path.join(os.tmpdir(), "parkxchange-contract-"));
try {
  const tsFresh = path.join(tmp, "schema.ts");
  run("pnpm", ["exec", "openapi-typescript", spec, "-o", tsFresh], pkg);

  const goDir = path.join(tmp, "contract");
  mkdirSync(goDir);
  const goFresh = path.join(goDir, "types.gen.go");
  const tmpConfig = path.join(tmp, "oapi-codegen.yaml");
  writeFileSync(
    tmpConfig,
    readFileSync(config, "utf8").replace(
      /output:\s+.+$/m,
      `output: ${goFresh.replaceAll("\\", "/")}`,
    ),
  );
  run("go", ["tool", "oapi-codegen", "--config", tmpConfig, spec], path.join(repo, "services/api"));

  const diffs = [];
  if (readFileSync(tsCommitted, "utf8") !== readFileSync(tsFresh, "utf8")) {
    diffs.push("packages/api-contract/src/schema.ts");
  }
  if (readFileSync(goCommitted, "utf8") !== readFileSync(goFresh, "utf8")) {
    diffs.push("services/api/internal/contract/types.gen.go");
  }

  if (diffs.length > 0) {
    fail(
      `generated types have drifted from openapi.yaml:\n  ${diffs.join("\n  ")}\n` +
        "run `task contract:generate` and commit the result",
    );
  }

  console.log("contract types match openapi.yaml");
} finally {
  rmSync(tmp, { recursive: true, force: true });
}
