"""Opening the API keys companies save in Settings > AI provider.

Go seals each key (aiproviders.Seal): AES-256-GCM with a key derived from
HP_AI_KEYS_SECRET by SHA-256, a 12-byte nonce, the provider as associated
data, written "v1." + base64url(nonce || ciphertext || tag). Only this worker
opens them, just before calling the provider; an opened key is never logged
or stored.
"""

from __future__ import annotations

import base64
import binascii
import hashlib

from cryptography.exceptions import InvalidTag
from cryptography.hazmat.primitives.ciphers.aead import AESGCM

_VERSION = "v1."
_NONCE = 12


class SealedKeyError(ValueError):
    """The key could not be opened; the message is safe to show users."""


def open_key(sealed: str, provider: str, secret: str) -> str:
    if not secret:
        raise SealedKeyError("the AI worker cannot open saved AI keys (HP_AI_KEYS_SECRET is not set)")
    if not sealed.startswith(_VERSION):
        raise SealedKeyError("the saved AI key is in a format this worker does not know")
    body = sealed[len(_VERSION) :]
    try:
        raw = base64.urlsafe_b64decode(body + "=" * (-len(body) % 4))
    except (binascii.Error, ValueError) as exc:
        raise SealedKeyError("the saved AI key is damaged: enter it again") from exc
    if len(raw) <= _NONCE:
        raise SealedKeyError("the saved AI key is damaged: enter it again")
    aes = AESGCM(hashlib.sha256(secret.encode()).digest())
    try:
        key = aes.decrypt(raw[:_NONCE], raw[_NONCE:], f"hiringplug-ai-key:{provider}".encode())
    except InvalidTag as exc:
        raise SealedKeyError(
            "the saved AI key cannot be opened (HP_AI_KEYS_SECRET changed since): enter it again"
        ) from exc
    return key.decode()
