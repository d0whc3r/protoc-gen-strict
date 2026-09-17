// Type-level check that the .strict.ts overlays narrow what they claim to, stay
// assignable to the types protoc-gen-es generated, and carry the narrowing into
// the retyped descriptors. This is the only test covering the emitters.
import type { MessageValidType } from "@bufbuild/protobuf";
import { asEmail, asUuid, createStrict, type Email, type Uuid } from "./gen/typescript/strict/types";
import type { Address, User } from "./gen/typescript/example/v1/user_pb";
import type { AddressStrict, UserStrict } from "./gen/typescript/example/v1/user.strict";
import type {
  CreateProductRequestStrict,
  ProductStrict,
  UpdateProductRequestStrict,
} from "./gen/typescript/shop/catalog/v1/product.strict";
import {
  CatalogServiceStrict,
  CreateProductRequestOutputOnlyFields,
  ProductOutputOnlyFields,
  CreateProductRequestStrictSchema,
  DeleteProductRequestStrictSchema,
  ListProductsRequestStrictSchema,
} from "./gen/typescript/shop/catalog/v1/product.strict";
import type {
  DeleteProductRequestStrict,
  ListProductsRequestStrict,
} from "./gen/typescript/shop/catalog/v1/product.strict";
import type {
  ContactPointStrict,
  StockMovementKindStrict,
  StockMovementStrict,
} from "./gen/typescript/shop/inventory/v1/warehouse.strict";
import { StockMovementKind } from "./gen/typescript/shop/inventory/v1/warehouse_pb";
import type { ProductStatusStrict } from "./gen/typescript/shop/catalog/v1/product.strict";
import type { MoneyStrict } from "./gen/typescript/shop/common/v1/common.strict";
import { Currency, SortDirection } from "./gen/typescript/shop/common/v1/common_pb";
import { ProductStatus } from "./gen/typescript/shop/catalog/v1/product_pb";
import type {
  AmbiguityCoverageStrict,
  CelNarrowingCoverageStrict,
  StringRuleCoverageStrict,
} from "./gen/typescript/shop/coverage/v1/rules.strict";
import type { AmbiguityCoverage } from "./gen/typescript/shop/coverage/v1/rules_pb";

type Expect<T extends true> = T;
type Extends<A, B> = A extends B ? true : false;
type Never<T> = [T] extends [never] ? true : false;
/** True when K is an optional property of T. */
type Optional<T, K extends keyof T> = Record<string, never> extends Pick<T, K> ? true : false;

// --- assignability ---------------------------------------------------------
// A strict type is still the generated type, so protobuf-es APIs accept it.
type _Assignable = Expect<Extends<UserStrict, User>>;
type _AssignableNested = Expect<Extends<AddressStrict, Address>>;

// --- string shapes -----------------------------------------------------------
// string.uuid and string.email become template literal types: a literal of the
// right shape is assignable with no constructor to call.
type _Uuid = Expect<Extends<ProductStrict["id"], Uuid>>;
type _Email = Expect<Extends<UserStrict["email"], Email>>;

const _literals: Pick<UserStrict, "email"> = { email: "someone@example.com" };
void _literals;

const _rawUuid: ProductStrict["id"] = "3fa85f64-5717-4562-b3fc-2c963f66afa6";
void _rawUuid;

// @ts-expect-error the shape is still checked
const _notAUuid: ProductStrict["id"] = "hello";
void _notAUuid;

// A value only known at runtime fits no shape, so it needs a cast — there is no
// constructor to call, and protovalidate stays the authority on validity.
declare const runtimeString: string;
const _castUuid: ProductStrict["id"] = runtimeString as Uuid;
void _castUuid;

// The helpers are that cast and nothing more.
const _helperUuid: ProductStrict["id"] = asUuid(runtimeString);
void _helperUuid;
const _helperEmail: UserStrict["email"] = asEmail(runtimeString);
void _helperEmail;

