// Runtime checks of protoc-gen-strict-schema's target=zod3 output (zod 3), with
// protovalidate-es as the reference:
//
//   construction   every .zod3.ts imports; every message has its schemas
//   types          z.input<typeof xZod> against protobuf-es's XJson, checked by tsc
//   round trip     toJson(create(XSchema, init)) parses and is valid
//   differential   xZod.safeParse(x).success === fromJson(x) + validate() succeeding
//   composition    .shape, .pick, .omit, .extend, .partial keep the field rules
//   declarations   each gen-js .d.ts declares what tsc infers for the .ts (tsc)
//   JS as TS       the gen-js .js modules give the same verdicts and issues
//
// The JSON Schema checks live in ../verify/assert.schema.ts. Failures are
// collected and printed as one table; any failure fails the run.
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
import { z } from "zod";

import { cases, roundTrips, type Json } from "../schema-corpus";
import {
  fail,
  inputs,
  issueList,
  issuesText,
  pathText,
  report,
  samePath,
  show,
  type Issue,
  type Narrowing,
  type Widening,
} from "../schema-harness";

// protoc-gen-es ran with json_types=true here: the descriptors' types carry
// the <Message>Json shapes.
import * as validatePb from "./gen/typescript/buf/validate/validate_pb";
import * as userPb from "./gen/typescript/example/v1/user_pb";
import * as fieldBehaviorPb from "./gen/typescript/google/api/field_behavior_pb";
import * as httpPb from "./gen/typescript/google/api/http_pb";
import * as productPb from "./gen/typescript/shop/catalog/v1/product_pb";
import * as commonPb from "./gen/typescript/shop/common/v1/common_pb";
import * as rulesPb from "./gen/typescript/shop/coverage/v1/rules_pb";
import * as warehousePb from "./gen/typescript/shop/inventory/v1/warehouse_pb";
import * as schemaPb from "./gen/typescript/shop/schema/v1/schema_pb";

import * as validateZod from "./gen/typescript/buf/validate/validate.zod3";
import * as userZod from "./gen/typescript/example/v1/user.zod3";
import * as fieldBehaviorZod from "./gen/typescript/google/api/field_behavior.zod3";
import * as httpZod from "./gen/typescript/google/api/http.zod3";
import * as productZod from "./gen/typescript/shop/catalog/v1/product.zod3";
import * as commonZod from "./gen/typescript/shop/common/v1/common.zod3";
import * as rulesZod from "./gen/typescript/shop/coverage/v1/rules.zod3";
import * as warehouseZod from "./gen/typescript/shop/inventory/v1/warehouse.zod3";
import * as schemaZod from "./gen/typescript/shop/schema/v1/schema.zod3";

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

import * as validateZodJs from "./gen-js/buf/validate/validate.zod3.js";
import * as userZodJs from "./gen-js/example/v1/user.zod3.js";
import * as fieldBehaviorZodJs from "./gen-js/google/api/field_behavior.zod3.js";
import * as httpZodJs from "./gen-js/google/api/http.zod3.js";
import * as productZodJs from "./gen-js/shop/catalog/v1/product.zod3.js";
import * as commonZodJs from "./gen-js/shop/common/v1/common.zod3.js";
import * as rulesZodJs from "./gen-js/shop/coverage/v1/rules.zod3.js";
import * as warehouseZodJs from "./gen-js/shop/inventory/v1/warehouse.zod3.js";
import * as schemaZodJs from "./gen-js/shop/schema/v1/schema.zod3.js";

import type * as protovalidateDts from "./gen-js/strict/protovalidate.js";
import type * as wktDts from "./gen-js/strict/wkt.zod3.js";
import type * as protovalidateTs from "./gen/typescript/strict/protovalidate";
import type * as wktTs from "./gen/typescript/strict/wkt.zod3";

// ---------------------------------------------------------------------------
// Types: checked by `tsc --noEmit`, nothing runs.
//
// Forward names every message or enum of a descriptor module whose Zod input
// is not assignable to protobuf-es's JSON type; Reverse the other way round.
// None<T> fails to compile unless T is never, and the error names the culprit.

type Assignable<A, B> = [A] extends [B] ? true : false;
type JsonOf<D> = D extends DescMessage ? MessageJsonType<D> : D extends DescEnum ? EnumJsonType<D> : never;
type InputOf<Z> = Z extends z.ZodTypeAny ? z.input<Z> : never;
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

