// Runtime checks of protoc-gen-strict-schema's target=json+zod output (zod 4),
// with protovalidate-es as the reference:
//
//   construction   every .zod.ts and .schema.ts imports; every message has its schemas
//   types          z.input<typeof xZod> against protobuf-es's XJson, checked by tsc
//   round trip     toJson(create(XSchema, init)) parses and is valid
//   differential   xZod.safeParse(x).success === fromJson(x) + validate() succeeding
//   composition    .shape, .pick, .omit, .extend, .partial keep the field rules
//   JSON Schema    every loosened bundle compiles (ajv) and accepts the valid values upstream rejects
//   JSON differential  every corpus input protovalidate accepts, ajv accepts
//   fixesTable drift  every fixesTable entry still finds a keyword to drop upstream
//   declarations   each gen-js .d.ts declares what tsc infers for the .ts (tsc)
//   JS as TS       the gen-js .js modules give the same verdicts, issues and schemas
//
// Failures are collected and printed as one table; any failure fails the run.
import {
  create,
  createRegistry,
  fromJson,
  toJson,
  type DescEnum,
  type DescFile,
  type DescMessage,
  type EnumJsonType,
  type JsonValue,
  type MessageJsonType,
} from "@bufbuild/protobuf";
import type { Path } from "@bufbuild/protobuf/reflect";
import {
  file_google_protobuf_any,
  file_google_protobuf_duration,
  file_google_protobuf_empty,
  file_google_protobuf_field_mask,
  file_google_protobuf_struct,
  file_google_protobuf_timestamp,
  file_google_protobuf_wrappers,
} from "@bufbuild/protobuf/wkt";
import { createValidator, type Violation } from "@bufbuild/protovalidate";
import Ajv2020 from "ajv/dist/2020.js";
import addFormats from "ajv-formats";
import { z } from "zod";

import {
  ambiguityBase,
  bytesBase,
  cases,
  collectionBase,
  implicitBase,
  roundTrips,
  stringContentBase,
  type Json,
  type Patch,
} from "../schema-corpus";
import {
  fail,
  inputs,
  issueList,
  issuesText,
  merge,
  pathText,
  report,
  samePath,
  show,
  type Issue,
  type Narrowing,
  type Widening,
} from "../schema-harness";

import * as validatePb from "./gen/typescript/buf/validate/validate_pb";
import * as userPb from "./gen/typescript/example/v1/user_pb";
import * as fieldBehaviorPb from "./gen/typescript/google/api/field_behavior_pb";
import * as httpPb from "./gen/typescript/google/api/http_pb";
import * as productPb from "./gen/typescript/shop/catalog/v1/product_pb";
import * as commonPb from "./gen/typescript/shop/common/v1/common_pb";
import * as rulesPb from "./gen/typescript/shop/coverage/v1/rules_pb";
import * as warehousePb from "./gen/typescript/shop/inventory/v1/warehouse_pb";
import * as schemaPb from "./gen/typescript/shop/schema/v1/schema_pb";

import * as validateZod from "./gen/typescript/buf/validate/validate.zod";
import * as userZod from "./gen/typescript/example/v1/user.zod";
import * as fieldBehaviorZod from "./gen/typescript/google/api/field_behavior.zod";
import * as httpZod from "./gen/typescript/google/api/http.zod";
import * as productZod from "./gen/typescript/shop/catalog/v1/product.zod";
import * as commonZod from "./gen/typescript/shop/common/v1/common.zod";
import * as rulesZod from "./gen/typescript/shop/coverage/v1/rules.zod";
import * as warehouseZod from "./gen/typescript/shop/inventory/v1/warehouse.zod";
import * as schemaZod from "./gen/typescript/shop/schema/v1/schema.zod";

import * as validateJsonSchema from "./gen/typescript/buf/validate/validate.schema";
import * as userJsonSchema from "./gen/typescript/example/v1/user.schema";
import * as httpJsonSchema from "./gen/typescript/google/api/http.schema";
import * as productJsonSchema from "./gen/typescript/shop/catalog/v1/product.schema";
import * as commonJsonSchema from "./gen/typescript/shop/common/v1/common.schema";
import * as rulesJsonSchema from "./gen/typescript/shop/coverage/v1/rules.schema";
import * as warehouseJsonSchema from "./gen/typescript/shop/inventory/v1/warehouse.schema";
import * as schemaJsonSchema from "./gen/typescript/shop/schema/v1/schema.schema";
import { fixesTable, loosenWith, type Fixes } from "./gen/typescript/strict/jsonschema";

// The same modules as JavaScript and declarations (buf.gen.js.yaml).
import * as validatePbJs from "./gen-js/buf/validate/validate_pb.js";
import * as userPbJs from "./gen-js/example/v1/user_pb.js";
import * as fieldBehaviorPbJs from "./gen-js/google/api/field_behavior_pb.js";
import * as httpPbJs from "./gen-js/google/api/http_pb.js";
import * as productPbJs from "./gen-js/shop/catalog/v1/product_pb.js";
import * as commonPbJs from "./gen-js/shop/common/v1/common_pb.js";
import * as rulesPbJs from "./gen-js/shop/coverage/v1/rules_pb.js";
import * as warehousePbJs from "./gen-js/shop/inventory/v1/warehouse_pb.js";
import * as schemaPbJs from "./gen-js/shop/schema/v1/schema_pb.js";

