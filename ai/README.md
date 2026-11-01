# HiringPlug AI worker (Python)

Processes jobs on the `ai` queue: reading CVs (text extraction and OCR) now;
CV parsing, formatting, blind CVs and matching next. See
[docs/ai-worker.md](../docs/ai-worker.md) for the design.

```bash
uv sync                      # install
uv run pytest                # tests (needs PostgreSQL for `db` tests, Tesseract for `ocr` tests)
uv run hiringplug-ai         # run the worker (reads HP_AI_* settings)
```
