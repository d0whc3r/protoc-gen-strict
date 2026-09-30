// Inputs for the protoc-gen-strict-schema runtime checks, shared by
// verify/assert.schema.ts (zod 4) and verify-zod3/assert.ts (zod 3). Plain
// data, no imports: each project resolves the type names against its own
// generated code.
//
// Every input is canonical protojson, the form protobuf-es toJson writes:
// JSON names, enum names, int64 as decimal strings, bytes as base64. The
// forms the schemas reject on purpose (proto names as keys, enum numbers,
// int32 as a string, int64 as a number, null) are left out.

export type Json = null | boolean | number | string | Json[] | { [key: string]: Json };

/** Value is an input for one key; `undefined` leaves the key out. */
export type Value = Json | undefined;

/** Patch is spread over a base; a key set to `undefined` is removed. */
export type Patch = { [key: string]: Value };

/**
 * Case lists the inputs of one message, `type` its full name, e.g.
 * "shop.schema.v1.FormatCoverage". Each value in `fields` is set on `base`
 * under its JSON name, e.g. `hostAndPort`; each of `patches` is spread over
 * `base`.
 */
export type Case = { type: string; base?: Patch; fields?: { [jsonName: string]: Value[] }; patches?: Patch[] };

/** RoundTrip is a create() init for `type`, valid under protovalidate. */
export type RoundTrip = { type: string; init: { [key: string]: unknown } };

const uuid = "123e4567-e89b-12d3-a456-426614174000";
const tuuid = "123e4567e89b12d3a456426614174000";
const past = "2000-01-01T00:00:00Z";
const future = "3000-01-01T00:00:00Z";
const colors = ["SCHEMA_COLOR_UNSPECIFIED", "SCHEMA_COLOR_RED", "SCHEMA_COLOR_GREEN", "SCHEMA_COLOR_BLUE"];
const anyDuration = { "@type": "type.googleapis.com/google.protobuf.Duration", value: "1s" };

// Valid instances, the bases of the cases that vary one field.

/** ImplicitCoverage with every rule satisfied; each key is needed. */
export const implicitBase: Patch = {
  shortCode: "abc",
  count: 1,
  tags: ["t"],
  email: "a@b.io",
  requiredName: "n",
  requiredCount: 1,
  accepted: true,
  color: "SCHEMA_COLOR_RED",
  requiredTags: ["t"],
  requiredItem: {},
};

/** CollectionCoverage's implicit fields that break a rule when absent. */
export const collectionBase: Patch = { bounded: ["a"], labels: { ab: "https://example.com" } };

export const stringContentBase: Patch = { nickname: "" };

/** AmbiguityCoverage with its message CEL satisfied. */
export const ambiguityBase: Patch = {
  "external-id": "123e4567-e89b-12d3-a456-426614174000",
  constructor: "123e4567-e89b-12d3-a456-426614174000",
  label: "l",
  nickname: "n",
  email: "a@b.io",
  quantity: 10,
};
export const bytesBase: Patch = { requiredBlob: "" };

// A valid shop.coverage.v1.StringRuleCoverage: every field is implicit and
// carries a rule its zero value breaks. Also a create() init: all strings.
const stringRulesNoUuid = {
  exactLen: "abcdefghij",
  lenBounds: "ab",
  byteBounds: "ab",
  pattern: "a_b",
  affixes: "id_a-b_v1",
  enumerated: "alpha",
  constant: "fixed",
  email: "a@b.io",
  hostname: "example.com",
  uri: "https://example.com",
  uriRef: "/a",
  tuuid,
  ipv4: "192.0.2.1",
  ipv6: "::1",
  ip: "::1",
  ipPrefix: "10.0.0.0/8",
  hostAndPort: "example.com:80",
  headerName: "Content-Type",
};
const stringRules = { ...stringRulesNoUuid, uuid };

// A valid shop.coverage.v1.NumericRuleCoverage.
const numericRules = { i32: 1, u64: "1", f32: 1, f64: "1", sf32: -1, sf64: "-1", factor: 1, constant: 42, excluded: 1 };

const userBase: Patch = { id: "usr_12345", email: "a@b.io", displayName: "Joe", age: 30, roles: ["admin"] };
const money = { units: "10", currency: "CURRENCY_EUR" };
const productNoId = { sku: "ABC-123", name: "Widget", price: money };
const productBase = { id: uuid, ...productNoId };
const addressNoLine1 = { city: "Springfield", countryCode: "US", postalCode: "12345" };
const address = { line1: "1 Main St", ...addressNoLine1 };

