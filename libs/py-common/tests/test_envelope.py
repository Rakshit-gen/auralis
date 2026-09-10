"""Envelope decode/validate parity with the Go platform envelope.Validate()."""

from __future__ import annotations

import json

import pytest
from pydantic import ValidationError

from auralis_common.envelope import Envelope, new_envelope


def _wire(**overrides) -> bytes:
    base = {
        "event_id": "3b1e4c7a-0000-4000-8000-000000000001",
        "event_type": "user.registered",
        "event_version": 1,
        "occurred_at": "2026-01-01T00:00:00Z",
        "producer": "user",
        "correlation_id": "c-1",
        "causation_id": "",
        "payload": {"user_id": "u1"},
    }
    base.update(overrides)
    return json.dumps(base).encode()


def test_valid_envelope_round_trips():
    env = Envelope.from_bytes(_wire())
    assert env.event_type == "user.registered"
    assert Envelope.from_bytes(env.to_bytes()).payload == {"user_id": "u1"}


@pytest.mark.parametrize(
    "bad",
    [
        {"event_type": ""},
        {"event_version": 0},
        {"producer": ""},
        {"payload": {}},
    ],
)
def test_go_invalid_envelopes_are_rejected(bad):
    with pytest.raises(ValidationError):
        Envelope.from_bytes(_wire(**bad))


def test_new_envelope_is_valid():
    env = new_envelope("content.show_published", {"show_id": "s1"}, producer="content")
    assert env.event_version == 1