// string.min_len has no shape that excludes the empty string, so the field is
// left exactly as protoc-gen-es declared it.
type _MinLenUnnarrowed = Expect<Extends<string, UserStrict["displayName"]>>;

// --- collections -----------------------------------------------------------
// repeated.min_items = 1 rules out the empty list.
type _NonEmptyList = Expect<Extends<[string], UserStrict["roles"]>>;
// @ts-expect-error roles has min_items = 1
const _empty: UserStrict["roles"] = [];
void _empty;

// --- presence --------------------------------------------------------------
// required drops the `?` that protoc-gen-es emitted…
type _Required = Expect<Optional<ProductStrict, "price"> extends true ? false : true>;
type _RequiredType = Expect<Extends<ProductStrict["price"], MoneyStrict>>;
// …and a field with no such rule keeps it.
type _StillOptional = Expect<Optional<ProductStrict, "description">>;

// --- server-assigned fields ------------------------------------------------
// (google.api.field_behavior) = OUTPUT_ONLY makes the property readonly and
// changes nothing else: the value keeps the type protoc-gen-es gave it, and an
// optional field stays optional.
declare const _product: ProductStrict;
// @ts-expect-error id is OUTPUT_ONLY, so the server is the only writer
_product.id = _product.id;
// A field with no field_behavior stays writable.
_product.sku = _product.sku;
type _OutputOnlyStillOptional = Expect<Optional<ProductStrict, "publishedAt">>;

// The same fields are listed apart from the type, under the proto names a
// google.protobuf.FieldMask carries, so an update mask can subtract them.
type _OutputOnlyField = (typeof ProductOutputOnlyFields)[number];
type _OutputOnlyIsProtoName = Expect<Extends<"created_by_email", _OutputOnlyField>>;
// @ts-expect-error the list carries proto names, not the camelCase property
const _camel: _OutputOnlyField = "createdByEmail";
void _camel;
const _mask: string[] = ["sku", "name"].filter(
  (path) => !(ProductOutputOnlyFields as readonly string[]).includes(path),
);
void _mask;

// A field under an OUTPUT_ONLY one is server-assigned too, so the list carries
// the whole subtree as dotted paths — including a message that declares no
// OUTPUT_ONLY field of its own and only reaches them through what it wraps.
type _OutputOnlyNested = Expect<Extends<"created_at.nanos", _OutputOnlyField>>;
type _OutputOnlyReached = Expect<
  Extends<"product.id", (typeof CreateProductRequestOutputOnlyFields)[number]>
>;

// --- enums and oneofs ------------------------------------------------------
// Every enum gets a strict type without the member numbered 0, and it is still
// the enum protoc-gen-es declared.
type _EnumStrictAssignable = Expect<Extends<StockMovementKindStrict, StockMovementKind>>;
type _EnumStrictDropsZero = Expect<Never<Extract<StockMovementKindStrict, 0>>>;
const _declared: StockMovementKindStrict = StockMovementKind.INBOUND;
void _declared;
// @ts-expect-error UNSPECIFIED is the member the strict type rules out
const _unspecified: StockMovementKindStrict = StockMovementKind.UNSPECIFIED;
void _unspecified;

// A field takes that type whether or not a rule says so: `kind` carries
// enum.not_in = [0], `status` carries nothing but enum.defined_only.
type _EnumExcluded = Expect<Never<Extract<StockMovementStrict["kind"], 0>>>;
type _EnumRuleless = Expect<Extends<ProductStrict["status"], ProductStatusStrict>>;
type _EnumRulelessDropsZero = Expect<Never<Extract<ProductStrict["status"], 0>>>;
// A required oneof loses the "nothing set" arm.
type _OneofSet = Expect<Never<Extract<ContactPointStrict["channel"], { case: undefined }>>>;

// --- propagation -----------------------------------------------------------
// A message-typed field carries its target's narrowing, across files too.
type _Propagates = Expect<Extends<NonNullable<UserStrict["address"]>, AddressStrict>>;
type _PropagatesAcrossFiles = Expect<Extends<NonNullable<ProductStrict["price"]>, MoneyStrict>>;

