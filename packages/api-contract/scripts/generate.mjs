#!/usr/bin/env node
// Regenerates TypeScript and Go types from openapi.yaml.
// The committed files are the contract; `check.mjs` fails when they drift.

import { spawnSync } from "node:child_process";
import path from "node:path";
import { fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));
const pkg = path.resolve(here, "..");
const repo = path.resolve(pkg, "../..");
const spec = path.join(pkg, "openapi.yaml");
const tsOut = path.join(pkg, "src/schema.ts");
const config = path.join(pkg, "oapi-codegen.yaml");

function run(command, args, cwd) {
  const result = spawnSync(command, args, {
    cwd,
    stdio: "inherit",
    shell: process.platform === "win32",
  });
  if (result.status !== 0) {
    process.exit(result.status ?? 1);
  }
}

run("pnpm", ["exec", "openapi-typescript", spec, "-o", tsOut], pkg);
run("go", ["tool", "oapi-codegen", "--config", config, spec], path.join(repo, "services/api"));

console.log("generated", path.relative(repo, tsOut));
console.log("generated", "services/api/internal/contract/types.gen.go");
