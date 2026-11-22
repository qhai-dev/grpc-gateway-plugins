// Regenerates src/gen with Buf.
//
// This is a thin wrapper around `buf generate`, not a reimplementation of it. Buf owns
// input resolution, the dependency fetch and the plugin invocation; what is left here
// are the things Buf does not do for this project:
//
//   1. Prefer the Buf binary installed by npm, so `npm install` is the only setup step.
//   2. Fail legibly when Buf or Go is missing, instead of a bare ENOENT.
//   3. Remove the previous output first, so a deleted or renamed proto cannot leave an
//      orphaned generated file behind that still type-checks.
//
// Everything that decides the output lives in buf.gen.yaml. Adding a plugin or changing
// a contract option is a change to that file, not to this one.

import { spawnSync } from "node:child_process";
import { existsSync, rmSync } from "node:fs";
import { join } from "node:path";

import { exampleDir, requireGo, resolveBuf } from "./buf.mjs";

const outDir = join(exampleDir, "src", "gen");

function fail(message) {
  process.stderr.write(`${message}\n`);
  process.exit(1);
}

function run(command, args) {
  process.stdout.write(`$ ${command} ${args.join(" ")}\n`);
  const result = spawnSync(command, args, {
    // Buf resolves `local`, `out` and the plugin paths inside `local` against the
    // process working directory, so this must be the example directory.
    cwd: exampleDir,
    stdio: "inherit",
    env: process.env,
  });
  if (result.error) fail(`${command} failed to start: ${String(result.error)}`);
  if (result.status !== 0) fail(`${command} ${args.join(" ")} exited with ${result.status}`);
}

let buf;
try {
  buf = resolveBuf();
  requireGo();
} catch (error) {
  fail(error instanceof Error ? error.message : String(error));
}

// Dependencies are pinned in buf.lock. Regenerating code deliberately does not update
// them: a code change should not silently move a dependency. `npm run deps:update` is
// the explicit command for that.
if (!existsSync(join(exampleDir, "buf.lock"))) {
  fail("buf.lock is missing; run `npm run deps:update` once to create it.");
}

// A stale file from a removed proto would still compile and would be imported by
// nothing, which is the kind of leftover that survives for months unnoticed.
rmSync(outDir, { recursive: true, force: true });

run(buf, ["generate"]);

process.stdout.write("generated src/gen\n");