import * as validateZodJs from "./gen-js/buf/validate/validate.zod.js";
import * as userZodJs from "./gen-js/example/v1/user.zod.js";
import * as fieldBehaviorZodJs from "./gen-js/google/api/field_behavior.zod.js";
import * as httpZodJs from "./gen-js/google/api/http.zod.js";
import * as productZodJs from "./gen-js/shop/catalog/v1/product.zod.js";
import * as commonZodJs from "./gen-js/shop/common/v1/common.zod.js";
import * as rulesZodJs from "./gen-js/shop/coverage/v1/rules.zod.js";
import * as warehouseZodJs from "./gen-js/shop/inventory/v1/warehouse.zod.js";
import * as schemaZodJs from "./gen-js/shop/schema/v1/schema.zod.js";

import * as validateJsonSchemaJs from "./gen-js/buf/validate/validate.schema.js";
import * as userJsonSchemaJs from "./gen-js/example/v1/user.schema.js";
import * as httpJsonSchemaJs from "./gen-js/google/api/http.schema.js";
import * as productJsonSchemaJs from "./gen-js/shop/catalog/v1/product.schema.js";
import * as commonJsonSchemaJs from "./gen-js/shop/common/v1/common.schema.js";
import * as rulesJsonSchemaJs from "./gen-js/shop/coverage/v1/rules.schema.js";
import * as warehouseJsonSchemaJs from "./gen-js/shop/inventory/v1/warehouse.schema.js";
import * as schemaJsonSchemaJs from "./gen-js/shop/schema/v1/schema.schema.js";

import type * as jsonschemaDts from "./gen-js/strict/jsonschema.js";
import type * as protovalidateDts from "./gen-js/strict/protovalidate.js";
import type * as wktDts from "./gen-js/strict/wkt.zod.js";
import type * as jsonschemaTs from "./gen/typescript/strict/jsonschema";
import type * as protovalidateTs from "./gen/typescript/strict/protovalidate";
import type * as wktTs from "./gen/typescript/strict/wkt.zod";

// The same descriptors from protoc-gen-es with json_types=true: their
// GenMessage types carry the <Message>Json shapes.
import type * as userJson from "./gen/esjson/example/v1/user_pb";
import type * as productJson from "./gen/esjson/shop/catalog/v1/product_pb";
import type * as commonJson from "./gen/esjson/shop/common/v1/common_pb";
import type * as rulesJson from "./gen/esjson/shop/coverage/v1/rules_pb";
import type * as warehouseJson from "./gen/esjson/shop/inventory/v1/warehouse_pb";
import type * as schemaJson from "./gen/esjson/shop/schema/v1/schema_pb";

// ---------------------------------------------------------------------------
// Types: checked by `tsc --noEmit`, nothing runs.
//
// Forward names every message or enum of a descriptor module whose Zod input
// is not assignable to protobuf-es's JSON type; Reverse the other way round.
// None<T> fails to compile unless T is never, and the error names the culprit.

type Assignable<A, B> = [A] extends [B] ? true : false;
type JsonOf<D> = D extends DescMessage ? MessageJsonType<D> : D extends DescEnum ? EnumJsonType<D> : never;
type InputOf<Z> = Z extends z.ZodType ? z.input<Z> : never;
type None<T extends never> = T;

// ValueName is the stem a type's schemas are named with: protobuf-es's name in
// camelCase, e.g. Warehouse_Address -> warehouseAddress.
type Fold<N extends string> = N extends `${infer A}_${infer B}` ? `${A}${Capitalize<Fold<B>>}` : N;
type ValueName<N extends string> = Uncapitalize<Fold<N>>;

type Forward<P, Z> = {
  [K in keyof P]: K extends `${infer N}Schema`
    ? `${ValueName<N>}Zod` extends keyof Z
      ? Assignable<InputOf<Z[`${ValueName<N>}Zod`]>, JsonOf<P[K]>> extends true
        ? never
        : N
      : `${N}: no ${ValueName<N>}Zod`
    : never;
}[keyof P];

type Reverse<P, Z> = {
  [K in keyof P]: K extends `${infer N}Schema`
    ? `${ValueName<N>}Zod` extends keyof Z
      ? Assignable<JsonOf<P[K]>, InputOf<Z[`${ValueName<N>}Zod`]>> extends true
        ? never
        : N
      : `${N}: no ${ValueName<N>}Zod`
    : never;
}[keyof P];

export type ForwardUser = None<Forward<typeof userJson, typeof userZod>>;
export type ForwardCommon = None<Forward<typeof commonJson, typeof commonZod>>;
export type ForwardProduct = None<Forward<typeof productJson, typeof productZod>>;
export type ForwardWarehouse = None<Forward<typeof warehouseJson, typeof warehouseZod>>;
export type ForwardRules = None<Exclude<Forward<typeof rulesJson, typeof rulesZod>, Widening>>;
export type ForwardSchema = None<Exclude<Forward<typeof schemaJson, typeof schemaZod>, Widening>>;
export type ReallyWidening = None<Exclude<Widening, Forward<typeof rulesJson, typeof rulesZod> | Forward<typeof schemaJson, typeof schemaZod>>>;

