"""Check that the _strict.py overlays import and name real types."""
import collections.abc as abc
import pathlib
import sys
import typing
sys.path.insert(0, str(pathlib.Path(__file__).parent / "gen" / "python"))

from example.v1.user_strict import User, UserId, UserRoles
from shop.catalog.v1.product_strict import Product, ProductPrice, ProductLabels
from shop.common.v1 import common_pb2

# The message class is re-exported, so one import is enough to build a message.
assert User(id="usr_1").id == "usr_1"

# The alias carries the field type plus its rules as metadata.
assert typing.get_args(UserId)[0] is str
assert "string.min_len = 5" in typing.get_args(UserId)
assert typing.get_args(UserRoles)[0] == abc.Sequence[str]

# A message-typed field resolves to the class the official generator emitted,
# imported across proto files.
assert typing.get_args(ProductPrice)[0] is common_pb2.Money
assert "required" in typing.get_args(ProductPrice)
assert typing.get_args(ProductLabels)[0] == abc.Mapping[str, str]

# The annotation is usable where a type is expected.
def promote(user_id: UserId) -> Product: ...
assert typing.get_type_hints(promote, include_extras=True)["user_id"] is UserId

print("python strict overlays OK")