// --- retyped descriptors ---------------------------------------------------
// The narrowing reaches a call site that only ever holds a descriptor.
type _SchemaValidType = Expect<
  Extends<MessageValidType<typeof CreateProductRequestStrictSchema>, CreateProductRequestStrict>
>;
type _ServiceValidType = Expect<
  Extends<
    MessageValidType<(typeof CatalogServiceStrict)["method"]["createProduct"]["input"]>,
    CreateProductRequestStrict
  >
>;

// --- CEL translated into types ---------------------------------------------
// `this.label != ''` has no type equivalent: no string shape excludes the empty
// string, so the property is left as protoc-gen-es declared it.
type _CelNonEmptyUnnarrowed = Expect<Extends<string, CelNarrowingCoverageStrict["label"]>>;
// `this.slug == ''`
type _CelEmpty = Expect<Extends<CelNarrowingCoverageStrict["slug"], "">>;
// `this.flavor == 0` on an enum keeps only the zero-numbered member.
type _CelZero = Expect<Never<Exclude<CelNarrowingCoverageStrict["flavor"], 0>>>;
// `!has(this.note)` — the field may be omitted, and nothing but undefined assigned.
type _CelAbsent = Expect<Never<NonNullable<CelNarrowingCoverageStrict["note"]>>>;
type _CelAbsentStaysOptional = Expect<Optional<CelNarrowingCoverageStrict, "note">>;
// `has(this.detail)` changes the optionality, not the type.
type _CelPresent = Expect<Optional<CelNarrowingCoverageStrict, "detail"> extends true ? false : true>;
// A message-typed field a CEL path reaches through keeps the target's own
// strict type, whatever the path's own term did.
type _CelNested = Expect<Extends<CelNarrowingCoverageStrict["detail"], StringRuleCoverageStrict>>;

// `update_product.id_present` puts a CEL `!= ''` on a field that already carries
// string.uuid. The term is left to runtime, the field rule stands.
const _intersected: UpdateProductRequestStrict["product"]["id"] =
  "3fa85f64-5717-4562-b3fc-2c963f66afa6";
void _intersected;

// @ts-expect-error slug is constrained to the empty string
const _slug: CelNarrowingCoverageStrict["slug"] = "x";
void _slug;

// `has(this.inner.stamp)` on a nested path: making the property required is not
// enough, because protoc-gen-es writes the `undefined` into the property type.
type _CelNestedPresent = Expect<
  Never<Extract<NonNullable<CelNarrowingCoverageStrict["inner"]>["stamp"], undefined>>
>;

// --- names and the rules no type carries ------------------------------------
// The property is protobuf-es's local name, never the overridden JSON name:
// `user_id [json_name = "external-id"]` stays `userId`.
type _LocalName = Expect<Extends<AmbiguityCoverageStrict["userId"], Uuid>>;
// A property name JavaScript reserves is escaped, as protobuf-es escapes it.
type _EscapedName = Expect<Extends<AmbiguityCoverageStrict["constructor$"], Uuid>>;

// `has`/`!has` on a field with no presence is a value test, not a presence one,
// so `!has(this.legacy_note)` pins the value rather than the optionality.
type _ImplicitAbsence = Expect<Extends<AmbiguityCoverageStrict["legacyNote"], "">>;

// `has(this.nickname) && this.nickname != ''` keeps the half a type can carry:
// the property is required and holds no undefined.
type _PresenceRequired = Expect<
  Optional<AmbiguityCoverageStrict, "nickname"> extends true ? false : true
>;
type _PresenceNotUndefined = Expect<
  Never<Extract<AmbiguityCoverageStrict["nickname"], undefined>>
>;

// A oneof without `required` stands as protoc-gen-es declared it, member rules
// and the CEL term on a member included.
type _OneofUnnarrowed = Expect<
  Extends<AmbiguityCoverage["channel"], AmbiguityCoverageStrict["channel"]>
>;

