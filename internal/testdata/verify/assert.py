"""Check that the _strict.py overlays import and name real types."""
import collections.abc as abc
import pathlib
import sys
import typing
sys.path.insert(0, str(pathlib.Path(__file__).parent / "gen" / "python"))

import annotated_types

from example.v1 import user_strict
from example.v1.user_pb2 import User
from example.v1.user_strict import UserId, UserRoles
from shop.catalog.v1.product_pb2 import Product
from shop.catalog.v1.product_strict import ProductPrice, ProductLabels
from shop.coverage.v1.rules_strict import (
    AmbiguityCoverageOutsideWindow,
    CollectionRuleCoverageNames,
    PresenceRuleCoverageNeverChecked,
    StringRuleCoverageExactLen,
)
from shop.common.v1 import common_pb2

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

# A message-typed field resolves to the class the official generator emitted,
# reached through a module alias when it is declared in another proto file.
assert typing.get_args(ProductPrice)[0] is common_pb2.Money
assert "required" in typing.get_args(ProductPrice)
assert typing.get_args(ProductLabels)[0] == abc.Mapping[str, str]

# The annotation is usable where a type is expected.
def promote(user_id: UserId) -> Product: ...
assert typing.get_type_hints(promote, include_extras=True)["user_id"] is UserId

print("python strict overlays OK")