type ReverseAll =
  | Reverse<typeof userJson, typeof userZod>
  | Reverse<typeof commonJson, typeof commonZod>
  | Reverse<typeof productJson, typeof productZod>
  | Reverse<typeof warehouseJson, typeof warehouseZod>
  | Reverse<typeof rulesJson, typeof rulesZod>
  | Reverse<typeof schemaJson, typeof schemaZod>;


export type ReverseRuleFree = None<Exclude<ReverseAll, Narrowing>>;
export type ReallyNarrowing = None<Exclude<Narrowing, ReverseAll>>;

// Declarations: every export of a gen-js .d.ts has exactly the type tsc infers
// for the same export of the gen/typescript .ts: identical, and assignable
// both ways. Declared names names every export whose types differ, and every
// name only one of the two modules exports.
type Same<A, B> = (<T>() => T extends A ? 1 : 2) extends (<T>() => T extends B ? 1 : 2) ? true : false;

type Declared<D, T> = {
  [K in keyof D | keyof T]: K extends keyof D
    ? K extends keyof T
      ? [Same<D[K], T[K]>, Assignable<D[K], T[K]>, Assignable<T[K], D[K]>] extends [true, true, true]
        ? never
        : K
      : `${K & string}: only in the .d.ts`
    : `${K & string}: only in the .ts`;
}[keyof D | keyof T];

export type DeclaredValidateZod = None<Declared<typeof validateZodJs, typeof validateZod>>;
export type DeclaredUserZod = None<Declared<typeof userZodJs, typeof userZod>>;
export type DeclaredFieldBehaviorZod = None<Declared<typeof fieldBehaviorZodJs, typeof fieldBehaviorZod>>;
export type DeclaredHttpZod = None<Declared<typeof httpZodJs, typeof httpZod>>;
export type DeclaredProductZod = None<Declared<typeof productZodJs, typeof productZod>>;
export type DeclaredCommonZod = None<Declared<typeof commonZodJs, typeof commonZod>>;
export type DeclaredRulesZod = None<Declared<typeof rulesZodJs, typeof rulesZod>>;
export type DeclaredWarehouseZod = None<Declared<typeof warehouseZodJs, typeof warehouseZod>>;
export type DeclaredSchemaZod = None<Declared<typeof schemaZodJs, typeof schemaZod>>;

export type DeclaredValidateSchema = None<Declared<typeof validateJsonSchemaJs, typeof validateJsonSchema>>;
export type DeclaredUserSchema = None<Declared<typeof userJsonSchemaJs, typeof userJsonSchema>>;
export type DeclaredHttpSchema = None<Declared<typeof httpJsonSchemaJs, typeof httpJsonSchema>>;
export type DeclaredProductSchema = None<Declared<typeof productJsonSchemaJs, typeof productJsonSchema>>;
export type DeclaredCommonSchema = None<Declared<typeof commonJsonSchemaJs, typeof commonJsonSchema>>;
export type DeclaredRulesSchema = None<Declared<typeof rulesJsonSchemaJs, typeof rulesJsonSchema>>;
export type DeclaredWarehouseSchema = None<Declared<typeof warehouseJsonSchemaJs, typeof warehouseJsonSchema>>;
export type DeclaredSchemaSchema = None<Declared<typeof schemaJsonSchemaJs, typeof schemaJsonSchema>>;

export type DeclaredJsonschema = None<Declared<typeof jsonschemaDts, typeof jsonschemaTs>>;
export type DeclaredProtovalidate = None<Declared<typeof protovalidateDts, typeof protovalidateTs>>;
export type DeclaredWkt = None<Declared<typeof wktDts, typeof wktTs>>;

// The type exports, which a namespace's typeof does not list: only
// strict/jsonschema has any.
export type DeclaredJsonschemaTypes = None<
  | (Same<jsonschemaDts.Fix, jsonschemaTs.Fix> extends true ? never : "Fix")
  | (Same<jsonschemaDts.FieldFixes, jsonschemaTs.FieldFixes> extends true ? never : "FieldFixes")
  | (Same<jsonschemaDts.Fixes, jsonschemaTs.Fixes> extends true ? never : "Fixes")
>;

// The oracle: protobuf-es decodes the JSON, protovalidate-es validates it.
// Its registry holds every fixture file and the well-known types, so an Any
// packing any of them decodes.
const pbModules = [validatePb, fieldBehaviorPb, httpPb, userPb, productPb, commonPb, rulesPb, warehousePb, schemaPb];
const fixtureFiles = pbModules.flatMap((m) => Object.entries(m).filter(([k]) => k.startsWith("file_")).map(([, f]) => f as DescFile));
const registry = createRegistry(
  ...fixtureFiles,
  file_google_protobuf_any,
  file_google_protobuf_duration,
  file_google_protobuf_empty,
  file_google_protobuf_field_mask,
  file_google_protobuf_struct,
  file_google_protobuf_timestamp,
  file_google_protobuf_wrappers,
);
const validator = createValidator({ registry });

type Verdict = { valid: boolean; decoded: boolean; violations: readonly Violation[]; note: string };

function oracle(desc: DescMessage, json: unknown): Verdict {
  let msg;
  try {
    msg = fromJson(desc, json as JsonValue, { registry });
  } catch (e) {
    return { valid: false, decoded: false, violations: [], note: `fromJson: ${e instanceof Error ? e.message : String(e)}` };
  }

  const result = validator.validate(desc, msg);
  if (result.kind === "error") return { valid: false, decoded: true, violations: [], note: `error: ${result.error.message}` };
  if (result.kind === "valid") return { valid: true, decoded: true, violations: [], note: "valid" };

  const note = result.violations.map((v) => `${pathText(jsonPath(v.field))} [${v.ruleId}]`).join("; ");
  return { valid: false, decoded: true, violations: result.violations, note };
}

