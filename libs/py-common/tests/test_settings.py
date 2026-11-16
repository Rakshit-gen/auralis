from __future__ import annotations

import pytest
from pydantic import ValidationError

from auralis_common.settings import BaseServiceSettings

GOOD = "9f8e7d6c5b4a39281706f5e4d3c2b1a0aa"
DEV = "dev-identity-secret-change-me-0123456789"


def make(**kw):
    base = {"identity_secret": GOOD, "service_shared_token": GOOD, "kafka_brokers": "k:9092"}
    return BaseServiceSettings(_env_file=None, **{**base, **kw})


def test_real_secrets_pass():
    make()


@pytest.mark.parametrize("value", ["short", DEV])
def test_weak_secrets_are_refused(value):
    with pytest.raises(ValidationError):
        make(identity_secret=value)


def test_dev_secrets_allowed_locally():
    make(identity_secret=DEV, allow_dev_secrets=True)
