#!/usr/bin/env node
// Locates the pinned Buf and forwards arguments to it.
//
// `@bufbuild/buf` is a devDependency, so `npm install` provides a Buf without a system
// install and pins its version in the lockfile. The package is a thin loader; the real
// binary lives in a per-platform package such as `@bufbuild/buf-win32-x64`.
//
// The platform binary is preferred over the `.bin` shim because the shim is a .cmd
// wrapper on Windows and Node refuses to spawn those directly (EINVAL). A PATH lookup is
// the fallback, for anyone who would rather use their own Buf.
//
// Usage (from npm scripts, or directly):
//   node scripts/buf.mjs lint
//   node scripts/buf.mjs dep update
//   node scripts/buf.mjs generate

import { spawnSync } from "node:child_process";
import { existsSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

export const exampleDir = resolve(dirname(fileURLToPath(import.meta.url)), "..");

/** Per-platform npm packages that carry the real Buf binary. */
const PLATFORM_PACKAGES = {
  "win32-x64": "@bufbuild/buf-win32-x64",
  "win32-arm64": "@bufbuild/buf-win32-arm64",
  "darwin-x64": "@bufbuild/buf-darwin-x64",
  "darwin-arm64": "@bufbuild/buf-darwin-arm64",
  "linux-x64": "@bufbuild/buf-linux-x64",
  "linux-arm64": "@bufbuild/buf-linux-aarch64",
  "linux-armv7": "@bufbuild/buf-linux-armv7",
};

/**
 * Returns a usable Buf executable path, or throws with install instructions.
 */
export function resolveBuf() {
  const key = `${process.platform}-${process.arch}`;
  const packageName = PLATFORM_PACKAGES[key];
  if (packageName) {
    const binary = join(
      exampleDir,
      "node_modules",
      ...packageName.split("/"),
      "bin",
      process.platform === "win32" ? "buf.exe" : "buf",
    );
    if (existsSync(binary)) return binary;
  }

  const probe = spawnSync("buf", ["--version"], { stdio: "ignore" });
  if (!probe.error && probe.status === 0) return "buf";

  throw new Error(
    [
      `buf is not available for ${key}.`,
      "",
      "Install it with one of:",
      "  npm install                                  # pinned version, no system install",
      "  go install github.com/bufbuild/buf/cmd/buf@latest",
      "  brew install bufbuild/buf/buf",
    ].join("\n"),
  );
}

/**
 * Throws when Go is missing. The plugins run via `go run`, so Go is required even
 * though the generated output is TypeScript.
 */
export function requireGo() {
  const probe = spawnSync("go", ["version"], { stdio: "ignore" });
  if (!probe.error && probe.status === 0) return;
  throw new Error(
    [
      "go is not installed or not on PATH.",
      "",
      "buf.gen.yaml runs the plugins with `go run`, so Go is required even though the",
      "generated output is TypeScript. Install it from https://go.dev/dl/.",
    ].join("\n"),
  );
}

// Only forward arguments when run as a program, so the module can be imported by the
// generate script without executing Buf twice.
const invokedDirectly = process.argv[1] !== undefined &&
  resolve(process.argv[1]) === fileURLToPath(import.meta.url);

if (invokedDirectly) {
  try {
    const buf = resolveBuf();
    const result = spawnSync(buf, process.argv.slice(2), {
      // Buf resolves `local`, `out` and plugin paths against the working directory.
      cwd: exampleDir,
      stdio: "inherit",
      env: process.env,
    });
    if (result.error) {
      process.stderr.write(`${String(result.error)}\n`);
      process.exit(1);
    }
    process.exit(result.status ?? 1);
  } catch (error) {
    process.stderr.write(`${error instanceof Error ? error.message : String(error)}\n`);
    process.exit(1);
  }
}