// jsonPath spells a violation's field path the way the JSON is written: JSON
// names, list indexes as numbers, map keys as strings. A oneof has no key.
function jsonPath(path: Path): (string | number)[] {
  return path.flatMap((s): (string | number)[] => {
    switch (s.kind) {
      case "list_sub":
        return [s.index];
      case "map_sub":
        return [String(s.key)];
      case "oneof":
        return [];
      default:
        return [s.jsonName];
    }
  });
}

// ---------------------------------------------------------------------------
// Construction: every message of every generated file has <M>Zod and
// <M>ZodObject, every enum <E>Zod, and every shape entry resolves.

type Entry = { desc: DescMessage; zod: z.ZodType; object: z.ZodObject };

// valueName is ValueName at runtime.
function valueName(name: string): string {
  const folded = name.replace(/_+(.?)/g, (_, c: string) => c.toUpperCase());
  return folded.charAt(0).toLowerCase() + folded.slice(1);
}

function construct(tree: string, modules: [pb: object, zod: object][]): Map<string, Entry> {
  const table = new Map<string, Entry>();

  for (const [pb, zod] of modules) {
    const exports = zod as Record<string, unknown>;

    for (const [key, desc] of Object.entries(pb) as [string, DescMessage | DescEnum][]) {
      if (!key.endsWith("Schema") || (desc.kind !== "message" && desc.kind !== "enum")) continue;
      const name = valueName(key.slice(0, -"Schema".length));

      const schema = exports[`${name}Zod`];
      if (!(schema instanceof z.ZodType)) {
        fail("construction", `${tree} ${desc.typeName}`, `no ${name}Zod`);
        continue;
      }
      if (desc.kind === "enum") continue;

      const object = exports[`${name}ZodObject`];
      if (!(object instanceof z.ZodObject)) {
        fail("construction", `${tree} ${desc.typeName}`, `no ${name}ZodObject`);
        continue;
      }
      for (const [field, part] of Object.entries(object.shape)) {
        if (!(part instanceof z.ZodType)) fail("construction", `${tree} ${desc.typeName}`, `${name}ZodObject.shape.${field} is not a schema`);
      }
      table.set(desc.typeName, { desc, zod: schema, object });
    }
  }
  return table;
}

const messages = construct("ts", [
  [validatePb, validateZod],
  [fieldBehaviorPb, fieldBehaviorZod],
  [httpPb, httpZod],
  [userPb, userZod],
  [productPb, productZod],
  [commonPb, commonZod],
  [rulesPb, rulesZod],
  [warehousePb, warehouseZod],
  [schemaPb, schemaZod],
]);

const jsMessages = construct("js", [
  [validatePbJs, validateZodJs],
  [fieldBehaviorPbJs, fieldBehaviorZodJs],
  [httpPbJs, httpZodJs],
  [userPbJs, userZodJs],
  [productPbJs, productZodJs],
  [commonPbJs, commonZodJs],
  [rulesPbJs, rulesZodJs],
  [warehousePbJs, warehouseZodJs],
  [schemaPbJs, schemaZodJs],
]);
if (jsMessages.size !== messages.size) fail("construction", "js", `${jsMessages.size} messages, the .ts tree has ${messages.size}`);

function lookup(type: string): Entry | undefined {
  const entry = messages.get(type);
  if (entry === undefined) fail("corpus", type, "no such message");
  return entry;
}

// ---------------------------------------------------------------------------
// Round trip: what protobuf-es writes, Zod accepts.

for (const { type, init } of roundTrips) {
  const entry = lookup(type);
  if (entry === undefined) continue;

  let json: JsonValue;
  try {
    json = toJson(entry.desc, create(entry.desc, init as never), { registry });
  } catch (e) {
    fail("round trip", type, `create/toJson: ${String(e)}`);
    continue;
  }

  const parsed = entry.zod.safeParse(json);
  const verdict = oracle(entry.desc, json);
  if (parsed.success && verdict.valid) continue;

  const issues = parsed.success ? [] : (parsed.error.issues as Issue[]);
  fail("round trip", `${type} ${show(json)}`, `zod: ${issuesText(issues)}\nprotovalidate: ${verdict.note}`);
}

// ---------------------------------------------------------------------------
// Differential: Zod and protovalidate agree on every corpus input. Where both
// reject a well-formed input, every violation has a Zod issue at its path:
// native, or custom with the same ruleId. An aborting issue (wrong type,
// missing key, value outside a literal or enum) makes Zod skip refinements,
// so those inputs are compared on the verdict only.

const aborting = new Set(["invalid_type", "invalid_value", "invalid_union", "invalid_key", "invalid_element", "unrecognized_keys"]);

let differentialTotal = 0;
let differentialValid = 0;
let pathsCompared = 0;
let jsCompared = 0;