// IGNORE_ALWAYS switches off the recursion into the message too, so the field
// keeps the generated type rather than the target's strict one.
type _IgnoredKeepsGeneratedType = Expect<
  Extends<AmbiguityCoverage["uncheckedDetail"], AmbiguityCoverageStrict["uncheckedDetail"]>
>;

// --- createStrict ----------------------------------------------------------
// The constructor takes protobuf-es's own initializer, checks each value
// against what the rules narrowed the field to, and reports the strict type.
const page = { page: 1, pageSize: 20 };

const _built = createStrict(ListProductsRequestStrictSchema, {
  pagination: { page: 1, pageSize: 20, pageToken: "abcdefgh" },
  nameContains: "widget",
  orderDirection: SortDirection.ASC,
});
type _BuiltIsStrict = Expect<Extends<typeof _built, ListProductsRequestStrict>>;

// A field string.min_len is the only rule on takes any string: the bound has no
// type equivalent, so the empty one and a runtime one both pass.
createStrict(ListProductsRequestStrictSchema, {
  pagination: page,
  nameContains: "",
  orderDirection: SortDirection.ASC,
});
createStrict(ListProductsRequestStrictSchema, {
  pagination: page,
  nameContains: runtimeString,
  orderDirection: SortDirection.ASC,
});

// A field create() would default past its narrowing has to be supplied.
// @ts-expect-error pagination is required
createStrict(ListProductsRequestStrictSchema, { nameContains: "widget" });

// An enum field is one of them: create() would leave it at the zero member the
// enum's strict type rules out, so the initializer has to name a real one.
// @ts-expect-error orderDirection is required
createStrict(ListProductsRequestStrictSchema, { pagination: page, nameContains: "widget" });

// string.uuid accepts a well-shaped literal, and nothing else.
const _deleted = createStrict(DeleteProductRequestStrictSchema, {
  id: "3fa85f64-5717-4562-b3fc-2c963f66afa6",
});
type _DeletedIsStrict = Expect<Extends<typeof _deleted, DeleteProductRequestStrict>>;
// @ts-expect-error not a UUID
createStrict(DeleteProductRequestStrictSchema, { id: "hello" });
// @ts-expect-error a bare string is not a UUID
createStrict(DeleteProductRequestStrictSchema, { id: runtimeString });
createStrict(DeleteProductRequestStrictSchema, { id: runtimeString as Uuid });

// The read-only fields of a create request are pinned to what `create` writes:
// `this.product.id == ''` narrows id to `Uuid & ""`, which no caller can write.
// The initializer leaves them out, labels included — `create` writes the map.
const draft = {
  sku: "ABC-1",
  name: "widget",
  price: { currency: Currency.EUR, units: 10n, nanos: 0 },
  status: ProductStatus.ACTIVE,
} as const;
const _created = createStrict(CreateProductRequestStrictSchema, { product: draft });
type _CreatedIsStrict = Expect<Extends<typeof _created, CreateProductRequestStrict>>;

// Writing the pinned value is still allowed…
createStrict(CreateProductRequestStrictSchema, { product: { ...draft, id: "" } });
// …and nothing else is.
createStrict(CreateProductRequestStrictSchema, {
  // @ts-expect-error a create request must leave id empty
  product: { ...draft, id: "3fa85f64-5717-4562-b3fc-2c963f66afa6" },
});

// status is still an enum whose zero member the strict alias excludes, so the
// initializer has to name a real member.
createStrict(CreateProductRequestStrictSchema, {
  // @ts-expect-error status is required
  product: { sku: "ABC-1", name: "widget", price: draft.price },
});

// An initializer's own optional properties survive the check. A form value, or
// anything spread from one, carries `?` on the fields the user left blank, and
// a nested message carries an optional `$unknown` — neither may be forced to be
// present just because the strict type mentions the key.
declare const partialDraft: {
  sku: string;
  name: string;
  price: MoneyStrict;
  discountPrice?: MoneyStrict;
  status: ProductStatusStrict;
};
createStrict(CreateProductRequestStrictSchema, { product: partialDraft });