export type ForwardUser = None<Forward<typeof userPb, typeof userZod>>;
export type ForwardCommon = None<Forward<typeof commonPb, typeof commonZod>>;
export type ForwardProduct = None<Forward<typeof productPb, typeof productZod>>;
export type ForwardWarehouse = None<Forward<typeof warehousePb, typeof warehouseZod>>;
export type ForwardRules = None<Exclude<Forward<typeof rulesPb, typeof rulesZod>, Widening>>;
export type ForwardSchema = None<Exclude<Forward<typeof schemaPb, typeof schemaZod>, Widening>>;
export type ReallyWidening = None<Exclude<Widening, Forward<typeof rulesPb, typeof rulesZod> | Forward<typeof schemaPb, typeof schemaZod>>>;

type ReverseAll =
  | Reverse<typeof userPb, typeof userZod>
  | Reverse<typeof commonPb, typeof commonZod>
  | Reverse<typeof productPb, typeof productZod>
  | Reverse<typeof warehousePb, typeof warehouseZod>
  | Reverse<typeof rulesPb, typeof rulesZod>
  | Reverse<typeof schemaPb, typeof schemaZod>;


export type ReverseRuleFree = None<Exclude<ReverseAll, Narrowing>>;
export type ReallyNarrowing = None<Exclude<Narrowing, ReverseAll>>;

// Declarations: every export of a gen-js .d.ts has exactly the type tsc infers
// for the same export of the gen/typescript .ts: identical, and assignable
// both ways. Declared names every export whose types differ, and every name
// only one of the two modules exports.
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
export type DeclaredProtovalidate = None<Declared<typeof protovalidateDts, typeof protovalidateTs>>;
export type DeclaredWkt = None<Declared<typeof wktDts, typeof wktTs>>;

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

type Entry = { desc: DescMessage; zod: z.ZodTypeAny; object: z.AnyZodObject };

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
      for (const [field, part] of Object.entries(object.shape as Record<string, unknown>)) {
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

const aborting = new Set(["invalid_type", "invalid_literal", "invalid_enum_value", "invalid_union", "unrecognized_keys"]);

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
// Composition: the parts of a <M>ZodObject keep validating on their own. In
// Zod 3 a refined <M>Zod is a ZodEffects, so composition starts from
// <M>ZodObject; a recursive message's ZodObject spreads its base object's
// .shape into a z.strictObject, so it has a .shape too.

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

{
  const { formatCoverageZodObject, stringContentCoverageZodObject, implicitCoverageZodObject, treeNodeZodObject } = schemaZod;
  const { userZodObject } = userZod;

  const host = z.strictObject({ host: formatCoverageZodObject.shape.hostname });
  rejects("shape.hostname", host, { host: "-bad" }, ["host"], "string.hostname");
  accepts("shape.hostname", host, { host: "example.com." });

  const picked = userZodObject.pick({ id: true, email: true });
  rejects("pick", picked, { id: "abc_12345", email: "a@b.io" }, ["id"], "user.id.prefix");
  rejects("pick", picked, { id: "usr", email: "a@b.io" }, ["id"], "string.min_len");
  rejects("pick", picked, { id: "usr_12345", email: "a..b@" }, ["email"]);
  accepts("pick", picked, { id: "usr_12345", email: "a@b.io" });

  const omitted = stringContentCoverageZodObject.omit({ nickname: true });
  accepts("omit", omitted, {});
  rejects("omit", omitted, { code: "a b" }, ["code"], "string_content.code.no_spaces");
  rejects("omit", omitted, { minLen: "😀" }, ["minLen"], "string.min_len");

  const extended = formatCoverageZodObject.extend({ note: z.string() });
  rejects("extend", extended, { note: "n", ipPrefix: "10.1.0.0/8" }, ["ipPrefix"], "string.ip_prefix");
  accepts("extend", extended, { note: "n", ipPrefix: "10.0.0.128/25" });

  const partial = implicitCoverageZodObject.partial();
  accepts("partial", partial, {});
  rejects("partial", partial, { shortCode: "ab" }, ["shortCode"], "string.min_len");
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

  // The refined schema is a ZodEffects: no .pick, and its inner type is the
  // composable object.
  if (!(userZod.userZod instanceof z.ZodEffects) || "pick" in userZod.userZod || userZod.userZod.innerType() !== userZodObject) {
    fail("composition", "userZod", "expected a ZodEffects over userZodObject, without .pick");
  }
}

// ---------------------------------------------------------------------------
// Report

console.log(`construction: ${messages.size} messages (.ts), ${jsMessages.size} (.js)`);
console.log(`round trip: ${roundTrips.length} messages`);
console.log(`differential: ${differentialTotal} inputs, ${differentialValid} valid under protovalidate, ${pathsCompared} violation paths compared`);
console.log(`JS as TS: ${jsCompared} inputs compared`);

report("zod3");