for (const c of cases) {
  const entry = lookup(c.type);
  if (entry === undefined) continue;
  const js = jsMessages.get(c.type);

  for (const { label, input } of inputs(c)) {
    differentialTotal++;
    const parsed = entry.zod.safeParse(input);
    const verdict = oracle(entry.desc, input);
    const issues = parsed.success ? [] : (parsed.error.issues as Issue[]);
    if (verdict.valid) differentialValid++;

    // The .js module: the same verdict, the same issues in the same order.
    if (js === undefined) {
      fail("JS as TS", c.type, "no .js schema");
    } else {
      jsCompared++;
      const jsParsed = js.zod.safeParse(input);
      const jsIssues = jsParsed.success ? [] : (jsParsed.error.issues as Issue[]);
      if (jsParsed.success !== parsed.success || issueList(jsIssues) !== issueList(issues)) {
        fail("JS as TS", `${c.type} ${label}`, `.ts: ${parsed.success ? "accepts" : issuesText(issues)}\n.js: ${jsParsed.success ? "accepts" : issuesText(jsIssues)}`);
      }
    }

    if (parsed.success !== verdict.valid) {
      const zodSays = parsed.success ? "accepts" : "rejects";
      const pvSays = verdict.valid ? "accepts" : "rejects";
      fail("differential", `${c.type} ${label}`, `zod ${zodSays}, protovalidate ${pvSays}\nzod: ${issuesText(issues)}\nprotovalidate: ${verdict.note}`);
      continue;
    }
    // A decode failure is reported once, by the refinement nearest the value.
    const decodeFailures = issues.filter((i) => i.code === "custom" && i.params?.ruleId === undefined).map((i) => i.message);
    if (new Set(decodeFailures).size !== decodeFailures.length) {
      fail("differential duplicate", `${c.type} ${label}`, `zod: ${issuesText(issues)}`);
    }

    if (verdict.valid || !verdict.decoded || issues.some((i) => aborting.has(i.code))) continue;

    for (const v of verdict.violations) {
      pathsCompared++;
      const path = jsonPath(v.field);
      const matched = issues.some((i) => samePath(i.path, path) && (i.code !== "custom" || i.params?.ruleId === v.ruleId));
      if (!matched) {
        fail("differential path", `${c.type} ${label}`, `no Zod issue at ${pathText(path)} for [${v.ruleId}]\nzod: ${issuesText(issues)}`);
      }
    }
  }
}

// ---------------------------------------------------------------------------
// Composition: the parts of a <M>ZodObject keep validating on their own.

type Parser = { safeParse(input: unknown): { success: boolean; error?: { issues: unknown[] } } };

function accepts(subject: string, schema: Parser, input: unknown): void {
  const result = schema.safeParse(input);
  if (!result.success) fail("composition", `${subject} ${show(input)}`, `rejected: ${issuesText((result.error?.issues ?? []) as Issue[])}`);
}

// rejects expects an issue at `path`; with a ruleId, a custom one carrying it.
function rejects(subject: string, schema: Parser, input: unknown, path: PropertyKey[], ruleId?: string): void {
  const result = schema.safeParse(input);
  const issues = (result.error?.issues ?? []) as Issue[];
  const hit = issues.some((i) => samePath(i.path, path) && (ruleId === undefined || i.params?.ruleId === ruleId));
  if (!hit) fail("composition", `${subject} ${show(input)}`, `expected an issue at ${pathText(path)}${ruleId ? ` [${ruleId}]` : ""}, got: ${issuesText(issues)}`);
}

function throws(subject: string, run: () => unknown): void {
  try {
    run();
  } catch {
    return;
  }
  fail("composition", subject, "did not throw");
}

{
  const { formatCoverageZodObject, stringContentCoverageZodObject, implicitCoverageZodObject, treeNodeZodObject } = schemaZod;
  const { userZodObject } = userZod;

  const host = z.strictObject({ host: formatCoverageZodObject.shape.hostname });
  rejects("shape.hostname", host, { host: "-bad" }, ["host"], "string.hostname");
  accepts("shape.hostname", host, { host: "example.com." });

  const picked = userZodObject.pick({ id: true, email: true });
  rejects("pick", picked, { id: "abc_12345", email: "a@b.io" }, ["id"], "user.id.prefix");
  rejects("pick", picked, { id: "usr", email: "a@b.io" }, ["id"]);
  rejects("pick", picked, { id: "usr_12345", email: "a..b@" }, ["email"]);
  accepts("pick", picked, { id: "usr_12345", email: "a@b.io" });

  const omitted = stringContentCoverageZodObject.omit({ nickname: true });
  accepts("omit", omitted, {});
  rejects("omit", omitted, { code: "a b" }, ["code"], "string_content.code.no_spaces");
  rejects("omit", omitted, { minLen: "😀" }, ["minLen"]);

  const extended = formatCoverageZodObject.extend({ note: z.string() });
  rejects("extend", extended, { note: "n", ipPrefix: "10.1.0.0/8" }, ["ipPrefix"], "string.ip_prefix");
  accepts("extend", extended, { note: "n", ipPrefix: "10.0.0.128/25" });

  const partial = implicitCoverageZodObject.partial();
  accepts("partial", partial, {});
  rejects("partial", partial, { shortCode: "ab" }, ["shortCode"]);
  rejects("partial", partial, { requiredCount: 0 }, ["requiredCount"], "required");

  const kids = z.strictObject({ kids: treeNodeZodObject.shape.children });
  rejects("recursive shape", kids, { kids: [{}] }, ["kids", 0, "label"], "string.min_len");
  accepts("recursive shape", kids, { kids: [{ label: "x", children: [{ label: "y" }] }] });

  // A violation below a cycle, which protovalidate-es misses (schema-corpus.ts).
  rejects("cycle", schemaZod.pingZod, { pong: { ping: { note: "x".repeat(141) } } }, ["pong", "ping", "note"]);
  rejects("cycle", schemaZod.pongZod, { ping: { note: "x".repeat(141) } }, ["ping", "note"]);

  // An Any of a type no module registered cannot be decoded, so it is not
  // checked: under-narrowing, left to the server. Not in the corpus, since
  // the oracle cannot decode it either.
  accepts("unregistered Any", schemaZod.wellKnownCoverageZod, { detail: { "@type": "type.googleapis.com/foo.Unknown" } });

  // Zod 4 refuses to pick from an object that carries refinements.
  throws("userZod.pick (refined)", () => userZod.userZod.pick({ email: true }));
}

