// Publishes the GoReleaser build in dist/ to npm the way esbuild ships its
// binary: one package per command and platform holding the binary, and one
// package per command whose bin shim, npm/bin.js, runs it.
//
//   @d0whc3r/protoc-gen-strict               bin shim, one optionalDependency per platform
//   @d0whc3r/protoc-gen-strict-darwin-arm64  os: darwin, cpu: arm64, the binary
//   @d0whc3r/protoc-gen-strict-linux-x64     os: linux, cpu: x64, the binary
//   ...
//
// npm installs only the platform package whose os and cpu match, so no install
// script downloads anything and --ignore-scripts changes nothing.
//
// Run from the repository root, after `goreleaser release`. `--pack` writes the
// tarballs into dist/npm instead of publishing them.
import { execFileSync } from "node:child_process";
import { chmodSync, copyFileSync, mkdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { join, resolve } from "node:path";

const SCOPE = "@d0whc3r";
const STAGE = "dist/npm";
const REPOSITORY = "git+https://github.com/d0whc3r/protoc-gen-strict.git";

// GoReleaser's GOOS and GOARCH, as Node's process.platform and process.arch.
const PLATFORMS = { darwin: "darwin", linux: "linux", windows: "win32" };
const ARCHES = { amd64: "x64", arm64: "arm64" };

// The command packages, by GoReleaser build id.
const COMMANDS = {
  "protoc-gen-strict":
    "protoc/buf plugin: strict TypeScript and Python types, and OpenAPI field options, from buf.validate rules",
  "protoc-gen-strict-schema": "protoc/buf plugin: JSON Schema and Zod runtime schemas from buf.validate rules",
};

const pack = process.argv.includes("--pack");
const { version } = readJSON("dist/metadata.json");
// GoReleaser builds in parallel, so artifacts.json lists them in any order.
const binaries = readJSON("dist/artifacts.json")
  .filter((a) => a.type === "Binary")
  .sort((a, b) => a.path.localeCompare(b.path));

rmSync(STAGE, { recursive: true, force: true });

for (const [command, description] of Object.entries(COMMANDS)) {
  // The platform packages first: the command package depends on them.
  const optionalDependencies = {};
  for (const binary of binaries.filter((b) => b.extra.ID === command)) {
    const os = PLATFORMS[binary.goos];
    const cpu = ARCHES[binary.goarch];
    if (!os || !cpu) throw new Error(`${binary.path}: no npm os/cpu for ${binary.goos}/${binary.goarch}`);

    const name = `${SCOPE}/${command}-${os}-${cpu}`;
    const dir = stage(name, {
      description: `The ${command} binary for ${os}-${cpu}`,
      repository: { type: "git", url: REPOSITORY },
      os: [os],
      cpu: [cpu],
      // Yarn PnP would otherwise leave the binary inside its zip cache.
      preferUnplugged: true,
    });
    copyFileSync(binary.path, join(dir, binary.name));
    // npm packs the mode on disk, and upload-artifact drops it.
    chmodSync(join(dir, binary.name), 0o755);
    release(dir, name);
    optionalDependencies[name] = version;
  }

  const name = `${SCOPE}/${command}`;
  const dir = stage(name, {
    description,
    // The directory makes npm resolve the README's relative links from there.
    repository: { type: "git", url: REPOSITORY, directory: `cmd/${command}` },
    bin: { [command]: "bin.js" },
    optionalDependencies,
  });
  copyFileSync("npm/bin.js", join(dir, "bin.js"));
  copyFileSync(`cmd/${command}/README.md`, join(dir, "README.md"));
  release(dir, name);
}

// stage writes the package.json and LICENSE of package name into its own
// directory under STAGE, and returns that directory.
function stage(name, fields) {
  const dir = join(STAGE, name.slice(SCOPE.length + 1));
  mkdirSync(dir, { recursive: true });
  writeFileSync(join(dir, "package.json"), `${JSON.stringify({ name, version, license: "MIT", ...fields }, null, 2)}\n`);
  copyFileSync("LICENSE", join(dir, "LICENSE"));
  return dir;
}

// release publishes the package in dir, or packs it under --pack. A version
// already on npm is skipped, so a re-run finishes what a failed run started:
// npm refuses to publish the same version twice.
function release(dir, name) {
  if (pack) {
    execFileSync("npm", ["pack", "--pack-destination", resolve(STAGE)], { cwd: dir, stdio: "inherit" });
    return;
  }
  if (published(name)) {
    console.log(`${name}@${version} is already on npm, skipped`);
    return;
  }
  execFileSync("npm", ["publish", "--access", "public", "--provenance"], { cwd: dir, stdio: "inherit" });
}

function published(name) {
  try {
    const out = execFileSync("npm", ["view", `${name}@${version}`, "version"], {
      encoding: "utf8",
      stdio: ["ignore", "pipe", "ignore"],
    });
    return out.trim() !== "";
  } catch {
    return false; // E404: the package has never been published
  }
}

function readJSON(path) {
  return JSON.parse(readFileSync(path, "utf8"));
}
