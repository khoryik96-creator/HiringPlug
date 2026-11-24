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
# A CV's section headings ("WORK EXPERIENCE", "Education:") say nothing of their own.
_HEADING = re.compile(
    r"^((professional|career|work|employment|personal|key|technical|core|relevant) )?"
    r"(summary|profile|objective|experience|history|employment|education|qualifications?|skills|"
    r"competenc(y|ies)|certifications?|languages?|references?|achievements?|projects?|interests|"
    r"details|particulars|information|background)( (history|summary|details|and \w+))?\s*:?$",
    re.IGNORECASE,
)
TOP_LINES, TOP_WORDS = 4, 5  # a CV's opening short lines: its owner's name, where they live


def _lines(text: str) -> list[str]:
    out = []
    for raw in text.replace("\r", "").split("\n"):
        line = re.sub(r"\s+", " ", _BULLET.sub("", raw)).strip()
        if line and _CONTACT.search(line) and len(line.split()) < 12:
            continue  # an email, a phone number or a link: no meaning
        out.append(line)
    return out


def _work_lines(text: str) -> list[str]:
    """A CV's lines without its headings, nor the short lines it opens with
    (the person's name, where they live), which say nothing of their work."""
    out: list[str] = []
    top = 0
    for line in _lines(text):
        if line and _HEADING.match(line):
            continue
        if line and top < TOP_LINES and not any(out):  # still at the top: nothing kept yet
            top += 1
            if len(line.split()) <= TOP_WORDS and not re.search(r"[\d|]", line):
                continue
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

    for line in _work_lines(text):
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


# Headings of a job description, and what is under them. A heading is a whole
# line ("Job Requirements:"), never the start of a need ("Experience in AutoCount").
_NEEDS_WORD = (
    r"(?:(?:key|job|main|major|primary|core|your|the|minimum|basic|general|essential|preferred|desired|"
    r"candidate|position|role|specific|technical|other|additional|required|work(?:ing)?)\s+)*"
    r"(?:requirements?|qualifications?|responsibilit(?:y|ies)|duties|duty|skills?(?: set)?|experience|"
    r"competenc(?:y|ies)|criteria|scope(?: of work)?|description|specifications?|profile|expectations|"
    r"roles?|preferred|advantages?|essentials?|desirable)"
)
_NEEDS_WORDS = rf"{_NEEDS_WORD}(?:\s*(?:,|&|/|\band\b)\s*{_NEEDS_WORD})*(?:\s+(?:required|needed|wanted))?"
_NEEDS_HEADING = re.compile(
    rf"^[*_#\s]*(?:{_NEEDS_WORDS}|"
    r"what you(?:'|’)ll (?:do|need|bring|be doing)|what you will (?:do|need|bring|be doing)|what you need|"
    r"what we(?:'|’)re looking for|what we are looking for|who we(?:'|’)re looking for|"
    r"who we are looking for|"
    r"what we (?:need|expect)|who you are|about you|(?:about )?the (?:role|job|position)|about the role|"
    r"your role|you will|you(?:'|’)ll|you (?:have|bring|are|need)|must[- ]haves?|nice[- ]to[- ]haves?|"
    r"good to have|(?:the )?ideal candidate|tanggungjawab|kelayakan|syarat(?:-syarat)?(?: kelayakan)?|"
    r"keperluan(?: jawatan)?"
    r")\s*[:：.\-–]?[*_\s]*$",
    re.IGNORECASE,
)
# A heading with its first need on the same line: "Requirements: Degree in Accounting".
_NEEDS_LABEL = re.compile(rf"^[*_#\s]*(?:{_NEEDS_WORDS})\s*[:：]\s*\S", re.IGNORECASE)
_OTHER_HEADING = re.compile(
    r"^[*_#\s]*(?:about (?!(?:the )?(?:role|job|position)\b|you\b)[\w&'’ .-]{1,40}|benefits?|what we offer|"
    r"we offer|our offer|perks|why (?:join|work)\b.*|how to apply|to apply|"
    r"interested (?:candidates|applicants|parties)\b.*|"
    r"our (?:company|culture|story|values|client)|company (?:overview|profile|background|description)|"
    r"who we are|life at\b.*|what(?:'|’)s in it for you|what you(?:'|’)ll get|what you get|"
    r"what we can offer|salary|remuneration|compensation(?: (?:and|&) benefits)?|package|rewards?|"
    r"location|work(?:ing)? (?:hours|days|location)|equal opportunit\w*|(?:job )?highlights|"
    r"(?:additional|other) information|note)\s*[:：.\-–]?[*_\s]*$",
    re.IGNORECASE,
)
_NOT_A_NEED = re.compile(
    r"\b(salary|bonus|allowance|insurance|annual leave|benefits?|apply now|send your|equal opportunit|"
    r"shortlisted candidates|working hours|located at|office in)\b|"
    r"^(location|work(ing)? (days|location)|job type|employment type|start date|closing date)\s*[:：]",
    re.IGNORECASE,
)
HEADING_WORDS = 7  # a heading is a short line


def needs(description: str, limit: int = MAX_NEEDS) -> list[str]:
    """What a job description asks for, line by line: the lines under its
    requirements and responsibilities headings and its bulleted lines (unless
    under a heading such as benefits or about us), in the description's order,
    else its sentences; never benefits, pay or how to apply."""
    lines = _lines(description)
    raw = description.replace("\r", "").split("\n")
    bulleted = {re.sub(r"\s+", " ", _BULLET.sub("", r)).strip() for r in raw if _BULLET.match(r)}
    chosen: list[str] = []
    section = ""  # "needs", "other", or "" (no heading yet, or one that says neither)
    for line in lines:
        if not line:
            continue
        if len(line.split()) <= HEADING_WORDS and (
            _NEEDS_HEADING.match(line)
            or _OTHER_HEADING.match(line)
            or (line.endswith((":", "：")) and line not in bulleted)  # a bullet ending ":" leads its list
        ):
            section = "needs" if _NEEDS_HEADING.match(line) else "other" if _OTHER_HEADING.match(line) else ""
            continue
        if _NEEDS_LABEL.match(line):
            section = "needs"
        if _NOT_A_NEED.search(line) or len(line.split()) < 3:
            continue
        if section == "needs" or (section == "" and line in bulleted):
            chosen.append(line)
    if not chosen:
        chosen = [s for line in lines for s in _SENTENCE.split(line) if len(s.split()) >= 4]
        chosen = [s for s in chosen if not _NOT_A_NEED.search(s)]
    return [line[:600] for line in chosen[:limit]]