// ---------------------------------------------------------------------------
// JSON Schema: ajv's 2020-12 build with ajv-formats, default options except
// strict: false (protoschema-jsonschema writes annotations ajv does not know);
// the u flag stays on.

type Node = { [keyword: string]: unknown };
type Bundle = { $id: string; $defs: Record<string, Node> };

function compile(schema: object) {
  const ajv = new Ajv2020({ strict: false });
  addFormats(ajv);
  return ajv.compile(schema);
}

// Every <M>JsonSchema compiles. Keyed by message name for the checks below.
const jsonSchemas = new Map<string, Bundle>();
const schemaModules = [validateJsonSchema, httpJsonSchema, userJsonSchema, productJsonSchema, commonJsonSchema, rulesJsonSchema, warehouseJsonSchema, schemaJsonSchema];

for (const ns of schemaModules) {
  for (const [name, schema] of Object.entries(ns) as [string, Bundle][]) {
    if (!name.endsWith("JsonSchema")) continue;
    try {
      compile(schema);
    } catch (e) {
      fail("JSON Schema compile", name, String(e));
    }
    jsonSchemas.set(schema.$id.replace(/\.jsonschema\.bundle\.json$/, ""), schema);
  }
}

// The .js modules export the same schemas, deep-equal: keys sorted, so key
// order does not count.
function canonical(value: unknown): string {
  return JSON.stringify(value, (_, v: unknown) =>
    isNode(v) ? Object.fromEntries(Object.entries(v).sort(([a], [b]) => (a < b ? -1 : a > b ? 1 : 0))) : v,
  );
}

const schemaPairs = [
  [validateJsonSchema, validateJsonSchemaJs],
  [httpJsonSchema, httpJsonSchemaJs],
  [userJsonSchema, userJsonSchemaJs],
  [productJsonSchema, productJsonSchemaJs],
  [commonJsonSchema, commonJsonSchemaJs],
  [rulesJsonSchema, rulesJsonSchemaJs],
  [warehouseJsonSchema, warehouseJsonSchemaJs],
  [schemaJsonSchema, schemaJsonSchemaJs],
] as const;

let jsSchemasCompared = 0;
for (const [ts, js] of schemaPairs) {
  const tsExports = ts as Record<string, unknown>;
  const jsExports = js as Record<string, unknown>;
  for (const name of new Set([...Object.keys(tsExports), ...Object.keys(jsExports)])) {
    jsSchemasCompared++;
    if (canonical(tsExports[name]) !== canonical(jsExports[name])) fail("JS as TS", name, "the .js schema differs from the .ts one");
  }
}

// The raw bundles, as protoschema-jsonschema wrote them.
const raws = new Map<string, Bundle>();
for (const [type, schema] of jsonSchemas) {
  const raw = (await import(`./gen/typescript/jsonschema/${schema.$id}`, { with: { type: "json" } })) as { default: Bundle };
  raws.set(type, raw.default);
}

// The "Valid value it rejects" examples of docs/rule-coverage-schema.md: the
// loosened schema accepts each. For an example with a fix class, removing the
// fix of its field (every other fix applied) makes the bundle reject, or fail to
// compile, at least one example of the class. `def` names a fix on a well-known type's def instead
// of on the field.
type Example = { cls?: string; type: string; field: string; values: Json[]; base?: Patch; def?: string };

const format = "shop.schema.v1.FormatCoverage";
const content = "shop.schema.v1.StringContentCoverage";
const bytes = "shop.schema.v1.BytesCoverage";
const collection = "shop.schema.v1.CollectionCoverage";
const ignore = "shop.schema.v1.IgnoreCoverage";
const wellKnown = "shop.schema.v1.WellKnownCoverage";
const uris = ["mailto:a@b.io", "urn:isbn:1", "https://user@example.com/", "http://[::1]/"];

