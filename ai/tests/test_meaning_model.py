"""The meaning model reads texts in batches of about the same length (a
batch is padded to its longest): each text reads the same however it is
batched, and comes back in the order asked for."""

from __future__ import annotations

import os

import numpy as np
import pytest

from hiringplug_ai.meaning import model as meaning_model


@pytest.fixture(scope="module")
def model() -> meaning_model.MeaningModel:
    if os.environ.get("HP_SKIP_MEANING_TESTS") == "1":
        pytest.skip("HP_SKIP_MEANING_TESTS=1")
    loaded = meaning_model.load()
    if loaded is None:
        pytest.fail("the meaning model is not installed: run `task setup` (or set HP_SKIP_MEANING_TESTS=1)")
    return loaded


def test_texts_come_back_in_the_order_asked_whatever_their_length(model: meaning_model.MeaningModel) -> None:
    texts = [
        "Accountant",
        "Ran full sets of accounts for twelve SME clients, month-end closing and audits " * 4,
        "",
        "Recruiter",
        "Hired 120 engineers a year for a bank",
    ] * 9  # more than one batch
    together = model.embed(texts)
    assert together.shape == (len(texts), meaning_model.DIMENSIONS)
    for i, text in enumerate(texts[:5]):
        alone = model.embed([text])[0]
        assert np.allclose(together[i], alone, atol=1e-4), text[:20]
    assert np.allclose(together[0], together[5]), "the same text, the same reading"
