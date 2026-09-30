// Plain Node, no tsx and no bundler: every .js module in gen-js loads through
// Node's own ESM loader, which resolves the `.js` relative imports and the
// JSON import attributes, and a few schemas parse one valid and one invalid
// canonical input. Also checks the declaration files: assert.schema.ts
// compares every generated one with its .ts, and none exports a name its .js
// lacks.
import { readdirSync, readFileSync } from "node:fs";
import Ajv2020 from "ajv/dist/2020.js";
import addFormats from "ajv-formats";

const root = new URL("./gen-js/", import.meta.url);
const failures = [];
const all = readdirSync(root, { recursive: true }).map(String).sort();

// Every module, imported by URL.
const files = all.filter((f) => f.endsWith(".js"));
const modules = {};
for (const file of files) {
  try {
    modules[file] = await import(new URL(file, root).href);
  } catch (e) {
    failures.push(`import ${file}: ${e.message}`);
  }
}

// A declaration file exports every top-level declaration unless it says
// `export {}`: a helper it declares without `export` is then importable in
// TypeScript and missing at runtime.
const harness = readFileSync(new URL("./assert.schema.ts", import.meta.url), "utf8");
for (const file of all.filter((f) => f.endsWith(".d.ts") && !f.endsWith("_pb.d.ts"))) {
  if (!harness.includes(`"./gen-js/${file.replace(/\.d\.ts$/, ".js")}"`)) failures.push(`${file}: not compared with its .ts in assert.schema.ts`);

  const text = readFileSync(new URL(file, root), "utf8");
  const local = [...text.matchAll(/^(?:declare +(?:const|let|var|function|class|enum|namespace) +|type +|interface +)([A-Za-z_$][\w$]*)/gm)].map((m) => m[1]);
  if (local.length > 0 && !/^export \{\};?$/m.test(text)) {
    failures.push(`${file}: exports ${local.join(", ")} for want of \`export {}\``);
  }

  // Every value the declarations export, the .js exports too.
  const js = modules[file.replace(/\.d\.ts$/, ".js")] ?? {};
  const declared = [...text.matchAll(/^export declare (?:const|function) ([A-Za-z_$][\w$]*)/gm)].map((m) => m[1]);
  const missing = declared.filter((name) => !(name in js));
  if (missing.length > 0) failures.push(`${file}: declares ${missing.join(", ")}, which the .js does not export`);
}

const uuid = "123e4567-e89b-12d3-a456-426614174000";

// [module, export, valid input, invalid input]
const zodChecks = [
  ["example/v1/user.zod.js", "userZod", { id: "usr_12345", email: "a@b.io", displayName: "Joe", age: 30, roles: ["admin"] }, { email: "a@b.io" }],
  ["shop/schema/v1/schema.zod.js", "formatCoverageZod", { hostname: "example.com." }, { hostname: "-bad" }],
  ["shop/schema/v1/schema.zod.js", "treeNodeZod", { label: "root", children: [{ label: "c" }] }, { label: "root", children: [{}] }],
  ["shop/schema/v1/schema.zod.js", "wellKnownCoverageZod", { detail: { "@type": "type.googleapis.com/google.protobuf.Duration", value: "1s" } }, { timeout: "0.5s" }],
  ["shop/catalog/v1/product.zod.js", "getProductRequestZod", { id: uuid }, {}],
];

for (const [file, name, valid, invalid] of zodChecks) {
  const schema = modules[file]?.[name];
  if (schema === undefined) {
    failures.push(`${file}: no ${name}`);
    continue;
  }
  if (!schema.safeParse(valid).success) failures.push(`${name} rejects ${JSON.stringify(valid)}`);
  if (schema.safeParse(invalid).success) failures.push(`${name} accepts ${JSON.stringify(invalid)}`);
}

// The loosened JSON Schemas, through ajv.
const jsonChecks = [
  ["shop/schema/v1/schema.schema.js", "formatCoverageJsonSchema", { hostname: "example.com." }, { hostname: 1 }],
  ["shop/schema/v1/schema.schema.js", "wellKnownCoverageJsonSchema", { timeout: "1.5s" }, { timeout: 1 }],
];

for (const [file, name, valid, invalid] of jsonChecks) {
  const schema = modules[file]?.[name];
  if (schema === undefined) {
    failures.push(`${file}: no ${name}`);
    continue;
  }
  const ajv = new Ajv2020({ strict: false });
  addFormats(ajv);
  const validate = ajv.compile(schema);
  if (!validate(valid)) failures.push(`${name} rejects ${JSON.stringify(valid)}: ${JSON.stringify(validate.errors)}`);
  if (validate(invalid)) failures.push(`${name} accepts ${JSON.stringify(invalid)}`);
}

console.log(`plain node: ${Object.keys(modules).length} of ${files.length} modules loaded, ${zodChecks.length + jsonChecks.length} schemas parsed`);
if (failures.length > 0) {
  console.log(failures.join("\n"));
  process.exitCode = 1;
}
