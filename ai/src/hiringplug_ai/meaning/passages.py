"""Cutting texts into what the meaning model reads: a CV into passages (a
role and what was done in it, a summary), and a job description into the
needs it lists (what the person must have or will do), leaving out the
company's story, benefits and how to apply."""

from __future__ import annotations

import re

PASSAGE_WORDS = 90  # a passage ends at the first line break after this many words
MIN_WORDS = 8  # fewer: a heading or a name, nothing to compare
MAX_CHARS = 1500
MAX_PASSAGES = 16  # the newest work and the summary come first in a CV
MAX_NEEDS = 12

_BULLET = re.compile(r"^\s*(?:[-*•▪●◦‣►▶✓✔·o]|\d{1,2}[.)])\s+")
_CONTACT = re.compile(r"@|https?://|www\.|linkedin\.com/|(?:\+?\d[\d\s().-]{7,}\d)")
_SENTENCE = re.compile(r"(?<=[.;!?])\s+")


def _lines(text: str) -> list[str]:
    out = []
    for raw in text.replace("\r", "").split("\n"):
        line = re.sub(r"\s+", " ", _BULLET.sub("", raw)).strip()
        if line and _CONTACT.search(line) and len(line.split()) < 12:
            continue  # an email, a phone number or a link: no meaning
        out.append(line)
    return out


def _cut(words: list[str], size: int) -> list[str]:
    return [" ".join(words[i : i + size]) for i in range(0, len(words), size)]


def passages(text: str, limit: int = MAX_PASSAGES) -> list[str]:
    """A CV's text in passages of about PASSAGE_WORDS words, cut at line
    breaks (a long line at sentences), in the CV's order."""
    out: list[str] = []
    current: list[str] = []

    def close() -> None:
        joined = " ".join(current).strip()
        current.clear()
        if len(joined.split()) >= MIN_WORDS:
            out.append(joined[:MAX_CHARS])

    for line in _lines(text):
        if not line:  # a blank line closes a passage that has enough
            if sum(len(x.split()) for x in current) >= PASSAGE_WORDS // 3:
                close()
            continue
        pieces = [line]
        if len(line.split()) > PASSAGE_WORDS:
            pieces = [p for s in _SENTENCE.split(line) for p in _cut(s.split(), PASSAGE_WORDS)]
        for piece in pieces:
            if current and sum(len(x.split()) for x in current) + len(piece.split()) > PASSAGE_WORDS * 1.3:
                close()
            current.append(piece)
            if sum(len(x.split()) for x in current) >= PASSAGE_WORDS:
                close()
        if len(out) >= limit:
            break
    close()
    return out[:limit]


# Headings of a job description, and what is under them.
_NEEDS_HEADING = re.compile(
    r"^(key )?(requirements?|qualifications?|responsibilit(y|ies)|duties|what you('|’)ll do|what you will do|"
    r"what we('|’)re looking for|what we are looking for|you (will|have|bring)|skills|experience|must have|"
    r"the role|job (description|scope|duties)|about the role|your role|criteria|competenc(y|ies))\b",
    re.IGNORECASE,
)
_OTHER_HEADING = re.compile(
    r"^(about (us|the company)|benefits|what we offer|perks|why join|how to apply|"
    r"our (company|culture|story)|"
    r"salary|remuneration|location|working hours|equal opportunit)",
    re.IGNORECASE,
)
_NOT_A_NEED = re.compile(
    r"\b(salary|bonus|allowance|insurance|annual leave|benefits?|apply now|send your|equal opportunit|"
    r"shortlisted candidates|working hours|located at|office in)\b",
    re.IGNORECASE,
)


def needs(description: str, limit: int = MAX_NEEDS) -> list[str]:
    """What a job description asks for, line by line: the lines under its
    requirements and responsibilities headings, else its bulleted lines,
    else its sentences; never benefits, pay or how to apply."""
    lines = _lines(description)
    under, bullets, inside = [], [], False
    raw = description.replace("\r", "").split("\n")
    bulleted = {re.sub(r"\s+", " ", _BULLET.sub("", r)).strip() for r in raw if _BULLET.match(r)}
    for line in lines:
        if not line:
            continue
        short = len(line.split()) <= 6
        if short and _NEEDS_HEADING.match(line):
            inside = True
            continue
        if short and (_OTHER_HEADING.match(line) or line.endswith(":")):
            inside = bool(_NEEDS_HEADING.match(line))
            continue
        if _NOT_A_NEED.search(line) or len(line.split()) < 3:
            continue
        if inside:
            under.append(line)
        elif line in bulleted:
            bullets.append(line)
    chosen = under or bullets
    if not chosen:
        chosen = [s for line in lines for s in _SENTENCE.split(line) if len(s.split()) >= 4]
        chosen = [s for s in chosen if not _NOT_A_NEED.search(s)]
    return [line[:600] for line in chosen[:limit]]