const examples: Example[] = [
  { cls: "string.hostname pattern", type: format, field: "hostname", values: ["example.com."] },
  { cls: "string.hostname pattern", type: format, field: "address", values: ["example.com."] },
  // jsonschema:hide moves the field into patternProperties.
  { cls: "string.hostname pattern", type: format, field: "hiddenHost", values: ["example.com."] },
  { cls: "string.email format", type: format, field: "email", values: ["user@localhost", "a!b@x.io"] },
  { cls: "string.email format", type: "shop.schema.v1.ImplicitCoverage", field: "email", values: ["user@localhost"], base: implicitBase },
  { cls: "string.ip pattern", type: format, field: "ip", values: ["fe80::1%eth0", "::ffff:192.0.2.1"] },
  { cls: "string.ipv6 format", type: format, field: "ipv6", values: ["fe80::1%eth0"] },
  { cls: "string.uri pattern", type: format, field: "uri", values: uris },
  { cls: "string.uri pattern", type: format, field: "uriRef", values: uris },
  { cls: "string.uri pattern", type: collection, field: "links", values: [uris], base: collectionBase },
  { cls: "string.uri pattern", type: collection, field: "labels", values: [{ ab: "mailto:a@b.io" }], base: collectionBase },
  { cls: "string.host_and_port pattern", type: format, field: "hostAndPort", values: ["example.com:0", "example.com.:80"] },
  { cls: "string.*_with_prefixlen pattern", type: format, field: "ipWithPrefixlen", values: ["::ffff:192.0.2.1/96"] },
  { cls: "string.*_with_prefixlen pattern", type: format, field: "ipv6WithPrefixlen", values: ["::ffff:192.0.2.1/96"] },
  { cls: "string.*_prefix pattern", type: format, field: "ipPrefix", values: ["10.0.0.128/25", "2001:db8:0:0:0:0:0:0/32"] },
  { cls: "string.*_prefix pattern", type: format, field: "ipv4Prefix", values: ["10.0.0.128/25"] },
  { cls: "string.*_prefix pattern", type: format, field: "ipv6Prefix", values: ["2001:db8:0:0:0:0:0:0/32"] },
  { cls: "well_known_regex pattern (u flag)", type: format, field: "headerName", values: ["Content-Type"] },
  { cls: "string.len_bytes maxLength 0", type: content, field: "lenBytes", values: ["abcd", "😀"], base: stringContentBase },
  { cls: "string.pattern (RE2)", type: content, field: "patternInlineFlag", values: ["ABC"], base: stringContentBase },
  { cls: "string.pattern (RE2)", type: content, field: "patternNonSpace", values: ["a\u00a0b"], base: stringContentBase },
  { cls: "string.prefix/suffix pattern", type: content, field: "prefix", values: ["+15551234"], base: stringContentBase },
  { cls: "string.prefix/suffix pattern", type: content, field: "prefixAndSuffix", values: ["pre\nsuf"], base: stringContentBase },
  // The pattern overwrites upstream's uuid pattern; both are left to runtime.
  { type: content, field: "uuidPattern", values: ["123e4567-e89b-12d3-a456-426614174000"], base: stringContentBase },
  // required on a field with presence: upstream writes no minLength here.
  { type: content, field: "nickname", values: [""] },
  { type: bytes, field: "requiredBlob", values: [""] },
  { cls: "bytes base64 pattern", type: bytes, field: "raw", values: ["-_8"], base: bytesBase },
  { cls: "bytes base64 pattern", type: bytes, field: "exactLen", values: ["-_-_-w"], base: bytesBase },
  { cls: "bytes base64 pattern", type: bytes, field: "requiredBlob", values: ["-_8"] },
  { cls: "bytes base64 pattern", type: wellKnown, field: "blob", values: ["-_8"], def: "google.protobuf.BytesValue.jsonschema.json" },
  { cls: "google.protobuf.Duration format", type: wellKnown, field: "timeout", values: ["1.5s"], def: "google.protobuf.Duration.jsonschema.json" },
  { cls: "map<bool, _> propertyNames", type: collection, field: "flags", values: [{ true: "x", false: "y" }], base: collectionBase },
  { cls: "ignore = IGNORE_IF_ZERO_VALUE", type: ignore, field: "email", values: [""] },
  { cls: "ignore = IGNORE_IF_ZERO_VALUE", type: ignore, field: "threshold", values: [0] },
  { cls: "ignore = IGNORE_IF_ZERO_VALUE", type: ignore, field: "ids", values: [[""]] },
  { cls: "ignore = IGNORE_IF_ZERO_VALUE", type: ignore, field: "codes", values: [{ a: "" }] },
  // A float's JSON is the float32 widened to a double: 0.1f is 0.10000000149011612.
  { cls: "float bounds", type: "shop.schema.v1.NumberCoverage", field: "capped", values: [0.10000000149011612] },
  // IGNORE_ALWAYS on a message field skips the nested message too.
  { type: "shop.coverage.v1.AmbiguityCoverage", field: "uncheckedDetail", values: [{ exactLen: "x" }], base: ambiguityBase },
];

// without returns fixesTable minus one field's (or one def's) entry.
function without(def: string, field: string): Fixes {
  const { [field]: _, ...rest } = fixesTable[def] ?? {};
  return { ...fixesTable, [def]: rest };
}