// The differential corpus. Values come from the rule tables of
// docs/rule-coverage-schema.md, plus the edges of each bound. protovalidate-es
// decides each one, not this file.
export const cases: Case[] = [
  // One well-known format per field.
  {
    type: "shop.schema.v1.FormatCoverage",
    fields: {
      hostname: ["example.com", "example.com.", "-bad", "bad-", "123", "a.b.c", "a..b", "", `${"a".repeat(64)}.com`, "exa_mple.com", "1.2.3.4a"],
      email: ["a@b.io", "user@localhost", "a!b@x.io", "a..b@x.io", "@x.io", "a@", "", "a@-x.io", "a b@x.io"],
      ip: ["192.0.2.1", "::1", "fe80::1%eth0", "::ffff:192.0.2.1", "01.2.3.4", "1.2.3", "", "2001:db8::1", "::ffff:192.0.2.256"],
      ipv4: ["192.0.2.1", "192.0.2.01", "256.0.0.1", "::1", ""],
      ipv6: ["::1", "fe80::1%eth0", "::ffff:192.0.2.1", "192.0.2.1", "1::2::3", ""],
      uri: [
        "https://example.com/a?b#c",
        "mailto:a@b.io",
        "urn:isbn:1",
        "https://user@example.com/",
        "http://[::1]/",
        "foo bar",
        "/relative",
        "",
        "http://[fe80::1%25eth0]/",
        "https://example.com/%zz",
      ],
      uriRef: ["/relative", "./a?b", "mailto:a@b.io", "foo bar", "", "http://[::1]/"],
      address: ["example.com", "example.com.", "192.0.2.1", "::1", "-bad", ""],
      hostAndPort: ["example.com:80", "example.com:0", "example.com:99999", "example.com.:80", "[::1]:443", "::1:443", "example.com", "192.0.2.1:65535", ""],
      ipWithPrefixlen: ["192.0.2.1/24", "::ffff:192.0.2.1/96", "2001:db8::1/64", "192.0.2.1", "192.0.2.1/33"],
      ipv4WithPrefixlen: ["192.0.2.1/24", "192.0.2.01/24", "::1/64"],
      ipv6WithPrefixlen: ["2001:db8::1/64", "::ffff:192.0.2.1/96", "192.0.2.1/24", "::1/129"],
      ipPrefix: ["10.0.0.128/25", "10.1.0.0/8", "10.0.0.0/8", "2001:db8:0:0:0:0:0:0/32", "2001:db8::/32", "2001:db8::1/32", "10.0.0.0"],
      ipv4Prefix: ["10.0.0.128/25", "10.1.0.0/8", "10.0.0.0/8", "2001:db8::/32"],
      ipv6Prefix: ["2001:db8:0:0:0:0:0:0/32", "2001:db8::/32", "2001:db8::1/32", "10.0.0.0/8"],
      uuid: [uuid, uuid.toUpperCase(), tuuid, ""],
      tuuid: [tuuid, uuid, ""],
      ulid: ["01ARZ3NDEKTSV4RRFFQ69G5FAV", "81ARZ3NDEKTSV4RRFFQ69G5FAV", "01ARZ3NDEKTSV4RRFFQ69G5FAI", ""],
      protobufFqn: ["foo.Bar", ".foo.Bar", "foo..Bar", "1foo", ""],
      protobufDotFqn: [".foo.Bar", "foo.Bar", ""],
      headerName: ["Content-Type", ":authority", "a,b", "a b", "", "x\u0000"],
      headerValue: ["text/html", "a\tb", "a\nb", ""],
      hiddenHost: ["example.com", "example.com.", "-bad", ""],
    },
  },

  // Lengths in code points and bytes, RE2 patterns, affixes, sets. "😀" is
  // 1 code point, 2 UTF-16 units and 4 UTF-8 bytes; "e\u0301" is 2 code
  // points and 1 grapheme.
  {
    type: "shop.schema.v1.StringContentCoverage",
    base: stringContentBase,
    fields: {
      minLen: ["ab", "a", "😀", "😀😀", "é", ""],
      maxLen: ["abcdefgh", "abcdefghi", "😀".repeat(8), "😀".repeat(9), "e\u0301".repeat(4)],
      exactLen: ["abcd", "abc", "abcde", "😀".repeat(4), "😀😀", "e\u0301e\u0301"],
      lenBytes: ["abcd", "éé", "😀", "é", "abcde", ""],
      minBytes: ["ab", "é", "a", ""],
      maxBytes: ["abcdefgh", "😀😀", "😀😀a", "abcdefghi"],
      patternInlineFlag: ["abc", "ABC", "AbC", "abd", "abcd"],
      patternNonSpace: ["abc", "a\u00a0b", "a b", "a\tb", ""],
      prefix: ["+15551234", "+1", "15551234", "1+"],
      suffix: ["x.io", ".io", "xio", "x.io\n"],
      contains: ["a.b", ".", "ab", ""],
      prefixAndSuffix: ["presuf", "pre\nsuf", "pre-suf", "presu", "suf", "presufpre"],
      notContains: ["ab", "a b", " "],
      constValue: ["fixed", "fixe", "Fixed", ""],
      inSet: ["a, b", "c", "a", "b", "a,b", ""],
      notInSet: ["x", "y", "z", ""],
      nickname: ["", "nick", undefined],
      code: ["ab", "a", "a b", " ", "abc d", "abcd"],
      uuidPattern: [uuid, uuid.toUpperCase(), "abc-123", ""],
      escaped: ['"\\*/\u2028', '"\\*/x', '"\\*/y', '"\\*/', "x", ""],
    },
  },

  // An absent key is the zero value.
  {
    type: "shop.schema.v1.ImplicitCoverage",
    base: implicitBase,
    fields: {
      shortCode: [undefined, "ab", "abc", "😀😀😀"],
      count: [undefined, 0, -1, 1],
      tags: [undefined, [], ["a"]],
      email: [undefined, "user@localhost", "bad"],
      requiredName: [undefined, "", "x"],
      requiredCount: [undefined, 0, 5, -5],
      accepted: [undefined, false, true],
      color: [undefined, "SCHEMA_COLOR_UNSPECIFIED", "SCHEMA_COLOR_BLUE"],
      requiredTags: [undefined, [], [""]],
      requiredItem: [undefined, {}, { text: "x".repeat(33) }],
    },
    patches: [{}, { shortCode: undefined, count: undefined, tags: undefined, email: undefined }],
  },

  // Standard and URL-safe base64, padded or not.
  {
    type: "shop.schema.v1.BytesCoverage",
    base: bytesBase,
    fields: {
      raw: ["", "AQID", "-_8", "+/8=", "+/8", "_-8", "AQ"],
      exactLen: ["AQIDBA==", "AQIDBA", "-_-_-w", "AQID", "AQIDBAU="],
      minLen: ["AQ==", "AQI=", "AQI", ""],
      maxLen: ["AQIDBAUGBwg=", "AQIDBAUGBwgJ"],
      pattern: ["YWJj", "QUJD", "YQ", ""],
      prefix: ["AQID", "AQI=", "AgE=", ""],
      ip: ["fwAAAQ==", "fwAAAQ", "fwAA", "AAAAAAAAAAAAAAAAAAAAAQ==", ""],
      uuid: ["EjRWeJCrze8SNFZ4kKvN7w==", "-_v7-_v7-_v7-_v7-_v7-w", "EjRWeJCrze8SNFZ4kKvN", ""],
      requiredBlob: ["", "AQID", undefined],
    },
  },

  // The edges of every bound.
  {
    type: "shop.schema.v1.NumberCoverage",
    fields: {
      bounded: [0, 1, 100, 101, -1],
      reversedRange: [4, 5, 7, 10, 11, -2147483648, 2147483647],
      constValue: [7, 8, 0],
      inSet: [1, 3, 4, 0],
      notInSet: [0, -1, 1],
      unsigned: [0, 1, 65535, 65536, 4294967295],
      zigzag: [-129, -128, 2147483647, -2147483648],
      fixed: [1000, 1001, 0],
      signedFixed: [0, -1, -2147483648],
      big: ["0", "1", "-1", "9223372036854775807", "-9223372036854775808"],
      bigUnsigned: ["0", "1000000", "1000001", "18446744073709551615"],
      ratio: [1.5, 0, -3.4028234663852886e38, "NaN", "Infinity", "-Infinity"],
      score: [0, 0.1, -0.1, 1.7976931348623157e308, "NaN", "Infinity", "-Infinity"],
      factor: [0.5, 1, 2, 1.5, "NaN", "Infinity"],
      capped: [0.1, 0.10000000149011612, 0.10000001, 0.2, -1, "NaN", "Infinity", "-Infinity"],
    },
  },

  // Sizes, unique, items, map keys and values.
  {
    type: "shop.schema.v1.CollectionCoverage",
    base: collectionBase,
    fields: {
      bounded: [undefined, [], ["a", "b", "c"], ["a", "b", "c", "d"]],
      unique: [["a", "b"], ["a", "a"], []],
      links: [["https://example.com", "mailto:a@b.io", "urn:isbn:1"], ["https://x.io", "foo bar"], ["http://[::1]/"]],
      names: [["a"], [""], ["a", ""]],
      labels: [
        undefined,
        {},
        { a: "https://x.io" },
        { ab: "foo bar" },
        { ab: "mailto:a@b.io" },
        { ab: "x:/", cd: "x:/", ef: "x:/", gh: "x:/", ij: "x:/" },
        { "😀😀": "x:/" },
        { "😀": "x:/" },
      ],
      flags: [{ true: "x", false: "y" }, {}],
      byNumber: [{ "1": "a", "-5": "b" }, { "2147483647": "x" }],
      items: [[{ text: "x" }], [{}, { text: "x".repeat(33) }]],
      itemsByKey: [{ k: { text: "x" } }, { k: { text: "x".repeat(33) } }],
    },
  },

  // The zero value skips the rules.
  {
    type: "shop.schema.v1.IgnoreCoverage",
    fields: {
      email: [undefined, "", "a@b.io", "bad"],
      threshold: [undefined, 0, 3, 5, 6],
      ids: [["", uuid], ["bad"], []],
      neverChecked: ["anything", ""],
      codes: [{ a: "" }, { a: "abc" }, { a: "ab" }, { a: "", b: "ab" }, {}],
    },
  },

  // 0, 1 and 2 members of each oneof.
  {
    type: "shop.schema.v1.OneofCoverage",
    base: { email: "a@b.io", firstName: "F" },
    patches: [
      {},
      { email: undefined },
      { email: undefined, phone: "1" },
      { phone: "1" },
      { email: "" },
      { email: "bad" },
      { query: "q" },
      { page: 2 },
      { query: "q", page: 2 },
      { query: "" },
      { firstName: undefined },
      { lastName: "L" },
      { firstName: undefined, lastName: "L" },
      { firstName: "" },
    ],
  },

  // Every well-known type.
  {
    type: "shop.schema.v1.WellKnownCoverage",
    fields: {
      timeout: ["1s", "1.5s", "300s", "0.5s", "301s", "0s", "-1s", "1.000000001s", "299.999999999s"],
      expiresAt: [future, past],
      createdAt: ["2024-01-01T00:00:00Z", "2024-01-01T00:00:00.123456789Z"],
      detail: [
        anyDuration,
        { "@type": "type.googleapis.com/google.protobuf.Timestamp", value: past },
        { "@type": "type.googleapis.com/shop.schema.v1.Item", text: "x" },
        {},
      ],
      metadata: [{ a: 1, b: [true, null], c: { d: "e" } }, {}],
      value: [null, 1, "s", [1, "a"], { k: null }, true],
      list: [[1, "a", null], []],
      nothing: [{}],
      updateMask: ["a.b,c", "", "fooBar"],
      label: ["", "x"],
      blob: ["AQID", "-_8", ""],
      bigCount: ["123", "-9223372036854775808"],
      ratio: ["NaN", "Infinity", "-Infinity", 1.5],
      flag: [true, false],
      nullValue: [null],
      checkedNull: [null],
      smallRatio: [1.5, "NaN", "-Infinity", 3.5e38],
      smallCount: [-2147483648, 2147483647, 2147483648, 1.5],
      unsignedCount: [0, 4294967295, -1],
      bigUnsigned: ["18446744073709551615", "18446744073709551616", "-1"],
    },
  },

  // Every member against every enum rule.
  {
    type: "shop.schema.v1.EnumCoverage",
    fields: { defined: colors, allowed: colors, excluded: colors, pinned: colors, plain: colors },
  },

  // json_name, and a digit after "_".
  {
    type: "shop.schema.v1.JsonNameCoverage",
    base: { extId: "x" },
    fields: {
      extId: [undefined, "", "x"],
      metric1st: [0, -1, 5],
      displayName: ["x".repeat(64), "x".repeat(65)],
    },
  },

  // Recursion: TreeNode through a list and a singular field, Ping and Pong.
  // Not here: a Ping.note violation below a Pong. protovalidate-es 1.3.0 misses
  // it, depending on which of the two it planned first (Planner.plan prunes
  // the cycle's first message while its plan is still empty), so the
  // harnesses assert the Zod verdict on it directly.
  {
    type: "shop.schema.v1.TreeNode",
    base: { label: "root" },
    fields: {
      label: [undefined, "", "x"],
      children: [[{ label: "c" }], [{}], [{ label: "c", children: [{}] }]],
      parent: [{ label: "p" }, {}, { label: "p", parent: {} }],
      skipped: [{}, { label: "" }, { bogus: 1 }, "anything"],
    },
  },
  {
    type: "shop.schema.v1.Ping",
    fields: {
      note: ["x".repeat(140), "x".repeat(141)],
      pong: [{}, { ping: { note: "ok" } }, { ping: { pong: {} } }],
    },
  },
  { type: "shop.schema.v1.Pong", fields: { ping: [{ note: "ok" }, {}] } },

  // Message CEL, on a nested message too.
  {
    type: "shop.schema.v1.MessageCelCoverage",
    patches: [
      {},
      { low: 1, high: 2 },
      { low: 2, high: 1 },
      { low: 1, high: 2, window: { startMinute: 1, endMinute: 2 } },
      { low: 1, high: 2, window: { startMinute: 2, endMinute: 1 } },
      { low: 1, high: 2, window: {} },
    ],
  },
  { type: "shop.schema.v1.MessageCelCoverage.Window", patches: [{}, { startMinute: 1, endMinute: 2 }] },

  // A message oneof exempts the zero value of its implicit members.
  {
    type: "shop.schema.v1.MessageOneofCoverage",
    patches: [
      {},
      { code: "" },
      { tags: [] },
      { label: "" },
      { code: "ab" },
      { code: "abc" },
      { tags: ["a"] },
      { tags: ["a", "b"] },
      { label: "x" },
      { code: "", tags: [] },
      { code: "abc", label: "x" },
      { code: "abc", tags: ["a", "b"] },
      { code: "ab", label: "x" },
      { color: "SCHEMA_COLOR_UNSPECIFIED" },
      { color: "SCHEMA_COLOR_RED" },
      { color: "SCHEMA_COLOR_RED", code: "abc" },
      { alias: "" },
      { alias: "a" },
      { alias: "a", label: "x" },
    ],
  },

  // A field rule field() cannot evaluate: its message leaves divisor at 0,
  // and the message rule divides by it. message() reports it instead.
  {
    type: "shop.schema.v1.EvaluationCoverage",
    base: { divisor: 1 },
    fields: { total: [2, 3, 1000, 1001, undefined], divisor: [undefined, 0, 2] },
  },

  // A field CEL rule, an RE2 pattern, a nested message.
  {
    type: "example.v1.User",
    base: userBase,
    fields: {
      id: [undefined, "usr_1", "abc_12345", `usr_${"x".repeat(60)}`, `usr_${"x".repeat(61)}`],
      email: [undefined, "", "bad", "user@localhost"],
      displayName: [undefined, "Jo", "Joe!", "Joe Doe_1"],
      age: [undefined, 0, 130, 131],
      loginCount: [-1, 0, 5],
      roles: [undefined, [], ["a", "a"], [""], ["a", "b"]],
      address: [{}, { country: "US", postalCode: "123" }, { country: "USA", postalCode: "123" }, { country: "😀😀", postalCode: "123" }],
    },
  },
  { type: "example.v1.Address", patches: [{}, { country: "US", postalCode: "12345" }, { country: "U", postalCode: "12" }] },

  {
    type: "shop.common.v1.Money",
    base: { units: "1", nanos: 1, currency: "CURRENCY_EUR" },
    fields: {
      units: [undefined, "1000000000", "1000000001", "-1000000001"],
      nanos: [undefined, 999999999, 1000000000, -1],
      currency: [undefined, "CURRENCY_UNSPECIFIED", "CURRENCY_GBP"],
    },
  },
  {
    type: "shop.common.v1.Pagination",
    base: { page: 1, pageSize: 10 },
    fields: {
      page: [undefined, 0, 1],
      pageSize: [undefined, 100, 101],
      pageToken: ["1234567", "12345678", "x".repeat(512), "x".repeat(513)],
    },
  },
  {
    type: "shop.common.v1.TimeRange",
    base: { start: "2024-01-01T00:00:00Z" },
    patches: [{}, { start: undefined }, { end: "2024-01-02T00:00:00Z" }, { end: "2023-12-31T00:00:00Z" }],
  },
  {
    type: "shop.common.v1.LabelSet",
    fields: { labels: [undefined, {}, { a: "b" }, { A: "b" }, { "a-b_c": "x".repeat(255) }, { a: "x".repeat(256) }, { "": "x" }] },
  },

  {
    type: "shop.catalog.v1.Product",
    base: productBase,
    fields: {
      id: [undefined, uuid, "not-a-uuid"],
      sku: ["AB", "ABC-123", "abc-123", "ABC--1"],
      price: [undefined, {}, { units: "1", nanos: -1, currency: "CURRENCY_EUR" }],
      discountPrice: [
        { units: "5", currency: "CURRENCY_EUR" },
        { units: "20", currency: "CURRENCY_EUR" },
      ],
      status: ["PRODUCT_STATUS_DRAFT", "PRODUCT_STATUS_ARCHIVED"],
      tags: [["a-1"], ["a", "a"], ["A"], Array.from({ length: 21 }, (_, i) => `t${i}`)],
      labels: [{ k: "v" }, Object.fromEntries(Array.from({ length: 17 }, (_, i) => [`k${i}`, "v"]))],
      createdByEmail: ["a@b.io", "bad"],
      version: ["0", "-1"],
    },
    patches: [
      { status: "PRODUCT_STATUS_DRAFT", publishedAt: past },
      { status: "PRODUCT_STATUS_ACTIVE", publishedAt: past },
    ],
  },
  { type: "shop.catalog.v1.GetProductRequest", patches: [{}, { id: uuid }, { sku: "ab" }, { sku: "abc" }, { id: uuid, sku: "abc" }] },
  {
    type: "shop.catalog.v1.ListProductsRequest",
    base: { pagination: { page: 1, pageSize: 10 } },
    patches: [
      {},
      { pagination: undefined },
      { statuses: ["PRODUCT_STATUS_ACTIVE"] },
      { statuses: ["PRODUCT_STATUS_UNSPECIFIED"] },
      { orderBy: "name" },
      { orderBy: "id" },
      { minPrice: money },
      { minPrice: money, maxPrice: money },
      { nameContains: "" },
    ],
  },
  { type: "shop.catalog.v1.ListProductsResponse", patches: [{}, { totalSize: "-1" }, { products: [productBase] }, { products: [{}] }] },
  {
    type: "shop.catalog.v1.DeleteProductRequest",
    patches: [{ id: uuid }, { id: "" }, {}, { id: uuid, expectedVersion: "-1" }, { id: uuid, expectedVersion: "0" }],
  },
  {
    type: "shop.catalog.v1.UpdateProductRequest",
    patches: [{ product: productBase }, { product: productNoId }, { product: productBase, updateMask: "name,sku" }],
  },
  { type: "shop.catalog.v1.CreateProductRequest", patches: [{ product: productBase }, { product: productNoId }, {}] },
  { type: "shop.catalog.v1.GetProductResponse", patches: [{}, { product: productBase }, { product: {} }] },

  {
    type: "shop.inventory.v1.Warehouse",
    base: { id: uuid, name: "W", address, contacts: [{ name: "A", email: "a@b.io" }] },
    fields: {
      maxDwellTime: ["3600s", "3599s", "2592000s", "2592001s"],
      openingHours: [
        { mon: { opensAtMinute: 480, closesAtMinute: 1020 } },
        { xyz: { closesAtMinute: 1 } },
        { mon: { opensAtMinute: 600, closesAtMinute: 500 } },
        {},
      ],
      contacts: [
        [],
        [{ name: "A", phone: "+15551234567" }],
        [{ name: "A", phone: "555" }],
        [{ name: "A" }],
        [{ name: "A", email: "a@b.io", phone: "+15551234567" }],
        [{ name: "A", extension: 99 }],
      ],
      address: [undefined, { ...address, countryCode: "us" }, addressNoLine1],
    },
  },
  {
    type: "shop.inventory.v1.StockMovement",
    base: { warehouseId: uuid, productId: uuid, kind: "STOCK_MOVEMENT_KIND_INBOUND", quantityDelta: 5, occurredAt: past },
    patches: [
      {},
      { kind: "STOCK_MOVEMENT_KIND_OUTBOUND" },
      { kind: undefined },
      { occurredAt: future },
      { occurredAt: undefined },
      { purchaseOrderId: "p", customerOrderId: "c" },
      { purchaseOrderId: "p" },
      { quantityDelta: 100001 },
    ],
  },
  {
    type: "shop.inventory.v1.StockLevel",
    base: { productId: uuid },
    patches: [
      {},
      { productId: undefined },
      { onHand: "10", reserved: "5" },
      { onHand: "5", reserved: "10" },
      { onHand: "100000001", reserved: "1" },
      { countedAt: past },
      { reorderThreshold: 10001 },
    ],
  },
  {
    type: "shop.inventory.v1.ContactPoint",
    patches: [{ name: "A", email: "a@b.io" }, { name: "A" }, { email: "a@b.io" }, { name: "A", extension: 99 }, { name: "A", extension: 100 }],
  },
  { type: "shop.inventory.v1.OpeningHours", patches: [{}, { opensAtMinute: 1, closesAtMinute: 2 }, { opensAtMinute: 1439, closesAtMinute: 1441 }] },

  {
    type: "shop.coverage.v1.StringRuleCoverage",
    base: stringRules,
    fields: {
      exactLen: ["😀".repeat(10), "abcdefghijk"],
      affixes: ["id_a b-_v1", "id_-_v1", "x_a-b_v1"],
      enumerated: ["beta", "deprecated", "gamma"],
      headerName: ["a,b", "a b"],
    },
    patches: [{}, { uuid: undefined }],
  },
  {
    type: "shop.coverage.v1.NumericRuleCoverage",
    base: numericRules,
    fields: {
      i64: ["9000", "9001", "-9001"],
      s64: ["1024", "1025"],
      f64: ["18446744073709551615", "0"],
      ratio: [1, 1.5, "NaN"],
      factor: [2, 3],
      excluded: [0, -1, 2],
    },
  },
  {
    type: "shop.coverage.v1.ScalarRuleCoverage",
    base: {
      accepted: true,
      payload: "AQ==",
      signature: "AQIAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
      remoteIp: "fwAAAQ==",
      subset: "FLAVOR_SWEET",
      pinned: "FLAVOR_SOUR",
    },
    patches: [{}, { accepted: false }, { signature: "AQID" }, { subset: "FLAVOR_SOUR" }, { defined: "FLAVOR_BITTER" }],
  },
  {
    type: "shop.coverage.v1.WellKnownRuleCoverage",
    patches: [
      {},
      { interval: "60s" },
      { interval: "61s" },
      { seenAt: past },
      { seenAt: future },
      { recent: past },
      { afterEpoch: "1969-12-31T23:59:59Z" },
      { detail: anyDuration },
    ],
  },
  {
    type: "shop.coverage.v1.CollectionRuleCoverage",
    base: { names: ["a"], counters: { a: 1 } },
    patches: [
      {},
      { names: ["a", "a"] },
      { flavors: ["FLAVOR_UNSPECIFIED"] },
      { nested: [stringRules] },
      { nested: [{}] },
      { counters: { A: 1 } },
      { counters: { a: -1 } },
      { readings: { r: numericRules } },
      { readings: { r: {} } },
    ],
  },
  {
    type: "shop.coverage.v1.PresenceRuleCoverage",
    base: { requiredValue: "v", requiredMessage: stringRules },
    patches: [
      {},
      { requiredValue: "" },
      { optionalValue: "ab" },
      { skipWhenEmpty: "" },
      { skipWhenEmpty: "bad" },
      { neverChecked: "x" },
      { requiredMessage: {} },
    ],
  },
  {
    type: "shop.coverage.v1.CelRuleCoverage",
    base: { identifier: "itm_1", windowStart: 0, windowEnd: 15, slugs: ["a"], nested: stringRules },
    patches: [
      {},
      { identifier: "abc_1" },
      { identifier: `itm_${"x".repeat(37)}` },
      { windowStart: 7 },
      { windowEnd: 0 },
      { windowEnd: 1455 },
      { slugs: ["A"] },
      { slugs: [""] },
      { nested: stringRulesNoUuid },
    ],
  },
  {
    type: "shop.coverage.v1.CelNarrowingCoverage",
    base: { label: "l", detail: stringRules, inner: { stamp: past, mask: "a" } },
    patches: [{}, { slug: "s" }, { note: "n" }, { inner: { stamp: past } }, { detail: stringRulesNoUuid }],
  },
  { type: "shop.coverage.v1.CelNarrowingCoverage.Inner", patches: [{}, { stamp: past, flavor: "FLAVOR_SWEET", mask: "a.b" }] },
  {
    type: "shop.coverage.v1.AmbiguityCoverage",
    base: ambiguityBase,
    patches: [
      {},
      { uncheckedDetail: stringRules },
      { uncheckedDetail: {} },
      { outsideWindow: 15 },
      { outsideWindow: 25 },
      { optionalTags: ["", "abc"] },
      { optionalTags: ["ab"] },
      { email: undefined, phone: "12345" },
      { legacyNote: "x" },
      { quantity: 15 },
    ],
  },
];

