"""Go's name rules (backend/internal/platform/redact/name_words.json) are CV
Studio's: the same word lists, so a blind CV's last check in Go and CV
Studio's own sweep agree on what is a name."""

from __future__ import annotations

import json
from pathlib import Path

from hiringplug_ai import cvstudio

GO_WORDS = (
    Path(__file__).resolve().parents[2] / "backend" / "internal" / "platform" / "redact" / "name_words.json"
)


def test_gos_name_words_are_cv_studios() -> None:
    go = json.loads(GO_WORDS.read_text(encoding="utf-8"))
    engine = cvstudio.NAME_RULES
    assert go["minLetters"] == 3
    assert set(go["common"]) == set(engine._BLIND_COMMON_WORD_NAMES)
    assert set(go["places"]) == set(engine._BLIND_PLACE_WORD_NAMES)
    assert set(go["placeholders"]) == set(engine._BLIND_GENERIC_PLACEHOLDER_WORDS)
    assert set(go["particles"]) == set(engine._BLIND_NAME_PARTICLES)
    assert set(go["technology"]) == set(engine._BLIND_TECHNOLOGY_GIVEN_NAMES)
    assert set(go["orgTech"]) == set(engine._BLIND_ORG_TECH_ALLOWLIST)
    assert set(go["honorifics"]) == set(engine._BLIND_HONORIFICS)
    # The length rule, as the engine applies it.
    assert not engine._blind_name_word_is_sweepable("Ng")
    assert engine._blind_name_word_is_sweepable("Tan")
