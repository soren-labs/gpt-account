from __future__ import annotations

import pytest

from gpa.errors import GPAError
from gpa.lock import StoreLock
from gpa.switcher import locked


def test_lock_blocks_second_holder(store) -> None:
    with locked(store):
        with pytest.raises(GPAError, match="another gpa command"):
            StoreLock(store.lock_path).acquire()