// create() inits covering every field kind, each valid under protovalidate.
// bytes are Uint8Array, 64-bit integers bigint, enums numbers.
const ts = { seconds: 1704067200n, nanos: 123456789 };
const bytes16 = new Uint8Array([0x12, 0x34, 0x56, 0x78, 0x90, 0xab, 0xcd, 0xef, 0x12, 0x34, 0x56, 0x78, 0x90, 0xab, 0xcd, 0xef]);
// google.protobuf.Duration{seconds: 1} in the binary format.
const anyDurationInit = { typeUrl: "type.googleapis.com/google.protobuf.Duration", value: new Uint8Array([0x08, 0x01]) };
const wellKnown = {
  timeout: { seconds: 1n, nanos: 500000000 },
  expiresAt: { seconds: 32503680000n },
  createdAt: ts,
  detail: anyDurationInit,
  metadata: { a: 1, b: [true, null], c: { d: "e" } },
  value: { kind: { case: "listValue", value: { values: [{ kind: { case: "stringValue", value: "s" } }] } } },
  list: { values: [{ kind: { case: "numberValue", value: 1 } }, { kind: { case: "nullValue", value: 0 } }] },
  nothing: {},
  updateMask: { paths: ["a.b", "c"] },
  label: "x",
  blob: new Uint8Array([0xfb, 0xff]),
  bigCount: -9223372036854775808n,
  flag: true,
  smallRatio: 0.5,
  smallCount: -2147483648,
  unsignedCount: 4294967295,
  bigUnsigned: 18446744073709551615n,
};
const numericInit = {
  i32: 1,
  u64: 18446744073709551615n,
  f32: 1,
  f64: 18446744073709551615n,
  sf32: -1,
  sf64: -9223372036854775808n,
  s64: -9223372036854775808n,
  factor: 1,
  constant: 42,
  excluded: 1,
};

