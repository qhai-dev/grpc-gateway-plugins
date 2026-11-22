// Starts the mock gateway and the Vite dev server together.
//
// The example is meant to be runnable straight from a clone, and it needs two
// processes because the mock stands in for the Go backend. This script starts both,
// forwards their output, and shuts both down when it exits normally or on Ctrl-C.
//
// Limitations worth stating: a SIGKILL of this process cannot run any handler, so the
// children survive it. That is a property of hard kills rather than of this script;
// `npm run dev` and `npm run mock` in two terminals avoid the question entirely.

import { spawn } from "node:child_process";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const exampleDir = resolve(here, "..");

const node = process.execPath;
const viteBin = join(exampleDir, "node_modules", "vite", "bin", "vite.js");
const mockBin = join(exampleDir, "mock-server", "server.mjs");

/** @type {import("node:child_process").ChildProcess[]} */
const children = [];
let shuttingDown = false;

function start(name, args) {
  const child = spawn(node, args, {
    cwd: exampleDir,
    stdio: "inherit",
    env: process.env,
  });
  child.on("exit", (code, signal) => {
    if (shuttingDown) return;
    process.stdout.write(`${name} exited (code=${code ?? "null"} signal=${signal ?? "null"})\n`);
    // One process dying means the example is unusable, so stop the other too.
    shutdown(code ?? 1);
  });
  child.on("error", (error) => {
    process.stderr.write(`${name} failed to start: ${String(error)}\n`);
    shutdown(1);
  });
  children.push(child);
  return child;
}

function shutdown(code) {
  if (shuttingDown) return;
  shuttingDown = true;
  for (const child of children) {
    if (child.exitCode === null && child.signalCode === null) {
      child.kill("SIGTERM");
    }
  }
  process.exitCode = code;
}

process.on("SIGINT", () => shutdown(0));
process.on("SIGTERM", () => shutdown(0));
process.on("SIGHUP", () => shutdown(0));

process.stdout.write("starting mock grpc-gateway on :8080 and vite on :5173\n");
start("mock-gateway", [mockBin]);
start("vite", [viteBin, "--port", "5173", "--strictPort"]);
