"""Check that the _strict.py overlays import and name real types."""
import collections.abc as abc
import pathlib
import sys
import typing
sys.path.insert(0, str(pathlib.Path(__file__).parent / "gen" / "python"))

import annotated_types
from google.protobuf import timestamp_pb2

from example.v1 import user_strict
from example.v1.user_pb2 import User
from example.v1.user_strict import UserId, UserRoles
from shop.catalog.v1.product_pb2 import Product
from shop.catalog.v1.product_strict import (
    CreateProductRequestOutputOnlyFields,
    ProductCreatedAt,
    ProductLabels,
    ProductOutputOnlyFields,
    ProductPrice,
)
from shop.coverage.v1.rules_strict import (
    AmbiguityCoverageOutsideWindow,
    CollectionRuleCoverageNames,
    PresenceRuleCoverageNeverChecked,
    StringRuleCoverageExactLen,
)
from shop.common.v1 import common_pb2
from shop.common.v1.common_pb2 import Currency
from shop.common.v1.common_strict import CurrencyStrict, MoneyCurrency

# The overlay is annotations only. The message classes stay in the module
# protoc-gen-python wrote them in, so nothing here shadows them.
assert not hasattr(user_strict, "User")
assert User(id="usr_1").id == "usr_1"

# The alias carries the field type plus its rules as metadata.
assert typing.get_args(UserId)[0] is str
assert typing.get_args(UserRoles)[0] == abc.Sequence[str]

# A rule annotated_types can express is carried as that constructor, so anything
# reading the vocabulary enforces it; the rest stay strings.
assert annotated_types.MinLen(5) in typing.get_args(UserId)
assert annotated_types.MaxLen(64) in typing.get_args(UserId)
assert "string.pattern = ^[a-z]+(_[a-z]+)*$" not in typing.get_args(UserId)

# An exact length is the two bounds at the same value.
assert annotated_types.MinLen(10) in typing.get_args(StringRuleCoverageExactLen)
assert annotated_types.MaxLen(10) in typing.get_args(StringRuleCoverageExactLen)

# A collection's own bounds are carried; a rule on its elements is not, because
# MinLen would then count the wrong thing.
assert annotated_types.MinLen(1) in typing.get_args(CollectionRuleCoverageNames)
assert "repeated.items.string.min_len = 1" in typing.get_args(CollectionRuleCoverageNames)

# `ignore` switches the sibling rules off, and a reversed range means "outside
# it" — neither survives as a constraint.
assert "ignore = IGNORE_ALWAYS" in typing.get_args(PresenceRuleCoverageNeverChecked)
assert not any(
    isinstance(meta, annotated_types.BaseMetadata)
    for meta in typing.get_args(AmbiguityCoverageOutsideWindow)
)

# Every enum gets a strict alias over the class protoc-gen-python emitted. The
# generated class subclasses int, so Ge(1) is the same set as "not the zero
# member" — no buf.validate rule says so, the naming convention does.
assert typing.get_args(CurrencyStrict)[0] is Currency
assert annotated_types.Ge(1) in typing.get_args(CurrencyStrict)

# A field of that type is annotated with the alias, not the bare class. Python
# flattens a nested Annotated, so the field alias ends up carrying Ge(1) itself.
assert typing.get_args(MoneyCurrency)[0] is Currency
assert annotated_types.Ge(1) in typing.get_args(MoneyCurrency)

# A message-typed field resolves to the class the official generator emitted,
# reached through a module alias when it is declared in another proto file.
assert typing.get_args(ProductPrice)[0] is common_pb2.Money
assert "required" in typing.get_args(ProductPrice)
assert typing.get_args(ProductLabels)[0] == abc.Mapping[str, str]

# (google.api.field_behavior) = OUTPUT_ONLY is carried as metadata text:
# annotated_types has no vocabulary for a field only the server writes. It is
# the only annotation here that gives an otherwise ruleless field an alias.
assert typing.get_args(ProductCreatedAt)[0] is timestamp_pb2.Timestamp
assert "google.api.field_behavior = OUTPUT_ONLY" in typing.get_args(ProductCreatedAt)

# The same fields are listed apart from the aliases, under the proto names the
# descriptor and a google.protobuf.FieldMask use.
assert "created_at" in ProductOutputOnlyFields
assert "createdAt" not in ProductOutputOnlyFields


# A field under an OUTPUT_ONLY one is server-assigned too, so the list carries
# the whole subtree as dotted paths, and every one of them resolves against the
# descriptor a FieldMask would be applied to.
def resolves(descriptor, path: str) -> bool:
    head, _, rest = path.partition(".")
    field = descriptor.fields_by_name.get(head)
    if field is None:
        return False
    return not rest or resolves(field.message_type, rest)


assert "created_at.nanos" in ProductOutputOnlyFields
assert all(resolves(Product.DESCRIPTOR, path) for path in ProductOutputOnlyFields)

# CreateProductRequest declares no OUTPUT_ONLY field of its own: every path it
# has is one the Product it wraps reaches.
assert set(CreateProductRequestOutputOnlyFields) == {
    "product." + path for path in ProductOutputOnlyFields
}

# The annotation is usable where a type is expected.
def promote(user_id: UserId) -> Product: ...
assert typing.get_type_hints(promote, include_extras=True)["user_id"] is UserId

print("python strict overlays OK")
