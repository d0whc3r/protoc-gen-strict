// Type-level check that the .strict.ts overlays narrow what they claim to, stay
// assignable to the types protoc-gen-es generated, and carry the narrowing into
// the retyped descriptors. This is the only test covering the emitters.
import type { MessageValidType } from "@bufbuild/protobuf";
import { email, nonEmpty, uuid, type Email, type NonEmpty, type Uuid } from "./gen/typescript/strict/types";
import type { Address, User } from "./gen/typescript/example/v1/user_pb";
import type { AddressStrict, UserStrict } from "./gen/typescript/example/v1/user.strict";
import type {
  CreateProductRequestStrict,
  ProductStrict,
  UpdateProductRequestStrict,
} from "./gen/typescript/shop/catalog/v1/product.strict";
import { CatalogServiceStrict, CreateProductRequestStrictSchema } from "./gen/typescript/shop/catalog/v1/product.strict";
import type { ContactPointStrict, StockMovementStrict } from "./gen/typescript/shop/inventory/v1/warehouse.strict";
import type { MoneyStrict } from "./gen/typescript/shop/common/v1/common.strict";
import type {
  AmbiguityCoverageStrict,
  CelNarrowingCoverageStrict,
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

// --- brands ----------------------------------------------------------------
// string.uuid, string.email and string.min_len become nominal types, so an
// arbitrary string can no longer reach the field.
type _Uuid = Expect<Extends<ProductStrict["id"], Uuid>>;
type _Email = Expect<Extends<UserStrict["email"], Email>>;
type _NonEmptyBrand = Expect<Extends<UserStrict["displayName"], NonEmpty>>;

// The constructor is the only way in, and its result fits.
const _viaConstructors: Pick<UserStrict, "email" | "displayName"> = {
  email: email("someone@example.com"),
  displayName: nonEmpty("Ada"),
};
void _viaConstructors;
void uuid;

// @ts-expect-error a raw string is not a UUID
const _rawUuid: ProductStrict["id"] = "3fa85f64-5717-4562-b3fc-2c963f66afa6";
void _rawUuid;

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

// --- enums and oneofs ------------------------------------------------------
// enum.not_in = [0] removes the zero member.
type _EnumExcluded = Expect<Never<Extract<StockMovementStrict["kind"], 0>>>;
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
// `this.label != ''`
type _CelNonEmpty = Expect<Extends<CelNarrowingCoverageStrict["label"], NonEmpty>>;
// `this.slug == ''`
type _CelEmpty = Expect<Extends<CelNarrowingCoverageStrict["slug"], "">>;
// `this.flavor == 0` on an enum keeps only the zero-numbered member.
type _CelZero = Expect<Never<Exclude<CelNarrowingCoverageStrict["flavor"], 0>>>;
// `!has(this.note)` — the field may be omitted, and nothing but undefined assigned.
type _CelAbsent = Expect<Never<NonNullable<CelNarrowingCoverageStrict["note"]>>>;
type _CelAbsentStaysOptional = Expect<Optional<CelNarrowingCoverageStrict, "note">>;
// `has(this.detail)` changes the optionality, not the type.
type _CelPresent = Expect<Optional<CelNarrowingCoverageStrict, "detail"> extends true ? false : true>;
// `this.detail.uuid != ''` narrows through a message-typed field, on top of the
// target's own rules rather than instead of them.
type _CelNested = Expect<Extends<CelNarrowingCoverageStrict["detail"]["uuid"], NonEmpty>>;

// The constructors compose, which is how a field carrying both a field rule and
// a CEL term is built.
// `update_product.id_present` puts a CEL `!= ''` on a field that already carries
// string.uuid, so the type is Uuid & NonEmpty.
const _intersected: UpdateProductRequestStrict["product"]["id"] = nonEmpty(
  uuid("3fa85f64-5717-4562-b3fc-2c963f66afa6"),
);
void _intersected;

// @ts-expect-error slug is constrained to the empty string
const _slug: CelNarrowingCoverageStrict["slug"] = nonEmpty("x");
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
type _EscapedName = Expect<Extends<AmbiguityCoverageStrict["constructor$"], NonEmpty>>;

// `has`/`!has` on a field with no presence is a value test, not a presence one:
// the property protoc-gen-es declares required must stay inhabited.
type _ImplicitPresence = Expect<Extends<AmbiguityCoverageStrict["label"], NonEmpty>>;
type _ImplicitAbsence = Expect<Extends<AmbiguityCoverageStrict["legacyNote"], "">>;

// `has(this.nickname) && this.nickname != ''` keeps both halves: the property is
// required, holds no undefined, and is non-empty.
type _PresenceAndValue = Expect<Extends<AmbiguityCoverageStrict["nickname"], NonEmpty>>;
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
