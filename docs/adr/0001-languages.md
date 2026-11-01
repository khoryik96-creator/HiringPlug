# 0001 Go + Python + TypeScript

**Decision.** The backend is Go (API, business rules, database, core jobs) and
Python (documents and AI), and the web app is TypeScript (React).

**Why.**
- **Go** for the core: fast, low on memory, strict compiler, very stable
  language and libraries, single-binary deploys, and a built-in race detector.
  An ATS's core must not break, and Go's simplicity keeps code reviewable,
  including code written by AI agents.
- **Python** for OCR, document parsing and AI: it has by far the best libraries
  (Tesseract bindings, pdfplumber, PDFium, python-docx, ML tooling), and your
  existing CV Studio logic is in Python. AI work changes fastest and is least
  predictable, so it is isolated behind the job queue.
- **TypeScript** because anything interactive in a browser is written in
  JavaScript or TypeScript in practice; React has the components an ATS needs
  (boards, forms, tables).

**Costs and how they are handled.** Three toolchains instead of one. The
contract generates types for all three, so they cannot drift; one `task verify`
runs every check; and each language's role is narrow: Go owns rules and data,
Python only computes, and the web app only displays and submits.

**Alternatives.** TypeScript everywhere (one language, but weaker for OCR/ML
and you prefer Go for the core); C#/.NET or Java (also strong, more ceremony);
Go-only server-rendered pages (htmx), which would be harder for drag-and-drop
boards and rich search.