export const roundTrips: RoundTrip[] = [
  {
    type: "shop.schema.v1.FormatCoverage",
    init: {
      hostname: "example.com.",
      email: "user@localhost",
      ip: "fe80::1%eth0",
      ipv4: "192.0.2.1",
      ipv6: "::ffff:192.0.2.1",
      uri: "mailto:a@b.io",
      uriRef: "./a",
      address: "example.com.",
      hostAndPort: "example.com:0",
      ipWithPrefixlen: "::ffff:192.0.2.1/96",
      ipv4WithPrefixlen: "192.0.2.1/24",
      ipv6WithPrefixlen: "2001:db8::1/64",
      ipPrefix: "10.0.0.128/25",
      ipv4Prefix: "10.0.0.128/25",
      ipv6Prefix: "2001:db8::/32",
      uuid,
      tuuid,
      ulid: "01ARZ3NDEKTSV4RRFFQ69G5FAV",
      protobufFqn: "a.B",
      protobufDotFqn: ".a.B",
      headerName: "Content-Type",
      headerValue: "text/html",
      hiddenHost: "example.com.",
    },
  },
  {
    type: "shop.schema.v1.StringContentCoverage",
    init: {
      minLen: "😀😀",
      maxLen: "😀".repeat(8),
      exactLen: "e\u0301e\u0301",
      lenBytes: "😀",
      minBytes: "é",
      maxBytes: "😀😀",
      patternInlineFlag: "ABC",
      patternNonSpace: "a\u00a0b",
      prefix: "+1",
      suffix: ".io",
      contains: ".",
      prefixAndSuffix: "pre\nsuf",
      notContains: "x",
      constValue: "fixed",
      inSet: "a, b",
      notInSet: "z",
      nickname: "",
      code: "ab",
      uuidPattern: uuid,
      escaped: '"\\*/\u2028',
    },
  },
  {
    type: "shop.schema.v1.ImplicitCoverage",
    init: { shortCode: "abc", count: 1, tags: ["t"], email: "a@b.io", requiredName: "n", requiredCount: 1, accepted: true, color: 1, requiredTags: ["t"], requiredItem: {} },
  },
  {
    type: "shop.schema.v1.BytesCoverage",
    init: {
      raw: new Uint8Array([0xfb, 0xff]),
      exactLen: new Uint8Array([1, 2, 3, 4]),
      minLen: new Uint8Array([1, 2]),
      maxLen: new Uint8Array([1, 2, 3, 4, 5, 6, 7, 8]),
      pattern: new Uint8Array([0x61, 0x62, 0x63]),
      prefix: new Uint8Array([1, 2, 3]),
      ip: new Uint8Array([127, 0, 0, 1]),
      uuid: bytes16,
      requiredBlob: new Uint8Array(0),
    },
  },
  {
    type: "shop.schema.v1.NumberCoverage",
    init: {
      bounded: 100,
      reversedRange: 11,
      constValue: 7,
      inSet: 2,
      notInSet: 1,
      unsigned: 65535,
      zigzag: -128,
      fixed: 1000,
      signedFixed: -2147483648,
      big: 9223372036854775807n,
      bigUnsigned: 1000000n,
      ratio: 3.4028234663852886e38,
      score: Number.POSITIVE_INFINITY,
      factor: 0.5,
      capped: 0.10000000149011612,
    },
  },
  { type: "shop.coverage.v1.NumericRuleCoverage", init: numericInit },
  {
    type: "shop.schema.v1.CollectionCoverage",
    init: {
      bounded: ["a", "b"],
      unique: ["a", "b"],
      links: ["mailto:a@b.io", "http://[::1]/"],
      names: ["n"],
      labels: { ab: "urn:isbn:1" },
      flags: { true: "t", false: "f" },
      byNumber: { 1: "one", [-5]: "minus five" },
      items: [{ text: "x" }, {}],
      itemsByKey: { k: { text: "y" } },
    },
  },
  { type: "shop.schema.v1.IgnoreCoverage", init: { email: "", threshold: 0, ids: ["", uuid], neverChecked: "", codes: { a: "", b: "abc" } } },
  { type: "shop.schema.v1.IgnoreCoverage", init: { email: "a@b.io", threshold: 6 } },
  {
    type: "shop.schema.v1.OneofCoverage",
    init: { contact: { case: "email", value: "a@b.io" }, filter: { case: "page", value: 3 }, firstName: "F" },
  },
  {
    type: "shop.schema.v1.OneofCoverage",
    init: { contact: { case: "phone", value: "123" }, filter: { case: "query", value: "q" }, lastName: "L" },
  },
  { type: "shop.schema.v1.WellKnownCoverage", init: { ...wellKnown, ratio: Number.NaN } },
  { type: "shop.schema.v1.WellKnownCoverage", init: { ...wellKnown, ratio: Number.POSITIVE_INFINITY } },
  { type: "shop.schema.v1.WellKnownCoverage", init: { ...wellKnown, ratio: Number.NEGATIVE_INFINITY } },
  { type: "shop.schema.v1.EnumCoverage", init: { defined: 3, allowed: 1, excluded: 0, pinned: 2, plain: 3 } },
  { type: "shop.schema.v1.JsonNameCoverage", init: { externalId: "x", metric1st: 3, displayName: "d" } },
  {
    type: "shop.schema.v1.TreeNode",
    init: { label: "root", children: [{ label: "a", children: [{ label: "b" }] }], parent: { label: "p" }, skipped: { label: "" } },
  },
  { type: "shop.schema.v1.Ping", init: { note: "n", pong: { ping: { note: "m", pong: {} } } } },
  { type: "shop.schema.v1.MessageCelCoverage", init: { low: 1, high: 2, window: { startMinute: 1, endMinute: 2 } } },
  { type: "shop.schema.v1.MessageOneofCoverage", init: { code: "abc" } },
  { type: "shop.schema.v1.MessageOneofCoverage", init: { tags: ["a", "b"] } },
  { type: "shop.schema.v1.MessageOneofCoverage", init: { color: 2 } },
  { type: "shop.schema.v1.EvaluationCoverage", init: { divisor: 1, total: 2 } },
  {
    type: "example.v1.User",
    init: { id: "usr_12345", email: "a@b.io", displayName: "Joe", age: 30, loginCount: 2, roles: ["a", "b"], address: { country: "US", postalCode: "12345" } },
  },
  {
    type: "shop.catalog.v1.Product",
    init: {
      id: uuid,
      sku: "ABC-123",
      name: "Widget",
      price: { units: 10n, nanos: 500000000, currency: 1 },
      discountPrice: { units: 5n, currency: 1 },
      status: 1,
      tags: ["a"],
      labels: { k: "v" },
      createdAt: ts,
      updatedAt: ts,
      version: 3n,
    },
  },
  {
    type: "shop.catalog.v1.ListProductsRequest",
    init: { pagination: { page: 1, pageSize: 10 }, statuses: [1, 2], orderBy: "name", orderDirection: 1 },
  },
  {
    type: "shop.inventory.v1.Warehouse",
    init: {
      id: uuid,
      name: "W",
      address: { line1: "1", city: "c", countryCode: "US", postalCode: "12345" },
      maxDwellTime: { seconds: 3600n },
      openingHours: { mon: { opensAtMinute: 480, closesAtMinute: 1020 } },
      contacts: [{ name: "A", channel: { case: "email", value: "a@b.io" } }],
    },
  },
  {
    type: "shop.inventory.v1.StockMovement",
    init: {
      id: uuid,
      warehouseId: uuid,
      productId: uuid,
      kind: 2,
      quantityDelta: -5,
      occurredAt: { seconds: 1577836800n },
      unitCost: { units: 1n, currency: 2 },
      purchaseOrderId: "p",
    },
  },
  { type: "shop.inventory.v1.StockLevel", init: { productId: uuid, onHand: 10n, reserved: 5n, reorderThreshold: 10 } },
  { type: "shop.common.v1.TimeRange", init: { start: ts, end: { seconds: 1704067201n } } },
  { type: "shop.common.v1.LabelSet", init: { labels: { a: "b" } } },
  {
    type: "shop.coverage.v1.WellKnownRuleCoverage",
    init: {
      timeout: { seconds: 2n },
      interval: { seconds: 60n },
      seenAt: { seconds: 946684800n },
      expiresAt: { seconds: 32503680000n },
      afterEpoch: { seconds: 1n },
      detail: anyDurationInit,
      metadata: { x: "y" },
      updateMask: { paths: ["a"] },
    },
  },
  {
    type: "shop.coverage.v1.ScalarRuleCoverage",
    init: {
      accepted: true,
      payload: new Uint8Array([1]),
      signature: new Uint8Array([1, 2, ...new Array<number>(30).fill(0)]),
      remoteIp: new Uint8Array([127, 0, 0, 1]),
      subset: 1,
      pinned: 3,
    },
  },
  {
    type: "shop.coverage.v1.CollectionRuleCoverage",
    init: { names: ["a"], flavors: [1, 4], nested: [stringRules], counters: { a: 1 }, readings: { r: numericInit } },
  },
  {
    type: "shop.coverage.v1.AmbiguityCoverage",
    init: {
      userId: uuid,
      constructor$: uuid,
      label: "l",
      nickname: "n",
      channel: { case: "email", value: "a@b.io" },
      quantity: 10,
      optionalTags: ["", "abc"],
      outsideWindow: 25,
    },
  },
];
