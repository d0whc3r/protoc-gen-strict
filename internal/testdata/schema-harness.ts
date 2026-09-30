// What verify/assert.schema.ts (zod 4) and verify-zod3/assert.ts (zod 3)
// share: the messages each type check expects to differ from protobuf-es's JSON
// type, and the helpers that collect and print failures. No package imports:
// each project resolves its own.

import type { Case, Json, Patch } from "./schema-corpus";

// The messages whose Zod input is narrower than their JSON type: a required
// key, a const or in literal, a filtered enum, or a field of such a message.
// Every other message and enum must match both ways, and each one listed
// here must really narrow.
export type Narrowing =
  | "User" // email: required
  | "Money" // currency: required, filtered enum
  | "TimeRange" // start: required
  | "Product" // price: required
  | "CreateProductRequest" // product: required
  | "CreateProductResponse" // Product
  | "GetProductResponse" // Product
  | "ListProductsRequest" // pagination: required; statuses, order_by: filtered
  | "ListProductsResponse" // Product
  | "UpdateProductRequest" // product: required
  | "UpdateProductResponse" // Product
  | "DeleteProductRequest" // id: required
  | "Warehouse" // address: required
  | "StockMovement" // warehouse_id: required; kind: filtered enum
  | "StringRuleCoverage" // enumerated: string.in; constant: string.const
  | "NumericRuleCoverage" // constant: int32.const
  | "ScalarRuleCoverage" // accepted: bool.const; subset, pinned: filtered enums
  | "CollectionRuleCoverage" // flavors: filtered enum; nested: StringRuleCoverage
  | "PresenceRuleCoverage" // required_value, required_message: required
  | "CelRuleCoverage" // nested: StringRuleCoverage
  | "CelNarrowingCoverage" // detail: StringRuleCoverage
  | "StringContentCoverage" // const_value, in_set; nickname: required
  | "ImplicitCoverage" // required keys
  | "BytesCoverage" // required_blob: required
  | "NumberCoverage" // const_value, in_set
  | "EnumCoverage"; // allowed, excluded, pinned: filtered enums

// The messages whose Zod input is wider than their JSON type, on purpose: a
// field with ignore = IGNORE_ALWAYS on a message is z.unknown(), since
// protovalidate never evaluates the message inside. Each one listed must
// really widen.
export type Widening =
  | "AmbiguityCoverage" // unchecked_detail: IGNORE_ALWAYS
  | "TreeNode"; // skipped: IGNORE_ALWAYS

// ---------------------------------------------------------------------------
// Failures are collected and printed as one table; any failure fails the run.

type Failure = { check: string; subject: string; detail: string };
const failures: Failure[] = [];

export function fail(check: string, subject: string, detail: string): void {
  failures.push({ check, subject, detail });
}

/** report prints the failures and throws if there is any. */
export function report(what: string): void {
  if (failures.length > 0) {
    console.log(`\n${failures.length} failure(s):`);
    for (const { check, subject, detail } of failures) {
      console.log(`\n[${check}] ${subject}\n  ${detail.replaceAll("\n", "\n  ")}`);
    }
    throw new Error(`${failures.length} ${what} check(s) failed`);
  }
  console.log(`all ${what} checks passed`);
}

/** Issue is the part of a Zod issue both majors share. */
export type Issue = { code: string; path: PropertyKey[]; message: string; params?: { ruleId?: string } };

export function show(value: unknown): string {
  const text = JSON.stringify(value, (_, v: unknown) => (v === undefined ? "(absent)" : v)) ?? "(absent)";
  return text.length > 90 ? `${text.slice(0, 87)}...` : text;
}

export function pathText(path: readonly PropertyKey[]): string {
  return path.length === 0 ? "(root)" : path.map(String).join(".");
}

export function issuesText(issues: readonly Issue[]): string {
  if (issues.length === 0) return "none";
  return issues.map((i) => `${pathText(i.path)} ${i.code}${i.params?.ruleId ? `[${i.params.ruleId}]` : ""}`).join("; ");
}

// ---------------------------------------------------------------------------
// The differential's inputs, and how its issues compare.

/** merge spreads a patch over a base; a key set to `undefined` is removed. */
export function merge(base: Patch | undefined, patch: Patch): Record<string, Json> {
  const out: Patch = { ...base, ...patch };
  for (const key of Object.keys(out)) {
    if (out[key] === undefined) delete out[key];
  }
  return out as Record<string, Json>;
}

/** inputs expands a case: one input per field value, one per patch. */
export function inputs(c: Case): { label: string; input: Record<string, Json> }[] {
  const fields = Object.entries(c.fields ?? {}).flatMap(([field, values]) =>
    values.map((value) => ({ label: `${field} = ${show(value)}`, input: merge(c.base, { [field]: value }) })),
  );
  const patches = (c.patches ?? []).map((patch) => ({ label: show(patch), input: merge(c.base, patch) }));
  return [...fields, ...patches];
}

export function samePath(a: readonly PropertyKey[], b: readonly PropertyKey[]): boolean {
  return a.length === b.length && a.every((s, i) => s === b[i]);
}

/** issueList is what the .js and .ts modules must agree on, issue by issue. */
export function issueList(issues: readonly Issue[]): string {
  return JSON.stringify(issues.map((i) => [i.path, i.code, i.params?.ruleId ?? null]));
}
