#!/usr/bin/env node
// Runs the binary of the platform package npm installed next to this one. The
// command package lists one optionalDependency per platform, and npm installs
// only the one whose os and cpu match. See npm/publish.mjs.
"use strict";

const { spawnSync } = require("node:child_process");
const pkg = require("./package.json");

const command = Object.keys(pkg.bin)[0];
const platform = `${process.platform}-${process.arch}`;
const platformPackage = `${pkg.name}-${platform}`;
const executable = process.platform === "win32" ? `${command}.exe` : command;
const goInstall = `go install github.com/d0whc3r/protoc-gen-strict/cmd/${command}@v${pkg.version}`;

if (!(platformPackage in pkg.optionalDependencies)) {
  console.error(`${command}: no prebuilt binary for ${platform}. Build it instead: ${goInstall}`);
  process.exit(1);
}

let binary;
try {
  binary = require.resolve(`${platformPackage}/${executable}`);
} catch {
  console.error(
    `${command}: ${platformPackage} is not installed. npm skips it when optional dependencies are omitted ` +
      `(--omit=optional, --no-optional) or the lockfile was written on another platform. Reinstall, or: ${goInstall}`,
  );
  process.exit(1);
}

// protoc and buf talk to the plugin over stdin and stdout: hand it the same ones.
const { status, error } = spawnSync(binary, process.argv.slice(2), { stdio: "inherit" });
if (error) throw error;
process.exit(status ?? 1);
