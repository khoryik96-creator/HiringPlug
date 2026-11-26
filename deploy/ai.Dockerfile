# The Python AI worker, with Tesseract for OCR, LibreOffice for old Word files
# and PDFs, and Node for CV Studio's Word renderer.
FROM python:3.13-slim
RUN apt-get update \
 && apt-get install -y --no-install-recommends tesseract-ocr tesseract-ocr-eng antiword libreoffice-writer-nogui nodejs npm \
 && rm -rf /var/lib/apt/lists/*
COPY --from=ghcr.io/astral-sh/uv:0.11 /uv /usr/local/bin/uv
WORKDIR /app
COPY ai/pyproject.toml ai/uv.lock ai/README.md ./
RUN uv sync --frozen --no-dev --no-install-project
# Search by meaning's model, pinned and checked (SHA-256), part of the image:
# the worker never downloads anything while it runs. Fetched with only the two
# files that name it, so a change to the worker's code does not fetch it again.
ENV HP_AI_MEANING_MODEL_DIR=/app/meaning-model
COPY ai/src/hiringplug_ai/meaning/fetch.py ai/src/hiringplug_ai/meaning/folder.py /tmp/fetch/hiringplug_ai/meaning/
RUN touch /tmp/fetch/hiringplug_ai/__init__.py /tmp/fetch/hiringplug_ai/meaning/__init__.py \
 && PYTHONPATH=/tmp/fetch /app/.venv/bin/python -m hiringplug_ai.meaning.fetch /app/meaning-model \
 && rm -rf /tmp/fetch
COPY ai/src ./src
RUN uv sync --frozen --no-dev
# CV Studio's engine (rules, prompts, renderer and template), unchanged.
COPY ai/cvstudio ./cvstudio
# Which version it runs, reported to Settings > System (heartbeat.py finds it above the package).
COPY VERSION ./VERSION
RUN npm ci --omit=dev --prefix cvstudio/engine
# LibreOffice (old .doc files) needs a writable home.
ENV HOME=/tmp
USER nobody
CMD ["/app/.venv/bin/hiringplug-ai"]