const fixMatters = new Map<string, boolean>();
for (const { cls, type, field, values, base, def } of examples) {
  const entry = lookup(type);
  const loosened = jsonSchemas.get(type);
  const raw = raws.get(type);
  if (entry === undefined || loosened === undefined || raw === undefined) {
    fail("JSON Schema", type, "no JSON Schema for the message");
    continue;
  }

  const accept = compile(loosened);
  const fixes = def === undefined ? without(`${type}.jsonschema.json`, field) : without(def, "");
  let unfixed: ((input: unknown) => boolean) | undefined;
  try {
    unfixed = compile(loosenWith(fixes, raw));
  } catch {
    unfixed = undefined; // BREAKS: the bundle does not compile without the fix
  }
  if (cls !== undefined && !fixMatters.has(cls)) fixMatters.set(cls, false);

  for (const value of values) {
    const input = merge(base, { [field]: value });
    const subject = `${type} ${field} = ${show(value)}`;

    const verdict = oracle(entry.desc, input);
    if (!verdict.valid) fail("JSON Schema example", subject, `not valid under protovalidate: ${verdict.note}`);
    if (!accept(input)) fail("JSON Schema valid value", subject, `the loosened schema rejects it: ${JSON.stringify(accept.errors)}`);
    if (cls !== undefined && (unfixed === undefined || !unfixed(input))) fixMatters.set(cls, true);
  }
}

for (const [cls, matters] of fixMatters) {
  if (!matters) fail("JSON Schema fix matters", cls, "without its fix the bundle still accepts every example");
}

// ---------------------------------------------------------------------------
// JSON Schema differential: the whole corpus through the loosened schema of
// the message each case targets. What protovalidate accepts, ajv must accept;
// the converse is expected, the schema being looser by design, and only
// counted. Nested messages have no bundle and are skipped, and so is an input
// holding null for a NullValue field: the deliberate null rejection.

function nullValueNull(desc: DescMessage, input: Record<string, Json>): boolean {
  return desc.fields.some((f) => f.fieldKind === "enum" && f.enum.typeName === "google.protobuf.NullValue" && input[f.jsonName] === null);
}

const jsonValidators = new Map<string, ReturnType<typeof compile>>();
let jsonTotal = 0;
let jsonLooser = 0;

for (const c of cases) {
  const entry = messages.get(c.type);
  const loosened = jsonSchemas.get(c.type);
  if (entry === undefined || loosened === undefined) continue;

  let validate = jsonValidators.get(c.type);
  if (validate === undefined) {
    validate = compile(loosened);
    jsonValidators.set(c.type, validate);
  }

  for (const { label, input } of inputs(c)) {
    if (nullValueNull(entry.desc, input)) continue;
    jsonTotal++;
    const accepted = validate(input);
    const verdict = oracle(entry.desc, input);
    if (verdict.valid && !accepted) fail("JSON Schema differential", `${c.type} ${label}`, `ajv rejects a protovalidate-valid value: ${JSON.stringify(validate.errors)}`);
    if (!verdict.valid && accepted) jsonLooser++;
  }
}

// ---------------------------------------------------------------------------
// fixesTable drift: each fix finds, in at least one raw bundle, its def and field
// node (properties[jsonName] or a patternProperties entry naming it, then
// `at`) holding one of its keywords, directly or in an anyOf branch.

function isNode(value: unknown): value is Node {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function holds(node: unknown, keywords: readonly string[]): boolean {
  if (!isNode(node)) return false;
  if (keywords.some((k) => k in node)) return true;
  return Array.isArray(node.anyOf) && node.anyOf.some((branch) => holds(branch, keywords));
}

// fieldNodes finds the field where the runtime's loosen does: properties[name],
// and every patternProperties key `^(a|b|…)$` whose alternatives include the
// JSON name or the alias.
function fieldNodes(def: Node, name: string, alias: string | undefined): unknown[] {
  if (name === "") return [def];
  const properties = isNode(def.properties) ? def.properties : {};
  const patterns = isNode(def.patternProperties) ? def.patternProperties : {};
  const aliased = Object.entries(patterns).filter(([key]) => {
    const names = /^\^\((.*)\)\$$/.exec(key)?.[1]?.split("|") ?? [];
    return names.includes(name) || (alias !== undefined && names.includes(alias));
  });
  return [properties[name], ...aliased.map(([, node]) => node)];
}

let fixCount = 0;
for (const [def, fields] of Object.entries(fixesTable)) {
  const holders = [...raws.values()].map((b) => b.$defs[def]).filter(isNode);
  if (holders.length === 0) {
    fail("fixesTable drift", def, "no raw bundle has this def");
    continue;
  }

  for (const [name, { alias, fixes }] of Object.entries(fields)) {
    for (const fix of fixes) {
      fixCount++;
      const hit = holders.some((d) => fieldNodes(d, name, alias).some((node) => holds(fix.at === undefined ? node : isNode(node) ? node[fix.at] : undefined, fix.drop)));
      if (!hit) fail("fixesTable drift", `${def} ${name || "(def)"}`, `no ${fix.at ?? "field node"} holds any of ${fix.drop.join(", ")}`);
    }
  }
}

// ---------------------------------------------------------------------------
// Report

console.log(`construction: ${messages.size} messages (.ts), ${jsMessages.size} (.js)`);
console.log(`round trip: ${roundTrips.length} messages`);
console.log(`differential: ${differentialTotal} inputs, ${differentialValid} valid under protovalidate, ${pathsCompared} violation paths compared`);
console.log(`JSON Schema: ${jsonSchemas.size} schemas, ${fixMatters.size} fix classes, ${fixCount} fixes`);
console.log(`JSON Schema differential: ${jsonTotal} inputs, ${jsonLooser} invalid ones ajv accepts (looser by design)`);
console.log(`JS as TS: ${jsCompared} inputs, ${jsSchemasCompared} JSON Schemas compared`);

report("schema");
